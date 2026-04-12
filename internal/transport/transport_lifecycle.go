package transport

import (
	"crypto/tls"
	"encoding/binary"
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

func (t *Transport) Start(port int) error {
	cert, err := tls.LoadX509KeyPair(
		identity.CertPath(),
		identity.KeyPath(),
	)
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

func (t *Transport) handleInbound(conn *tls.Conn) {
	t.connectMu.Lock()
	if t.GetSession() != nil {
		t.connectMu.Unlock()
		conn.Close()
		return
	}
	peerHello, peerFingerprint, err := t.exchangeHelloInbound(conn)
	if err != nil {
		t.connectMu.Unlock()
		log.Printf("inbound handshake rejected: %v", err)
		conn.Close()
		return
	}
	// Disable Nagle so small control frames (mouse moves, keys) are sent
	// immediately rather than being held for batching by the kernel.
	if tc, ok := conn.NetConn().(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	t.mu.Lock()
	s := &Session{
		conn:            conn,
		PeerID:          peerHello.DeviceID,
		PeerName:        peerHello.Name,
		PeerFingerprint: peerFingerprint,
		Role:            "controlled",
		closeCh:         make(chan struct{}),
	}
	t.session = s
	t.mu.Unlock()
	_ = conn.SetDeadline(time.Time{})
	t.connectMu.Unlock()

	log.Printf("inbound connection from %s (%s)", peerHello.Name, shortPeerID(peerHello.DeviceID))
	if t.OnConnect != nil {
		safeCall("OnConnect/inbound", func() { t.OnConnect(peerHello.DeviceID, peerHello.Name, "controlled") })
	}

	t.readLoop(s)
}

func (t *Transport) ConnectTo(address string, pairingCode string) error {
	t.connectMu.Lock()
	if t.GetSession() != nil {
		t.connectMu.Unlock()
		return fmt.Errorf("already connected")
	}

	cert, err := tls.LoadX509KeyPair(identity.CertPath(), identity.KeyPath())
	if err != nil {
		t.connectMu.Unlock()
		return fmt.Errorf("load cert: %w", err)
	}

	conn, err := tls.Dial("tcp", address, &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.connectMu.Unlock()
		return fmt.Errorf("connect: %w", err)
	}
	peerHello, peerFingerprint, err := t.exchangeHelloOutbound(conn, pairingCode)
	if err != nil {
		t.connectMu.Unlock()
		conn.Close()
		return err
	}
	if err := t.authorizeOutboundPeer(peerHello, peerFingerprint, address, pairingCode); err != nil {
		t.connectMu.Unlock()
		conn.Close()
		return err
	}
	// Disable Nagle so small control frames (mouse moves, keys) are sent
	// immediately rather than being held for batching by the kernel.
	if tc, ok := conn.NetConn().(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	t.mu.Lock()
	s := &Session{
		conn:            conn,
		PeerID:          peerHello.DeviceID,
		PeerName:        peerHello.Name,
		PeerFingerprint: peerFingerprint,
		Role:            "controller",
		closeCh:         make(chan struct{}),
	}
	t.session = s
	t.mu.Unlock()
	_ = conn.SetDeadline(time.Time{})
	t.connectMu.Unlock()

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
	if f.Type == protocol.MsgMouseMove && len(f.Payload) == 8 {
		// Fast path: encode DX/DY directly into the pool buffer without going
		// through a separate payload allocation from MouseMoveMsg.Encode().
		dx := int32(binary.BigEndian.Uint32(f.Payload[0:]))
		dy := int32(binary.BigEndian.Uint32(f.Payload[4:]))
		return protocol.WriteFrameMouseMove(s.conn, dx, dy)
	}
	return protocol.WriteFrame(s.conn, f)
}

func (t *Transport) GetSession() *Session {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.session
}
