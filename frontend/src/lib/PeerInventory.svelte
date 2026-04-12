<script>
  import PeerRow from './PeerRow.svelte';

  export let peers;
  export let trustedPeerCount;
  export let lanPeerCount;
  export let tailscalePeerCount;
  export let manualPeerCount;
  export let session;
  export let onConnect;
  export let onDisconnect;
  export let onRemove;
  export let onAdd;

  let newPeerAddr = '';
  let addError = '';

  async function handleAdd() {
    const addr = newPeerAddr.trim();
    if (!addr) {
      return;
    }
    const portMatch = addr.match(/:(\d+)$/);
    if (portMatch) {
      const port = parseInt(portMatch[1], 10);
      if (port < 1 || port > 65535) {
        addError = 'Port must be between 1 and 65535.';
        return;
      }
    }
    addError = '';
    try {
      await onAdd(addr);
      newPeerAddr = '';
    } catch (error) {
      addError = error?.message || 'Failed to add peer';
    }
  }
</script>

<article class="panel peer-panel">
  <div class="panel-heading panel-heading-stack">
    <div>
      <span class="eyebrow">Peer inventory</span>
      <h3>Trusted endpoints</h3>
    </div>

    <div class="chip-row">
      <span class="mini-chip">{peers.length} total</span>
      <span class="mini-chip">{trustedPeerCount} pinned</span>
      <span class="mini-chip">{lanPeerCount} LAN</span>
      <span class="mini-chip">{tailscalePeerCount} Tailnet</span>
      <span class="mini-chip">{manualPeerCount} Manual</span>
    </div>
  </div>

  <form class="manual-form" on:submit|preventDefault={handleAdd}>
    <div class="manual-copy">
      <label for="peer-address">Manual endpoint</label>
      <p>Add a host, IPv4, IPv6, or hostname with an optional port.</p>
    </div>
    <input id="peer-address" type="text" bind:value={newPeerAddr} placeholder="192.168.1.50:24831 or [fd7a:115c:a1e0::5]:24831" />
    <button class="button button-primary" type="submit">Add Endpoint</button>
  </form>

  {#if addError}
    <div class="notice notice-inline">{addError}</div>
  {/if}

  {#if peers.length === 0}
    <div class="empty-state">
      <strong>No peers available yet</strong>
      <p>Start MultiSnekKVM on another workstation or add a manual endpoint to seed the trust graph.</p>
    </div>
  {:else}
    <div class="peer-list">
      {#each peers as peer (peer.id)}
        <PeerRow {peer} {session} {onConnect} {onDisconnect} {onRemove} />
      {/each}
    </div>
  {/if}
</article>

<style>
  .peer-panel { padding: 1.1rem; }
  .manual-form {
    display: grid;
    grid-template-columns: minmax(13rem, 0.8fr) minmax(0, 1fr) auto;
    gap: 0.8rem;
    align-items: end;
    margin-bottom: 1rem;
  }
  .manual-copy {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  .peer-list {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  @media (max-width: 1180px) {
    .manual-form { grid-template-columns: 1fr; }
  }
</style>
