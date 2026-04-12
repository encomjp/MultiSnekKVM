package app

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"multisnekkvm/internal/logutil"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) Startup(ctx context.Context) {
	logutil.LogKV("app.startup.begin",
		"requested_name", a.device.Name,
		"port", a.device.Port,
		"audio_mode", a.audioMode,
		"audio_timing", a.audioTiming,
		"audio_transport", a.audioTransportMode,
		"audio_profile", a.audioProfile,
		"mic_mode", a.micMode,
	)

	a.ctx, a.cancel = context.WithCancel(ctx)
	a.initRealtimeAudioQueue()
	a.initSendMux()
	a.initInboundDispatch()

	identity, err := LoadOrCreateIdentity(a.device.Name)
	if err != nil {
		log.Printf("identity error: %v", err)
		a.device.ID = "unknown"
		a.device.Fingerprint = "unknown"
	} else {
		a.device.ID = identity.DeviceID
		a.device.Name = identity.Name
		a.device.Fingerprint = identity.Fingerprint
	}
	logutil.LogKV("app.identity.ready",
		"device_id", shortPeerID(a.device.ID),
		"device_name", a.device.Name,
		"fingerprint", shortPeerID(a.device.Fingerprint),
	)

	trustStore, err := OpenTrustStore()
	if err != nil {
		log.Printf("trust store error: %v", err)
	}
	a.trust = trustStore
	logutil.LogKV("app.trust.ready", "available", trustStore != nil)

	a.inputHook = NewInputHook()
	cfg := a.settings.Get()
	a.inputHook.SetEdgeSide(cfg.EdgeSide)
	a.inputHook.SetSensitivity(cfg.Sensitivity)
	if cfg.ExitVKCode != 0 {
		a.inputHook.SetExitHotkey(cfg.ExitModifiers, cfg.ExitVKCode)
	}
	// Apply saved trigger zone and return anchor if configured.
	if cfg.TriggerMonitorID != "" || cfg.ReturnMonitorID != "" {
		monitors := EnumLocalMonitors()
		if cfg.TriggerMonitorID != "" {
			for _, mon := range monitors {
				if mon.ID == cfg.TriggerMonitorID && cfg.TriggerSide != "" {
					sp := cfg.TriggerStartPct
					ep := cfg.TriggerEndPct
					if ep == 0 {
						ep = 1
					}
					a.inputHook.SetTriggerZone(mon, cfg.TriggerSide, sp, ep)
					break
				}
			}
		}
		if cfg.ReturnMonitorID != "" {
			for _, mon := range monitors {
				if mon.ID == cfg.ReturnMonitorID {
					a.inputHook.SetReturnAnchor(mon, cfg.ReturnXPct, cfg.ReturnYPct)
					break
				}
			}
		}
	}
	a.inputHook.SetOnStateChange(func() {
		wailsRuntime.EventsEmit(a.ctx, "session-updated", a.GetSession())
		a.handleControlStateChange()
	})
	a.inputHook.SetOnEdgeDrag(func() {
		a.handleEdgeDrag()
	})

	audioStreamer, err := NewAudioStreamer()
	if err != nil {
		log.Printf("audio init error: %v (audio disabled)", err)
	}
	a.audio = audioStreamer
	logutil.LogKV("app.audio.ready",
		"available", a.audio != nil,
		"capture_device", a.captureDeviceID,
		"playback_device", a.playbackDeviceID,
		"mic_device", a.micDeviceID,
		"mic_playback_device", a.micPlaybackDeviceID,
	)

	if a.audio != nil {
		if a.captureDeviceID != "" {
			a.audio.SetCaptureDeviceID(a.captureDeviceID)
		}
		if a.playbackDeviceID != "" {
			a.audio.SetPlaybackDeviceID(a.playbackDeviceID)
		}
		if a.micDeviceID != "" {
			a.audio.SetMicDeviceID(a.micDeviceID)
		}
		if a.micPlaybackDeviceID != "" {
			a.audio.SetMicPlaybackDeviceID(a.micPlaybackDeviceID)
		}
	}
	a.initAudioRuntime()

	a.fileTx = NewFileTransferManager()
	a.fileTx.SetOnComplete(func(tempDir string, names []string) {
		a.mu.Lock()
		a.pendingRecvDirs = append(a.pendingRecvDirs, tempDir)
		a.mu.Unlock()
		wailsRuntime.EventsEmit(a.ctx, "file-received", map[string]interface{}{
			"count":   len(names),
			"names":   names,
			"tempDir": tempDir,
		})
	})

	a.transport = NewTransport(a.device, identity, trustStore)
	a.transport.OnFrame = func(f Frame) {
		// File-transfer and audio frames are dispatched to dedicated goroutines to
		// prevent disk I/O and WASAPI decode from blocking the readLoop goroutine.
		// A blocked readLoop stops TCP reads, which causes backpressure to fill the
		// peer's send buffer, which delays heartbeats → 15 s timeout → disconnect.
		switch f.Type {
		// File control frames are protocol-critical — a dropped Offer/Accept/Done/Cancel
		// wedges the transfer permanently. Block readLoop briefly (these are rare) rather
		// than risk a silent drop.
		case MsgFileTransferOffer, MsgFileTransferAccept, MsgFileTransferDone, MsgFileTransferCancel:
			select {
			case a.fileInboundCh <- f:
			case <-a.ctx.Done():
			}
			return
		// File chunks can be dropped under pressure — the transfer will fail with a
		// protocol error and can be retried, which is safer than silently corrupting it.
		case MsgFileChunk:
			select {
			case a.fileInboundCh <- f:
			default:
				log.Printf("inbound-file: chunk queue full, dropping chunk (transfer will fail)")
			}
			return
		case MsgAudioData, MsgMicData:
			select {
			case a.audioInboundCh <- f:
			default:
				log.Printf("inbound-audio: queue full, dropping audio frame type=0x%02x", f.Type)
			}
			return
		}
		start := time.Now()
		a.handleFrame(f)
		if ms := time.Since(start).Milliseconds(); ms >= 5 && f.Type != MsgMouseMove {
			log.Printf("handleFrame: slow dispatch type=0x%02x took=%dms", f.Type, ms)
		}
	}
	SafeGo("recv-stats", func() { a.recvStatsLoop() })
	a.transport.OnConnect = func(peerID, peerName, role string) {
		a.drainSendMux()
		a.drainInboundChannels()
		a.bumpRealtimeSendGeneration()
		a.resetAudioPipelines()
		a.sendEdgeConfig()
		if role == "controller" {
			a.inputHook.SetConnected(true, func(f Frame) {
				a.enqueueSend(f)
			})
		}
		a.fileTx.SetSendFn(func(f Frame) { a.enqueueSend(f) })
		a.mu.Lock()
		a.latencyMs = -1
		a.latencyPrev = -1
		a.jitterMs = -1
		a.lastConnectTime = time.Now()
		a.sessionRole = role
		a.mu.Unlock()
		a.saveLastPeer(peerID, peerName)
		wailsRuntime.EventsEmit(a.ctx, "session-updated", a.GetSession())
		if role == "controller" {
			a.mu.RLock()
			mode := a.audioMode
			timing := a.audioTiming
			mic := a.micMode
			a.mu.RUnlock()
			if mode != "off" && timing == "always" {
				a.startAudioForMode(mode)
			}
			if mic != "off" && timing == "always" {
				a.startMicForMode(mic)
			}
		}
		wailsRuntime.EventsEmit(a.ctx, "peers-updated", a.GetPeers())
	}
	a.transport.OnDisconnect = func() {
		a.drainSendMux()
		a.drainInboundChannels()
		a.bumpRealtimeSendGeneration()
		a.stopAllAudio()
		a.stopAllMic()
		a.resetAudioPipelines()
		a.inputHook.SetConnected(false, nil)
		a.resetControlledState()
		a.fileTx.CancelAll()
		a.fileTx.SetSendFn(nil)
		a.mu.Lock()
		a.clipboardState.Reset()
		a.latencyMs = -1
		a.latencyPrev = -1
		a.jitterMs = -1
		a.sessionRole = ""
		peerAddr := a.lastPeerAddr
		reconnect := a.autoReconnect
		alreadyReconnecting := a.reconnecting
		connDuration := time.Since(a.lastConnectTime)
		a.mu.Unlock()
		wailsRuntime.EventsEmit(a.ctx, "session-updated", a.GetSession())
		wailsRuntime.EventsEmit(a.ctx, "peers-updated", a.GetPeers())
		if reconnect && peerAddr != "" && !alreadyReconnecting {
			if connDuration < 3*time.Second {
				log.Printf("auto-reconnect: skipped — connection lasted %v (likely rejected by peer)", connDuration)
			} else {
				SafeGo("auto-reconnect", func() {
					a.reconnectLoop(a.ctx, peerAddr)
				})
			}
		}
	}

	if err := a.transport.Start(a.device.Port); err != nil {
		log.Printf("transport error: %v", err)
	}
	logutil.LogKV("app.transport.ready", "listening", a.transport != nil && a.transport.IsListening(), "port", a.device.Port)

	a.tailscale = NewTailscaleService()
	a.discovery = NewDiscovery(a.device, 24830, a.tailscale)
	logutil.LogKV("app.discovery.ready", "broadcast_port", 24830, "tailscale_available", a.tailscale != nil)

	a.health = NewHealthMonitor()
	a.health.RegisterWithRemediation("transport", func() (bool, string) {
		if a.transport == nil || !a.transport.IsListening() {
			return false, "listener not running"
		}
		return true, "listening"
	}, 3, func() {
		log.Printf("[health-remediation] transport listener down for 3 consecutive checks — manual restart may be required")
		wailsRuntime.EventsEmit(a.ctx, "health-alert", RemediationAlert{
			Subsystem: "transport",
			Message:   "Transport listener is not running. Try restarting MultiSnekKVM.",
		})
	})
	a.health.Register("discovery", func() (bool, string) {
		if a.discovery == nil {
			return false, "not initialized"
		}
		count := len(a.discovery.Peers())
		return true, fmt.Sprintf("%d peers", count)
	})
	a.health.Register("tailscale", func() (bool, string) {
		if a.tailscale == nil {
			return true, "not configured"
		}
		st := a.tailscale.Status()
		if !st.Available {
			return true, "not installed"
		}
		if !st.Connected {
			return false, "disconnected"
		}
		return true, st.BackendState
	})
	a.health.Register("send-mux", func() (bool, string) {
		queuedAny := len(a.muxHigh) > 0 || len(a.muxMouse) > 0 || len(a.muxFile) > 0
		if !queuedAny {
			return true, "idle"
		}
		lastNs := atomic.LoadInt64(&a.muxLastSentNs)
		if lastNs == 0 {
			return true, "no data sent yet"
		}
		idleMs := time.Since(time.Unix(0, lastNs)).Milliseconds()
		if idleMs >= 1000 {
			return false, fmt.Sprintf("stalled %dms (H:%d M:%d F:%d)", idleMs, len(a.muxHigh), len(a.muxMouse), len(a.muxFile))
		}
		return true, fmt.Sprintf("active H:%d M:%d F:%d", len(a.muxHigh), len(a.muxMouse), len(a.muxFile))
	})
	a.health.Register("memory", func() (bool, string) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		heapMB := m.HeapAlloc / 1024 / 1024
		if heapMB > 512 {
			return false, fmt.Sprintf("heap %dMB (high — consider restarting)", heapMB)
		}
		return true, fmt.Sprintf("heap %dMB", heapMB)
	})

	SafeGoRestart(a.ctx, "tailscale", func(ctx context.Context) { a.tailscale.Run(ctx) })
	SafeGoRestart(a.ctx, "discovery", func(ctx context.Context) { a.discovery.Run(ctx) })
	SafeGoRestart(a.ctx, "emit-updates", func(ctx context.Context) { a.emitUpdates() })
	SafeGoRestart(a.ctx, "clipboard-sync", func(ctx context.Context) { a.clipboardSync() })
	SafeGoRestart(a.ctx, "latency-loop", func(ctx context.Context) { a.latencyLoop() })
	SafeGoRestart(a.ctx, "secure-desktop-monitor", func(ctx context.Context) { a.secureDesktopMonitor(ctx) })
	SafeGoRestart(a.ctx, "controlled-mode-watchdog", func(ctx context.Context) { a.controlledModeWatchdog(ctx) })
	SafeGoRestart(a.ctx, "log-health-watchdog", func(ctx context.Context) { a.logHealthWatchdog(ctx) })
	SafeGoRestart(a.ctx, "health-monitor", func(ctx context.Context) {
		a.health.Run(ctx, func(s HealthStatus) {
			wailsRuntime.EventsEmit(ctx, "health-updated", s)
		})
	})

	a.tray = NewTrayManager(a)
	a.tray.Start()

	// Watch for OS suspend/resume events to handle network disruption
	// and release held input state on sleep.
	a.power = WatchPowerEvents(func(event string) {
		switch event {
		case "suspend":
			log.Println("system suspending — releasing input and disconnecting")
			a.releaseInjectedRemoteKeys()
			if a.transport != nil && a.transport.GetSession() != nil {
				a.transport.Disconnect()
			}
		case "resume":
			log.Println("system resumed — triggering reconnect")
			a.mu.RLock()
			peerAddr := a.lastPeerAddr
			reconnect := a.autoReconnect
			a.mu.RUnlock()
			if reconnect && peerAddr != "" {
				SafeGo("power-resume-reconnect", func() {
					a.reconnectLoop(a.ctx, peerAddr)
				})
			}
		}
	})

	if a.settings.Get().StartMinimized {
		go func() {
			time.Sleep(300 * time.Millisecond)
			a.tray.HideWindow()
		}()
	}

	logutil.LogKV("app.startup.ready",
		"device_id", shortPeerID(a.device.ID),
		"device_name", a.device.Name,
		"start_minimized", a.settings.Get().StartMinimized,
		"auto_reconnect", a.autoReconnect,
	)
}
