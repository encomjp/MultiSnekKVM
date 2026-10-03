<script lang="ts" module>
  export interface SelectOption {
    value: string;
    label: string;
  }
</script>

<script lang="ts">
  interface Props {
    label: string;
    value: string;
    options: ReadonlyArray<SelectOption>;
    onChange: (value: string) => void;
    hint?: string;
    disabled?: boolean;
    pending?: boolean;
    error?: string;
  }

  let { label, value, options, onChange, hint, disabled = false, pending = false, error = '' }: Props = $props();

  const uid = $props.id();
</script>

<div class="select-field">
  <label class="field-label" for="{uid}-select">{label}</label>
  <div class="select-wrap">
    <select
      id="{uid}-select"
      class="input select"
      {value}
      {disabled}
      aria-busy={pending || undefined}
      aria-describedby={hint ? `${uid}-hint` : undefined}
      onchange={(event) => {
        const next = event.currentTarget.value;
        event.currentTarget.value = value;
        onChange(next);
      }}
    >
      {#each options as option (option.value)}
        <option value={option.value}>{option.label}</option>
      {/each}
    </select>
    <svg class="chevron" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
  </div>
  {#if hint}<span id="{uid}-hint" class="hint">{hint}</span>{/if}
  {#if error}<span class="field-error">{error}</span>{/if}
</div>

<style>
  .select-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }

  .select-wrap {
    position: relative;
  }

  .select {
    width: 100%;
    appearance: none;
    -webkit-appearance: none;
    padding-right: 34px;
    cursor: pointer;
  }

  .select:disabled {
    cursor: not-allowed;
  }

  .chevron {
    position: absolute;
    right: 12px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--muted);
    pointer-events: none;
  }

  .hint {
    font-size: 12px;
    color: var(--muted);
  }

  option {
    background: var(--panel);
    color: var(--text);
  }
</style>
