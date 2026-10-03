<script lang="ts">
  import { getAppState } from '../../stores/app.svelte';
  import { groupedFingerprint, shortFingerprint, timeAgo } from '../../utils';
  import Card from '../../ui/Card.svelte';
  import Icon from '../../ui/Icon.svelte';
  import StatusDot from '../../ui/StatusDot.svelte';

  const app = getAppState();
  const conn = app.connection;

  const paired = $derived(conn.peers.filter((peer) => peer.trusted));
  const ts = $derived(conn.tailscale);

  async function copy(value: string, what: string) {
    try {
      await navigator.clipboard?.writeText(value);
      app.toasts.success(`${what} copied.`);
    } catch {
      app.toasts.error("Couldn't copy to the clipboard.");
    }
  }
</script>

<div class="stack">
  <Card title="This PC" description="Other PCs check this certificate fingerprint every time they connect.">
    <dl class="facts">
      <div>
        <dt>Name</dt>
        <dd>{conn.device.name || '—'}</dd>
      </div>
      <div>
        <dt>Device ID</dt>
        <dd class="mono selectable">{conn.device.id || '—'}</dd>
      </div>
      <div class="wide">
        <dt>Certificate fingerprint</dt>
        <dd class="fp">
          <span class="mono selectable">{groupedFingerprint(conn.device.fingerprint) || 'pending'}</span>
          {#if conn.device.fingerprint}
            <button
              type="button"
              class="btn btn-sm btn-icon"
              aria-label="Copy certificate fingerprint"
              onclick={() => copy(conn.device.fingerprint, 'Fingerprint')}
            >
              <Icon name="copy" />
            </button>
          {/if}
        </dd>
      </div>
      <div>
        <dt>Listening port</dt>
        <dd class="mono">TCP {conn.device.port}</dd>
      </div>
      <div>
        <dt>Pairing PIN</dt>
        <dd>Shown under This PC in the sidebar</dd>
      </div>
    </dl>
    <p class="notice">
      <Icon name="shield" />
      <span>
        Connections use TLS 1.3 with pinned certificates. First-time pairing uses a PIN-based SPAKE2 exchange; the PIN
        itself is never sent over the network.
      </span>
    </p>
  </Card>

  <Card title="Paired devices" description="These PCs can connect without a PIN. Untrust one to require pairing again.">
    {#if paired.length === 0}
      <p class="muted small">No paired devices.</p>
    {:else}
      <ul class="paired">
        {#each paired as peer (peer.id)}
          {@const busy = conn.busyPeers[peer.id] === 'untrust'}
          {@const active = conn.session.connected && conn.session.peerID === peer.id}
          <li class="paired-row">
            <Icon name="lock" />
            <div class="paired-text">
              <span class="paired-name">{peer.name || peer.address}</span>
              <span class="muted small">
                <span class="mono">{shortFingerprint(peer.fingerprint)}</span> · last seen {timeAgo(peer.lastSeen)}
              </span>
            </div>
            <button
              type="button"
              class="btn btn-sm btn-ghost"
              disabled={busy || active}
              title={active ? 'Disconnect first' : undefined}
              aria-label="Untrust {peer.name || peer.address}"
              onclick={() => conn.untrust(peer)}>{busy ? 'Untrusting…' : 'Untrust'}</button
            >
          </li>
        {/each}
      </ul>
    {/if}
  </Card>

  <Card title="Tailscale" description="Used to find and reach your devices away from the local network.">
    {#snippet actions()}
      <span class="ts-state">
        <StatusDot tone={!ts.available ? 'off' : ts.connected ? 'ok' : 'warn'} />
        {!ts.available ? 'Not installed' : ts.connected ? 'Connected' : ts.backendState || 'Not connected'}
      </span>
    {/snippet}
    {#if ts.available}
      <dl class="facts">
        <div>
          <dt>Node</dt>
          <dd class="mono">{ts.selfName || '—'}</dd>
        </div>
        <div>
          <dt>Tailnet</dt>
          <dd class="mono">{ts.tailnet || '—'}</dd>
        </div>
        <div>
          <dt>Addresses</dt>
          <dd class="mono selectable">{ts.selfIPs.join(', ') || '—'}</dd>
        </div>
        <div>
          <dt>Peers</dt>
          <dd>{ts.peerCount} on the tailnet · {ts.targetCount} running MultiSnek</dd>
        </div>
      </dl>
      {#if ts.lastError}<p class="field-error">{ts.lastError}</p>{/if}
    {:else}
      <p class="muted small">Install Tailscale to connect to your PCs from anywhere. LAN and direct links work without it.</p>
    {/if}
  </Card>
</div>

<style>
  .stack {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .facts {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px 18px;
    margin: 0;
  }

  .facts .wide {
    grid-column: 1 / -1;
  }

  dt {
    font-size: 12px;
    color: var(--muted);
  }

  dd {
    margin: 2px 0 0;
    overflow-wrap: anywhere;
  }

  .fp {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .paired {
    display: flex;
    flex-direction: column;
  }

  .paired-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 0;
    border-bottom: 1px solid var(--line);
    color: var(--muted);
  }

  .paired-row:last-child {
    border-bottom: 0;
  }

  .paired-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    color: var(--text);
  }

  .paired-name {
    font-weight: 500;
  }

  .ts-state {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--muted);
  }

  @media (max-width: 700px) {
    .facts {
      grid-template-columns: 1fr;
    }
  }
</style>
