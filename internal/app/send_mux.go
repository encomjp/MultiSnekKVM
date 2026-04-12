package app

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"multisnekkvm/internal/protocol"
)

// Send mux: single writer goroutine with three priority lanes.
//
//	muxHigh  – reliable control frames (keys, clicks, audio, pong, etc.)  cap 128
//	muxMouse – mouse-move frames, coalescing / drop-old                    cap 2
//	muxFile  – file data chunks, backpressure on producer                  cap 8
//
// The mux drains up to muxBurst high/mouse frames before allowing one file
// chunk, giving input priority over bulk transfers while still making progress.
const (
	muxHighCap  = 128
	muxMouseCap = 2
	muxFileCap  = 8
	muxBurst    = 16

	muxStatsInterval     = 5 * time.Second
	muxSlowWriteMs       = 10              // log warn if sendFrame takes longer than this
	muxStallMs           = 500             // log warn if mux hasn't sent for this long while queued
	muxForceDisconnectMs = 8000            // force session close if input frames are stuck this long (> write deadline)
	muxHeartbeatInterval = 5 * time.Second // must match transport heartbeatTimeout / 3
)

func (a *App) initSendMux() {
	if a.muxHigh != nil {
		return
	}
	a.muxHigh = make(chan Frame, muxHighCap)
	a.muxMouse = make(chan Frame, muxMouseCap)
	a.muxFile = make(chan Frame, muxFileCap)
	SafeGo("send-mux", func() { a.muxLoop() })
	SafeGo("send-mux-stats", func() { a.muxStatsLoop() })
	SafeGo("send-mux-stall", func() { a.muxStallWatchdog() })
}

// muxLoop is the sole goroutine that writes to the transport.
// It also owns heartbeat delivery: by sending MsgHeartbeat directly from the
// idle select rather than from a competing goroutine, we eliminate the second
// concurrent writer (the old heartbeatLoop) and the associated mutex contention.
func (a *App) muxLoop() {
	hb := time.NewTicker(muxHeartbeatInterval)
	defer hb.Stop()

	for {
		burst := a.drainHighPriority()

		// One file chunk per burst cycle (weighted fairness).
		select {
		case f := <-a.muxFile:
			a.muxSend(f, "file")
		default:
		}

		if burst > 0 {
			continue
		}

		// All lanes empty — block until something arrives or heartbeat fires.
		select {
		case f := <-a.muxHigh:
			a.muxSend(f, "high")
		case f := <-a.muxMouse:
			a.muxSend(f, "mouse")
		case f := <-a.muxFile:
			a.muxSend(f, "file")
		case <-hb.C:
			// Only send if no data frame was recently sent; any data frame already
			// resets the peer's read deadline so a heartbeat would be redundant.
			lastNs := atomic.LoadInt64(&a.muxLastSentNs)
			if lastNs == 0 || time.Since(time.Unix(0, lastNs)) >= muxHeartbeatInterval {
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
	_ = a.sendFrame(f)
	elapsed := time.Since(start)

	atomic.StoreInt64(&a.muxLastSentNs, start.UnixNano())

	switch lane {
	case "high":
		atomic.AddUint64(&a.muxHighSentN, 1)
	case "mouse":
		atomic.AddUint64(&a.muxMouseSentN, 1)
	case "file":
		atomic.AddUint64(&a.muxFileSentN, 1)
	case "heartbeat":
		// Not counted in per-lane stats — heartbeats are keepalives, not data.
	}

	if ms := elapsed.Milliseconds(); ms >= muxSlowWriteMs {
		log.Printf("send-mux: slow write lane=%s type=0x%02x took=%dms queue=H:%d/M:%d/F:%d",
			lane, f.Type, ms,
			len(a.muxHigh), len(a.muxMouse), len(a.muxFile))
	}
}

// drainHighPriority sends up to muxBurst high-priority frames and returns
// the number sent.
//
// Mouse moves are given first pick in each iteration so continuous audio or
// control frames cannot delay input delivery by more than one frame.
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
// File chunks block the producer for backpressure.
// Everything else is non-blocking high-priority.
func (a *App) enqueueSend(f Frame) {
	switch f.Type {
	case protocol.MsgMouseMove:
		select {
		case a.muxMouse <- f:
		default:
			// Channel full. Drain all queued moves and accumulate their DX/DY
			// into f so total cursor displacement is preserved. Mouse moves come
			// from the single WH_MOUSE_LL hook thread so this drain is safe.
			m, _ := protocol.DecodeMouseMove(f.Payload)
		drain:
			for {
				select {
				case old := <-a.muxMouse:
					o, _ := protocol.DecodeMouseMove(old.Payload)
					m.DX += o.DX
					m.DY += o.DY
					atomic.AddUint64(&a.muxMouseCoalescedN, 1)
				default:
					break drain
				}
			}
			// If opposite moves cancel to zero, no net displacement occurred —
			// discard rather than sending (0,0) which is the remote-wake sentinel.
			if m.DX == 0 && m.DY == 0 {
				break
			}
			select {
			case a.muxMouse <- Frame{Type: protocol.MsgMouseMove, Payload: m.Encode()}:
			default:
			}
		}

	case protocol.MsgFileChunk:
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
		}
	}
}

// drainSendMux discards all queued frames. Call on session connect/disconnect
// to prevent stale frames leaking into a new session.
func (a *App) drainSendMux() {
	n := 0
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

// muxStatsLoop logs outbound throughput every muxStatsInterval.
func (a *App) muxStatsLoop() {
	ticker := time.NewTicker(muxStatsInterval)
	defer ticker.Stop()

	var prevHigh, prevMouse, prevFile uint64

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			high := atomic.LoadUint64(&a.muxHighSentN)
			mouse := atomic.LoadUint64(&a.muxMouseSentN)
			file := atomic.LoadUint64(&a.muxFileSentN)
			dropped := atomic.LoadUint64(&a.muxHighDroppedN)
			coalesced := atomic.LoadUint64(&a.muxMouseCoalescedN)

			dHigh := high - prevHigh
			dMouse := mouse - prevMouse
			dFile := file - prevFile
			prevHigh, prevMouse, prevFile = high, mouse, file

			secs := muxStatsInterval.Seconds()
			log.Printf("send-mux stats: high=%.0f/s mouse=%.0f/s file=%.0f/s | queue H:%d/M:%d/F:%d | dropped=%d coalesced=%d",
				float64(dHigh)/secs,
				float64(dMouse)/secs,
				float64(dFile)/secs,
				len(a.muxHigh), len(a.muxMouse), len(a.muxFile),
				dropped, coalesced,
			)
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
			queuedAny := len(a.muxHigh) > 0 || len(a.muxMouse) > 0 || len(a.muxFile) > 0
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
				log.Printf("send-mux STALL: no send for %dms with frames queued H:%d/M:%d/F:%d",
					idleMs, len(a.muxHigh), len(a.muxMouse), len(a.muxFile))
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
			if inputStuck && idleMs >= muxForceDisconnectMs && lastNs != forcedAt {
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
