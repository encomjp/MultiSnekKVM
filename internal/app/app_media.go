package app

import (
	"fmt"
	"log"
	"time"

	"multisnekkvm/internal/audio"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) GetAudioMode() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.audioMode
}

func (a *App) SetAudioMode(mode string) {
	if mode != "off" && mode != "remote" && mode != "local" {
		return
	}
	a.mu.Lock()
	old := a.audioMode
	a.audioMode = mode
	a.mu.Unlock()
	a.settings.Update(func(s *Settings) { s.AudioMode = mode })

	if old == mode {
		return
	}
	a.scheduleControllerMediaReconcile("audio-mode-change")
}

func (a *App) GetAudioTiming() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.audioTiming
}

func (a *App) SetAudioTiming(timing string) {
	if timing != "always" && timing != "switched" {
		return
	}
	a.mu.Lock()
	old := a.audioTiming
	a.audioTiming = timing
	a.mu.Unlock()
	a.settings.Update(func(s *Settings) { s.AudioTiming = timing })

	if old == timing {
		return
	}
	a.scheduleControllerMediaReconcile("audio-timing-change")
}

func (a *App) isController() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionRole == "controller"
}

func (a *App) startAudioForMode(mode string) {
	if a.audio == nil {
		return
	}
	switch mode {
	case "remote":
		a.enqueueSend(Frame{Type: MsgAudioStart})
		log.Println("audio: hearing remote")
	case "local":
		a.startCapture(audio.StreamDesktop)
		log.Println("audio: sending local")
	}
}

func (a *App) stopAllAudio() {
	if a.audio == nil {
		return
	}
	a.audio.StopCapture(audio.StreamDesktop)
	a.audio.StopPlayback(audio.StreamDesktop)
	if s := a.transport.GetSession(); s != nil {
		a.enqueueSend(Frame{Type: MsgAudioStop})
	}
}

func (a *App) handleControlStateChange() {
	if !a.isController() {
		return
	}
	a.mu.RLock()
	timing := a.audioTiming
	a.mu.RUnlock()

	if timing != "switched" {
		return
	}
	a.scheduleControllerMediaReconcile("control-state-change")
}

func (a *App) GetMicMode() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.micMode
}

func (a *App) SetMicMode(mode string) {
	if mode != "off" && mode != "send" && mode != "receive" {
		return
	}
	a.mu.Lock()
	old := a.micMode
	a.micMode = mode
	a.mu.Unlock()
	a.settings.Update(func(s *Settings) { s.MicMode = mode })

	if old == mode {
		return
	}
	a.scheduleControllerMediaReconcile("mic-mode-change")
}

func (a *App) startMicForMode(mode string) {
	if a.audio == nil {
		return
	}
	switch mode {
	case "receive":
		a.enqueueSend(Frame{Type: MsgMicStart})
		log.Println("mic: hearing remote mic")
	case "send":
		a.startCapture(audio.StreamMic)
		log.Println("mic: sending local mic")
	}
}

func (a *App) stopAllMic() {
	if a.audio == nil {
		return
	}
	a.audio.StopCapture(audio.StreamMic)
	a.audio.StopPlayback(audio.StreamMic)
	if s := a.transport.GetSession(); s != nil {
		a.enqueueSend(Frame{Type: MsgMicStop})
	}
}

func (a *App) latencyLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			if a.transport.GetSession() == nil {
				continue
			}
			ts := uint64(time.Now().UnixNano())
			a.enqueueSend(Frame{
				Type:    MsgPing,
				Payload: PingMsg{TimestampNano: ts}.Encode(),
			})
		}
	}
}

func (a *App) SendFiles(paths []string) {
	if a.fileTx == nil || a.transport.GetSession() == nil {
		return
	}
	a.fileTx.StartSend(paths)
}

func (a *App) PickAndSendFiles() error {
	if a.fileTx == nil || a.transport.GetSession() == nil {
		return fmt.Errorf("not connected")
	}
	paths, err := wailsRuntime.OpenMultipleFilesDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select files to send",
	})
	if err != nil {
		return fmt.Errorf("file dialog: %w", err)
	}
	if len(paths) == 0 {
		return nil
	}
	a.fileTx.StartSend(paths)
	return nil
}

func (a *App) GetMuteSource() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.muteSource
}

// SetMuteSource mutes this PC's speakers while its desktop audio is sent to
// the peer (restored when sending stops).
func (a *App) SetMuteSource(enabled bool) {
	a.mu.Lock()
	changed := a.muteSource != enabled
	a.muteSource = enabled
	a.mu.Unlock()
	if !changed {
		return
	}
	a.settings.Update(func(s *Settings) { s.MuteSource = enabled })
	if a.audio != nil {
		a.audio.SetMuteSource(enabled)
	}
}

func (a *App) GetAudioDevices() []AudioDevice {
	render := ListRenderDevices()
	capture := ListCaptureDevices()
	return append(render, capture...)
}

// setDeviceID stores a device choice and moves the affected running stream
// to the new device (previously the stream was stopped and stayed off).
func (a *App) setDeviceID(field *string, id string, update func(*Settings, string)) bool {
	a.mu.Lock()
	changed := *field != id
	*field = id
	a.mu.Unlock()
	if changed {
		a.settings.Update(func(s *Settings) { update(s, id) })
	}
	return changed && a.audio != nil
}

func (a *App) GetCaptureDeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.captureDeviceID
}

func (a *App) SetCaptureDeviceID(id string) {
	if a.setDeviceID(&a.captureDeviceID, id, func(s *Settings, value string) {
		s.CaptureDeviceID = value
	}) {
		a.audio.SetCaptureDeviceID(id)
	}
}

func (a *App) GetPlaybackDeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.playbackDeviceID
}

func (a *App) SetPlaybackDeviceID(id string) {
	if a.setDeviceID(&a.playbackDeviceID, id, func(s *Settings, value string) {
		s.PlaybackDeviceID = value
	}) {
		a.audio.SetPlaybackDeviceID(id)
	}
}

func (a *App) GetMicDeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.micDeviceID
}

func (a *App) SetMicDeviceID(id string) {
	if a.setDeviceID(&a.micDeviceID, id, func(s *Settings, value string) {
		s.MicDeviceID = value
	}) {
		a.audio.SetMicDeviceID(id)
	}
}

func (a *App) GetMicPlaybackDeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.micPlaybackDeviceID
}

func (a *App) SetMicPlaybackDeviceID(id string) {
	if a.setDeviceID(&a.micPlaybackDeviceID, id, func(s *Settings, value string) {
		s.MicPlaybackDeviceID = value
	}) {
		a.audio.SetMicPlaybackDeviceID(id)
	}
}

func (a *App) GetStartMinimized() bool {
	return a.settings.Get().StartMinimized
}

func (a *App) SetStartMinimized(enabled bool) {
	a.settings.Update(func(s *Settings) { s.StartMinimized = enabled })
}
