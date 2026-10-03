<script lang="ts">
  import type { PairingRequest } from './stores/app.svelte';
  import { getAppState } from './stores/app.svelte';
  import { hostOf, sanitizePin } from './utils';
  import Dialog from './ui/Dialog.svelte';
  import Icon from './ui/Icon.svelte';

  interface Props {
    request: PairingRequest;
  }

  let { request }: Props = $props();

  const app = getAppState();
  const conn = app.connection;

  let pin = $state('');
  let error = $state('');
  let busy = $state(false);

  const name = $derived(request.peer.name || hostOf(request.address));
  const ready = $derived(pin.length === 6);
  const remaining = $derived(6 - pin.length);

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (!ready || busy) return;
    busy = true;
    error = '';
    try {
      await conn.pair(request.peer, request.address, pin);
      app.closePairing();
      app.navigate('session');
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="Pair with {name}" onClose={() => app.closePairing()} dismissible={!busy}>
  {#snippet description()}
    On {name}, open MultiSnek and read the PIN under <strong>This PC</strong>.
  {/snippet}

  <form id="pairing-form" class="pair-form" onsubmit={submit} novalidate>
    <label for="pairing-pin" class="field-label">Pairing PIN</label>
    <input
      id="pairing-pin"
      class="pin-input mono"
      type="text"
      inputmode="numeric"
      autocomplete="one-time-code"
      maxlength="6"
      placeholder="000000"
      data-autofocus
      value={pin}
      aria-invalid={error ? 'true' : undefined}
      aria-describedby="pairing-pin-hint{error ? ' pairing-pin-error' : ''}"
      oninput={(event) => {
        pin = sanitizePin(event.currentTarget.value);
        event.currentTarget.value = pin;
        error = '';
      }}
    />
    <span id="pairing-pin-hint" class="hint">
      {ready ? 'Ready to pair.' : `${remaining} more digit${remaining === 1 ? '' : 's'}`}
    </span>
    {#if error}<p id="pairing-pin-error" class="field-error" role="alert">{error}</p>{/if}
  </form>

  <div class="notice accent">
    <span class="lock"><Icon name="lock" /></span>
    <span>
      The PIN never leaves this PC. Both computers prove they know it, then remember each other's certificate, so you
      only do this once.
    </span>
  </div>

  {#snippet footer()}
    <button type="button" class="btn" onclick={() => app.closePairing()} disabled={busy}>Cancel</button>
    <button type="submit" form="pairing-form" class="btn btn-primary" disabled={!ready || busy}>
      {#if busy}<span class="spinner" aria-hidden="true"></span>Pairing…{:else}Pair &amp; connect{/if}
    </button>
  {/snippet}
</Dialog>

<style>
  .pair-form {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .pin-input {
    min-height: 56px;
    padding: 0 16px;
    border-radius: var(--radius-md);
    border: 1px solid var(--line-strong);
    background: var(--panel2);
    color: var(--text);
    font-size: 28px;
    letter-spacing: 0.5em;
    text-indent: 0.5em;
    text-align: center;
  }

  .pin-input::placeholder {
    color: var(--muted);
    opacity: 0.5;
  }

  .hint {
    font-size: 12px;
    color: var(--muted);
  }

  .lock {
    color: var(--accent);
  }

  strong {
    color: var(--text);
    font-weight: 600;
  }
</style>
