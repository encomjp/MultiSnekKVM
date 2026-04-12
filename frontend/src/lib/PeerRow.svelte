<script>
  import { onDestroy } from 'svelte';
  import { timeAgo, preferredRouteLabel, orderedRoutes, routeLabel, shortFingerprint, peerAddresses, trustLabel, copyToClipboard } from './utils';

  export let peer;
  export let session;
  export let onConnect;
  export let onDisconnect;
  export let onRemove;

  let connecting = false;
  let removing = false;
  let copiedToken = '';
  let copiedTimer = null;

  $: isCurrent = session.connected && session.peerID === peer.id;
  $: if (!session.connected) connecting = false;

  async function handleConnect() {
    connecting = true;
    try {
      await onConnect(peer.address);
    } catch (_) {}
    connecting = false;
  }

  async function handleDisconnect() {
    try { await onDisconnect(); } catch (_) {}
  }

  async function handleRemove() {
    removing = true;
    try {
      await onRemove(peer.address);
    } catch (_) {}
    removing = false;
  }

  async function copyValue(token, value) {
    try {
      await copyToClipboard(value);
      copiedToken = token;
      clearTimeout(copiedTimer);
      copiedTimer = setTimeout(() => { copiedToken = ''; copiedTimer = null; }, 1600);
    } catch (_) {}
  }

  onDestroy(() => {
	clearTimeout(copiedTimer);
	copiedTimer = null;
  });
</script>

<article class="peer-row" class:is-current={isCurrent}>
  <div class="peer-row-main">
    <div class="peer-row-head">
      <div>
        <div class="peer-title-line">
          <h4>{peer.name}</h4>
          <span class="peer-source">{peer.source}</span>
          <span class:trust-chip-ready={peer.trusted} class="trust-chip">{trustLabel(peer)}</span>
        </div>
        <p>{timeAgo(peer.lastSeen)} • preferred route {preferredRouteLabel(peer.preferredRoute)}</p>
      </div>

      <div class="chip-row chip-row-tight">
        {#each orderedRoutes(peer.routes || []) as route}
          <span class="route-chip" class:route-lan={route === 'lan'} class:route-tailnet={route === 'tailscale'} class:route-manual={route === 'manual'}>{routeLabel(route)}</span>
        {/each}
      </div>
    </div>

    <div class="peer-details-grid">
      <div>
        <span class="meta-label">Primary address</span>
        <span class="detail-action-row">
          <span class="meta-value mono selectable">{peer.address}</span>
          <button class="inline-copy" on:click={() => copyValue(`addr-${peer.id}`, peer.address)}>
            {copiedToken === `addr-${peer.id}` ? 'Copied' : 'Copy'}
          </button>
        </span>
      </div>
      <div>
        <span class="meta-label">All addresses</span>
        <span class="meta-value mono selectable selectable-wrap">{peerAddresses(peer)}</span>
      </div>
      <div>
        <span class="meta-label">Fingerprint</span>
        <span class="detail-action-row">
          <span class="meta-value mono selectable">{shortFingerprint(peer.fingerprint)}</span>
          {#if peer.fingerprint}
            <button class="inline-copy" on:click={() => copyValue(`fp-${peer.id}`, peer.fingerprint)}>
              {copiedToken === `fp-${peer.id}` ? 'Copied' : 'Copy'}
            </button>
          {/if}
        </span>
      </div>
      <div>
        <span class="meta-label">Availability</span>
        <span class="meta-value">{peer.status}</span>
      </div>
    </div>
  </div>

  <div class="peer-actions">
    {#if isCurrent}
      <button class="button button-danger" on:click={handleDisconnect}>Disconnect</button>
    {:else}
      <button
        class="button button-primary"
        on:click={handleConnect}
        disabled={connecting || session.connected}
      >
        {connecting ? 'Connecting...' : peer.trusted ? 'Connect' : 'Pair & Connect'}
      </button>
    {/if}

    {#if peer.source === 'manual'}
      <button class="button button-secondary" on:click={handleRemove} disabled={removing}>
        {removing ? 'Removing...' : 'Remove'}
      </button>
    {/if}
  </div>
</article>

<style>
  .peer-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.95rem;
    padding: 1rem;
    border-radius: 1.05rem;
    border: 1px solid rgba(126, 145, 168, 0.14);
    background: rgba(255, 255, 255, 0.03);
  }
  .peer-row.is-current {
    border-color: rgba(99, 211, 173, 0.28);
    box-shadow: inset 0 0 0 1px rgba(99, 211, 173, 0.14);
  }
  .peer-row-main {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
  }
  .peer-row-head {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.8rem;
  }
  .peer-title-line {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }
  .peer-title-line h4 {
    font-size: 1rem;
    font-weight: 620;
    letter-spacing: -0.03em;
  }
  .peer-row-head p {
    margin-top: 0.28rem;
    color: var(--text-secondary);
  }
  .peer-details-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.8rem;
  }
  .peer-actions {
    display: flex;
    align-items: center;
    gap: 0.65rem;
  }
</style>
