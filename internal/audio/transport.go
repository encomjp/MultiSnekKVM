package audio

import (
	"fmt"
	"time"

	"multisnekkvm/internal/protocol"
)

const (
	audioTransportPCM  = "pcm"
	audioTransportOpus = "opus"

	audioProfileLowLatency = "low-latency"
	audioProfileBalanced   = "balanced"
	audioProfileMusic      = "music"

	// TransportPCM is the exported constant for use by the root package.
	TransportPCM = audioTransportPCM
)

type audioProfileSpec struct {
	Name                 string
	TargetClientDuration time.Duration
	PrebufferDuration    time.Duration
	MaxBufferedDuration  time.Duration
	ReprimeThreshold     time.Duration
	OpusFrameDuration    time.Duration
	OpusBitrate          int
	OpusComplexity       int
	OpusRestrictedDelay  bool
	OpusEnableDTX        bool
	OpusEnableFEC        bool
	OpusPacketLossPct    int
}

// OutboundRealtimeStream encodes and frames captured audio for sending.
type OutboundRealtimeStream interface {
	Configure(transportMode, profile string, format []byte) ([]protocol.Frame, error)
	ProcessData(payload []byte) ([]protocol.Frame, error)
	Reset()
}

// InboundRealtimeStream decodes received audio frames for playback.
type InboundRealtimeStream interface {
	SetTransport(transportMode string) error
	ProcessFormat(payload []byte) ([]byte, error)
	ProcessData(payload []byte) ([]byte, error)
	Reset()
}

func NormalizeTransportMode(mode string) string {
	switch mode {
	case audioTransportOpus:
		return audioTransportOpus
	default:
		return audioTransportPCM
	}
}

func ValidTransportMode(mode string) bool {
	return NormalizeTransportMode(mode) == mode
}

func NormalizeProfile(profile string) string {
	switch profile {
	case audioProfileLowLatency:
		return audioProfileLowLatency
	case audioProfileMusic:
		return audioProfileMusic
	default:
		return audioProfileBalanced
	}
}

func ValidProfile(profile string) bool {
	return NormalizeProfile(profile) == profile
}

func audioProfileSpecForName(profile string) audioProfileSpec {
	switch NormalizeProfile(profile) {
	case audioProfileLowLatency:
		return audioProfileSpec{
			Name:                 audioProfileLowLatency,
			TargetClientDuration: 12 * time.Millisecond,
			PrebufferDuration:    15 * time.Millisecond,
			MaxBufferedDuration:  80 * time.Millisecond,
			ReprimeThreshold:     30 * time.Millisecond,
			OpusFrameDuration:    10 * time.Millisecond,
			OpusBitrate:          96000,
			OpusComplexity:       4,
			OpusRestrictedDelay:  true,
			OpusEnableDTX:        true,
			OpusEnableFEC:        true,
			OpusPacketLossPct:    15,
		}
	case audioProfileMusic:
		return audioProfileSpec{
			Name:                 audioProfileMusic,
			TargetClientDuration: 20 * time.Millisecond,
			PrebufferDuration:    35 * time.Millisecond,
			MaxBufferedDuration:  140 * time.Millisecond,
			ReprimeThreshold:     70 * time.Millisecond,
			OpusFrameDuration:    20 * time.Millisecond,
			OpusBitrate:          192000,
			OpusComplexity:       10,
			OpusRestrictedDelay:  false,
			OpusEnableDTX:        false,
			OpusEnableFEC:        false,
			OpusPacketLossPct:    5,
		}
	default:
		return audioProfileSpec{
			Name:                 audioProfileBalanced,
			TargetClientDuration: 20 * time.Millisecond,
			PrebufferDuration:    25 * time.Millisecond,
			MaxBufferedDuration:  100 * time.Millisecond,
			ReprimeThreshold:     50 * time.Millisecond,
			OpusFrameDuration:    20 * time.Millisecond,
			OpusBitrate:          128000,
			OpusComplexity:       6,
			OpusRestrictedDelay:  false,
			OpusEnableDTX:        true,
			OpusEnableFEC:        true,
			OpusPacketLossPct:    10,
		}
	}
}

func encodeAudioTransportMode(mode string) []byte {
	if NormalizeTransportMode(mode) == audioTransportOpus {
		return []byte{1}
	}
	return []byte{0}
}

func DecodeTransportMode(payload []byte) (string, error) {
	if len(payload) != 1 {
		return "", fmt.Errorf("invalid audio transport payload length=%d", len(payload))
	}
	switch payload[0] {
	case 0:
		return audioTransportPCM, nil
	case 1:
		return audioTransportOpus, nil
	default:
		return "", fmt.Errorf("unknown audio transport mode %d", payload[0])
	}
}

func estimatedPlaybackBaseLatency(transportMode, profile string) time.Duration {
	spec := audioProfileSpecForName(profile)
	base := spec.TargetClientDuration + spec.PrebufferDuration
	if NormalizeTransportMode(transportMode) == audioTransportOpus {
		base += spec.OpusFrameDuration
	}
	return base
}

func roundedDurationMilliseconds(duration time.Duration) int {
	if duration <= 0 {
		return 0
	}
	return int((duration + time.Millisecond/2) / time.Millisecond)
}

// EstimatedPlaybackBaseLatencyMs returns the local buffering portion of the audio path.
func EstimatedPlaybackBaseLatencyMs(transportMode, profile string) int {
	return roundedDurationMilliseconds(estimatedPlaybackBaseLatency(transportMode, profile))
}

// EstimatedPlaybackLatencyMs returns a rough end-to-end playback estimate using current RTT.
func EstimatedPlaybackLatencyMs(rttMs int, transportMode, profile string) int {
	if rttMs < 0 {
		return -1
	}
	oneWay := time.Duration(rttMs) * time.Millisecond / 2
	return roundedDurationMilliseconds(estimatedPlaybackBaseLatency(transportMode, profile) + oneWay)
}
