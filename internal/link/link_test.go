package link

import (
    "net"
    "reflect"
    "testing"
)

func TestKindOnlyClaimsRecognizedAdapters(t *testing.T) {
    cases := map[string]string{
        "USB4 P2P Network Adapter": "usb4",
        "Thunderbolt(TM) Networking": "usb4",
        "USB 3.0 Ethernet Adapter": "usb-bridge",
        "RNDIS USB Network Device": "usb-bridge",
        "USB NCM Network": "usb-bridge",
        "Bluetooth Device (Personal Area Network)": "bluetooth",
        "Wi-Fi": "wifi",
        "Wireless LAN": "wifi",
        "Ethernet 3": "ethernet",
        "USB charging cable": "network",
        "Local Area Connection": "network",
    }
    for name, expected := range cases {
        if got := Kind(name); got != expected {
            t.Errorf("Kind(%q) = %q, want %q", name, got, expected)
        }
    }
}

func TestRouteOrder(t *testing.T) {
    kinds := map[string]string{
        "bt": "bluetooth", "wifi": "wifi",
        "usb4": "usb4", "bridge": "usb-bridge",
        "ethernet": "ethernet", "tailnet": "tailscale",
    }
    actual := SortAddresses([]string{"bt", "tailnet", "wifi", "ethernet", "bridge", "usb4"}, kinds)
    expected := []string{"usb4", "bridge", "ethernet", "wifi", "tailnet", "bt"}
    if !reflect.DeepEqual(actual, expected) {
        t.Fatalf("SortAddresses = %v; want %v", actual, expected)
    }
}

func TestKindForIPFallback(t *testing.T) {
    if got := KindForIP(net.ParseIP("100.92.10.17"), nil); got != "tailscale" {
        t.Fatalf("tailnet route: %q", got)
    }
    if got := KindForIP(net.ParseIP("169.254.12.7"), nil); got != "lan" {
        t.Fatalf("unidentified link-local route: %q", got)
    }
    if got := KindForAddress("no-port", nil); got != "network" {
        t.Fatalf("invalid endpoint route: %q", got)
    }
}

func TestRecognizedAdapterSubnet(t *testing.T) {
    adapters := []Adapter{{
        Name: "USB4 direct link", Kind: "usb4",
        Addresses: []string{"169.254.5.1"},
        subnets: []*net.IPNet{{IP: net.ParseIP("169.254.5.1").To4(), Mask: net.CIDRMask(16, 32)}},
    }}
    // If Windows selects a different route for the address, never falsely
    // claim it traverses USB4. A matching adapter subnet permits USB4 when
    // the OS cannot resolve a local source IP yet.
    got := KindForIP(net.ParseIP("169.254.5.2"), adapters)
    if got != "usb4" && got != "lan" {
        t.Fatalf("unrecognized direct route: %q", got)
    }
}
