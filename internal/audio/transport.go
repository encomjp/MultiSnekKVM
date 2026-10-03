package audio

import (
	"fmt"
	"time"
)

const (
	audioTransportAuto = "auto"
	audioTransportPCM  = "pcm"
	audioTransportOpus = "opus"

	audioProfileLowLatency = "low-latency"
	audioProfileBalanced   = "balanced"
	audioProfileMusic      = "music"

	// TransportAuto, TransportPCM and TransportOpus are the user-selectable
	// desktop-audio transports. The microphone always uses Opus.
	TransportAuto = audioTransportAuto
	TransportPCM  = audioTransportPCM
	TransportOpus = audioTransportOpus
)

type audioProfileSpec struct {
	Name                 string
	TargetClientDuration time.Duration // WASAPI shared-mode buffer request
	OpusFrameDuration    time.Duration
	OpusBitrate          int
	OpusComplexity       int
	OpusRestrictedDelay  bool
}

// NormalizeTransportMode maps unknown values to the default, "auto".
func NormalizeTransportMode(mode string) string {
	switch mode {
	case audioTransportPCM, audioTransportOpus:
		return mode
	default:
		return audioTransportAuto
	}
}

func ValidTransportMode(mode string) bool {
	return NormalizeTransportMode(mode) == mode
}

// ResolveTransport picks the wire transport for a stream. "auto" sends
// uncompressed PCM only over direct wired links (USB4, USB network bridges,
// Ethernet) and Opus over everything else (Wi-Fi, Tailscale, Bluetooth,
// unknown routes), where PCM's ~1.5 Mbit/s per stream causes stalls.
// The microphone always uses Opus.
func ResolveTransport(kind StreamKind, mode, routeKind string) string {
	// Bluetooth (direct or PAN) is far too slow for raw PCM whatever the
	// setting says; the microphone is always compressed.
	if kind == StreamMic || routeKind == "bluetooth" {
		return audioTransportOpus
	}
	switch NormalizeTransportMode(mode) {
	case audioTransportPCM:
		return audioTransportPCM
	case audioTransportOpus:
		return audioTransportOpus
	}
	switch routeKind {
	case "usb4", "usb-bridge", "ethernet":
		return audioTransportPCM
	default:
		return audioTransportOpus
	}
}

// bluetoothMaxOpusBitrate keeps desktop audio plus input comfortably inside
// Classic Bluetooth's real-world throughput (about 1-2 Mbit/s shared).
const bluetoothMaxOpusBitrate = 96000

// LinkBitrateCap returns the maximum Opus bitrate for a route kind, or 0
// for no cap.
func LinkBitrateCap(routeKind string) int {
	if routeKind == "bluetooth" {
		return bluetoothMaxOpusBitrate
	}
	return 0
}

func NormalizeProfile(profile string) string {
	switch profile {
	case audioProfileLowLatency, audioProfileMusic:
		return profile
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
			TargetClientDuration: 10 * time.Millisecond,
			OpusFrameDuration:    10 * time.Millisecond,
			OpusBitrate:          128000,
			OpusComplexity:       5,
			OpusRestrictedDelay:  true,
		}
	case audioProfileMusic:
		return audioProfileSpec{
			Name:                 audioProfileMusic,
			TargetClientDuration: 20 * time.Millisecond,
			OpusFrameDuration:    20 * time.Millisecond,
			OpusBitrate:          256000,
			OpusComplexity:       10,
		}
	default:
		return audioProfileSpec{
			Name:                 audioProfileBalanced,
			TargetClientDuration: 20 * time.Millisecond,
			OpusFrameDuration:    20 * time.Millisecond,
			OpusBitrate:          160000,
			OpusComplexity:       8,
		}
	}
}

func encodeAudioTransportMode(mode string) []byte {
	if mode == audioTransportOpus {
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

// estimatedPlaybackBaseLatency is the local buffering part of the path:
// device buffer + jitter-buffer target (+ one Opus frame).
func estimatedPlaybackBaseLatency(transportMode, profile string) time.Duration {
	spec := audioProfileSpecForName(profile)
	base := spec.TargetClientDuration + playoutConfigForProfile(profile).Target
	if transportMode == audioTransportOpus {
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
