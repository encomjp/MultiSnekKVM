//go:build windows

package filetransfer

import (
    "encoding/binary"
    "encoding/json"
    "os"
    "testing"

    "multisnekkvm/internal/protocol"
)

func TestHandleChunkRejectsOutOfBoundsOffset(t *testing.T) {
    ft := NewFileTransferManager()
    ft.SetSendFn(func(protocol.Frame) {})
    offer := FileTransferOffer{ID: 0x7821, Files: []FileTransferFileInfo{{Name: "test.bin", Size: 3}}, Total: 3}
    payload, err := json.Marshal(offer)
    if err != nil { t.Fatal(err) }
    ft.handleOffer(payload)

    ft.mu.Lock()
    recv := ft.active[offer.ID]
    ft.mu.Unlock()
    if recv == nil { t.Fatal("offer not accepted") }
    t.Cleanup(func() { _ = os.RemoveAll(recv.tempDir) })

    chunk := make([]byte, 16+3)
    binary.BigEndian.PutUint32(chunk[0:4], offer.ID)
    binary.BigEndian.PutUint32(chunk[4:8], 0)
    binary.BigEndian.PutUint64(chunk[8:16], 1)
    copy(chunk[16:], "abc")
    ft.handleChunk(chunk)

    ft.mu.Lock()
    _, active := ft.active[offer.ID]
    ft.mu.Unlock()
    if active { t.Fatal("out-of-bounds chunk must abort transfer") }
    if _, err := os.Stat(recv.tempDir); !os.IsNotExist(err) {
        t.Fatalf("aborted transfer temp dir remains: %v", err)
    }
}
