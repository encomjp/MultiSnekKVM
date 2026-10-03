package transport

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	spake2 "github.com/backkem/spake2-go"

	"multisnekkvm/internal/protocol"
)

const pairingProtocolMarker = "spake2-v2"
const maxPairingMessageBytes = 4096

// pairingOptions binds the PAKE transcript to the certificates actually seen
// in THIS TLS connection. Relaying the PAKE over two different TLS sessions
// cannot authenticate the attacker's certificate as the intended peer.
func pairingOptions(clientFingerprint, serverFingerprint string) *spake2.Options {
	opts := spake2.DefaultOptions()
	opts.AAD = []byte("MultiSnekKVM:SPAKE2-v2\x00" +
		strings.ToLower(clientFingerprint) + "\x00" + strings.ToLower(serverFingerprint))
	return opts
}

func readPairingFrame(conn *tls.Conn, expected byte) ([]byte, error) {
	frame, err := protocol.ReadFrame(conn)
	if err != nil {
		return nil, err
	}
	if frame.Type != expected || len(frame.Payload) == 0 || len(frame.Payload) > maxPairingMessageBytes {
		return nil, fmt.Errorf("invalid pairing message type=0x%02x length=%d", frame.Type, len(frame.Payload))
	}
	return frame.Payload, nil
}

func writePairingFrame(conn *tls.Conn, msgType byte, payload []byte) error {
	if len(payload) == 0 || len(payload) > maxPairingMessageBytes {
		return fmt.Errorf("invalid outbound pairing message length=%d", len(payload))
	}
	return protocol.WriteFrame(conn, protocol.Frame{Type: msgType, Payload: payload})
}

func (t *Transport) runPairingOutbound(conn *tls.Conn, pin, clientFingerprint, serverFingerprint string) error {
	client := spake2.NewClient([]byte(pin), pairingOptions(clientFingerprint, serverFingerprint))
	start, err := client.Start()
	if err != nil {
		return fmt.Errorf("start pairing: %w", err)
	}
	if err := writePairingFrame(conn, protocol.MsgPairingStart, start); err != nil {
		return err
	}
	response, err := readPairingFrame(conn, protocol.MsgPairingExchange)
	if err != nil {
		return fmt.Errorf("pairing rejected by remote peer: %w", err)
	}
	confirm, err := client.Finish(response)
	if err != nil {
		return fmt.Errorf("pairing exchange failed: %w", err)
	}
	if err := writePairingFrame(conn, protocol.MsgPairingClientConfirm, confirm); err != nil {
		return err
	}
	serverConfirm, err := readPairingFrame(conn, protocol.MsgPairingServerConfirm)
	if err != nil {
		return fmt.Errorf("pairing confirmation rejected: %w", err)
	}
	if err := client.Verify(serverConfirm); err != nil {
		return fmt.Errorf("peer could not prove knowledge of the pairing PIN and TLS identity: %w", err)
	}
	return nil
}

func (t *Transport) runPairingInbound(conn *tls.Conn, clientFingerprint, serverFingerprint string) (string, error) {
	// Snapshot the current PIN and enforce online throttling before starting PAKE.
	now := time.Now()
	t.mu.Lock()
	pin, _ := t.ensurePairingCodeLocked(now)
	lockedUntil := t.pairingLockedUntil
	t.mu.Unlock()
	if !lockedUntil.IsZero() && now.Before(lockedUntil) {
		return "", fmt.Errorf("pairing PIN temporarily locked")
	}
	if pin == "" {
		return "", fmt.Errorf("pairing PIN unavailable")
	}
	success := false
	defer func() {
		if !success {
			t.mu.Lock()
			t.recordPairingFailureLocked(time.Now())
			t.mu.Unlock()
		}
	}()

	server := spake2.NewServer([]byte(pin), pairingOptions(clientFingerprint, serverFingerprint))
	start, err := readPairingFrame(conn, protocol.MsgPairingStart)
	if err != nil {
		return "", err
	}
	response, err := server.Exchange(start)
	if err != nil {
		return "", fmt.Errorf("pairing exchange failed: %w", err)
	}
	if err := writePairingFrame(conn, protocol.MsgPairingExchange, response); err != nil {
		return "", err
	}
	confirm, err := readPairingFrame(conn, protocol.MsgPairingClientConfirm)
	if err != nil {
		return "", err
	}
	serverConfirm, err := server.Confirm(confirm)
	if err != nil {
		return "", fmt.Errorf("pairing PIN or TLS identity incorrect: %w", err)
	}
	if err := writePairingFrame(conn, protocol.MsgPairingServerConfirm, serverConfirm); err != nil {
		return "", err
	}
	success = true
	return pin, nil
}
