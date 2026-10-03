<script lang="ts">
  import { getAppState } from '../stores/app.svelte';
  import { hotkeyKeys } from '../hotkeys';
  import { oppositeEdge, profileSummary, transportSummary } from '../labels';
  import {
    formatLatency,
    jitterLabel,
    lastPeerAddress,
    lastPeerRoute,
    latencyQuality,
    latencyQualityLabel,
    routeLabel,
  } from '../utils';
  import Icon from '../ui/Icon.svelte';
  import Kbd from '../ui/Kbd.svelte';
  import ActivityFeed from './session/ActivityFeed.svelte';
  import HealthCard from './session/HealthCard.svelte';
  import QuickConnect from './session/QuickConnect.svelte';
  import QuickControls from './session/QuickControls.svelte';
  import ScreenArrangement from './session/ScreenArrangement.svelte';

  const app = getAppState();
  const conn = app.connection;
  const settings = app.settings;

  const session = $derived(conn.session);
  const edge = $derived(settings.values.triggerZone?.side ?? settings.values.edgeSide);
  const returnEdge = $derived(oppositeEdge(edge));
  const exitKeys = $derived(hotkeyKeys(settings.values.exitHotkey));
  const lastName = $derived(conn.lastPeer?.name || '');
  const lastRoute = $derived(lastPeerRoute(conn.lastPeer));

  const routeKind = $derived(session.route || conn.activePeer?.preferredRoute || '');
  const quality = $derived(latencyQuality(session.latencyMs));

  const audioSummary = $derived.by(() => {
    const mode = settings.values.audioMode;
    if (mode === 'remote') return `Hearing ${session.peerName || 'remote'}`;
    if (mode === 'local') return 'Sending this PC';
    return 'Off';
  });

  const audioDetail = $derived(
    settings.values.audioMode === 'off'
      ? 'Desktop audio is not streamed'
      : `${transportSummary(settings.values.audioTransport)} · ${profileSummary(settings.values.audioProfile)} · ${formatLatency(session.audioLatencyMs)}`,
  );

  function splitMs(ms: number): { value: string; unit: string } {
    if (ms < 0) return { value: '—', unit: '' };
    if (ms === 0) return { value: '<1', unit: ' ms' };
    return { value: Number.isInteger(ms) ? String(ms) : ms.toFixed(1), unit: ' ms' };
  }

  async function reconnect() {
    await conn.reconnect();
  }
</script>

<div class="screen session">
  <header class="screen-header session-header">
    <div class="headline">
      {#if session.connected && session.controlling}
        <span class="pill"><span class="pill-dot"></span>Controlling</span>
        <h1 class="peer-title">{session.peerName}</h1>
        <p>
          Your mouse and keyboard are on {session.peerName}. Move back across the {returnEdge} edge, or press
          <Kbd keys={exitKeys} /> to come back.
        </p>
      {:else if session.connected && session.role === 'controlled'}
        <span class="pill info"><span class="pill-dot"></span>Being controlled</span>
        <h1 class="peer-title">{session.peerName}</h1>
        <p>{session.peerName} is using this PC's keyboard and mouse. Control returns when its pointer leaves this screen.</p>
      {:else if session.connected}
        <span class="pill"><span class="pill-dot"></span>Connected</span>
        <h1 class="peer-title">{session.peerName}</h1>
        <p>Push the pointer past the {edge} edge of your screen to control {session.peerName}.</p>
      {:else}
        <span class="pill neutral"><span class="pill-dot"></span>Not connected</span>
        <h1 class="peer-title">{lastName || 'No session'}</h1>
        <p>
          {#if lastName}
            Your last session was with {lastName}{lastRoute ? ` over ${routeLabel(lastRoute)}` : ''}. Reconnect, or pick
            another device.
          {:else}
            Pick a device to start a session. Devices on your network and tailnet appear automatically.
          {/if}
        </p>
      {/if}
    </div>
    <div class="header-actions">
      {#if session.connected}
        <button type="button" class="btn" onclick={() => conn.sendFiles()}>
          <Icon name="upload" />Send files
        </button>
        <button type="button" class="btn btn-danger" onclick={() => conn.disconnect()} disabled={conn.disconnecting}>
          {#if conn.disconnecting}<span class="spinner" aria-hidden="true"></span>{/if}
          {conn.disconnecting ? 'Disconnecting…' : 'Disconnect'}
        </button>
      {:else}
        <button type="button" class="btn" onclick={() => app.navigate('devices')}>Browse devices</button>
        {#if lastPeerAddress(conn.lastPeer)}
          <button type="button" class="btn btn-primary" onclick={reconnect} disabled={conn.reconnecting}>
            {#if conn.reconnecting}<span class="spinner" aria-hidden="true"></span>{/if}
            {conn.reconnecting ? 'Reconnecting…' : 'Reconnect'}
          </button>
        {/if}
      {/if}
    </div>
  </header>

  {#if session.connected}
    {@const latency = splitMs(session.latencyMs)}
    {@const jitter = splitMs(session.jitterMs)}
    <section aria-label="Connection details" class="details">
      <div class="detail">
        <span class="detail-label">Route</span>
        <span class="detail-value with-icon"><Icon name="bolt" />{routeLabel(routeKind)}</span>
        {#if session.remoteAddress}<span class="detail-sub mono selectable">{session.remoteAddress}</span>{/if}
      </div>
      <div class="detail">
        <span class="detail-label">Latency</span>
        <span class="detail-value big mono">{latency.value}<span class="unit">{latency.unit}</span></span>
        <span class="detail-sub" class:good={quality === 'excellent' || quality === 'good'}>{latencyQualityLabel(quality)}</span>
      </div>
      <div class="detail">
        <span class="detail-label">Jitter</span>
        <span class="detail-value big mono">{jitter.value}<span class="unit">{jitter.unit}</span></span>
        <span class="detail-sub">{jitterLabel(session.jitterMs)}</span>
      </div>
      <div class="detail">
        <span class="detail-label">Audio</span>
        <span class="detail-value">{audioSummary}</span>
        <span class="detail-sub">{audioDetail}</span>
      </div>
    </section>
  {/if}

  <div class="columns">
    <div class="col-main">
      {#if !session.connected}
        <QuickConnect />
      {/if}
      <ScreenArrangement />
      <ActivityFeed />
    </div>
    <div class="col-side">
      <QuickControls />
      <HealthCard />
    </div>
  </div>
</div>

<style>
  .session-header {
    align-items: flex-end;
  }

  .headline {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
    flex: 1 1 360px;
  }

  .headline .pill {
    align-self: flex-start;
  }

  .pill-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
  }

  .peer-title {
    font-size: 30px;
    overflow-wrap: anywhere;
  }

  .header-actions {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }

  .details {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 1px;
    background: var(--line);
    border: 1px solid var(--line);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }

  .detail {
    background: var(--panel);
    padding: 14px 16px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .detail-label {
    font-size: 12px;
    color: var(--muted);
  }

  .detail-value {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .with-icon {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .big {
    font-size: 20px;
  }

  .unit {
    font-size: 13px;
    color: var(--muted);
  }

  .detail-sub {
    font-size: 12px;
    color: var(--muted);
    overflow-wrap: anywhere;
  }

  .detail-sub.good {
    color: var(--accent);
  }

  .columns {
    display: flex;
    flex-wrap: wrap;
    gap: 20px;
    align-items: flex-start;
  }

  .col-main {
    flex: 999 1 420px;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .col-side {
    flex: 1 1 280px;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  @media (max-width: 900px) {
    .details {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
