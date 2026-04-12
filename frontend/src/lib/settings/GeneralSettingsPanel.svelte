<script lang="ts">
  import type { MonitorInfo } from '../types';

  export let busySetting = '';
  export let edgeSide = 'right';
  export let sensitivity = 1;
  export let autostart = false;
  export let startMinimized = false;
  export let autoReconnect = true;
  export let isControllerConnected = false;
  export let exitHotkeyModifiers = 0;
  export let exitHotkeyVKCode = 0;
  export let localMonitors: MonitorInfo[] = [];
  export let triggerMonitorID = '';
  export let triggerSide = '';
  export let triggerStartPct = 0;
  export let triggerEndPct = 1;
  export let returnMonitorID = '';
  export let returnXPct = 0.5;
  export let returnYPct = 0.5;
  export let onUpdateEdgeSide: (side: string) => void | Promise<void>;
  export let onUpdateSensitivity: (value: string) => void | Promise<void>;
  export let onUpdateAutostart: (enabled: boolean) => void | Promise<void>;
  export let onUpdateStartMinimized: (enabled: boolean) => void | Promise<void>;
  export let onUpdateAutoReconnect: (enabled: boolean) => void | Promise<void>;
  export let onUpdateExitHotkey: (modifiers: number, vkCode: number) => void | Promise<void>;
  export let onUpdateTriggerZone: (monitorID: string, side: string, startPct: number, endPct: number) => void | Promise<void>;
  export let onUpdateReturnAnchor: (monitorID: string, xPct: number, yPct: number) => void | Promise<void>;

  // VK code map for displaying saved hotkeys from backend (Windows VK codes)
  const vkNames: Record<number, string> = {
    0x08: 'Backspace', 0x09: 'Tab', 0x0D: 'Enter', 0x1B: 'Esc',
    0x20: 'Space', 0x21: 'PgUp', 0x22: 'PgDn', 0x23: 'End', 0x24: 'Home',
    0x25: '←', 0x26: '↑', 0x27: '→', 0x28: '↓',
    0x2C: 'PrtSc', 0x2D: 'Ins', 0x2E: 'Del',
    // Digits
    0x30: '0', 0x31: '1', 0x32: '2', 0x33: '3', 0x34: '4',
    0x35: '5', 0x36: '6', 0x37: '7', 0x38: '8', 0x39: '9',
    // Numpad
    0x60: 'Num0', 0x61: 'Num1', 0x62: 'Num2', 0x63: 'Num3', 0x64: 'Num4',
    0x65: 'Num5', 0x66: 'Num6', 0x67: 'Num7', 0x68: 'Num8', 0x69: 'Num9',
    0x6A: 'Num×', 0x6B: 'Num+', 0x6D: 'Num−', 0x6E: 'Num.', 0x6F: 'Num÷',
    // F-keys
    0x70: 'F1', 0x71: 'F2', 0x72: 'F3', 0x73: 'F4', 0x74: 'F5', 0x75: 'F6',
    0x76: 'F7', 0x77: 'F8', 0x78: 'F9', 0x79: 'F10', 0x7A: 'F11', 0x7B: 'F12',
    // OEM punctuation (US layout)
    0xBA: ';', 0xBB: '=', 0xBC: ',', 0xBD: '-', 0xBE: '.', 0xBF: '/',
    0xC0: '`', 0xDB: '[', 0xDC: '\\', 0xDD: ']', 0xDE: "'",
  };

  function modifierLabel(mod: number): string {
    const parts = [];
    if (mod & 1) parts.push('Ctrl');
    if (mod & 2) parts.push('Alt');
    if (mod & 4) parts.push('Shift');
    if (mod & 8) parts.push('Win');
    return parts.join('+');
  }

  function hotkeyLabel(mod: number, vk: number): string {
    if (vk === 0) return 'Esc (default)';
    const modStr = modifierLabel(mod);
    const keyStr = vkNames[vk] || (vk >= 0x41 && vk <= 0x5A ? String.fromCharCode(vk) : `VK:0x${vk.toString(16).toUpperCase()}`);
    return modStr ? `${modStr}+${keyStr}` : keyStr;
  }

  // Derive friendly key name from a KeyboardEvent (used during capture)
  function keyLabelFromEvent(e: KeyboardEvent): string {
    const k = e.key;
    if (k.length === 1) return k.toUpperCase();
    const named: Record<string, string> = {
      'Backspace': 'Backspace', 'Tab': 'Tab', 'Enter': 'Enter', 'Escape': 'Esc',
      ' ': 'Space', 'PageUp': 'PgUp', 'PageDown': 'PgDn',
      'End': 'End', 'Home': 'Home', 'Insert': 'Ins', 'Delete': 'Del',
      'ArrowLeft': '←', 'ArrowRight': '→', 'ArrowUp': '↑', 'ArrowDown': '↓',
      'PrintScreen': 'PrtSc', 'ScrollLock': 'ScrLk', 'Pause': 'Pause',
      'F1': 'F1', 'F2': 'F2', 'F3': 'F3', 'F4': 'F4', 'F5': 'F5', 'F6': 'F6',
      'F7': 'F7', 'F8': 'F8', 'F9': 'F9', 'F10': 'F10', 'F11': 'F11', 'F12': 'F12',
    };
    return named[k] || k;
  }

  let capturingHotkey = false;
  let captureDisplay = '';
  let captureWarning = '';

  function autoFocus(node: HTMLElement) {
    node.focus();
    return {};
  }

  function startCapture() {
    capturingHotkey = true;
    captureDisplay = '';
    captureWarning = '';
  }

  function onCaptureKeydown(e: KeyboardEvent) {
    if (!capturingHotkey || busySetting !== '') return;
    e.preventDefault();
    e.stopPropagation();

    // Esc cancels capture (also the hardcoded fallback — can't be reassigned)
    if (e.key === 'Escape') {
      cancelCapture();
      return;
    }

    if (['Control', 'Alt', 'Shift', 'Meta'].includes(e.key)) return;

    const vk = e.keyCode; // WebView2 returns Windows VK codes
    let mod = 0;
    if (e.ctrlKey) mod |= 1;
    if (e.altKey) mod |= 2;
    if (e.shiftKey) mod |= 4;
    if (e.metaKey) mod |= 8;

    // Ctrl+Alt collides with AltGr on international keyboard layouts
    if ((mod & 3) === 3) {
      captureWarning = 'Ctrl+Alt collides with AltGr on international layouts — pick another combo';
      return;
    }

    // Bare keys (no modifier) are only safe for function keys — everything else
    // would interrupt remote typing (e.g. bare 'A' would exit on every 'a' press)
    const isFKey = vk >= 0x70 && vk <= 0x7B;
    if (mod === 0 && !isFKey) {
      captureWarning = 'Bare keys like letters or digits would interrupt remote typing. Add a modifier (Ctrl, Alt, Shift) or use F1–F12.';
      return;
    }

    const keyStr = keyLabelFromEvent(e);
    const modStr = modifierLabel(mod);
    capturingHotkey = false;
    captureWarning = '';
    captureDisplay = modStr ? `${modStr}+${keyStr}` : keyStr;
    onUpdateExitHotkey(mod, vk);
  }

  function cancelCapture() {
    capturingHotkey = false;
    captureWarning = '';
  }

  function resetHotkey() {
    capturingHotkey = false;
    captureDisplay = '';
    captureWarning = '';
    onUpdateExitHotkey(0, 0);
  }

  // Monitor layout
  $: hasMonitors = localMonitors.length > 0;
  $: monitorLayoutBounds = computeLayoutBounds(localMonitors);
  $: edgeLayoutLocked = busySetting !== '' || isControllerConnected;

  // Reconcile stale IDs when monitor topology changes
  $: {
    if (!isControllerConnected) {
      const ids = new Set(localMonitors.map(m => m.id));
      if (triggerMonitorID && !ids.has(triggerMonitorID)) {
        triggerMonitorID = '';
        triggerSide = '';
        onUpdateTriggerZone('', edgeSide, 0, 1);
      }
      if (returnMonitorID && !ids.has(returnMonitorID)) {
        returnMonitorID = '';
        onUpdateReturnAnchor('', 0.5, 0.5);
      }
    }
  }

  // Cancel hotkey capture if settings become busy (e.g. another save in flight)
  $: if (busySetting !== '' && capturingHotkey) cancelCapture();

  // Clear stale captureDisplay whenever backend hotkey state changes (e.g. after save-failure rollback)
  $: { exitHotkeyModifiers; exitHotkeyVKCode; captureDisplay = ''; }

  const SVG_W = 300;
  const SVG_H = 130;
  const PAD = 10;

  function computeLayoutBounds(mons: MonitorInfo[]) {
    if (!mons.length) return { minX: 0, minY: 0, w: 1, h: 1 };
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const m of mons) {
      minX = Math.min(minX, m.x);
      minY = Math.min(minY, m.y);
      maxX = Math.max(maxX, m.x + m.width);
      maxY = Math.max(maxY, m.y + m.height);
    }
    return { minX, minY, w: maxX - minX, h: maxY - minY };
  }

  // Computes SVG pixel rect, centered within the viewBox
  function monitorRect(mon: MonitorInfo, bounds: typeof monitorLayoutBounds) {
    const availW = SVG_W - PAD * 2;
    const availH = SVG_H - PAD * 2;
    const scale = Math.min(availW / bounds.w, availH / bounds.h) * 0.9;
    const layoutW = bounds.w * scale;
    const layoutH = bounds.h * scale;
    const offsetX = PAD + (availW - layoutW) / 2;
    const offsetY = PAD + (availH - layoutH) / 2;
    return {
      x: (mon.x - bounds.minX) * scale + offsetX,
      y: (mon.y - bounds.minY) * scale + offsetY,
      w: mon.width * scale,
      h: mon.height * scale,
    };
  }

  // Clean backend monitor name (\\.\DISPLAY1 → "Display 1")
  function friendlyName(mon: MonitorInfo): string {
    const base = mon.name.replace(/^.*\\/, '').replace(/^DISPLAY(\d+)$/, 'Display $1');
    return mon.isPrimary ? `${base} ★` : base;
  }

  const edgeSides = ['left', 'right', 'top', 'bottom'] as const;

  function selectTriggerMonitor(id: string) {
    if (edgeLayoutLocked) return;
    if (triggerMonitorID === id) return; // Preserve existing zone — no lossy reset
    triggerMonitorID = id;
    triggerSide = triggerSide || edgeSide || 'right';
    triggerStartPct = 0;
    triggerEndPct = 1;
    onUpdateTriggerZone(id, triggerSide, 0, 1);
  }

  function selectTriggerSide(side: string) {
    if (edgeLayoutLocked || !triggerMonitorID) return;
    triggerSide = side;
    onUpdateTriggerZone(triggerMonitorID, side, triggerStartPct, triggerEndPct);
  }

  function updateZonePct() {
    if (edgeLayoutLocked || !triggerMonitorID) return;
    // Ensure start < end with at least 5% gap
    if (triggerStartPct >= triggerEndPct) triggerEndPct = Math.min(1, triggerStartPct + 0.05);
    onUpdateTriggerZone(triggerMonitorID, triggerSide, triggerStartPct, triggerEndPct);
  }

  function selectReturnMonitor(id: string) {
    if (edgeLayoutLocked) return;
    returnMonitorID = id;
    returnXPct = 0.5;
    returnYPct = 0.5;
    onUpdateReturnAnchor(id, 0.5, 0.5);
  }

  function clearZones() {
    if (edgeLayoutLocked) return;
    triggerMonitorID = '';
    triggerSide = '';
    returnMonitorID = '';
    triggerStartPct = 0;
    triggerEndPct = 1;
    returnXPct = 0.5;
    returnYPct = 0.5;
    onUpdateTriggerZone('', edgeSide, 0, 1);
    onUpdateReturnAnchor('', 0.5, 0.5);
  }

  function pct(v: number) { return `${Math.round(v * 100)}%`; }
</script>

<div class="settings-grid">
  <!-- Mouse edge -->
  <article class="card">
    <h3>Mouse &amp; screen edge</h3>
    <p class="hint">Which edge hands control to the other computer.</p>
    {#if isControllerConnected}
      <p class="hint hint-lock">Disconnect from the current host to change the edge or monitor layout.</p>
    {/if}
    {#if triggerMonitorID}
      <p class="hint hint-override">⚡ Overridden by custom trigger zone — change the side below.</p>
    {:else}
      <div class="segmented-control edge-control" role="radiogroup" aria-label="Trigger edge">
        {#each edgeSides as side}
          <button
            type="button"
            role="radio"
            aria-checked={edgeSide === side}
            class:active={edgeSide === side}
            on:click={() => onUpdateEdgeSide(side)}
            disabled={edgeLayoutLocked}
          >{side.charAt(0).toUpperCase() + side.slice(1)}</button>
        {/each}
      </div>
    {/if}
    <div class="range-block">
      <div class="range-header">
        <label class="range-label" for="sens-slider">Cursor speed</label>
        <span class="speed-badge">{sensitivity.toFixed(1)}×</span>
      </div>
      <input type="range" id="sens-slider" class="range-slider" min="0.1" max="3" step="0.1" bind:value={sensitivity} on:change={(e) => onUpdateSensitivity(e.currentTarget.value)} disabled={busySetting !== ''} />
    </div>
  </article>

  <!-- Exit hotkey -->
  <article class="card">
    <h3>Exit hotkey</h3>
    <p class="hint">Key to stop controlling the remote computer. <strong>Esc always works</strong> as a fallback.</p>
    <div class="hotkey-row">
      <span class="hotkey-display" aria-live="polite">
        {captureDisplay || hotkeyLabel(exitHotkeyModifiers, exitHotkeyVKCode)}
      </span>
      {#if capturingHotkey}
        <div
          class="hotkey-capture-overlay"
          use:autoFocus
          tabindex="0"
          role="button"
          aria-label="Press a key combination to set the exit hotkey"
          on:keydown={onCaptureKeydown}
          on:blur={cancelCapture}
        >Press a key…</div>
      {:else}
        <button type="button" class="btn-sm" on:click={startCapture} disabled={busySetting !== ''}>Change</button>
        {#if exitHotkeyVKCode !== 0}
          <button type="button" class="btn-sm btn-ghost" on:click={resetHotkey} disabled={busySetting !== ''}>Reset</button>
        {/if}
      {/if}
    </div>
    {#if captureWarning}
      <p class="capture-warning" role="alert">{captureWarning}</p>
    {/if}
  </article>

  <!-- Monitor layout -->
  {#if hasMonitors}
  <article class="card card-wide">
    <h3>Monitor layout &amp; trigger zone</h3>
    <p class="hint">Click a monitor tile to choose where your cursor crosses to the remote machine. The blue band shows the active trigger zone.</p>
    {#if isControllerConnected}
      <p class="hint hint-lock">Monitor layout is locked for the current host session.</p>
    {/if}

    <div class="monitor-layout">
      <!-- svelte-ignore a11y-interactive-supports-focus -->
      <svg
        class="monitor-svg"
        viewBox="0 0 {SVG_W} {SVG_H}"
        role="group"
        aria-label="Monitor layout — click a tile to select trigger monitor"
      >
        {#each localMonitors as mon}
          {@const r = monitorRect(mon, monitorLayoutBounds)}
          <rect
            x={r.x} y={r.y} width={r.w} height={r.h}
             rx="5" ry="5"
             class="monitor-tile"
             class:monitor-selected={triggerMonitorID === mon.id}
             class:monitor-disabled={edgeLayoutLocked}
             role="button"
             tabindex={edgeLayoutLocked ? -1 : 0}
             aria-pressed={triggerMonitorID === mon.id}
             aria-disabled={edgeLayoutLocked}
             aria-label="{friendlyName(mon)} {mon.width}×{mon.height}{triggerMonitorID === mon.id ? ' (selected)' : ''}"
             on:click={() => !edgeLayoutLocked && selectTriggerMonitor(mon.id)}
             on:keydown={(e) => !edgeLayoutLocked && (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), selectTriggerMonitor(mon.id))}
           />
          <text x={r.x + r.w / 2} y={r.y + r.h / 2 - 4} class="monitor-label" text-anchor="middle">
            {friendlyName(mon)}
          </text>
          <text x={r.x + r.w / 2} y={r.y + r.h / 2 + 10} class="monitor-label monitor-res" text-anchor="middle">
            {mon.width}×{mon.height}
          </text>
          {#if triggerMonitorID === mon.id && triggerSide}
            {@const sp = triggerStartPct}
            {@const ep = triggerEndPct}
            {@const span = Math.max(0, ep - sp)}
            {#if triggerSide === 'left'}
              <rect x={r.x} y={r.y + r.h * sp} width={4} height={r.h * span} class="zone-band" rx="2" ry="2" />
            {:else if triggerSide === 'right'}
              <rect x={r.x + r.w - 4} y={r.y + r.h * sp} width={4} height={r.h * span} class="zone-band" rx="2" ry="2" />
            {:else if triggerSide === 'top'}
              <rect x={r.x + r.w * sp} y={r.y} width={r.w * span} height={4} class="zone-band" rx="2" ry="2" />
            {:else}
              <rect x={r.x + r.w * sp} y={r.y + r.h - 4} width={r.w * span} height={4} class="zone-band" rx="2" ry="2" />
            {/if}
          {/if}
          {#if returnMonitorID === mon.id}
            <circle
              cx={r.x + r.w * returnXPct}
              cy={r.y + r.h * returnYPct}
              r="5" class="return-dot"
            />
          {/if}
        {/each}
      </svg>
    </div>

    {#if !triggerMonitorID}
      <p class="hint hint-idle">↑ Click a monitor tile to set the trigger edge.</p>
    {:else}
      <div class="zone-controls">
        <div class="zone-row">
          <span class="zone-label">Trigger side</span>
          <div class="mini-seg" role="radiogroup" aria-label="Trigger side">
            {#each edgeSides as s}
              <button
                type="button"
                role="radio"
                aria-checked={triggerSide === s}
                class:active={triggerSide === s}
                on:click={() => selectTriggerSide(s)}
                disabled={edgeLayoutLocked}
              >{s.charAt(0).toUpperCase() + s.slice(1)}</button>
            {/each}
          </div>
        </div>
        <div class="zone-row">
          <span class="zone-label">Zone start</span>
          <div class="pct-row">
            <input type="range" class="range-slider" min="0" max="0.95" step="0.05"
              bind:value={triggerStartPct} on:change={updateZonePct}
              disabled={edgeLayoutLocked} aria-label="Zone start" />
            <span class="speed-badge">{pct(triggerStartPct)}</span>
          </div>
        </div>
        <div class="zone-row">
          <span class="zone-label">Zone end</span>
          <div class="pct-row">
            <input type="range" class="range-slider" min="0.05" max="1" step="0.05"
              bind:value={triggerEndPct} on:change={updateZonePct}
              disabled={edgeLayoutLocked} aria-label="Zone end" />
            <span class="speed-badge">{pct(triggerEndPct)}</span>
          </div>
        </div>
        <div class="zone-row">
          <span class="zone-label">Cursor returns to</span>
          <select class="device-select" bind:value={returnMonitorID}
            on:change={() => selectReturnMonitor(returnMonitorID)}
            disabled={edgeLayoutLocked}>
            <option value="" disabled>Choose monitor…</option>
            {#each localMonitors as m}
              <option value={m.id}>{friendlyName(m)}</option>
            {/each}
          </select>
        </div>
        <div class="zone-row">
          <span class="zone-label">Return position</span>
          <div class="pct-inputs">
            <label>
              X
              <input type="range" min="0" max="1" step="0.05" bind:value={returnXPct}
                on:change={() => returnMonitorID && onUpdateReturnAnchor(returnMonitorID, returnXPct, returnYPct)}
                disabled={edgeLayoutLocked || !returnMonitorID} />
              <span class="speed-badge">{pct(returnXPct)}</span>
            </label>
            <label>
              Y
              <input type="range" min="0" max="1" step="0.05" bind:value={returnYPct}
                on:change={() => returnMonitorID && onUpdateReturnAnchor(returnMonitorID, returnXPct, returnYPct)}
                disabled={edgeLayoutLocked || !returnMonitorID} />
              <span class="speed-badge">{pct(returnYPct)}</span>
            </label>
          </div>
        </div>
        <button type="button" class="btn-sm btn-ghost" on:click={clearZones} disabled={edgeLayoutLocked}>
          Clear all zones (use defaults)
        </button>
      </div>
    {/if}
  </article>
  {/if}

  <!-- Startup toggles -->
  <article class="card">
    <h3>Startup &amp; behavior</h3>
    <div class="setting-list">
      <div class="setting-row">
        <div class="setting-copy">
          <strong>Start with Windows</strong>
          <span>Launch when you sign in.</span>
        </div>
        <label class="toggle-switch">
          <input type="checkbox" aria-label="Start with Windows" checked={autostart} on:change={(e) => onUpdateAutostart(e.currentTarget.checked)} disabled={busySetting !== ''} />
          <span class="toggle-slider"></span>
        </label>
      </div>
      <div class="setting-row">
        <div class="setting-copy">
          <strong>Start minimized</strong>
          <span>Open quietly to the tray.</span>
        </div>
        <label class="toggle-switch">
          <input type="checkbox" aria-label="Start minimized" checked={startMinimized} on:change={(e) => onUpdateStartMinimized(e.currentTarget.checked)} disabled={busySetting !== ''} />
          <span class="toggle-slider"></span>
        </label>
      </div>
      <div class="setting-row">
        <div class="setting-copy">
          <strong>Auto-reconnect</strong>
          <span>Resume the last session after interruptions.</span>
        </div>
        <label class="toggle-switch">
          <input type="checkbox" aria-label="Auto-reconnect" checked={autoReconnect} on:change={(e) => onUpdateAutoReconnect(e.currentTarget.checked)} disabled={busySetting !== ''} />
          <span class="toggle-slider"></span>
        </label>
      </div>
    </div>
  </article>
</div>

<style>
  .settings-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.75rem;
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
    background: var(--panel);
    padding: 1.25rem;
  }

  .card-wide {
    grid-column: 1 / -1;
  }

  .card h3 {
    font-size: 0.92rem;
    font-weight: 700;
    color: var(--text-strong);
    letter-spacing: -0.01em;
  }

  .hint {
    font-size: 0.85rem;
    color: var(--text-secondary);
    line-height: 1.55;
  }

  .hint-idle {
    text-align: center;
    padding: 0.5rem 0;
    font-style: italic;
  }

  .hint-override {
    color: var(--accent);
    font-size: 0.82rem;
  }

  .hint-lock {
    color: var(--warning, #92400e);
  }

  .setting-list {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .setting-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  .setting-copy {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .setting-copy strong {
    font-size: 0.9rem;
    color: var(--text-strong);
  }

  .setting-copy span {
    font-size: 0.82rem;
    color: var(--text-secondary);
  }

  .range-block {
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
  }

  .range-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .range-label {
    font-size: 0.88rem;
    font-weight: 600;
    color: var(--text-strong);
  }

  /* Renamed from .badge to avoid global class collision */
  .speed-badge {
    display: inline-flex;
    align-items: center;
    padding: 0.18rem 0.55rem;
    border-radius: 999px;
    background: var(--panel-strong);
    color: var(--text-secondary);
    font-size: 0.76rem;
    font-weight: 700;
    flex-shrink: 0;
  }

  .edge-control {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 0.35rem;
  }

  /* Exit hotkey */
  .hotkey-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .hotkey-display {
    flex: 1;
    font-family: var(--font-mono, monospace);
    font-size: 0.88rem;
    color: var(--text-strong);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 0.35rem 0.65rem;
    min-width: 0;
  }

  .hotkey-capture-overlay {
    flex: 1;
    font-size: 0.88rem;
    color: var(--accent);
    background: var(--panel-accent);
    border: 1px dashed var(--accent-border);
    border-radius: var(--radius-md);
    padding: 0.35rem 0.65rem;
    cursor: text;
    outline: none;
  }

  .hotkey-capture-overlay:focus-visible {
    box-shadow: var(--shadow-focus);
  }

  .capture-warning {
    font-size: 0.82rem;
    color: var(--warning, #92400e);
    background: var(--warning-bg, #fffbeb);
    border-radius: var(--radius-md);
    padding: 0.35rem 0.65rem;
    margin: 0;
  }

  .btn-sm {
    padding: 0.28rem 0.75rem;
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-strong);
    font-size: 0.82rem;
    font-weight: 600;
    cursor: pointer;
    white-space: nowrap;
    transition: background var(--transition-fast), border-color var(--transition-fast);
  }

  .btn-sm:hover:not(:disabled) {
    background: var(--panel-accent);
    border-color: var(--accent-border);
  }

  .btn-sm:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .btn-sm:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .btn-ghost {
    background: transparent;
    border-color: transparent;
    color: var(--text-secondary);
  }

  .btn-ghost:hover:not(:disabled) {
    background: var(--panel-strong);
    border-color: var(--border);
  }

  /* Monitor layout */
  .monitor-layout {
    display: flex;
    justify-content: center;
    overflow-x: auto;
  }

  .monitor-svg {
    display: block;
    width: 100%;
    height: auto;
    max-width: 500px;
  }

  :global(.monitor-tile) {
    fill: var(--surface);
    stroke: var(--border);
    stroke-width: 1.5;
    cursor: pointer;
    transition: fill 0.12s;
  }

  :global(.monitor-tile:hover) {
    fill: var(--panel-accent);
  }

  :global(.monitor-tile:focus-visible) {
    outline: none;
    stroke: var(--accent);
    stroke-width: 2.5;
    filter: drop-shadow(0 0 3px color-mix(in srgb, var(--accent) 60%, transparent));
  }

  :global(.monitor-tile.monitor-selected) {
    stroke: var(--accent);
    stroke-width: 2;
    fill: var(--panel-accent);
  }

  :global(.monitor-tile.monitor-disabled) {
    cursor: not-allowed;
    opacity: 0.5;
  }

  @media (prefers-reduced-motion: reduce) {
    :global(.monitor-tile) { transition: none; }
  }

  :global(.monitor-label) {
    font-size: 10px;
    fill: var(--text-secondary);
    pointer-events: none;
    user-select: none;
  }

  :global(.monitor-res) {
    font-size: 8px;
    opacity: 0.7;
  }

  :global(.zone-band) {
    fill: var(--accent);
    opacity: 0.75;
    pointer-events: none;
  }

  :global(.return-dot) {
    fill: var(--success, #2f855a);
    stroke: #fff;
    stroke-width: 1.5;
    pointer-events: none;
  }

  .zone-controls {
    display: flex;
    flex-direction: column;
    gap: 0.65rem;
  }

  .zone-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .zone-label {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-strong);
    min-width: 9rem;
  }

  .pct-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex: 1;
  }

  .pct-row .range-slider {
    flex: 1;
  }

  .mini-seg {
    display: flex;
    gap: 0.3rem;
  }

  .mini-seg button {
    padding: 0.2rem 0.55rem;
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    background: var(--surface);
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
    cursor: pointer;
    text-transform: capitalize;
    transition: background var(--transition-fast), border-color var(--transition-fast), color var(--transition-fast);
  }

  .mini-seg button:hover:not(:disabled) {
    background: var(--panel-strong);
    color: var(--text-strong);
  }

  .mini-seg button:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .mini-seg button.active {
    background: var(--panel-accent);
    border-color: var(--accent-border);
    color: var(--text-strong);
  }

  .mini-seg button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .pct-inputs {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    flex-wrap: wrap;
  }

  .pct-inputs label {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--text-strong);
  }

  @media (max-width: 960px) {
    .settings-grid {
      grid-template-columns: 1fr;
    }
    .card-wide {
      grid-column: unset;
    }
  }

  @media (max-width: 640px) {
    .setting-row {
      flex-direction: column;
      align-items: flex-start;
    }
    .edge-control {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>

