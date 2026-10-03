package app

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"multisnekkvm/internal/protocol"
)

// Send mux: single writer goroutine with four priority lanes.
//
//	muxMouse – mouse-move frames, coalescing / drop-old                    cap 2
//	muxHigh  – reliable control frames (keys, clicks, pong, etc.)          cap 128
//	muxAudio – audio/mic stream frames, ordered, drop-oldest-data          cap 128
//	muxFile  – file data chunks, backpressure on producer                  cap 8
//
// Priority is mouse > high > audio > file. The mux drains up to muxBurst
// mouse/high frames, then up to muxAudioBurst audio frames (re-checking the
// input lanes after each one), then allows one file chunk, giving input
// priority over media and bulk transfers while still making progress.
const (
	muxHighCap    = 128
	muxMouseCap   = 2
	muxAudioCap   = 128
	muxFileCap    = 8
	muxBurst      = 16
	muxAudioBurst = 4

	muxStatsInterval     = 5 * time.Second
	muxSlowWriteMs       = 10   // log warn if sendFrame takes longer than this
	muxStallMs           = 500  // log warn if mux hasn't sent for this long while queued
	muxForceDisconnectMs = 8000 // force session close if input frames are stuck this long (> write deadline)
	// The peer's read deadline is 15s. Checking every 2s and sending a
	// heartbeat once nothing was sent for 4s bounds the idle gap to ~6s.
	muxHeartbeatTick = 2 * time.Second
	muxHeartbeatIdle = 4 * time.Second
	muxErrLogEvery   = 5 * time.Second
)

// audioLane is an ordered FIFO for audio and mic stream frames. It never
// blocks the producer: when full, the oldest queued data frame is dropped.
// Transport/format frames are never dropped; they may exceed the capacity,
// which can only happen if the lane is somehow full of control frames.
type audioLane struct {
	mu  sync.Mutex
	q   []Frame
	sig chan struct{} // cap 1; signalled on push to wake the idle mux
}

func isAudioDataFrame(t byte) bool {
	return t == protocol.MsgAudioData || t == protocol.MsgMicData
}

// isAudioLaneFrame reports whether t travels in the audio lane. Start/Stop
// requests are control messages and stay in the high lane.
func isAudioLaneFrame(t byte) bool {
	switch t {
	case protocol.MsgAudioTransport, protocol.MsgAudioFormat, protocol.MsgAudioData,
		protocol.MsgMicTransport, protocol.MsgMicFormat, protocol.MsgMicData:
		return true
	}
	return false
}

// push appends f and returns the number of frames dropped to make room.
func (l *audioLane) push(f Frame) (dropped int) {
	l.mu.Lock()
	if len(l.q) >= muxAudioCap {
		idx := -1
		for i := range l.q {
			if isAudioDataFrame(l.q[i].Type) {
				idx = i
				break
			}
		}
		switch {
		case idx >= 0:
			copy(l.q[idx:], l.q[idx+1:])
			l.q[len(l.q)-1] = Frame{}
			l.q = l.q[:len(l.q)-1]
			dropped = 1
		case isAudioDataFrame(f.Type):
			// The lane holds only control frames: the incoming data frame
			// is the oldest droppable one.
			l.mu.Unlock()
			return 1
		}
	}
	l.q = append(l.q, f)
	l.mu.Unlock()
	if l.sig != nil {
		select {
		case l.sig <- struct{}{}:
		default:
		}
	}
	return dropped
}

func (l *audioLane) pop() (Frame, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.q) == 0 {
		return Frame{}, false
	}
	f := l.q[0]
	l.q[0] = Frame{}
	l.q = l.q[1:]
	if len(l.q) == 0 {
		l.q = nil // release the backing array once drained
	}
	return f, true
}

func (l *audioLane) len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.q)
}

func (l *audioLane) clear() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.q)
	l.q = nil
	return n
}

// initSendMux creates the lanes. The writer goroutines are started by
// startSendMux once a.transport has been assigned.
func (a *App) initSendMux() {
	if a.muxHigh != nil {
		return
	}
	a.muxHigh = make(chan Frame, muxHighCap)
	a.muxMouse = make(chan Frame, muxMouseCap)
	a.muxFile = make(chan Frame, muxFileCap)
	a.muxAudio.sig = make(chan struct{}, 1)
}

// startSendMux launches the mux goroutines. It must run after a.transport
// is assigned so the goroutines never observe it changing.
func (a *App) startSendMux() {
	SafeGo("send-mux", func() { a.muxLoop() })
	SafeGo("send-mux-stats", func() { a.muxStatsLoop() })
	SafeGo("send-mux-stall", func() { a.muxStallWatchdog() })
}

// muxLoop is the sole goroutine that writes to the transport.
// It also owns heartbeat delivery: by sending MsgHeartbeat directly from the
// idle select rather than from a competing goroutine, we eliminate the second
// concurrent writer (the old heartbeatLoop) and the associated mutex contention.
func (a *App) muxLoop() {
	hb := time.NewTicker(muxHeartbeatTick)
	defer hb.Stop()

	for {
		progressed := a.drainHighPriority() > 0

		// Audio after input; yield back to input as soon as any is queued.
		for i := 0; i < muxAudioBurst; i++ {
			f, ok := a.muxAudio.pop()
			if !ok {
				break
			}
			a.muxSend(f, "audio")
			progressed = true
			if len(a.muxMouse) > 0 || len(a.muxHigh) > 0 {
				break
			}
		}

		// One file chunk per cycle (weighted fairness).
		select {
		case f := <-a.muxFile:
			a.muxSend(f, "file")
			progressed = true
		default:
		}

		if progressed {
			continue
		}

		// All lanes empty — block until something arrives or heartbeat fires.
		select {
		case f := <-a.muxMouse:
			a.muxSend(f, "mouse")
		case f := <-a.muxHigh:
			a.muxSend(f, "high")
		case <-a.muxAudio.sig:
			// Audio queued; the next iteration sends it after any input.
		case f := <-a.muxFile:
			a.muxSend(f, "file")
		case <-hb.C:
			// Only send if no frame was recently sent; any data frame already
			// resets the peer's read deadline so a heartbeat would be redundant.
			lastNs := atomic.LoadInt64(&a.muxLastSentNs)
			if lastNs == 0 || time.Since(time.Unix(0, lastNs)) >= muxHeartbeatIdle {
				a.muxSend(Frame{Type: protocol.MsgHeartbeat}, "heartbeat")
			}
		case <-a.ctx.Done():
			return
		}
	}
}

// muxSend calls sendFrame and records timing + counters.
func (a *App) muxSend(f Frame, lane string) {
	if f.Type == protocol.MsgPing {
		if len(f.Payload) == 8 {
			binary.BigEndian.PutUint64(f.Payload, uint64(time.Now().UnixNano()))
		} else {
			f.Payload = protocol.PingMsg{TimestampNano: uint64(time.Now().UnixNano())}.Encode()
		}
	}
	start := time.Now()
	err := a.sendFrame(f)
	elapsed := time.Since(start)

	if err != nil {
		a.logMuxSendError(f, lane, err)
	} else {
		// Only successful writes count as "sent" for heartbeat/stall logic.
		atomic.StoreInt64(&a.muxLastSentNs, start.UnixNano())
		switch lane {
		case "high":
			atomic.AddUint64(&a.muxHighSentN, 1)
		case "mouse":
			atomic.AddUint64(&a.muxMouseSentN, 1)
		case "audio":
			atomic.AddUint64(&a.muxAudioSentN, 1)
		case "file":
			atomic.AddUint64(&a.muxFileSentN, 1)
		case "heartbeat":
			// Not counted in per-lane stats — heartbeats are keepalives, not data.
		}
	}

	if ms := elapsed.Milliseconds(); ms >= muxSlowWriteMs {
		log.Printf("send-mux: slow write lane=%s type=0x%02x took=%dms queue=H:%d/M:%d/A:%d/F:%d",
			lane, f.Type, ms,
			len(a.muxHigh), len(a.muxMouse), a.muxAudio.len(), len(a.muxFile))
	}
}

// logMuxSendError logs send failures at most once per muxErrLogEvery and
// reports how many were suppressed. Only the mux goroutine calls it.
func (a *App) logMuxSendError(f Frame, lane string, err error) {
	a.muxErrSuppressed++
	now := time.Now()
	if now.Sub(a.muxErrLoggedAt) < muxErrLogEvery {
		return
	}
	suppressed := a.muxErrSuppressed - 1
	a.muxErrSuppressed = 0
	a.muxErrLoggedAt = now
	if suppressed > 0 {
		log.Printf("send-mux: send failed lane=%s type=0x%02x: %v (%d similar suppressed)", lane, f.Type, err, suppressed)
		return
	}
	log.Printf("send-mux: send failed lane=%s type=0x%02x: %v", lane, f.Type, err)
}

// drainHighPriority sends up to muxBurst mouse/high frames and returns
// the number sent.
//
// Mouse moves are given first pick in each iteration so continuous control
// frames cannot delay input delivery by more than one frame.
func (a *App) drainHighPriority() int {
	sent := 0
	for sent < muxBurst {
		// Mouse first: flush any pending move before the next high-priority frame.
		select {
		case f := <-a.muxMouse:
			a.muxSend(f, "mouse")
			sent++
		default:
		}
		// Drain one high-priority frame; exit when the lane is exhausted.
		select {
		case f := <-a.muxHigh:
			a.muxSend(f, "high")
			sent++
		default:
			return sent
		}
	}
	return sent
}

// enqueueSend routes a frame to the correct mux lane.
// Mouse moves coalesce by accumulating deltas (no displacement lost).
// Audio/mic stream frames go to the ordered audio lane (never blocks).
// File chunks block the producer for backpressure.
// Everything else is non-blocking high-priority.
func (a *App) enqueueSend(f Frame) {
	switch {
	case f.Type == protocol.MsgMouseMove:
		a.enqueueMouseMove(f)

	case isAudioLaneFrame(f.Type):
		if dropped := a.muxAudio.push(f); dropped > 0 {
			atomic.AddUint64(&a.muxAudioDroppedN, uint64(dropped))
		}

	case f.Type == protocol.MsgFileChunk:
		// Block the file-send goroutine when the lane is full (backpressure).
		select {
		case a.muxFile <- f:
		case <-a.ctx.Done():
		}

	default:
		select {
		case a.muxHigh <- f:
		default:
			atomic.AddUint64(&a.muxHighDroppedN, 1)
			log.Printf("send-mux: high lane full, dropping frame 0x%02x queue=%d", f.Type, len(a.muxHigh))
			// A lost key/button transition or switch-back is unsafe: terminate
			// the session so both peers release their held input state.
			switch f.Type {
			case protocol.MsgKeyDown, protocol.MsgKeyUp, protocol.MsgMouseClick,
				protocol.MsgSwitchBack, protocol.MsgUnicodeText:
				if a.transport != nil {
					if session := a.transport.GetSession(); session != nil {
						session.Close()
					}
				}
			}
		}
	}
}

// enqueueMouseMove queues a mouse move, coalescing with queued moves when
// the lane is full. Mouse moves come from the single WH_MOUSE_LL hook
// thread, so the drain below is race-free.
func (a *App) enqueueMouseMove(f Frame) {
	select {
	case a.muxMouse <- f:
		return
	default:
	}
	if len(f.Payload) != 8 {
		return // malformed; never produced by the hook
	}
	// Channel full. Drain all queued moves and accumulate their DX/DY so
	// total cursor displacement is preserved.
	dx := int32(binary.BigEndian.Uint32(f.Payload[0:4]))
	dy := int32(binary.BigEndian.Uint32(f.Payload[4:8]))
drain:
	for {
		select {
		case old := <-a.muxMouse:
			if len(old.Payload) == 8 {
				dx += int32(binary.BigEndian.Uint32(old.Payload[0:4]))
				dy += int32(binary.BigEndian.Uint32(old.Payload[4:8]))
			}
			atomic.AddUint64(&a.muxMouseCoalescedN, 1)
		default:
			break drain
		}
	}
	// If opposite moves cancel to zero, no net displacement occurred —
	// discard rather than sending (0,0) which is the remote-wake sentinel.
	if dx == 0 && dy == 0 {
		return
	}
	// The hook allocates a fresh payload per frame, so it is reused in place
	// instead of allocating a new one.
	binary.BigEndian.PutUint32(f.Payload[0:4], uint32(dx))
	binary.BigEndian.PutUint32(f.Payload[4:8], uint32(dy))
	select {
	case a.muxMouse <- f:
	default:
	}
}

// drainSendMux discards all queued frames. Call on session connect/disconnect
// to prevent stale frames leaking into a new session.
func (a *App) drainSendMux() {
	n := a.muxAudio.clear()
	for {
		select {
		case <-a.muxHigh:
			n++
		case <-a.muxMouse:
			n++
		case <-a.muxFile:
			n++
		default:
			if n > 0 {
				log.Printf("send-mux: drained %d stale frame(s) on session reset", n)
			}
			return
		}
	}
}

// muxStatsLoop logs outbound throughput every muxStatsInterval, but only
// when something changed since the previous report or frames are queued.
func (a *App) muxStatsLoop() {
	ticker := time.NewTicker(muxStatsInterval)
	defer ticker.Stop()

	var prevHigh, prevMouse, prevAudio, prevFile, prevDropped, prevAudioDropped, prevCoalesced uint64

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			high := atomic.LoadUint64(&a.muxHighSentN)
			mouse := atomic.LoadUint64(&a.muxMouseSentN)
			audio := atomic.LoadUint64(&a.muxAudioSentN)
			file := atomic.LoadUint64(&a.muxFileSentN)
			dropped := atomic.LoadUint64(&a.muxHighDroppedN)
			audioDropped := atomic.LoadUint64(&a.muxAudioDroppedN)
			coalesced := atomic.LoadUint64(&a.muxMouseCoalescedN)
			qH, qM, qA, qF := len(a.muxHigh), len(a.muxMouse), a.muxAudio.len(), len(a.muxFile)

			changed := high != prevHigh || mouse != prevMouse || audio != prevAudio || file != prevFile ||
				dropped != prevDropped || audioDropped != prevAudioDropped || coalesced != prevCoalesced
			if !changed && qH == 0 && qM == 0 && qA == 0 && qF == 0 {
				continue
			}

			secs := muxStatsInterval.Seconds()
			log.Printf("send-mux stats: high=%.0f/s mouse=%.0f/s audio=%.0f/s file=%.0f/s | queue H:%d/M:%d/A:%d/F:%d | dropped=%d audioDropped=%d coalesced=%d",
				float64(high-prevHigh)/secs,
				float64(mouse-prevMouse)/secs,
				float64(audio-prevAudio)/secs,
				float64(file-prevFile)/secs,
				qH, qM, qA, qF,
				dropped, audioDropped, coalesced,
			)
			prevHigh, prevMouse, prevAudio, prevFile = high, mouse, audio, file
			prevDropped, prevAudioDropped, prevCoalesced = dropped, audioDropped, coalesced
		}
	}
}

// muxStallWatchdog warns if the mux has queued frames but hasn't sent
// anything for longer than muxStallMs. If input frames (high/mouse) are
// stuck for longer than muxForceDisconnectMs, the session is force-closed
// to trigger the auto-reconnect path.
func (a *App) muxStallWatchdog() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var warnedAt int64
	var forcedAt int64

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			queuedAny := len(a.muxHigh) > 0 || len(a.muxMouse) > 0 || len(a.muxFile) > 0 || a.muxAudio.len() > 0
			if !queuedAny {
				continue
			}
			lastNs := atomic.LoadInt64(&a.muxLastSentNs)
			if lastNs == 0 {
				continue
			}
			idleMs := time.Since(time.Unix(0, lastNs)).Milliseconds()

			if idleMs >= muxStallMs && lastNs != warnedAt {
				warnedAt = lastNs
				log.Printf("send-mux STALL: no send for %dms with frames queued H:%d/M:%d/A:%d/F:%d",
					idleMs, len(a.muxHigh), len(a.muxMouse), a.muxAudio.len(), len(a.muxFile))
				log.Printf("send-mux STALL: transport=%v session=%v",
					a.transport != nil,
					func() string {
						if a.transport == nil {
							return "nil"
						}
						if s := a.transport.GetSession(); s != nil {
							return fmt.Sprintf("connected peer=%s", s.PeerName)
						}
						return "disconnected"
					}(),
				)
			}

			// If high-priority input frames are stuck well past the write deadline,
			// force-close the session to unblock the mux and trigger reconnect.
			inputStuck := len(a.muxHigh) > 0 || len(a.muxMouse) > 0
			if inputStuck && idleMs >= muxForceDisconnectMs && lastNs != forcedAt && a.transport != nil {
				forcedAt = lastNs
				if s := a.transport.GetSession(); s != nil {
					log.Printf("send-mux STALL CRITICAL: %dms stall with input frames queued, forcing session disconnect to recover",
						idleMs)
					s.Close()
				}
			}
		}
	}
}
