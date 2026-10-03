package transport

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"time"

	"multisnekkvm/internal/identity"
	"multisnekkvm/internal/protocol"
)

// IsListening reports whether the transport is actively listening for connections.
func (t *Transport) IsListening() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.listener != nil
}

// localCertificate loads the identity certificate once and caches it.
// A failed load is not cached so a later call can retry.
func (t *Transport) localCertificate() (tls.Certificate, error) {
	t.certMu.Lock()
	defer t.certMu.Unlock()
	if t.cert != nil {
		return *t.cert, nil
	}
	cert, err := tls.LoadX509KeyPair(identity.CertPath(), identity.KeyPath())
	if err != nil {
		return tls.Certificate{}, err
	}
	t.cert = &cert
	return cert, nil
}

func (t *Transport) Start(port int) error {
	cert, err := t.localCertificate()
	if err != nil {
		return fmt.Errorf("load cert: %w", err)
	}

	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAnyClientCert,
	})
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	t.mu.Lock()
	t.listener = listener
	t.mu.Unlock()

	go t.acceptLoop(listener)
	log.Printf("transport listening on :%d", port)
	return nil
}

func (t *Transport) Stop() {
	t.mu.Lock()
	l := t.listener
	t.listener = nil
	t.mu.Unlock()
	if l != nil {
		l.Close()
	}
	t.mu.RLock()
	s := t.session
	t.mu.RUnlock()
	if s != nil {
		s.Close()
	}
}

func (t *Transport) acceptLoop(l net.Listener) {
	// Clear t.listener when the accept loop exits so IsListening() returns false.
	// Only nil the field if it still points to our listener (guards against a
	// future re-Start overwriting it before we exit).
	defer func() {
		t.mu.Lock()
		if t.listener == l {
			t.listener = nil
		}
		t.mu.Unlock()
	}()

	// Start periodic cleanup of the rate limiter map.
	rateDone := make(chan struct{})
	defer close(rateDone)
	go t.cleanRateLimit(rateDone)

	for {
		conn, err := l.Accept()
		if err != nil {
			// Temporary errors: brief sleep and retry so a transient
			// condition doesn't permanently kill the accept loop.
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			// Permanent error (listener closed, etc.): stop the loop.
			return
		}

		// Per-IP rate limiting: reject connections that exceed the
		// allowed rate within the sliding window.
		remoteIP, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		if t.connRateLimited(remoteIP) {
			log.Printf("rate-limited inbound connection from %s", remoteIP)
			conn.Close()
			continue
		}

		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			conn.Close()
			continue
		}
		go t.handleInbound(tlsConn)
	}
}

// installSession publishes s as the active session unless one already
// exists. connectMu serializes installation only; the (potentially slow,
// unauthenticated) TLS handshake, hello exchange and pairing run before it
// so one stalled client cannot block every other connection attempt.
func (t *Transport) installSession(s *Session) bool {
	t.connectMu.Lock()
	defer t.connectMu.Unlock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.session != nil {
		return false
	}
	t.session = s
	return true
}

func (t *Transport) handleInbound(conn *tls.Conn) {
	// Cheap early rejection; re-checked atomically in installSession.
	if t.GetSession() != nil {
		conn.Close()
		return
	}
	peerHello, peerFingerprint, err := t.exchangeHelloInbound(conn)
	if err != nil {
		log.Printf("inbound handshake rejected: %v", err)
		conn.Close()
		return
	}
	// Disable Nagle so small control frames (mouse moves, keys) are sent
	// immediately rather than being held for batching by the kernel.
	if tc, ok := conn.NetConn().(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	_ = conn.SetDeadline(time.Time{})
	s := &Session{
		conn:            conn,
		PeerID:          peerHello.DeviceID,
		PeerName:        peerHello.Name,
		PeerFingerprint: peerFingerprint,
		Role:            "controlled",
		closeCh:         make(chan struct{}),
	}
	if !t.installSession(s) {
		log.Printf("inbound connection from %s (%s) dropped: already connected", peerHello.Name, shortPeerID(peerHello.DeviceID))
		conn.Close()
		return
	}

	log.Printf("inbound connection from %s (%s)", peerHello.Name, shortPeerID(peerHello.DeviceID))
	if t.OnConnect != nil {
		safeCall("OnConnect/inbound", func() { t.OnConnect(peerHello.DeviceID, peerHello.Name, "controlled") })
	}

	t.readLoop(s)
}

func (t *Transport) ConnectTo(address string, pairingCode string) error {
	if t.GetSession() != nil {
		return fmt.Errorf("already connected")
	}

	cert, err := t.localCertificate()
	if err != nil {
		return fmt.Errorf("load cert: %w", err)
	}

	// Dial, handshake and authorization happen outside connectMu.
	dialer := &net.Dialer{Timeout: handshakeTimeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	})
	if err != nil {
		return &dialError{err: err}
	}
	peerHello, peerFingerprint, err := t.exchangeHelloOutbound(conn, pairingCode)
	if err != nil {
		conn.Close()
		return err
	}
	if err := t.authorizeOutboundPeer(peerHello, peerFingerprint, address, pairingCode); err != nil {
		conn.Close()
		return err
	}
	// Disable Nagle so small control frames (mouse moves, keys) are sent
	// immediately rather than being held for batching by the kernel.
	if tc, ok := conn.NetConn().(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	_ = conn.SetDeadline(time.Time{})
	s := &Session{
		conn:            conn,
		PeerID:          peerHello.DeviceID,
		PeerName:        peerHello.Name,
		PeerFingerprint: peerFingerprint,
		Role:            "controller",
		closeCh:         make(chan struct{}),
	}
	if !t.installSession(s) {
		conn.Close()
		return fmt.Errorf("already connected")
	}

	log.Printf("connected to %s (%s)", peerHello.Name, shortPeerID(peerHello.DeviceID))
	if t.OnConnect != nil {
		safeCall("OnConnect/outbound", func() { t.OnConnect(peerHello.DeviceID, peerHello.Name, "controller") })
	}

	go t.readLoop(s)
	return nil
}

func (t *Transport) Disconnect() {
	t.mu.Lock()
	s := t.session
	t.session = nil
	t.mu.Unlock()
	if s != nil {
		s.Close()
		log.Printf("disconnected from %s", s.PeerName)
		if t.OnDisconnect != nil {
			safeCall("OnDisconnect/disconnect", t.OnDisconnect)
		}
	}
}

func (t *Transport) Send(f protocol.Frame) error {
	t.mu.RLock()
	s := t.session
	t.mu.RUnlock()
	if s == nil {
		return fmt.Errorf("not connected")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Set write deadline before each write; no need to clear it afterwards
	// because the next Send refreshes it and heartbeats prevent stale deadlines.
	_ = s.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return protocol.WriteFrame(s.conn, f)
}

func (t *Transport) GetSession() *Session {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.session
}
