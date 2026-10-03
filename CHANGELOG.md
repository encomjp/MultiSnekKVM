# Changelog

All notable changes to this project will be documented in this file.

## [0.4.0] - 2026-10-03

### Added

- **Native Bluetooth link.** Two paired Windows PCs can now connect directly over Classic Bluetooth (RFCOMM) with no network setup. The same TLS 1.3 session, first-time PIN pairing, trust checks and rate limiting run on top of it, so input, clipboard, files and audio all work.
- **Bluetooth discovery.** Each PC advertises a MultiSnek service to its paired devices, so paired PCs running MultiSnek appear automatically under Devices (merged with their LAN/Tailscale entry) and can also be added by hand as `bt://AA:BB:CC:DD:EE:FF` or a bare MAC address.
- Devices screen: Bluetooth card with status, refresh and an on/off toggle; Settings: "Bluetooth connections" toggle. Direct Bluetooth is labelled "Bluetooth" and the older network-tethering kind stays "Bluetooth PAN".

### Changed

- Over Bluetooth, audio is always compressed (Opus): desktop audio is capped at 96 kbit/s, the microphone stays at 64 kbit/s, whatever the audio transport setting says. Bluetooth ranks after every IP route, so it is only used when nothing faster is available.

### Notes

- Pair both PCs in Windows Bluetooth settings first. Classic Bluetooth gives roughly 1-2 Mbit/s shared, so large file transfers are slow.
- The Windows Bluetooth calls (service registration and lookup, RFCOMM connect) are covered by unit tests of the surrounding logic and by tests of the connection layer over real Winsock sockets, but had not been run on two physical Bluetooth radios at release time.

## [0.3.0] - 2026-10-03

### Breaking

- First-time pairing now uses a certificate-bound SPAKE2 exchange; the six-digit PIN is no longer sent on the wire. Both devices must run 0.3.0 or later to pair. Already-trusted peers reconnect without re-pairing.

### Changed

- New interface: sidebar navigation, Session screen (route, latency, jitter, audio, screen arrangement, activity feed with Save/Discard for received files), Devices screen with per-route radio picker and adapter panel, Settings split into Input & screens, Audio, Startup, Security and Diagnostics, dark/light/system theme. Bundled Geist fonts (no network requests).
- Frontend rebuilt on Svelte 5, Vite 7 and strict TypeScript with typed Go bindings, per-setting save state and a consistent lockfile (`npm ci`).
- Desktop audio transport has a new default, `auto`: PCM over USB4/USB-bridge/Ethernet, Opus over Wi-Fi, Tailscale and Bluetooth. Old settings with the previous `pcm` default are migrated once.
- Microphone always uses Opus (mono, 64 kb/s, no DTX/FEC).

### Added

- Automatic route selection: peer addresses are ranked by the Windows adapter actually used (USB4/Thunderbolt, USB network bridge, Ethernet, LAN, Wi-Fi, other, Tailscale, Bluetooth PAN). Manual connect falls back to other addresses of the same device on network failures only.
- USB4/Thunderbolt, USB bridge and Bluetooth PAN setup guide and local adapter list.
- Session status shows the actual route and remote address; `health-alert` events are shown in the UI.
- Adaptive audio jitter buffer with clock-drift compensation, shared by microphone and desktop audio.
- PR validation CI: Go tests, race tests, gofmt, go vet, tidy modules, frontend type-check, tests and build.

### Fixed

- Microphone passthrough: loud white noise (the controller reset its receiver mid-stream and played Opus packets as PCM), and "erased"/gated audio (mic frames dropped by a shared send queue, whole buffers of silence on every hiccup, chunk cuts from sound-card clock drift, Opus DTX). Format changes can no longer overtake audio data.
- Audio has its own send lane, so heavy audio can no longer push out keystrokes and close the session.
- Changing an audio device reopens the stream instead of stopping it; "Mute source" is now applied.
- A stalled unauthenticated client could block all connections: TLS handshake and pairing now run outside the connection lock.
- Received-file Save/Discard only accept folders the app created.
- Auto-reconnect stops on trust/certificate failures; sleep no longer starts reconnect attempts.
- File transfers apply back-pressure instead of dropping chunks (which aborted transfers).
- Hooks ignore injected events; modifiers are only released on the side that injected them.
- Edge-trigger bounds refresh after monitor changes; edge-loop and startup races fixed; discovery goroutines are panic-safe and capped.
- Settings are written atomically; rejected values are no longer saved.
- Outbound connections fail closed when the trust store is unavailable; revoking trust disconnects the active session.
- Outbound TLS dial and handshake are time-bounded; short protocol writes are reported as errors.
- Dropped key, mouse-button, text or switch-back frames close the session so held input is released on both sides.
- Stale held remote mouse buttons are released by the input watchdog (previously keys only).
- File transfer validates chunk bounds and ordering, uses unique temporary directories and rejects duplicate offers.

### Performance

- Network adapter and route lookups are cached instead of re-enumerated (and a UDP socket opened per address) on every peer/session query while holding the input lock.
- UI events are only emitted on change; clipboard polling skips unchanged clipboards; cached screen metrics for injection; fewer allocations in the frame, mouse and audio paths; quieter idle logging.

### Removed

- About 25 KB of dead UI components, the unused drag-and-drop capture code and four duplicated WASAPI loops.
- `Startup`/`Shutdown`/`BeforeClose` are no longer exposed to JavaScript.

### Security note

- The SPAKE2 implementation (`github.com/backkem/spake2-go` v0.0.1, with pre-release `go.dedis.ch/kyber/v4`) passes RFC 9382 test vectors but has not been independently audited.

## [0.2.0] - 2026-04-10

### Fixed

- Cursor disappearing on client unless touchpad touched: replaced `SetCursorPos` with `SendInput` (absolute, virtual-desktop coords) so a real `WM_MOUSEMOVE` is generated.
- UAC prompt blocks mouse: host now auto-exits remote mode on two consecutive NULL foreground windows (Secure Desktop detection); client sends switch-back signal when Secure Desktop is active.
- System tray randomly becomes unresponsive: dedicated goroutine with `runtime.LockOSThread()` before `systray.Run` ensures `GetMessageW` stays on the correct OS thread; click callbacks dispatched asynchronously to keep the message loop responsive.
- `globalHook` data race between hook callbacks and `SetConnected`: migrated to `atomic.Pointer[InputHook]`.
- Hook install failure left app in phantom remote mode: hooks are now installed and validated before `inRemoteMode` is set; failure fast-paths with cleanup.
- Edge re-entry after UAC exit: 500 ms cooldown after exiting remote mode prevents the cursor edge from immediately re-triggering.
- Mouse-button watchdog gap: watchdog now also arms when remote mouse buttons are held, preventing stuck buttons on dropped `MouseUp`.
- Zero-delta wake frame triggered return-edge: `handleRemoteMouseMove` returns early for `DX==0, DY==0` frames to avoid instant session bounce.
- Audio latency and reliability: 14 audio fixes including silence suppression removal, improved latency telemetry, and fail-fast file receives.
- Auto-reconnect now survives long outages without restarting the connection loop.
- Disabled broken edge drag-and-drop that caused spurious mode switches.

### Added

- Secure Desktop monitor goroutine with panic-safe restart supervision.

### Changed

- Settings screen split into focused panels (General, Audio, Input) for clarity.
- Internal app, audio, and input subsystems refactored into focused source files.
- UI and runtime polish: updated screenshots, refined shell helpers, restored settings responsiveness.

## [0.1.2] - 2026-03-30

### Fixed

- Released any still-pressed remote modifier keys when control returns, the session disconnects, or the app shuts down so Ctrl, Shift, Alt, and Win do not stay stuck.
- Sent an explicit switch-back signal when exiting remote mode with Escape so the controlled machine can clear pressed remote keys immediately.

### Added

- Added remote input state tracking and focused unit tests for releasing pressed remote keys safely.