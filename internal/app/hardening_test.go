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
