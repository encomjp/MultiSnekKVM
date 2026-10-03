<script lang="ts">
  // One polite live region for every toast. Individual toasts carry no
  // role="alert", so nothing is announced twice.
  import type { Toast, ToastStore } from '../stores/toasts.svelte';
  import Icon, { type IconName } from './Icon.svelte';

  interface Props {
    toasts: ToastStore;
  }

  let { toasts }: Props = $props();

  const ICON: Record<Toast['tone'], IconName> = {
    info: 'info',
    success: 'check',
    warning: 'alert',
    error: 'alert',
  };

  const PREFIX: Record<Toast['tone'], string> = {
    info: '',
    success: '',
    warning: 'Warning: ',
    error: 'Error: ',
  };
</script>

<div class="toast-region" aria-live="polite" aria-relevant="additions text" aria-label="Notifications">
  {#each toasts.items as toast (toast.id)}
    <div class="toast {toast.tone}">
      <span class="toast-icon"><Icon name={ICON[toast.tone]} /></span>
      <p class="toast-msg"><span class="sr-only">{PREFIX[toast.tone]}</span>{toast.message}</p>
      {#if toast.action}
        {@const action = toast.action}
        <button
          type="button"
          class="btn btn-sm toast-action"
          onclick={() => {
            action.run();
            toasts.dismiss(toast.id);
          }}>{action.label}</button
        >
      {/if}
      <button type="button" class="btn btn-ghost btn-icon btn-sm" aria-label="Dismiss notification" onclick={() => toasts.dismiss(toast.id)}>
        <Icon name="close" />
      </button>
    </div>
  {/each}
</div>

<style>
  .toast-region {
    position: fixed;
    right: 16px;
    bottom: 16px;
    z-index: 60;
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: min(420px, calc(100vw - 32px));
    pointer-events: none;
  }

  .toast {
    pointer-events: auto;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 8px 8px 14px;
    border-radius: var(--radius-md);
    border: 1px solid var(--line);
    border-left: 3px solid var(--info);
    background: var(--panel);
    box-shadow: var(--shadow-pop);
    animation: toast-in 160ms ease-out;
  }

  .toast-icon {
    display: flex;
    color: var(--info);
  }

  .success {
    border-left-color: var(--accent);
  }

  .success .toast-icon {
    color: var(--accent);
  }

  .warning {
    border-left-color: var(--warning);
  }

  .warning .toast-icon {
    color: var(--warning);
  }

  .error {
    border-left-color: var(--danger);
  }

  .error .toast-icon {
    color: var(--danger);
  }

  .toast-msg {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    overflow-wrap: anywhere;
  }

  .toast-action {
    flex: none;
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
</style>
