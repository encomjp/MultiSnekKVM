# Changelog

All notable changes to this project will be documented in this file.

## [0.3.0] - 2026-10-03

### Breaking

- First-time pairing now uses a certificate-bound SPAKE2 exchange; the six-digit PIN is no longer sent on the wire. Both devices must run 0.3.0 or later to pair. Already-trusted peers reconnect without re-pairing.

### Added

- Automatic route selection: peer addresses are ranked by the Windows adapter actually used (USB4/Thunderbolt, USB network bridge, Ethernet, LAN, Wi-Fi, other, Tailscale, Bluetooth PAN). Manual connect falls back to other addresses of the same device on network failures only.
- Devices screen: USB4/Thunderbolt, USB bridge and Bluetooth PAN setup guide, list of local adapter addresses, and a per-peer connection address picker.
- Session status shows the actual route and remote address.
- PR validation CI workflow (Go tests, race tests for transport/audio, frontend tests and build).

### Fixed

- Outbound connections fail closed when the trust store is unavailable; revoking trust disconnects the active session.
- Outbound TLS dial and handshake are time-bounded; short protocol writes are reported as errors.
- Dropped key, mouse-button, text or switch-back frames close the session so held input is released on both sides.
- Stale held remote mouse buttons are released by the input watchdog (previously keys only).
- File transfer validates chunk bounds and ordering, uses unique temporary directories and rejects duplicate offers.
- Audio capture/playback start and stop are serialized to prevent races on rapid toggling and format changes.

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