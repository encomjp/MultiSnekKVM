package app

import (
	"fmt"
	"log"
	"time"

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
		_ = a.audio.StartCapture(func(f Frame) {
			a.handleCapturedAudioFrame(f)
		})
		log.Println("audio: sending local")
	}
}

func (a *App) stopAllAudio() {
	if a.audio == nil {
		return
	}
	a.audio.StopCapture()
	a.audio.StopPlayback()
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
		_ = a.audio.StartMicCapture(func(f Frame) {
			a.handleCapturedMicFrame(f)
		})
		log.Println("mic: sending local mic")
	}
}

func (a *App) stopAllMic() {
	if a.audio == nil {
		return
	}
	a.audio.StopMicCapture()
	a.audio.StopMicPlayback()
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

func (a *App) SetMuteSource(enabled bool) {
	a.mu.Lock()
	a.muteSource = enabled
	a.mu.Unlock()
	a.settings.Update(func(s *Settings) { s.MuteSource = enabled })
}

func (a *App) GetAudioDevices() []AudioDevice {
	render := ListRenderDevices()
	capture := ListCaptureDevices()
	return append(render, capture...)
}

func (a *App) setDeviceID(field *string, id, reason string, update func(*Settings, string)) {
	a.mu.Lock()
	*field = id
	a.mu.Unlock()
	a.settings.Update(func(s *Settings) { update(s, id) })
	a.scheduleControllerMediaReconcile(reason)
}

func (a *App) GetCaptureDeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.captureDeviceID
}

func (a *App) SetCaptureDeviceID(id string) {
	a.setDeviceID(&a.captureDeviceID, id, "capture-device-change", func(s *Settings, value string) {
		s.CaptureDeviceID = value
	})
}

func (a *App) GetPlaybackDeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.playbackDeviceID
}

func (a *App) SetPlaybackDeviceID(id string) {
	a.setDeviceID(&a.playbackDeviceID, id, "playback-device-change", func(s *Settings, value string) {
		s.PlaybackDeviceID = value
	})
}

func (a *App) GetMicDeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.micDeviceID
}

func (a *App) SetMicDeviceID(id string) {
	a.setDeviceID(&a.micDeviceID, id, "mic-device-change", func(s *Settings, value string) {
		s.MicDeviceID = value
	})
}

func (a *App) GetMicPlaybackDeviceID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.micPlaybackDeviceID
}

func (a *App) SetMicPlaybackDeviceID(id string) {
	a.setDeviceID(&a.micPlaybackDeviceID, id, "mic-playback-device-change", func(s *Settings, value string) {
		s.MicPlaybackDeviceID = value
	})
}

func (a *App) GetStartMinimized() bool {
	return a.settings.Get().StartMinimized
}

func (a *App) SetStartMinimized(enabled bool) {
	a.settings.Update(func(s *Settings) { s.StartMinimized = enabled })
}
