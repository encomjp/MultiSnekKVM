package app

import "log"

const (
	fileInboundCap  = 64
	audioInboundCap = 256
)

func (a *App) initInboundDispatch() {
	a.fileInboundCh = make(chan Frame, fileInboundCap)
	a.audioInboundCh = make(chan Frame, audioInboundCap)
	SafeGo("inbound-file", func() { a.inboundFileLoop() })
	SafeGo("inbound-audio", func() { a.inboundAudioLoop() })
}

// inboundFileLoop processes file-transfer frames off the readLoop goroutine.
// File transfer involves synchronous disk I/O (WriteAt) that would stall the
// TCP receive pipeline and cause backpressure if run on the readLoop thread.
func (a *App) inboundFileLoop() {
	for {
		select {
		case f := <-a.fileInboundCh:
			a.handleFrame(f)
		case <-a.ctx.Done():
			return
		}
	}
}

// inboundAudioLoop processes audio data frames off the readLoop goroutine.
// WASAPI decode/enqueue can block briefly; running it on the readLoop would
// stall mouse and keyboard frame processing behind it.
func (a *App) inboundAudioLoop() {
	for {
		select {
		case f := <-a.audioInboundCh:
			a.handleFrame(f)
		case <-a.ctx.Done():
			return
		}
	}
}

// drainInboundChannels discards any stale frames still queued from the
// previous session. Call on session connect to prevent old file/audio frames
// leaking into the new pipeline.
func (a *App) drainInboundChannels() {
	n := 0
	for {
		select {
		case <-a.fileInboundCh:
			n++
		case <-a.audioInboundCh:
			n++
		default:
			if n > 0 {
				log.Printf("inbound: drained %d stale frame(s) on session reset", n)
			}
			return
		}
	}
}
