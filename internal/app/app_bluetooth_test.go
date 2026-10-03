package app

import (
	"reflect"
	"testing"

	"multisnekkvm/internal/bluetooth"
	"multisnekkvm/internal/discovery"
	"multisnekkvm/internal/link"
)

func TestNormalizePeerAddressAcceptsBluetooth(t *testing.T) {
	for in, want := range map[string]string{
		"bt://aa:bb:cc:dd:ee:ff":     "bt://AA:BB:CC:DD:EE:FF",
		"00:1a:7d:da:71:13":          "bt://00:1A:7D:DA:71:13", // bare MAC
		"  BT://00:1A:7D:DA:71:13  ": "",                       // scheme is case-sensitive: falls through to host:port
	} {
		got, err := normalizePeerAddress(in, 24831)
		if want == "" {
			if bluetooth.IsAddress(got) {
				t.Errorf("%q must not be treated as bluetooth, got %q", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("normalizePeerAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := normalizePeerAddress("bt://not-a-mac", 24831); err == nil {
		t.Error("malformed bluetooth address must be rejected")
	}
	// IPv6 and host:port handling is unchanged.
	if got, _ := normalizePeerAddress("fe80::1", 24831); got != "[fe80::1]:24831" {
		t.Errorf("ipv6 = %q", got)
	}
	if got, _ := normalizePeerAddress("192.168.0.5", 24831); got != "192.168.0.5:24831" {
		t.Errorf("ipv4 = %q", got)
	}
}

func TestBluetoothAddressesAreTheBluetoothRoute(t *testing.T) {
	if k := link.RouteKind("bt://00:1A:7D:DA:71:13"); k != "bluetooth" {
		t.Fatalf("RouteKind = %q", k)
	}
	if k := link.KindForAddress("bt://00:1A:7D:DA:71:13", nil); k != "bluetooth" {
		t.Fatalf("KindForAddress = %q", k)
	}
	// Bluetooth ranks after every IP route, so it is only chosen when it is the only way.
	for _, better := range []string{"usb4", "usb-bridge", "ethernet", "lan", "wifi", "network", "tailscale"} {
		if link.Rank(better) >= link.Rank("bluetooth") {
			t.Errorf("%s must outrank bluetooth", better)
		}
	}
}

func TestReconnectCandidatesIncludeBluetoothLast(t *testing.T) {
	cfg := Settings{LastPeerID: "p", LastPeerName: "P", LastPeerAddr: map[string]string{
		"bluetooth": "bt://00:1A:7D:DA:71:13",
		"lan":       "192.168.0.10:24831",
	}}
	got, _ := reconnectCandidatesFor(cfg, nil)
	want := []string{"192.168.0.10:24831", "bt://00:1A:7D:DA:71:13"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}

	// A peer found only over Bluetooth discovery (no AddressKinds) is still classified.
	peers := []discovery.DiscoveredPeer{{DeviceID: "p", Addresses: []string{"bt://00:1A:7D:DA:71:13", "100.64.0.9:24831"}}}
	got, _ = reconnectCandidatesFor(Settings{LastPeerID: "p"}, peers)
	want = []string{"100.64.0.9:24831", "bt://00:1A:7D:DA:71:13"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func TestBluetoothStatusBeforeStartup(t *testing.T) {
	a := &App{}
	st := a.GetBluetoothStatus()
	if st.Available || st.Error == "" || st.Devices == nil {
		t.Fatalf("status before startup should be a safe 'unavailable': %+v", st)
	}
	a.RefreshBluetooth() // must not panic without a manager
}
