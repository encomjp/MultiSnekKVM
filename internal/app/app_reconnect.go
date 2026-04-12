package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net"
	"strings"
	"time"

	"multisnekkvm/internal/discovery"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func reconnectCandidatesFor(cfg Settings, peers []discovery.DiscoveredPeer) ([]string, string) {
	if cfg.LastPeerID == "" {
		return nil, ""
	}

	appendUnique := func(candidates []string, seen map[string]struct{}, addr string) []string {
		if addr == "" {
			return candidates
		}
		if _, ok := seen[addr]; ok {
			return candidates
		}
		seen[addr] = struct{}{}
		return append(candidates, addr)
	}

	seen := make(map[string]struct{})
	var lanFresh []string
	var tailscaleFresh []string

	for _, dp := range peers {
		if dp.DeviceID != cfg.LastPeerID {
			continue
		}
		for _, addr := range dp.Addresses {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				continue
			}
			ip := net.ParseIP(host)
			if ip != nil && !isTailscaleIP(ip) {
				lanFresh = appendUnique(lanFresh, seen, addr)
			} else {
				tailscaleFresh = appendUnique(tailscaleFresh, seen, addr)
			}
		}
	}

	var lanSaved []string
	var tailscaleSaved []string
	if addr, ok := cfg.LastPeerAddr["lan"]; ok {
		lanSaved = appendUnique(lanSaved, seen, addr)
	}
	if addr, ok := cfg.LastPeerAddr["tailscale"]; ok {
		tailscaleSaved = appendUnique(tailscaleSaved, seen, addr)
	}

	candidates := append([]string{}, lanFresh...)
	candidates = append(candidates, lanSaved...)
	candidates = append(candidates, tailscaleFresh...)
	candidates = append(candidates, tailscaleSaved...)

	return candidates, cfg.LastPeerName
}

func (a *App) reconnectCandidates() ([]string, string) {
	var peers []discovery.DiscoveredPeer
	if a.discovery != nil {
		peers = a.discovery.Peers()
	}
	return reconnectCandidatesFor(a.settings.Get(), peers)
}

func (a *App) tryConnectCandidates(logPrefix string, candidates []string) (string, error) {
	var lastErr error
	for _, addr := range candidates {
		log.Printf("%s: trying %s", logPrefix, addr)
		if err := a.transport.ConnectTo(addr, ""); err != nil {
			log.Printf("%s: %s failed: %v", logPrefix, addr, err)
			lastErr = err
			continue
		}
		return addr, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no candidates available")
	}
	return "", lastErr
}

func (a *App) reconnectLoop(ctx context.Context, expectedAddr string) {
	a.mu.Lock()
	if a.reconnecting {
		a.mu.Unlock()
		return
	}
	a.reconnecting = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.reconnecting = false
		a.mu.Unlock()
	}()

	if a.health != nil {
		a.health.SetReconnecting(true)
		defer a.health.SetReconnecting(false)
	}

	delay := 2 * time.Second
	const maxDelay = 30 * time.Second

	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return
		}

		a.mu.RLock()
		current := a.lastPeerAddr
		reconnectEnabled := a.autoReconnect
		a.mu.RUnlock()
		if !reconnectEnabled {
			log.Printf("auto-reconnect: cancelled (disabled)")
			return
		}
		if current != expectedAddr {
			log.Printf("auto-reconnect: cancelled (peer changed)")
			return
		}

		if a.transport.GetSession() != nil {
			log.Printf("auto-reconnect: session already active, stopping")
			return
		}

		candidates, peerName := a.reconnectCandidates()
		if len(candidates) == 0 {
			log.Printf("auto-reconnect: no addresses known for peer %s yet (attempt %d, retry in %v)", peerName, attempt, delay)
		} else {
			log.Printf("auto-reconnect: attempt %d to %s via %d candidates (backoff %v)", attempt, peerName, len(candidates), delay)
			wailsRuntime.EventsEmit(a.ctx, "session-updated", a.GetSession())

			connectedAddr, lastErr := a.tryConnectCandidates("auto-reconnect", candidates)

			if connectedAddr != "" {
				log.Printf("auto-reconnect: success to %s", connectedAddr)
				time.Sleep(3 * time.Second)
				if a.transport.GetSession() != nil {
					return
				}
				log.Printf("auto-reconnect: session dropped immediately after connect (likely rejected)")
			} else {
				log.Printf("auto-reconnect: failed across all candidates: %v", lastErr)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func (a *App) GetHealthStatus() HealthStatus {
	if a.health == nil {
		return HealthStatus{Healthy: true}
	}
	return a.health.Status()
}

func (a *App) GetAutoReconnect() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.autoReconnect
}

func (a *App) SetAutoReconnect(enabled bool) {
	a.mu.Lock()
	a.autoReconnect = enabled
	a.mu.Unlock()
	a.settings.Update(func(s *Settings) { s.AutoReconnect = &enabled })
}

func (a *App) TrustPeer(address, pairingCode string) error {
	pairingCode = strings.TrimSpace(pairingCode)
	if pairingCode == "" {
		return fmt.Errorf("pairing PIN is required")
	}
	if len(pairingCode) != 6 {
		return fmt.Errorf("pairing PIN must be 6 digits")
	}
	for _, c := range pairingCode {
		if c < '0' || c > '9' {
			return fmt.Errorf("pairing PIN must be 6 digits")
		}
	}
	return a.connectWithPairingCode(address, pairingCode)
}

func (a *App) UntrustPeer(peerID string) error {
	if a.trust == nil {
		return fmt.Errorf("trust store unavailable")
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return fmt.Errorf("peer id is required")
	}
	if err := a.trust.Remove(peerID); err != nil {
		return err
	}
	a.settings.Update(func(s *Settings) {
		if s.LastPeerID == peerID {
			s.LastPeerID = ""
			s.LastPeerName = ""
			s.LastPeerAddr = nil
		}
	})
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "peers-updated", a.GetPeers())
	}
	return nil
}

func (a *App) Reconnect() error {
	if a.transport == nil {
		return fmt.Errorf("transport unavailable")
	}
	if a.transport.GetSession() != nil {
		return fmt.Errorf("already connected")
	}

	cfg := a.settings.Get()
	if cfg.LastPeerID == "" {
		return fmt.Errorf("no previous peer saved")
	}

	candidates, peerName := a.reconnectCandidates()
	if len(candidates) == 0 {
		return fmt.Errorf("no addresses known for peer %s", peerName)
	}

	log.Printf("reconnect: trying %d addresses for %s", len(candidates), peerName)
	connectedAddr, err := a.tryConnectCandidates("reconnect", candidates)
	if err == nil {
		log.Printf("reconnect: connected via %s", connectedAddr)
		return nil
	}
	return fmt.Errorf("all addresses failed: %v", err)
}

func generatePairingCode() string {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", value.Int64())
}

func (a *App) GetLastPeer() map[string]string {
	cfg := a.settings.Get()
	if cfg.LastPeerID == "" {
		return nil
	}
	result := map[string]string{
		"id":   cfg.LastPeerID,
		"name": cfg.LastPeerName,
	}
	for route, addr := range cfg.LastPeerAddr {
		result[route] = addr
	}
	return result
}

func (a *App) saveLastPeer(peerID, peerName string) {
	addrs := make(map[string]string)

	if s := a.transport.GetSession(); s != nil && s.Role == "controller" {
		remoteAddr := s.RemoteAddr()
		host, _, err := net.SplitHostPort(remoteAddr)
		if err == nil {
			ip := net.ParseIP(host)
			if ip != nil && isTailscaleIP(ip) {
				addrs["tailscale"] = remoteAddr
			} else {
				addrs["lan"] = remoteAddr
			}
		}
	}

	if a.discovery != nil {
		for _, dp := range a.discovery.Peers() {
			if dp.DeviceID != peerID {
				continue
			}
			for _, addr := range dp.Addresses {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					continue
				}
				ip := net.ParseIP(host)
				if ip != nil && isTailscaleIP(ip) {
					if addrs["tailscale"] == "" {
						addrs["tailscale"] = addr
					}
				} else if addrs["lan"] == "" {
					addrs["lan"] = addr
				}
			}
		}
	}

	a.settings.Update(func(s *Settings) {
		s.LastPeerID = peerID
		s.LastPeerName = peerName
		s.LastPeerAddr = addrs
	})
	log.Printf("saved last peer: %s (%s) addrs=%v", peerName, shortPeerID(peerID), addrs)
}

func shortPeerID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
