package audio

import "testing"

func TestEstimatedPlaybackBaseLatencyMs(t *testing.T) {
	if got := EstimatedPlaybackBaseLatencyMs(TransportPCM, audioProfileBalanced); got != 60 {
		t.Fatalf("balanced pcm base latency = %dms, want 60ms", got)
	}
	if got := EstimatedPlaybackBaseLatencyMs(audioTransportOpus, audioProfileLowLatency); got != 40 {
		t.Fatalf("low-latency opus base latency = %dms, want 40ms", got)
	}
}

func TestEstimatedPlaybackLatencyMs(t *testing.T) {
	if got := EstimatedPlaybackLatencyMs(20, TransportPCM, audioProfileBalanced); got != 70 {
		t.Fatalf("balanced pcm latency = %dms, want 70ms", got)
	}
	if got := EstimatedPlaybackLatencyMs(19, audioTransportOpus, audioProfileLowLatency); got != 50 {
		t.Fatalf("low-latency opus latency = %dms, want 50ms", got)
	}
	if got := EstimatedPlaybackLatencyMs(-1, TransportPCM, audioProfileBalanced); got != -1 {
		t.Fatalf("unknown RTT latency = %dms, want -1ms", got)
	}
}
