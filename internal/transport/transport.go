package transport

import (
	"crypto/tls"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"multisnekkvm/internal/identity"
	"multisnekkvm/internal/protocol"
	"multisnekkvm/internal/trust"
)

const (
	handshakeTimeout   = 10 * time.Second
	heartbeatTimeout   = 15 * time.Second
	writeTimeout       = 5 * time.Second
	pairingCodeTTL     = 2 * time.Minute
	pairingBaseBackoff = 2 * time.Second
	pairingMaxFailures = 5

	// Rate-limiting for inbound connections to mitigate resource exhaustion.
	rateLimitWindow    = 30 * time.Second
	rateLimitMaxPerIP  = 5
	rateLimitCleanFreq = 60 * time.Second
)

var pairingCodeGenerator = generateRandomPairingCode

type Session struct {
	mu              sync.RWMutex
	conn            *tls.Conn
	PeerID          string
	PeerName        string
	PeerFingerprint string
	Role            string // "controller" or "controlled"
	closeCh         chan struct{}
	closeOnce       sync.Once
}

func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.closeCh)
		if s.conn != nil {
			s.conn.Close()
		}
	})
}

// RemoteAddr returns the remote address of the session connection.
func (s *Session) RemoteAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.conn == nil {
		return ""
	}
	return s.conn.RemoteAddr().String()
}

type Transport struct {
	device             identity.DeviceInfo
	identity           *identity.Identity
	trust              *trust.Store
	pairingCode        string
	pairingIssuedAt    time.Time
	pairingFailures    int
	pairingLockedUntil time.Time

	listener net.Listener

	mu        sync.RWMutex
	session   *Session
	connectMu sync.Mutex

	// Per-IP connection rate limiter.
	rateMu    sync.Mutex
	rateConns map[string][]time.Time

	OnFrame      func(protocol.Frame)
	OnConnect    func(peerID, peerName, role string)
	OnDisconnect func()
}

func NewTransport(device identity.DeviceInfo, id *identity.Identity, trustStore *trust.Store) *Transport {
	t := &Transport{
		device:    device,
		identity:  id,
		trust:     trustStore,
		rateConns: make(map[string][]time.Time),
	}
	t.setPairingCodeLocked(normalizePairingCode(device.PairingCode), time.Now())
	return t
}

// connRateLimited returns true if the given IP has exceeded the allowed
// connection rate within the sliding window.
func (t *Transport) connRateLimited(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-rateLimitWindow)

	t.rateMu.Lock()
	defer t.rateMu.Unlock()

	if t.rateConns == nil {
		t.rateConns = make(map[string][]time.Time)
	}

	// Prune old entries for this IP.
	times := t.rateConns[ip]
	pruned := times[:0]
	for _, ts := range times {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}

	if len(pruned) >= rateLimitMaxPerIP {
		t.rateConns[ip] = pruned
		return true
	}

	t.rateConns[ip] = append(pruned, now)
	return false
}

// cleanRateLimit periodically removes stale entries from the rate limiter map.
func (t *Transport) cleanRateLimit(done <-chan struct{}) {
	ticker := time.NewTicker(rateLimitCleanFreq)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-rateLimitWindow)
			t.rateMu.Lock()
			for ip, times := range t.rateConns {
				pruned := times[:0]
				for _, ts := range times {
					if ts.After(cutoff) {
						pruned = append(pruned, ts)
					}
				}
				if len(pruned) == 0 {
					delete(t.rateConns, ip)
				} else {
					t.rateConns[ip] = pruned
				}
			}
			t.rateMu.Unlock()
		}
	}
}

// safeCall invokes fn, recovering any panic and logging it with a stack trace.
// Use this to wrap all user-supplied callbacks (OnFrame, OnConnect, OnDisconnect)
// so that a misbehaving callback cannot crash the transport goroutine.
func safeCall(tag string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("transport: %s panic: %v\n%s", tag, r, debug.Stack())
		}
	}()
	fn()
}
