package transport

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"multisnekkvm/internal/protocol"
)

// btLikeConn hides the *net.TCPConn and reports a "bt://" remote address, so
// the transport sees exactly what it gets from a Bluetooth RFCOMM socket:
// a non-TCP stream with a non-host:port address.
type btLikeConn struct {
	net.Conn
	remote string
}

func (c btLikeConn) RemoteAddr() net.Addr { return btLikeAddr(c.remote) }

type btLikeAddr string

func (a btLikeAddr) Network() string { return "bluetooth" }
func (a btLikeAddr) String() string  { return string(a) }

type btLikeListener struct {
	net.Listener
	remote string
}

func (l btLikeListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return btLikeConn{Conn: c, remote: l.remote}, nil
}

// bluetoothPair connects client to host over a Bluetooth-like link.
func bluetoothPair(t *testing.T) (host, client *Transport, clientDir string, dialed *[]string) {
	t.Helper()
	hostDir, clientDir := t.TempDir(), t.TempDir()
	hostDev := makeEphemeralIdentity(t, hostDir, "BtHost")
	clientDev := makeEphemeralIdentity(t, clientDir, "BtClient")
	host = NewTransport(hostDev, nil, openTrustStoreAt(t, hostDir))
	host.SetPairingCode("654321")
	client = NewTransport(clientDev, nil, openTrustStoreAt(t, clientDir))

	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tcp.Close(); host.Stop() })
	withAppData(hostDir, func() {
		err = host.Serve(btLikeListener{Listener: tcp, remote: "bt://00:00:00:00:00:0C"})
	})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var mu sync.Mutex
	var log []string
	dialed = &log
	client.SetBluetoothDialer(func(address string, timeout time.Duration) (net.Conn, error) {
		mu.Lock()
		log = append(log, address)
		mu.Unlock()
		c, err := net.DialTimeout("tcp", tcp.Addr().String(), timeout)
		if err != nil {
			return nil, err
		}
		return btLikeConn{Conn: c, remote: address}, nil
	})
	return host, client, clientDir, dialed
}

// First-time pairing (SPAKE2 over the certificate-bound TLS session) and
// input frames work over a link that is not TCP.
func TestBluetoothLikeLinkPairsAndDeliversFrames(t *testing.T) {
	host, client, clientDir, dialed := bluetoothPair(t)

	var mu sync.Mutex
	var got []protocol.Frame
	host.OnFrame = func(f protocol.Frame) { mu.Lock(); got = append(got, f); mu.Unlock() }
	connected := make(chan struct{}, 2)
	host.OnConnect = func(string, string, string) { connected <- struct{}{} }
	client.OnConnect = func(string, string, string) { connected <- struct{}{} }

	var err error
	withAppData(clientDir, func() { err = client.ConnectTo("bt://00:00:00:00:00:01", "654321") })
	if err != nil {
		t.Fatalf("ConnectTo over bluetooth: %v", err)
	}
	t.Cleanup(client.Disconnect)
	for i := 0; i < 2; i++ {
		select {
		case <-connected:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for both sides to connect")
		}
	}
	if len(*dialed) != 1 || (*dialed)[0] != "bt://00:00:00:00:00:01" {
		t.Fatalf("bluetooth dialer saw %v", *dialed)
	}
	if s := client.GetSession(); s == nil || s.RemoteAddr() != "bt://00:00:00:00:00:01" {
		t.Fatalf("client session remote address = %v", s)
	}
	if s := host.GetSession(); s == nil || s.RemoteAddr() != "bt://00:00:00:00:00:0C" {
		t.Fatalf("host session remote address = %v", s)
	}

	for i := 0; i < 20; i++ {
		if err := client.Send(protocol.Frame{Type: protocol.MsgKeyDown, Payload: []byte{byte(i), 0, 0, 0}}); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	waitFrames(t, &got, &mu, 20, 3*time.Second)
}

func TestBluetoothAddressWithoutDialerIsADialError(t *testing.T) {
	dir := t.TempDir()
	dev := makeEphemeralIdentity(t, dir, "NoBt")
	tr := NewTransport(dev, nil, openTrustStoreAt(t, dir))
	var err error
	withAppData(dir, func() { err = tr.ConnectTo("bt://00:00:00:00:00:01", "") })
	if !errors.Is(err, ErrDial) {
		t.Fatalf("want ErrDial so callers can try another route, got %v", err)
	}
}

func TestBluetoothDialFailureIsADialError(t *testing.T) {
	dir := t.TempDir()
	dev := makeEphemeralIdentity(t, dir, "BtFail")
	tr := NewTransport(dev, nil, openTrustStoreAt(t, dir))
	tr.SetBluetoothDialer(func(string, time.Duration) (net.Conn, error) { return nil, errors.New("device out of range") })
	var err error
	withAppData(dir, func() { err = tr.ConnectTo("bt://00:00:00:00:00:01", "") })
	if !errors.Is(err, ErrDial) {
		t.Fatalf("want ErrDial, got %v", err)
	}
}

// Without a pairing code the PAKE step is skipped and an untrusted peer is
// rejected, over Bluetooth exactly as over TCP.
func TestBluetoothLikeLinkRejectsWrongPIN(t *testing.T) {
	_, client, clientDir, _ := bluetoothPair(t)
	var err error
	withAppData(clientDir, func() { err = client.ConnectTo("bt://00:00:00:00:00:01", "111111") })
	if err == nil {
		t.Fatal("wrong PIN must not connect")
	}
	if errors.Is(err, ErrDial) {
		t.Fatalf("a PIN failure must not look like a dial failure (it would retry other routes): %v", err)
	}
}
