package discovery

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"multisnekkvm/internal/identity"
)

func TestMergeExtraPeersAddsRouteToKnownPeer(t *testing.T) {
	now := time.Now()
	lan := []DiscoveredPeer{{DeviceID: "studio", Name: "Studio PC", Addresses: []string{"192.168.0.42:24831"}}}
	got := mergeExtraPeers(lan, []ExtraPeer{{DeviceID: "studio", Name: "ignored", Address: "bt://00:1A:7D:DA:71:13"}}, now)
	if len(got) != 1 {
		t.Fatalf("want one merged peer, got %d", len(got))
	}
	if got[0].Name != "Studio PC" {
		t.Errorf("existing name must win, got %q", got[0].Name)
	}
	want := []string{"192.168.0.42:24831", "bt://00:1A:7D:DA:71:13"}
	if !reflect.DeepEqual(got[0].Addresses, want) {
		t.Errorf("addresses = %v, want %v", got[0].Addresses, want)
	}
}

func TestMergeExtraPeersCreatesBluetoothOnlyPeer(t *testing.T) {
	now := time.Now()
	got := mergeExtraPeers(nil, []ExtraPeer{{DeviceID: "laptop", Name: "Laptop", Address: "bt://00:00:00:00:00:02"}}, now)
	if len(got) != 1 || got[0].DeviceID != "laptop" || got[0].Name != "Laptop" || !got[0].LastSeen.Equal(now) {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeExtraPeersIgnoresDuplicatesAndInvalid(t *testing.T) {
	now := time.Now()
	got := mergeExtraPeers(nil, []ExtraPeer{
		{DeviceID: "a", Address: "bt://00:00:00:00:00:01"},
		{DeviceID: "a", Address: "bt://00:00:00:00:00:01"}, // duplicate
		{DeviceID: "", Address: "bt://00:00:00:00:00:03"},  // no id
		{DeviceID: "b", Address: ""},                       // no address
	}, now)
	if len(got) != 1 || len(got[0].Addresses) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeExtraPeersRespectsPeerCap(t *testing.T) {
	var extras []ExtraPeer
	for i := 0; i < maxPeers+10; i++ {
		extras = append(extras, ExtraPeer{DeviceID: fmt.Sprintf("d%d", i), Address: fmt.Sprintf("bt://00:00:00:00:%02X:01", i)})
	}
	if got := mergeExtraPeers(nil, extras, time.Now()); len(got) != maxPeers {
		t.Fatalf("peer table grew to %d, cap is %d", len(got), maxPeers)
	}
}

func TestPeersIncludesExtraSourceWithBluetoothRoute(t *testing.T) {
	d := NewDiscovery(identity.DeviceInfo{ID: "self", Name: "This PC", Port: 24831}, 0, nil)
	d.SetExtraSource(func() []ExtraPeer {
		return []ExtraPeer{{DeviceID: "laptop", Name: "Laptop", Address: "bt://00:00:00:00:00:02"}}
	})
	peers := d.Peers()
	if len(peers) != 1 {
		t.Fatalf("want 1 peer, got %d", len(peers))
	}
	p := peers[0]
	if p.Address != "bt://00:00:00:00:00:02" || p.AddressKinds[p.Address] != "bluetooth" || !reflect.DeepEqual(p.Routes, []string{"bluetooth"}) {
		t.Fatalf("bluetooth peer resolved as %+v", p)
	}
}
