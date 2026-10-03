<script lang="ts">
  import { getAppState } from './stores/app.svelte';
  import { formatPin } from './utils';
  import StatusDot from './ui/StatusDot.svelte';

  const app = getAppState();
  const conn = app.connection;

  let showPin = $state(false);

  const pin = $derived(conn.device.pairingCode ?? '');
  const status = $derived.by(() => {
    if (!app.ready) return { label: 'Starting…', tone: 'off' as const };
    if (conn.isControlled) return { label: 'In use', tone: 'info' as const };
    if (conn.session.connected) return { label: 'In session', tone: 'ok' as const };
    return { label: 'Listening', tone: 'ok' as const };
  });
</script>

<section class="this-pc" aria-label="This PC">
  <div class="row">
    <span class="caption">This PC</span>
    <span class="status" class:muted-status={status.tone === 'off'} class:info={status.tone === 'info'}>
      <StatusDot tone={status.tone} size={6} />{status.label}{#if status.label === 'Listening'}<span class="mono port">:{conn.device.port}</span>{/if}
    </span>
  </div>
  <span class="name">{conn.device.name || 'This computer'}</span>
  {#if pin}
    <div class="pin-row">
      <div class="pin-block">
        <span class="caption">Pairing PIN</span>
        <span class="pin mono">
          {#if showPin}{formatPin(pin)}{:else}<span aria-hidden="true">••• •••</span><span class="sr-only">hidden</span>{/if}
        </span>
      </div>
      <button
        type="button"
        class="btn btn-sm pin-toggle"
        aria-label={showPin ? 'Hide pairing PIN' : 'Show pairing PIN'}
        onclick={() => (showPin = !showPin)}>{showPin ? 'Hide' : 'Show'}</button
      >
    </div>
  {/if}
</section>

<style>
  .this-pc {
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    background: var(--panel2);
  }

  .row,
  .pin-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .caption {
    font-size: 12px;
    color: var(--muted);
  }

  .status {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--accent);
  }

  .status.info {
    color: var(--info);
  }

  .status.muted-status {
    color: var(--muted);
  }

  .port {
    margin-left: -4px;
    color: var(--muted);
  }

  .name {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .pin-block {
    display: flex;
    flex-direction: column;
  }

  .pin {
    font-size: 15px;
    letter-spacing: 0.08em;
  }

  .pin-toggle {
    min-width: 56px;
    font-size: 12px;
  }

  @media (max-width: 760px) {
    .this-pc {
      flex-direction: row;
      flex-wrap: wrap;
      align-items: center;
      gap: 6px 14px;
      padding: 8px 12px;
      width: 100%;
    }
  }
</style>
