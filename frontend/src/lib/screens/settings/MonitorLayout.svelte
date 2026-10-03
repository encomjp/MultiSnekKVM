<script lang="ts">
  // Monitor layout editor. Edits happen in a local draft (dragging, sliders)
  // and are saved once when the gesture ends, never as a side effect of
  // rendering. Stale monitor IDs are reported, not silently rewritten.
  import type { EdgeSide, MonitorInfo, ReturnAnchor, TriggerZone } from '../../api/types';
  import { getAppState } from '../../stores/app.svelte';
  import { EDGE_OPTIONS } from '../../labels';
  import Card from '../../ui/Card.svelte';
  import Icon from '../../ui/Icon.svelte';
  import SegmentedControl from '../../ui/SegmentedControl.svelte';
  import Select from '../../ui/Select.svelte';

  interface Props {
    locked: boolean;
  }

  let { locked }: Props = $props();

  const app = getAppState();
  const settings = app.settings;

  const MIN_SPAN = 0.05;

  let draftZone = $state<TriggerZone | null>(null);
  let draftAnchor = $state<ReturnAnchor | null>(null);

  const monitors = $derived(settings.monitors);
  const zone = $derived(draftZone ?? settings.values.triggerZone);
  const anchor = $derived(draftAnchor ?? settings.values.returnAnchor);
  const ids = $derived(new Set(monitors.map((m) => m.id)));
  const staleZone = $derived(!!settings.values.triggerZone && !ids.has(settings.values.triggerZone.monitorID));
  const staleAnchor = $derived(!!settings.values.returnAnchor && !ids.has(settings.values.returnAnchor.monitorID));
  const busy = $derived(settings.status.triggerZone.pending || settings.status.returnAnchor.pending);

  const bounds = $derived.by(() => {
    if (!monitors.length) return { minX: 0, minY: 0, w: 1, h: 1 };
    const minX = Math.min(...monitors.map((m) => m.x));
    const minY = Math.min(...monitors.map((m) => m.y));
    const maxX = Math.max(...monitors.map((m) => m.x + m.width));
    const maxY = Math.max(...monitors.map((m) => m.y + m.height));
    return { minX, minY, w: Math.max(1, maxX - minX), h: Math.max(1, maxY - minY) };
  });

  function friendlyName(monitor: MonitorInfo): string {
    const base = monitor.name.replace(/^\\\\\.\\/, '').replace(/^DISPLAY(\d+)$/i, 'Display $1') || monitor.id;
    return base;
  }

  function box(monitor: MonitorInfo) {
    return {
      left: ((monitor.x - bounds.minX) / bounds.w) * 100,
      top: ((monitor.y - bounds.minY) / bounds.h) * 100,
      width: (monitor.width / bounds.w) * 100,
      height: (monitor.height / bounds.h) * 100,
    };
  }

  const pct = (value: number) => `${Math.round(value * 100)}%`;
  const clamp = (value: number, min = 0, max = 1) => Math.min(max, Math.max(min, value));

  function barStyle(z: TriggerZone): string {
    const span = Math.max(MIN_SPAN, z.endPct - z.startPct);
    switch (z.side) {
      case 'left':
        return `left:-5px;top:${z.startPct * 100}%;width:8px;height:${span * 100}%`;
      case 'right':
        return `right:-5px;top:${z.startPct * 100}%;width:8px;height:${span * 100}%`;
      case 'top':
        return `top:-5px;left:${z.startPct * 100}%;height:8px;width:${span * 100}%`;
      default:
        return `bottom:-5px;left:${z.startPct * 100}%;height:8px;width:${span * 100}%`;
    }
  }

  function commitZone(next: TriggerZone | null) {
    draftZone = null;
    void settings.set('triggerZone', next);
  }

  function commitAnchor(next: ReturnAnchor | null) {
    draftAnchor = null;
    void settings.set('returnAnchor', next);
  }

  function selectHandoffMonitor(id: string) {
    if (locked) return;
    if (!id) return commitZone(null);
    if (zone?.monitorID === id) return;
    commitZone({ monitorID: id, side: zone?.side ?? settings.values.edgeSide, startPct: 0, endPct: 1 });
  }

  function setSide(side: EdgeSide) {
    if (zone) commitZone({ ...zone, side });
  }

  function setRange(which: 'start' | 'end', value: number, commit: boolean) {
    if (!zone) return;
    let { startPct, endPct } = zone;
    if (which === 'start') {
      startPct = clamp(value, 0, 1 - MIN_SPAN);
      endPct = Math.max(endPct, startPct + MIN_SPAN);
    } else {
      endPct = clamp(value, MIN_SPAN, 1);
      startPct = Math.min(startPct, endPct - MIN_SPAN);
    }
    const next = { ...zone, startPct: round(startPct), endPct: round(endPct) };
    if (commit) commitZone(next);
    else draftZone = next;
  }

  function selectReturnMonitor(id: string) {
    if (locked) return;
    if (!id) return commitAnchor(null);
    if (anchor?.monitorID === id) return;
    commitAnchor({ monitorID: id, xPct: 0.5, yPct: 0.5 });
  }

  function setAnchorAxis(axis: 'xPct' | 'yPct', value: number, commit: boolean) {
    if (!anchor) return;
    const next = { ...anchor, [axis]: round(clamp(value)) };
    if (commit) commitAnchor(next);
    else draftAnchor = next;
  }

  function resetAll() {
    if (settings.values.triggerZone || staleZone) commitZone(null);
    if (settings.values.returnAnchor || staleAnchor) commitAnchor(null);
  }

  const round = (value: number) => Math.round(value * 100) / 100;

  // ---- pointer dragging (keyboard users have the sliders below) ----

  function dragBar(event: PointerEvent, monitorEl: HTMLElement) {
    if (locked || !zone) return;
    event.preventDefault();
    event.stopPropagation();
    const start = { ...zone };
    const rect = monitorEl.getBoundingClientRect();
    const horizontal = start.side === 'top' || start.side === 'bottom';
    const origin = horizontal ? event.clientX : event.clientY;
    const size = horizontal ? rect.width : rect.height;
    const span = start.endPct - start.startPct;
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture?.(event.pointerId);

    const move = (e: PointerEvent) => {
      const delta = ((horizontal ? e.clientX : e.clientY) - origin) / Math.max(1, size);
      const startPct = clamp(start.startPct + delta, 0, 1 - span);
      draftZone = { ...start, startPct: round(startPct), endPct: round(startPct + span) };
    };
    const up = () => {
      target.removeEventListener('pointermove', move);
      target.removeEventListener('pointerup', up);
      target.removeEventListener('pointercancel', up);
      if (draftZone) commitZone(draftZone);
    };
    target.addEventListener('pointermove', move);
    target.addEventListener('pointerup', up);
    target.addEventListener('pointercancel', up);
  }

  function dragDot(event: PointerEvent, monitorEl: HTMLElement) {
    if (locked || !anchor) return;
    event.preventDefault();
    event.stopPropagation();
    const rect = monitorEl.getBoundingClientRect();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture?.(event.pointerId);
    const base = { ...anchor };
    const move = (e: PointerEvent) => {
      draftAnchor = {
        ...base,
        xPct: round(clamp((e.clientX - rect.left) / Math.max(1, rect.width))),
        yPct: round(clamp((e.clientY - rect.top) / Math.max(1, rect.height))),
      };
    };
    const up = () => {
      target.removeEventListener('pointermove', move);
      target.removeEventListener('pointerup', up);
      target.removeEventListener('pointercancel', up);
      if (draftAnchor) commitAnchor(draftAnchor);
    };
    target.addEventListener('pointermove', move);
    target.addEventListener('pointerup', up);
    target.addEventListener('pointercancel', up);
  }

  const monitorOptions = $derived(monitors.map((m) => ({ value: m.id, label: `${friendlyName(m)}${m.isPrimary ? ' (primary)' : ''}` })));
  const zoneMonitor = $derived(monitors.find((m) => m.id === zone?.monitorID));
  const anchorMonitor = $derived(monitors.find((m) => m.id === anchor?.monitorID));
</script>

<Card
  title="Monitor layout"
  description="Drag the green bar to set where the hand-off happens. The dot is where the pointer lands when you come back."
>
  {#snippet actions()}
    <button
      type="button"
      class="btn btn-sm"
      disabled={locked || busy || (!settings.values.triggerZone && !settings.values.returnAnchor)}
      onclick={resetAll}
    >
      <Icon name="reset" />Reset
    </button>
  {/snippet}

  {#if monitors.length === 0}
    <p class="muted small">Monitor information isn't available yet. The whole screen edge is used for hand-off.</p>
  {:else}
    {#if staleZone || staleAnchor}
      <div class="notice warning" role="status">
        <Icon name="alert" />
        <span>A monitor used by your layout is no longer connected. Reset the layout or pick another monitor.</span>
      </div>
    {/if}

    <div class="stage" class:locked>
      <div class="canvas" style:aspect-ratio="{bounds.w} / {bounds.h}">
        {#each monitors as monitor (monitor.id)}
          {@const b = box(monitor)}
          {@const isZone = zone?.monitorID === monitor.id}
          <div
            class="monitor"
            class:selected={isZone}
            style:left="{b.left}%"
            style:top="{b.top}%"
            style:width="{b.width}%"
            style:height="{b.height}%"
          >
            <button
              type="button"
              class="monitor-hit"
              aria-pressed={isZone}
              disabled={locked}
              onclick={() => selectHandoffMonitor(monitor.id)}
            >
              <span class="monitor-name">{friendlyName(monitor)}{monitor.isPrimary ? ' · primary' : ''}</span>
              <span class="monitor-res mono">{monitor.width}×{monitor.height}</span>
              <span class="sr-only">{isZone ? '(hand-off monitor)' : 'Use for hand-off'}</span>
            </button>
            {#if isZone && zone}
              <span
                class="bar"
                class:horizontal={zone.side === 'top' || zone.side === 'bottom'}
                style={barStyle(zone)}
                aria-hidden="true"
                onpointerdown={(e) => dragBar(e, (e.currentTarget as HTMLElement).parentElement as HTMLElement)}
              ></span>
            {/if}
            {#if anchor?.monitorID === monitor.id}
              <span
                class="dot"
                style:left="{anchor.xPct * 100}%"
                style:top="{anchor.yPct * 100}%"
                aria-hidden="true"
                onpointerdown={(e) => dragDot(e, (e.currentTarget as HTMLElement).parentElement as HTMLElement)}
              ></span>
            {/if}
          </div>
        {/each}
      </div>
    </div>

    <div class="legend">
      <span class="legend-item">
        <span class="swatch-bar" aria-hidden="true"></span>
        {#if zone && zoneMonitor}
          Hand-off zone · {friendlyName(zoneMonitor)}, {zone.side} edge, {pct(zone.startPct)}–{pct(zone.endPct)}
        {:else}
          Hand-off zone · whole {settings.values.edgeSide} edge
        {/if}
      </span>
      <span class="legend-item">
        <span class="swatch-dot" aria-hidden="true"></span>
        {#if anchor && anchorMonitor}
          Return point · {friendlyName(anchorMonitor)}, {pct(anchor.xPct)}, {pct(anchor.yPct)}
        {:else}
          Return point · where the pointer left
        {/if}
      </span>
    </div>

    <div class="controls">
      <div class="control-group">
        <Select
          label="Hand-off monitor"
          value={zone?.monitorID && ids.has(zone.monitorID) ? zone.monitorID : ''}
          options={[{ value: '', label: 'Any monitor on the screen edge' }, ...monitorOptions]}
          disabled={locked}
          pending={settings.status.triggerZone.pending}
          onChange={selectHandoffMonitor}
        />
        {#if zone}
          <SegmentedControl
            label="Hand-off side"
            options={EDGE_OPTIONS}
            value={zone.side}
            disabled={locked}
            onChange={setSide}
          />
          <div class="range-row">
            <label for="zone-start" class="field-label">Zone starts at</label>
            <span class="mono small">{pct(zone.startPct)}</span>
          </div>
          <input
            id="zone-start"
            type="range"
            min="0"
            max="0.95"
            step="0.05"
            value={zone.startPct}
            disabled={locked}
            oninput={(e) => setRange('start', Number(e.currentTarget.value), false)}
            onchange={(e) => setRange('start', Number(e.currentTarget.value), true)}
          />
          <div class="range-row">
            <label for="zone-end" class="field-label">Zone ends at</label>
            <span class="mono small">{pct(zone.endPct)}</span>
          </div>
          <input
            id="zone-end"
            type="range"
            min="0.05"
            max="1"
            step="0.05"
            value={zone.endPct}
            disabled={locked}
            oninput={(e) => setRange('end', Number(e.currentTarget.value), false)}
            onchange={(e) => setRange('end', Number(e.currentTarget.value), true)}
          />
        {/if}
        {#if settings.status.triggerZone.error}<p class="field-error">{settings.status.triggerZone.error}</p>{/if}
      </div>

      <div class="control-group">
        <Select
          label="Return point monitor"
          value={anchor?.monitorID && ids.has(anchor.monitorID) ? anchor.monitorID : ''}
          options={[{ value: '', label: 'Where the pointer left (default)' }, ...monitorOptions]}
          disabled={locked}
          pending={settings.status.returnAnchor.pending}
          onChange={selectReturnMonitor}
        />
        {#if anchor}
          <div class="range-row">
            <label for="anchor-x" class="field-label">Horizontal position</label>
            <span class="mono small">{pct(anchor.xPct)}</span>
          </div>
          <input
            id="anchor-x"
            type="range"
            min="0"
            max="1"
            step="0.01"
            value={anchor.xPct}
            disabled={locked}
            oninput={(e) => setAnchorAxis('xPct', Number(e.currentTarget.value), false)}
            onchange={(e) => setAnchorAxis('xPct', Number(e.currentTarget.value), true)}
          />
          <div class="range-row">
            <label for="anchor-y" class="field-label">Vertical position</label>
            <span class="mono small">{pct(anchor.yPct)}</span>
          </div>
          <input
            id="anchor-y"
            type="range"
            min="0"
            max="1"
            step="0.01"
            value={anchor.yPct}
            disabled={locked}
            oninput={(e) => setAnchorAxis('yPct', Number(e.currentTarget.value), false)}
            onchange={(e) => setAnchorAxis('yPct', Number(e.currentTarget.value), true)}
          />
        {/if}
        {#if settings.status.returnAnchor.error}<p class="field-error">{settings.status.returnAnchor.error}</p>{/if}
      </div>
    </div>
  {/if}
</Card>

<style>
  .stage {
    display: flex;
    justify-content: center;
    padding: 36px 28px;
    background: var(--panel2);
    border-radius: var(--radius-md);
  }

  .canvas {
    position: relative;
    width: min(100%, 560px);
    max-height: 200px;
  }

  .monitor {
    position: absolute;
    padding: 3px;
  }

  .monitor-hit {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 2px;
    border: 1.5px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--panel);
    color: var(--muted);
    font-size: 12px;
    overflow: hidden;
    padding: 4px;
  }

  .monitor-hit:hover:not(:disabled) {
    border-color: var(--text);
  }

  .selected .monitor-hit {
    border-color: var(--accent);
  }

  .monitor-hit:disabled {
    cursor: not-allowed;
  }

  .monitor-name {
    text-align: center;
  }

  .monitor-res {
    font-size: 11px;
  }

  .bar {
    position: absolute;
    background: var(--accent);
    border-radius: 4px;
    cursor: ns-resize;
    touch-action: none;
  }

  .bar.horizontal {
    cursor: ew-resize;
  }

  .dot {
    position: absolute;
    width: 14px;
    height: 14px;
    margin: -7px 0 0 -7px;
    border-radius: 50%;
    border: 2px solid var(--accent);
    background: var(--panel);
    cursor: grab;
    touch-action: none;
  }

  .locked .bar,
  .locked .dot {
    cursor: not-allowed;
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 18px;
    font-size: 12px;
    color: var(--muted);
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .swatch-bar {
    width: 14px;
    height: 6px;
    border-radius: 3px;
    background: var(--accent);
  }

  .swatch-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    border: 2px solid var(--accent);
  }

  .controls {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 18px;
    padding-top: 4px;
    border-top: 1px solid var(--line);
  }

  .control-group {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding-top: 12px;
    min-width: 0;
  }

  .range-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    margin-bottom: -6px;
  }

  @media (max-width: 880px) {
    .controls {
      grid-template-columns: 1fr;
    }
  }
</style>
