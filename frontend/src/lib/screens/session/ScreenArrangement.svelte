<script lang="ts">
  import type { MonitorInfo } from '../../api/types';
  import { getAppState } from '../../stores/app.svelte';
  import Card from '../../ui/Card.svelte';
  import Icon from '../../ui/Icon.svelte';

  const app = getAppState();
  const conn = app.connection;
  const settings = app.settings;

  const BOX_W = 240;
  const BOX_H = 90;

  const edge = $derived(settings.values.triggerZone?.side ?? settings.values.edgeSide);
  const vertical = $derived(edge === 'top' || edge === 'bottom');
  const remoteFirst = $derived(edge === 'left' || edge === 'top');
  const remoteName = $derived(
    conn.session.peerName || conn.activePeer?.name || conn.lastPeer?.name || 'Other PC',
  );
  const controlling = $derived(conn.session.connected && conn.session.controlling);

  const monitors = $derived<MonitorInfo[]>(
    settings.monitors.length
      ? settings.monitors
      : [{ id: 'primary', name: '1', x: 0, y: 0, width: 16, height: 9, isPrimary: true }],
  );

  const layout = $derived.by(() => {
    const minX = Math.min(...monitors.map((m) => m.x));
    const minY = Math.min(...monitors.map((m) => m.y));
    const maxX = Math.max(...monitors.map((m) => m.x + m.width));
    const maxY = Math.max(...monitors.map((m) => m.y + m.height));
    const scale = Math.min(BOX_W / Math.max(1, maxX - minX), BOX_H / Math.max(1, maxY - minY));
    const gap = monitors.length > 1 ? 3 : 0;
    return {
      width: (maxX - minX) * scale,
      height: (maxY - minY) * scale,
      rects: monitors.map((m, index) => ({
        monitor: m,
        label: displayNumber(m, index),
        left: (m.x - minX) * scale + gap / 2,
        top: (m.y - minY) * scale + gap / 2,
        width: Math.max(8, m.width * scale - gap),
        height: Math.max(8, m.height * scale - gap),
      })),
    };
  });

  /** Monitor that hands off: the trigger zone's, else the primary. */
  const handoffMonitorID = $derived(
    settings.values.triggerZone?.monitorID ?? monitors.find((m) => m.isPrimary)?.id ?? monitors[0]?.id,
  );
  const zone = $derived(settings.values.triggerZone ?? { startPct: 0.15, endPct: 0.85 });

  function displayNumber(monitor: MonitorInfo, index: number): string {
    const match = /DISPLAY(\d+)/i.exec(monitor.name);
    return match ? match[1] : String(index + 1);
  }

  function markerStyle(width: number, height: number): string {
    const span = Math.max(0.05, zone.endPct - zone.startPct);
    if (edge === 'left' || edge === 'right') {
      const side = edge === 'right' ? 'right: -3px' : 'left: -3px';
      return `${side}; top: ${zone.startPct * height}px; width: 4px; height: ${span * height}px`;
    }
    const side = edge === 'bottom' ? 'bottom: -3px' : 'top: -3px';
    return `${side}; left: ${zone.startPct * width}px; height: 4px; width: ${span * width}px`;
  }
</script>

<Card title="Screen arrangement">
  {#snippet actions()}
    <button type="button" class="link-btn" onclick={() => app.navigate('settings', 'input')}>Edit layout</button>
  {/snippet}
  <div
    class="stage"
    class:vertical
    class:reverse={remoteFirst}
    role="img"
    aria-label="{conn.device.name || 'This PC'} hands off to {remoteName} across the {edge} edge"
  >
    <div class="node">
      <div class="monitors" style:width="{layout.width}px" style:height="{layout.height}px">
        {#each layout.rects as rect (rect.monitor.id)}
          <div
            class="monitor"
            style:left="{rect.left}px"
            style:top="{rect.top}px"
            style:width="{rect.width}px"
            style:height="{rect.height}px"
          >
            {rect.label}
            {#if rect.monitor.id === handoffMonitorID}
              <span class="marker" style={markerStyle(rect.width, rect.height)}></span>
            {/if}
          </div>
        {/each}
      </div>
      <span class="node-name">{conn.device.name || 'This PC'}</span>
    </div>
    <div class="link">
      <span class="arrow" class:rot-down={edge === 'bottom'} class:rot-up={edge === 'top'} class:rot-left={edge === 'left'}>
        <Icon name="arrow" size={28} />
      </span>
      <span class="edge mono">{edge}</span>
    </div>
    <div class="node">
      <div class="remote" class:active={controlling}>
        {#if controlling}
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M5 3l14 8-6 1.5L9.5 19z" /></svg>
        {/if}
      </div>
      <span class="node-name">{remoteName}</span>
    </div>
  </div>
</Card>

<style>
  .stage {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    flex-wrap: wrap;
    padding: 28px 8px;
    background: var(--panel2);
    border-radius: var(--radius-md);
  }

  .stage.reverse {
    flex-direction: row-reverse;
  }

  .stage.vertical {
    flex-direction: column;
  }

  .stage.vertical.reverse {
    flex-direction: column-reverse;
  }

  .node {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
  }

  .node-name {
    font-size: 13px;
    font-weight: 500;
  }

  .monitors {
    position: relative;
  }

  .monitor {
    position: absolute;
    border: 1.5px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--panel);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    color: var(--muted);
  }

  .marker {
    position: absolute;
    background: var(--accent);
    border-radius: 2px;
  }

  .link {
    width: 70px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    color: var(--accent);
  }

  .arrow {
    display: flex;
  }

  .rot-down {
    transform: rotate(90deg);
  }

  .rot-up {
    transform: rotate(-90deg);
  }

  .rot-left {
    transform: rotate(180deg);
  }

  .edge {
    font-size: 11px;
  }

  .remote {
    width: 132px;
    height: 80px;
    border: 1.5px dashed var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--panel);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text);
  }

  .remote.active {
    border: 1.5px solid var(--accent);
    background: var(--accent-soft);
  }
</style>
