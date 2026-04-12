<script>
  import { sessionStateLabel, preferredRouteLabel, formatLatency, latencyQuality, healthSummary } from './utils';

  export let session;
  export let activePeer;
  export let edgeSide;
  export let health;
  export let autoReconnect;
  export let onDisconnect;
  export let onAutoReconnectChange;

  let disconnecting = false;

  $: if (!session.connected) disconnecting = false;

  $: reconnecting = health && health.reconnecting;

  $: sessionHeadline = reconnecting
    ? 'Reconnecting...'
    : !session.connected
      ? 'No active desk handoff'
      : session.controlling
        ? `Controlling ${session.peerName}`
        : session.role === 'controlled'
          ? `${session.peerName} has control of this node`
          : `Connected to ${session.peerName}`;

  $: sessionSummary = reconnecting
    ? 'Connection lost. Attempting to reconnect automatically...'
    : !session.connected
      ? 'Discovery is active. Connect to a trusted peer to start a keyboard and mouse handoff.'
      : session.controlling
        ? 'Keyboard, mouse, clipboard, and optional audio are flowing to the remote workstation. Press Escape to release control.'
        : session.role === 'controlled'
          ? 'This workstation is currently being controlled by the connected peer. Local edge detection is paused until the session ends.'
          : `Move the pointer to the ${edgeSide} edge of this display to enter remote control.`;

  $: pingQuality   = latencyQuality(session.latencyMs,   [10, 50, 150]);
  $: audioQuality  = latencyQuality(session.audioLatencyMs, [80, 150, 300]);
  $: jitterQuality = latencyQuality(session.jitterMs,    [5, 20, 50]);

  async function handleDisconnect() {
    disconnecting = true;
    try {
      await onDisconnect();
    } catch (_) {}
    disconnecting = false;
  }
</script>

<article class="panel overview-card">
  <div class="panel-heading">
    <div>
      <span class="eyebrow">Session posture</span>
      <h3>{sessionHeadline}</h3>
    </div>
    <span class="status-chip" class:is-active={session.connected} class:is-reconnecting={reconnecting}>
      {reconnecting ? 'Reconnecting' : sessionStateLabel(session)}
    </span>
  </div>

  <p class="panel-copy">{sessionSummary}</p>

  <div class="session-meta">
    <div>
      <span class="meta-label">Preferred route</span>
      <span class="meta-value">{activePeer ? preferredRouteLabel(activePeer.preferredRoute) : 'Auto'}</span>
    </div>
    <div>
      <span class="meta-label">Remote peer</span>
      <span class="meta-value">{activePeer ? activePeer.name : 'None'}</span>
    </div>
    {#if session.connected}
      <div class="conn-metrics">
        <div class="metric-pill" aria-label="Ping: {formatLatency(session.latencyMs)}, quality {pingQuality}">
          <span class="metric-dot quality-{pingQuality}" aria-hidden="true"></span>
          <span class="metric-name">Ping</span>
          <span class="metric-val">{formatLatency(session.latencyMs)}</span>
        </div>
        <div class="metric-pill" aria-label="Audio latency: {formatLatency(session.audioLatencyMs)}, quality {audioQuality}">
          <span class="metric-dot quality-{audioQuality}" aria-hidden="true"></span>
          <span class="metric-name">Audio est.</span>
          <span class="metric-val">{formatLatency(session.audioLatencyMs)}</span>
        </div>
        <div class="metric-pill" aria-label="Jitter: {formatLatency(session.jitterMs)}, quality {jitterQuality}">
          <span class="metric-dot quality-{jitterQuality}" aria-hidden="true"></span>
          <span class="metric-name">Jitter</span>
          <span class="metric-val">{formatLatency(session.jitterMs)}</span>
        </div>
      </div>
      <div>
        <span class="meta-label">Clipboard</span>
        <span class="meta-value">Synced</span>
      </div>
    {/if}
    <div>
      <span class="meta-label">System health</span>
      <span class="meta-value" class:is-degraded={health && !health.healthy}>{healthSummary(health)}</span>
    </div>
    <div>
      <span class="meta-label">Auto-reconnect</span>
      <label class="toggle-inline" title="Reconnects automatically if the session drops unexpectedly">
        <input type="checkbox" checked={autoReconnect} on:change={(e) => onAutoReconnectChange(e.currentTarget.checked)} />
        <span>{autoReconnect ? 'On' : 'Off'}</span>
      </label>
      <p class="toggle-hint">Reconnects if the session drops unexpectedly.</p>
    </div>
  </div>

  {#if session.connected}
    <button class="button button-danger" on:click={handleDisconnect} disabled={disconnecting}>
      {disconnecting ? 'Disconnecting...' : 'Disconnect Session'}
    </button>
  {/if}
</article>

<style>
  .overview-card { padding: 1.1rem; }
  .session-meta {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.8rem;
    margin: 1rem 0 1.1rem;
  }
  .conn-metrics {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
    grid-column: 1 / -1;
  }
  .metric-pill {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.3rem 0.65rem;
    border-radius: 2rem;
    background: rgba(126, 145, 168, 0.08);
    border: 1px solid rgba(126, 145, 168, 0.18);
    font-size: 0.8rem;
  }
  .metric-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .quality-good    { background: var(--color-success, #22c55e); }
  .quality-ok      { background: var(--color-ok, #facc15); }
  .quality-warn    { background: var(--color-warning, #f59e0b); }
  .quality-bad     { background: var(--color-danger, #ef4444); }
  .quality-measuring { background: rgba(126, 145, 168, 0.4); }
  .metric-name {
    color: var(--text-muted);
    font-weight: 600;
    letter-spacing: 0.03em;
  }
  .metric-val { font-variant-numeric: tabular-nums; }
  .is-reconnecting {
    background: var(--color-warning, #f59e0b);
    animation: pulse 1.5s ease-in-out infinite;
  }
  .is-degraded { color: var(--color-warning, #f59e0b); }
  .toggle-inline {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    cursor: pointer;
  }
  .toggle-hint {
    margin-top: 0.25rem;
    font-size: 0.78rem;
    color: var(--text-muted);
    line-height: 1.4;
  }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.6; }
  }
</style>
