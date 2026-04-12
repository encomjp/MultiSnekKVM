---
description: "Use when editing Svelte frontend code, adding UI features, calling Go backend methods, subscribing to Wails runtime events, or styling the control-center dashboard."
applyTo: "frontend/**"
---

# Frontend Conventions

## Modular component architecture

```
frontend/src/
  App.svelte          → Slim orchestrator: state, events, backend calls, layout
  components.css       → Shared component styles (imported by App.svelte)
  lib/
    constants.ts       → Shared constants (emptySession, emptyTailscale, routeRank)
    utils.ts           → Pure helper functions (formatting, labels, normalization)
    TopBar.svelte      → Header bar with status pills
    HeroPanel.svelte   → Overview metrics grid
    NodeIdentity.svelte → Local device info card
    SessionCard.svelte  → Session posture card with connect/disconnect
    TailscaleCard.svelte → Tailscale integration status
    PeerInventory.svelte → Peer list with add form
    PeerRow.svelte      → Individual peer row with actions
    InputPolicy.svelte  → Edge side + sensitivity controls
    AudioPolicy.svelte  → Audio mode + timing controls
    DiscoveryNotes.svelte → Footer notes
```

- `App.svelte` is the **orchestrator only** — it owns state, backend calls, event subscriptions, and passes data down as props
- Components live in `frontend/src/lib/` — each is a focused, self-contained `.svelte` file with scoped styles
- Pure logic goes in `lib/utils.ts` and `lib/constants.ts` — keep components thin
- No Svelte stores, no router, no state management library
- State is local `let` variables in `App.svelte` with `$:` reactive declarations
- Do NOT create deeply nested component trees — keep it flat under `lib/`

## Backend communication

- **Pull (RPC)**: Call `window.go.main.App.MethodName()` — returns a Promise
- **Push (events)**: Subscribe with `window.runtime.EventsOn('event-name', callback)`
- Active event names: `peers-updated`, `session-updated`, `tailscale-updated`
- All backend calls and event subscriptions live in `App.svelte`, not in child components
- Child components receive callbacks as props (e.g. `onConnect`, `onDisconnect`)

Do NOT import from `wailsjs/` — those generated bindings are stale. Always use `window.go.main.App.*` directly.

## Types

- `window.go` and `window.runtime` are declared as `any` in `global.d.ts`
- TypeScript is intentionally loose (`strict: false`, `noImplicitAny: false`)
- Do not tighten TS strictness or add type annotations to existing untyped code

## Patterns

```svelte
<!-- Backend call (in App.svelte) -->
const result = await window.go.main.App.GetPeers();

<!-- Event subscription (in App.svelte onMount) -->
window.runtime.EventsOn('peers-updated', (data) => {
  peers = data || [];
});

<!-- Reactive derived state (in App.svelte) -->
$: onlinePeers = peers.filter((p) => p.status === 'online');

<!-- Passing data to child components -->
<PeerInventory {peers} {session} onConnect={connect} onDisconnect={disconnect} />
```

## Testing

- Test runner: Vitest (`cd frontend && npm test`)
- Unit tests for `lib/utils.ts` and `lib/constants.ts` in `src/__tests__/`
- Component render tests using `@testing-library/svelte` in `src/__tests__/`
- Wails globals are mocked in `src/__tests__/setup.ts`
- Run tests before committing frontend changes

## Styling

- Global design tokens in `frontend/src/style.css` and `frontend/src/theme.css`
- Shared component styles in `frontend/src/components.css` (imported once by App.svelte)
- Component-scoped styles in each `.svelte` file's `<style>` block
- The UI is an operations dashboard — prefer data-dense, compact layouts
