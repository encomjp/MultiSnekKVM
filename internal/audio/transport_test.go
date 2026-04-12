package audio

import "testing"

func TestEstimatedPlaybackBaseLatencyMs(t *testing.T) {
	if got := EstimatedPlaybackBaseLatencyMs(TransportPCM, audioProfileBalanced); got != 45 {
		t.Fatalf("balanced pcm base latency = %dms, want 45ms", got)
	}
	if got := EstimatedPlaybackBaseLatencyMs(audioTransportOpus, audioProfileLowLatency); got != 37 {
		t.Fatalf("low-latency opus base latency = %dms, want 37ms", got)
	}
}

func TestEstimatedPlaybackLatencyMs(t *testing.T) {
	if got := EstimatedPlaybackLatencyMs(20, TransportPCM, audioProfileBalanced); got != 55 {
		t.Fatalf("balanced pcm latency = %dms, want 55ms", got)
	}
	if got := EstimatedPlaybackLatencyMs(19, audioTransportOpus, audioProfileLowLatency); got != 47 {
		t.Fatalf("low-latency opus latency = %dms, want 47ms", got)
	}
	if got := EstimatedPlaybackLatencyMs(-1, TransportPCM, audioProfileBalanced); got != -1 {
		t.Fatalf("unknown RTT latency = %dms, want -1ms", got)
	}
}
