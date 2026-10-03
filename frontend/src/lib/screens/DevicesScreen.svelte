<script lang="ts">
  import { getAppState } from '../stores/app.svelte';
  import { isManualPeer, isPeerOnline, pluralize } from '../utils';
  import Icon from '../ui/Icon.svelte';
  import AdaptersPanel from './devices/AdaptersPanel.svelte';
  import PeerCard from './devices/PeerCard.svelte';

  type Filter = 'all' | 'online' | 'paired' | 'manual';

  const app = getAppState();
  const conn = app.connection;

  let filter = $state<Filter>('all');
  let newAddress = $state('');
  let addError = $state('');
  let adding = $state(false);

  const counts = $derived({
    all: conn.peers.length,
    online: conn.peers.filter(isPeerOnline).length,
    paired: conn.peers.filter((peer) => peer.trusted).length,
    manual: conn.peers.filter(isManualPeer).length,
  });

  const FILTERS: ReadonlyArray<{ id: Filter; label: string }> = [
    { id: 'all', label: 'All' },
    { id: 'online', label: 'Online' },
    { id: 'paired', label: 'Paired' },
    { id: 'manual', label: 'Manual' },
  ];

  const visible = $derived(
    conn.peers
      .filter((peer) => {
        if (filter === 'online') return isPeerOnline(peer);
        if (filter === 'paired') return peer.trusted;
        if (filter === 'manual') return isManualPeer(peer);
        return true;
      })
      // Active session first, then online, then by name.
      .sort((a, b) => {
        const rank = (p: typeof a) =>
          (conn.session.connected && conn.session.peerID === p.id ? 0 : 2) + (isPeerOnline(p) ? 0 : 1);
        return rank(a) - rank(b) || (a.name || a.address).localeCompare(b.name || b.address);
      }),
  );

  async function add(event: SubmitEvent) {
    event.preventDefault();
    if (adding) return;
    adding = true;
    addError = await conn.addPeer(newAddress);
    adding = false;
    if (!addError) newAddress = '';
  }
</script>

<div class="screen devices">
  <header class="screen-header">
    <div>
      <h1>Devices</h1>
      <p>
        {pluralize(counts.all, 'known device')} · {counts.online} online. The best route is chosen automatically; pick
        another if you need to.
      </p>
    </div>
    <form class="add-form" onsubmit={add} novalidate>
      <div class="add-row">
        <label for="add-peer" class="sr-only">Peer IP address or hostname</label>
        <input
          id="add-peer"
          class="input mono add-input"
          type="text"
          placeholder="IP or hostname, e.g. 192.168.0.42"
          autocomplete="off"
          spellcheck="false"
          bind:value={newAddress}
          aria-invalid={addError ? 'true' : undefined}
          aria-describedby={addError ? 'add-peer-error' : undefined}
          oninput={() => (addError = '')}
        />
        <button type="submit" class="btn btn-primary" disabled={!newAddress.trim() || adding}>
          <Icon name="plus" />Add device
        </button>
      </div>
      {#if addError}<p id="add-peer-error" class="field-error">{addError}</p>{/if}
    </form>
  </header>

  <div role="group" aria-label="Filter devices" class="filters">
    {#each FILTERS as item (item.id)}
      <button
        type="button"
        class="filter"
        class:on={filter === item.id}
        aria-pressed={filter === item.id}
        onclick={() => (filter = item.id)}
      >
        {item.label} <span class="count mono">{counts[item.id]}</span>
      </button>
    {/each}
  </div>

  <div class="layout">
    {#if visible.length === 0}
      <div class="empty card">
        <Icon name="devices" size={28} />
        <p>
          {filter === 'all'
            ? 'No devices yet. Start MultiSnek on the other PC, or add it by IP address above.'
            : 'No devices match this filter.'}
        </p>
      </div>
    {:else}
      <ul class="peer-list" aria-label="Devices">
        {#each visible as peer (peer.id)}
          <PeerCard {peer} />
        {/each}
      </ul>
    {/if}
    <AdaptersPanel />
  </div>
</div>

<style>
  .add-form {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .add-row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .add-input {
    width: 260px;
    max-width: 100%;
    font-size: 13px;
  }

  .filters {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }

  .filter {
    min-height: 36px;
    padding: 0 14px;
    border-radius: 999px;
    font-size: 13px;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text);
  }

  .filter:hover:not(.on) {
    border-color: var(--line-strong);
  }

  .filter.on {
    border-color: var(--text);
    background: var(--text);
    color: var(--bg);
    font-weight: 600;
  }

  .count {
    font-size: 12px;
    opacity: 0.8;
  }

  .layout {
    display: flex;
    flex-wrap: wrap;
    gap: 20px;
    align-items: flex-start;
  }

  .peer-list,
  .empty {
    flex: 999 1 460px;
    min-width: 0;
  }

  .peer-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 36px 18px;
    color: var(--muted);
    text-align: center;
  }
</style>
