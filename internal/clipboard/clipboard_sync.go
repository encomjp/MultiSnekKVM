package clipboard

import "multisnekkvm/internal/protocol"

const (
	clipboardSyncHeaderSize = 4
	MaxClipboardTextBytes   = protocol.MaxFramePayloadBytes - clipboardSyncHeaderSize
	MaxClipboardUTF16Bytes  = uintptr(MaxClipboardTextBytes+1) * 2
)

type State struct {
	lastLocalText      string
	pendingRemoteText  string
	suppressRemoteOnce bool
	lastSeq            uint32
}

func (s *State) Reset() {
	s.lastLocalText = ""
	s.pendingRemoteText = ""
	s.suppressRemoteOnce = false
	s.lastSeq = 0
}

func (s *State) RecordRemoteClipboard(text string) {
	if text == s.lastLocalText {
		s.pendingRemoteText = ""
		s.suppressRemoteOnce = false
		return
	}
	s.pendingRemoteText = text
	s.suppressRemoteOnce = true
}

func (s *State) PrepareOutboundClipboard(text string) (protocol.ClipboardSyncMsg, bool, bool) {
	if text == "" || text == s.lastLocalText {
		return protocol.ClipboardSyncMsg{}, false, false
	}

	s.lastLocalText = text

	if s.suppressRemoteOnce {
		pendingRemoteText := s.pendingRemoteText
		s.pendingRemoteText = ""
		s.suppressRemoteOnce = false
		if text == pendingRemoteText {
			return protocol.ClipboardSyncMsg{}, false, false
		}
	}

	if len(text) > MaxClipboardTextBytes {
		return protocol.ClipboardSyncMsg{}, false, true
	}

	s.lastSeq++
	return protocol.ClipboardSyncMsg{Seq: s.lastSeq, Text: text}, true, false
}
