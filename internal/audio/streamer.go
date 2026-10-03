package audio

import (
	"fmt"
	"sync"
)

type AudioDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Flow string `json:"flow"`
}

// CaptureHandler receives captured audio on the capture thread. The format is
// delivered before any data and again whenever the device is reopened. Data
// slices are only valid for the duration of the call.
type CaptureHandler interface {
	OnCaptureFormat(format []byte)
	OnCaptureData(data []byte)
}

// worker runs one capture or render loop. Start and stop are serialised, and
// stop waits for the loop to exit, so a restart never overlaps the old loop.
type worker struct {
	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

func (w *worker) start(run func(stop <-chan struct{})) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.runningLocked() {
		return false
	}
	stop, done := make(chan struct{}), make(chan struct{})
	w.stop, w.done = stop, done
	go func() {
		defer close(done)
		run(stop)
	}()
	return true
}

func (w *worker) stopAndWait() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop == nil {
		return
	}
	close(w.stop)
	<-w.done
	w.stop, w.done = nil, nil
}

func (w *worker) running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.runningLocked()
}

func (w *worker) runningLocked() bool {
	if w.done == nil {
		return false
	}
	select {
	case <-w.done:
		return false
	default:
		return true
	}
}

// AudioStreamer owns the capture and render loops for desktop audio and the
// microphone, plus one jitter buffer (Playout) per stream.
type AudioStreamer struct {
	mu            sync.Mutex
	profile       string
	muteSource    bool
	captureDevice [2]string // desktop: render endpoint to loop back; mic: capture endpoint
	renderDevice  [2]string // desktop playback; mic playback ("" = desktop playback device)
	handlers      [2]CaptureHandler

	captures [2]worker
	renders  [2]worker
	players  [2]*Playout
}

func NewAudioStreamer() (*AudioStreamer, error) {
	a := &AudioStreamer{profile: audioProfileBalanced}
	for i := range a.players {
		a.players[i] = newPlayout(playoutConfigForProfile(a.profile))
	}
	return a, nil
}

// Player returns the jitter buffer that feeds a stream's render loop.
func (a *AudioStreamer) Player(kind StreamKind) *Playout { return a.players[kind] }

func (a *AudioStreamer) SetQualityProfile(profile string) error {
	if !ValidProfile(profile) {
		return fmt.Errorf("invalid audio quality profile %q", profile)
	}
	a.mu.Lock()
	a.profile = profile
	a.mu.Unlock()
	for _, p := range a.players {
		p.SetConfig(playoutConfigForProfile(profile))
	}
	return nil
}

func (a *AudioStreamer) qualityProfile() audioProfileSpec {
	a.mu.Lock()
	defer a.mu.Unlock()
	return audioProfileSpecForName(a.profile)
}

// SetMuteSource mutes this PC's speakers while its desktop audio is being
// captured for the peer. The previous mute state is restored when capture stops.
func (a *AudioStreamer) SetMuteSource(enabled bool) {
	a.mu.Lock()
	changed := a.muteSource != enabled
	a.muteSource = enabled
	a.mu.Unlock()
	if changed {
		a.restartCapture(StreamDesktop)
	}
}

func (a *AudioStreamer) muteSourceEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.muteSource
}

// StartCapture starts capturing a stream. If it is already running, it
// returns false and the existing handler keeps receiving data.
func (a *AudioStreamer) StartCapture(kind StreamKind, h CaptureHandler) bool {
	a.mu.Lock()
	a.handlers[kind] = h
	a.mu.Unlock()
	return a.captures[kind].start(func(stop <-chan struct{}) { a.runCapture(kind, h, stop) })
}

func (a *AudioStreamer) StopCapture(kind StreamKind) { a.captures[kind].stopAndWait() }

func (a *AudioStreamer) IsCapturing(kind StreamKind) bool { return a.captures[kind].running() }

func (a *AudioStreamer) restartCapture(kind StreamKind) {
	if !a.captures[kind].running() {
		return
	}
	a.mu.Lock()
	h := a.handlers[kind]
	a.mu.Unlock()
	a.captures[kind].stopAndWait()
	if h != nil {
		a.StartCapture(kind, h)
	}
}

// StartPlayback opens the render endpoint for a stream; it is a no-op when
// already playing.
func (a *AudioStreamer) StartPlayback(kind StreamKind) {
	a.renders[kind].start(func(stop <-chan struct{}) { a.runRender(kind, stop) })
}

// StopPlayback closes the render endpoint and discards buffered audio.
func (a *AudioStreamer) StopPlayback(kind StreamKind) {
	a.renders[kind].stopAndWait()
	a.players[kind].Reset()
}

func (a *AudioStreamer) IsPlaying(kind StreamKind) bool { return a.renders[kind].running() }

func (a *AudioStreamer) restartPlayback(kind StreamKind) {
	if !a.renders[kind].running() {
		return
	}
	a.renders[kind].stopAndWait()
	a.StartPlayback(kind)
}

func (a *AudioStreamer) setDevice(slot *string, id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if *slot == id {
		return false
	}
	*slot = id
	return true
}

// Device changes reopen running loops on the new device instead of stopping
// them for good.
func (a *AudioStreamer) SetCaptureDeviceID(id string) {
	if a.setDevice(&a.captureDevice[StreamDesktop], id) {
		a.restartCapture(StreamDesktop)
	}
}

func (a *AudioStreamer) SetMicDeviceID(id string) {
	if a.setDevice(&a.captureDevice[StreamMic], id) {
		a.restartCapture(StreamMic)
	}
}

func (a *AudioStreamer) SetPlaybackDeviceID(id string) {
	if a.setDevice(&a.renderDevice[StreamDesktop], id) {
		a.restartPlayback(StreamDesktop)
		if a.renderDeviceID(StreamMic) == id {
			a.restartPlayback(StreamMic)
		}
	}
}

func (a *AudioStreamer) SetMicPlaybackDeviceID(id string) {
	if a.setDevice(&a.renderDevice[StreamMic], id) {
		a.restartPlayback(StreamMic)
	}
}

func (a *AudioStreamer) captureDeviceID(kind StreamKind) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.captureDevice[kind]
}

func (a *AudioStreamer) renderDeviceID(kind StreamKind) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if kind == StreamMic && a.renderDevice[StreamMic] == "" {
		return a.renderDevice[StreamDesktop]
	}
	return a.renderDevice[kind]
}

func (a *AudioStreamer) Close() {
	for k := range a.captures {
		a.captures[k].stopAndWait()
		a.renders[k].stopAndWait()
	}
}
