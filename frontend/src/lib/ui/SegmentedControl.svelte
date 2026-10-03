<script lang="ts" module>
  export interface SegmentOption<V extends string> {
    value: V;
    label: string;
    disabled?: boolean;
  }
</script>

<script lang="ts" generics="T extends string">
  // A segmented choice built on native radio inputs, so it gets radio
  // semantics, arrow-key navigation and form labelling for free.
  import { tick } from 'svelte';

  interface Props {
    /** Accessible name of the group; also shown above unless hideLabel. */
    label: string;
    hideLabel?: boolean;
    options: ReadonlyArray<SegmentOption<T>>;
    value: T;
    onChange: (value: T) => void;
    disabled?: boolean;
    pending?: boolean;
    /** Minimum width of the control, e.g. for wide edge pickers. */
    minWidth?: string;
  }

  let {
    label,
    hideLabel = false,
    options,
    value,
    onChange,
    disabled = false,
    pending = false,
    minWidth,
  }: Props = $props();

  const uid = $props.id();
  let group: HTMLDivElement | undefined = $state();

  async function select(next: T) {
    if (next !== value) onChange(next);
    // Keep the DOM in step with the model even if the change was rejected.
    await tick();
    for (const input of group?.querySelectorAll<HTMLInputElement>('input[type="radio"]') ?? []) {
      input.checked = input.value === value;
    }
  }
</script>

<div class="segmented" class:pending>
  <span id="{uid}-label" class="seg-label" class:sr-only={hideLabel}>{label}</span>
  <div
    bind:this={group}
    class="seg-track"
    role="radiogroup"
    aria-labelledby="{uid}-label"
    aria-busy={pending || undefined}
    style:grid-template-columns="repeat({options.length}, minmax(0, 1fr))"
    style:min-width={minWidth}
  >
    {#each options as option (option.value)}
      <label class="seg-option" class:checked={option.value === value} class:disabled={disabled || option.disabled}>
        <input
          type="radio"
          class="sr-only"
          name="{uid}-radio"
          value={option.value}
          checked={option.value === value}
          disabled={disabled || option.disabled}
          onchange={() => select(option.value)}
        />
        <span>{option.label}</span>
      </label>
    {/each}
  </div>
</div>

<style>
  .segmented {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-width: 0;
  }

  .seg-label {
    font-size: 12px;
    color: var(--muted);
  }

  .seg-track {
    display: grid;
    gap: 4px;
    padding: 4px;
    background: var(--panel2);
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
  }

  .seg-option {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 36px;
    padding: 0 8px;
    border-radius: 7px;
    font-size: 13px;
    color: var(--muted);
    text-align: center;
    cursor: pointer;
    user-select: none;
    transition: background-color 120ms ease, color 120ms ease;
  }

  .seg-option:hover:not(.disabled):not(.checked) {
    color: var(--text);
  }

  .seg-option.checked {
    background: var(--panel);
    color: var(--text);
    font-weight: 600;
    box-shadow: var(--shadow-seg);
  }

  .seg-option.disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .seg-option:has(input:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }

  .pending .seg-track {
    opacity: 0.8;
  }
</style>
