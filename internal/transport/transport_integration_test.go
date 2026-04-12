package transport

// End-to-end integration tests: two Transport instances talking on localhost.
//
// These tests cover the full protocol stack — TLS handshake, hello exchange,
// pairing-code validation, frame delivery, heartbeat filtering, listener
// cleanup on unexpected death, concurrent sends, and reconnect.
//
// Each test creates ephemeral TLS identities (cert+key written to t.TempDir()
// under MultiSnekKVM/identity/) so that identity.CertPath() / identity.KeyPath()
// resolve correctly when APPDATA is set to that dir during Start / ConnectTo.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"multisnekkvm/internal/identity"
	"multisnekkvm/internal/protocol"
	"multisnekkvm/internal/trust"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// makeEphemeralIdentity generates an ephemeral ECDSA P-256 key + self-signed
// TLS cert, writes them under dir/MultiSnekKVM/identity/ (the layout that
// identity.CertPath() / identity.KeyPath() expect when APPDATA=dir), and
// returns a DeviceInfo ready to pass to NewTransport.
func makeEphemeralIdentity(t *testing.T, dir, name string) identity.DeviceInfo {
	t.Helper()
	identDir := filepath.Join(dir, "MultiSnekKVM", "identity")
	if err := os.MkdirAll(identDir, 0o700); err != nil {
		t.Fatalf("mkdir identity: %v", err)
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand device id: %v", err)
	}
	deviceID := hex.EncodeToString(buf)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   name,
			Organization: []string{"MultiSnekKVM"},
			SerialNumber: deviceID,
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(filepath.Join(identDir, "device-cert.pem"), certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(identDir, "device-key.pem"), keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	fp := sha256.Sum256(cert.Raw)
	return identity.DeviceInfo{
		ID:          deviceID,
		Name:        name,
		Fingerprint: hex.EncodeToString(fp[:]),
		Port:        24831,
	}
}

// openTrustStoreAt opens a trust store rooted at dir (APPDATA=dir).
func openTrustStoreAt(t *testing.T, dir string) *trust.Store {
	t.Helper()
	orig := os.Getenv("APPDATA")
	os.Setenv("APPDATA", dir)
	store, err := trust.OpenStore()
	os.Setenv("APPDATA", orig)
	if err != nil {
		t.Fatalf("open trust store at %s: %v", dir, err)
	}
	return store
}

// withAppData sets APPDATA=dir, calls fn, then restores.
func withAppData(dir string, fn func()) {
	orig := os.Getenv("APPDATA")
	os.Setenv("APPDATA", dir)
	fn()
	os.Setenv("APPDATA", orig)
}

// makeTransportPair creates two Transports (host + client) with ephemeral
// identities. The host is already listening; call connectPair to dial.
//
// Returns: host, client, hostPort, clientDir.
// clientDir must be passed to connectPair so APPDATA is set correctly.
func makeTransportPair(t *testing.T) (host *Transport, client *Transport, hostPort int, clientDir string) {
	t.Helper()
	hostDir := t.TempDir()
	clientDir = t.TempDir()

	hostDev := makeEphemeralIdentity(t, hostDir, "IntegTestHost")
	clientDev := makeEphemeralIdentity(t, clientDir, "IntegTestClient")

	host = NewTransport(hostDev, nil, openTrustStoreAt(t, hostDir))
	host.SetPairingCode("654321")
	client = NewTransport(clientDev, nil, openTrustStoreAt(t, clientDir))

	var startErr error
	withAppData(hostDir, func() { startErr = host.Start(0) })
	if startErr != nil {
		t.Fatalf("host Start: %v", startErr)
	}
	t.Cleanup(func() { host.Stop() })

	hostPort = host.listener.Addr().(*net.TCPAddr).Port
	return host, client, hostPort, clientDir
}

// connectPair dials the client to the host and waits for both OnConnect
// callbacks to fire. It fatals the test on timeout.
func connectPair(t *testing.T, host, client *Transport, hostPort int, clientDir string) {
	t.Helper()
	hostConnCh := make(chan string, 1)
	clientConnCh := make(chan string, 1)
	origHostConn := host.OnConnect
	origClientConn := client.OnConnect
	host.OnConnect = func(id, name, role string) {
		hostConnCh <- role
		if origHostConn != nil {
			origHostConn(id, name, role)
		}
	}
	client.OnConnect = func(id, name, role string) {
		clientConnCh <- role
		if origClientConn != nil {
			origClientConn(id, name, role)
		}
	}

	var connErr error
	withAppData(clientDir, func() {
		connErr = client.ConnectTo(fmt.Sprintf("127.0.0.1:%d", hostPort), "654321")
	})
	if connErr != nil {
		t.Fatalf("client ConnectTo: %v", connErr)
	}
	t.Cleanup(func() { client.Disconnect() })

	for _, ch := range []chan string{hostConnCh, clientConnCh} {
		select {
		case <-ch:
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for OnConnect")
		}
	}
}

// waitFrames blocks until at least n frames have been collected into *frames
// (guarded by mu), or fatals after the deadline.
func waitFrames(t *testing.T, frames *[]protocol.Frame, mu *sync.Mutex, n int, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for {
		mu.Lock()
		count := len(*frames)
		mu.Unlock()
		if count >= n {
			return
		}
		if time.Now().After(end) {
			mu.Lock()
			count = len(*frames)
			mu.Unlock()
			t.Fatalf("received %d/%d frames within %v", count, n, deadline)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestTransportEndToEndFrameDelivery verifies that frames sent from the
// controller reach the controlled peer with the correct type and payload.
func TestTransportEndToEndFrameDelivery(t *testing.T) {
	hostDir := t.TempDir()
	clientDir := t.TempDir()
	hostDev := makeEphemeralIdentity(t, hostDir, "IntegTestHost")
	clientDev := makeEphemeralIdentity(t, clientDir, "IntegTestClient")

	host := NewTransport(hostDev, nil, openTrustStoreAt(t, hostDir))
	host.SetPairingCode("123456")
	client := NewTransport(clientDev, nil, openTrustStoreAt(t, clientDir))

	var hostFrames []protocol.Frame
	var hostMu sync.Mutex
	var clientFrames []protocol.Frame
	var clientMu sync.Mutex
	hostConnCh := make(chan string, 1)
	clientConnCh := make(chan string, 1)

	host.OnFrame = func(f protocol.Frame) {
		hostMu.Lock()
		hostFrames = append(hostFrames, f)
		hostMu.Unlock()
	}
	host.OnConnect = func(_, _, role string) { hostConnCh <- role }
	client.OnFrame = func(f protocol.Frame) {
		clientMu.Lock()
		clientFrames = append(clientFrames, f)
		clientMu.Unlock()
	}
	client.OnConnect = func(_, _, role string) { clientConnCh <- role }

	var startErr error
	withAppData(hostDir, func() { startErr = host.Start(0) })
	if startErr != nil {
		t.Fatalf("host Start: %v", startErr)
	}
	t.Cleanup(func() { host.Stop() })
	hostPort := host.listener.Addr().(*net.TCPAddr).Port

	var connErr error
	withAppData(clientDir, func() {
		connErr = client.ConnectTo(fmt.Sprintf("127.0.0.1:%d", hostPort), "123456")
	})
	if connErr != nil {
		t.Fatalf("client ConnectTo: %v", connErr)
	}
	t.Cleanup(func() { client.Disconnect() })

	// Both sides should report the correct role.
	select {
	case role := <-hostConnCh:
		if role != "controlled" {
			t.Errorf("host role = %q, want controlled", role)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("host OnConnect timeout")
	}
	select {
	case role := <-clientConnCh:
		if role != "controller" {
			t.Errorf("client role = %q, want controller", role)
		}
	case <-time.After(time.Second):
		t.Fatal("client OnConnect timeout")
	}

	// Send frames client → host (normal control flow).
	clientToHost := []protocol.Frame{
		{Type: protocol.MsgMouseMove, Payload: protocol.MouseMoveMsg{DX: 10, DY: -5}.Encode()},
		{Type: protocol.MsgKeyDown, Payload: protocol.KeyMsg{VKCode: 0x41, ScanCode: 0x1e}.Encode()},
		{Type: protocol.MsgKeyUp, Payload: protocol.KeyMsg{VKCode: 0x41, ScanCode: 0x1e}.Encode()},
		{Type: protocol.MsgMouseClick, Payload: protocol.MouseClickMsg{Button: 0, Pressed: true}.Encode()},
		{Type: protocol.MsgMouseScroll, Payload: protocol.MouseScrollMsg{Delta: 3}.Encode()},
	}
	for i, f := range clientToHost {
		if err := client.Send(f); err != nil {
			t.Fatalf("client Send frame[%d]: %v", i, err)
		}
	}
	waitFrames(t, &hostFrames, &hostMu, len(clientToHost), 3*time.Second)

	hostMu.Lock()
	for i, want := range clientToHost {
		if hostFrames[i].Type != want.Type {
			t.Errorf("hostFrames[%d] type = 0x%02x, want 0x%02x", i, hostFrames[i].Type, want.Type)
		}
		if string(hostFrames[i].Payload) != string(want.Payload) {
			t.Errorf("hostFrames[%d] payload mismatch", i)
		}
	}
	hostMu.Unlock()

	// Send a frame host → client (e.g. MsgSwitchBack).
	switchBack := protocol.Frame{Type: protocol.MsgSwitchBack}
	if err := host.Send(switchBack); err != nil {
		t.Fatalf("host Send MsgSwitchBack: %v", err)
	}
	waitFrames(t, &clientFrames, &clientMu, 1, 2*time.Second)
	clientMu.Lock()
	if clientFrames[0].Type != protocol.MsgSwitchBack {
		t.Errorf("client received 0x%02x, want MsgSwitchBack", clientFrames[0].Type)
	}
	clientMu.Unlock()
}

// TestTransportHeartbeatsNotDeliveredToOnFrame verifies that heartbeat frames
// are silently consumed by the read loop and never forwarded to OnFrame.
func TestTransportHeartbeatsNotDeliveredToOnFrame(t *testing.T) {
	host, client, hostPort, clientDir := makeTransportPair(t)

	var hostFrameCount int
	var mu sync.Mutex
	host.OnFrame = func(f protocol.Frame) {
		mu.Lock()
		hostFrameCount++
		mu.Unlock()
	}

	connectPair(t, host, client, hostPort, clientDir)

	// Send several heartbeats + one real frame.
	for i := 0; i < 5; i++ {
		if err := client.Send(protocol.Frame{Type: protocol.MsgHeartbeat}); err != nil {
			t.Fatalf("send heartbeat %d: %v", i, err)
		}
	}
	realFrame := protocol.Frame{Type: protocol.MsgMouseMove, Payload: protocol.MouseMoveMsg{DX: 1, DY: 1}.Encode()}
	if err := client.Send(realFrame); err != nil {
		t.Fatalf("send real frame: %v", err)
	}

	// Wait for the real frame to arrive as a synchronisation point.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := hostFrameCount
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real frame not received within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Give any spurious heartbeat deliveries time to appear.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	final := hostFrameCount
	mu.Unlock()
	if final != 1 {
		t.Errorf("OnFrame called %d times after 5 heartbeats + 1 real frame; want exactly 1", final)
	}
}

// TestTransportPairingCodeRejected verifies that ConnectTo returns an error
// when the client presents the wrong pairing PIN.
func TestTransportPairingCodeRejected(t *testing.T) {
	hostDir := t.TempDir()
	clientDir := t.TempDir()

	hostDev := makeEphemeralIdentity(t, hostDir, "RejectHost")
	clientDev := makeEphemeralIdentity(t, clientDir, "RejectClient")

	host := NewTransport(hostDev, nil, openTrustStoreAt(t, hostDir))
	host.SetPairingCode("999999")
	client := NewTransport(clientDev, nil, openTrustStoreAt(t, clientDir))

	var startErr error
	withAppData(hostDir, func() { startErr = host.Start(0) })
	if startErr != nil {
		t.Fatalf("host Start: %v", startErr)
	}
	t.Cleanup(func() { host.Stop() })
	hostPort := host.listener.Addr().(*net.TCPAddr).Port

	var connErr error
	withAppData(clientDir, func() {
		connErr = client.ConnectTo(fmt.Sprintf("127.0.0.1:%d", hostPort), "000000")
	})
	if connErr == nil {
		t.Fatal("expected ConnectTo to fail with wrong pairing code, but it succeeded")
	}

	// Host should still be listening — not crashed by the rejected attempt.
	if !host.IsListening() {
		t.Error("host stopped listening after a rejected pairing attempt")
	}
}

// TestTransportIsListeningFalseAfterStop verifies that IsListening() returns
// false immediately after Stop() is called.
func TestTransportIsListeningFalseAfterStop(t *testing.T) {
	dir := t.TempDir()
	dev := makeEphemeralIdentity(t, dir, "StopHost")
	tr := NewTransport(dev, nil, openTrustStoreAt(t, dir))

	var startErr error
	withAppData(dir, func() { startErr = tr.Start(0) })
	if startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	if !tr.IsListening() {
		t.Fatal("expected IsListening() = true after Start")
	}

	tr.Stop()
	if tr.IsListening() {
		t.Fatal("expected IsListening() = false after Stop")
	}
}

// TestTransportIsListeningFalseAfterListenerDies verifies that IsListening()
// eventually returns false when the underlying net.Listener is closed from
// outside (simulating an OS-level error that kills the accept loop). This
// directly tests the conditional-defer fix added in the real-world bug audit.
func TestTransportIsListeningFalseAfterListenerDies(t *testing.T) {
	dir := t.TempDir()
	dev := makeEphemeralIdentity(t, dir, "DieHost")
	tr := NewTransport(dev, nil, openTrustStoreAt(t, dir))

	var startErr error
	withAppData(dir, func() { startErr = tr.Start(0) })
	if startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	if !tr.IsListening() {
		t.Fatal("expected IsListening() = true after Start")
	}

	// Grab the listener and close it from outside — this simulates the OS
	// forcibly closing the listening socket.
	tr.mu.RLock()
	l := tr.listener
	tr.mu.RUnlock()
	if l == nil {
		t.Fatal("expected tr.listener to be non-nil after Start")
	}
	l.Close()

	// acceptLoop must detect the closed listener and exit via the conditional
	// defer, setting t.listener = nil. Give it up to 1 second.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !tr.IsListening() {
			return // pass
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("IsListening() still true 1s after listener was closed externally; conditional-defer fix may be broken")
}

// TestTransportConcurrentSends verifies that multiple goroutines sending
// frames simultaneously do not corrupt the session or trigger data races.
// Run with: go test -race ./internal/transport/... -run TestTransportConcurrentSends
func TestTransportConcurrentSends(t *testing.T) {
	host, client, hostPort, clientDir := makeTransportPair(t)

	var received int
	var mu sync.Mutex
	host.OnFrame = func(f protocol.Frame) {
		mu.Lock()
		received++
		mu.Unlock()
	}

	connectPair(t, host, client, hostPort, clientDir)

	const senders = 8
	const framesEach = 50
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			f := protocol.Frame{
				Type:    protocol.MsgMouseMove,
				Payload: protocol.MouseMoveMsg{DX: int32(id), DY: int32(id)}.Encode(),
			}
			for j := 0; j < framesEach; j++ {
				if err := client.Send(f); err != nil {
					// Connection may have closed; that's acceptable under this load.
					return
				}
			}
		}(i)
	}
	wg.Wait()

	// Wait for all sent frames to drain to the host.
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := received
		mu.Unlock()
		if n >= senders*framesEach {
			return
		}
		if time.Now().After(deadline) {
			mu.Lock()
			final := received
			mu.Unlock()
			t.Fatalf("received %d/%d frames within 5s", final, senders*framesEach)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestTransportLargePayloadRoundtrip verifies that a near-maximum-size frame
// (close to MaxFramePayloadBytes) is delivered without corruption.
func TestTransportLargePayloadRoundtrip(t *testing.T) {
	host, client, hostPort, clientDir := makeTransportPair(t)

	doneCh := make(chan []byte, 1)
	host.OnFrame = func(f protocol.Frame) {
		if f.Type == protocol.MsgClipboard {
			doneCh <- append([]byte(nil), f.Payload...)
		}
	}
	connectPair(t, host, client, hostPort, clientDir)

	// 512 KB payload — well within the 1 MB limit, large enough to stress framing.
	payload := make([]byte, 512*1024)
	for i := range payload {
		payload[i] = byte(i & 0xff)
	}
	if err := client.Send(protocol.Frame{Type: protocol.MsgClipboard, Payload: payload}); err != nil {
		t.Fatalf("Send large frame: %v", err)
	}

	select {
	case got := <-doneCh:
		if len(got) != len(payload) {
			t.Fatalf("large payload length = %d, want %d", len(got), len(payload))
		}
		for i, b := range payload {
			if got[i] != b {
				t.Fatalf("payload byte[%d] = 0x%02x, want 0x%02x", i, got[i], b)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("large frame not received within 5s")
	}
}

// TestTransportReconnectAfterDisconnect verifies that both sides can
// re-establish a session after the client disconnects.
func TestTransportReconnectAfterDisconnect(t *testing.T) {
	hostDir := t.TempDir()
	clientDir := t.TempDir()

	hostDev := makeEphemeralIdentity(t, hostDir, "ReconHost")
	clientDev := makeEphemeralIdentity(t, clientDir, "ReconClient")

	hostTrust := openTrustStoreAt(t, hostDir)
	clientTrust := openTrustStoreAt(t, clientDir)

	host := NewTransport(hostDev, nil, hostTrust)
	host.SetPairingCode("111111")
	client := NewTransport(clientDev, nil, clientTrust)

	var startErr error
	withAppData(hostDir, func() { startErr = host.Start(0) })
	if startErr != nil {
		t.Fatalf("host Start: %v", startErr)
	}
	t.Cleanup(func() { host.Stop() })
	hostPort := host.listener.Addr().(*net.TCPAddr).Port

	hostDiscCh := make(chan struct{}, 2)
	host.OnDisconnect = func() { hostDiscCh <- struct{}{} }

	// First connection.
	var connErr error
	withAppData(clientDir, func() {
		connErr = client.ConnectTo(fmt.Sprintf("127.0.0.1:%d", hostPort), "111111")
	})
	if connErr != nil {
		t.Fatalf("first ConnectTo: %v", connErr)
	}

	client.Disconnect()
	select {
	case <-hostDiscCh:
	case <-time.After(2 * time.Second):
		t.Fatal("host did not detect first disconnect within 2s")
	}

	// Pairing code was consumed on first connect; host rotated to a new one.
	// Since the peers are now trusted (pinned), reconnect without pairing code.
	withAppData(clientDir, func() {
		connErr = client.ConnectTo(fmt.Sprintf("127.0.0.1:%d", hostPort), "")
	})
	if connErr != nil {
		t.Fatalf("second ConnectTo (trusted reconnect): %v", connErr)
	}
	t.Cleanup(func() { client.Disconnect() })

	// Verify the session is up by sending a frame.
	receivedCh := make(chan struct{}, 1)
	host.OnFrame = func(f protocol.Frame) {
		if f.Type == protocol.MsgHeartbeat {
			return
		}
		select {
		case receivedCh <- struct{}{}:
		default:
		}
	}
	if err := client.Send(protocol.Frame{Type: protocol.MsgMouseMove, Payload: protocol.MouseMoveMsg{DX: 1}.Encode()}); err != nil {
		t.Fatalf("send after reconnect: %v", err)
	}
	select {
	case <-receivedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("frame not received after reconnect")
	}
}
