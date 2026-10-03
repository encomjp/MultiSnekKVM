<script lang="ts">
  import { getAppState } from '../../stores/app.svelte';
  import { AUDIO_MODE_OPTIONS, MIC_MODE_OPTIONS } from '../../labels';
  import Card from '../../ui/Card.svelte';
  import SegmentedControl from '../../ui/SegmentedControl.svelte';
  import Toggle from '../../ui/Toggle.svelte';

  const app = getAppState();
  const settings = app.settings;
  const conn = app.connection;

  // Audio is decided by the controlling PC while this one is being controlled.
  const audioLocked = $derived(conn.isControlled);
</script>

<Card title="Quick controls">
  {#if audioLocked}
    <p class="notice">Audio follows {conn.session.peerName}'s settings while it controls this PC.</p>
  {/if}
  <SegmentedControl
    label="Desktop audio"
    options={AUDIO_MODE_OPTIONS}
    value={settings.values.audioMode}
    pending={settings.status.audioMode.pending}
    disabled={audioLocked}
    onChange={(value) => settings.set('audioMode', value)}
  />
  <SegmentedControl
    label="Microphone"
    options={MIC_MODE_OPTIONS}
    value={settings.values.micMode}
    pending={settings.status.micMode.pending}
    disabled={audioLocked}
    onChange={(value) => settings.set('micMode', value)}
  />
  <Toggle
    label="Auto-reconnect"
    description="Resume the session after a dropped link"
    checked={settings.values.autoReconnect}
    pending={settings.status.autoReconnect.pending}
    onChange={(value) => settings.set('autoReconnect', value)}
  />
</Card>
