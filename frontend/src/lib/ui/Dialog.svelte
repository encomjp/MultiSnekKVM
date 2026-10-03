<script lang="ts">
  // Modal dialog: traps Tab focus, closes on Escape or scrim click, and
  // returns focus to whatever opened it.
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  interface Props {
    title: string;
    description?: Snippet;
    onClose: () => void;
    /** Block closing, e.g. while a request is in flight. */
    dismissible?: boolean;
    children: Snippet;
    footer?: Snippet;
  }

  let { title, description, onClose, dismissible = true, children, footer }: Props = $props();

  const uid = $props.id();

  const FOCUSABLE =
    'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

  function close() {
    if (dismissible) onClose();
  }

  function trap(node: HTMLElement) {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const focusables = () => Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE));
    const initial = node.querySelector<HTMLElement>('[data-autofocus]') ?? focusables()[0] ?? node;
    initial.focus();

    function onKeydown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        close();
        return;
      }
      if (event.key !== 'Tab') return;
      const items = focusables();
      if (items.length === 0) {
        event.preventDefault();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && (active === first || !node.contains(active))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && (active === last || !node.contains(active))) {
        event.preventDefault();
        first.focus();
      }
    }

    node.addEventListener('keydown', onKeydown);
    return {
      destroy() {
        node.removeEventListener('keydown', onKeydown);
        if (opener?.isConnected) opener.focus();
      },
    };
  }
</script>

<div class="dialog-layer">
  <div class="scrim" aria-hidden="true" onclick={close}></div>
  <div
    class="dialog"
    role="dialog"
    aria-modal="true"
    aria-labelledby="{uid}-title"
    aria-describedby={description ? `${uid}-desc` : undefined}
    tabindex="-1"
    use:trap
  >
    <div class="dialog-head">
      <div>
        <h2 id="{uid}-title" class="dialog-title">{title}</h2>
        {#if description}<p id="{uid}-desc" class="dialog-desc">{@render description()}</p>{/if}
      </div>
      <button type="button" class="btn btn-icon btn-sm close" aria-label="Close" onclick={close} disabled={!dismissible}>
        <Icon name="close" strokeWidth={2} />
      </button>
    </div>
    {@render children()}
    {#if footer}<div class="dialog-foot">{@render footer()}</div>{/if}
  </div>
</div>

<style>
  .dialog-layer {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
  }

  .scrim {
    position: absolute;
    inset: 0;
    background: var(--scrim);
  }

  .dialog {
    position: relative;
    width: 100%;
    max-width: 440px;
    max-height: calc(100vh - 32px);
    overflow-y: auto;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius-xl);
    padding: 24px;
    display: flex;
    flex-direction: column;
    gap: 18px;
    box-shadow: var(--shadow-pop);
    animation: dialog-in 140ms ease-out;
  }

  .dialog:focus {
    outline: none;
  }

  .dialog-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
  }

  .dialog-title {
    font-size: 19px;
    font-weight: 650;
  }

  .dialog-desc {
    margin-top: 4px;
    color: var(--muted);
  }

  .close {
    flex: none;
    color: var(--muted);
  }

  .dialog-foot {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
    flex-wrap: wrap;
  }

  @keyframes dialog-in {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
</style>
