package inputstate

import (
	"sort"

	"multisnekkvm/internal/protocol"
)

const RemoteKeyFlagExtended uint32 = 0x01

type KeyState struct {
	pressed map[protocol.KeyMsg]struct{}
}

func (s *KeyState) Record(msg protocol.KeyMsg, down bool) {
	key := normalizeRemoteKey(msg)
	if !down {
		if s.pressed == nil {
			return
		}
		delete(s.pressed, key)
		return
	}

	if s.pressed == nil {
		s.pressed = make(map[protocol.KeyMsg]struct{})
	}
	s.pressed[key] = struct{}{}
}

func (s *KeyState) ReleaseAll() []protocol.KeyMsg {
	if len(s.pressed) == 0 {
		return nil
	}

	keys := make([]protocol.KeyMsg, 0, len(s.pressed))
	for key := range s.pressed {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].VKCode != keys[j].VKCode {
			return keys[i].VKCode < keys[j].VKCode
		}
		if keys[i].ScanCode != keys[j].ScanCode {
			return keys[i].ScanCode < keys[j].ScanCode
		}
		return keys[i].Flags < keys[j].Flags
	})

	clear(s.pressed)
	return keys
}

func (s *KeyState) HasPressed() bool {
	return len(s.pressed) > 0
}

func normalizeRemoteKey(msg protocol.KeyMsg) protocol.KeyMsg {
	msg.Flags &= RemoteKeyFlagExtended
	return msg
}
