package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"multisnekkvm/internal/logutil"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) BeforeClose(_ context.Context) bool {
	a.mu.RLock()
	quit := a.quitRequested
	a.mu.RUnlock()
	if quit {
		return false
	}
	if a.tray != nil {
		a.tray.HideWindow()
	}
	return true
}

func (a *App) Shutdown(_ context.Context) {
	logutil.LogKV("app.shutdown.begin",
		"connected", a.transport != nil && a.transport.GetSession() != nil,
		"playing_audio", a.audio != nil && a.audio.IsPlaying(),
		"capturing_audio", a.audio != nil && a.audio.IsCapturing(),
	)
	if a.power != nil {
		a.power.Stop()
	}
	if a.audio != nil {
		a.audio.Close()
	}
	a.releaseInjectedRemoteKeys()
	if a.inputHook != nil {
		a.inputHook.SetConnected(false, nil)
	}
	if a.transport != nil {
		a.transport.Stop()
	}
	if a.cancel != nil {
		a.cancel()
	}
	// Clean up any receive temp dirs the user didn't save or discard.
	a.mu.Lock()
	dirs := a.pendingRecvDirs
	a.pendingRecvDirs = nil
	a.mu.Unlock()
	for _, dir := range dirs {
		_ = os.RemoveAll(dir)
	}
	logutil.LogKV("app.shutdown.complete")
}

func (a *App) emitUpdates() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			if a.syncPairingCode() {
				wailsRuntime.EventsEmit(a.ctx, "device-updated", a.deviceSnapshot())
			}
			sess := a.GetSession()
			wailsRuntime.EventsEmit(a.ctx, "peers-updated", a.GetPeers())
			wailsRuntime.EventsEmit(a.ctx, "session-updated", sess)
			wailsRuntime.EventsEmit(a.ctx, "tailscale-updated", a.GetTailscaleStatus())
			if a.tray != nil {
				a.tray.UpdateStatus(sess.Connected, sess.PeerName)
			}
		}
	}
}

func (a *App) clipboardSync() {
	lastOversizedClipboard := false
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			s := a.transport.GetSession()
			if s == nil {
				continue
			}
			text, tooLarge := GetClipboardTextForSync()
			if tooLarge {
				if !lastOversizedClipboard {
					log.Printf("clipboard sync: skipping oversized local clipboard text (max %d UTF-8 bytes / %d UTF-16 bytes)", maxClipboardTextBytes, maxClipboardUTF16Bytes)
				}
				lastOversizedClipboard = true
				continue
			}
			lastOversizedClipboard = false

			a.mu.Lock()
			msg, shouldSend, droppedTooLarge := a.clipboardState.PrepareOutboundClipboard(text)
			a.mu.Unlock()
			if droppedTooLarge {
				log.Printf("clipboard sync: dropping local clipboard text larger than %d bytes", maxClipboardTextBytes)
				continue
			}
			if !shouldSend {
				continue
			}

			a.enqueueSend(Frame{
				Type:    MsgClipboard,
				Payload: msg.Encode(),
			})
		}
	}
}

// secureDesktopMonitor exits remote-controlled mode when a UAC / Secure Desktop
// prompt is detected.  GetForegroundWindow() returns NULL from non-elevated
// processes while the Secure Desktop owns the input focus.  Two consecutive
// NULL results (≈ 200 ms apart) are required to avoid false positives during
// normal window activation transitions.
func (a *App) secureDesktopMonitor(_ context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	nullCount := 0
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}

		a.mu.RLock()
		controlled := a.peerControlActive
		a.mu.RUnlock()
		if !controlled {
			nullCount = 0
			continue
		}

		if IsSecureDesktopActive() {
			nullCount++
			if nullCount >= 2 {
				log.Println("secure desktop detected (UAC) — switching back to local input")
				nullCount = 0
				a.enqueueSend(Frame{Type: MsgSwitchBack})
				a.pausePeerControlUntilWake()
			}
		} else {
			nullCount = 0
		}
	}
}

// controlledModeWatchdog auto-releases controlled state when no inbound control
// input has been received for remoteControlIdleTimeout. This handles cases where
// the transport heartbeat keeps the connection alive but the controller has gone
// idle (e.g., locked screen on the controller machine, OLE hang, or silent peer).
const remoteControlIdleTimeout = 30 * time.Second

func (a *App) controlledModeWatchdog(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		a.mu.RLock()
		controlled := a.peerControlActive
		a.mu.RUnlock()
		if !controlled {
			continue
		}

		lastNs := atomic.LoadInt64(&a.lastRemoteInputNs)
		if lastNs == 0 {
			continue
		}
		idle := time.Since(time.Unix(0, lastNs))
		if idle < remoteControlIdleTimeout {
			continue
		}

		log.Printf("controlled-mode watchdog: no remote input for %.0fs, auto-releasing controlled state", idle.Seconds())
		a.pausePeerControlUntilWake()
		wailsRuntime.EventsEmit(a.ctx, "session-updated", a.GetSession())
	}
}

// GetLoadMetrics returns a snapshot of internal queue depths and cumulative
// counters useful for diagnosing lag or stalls. Called from frontend diagnostics.
func (a *App) GetLoadMetrics() map[string]int64 {
	return map[string]int64{
		"sendQueueHigh":     int64(len(a.muxHigh)),
		"sendQueueMouse":    int64(len(a.muxMouse)),
		"sendQueueFile":     int64(len(a.muxFile)),
		"muxHighSent":       int64(atomic.LoadUint64(&a.muxHighSentN)),
		"muxMouseSent":      int64(atomic.LoadUint64(&a.muxMouseSentN)),
		"muxMouseCoalesced": int64(atomic.LoadUint64(&a.muxMouseCoalescedN)),
		"muxHighDropped":    int64(atomic.LoadUint64(&a.muxHighDroppedN)),
		"recvMouseMove":     int64(atomic.LoadUint64(&a.recvMouseMoveN)),
		"recvKey":           int64(atomic.LoadUint64(&a.recvKeyN)),
		"recvAudio":         int64(atomic.LoadUint64(&a.recvAudioN)),
		"fileQueueDepth":    int64(len(a.fileInboundCh)),
		"audioQueueDepth":   int64(len(a.audioInboundCh)),
		"goroutines":        int64(runtime.NumGoroutine()),
	}
}

// logHealthWatchdog periodically analyzes the in-memory log ring for anomaly
// patterns (panics, stalls, repeated errors) and logs a compact summary.
// It emits a health-alert event when critical patterns are found.
// All watchdog-emitted lines are prefixed with [log-watchdog] so that the
// log analyzer itself ignores them on the next pass (prevents self-amplification).
func (a *App) logHealthWatchdog(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			analysis := logutil.AnalyzeRecentLogs()
			if len(analysis.Events) == 0 {
				continue
			}
			for _, ev := range analysis.Events {
				if ev.Level == "critical" || ev.Level == "error" {
					log.Printf("[log-watchdog] pattern=%s level=%s count=%d sample=%q",
						ev.Pattern, ev.Level, ev.Count, trimSample(ev.Sample, 120))
				}
			}
			if analysis.TotalErrors > 10 {
				log.Printf("[log-watchdog] elevated error rate: %d errors in last %d log lines",
					analysis.TotalErrors, analysis.WindowLines)
				wailsRuntime.EventsEmit(ctx, "health-alert", RemediationAlert{
					Subsystem: "log-analysis",
					Message:   fmt.Sprintf("%d errors detected in recent logs — check diagnostics", analysis.TotalErrors),
				})
			}
		}
	}
}

func trimSample(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
