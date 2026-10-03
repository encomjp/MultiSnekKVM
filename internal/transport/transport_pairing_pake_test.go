package transport

import (
	"strings"
	"testing"

	spake2 "github.com/backkem/spake2-go"

	"multisnekkvm/internal/protocol"
)

func TestPairingProtocolDoesNotPutPINInHello(t *testing.T) {
	pin := "123456"
	hello := protocol.HelloMsg{DeviceID: "device", Name: "computer", Fingerprint: "fingerprint", PairingCode: pairingProtocolMarker}
	if strings.Contains(string(hello.Encode()), pin) {
		t.Fatal("PIN must not be serialized in the unauthenticated hello")
	}
}

func TestPairingPAKEBindsObservedCertificateFingerprints(t *testing.T) {
	const pin = "123456"
	client := spake2.NewClient([]byte(pin), pairingOptions("client-fingerprint", "server-fingerprint"))
	server := spake2.NewServer([]byte(pin), pairingOptions("client-fingerprint", "server-fingerprint"))
	start, err := client.Start()
	if err != nil {
		t.Fatal(err)
	}
	reply, err := server.Exchange(start)
	if err != nil {
		t.Fatal(err)
	}
	clientConfirm, err := client.Finish(reply)
	if err != nil {
		t.Fatal(err)
	}
	serverConfirm, err := server.Confirm(clientConfirm)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Verify(serverConfirm); err != nil {
		t.Fatal(err)
	}
}

func TestPairingPAKERejectsCertificateSubstitution(t *testing.T) {
	const pin = "123456"
	// Simulate a relayed handshake across distinct TLS certificates.
	client := spake2.NewClient([]byte(pin), pairingOptions("client-fingerprint", "attacker-server-cert"))
	server := spake2.NewServer([]byte(pin), pairingOptions("attacker-client-cert", "server-fingerprint"))
	start, err := client.Start()
	if err != nil {
		t.Fatal(err)
	}
	reply, err := server.Exchange(start)
	if err != nil {
		t.Fatal(err)
	}
	clientConfirm, err := client.Finish(reply)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Confirm(clientConfirm); err == nil {
		t.Fatal("relayed PAKE across different TLS identities must be rejected")
	}
}

func TestPairingPAKERejectsIncorrectPIN(t *testing.T) {
	client := spake2.NewClient([]byte("111111"), pairingOptions("client-fp", "server-fp"))
	server := spake2.NewServer([]byte("222222"), pairingOptions("client-fp", "server-fp"))
	start, err := client.Start()
	if err != nil {
		t.Fatal(err)
	}
	reply, err := server.Exchange(start)
	if err != nil {
		t.Fatal(err)
	}
	confirm, err := client.Finish(reply)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Confirm(confirm); err == nil {
		t.Fatal("different PINs must not establish trust")
	}
}
