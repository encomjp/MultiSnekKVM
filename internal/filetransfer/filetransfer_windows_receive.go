//go:build windows

package filetransfer

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/protocol"
)

func (ar *activeRecv) closeFiles() []string {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	filePaths := make([]string, 0, len(ar.files))
	for i, fileHandle := range ar.files {
		if fileHandle == nil {
			continue
		}
		filePaths = append(filePaths, fileHandle.Name())
		fileHandle.Close()
		ar.files[i] = nil
	}
	return filePaths
}

func (ar *activeRecv) validateComplete() error {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	if len(ar.received) != len(ar.offer.Files) {
		return fmt.Errorf("received file count mismatch: got %d want %d", len(ar.received), len(ar.offer.Files))
	}
	var total uint64
	for i, fileInfo := range ar.offer.Files {
		if ar.received[i] != fileInfo.Size {
			return fmt.Errorf("file %q incomplete: got %d want %d", fileInfo.Name, ar.received[i], fileInfo.Size)
		}
		total += ar.received[i]
	}
	if total != ar.offer.Total {
		return fmt.Errorf("transfer total incomplete: got %d want %d", total, ar.offer.Total)
	}
	return nil
}

func normalizeInboundOfferNames(files []FileTransferFileInfo) ([]string, error) {
	used := make(map[string]struct{}, len(files))
	names := make([]string, len(files))
	for i, fileInfo := range files {
		name := sanitizeTransferName(fileInfo.Name)
		if name == "" {
			return nil, fmt.Errorf("invalid file name at index %d", i)
		}
		if !reserveTransferName(name, used) {
			return nil, fmt.Errorf("duplicate file name %q", name)
		}
		names[i] = name
	}
	return names, nil
}

func (ft *FileTransferManager) HandleInbound(frame protocol.Frame) {
	switch frame.Type {
	case protocol.MsgFileTransferOffer:
		ft.handleOffer(frame.Payload)
	case protocol.MsgFileTransferAccept:
		if len(frame.Payload) >= 4 {
			ft.handleAccept(binary.BigEndian.Uint32(frame.Payload))
		}
	case protocol.MsgFileChunk:
		ft.handleChunk(frame.Payload)
	case protocol.MsgFileTransferDone:
		if len(frame.Payload) >= 4 {
			ft.handleDone(binary.BigEndian.Uint32(frame.Payload))
		}
	case protocol.MsgFileTransferCancel:
		if len(frame.Payload) >= 4 {
			ft.handleCancel(binary.BigEndian.Uint32(frame.Payload))
		}
	}
}

func (ft *FileTransferManager) handleAccept(id uint32) {
	ft.mu.Lock()
	send, ok := ft.outbound[id]
	ft.mu.Unlock()
	if ok && send != nil {
		send.markAccepted()
	}
	log.Printf("ft: peer accepted transfer id=%d", id)
}

func (ft *FileTransferManager) handleOffer(payload []byte) {
	var offer FileTransferOffer
	if err := json.Unmarshal(payload, &offer); err != nil {
		log.Printf("ft: bad offer: %v", err)
		return
	}
	names, err := normalizeInboundOfferNames(offer.Files)
	if err != nil {
		log.Printf("ft: rejected offer id=%d: %v", offer.ID, err)
		ft.sendCancel(offer.ID, 1)
		return
	}
	for i, name := range names {
		offer.Files[i].Name = name
	}
	log.Printf("ft: received offer id=%d files=%d total=%d B", offer.ID, len(offer.Files), offer.Total)

	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("multisnek_recv_%d", offer.ID))
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		log.Printf("ft: mkdir %s: %v", tempDir, err)
		ft.sendCancel(offer.ID, 1)
		return
	}

	fileHandles := make([]*os.File, len(offer.Files))
	for i, fileInfo := range offer.Files {
		dest := filepath.Join(tempDir, fileInfo.Name)
		fh, err := os.Create(dest)
		if err != nil {
			log.Printf("ft: create %s: %v", dest, err)
			for j, openFile := range fileHandles {
				if openFile != nil {
					openFile.Close()
					fileHandles[j] = nil
				}
			}
			os.RemoveAll(tempDir)
			ft.sendCancel(offer.ID, 1)
			return
		}
		fileHandles[i] = fh
	}

	recv := &activeRecv{
		offer:    offer,
		tempDir:  tempDir,
		files:    fileHandles,
		received: make([]uint64, len(offer.Files)),
		done:     make(chan struct{}),
	}

	ft.mu.Lock()
	ft.active[offer.ID] = recv
	ft.mu.Unlock()

	pay := make([]byte, 4)
	binary.BigEndian.PutUint32(pay, offer.ID)
	ft.send(protocol.Frame{Type: protocol.MsgFileTransferAccept, Payload: pay})

	logutil.SafeGo("ft-progress", func() { ft.runProgressDialog(recv) })
}

func (ft *FileTransferManager) handleChunk(payload []byte) {
	if len(payload) < 16 {
		return
	}
	id := binary.BigEndian.Uint32(payload[0:4])
	fileIdx := int(binary.BigEndian.Uint32(payload[4:8]))
	offset := binary.BigEndian.Uint64(payload[8:16])
	data := payload[16:]

	ft.mu.Lock()
	recv, ok := ft.active[id]
	ft.mu.Unlock()
	if !ok {
		return
	}

	select {
	case <-recv.done:
		return
	default:
	}

	recv.mu.Lock()
	if fileIdx < 0 || fileIdx >= len(recv.files) || recv.files[fileIdx] == nil {
		recv.mu.Unlock()
		return
	}
	if _, err := recv.files[fileIdx].WriteAt(data, int64(offset)); err != nil {
		recv.mu.Unlock()
		log.Printf("ft: write error file[%d] offset=%d: %v", fileIdx, offset, err)
		ft.abortReceive(id, recv, 1)
		return
	}
	recv.received[fileIdx] += uint64(len(data))
	recv.total += uint64(len(data))
	recv.mu.Unlock()
}

func (ft *FileTransferManager) handleDone(id uint32) {
	ft.mu.Lock()
	recv, ok := ft.active[id]
	if ok {
		delete(ft.active, id)
	}
	ft.mu.Unlock()
	if !ok {
		return
	}
	if err := recv.validateComplete(); err != nil {
		recv.closeFiles()
		os.RemoveAll(recv.tempDir)
		recv.finish()
		ft.sendCancel(id, 1)
		log.Printf("ft: receive rejected id=%d: %v", id, err)
		return
	}

	filePaths := recv.closeFiles()
	recv.finish()
	log.Printf("ft: receive complete id=%d -> %s (%d files)", id, recv.tempDir, len(filePaths))

	if len(filePaths) > 0 {
		setClipboardFiles(filePaths)
		log.Printf("ft: %d files placed on clipboard - user can Ctrl+V", len(filePaths))
	}

	// Notify app layer so it can show a UI prompt for saving.
	ft.mu.Lock()
	cb := ft.onComplete
	ft.mu.Unlock()
	if cb != nil {
		names := make([]string, len(recv.offer.Files))
		for i, f := range recv.offer.Files {
			names[i] = f.Name
		}
		cb(recv.tempDir, names)
	}
}

func (ft *FileTransferManager) handleCancel(id uint32) {
	ft.mu.Lock()
	recv, ok := ft.active[id]
	if ok {
		delete(ft.active, id)
	}
	send, sending := ft.outbound[id]
	if sending {
		delete(ft.outbound, id)
	}
	ft.mu.Unlock()
	if sending {
		send.markCanceled()
		log.Printf("ft: outbound transfer cancelled by peer id=%d", id)
	}
	if !ok {
		return
	}
	recv.closeFiles()
	os.RemoveAll(recv.tempDir)
	recv.finish()
	log.Printf("ft: transfer cancelled by peer id=%d", id)
}

func (ft *FileTransferManager) abortReceive(id uint32, recv *activeRecv, reason byte) {
	ft.mu.Lock()
	current, ok := ft.active[id]
	if ok && current == recv {
		delete(ft.active, id)
	}
	ft.mu.Unlock()
	if !ok || current != recv {
		return
	}
	recv.closeFiles()
	os.RemoveAll(recv.tempDir)
	recv.finish()
	ft.sendCancel(id, reason)
}

func (ft *FileTransferManager) sendCancel(id uint32, reason byte) {
	pay := make([]byte, 5)
	binary.BigEndian.PutUint32(pay, id)
	pay[4] = reason
	ft.send(protocol.Frame{Type: protocol.MsgFileTransferCancel, Payload: pay})
}

func (ft *FileTransferManager) CancelAll() {
	ft.mu.Lock()
	ids := make([]uint32, 0, len(ft.active))
	cancelIDs := make(map[uint32]struct{}, len(ft.active)+len(ft.outbound))
	for id := range ft.active {
		ids = append(ids, id)
		cancelIDs[id] = struct{}{}
	}
	outbound := make([]*activeSend, 0, len(ft.outbound))
	for id, send := range ft.outbound {
		delete(ft.outbound, id)
		outbound = append(outbound, send)
		cancelIDs[id] = struct{}{}
	}
	ft.mu.Unlock()
	for id := range cancelIDs {
		ft.sendCancel(id, 0)
	}
	for _, send := range outbound {
		send.markCanceled()
	}
	for _, id := range ids {
		ft.handleCancel(id)
	}
}

func (ft *FileTransferManager) runProgressDialog(recv *activeRecv) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := pCoInitializeEx.Call(0, coinitApartment)
	needUninit := hr == 0
	if needUninit {
		defer pCoUninitialize.Call()
	}

	var ppv uintptr
	hr, _, _ = pCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidProgressDialog)),
		0,
		clsctxInprocSrv,
		uintptr(unsafe.Pointer(&iidIOperationsProgressDialog)),
		uintptr(unsafe.Pointer(&ppv)),
	)
	if hr != 0 || ppv == 0 {
		log.Printf("ft: IOperationsProgressDialog create failed hr=0x%08X - no progress UI", uint32(hr))
		<-recv.done
		return
	}
	defer comRelease(ppv)

	comCall(ppv, vtblStartProgressDialog, 0, 0)
	comCall(ppv, vtblSetOperation, spactionCopying)
	comCall(ppv, vtblSetMode, 0)

	totalFiles := uint64(len(recv.offer.Files))
	totalBytes := recv.offer.Total

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var msg [48]byte

	for {
		select {
		case <-recv.done:
			comCall(ppv, vtblStopProgressDialog)
			return
		case <-ticker.C:
		}

		for {
			r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, pmRemove)
			if r == 0 {
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg[0])))
		}

		recv.mu.Lock()
		bytesRecv := recv.total
		filesComplete := uint64(0)
		for i, received := range recv.received {
			if received >= recv.offer.Files[i].Size {
				filesComplete++
			}
		}
		recv.mu.Unlock()

		comCall(ppv, vtblUpdateProgress,
			uintptr(bytesRecv),
			uintptr(totalBytes),
			uintptr(bytesRecv),
			uintptr(totalBytes),
			uintptr(filesComplete),
			uintptr(totalFiles),
		)

		var status uint32
		comCall(ppv, vtblGetOperationStatus, uintptr(unsafe.Pointer(&status)))
		if status == pdopsCancelled {
			log.Printf("ft: user cancelled receive id=%d", recv.offer.ID)
			ft.sendCancel(recv.offer.ID, 0)
			ft.mu.Lock()
			delete(ft.active, recv.offer.ID)
			ft.mu.Unlock()
			recv.closeFiles()
			os.RemoveAll(recv.tempDir)
			recv.finish()
			comCall(ppv, vtblStopProgressDialog)
			return
		}
	}
}
