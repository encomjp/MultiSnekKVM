//go:build windows

package filetransfer

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"multisnekkvm/internal/protocol"
)

func TestStartSendMakesDuplicateBasenamesUnique(t *testing.T) {
	root := t.TempDir()
	leftDir := filepath.Join(root, "left")
	rightDir := filepath.Join(root, "right")
	if err := os.MkdirAll(leftDir, 0o755); err != nil {
		t.Fatalf("mkdir left: %v", err)
	}
	if err := os.MkdirAll(rightDir, 0o755); err != nil {
		t.Fatalf("mkdir right: %v", err)
	}

	leftPath := filepath.Join(leftDir, "report.txt")
	rightPath := filepath.Join(rightDir, "report.txt")
	if err := os.WriteFile(leftPath, []byte("left"), 0o600); err != nil {
		t.Fatalf("write left: %v", err)
	}
	if err := os.WriteFile(rightPath, []byte("right"), 0o600); err != nil {
		t.Fatalf("write right: %v", err)
	}

	frames := make(chan protocol.Frame, 8)
	ft := NewFileTransferManager()
	ft.SetSendFn(func(frame protocol.Frame) {
		frames <- frame
	})

	ft.StartSend([]string{leftPath, rightPath})

	offerFrame := waitForFrameType(t, frames, protocol.MsgFileTransferOffer)
	var offer FileTransferOffer
	if err := json.Unmarshal(offerFrame.Payload, &offer); err != nil {
		t.Fatalf("unmarshal offer: %v", err)
	}
	if len(offer.Files) != 2 {
		t.Fatalf("offer file count = %d, want 2", len(offer.Files))
	}
	if offer.Files[0].Name == offer.Files[1].Name {
		t.Fatalf("offer kept colliding names %q", offer.Files[0].Name)
	}

	ft.handleAccept(offer.ID)
	_ = waitForFrameType(t, frames, protocol.MsgFileTransferDone)
	ft.CancelAll()
}

func TestHandleOfferRejectsCollidingNames(t *testing.T) {
	ft := NewFileTransferManager()
	var (
		mu     sync.Mutex
		frames []protocol.Frame
	)
	ft.SetSendFn(func(frame protocol.Frame) {
		mu.Lock()
		frames = append(frames, frame)
		mu.Unlock()
	})

	offer := FileTransferOffer{
		ID: 77,
		Files: []FileTransferFileInfo{
			{Name: "report.txt", Size: 3},
			{Name: "report.txt", Size: 5},
		},
		Total: 8,
	}
	payload, err := json.Marshal(offer)
	if err != nil {
		t.Fatalf("marshal offer: %v", err)
	}

	t.Cleanup(func() {
		ft.mu.Lock()
		recv := ft.active[offer.ID]
		ft.mu.Unlock()
		if recv != nil {
			ft.handleCancel(offer.ID)
		}
	})

	ft.handleOffer(payload)

	mu.Lock()
	defer mu.Unlock()
	if len(frames) == 0 {
		t.Fatal("expected a response frame")
	}
	if frames[0].Type != protocol.MsgFileTransferCancel {
		t.Fatalf("first response type = 0x%X, want cancel 0x%X", frames[0].Type, protocol.MsgFileTransferCancel)
	}

	ft.mu.Lock()
	_, active := ft.active[offer.ID]
	ft.mu.Unlock()
	if active {
		t.Fatal("colliding offer should not remain active")
	}
}

func TestHandleDoneRejectsIncompleteTransfer(t *testing.T) {
	ft := NewFileTransferManager()
	var (
		mu     sync.Mutex
		frames []protocol.Frame
	)
	ft.SetSendFn(func(frame protocol.Frame) {
		mu.Lock()
		frames = append(frames, frame)
		mu.Unlock()
	})

	offer := FileTransferOffer{
		ID: 91,
		Files: []FileTransferFileInfo{
			{Name: "note.txt", Size: 5},
		},
		Total: 5,
	}
	payload, err := json.Marshal(offer)
	if err != nil {
		t.Fatalf("marshal offer: %v", err)
	}
	tempDir := filepath.Join(os.TempDir(), "multisnek_recv_91")
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	ft.handleOffer(payload)

	chunk := make([]byte, 16+3)
	binary.BigEndian.PutUint32(chunk[0:4], offer.ID)
	binary.BigEndian.PutUint32(chunk[4:8], 0)
	binary.BigEndian.PutUint64(chunk[8:16], 0)
	copy(chunk[16:], []byte("hey"))
	ft.handleChunk(chunk)
	ft.handleDone(offer.ID)

	mu.Lock()
	defer mu.Unlock()
	if len(frames) < 2 {
		t.Fatalf("expected accept and cancel frames, got %d", len(frames))
	}
	if frames[0].Type != protocol.MsgFileTransferAccept {
		t.Fatalf("first response type = 0x%X, want accept 0x%X", frames[0].Type, protocol.MsgFileTransferAccept)
	}
	if frames[len(frames)-1].Type != protocol.MsgFileTransferCancel {
		t.Fatalf("final response type = 0x%X, want cancel 0x%X", frames[len(frames)-1].Type, protocol.MsgFileTransferCancel)
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("incomplete transfer should clean up %s, stat err=%v", tempDir, err)
	}
	ft.mu.Lock()
	_, active := ft.active[offer.ID]
	ft.mu.Unlock()
	if active {
		t.Fatal("incomplete transfer should not remain active")
	}
}

func TestHandleChunkCancelsReceiveOnWriteError(t *testing.T) {
	ft := NewFileTransferManager()
	var (
		mu     sync.Mutex
		frames []protocol.Frame
	)
	ft.SetSendFn(func(frame protocol.Frame) {
		mu.Lock()
		frames = append(frames, frame)
		mu.Unlock()
	})

	offer := FileTransferOffer{
		ID:    92,
		Files: []FileTransferFileInfo{{Name: "note.txt", Size: 5}},
		Total: 5,
	}
	payload, err := json.Marshal(offer)
	if err != nil {
		t.Fatalf("marshal offer: %v", err)
	}
	tempDir := filepath.Join(os.TempDir(), "multisnek_recv_92")
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	ft.handleOffer(payload)

	ft.mu.Lock()
	recv := ft.active[offer.ID]
	ft.mu.Unlock()
	if recv == nil {
		t.Fatal("expected active receive after offer")
	}

	recv.mu.Lock()
	filePath := recv.files[0].Name()
	if err := recv.files[0].Close(); err != nil {
		recv.mu.Unlock()
		t.Fatalf("close file: %v", err)
	}
	readOnly, err := os.Open(filePath)
	if err != nil {
		recv.mu.Unlock()
		t.Fatalf("open read-only file: %v", err)
	}
	recv.files[0] = readOnly
	recv.mu.Unlock()

	chunk := make([]byte, 16+5)
	binary.BigEndian.PutUint32(chunk[0:4], offer.ID)
	binary.BigEndian.PutUint32(chunk[4:8], 0)
	binary.BigEndian.PutUint64(chunk[8:16], 0)
	copy(chunk[16:], []byte("hello"))
	ft.handleChunk(chunk)

	mu.Lock()
	defer mu.Unlock()
	if len(frames) < 2 {
		t.Fatalf("expected accept and cancel frames, got %d", len(frames))
	}
	if frames[0].Type != protocol.MsgFileTransferAccept {
		t.Fatalf("first response type = 0x%X, want accept 0x%X", frames[0].Type, protocol.MsgFileTransferAccept)
	}
	if frames[len(frames)-1].Type != protocol.MsgFileTransferCancel {
		t.Fatalf("final response type = 0x%X, want cancel 0x%X", frames[len(frames)-1].Type, protocol.MsgFileTransferCancel)
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("write error should clean up %s, stat err=%v", tempDir, err)
	}
	ft.mu.Lock()
	_, active := ft.active[offer.ID]
	ft.mu.Unlock()
	if active {
		t.Fatal("write error should remove active receive")
	}
}

func TestCancelAllSendsCancelForPendingOutboundOffers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	frames := make(chan protocol.Frame, 4)
	ft := NewFileTransferManager()
	ft.SetSendFn(func(frame protocol.Frame) {
		frames <- frame
	})

	ft.StartSend([]string{path})
	offerFrame := waitForFrameType(t, frames, protocol.MsgFileTransferOffer)
	var offer FileTransferOffer
	if err := json.Unmarshal(offerFrame.Payload, &offer); err != nil {
		t.Fatalf("unmarshal offer: %v", err)
	}

	ft.CancelAll()

	ft.mu.Lock()
	_, pending := ft.outbound[offer.ID]
	ft.mu.Unlock()
	if pending {
		t.Fatal("CancelAll should clear pending outbound offers")
	}
	cancelFrame := waitForFrameType(t, frames, protocol.MsgFileTransferCancel)
	if len(cancelFrame.Payload) != 5 {
		t.Fatalf("cancel payload len = %d, want 5", len(cancelFrame.Payload))
	}
	if gotID := binary.BigEndian.Uint32(cancelFrame.Payload[:4]); gotID != offer.ID {
		t.Fatalf("cancel id = %d, want %d", gotID, offer.ID)
	}
	if cancelFrame.Payload[4] != 0 {
		t.Fatalf("cancel reason = %d, want 0", cancelFrame.Payload[4])
	}

	select {
	case frame := <-frames:
		if frame.Type == protocol.MsgFileTransferDone {
			t.Fatal("sender should not emit done after CancelAll")
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func TestCancelAllSendsCancelForActiveReceive(t *testing.T) {
	frames := make(chan protocol.Frame, 8)
	ft := NewFileTransferManager()
	ft.SetSendFn(func(frame protocol.Frame) {
		frames <- frame
	})

	offer := FileTransferOffer{
		ID:    105,
		Files: []FileTransferFileInfo{{Name: "note.txt", Size: 5}},
		Total: 5,
	}
	payload, err := json.Marshal(offer)
	if err != nil {
		t.Fatalf("marshal offer: %v", err)
	}
	tempDir := filepath.Join(os.TempDir(), "multisnek_recv_105")
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	ft.handleOffer(payload)
	acceptFrame := waitForFrameType(t, frames, protocol.MsgFileTransferAccept)
	if gotID := binary.BigEndian.Uint32(acceptFrame.Payload[:4]); gotID != offer.ID {
		t.Fatalf("accept id = %d, want %d", gotID, offer.ID)
	}

	ft.CancelAll()

	cancelFrame := waitForFrameType(t, frames, protocol.MsgFileTransferCancel)
	if gotID := binary.BigEndian.Uint32(cancelFrame.Payload[:4]); gotID != offer.ID {
		t.Fatalf("cancel id = %d, want %d", gotID, offer.ID)
	}
	if cancelFrame.Payload[4] != 0 {
		t.Fatalf("cancel reason = %d, want 0", cancelFrame.Payload[4])
	}
	ft.mu.Lock()
	_, active := ft.active[offer.ID]
	ft.mu.Unlock()
	if active {
		t.Fatal("CancelAll should clear active inbound transfers")
	}
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("CancelAll should remove %s, stat err=%v", tempDir, err)
	}
}

func TestStreamFilesCancelsTransferWhenFileOpenFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	frames := make(chan protocol.Frame, 8)
	ft := NewFileTransferManager()
	ft.SetSendFn(func(frame protocol.Frame) {
		frames <- frame
	})

	ft.StartSend([]string{path})
	offerFrame := waitForFrameType(t, frames, protocol.MsgFileTransferOffer)
	var offer FileTransferOffer
	if err := json.Unmarshal(offerFrame.Payload, &offer); err != nil {
		t.Fatalf("unmarshal offer: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	ft.handleAccept(offer.ID)
	cancelFrame := waitForFrameType(t, frames, protocol.MsgFileTransferCancel)
	if len(cancelFrame.Payload) != 5 {
		t.Fatalf("cancel payload len = %d, want 5", len(cancelFrame.Payload))
	}
	if gotID := binary.BigEndian.Uint32(cancelFrame.Payload[:4]); gotID != offer.ID {
		t.Fatalf("cancel id = %d, want %d", gotID, offer.ID)
	}
	if cancelFrame.Payload[4] != 1 {
		t.Fatalf("cancel reason = %d, want 1", cancelFrame.Payload[4])
	}

	select {
	case frame := <-frames:
		if frame.Type == protocol.MsgFileTransferDone {
			t.Fatal("sender should not emit done after a local file open failure")
		}
	case <-time.After(200 * time.Millisecond):
	}

	ft.mu.Lock()
	_, pending := ft.outbound[offer.ID]
	ft.mu.Unlock()
	if pending {
		t.Fatal("accepted transfer should be removed from pending map")
	}
}

func TestStartSendSkipsOfferWhenRandomReadFails(t *testing.T) {
	originalRandomRead := randomRead
	randomRead = func(b []byte) (int, error) {
		_ = b
		return 0, errors.New("entropy unavailable")
	}
	defer func() {
		randomRead = originalRandomRead
	}()

	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	frames := make(chan protocol.Frame, 2)
	ft := NewFileTransferManager()
	ft.SetSendFn(func(frame protocol.Frame) {
		frames <- frame
	})

	ft.StartSend([]string{path})

	select {
	case frame := <-frames:
		t.Fatalf("did not expect outbound frame when random read fails: %#v", frame)
	case <-time.After(150 * time.Millisecond):
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()
	if len(ft.outbound) != 0 {
		t.Fatalf("outbound transfers = %d, want 0", len(ft.outbound))
	}
}

func TestStartSendSkipsOfferWhenMarshalFails(t *testing.T) {
	originalJSONMarshal := jsonMarshal
	jsonMarshal = func(v any) ([]byte, error) {
		_ = v
		return nil, errors.New("marshal failed")
	}
	defer func() {
		jsonMarshal = originalJSONMarshal
	}()

	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	frames := make(chan protocol.Frame, 2)
	ft := NewFileTransferManager()
	ft.SetSendFn(func(frame protocol.Frame) {
		frames <- frame
	})

	ft.StartSend([]string{path})

	select {
	case frame := <-frames:
		t.Fatalf("did not expect outbound frame when marshal fails: %#v", frame)
	case <-time.After(150 * time.Millisecond):
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()
	if len(ft.outbound) != 0 {
		t.Fatalf("outbound transfers = %d, want 0", len(ft.outbound))
	}
}

func TestHandleCancelClearsRecvFiles(t *testing.T) {
	ft := NewFileTransferManager()
	offer := FileTransferOffer{
		ID:    143,
		Files: []FileTransferFileInfo{{Name: "note.txt", Size: 5}},
		Total: 5,
	}
	payload, err := json.Marshal(offer)
	if err != nil {
		t.Fatalf("marshal offer: %v", err)
	}
	tempDir := filepath.Join(os.TempDir(), "multisnek_recv_143")
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	ft.handleOffer(payload)
	ft.mu.Lock()
	recv := ft.active[offer.ID]
	ft.mu.Unlock()
	if recv == nil {
		t.Fatal("expected active receive after offer")
	}

	ft.handleCancel(offer.ID)

	recv.mu.Lock()
	defer recv.mu.Unlock()
	if recv.files[0] != nil {
		t.Fatal("expected cancelled receive file handle to be cleared")
	}
}

func waitForFrameType(t *testing.T, frames <-chan protocol.Frame, want byte) protocol.Frame {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case frame := <-frames:
			if frame.Type == want {
				return frame
			}
		case <-timeout:
			t.Fatalf("timed out waiting for frame type 0x%X", want)
		}
	}
}
