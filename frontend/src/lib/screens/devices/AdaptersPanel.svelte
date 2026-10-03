<script lang="ts">
  import { getAppState } from '../../stores/app.svelte';
  import { interfaceLabel } from '../../utils';
  import Icon from '../../ui/Icon.svelte';

  const app = getAppState();
  const conn = app.connection;

  const KIND_ORDER: Record<string, number> = { usb4: 0, 'usb-bridge': 1, ethernet: 2, wifi: 3, bluetooth: 4, network: 5 };
  const adapters = $derived(
    [...conn.interfaces].sort((a, b) => (KIND_ORDER[a.kind] ?? 9) - (KIND_ORDER[b.kind] ?? 9)),
  );
</script>

<aside class="adapters card" aria-labelledby="adapters-title">
  <div class="card-head">
    <h2 id="adapters-title">This PC's adapters</h2>
    <button
      type="button"
      class="btn btn-icon btn-sm"
      aria-label="Refresh adapters"
      disabled={conn.interfacesLoading}
      onclick={() => conn.refreshInterfaces()}
    >
      <Icon name="refresh" />
    </button>
  </div>

  {#if adapters.length === 0}
    <p class="muted small">
      {conn.interfacesLoading
        ? 'Reading adapters…'
        : 'No usable network adapters found. Connect a cable or network, then refresh.'}
    </p>
  {:else}
    <ul class="adapter-list">
      {#each adapters as adapter (adapter.name)}
        <li class="adapter">
          <span class="adapter-row">
            <span class="adapter-kind">{interfaceLabel(adapter.kind)}</span>
            <span class="mono small selectable">{adapter.addresses[0] ?? '—'}</span>
          </span>
          <span class="adapter-desc">
            {adapter.description && adapter.description !== adapter.name
              ? `${adapter.description} · ${adapter.name}`
              : adapter.name}
          </span>
          {#if adapter.addresses.length > 1}
            <span class="mono adapter-more selectable">{adapter.addresses.slice(1).join(' · ')}</span>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  <details class="guide">
    <summary>Connect over USB4 or Bluetooth</summary>
    <ul class="guide-list">
      <li>
        USB4 / Thunderbolt: plug both PCs together. Windows creates a network adapter with a 169.254.x.x address;
        MultiSnek finds it and prefers it.
      </li>
      <li>
        Bluetooth (direct): pair the PCs in Windows Bluetooth settings and turn on Bluetooth above. MultiSnek finds
        paired PCs on its own, no network needed. Fine for input and compressed audio; slow for files.
      </li>
      <li>
        Bluetooth PAN: Bluetooth tethering that shows up as a network adapter in the list above. Works too, but the
        direct link is simpler.
      </li>
      <li>USB bridge cable: works only if its driver exposes a network adapter.</li>
      <li>A plain USB-C cable between two PCs is not a network.</li>
    </ul>
  </details>
  <p class="muted small">
    MultiSnek uses TCP <span class="mono">{conn.device.port}</span>. Allow it in Windows Firewall on new networks.
  </p>
</aside>

<style>
  .adapters {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .adapter-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .adapter {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 10px 12px;
    border-radius: var(--radius);
    background: var(--panel2);
  }

  .adapter-row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
  }

  .adapter-kind {
    font-weight: 500;
  }

  .adapter-desc,
  .adapter-more {
    font-size: 12px;
    color: var(--muted);
    overflow-wrap: anywhere;
  }

  .guide {
    border-top: 1px solid var(--line);
    padding-top: 12px;
  }

  .guide summary {
    cursor: pointer;
    font-weight: 500;
    min-height: 32px;
    display: flex;
    align-items: center;
  }

  .guide-list {
    list-style: disc;
    margin: 8px 0 0;
    padding-left: 18px;
    color: var(--muted);
    font-size: 13px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
</style>
