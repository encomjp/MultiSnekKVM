package app

import (
	"encoding/binary"
	"log"
	"sync/atomic"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"multisnekkvm/internal/input"
	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/protocol"
)

func (a *App) handleFrame(f Frame) {
	switch f.Type {
	case MsgEdgeConfig:
		m := DecodeEdgeConfig(f.Payload)
		side := input.NormalizeEdgeSide(m.EdgeSide)
		if side == "" {
			log.Printf("ignoring invalid edge side %q", m.EdgeSide)
			return
		}
		a.configurePeerControl(side)
		log.Printf("controller edge side: %s (return edge: %s)", side, oppositeEdge(side))
	case MsgMouseMove:
		atomic.AddUint64(&a.recvMouseMoveN, 1)
		m, err := DecodeMouseMove(f.Payload)
		if err != nil {
			log.Printf("drop malformed mouse move: %v", err)
			return
		}
		a.handleRemoteMouseMove(m)
	case MsgMouseClick:
		m, err := DecodeMouseClick(f.Payload)
		if err != nil {
			log.Printf("drop malformed mouse click: %v", err)
			return
		}
		if _, allowed := a.notePeerControlInput(false); !allowed {
			return
		}
		a.remoteInputMu.Lock()
		if m.Button < 3 {
			a.mu.Lock()
			a.remoteMouseButtons[m.Button] = m.Pressed
			a.mu.Unlock()
		}
		InjectMouseClick(m.Button, m.Pressed)
		a.touchRemoteKeyWatchdog()
		a.remoteInputMu.Unlock()
	case MsgMouseScroll:
		m, err := DecodeMouseScroll(f.Payload)
		if err != nil {
			log.Printf("drop malformed mouse scroll: %v", err)
			return
		}
		if _, allowed := a.notePeerControlInput(false); !allowed {
			return
		}
		InjectMouseScroll(m.Delta)
		a.touchRemoteKeyWatchdog()
	case MsgKeyDown:
		atomic.AddUint64(&a.recvKeyN, 1)
		m, err := DecodeKey(f.Payload)
		if err != nil {
			log.Printf("drop malformed key down: %v", err)
			return
		}
		if _, allowed := a.notePeerControlInput(false); !allowed {
			return
		}
		a.remoteInputMu.Lock()
		a.mu.Lock()
		a.remoteKeyState.Record(m, true)
		a.mu.Unlock()
		InjectKey(uint16(m.VKCode), uint16(m.ScanCode), m.Flags, true)
		a.touchRemoteKeyWatchdog()
		a.remoteInputMu.Unlock()
	case MsgKeyUp:
		atomic.AddUint64(&a.recvKeyN, 1)
		m, err := DecodeKey(f.Payload)
		if err != nil {
			log.Printf("drop malformed key up: %v", err)
			return
		}
		if _, allowed := a.notePeerControlInput(false); !allowed {
			return
		}
		a.remoteInputMu.Lock()
		a.mu.Lock()
		a.remoteKeyState.Record(m, false)
		a.mu.Unlock()
		InjectKey(uint16(m.VKCode), uint16(m.ScanCode), m.Flags, false)
		a.touchRemoteKeyWatchdog()
		a.remoteInputMu.Unlock()
	case MsgClipboard:
		m, err := DecodeClipboardSync(f.Payload)
		if err != nil {
			text := DecodeClipboard(f.Payload).Text
			if len(text) <= maxFramePayloadBytes {
				a.mu.Lock()
				a.clipboardState.RecordRemoteClipboard(text)
				a.mu.Unlock()
				SetClipboardText(text)
			} else {
				log.Printf("drop oversized legacy clipboard payload: %d bytes", len(text))
			}
			return
		}
		if len(m.Text) <= maxClipboardTextBytes {
			a.mu.Lock()
			a.clipboardState.RecordRemoteClipboard(m.Text)
			a.mu.Unlock()
			SetClipboardText(m.Text)
		} else {
			log.Printf("drop oversized clipboard payload: %d bytes", len(m.Text))
		}
	case MsgPing:
		a.enqueueSend(Frame{Type: MsgPong, Payload: f.Payload})
	case MsgPong:
		if len(f.Payload) != 8 {
			return
		}
		timestampNano := binary.BigEndian.Uint64(f.Payload)
		now := uint64(time.Now().UnixNano())
		if now > timestampNano {
			rtt := int((now - timestampNano) / 1e6)
			firstMeasurement, transportMode, profile, audioLatencyMs := a.updateSessionLatency(rtt)
			wailsRuntime.EventsEmit(a.ctx, "session-updated", a.GetSession())
			if firstMeasurement {
				peerName := ""
				if session := a.transport.GetSession(); session != nil {
					peerName = session.PeerName
				}
				logutil.LogKV("session.latency.ready",
					"peer", peerName,
					"rtt_ms", rtt,
					"audio_estimate_ms", audioLatencyMs,
					"audio_transport", transportMode,
					"audio_profile", profile,
				)
			}
		}
	case MsgSwitchBack:
		log.Println("peer requested switch back")
		a.pausePeerControlUntilWake()
		a.inputHook.ExitRemoteMode()
	case MsgAudioTransport:
		a.handleInboundAudioTransport(f.Payload)
	case MsgAudioFormat:
		a.handleInboundAudioFormat(f.Payload)
	case MsgAudioStart:
		if a.audio != nil {
			_ = a.audio.StartCapture(func(f Frame) {
				a.handleCapturedAudioFrame(f)
			})
		}
	case MsgAudioStop:
		if a.audio != nil {
			a.audio.StopCapture()
		}
	case MsgAudioData:
		atomic.AddUint64(&a.recvAudioN, 1)
		a.handleInboundAudioData(f.Payload)
	case MsgMicTransport:
		a.handleInboundMicTransport(f.Payload)
	case MsgMicFormat:
		a.handleInboundMicFormat(f.Payload)
	case MsgMicStart:
		if a.audio != nil {
			_ = a.audio.StartMicCapture(func(f Frame) {
				a.handleCapturedMicFrame(f)
			})
		}
	case MsgMicStop:
		if a.audio != nil {
			a.audio.StopMicCapture()
		}
	case MsgMicData:
		atomic.AddUint64(&a.recvAudioN, 1)
		a.handleInboundMicData(f.Payload)
	case MsgUnicodeText:
		m, err := DecodeUnicodeText(f.Payload)
		if err != nil {
			log.Printf("drop malformed unicode text: %v", err)
			return
		}
		if _, allowed := a.notePeerControlInput(false); !allowed {
			return
		}
		InjectUnicode(m.Char)
		a.touchRemoteKeyWatchdog()
	case MsgFileTransferOffer, MsgFileTransferAccept, MsgFileChunk, MsgFileTransferDone, MsgFileTransferCancel:
		if f.Type == MsgFileChunk {
			atomic.AddUint64(&a.recvFileChunkN, 1)
		} else {
			atomic.AddUint64(&a.recvOtherN, 1)
		}
		if a.fileTx != nil {
			a.fileTx.HandleInbound(f)
		}
	}
}

func (a *App) handleRemoteMouseMove(m protocol.MouseMoveMsg) {
	if m.DX == 0 && m.DY == 0 {
		// Zero-delta frame is the remote-wake signal; only release stuck keys.
		// Skip injection, watchdog arm, and return-edge check to prevent
		// immediately bouncing the session back if the cursor is at the edge.
		a.notePeerControlInput(true)
		a.releaseInjectedRemoteKeys()
		return
	}
	activated, allowed := a.notePeerControlInput(false)
	if !allowed {
		return
	}
	InjectMouseMove(m.DX, m.DY)
	// Only take the mutex to update the watchdog when input is actually held.
	// During pure mouse movement (the common case) remoteInputActiveN is 0 and
	// this call would unconditionally clear a timer that doesn't exist — skip it.
	if atomic.LoadUint64(&a.remoteInputActiveN) != 0 {
		a.touchRemoteKeyWatchdog()
	}
	if activated {
		return
	}
	a.checkReturnEdge()
}
