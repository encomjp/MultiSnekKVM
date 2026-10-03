package app

import (
	"log"
	"sync/atomic"
	"time"

	"multisnekkvm/internal/audio"
	"multisnekkvm/internal/link"
	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/protocol"
	"multisnekkvm/internal/settings"
)

// streamCapture feeds captured device audio for one stream through its
// OutboundStream and onto the send mux. It runs on the capture thread.
type streamCapture struct {
	a         *App
	kind      audio.StreamKind
	out       *audio.OutboundStream
	errLogged atomic.Int64
}

func (c *streamCapture) OnCaptureFormat(format []byte) {
	mode := c.a.resolveAudioTransport(c.kind)
	_, profile := c.a.currentAudioTransportSettings()
	frames, effective, err := c.out.Configure(mode, profile, format)
	if err != nil {
		log.Printf("%s capture: %v", c.kind, err)
		return
	}
	if effective != mode {
		log.Printf("%s capture: %s unavailable, sending %s", c.kind, mode, effective)
	}
	logutil.LogKV("app.audio.stream", "stream", c.kind.String(), "transport", effective, "profile", profile)
	c.a.sendAudioFrames(frames)
}

func (c *streamCapture) OnCaptureData(data []byte) {
	frames, err := c.out.Process(data)
	if err != nil {
		if now := time.Now().Unix(); c.errLogged.Swap(now) != now {
			log.Printf("%s capture: %v", c.kind, err)
		}
	}
	c.a.sendAudioFrames(frames)
}

func (a *App) sendAudioFrames(frames []protocol.Frame) {
	for _, f := range frames {
		a.enqueueSend(f)
	}
}

func (a *App) initAudioRuntime() {
	for _, kind := range []audio.StreamKind{audio.StreamDesktop, audio.StreamMic} {
		out := audio.NewOutboundStream(kind)
		a.audioOut[kind] = out
		a.audioIn[kind] = audio.NewInboundStream(kind)
		a.audioCapture[kind] = &streamCapture{a: a, kind: kind, out: out}
	}
	a.applyAudioQualityProfile()
	if a.audio != nil {
		a.audio.SetMuteSource(a.GetMuteSource())
	}
	a.logAudioPipelineConfig("audio-runtime-init")
}

// resetAudioStreams forgets per-session stream state. It is only called when
// a session starts or ends: resetting a receiver mid-session made it play
// Opus packets as raw PCM (white noise) until the sender restarted.
func (a *App) resetAudioStreams() {
	for kind := range a.audioIn {
		if a.audioIn[kind] != nil {
			a.audioIn[kind].Reset()
		}
		if a.audioOut[kind] != nil {
			a.audioOut[kind].Reset()
		}
		if a.audio != nil {
			a.audio.Player(audio.StreamKind(kind)).Reset()
		}
	}
}

func (a *App) currentAudioTransportSettings() (string, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return audio.NormalizeTransportMode(a.audioTransportMode), audio.NormalizeProfile(a.audioProfile)
}

// resolveAudioTransport applies "auto" using the route of the live session.
func (a *App) resolveAudioTransport(kind audio.StreamKind) string {
	mode, _ := a.currentAudioTransportSettings()
	route := ""
	if a.transport != nil {
		if s := a.transport.GetSession(); s != nil {
			route = link.KindForAddress(s.RemoteAddr(), link.Adapters())
		}
	}
	return audio.ResolveTransport(kind, mode, route)
}

// startCapture starts sending a stream. If it is already running (e.g. the
// peer asked again after losing state), the format is re-announced instead.
func (a *App) startCapture(kind audio.StreamKind) {
	if a.audio == nil {
		return
	}
	if a.audio.StartCapture(kind, a.audioCapture[kind]) {
		return
	}
	frames, err := a.audioOut[kind].Reannounce()
	if err != nil {
		return // first format not captured yet; it will be announced shortly
	}
	a.sendAudioFrames(frames)
}

// reconfigureCaptures applies a transport/profile change to running captures.
func (a *App) reconfigureCaptures(reason string) {
	if a.audio == nil {
		return
	}
	_, profile := a.currentAudioTransportSettings()
	for _, kind := range []audio.StreamKind{audio.StreamDesktop, audio.StreamMic} {
		if !a.audio.IsCapturing(kind) {
			continue
		}
		frames, mode, err := a.audioOut[kind].Reconfigure(a.resolveAudioTransport(kind), profile)
		if err != nil {
			continue
		}
		log.Printf("%s capture: now %s/%s after %s", kind, mode, profile, reason)
		a.sendAudioFrames(frames)
	}
}

// acceptsInboundAudio reports whether received audio for kind should play.
// The controller only plays what it asked for, so frames still in flight
// after it turned a stream off don't restart playback.
func (a *App) acceptsInboundAudio(kind audio.StreamKind) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.sessionRole != "controller" {
		return true
	}
	if kind == audio.StreamMic {
		return a.micMode == "receive"
	}
	return a.audioMode == "remote"
}

// handleInboundAudioFrame processes transport, format and data frames of a
// stream strictly in arrival order (all are routed through audioInboundCh).
func (a *App) handleInboundAudioFrame(kind audio.StreamKind, f Frame) {
	if a.audio == nil {
		return
	}
	in := a.audioIn[kind]
	switch f.Type {
	case protocol.MsgAudioTransport, protocol.MsgMicTransport:
		if err := in.HandleTransport(f.Payload); err != nil {
			log.Printf("%s playback: %v", kind, err)
		}
	case protocol.MsgAudioFormat, protocol.MsgMicFormat:
		rate, ch, err := in.HandleFormat(f.Payload)
		if err != nil {
			log.Printf("%s playback: %v", kind, err)
			return
		}
		a.audio.Player(kind).SetSource(rate, ch)
		log.Printf("%s playback: receiving %d Hz, %d ch", kind, rate, ch)
	default:
		atomic.AddUint64(&a.recvAudioN, 1)
		samples, err := in.HandleData(f.Payload)
		if err != nil {
			if now := time.Now().Unix(); a.audioDecodeErrLogged.Swap(now) != now {
				log.Printf("%s playback: %v", kind, err)
			}
			return
		}
		if samples == nil || !a.acceptsInboundAudio(kind) {
			return
		}
		a.audio.Player(kind).Push(samples)
		a.audio.StartPlayback(kind)
	}
}

func (a *App) currentAudioLatencyState() (int, string, string, int) {
	a.mu.RLock()
	rttMs := a.latencyMs
	a.mu.RUnlock()
	transportMode := a.resolveAudioTransport(audio.StreamDesktop)
	_, profile := a.currentAudioTransportSettings()
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
	a.mu.Unlock()
	transportMode := a.resolveAudioTransport(audio.StreamDesktop)
	_, profile := a.currentAudioTransportSettings()
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
	if a.audio == nil {
		return
	}
	_, profile := a.currentAudioTransportSettings()
	if err := a.audio.SetQualityProfile(profile); err != nil {
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

func (a *App) mediaControlGenerationIsCurrent(generation uint64) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return generation == a.mediaControlGeneration
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
	if a.audio == nil {
		return
	}
	a.mu.RLock()
	capture, playback, mic, micPlayback := a.captureDeviceID, a.playbackDeviceID, a.micDeviceID, a.micPlaybackDeviceID
	a.mu.RUnlock()
	a.audio.SetCaptureDeviceID(capture)
	a.audio.SetPlaybackDeviceID(playback)
	a.audio.SetMicDeviceID(mic)
	a.audio.SetMicPlaybackDeviceID(micPlayback)
}

// reconcileControllerMediaState brings both streams in line with the
// controller's settings. Receivers are not reset here; the peer re-announces
// its format whenever it (re)starts capturing.
func (a *App) reconcileControllerMediaState(generation uint64) {
	a.applyAudioQualityProfile()
	a.stopAllAudio()
	a.stopAllMic()
	if !a.mediaControlGenerationIsCurrent(generation) {
		return
	}
	if a.transport.GetSession() == nil || !a.isController() {
		return
	}

	a.mu.RLock()
	mode, timing, mic := a.audioMode, a.audioTiming, a.micMode
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

func (a *App) GetAudioTransport() string {
	mode, _ := a.currentAudioTransportSettings()
	return mode
}

// SetAudioTransport selects the desktop-audio transport: "auto" (default),
// "pcm" or "opus". The microphone always uses Opus.
func (a *App) SetAudioTransport(mode string) {
	if !audio.ValidTransportMode(mode) {
		return
	}
	a.mu.Lock()
	old := a.audioTransportMode
	a.audioTransportMode = mode
	a.mu.Unlock()
	if old == mode {
		return
	}
	a.settings.Update(func(s *settings.Settings) { s.AudioTransport = mode })
	a.logAudioPipelineConfig("audio-transport-change")
	a.reconfigureCaptures("audio-transport-change")
}

func (a *App) GetAudioProfile() string {
	_, profile := a.currentAudioTransportSettings()
	return profile
}

func (a *App) SetAudioProfile(profile string) {
	if !audio.ValidProfile(profile) {
		return
	}
	a.mu.Lock()
	old := a.audioProfile
	a.audioProfile = profile
	a.mu.Unlock()
	if old == profile {
		return
	}
	a.settings.Update(func(s *settings.Settings) { s.AudioProfile = profile })
	a.applyAudioQualityProfile()
	a.logAudioPipelineConfig("audio-profile-change")
	a.reconfigureCaptures("audio-profile-change")
}

func (a *App) audioPlaying() bool {
	return a.audio != nil && (a.audio.IsPlaying(audio.StreamDesktop) || a.audio.IsPlaying(audio.StreamMic))
}

func (a *App) audioCapturing() bool {
	return a.audio != nil && (a.audio.IsCapturing(audio.StreamDesktop) || a.audio.IsCapturing(audio.StreamMic))
}
