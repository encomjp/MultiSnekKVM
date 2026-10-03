<script lang="ts">
  interface Props {
    keys: readonly string[];
    size?: 'sm' | 'md';
  }

  let { keys, size = 'sm' }: Props = $props();
</script>

<span class="keys {size}">
  {#each keys as key, index (index)}<kbd>{key}</kbd>{#if index < keys.length - 1}<span class="sr-only">+</span>{/if}{/each}
</span>

<style>
  .keys {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 4px;
    vertical-align: baseline;
  }

  kbd {
    font-size: 12px;
    line-height: 1.4;
    border: 1px solid var(--line);
    border-radius: 4px;
    padding: 1px 5px;
    background: var(--panel);
    color: var(--text);
  }

  .md {
    gap: 6px;
  }

  .md kbd {
    font-size: 13px;
    border-bottom-width: 3px;
    border-radius: 6px;
    padding: 4px 10px;
    background: var(--panel2);
  }
</style>
