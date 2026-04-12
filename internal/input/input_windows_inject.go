//go:build windows

package input

import "unsafe"

// modifierVKCodes lists all Windows virtual key codes for modifier keys that
// can become stuck when a session transitions mid-keypress.
var modifierVKCodes = []uint16{
	0xA0, // VK_LSHIFT
	0xA1, // VK_RSHIFT
	0xA2, // VK_LCONTROL
	0xA3, // VK_RCONTROL
	0xA4, // VK_LMENU  (Left Alt)
	0xA5, // VK_RMENU  (Right Alt / AltGr)
	0x5B, // VK_LWIN
	0x5C, // VK_RWIN
}

// ReleaseAllModifiers sends a key-up event for every modifier key.  This
// prevents "stuck key" syndrome when a session starts or stops while a
// modifier is physically held on the remote side.
func ReleaseAllModifiers() {
	for _, vk := range modifierVKCodes {
		var kbFlags uint32 = keyEventFUp
		// Right-hand modifiers (RSHIFT, RCONTROL, RMENU, RWIN) are extended.
		if vk == 0xA1 || vk == 0xA3 || vk == 0xA5 || vk == 0x5C {
			kbFlags |= keyEventFExtended
		}
		input := keybdInput{
			Type: inputKeyboard,
			Ki: keybdInputData{
				Wvk:   vk,
				Flags: kbFlags,
			},
		}
		pSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	}
}

func InjectMouseMove(dx, dy int32) {
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	newX := pt.X + dx
	newY := pt.Y + dy

	// Normalize to the 0-65535 range required by MOUSEEVENTF_ABSOLUTE.
	// Using MOUSEEVENTF_VIRTUALDESK maps coordinates across all monitors.
	vx, _, _ := pGetSystemMetrics.Call(smXVirtualScreen)
	vy, _, _ := pGetSystemMetrics.Call(smYVirtualScreen)
	vw, _, _ := pGetSystemMetrics.Call(smCxVirtualScreen)
	vh, _, _ := pGetSystemMetrics.Call(smCyVirtualScreen)
	virtLeft, virtTop := int32(vx), int32(vy)
	virtWidth, virtHeight := int32(vw), int32(vh)

	var normX, normY int32
	if virtWidth > 1 {
		normX = (newX-virtLeft)*65535 / (virtWidth - 1)
	}
	if virtHeight > 1 {
		normY = (newY-virtTop)*65535 / (virtHeight - 1)
	}

	// SendInput with MOUSEEVENTF_ABSOLUTE injects a real synthetic hardware
	// event into the input stream. Unlike SetCursorPos, this triggers
	// WM_MOUSEMOVE and WM_SETCURSOR in the foreground window, which causes
	// apps that hide the cursor on idle to show it again — the same behaviour
	// as a physical touchpad or mouse movement.
	input := mouseInput{
		Type: inputMouse,
		Mi: mouseInputData{
			Dx:    normX,
			Dy:    normY,
			Flags: mousefMove | mousefAbsolute | mousefVirtualDesk,
		},
	}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func InjectMouseClick(button byte, pressed bool) {
	var flags uint32
	switch button {
	case 0:
		if pressed {
			flags = mousefLeftDown
		} else {
			flags = mousefLeftUp
		}
	case 1:
		if pressed {
			flags = mousefRightDown
		} else {
			flags = mousefRightUp
		}
	case 2:
		if pressed {
			flags = mousefMiddleDown
		} else {
			flags = mousefMiddleUp
		}
	}
	input := mouseInput{
		Type: inputMouse,
		Mi:   mouseInputData{Flags: flags},
	}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func InjectMouseScroll(delta int32) {
	input := mouseInput{
		Type: inputMouse,
		Mi: mouseInputData{
			MouseData: uint32(delta),
			Flags:     mousefWheel,
		},
	}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func InjectKey(vkCode uint16, scanCode uint16, flags uint32, down bool) {
	var kbFlags uint32
	if !down {
		kbFlags |= keyEventFUp
	}
	if flags&0x01 != 0 {
		kbFlags |= keyEventFExtended
	}
	input := keybdInput{
		Type: inputKeyboard,
		Ki: keybdInputData{
			Wvk:   vkCode,
			WScan: scanCode,
			Flags: kbFlags,
		},
	}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

// InjectUnicode injects a Unicode character as a keydown+keyup pair using
// KEYEVENTF_UNICODE, bypassing the host keyboard layout mapping.
// This is used for layout-independent text input from a remote controller.
func InjectUnicode(char uint32) {
	if char > 0xFFFF {
		// Supplementary plane — surrogate pair required; skip for now.
		return
	}
	scan := uint16(char)
	down := keybdInput{
		Type: inputKeyboard,
		Ki:   keybdInputData{WScan: scan, Flags: keyEventFUnicode},
	}
	up := keybdInput{
		Type: inputKeyboard,
		Ki:   keybdInputData{WScan: scan, Flags: keyEventFUnicode | keyEventFUp},
	}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&down)), unsafe.Sizeof(down))
	pSendInput.Call(1, uintptr(unsafe.Pointer(&up)), unsafe.Sizeof(up))
}
