package link

import (
	"net"
	"sync"
	"time"

	"multisnekkvm/internal/bluetooth"
)

// Enumerating Windows adapters (net.Interfaces, per-interface Addrs and
// GetAdaptersAddresses) and resolving the outbound route for an IP
// (net.DialUDP) are comparatively expensive syscalls. Peer listings and
// session status are polled frequently, so both results are cached.
const (
	adapterCacheTTL = 5 * time.Second
	routeCacheTTL   = 10 * time.Second
)

type routeEntry struct {
	kind    string
	expires time.Time
}

type routeCache struct {
	mu        sync.Mutex
	adapters  []Adapter
	fetchedAt time.Time
	valid     bool
	routes    map[string]routeEntry

	// Injectable for tests.
	now       func() time.Time
	enumerate func() []Adapter
	resolve   func(ip net.IP, adapters []Adapter) string
}

func newRouteCache() *routeCache {
	return &routeCache{
		routes:    make(map[string]routeEntry),
		now:       time.Now,
		enumerate: EnumerateAdapters,
		resolve:   KindForIP,
	}
}

var defaultCache = newRouteCache()

// Adapters returns a snapshot of active adapters that is at most a few
// seconds old. The returned slice must be treated as read-only.
func Adapters() []Adapter { return defaultCache.get(false) }

// RefreshAdapters forces a fresh enumeration, e.g. when the user opens
// the connection interface list. The returned slice is read-only.
func RefreshAdapters() []Adapter { return defaultCache.get(true) }

// RouteKind returns the cached route kind for a host:port address.
func RouteKind(address string) string {
	if bluetooth.IsAddress(address) {
		return "bluetooth"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return "network"
	}
	return defaultCache.kindForIP(net.ParseIP(host))
}

// RouteKindForIP returns the cached route kind for an IP.
func RouteKindForIP(ip net.IP) string { return defaultCache.kindForIP(ip) }

func (c *routeCache) get(force bool) []Adapter {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.adaptersLocked(force)
}

func (c *routeCache) adaptersLocked(force bool) []Adapter {
	now := c.now()
	if !force && c.valid && now.Sub(c.fetchedAt) < adapterCacheTTL {
		return c.adapters
	}
	fresh := c.enumerate()
	if fresh == nil {
		fresh = []Adapter{}
	}
	if !c.valid || !sameAdapters(c.adapters, fresh) {
		// Routes depend on adapter configuration; drop them on change.
		// An unchanged snapshot keeps its identity so in-flight route
		// resolutions against it remain cacheable.
		c.routes = make(map[string]routeEntry)
		c.adapters = fresh
	}
	c.fetchedAt = now
	c.valid = true
	return c.adapters
}

func (c *routeCache) kindForIP(ip net.IP) string {
	if ip == nil {
		return c.resolve(nil, nil)
	}
	key := ip.String()
	c.mu.Lock()
	adapters := c.adaptersLocked(false)
	now := c.now()
	if entry, ok := c.routes[key]; ok && now.Before(entry.expires) {
		c.mu.Unlock()
		return entry.kind
	}
	c.mu.Unlock()

	// Resolve without the lock: DialUDP is a syscall that must not
	// serialize unrelated lookups.
	kind := c.resolve(ip, adapters)

	c.mu.Lock()
	// Only store if the snapshot we resolved against is still current.
	if sameAdapterSlice(c.adapters, adapters) {
		c.routes[key] = routeEntry{kind: kind, expires: now.Add(routeCacheTTL)}
	}
	c.mu.Unlock()
	return kind
}

func sameAdapterSlice(a, b []Adapter) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

func sameAdapters(a, b []Adapter) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Kind != b[i].Kind ||
			a[i].Description != b[i].Description || len(a[i].subnets) != len(b[i].subnets) {
			return false
		}
		for j := range a[i].subnets {
			if a[i].subnets[j].String() != b[i].subnets[j].String() {
				return false
			}
		}
		if len(a[i].Addresses) != len(b[i].Addresses) {
			return false
		}
		for j := range a[i].Addresses {
			if a[i].Addresses[j] != b[i].Addresses[j] {
				return false
			}
		}
	}
	return true
}
