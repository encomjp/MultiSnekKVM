// Package link discovers IP-capable adapters and ranks routes without
// inventing hardware support. USB4NET, network-class USB bridges and
// Bluetooth PAN are usable only after Windows exposes an IP adapter.
package link

import (
	"multisnekkvm/internal/bluetooth"
	"net"
	"sort"
	"strings"
)

type Adapter struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"`
	Addresses   []string `json:"addresses"`
	subnets     []*net.IPNet
}

func Kind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "usb4"), strings.Contains(n, "thunderbolt"):
		return "usb4"
	case strings.Contains(n, "bluetooth"), strings.Contains(n, "personal area"):
		return "bluetooth"
	case strings.Contains(n, "usb") && (strings.Contains(n, "ethernet") ||
		strings.Contains(n, "rndis") || strings.Contains(n, "ncm") ||
		strings.Contains(n, "bridge") || strings.Contains(n, "network")):
		return "usb-bridge"
	case strings.Contains(n, "wi-fi"), strings.Contains(n, "wifi"),
		strings.Contains(n, "wlan"), strings.Contains(n, "wireless"):
		return "wifi"
	case strings.Contains(n, "ethernet"), strings.Contains(n, "gigabit"),
		strings.Contains(n, "realtek pcie"):
		return "ethernet"
	default:
		return "network"
	}
}

// KindFromNames uses the driver description when it identifies hardware
// more precisely than the user-customizable network connection alias.
func KindFromNames(name, description string) string {
	kind := Kind(description)
	if kind != "network" {
		return kind
	}
	return Kind(name)
}

// EnumerateAdapters returns active interfaces with usable IP addresses by
// querying the OS directly. Adapter descriptions vary by Windows driver; an
// unrecognized name is "network", never assumed to be USB or Ethernet.
// Most callers should use the cached Adapters instead.
func EnumerateAdapters() []Adapter {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []Adapter{}
	}
	result := make([]Adapter, 0, len(ifaces))
	descriptions := adapterDescriptions()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		description := descriptions[iface.Index]
		a := Adapter{Name: iface.Name, Description: description, Kind: KindFromNames(iface.Name, description), Addresses: []string{}}
		for _, address := range addrs {
			subnet, ok := address.(*net.IPNet)
			if !ok || subnet.IP == nil || subnet.IP.IsLoopback() || subnet.IP.IsUnspecified() {
				continue
			}
			// Scoped IPv6 link-local endpoints need an interface zone and
			// are not offered by the IPv4-based peer discovery mechanism.
			if subnet.IP.To4() == nil && subnet.IP.IsLinkLocalUnicast() {
				continue
			}
			a.Addresses = append(a.Addresses, subnet.IP.String())
			a.subnets = append(a.subnets, subnet)
		}
		if len(a.Addresses) != 0 {
			sort.Strings(a.Addresses)
			result = append(result, a)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if Rank(result[i].Kind) != Rank(result[j].Kind) {
			return Rank(result[i].Kind) < Rank(result[j].Kind)
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// Rank sets route preference on NEW connections, not mid-session. Bluetooth
// PAN is a fallback because it may struggle with real-time audio and files.
func Rank(kind string) int {
	switch kind {
	case "usb4":
		return 0
	case "usb-bridge":
		return 1
	case "ethernet":
		return 2
	case "lan":
		return 3
	case "wifi":
		return 4
	case "network":
		return 5
	case "tailscale":
		return 6
	case "bluetooth":
		return 7
	case "manual":
		return 8
	default:
		return 9
	}
}

func IsTailscaleIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		return v4[0] == 100 && v4[1]&0xc0 == 64
	}
	return len(ip) >= 6 && ip[0] == 0xfd && ip[1] == 0x7a &&
		ip[2] == 0x11 && ip[3] == 0x5c && ip[4] == 0xa1 && ip[5] == 0xe0
}

// KindForIP asks the OS to choose the outbound source IP for a UDP socket;
// no network payload is sent. The selected adapter also controls TCP route
// choice for the same destination under normal Windows routing rules.
// If the route cannot be resolved we retain the original LAN behavior.
func KindForIP(ip net.IP, adapters []Adapter) string {
	if IsTailscaleIP(ip) {
		return "tailscale"
	}
	if ip == nil {
		return "network"
	}

	network := "udp6"
	if ip.To4() != nil {
		network = "udp4"
	}
	conn, err := net.DialUDP(network, nil, &net.UDPAddr{IP: ip, Port: 24831})
	if err == nil {
		local := conn.LocalAddr().(*net.UDPAddr).IP
		_ = conn.Close()
		if !local.IsUnspecified() {
			for _, adapter := range adapters {
				for _, addr := range adapter.Addresses {
					if local.Equal(net.ParseIP(addr)) {
						return adapter.Kind
					}
				}
			}
		}
	}

	// A directly connected subnet can still be identified when the OS
	// refuses to choose a route before link configuration finishes.
	for _, adapter := range adapters {
		for _, subnet := range adapter.subnets {
			if subnet.Contains(ip) {
				return adapter.Kind
			}
		}
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return "lan"
	}
	return "network"
}

func KindForAddress(address string, adapters []Adapter) string {
	if bluetooth.IsAddress(address) {
		return "bluetooth"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return "network"
	}
	return KindForIP(net.ParseIP(host), adapters)
}

func SortAddresses(addresses []string, kinds map[string]string) []string {
	out := append([]string(nil), addresses...)
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := Rank(kinds[out[i]]), Rank(kinds[out[j]])
		if li != lj {
			return li < lj
		}
		return out[i] < out[j]
	})
	return out
}
