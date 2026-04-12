# MultiSnekKVM — Known Issues Checklist

Cross-referenced from Barrier, Input Leap, Deskflow, and Lan Mouse GitHub issues.

## P0 — Critical

- [x] **Modifier keys stuck on session transition** — Ctrl/Alt/Win/Shift stay permanently "pressed" after switching peers. Need explicit key-up for all modifiers on session start/stop/switch. ([Barrier #207](https://github.com/debauchee/barrier/issues/207), [Deskflow #9011](https://github.com/deskflow/deskflow/issues/9011), [Lan Mouse #79](https://github.com/feschber/lan-mouse/issues/79))
  - Files: `input/input_windows_inject.go`, `app/app_lifecycle.go`
  - Fix: Added `ReleaseAllModifiers()` that sends key-up for all 8 modifier VK codes, called at end of `releaseInjectedRemoteKeys()` on every transition.

- [x] **Large clipboard freezes the app** — Copying a large screenshot (85 MB+) while switching causes lockup. Need async clipboard with size cap and cancellation. ([Barrier #775](https://github.com/debauchee/barrier/issues/775), [Barrier #470](https://github.com/debauchee/barrier/issues/470))
  - Files: `clipboard/clipboard_sync.go`
  - Status: Already mitigated — size limits enforced on both inbound/outbound, async polling loop in dedicated goroutine.

- [x] **TLS resource exhaustion / rate limiting** — Unauthenticated connections can exhaust resources. Need per-IP rate limiting and TLS handshake timeout. ([Deskflow #7806](https://github.com/deskflow/deskflow/issues/7806) — 5 CVEs)
  - Files: `transport/transport.go`, `transport/transport_lifecycle.go`
  - Fix: Added per-IP sliding-window rate limiter (5 connections/30s per IP) in accept loop with periodic cleanup.

## P1 — High

- [x] **Memory/goroutine leaks during reconnection loops** — Socket FDs and goroutines accumulate over days. Health monitor should track goroutine count and flag leaks. ([Deskflow #8653](https://github.com/deskflow/deskflow/issues/8653), [Deskflow #9454](https://github.com/deskflow/deskflow/issues/9454))
  - Files: `resilience/resilience.go`
  - Fix: Health monitor now tracks goroutine count, baseline delta, high-water mark, and logs warnings when delta exceeds 50.

- [x] **Sleep/resume breaks connections** — Network connections die on sleep; need Win32 power event handling to trigger clean disconnect + immediate reconnect on resume. ([Deskflow #8568](https://github.com/deskflow/deskflow/issues/8568), [Deskflow #8652](https://github.com/deskflow/deskflow/issues/8652))
  - Files: `sysutil/power_windows.go`, `app/app_startup.go`, `app/app_background.go`
  - Fix: Added PowerWatcher using WM_POWERBROADCAST message-only window. On suspend: releases keys + disconnects. On resume: triggers auto-reconnect.

- [x] **Silent connection drops ("connected but dead")** — TCP stays open but input stops working. Need application-level heartbeat beyond TCP keepalive. ([Barrier #589](https://github.com/debauchee/barrier/issues/589), [Deskflow #8368](https://github.com/deskflow/deskflow/issues/8368))
  - Files: `transport/transport_session.go`
  - Status: Already mitigated — heartbeat loop (5s interval) + read deadline (15s timeout) detects dead connections.

## P2 — Medium

- [x] **Scroll wheel sensitivity** — Scroll delta values differ across machines/mice. Need WHEEL_DELTA normalization. ([Lan Mouse #329](https://github.com/feschber/lan-mouse/issues/329), [Lan Mouse #115](https://github.com/feschber/lan-mouse/issues/115))
  - Files: `input/input_windows_inject.go`
  - Status: Already correct — both sides use raw Windows WHEEL_DELTA (120 per notch) as the common unit.

- [ ] **Multi-monitor DPI coordinate mapping** — Input coordinates wrong on mixed-DPI multi-monitor setups. Need per-monitor DPI awareness. ([Barrier #94](https://github.com/debauchee/barrier/issues/94), [Barrier #1638](https://github.com/debauchee/barrier/issues/1638))
  - Files: `input/input_windows_inject.go`
  - Note: Current mouse move uses relative deltas (`SetCursorPos` with dx/dy), which is DPI-agnostic. Physical pixel movement is 1:1 regardless of scaling. Low risk for current architecture.

- [x] **Audio queue memory accumulation** — Buffered audio frames pile up under backpressure. Need drop-oldest policy. ([Deskflow #8653](https://github.com/deskflow/deskflow/issues/8653))
  - Files: `app/audio_queue.go`
  - Status: Already had drop-oldest policy. Added drop counter (`realtimeDropCount`) for observability.
