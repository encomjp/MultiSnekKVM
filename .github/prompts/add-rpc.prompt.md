---
description: "Scaffold a new Wails RPC method end-to-end: Go method on App, frontend call in App.svelte, and optional event wiring."
argument-hint: "Describe the RPC method (e.g. 'GetTrustedPeers returns a list of trusted peers')"
agent: "wails-dev"
tools: [read, edit, search]
---

Add a new Wails RPC method to the MultiSnekKVM project based on the user's description: $input

## Steps

### 1. Go backend — add method to `App` in [app.go](app.go)

- Method must be exported (PascalCase) and on `*App` receiver
- Use `a.mu.RLock()`/`a.mu.RUnlock()` for read-only methods, `a.mu.Lock()`/`a.mu.Unlock()` for mutations
- If the method returns a struct, add `json` tags to all fields
- Follow the existing pattern — see `GetEdgeSide()`, `SetEdgeSide()`, `GetPeers()` for examples
- If the method changes state that the frontend cares about, emit an event:
  ```go
  wailsRuntime.EventsEmit(a.ctx, "event-name", payload)
  ```

### 2. Frontend — call from [App.svelte](frontend/src/App.svelte)

- Call via `window.go.main.App.MethodName()` — never import from `wailsjs/`
- For getters: add to the `Promise.all` in `refreshAll()` if it should load at boot
- For setters: wire to a UI control (button, select, toggle) with an async handler
- For state changes: subscribe to the backend event in the `onMount` block if a new event was added
- Use local `let` variables for new state — no Svelte stores

### 3. Verify

- Confirm the Go method compiles: check for syntax and type errors
- Confirm the frontend call matches the exact method name
- If a new struct was added, ensure all fields have `json` tags

## Output

Show the exact code added to each file. Keep changes minimal — don't refactor surrounding code.
