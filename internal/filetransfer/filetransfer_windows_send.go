//go:build windows

package filetransfer

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/protocol"
)

func sanitizeTransferName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "/", string(filepath.Separator))
	trimmed = filepath.Base(trimmed)
	if trimmed == "" || trimmed == "." || trimmed == ".." {
		return ""
	}
	return trimmed
}

func reserveTransferName(name string, used map[string]struct{}) bool {
	key := strings.ToLower(name)
	if _, exists := used[key]; exists {
		return false
	}
	used[key] = struct{}{}
	return true
}

func uniqueTransferName(name string, used map[string]struct{}) string {
	base := sanitizeTransferName(name)
	if base == "" {
		base = "file"
	}
	if reserveTransferName(base, used) {
		return base
	}

	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for attempt := 2; ; attempt++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, attempt, ext)
		if reserveTransferName(candidate, used) {
			return candidate
		}
	}
}

func (ft *FileTransferManager) clearOutbound(id uint32, send *activeSend) {
	ft.mu.Lock()
	if current, ok := ft.outbound[id]; ok && current == send {
		delete(ft.outbound, id)
	}
	ft.mu.Unlock()
}

func (ft *FileTransferManager) StartSend(paths []string) {
	var infos []FileTransferFileInfo
	var validPaths []string
	var total uint64
	usedNames := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		fi, err := os.Stat(path)
		if err != nil || fi.IsDir() {
			continue
		}
		infos = append(infos, FileTransferFileInfo{Name: uniqueTransferName(filepath.Base(path), usedNames), Size: uint64(fi.Size())})
		validPaths = append(validPaths, path)
		total += uint64(fi.Size())
	}
	if len(infos) == 0 {
		return
	}

	var idBuf [4]byte
	n, err := randomRead(idBuf[:])
	if err != nil {
		log.Printf("ft: generate offer id: %v", err)
		return
	}
	if n != len(idBuf) {
		log.Printf("ft: short offer id read: got %d want %d", n, len(idBuf))
		return
	}
	offer := FileTransferOffer{
		ID:    binary.BigEndian.Uint32(idBuf[:]),
		Files: infos,
		Total: total,
	}
	payload, err := jsonMarshal(offer)
	if err != nil {
		log.Printf("ft: marshal offer id=%d: %v", offer.ID, err)
		return
	}

	send := newActiveSend()
	ft.mu.Lock()
	ft.outbound[offer.ID] = send
	ft.mu.Unlock()

	ft.send(protocol.Frame{Type: protocol.MsgFileTransferOffer, Payload: payload})
	log.Printf("ft: offer sent id=%d files=%d total=%d B", offer.ID, len(infos), total)

	logutil.SafeGo("ft-send", func() { ft.streamFiles(offer.ID, validPaths, infos, send) })
}

const ftChunkSize = 32 * 1024

func (ft *FileTransferManager) streamFiles(id uint32, paths []string, infos []FileTransferFileInfo, send *activeSend) {
	select {
	case <-send.accepted:
		log.Printf("ft: peer accepted id=%d, starting stream", id)
	case <-send.canceled:
		ft.clearOutbound(id, send)
		log.Printf("ft: transfer cancelled before accept id=%d", id)
		return
	case <-time.After(10 * time.Second):
		ft.clearOutbound(id, send)
		log.Printf("ft: no accept received for id=%d, aborting", id)
		return
	}

	buf := make([]byte, ftChunkSize)
	for fileIdx, path := range paths {
		select {
		case <-send.canceled:
			ft.clearOutbound(id, send)
			log.Printf("ft: transfer cancelled during send id=%d", id)
			return
		default:
		}

		fileHandle, err := os.Open(path)
		if err != nil {
			log.Printf("ft: open %s: %v", path, err)
			ft.clearOutbound(id, send)
			ft.sendCancel(id, 1)
			return
		}
		var offset uint64
		for {
			select {
			case <-send.canceled:
				fileHandle.Close()
				ft.clearOutbound(id, send)
				log.Printf("ft: transfer cancelled during send id=%d", id)
				return
			default:
			}

			n, err := fileHandle.Read(buf)
			if n > 0 {
				chunk := make([]byte, 16+n)
				binary.BigEndian.PutUint32(chunk[0:4], id)
				binary.BigEndian.PutUint32(chunk[4:8], uint32(fileIdx))
				binary.BigEndian.PutUint64(chunk[8:16], offset)
				copy(chunk[16:], buf[:n])
				ft.send(protocol.Frame{Type: protocol.MsgFileChunk, Payload: chunk})
				offset += uint64(n)
			}
			if err != nil {
				if err != io.EOF {
					fileHandle.Close()
					ft.clearOutbound(id, send)
					log.Printf("ft: read %s: %v", path, err)
					ft.sendCancel(id, 1)
					return
				}
				break
			}
		}
		fileHandle.Close()
		log.Printf("ft: sent file [%d] %s (%d B)", fileIdx, infos[fileIdx].Name, offset)
	}

	pay := make([]byte, 4)
	binary.BigEndian.PutUint32(pay, id)
	ft.clearOutbound(id, send)
	ft.send(protocol.Frame{Type: protocol.MsgFileTransferDone, Payload: pay})
	log.Printf("ft: send complete id=%d", id)
}
