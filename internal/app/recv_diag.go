package app

import (
	"log"
	"sync/atomic"
	"time"
)

const recvStatsInterval = 5 * time.Second

// recvStatsLoop periodically logs inbound frame rates per type.
// Runs on the host side to help diagnose receive stalls or unexpected frame
// bursts. Goroutine lifetime is tied to the app context.
func (a *App) recvStatsLoop() {
	ticker := time.NewTicker(recvStatsInterval)
	defer ticker.Stop()

	var prevMouse, prevKey, prevAudio, prevFile, prevOther uint64

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			mouse := atomic.LoadUint64(&a.recvMouseMoveN)
			key := atomic.LoadUint64(&a.recvKeyN)
			audio := atomic.LoadUint64(&a.recvAudioN)
			file := atomic.LoadUint64(&a.recvFileChunkN)
			other := atomic.LoadUint64(&a.recvOtherN)

			dMouse := mouse - prevMouse
			dKey := key - prevKey
			dAudio := audio - prevAudio
			dFile := file - prevFile
			dOther := other - prevOther
			prevMouse, prevKey, prevAudio, prevFile, prevOther = mouse, key, audio, file, other

			total := dMouse + dKey + dAudio + dFile + dOther
			if total == 0 {
				continue // suppress logs while idle
			}

			secs := recvStatsInterval.Seconds()
			log.Printf("recv stats: mouse=%.0f/s key=%.0f/s audio=%.0f/s file=%.0f/s other=%.0f/s",
				float64(dMouse)/secs,
				float64(dKey)/secs,
				float64(dAudio)/secs,
				float64(dFile)/secs,
				float64(dOther)/secs,
			)
		}
	}
}
