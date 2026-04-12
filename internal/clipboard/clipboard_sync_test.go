package clipboard

import (
	"strings"
	"testing"
)

func TestClipboardSyncStateDropsOversizedOutboundClipboard(t *testing.T) {
	state := State{}

	msg, shouldSend, droppedTooLarge := state.PrepareOutboundClipboard(strings.Repeat("a", MaxClipboardTextBytes+1))
	if shouldSend {
		t.Fatalf("expected oversized clipboard text to be skipped, got send with seq=%d", msg.Seq)
	}
	if !droppedTooLarge {
		t.Fatal("expected oversized clipboard text to be reported as too large")
	}

	msg, shouldSend, droppedTooLarge = state.PrepareOutboundClipboard("small text")
	if droppedTooLarge {
		t.Fatal("did not expect small clipboard text to be marked too large")
	}
	if !shouldSend {
		t.Fatal("expected small clipboard text to send after oversized text was skipped")
	}
	if msg.Seq != 1 || msg.Text != "small text" {
		t.Fatalf("unexpected outbound clipboard message: %+v", msg)
	}
}

func TestClipboardSyncStateSuppressesImmediateRemoteEchoOnce(t *testing.T) {
	state := State{}

	state.RecordRemoteClipboard("foo")
	msg, shouldSend, droppedTooLarge := state.PrepareOutboundClipboard("foo")
	if droppedTooLarge {
		t.Fatal("did not expect remote echo to be marked too large")
	}
	if shouldSend {
		t.Fatalf("expected immediate remote echo to be suppressed, got send with seq=%d", msg.Seq)
	}

	msg, shouldSend, droppedTooLarge = state.PrepareOutboundClipboard("foo")
	if droppedTooLarge {
		t.Fatal("did not expect unchanged clipboard text to be marked too large")
	}
	if shouldSend {
		t.Fatalf("expected unchanged clipboard text to stay suppressed, got send with seq=%d", msg.Seq)
	}
}

func TestClipboardSyncStateResendsSameTextAfterIntermediateLocalChange(t *testing.T) {
	state := State{}

	state.RecordRemoteClipboard("foo")
	if _, shouldSend, _ := state.PrepareOutboundClipboard("foo"); shouldSend {
		t.Fatal("expected first reflected remote clipboard text to be suppressed")
	}

	msg, shouldSend, droppedTooLarge := state.PrepareOutboundClipboard("bar")
	if droppedTooLarge || !shouldSend {
		t.Fatal("expected intermediate local clipboard text to send")
	}
	if msg.Seq != 1 || msg.Text != "bar" {
		t.Fatalf("unexpected intermediate outbound clipboard message: %+v", msg)
	}

	msg, shouldSend, droppedTooLarge = state.PrepareOutboundClipboard("foo")
	if droppedTooLarge {
		t.Fatal("did not expect later local clipboard text to be marked too large")
	}
	if !shouldSend {
		t.Fatal("expected later local clipboard text to send after suppression was consumed")
	}
	if msg.Seq != 2 || msg.Text != "foo" {
		t.Fatalf("unexpected later outbound clipboard message: %+v", msg)
	}
}

func TestClipboardSyncStateResetClearsSuppressionAndSequence(t *testing.T) {
	state := State{}

	state.RecordRemoteClipboard("foo")
	if _, shouldSend, _ := state.PrepareOutboundClipboard("foo"); shouldSend {
		t.Fatal("expected reflected remote clipboard text to be suppressed before reset")
	}

	state.Reset()

	msg, shouldSend, droppedTooLarge := state.PrepareOutboundClipboard("foo")
	if droppedTooLarge {
		t.Fatal("did not expect reset clipboard state to mark text too large")
	}
	if !shouldSend {
		t.Fatal("expected clipboard text to send after session state reset")
	}
	if msg.Seq != 1 || msg.Text != "foo" {
		t.Fatalf("unexpected outbound clipboard message after reset: %+v", msg)
	}
}
