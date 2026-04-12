//go:build windows

package sysutil

import (
	"log"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32DLL               = syscall.NewLazyDLL("user32.dll")
	pRegisterClassExW       = user32DLL.NewProc("RegisterClassExW")
	pCreateWindowExW        = user32DLL.NewProc("CreateWindowExW")
	pDefWindowProcW         = user32DLL.NewProc("DefWindowProcW")
	pGetMessageW            = user32DLL.NewProc("GetMessageW")
	pTranslateMessage       = user32DLL.NewProc("TranslateMessage")
	pDispatchMessageW       = user32DLL.NewProc("DispatchMessageW")
	pDestroyWindow          = user32DLL.NewProc("DestroyWindow")
	pPostQuitMessage        = user32DLL.NewProc("PostQuitMessage")
	pPostThreadMessageW_pwr = user32DLL.NewProc("PostThreadMessageW")
	kernel32DLL             = syscall.NewLazyDLL("kernel32.dll")
	pGetModuleHandleW       = kernel32DLL.NewProc("GetModuleHandleW")
	pGetCurrentThreadId_pwr = kernel32DLL.NewProc("GetCurrentThreadId")
)

const (
	wmPowerBroadcast  = 0x0218
	wmQuitMsg         = 0x0012
	pbmAPMSuspend     = 0x0004
	pbmAPMResumeAuto  = 0x0012
	pbmAPMResumeSusp  = 0x0007
	csHRedraw         = 0x0002
	csVRedraw         = 0x0001
	hwndMessage       = ^uintptr(2) // HWND_MESSAGE = (HWND)-3
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

type msgStruct struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// PowerCallback is called when the system suspends or resumes.
type PowerCallback func(event string)

var globalPowerCallback PowerCallback

func powerWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if msg == wmPowerBroadcast {
		switch wParam {
		case pbmAPMSuspend:
			if globalPowerCallback != nil {
				globalPowerCallback("suspend")
			}
		case pbmAPMResumeAuto, pbmAPMResumeSusp:
			if globalPowerCallback != nil {
				globalPowerCallback("resume")
			}
		}
	}
	ret, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

// PowerWatcher receives suspend/resume notifications via a hidden
// message-only window.  Call Stop() or cancel the returned channel to
// tear it down.
type PowerWatcher struct {
	threadID uint32
	done     chan struct{}
}

// WatchPowerEvents starts listening for suspend/resume OS events.
// The callback is invoked with "suspend" or "resume".  Must be stopped
// by calling Stop().
func WatchPowerEvents(cb PowerCallback) *PowerWatcher {
	globalPowerCallback = cb
	pw := &PowerWatcher{done: make(chan struct{})}
	go pw.run()
	return pw
}

func (pw *PowerWatcher) run() {
	defer close(pw.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := pGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("MultiSnekKVMPower")

	wc := wndClassExW{
		Style:         csHRedraw | csVRedraw,
		LpfnWndProc:   syscall.NewCallback(powerWndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0,
		hwndMessage, 0, hInst, 0,
	)
	if hwnd == 0 {
		log.Println("power watcher: failed to create message window")
		return
	}

	tid, _, _ := pGetCurrentThreadId_pwr.Call()
	pw.threadID = uint32(tid)

	var msg msgStruct
	for {
		ret, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	pDestroyWindow.Call(hwnd)
}

// Stop shuts down the power event listener.
func (pw *PowerWatcher) Stop() {
	if pw.threadID != 0 {
		pPostThreadMessageW_pwr.Call(uintptr(pw.threadID), wmQuitMsg, 0, 0)
	}
	<-pw.done
	globalPowerCallback = nil
}
