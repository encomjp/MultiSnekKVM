//go:build windows

package filetransfer

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"multisnekkvm/internal/protocol"
)

var (
	ole32             = syscall.NewLazyDLL("ole32.dll")
	user32            = syscall.NewLazyDLL("user32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	shell32dll        = syscall.NewLazyDLL("shell32.dll")
	pCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	pCoUninitialize   = ole32.NewProc("CoUninitialize")
	pCoCreateInstance = ole32.NewProc("CoCreateInstance")
	pOleGetClipboard  = ole32.NewProc("OleGetClipboard")
	pReleaseStgMedium = ole32.NewProc("ReleaseStgMedium")
	pGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	pPeekMessageW     = user32.NewProc("PeekMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessageW = user32.NewProc("DispatchMessageW")
	pOpenClipboard    = user32.NewProc("OpenClipboard")
	pCloseClipboard   = user32.NewProc("CloseClipboard")
	pEmptyClipboard   = user32.NewProc("EmptyClipboard")
	pSetClipboardData = user32.NewProc("SetClipboardData")
	pGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	pGlobalFree       = kernel32.NewProc("GlobalFree")
	pGlobalLock       = kernel32.NewProc("GlobalLock")
	pGlobalSize       = kernel32.NewProc("GlobalSize")
	pGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	pShellExecuteW    = shell32dll.NewProc("ShellExecuteW")
	randomRead        = rand.Read
	jsonMarshal       = json.Marshal
)

func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func comRelease(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

const (
	pmRemove = 0x0001

	vtblStartProgressDialog = 3
	vtblStopProgressDialog  = 4
	vtblSetOperation        = 5
	vtblSetMode             = 6
	vtblUpdateProgress      = 7
	vtblGetOperationStatus  = 13

	spactionCopying = uintptr(2)
	pdopsRunning    = uint32(1)
	pdopsPaused     = uint32(2)
	pdopsCancelled  = uint32(3)
	coinitApartment = uintptr(0x2)
	clsctxInprocSrv = uintptr(0x1)

	cfHDrop         = uint16(15)
	tymedHGlobal    = uint32(1)
	dvaspectContent = uint32(1)
	vkLButton       = uintptr(0x01)
	gmemMoveable    = 0x0002
)

var clsidProgressDialog = [16]byte{
	0x52, 0x38, 0x38, 0xF8, 0xD3, 0xFC, 0xD1, 0x11,
	0xA6, 0xB9, 0x00, 0x60, 0x97, 0xDF, 0x5B, 0xD4,
}

var iidIOperationsProgressDialog = [16]byte{
	0x51, 0xB8, 0x9F, 0x0C, 0xC9, 0xE5, 0xEB, 0x43,
	0xA3, 0x70, 0xF0, 0x67, 0x7B, 0x13, 0x87, 0x4C,
}

type formatEtc struct {
	CfFormat uint16
	_        [6]byte
	Ptd      uintptr
	DwAspect uint32
	Lindex   int32
	Tymed    uint32
	_        uint32
}

type stgMedium struct {
	Tymed          uint32
	_              uint32
	HGlobal        uintptr
	PUnkForRelease uintptr
}

type dropFiles struct {
	PFiles uint32
	PtX    int32
	PtY    int32
	FNC    uint32
	FWide  uint32
}

type FileTransferFileInfo struct {
	Name string `json:"name"`
	Size uint64 `json:"size"`
}

type FileTransferOffer struct {
	ID    uint32                 `json:"id"`
	Files []FileTransferFileInfo `json:"files"`
	Total uint64                 `json:"total"`
}

type FileTransferManager struct {
	mu         sync.Mutex
	active     map[uint32]*activeRecv
	outbound   map[uint32]*activeSend
	sendFn     func(protocol.Frame)
	onComplete func(tempDir string, names []string)
}

type activeSend struct {
	accepted   chan struct{}
	canceled   chan struct{}
	acceptOnce sync.Once
	cancelOnce sync.Once
}

func newActiveSend() *activeSend {
	return &activeSend{
		accepted: make(chan struct{}),
		canceled: make(chan struct{}),
	}
}

func (as *activeSend) markAccepted() {
	as.acceptOnce.Do(func() { close(as.accepted) })
}

func (as *activeSend) markCanceled() {
	as.cancelOnce.Do(func() { close(as.canceled) })
}

type activeRecv struct {
	offer    FileTransferOffer
	tempDir  string
	files    []*os.File
	mu       sync.Mutex
	received []uint64
	total    uint64
	done     chan struct{}
	doneOnce sync.Once
}

func (ar *activeRecv) finish() { ar.doneOnce.Do(func() { close(ar.done) }) }

func NewFileTransferManager() *FileTransferManager {
	return &FileTransferManager{
		active:   make(map[uint32]*activeRecv),
		outbound: make(map[uint32]*activeSend),
	}
}

func (ft *FileTransferManager) SetSendFn(fn func(protocol.Frame)) {
	ft.mu.Lock()
	ft.sendFn = fn
	ft.mu.Unlock()
}

// SetOnComplete registers a callback invoked after each successful file transfer.
// The callback receives the temp directory path and the list of received file names.
func (ft *FileTransferManager) SetOnComplete(fn func(tempDir string, names []string)) {
	ft.mu.Lock()
	ft.onComplete = fn
	ft.mu.Unlock()
}

func (ft *FileTransferManager) send(f protocol.Frame) {
	ft.mu.Lock()
	fn := ft.sendFn
	ft.mu.Unlock()
	if fn != nil {
		fn(f)
	}
}
