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
	"multisnekkvm/internal/link"
	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/resilience"
)

const (
	peerTTL = 15 * time.Second
	// maxPeers bounds memory use if a LAN floods us with fake device IDs.
	maxPeers = 64
	// maxAddressesPerPeer bounds per-peer address growth for the same reason.
	maxAddressesPerPeer = 16
)

type broadcastMessage struct {
	DeviceID string `json:"id"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
}

type DiscoveredPeer struct {
	DeviceID     string
	Name         string
	Address      string
	Addresses    []string
	Fingerprint  string
	Routes       []string
	AddressKinds map[string]string
	LastSeen     time.Time
	addressSeen  map[string]time.Time
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
	// SafeGoRestart recovers panics and restarts with exponential backoff
	// whenever a loop returns early (e.g. ListenUDP failing because the
	// port is temporarily unavailable).
	resilience.SafeGoRestart(ctx, "discovery-listen", d.listen)
	resilience.SafeGoRestart(ctx, "discovery-broadcast", d.broadcast)
	resilience.SafeGoRestart(ctx, "discovery-cleanup", d.cleanup)
	<-ctx.Done()
}

// PeerCount returns the number of tracked peers without resolving routes.
func (d *Discovery) PeerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.peers)
}

func (d *Discovery) Peers() []DiscoveredPeer {
	d.mu.RLock()
	result := make([]DiscoveredPeer, 0, len(d.peers))
	for _, p := range d.peers {
		copyPeer := *p
		copyPeer.Addresses = append([]string(nil), p.Addresses...)
		copyPeer.addressSeen = nil
		result = append(result, copyPeer)
	}
	d.mu.RUnlock()

	// Detect the Windows-selected outgoing network interface outside the
	// discovery mutex. USB4NET and network-class USB bridges are ordinary
	// IP interfaces and do not require a separate wire protocol.
	for i := range result {
		peer := &result[i]
		peer.AddressKinds = make(map[string]string, len(peer.Addresses))
		routes := make(map[string]bool)
		for _, addr := range peer.Addresses {
			kind := link.RouteKind(addr)
			peer.AddressKinds[addr] = kind
			routes[kind] = true
		}
		peer.Addresses = link.SortAddresses(peer.Addresses, peer.AddressKinds)
		peer.Address = ""
		if len(peer.Addresses) > 0 {
			peer.Address = peer.Addresses[0]
		}
		peer.Routes = make([]string, 0, len(routes))
		for route := range routes {
			peer.Routes = append(peer.Routes, route)
		}
		sort.Slice(peer.Routes, func(i, j int) bool {
			left, right := peer.Routes[i], peer.Routes[j]
			if link.Rank(left) != link.Rank(right) {
				return link.Rank(left) < link.Rank(right)
			}
			return left < right
		})
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
		// Returning lets SafeGoRestart retry with backoff.
		log.Printf("discovery: listen error: %v", err)
		return
	}
	defer conn.Close()

	stop := make(chan struct{})
	defer close(stop)
	logutil.SafeGo("discovery-listen-close", func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	})

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

		address := net.JoinHostPort(remoteAddr.IP.String(), fmt.Sprintf("%d", msg.Port))

		now := time.Now()
		d.mu.Lock()
		peer := d.peers[msg.DeviceID]
		if peer == nil {
			if len(d.peers) >= maxPeers {
				d.mu.Unlock()
				logDroppedPeer(msg.DeviceID)
				continue
			}
			peer = &DiscoveredPeer{
				DeviceID:    msg.DeviceID,
				addressSeen: make(map[string]time.Time),
			}
			d.peers[msg.DeviceID] = peer
		}
		peer.Name = msg.Name
		peer.LastSeen = now
		if _, known := peer.addressSeen[address]; known || len(peer.addressSeen) < maxAddressesPerPeer {
			peer.addressSeen[address] = now
		}
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

	// Address, Routes and AddressKinds are derived per call in Peers()
	// from the OS-selected route for each address.
	peer.Addresses = peer.Addresses[:0]
	for address := range peer.addressSeen {
		peer.Addresses = append(peer.Addresses, address)
	}
	sort.Strings(peer.Addresses)
}

var (
	droppedPeerLogMu sync.Mutex
	droppedPeerLogAt time.Time
)

func logDroppedPeer(id string) {
	droppedPeerLogMu.Lock()
	defer droppedPeerLogMu.Unlock()
	if time.Since(droppedPeerLogAt) < time.Minute {
		return
	}
	droppedPeerLogAt = time.Now()
	log.Printf("discovery: peer table full (%d), ignoring new device %.8s", maxPeers, id)
}
