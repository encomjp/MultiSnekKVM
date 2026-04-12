package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"multisnekkvm/internal/identity"
)

const peerTTL = 15 * time.Second

type broadcastMessage struct {
	DeviceID string `json:"id"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
}

type DiscoveredPeer struct {
	DeviceID    string
	Name        string
	Address     string
	Addresses   []string
	Fingerprint string
	Routes      []string
	LastSeen    time.Time
	addressSeen map[string]time.Time
	routeSeen   map[string]time.Time
}

// IPsProvider is satisfied by anything that can return Tailscale target IPs.
type IPsProvider interface {
	TargetIPs() []string
}

type Discovery struct {
	device    identity.DeviceInfo
	port      int
	tailscale IPsProvider
	mu        sync.RWMutex
	peers     map[string]*DiscoveredPeer
}

func NewDiscovery(device identity.DeviceInfo, broadcastPort int, ts IPsProvider) *Discovery {
	return &Discovery{
		device:    device,
		port:      broadcastPort,
		tailscale: ts,
		peers:     make(map[string]*DiscoveredPeer),
	}
}

func (d *Discovery) Run(ctx context.Context) {
	go d.listen(ctx)
	go d.broadcast(ctx)
	go d.cleanup(ctx)
	<-ctx.Done()
}

func (d *Discovery) Peers() []DiscoveredPeer {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]DiscoveredPeer, 0, len(d.peers))
	for _, p := range d.peers {
		copyPeer := *p
		copyPeer.Addresses = append([]string(nil), p.Addresses...)
		copyPeer.Routes = append([]string(nil), p.Routes...)
		result = append(result, copyPeer)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].DeviceID < result[j].DeviceID
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func (d *Discovery) broadcast(ctx context.Context) {
	msg := broadcastMessage{
		DeviceID: d.device.ID,
		Name:     d.device.Name,
		Port:     d.device.Port,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("discovery: marshal error: %v", err)
		return
	}

	d.sendBroadcasts(data)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sendBroadcasts(data)
		}
	}
}

func (d *Discovery) sendBroadcasts(data []byte) {
	ifaces, err := net.Interfaces()
	if err != nil {
		d.sendTo(data, net.IPv4bcast, nil)
		return
	}

	sent := false
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}

			ip := ipNet.IP.To4()
			mask := ipNet.Mask
			bcast := make(net.IP, 4)
			for i := range ip {
				bcast[i] = ip[i] | ^mask[i]
			}

			d.sendTo(data, bcast, ip)
			sent = true
		}
	}

	if !sent {
		d.sendTo(data, net.IPv4bcast, nil)
	}

	if d.tailscale != nil {
		for _, target := range d.tailscale.TargetIPs() {
			if ip := net.ParseIP(target); ip != nil {
				d.sendTo(data, ip, nil)
			}
		}
	}
}

func (d *Discovery) sendTo(data []byte, broadcastIP net.IP, localIP net.IP) {
	var laddr *net.UDPAddr
	if localIP != nil {
		laddr = &net.UDPAddr{IP: localIP}
	}

	conn, err := net.DialUDP("udp4", laddr, &net.UDPAddr{
		IP:   broadcastIP,
		Port: d.port,
	})
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write(data)
}

func (d *Discovery) listen(ctx context.Context) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: d.port})
	if err != nil {
		log.Printf("discovery: listen error: %v", err)
		return
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, 4096)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		var msg broadcastMessage
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}

		if msg.DeviceID == d.device.ID {
			continue
		}

		route := routeForIP(remoteAddr.IP)
		address := net.JoinHostPort(remoteAddr.IP.String(), fmt.Sprintf("%d", msg.Port))

		now := time.Now()
		d.mu.Lock()
		peer := d.peers[msg.DeviceID]
		if peer == nil {
			peer = &DiscoveredPeer{
				DeviceID:    msg.DeviceID,
				addressSeen: make(map[string]time.Time),
				routeSeen:   make(map[string]time.Time),
			}
			d.peers[msg.DeviceID] = peer
		}
		peer.Name = msg.Name
		peer.LastSeen = now
		peer.addressSeen[address] = now
		peer.routeSeen[route] = now
		refreshPeerSnapshot(peer, now.Add(-peerTTL))
		d.mu.Unlock()
	}
}

func (d *Discovery) cleanup(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.mu.Lock()
			cutoff := time.Now().Add(-peerTTL)
			for id, peer := range d.peers {
				if peer.LastSeen.Before(cutoff) {
					delete(d.peers, id)
					continue
				}
				refreshPeerSnapshot(peer, cutoff)
			}
			d.mu.Unlock()
		}
	}
}

func refreshPeerSnapshot(peer *DiscoveredPeer, cutoff time.Time) {
	for address, seenAt := range peer.addressSeen {
		if seenAt.Before(cutoff) {
			delete(peer.addressSeen, address)
		}
	}
	for route, seenAt := range peer.routeSeen {
		if seenAt.Before(cutoff) {
			delete(peer.routeSeen, route)
		}
	}

	peer.Addresses = peer.Addresses[:0]
	for address := range peer.addressSeen {
		peer.Addresses = append(peer.Addresses, address)
	}
	sort.Strings(peer.Addresses)

	peer.Routes = peer.Routes[:0]
	for route := range peer.routeSeen {
		peer.Routes = append(peer.Routes, route)
	}
	sort.Strings(peer.Routes)

	peer.Address = ""
	for _, address := range peer.Addresses {
		if shouldPreferAddress(peer.Address, address) {
			peer.Address = address
		}
	}
}

func routeForIP(ip net.IP) string {
	if IsTailscaleIP(ip) {
		return "tailscale"
	}
	return "lan"
}

func shouldPreferAddress(current, candidate string) bool {
	if current == "" {
		return true
	}
	currentHost, _, currentErr := net.SplitHostPort(current)
	candidateHost, _, candidateErr := net.SplitHostPort(candidate)
	if currentErr != nil || candidateErr != nil {
		return current == ""
	}
	return addressRank(candidateHost) < addressRank(currentHost)
}

func addressRank(host string) int {
	ip := net.ParseIP(host)
	if ip == nil {
		return 1
	}
	if IsTailscaleIP(ip) {
		return 1
	}
	return 0
}

// IsTailscaleIP reports whether ip is in a Tailscale address range.
func IsTailscaleIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 100 && ip4[1]&0xc0 == 64
	}
	return len(ip) >= 6 && ip[0] == 0xfd && ip[1] == 0x7a && ip[2] == 0x11 && ip[3] == 0x5c && ip[4] == 0xa1 && ip[5] == 0xe0
}
