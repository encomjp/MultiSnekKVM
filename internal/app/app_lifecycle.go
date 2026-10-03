package app

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"
)

func (a *App) sendFrame(f Frame) error {
	if a.transport == nil {
		return fmt.Errorf("transport unavailable")
	}
	// Slow writes and failures are logged (rate-limited) by muxSend, the
	// only caller.
	return a.transport.Send(f)
}

func (a *App) sendEdgeConfig() {
	if a.transport == nil || a.inputHook == nil || a.transport.GetSession() == nil {
		return
	}
	side := a.inputHook.GetEdgeSide()
	a.enqueueSend(Frame{Type: MsgEdgeConfig, Payload: EdgeConfigMsg{EdgeSide: side}.Encode()})
}

func (a *App) resetControlledState() {
	a.releaseInjectedRemoteKeys()
	// Reset inactivity stamp so the watchdog doesn't fire on the next session.
	atomic.StoreInt64(&a.lastRemoteInputNs, 0)
	a.mu.Lock()
	a.controllerEdgeSide = ""
	a.peerControlActive = false
	a.peerControlRequiresWake = false
	a.switchBackSent = false
	a.mu.Unlock()
}

func (a *App) configurePeerControl(side string) {
	a.mu.Lock()
	active := a.peerControlActive
	a.controllerEdgeSide = side
	if !active {
		a.peerControlRequiresWake = false
		a.switchBackSent = false
	}
	a.mu.Unlock()
	if !active {
		atomic.StoreInt64(&a.lastRemoteInputNs, 0)
	}
}

func (a *App) notePeerControlInput(forceWake bool) (activated bool, allowed bool) {
	a.mu.Lock()
	if !a.peerControlActive {
		if !forceWake && a.peerControlRequiresWake {
			a.mu.Unlock()
			return false, false
		}
		a.peerControlActive = true
		a.peerControlRequiresWake = false
		a.switchBackSent = false
		activated = true
	}
	allowed = true
	a.mu.Unlock()
	if !forceWake {
		// The caller is about to inject input on this (controlled) machine.
		atomic.StoreUint32(&a.injectedInputN, 1)
	}
	atomic.StoreInt64(&a.lastRemoteInputNs, time.Now().UnixNano())
	return activated, allowed
}

func (a *App) pausePeerControlUntilWake() {
	a.releaseInjectedRemoteKeys()
	atomic.StoreInt64(&a.lastRemoteInputNs, 0)
	a.mu.Lock()
	a.peerControlActive = false
	a.peerControlRequiresWake = true
	a.switchBackSent = false
	a.mu.Unlock()
}

func (a *App) releaseInjectedRemoteKeys() {
	a.remoteInputMu.Lock()
	defer a.remoteInputMu.Unlock()
	a.releaseInjectedRemoteKeysLocked()
}

// releaseInjectedRemoteKeysLocked performs the actual release.
// Caller must hold remoteInputMu.
func (a *App) releaseInjectedRemoteKeysLocked() {
	a.mu.Lock()
	a.remoteKeyDeadline = time.Time{}
	if a.remoteKeyTimer != nil {
		a.remoteKeyTimer.Stop()
	}
	keys := a.remoteKeyState.ReleaseAll()
	var pressedButtons [3]bool
	for i := range a.remoteMouseButtons {
		if a.remoteMouseButtons[i] {
			pressedButtons[i] = true
			a.remoteMouseButtons[i] = false
		}
	}
	a.mu.Unlock()

	for _, key := range keys {
		InjectKey(uint16(key.VKCode), uint16(key.ScanCode), key.Flags, false)
	}
	for i, pressed := range pressedButtons {
		if pressed {
			InjectMouseClick(byte(i), false)
		}
	}
	// Release all modifier keys to prevent stuck modifiers that can occur
	// when the remote side releases a key after the session is already torn
	// down (or the key-up is lost in transit). Only done where remote input
	// was actually injected (the controlled side): on the controller these
	// synthetic key-ups would fight the user's physical keyboard.
	if atomic.SwapUint32(&a.injectedInputN, 0) != 0 {
		ReleaseAllModifiers()
	}
	// Clear the fast-path flag so handleRemoteMouseMove skips touchRemoteKeyWatchdog.
	atomic.StoreUint64(&a.remoteInputActiveN, 0)
}

func (a *App) touchRemoteKeyWatchdog() {
	// Called right after injecting remote input.
	atomic.StoreUint32(&a.injectedInputN, 1)
	// Stamp every inbound control input for the controlled-mode inactivity watchdog.
	atomic.StoreInt64(&a.lastRemoteInputNs, time.Now().UnixNano())
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.remoteKeyState.HasPressed() && !anyRemoteMouseButtonHeld(a.remoteMouseButtons) {
		a.remoteKeyDeadline = time.Time{}
		if a.remoteKeyTimer != nil {
			a.remoteKeyTimer.Stop()
		}
		atomic.StoreUint64(&a.remoteInputActiveN, 0)
		return
	}
	atomic.StoreUint64(&a.remoteInputActiveN, 1)
	a.remoteKeyDeadline = time.Now().Add(remoteKeyIdleTimeout)
	if a.remoteKeyTimer == nil {
		a.remoteKeyTimer = time.AfterFunc(remoteKeyIdleTimeout, a.handleRemoteKeyWatchdog)
		return
	}
	a.remoteKeyTimer.Stop()
	a.remoteKeyTimer.Reset(remoteKeyIdleTimeout)
}

func anyRemoteMouseButtonHeld(buttons [3]bool) bool {
	return buttons[0] || buttons[1] || buttons[2]
}

func (a *App) handleRemoteKeyWatchdog() {
	a.mu.Lock()
	deadline := a.remoteKeyDeadline
	hasPressed := a.remoteKeyState.HasPressed() || anyRemoteMouseButtonHeld(a.remoteMouseButtons)
	if !hasPressed || deadline.IsZero() {
		a.mu.Unlock()
		return
	}
	remaining := time.Until(deadline)
	if remaining > 0 {
		if a.remoteKeyTimer != nil {
			a.remoteKeyTimer.Reset(remaining)
		}
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	// Acquire remoteInputMu before releasing to serialize against inbound handlers,
	// then re-verify the deadline is still expired to close the race where a fresh
	// inbound event refreshes the deadline between the check above and here.
	a.remoteInputMu.Lock()
	a.mu.Lock()
	if !a.remoteKeyDeadline.IsZero() && time.Until(a.remoteKeyDeadline) > 0 {
		remaining = time.Until(a.remoteKeyDeadline)
		if a.remoteKeyTimer != nil {
			a.remoteKeyTimer.Reset(remaining)
		}
		a.mu.Unlock()
		a.remoteInputMu.Unlock()
		return
	}
	a.mu.Unlock()

	log.Printf("remote key watchdog: releasing stale injected keys after %v idle", remoteKeyIdleTimeout)
	a.releaseInjectedRemoteKeysLocked()
	a.remoteInputMu.Unlock()
}

func (a *App) maybeResetSwitchBackSent() {
	a.mu.RLock()
	side := a.controllerEdgeSide
	sent := a.switchBackSent
	a.mu.RUnlock()
	if side == "" || !sent {
		return
	}

	x, y := GetCursorPosition()
	left, right, top, bottom := GetScreenBounds()
	if isAtReturnEdge(side, x, y, left, right, top, bottom) {
		return
	}

	a.mu.Lock()
	a.switchBackSent = false
	a.mu.Unlock()
}
