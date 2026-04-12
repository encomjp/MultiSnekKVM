package app

import (
	"log"

	"multisnekkvm/internal/audio"
	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/protocol"
	"multisnekkvm/internal/settings"
)

func (a *App) initAudioRuntime() {
	a.audioOutbound = audio.NewOutboundRealtimeStream("audio", protocol.MsgAudioTransport, protocol.MsgAudioFormat, protocol.MsgAudioData)
	a.micOutbound = audio.NewOutboundRealtimeStream("mic", protocol.MsgMicTransport, protocol.MsgMicFormat, protocol.MsgMicData)
	a.audioInbound = audio.NewInboundRealtimeStream("audio")
	a.micInbound = audio.NewInboundRealtimeStream("mic")
	a.applyAudioQualityProfile()
	a.logAudioPipelineConfig("audio-runtime-init")
}

func (a *App) resetAudioPipelines() {
	if a.audioOutbound != nil {
		a.audioOutbound.Reset()
	}
	if a.micOutbound != nil {
		a.micOutbound.Reset()
	}
	if a.audioInbound != nil {
		a.audioInbound.Reset()
	}
	if a.micInbound != nil {
		a.micInbound.Reset()
	}
}

func (a *App) currentAudioTransportSettings() (string, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return audio.NormalizeTransportMode(a.audioTransportMode), audio.NormalizeProfile(a.audioProfile)
}

func (a *App) currentAudioLatencyState() (int, string, string, int) {
	a.mu.RLock()
	rttMs := a.latencyMs
	transportMode := audio.NormalizeTransportMode(a.audioTransportMode)
	profile := audio.NormalizeProfile(a.audioProfile)
	a.mu.RUnlock()
	return rttMs, transportMode, profile, audio.EstimatedPlaybackLatencyMs(rttMs, transportMode, profile)
}

func (a *App) updateSessionLatency(rttMs int) (bool, string, string, int) {
	a.mu.Lock()
	firstMeasurement := a.latencyMs < 0
	if a.latencyPrev >= 0 {
		delta := rttMs - a.latencyPrev
		if delta < 0 {
			delta = -delta
		}
		if a.jitterMs < 0 {
			a.jitterMs = delta
		} else {
			a.jitterMs = (a.jitterMs*7 + delta) / 8
		}
	}
	a.latencyMs = rttMs
	a.latencyPrev = rttMs
	transportMode := audio.NormalizeTransportMode(a.audioTransportMode)
	profile := audio.NormalizeProfile(a.audioProfile)
	a.mu.Unlock()
	return firstMeasurement, transportMode, profile, audio.EstimatedPlaybackLatencyMs(rttMs, transportMode, profile)
}

func (a *App) currentJitterMs() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.jitterMs
}

func (a *App) logAudioPipelineConfig(reason string) {
	rttMs, transportMode, profile, estimatedLatencyMs := a.currentAudioLatencyState()
	logutil.LogKV("app.audio.config",
		"reason", reason,
		"transport", transportMode,
		"profile", profile,
		"base_latency_ms", audio.EstimatedPlaybackBaseLatencyMs(transportMode, profile),
		"rtt_ms", rttMs,
		"estimated_latency_ms", estimatedLatencyMs,
	)
}

func (a *App) applyAudioQualityProfile() {
	a.mu.RLock()
	streamer := a.audio
	profile := audio.NormalizeProfile(a.audioProfile)
	a.mu.RUnlock()
	if streamer == nil {
		return
	}
	if err := streamer.SetQualityProfile(profile); err != nil {
		log.Printf("audio quality profile rejected: %v", err)
	}
}

func (a *App) nextMediaControlGeneration() uint64 {
	a.mu.Lock()
	a.mediaControlGeneration++
	generation := a.mediaControlGeneration
	a.mu.Unlock()
	return generation
}

func (a *App) currentMediaControlGeneration() uint64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mediaControlGeneration
}

func (a *App) mediaControlGenerationIsCurrent(generation uint64) bool {
	return generation == a.currentMediaControlGeneration()
}

func (a *App) scheduleControllerMediaReconcile(name string) {
	generation := a.nextMediaControlGeneration()
	logutil.SafeGo(name, func() {
		a.mediaControlMu.Lock()
		defer a.mediaControlMu.Unlock()
		if !a.mediaControlGenerationIsCurrent(generation) {
			return
		}
		a.reconcileControllerMediaState(generation)
	})
}

func (a *App) syncAudioDeviceSelection() {
	a.mu.RLock()
	streamer := a.audio
	captureDeviceID := a.captureDeviceID
	playbackDeviceID := a.playbackDeviceID
	micDeviceID := a.micDeviceID
	micPlaybackDeviceID := a.micPlaybackDeviceID
	a.mu.RUnlock()
	if streamer == nil {
		return
	}
	streamer.SetCaptureDeviceID(captureDeviceID)
	streamer.SetPlaybackDeviceID(playbackDeviceID)
	streamer.SetMicDeviceID(micDeviceID)
	streamer.SetMicPlaybackDeviceID(micPlaybackDeviceID)
}

func (a *App) reconcileControllerMediaState(generation uint64) {
	a.applyAudioQualityProfile()
	a.syncAudioDeviceSelection()
	if !a.mediaControlGenerationIsCurrent(generation) {
		return
	}
	a.stopAllAudio()
	a.stopAllMic()
	if !a.mediaControlGenerationIsCurrent(generation) {
		return
	}

	s := a.transport.GetSession()
	if s == nil || !a.isController() {
		return
	}
	a.bumpRealtimeSendGeneration()
	a.resetAudioPipelines()

	a.mu.RLock()
	mode := a.audioMode
	timing := a.audioTiming
	mic := a.micMode
	a.mu.RUnlock()

	if timing != "always" && !(timing == "switched" && a.inputHook.IsInRemoteMode()) {
		return
	}
	if mode != "off" {
		a.startAudioForMode(mode)
	}
	if mic != "off" {
		a.startMicForMode(mic)
	}
}

func (a *App) restartControllerMediaIfNeeded() {
	a.scheduleControllerMediaReconcile("controller-media-restart")
}

func (a *App) restartPassiveOutboundCaptures(reason string) {
	if a.audio == nil || a.isController() {
		return
	}
	restartAudio := audioIsCapturing(a.audio)
	restartMic := audioIsMicCapturing(a.audio)
	if !restartAudio && !restartMic {
		return
	}

	a.bumpRealtimeSendGeneration()
	if restartAudio {
		audioStopCapture(a.audio)
	}
	if restartMic {
		audioStopMicCapture(a.audio)
	}
	a.resetAudioPipelines()

	if restartAudio {
		if err := audioStartCapture(a.audio, func(f Frame) {
			a.handleCapturedAudioFrame(f)
		}); err != nil {
			log.Printf("audio capture restart failed after %s: %v", reason, err)
		}
	}
	if restartMic {
		if err := audioStartMicCapture(a.audio, func(f Frame) {
			a.handleCapturedMicFrame(f)
		}); err != nil {
			log.Printf("mic capture restart failed after %s: %v", reason, err)
		}
	}
}

func (a *App) GetAudioTransport() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return audio.NormalizeTransportMode(a.audioTransportMode)
}

func (a *App) SetAudioTransport(mode string) {
	if !audio.ValidTransportMode(mode) {
		return
	}
	a.mu.Lock()
	old := a.audioTransportMode
	a.audioTransportMode = mode
	a.mu.Unlock()
	a.settings.Update(func(s *settings.Settings) { s.AudioTransport = mode })
	if old == mode {
		return
	}
	a.logAudioPipelineConfig("audio-transport-change")
	a.restartPassiveOutboundCaptures("audio-transport-change")
	a.scheduleControllerMediaReconcile("audio-transport-change")
}

func (a *App) GetAudioProfile() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return audio.NormalizeProfile(a.audioProfile)
}

func (a *App) SetAudioProfile(profile string) {
	if !audio.ValidProfile(profile) {
		return
	}
	a.mu.Lock()
	old := a.audioProfile
	a.audioProfile = profile
	a.mu.Unlock()
	a.settings.Update(func(s *settings.Settings) { s.AudioProfile = profile })
	a.applyAudioQualityProfile()
	if old == profile {
		return
	}
	a.logAudioPipelineConfig("audio-profile-change")
	a.restartPassiveOutboundCaptures("audio-profile-change")
	a.scheduleControllerMediaReconcile("audio-profile-change")
}

func (a *App) handleInboundAudioTransport(payload []byte) {
	if a.audioInbound == nil {
		return
	}
	transportMode, err := audio.DecodeTransportMode(payload)
	if err != nil {
		log.Printf("audio playback transport rejected: %v", err)
		return
	}
	if err := a.audioInbound.SetTransport(transportMode); err != nil {
		log.Printf("audio playback transport unavailable: %v", err)
	}
}

func (a *App) handleInboundAudioFormat(payload []byte) {
	if a.audio == nil {
		return
	}
	playbackFormat := payload
	if a.audioInbound != nil {
		decodedFormat, err := a.audioInbound.ProcessFormat(payload)
		if err != nil {
			log.Printf("audio playback format rejected: %v", err)
			return
		}
		playbackFormat = decodedFormat
	}
	if err := a.audio.SetPlaybackFormat(playbackFormat); err != nil {
		log.Printf("audio playback format rejected: %v", err)
	}
}

func (a *App) handleInboundAudioData(payload []byte) {
	if a.audio == nil {
		return
	}
	decodedPayload := payload
	if a.audioInbound != nil {
		pcmPayload, err := a.audioInbound.ProcessData(payload)
		if err != nil {
			log.Printf("audio playback data rejected: %v", err)
			return
		}
		decodedPayload = pcmPayload
	}
	if len(decodedPayload) == 0 {
		return
	}
	if err := a.audio.StartPlayback(); err != nil {
		log.Printf("audio playback start deferred: %v", err)
	}
	a.audio.EnqueueAudio(decodedPayload)
}

func (a *App) handleInboundMicTransport(payload []byte) {
	if a.micInbound == nil {
		return
	}
	transportMode, err := audio.DecodeTransportMode(payload)
	if err != nil {
		log.Printf("mic playback transport rejected: %v", err)
		return
	}
	if err := a.micInbound.SetTransport(transportMode); err != nil {
		log.Printf("mic playback transport unavailable: %v", err)
	}
}

func (a *App) handleInboundMicFormat(payload []byte) {
	if a.audio == nil {
		return
	}
	playbackFormat := payload
	if a.micInbound != nil {
		decodedFormat, err := a.micInbound.ProcessFormat(payload)
		if err != nil {
			log.Printf("mic playback format rejected: %v", err)
			return
		}
		playbackFormat = decodedFormat
	}
	if err := a.audio.SetMicPlaybackFormat(playbackFormat); err != nil {
		log.Printf("mic playback format rejected: %v", err)
	}
}

func (a *App) handleInboundMicData(payload []byte) {
	if a.audio == nil {
		return
	}
	decodedPayload := payload
	if a.micInbound != nil {
		pcmPayload, err := a.micInbound.ProcessData(payload)
		if err != nil {
			log.Printf("mic playback data rejected: %v", err)
			return
		}
		decodedPayload = pcmPayload
	}
	if len(decodedPayload) == 0 {
		return
	}
	if err := a.audio.StartMicPlayback(); err != nil {
		log.Printf("mic playback start deferred: %v", err)
	}
	a.audio.EnqueueMicAudio(decodedPayload)
}
