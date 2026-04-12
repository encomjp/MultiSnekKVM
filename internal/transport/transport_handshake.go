package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"strings"
	"time"

	"multisnekkvm/internal/identity"
	"multisnekkvm/internal/protocol"
)

func (t *Transport) exchangeHelloOutbound(conn *tls.Conn, pairingCode string) (protocol.HelloMsg, string, error) {
	peerCert, peerFingerprint, err := t.beginHelloExchange(conn)
	if err != nil {
		return protocol.HelloMsg{}, "", err
	}

	hello := protocol.HelloMsg{
		DeviceID:    t.device.ID,
		Name:        t.device.Name,
		Fingerprint: t.device.Fingerprint,
		PairingCode: normalizePairingCode(pairingCode),
	}
	if err := protocol.WriteFrame(conn, protocol.Frame{Type: protocol.MsgHello, Payload: hello.Encode()}); err != nil {
		return protocol.HelloMsg{}, "", fmt.Errorf("send hello: %w", err)
	}

	peerHello, err := t.readPeerHello(conn, peerCert)
	if err != nil {
		if normalizePairingCode(pairingCode) != "" && isEOFError(err) {
			return protocol.HelloMsg{}, "", fmt.Errorf("pairing PIN rejected by remote peer")
		}
		return protocol.HelloMsg{}, "", err
	}

	return peerHello, peerFingerprint, nil
}

func isEOFError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	return strings.Contains(err.Error(), "EOF")
}

func (t *Transport) exchangeHelloInbound(conn *tls.Conn) (protocol.HelloMsg, string, error) {
	peerCert, peerFingerprint, err := t.beginHelloExchange(conn)
	if err != nil {
		return protocol.HelloMsg{}, "", err
	}

	peerHello, err := t.readPeerHello(conn, peerCert)
	if err != nil {
		return protocol.HelloMsg{}, "", err
	}
	if err := t.authorizeInboundPeer(peerHello, peerFingerprint, conn.RemoteAddr().String()); err != nil {
		return protocol.HelloMsg{}, "", err
	}

	hello := protocol.HelloMsg{
		DeviceID:    t.device.ID,
		Name:        t.device.Name,
		Fingerprint: t.device.Fingerprint,
	}
	if err := protocol.WriteFrame(conn, protocol.Frame{Type: protocol.MsgHello, Payload: hello.Encode()}); err != nil {
		return protocol.HelloMsg{}, "", fmt.Errorf("send hello: %w", err)
	}

	return peerHello, peerFingerprint, nil
}

func (t *Transport) beginHelloExchange(conn *tls.Conn) (*x509.Certificate, string, error) {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return nil, "", fmt.Errorf("set handshake deadline: %w", err)
	}
	if err := conn.Handshake(); err != nil {
		return nil, "", fmt.Errorf("handshake: %w", err)
	}

	peerCert, err := peerCertificate(conn)
	if err != nil {
		return nil, "", err
	}
	peerFingerprint := identity.CertificateFingerprint(peerCert)
	return peerCert, peerFingerprint, nil
}

func (t *Transport) readPeerHello(conn *tls.Conn, peerCert *x509.Certificate) (protocol.HelloMsg, error) {
	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return protocol.HelloMsg{}, fmt.Errorf("read hello: %w", err)
	}
	if frame.Type != protocol.MsgHello {
		return protocol.HelloMsg{}, fmt.Errorf("unexpected handshake frame: 0x%02x", frame.Type)
	}

	peerHello, err := protocol.DecodeHello(frame.Payload)
	if err != nil {
		return protocol.HelloMsg{}, fmt.Errorf("decode hello: %w", err)
	}
	if err := validatePeerHello(peerHello, peerCert); err != nil {
		return protocol.HelloMsg{}, err
	}

	return peerHello, nil
}

func peerCertificate(conn *tls.Conn) (*x509.Certificate, error) {
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, fmt.Errorf("peer certificate missing")
	}
	return state.PeerCertificates[0], nil
}

func validatePeerHello(hello protocol.HelloMsg, peerCert *x509.Certificate) error {
	if peerCert == nil {
		return fmt.Errorf("peer certificate missing")
	}
	if !identity.ContainsStringFold(peerCert.Subject.Organization, "MultiSnekKVM") {
		return fmt.Errorf("peer certificate is not tagged for MultiSnekKVM")
	}
	if strings.TrimSpace(hello.DeviceID) == "" {
		return fmt.Errorf("peer hello missing device id")
	}
	if strings.TrimSpace(peerCert.Subject.SerialNumber) == "" {
		return fmt.Errorf("peer certificate missing device id")
	}
	if hello.DeviceID != strings.TrimSpace(peerCert.Subject.SerialNumber) {
		return fmt.Errorf("peer device id does not match certificate")
	}
	if strings.TrimSpace(hello.Name) == "" {
		return fmt.Errorf("peer hello missing name")
	}
	if !strings.EqualFold(hello.Name, strings.TrimSpace(peerCert.Subject.CommonName)) {
		return fmt.Errorf("peer name does not match certificate (hello=%q cert=%q)", hello.Name, peerCert.Subject.CommonName)
	}
	if strings.TrimSpace(hello.Fingerprint) == "" {
		return fmt.Errorf("peer hello missing fingerprint")
	}
	if !strings.EqualFold(hello.Fingerprint, identity.CertificateFingerprint(peerCert)) {
		return fmt.Errorf("peer fingerprint mismatch")
	}
	return nil
}

func shortPeerID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
