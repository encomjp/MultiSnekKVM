//go:build windows

package input

import (
	"sync/atomic"
	"testing"
	"time"

	"multisnekkvm/internal/protocol"
)

func TestOpenClipboardWithRetryEventuallySucceeds(t *testing.T) {
	originalOpen := openClipboardFn
	originalSleep := clipboardRetrySleep
	defer func() {
		openClipboardFn = originalOpen
		clipboardRetrySleep = originalSleep
	}()

	attempts := 0
	sleeps := 0
	openClipboardFn = func() bool {
		attempts++
		return attempts == 3
	}
	clipboardRetrySleep = func(_ time.Duration) {
		sleeps++
	}

	if !openClipboardWithRetry() {
		t.Fatal("expected clipboard open retry to eventually succeed")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if sleeps != 2 {
		t.Fatalf("sleep calls = %d, want 2", sleeps)
	}
}

func TestOpenClipboardWithRetryStopsAfterMaxAttempts(t *testing.T) {
	originalOpen := openClipboardFn
	originalSleep := clipboardRetrySleep
	defer func() {
		openClipboardFn = originalOpen
		clipboardRetrySleep = originalSleep
	}()

	attempts := 0
	sleeps := 0
	openClipboardFn = func() bool {
		attempts++
		return false
	}
	clipboardRetrySleep = func(_ time.Duration) {
		sleeps++
	}

	if openClipboardWithRetry() {
		t.Fatal("expected clipboard open retry to fail after max attempts")
	}
	if attempts != clipboardOpenMaxAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, clipboardOpenMaxAttempts)
	}
	if sleeps != clipboardOpenMaxAttempts-1 {
		t.Fatalf("sleep calls = %d, want %d", sleeps, clipboardOpenMaxAttempts-1)
	}
}

func TestSendRemoteWakeSendsZeroDeltaMouseMove(t *testing.T) {
	var frames []protocol.Frame
	ih := &InputHook{}
	ih.sendFn = func(f protocol.Frame) {
		frames = append(frames, f)
	}

	ih.sendRemoteWake()

	if len(frames) != 1 {
		t.Fatalf("frame count = %d, want 1", len(frames))
	}
	if frames[0].Type != protocol.MsgMouseMove {
		t.Fatalf("frame type = %d, want %d", frames[0].Type, protocol.MsgMouseMove)
	}
	m, err := protocol.DecodeMouseMove(frames[0].Payload)
	if err != nil {
		t.Fatalf("decode wake move: %v", err)
	}
	if m.DX != 0 || m.DY != 0 {
		t.Fatalf("wake move = %+v, want zero delta", m)
	}
}

func TestSetConnectedDisconnectTimesOutWhenRemoteLoopDoesNotExit(t *testing.T) {
	originalPostQuit := postThreadQuitMessage
	originalTimeout := edgeStopWaitTimeout
	defer func() {
		postThreadQuitMessage = originalPostQuit
		edgeStopWaitTimeout = originalTimeout
	}()

	postThreadQuitMessage = func(threadID uint32) bool {
		_ = threadID
		return false
	}
	edgeStopWaitTimeout = 10 * time.Millisecond

	ih := &InputHook{
		connected:    true,
		inRemoteMode: true,
		hookThreadID: 123,
		edgeStopCh:   make(chan struct{}),
		edgeDoneCh:   make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		ih.SetConnected(false, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("SetConnected(false) blocked waiting for a stuck remote loop")
	}
}

func TestEvaluateTriggerPointRequiresLeavingEdgeBeforeRearm(t *testing.T) {
	ih := &InputHook{
		edgeSide:     "right",
		triggerArmed: false,
		virtLeft:     0,
		virtRight:    100,
		virtTop:      0,
		virtBottom:   100,
	}

	active, armed := ih.evaluateTriggerPoint(point{X: 100, Y: 50})
	if !active || armed {
		t.Fatalf("edge point got active=%v armed=%v, want active=true armed=false", active, armed)
	}

	active, armed = ih.evaluateTriggerPoint(point{X: 50, Y: 50})
	if active || armed {
		t.Fatalf("off-edge point should reset arming, got active=%v armed=%v", active, armed)
	}

	active, armed = ih.evaluateTriggerPoint(point{X: 100, Y: 50})
	if !active || !armed {
		t.Fatalf("edge point after leaving should be armed, got active=%v armed=%v", active, armed)
	}
}

func TestReturnPositionPreservesTriggerHeightWithoutReturnAnchor(t *testing.T) {
	ih := &InputHook{
		edgeSide:            "right",
		virtLeft:            0,
		virtRight:           999,
		virtTop:             0,
		virtBottom:          699,
		centerX:             500,
		centerY:             350,
		hasLastTriggerPoint: true,
		lastTriggerY:        123,
	}

	x, y := ih.returnPosition()

	if x != 959 || y != 123 {
		t.Fatalf("returnPosition = (%d,%d), want (959,123)", x, y)
	}
}

func TestNotifyStateChangeCoalescesConcurrentTransitions(t *testing.T) {
	originalTimeout := stateChangeWarnTimeout
	defer func() {
		stateChangeWarnTimeout = originalTimeout
	}()
	stateChangeWarnTimeout = time.Second

	ih := &InputHook{}
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	var calls atomic.Int32
	ih.SetOnStateChange(func() {
		calls.Add(1)
		started <- struct{}{}
		<-release
	})

	ih.notifyStateChange()
	<-started

	ih.notifyStateChange()

	select {
	case <-started:
		t.Fatal("second state change callback started before the first finished")
	case <-time.After(30 * time.Millisecond):
	}

	close(release)

	deadline := time.After(200 * time.Millisecond)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("state change callback count = %d, want 2", calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}
