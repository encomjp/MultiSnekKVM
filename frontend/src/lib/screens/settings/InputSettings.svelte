<script lang="ts">
  import { getAppState } from '../../stores/app.svelte';
  import { EDGE_OPTIONS } from '../../labels';
  import Card from '../../ui/Card.svelte';
  import Icon from '../../ui/Icon.svelte';
  import SegmentedControl from '../../ui/SegmentedControl.svelte';
  import HotkeyRecorder from './HotkeyRecorder.svelte';
  import MonitorLayout from './MonitorLayout.svelte';

  const app = getAppState();
  const settings = app.settings;
  const conn = app.connection;

  const locked = $derived(conn.isController);

  // Local draft while the slider moves; one save when it is released.
  let speedDraft = $state<number | null>(null);
  const speed = $derived(speedDraft ?? settings.values.sensitivity);

  async function commitSpeed(value: number) {
    speedDraft = null;
    if (value !== settings.values.sensitivity) await settings.set('sensitivity', value);
  }
</script>

<div class="stack">
  {#if locked}
    <div class="notice" role="status">
      <Icon name="info" />
      <span>Edge and layout are locked while you control {conn.session.peerName}. Disconnect to change them.</span>
    </div>
  {/if}

  <Card title="Screen edge" description="Push the pointer past this edge to move to the other PC.">
    {#snippet actions()}
      <SegmentedControl
        label="Screen edge"
        hideLabel
        options={EDGE_OPTIONS}
        value={settings.values.edgeSide}
        disabled={locked}
        pending={settings.status.edgeSide.pending}
        minWidth="min(320px, 100%)"
        onChange={(value) => settings.set('edgeSide', value)}
      />
    {/snippet}
    {#if settings.values.triggerZone}
      <p class="muted small">
        A custom hand-off zone is set below; it takes priority over this edge. Reset the layout to use the edge alone.
      </p>
    {/if}
    {#if settings.status.edgeSide.error}<p class="field-error">{settings.status.edgeSide.error}</p>{/if}
  </Card>

  <MonitorLayout {locked} />

  <div class="pair">
    <Card title="Pointer speed on remote">
      {#snippet actions()}
        <span class="mono value">{speed.toFixed(2)}×</span>
      {/snippet}
      <label for="pointer-speed" class="sr-only">Pointer speed</label>
      <input
        id="pointer-speed"
        type="range"
        min="0.25"
        max="3"
        step="0.05"
        value={speed}
        aria-valuetext="{speed.toFixed(2)} times"
        oninput={(event) => (speedDraft = Number(event.currentTarget.value))}
        onchange={(event) => commitSpeed(Number(event.currentTarget.value))}
      />
      <div class="scale"><span>Slower</span><span>Faster</span></div>
      {#if settings.status.sensitivity.error}<p class="field-error">{settings.status.sensitivity.error}</p>{/if}
    </Card>

    <Card title="Exit hotkey" description="Takes control back from the other PC at any time.">
      <HotkeyRecorder />
    </Card>
  </div>
</div>

<style>
  .stack {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .pair {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 18px;
  }

  .value {
    font-size: 13px;
  }

  .scale {
    display: flex;
    justify-content: space-between;
    font-size: 12px;
    color: var(--muted);
  }

  @media (max-width: 880px) {
    .pair {
      grid-template-columns: 1fr;
    }
  }
</style>
