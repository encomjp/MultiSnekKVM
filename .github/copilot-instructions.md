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

- `main.go` is process/bootstrap root (Wails setup + supervisor/child watchdog).
- `app.go` is the composition root; subsystem wiring happens in `App.startup()`.
- `app.go` exported PascalCase methods are the Wails RPC surface.
- `transport.go` handles TLS session lifecycle, handshake validation, and peer authorization.
- `discovery.go` handles LAN/Tailscale discovery loops and route ranking.
- `resilience.go` owns goroutine restart strategy and health monitor snapshots/events.

## Conventions

- Keep backend in flat `package main` unless explicitly refactoring project structure.
- Mirror platform features with paired `*_windows.go` and `*_stub.go` files.
- Frontend calls backend via `window.go.main.App.*` (not generated wrappers).
- Frontend subscribes to runtime events in `App.svelte` via `window.runtime.EventsOn(...)`.
- Active event names include `peers-updated`, `session-updated`, `tailscale-updated`, and `health-updated`.
- Keep TypeScript compatibility with current loose config (`strict: false`, `noImplicitAny: false`) unless asked to tighten it.

## Pitfalls

- Generated Wails bindings under `frontend/wailsjs/` may be stale; treat `app.go` as API truth.
- Trust behavior in code is currently bidirectional TOFU (inbound and outbound auto-trust unknown peers with pinning).
- `TrustPeer` is intentionally a no-op compatibility method.
- Naming drift exists (`MultiSnekKVM` vs `Multisnek`) across README/build metadata.
- Prefer source files over README for behavior details if they conflict.

`transport.go` intentionally uses `InsecureSkipVerify`; peer validation is enforced by explicit hello/certificate checks plus fingerprint pinning. Do not remove this without replacing the full trust flow.

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
