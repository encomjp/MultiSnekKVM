<script lang="ts" module>
  export type DotTone = 'ok' | 'warn' | 'bad' | 'info' | 'off';
</script>

<script lang="ts">
  interface Props {
    tone?: DotTone;
    size?: number;
    /** Screen-reader text; omit when adjacent text already says it. */
    label?: string;
  }

  let { tone = 'ok', size = 8, label }: Props = $props();
</script>

<span class="dot {tone}" style:width="{size}px" style:height="{size}px" aria-hidden={label ? undefined : 'true'}></span>{#if label}<span class="sr-only">{label}</span>{/if}

<style>
  .dot {
    display: inline-block;
    flex: none;
    border-radius: 50%;
    background: var(--accent);
  }

  .warn {
    background: var(--warning);
  }

  .bad {
    background: var(--danger);
  }

  .info {
    background: var(--info);
  }

  .off {
    background: transparent;
    border: 1.5px solid var(--line-strong);
  }
</style>
