# MultiSnekKVM - Workspace Instructions

Desktop KVM switch for trusted peers across LAN and Tailscale. Stack: Go + Wails v2 + Svelte 4. Windows-primary.

## Build and Test

Use these as source-of-truth commands:

- Full-stack dev: `wails dev` (repo root)
- Production build: `.\build.ps1` (preferred over raw `wails build`)
- Frontend dev only: `cd frontend && npm run dev`
- Frontend tests: `cd frontend && npm test`
- Frontend build: `cd frontend && npm run build`

Environment notes:

- `go.mod` requires Go `1.25.0`.
- `.\build.ps1` expects MSYS2 MinGW at `C:\msys64\mingw64\bin`.
- `.\build.ps1` enables CGO and static links Opus/Ogg libs.

## Architecture

- `main.go` only embeds assets/icons and calls `internal/bootstrap.Run`.
- `internal/bootstrap` runs the supervisor/child watchdog and configures `wails.Run`. Lifecycle hooks come from `app.LifecycleHooks(app)`; `*app.App` is the only bound struct.
- `internal/app` is the composition root. `startup()` (app_startup.go) wires every subsystem; exported methods on `*app.App` are the Wails RPC surface (lifecycle methods are deliberately unexported so they are not bound). Key files: `send_mux.go` (single writer with mouse > high > audio > file lanes, heartbeats), `app_frames.go` (inbound frame dispatch), `app_events.go` (deduplicated frontend events), `app_reconnect.go`, `app_peers.go`, `app_control.go`.
- `internal/transport` handles the TLS listener/dialer, hello validation, SPAKE2 PIN pairing and peer authorization. `transport.ErrDial` marks connectivity failures (only those may be retried on another address).
- `internal/protocol` defines the frame format and message codecs.
- `internal/discovery` runs LAN broadcast/Tailscale discovery; `internal/link` enumerates adapters and ranks routes (cached: `link.Adapters`, `link.RouteKind`; `link.RefreshAdapters` forces a refresh).
- `internal/input` (hooks, injection, edge detection), `internal/audio`, `internal/filetransfer`, `internal/clipboard`, `internal/trust`, `internal/identity`, `internal/settings`, `internal/tailscale`, `internal/sysutil` (power events), `internal/resilience` (panic-safe restart + health monitor), `internal/logutil`.

## Conventions

- Keep subsystems in their `internal/*` package; `internal/app` re-exports what it needs via aliases in `app.go`.
- Mirror platform features with paired `*_windows.go` and `*_stub.go` files.
- Frontend calls backend via `window.go.app.App.*` (falling back to `window.go.main.App.*`; see `frontend/src/lib/appShell.ts`), not generated wrappers.
- Frontend subscribes to runtime events in `App.svelte` via `window.runtime.EventsOn(...)`.
- Active event names include `device-updated`, `peers-updated`, `session-updated`, `tailscale-updated`, `health-updated`, `health-alert` and `file-received`. `peers-updated`, `session-updated` and `tailscale-updated` are only emitted when their payload changed.
- Start goroutines with `logutil.SafeGo` / `resilience.SafeGoRestart`, never a bare `go` for long-lived loops.
- Run `gofmt` on Go changes (CI enforces it; `.gitattributes` keeps `.go` files LF) and `go vet -unsafeptr=false ./internal/...`.
- Keep TypeScript compatibility with current loose config (`strict: false`, `noImplicitAny: false`) unless asked to tighten it.

## Pitfalls

- Generated Wails bindings under `frontend/wailsjs/` may be stale; treat `internal/app` as API truth.
- Unknown peers must pair with the 6-digit PIN shown on the other machine (SPAKE2, certificate-bound); afterwards the certificate fingerprint is pinned in the trust store. There is no silent trust-on-first-use.
- `TrustPeer(address, pin)` performs that PIN pairing and connects; `UntrustPeer` revokes trust and drops an active session.
- `SaveReceivedFiles` / `DiscardReceivedFiles` only accept temp dirs announced via the `file-received` event.
- Naming drift exists (`MultiSnekKVM` vs `Multisnek`) across README/build metadata.
- Prefer source files over README for behavior details if they conflict.

`internal/transport` intentionally uses `InsecureSkipVerify`; peer validation is enforced by explicit hello/certificate checks, PIN pairing and fingerprint pinning. Do not remove this without replacing the full trust flow.

## Link Map

Link to these files instead of duplicating their contents:

- `README.md` for user-facing setup and feature overview.
- `.github/instructions/frontend.instructions.md` for frontend-focused guidance (`applyTo: frontend/**`).
- `.github/agents/wails-dev.agent.md` for cross Go/Wails/Svelte implementation workflow.
- `.github/prompts/add-rpc.prompt.md` for RPC-scaffolding task flow.

## Design Context

Use this context for frontend work and any polish/design-oriented skill flows.

### Users

- MultiSnekKVM is for people who actively work across two or more trusted Windows machines on the same LAN or tailnet.
- Primary users are technical operators such as developers, homelab users, power users, and creators.
- They use the app in short, frequent bursts and need connection state, trust posture, and device availability to be obvious at a glance.

### Brand Personality

- The product should feel technical, calm, and trustworthy.
- Favor confidence and operational competence over novelty or playfulness.

### Aesthetic Direction

- Use a desktop-first operations-dashboard aesthetic with compact density, crisp hierarchy, and restrained emphasis.
- Build on the existing cool-blue accent palette, clean cards, subtle elevation, and light/dark theme support already present in the repo.
- Avoid gamer RGB styling, noisy admin-console clutter, excessive ornament, or distracting motion.

### Design Principles

- Operational clarity first: session state, trust state, route choice, and health should be immediately legible.
- Calm over noisy: use restraint in color, motion, and copy so the product feels reliable under continuous use.
- Dense but breathable: preserve desktop efficiency without making the layout cramped or visually tiring.
- State coverage matters: every async or risky action should have clear loading, success, error, and disabled states.
- Accessible by default: preserve visible focus, keyboard navigation, good contrast, and reduced-motion support.

### Quality Bar

- Treat frontend work in this repository as flagship-quality utility software, not throwaway MVP UI.

### Accessibility

- Target WCAG AA contrast as the default baseline.
- Do not rely on color alone to communicate status.
- Maintain clear focus indicators, sensible keyboard order, and reduced-motion support.
