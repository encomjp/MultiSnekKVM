package transport

import (
	"testing"
	"time"

	"multisnekkvm/internal/protocol"
	"multisnekkvm/internal/trust"
)

func openTestTrustStore(t *testing.T) *trust.Store {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	store, err := trust.OpenStore()
	if err != nil {
		t.Fatalf("open trust store: %v", err)
	}
	return store
}

func setTestPairingCodes(t *testing.T, codes ...string) {
	t.Helper()
	original := pairingCodeGenerator
	index := 0
	pairingCodeGenerator = func() string {
		if index >= len(codes) {
			return codes[len(codes)-1]
		}
		code := codes[index]
		index++
		return code
	}
	t.Cleanup(func() {
		pairingCodeGenerator = original
	})
}

func TestAuthorizeInboundPeerRequiresMatchingPairingCodeForUnknownPeer(t *testing.T) {
	setTestPairingCodes(t, "482912", "482913")
	store := openTestTrustStore(t)
	transport := &Transport{trust: store}
	transport.SetPairingCode("482911")
	peerHello := protocol.HelloMsg{DeviceID: "peer-1", Name: "Deskbox", PairingCode: "482911"}

	if err := transport.authorizeInboundPeer(peerHello, "fingerprint-1", "192.168.0.10:24831"); err != nil {
		t.Fatalf("authorize inbound peer: %v", err)
	}
	if !store.IsTrusted("peer-1", "fingerprint-1") {
		t.Fatal("expected inbound pairing to trust the peer")
	}
	if got := transport.CurrentPairingCode(); got != "482912" {
		t.Fatalf("pairing code after success = %q, want rotated code 482912", got)
	}

	store = openTestTrustStore(t)
	transport = &Transport{trust: store}
	transport.SetPairingCode("482911")
	peerHello.PairingCode = "111111"
	if err := transport.authorizeInboundPeer(peerHello, "fingerprint-1", "192.168.0.10:24831"); err == nil {
		t.Fatal("expected inbound pairing to reject the wrong pairing code")
	}
	if _, ok := store.GetByDeviceID("peer-1"); ok {
		t.Fatal("did not expect rejected pairing to persist trust")
	}
}

func TestAuthorizeInboundPeerRotatesExpiredPairingCode(t *testing.T) {
	setTestPairingCodes(t, "700001")
	transport := &Transport{}
	transport.SetPairingCode("482911")
	transport.mu.Lock()
	transport.pairingIssuedAt = time.Now().Add(-pairingCodeTTL - time.Second)
	transport.mu.Unlock()

	if got := transport.CurrentPairingCode(); got != "700001" {
		t.Fatalf("expired pairing code = %q, want rotated code 700001", got)
	}
}

func TestAuthorizeInboundPeerLocksAndRotatesAfterRepeatedFailures(t *testing.T) {
	setTestPairingCodes(t, "482912")
	store := openTestTrustStore(t)
	transport := &Transport{trust: store}
	transport.SetPairingCode("482911")
	peerHello := protocol.HelloMsg{DeviceID: "peer-1", Name: "Deskbox", PairingCode: "000000"}

	for attempt := 0; attempt < pairingMaxFailures; attempt++ {
		if err := transport.authorizeInboundPeer(peerHello, "fingerprint-1", "192.168.0.10:24831"); err == nil {
			t.Fatalf("expected failure %d to reject wrong pairing code", attempt+1)
		}
		if attempt < pairingMaxFailures-1 {
			transport.mu.Lock()
			transport.pairingLockedUntil = time.Time{}
			transport.mu.Unlock()
		}
	}

	transport.mu.RLock()
	lockedUntil := transport.pairingLockedUntil
	transport.mu.RUnlock()
	if !lockedUntil.After(time.Now()) {
		t.Fatal("expected repeated failures to lock the pairing PIN temporarily")
	}
	if got := transport.CurrentPairingCode(); got != "482912" {
		t.Fatalf("pairing code after repeated failures = %q, want rotated code 482912", got)
	}
	if _, ok := store.GetByDeviceID("peer-1"); ok {
		t.Fatal("did not expect failed pairing attempts to persist trust")
	}
}

func TestAuthorizeInboundPeerRequiresMatchingPairingCodeEvenForTrustedPeer(t *testing.T) {
	setTestPairingCodes(t, "482912")
	store := openTestTrustStore(t)
	if err := store.Upsert(trust.TrustedPeer{
		DeviceID:     "peer-1",
		Name:         "Deskbox",
		Fingerprint:  "fingerprint-1",
		PairedAt:     time.Now().UTC(),
		LastSeenAt:   time.Now().UTC(),
		LastEndpoint: "192.168.0.10:24831",
	}); err != nil {
		t.Fatalf("seed trust store: %v", err)
	}
	transport := &Transport{trust: store}
	transport.SetPairingCode("482911")
	peerHello := protocol.HelloMsg{DeviceID: "peer-1", Name: "Deskbox", PairingCode: "111111"}

	if err := transport.authorizeInboundPeer(peerHello, "fingerprint-1", "192.168.0.10:24831"); err == nil {
		t.Fatal("expected trusted peer pairing attempt with wrong PIN to be rejected")
	}
	if got := transport.CurrentPairingCode(); got != "482911" {
		t.Fatalf("pairing code after trusted-peer rejection = %q, want unchanged 482911", got)
	}
	transport.mu.Lock()
	transport.pairingLockedUntil = time.Time{}
	transport.mu.Unlock()

	peerHello.PairingCode = "482911"
	if err := transport.authorizeInboundPeer(peerHello, "fingerprint-1", "192.168.0.10:24831"); err != nil {
		t.Fatalf("authorize trusted peer with matching pairing code: %v", err)
	}
	if got := transport.CurrentPairingCode(); got != "482912" {
		t.Fatalf("pairing code after trusted-peer success = %q, want rotated code 482912", got)
	}
}

func TestAuthorizeOutboundPeerRequiresPairingCodeOnlyForFirstTrust(t *testing.T) {
	store := openTestTrustStore(t)
	transport := &Transport{trust: store}
	peerHello := protocol.HelloMsg{DeviceID: "peer-2", Name: "Travel Laptop"}

	if err := transport.authorizeOutboundPeer(peerHello, "fingerprint-2", "100.64.0.10:24831", ""); err == nil {
		t.Fatal("expected first outbound trust to require a pairing code")
	}

	if err := transport.authorizeOutboundPeer(peerHello, "fingerprint-2", "100.64.0.10:24831", "482911"); err != nil {
		t.Fatalf("authorize outbound peer with pairing code: %v", err)
	}
	if !store.IsTrusted("peer-2", "fingerprint-2") {
		t.Fatal("expected outbound pairing to trust the peer")
	}

	if err := transport.authorizeOutboundPeer(peerHello, "fingerprint-2", "100.64.0.10:24831", ""); err != nil {
		t.Fatalf("expected subsequent outbound connect to use stored trust: %v", err)
	}
}
