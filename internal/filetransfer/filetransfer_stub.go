//go:build !windows

package filetransfer

import "multisnekkvm/internal/protocol"

type FileTransferFileInfo struct {
	Name string `json:"name"`
	Size uint64 `json:"size"`
}

type FileTransferOffer struct {
	ID    uint32                 `json:"id"`
	Files []FileTransferFileInfo `json:"files"`
	Total uint64                 `json:"total"`
}

type FileTransferManager struct{}

func NewFileTransferManager() *FileTransferManager               { return &FileTransferManager{} }
func (ft *FileTransferManager) SetSendFn(_ func(protocol.Frame)) {}
func (ft *FileTransferManager) StartSend(_ []string)             {}
func (ft *FileTransferManager) HandleInbound(_ protocol.Frame)   {}
func (ft *FileTransferManager) CancelAll()                       {}
func CaptureActiveDrag() []string                                { return nil }
