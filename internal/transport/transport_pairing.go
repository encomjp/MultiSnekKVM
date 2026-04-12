package transport

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"multisnekkvm/internal/protocol"
	"multisnekkvm/internal/trust"
)

func (t *Transport) SetPairingCode(code string) {
	t.mu.Lock()
	t.setPairingCodeLocked(normalizePairingCode(code), time.Now())
	t.mu.Unlock()
}

func (t *Transport) CurrentPairingCode() string {
	code, _ := t.RefreshPairingCode()
	return code
}

func (t *Transport) RefreshPairingCode() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ensurePairingCodeLocked(time.Now())
}

func (t *Transport) authorizeInboundPeer(peerHello protocol.HelloMsg, peerFingerprint, endpoint string) error {
	if t.trust == nil {
		return fmt.Errorf("trust store unavailable")
	}
	now := time.Now()
	presentedPairingCode := normalizePairingCode(peerHello.PairingCode)
	requirePairingValidation := presentedPairingCode != ""
	record, ok := t.trust.GetByDeviceID(peerHello.DeviceID)
	if ok {
		if !strings.EqualFold(record.Fingerprint, peerFingerprint) {
			return fmt.Errorf("trusted fingerprint mismatch for %s", peerHello.Name)
		}
	}
	if requirePairingValidation || !ok {
		t.mu.Lock()
		localPairingCode, _ := t.ensurePairingCodeLocked(now)
		if !t.pairingLockedUntil.IsZero() && now.Before(t.pairingLockedUntil) {
			remaining := time.Until(t.pairingLockedUntil).Round(time.Second)
			t.mu.Unlock()
			return fmt.Errorf("pairing PIN temporarily locked; try again in %v", remaining)
		}
		if localPairingCode == "" {
			t.mu.Unlock()
			return fmt.Errorf("pairing PIN unavailable for %s", peerHello.Name)
		}
		if presentedPairingCode != localPairingCode {
			t.recordPairingFailureLocked(now)
			t.mu.Unlock()
			return fmt.Errorf("pairing PIN rejected for %s", peerHello.Name)
		}
		t.mu.Unlock()
	}
	if !ok {
		if err := t.trust.Upsert(trust.TrustedPeer{
			DeviceID:     peerHello.DeviceID,
			Name:         peerHello.Name,
			Fingerprint:  peerFingerprint,
			PairedAt:     now.UTC(),
			LastSeenAt:   now.UTC(),
			LastEndpoint: endpoint,
		}); err != nil {
			return fmt.Errorf("persist trusted peer: %w", err)
		}
	}
	if requirePairingValidation || !ok {
		// Consume the PIN only after any required trust state is persisted so a transient
		// write failure doesn't waste a valid code.
		t.mu.Lock()
		t.setPairingCodeLocked("", now)
		t.mu.Unlock()
	}
	if err := t.trust.MarkSeen(peerHello.DeviceID, endpoint, now); err != nil {
		log.Printf("trust store update failed: %v", err)
	}
	return nil
}

func (t *Transport) authorizeOutboundPeer(peerHello protocol.HelloMsg, peerFingerprint, endpoint string, pairingCode string) error {
	if t.trust == nil {
		return nil
	}
	record, ok := t.trust.GetByDeviceID(peerHello.DeviceID)
	if ok {
		if !strings.EqualFold(record.Fingerprint, peerFingerprint) {
			return fmt.Errorf("trusted fingerprint mismatch for %s", peerHello.Name)
		}
	} else {
		if normalizePairingCode(pairingCode) == "" {
			return fmt.Errorf("peer %s is not trusted; enter the pairing PIN shown on that machine", peerHello.Name)
		}
		now := time.Now().UTC()
		if err := t.trust.Upsert(trust.TrustedPeer{
			DeviceID:     peerHello.DeviceID,
			Name:         peerHello.Name,
			Fingerprint:  peerFingerprint,
			PairedAt:     now,
			LastSeenAt:   now,
			LastEndpoint: endpoint,
		}); err != nil {
			return fmt.Errorf("persist trusted peer: %w", err)
		}
	}
	if err := t.trust.MarkSeen(peerHello.DeviceID, endpoint, time.Now()); err != nil {
		log.Printf("trust store update failed: %v", err)
	}
	return nil
}

func (t *Transport) currentPairingCode() string {
	return t.CurrentPairingCode()
}

func normalizePairingCode(code string) string {
	return strings.TrimSpace(code)
}

func (t *Transport) ensurePairingCodeLocked(now time.Time) (string, bool) {
	if t.pairingCode == "" || t.pairingIssuedAt.IsZero() || now.Sub(t.pairingIssuedAt) >= pairingCodeTTL {
		t.setPairingCodeLocked("", now)
		return t.pairingCode, true
	}
	return t.pairingCode, false
}

func (t *Transport) setPairingCodeLocked(code string, now time.Time) {
	code = normalizePairingCode(code)
	if code == "" {
		code = pairingCodeGenerator()
	}
	if now.IsZero() {
		now = time.Now()
	}
	t.pairingCode = code
	t.pairingIssuedAt = now
	t.pairingFailures = 0
	t.pairingLockedUntil = time.Time{}
}

func (t *Transport) recordPairingFailureLocked(now time.Time) {
	t.pairingFailures++
	backoff := pairingBaseBackoff * time.Duration(1<<(t.pairingFailures-1))
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	t.pairingLockedUntil = now.Add(backoff)
	if t.pairingFailures >= pairingMaxFailures {
		t.setPairingCodeLocked("", now)
		t.pairingLockedUntil = now.Add(30 * time.Second)
	}
}

func generateRandomPairingCode() string {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", value.Int64())
}
