package link

import (
	"net"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func newTestCache(clock *fakeClock, adapters *[]Adapter, enumerations, resolutions *int) *routeCache {
	c := newRouteCache()
	c.now = clock.now
	c.enumerate = func() []Adapter {
		*enumerations++
		return append([]Adapter(nil), (*adapters)...)
	}
	c.resolve = func(ip net.IP, list []Adapter) string {
		*resolutions++
		if len(list) == 0 {
			return "lan"
		}
		return list[0].Kind
	}
	return c
}

func TestAdapterCacheTTLAndForce(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	adapters := []Adapter{{Name: "eth", Kind: "ethernet", Addresses: []string{"10.0.0.1"}}}
	enums, res := 0, 0
	c := newTestCache(clock, &adapters, &enums, &res)

	c.get(false)
	c.get(false)
	if enums != 1 {
		t.Fatalf("expected 1 enumeration within TTL, got %d", enums)
	}
	clock.advance(adapterCacheTTL + time.Millisecond)
	c.get(false)
	if enums != 2 {
		t.Fatalf("expected refresh after TTL, got %d", enums)
	}
	c.get(true)
	if enums != 3 {
		t.Fatalf("expected forced refresh, got %d", enums)
	}
}

func TestRouteCacheTTLAndInvalidation(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	adapters := []Adapter{{Name: "eth", Kind: "ethernet", Addresses: []string{"10.0.0.1"}}}
	enums, res := 0, 0
	c := newTestCache(clock, &adapters, &enums, &res)
	ip := net.ParseIP("10.0.0.2")

	if got := c.kindForIP(ip); got != "ethernet" {
		t.Fatalf("kind = %q", got)
	}
	c.kindForIP(ip)
	if res != 1 {
		t.Fatalf("expected cached route, got %d resolutions", res)
	}

	// Adapter refresh with identical configuration keeps routes cached.
	clock.advance(adapterCacheTTL + time.Millisecond)
	c.kindForIP(ip)
	if res != 1 {
		t.Fatalf("unchanged adapters must keep route cache, got %d resolutions", res)
	}

	// Route TTL expiry forces a new resolution.
	clock.advance(routeCacheTTL)
	c.kindForIP(ip)
	if res != 2 {
		t.Fatalf("expected re-resolution after TTL, got %d", res)
	}

	// Changed adapter snapshot invalidates cached routes.
	adapters = []Adapter{{Name: "usb4", Kind: "usb4", Addresses: []string{"169.254.0.1"}}}
	c.get(true)
	if got := c.kindForIP(ip); got != "usb4" {
		t.Fatalf("kind after adapter change = %q", got)
	}
	if res != 3 {
		t.Fatalf("expected invalidation on adapter change, got %d", res)
	}
}
