<script lang="ts">
  import { formatLatency, latencyQuality } from './utils';

  export let sessionStatusText = '';
  export let sessionStatusSubtext = '';
  export let deviceName = '';
  export let onlinePeerCount = 0;
  export let activeRouteLabel = 'Standby';
  export let latencyLabel = 'Standby';
  export let latencyTone: 'good' | 'ok' | 'idle' = 'idle';
  export let sessionConnected = false;
  export let latencyMs = -1;
  export let audioLatencyMs = -1;
  export let jitterMs = -1;

  export let sessionPeerName = '';
  export let reconnecting = false;
  export let disconnecting = false;
  export let healthReconnecting = false;
  export let healthHealthy = true;
  export let healthSummaryText = '';
  export let lastPeerName = '';
  export let pairingCode = '';
  export let onReconnect: () => void | Promise<void>;
  export let onDisconnect: () => void | Promise<void>;
  export let onBrowseDevices: () => void;
  export let onSendFiles: (() => void | Promise<void>) | undefined = undefined;

  let pairingCodeVisible = false;

  $: emptyState = !sessionConnected && !lastPeerName;
  $: pingQuality   = latencyQuality(latencyMs,   [10, 50, 150]);
  $: audioQuality  = latencyQuality(audioLatencyMs, [80, 150, 300]);
  $: jitterQuality = latencyQuality(jitterMs,    [5, 20, 50]);
  $: statusDetail = healthReconnecting
    ? 'Trying the last known device in the background.'
    : emptyState
      ? onlinePeerCount > 0
        ? `${onlinePeerCount} device${onlinePeerCount !== 1 ? 's are' : ' is'} ready to connect from Devices.`
        : 'Add a trusted device to start switching between systems.'
      : sessionStatusSubtext;
  $: if (!pairingCode) {
    pairingCodeVisible = false;
  }

  function togglePairingCodeVisibility() {
    pairingCodeVisible = !pairingCodeVisible;
  }
</script>

<section class="screen" data-screen="overview">
  {#if healthReconnecting}
    <div class="health-banner reconnecting" role="status" aria-live="polite">
      <span class="health-spinner" aria-hidden="true"></span>
      <span>Reconnect in progress.</span>
    </div>
  {:else if !healthHealthy}
    <div class="health-banner" role="status" aria-live="polite">
      <span class="health-dot"></span>
      <span>{healthSummaryText || 'System health needs attention.'}</span>
    </div>
  {/if}

  <div class="hero">
    <div class="status-row">
      <span class="status-dot" class:connected={sessionConnected}></span>
      <span class="status-label">{sessionStatusText}</span>
    </div>

    <h1 class="peer-name">
      {sessionConnected ? sessionPeerName : (lastPeerName || 'No devices')}
    </h1>

    {#if statusDetail}
      <p class="status-detail">{statusDetail}</p>
    {/if}

    <div class="actions">
      {#if sessionConnected}
        <button class="btn btn-danger" on:click={onDisconnect} disabled={disconnecting}>
          {#if disconnecting}<span class="btn-spinner"></span>{/if}
          {disconnecting ? 'Ending…' : 'Disconnect'}
        </button>
        {#if onSendFiles}
          <button class="btn btn-secondary" on:click={onSendFiles}>
            Send files
          </button>
        {/if}
      {:else}
        {#if lastPeerName}
          <button class="btn btn-primary" on:click={onReconnect} disabled={reconnecting}>
            {#if reconnecting}<span class="btn-spinner"></span>{/if}
            {reconnecting ? 'Connecting…' : 'Reconnect'}
          </button>
        {/if}
        <button class="btn btn-secondary" on:click={onBrowseDevices}>
          {onlinePeerCount > 0 ? 'Browse devices' : 'Add a device'}
        </button>
      {/if}
    </div>

    {#if sessionConnected}
      <div class="conn-metrics" aria-label="Connection quality">
        <div class="metric-pill" aria-label="Ping: {formatLatency(latencyMs)}, quality {pingQuality}">
          <span class="metric-dot quality-{pingQuality}" aria-hidden="true"></span>
          <span class="metric-name">Ping</span>
          <span class="metric-val">{formatLatency(latencyMs)}</span>
        </div>
        <div class="metric-pill" aria-label="Audio latency: {formatLatency(audioLatencyMs)}, quality {audioQuality}">
          <span class="metric-dot quality-{audioQuality}" aria-hidden="true"></span>
          <span class="metric-name">Audio est.</span>
          <span class="metric-val">{formatLatency(audioLatencyMs)}</span>
        </div>
        <div class="metric-pill" aria-label="Jitter: {formatLatency(jitterMs)}, quality {jitterQuality}">
          <span class="metric-dot quality-{jitterQuality}" aria-hidden="true"></span>
          <span class="metric-name">Jitter</span>
          <span class="metric-val">{formatLatency(jitterMs)}</span>
        </div>
      </div>
    {/if}

    {#if pairingCode}
      <div class="pairing-prompt">
        <span class="pairing-prompt-label">Pairing a new device?</span>
        <button
          class="btn btn-secondary"
          type="button"
          aria-expanded={pairingCodeVisible}
          aria-controls="pairing-code-panel"
          on:click={togglePairingCodeVisibility}
        >
          {pairingCodeVisible ? 'Hide pairing PIN' : 'Show pairing PIN'}
        </button>
      </div>
      {#if pairingCodeVisible}
        <div class="pairing-panel" id="pairing-code-panel" aria-live="polite">
          <span class="pairing-label">First-time pairing PIN</span>
          <strong class="pairing-code mono selectable">{pairingCode}</strong>
          <p>Enter this code on the other machine the first time it asks to pair and trust this workstation. It rotates after use, expiry, or repeated failed attempts.</p>
        </div>
      {/if}
    {/if}
  </div>

  <footer class="meta-row" aria-label="Connection summary">
    <span class="meta-item">{deviceName || 'This computer'}</span>
    <span class="dot-sep" aria-hidden="true">·</span>
    <span class="meta-item">{onlinePeerCount} device{onlinePeerCount !== 1 ? 's' : ''} available</span>
    <span class="dot-sep" aria-hidden="true">·</span>
    <span class="meta-item">{activeRouteLabel}</span>
    {#if sessionConnected && latencyTone !== 'idle'}
      <span class="dot-sep" aria-hidden="true">·</span>
      <span class="meta-item" class:tone-good={latencyTone === 'good'} class:tone-ok={latencyTone === 'ok'}>{latencyLabel}</span>
    {/if}
  </footer>
</section>

<style>
  .screen {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  .health-banner {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.65rem 1.1rem;
    background: var(--danger-bg);
    border-bottom: 1px solid rgba(199, 76, 83, 0.2);
    font-size: 0.85rem;
    color: var(--danger);
  }

  .health-banner.reconnecting {
    background: var(--panel-accent);
    border-bottom-color: var(--accent-border);
    color: var(--accent-strong);
  }

  .health-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--danger);
    flex-shrink: 0;
  }

  .health-spinner {
    width: 0.95rem;
    height: 0.95rem;
    border-radius: 50%;
    border: 2px solid currentColor;
    border-top-color: transparent;
    flex-shrink: 0;
    animation: spin 0.8s linear infinite;
  }

  .hero {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 1.5rem;
    padding: 2rem;
    text-align: center;
    min-height: 0;
  }

  .conn-metrics {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
    justify-content: center;
  }
  .metric-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.3rem 0.7rem;
    border-radius: 2rem;
    background: rgba(126, 145, 168, 0.08);
    border: 1px solid rgba(126, 145, 168, 0.18);
    font-size: 0.82rem;
  }
  .metric-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .quality-good      { background: var(--success, #22c55e); }
  .quality-ok        { background: var(--warning, #facc15); }
  .quality-warn      { background: var(--warning-strong, #f59e0b); }
  .quality-bad       { background: var(--danger, #ef4444); }
  .quality-measuring { background: rgba(126, 145, 168, 0.4); }
  .metric-name {
    color: var(--text-muted);
    font-weight: 600;
    letter-spacing: 0.03em;
  }
  .metric-val { font-variant-numeric: tabular-nums; }

  .status-row {
    display: flex;
    align-items: center;
    gap: 0.55rem;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-muted);
    flex-shrink: 0;
    transition: background 0.3s;
  }

  .status-dot.connected {
    background: var(--success);
    box-shadow: 0 0 0 3px var(--success-bg);
  }

  .status-label {
    font-size: 0.82rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .peer-name {
    font-size: clamp(2rem, 5vw, 3rem);
    letter-spacing: -0.04em;
    color: var(--text-strong);
    line-height: 1.1;
    max-width: 32rem;
    text-wrap: balance;
    overflow-wrap: anywhere;
  }

  .status-detail {
    max-width: 34rem;
    font-size: 0.98rem;
    line-height: 1.55;
    color: var(--text-secondary);
    text-wrap: balance;
  }

  .actions {
    display: flex;
    gap: 0.75rem;
    flex-wrap: wrap;
    justify-content: center;
  }

  .pairing-prompt {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .pairing-prompt-label {
    font-size: 0.9rem;
    color: var(--text-secondary);
  }

  .pairing-panel {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.55rem;
    padding: 1rem 1.15rem;
    border: 1px solid var(--accent-border);
    border-radius: 1rem;
    background: var(--panel-accent);
    max-width: 26rem;
  }

  .pairing-label {
    font-size: 0.76rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .pairing-code {
    font-size: clamp(1.7rem, 5vw, 2.4rem);
    letter-spacing: 0.22em;
    color: var(--text-strong);
    padding-left: 0.22em;
  }

  .pairing-panel p {
    margin: 0;
    color: var(--text-secondary);
    line-height: 1.5;
    max-width: 22rem;
  }

  .meta-row {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-wrap: wrap;
    gap: 0.6rem;
    padding: 0.85rem 1.5rem;
    border-top: 1px solid var(--border);
    font-size: 0.8rem;
    color: var(--text-muted);
    text-align: center;
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
  }

  .dot-sep {
    opacity: 0.35;
  }

  .tone-good { color: var(--success); }
  .tone-ok   { color: var(--warning); }

  @media (max-width: 720px) {
    .hero {
      padding: 1.5rem 1.1rem 1.75rem;
      gap: 1.15rem;
    }

    .status-detail {
      font-size: 0.92rem;
    }

    .actions {
      width: 100%;
    }

    .actions .btn {
      flex: 1 1 12rem;
    }

    .pairing-prompt {
      width: 100%;
    }

    .pairing-prompt .btn {
      flex: 1 1 12rem;
    }

    .meta-row {
      padding: 0.9rem 1rem 1.05rem;
      row-gap: 0.25rem;
    }

    .dot-sep {
      display: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .health-spinner {
      animation: none;
      border-top-color: currentColor;
      opacity: 0.7;
    }
  }
</style>
