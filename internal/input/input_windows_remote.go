//go:build windows

package input

import (
	"log"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/protocol"
)

// hookKeyboardLayout caches the HKL (keyboard layout handle) of the controller's
// thread at remote-mode entry. Using it in the hook callback avoids calling
// GetForegroundWindow + GetWindowThreadProcessId + GetKeyboardLayout on every
// key press — those 3 extra API calls can push the hook callback past
// LowLevelHooksTimeout (~300-1000 ms), causing Windows to silently remove the hook.
var hookKeyboardLayout uintptr

var (
	postThreadQuitMessage = func(threadID uint32) bool {
		ret, _, _ := pPostThreadMessageW.Call(uintptr(threadID), wmQuit, 0, 0)
		return ret != 0
	}
	edgeStopWaitTimeout    = 2 * time.Second
	stateChangeWarnTimeout = 2 * time.Second
)

func (ih *InputHook) SetConnected(connected bool, sendFn func(protocol.Frame)) {
	ih.mu.Lock()
	wasConnected := ih.connected
	ih.connected = connected
	ih.sendFn = sendFn
	if connected && !wasConnected {
		ih.triggerArmed = false
	}
	ih.mu.Unlock()

	if connected && !wasConnected {
		globalHook.Store(ih)
		ih.edgeStopCh = make(chan struct{})
		ih.edgeDoneCh = make(chan struct{})
		go func() {
			defer close(ih.edgeDoneCh)
			ih.edgeLoop()
		}()
		log.Println("edge monitoring started")
	} else if !connected && wasConnected {
		ih.mu.RLock()
		inRemote := ih.inRemoteMode
		stopCh := ih.edgeStopCh
		doneCh := ih.edgeDoneCh
		ih.mu.RUnlock()
		if inRemote {
			ih.requestExitRemote()
		}
		globalHook.Store(nil)
		if stopCh != nil {
			close(stopCh)
		}
		if doneCh != nil {
			select {
			case <-doneCh:
			case <-time.After(edgeStopWaitTimeout):
				log.Printf("edge monitoring stop timed out after %v", edgeStopWaitTimeout)
			}
		}
		log.Println("edge monitoring stopped")
	}
}

func (ih *InputHook) IsInRemoteMode() bool {
	ih.mu.RLock()
	defer ih.mu.RUnlock()
	return ih.inRemoteMode
}

func (ih *InputHook) ExitRemoteMode() {
	ih.mu.RLock()
	inRemote := ih.inRemoteMode
	ih.mu.RUnlock()
	if inRemote {
		ih.requestExitRemote()
	}
}

func (ih *InputHook) requestExitRemote() {
	ih.mu.Lock()
	ih.inRemoteMode = false
	tid := ih.hookThreadID
	ih.mu.Unlock()
	if tid != 0 && !postThreadQuitMessage(tid) {
		log.Printf("failed to post WM_QUIT to remote input hook thread %d", tid)
	}
}

const edgeDwellDuration = 50 * time.Millisecond

func (ih *InputHook) edgeLoop() {
	var edgeDwellStart time.Time
	for {
		select {
		case <-ih.edgeStopCh:
			return
		default:
		}

		ih.mu.RLock()
		connected := ih.connected
		inRemote := ih.inRemoteMode
		ih.mu.RUnlock()

		if !connected {
			return
		}

		if !inRemote {
			var pt point
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			zoneActive, armed := ih.evaluateTriggerPoint(pt)

			// Suppress edge trigger for a short window after remote mode exits
			// to prevent immediately bouncing back (e.g. after a UAC exit).
			ih.mu.RLock()
			cooldownOk := time.Since(ih.lastRemoteExitTime) >= 500*time.Millisecond
			ih.mu.RUnlock()

			if cooldownOk && zoneActive && armed {
				// Require the cursor to dwell at the edge for edgeDwellDuration
				// before triggering — prevents accidental triggers on fast moves.
				if edgeDwellStart.IsZero() {
					edgeDwellStart = time.Now()
				}
				if time.Since(edgeDwellStart) < edgeDwellDuration {
					time.Sleep(8 * time.Millisecond)
					continue
				}

				// Dwell satisfied. If LMB is still held (window drag / file drag):
				// call the drag callback, then wait for button release. After
				// release, require a fresh dwell — the cursor is still at the edge
				// at mouse-up, and entering remote mode immediately would break
				// any drop target on the host (dropped files would land in void).
				if isLeftMouseButtonDown() {
					ih.mu.RLock()
					dragFn := ih.onEdgeDrag
					ih.mu.RUnlock()
					if dragFn != nil {
						dragFn()
					}
					for isLeftMouseButtonDown() {
						select {
						case <-ih.edgeStopCh:
							return
						default:
						}
						time.Sleep(10 * time.Millisecond)
					}
					// Fresh dwell required: cursor is still at the edge.
					edgeDwellStart = time.Time{}
					continue
				}

				ih.rememberTriggerPoint(pt)
				ih.runRemoteMode()
				edgeDwellStart = time.Time{}
				ih.mu.Lock()
				ih.lastRemoteExitTime = time.Now()
				ih.triggerArmed = false
				ih.mu.Unlock()
				continue
			} else {
				// Cursor has left the edge zone — reset dwell.
				edgeDwellStart = time.Time{}
			}
		}

		time.Sleep(8 * time.Millisecond)
	}
}

func (ih *InputHook) runRemoteMode() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ih.refreshScreenMetrics()

	// Install hooks BEFORE setting inRemoteMode so a failed install doesn't
	// leave us stuck claiming remote mode with no actual input forwarding.
	kbHook, _, _ := pSetWindowsHookExW.Call(whKeyboardLL, keyboardProcCallback, 0, 0)
	msHook, _, _ := pSetWindowsHookExW.Call(whMouseLL, mouseProcCallback, 0, 0)
	if kbHook == 0 || msHook == 0 {
		log.Println("remote mode: failed to install input hooks, aborting")
		if kbHook != 0 {
			pUnhookWindowsHookEx.Call(kbHook)
		}
		if msHook != 0 {
			pUnhookWindowsHookEx.Call(msHook)
		}
		return
	}

	tid, _, _ := pGetCurrentThreadId.Call()

	// Cache the keyboard layout of the foreground window's thread — that is the
	// layout the user is actually typing with. The hook thread inherits the process
	// default, which can differ from the active input method.
	// This cached value is used by resolveUnicodeChar to avoid calling the 3 expensive
	// Win32 APIs (GetForegroundWindow + GetWindowThreadProcessId + GetKeyboardLayout)
	// inside every hook callback, which risks exceeding LowLevelHooksTimeout.
	{
		fgWnd, _, _ := pGetForegroundWindow.Call()
		var fgTid32 uint32
		pGetWindowThreadProcessId.Call(fgWnd, uintptr(unsafe.Pointer(&fgTid32)))
		hkl, _, _ := pGetKeyboardLayout.Call(uintptr(fgTid32))
		atomic.StoreUintptr(&hookKeyboardLayout, hkl)
	}

	ih.mu.Lock()
	ih.inRemoteMode = true
	ih.kbHook = kbHook
	ih.msHook = msHook
	ih.hookThreadID = uint32(tid)
	ih.mu.Unlock()

	pSetCursorPos.Call(uintptr(ih.centerX), uintptr(ih.centerY))
	ih.sendRemoteWake()

	log.Println("remote mode active — mouse/keyboard forwarding to peer")
	ih.notifyStateChange()

	// UAC monitor: exit remote mode when the Secure Desktop (UAC prompt) is
	// active.  GetForegroundWindow() returns NULL from non-elevated processes
	// while the Secure Desktop owns input.  Require 2 consecutive NULLs
	// (~200 ms apart) to avoid false positives during normal window switches.
	uacDone := make(chan struct{})
	go func() {
		defer close(uacDone)
		nullCount := 0
		for {
			select {
			case <-uacDone:
				return
			default:
			}
			ih.mu.RLock()
			still := ih.inRemoteMode
			ih.mu.RUnlock()
			if !still {
				return
			}
			r, _, _ := pGetForegroundWindow.Call()
			if r == 0 {
				nullCount++
				if nullCount >= 2 {
					log.Println("remote mode: Secure Desktop detected (UAC), exiting remote mode")
					ih.requestExitRemote()
					return
				}
			} else {
				nullCount = 0
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	var msg struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      point
	}
	for {
		ret, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 {
			break
		}
		ih.mu.RLock()
		inRemote := ih.inRemoteMode
		ih.mu.RUnlock()
		if !inRemote {
			break
		}
	}

	// Signal the UAC monitor to stop and wait for it.
	// uacDone is already closed when the goroutine returns naturally; close
	// triggers it to stop if the message loop ended first.
	select {
	case <-uacDone:
	default:
		// goroutine may be sleeping; it will exit when it sees inRemoteMode==false
	}

	if kbHook != 0 {
		pUnhookWindowsHookEx.Call(kbHook)
	}
	if msHook != 0 {
		pUnhookWindowsHookEx.Call(msHook)
	}

	ih.mu.Lock()
	ih.kbHook = 0
	ih.msHook = 0
	ih.inRemoteMode = false
	ih.hookThreadID = 0
	ih.mu.Unlock()

	rx, ry := ih.returnPosition()
	pSetCursorPos.Call(uintptr(rx), uintptr(ry))

	log.Println("remote mode deactivated")
	ih.notifyStateChange()
}

func (ih *InputHook) notifyStateChange() {
	ih.mu.Lock()
	fn := ih.onStateChange
	if fn == nil {
		ih.mu.Unlock()
		return
	}
	if ih.stateChangeRunning {
		ih.stateChangePending = true
		ih.mu.Unlock()
		return
	}
	ih.stateChangeRunning = true
	ih.mu.Unlock()

	logutil.SafeGo("input-state-change", func() {
		defer func() {
			ih.mu.Lock()
			ih.stateChangeRunning = false
			ih.stateChangePending = false
			ih.mu.Unlock()
		}()

		current := fn
		for {
			runStateChangeCallback(current)

			ih.mu.Lock()
			current = ih.onStateChange
			pending := ih.stateChangePending && current != nil
			ih.stateChangePending = false
			ih.mu.Unlock()
			if !pending {
				return
			}
		}
	})
}

func runStateChangeCallback(fn func()) {
	if fn == nil {
		return
	}
	done := make(chan struct{})
	timer := time.AfterFunc(stateChangeWarnTimeout, func() {
		select {
		case <-done:
		default:
			log.Printf("input: onStateChange still running after %v", stateChangeWarnTimeout)
		}
	})
	defer func() {
		close(done)
		timer.Stop()
	}()
	fn()
}

func (ih *InputHook) sendRemoteWake() {
	ih.mu.RLock()
	fn := ih.sendFn
	ih.mu.RUnlock()
	if fn == nil {
		return
	}
	fn(protocol.Frame{Type: protocol.MsgMouseMove, Payload: protocol.MouseMoveMsg{}.Encode()})
}

// isLeftMouseButtonDown returns true if the left mouse button is currently held.
func isLeftMouseButtonDown() bool {
	r, _, _ := pGetAsyncKeyState.Call(0x01) // VK_LBUTTON
	return r&0x8000 != 0
}

func keyboardProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	gh := globalHook.Load()
	if nCode >= 0 && gh != nil && gh.IsInRemoteMode() {
		kb := (*kbdLLHookStruct)(unsafe.Pointer(lParam))

		isKeyDown := wParam == wmKeyDown || wParam == wmSysKeyDown

		// --- Exit hotkey detection ---
		if isKeyDown {
			gh.mu.RLock()
			cfgVK := gh.exitVKCode
			cfgMod := gh.exitModifiers
			gh.mu.RUnlock()

			// Determine currently-pressed modifier mask.
			var heldMod uint8
			if asyncKeyDown(0xA2) || asyncKeyDown(0xA3) { // VK_LCONTROL / VK_RCONTROL
				heldMod |= 1
			}
			if asyncKeyDown(0xA4) || asyncKeyDown(0xA5) { // VK_LMENU / VK_RMENU
				heldMod |= 2
			}
			if asyncKeyDown(0xA0) || asyncKeyDown(0xA1) { // VK_LSHIFT / VK_RSHIFT
				heldMod |= 4
			}
			if asyncKeyDown(0x5B) || asyncKeyDown(0x5C) { // VK_LWIN / VK_RWIN
				heldMod |= 8
			}

			// Hard ESC fallback (always works regardless of configured hotkey).
			escExit := kb.VKCode == 0x1B && heldMod == 0
			// Configured hotkey exit.
			configuredExit := false
			if cfgVK != 0 {
				configuredExit = uint32(kb.VKCode) == uint32(cfgVK) && heldMod == cfgMod
			}

			if escExit || configuredExit {
				gh.mu.Lock()
				fn := gh.sendFn
				gh.inRemoteMode = false
				gh.mu.Unlock()
				if fn != nil {
					fn(protocol.Frame{Type: protocol.MsgSwitchBack})
				}
				pPostQuitMessage.Call(0)
				return 1
			}
		}

		// --- Key event forwarding ---
		var msgType byte
		switch wParam {
		case wmKeyDown, wmSysKeyDown:
			msgType = protocol.MsgKeyDown
		case wmKeyUp, wmSysKeyUp:
			msgType = protocol.MsgKeyUp
		default:
			ret, _, _ := pCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
			return ret
		}

		gh.mu.RLock()
		fn := gh.sendFn
		gh.mu.RUnlock()
		if fn == nil {
			return 1
		}

		// Unicode path: for plain keydown with no Ctrl/Alt/Win modifier, attempt
		// to resolve the character using the controller's keyboard layout so the
		// host receives the correct character regardless of its own layout.
		if msgType == protocol.MsgKeyDown {
			if ch, ok := resolveUnicodeChar(kb.VKCode, kb.ScanCode); ok {
				fn(protocol.Frame{Type: protocol.MsgUnicodeText, Payload: protocol.UnicodeTextMsg{Char: ch}.Encode()})
				return 1
			}
		}

		fn(protocol.Frame{Type: msgType, Payload: protocol.KeyMsg{
			VKCode: kb.VKCode, ScanCode: kb.ScanCode, Flags: kb.Flags,
		}.Encode()})
		return 1
	}

	ret, _, _ := pCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

// asyncKeyDown returns true if the given VK code is currently pressed.
func asyncKeyDown(vk uint32) bool {
	r, _, _ := pGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

// resolveUnicodeChar converts a VK+scan code to a Unicode character using the
// current thread's keyboard layout, but ONLY for plain printable characters
// (no Ctrl/Alt/Win modifier held, result is a single printable code point).
// Returns (char, true) on success; (0, false) if the raw-key path should be used.
func resolveUnicodeChar(vkCode, scanCode uint32) (uint32, bool) {
	// Skip if any Ctrl, Alt, or Win modifier is down.
	if asyncKeyDown(0x11) || asyncKeyDown(0x12) || asyncKeyDown(0x5B) || asyncKeyDown(0x5C) {
		return 0, false
	}

	// Skip non-printable VK ranges: function keys, navigation, modifier keys, etc.
	switch {
	case vkCode < 0x20: // control chars
		return 0, false
	case vkCode >= 0x70 && vkCode <= 0x7B: // F1–F12
		return 0, false
	case vkCode >= 0x10 && vkCode <= 0x12: // Shift, Ctrl, Alt
		return 0, false
	case vkCode == 0x1B, vkCode == 0x08, vkCode == 0x09, vkCode == 0x0D: // Esc, Backspace, Tab, Enter
		return 0, false
	case vkCode >= 0x21 && vkCode <= 0x2F: // PageUp–Delete, Home, End, etc.
		return 0, false
	case vkCode >= 0x5B && vkCode <= 0x5F: // Win, Apps, Sleep
		return 0, false
	case vkCode >= 0xA0 && vkCode <= 0xA5: // Shift/Ctrl/Alt L/R
		return 0, false
	}

	// Get keyboard state for the current thread to include Shift/CapsLock.
	var kbState [256]byte
	r, _, _ := pGetKeyboardState.Call(uintptr(unsafe.Pointer(&kbState[0])))
	if r == 0 {
		return 0, false
	}

	// Use the layout cached at remote-mode entry to avoid Win32 API calls
	// (GetForegroundWindow + GetWindowThreadProcessId + GetKeyboardLayout) inside
	// the hook callback, which can cause Windows to silently drop the hook.
	layout := atomic.LoadUintptr(&hookKeyboardLayout)

	var buf [4]uint16
	n, _, _ := pToUnicodeEx.Call(
		uintptr(vkCode),
		uintptr(scanCode),
		uintptr(unsafe.Pointer(&kbState[0])),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
		layout,
	)
	// n==1: single character resolved; n<0: dead key (skip); n==0: no translation
	if n == 1 && buf[0] >= 0x20 {
		return uint32(buf[0]), true
	}
	return 0, false
}

func mouseProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	gh := globalHook.Load()
	if nCode >= 0 && gh != nil && gh.IsInRemoteMode() {
		ms := (*msLLHookStruct)(unsafe.Pointer(lParam))

		gh.mu.RLock()
		fn := gh.sendFn
		sens := gh.sensitivity
		gh.mu.RUnlock()
		if fn == nil {
			return 1
		}

		switch wParam {
		case wmMouseMove:
			dx := ms.Pt.X - gh.centerX
			dy := ms.Pt.Y - gh.centerY
			if dx != 0 || dy != 0 {
				sdx := int32(float64(dx) * sens)
				sdy := int32(float64(dy) * sens)
				if sdx == 0 && dx != 0 {
					sdx = 1
					if dx < 0 {
						sdx = -1
					}
				}
				if sdy == 0 && dy != 0 {
					sdy = 1
					if dy < 0 {
						sdy = -1
					}
				}
				fn(protocol.Frame{Type: protocol.MsgMouseMove, Payload: protocol.MouseMoveMsg{DX: sdx, DY: sdy}.Encode()})
				pSetCursorPos.Call(uintptr(gh.centerX), uintptr(gh.centerY))
			}
		case wmLButtonDown:
			fn(protocol.Frame{Type: protocol.MsgMouseClick, Payload: protocol.MouseClickMsg{Button: 0, Pressed: true}.Encode()})
		case wmLButtonUp:
			fn(protocol.Frame{Type: protocol.MsgMouseClick, Payload: protocol.MouseClickMsg{Button: 0, Pressed: false}.Encode()})
		case wmRButtonDown:
			fn(protocol.Frame{Type: protocol.MsgMouseClick, Payload: protocol.MouseClickMsg{Button: 1, Pressed: true}.Encode()})
		case wmRButtonUp:
			fn(protocol.Frame{Type: protocol.MsgMouseClick, Payload: protocol.MouseClickMsg{Button: 1, Pressed: false}.Encode()})
		case wmMButtonDown:
			fn(protocol.Frame{Type: protocol.MsgMouseClick, Payload: protocol.MouseClickMsg{Button: 2, Pressed: true}.Encode()})
		case wmMButtonUp:
			fn(protocol.Frame{Type: protocol.MsgMouseClick, Payload: protocol.MouseClickMsg{Button: 2, Pressed: false}.Encode()})
		case wmMouseWheel:
			delta := int32(int16(ms.MouseData >> 16))
			fn(protocol.Frame{Type: protocol.MsgMouseScroll, Payload: protocol.MouseScrollMsg{Delta: delta}.Encode()})
		}

		return 1
	}

	ret, _, _ := pCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}
