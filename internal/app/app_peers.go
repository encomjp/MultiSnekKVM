package app

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

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
	a.mu.RLock()
	defer a.mu.RUnlock()

	seen := make(map[string]bool)
	var peers []PeerInfo

	if a.discovery != nil {
		for _, dp := range a.discovery.Peers() {
			fingerprint := dp.Fingerprint
			if fingerprint == "" && a.trust != nil {
				if record, ok := a.trust.GetByDeviceID(dp.DeviceID); ok {
					fingerprint = record.Fingerprint
				}
			}
			seen[dp.Address] = true
			routes := append([]string(nil), dp.Routes...)
			sort.Strings(routes)
			peers = append(peers, PeerInfo{
				ID:             dp.DeviceID,
				Name:           dp.Name,
				Address:        dp.Address,
				Addresses:      append([]string(nil), dp.Addresses...),
				Fingerprint:    fingerprint,
				Source:         peerSourceLabel(routes),
				Routes:         routes,
				PreferredRoute: preferredRoute(routes),
				Trusted:        a.trust != nil && a.trust.IsTrusted(dp.DeviceID, fingerprint),
				Status:         "online",
				LastSeen:       dp.LastSeen.Unix(),
			})
		}
	}

	for _, mp := range a.manualPeers {
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

func (a *App) connectWithPairingCode(address, pairingCode string) error {
	if a.transport == nil {
		return fmt.Errorf("transport unavailable")
	}
	normalized, err := normalizePeerAddress(address, a.device.Port)
	if err != nil {
		return err
	}
	if err := a.transport.ConnectTo(normalized, strings.TrimSpace(pairingCode)); err != nil {
		return err
	}
	a.mu.Lock()
	a.lastPeerAddr = normalized
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
	for _, route := range routes {
		if route == "lan" {
			return route
		}
	}
	if len(routes) == 0 {
		return ""
	}
	return routes[0]
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
