package inputstate

import (
	"testing"

	"multisnekkvm/internal/protocol"
)

func TestRemoteKeyStateReleaseAllReturnsPressedKeysOnce(t *testing.T) {
	state := KeyState{}
	state.Record(protocol.KeyMsg{VKCode: 0x11, ScanCode: 0x1d}, true)
	state.Record(protocol.KeyMsg{VKCode: 0xa2, ScanCode: 0x1d, Flags: RemoteKeyFlagExtended}, true)
	state.Record(protocol.KeyMsg{VKCode: 0x11, ScanCode: 0x1d}, true)

	released := state.ReleaseAll()
	if len(released) != 2 {
		t.Fatalf("expected 2 pressed keys to release, got %d", len(released))
	}
	if released[0] != (protocol.KeyMsg{VKCode: 0x11, ScanCode: 0x1d}) {
		t.Fatalf("unexpected first released key: %+v", released[0])
	}
	if released[1] != (protocol.KeyMsg{VKCode: 0xa2, ScanCode: 0x1d, Flags: RemoteKeyFlagExtended}) {
		t.Fatalf("unexpected second released key: %+v", released[1])
	}

	if releasedAgain := state.ReleaseAll(); len(releasedAgain) != 0 {
		t.Fatalf("expected key state to clear after release, got %d keys", len(releasedAgain))
	}
}

func TestRemoteKeyStateMatchesKeyUpsByNormalizedFlags(t *testing.T) {
	state := KeyState{}
	state.Record(protocol.KeyMsg{VKCode: 0xa3, ScanCode: 0x1d, Flags: RemoteKeyFlagExtended}, true)
	state.Record(protocol.KeyMsg{VKCode: 0xa3, ScanCode: 0x1d, Flags: RemoteKeyFlagExtended | 0x80}, false)

	if released := state.ReleaseAll(); len(released) != 0 {
		t.Fatalf("expected matching key up to clear pressed state, got %+v", released)
	}
}
