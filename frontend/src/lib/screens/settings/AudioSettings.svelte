<script lang="ts">
  import type { SettingKey } from '../../stores/settings.svelte';
  import { getAppState } from '../../stores/app.svelte';
  import {
    AUDIO_MODE_OPTIONS,
    AUDIO_PROFILE_OPTIONS,
    AUDIO_TIMING_OPTIONS,
    AUDIO_TRANSPORT_OPTIONS,
    MIC_MODE_OPTIONS,
    transportHint,
  } from '../../labels';
  import Card from '../../ui/Card.svelte';
  import Icon from '../../ui/Icon.svelte';
  import SegmentedControl from '../../ui/SegmentedControl.svelte';
  import Select, { type SelectOption } from '../../ui/Select.svelte';
  import Toggle from '../../ui/Toggle.svelte';

  const app = getAppState();
  const settings = app.settings;
  const conn = app.connection;

  const locked = $derived(conn.isControlled);
  const v = $derived(settings.values);
  const s = $derived(settings.status);

  function deviceOptions(flow: 'render' | 'capture', current: string, defaultLabel: string): SelectOption[] {
    const options: SelectOption[] = [{ value: '', label: defaultLabel }];
    for (const device of settings.audioDevices) {
      if (device.flow === flow) options.push({ value: device.id, label: device.name || device.id });
    }
    if (current && !options.some((option) => option.value === current)) {
      options.push({ value: current, label: 'Unavailable device' });
    }
    return options;
  }

  const setString = (key: SettingKey & ('captureDeviceID' | 'playbackDeviceID' | 'micDeviceID' | 'micPlaybackDeviceID')) =>
    (value: string) => settings.set(key, value);
</script>

<div class="stack">
  {#if locked}
    <div class="notice" role="status">
      <Icon name="info" />
      <span>Audio follows {conn.session.peerName}'s settings while it controls this PC. Disconnect to change these.</span>
    </div>
  {/if}

  <Card title="Desktop audio" description="Stream what one PC plays to the other's speakers.">
    {#snippet actions()}
      <button type="button" class="btn btn-sm" onclick={() => settings.refreshAudioDevices()}>
        <Icon name="refresh" />Refresh devices
      </button>
    {/snippet}
    <SegmentedControl
      label="Audio direction"
      options={AUDIO_MODE_OPTIONS}
      value={v.audioMode}
      disabled={locked}
      pending={s.audioMode.pending}
      onChange={(value) => settings.set('audioMode', value)}
    />
    {#if s.audioMode.error}<p class="field-error">{s.audioMode.error}</p>{/if}

    {#if v.audioMode === 'remote'}
      <Select
        label="Play remote audio through"
        value={v.playbackDeviceID}
        options={deviceOptions('render', v.playbackDeviceID, 'Default speakers')}
        disabled={locked}
        pending={s.playbackDeviceID.pending}
        error={s.playbackDeviceID.error}
        onChange={setString('playbackDeviceID')}
      />
    {:else if v.audioMode === 'local'}
      <Select
        label="Capture this PC's audio from"
        value={v.captureDeviceID}
        options={deviceOptions('render', v.captureDeviceID, 'Default speakers (loopback)')}
        disabled={locked}
        pending={s.captureDeviceID.pending}
        error={s.captureDeviceID.error}
        onChange={setString('captureDeviceID')}
      />
    {/if}

    {#if v.audioMode !== 'off'}
      <Toggle
        label="Mute the source PC while streaming"
        description="Avoids hearing the same sound from both PCs"
        checked={v.muteSource}
        disabled={locked}
        pending={s.muteSource.pending}
        error={s.muteSource.error}
        onChange={(value) => settings.set('muteSource', value)}
      />
    {/if}

    {#if v.audioMode !== 'off' || v.micMode !== 'off'}
      <SegmentedControl
        label="Stream audio"
        options={AUDIO_TIMING_OPTIONS}
        value={v.audioTiming}
        disabled={locked}
        pending={s.audioTiming.pending}
        onChange={(value) => settings.set('audioTiming', value)}
      />
    {/if}
  </Card>

  <Card title="Microphone" description="Use one PC's microphone on the other.">
    <SegmentedControl
      label="Microphone"
      options={MIC_MODE_OPTIONS}
      value={v.micMode}
      disabled={locked}
      pending={s.micMode.pending}
      onChange={(value) => settings.set('micMode', value)}
    />
    {#if s.micMode.error}<p class="field-error">{s.micMode.error}</p>{/if}
    {#if v.micMode === 'send'}
      <Select
        label="Microphone to send"
        value={v.micDeviceID}
        options={deviceOptions('capture', v.micDeviceID, 'Default microphone')}
        disabled={locked}
        pending={s.micDeviceID.pending}
        error={s.micDeviceID.error}
        onChange={setString('micDeviceID')}
      />
    {:else if v.micMode === 'receive'}
      <Select
        label="Play the remote microphone through"
        value={v.micPlaybackDeviceID}
        options={deviceOptions('render', v.micPlaybackDeviceID, 'Default speakers')}
        disabled={locked}
        pending={s.micPlaybackDeviceID.pending}
        error={s.micPlaybackDeviceID.error}
        onChange={setString('micPlaybackDeviceID')}
      />
    {/if}
    <p class="notice">
      <Icon name="mic" />
      <span>
        The microphone always uses Opus voice coding, tuned to stay smooth over LAN and Tailscale links. The transport
        setting below applies to desktop audio only.
      </span>
    </p>
  </Card>

  <Card title="Quality" description="How desktop audio is encoded on the wire.">
    <div class="grid">
      <Select
        label="Transport"
        value={v.audioTransport}
        options={AUDIO_TRANSPORT_OPTIONS}
        hint={transportHint(v.audioTransport)}
        disabled={locked}
        pending={s.audioTransport.pending}
        error={s.audioTransport.error}
        onChange={(value) => settings.set('audioTransport', value as typeof v.audioTransport)}
      />
      <SegmentedControl
        label="Profile"
        options={AUDIO_PROFILE_OPTIONS}
        value={v.audioProfile}
        disabled={locked}
        pending={s.audioProfile.pending}
        onChange={(value) => settings.set('audioProfile', value)}
      />
    </div>
    {#if s.audioProfile.error}<p class="field-error">{s.audioProfile.error}</p>{/if}
    <p class="notice">
      <Icon name="bluetooth" />
      <span>Over Bluetooth, desktop audio is always compressed (Opus ≤ 96 kbit/s).</span>
    </p>
  </Card>
</div>

<style>
  .stack {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 18px;
    align-items: start;
  }

  @media (max-width: 880px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
