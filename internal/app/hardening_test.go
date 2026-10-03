package app

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"multisnekkvm/internal/transport"
)

func TestTryCandidatesStopsOnTrustError(t *testing.T) {
	var tried []string
	trustErr := errors.New("trusted fingerprint mismatch for peer")
	_, err := tryCandidates("test", []string{"a:1", "b:1", "c:1"}, func(addr string) error {
		tried = append(tried, addr)
		return trustErr
	})
	if !errors.Is(err, trustErr) {
		t.Fatalf("err = %v, want trust error", err)
	}
	if !reflect.DeepEqual(tried, []string{"a:1"}) {
		t.Fatalf("tried %v; a trust failure must not advance to other addresses", tried)
	}
}

func TestTryCandidatesAdvancesOnDialError(t *testing.T) {
	var tried []string
	addr, err := tryCandidates("test", []string{"a:1", "b:1", "c:1"}, func(addr string) error {
		tried = append(tried, addr)
		if addr == "b:1" {
			return nil
		}
		return fmt.Errorf("wrapped: %w", transport.ErrDial)
	})
	if err != nil || addr != "b:1" {
		t.Fatalf("addr=%q err=%v", addr, err)
	}
	if !reflect.DeepEqual(tried, []string{"a:1", "b:1"}) {
		t.Fatalf("tried %v", tried)
	}
}

func TestReleaseAllModifiersOnlyAfterInjectedInput(t *testing.T) {
	orig := ReleaseAllModifiers
	defer func() { ReleaseAllModifiers = orig }()
	calls := 0
	ReleaseAllModifiers = func() { calls++ }

	a := &App{}
	a.releaseInjectedRemoteKeys()
	if calls != 0 {
		t.Fatalf("controller side (nothing injected) must not release modifiers, got %d calls", calls)
	}

	if _, allowed := a.notePeerControlInput(false); !allowed {
		t.Fatal("expected input to be allowed")
	}
	a.releaseInjectedRemoteKeys()
	if calls != 1 {
		t.Fatalf("controlled side must release modifiers after injecting, got %d calls", calls)
	}
	a.releaseInjectedRemoteKeys()
	if calls != 1 {
		t.Fatalf("no injection since last release; got %d calls", calls)
	}
}
