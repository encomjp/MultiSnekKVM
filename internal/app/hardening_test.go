package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func TestDiscardReceivedFilesOnlyRemovesRegisteredDirs(t *testing.T) {
	root := t.TempDir()
	registered := filepath.Join(root, "recv-1")
	other := filepath.Join(root, "precious")
	for _, d := range []string{registered, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{}
	a.addRecvDir(registered)

	a.DiscardReceivedFiles(other)
	a.DiscardReceivedFiles(registered + string(filepath.Separator) + "..")
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unregistered directory must not be removed: %v", err)
	}

	// Equivalent spelling of the registered path is accepted after Clean.
	a.DiscardReceivedFiles(filepath.Join(root, ".", "recv-1"))
	if _, err := os.Stat(registered); !os.IsNotExist(err) {
		t.Fatalf("registered directory should be removed, stat err=%v", err)
	}
	if _, ok := a.lookupRecvDir(registered); ok {
		t.Fatal("directory must be unregistered after discard")
	}
	// A second discard of the same path is now rejected.
	if err := os.MkdirAll(registered, 0o700); err != nil {
		t.Fatal(err)
	}
	a.DiscardReceivedFiles(registered)
	if _, err := os.Stat(registered); err != nil {
		t.Fatalf("no-longer-registered directory must not be removed: %v", err)
	}
}

func TestSaveReceivedFilesRejectsUnknownDir(t *testing.T) {
	a := &App{}
	if _, err := a.SaveReceivedFiles(t.TempDir()); err == nil {
		t.Fatal("expected unknown directory to be rejected")
	}
}

func TestMoveFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(dst); err != nil || string(b) != "hello" {
		t.Fatalf("dst = %q, %v", b, err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source should be gone after move")
	}
}
