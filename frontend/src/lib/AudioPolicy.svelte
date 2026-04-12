<script>
  export let audioMode;
  export let audioTiming;
  export let busySetting;
  export let onModeChange;
  export let onTimingChange;
</script>

<article class="panel sidebar-panel">
  <div class="panel-heading panel-heading-stack">
    <div>
      <span class="eyebrow">Audio routing</span>
      <h3>Playback policy</h3>
    </div>
  </div>

  <div class="control-group">
    <span class="meta-label">Direction</span>
    <div class="segmented-control segmented-control-stack">
      <button class:active={audioMode === 'off'} on:click={() => onModeChange('off')} disabled={busySetting !== ''}>Off</button>
      <button class:active={audioMode === 'remote'} on:click={() => onModeChange('remote')} disabled={busySetting !== ''}>Receive remote audio</button>
      <button class:active={audioMode === 'local'} on:click={() => onModeChange('local')} disabled={busySetting !== ''}>Send local audio</button>
    </div>
  </div>

  <div class="control-group">
    <span class="meta-label">Activation</span>
    <div class="segmented-control">
      <button class:active={audioTiming === 'always'} on:click={() => onTimingChange('always')} disabled={busySetting !== ''}>Always</button>
      <button class:active={audioTiming === 'switched'} on:click={() => onTimingChange('switched')} disabled={busySetting !== ''}>Only while controlling</button>
    </div>
    <p>Receive plays the remote computer's audio on this machine. Send routes this computer's audio to the remote workstation.</p>
  </div>
</article>

<style>
  .sidebar-panel {
    padding: 1.1rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
</style>
