<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title: string;
    description?: string;
    /** Right-aligned header content, e.g. a status or a link. */
    actions?: Snippet;
    children?: Snippet;
    class?: string;
  }

  let { title, description, actions, children, class: className = '' }: Props = $props();

  const uid = $props.id();
</script>

<section class="card ui-card {className}" aria-labelledby="{uid}-title">
  <div class="card-head">
    <div class="card-heading">
      <h2 id="{uid}-title">{title}</h2>
      {#if description}<p class="card-sub">{description}</p>{/if}
    </div>
    {#if actions}<div class="card-actions">{@render actions()}</div>{/if}
  </div>
  {#if children}<div class="card-body">{@render children()}</div>{/if}
</section>

<style>
  .ui-card {
    display: flex;
    flex-direction: column;
    gap: 14px;
    min-width: 0;
  }

  .card-head {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .card-heading {
    flex: 1 1 auto;
    min-width: 0;
  }

  .card-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .card-body {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-width: 0;
  }
</style>
