<script lang="ts">
  import { orderedRoutes, preferredRouteLabel, routeLabel, timeAgo } from './utils';
  import type { Peer } from './types';

  export let peerFilter: 'all' | 'online' | 'trusted' | 'manual' = 'all';
  export let newPeerAddr = '';
  export let addError = '';
  export let peers: Peer[] = [];
  export let filteredPeers: Peer[] = [];
  export let onlinePeerCount = 0;
  export let trustedPeerCount = 0;
  export let manualPeerCount = 0;
  export let sessionConnected = false;
  export let sessionPeerID = '';
  export let connectingAddress = '';
  export let removingAddress = '';
  export let forgettingPeerID = '';
  export let onAdd: () => void | Promise<void>;
  export let onConnect: (address: string, needsTrust: boolean) => void | Promise<void>;
  export let onDisconnect: () => void | Promise<void>;
  export let onRemove: (address: string) => void | Promise<void>;
  export let onForgetTrust: (peer: Peer) => void | Promise<void>;

  function sourceLabel(source?: string) {
    if (!source) return 'Discovered';
    if (source === 'hybrid') return 'Hybrid';
    if (source === 'tailscale') return 'Tailnet';
    if (source === 'lan') return 'LAN';
    if (source === 'manual') return 'Manual';
    return source.charAt(0).toUpperCase() + source.slice(1);
  }
</script>

<section class="screen" data-screen="devices">
  <div class="toolbar">
    <div class="filter-strip" role="tablist" aria-label="Device filters">
      <button type="button" class="filter-chip" class:active={peerFilter === 'all'} on:click={() => peerFilter = 'all'}>
        All <span>{peers.length}</span>
      </button>
      <button type="button" class="filter-chip" class:active={peerFilter === 'online'} on:click={() => peerFilter = 'online'}>
        Online <span>{onlinePeerCount}</span>
      </button>
      <button type="button" class="filter-chip" class:active={peerFilter === 'trusted'} on:click={() => peerFilter = 'trusted'}>
        Trusted <span>{trustedPeerCount}</span>
      </button>
      <button type="button" class="filter-chip" class:active={peerFilter === 'manual'} on:click={() => peerFilter = 'manual'}>
        Manual <span>{manualPeerCount}</span>
      </button>
    </div>

    <form class="add-form" on:submit|preventDefault={onAdd}>
      <input class="input add-input" type="text" bind:value={newPeerAddr} placeholder="Add device — IP or hostname" />
      <button type="submit" class="btn btn-primary">Add</button>
    </form>
  </div>

  {#if addError}
    <p class="inline-error">{addError}</p>
  {/if}

  {#if filteredPeers.length === 0}
    <div class="empty-state">
      <svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>
      <p>{peerFilter === 'all' ? 'No devices yet. Add one above or start MultiSnek on another computer.' : 'No devices match this filter.'}</p>
    </div>
  {:else}
    <div class="device-list">
      {#each filteredPeers as peer (peer.id)}
        <article class="device-row" class:is-active={sessionConnected && sessionPeerID === peer.id}>
          <div class="device-main">
            <div class="device-name-row">
              <span class="status-dot" class:online={peer.status === 'online'}></span>
              <h2>{peer.name || 'Unknown device'}</h2>
              {#if sessionConnected && sessionPeerID === peer.id}
                <span class="badge session-badge">Active</span>
              {/if}
            </div>
            <p class="device-address mono selectable">{peer.address || '—'}</p>
          </div>

          <div class="device-badges">
            <span class="badge" class:trusted-badge={peer.trusted}>{peer.trusted ? 'Trusted' : 'PIN required'}</span>
            <div class="route-stack">
              {#each orderedRoutes(peer.routes || []) as route}
                <span class="route-pill route-pill-{route === 'tailscale' ? 'tailnet' : route}">{routeLabel(route)}</span>
              {/each}
            </div>
          </div>

          <div class="device-actions">
            {#if sessionConnected && sessionPeerID === peer.id}
              <button class="btn btn-danger" on:click={onDisconnect}>Disconnect</button>
            {:else}
              <button
                class="btn {peer.trusted ? 'btn-primary' : 'btn-secondary'}"
                on:click={() => onConnect(peer.address, !peer.trusted)}
                disabled={!peer.address || !!connectingAddress || sessionConnected}
              >
                {#if connectingAddress === peer.address}<span class="btn-spinner"></span>{/if}
                {connectingAddress === peer.address ? 'Connecting…' : peer.trusted ? 'Connect' : 'Pair & Connect'}
              </button>
            {/if}
            {#if peer.trusted}
              <button
                class="btn btn-ghost"
                on:click={() => onForgetTrust(peer)}
                disabled={forgettingPeerID === peer.id || (sessionConnected && sessionPeerID === peer.id)}
              >
                {forgettingPeerID === peer.id ? 'Untrusting…' : 'Untrust'}
              </button>
            {/if}
            {#if peer.source === 'manual'}
              <button class="btn btn-ghost" on:click={() => onRemove(peer.address)} disabled={removingAddress === peer.address}>
                {removingAddress === peer.address ? '…' : 'Remove'}
              </button>
            {/if}
          </div>
        </article>
      {/each}
    </div>
  {/if}
</section>

<style>
  .screen {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
  }

  .filter-strip {
    display: flex;
    gap: 0.4rem;
    flex-wrap: wrap;
  }

  .filter-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-secondary);
    padding: 0.42rem 0.82rem;
    font-weight: 600;
    font-size: 0.88rem;
    transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
  }

  .filter-chip span {
    display: inline-flex;
    min-width: 1.4rem;
    justify-content: center;
    border-radius: 999px;
    background: var(--panel-strong);
    padding: 0.1rem 0.38rem;
    color: var(--text-strong);
    font-size: 0.76rem;
  }

  .filter-chip.active {
    border-color: var(--accent-border);
    color: var(--text-strong);
    background: var(--panel-accent);
  }

  .add-form {
    display: flex;
    gap: 0.6rem;
    align-items: center;
    flex: 0 1 auto;
    min-width: 0;
  }

  .add-input {
    width: 22rem;
    max-width: 100%;
    min-height: 2.5rem;
  }

  .inline-error {
    color: var(--danger);
    font-size: 0.86rem;
    margin-top: -0.4rem;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.75rem;
    padding: 4rem 1.5rem;
    color: var(--text-muted);
    text-align: center;
  }

  .empty-state p {
    color: var(--text-secondary);
    max-width: 30rem;
    line-height: 1.6;
  }

  .device-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .device-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto auto;
    gap: 1rem;
    align-items: center;
    padding: 0.9rem 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
    background: var(--panel);
  }

  .device-row.is-active {
    border-color: var(--accent-border);
    background: var(--panel-accent);
  }

  .device-main {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .device-name-row {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    flex-wrap: wrap;
  }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--text-muted);
    flex-shrink: 0;
  }

  .status-dot.online {
    background: var(--success);
  }

  .device-main h2 {
    font-size: 0.98rem;
    font-weight: 650;
    letter-spacing: -0.01em;
  }

  .device-address {
    font-size: 0.8rem;
    color: var(--text-muted);
    padding-left: 1.1rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .device-badges {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .badge {
    display: inline-flex;
    align-items: center;
    padding: 0.2rem 0.55rem;
    border-radius: 999px;
    background: var(--panel-strong);
    color: var(--text-secondary);
    font-size: 0.74rem;
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  .trusted-badge {
    background: var(--success-bg);
    color: var(--success);
  }

  .session-badge {
    background: var(--panel-accent);
    color: var(--accent);
  }

  .route-stack {
    display: flex;
    gap: 0.35rem;
    flex-wrap: wrap;
  }

  .route-pill {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.18rem 0.52rem;
    font-size: 0.72rem;
    font-weight: 700;
    border: 1px solid var(--border);
    color: var(--text-secondary);
    background: var(--panel-strong);
  }

  .route-pill-lan {
    color: var(--success);
    border-color: rgba(89, 177, 129, 0.24);
    background: var(--success-bg);
  }

  .route-pill-tailnet {
    color: var(--accent);
    border-color: rgba(55, 126, 255, 0.24);
    background: var(--panel-accent);
  }

  .route-pill-manual {
    color: var(--warning);
    border-color: rgba(208, 150, 72, 0.26);
    background: var(--warning-bg);
  }

  .device-actions {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-shrink: 0;
  }

  .mono {
    font-family: var(--font-mono);
  }

  @media (max-width: 860px) {
    .device-row {
      grid-template-columns: 1fr;
    }
    .device-actions {
      justify-content: flex-start;
    }
    .add-input {
      width: 100%;
    }
    .toolbar {
      flex-direction: column;
      align-items: flex-start;
    }
    .add-form {
      width: 100%;
    }
  }
</style>
