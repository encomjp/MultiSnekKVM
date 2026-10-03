package bluetooth

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAddressRoundTrip(t *testing.T) {
	for _, in := range []string{"bt://aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF", " bt://(00:1a:7d:da:71:13) "} {
		got, err := NormalizeAddress(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if again, _ := NormalizeAddress(got); again != got {
			t.Fatalf("not idempotent: %q -> %q -> %q", in, got, again)
		}
	}
	a, _ := ParseAddress("bt://00:1A:7D:DA:71:13")
	if a != 0x001A7DDA7113 || FormatAddress(a) != "00:1A:7D:DA:71:13" {
		t.Fatalf("parse/format mismatch: %x %s", a, FormatAddress(a))
	}
	for _, bad := range []string{"", "bt://", "bt://AA:BB:CC", "bt://ZZ:BB:CC:DD:EE:FF", "192.168.0.2:24831", "bt://AAA:BB:CC:DD:EE:FF"} {
		if _, err := ParseAddress(bad); err == nil {
			t.Errorf("ParseAddress(%q) should fail", bad)
		}
	}
	if !IsAddress("bt://x") || IsAddress("192.168.0.2:24831") {
		t.Fatal("IsAddress wrong")
	}
}

func TestServiceNameCodec(t *testing.T) {
	name := EncodeServiceName("dev-123", "Studio | PC\n")
	id, n, ok := DecodeServiceName(name)
	if !ok || id != "dev-123" || n != "Studio  PC" {
		t.Fatalf("decoded %q %q %v from %q", id, n, ok, name)
	}
	for _, bad := range []string{"", "Other service", "MultiSnek|", "MultiSnek||x"} {
		if _, _, ok := DecodeServiceName(bad); ok {
			t.Errorf("%q must not decode", bad)
		}
	}
}

type fakeBackend struct {
	mu         sync.Mutex
	avail      error
	listenErr  error
	listeners  []*fakeListener
	peers      []Peer
	scanErr    error
	scans      atomic.Int32
	listenName string
}

type fakeListener struct {
	closed atomic.Bool
	ch     chan struct{}
}

func (l *fakeListener) Accept() (net.Conn, error) { <-l.ch; return nil, net.ErrClosed }
func (l *fakeListener) Close() error {
	if l.closed.CompareAndSwap(false, true) {
		close(l.ch)
	}
	return nil
}
func (l *fakeListener) Addr() net.Addr { return btAddrLike("bt://local") }

type btAddrLike string

func (a btAddrLike) Network() string { return "bluetooth" }
func (a btAddrLike) String() string  { return string(a) }

func (f *fakeBackend) Available() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.avail
}
func (f *fakeBackend) Listen(name string) (net.Listener, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listenErr != nil {
		return nil, f.listenErr
	}
	f.listenName = name
	l := &fakeListener{ch: make(chan struct{})}
	f.listeners = append(f.listeners, l)
	return l, nil
}
func (f *fakeBackend) Dial(addr string, _ time.Duration) (net.Conn, error) {
	a, b := net.Pipe()
	b.Close()
	return a, nil
}
func (f *fakeBackend) Scan(context.Context) ([]Peer, error) {
	f.scans.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Peer(nil), f.peers...), f.scanErr
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func startManager(t *testing.T, fb *fakeBackend, enabled bool) (*Manager, *atomic.Int32) {
	t.Helper()
	m := NewManager(fb, "self-id", "This PC", enabled)
	m.scanEvery = time.Hour // only explicit refreshes
	var served atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx, func(net.Listener) { served.Add(1) }); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return m, &served
}

func TestManagerListensAdvertisesAndScans(t *testing.T) {
	fb := &fakeBackend{peers: []Peer{
		{Address: "bt://00:00:00:00:00:02", DeviceID: "peer-b", Name: "B"},
		{Address: "bt://00:00:00:00:00:01", DeviceID: "peer-a", Name: "A"},
		{Address: "bt://00:00:00:00:00:09", DeviceID: "self-id", Name: "me"}, // own record must be ignored
	}}
	m, served := startManager(t, fb, true)
	waitFor(t, "listener", func() bool { return m.Status().Listening && served.Load() == 1 })
	waitFor(t, "peers", func() bool { return len(m.Peers()) == 2 })

	if id, name, ok := DecodeServiceName(fb.listenName); !ok || id != "self-id" || name != "This PC" {
		t.Fatalf("advertised service %q", fb.listenName)
	}
	peers := m.Peers()
	if peers[0].DeviceID != "peer-a" || peers[1].DeviceID != "peer-b" {
		t.Fatalf("peers not sorted/filtered: %+v", peers)
	}
	st := m.Status()
	if !st.Available || !st.Enabled || st.Error != "" || st.LastScan == 0 {
		t.Fatalf("status %+v", st)
	}
}

func TestManagerDisableClosesListenerAndClearsPeers(t *testing.T) {
	fb := &fakeBackend{peers: []Peer{{Address: "bt://00:00:00:00:00:01", DeviceID: "p", Name: "P"}}}
	m, _ := startManager(t, fb, true)
	waitFor(t, "peers", func() bool { return len(m.Peers()) == 1 })

	m.SetEnabled(false)
	waitFor(t, "listener closed", func() bool { return !m.Status().Listening })
	waitFor(t, "peers cleared", func() bool { return len(m.Peers()) == 0 })
	fb.mu.Lock()
	closed := fb.listeners[0].closed.Load()
	fb.mu.Unlock()
	if !closed {
		t.Fatal("backend listener was not closed")
	}

	m.SetEnabled(true)
	waitFor(t, "re-listen", func() bool { return m.Status().Listening })
}

func TestManagerReportsUnavailableAndRecovers(t *testing.T) {
	fb := &fakeBackend{avail: ErrUnavailable}
	m, _ := startManager(t, fb, true)
	waitFor(t, "unavailable status", func() bool {
		st := m.Status()
		return !st.Available && st.Error != "" && !st.Listening
	})
	if fb.scans.Load() != 0 {
		t.Fatal("must not scan without a radio")
	}
	fb.mu.Lock()
	fb.avail = nil
	fb.mu.Unlock()
	m.Refresh()
	waitFor(t, "recovered", func() bool { return m.Status().Available && m.Status().Listening })
}

func TestManagerSurfacesListenAndScanErrors(t *testing.T) {
	fb := &fakeBackend{listenErr: errors.New("radio busy"), scanErr: errors.New("sdp timeout")}
	m, _ := startManager(t, fb, true)
	waitFor(t, "listen error", func() bool { return m.Status().Error == "radio busy" })
	if m.Status().Listening {
		t.Fatal("must not report listening after a failed listen")
	}
}

func TestManagerNotifiesOnChange(t *testing.T) {
	fb := &fakeBackend{}
	m := NewManager(fb, "self-id", "This PC", true)
	m.scanEvery = time.Hour
	var changes atomic.Int32
	m.OnChange(func() { changes.Add(1) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx, func(net.Listener) {}); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	waitFor(t, "change notification", func() bool { return changes.Load() > 0 })
}

func TestManagerDialPassesThrough(t *testing.T) {
	m := NewManager(&fakeBackend{}, "self", "x", true)
	c, err := m.Dial("bt://00:00:00:00:00:01", time.Second)
	if err != nil || c == nil {
		t.Fatalf("dial: %v", err)
	}
	c.Close()
}
