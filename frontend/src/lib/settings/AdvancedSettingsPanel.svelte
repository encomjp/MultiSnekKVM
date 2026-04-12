<script lang="ts">
  import type { DeviceInfo, TailscaleStatus } from '../types';
  import { shortFingerprint } from '../utils';

  export let tailscale: TailscaleStatus;
  export let device: DeviceInfo;
  export let showLogs = false;
  export let logLines: string[] = [];
  export let onToggleLogs: () => void | Promise<void>;
</script>

<div class="settings-grid">
  <article class="card">
    <h3>Identity &amp; network</h3>
    <div class="info-grid">
      <div class="info-item">
        <span class="info-label">Fingerprint</span>
        <span class="info-value mono selectable">{shortFingerprint(device.fingerprint)}</span>
      </div>
      <div class="info-item">
        <span class="info-label">Port</span>
        <span class="info-value mono">{device.port || 24831}</span>
      </div>
      <div class="info-item">
        <span class="info-label">Tailscale</span>
        <span class="info-value">{tailscale.available ? (tailscale.connected ? 'Connected' : 'Disconnected') : 'Not installed'}</span>
      </div>
      <div class="info-item">
        <span class="info-label">Tailscale node</span>
        <span class="info-value">{tailscale.available ? (tailscale.selfName || 'N/A') : '-'}</span>
      </div>
    </div>
  </article>

  <article class="card">
    <div class="log-header">
      <h3>Logs</h3>
      <button class="btn btn-secondary" on:click={onToggleLogs}>
        {showLogs ? 'Hide' : 'View logs'}
      </button>
    </div>
    {#if showLogs}
      <div class="log-viewer">
        {#if logLines.length === 0}
          <span class="hint">No log entries yet.</span>
        {:else}
          {#each logLines as line}
            <div class="log-line {line.includes('ERROR') || line.includes('PANIC') ? 'log-line-error' : line.includes('WARN') ? 'log-line-warn' : 'log-line-info'}">{line}</div>
          {/each}
        {/if}
      </div>
    {:else}
      <p class="hint">Runtime logs for troubleshooting discovery, transport, or audio issues.</p>
    {/if}
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

  .info-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.6rem;
  }

  .info-item {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding: 0.75rem;
    border-radius: var(--radius-md);
    background: var(--panel-strong);
  }

  .info-label {
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .info-value {
    font-size: 0.88rem;
    color: var(--text-strong);
    font-weight: 600;
  }

  .log-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  .mono {
    font-family: var(--font-mono);
    font-size: 0.82rem;
  }

  @media (max-width: 960px) {
    .settings-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 640px) {
    .info-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
