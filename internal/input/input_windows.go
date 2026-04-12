//go:build windows

package input

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"multisnekkvm/internal/protocol"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	pSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	pCallNextHookEx      = user32.NewProc("CallNextHookEx")
	pUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	pGetMessageW         = user32.NewProc("GetMessageW")
	pPostQuitMessage     = user32.NewProc("PostQuitMessage")
	pSendInput           = user32.NewProc("SendInput")
	pGetCursorPos        = user32.NewProc("GetCursorPos")
	pSetCursorPos        = user32.NewProc("SetCursorPos")
	pGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	pPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	pGetClipboardData    = user32.NewProc("GetClipboardData")
	pOpenClipboard       = user32.NewProc("OpenClipboard")
	pCloseClipboard      = user32.NewProc("CloseClipboard")
	pEmptyClipboard      = user32.NewProc("EmptyClipboard")
	pSetClipboardData    = user32.NewProc("SetClipboardData")
	pGlobalAlloc         = kernel32.NewProc("GlobalAlloc")
	pGlobalFree          = kernel32.NewProc("GlobalFree")
	pGlobalLock          = kernel32.NewProc("GlobalLock")
	pGlobalSize          = kernel32.NewProc("GlobalSize")
	pGlobalUnlock        = kernel32.NewProc("GlobalUnlock")
	pGetCurrentThreadId  = kernel32.NewProc("GetCurrentThreadId")
	pGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	pGetForegroundWindow = user32.NewProc("GetForegroundWindow")

	// Used for layout-aware Unicode character capture
	pToUnicodeEx              = user32.NewProc("ToUnicodeEx")
	pGetKeyboardState         = user32.NewProc("GetKeyboardState")
	pGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")

	// Used for monitor enumeration
	pEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	pGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
)

var (
	keyboardProcCallback = syscall.NewCallback(keyboardProc)
	mouseProcCallback    = syscall.NewCallback(mouseProc)
	openClipboardFn      = func() bool {
		ret, _, _ := pOpenClipboard.Call(0)
		return ret != 0
	}
	closeClipboardFn = func() {
		pCloseClipboard.Call()
	}
	clipboardRetrySleep = time.Sleep
)

// globalHook is accessed from both the main goroutine (SetConnected) and
// from hook callbacks running on the hook thread; use atomic to prevent races.
var globalHook atomic.Pointer[InputHook]

const (
	whKeyboardLL  = 13
	whMouseLL     = 14
	wmKeyDown     = 0x0100
	wmKeyUp       = 0x0101
	wmSysKeyDown  = 0x0104
	wmSysKeyUp    = 0x0105
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonDown = 0x0204
	wmRButtonUp   = 0x0205
	wmMButtonDown = 0x0207
	wmMButtonUp   = 0x0208
	wmMouseWheel  = 0x020A
	wmQuit        = 0x0012

	inputMouse    = 0
	inputKeyboard = 1

	mousefMove        = 0x0001
	mousefLeftDown    = 0x0002
	mousefLeftUp      = 0x0004
	mousefRightDown   = 0x0008
	mousefRightUp     = 0x0010
	mousefMiddleDown  = 0x0020
	mousefMiddleUp    = 0x0040
	mousefWheel       = 0x0800
	mousefAbsolute    = 0x8000
	mousefVirtualDesk = 0x4000

	keyEventFUp       = 0x0002
	keyEventFExtended = 0x0001
	keyEventFUnicode  = 0x0004

	smCxScreen        = 0
	smCyScreen        = 1
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCxVirtualScreen = 78
	smCyVirtualScreen = 79

	cfUnicodeText = 13
	gmemMoveable  = 0x0002

	clipboardOpenMaxAttempts = 5
	clipboardOpenRetryDelay  = 10 * time.Millisecond
)

type point struct{ X, Y int32 }

type kbdLLHookStruct struct {
	VKCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type msLLHookStruct struct {
	Pt        point
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type mouseInput struct {
	Type uint32
	Mi   mouseInputData
}

type mouseInputData struct {
	Dx        int32
	Dy        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type keybdInput struct {
	Type uint32
	Ki   keybdInputData
	_pad [8]byte
}

type keybdInputData struct {
	Wvk       uint16
	WScan     uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type InputHook struct {
	mu                 sync.RWMutex
	connected          bool
	inRemoteMode       bool
	sendFn             func(protocol.Frame)
	onStateChange      func()
	onEdgeDrag         func()
	stateChangeRunning bool
	stateChangePending bool
	edgeSide           string
	triggerArmed       bool
	sensitivity        float64

	// Configurable exit hotkey. exitVKCode==0 means use ESC (0x1B).
	// exitModifiers is a bitmask: 1=Ctrl, 2=Alt, 4=Shift, 8=Win.
	exitVKCode    uint16
	exitModifiers uint8

	// Trigger zone — which monitor+edge+zone% activates remote mode.
	// If hasTriggerZone is false, the legacy full-edge behaviour is used.
	hasTriggerZone  bool
	triggerEdge     string // "left","right","top","bottom"
	triggerMonX     int32  // trigger monitor pixel bounds
	triggerMonY     int32
	triggerMonW     int32
	triggerMonH     int32
	triggerStartPct float32 // 0.0–1.0 along the edge
	triggerEndPct   float32

	// Return anchor — where the cursor lands after exiting remote mode.
	// If hasReturnAnchor is false, the legacy center-of-primary logic is used.
	hasReturnAnchor bool
	returnMonX      int32 // return monitor pixel bounds
	returnMonY      int32
	returnMonW      int32
	returnMonH      int32
	returnXPct      float32 // 0.0–1.0 within the monitor
	returnYPct      float32

	virtLeft            int32
	virtRight           int32
	virtTop             int32
	virtBottom          int32
	centerX             int32
	centerY             int32
	hasLastTriggerPoint bool
	lastTriggerX        int32
	lastTriggerY        int32

	hookThreadID uint32
	kbHook       uintptr
	msHook       uintptr

	edgeStopCh         chan struct{}
	edgeDoneCh         chan struct{}
	lastRemoteExitTime time.Time
}

func NewInputHook() *InputHook {
	ih := &InputHook{edgeSide: "right", triggerArmed: true, sensitivity: 1.0}
	ih.refreshScreenMetrics()
	return ih
}

func (ih *InputHook) SetOnStateChange(fn func()) {
	ih.mu.Lock()
	ih.onStateChange = fn
	ih.mu.Unlock()
}

func (ih *InputHook) SetOnEdgeDrag(fn func()) {
	ih.mu.Lock()
	ih.onEdgeDrag = fn
	ih.mu.Unlock()
}

func (ih *InputHook) refreshScreenMetrics() {
	vx, _, _ := pGetSystemMetrics.Call(smXVirtualScreen)
	vy, _, _ := pGetSystemMetrics.Call(smYVirtualScreen)
	vw, _, _ := pGetSystemMetrics.Call(smCxVirtualScreen)
	vh, _, _ := pGetSystemMetrics.Call(smCyVirtualScreen)
	pw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	ph, _, _ := pGetSystemMetrics.Call(smCyScreen)
	ih.virtLeft = int32(vx)
	ih.virtRight = int32(vx) + int32(vw) - 1
	ih.virtTop = int32(vy)
	ih.virtBottom = int32(vy) + int32(vh) - 1
	ih.centerX = int32(pw) / 2
	ih.centerY = int32(ph) / 2
}

func (ih *InputHook) SetEdgeSide(side string) {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	normalized := NormalizeEdgeSide(side)
	if normalized != "" {
		ih.edgeSide = normalized
	}
}

func (ih *InputHook) GetEdgeSide() string {
	ih.mu.RLock()
	defer ih.mu.RUnlock()
	return ih.edgeSide
}

func (ih *InputHook) SetSensitivity(s float64) {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	if s >= 0.1 && s <= 5.0 {
		ih.sensitivity = s
	}
}

func (ih *InputHook) GetSensitivity() float64 {
	ih.mu.RLock()
	defer ih.mu.RUnlock()
	return ih.sensitivity
}

func GetScreenBounds() (left, right, top, bottom int32) {
	vx, _, _ := pGetSystemMetrics.Call(smXVirtualScreen)
	vy, _, _ := pGetSystemMetrics.Call(smYVirtualScreen)
	vw, _, _ := pGetSystemMetrics.Call(smCxVirtualScreen)
	vh, _, _ := pGetSystemMetrics.Call(smCyVirtualScreen)
	return int32(vx), int32(vx) + int32(vw) - 1, int32(vy), int32(vy) + int32(vh) - 1
}

func GetCursorPosition() (x, y int32) {
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return pt.X, pt.Y
}

// IsSecureDesktopActive returns true when the Windows Secure Desktop is in
// the foreground (e.g. a UAC prompt).  GetForegroundWindow returns NULL from
// non-elevated processes while the Secure Desktop owns the input focus.
func IsSecureDesktopActive() bool {
	r, _, _ := pGetForegroundWindow.Call()
	return r == 0
}

// SetExitHotkey configures the key combination that exits remote mode.
// modifiers is a bitmask: 1=Ctrl, 2=Alt, 4=Shift, 8=Win.
// vkCode 0 means ESC (the built-in fallback). ESC always works regardless.
func (ih *InputHook) SetExitHotkey(modifiers uint8, vkCode uint16) {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	ih.exitModifiers = modifiers
	ih.exitVKCode = vkCode
}

// GetExitHotkey returns the current exit hotkey configuration.
func (ih *InputHook) GetExitHotkey() (modifiers uint8, vkCode uint16) {
	ih.mu.RLock()
	defer ih.mu.RUnlock()
	return ih.exitModifiers, ih.exitVKCode
}

// MonitorInfo describes a connected display.
type MonitorInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	X         int32  `json:"x"`
	Y         int32  `json:"y"`
	Width     int32  `json:"width"`
	Height    int32  `json:"height"`
	IsPrimary bool   `json:"isPrimary"`
}

type monitorInfoEx struct {
	CbSize    uint32
	Left      int32
	Top       int32
	Right     int32
	Bottom    int32
	WorkLeft  int32
	WorkTop   int32
	WorkRight int32
	WorkBot   int32
	DwFlags   uint32
	SzDevice  [32]uint16
}

var (
	enumMu       sync.Mutex
	enumMonitors []MonitorInfo
	enumCallback = syscall.NewCallback(monitorEnumProc)
)

func monitorEnumProc(hmon, _ uintptr, _ uintptr, _ uintptr) uintptr {
	var info monitorInfoEx
	info.CbSize = uint32(unsafe.Sizeof(info))
	pGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&info)))
	name := syscall.UTF16ToString(info.SzDevice[:])
	enumMonitors = append(enumMonitors, MonitorInfo{
		ID:        name,
		Name:      name,
		X:         info.Left,
		Y:         info.Top,
		Width:     info.Right - info.Left,
		Height:    info.Bottom - info.Top,
		IsPrimary: info.DwFlags&1 != 0,
	})
	return 1
}

// EnumLocalMonitors returns all connected monitors and their pixel geometry.
func EnumLocalMonitors() []MonitorInfo {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumMonitors = nil
	pEnumDisplayMonitors.Call(0, 0, enumCallback, 0)
	result := make([]MonitorInfo, len(enumMonitors))
	copy(result, enumMonitors)
	return result
}

// SetTriggerZone configures which monitor edge and zone % activates remote mode.
// mon is a MonitorInfo from EnumLocalMonitors; side is "left"/"right"/"top"/"bottom".
// startPct and endPct (0.0–1.0) define the active segment along the edge.
func (ih *InputHook) SetTriggerZone(mon MonitorInfo, side string, startPct, endPct float32) {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	ih.hasTriggerZone = true
	ih.triggerEdge = side
	ih.triggerMonX = mon.X
	ih.triggerMonY = mon.Y
	ih.triggerMonW = mon.Width
	ih.triggerMonH = mon.Height
	ih.triggerStartPct = startPct
	ih.triggerEndPct = endPct
	// Sync edgeSide so EdgeConfig sent to peer matches the actual edge used.
	ih.edgeSide = side
}

// ClearTriggerZone reverts to legacy full-edge detection using edgeSide.
func (ih *InputHook) ClearTriggerZone() {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	ih.hasTriggerZone = false
}

// SetReturnAnchor configures where the cursor warps after exiting remote mode.
// xPct, yPct (0.0–1.0) are relative to the monitor's top-left corner.
func (ih *InputHook) SetReturnAnchor(mon MonitorInfo, xPct, yPct float32) {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	ih.hasReturnAnchor = true
	ih.returnMonX = mon.X
	ih.returnMonY = mon.Y
	ih.returnMonW = mon.Width
	ih.returnMonH = mon.Height
	ih.returnXPct = xPct
	ih.returnYPct = yPct
}

// ClearReturnAnchor reverts to the legacy center-of-primary-monitor return.
func (ih *InputHook) ClearReturnAnchor() {
	ih.mu.Lock()
	defer ih.mu.Unlock()
	ih.hasReturnAnchor = false
}

func (ih *InputHook) evaluateTriggerPoint(pt point) (zoneActive, armed bool) {
	zoneActive = ih.triggerActive(pt)
	ih.mu.Lock()
	defer ih.mu.Unlock()
	if !zoneActive {
		ih.triggerArmed = true
		return false, false
	}
	return true, ih.triggerArmed
}

func (ih *InputHook) rememberTriggerPoint(pt point) {
	ih.mu.Lock()
	ih.hasLastTriggerPoint = true
	ih.lastTriggerX = pt.X
	ih.lastTriggerY = pt.Y
	ih.mu.Unlock()
}

// triggerActive checks whether the given cursor position is within the
// configured trigger zone. Must not be called with ih.mu held.
func (ih *InputHook) triggerActive(pt point) bool {
	ih.mu.RLock()
	hasTZ := ih.hasTriggerZone
	side := ih.edgeSide
	tzEdge := ih.triggerEdge
	mx, my, mw, mh := ih.triggerMonX, ih.triggerMonY, ih.triggerMonW, ih.triggerMonH
	sp, ep := ih.triggerStartPct, ih.triggerEndPct
	vl, vr, vt, vb := ih.virtLeft, ih.virtRight, ih.virtTop, ih.virtBottom
	ih.mu.RUnlock()

	if !hasTZ {
		// Legacy: full virtual-screen edge
		switch side {
		case "right":
			return pt.X >= vr
		case "left":
			return pt.X <= vl
		case "top":
			return pt.Y <= vt
		case "bottom":
			return pt.Y >= vb
		}
		return false
	}

	// Zone-aware: check we're at the monitor's edge and within the % band
	switch tzEdge {
	case "left":
		if pt.X > mx {
			return false
		}
		bandStart := my + int32(float32(mh)*sp)
		bandEnd := my + int32(float32(mh)*ep)
		return pt.Y >= bandStart && pt.Y < bandEnd
	case "right":
		if pt.X < mx+mw-1 {
			return false
		}
		bandStart := my + int32(float32(mh)*sp)
		bandEnd := my + int32(float32(mh)*ep)
		return pt.Y >= bandStart && pt.Y < bandEnd
	case "top":
		if pt.Y > my {
			return false
		}
		bandStart := mx + int32(float32(mw)*sp)
		bandEnd := mx + int32(float32(mw)*ep)
		return pt.X >= bandStart && pt.X < bandEnd
	case "bottom":
		if pt.Y < my+mh-1 {
			return false
		}
		bandStart := mx + int32(float32(mw)*sp)
		bandEnd := mx + int32(float32(mw)*ep)
		return pt.X >= bandStart && pt.X < bandEnd
	}
	return false
}

// returnPosition returns the pixel coordinate to warp the cursor to after
// exiting remote mode. Must not be called with ih.mu held.
func (ih *InputHook) returnPosition() (x, y int32) {
	ih.mu.RLock()
	hasRA := ih.hasReturnAnchor
	rx, ry, rw, rh := ih.returnMonX, ih.returnMonY, ih.returnMonW, ih.returnMonH
	rxPct, ryPct := ih.returnXPct, ih.returnYPct
	side := ih.edgeSide
	cx, cy := ih.centerX, ih.centerY
	hasLastTriggerPoint := ih.hasLastTriggerPoint
	tx, ty := ih.lastTriggerX, ih.lastTriggerY
	vl, vr, vt, vb := ih.virtLeft, ih.virtRight, ih.virtTop, ih.virtBottom
	ih.mu.RUnlock()

	if hasRA {
		return rx + int32(float32(rw)*rxPct), ry + int32(float32(rh)*ryPct)
	}

	clamp := func(value, low, high int32) int32 {
		if value < low {
			return low
		}
		if value > high {
			return high
		}
		return value
	}

	// Legacy fallback: return near the exit edge and preserve the original
	// cross-edge height/width from when remote mode started.
	switch side {
	case "right":
		if hasLastTriggerPoint {
			return vr - 40, clamp(ty, vt, vb)
		}
		return vr - 40, cy
	case "left":
		if hasLastTriggerPoint {
			return vl + 40, clamp(ty, vt, vb)
		}
		return vl + 40, cy
	case "top":
		if hasLastTriggerPoint {
			return clamp(tx, vl, vr), vt + 40
		}
		return cx, vt + 40
	default:
		if hasLastTriggerPoint {
			return clamp(tx, vl, vr), vb - 40
		}
		return cx, vb - 40
	}
}
