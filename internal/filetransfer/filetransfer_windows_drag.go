//go:build windows

package filetransfer

import (
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Windows API imports for drag-and-drop capture.
var (
	pOleInitialize    = ole32.NewProc("OleInitialize")
	pOleUninitialize  = ole32.NewProc("OleUninitialize")
	pRegisterDragDrop = ole32.NewProc("RegisterDragDrop")
	pRevokeDragDrop   = ole32.NewProc("RevokeDragDrop")
	pRegisterClassExW = user32.NewProc("RegisterClassExW")
	pCreateWindowExW  = user32.NewProc("CreateWindowExW")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pDefWindowProcW   = user32.NewProc("DefWindowProcW")
	pSetWindowPos     = user32.NewProc("SetWindowPos")
	pShowWindow       = user32.NewProc("ShowWindow")
	pSendInput        = user32.NewProc("SendInput")
	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pUnregisterClassW = user32.NewProc("UnregisterClassW")
	pGetCursorPosDrag = user32.NewProc("GetCursorPos")
)

// dragCaptureBusy is an atomic flag preventing concurrent CaptureActiveDrag calls.
var dragCaptureBusy int32

const (
	hwndTopmost   = ^uintptr(0)
	swpShowWindow = 0x0040
	swHide        = 0
	swShow        = 5

	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExTransparent = 0x00000020
	wsExToolWindow  = 0x00000080
	wsExAcceptFiles = 0x00000010

	csNoClose = 0x0200

	vkEscape        = 0x1B
	vkLButtonDrag   = 0x01
	inputKeyboard   = 1
	inputMouse      = 0
	keyEventFKeyUp  = 0x0002
	mousefLeftUpDrg = 0x0004
)

// COM GUIDs for IDropTarget and IUnknown.
var iidIUnknown = [16]byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46,
}
var iidIDropTarget = [16]byte{
	0x22, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46,
}

// goDropTarget implements the COM IDropTarget interface via a manual vtable.
// The first field (lpVtbl) must be a pointer to the vtable array; COM
// clients resolve methods through it.
type goDropTarget struct {
	lpVtbl   uintptr
	refCount int32
	mu       sync.Mutex
	files    []string
	captured bool
	entered  bool // true once DragEnter has been called (even without files)
}

// Pre-allocated vtable for all goDropTarget instances.  Each entry is a
// syscall.NewCallback wrapping the corresponding IDropTarget method.
var dtVtbl = [7]uintptr{
	syscall.NewCallback(dtQueryInterface),
	syscall.NewCallback(dtAddRef),
	syscall.NewCallback(dtRelease),
	syscall.NewCallback(dtDragEnter),
	syscall.NewCallback(dtDragOver),
	syscall.NewCallback(dtDragLeave),
	syscall.NewCallback(dtDrop),
}

func newGoDropTarget() *goDropTarget {
	dt := &goDropTarget{refCount: 1}
	dt.lpVtbl = uintptr(unsafe.Pointer(&dtVtbl[0]))
	return dt
}

func (dt *goDropTarget) hasFiles() bool {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.captured
}

func (dt *goDropTarget) getFiles() []string {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	cp := make([]string, len(dt.files))
	copy(cp, dt.files)
	return cp
}

func (dt *goDropTarget) wasEntered() bool {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.entered
}

// --- IUnknown ---

func dtQueryInterface(this uintptr, riid uintptr, ppv uintptr) uintptr {
	guid := (*[16]byte)(unsafe.Pointer(riid))
	if *guid == iidIUnknown || *guid == iidIDropTarget {
		*(*uintptr)(unsafe.Pointer(ppv)) = this
		dtAddRef(this)
		return 0 // S_OK
	}
	*(*uintptr)(unsafe.Pointer(ppv)) = 0
	return 0x80004002 // E_NOINTERFACE
}

func dtAddRef(this uintptr) uintptr {
	dt := (*goDropTarget)(unsafe.Pointer(this))
	return uintptr(atomic.AddInt32(&dt.refCount, 1))
}

func dtRelease(this uintptr) uintptr {
	dt := (*goDropTarget)(unsafe.Pointer(this))
	count := atomic.AddInt32(&dt.refCount, -1)
	return uintptr(count)
}

// --- IDropTarget ---

// DragEnter is called by OLE when a drag enters our registered window.
// COM signature: DragEnter(IDataObject*, DWORD, POINTL, DWORD*)
// On x64 Windows, POINTL (two LONGs = 8 bytes) is passed by value but
// the calling convention splits it into two register-based parameters
// (ptX, ptY) in Go's syscall.NewCallback mapping.
func dtDragEnter(this, pDataObj, grfKeyState, ptX, ptY, pdwEffect uintptr) uintptr {
	dt := (*goDropTarget)(unsafe.Pointer(this))
	var files []string
	if pDataObj != 0 {
		files = extractHDropFiles(pDataObj)
	}
	dt.mu.Lock()
	dt.entered = true
	if len(files) > 0 {
		dt.files = files
		dt.captured = true
	}
	dt.mu.Unlock()
	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = 0 // DROPEFFECT_NONE — match Barrier
	}
	return 0 // S_OK
}

func dtDragOver(this, grfKeyState, ptX, ptY, pdwEffect uintptr) uintptr {
	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = 0 // DROPEFFECT_NONE
	}
	return 0
}

func dtDragLeave(this uintptr) uintptr {
	return 0
}

func dtDrop(this, pDataObj, grfKeyState, ptX, ptY, pdwEffect uintptr) uintptr {
	dt := (*goDropTarget)(unsafe.Pointer(this))
	// Also capture on Drop in case DragEnter was missed.
	if pDataObj != 0 && !dt.hasFiles() {
		files := extractHDropFiles(pDataObj)
		if len(files) > 0 {
			dt.mu.Lock()
			dt.files = files
			dt.captured = true
			dt.mu.Unlock()
		}
	}
	if pdwEffect != 0 {
		*(*uint32)(unsafe.Pointer(pdwEffect)) = 0 // DROPEFFECT_NONE
	}
	return 0
}

// extractHDropFiles reads CF_HDROP file paths from a COM IDataObject.
// This reuses the same HDROP parsing logic as ReadOleDragFiles but takes
// the IDataObject pointer directly instead of querying the OLE clipboard.
func extractHDropFiles(dataObj uintptr) []string {
	fe := formatEtc{
		CfFormat: cfHDrop,
		DwAspect: dvaspectContent,
		Lindex:   -1,
		Tymed:    tymedHGlobal,
	}
	var medium stgMedium

	// IDataObject::GetData — vtable index 3
	hr := comCall(dataObj, 3,
		uintptr(unsafe.Pointer(&fe)),
		uintptr(unsafe.Pointer(&medium)),
	)
	if hr != 0 {
		return nil
	}
	defer pReleaseStgMedium.Call(uintptr(unsafe.Pointer(&medium)))

	if medium.Tymed != tymedHGlobal || medium.HGlobal == 0 {
		return nil
	}

	ptr, _, _ := pGlobalLock.Call(medium.HGlobal)
	if ptr == 0 {
		return nil
	}
	defer pGlobalUnlock.Call(medium.HGlobal)

	df := (*dropFiles)(unsafe.Pointer(ptr))
	fileListPtr := ptr + uintptr(df.PFiles)
	totalSize, _, _ := pGlobalSize.Call(medium.HGlobal)
	if totalSize <= uintptr(df.PFiles) {
		return nil
	}
	remaining := totalSize - uintptr(df.PFiles)

	var paths []string
	if df.FWide != 0 {
		maxWChars := int(remaining / 2)
		wchars := unsafe.Slice((*uint16)(unsafe.Pointer(fileListPtr)), maxWChars)
		for i := 0; i < len(wchars); {
			end := i
			for end < len(wchars) && wchars[end] != 0 {
				end++
			}
			if end == i {
				break
			}
			paths = append(paths, syscall.UTF16ToString(wchars[i:end+1]))
			i = end + 1
		}
	} else {
		ansiBytes := unsafe.Slice((*byte)(unsafe.Pointer(fileListPtr)), remaining)
		for i := 0; i < len(ansiBytes); {
			end := i
			for end < len(ansiBytes) && ansiBytes[end] != 0 {
				end++
			}
			if end == i {
				break
			}
			paths = append(paths, string(ansiBytes[i:end]))
			i = end + 1
		}
	}
	return paths
}

// ── Window class for the invisible drop-target window ──────────────────

var (
	dragWndClassName    *uint16
	dragWndClassAtom    uintptr
	dragWndClassMu      sync.Mutex
	dragWndProcCallback = syscall.NewCallback(dragWndProc)
)

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

func dragWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	ret, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func ensureDragWindowClass() uintptr {
	dragWndClassMu.Lock()
	defer dragWndClassMu.Unlock()
	if dragWndClassAtom != 0 {
		return dragWndClassAtom
	}
	dragWndClassName, _ = syscall.UTF16PtrFromString("MultiSnekDragCapture")
	hInst, _, _ := pGetModuleHandleW.Call(0)
	wcx := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		Style:         csNoClose,
		LpfnWndProc:   dragWndProcCallback,
		HInstance:     hInst,
		LpszClassName: dragWndClassName,
	}
	atom, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcx)))
	dragWndClassAtom = atom
	return dragWndClassAtom
}

func createDragCaptureWindow() uintptr {
	atom := ensureDragWindowClass()
	if atom == 0 {
		return 0
	}
	hInst, _, _ := pGetModuleHandleW.Call(0)
	name, _ := syscall.UTF16PtrFromString("MultiSnekDrop")
	// WS_EX_TRANSPARENT is omitted — while Barrier uses it on their drop
	// window, the OLE DoDragDrop loop relies on WindowFromPoint to find
	// drop targets, and WS_EX_TRANSPARENT causes the default WM_NCHITTEST
	// to return HTTRANSPARENT.  Omitting it is safer for our topmost
	// approach where we pop the window in during an active drag.
	hwnd, _, _ := pCreateWindowExW.Call(
		wsExTopmost|wsExToolWindow|wsExAcceptFiles,
		atom,
		uintptr(unsafe.Pointer(name)),
		wsPopup,
		0, 0, 1, 1,
		0, 0,
		hInst,
		0,
	)
	return hwnd
}

// ── Input simulation ───────────────────────────────────────────────────

type keybdInputDrag struct {
	Type uint32
	Ki   struct {
		Wvk       uint16
		WScan     uint16
		Flags     uint32
		Time      uint32
		ExtraInfo uintptr
	}
	_pad [8]byte
}

type mouseInputDrag struct {
	Type uint32
	Mi   struct {
		Dx        int32
		Dy        int32
		MouseData uint32
		Flags     uint32
		Time      uint32
		ExtraInfo uintptr
	}
}

func simulateEscapeKey() {
	var inputs [2]keybdInputDrag
	// Key down
	inputs[0].Type = inputKeyboard
	inputs[0].Ki.Wvk = vkEscape
	inputs[0].Ki.WScan = 0x01 // Escape scan code
	// Key up
	inputs[1].Type = inputKeyboard
	inputs[1].Ki.Wvk = vkEscape
	inputs[1].Ki.WScan = 0x01
	inputs[1].Ki.Flags = keyEventFKeyUp
	n, _, _ := pSendInput.Call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	if n == 0 {
		log.Println("drag-capture: SendInput(Escape) failed, UIPI may be blocking")
	}
}

func simulateLeftMouseUp() {
	var input mouseInputDrag
	input.Type = inputMouse
	input.Mi.Flags = mousefLeftUpDrg
	n, _, _ := pSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	if n == 0 {
		log.Println("drag-capture: SendInput(MouseUp) failed")
	}
}

// simulateMouseNudge injects a zero-delta MOUSEEVENTF_MOVE so that DoDragDrop
// re-evaluates WindowFromPoint and discovers a newly-positioned drop window.
func simulateMouseNudge() {
	var input mouseInputDrag
	input.Type = inputMouse
	input.Mi.Flags = 0x0001 // MOUSEEVENTF_MOVE, relative, dx=0 dy=0
	n, _, _ := pSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	if n == 0 {
		log.Println("drag-capture: SendInput(MouseNudge) failed")
	}
}

// ── Message pumping ────────────────────────────────────────────────────

// msgDrag is the Windows MSG struct with proper field alignment for x64.
type msgDrag struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

func pumpMessagesDrag() {
	var msg msgDrag
	for {
		r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)
		if r == 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func pumpFor(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		pumpMessagesDrag()
		time.Sleep(time.Millisecond)
	}
}

// ── Public API ─────────────────────────────────────────────────────────

// CaptureActiveDrag intercepts an ongoing Windows drag-and-drop operation.
// It creates a temporary hidden drop-target window under the cursor,
// simulates Escape to cancel the drag on the source side, and extracts
// dragged file paths from the IDataObject delivered to DragEnter.
//
// This follows the same strategy used by Barrier/Deskflow to capture
// cross-machine drags.  Must be called while a drag is in progress (left
// mouse button held down).
//
// Returns the list of local file paths being dragged, or nil if no drag
// data was available.
func CaptureActiveDrag() []string {
	// Prevent concurrent capture attempts.
	if !atomic.CompareAndSwapInt32(&dragCaptureBusy, 0, 1) {
		log.Println("drag-capture: already in progress, skipping")
		return nil
	}
	defer atomic.StoreInt32(&dragCaptureBusy, 0)

	// COM/OLE must be initialized on a dedicated OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := pOleInitialize.Call(0)
	// S_OK (0) or S_FALSE (1) both mean success.
	if hr != 0 && hr != 1 {
		log.Printf("drag-capture: OleInitialize failed hr=0x%08X", uint32(hr))
		return nil
	}
	defer pOleUninitialize.Call()

	dt := newGoDropTarget()

	hwnd := createDragCaptureWindow()
	if hwnd == 0 {
		log.Println("drag-capture: failed to create drop window")
		return nil
	}
	defer pDestroyWindow.Call(hwnd)

	hr, _, _ = pRegisterDragDrop.Call(hwnd, uintptr(unsafe.Pointer(dt)))
	if hr != 0 {
		log.Printf("drag-capture: RegisterDragDrop failed hr=0x%08X", uint32(hr))
		return nil
	}
	defer pRevokeDragDrop.Call(hwnd)

	// Position the drop window under the cursor so OLE delivers DragEnter.
	var pt struct{ X, Y int32 }
	pGetCursorPosDrag.Call(uintptr(unsafe.Pointer(&pt)))
	const dropWndSize = 40
	pSetWindowPos.Call(hwnd, hwndTopmost,
		uintptr(pt.X-dropWndSize/2), uintptr(pt.Y-dropWndSize/2),
		dropWndSize, dropWndSize,
		swpShowWindow)

	// Nudge the cursor (zero-delta move) so DoDragDrop re-evaluates
	// WindowFromPoint and discovers our newly-positioned topmost window.
	// Without this, DragEnter may never fire if the cursor is stationary.
	simulateMouseNudge()

	// Wait for DragEnter to fire.  Pump messages so OLE routes the drag
	// to our window.  Break early once DragEnter has been called
	// (regardless of whether it carried file data).
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && !dt.wasEntered() {
		pumpFor(10 * time.Millisecond)
	}

	if dt.hasFiles() {
		// Files captured — cancel the drag on the source side.
		// Simulate Escape so the source's DoDragDrop returns
		// DRAGDROP_S_CANCEL, then release the mouse button.
		simulateEscapeKey()
		pumpFor(50 * time.Millisecond)
		simulateLeftMouseUp()
		pumpFor(50 * time.Millisecond)
	} else {
		// No file data in drag (text, image, or no drag active).
		// Don't cancel — leave the source drag operation intact.
		log.Println("drag-capture: no file data in drag, leaving drag intact")
	}

	pShowWindow.Call(hwnd, swHide)

	files := dt.getFiles()
	if len(files) > 0 {
		log.Printf("drag-capture: captured %d files from active drag", len(files))
	}

	// Prevent GC from collecting the drop target while COM still holds a
	// reference to it.  The deferred RevokeDragDrop above releases the COM
	// reference, but this ensures dt survives until that point.
	runtime.KeepAlive(dt)

	return files
}
