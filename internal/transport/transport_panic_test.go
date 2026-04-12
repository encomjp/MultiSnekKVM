package transport

// Panic-recovery integration tests.
//
// Each test exercises a code path that would crash the process before the
// safeCall fixes were added.  The tests are structured to:
//   1. Trigger the panic via an adversarial callback.
//   2. Assert that the transport recovered gracefully (session state,
//      IsListening, ability to send further frames).
//
// Without the fixes every test here would crash the test binary with an
// unrecovered goroutine panic.

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"multisnekkvm/internal/protocol"
)

// TestConnRateLimitedNilMap verifies that connRateLimited does not panic when
// the Transport was not created via NewTransport (rateConns is nil).
func TestConnRateLimitedNilMap(t *testing.T) {
	tr := &Transport{} // zero-value — rateConns is nil
	// Before fix: panics "assignment to entry in nil map"
	if tr.connRateLimited("127.0.0.1") {
		t.Error("connRateLimited returned true for zero-value transport")
	}
	if tr.rateConns == nil {
		t.Error("rateConns was not lazily initialized by connRateLimited")
	}
}

// TestOnFramePanicReadLoopContinues verifies that a panic inside an OnFrame
// callback is recovered per-frame and the read loop keeps processing further
// frames on the same session.
func TestOnFramePanicReadLoopContinues(t *testing.T) {
	host, client, hostPort, clientDir := makeTransportPair(t)

	firstPanic := make(chan struct{})
	secondReceived := make(chan struct{})
	var callCount atomic.Int32

	host.OnFrame = func(f protocol.Frame) {
		n := callCount.Add(1)
		switch n {
		case 1:
			close(firstPanic)
			panic("intentional OnFrame panic (frame 1)")
		case 2:
			close(secondReceived)
		}
	}

	connectPair(t, host, client, hostPort, clientDir)

	// Frame 1: triggers the panic.
	if err := client.Send(protocol.Frame{Type: protocol.MsgSwitchBack}); err != nil {
		t.Fatalf("Send frame 1: %v", err)
	}
	select {
	case <-firstPanic:
	case <-time.After(2 * time.Second):
		t.Fatal("OnFrame was not called for frame 1 within 2s")
	}

	// Session must still be alive — readLoop continued after panic recovery.
	if host.GetSession() == nil {
		t.Fatal("host session is nil after OnFrame panic — readLoop did not continue")
	}

	// Frame 2: readLoop must process it, proving the loop survived.
	if err := client.Send(protocol.Frame{Type: protocol.MsgSwitchBack}); err != nil {
		t.Fatalf("Send frame 2: %v", err)
	}
	select {
	case <-secondReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("frame 2 was not received — readLoop did not survive the OnFrame panic")
	}
}

// TestOnConnectInboundPanicRecovered verifies that a panic in the host's
// OnConnect callback (fired in the handleInbound goroutine) is recovered,
// the session remains active, and subsequent frames are delivered.
func TestOnConnectInboundPanicRecovered(t *testing.T) {
	host, client, hostPort, clientDir := makeTransportPair(t)

	panicFired := make(chan struct{})
	host.OnConnect = func(id, name, role string) {
		close(panicFired)
		panic("intentional inbound OnConnect panic")
	}

	// connectPair wraps BOTH OnConnect callbacks; its host wrapper sends to
	// hostConnCh first, then calls our panicking original.  safeCall in
	// handleInbound catches the panic; readLoop still starts afterward.
	connectPair(t, host, client, hostPort, clientDir)

	select {
	case <-panicFired:
	case <-time.After(2 * time.Second):
		t.Fatal("host.OnConnect was not called within 2s")
	}
	time.Sleep(50 * time.Millisecond) // let readLoop goroutine start

	if host.GetSession() == nil {
		t.Error("host session nil after inbound OnConnect panic — readLoop did not start")
	}
	if err := client.Send(protocol.Frame{Type: protocol.MsgSwitchBack}); err != nil {
		t.Errorf("client.Send after inbound OnConnect panic: %v", err)
	}
}

// TestOnConnectOutboundPanicRecovered verifies that a panic in the client's
// OnConnect callback (fired on the ConnectTo call-site goroutine) is recovered,
// ConnectTo still returns nil, readLoop is started, and the session is usable.
func TestOnConnectOutboundPanicRecovered(t *testing.T) {
	host, client, hostPort, clientDir := makeTransportPair(t)

	panicFired := make(chan struct{})
	client.OnConnect = func(id, name, role string) {
		close(panicFired)
		panic("intentional outbound OnConnect panic")
	}

	// connectPair wraps client.OnConnect; its wrapper sends to clientConnCh
	// first, then calls our panicking original.  Without fix, the panic
	// propagates through ConnectTo to the test goroutine and crashes it.
	connectPair(t, host, client, hostPort, clientDir)

	select {
	case <-panicFired:
	case <-time.After(2 * time.Second):
		t.Fatal("client.OnConnect was not called within 2s")
	}
	time.Sleep(50 * time.Millisecond)

	if client.GetSession() == nil {
		t.Error("client session nil after outbound OnConnect panic — readLoop did not start")
	}
	// Host can send a frame to confirm the client session is fully live.
	if err := host.Send(protocol.Frame{Type: protocol.MsgSwitchBack}); err != nil {
		t.Errorf("host.Send after outbound OnConnect panic: %v", err)
	}
}

// TestOnDisconnectReadLoopPanicRecovered verifies that a panic in OnDisconnect
// fired from readLoop's defer (background goroutine) is recovered — the process
// survives, the session is cleaned up, and the host keeps listening.
func TestOnDisconnectReadLoopPanicRecovered(t *testing.T) {
	host, client, hostPort, clientDir := makeTransportPair(t)

	panicFired := make(chan struct{})
	var once sync.Once
	host.OnDisconnect = func() {
		once.Do(func() { close(panicFired) })
		panic("intentional readLoop OnDisconnect panic")
	}

	connectPair(t, host, client, hostPort, clientDir)

	// Drop client connection to trigger host's readLoop exit → OnDisconnect.
	client.Disconnect()

	select {
	case <-panicFired:
	case <-time.After(3 * time.Second):
		t.Fatal("host.OnDisconnect was not called within 3s")
	}
	time.Sleep(50 * time.Millisecond)

	if host.GetSession() != nil {
		t.Error("host.GetSession() not nil after session closed")
	}
	if !host.IsListening() {
		t.Error("host stopped listening after readLoop OnDisconnect panic")
	}
}

// TestOnDisconnectCallSitePanicRecovered verifies that a panic in OnDisconnect
// fired from Transport.Disconnect() (on the caller's goroutine) is recovered
// without propagating to the caller.
func TestOnDisconnectCallSitePanicRecovered(t *testing.T) {
	host, client, hostPort, clientDir := makeTransportPair(t)

	panicFired := make(chan struct{})
	client.OnDisconnect = func() {
		close(panicFired)
		panic("intentional Disconnect() OnDisconnect panic")
	}

	connectPair(t, host, client, hostPort, clientDir)

	// Without fix: panic propagates to this goroutine and crashes the test.
	client.Disconnect()

	select {
	case <-panicFired:
	case <-time.After(2 * time.Second):
		t.Fatal("client.OnDisconnect was not called within 2s")
	}

	if client.GetSession() != nil {
		t.Error("client session not nil after Disconnect()")
	}
}
