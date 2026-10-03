package app

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"multisnekkvm/internal/discovery"
	"multisnekkvm/internal/link"
	"multisnekkvm/internal/logutil"
)

func (a *App) syncPairingCode() bool {
	if a.transport == nil {
		return false
	}
	code, _ := a.transport.RefreshPairingCode()
	if code == "" {
		return false
	}
	a.mu.Lock()
	changed := a.device.PairingCode != code
	a.device.PairingCode = code
	a.mu.Unlock()
	return changed
}

func (a *App) deviceSnapshot() DeviceInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.device
}

func (a *App) GetDevice() DeviceInfo {
	a.syncPairingCode()
	return a.deviceSnapshot()
}

func (a *App) GetPeers() []PeerInfo {
	// Discovery resolves routes per address (syscalls); never hold a.mu
	// across it, since inbound input frames take a.mu.Lock.
	var discovered []discovery.DiscoveredPeer
	if a.discovery != nil {
		discovered = a.discovery.Peers()
	}

	a.mu.RLock()
	manual := make([]PeerInfo, 0, len(a.manualPeers))
	for _, mp := range a.manualPeers {
		manual = append(manual, mp)
	}
	a.mu.RUnlock()

	seen := make(map[string]bool)
	var peers []PeerInfo

	for _, dp := range discovered {
		fingerprint := dp.Fingerprint
		if fingerprint == "" && a.trust != nil {
			if record, ok := a.trust.GetByDeviceID(dp.DeviceID); ok {
				fingerprint = record.Fingerprint
			}
		}
		seen[dp.Address] = true
		routes := append([]string(nil), dp.Routes...)
		sort.Slice(routes, func(i, j int) bool { return link.Rank(routes[i]) < link.Rank(routes[j]) })
		peers = append(peers, PeerInfo{
			ID:             dp.DeviceID,
			Name:           dp.Name,
			Address:        dp.Address,
			Addresses:      append([]string(nil), dp.Addresses...),
			AddressKinds:   dp.AddressKinds,
			Fingerprint:    fingerprint,
			Source:         peerSourceLabel(routes),
			Routes:         routes,
			PreferredRoute: preferredRoute(routes),
			Trusted:        a.trust != nil && a.trust.IsTrusted(dp.DeviceID, fingerprint),
			Status:         "online",
			LastSeen:       dp.LastSeen.Unix(),
		})
	}

	for _, mp := range manual {
		if !seen[mp.Address] {
			mp.Trusted = a.trust != nil && a.trust.IsTrusted(mp.ID, mp.Fingerprint)
			if mp.Trusted && mp.Status == "added" {
				mp.Status = "trusted"
			}
			mp.Addresses = []string{mp.Address}
			mp.Routes = []string{"manual"}
			mp.PreferredRoute = "manual"
			peers = append(peers, mp)
		}
	}

	sort.Slice(peers, func(i, j int) bool {
		left := strings.ToLower(peers[i].Name)
		right := strings.ToLower(peers[j].Name)
		if peers[i].Status == peers[j].Status {
			return left < right
		}
		return peers[i].Status < peers[j].Status
	})

	return peers
}

func (a *App) GetTailscaleStatus() TailscaleStatus {
	if a.tailscale == nil {
		return TailscaleStatus{}
	}
	return a.tailscale.Status()
}

func (a *App) GetRecentLogs() []string {
	return GetRecentLogsSnapshot()
}

// GetLogAnalysis analyzes recent in-memory logs for anomaly patterns and returns
// a structured classification. Useful for frontend diagnostics and health dashboards.
func (a *App) GetLogAnalysis() logutil.LogAnalysis {
	return logutil.AnalyzeRecentLogs()
}

func (a *App) GetSession() SessionStatus {
	if a.transport == nil {
		return SessionStatus{LatencyMs: -1, AudioLatencyMs: -1, JitterMs: -1}
	}
	s := a.transport.GetSession()
	if s == nil {
		return SessionStatus{LatencyMs: -1, AudioLatencyMs: -1, JitterMs: -1}
	}
	controlling := false
	if a.inputHook != nil {
		controlling = a.inputHook.IsInRemoteMode()
	}
	lat, _, _, audioLat := a.currentAudioLatencyState()
	jitter := a.currentJitterMs()
	return SessionStatus{
		Route:          link.RouteKind(s.RemoteAddr()),
		RemoteAddress:  s.RemoteAddr(),
		Connected:      true,
		Controlling:    controlling,
		PeerName:       s.PeerName,
		PeerID:         s.PeerID,
		Role:           s.Role,
		LatencyMs:      lat,
		AudioLatencyMs: audioLat,
		JitterMs:       jitter,
	}
}

func (a *App) AddPeer(address string) error {
	normalized, err := normalizePeerAddress(address, a.device.Port)
	if err != nil {
		return err
	}
	if normalized == "" {
		return fmt.Errorf("address is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.manualPeers[normalized] = PeerInfo{
		ID:             normalized,
		Name:           normalized,
		Address:        normalized,
		Addresses:      []string{normalized},
		Source:         "manual",
		Routes:         []string{"manual"},
		PreferredRoute: "manual",
		Status:         "added",
		LastSeen:       time.Now().Unix(),
	}
	return nil
}

func (a *App) RemovePeer(address string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.manualPeers, address)
}

func (a *App) Connect(address string) error {
	return a.connectWithPairingCode(address, "")
}

// peerConnectionCandidates only tries addresses that discovery associated
// with the *same* device. An explicitly selected address always goes first.
func peerConnectionCandidates(selected string, peers []discovery.DiscoveredPeer) []string {
	candidates := []string{selected}
	for _, peer := range peers {
		found := peer.Address == selected
		for _, address := range peer.Addresses {
			if address == selected {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		for _, address := range peer.Addresses {
			if address != selected {
				candidates = append(candidates, address)
			}
		}
		break
	}
	return candidates
}

func (a *App) connectWithPairingCode(address, pairingCode string) error {
	if a.transport == nil {
		return fmt.Errorf("transport unavailable")
	}
	normalized, err := normalizePeerAddress(address, a.device.Port)
	if err != nil {
		return err
	}
	candidates := []string{normalized}
	if a.discovery != nil {
		candidates = peerConnectionCandidates(normalized, a.discovery.Peers())
	}

	// Only connectivity failures advance to the next address. An
	// authentication, PIN or trust failure must not silently switch to
	// another address or retry the PIN.
	code := strings.TrimSpace(pairingCode)
	connectedAddr, err := tryCandidates("connect", candidates, func(addr string) error {
		return a.transport.ConnectTo(addr, code)
	})
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.lastPeerAddr = connectedAddr
	if session := a.transport.GetSession(); session != nil {
		if manualPeer, ok := a.manualPeers[normalized]; ok {
			manualPeer.ID = session.PeerID
			manualPeer.Name = session.PeerName
			manualPeer.Fingerprint = session.PeerFingerprint
			manualPeer.Status = "trusted"
			manualPeer.LastSeen = time.Now().Unix()
			a.manualPeers[normalized] = manualPeer
		}
	}
	a.mu.Unlock()
	return nil
}

func (a *App) Disconnect() {
	a.mu.Lock()
	a.lastPeerAddr = ""
	a.mu.Unlock()
	a.releaseInjectedRemoteKeys()
	if a.inputHook != nil {
		a.inputHook.SetConnected(false, nil)
	}
	if a.transport != nil {
		a.transport.Disconnect()
	}
}

func peerSourceLabel(routes []string) string {
	if len(routes) == 0 {
		return "discovered"
	}
	if len(routes) == 1 {
		return routes[0]
	}
	return "hybrid"
}

func preferredRoute(routes []string) string {
	if len(routes) == 0 {
		return ""
	}
	preferred := routes[0]
	for _, route := range routes[1:] {
		if link.Rank(route) < link.Rank(preferred) {
			preferred = route
		}
	}
	return preferred
}

func normalizePeerAddress(raw string, defaultPort int) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("address is required")
	}

	if host, port, err := net.SplitHostPort(trimmed); err == nil {
		if strings.TrimSpace(host) == "" {
			return "", fmt.Errorf("host is required")
		}
		if strings.TrimSpace(port) == "" {
			port = strconv.Itoa(defaultPort)
		}
		return net.JoinHostPort(host, port), nil
	}

	if ip := net.ParseIP(trimmed); ip != nil {
		return net.JoinHostPort(trimmed, strconv.Itoa(defaultPort)), nil
	}

	if strings.Count(trimmed, ":") > 1 {
		return "", fmt.Errorf("IPv6 addresses with ports must use [addr]:port")
	}

	return net.JoinHostPort(trimmed, strconv.Itoa(defaultPort)), nil
}

// ConnectionInterface exposes active IP-capable adapters for direct-link setup.
// USB4/Thunderbolt networking and Bluetooth PAN appear as ordinary IP adapters
// when Windows and the attached hardware support them; no raw USB/Bluetooth
// transport is implied by this API.
type ConnectionInterface struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"`
	Addresses   []string `json:"addresses"`
}

func connectionInterfaceKind(name string) string {
	return link.Kind(name)
}

// GetConnectionInterfaces lists usable Windows IP adapters. A standard
// USB-C host port is not exposed as a direct link unless Windows has
// established USB4NET or the bridge driver offers a network adapter.
func (a *App) GetConnectionInterfaces() []ConnectionInterface {
	adapters := link.RefreshAdapters()
	result := make([]ConnectionInterface, 0, len(adapters))
	for _, adapter := range adapters {
		result = append(result, ConnectionInterface{
			Name: adapter.Name, Description: adapter.Description, Kind: adapter.Kind,
			Addresses: adapter.Addresses,
		})
	}
	return result
}
