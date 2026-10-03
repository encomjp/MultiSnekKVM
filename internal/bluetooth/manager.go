package bluetooth

import (
	"context"
	"log"
	"net"
	"sort"
	"sync"
	"time"
)

// Peer is a paired Bluetooth device that runs MultiSnek.
type Peer struct {
	// Address is the canonical "bt://AA:BB:CC:DD:EE:FF" form.
	Address string `json:"address"`
	// DeviceID is the peer's MultiSnek device ID, or "bt:<address>" when the
	// peer did not advertise one.
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"`
}

// Backend is the platform Bluetooth stack.
type Backend interface {
	// Available returns nil when a usable Bluetooth radio and stack exist.
	Available() error
	// Listen registers the MultiSnek service under serviceName and accepts
	// RFCOMM connections.
	Listen(serviceName string) (net.Listener, error)
	// Dial connects to the MultiSnek service on a device ("bt://..." or bare).
	Dial(address string, timeout time.Duration) (net.Conn, error)
	// Scan returns the paired/remembered devices that run MultiSnek.
	Scan(ctx context.Context) ([]Peer, error)
}

// Status is the user-visible state, sent to the UI.
type Status struct {
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Listening bool   `json:"listening"`
	Scanning  bool   `json:"scanning"`
	Error     string `json:"error,omitempty"`
	Devices   []Peer `json:"devices"`
	LastScan  int64  `json:"lastScan"`
}

const (
	scanInterval   = 30 * time.Second
	scanTimeout    = 50 * time.Second
	listenRetryMin = 15 * time.Second
)

// Manager owns the Bluetooth listener and the periodic scan for peers.
type Manager struct {
	backend          Backend
	selfID, selfName string
	scanEvery        time.Duration
	onChange         func()
	wake             chan struct{}

	mu        sync.Mutex
	enabled   bool
	listener  net.Listener
	listenErr error
	lastTry   time.Time
	scanning  bool
	scanErr   error
	peers     []Peer
	lastScan  time.Time
	unavail   error
}

func NewManager(b Backend, selfID, selfName string, enabled bool) *Manager {
	return &Manager{
		backend:   b,
		selfID:    selfID,
		selfName:  selfName,
		enabled:   enabled,
		scanEvery: scanInterval,
		wake:      make(chan struct{}, 1),
	}
}

// OnChange registers a callback invoked (from any goroutine) when Status changes.
func (m *Manager) OnChange(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *Manager) notify() {
	m.mu.Lock()
	fn := m.onChange
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (m *Manager) kick() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// SetEnabled turns Bluetooth on or off; the listener follows immediately.
func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	changed := m.enabled != enabled
	m.enabled = enabled
	m.lastTry = time.Time{} // an explicit toggle retries immediately
	m.mu.Unlock()
	if changed {
		m.kick()
	}
}

func (m *Manager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled
}

// Refresh rescans now.
func (m *Manager) Refresh() { m.kick() }

// Dial opens a Bluetooth connection to address.
func (m *Manager) Dial(address string, timeout time.Duration) (net.Conn, error) {
	return m.backend.Dial(address, timeout)
}

// Peers returns the Bluetooth peers found by the last scan.
func (m *Manager) Peers() []Peer {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Peer(nil), m.peers...)
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{
		Available: m.unavail == nil,
		Enabled:   m.enabled,
		Listening: m.listener != nil,
		Scanning:  m.scanning,
		Devices:   append([]Peer{}, m.peers...),
	}
	switch {
	case m.unavail != nil:
		st.Error = m.unavail.Error()
	case m.enabled && m.listenErr != nil:
		st.Error = m.listenErr.Error()
	case m.enabled && m.scanErr != nil:
		st.Error = m.scanErr.Error()
	}
	if !m.lastScan.IsZero() {
		st.LastScan = m.lastScan.Unix()
	}
	return st
}

// Run manages the listener and scans until ctx is done. serve is called once
// per listener and must accept connections in its own goroutine.
func (m *Manager) Run(ctx context.Context, serve func(net.Listener)) {
	defer m.closeListener()
	ticker := time.NewTicker(m.scanEvery)
	defer ticker.Stop()
	for {
		before := m.Status()
		m.reconcile(serve)
		if m.Enabled() && m.Status().Available {
			m.scan(ctx)
		} else {
			// Turned off or unavailable: stop offering stale peers.
			m.clearPeers()
		}
		if after := m.Status(); !sameStatus(before, after) {
			m.notify()
		}
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
	}
}

func (m *Manager) clearPeers() {
	m.mu.Lock()
	m.peers = nil
	m.mu.Unlock()
}

func (m *Manager) closeListener() {
	m.mu.Lock()
	l := m.listener
	m.listener = nil
	m.mu.Unlock()
	if l != nil {
		l.Close()
	}
}

func (m *Manager) reconcile(serve func(net.Listener)) {
	err := m.backend.Available()
	m.mu.Lock()
	m.unavail = err
	enabled, have := m.enabled, m.listener != nil
	retryOK := time.Since(m.lastTry) >= listenRetryMin
	m.mu.Unlock()

	switch {
	case err != nil || !enabled:
		if have {
			m.closeListener()
		}
	case !have && retryOK:
		l, lerr := m.backend.Listen(EncodeServiceName(m.selfID, m.selfName))
		m.mu.Lock()
		if lerr != nil {
			m.lastTry = time.Now() // throttle retries after a failure only
		} else {
			m.lastTry = time.Time{}
		}
		m.listenErr = lerr
		if lerr == nil {
			m.listener = l
		}
		m.mu.Unlock()
		if lerr != nil {
			log.Printf("bluetooth: cannot listen: %v", lerr)
			return
		}
		log.Printf("bluetooth: listening for MultiSnek connections")
		serve(l)
	}
}

func (m *Manager) scan(ctx context.Context) {
	m.mu.Lock()
	m.scanning = true
	m.mu.Unlock()
	m.notify()

	sctx, cancel := context.WithTimeout(ctx, scanTimeout)
	peers, err := m.backend.Scan(sctx)
	cancel()

	kept := peers[:0:0]
	for _, p := range peers {
		if p.DeviceID == m.selfID {
			continue
		}
		kept = append(kept, p)
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Address < kept[j].Address })

	m.mu.Lock()
	m.scanning = false
	m.scanErr = err
	if err == nil {
		m.peers = kept
		m.lastScan = time.Now()
	}
	m.mu.Unlock()
	if err != nil && ctx.Err() == nil {
		log.Printf("bluetooth: scan: %v", err)
	}
}

func sameStatus(a, b Status) bool {
	if a.Available != b.Available || a.Enabled != b.Enabled || a.Listening != b.Listening ||
		a.Scanning != b.Scanning || a.Error != b.Error || a.LastScan != b.LastScan || len(a.Devices) != len(b.Devices) {
		return false
	}
	for i := range a.Devices {
		if a.Devices[i] != b.Devices[i] {
			return false
		}
	}
	return true
}
