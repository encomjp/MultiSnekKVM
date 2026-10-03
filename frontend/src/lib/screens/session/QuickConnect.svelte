<script lang="ts">
  import { getAppState } from '../../stores/app.svelte';
  import { addressLabel, isPeerOnline } from '../../utils';
  import Card from '../../ui/Card.svelte';
  import Icon from '../../ui/Icon.svelte';
  import StatusDot from '../../ui/StatusDot.svelte';

  const app = getAppState();
  const conn = app.connection;

  const candidates = $derived(
    [...conn.peers]
      .filter((peer) => peer.trusted)
      .sort((a, b) => Number(isPeerOnline(b)) - Number(isPeerOnline(a)))
      .slice(0, 4),
  );
</script>

<Card title="Quick connect" description="Paired devices, online first.">
  {#snippet actions()}
    <button type="button" class="link-btn" onclick={() => app.navigate('devices')}>All devices</button>
  {/snippet}
  {#if candidates.length === 0}
    <p class="muted">No paired devices yet. Open Devices and use Pair &amp; connect with the other PC's PIN.</p>
  {:else}
    <ul class="quick-list">
      {#each candidates as peer (peer.id)}
        {@const online = isPeerOnline(peer)}
        {@const address = conn.addressFor(peer)}
        <li class="quick-row">
          <span class="quick-icon"><Icon name="devices" /></span>
          <div class="quick-text">
            <span class="quick-name">{peer.name || peer.address}</span>
            <span class="quick-sub">
              <StatusDot tone={online ? 'ok' : 'off'} size={6} />
              {online ? 'Online' : 'Offline'} · {addressLabel(peer, address)}
            </span>
          </div>
          <button
            type="button"
            class="btn btn-sm"
            class:btn-primary={online}
            disabled={!!conn.connecting}
            aria-label="Connect to {peer.name || peer.address}"
            onclick={() => conn.connect(peer)}
          >
            {#if conn.connecting === address}<span class="spinner" aria-hidden="true"></span>Connecting…{:else}Connect{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</Card>

<style>
  .quick-list {
    display: flex;
    flex-direction: column;
  }

  .quick-row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 0;
    border-bottom: 1px solid var(--line);
  }

  .quick-row:last-child {
    border-bottom: 0;
  }

  .quick-icon {
    width: 32px;
    height: 32px;
    flex: none;
    border-radius: var(--radius);
    background: var(--panel2);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .quick-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .quick-name {
    font-weight: 500;
  }

  .quick-sub {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--muted);
  }
</style>
