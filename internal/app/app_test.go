package app

import (
	"sync/atomic"
	"testing"
	"time"

	"multisnekkvm/internal/discovery"
	"multisnekkvm/internal/protocol"
)

type fakeOutboundRealtimeStream struct {
	name   string
	events *[]string
}

func (f *fakeOutboundRealtimeStream) Configure(transportMode, profile string, format []byte) ([]protocol.Frame, error) {
	_ = transportMode
	_ = profile
	_ = format
	return nil, nil
}

func (f *fakeOutboundRealtimeStream) ProcessData(payload []byte) ([]protocol.Frame, error) {
	_ = payload
	return nil, nil
}

func (f *fakeOutboundRealtimeStream) Reset() {
	*f.events = append(*f.events, f.name+"-reset")
}

func TestPreferredRoutePrefersLAN(t *testing.T) {
	route := preferredRoute([]string{"tailscale", "lan", "manual"})
	if route != "lan" {
		t.Fatalf("expected lan to be preferred, got %q", route)
	}
}

func TestPeerSourceLabelUsesHybridForMultipleRoutes(t *testing.T) {
	label := peerSourceLabel([]string{"lan", "tailscale"})
	if label != "hybrid" {
		t.Fatalf("expected hybrid label, got %q", label)
	}
}

func TestNormalizePeerAddress(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		defaultPort int
		want        string
		wantErr     bool
	}{
		{name: "host adds default port", raw: "deskbox", defaultPort: 24831, want: "deskbox:24831"},
		{name: "ipv4 adds default port", raw: "192.168.0.10", defaultPort: 24831, want: "192.168.0.10:24831"},
		{name: "explicit port kept", raw: "deskbox:3000", defaultPort: 24831, want: "deskbox:3000"},
		{name: "bare ipv6 adds default port", raw: "2001:db8::1", defaultPort: 24831, want: "[2001:db8::1]:24831"},
	}

	for _, test := range tests {
		got, err := normalizePeerAddress(test.raw, test.defaultPort)
		if test.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error, got %q", test.name, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", test.name, err)
		}
		if got != test.want {
			t.Fatalf("%s: expected %q, got %q", test.name, test.want, got)
		}
	}
}

func TestIsAtReturnEdge(t *testing.T) {
	if !isAtReturnEdge("right", 10, 50, 10, 100, 0, 100) {
		t.Fatal("expected right-side controller to switch back at the left bound")
	}
	if !isAtReturnEdge("top", 50, 100, 0, 100, 0, 100) {
		t.Fatal("expected top-side controller to switch back at the bottom bound")
	}
	if isAtReturnEdge("left", 50, 50, 0, 100, 0, 100) {
		t.Fatal("did not expect return edge away from the matching boundary")
	}
}

func TestReconnectCandidatesForPrefersFreshDiscoveryAddresses(t *testing.T) {
	candidates, peerName := reconnectCandidatesFor(Settings{
		LastPeerID:   "peer-1",
		LastPeerName: "Deskbox",
		LastPeerAddr: map[string]string{
			"lan":       "192.168.0.10:24831",
			"tailscale": "100.64.0.10:24831",
		},
	}, []discovery.DiscoveredPeer{{
		DeviceID:  "peer-1",
		Addresses: []string{"192.168.0.42:24831", "100.64.0.42:24831"},
	}})

	if peerName != "Deskbox" {
		t.Fatalf("peer name = %q, want Deskbox", peerName)
	}
	if len(candidates) != 4 {
		t.Fatalf("candidate count = %d, want 4", len(candidates))
	}
	if candidates[0] != "192.168.0.42:24831" {
		t.Fatalf("fresh LAN candidate should be first, got %q", candidates[0])
	}
	if candidates[1] != "192.168.0.10:24831" {
		t.Fatalf("saved LAN candidate should remain after fresh LAN, got %q", candidates[1])
	}
	if candidates[2] != "100.64.0.42:24831" {
		t.Fatalf("fresh tailscale candidate should follow LAN entries, got %q", candidates[2])
	}
	if candidates[3] != "100.64.0.10:24831" {
		t.Fatalf("saved tailscale candidate should remain as fallback, got %q", candidates[3])
	}
}

func TestReconnectCandidatesForMissingPeer(t *testing.T) {
	candidates, peerName := reconnectCandidatesFor(Settings{}, nil)
	if len(candidates) != 0 || peerName != "" {
		t.Fatalf("expected no candidates for missing peer, got %v / %q", candidates, peerName)
	}
}

func TestHandleRemoteMouseMoveWakeClearsStaleRemoteKeys(t *testing.T) {
	originalInjectKey := InjectKey
	originalInjectMouseMove := InjectMouseMove
	defer func() {
		InjectKey = originalInjectKey
		InjectMouseMove = originalInjectMouseMove
	}()

	var keyUps int
	InjectKey = func(vkCode uint16, scanCode uint16, flags uint32, down bool) {
		if down {
			t.Fatalf("expected wake to release keys only")
		}
		keyUps++
	}
	var mouseMoves []protocol.MouseMoveMsg
	InjectMouseMove = func(dx, dy int32) {
		mouseMoves = append(mouseMoves, protocol.MouseMoveMsg{DX: dx, DY: dy})
	}

	a := &App{}
	a.handleFrame(Frame{
		Type:    MsgEdgeConfig,
		Payload: protocol.EdgeConfigMsg{EdgeSide: "right"}.Encode(),
	})
	a.remoteKeyState.Record(protocol.KeyMsg{VKCode: 0x11, ScanCode: 0x1d}, true)

	a.handleRemoteMouseMove(protocol.MouseMoveMsg{})

	if keyUps != 1 {
		t.Fatalf("released keys = %d, want 1", keyUps)
	}
	if len(mouseMoves) != 0 {
		t.Fatalf("mouse moves = %+v, want none (wake frame must not inject)", mouseMoves)
	}
	if released := a.remoteKeyState.ReleaseAll(); len(released) != 0 {
		t.Fatalf("expected wake to clear stale key state, got %+v", released)
	}
	if !a.peerControlActive {
		t.Fatal("expected wake to mark peer control active")
	}
	if atomic.LoadInt64(&a.lastRemoteInputNs) == 0 {
		t.Fatal("expected wake to stamp lastRemoteInputNs")
	}
}

func TestEdgeConfigDoesNotActivatePeerControl(t *testing.T) {
	a := &App{}

	a.handleFrame(Frame{
		Type:    MsgEdgeConfig,
		Payload: protocol.EdgeConfigMsg{EdgeSide: "left"}.Encode(),
	})

	if a.peerControlActive {
		t.Fatal("expected edge config to remain idle until peer control starts")
	}
	if atomic.LoadInt64(&a.lastRemoteInputNs) != 0 {
		t.Fatal("expected edge config to leave lastRemoteInputNs at zero")
	}
}

func TestCheckReturnEdgePausesPeerControlUntilNextWake(t *testing.T) {
	originalGetCursorPosition := GetCursorPosition
	originalGetScreenBounds := GetScreenBounds
	defer func() {
		GetCursorPosition = originalGetCursorPosition
		GetScreenBounds = originalGetScreenBounds
	}()

	GetCursorPosition = func() (x, y int32) { return 10, 50 }
	GetScreenBounds = func() (left, right, top, bottom int32) { return 10, 100, 0, 100 }

	a := &App{
		controllerEdgeSide: "right",
		peerControlActive:  true,
		muxHigh:            make(chan Frame, 1),
	}

	a.checkReturnEdge()

	select {
	case f := <-a.muxHigh:
		if f.Type != MsgSwitchBack {
			t.Fatalf("frame type = 0x%02x, want switch-back", f.Type)
		}
	default:
		t.Fatal("expected switch-back frame to be queued")
	}
	if a.peerControlActive {
		t.Fatal("expected return edge to pause peer control immediately")
	}
	if !a.peerControlRequiresWake {
		t.Fatal("expected return edge to require a fresh remote wake before more input")
	}
}

func TestPeerControlIgnoresLateMouseMoveUntilWake(t *testing.T) {
	originalInjectMouseMove := InjectMouseMove
	defer func() {
		InjectMouseMove = originalInjectMouseMove
	}()

	var injected []protocol.MouseMoveMsg
	InjectMouseMove = func(dx, dy int32) {
		injected = append(injected, protocol.MouseMoveMsg{DX: dx, DY: dy})
	}

	a := &App{
		peerControlRequiresWake: true,
	}

	a.handleRemoteMouseMove(protocol.MouseMoveMsg{DX: 7, DY: 3})
	if len(injected) != 0 {
		t.Fatalf("late mouse move should be ignored until wake, got %+v", injected)
	}

	a.handleRemoteMouseMove(protocol.MouseMoveMsg{})
	a.handleRemoteMouseMove(protocol.MouseMoveMsg{DX: 7, DY: 3})
	if len(injected) != 1 {
		t.Fatalf("expected first post-wake move to inject once, got %+v", injected)
	}
}

func TestRemoteKeyWatchdogReleasesStaleInjectedKeysAfterTimeout(t *testing.T) {
	originalInjectKey := InjectKey
	originalTimeout := remoteKeyIdleTimeout
	defer func() {
		InjectKey = originalInjectKey
		remoteKeyIdleTimeout = originalTimeout
	}()

	remoteKeyIdleTimeout = 20 * time.Millisecond
	releasedCh := make(chan struct{}, 1)
	InjectKey = func(vkCode uint16, scanCode uint16, flags uint32, down bool) {
		if down {
			return
		}
		select {
		case releasedCh <- struct{}{}:
		default:
		}
	}

	a := &App{}
	a.remoteKeyState.Record(protocol.KeyMsg{VKCode: 0x10, ScanCode: 0x2a}, true)
	a.touchRemoteKeyWatchdog()

	select {
	case <-releasedCh:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected watchdog to release stale remote key")
	}

	if released := a.remoteKeyState.ReleaseAll(); len(released) != 0 {
		t.Fatalf("expected watchdog to clear key state, got %+v", released)
	}
}

func TestReleaseInjectedRemoteKeysReleasesHeldMouseButtons(t *testing.T) {
	originalInjectMouseClick := InjectMouseClick
	originalReleaseAllModifiers := ReleaseAllModifiers
	defer func() {
		InjectMouseClick = originalInjectMouseClick
		ReleaseAllModifiers = originalReleaseAllModifiers
	}()

	var released []byte
	InjectMouseClick = func(button byte, pressed bool) {
		if !pressed {
			released = append(released, button)
		}
	}
	ReleaseAllModifiers = func() {}

	a := &App{}
	a.remoteMouseButtons[0] = true // LMB held
	a.remoteMouseButtons[2] = true // MMB held

	a.releaseInjectedRemoteKeys()

	if len(released) != 2 {
		t.Fatalf("expected 2 mouse button releases, got %d: %v", len(released), released)
	}
	if a.remoteMouseButtons[0] || a.remoteMouseButtons[1] || a.remoteMouseButtons[2] {
		t.Fatalf("expected all remoteMouseButtons cleared after release")
	}
}

func TestRemoteKeyWatchdogDoesNotFireForMouseButtonHoldOnly(t *testing.T) {
	originalInjectMouseClick := InjectMouseClick
	originalTimeout := remoteKeyIdleTimeout
	defer func() {
		InjectMouseClick = originalInjectMouseClick
		remoteKeyIdleTimeout = originalTimeout
	}()

	remoteKeyIdleTimeout = 20 * time.Millisecond
	var clicks int
	InjectMouseClick = func(button byte, pressed bool) {
		clicks++
	}

	a := &App{}
	a.remoteMouseButtons[0] = true // LMB held, no keyboard keys pressed
	a.touchRemoteKeyWatchdog()

	time.Sleep(100 * time.Millisecond)
	if clicks != 0 {
		t.Fatalf("watchdog should not fire for mouse-button-only hold, but got %d InjectMouseClick calls", clicks)
	}
}

func TestMsgMouseClickInvalidButtonIsNotTracked(t *testing.T) {
	originalInjectMouseClick := InjectMouseClick
	defer func() {
		InjectMouseClick = originalInjectMouseClick
	}()

	var injected []struct {
		button  byte
		pressed bool
	}
	InjectMouseClick = func(button byte, pressed bool) {
		injected = append(injected, struct {
			button  byte
			pressed bool
		}{button, pressed})
	}

	a := &App{}
	a.handleFrame(Frame{
		Type:    MsgMouseClick,
		Payload: protocol.MouseClickMsg{Button: 5, Pressed: true}.Encode(),
	})

	if a.remoteMouseButtons[0] || a.remoteMouseButtons[1] || a.remoteMouseButtons[2] {
		t.Fatalf("button index 5 should not affect remoteMouseButtons")
	}
	if len(injected) != 1 || injected[0].button != 5 || !injected[0].pressed {
		t.Fatalf("InjectMouseClick should still be called for untracked buttons, got %v", injected)
	}
}

func TestRestartPassiveOutboundCapturesStopsBeforeResetAndRestarts(t *testing.T) {
	originalAudioIsCapturing := audioIsCapturing
	originalAudioStopCapture := audioStopCapture
	originalAudioStartCapture := audioStartCapture
	originalAudioIsMicCapturing := audioIsMicCapturing
	originalAudioStopMicCapture := audioStopMicCapture
	originalAudioStartMicCapture := audioStartMicCapture
	defer func() {
		audioIsCapturing = originalAudioIsCapturing
		audioStopCapture = originalAudioStopCapture
		audioStartCapture = originalAudioStartCapture
		audioIsMicCapturing = originalAudioIsMicCapturing
		audioStopMicCapture = originalAudioStopMicCapture
		audioStartMicCapture = originalAudioStartMicCapture
	}()

	var events []string
	audioIsCapturing = func(a *AudioStreamer) bool {
		_ = a
		return true
	}
	audioStopCapture = func(a *AudioStreamer) {
		_ = a
		events = append(events, "audio-stop")
	}
	audioStartCapture = func(a *AudioStreamer, sendFn func(Frame)) error {
		_ = a
		_ = sendFn
		events = append(events, "audio-start")
		return nil
	}
	audioIsMicCapturing = func(a *AudioStreamer) bool {
		_ = a
		return true
	}
	audioStopMicCapture = func(a *AudioStreamer) {
		_ = a
		events = append(events, "mic-stop")
	}
	audioStartMicCapture = func(a *AudioStreamer, sendFn func(Frame)) error {
		_ = a
		_ = sendFn
		events = append(events, "mic-start")
		return nil
	}

	a := &App{
		audio:         &AudioStreamer{},
		sessionRole:   "controlled",
		audioOutbound: &fakeOutboundRealtimeStream{name: "audio-out", events: &events},
		micOutbound:   &fakeOutboundRealtimeStream{name: "mic-out", events: &events},
	}

	a.restartPassiveOutboundCaptures("audio-transport-change")

	want := []string{"audio-stop", "mic-stop", "audio-out-reset", "mic-out-reset", "audio-start", "mic-start"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i, event := range want {
		if events[i] != event {
			t.Fatalf("events[%d] = %q, want %q (all=%v)", i, events[i], event, events)
		}
	}
}

func TestUpdateSessionLatencyComputesEWMAJitter(t *testing.T) {
	a := &App{latencyMs: -1, latencyPrev: -1, jitterMs: -1}

	// First sample: no jitter yet (need two samples for a delta).
	a.updateSessionLatency(10)
	if a.latencyMs != 10 {
		t.Fatalf("latencyMs = %d, want 10", a.latencyMs)
	}
	if a.jitterMs != -1 {
		t.Fatalf("jitterMs after first sample = %d, want -1 (need two samples)", a.jitterMs)
	}

	// Second sample: delta = |20-10| = 10; first jitter measurement = 10.
	a.updateSessionLatency(20)
	if a.jitterMs != 10 {
		t.Fatalf("jitterMs after second sample = %d, want 10", a.jitterMs)
	}

	// Third sample: delta = |20-20| = 0; EWMA = (10*7 + 0) / 8 = 8 (integer division).
	a.updateSessionLatency(20)
	if a.jitterMs != 8 {
		t.Fatalf("jitterMs after stable sample = %d, want 8", a.jitterMs)
	}
}

func TestUpdateSessionLatencyResetsJitterOnReconnect(t *testing.T) {
	a := &App{latencyMs: -1, latencyPrev: -1, jitterMs: -1}

	a.updateSessionLatency(10)
	a.updateSessionLatency(30)
	if a.jitterMs < 0 {
		t.Fatalf("jitterMs should be non-negative after two samples, got %d", a.jitterMs)
	}

	// Simulate reconnect: reset all three fields.
	a.latencyMs = -1
	a.latencyPrev = -1
	a.jitterMs = -1

	// First sample after reconnect should not produce a jitter value.
	a.updateSessionLatency(5)
	if a.jitterMs != -1 {
		t.Fatalf("jitterMs should be -1 after reconnect+single sample, got %d", a.jitterMs)
	}
}
