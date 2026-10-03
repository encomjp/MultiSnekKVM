//go:build windows

package audio

import (
	"sync"
	"testing"

	"multisnekkvm/internal/protocol"
)

// Rapid concurrent Start/Stop previously could close a nil or replaced stop
// channel, and reuse a WaitGroup while its previous worker still ran.
func TestAudioWorkerConcurrentStartStop(t *testing.T) {
	a, err := NewAudioStreamer()
	if err != nil { t.Fatal(err) }
	format := testPCM16WaveFormat(48000, 2)
	if err := a.SetPlaybackFormat(format); err != nil { t.Fatal(err) }
	if err := a.SetMicPlaybackFormat(format); err != nil { t.Fatal(err) }
	t.Cleanup(a.Close)
	noop := func(protocol.Frame) {}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				_ = a.StartCapture(noop)
				a.StopCapture()
				_ = a.StartMicCapture(noop)
				a.StopMicCapture()
				_ = a.StartPlayback()
				a.StopPlayback()
				_ = a.StartMicPlayback()
				a.StopMicPlayback()
			}
		}()
	}
	wg.Wait()
	if a.IsCapturing() || a.IsPlaying() || a.IsMicCapturing() || a.IsMicPlaying() {
		t.Fatal("audio workers must be stopped after concurrent start/stop")
	}
}
