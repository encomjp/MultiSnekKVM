<script lang="ts">
  import { getAppState } from '../../stores/app.svelte';
  import { pluralize, timeAgo } from '../../utils';
  import Icon from '../../ui/Icon.svelte';
  import Toggle from '../../ui/Toggle.svelte';

  const app = getAppState();
  const bt = app.bluetooth;

  const status = $derived(bt.status);
  const scanning = $derived(bt.scanning);

  const stateLine = $derived.by(() => {
    if (!status.available) return 'Not available on this PC';
    if (!status.enabled) return 'Off';
    if (scanning) return 'Scanning…';
    if (!status.listening) return 'On · not listening';
    const count = status.devices.length;
    return count > 0 ? `Ready · ${pluralize(count, 'paired PC')} running MultiSnek` : 'Ready · no paired PCs found yet';
  });

  const canRefresh = $derived(status.available && status.enabled && !scanning);
</script>

<section class="bluetooth card" aria-labelledby="bluetooth-title">
  <div class="card-head">
    <h2 id="bluetooth-title">Bluetooth</h2>
    <button
      type="button"
      class="btn btn-sm"
      disabled={!canRefresh}
      aria-busy={scanning || undefined}
      onclick={() => bt.refresh()}
    >
      {#if scanning}<span class="spinner" aria-hidden="true"></span>{:else}<Icon name="refresh" />{/if}Refresh
    </button>
  </div>

  <p class="state" role="status" data-state={!status.available ? 'unavailable' : !status.enabled ? 'off' : 'on'}>
    <Icon name="bluetooth" />
    <span>{stateLine}</span>
  </p>
  {#if status.error}<p class="field-error" role="alert">{status.error}</p>{/if}

  <Toggle
    label="Use Bluetooth"
    checked={status.enabled}
    disabled={!status.available}
    pending={bt.pending}
    error={bt.error}
    onChange={(value) => bt.setEnabled(value)}
  />

  {#if status.enabled && status.devices.length > 0}
    <ul class="found" aria-label="Paired PCs found over Bluetooth">
      {#each status.devices as device (device.address)}
        <li class="found-item">
          <span class="found-name">{device.name || device.deviceId || 'Unnamed PC'}</span>
          <span class="mono small found-addr selectable">{device.address}</span>
        </li>
      {/each}
    </ul>
    {#if status.lastScan}
      <p class="muted small">Last scan {timeAgo(status.lastScan)}.</p>
    {/if}
  {/if}

  <p class="muted small">
    Pair both PCs in Windows Bluetooth settings first; MultiSnek then finds each other automatically. Best for
    keyboard, mouse and compressed audio; large file transfers are slow.
  </p>
</section>

<style>
  .bluetooth {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-width: 0;
  }

  .state {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0;
    font-weight: 500;
  }

  .state :global(svg) {
    flex: none;
    color: var(--accent);
  }

  .state[data-state='off'] :global(svg),
  .state[data-state='unavailable'] :global(svg) {
    color: var(--muted);
  }

  .found {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .found-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 10px 12px;
    border-radius: var(--radius);
    background: var(--panel2);
    min-width: 0;
  }

  .found-name {
    font-weight: 500;
    overflow-wrap: anywhere;
  }

  .found-addr {
    color: var(--muted);
    overflow-wrap: anywhere;
  }
</style>
