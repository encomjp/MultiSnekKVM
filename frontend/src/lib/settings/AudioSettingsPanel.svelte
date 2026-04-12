<script lang="ts">
  import type { AudioDevice } from '../types';

  export let busySetting = '';
  export let audioMode = 'off';
  export let audioProfile = 'balanced';
  export let audioTransportMode = 'pcm';
  export let audioTiming = 'always';
  export let muteSource = false;
  export let micMode = 'off';
  export let audioDevices: AudioDevice[] = [];
  export let captureDeviceID = '';
  export let playbackDeviceID = '';
  export let micDeviceID = '';
  export let micPlaybackDeviceID = '';
  export let isBeingControlled = false;
  export let onUpdateAudioMode: (mode: string) => void | Promise<void>;
  export let onUpdateAudioProfile: (profile: string) => void | Promise<void>;
  export let onUpdateAudioTransportMode: (mode: string) => void | Promise<void>;
  export let onUpdateAudioTiming: (timing: string) => void | Promise<void>;
  export let onUpdateMuteSource: (enabled: boolean) => void | Promise<void>;
  export let onUpdateMicMode: (mode: string) => void | Promise<void>;
  export let onUpdateCaptureDevice: (id: string) => void | Promise<void>;
  export let onUpdatePlaybackDevice: (id: string) => void | Promise<void>;
  export let onUpdateMicDevice: (id: string) => void | Promise<void>;
  export let onUpdateMicPlaybackDevice: (id: string) => void | Promise<void>;
  export let onRefreshAudioDevices: (() => void | Promise<void>) | undefined = undefined;
</script>

<div class="settings-grid">
  {#if isBeingControlled}
    <div class="banner-muted wide">
      <strong>Audio locked</strong> - end the current session to change audio settings.
    </div>
  {/if}

  <article class="card">
    <div class="card-header">
      <h3>Playback</h3>
      {#if onRefreshAudioDevices}
        <button class="btn btn-ghost btn-sm" type="button" on:click={onRefreshAudioDevices} title="Refresh audio devices" aria-label="Refresh audio devices">
          ↻
        </button>
      {/if}
    </div>
    <div class="setting-list">
      <div class="setting-group">
        <span class="setting-group-label">Audio direction</span>
        <div class="segmented-control" role="radiogroup" aria-label="Audio direction">
          <button type="button" class="seg-btn" class:active={audioMode === 'off'} role="radio" aria-checked={audioMode === 'off'} on:click={() => onUpdateAudioMode('off')} disabled={busySetting !== '' || isBeingControlled}>Off</button>
          <button type="button" class="seg-btn" class:active={audioMode === 'remote'} role="radio" aria-checked={audioMode === 'remote'} on:click={() => onUpdateAudioMode('remote')} disabled={busySetting !== '' || isBeingControlled}>Hear remote</button>
          <button type="button" class="seg-btn" class:active={audioMode === 'local'} role="radio" aria-checked={audioMode === 'local'} on:click={() => onUpdateAudioMode('local')} disabled={busySetting !== '' || isBeingControlled}>Send local</button>
        </div>
      </div>
      {#if audioMode === 'remote'}
        <div class="setting-inline">
          <span class="inline-label">Play through</span>
          <select class="device-select" value={playbackDeviceID} on:change={(e) => onUpdatePlaybackDevice(e.currentTarget.value)} disabled={busySetting !== '' || isBeingControlled}>
            <option value="">Default speaker</option>
            {#each audioDevices.filter(d => d.flow === 'render') as d}
              <option value={d.id}>{d.name}</option>
            {/each}
          </select>
        </div>
      {/if}
      {#if audioMode === 'local'}
        <div class="setting-inline">
          <span class="inline-label">Capture from</span>
          <select class="device-select" value={captureDeviceID} on:change={(e) => onUpdateCaptureDevice(e.currentTarget.value)} disabled={busySetting !== '' || isBeingControlled}>
            <option value="">Default speaker (loopback)</option>
            {#each audioDevices.filter(d => d.flow === 'render') as d}
              <option value={d.id}>{d.name}</option>
            {/each}
          </select>
        </div>
      {/if}
      {#if audioMode !== 'off'}
        <label class="setting-check">
          <input type="checkbox" class="native-check" checked={muteSource} on:change={(e) => onUpdateMuteSource(e.currentTarget.checked)} disabled={busySetting !== '' || isBeingControlled} />
          <span>Mute this computer while audio is active</span>
        </label>
      {/if}
    </div>
  </article>

  <article class="card">
    <h3>Microphone</h3>
    <div class="setting-list">
      <div class="setting-row">
        <div class="setting-copy">
          <strong>Share microphone</strong>
          <span>Send this mic to the remote computer.</span>
        </div>
        <label class="toggle-switch">
          <input type="checkbox" aria-label="Share microphone" checked={micMode === 'send'} on:change={(e) => onUpdateMicMode(e.currentTarget.checked ? 'send' : 'off')} disabled={busySetting !== '' || isBeingControlled} />
          <span class="toggle-slider"></span>
        </label>
      </div>
      {#if micMode === 'send'}
        <div class="setting-inline">
          <span class="inline-label">Microphone</span>
          <select class="device-select" value={micDeviceID} on:change={(e) => onUpdateMicDevice(e.currentTarget.value)} disabled={busySetting !== '' || isBeingControlled}>
            <option value="">Default microphone</option>
            {#each audioDevices.filter(d => d.flow === 'capture') as d}
              <option value={d.id}>{d.name}</option>
            {/each}
          </select>
        </div>
      {/if}

      <div class="setting-row">
        <div class="setting-copy">
          <strong>Hear remote mic</strong>
          <span>Play the remote microphone here.</span>
        </div>
        <label class="toggle-switch">
          <input type="checkbox" aria-label="Hear remote microphone" checked={micMode === 'receive'} on:change={(e) => onUpdateMicMode(e.currentTarget.checked ? 'receive' : 'off')} disabled={busySetting !== '' || isBeingControlled} />
          <span class="toggle-slider"></span>
        </label>
      </div>
      {#if micMode === 'receive'}
        <div class="setting-inline">
          <span class="inline-label">Output device</span>
          <select class="device-select" value={micPlaybackDeviceID} on:change={(e) => onUpdateMicPlaybackDevice(e.currentTarget.value)} disabled={busySetting !== '' || isBeingControlled}>
            <option value="">Default speaker</option>
            {#each audioDevices.filter(d => d.flow === 'render') as d}
              <option value={d.id}>{d.name}</option>
            {/each}
          </select>
        </div>
      {/if}
    </div>
  </article>

  <article class="card wide">
    <h3>Transport</h3>
    <div class="setting-list setting-list-inline">
      <div class="setting-inline">
        <span class="inline-label">Mode</span>
        <select class="device-select" value={audioTransportMode} on:change={(e) => onUpdateAudioTransportMode(e.currentTarget.value)} disabled={busySetting !== '' || isBeingControlled}>
          <option value="pcm">Raw PCM</option>
          <option value="opus">Opus adaptive</option>
        </select>
      </div>
      <div class="setting-inline">
        <span class="inline-label">Quality</span>
        <select class="device-select" value={audioProfile} on:change={(e) => onUpdateAudioProfile(e.currentTarget.value)} disabled={busySetting !== '' || isBeingControlled}>
          <option value="low-latency">Low latency</option>
          <option value="balanced">Balanced</option>
          <option value="music">Music</option>
        </select>
      </div>
      {#if (audioMode !== 'off' || micMode !== 'off') && !isBeingControlled}
        <label class="setting-check">
          <input type="checkbox" class="native-check" checked={audioTiming === 'switched'} on:change={(e) => onUpdateAudioTiming(e.currentTarget.checked ? 'switched' : 'always')} disabled={busySetting !== ''} />
          <span>Only active when mouse is on remote screen</span>
        </label>
      {/if}
    </div>
  </article>
</div>

<style>
  .settings-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.75rem;
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
    background: var(--panel);
    padding: 1.25rem;
  }

  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .btn-sm {
    padding: 0.2rem 0.5rem;
    font-size: 0.85rem;
    min-width: unset;
  }

  .card h3 {
    font-size: 0.92rem;
    font-weight: 700;
    color: var(--text-strong);
    letter-spacing: -0.01em;
  }

  .wide {
    grid-column: 1 / -1;
  }

  .banner-muted {
    grid-column: 1 / -1;
    padding: 0.75rem 1rem;
    border-radius: var(--radius-lg);
    background: var(--panel-strong);
    color: var(--text-secondary);
    font-size: 0.88rem;
    line-height: 1.5;
    border: 1px solid var(--border);
  }

  .setting-list {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .setting-list-inline {
    gap: 0.9rem;
  }

  .setting-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  .setting-copy {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .setting-copy strong {
    font-size: 0.9rem;
    color: var(--text-strong);
  }

  .setting-copy span {
    font-size: 0.82rem;
    color: var(--text-secondary);
  }

  .setting-inline {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.75rem;
    border-radius: var(--radius-md);
    background: var(--panel-strong);
  }

  .inline-label {
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .setting-check {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    font-size: 0.88rem;
    color: var(--text-secondary);
  }

  @media (max-width: 960px) {
    .settings-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 640px) {
    .setting-row {
      flex-direction: column;
      align-items: flex-start;
    }
  }

  .setting-group {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .setting-group-label {
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--text-secondary);
  }

  .segmented-control {
    display: flex;
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    overflow: hidden;
  }

  .seg-btn {
    flex: 1;
    padding: 0.45rem 0.75rem;
    font-size: 0.82rem;
    font-weight: 600;
    border: none;
    background: var(--panel-strong);
    color: var(--text-secondary);
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }

  .seg-btn:not(:last-child) {
    border-right: 1px solid var(--border);
  }

  .seg-btn.active {
    background: var(--accent);
    color: white;
  }

  .seg-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .seg-btn:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
    z-index: 1;
  }
</style>
