package app

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) GetEdgeSide() string {
	if a.inputHook == nil {
		return ""
	}
	return a.inputHook.GetEdgeSide()
}

func (a *App) SetEdgeSide(side string) {
	if a.inputHook == nil {
		return
	}
	a.inputHook.SetEdgeSide(side)
	a.settings.Update(func(s *Settings) { s.EdgeSide = side })
	a.sendEdgeConfig()
}

func (a *App) GetSensitivity() float64 {
	if a.inputHook == nil {
		return 1.0
	}
	return a.inputHook.GetSensitivity()
}

func (a *App) SetSensitivity(s float64) {
	if a.inputHook == nil {
		return
	}
	a.inputHook.SetSensitivity(s)
	a.settings.Update(func(cfg *Settings) { cfg.Sensitivity = s })
}

func (a *App) GetAutostart() bool {
	return GetAutostart()
}

func (a *App) SetAutostart(enabled bool) error {
	return SetAutostart(enabled)
}

func (a *App) checkReturnEdge() {
	a.mu.RLock()
	side := a.controllerEdgeSide
	active := a.peerControlActive
	a.mu.RUnlock()

	if side == "" || !active {
		return
	}

	x, y := GetCursorPosition()
	left, right, top, bottom := GetScreenBounds()
	if !isAtReturnEdge(side, x, y, left, right, top, bottom) {
		return
	}

	log.Println("cursor at return edge, sending switch back")
	a.releaseInjectedRemoteKeys()
	a.enqueueSend(Frame{Type: MsgSwitchBack})
	a.pausePeerControlUntilWake()
}

func oppositeEdge(side string) string {
	switch side {
	case "right":
		return "left"
	case "left":
		return "right"
	case "top":
		return "bottom"
	case "bottom":
		return "top"
	}
	return ""
}

func isAtReturnEdge(side string, x, y, left, right, top, bottom int32) bool {
	switch side {
	case "right":
		return x <= left
	case "left":
		return x >= right
	case "top":
		return y >= bottom
	case "bottom":
		return y <= top
	default:
		return false
	}
}

// GetLocalMonitors returns the list of monitors connected to this machine.
func (a *App) GetLocalMonitors() []MonitorInfo {
	return EnumLocalMonitors()
}

// SetExitHotkey configures the key combination that exits remote control mode.
// modifiers bitmask: 1=Ctrl, 2=Alt, 4=Shift, 8=Win. vkCode 0 means use ESC only.
// ESC always works as a fallback regardless of the configured hotkey.
func (a *App) SetExitHotkey(modifiers int, vkCode int) {
	if a.inputHook == nil {
		return
	}
	mod := uint8(modifiers)
	vk := uint16(vkCode)
	a.inputHook.SetExitHotkey(mod, vk)
	a.settings.Update(func(s *Settings) {
		s.ExitModifiers = mod
		s.ExitVKCode = vk
	})
}

// GetExitHotkey returns the current exit hotkey configuration.
func (a *App) GetExitHotkey() map[string]int {
	if a.inputHook == nil {
		return map[string]int{"modifiers": 0, "vkCode": 0}
	}
	mod, vk := a.inputHook.GetExitHotkey()
	return map[string]int{"modifiers": int(mod), "vkCode": int(vk)}
}

// SetTriggerZone configures which monitor edge segment activates remote mode.
// monitorID is the monitor ID from GetLocalMonitors. side is "left"/"right"/"top"/"bottom".
// startPct and endPct (0.0–1.0) define the active segment along the edge.
func (a *App) SetTriggerZone(monitorID, side string, startPct, endPct float64) {
	if a.inputHook == nil {
		return
	}
	monitors := EnumLocalMonitors()
	for _, mon := range monitors {
		if mon.ID == monitorID {
			a.inputHook.SetTriggerZone(mon, side, float32(startPct), float32(endPct))
			a.inputHook.SetEdgeSide(side)
			a.settings.Update(func(s *Settings) {
				s.TriggerMonitorID = monitorID
				s.TriggerSide = side
				s.TriggerStartPct = float32(startPct)
				s.TriggerEndPct = float32(endPct)
				s.EdgeSide = side
			})
			a.sendEdgeConfig()
			return
		}
	}
	log.Printf("SetTriggerZone: monitor %q not found", monitorID)
}

// ClearTriggerZone reverts to legacy full-edge detection.
func (a *App) ClearTriggerZone() {
	if a.inputHook == nil {
		return
	}
	a.inputHook.ClearTriggerZone()
	a.settings.Update(func(s *Settings) {
		s.TriggerMonitorID = ""
		s.TriggerSide = ""
		s.TriggerStartPct = 0
		s.TriggerEndPct = 0
	})
}

// SetReturnAnchor configures where the cursor warps after exiting remote mode.
// xPct and yPct (0.0–1.0) are relative to the monitor's top-left corner.
func (a *App) SetReturnAnchor(monitorID string, xPct, yPct float64) {
	if a.inputHook == nil {
		return
	}
	monitors := EnumLocalMonitors()
	for _, mon := range monitors {
		if mon.ID == monitorID {
			a.inputHook.SetReturnAnchor(mon, float32(xPct), float32(yPct))
			a.settings.Update(func(s *Settings) {
				s.ReturnMonitorID = monitorID
				s.ReturnXPct = float32(xPct)
				s.ReturnYPct = float32(yPct)
			})
			return
		}
	}
	log.Printf("SetReturnAnchor: monitor %q not found", monitorID)
}

// GetTriggerZone returns the current trigger zone configuration.
// Returns empty strings/zero values when no zone is configured.
func (a *App) GetTriggerZone() map[string]interface{} {
	cfg := a.settings.Get()
	return map[string]interface{}{
		"monitorID": cfg.TriggerMonitorID,
		"side":      cfg.TriggerSide,
		"startPct":  float64(cfg.TriggerStartPct),
		"endPct":    float64(cfg.TriggerEndPct),
	}
}

// GetReturnAnchor returns the current return anchor configuration.
// Returns empty monitorID when no anchor is configured.
func (a *App) GetReturnAnchor() map[string]interface{} {
	cfg := a.settings.Get()
	return map[string]interface{}{
		"monitorID": cfg.ReturnMonitorID,
		"xPct":      float64(cfg.ReturnXPct),
		"yPct":      float64(cfg.ReturnYPct),
	}
}

// ClearReturnAnchor reverts to legacy center-of-primary-monitor return position.
func (a *App) ClearReturnAnchor() {
	if a.inputHook == nil {
		return
	}
	a.inputHook.ClearReturnAnchor()
	a.settings.Update(func(s *Settings) {
		s.ReturnMonitorID = ""
		s.ReturnXPct = 0
		s.ReturnYPct = 0
	})
}

// SaveReceivedFilesResult is the result of a SaveReceivedFiles call.
type SaveReceivedFilesResult struct {
	Saved []string `json:"saved"`
	Dest  string   `json:"dest"`
}

// SaveReceivedFiles opens a folder picker, then copies files from tempDir to the chosen folder.
// The clipboard is updated to point at the new locations. Returns an error string on failure.
func (a *App) SaveReceivedFiles(tempDir string) (SaveReceivedFilesResult, error) {
	dest, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Save received files to…",
	})
	if err != nil || dest == "" {
		return SaveReceivedFilesResult{}, nil // user cancelled
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return SaveReceivedFilesResult{}, fmt.Errorf("cannot read temp dir: %w", err)
	}

	var saved []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(tempDir, e.Name())
		dst, err := uniqueDest(dest, e.Name())
		if err != nil {
			return SaveReceivedFilesResult{}, err
		}
		if err := copyFile(src, dst); err != nil {
			return SaveReceivedFilesResult{}, fmt.Errorf("copy %s: %w", e.Name(), err)
		}
		saved = append(saved, dst)
	}

	if len(saved) > 0 {
		setClipboardFilesFn(saved)
	}

	// Clean up the temp directory now that files are safely in destination.
	_ = os.RemoveAll(tempDir)
	a.removeRecvDir(tempDir)

	return SaveReceivedFilesResult{Saved: saved, Dest: dest}, nil
}

// DiscardReceivedFiles removes a received-files temp directory without saving.
// Called when the user dismisses a file transfer toast.
func (a *App) DiscardReceivedFiles(tempDir string) {
	if tempDir == "" {
		return
	}
	a.removeRecvDir(tempDir)
	if err := os.RemoveAll(tempDir); err != nil {
		log.Printf("discard received files: %v", err)
	}
}

// removeRecvDir removes tempDir from the pending tracking list.
func (a *App) removeRecvDir(tempDir string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, d := range a.pendingRecvDirs {
		if d == tempDir {
			a.pendingRecvDirs = append(a.pendingRecvDirs[:i], a.pendingRecvDirs[i+1:]...)
			return
		}
	}
}

// uniqueDest returns a non-colliding destination path, appending " (N)" before the extension if needed.
// Returns an error if no free path can be found after 999 attempts.
func uniqueDest(dir, name string) (string, error) {
	dst := filepath.Join(dir, name)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return dst, nil
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("too many copies of %q in destination", name)
}

func copyFile(src, dst string) (retErr error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		out.Close()
		if retErr != nil {
			// Remove the partial destination file so retries use a clean slate.
			_ = os.Remove(dst)
		}
	}()
	_, retErr = io.Copy(out, in)
	return retErr
}
