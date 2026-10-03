<script lang="ts">
  // A real button that, once pressed, captures the next key combination.
  import { getAppState } from '../../stores/app.svelte';
  import { captureHotkey, hotkeyKeys, hotkeyLabel } from '../../hotkeys';
  import Kbd from '../../ui/Kbd.svelte';

  const app = getAppState();
  const settings = app.settings;

  let recording = $state(false);
  let warning = $state('');

  const keys = $derived(hotkeyKeys(settings.values.exitHotkey));
  const pending = $derived(settings.status.exitHotkey.pending);
  const isDefault = $derived(settings.values.exitHotkey.vkCode === 0);

  function start() {
    if (recording) {
      recording = false;
      return;
    }
    warning = '';
    recording = true;
  }

  function onKeydown(event: KeyboardEvent) {
    if (!recording) return;
    // While recording, every key belongs to the recorder (including Tab).
    event.preventDefault();
    event.stopPropagation();
    const result = captureHotkey(event);
    if (result.kind === 'cancel') {
      recording = false;
      warning = '';
    } else if (result.kind === 'invalid') {
      warning = result.message;
    } else if (result.kind === 'ok') {
      recording = false;
      warning = '';
      void settings.set('exitHotkey', result.hotkey);
    }
  }
</script>

<div class="recorder">
  <span class="current" aria-label="Current exit hotkey: {hotkeyLabel(settings.values.exitHotkey)}">
    <Kbd {keys} size="md" />
  </span>
  <div class="buttons">
    {#if !isDefault && !recording}
      <button type="button" class="btn btn-ghost btn-sm" disabled={pending} onclick={() => settings.set('exitHotkey', { modifiers: 0, vkCode: 0 })}>
        Use Esc
      </button>
    {/if}
    <button
      type="button"
      class="btn record"
      class:recording
      aria-pressed={recording}
      aria-describedby="hotkey-help"
      disabled={pending}
      onclick={start}
      onkeydown={onKeydown}
      onblur={() => (recording = false)}
    >
      {recording ? 'Press a key combination…' : 'Record new…'}
    </button>
  </div>
</div>
<p id="hotkey-help" class="muted small" aria-live="polite">
  {#if warning}
    <span class="warn">{warning}</span>
  {:else if recording}
    Press the new combination now, or Esc to cancel.
  {:else}
    Esc on its own always works as a fallback.
  {/if}
</p>
{#if settings.status.exitHotkey.error}<p class="field-error">{settings.status.exitHotkey.error}</p>{/if}

<style>
  .recorder {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
  }

  .buttons {
    display: flex;
    gap: 8px;
  }

  .record.recording {
    border-color: var(--accent);
    color: var(--accent);
    background: var(--accent-soft);
  }

  .warn {
    color: var(--warning);
  }
</style>
