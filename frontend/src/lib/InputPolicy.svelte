<script>
  export let edgeSide;
  export let sensitivity;
  export let busySetting;
  export let autostart;
  export let onEdgeChange;
  export let onSensitivityChange;
  export let onAutostartChange;
</script>

<article class="panel sidebar-panel">
  <div class="panel-heading panel-heading-stack">
    <div>
      <span class="eyebrow">Input policy</span>
      <h3>Handoff controls</h3>
    </div>
  </div>

  <div class="control-group">
    <span class="meta-label">Exit edge</span>
    <div class="segmented-control">
      <button class:active={edgeSide === 'left'} on:click={() => onEdgeChange('left')} disabled={busySetting !== ''}>Left edge</button>
      <button class:active={edgeSide === 'right'} on:click={() => onEdgeChange('right')} disabled={busySetting !== ''}>Right edge</button>
      <button class:active={edgeSide === 'top'} on:click={() => onEdgeChange('top')} disabled={busySetting !== ''}>Top edge</button>
      <button class:active={edgeSide === 'bottom'} on:click={() => onEdgeChange('bottom')} disabled={busySetting !== ''}>Bottom edge</button>
    </div>
  </div>

  <div class="control-group">
    <div class="slider-head">
      <span class="meta-label">Pointer scale</span>
      <span class="slider-value">{sensitivity.toFixed(1)}x</span>
    </div>
    <input type="range" min="0.1" max="3" step="0.1" bind:value={sensitivity} on:change={(event) => onSensitivityChange(event.currentTarget.value)} disabled={busySetting !== ''} />
    <div class="slider-ticks" aria-hidden="true">
      <span>Slow · 0.1×</span>
      <span>Normal · 1×</span>
      <span>Fast · 3×</span>
    </div>
    <p>Choose how aggressively pointer movement is translated during remote control.</p>
  </div>

  <div class="control-group">
    <span class="meta-label">Start with Windows</span>
    <div class="segmented-control">
      <button class:active={!autostart} on:click={() => onAutostartChange(false)} disabled={busySetting !== ''}>Disabled</button>
      <button class:active={autostart} on:click={() => onAutostartChange(true)} disabled={busySetting !== ''}>Enabled</button>
    </div>
    <p>Controls automatic launch via the current user Run registry key.</p>
  </div>
</article>

<style>
  .sidebar-panel {
    padding: 1.1rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .slider-head {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    align-items: center;
  }
  .slider-value { font-family: var(--font-mono); }
  .slider-ticks {
    display: flex;
    justify-content: space-between;
    font-size: 0.73rem;
    color: var(--text-muted);
    margin-top: 0.3rem;
    padding: 0 2px;
  }
</style>
