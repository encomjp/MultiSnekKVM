package app

import (
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/energye/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"multisnekkvm/internal/logutil"
)

var (
	_kernel32        = syscall.NewLazyDLL("kernel32.dll")
	_setWorkingSet   = _kernel32.NewProc("SetProcessWorkingSetSize")
	embeddedTrayIcon []byte
	embeddedAppIcon  []byte
)

func trimWorkingSet() {
	h, _ := syscall.GetCurrentProcess()
	// Passing ^uintptr(0) for both min and max tells Windows to trim to minimum.
	_setWorkingSet.Call(uintptr(h), ^uintptr(0), ^uintptr(0))
}

func SetEmbeddedIcons(trayIcon, appIcon []byte) {
	embeddedTrayIcon = append([]byte(nil), trayIcon...)
	embeddedAppIcon = append([]byte(nil), appIcon...)
}

// TrayManager manages the system tray icon and menu.
type TrayManager struct {
	app       *App
	iconBytes []byte
	visible   bool
	mStatus   *systray.MenuItem // kept for UpdateStatus calls from emitUpdates
}

func NewTrayManager(app *App) *TrayManager {
	return &TrayManager{app: app, visible: true}
}

// Start sets up the system tray. Must be called after Wails startup.
func (t *TrayManager) Start() {
	// Prefer compile-time embedded icon so tray icon works in packaged builds.
	ico := embeddedTrayIcon
	if len(ico) == 0 {
		// Development/runtime fallback from disk.
		var err error
		ico, err = os.ReadFile(trayIconPath())
		if err != nil {
			log.Printf("tray: cannot load icon from disk: %v", err)
		}
	}
	if len(ico) == 0 {
		// Last fallback for platforms that accept PNG icon bytes.
		ico = embeddedAppIcon
	}
	if len(ico) == 0 {
		// Absolute fallback to avoid empty icon in tray.
		ico = minimalICO()
	}
	t.iconBytes = ico

	// The systray message loop uses Windows GetMessageW which has thread
	// affinity: the window must be created and serviced on the same OS thread.
	// LockOSThread pins this goroutine so Go's scheduler never migrates it,
	// preventing the tray from silently losing all its messages.
	go func() {
		runtime.LockOSThread()
		systray.Run(t.onReady, t.onExit)
	}()
}

func (t *TrayManager) onReady() {
	systray.SetIcon(t.iconBytes)
	systray.SetTitle("Multisnek")
	systray.SetTooltip("Multisnek - KVM over Network")

	mShow := systray.AddMenuItem("Show Multisnek", "Show the main window")
	systray.AddSeparator()
	t.mStatus = systray.AddMenuItem("Status: Ready", "Connection status")
	t.mStatus.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit Multisnek")

	mShow.Click(func() { logutil.SafeGo("tray-show", func() { t.ShowWindow() }) })
	mQuit.Click(func() {
		logutil.SafeGo("tray-quit", func() {
			t.app.mu.Lock()
			t.app.quitRequested = true
			t.app.mu.Unlock()
			wailsRuntime.Quit(t.app.ctx)
		})
	})
	// Tray status is now driven by emitUpdates via UpdateStatus — no polling goroutine needed.
}

func (t *TrayManager) onExit() {
	// Called when systray loop exits
}

func (t *TrayManager) ShowWindow() {
	t.visible = true
	wailsRuntime.WindowShow(t.app.ctx)
	wailsRuntime.WindowSetAlwaysOnTop(t.app.ctx, true)
	wailsRuntime.WindowSetAlwaysOnTop(t.app.ctx, false)
	// Restore normal GC aggressiveness when window is visible.
	debug.SetGCPercent(50)
}

func (t *TrayManager) HideWindow() {
	t.visible = false
	wailsRuntime.WindowHide(t.app.ctx)
	log.Println("tray: window hidden, reclaiming memory")
	// Reclaim memory now that the UI is hidden.
	go func() {
		time.Sleep(500 * time.Millisecond)
		debug.SetGCPercent(30) // moderately aggressive GC while tray-only (not too aggressive for audio)
		debug.FreeOSMemory()   // force GC + return all free memory to OS
		trimWorkingSet()       // trim Windows working set (pages move to page file)
	}()
}

// UpdateStatus updates the tray menu connection status item.
// Called from emitUpdates whenever the session state changes.
func (t *TrayManager) UpdateStatus(connected bool, peerName string) {
	if t.mStatus == nil {
		return
	}
	title := "Status: Ready"
	if connected {
		title = "Connected to " + peerName
	}
	t.mStatus.SetTitle(title)
}

func (t *TrayManager) IsVisible() bool {
	return t.visible
}

func trayIconPath() string {
	// Try next to the executable first, then the build directory
	exe, err := os.Executable()
	if err == nil {
		dir := exeDir(exe)
		candidates := []string{
			dir + "/icon.ico",
			dir + "/appicon.png",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	// Development fallback
	return "build/windows/icon.ico"
}

func exeDir(exe string) string {
	for i := len(exe) - 1; i >= 0; i-- {
		if exe[i] == '/' || exe[i] == '\\' {
			return exe[:i]
		}
	}
	return "."
}

// minimalICO returns a valid 16x16 1-bit ICO file as fallback.
func minimalICO() []byte {
	// 16x16 monochrome ICO with a simple pattern
	return []byte{
		0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10,
		0x00, 0x00, 0x01, 0x00, 0x20, 0x00, 0x68, 0x04,
		0x00, 0x00, 0x16, 0x00, 0x00, 0x00,
	}
}
