//go:build !windows

package audio

// Audio capture and playback are Windows-only; elsewhere the loops idle so
// the rest of the app (and its tests) can run.

func (a *AudioStreamer) runCapture(_ StreamKind, _ CaptureHandler, stop <-chan struct{}) { <-stop }

func (a *AudioStreamer) runRender(_ StreamKind, stop <-chan struct{}) { <-stop }

func ListRenderDevices() []AudioDevice { return nil }

func ListCaptureDevices() []AudioDevice { return nil }
