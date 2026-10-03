<script lang="ts">
  interface Props {
    label: string;
    description?: string;
    checked: boolean;
    onChange: (checked: boolean) => void;
    disabled?: boolean;
    pending?: boolean;
    error?: string;
  }

  let { label, description, checked, onChange, disabled = false, pending = false, error = '' }: Props = $props();

  const uid = $props.id();
</script>

<div class="toggle-row">
  <label for="{uid}-input" class="toggle-text">
    <span class="toggle-label">{label}</span>
    {#if description}<span class="toggle-desc" id="{uid}-desc">{description}</span>{/if}
  </label>
  <input
    id="{uid}-input"
    type="checkbox"
    role="switch"
    class="switch"
    {checked}
    {disabled}
    aria-describedby={description ? `${uid}-desc` : undefined}
    aria-busy={pending || undefined}
    onchange={(event) => {
      const next = event.currentTarget.checked;
      // Stay controlled: the store decides (and may revert).
      event.currentTarget.checked = checked;
      onChange(next);
    }}
  />
</div>
{#if error}<p class="field-error toggle-error">{error}</p>{/if}

<style>
  .toggle-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    min-height: 44px;
  }

  .toggle-text {
    display: flex;
    flex-direction: column;
    cursor: pointer;
    min-width: 0;
  }

  .toggle-label {
    font-weight: 500;
  }

  .toggle-desc {
    font-size: 12px;
    color: var(--muted);
  }

  .switch {
    appearance: none;
    -webkit-appearance: none;
    flex: none;
    position: relative;
    width: 40px;
    height: 24px;
    margin: 0;
    border-radius: 999px;
    border: 1px solid var(--line-strong);
    background: var(--panel2);
    cursor: pointer;
    transition: background-color 140ms ease, border-color 140ms ease;
  }

  .switch::after {
    content: '';
    position: absolute;
    top: 3px;
    left: 3px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: var(--muted);
    transition: transform 140ms ease, background-color 140ms ease;
  }

  .switch:checked {
    background: var(--accent);
    border-color: var(--accent);
  }

  .switch:checked::after {
    transform: translateX(16px);
    background: var(--on-accent);
  }

  .switch:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .switch[aria-busy='true'] {
    opacity: 0.75;
  }

  .toggle-error {
    margin-top: -4px;
  }
</style>
