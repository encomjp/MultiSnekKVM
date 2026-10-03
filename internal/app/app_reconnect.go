package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"sort"
	"strings"
	"time"

	"multisnekkvm/internal/discovery"
	"multisnekkvm/internal/link"
	"multisnekkvm/internal/transport"
)

// reconnectCandidatesFor retains fresh addresses ahead of saved addresses
// within each route, and tries reliable direct links before slower fallbacks.
func reconnectCandidatesFor(cfg Settings, peers []discovery.DiscoveredPeer) ([]string, string) {
	if cfg.LastPeerID == "" {
		return nil, ""
	}
	type candidate struct {
		address, kind string
		fresh         bool
	}
	var available []candidate
	seen := make(map[string]bool)
	add := func(address, kind string, fresh bool) {
		if address == "" || seen[address] {
			return
		}
		seen[address] = true
		if kind == "" {
			kind = "network"
		}
		available = append(available, candidate{address, kind, fresh})
	}
	for _, peer := range peers {
		if peer.DeviceID != cfg.LastPeerID {
			continue
		}
		for _, address := range peer.Addresses {
			kind := peer.AddressKinds[address]
			if kind == "" {
				host, _, err := net.SplitHostPort(address)
				if err == nil && link.IsTailscaleIP(net.ParseIP(host)) {
					kind = "tailscale"
				} else {
					kind = "lan"
				}
			}
			add(address, kind, true)
		}
	}
	for kind, address := range cfg.LastPeerAddr {
		add(address, kind, false)
	}
	sort.SliceStable(available, func(i, j int) bool {
		l, r := link.Rank(available[i].kind), link.Rank(available[j].kind)
		if l != r {
			return l < r
		}
		if available[i].fresh != available[j].fresh {
			return available[i].fresh
		}
		return available[i].address < available[j].address
	})
	result := make([]string, 0, len(available))
	for _, c := range available {
		result = append(result, c.address)
	}
	return result, cfg.LastPeerName
}

func (a *App) reconnectCandidates() ([]string, string) {
	var peers []discovery.DiscoveredPeer
	if a.discovery != nil {
		peers = a.discovery.Peers()
	}
	return reconnectCandidatesFor(a.settings.Get(), peers)
}

func (a *App) tryConnectCandidates(logPrefix string, candidates []string) (string, error) {
	return tryCandidates(logPrefix, candidates, func(addr string) error {
		return a.transport.ConnectTo(addr, "")
	})
}

// tryCandidates connects to each address in order until one succeeds. Only
// connectivity failures (transport.ErrDial) advance to the next address; an
// authentication, PIN or trust failure stops immediately so a rejected peer
// identity is never silently retried on another address.
func tryCandidates(logPrefix string, candidates []string, connect func(addr string) error) (string, error) {
	var lastErr error
	for _, addr := range candidates {
		log.Printf("%s: trying %s", logPrefix, addr)
		err := connect(addr)
		if err == nil {
			return addr, nil
		}
		log.Printf("%s: %s failed: %v", logPrefix, addr, err)
		lastErr = err
		if !errors.Is(err, transport.ErrDial) {
			break
		}
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
			a.emitSessionUpdated()

			connectedAddr, lastErr := a.tryConnectCandidates("auto-reconnect", candidates)

			if connectedAddr != "" {
				log.Printf("auto-reconnect: success to %s", connectedAddr)
				// Remember the address that actually worked (it may differ
				// from the one we lost) unless the user changed peer meanwhile.
				a.mu.Lock()
				if a.lastPeerAddr == expectedAddr {
					a.lastPeerAddr = connectedAddr
				}
				a.mu.Unlock()
				expectedAddr = connectedAddr
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
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
	// Revoking trust must also terminate an active session.
	if a.transport != nil {
		if session := a.transport.GetSession(); session != nil && session.PeerID == peerID {
			a.transport.Disconnect()
		}
	}
	a.settings.Update(func(s *Settings) {
		if s.LastPeerID == peerID {
			s.LastPeerID = ""
			s.LastPeerName = ""
			s.LastPeerAddr = nil
		}
	})
	if a.ctx != nil {
		a.emitPeersUpdated()
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
		a.mu.Lock()
		a.lastPeerAddr = connectedAddr
		a.mu.Unlock()
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
		addr := s.RemoteAddr()
		kind := link.RouteKind(addr)
		addrs[kind] = addr
	}

	if a.discovery != nil {
		for _, dp := range a.discovery.Peers() {
			if dp.DeviceID != peerID {
				continue
			}
			for _, addr := range dp.Addresses {
				kind := dp.AddressKinds[addr]
				if kind == "" {
					kind = link.RouteKind(addr)
				}
				if addrs[kind] == "" {
					addrs[kind] = addr
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
