<script lang="ts">
  import type { ActivityIcon, ActivityTone, TransferEntry } from '../../stores/activity.svelte';
  import { getAppState } from '../../stores/app.svelte';
  import { formatClock, pluralize } from '../../utils';
  import Card from '../../ui/Card.svelte';
  import Icon, { type IconName } from '../../ui/Icon.svelte';

  const app = getAppState();
  const activity = app.activity;

  const ICONS: Record<ActivityIcon, IconName> = {
    file: 'file',
    link: 'bolt',
    unlink: 'unlink',
    lock: 'lock',
    alert: 'alert',
  };

  function fileSummary(entry: TransferEntry): string {
    const shown = entry.names.slice(0, 3).join(', ');
    const extra = entry.names.length > 3 ? ` and ${entry.names.length - 3} more` : '';
    return shown ? `${shown}${extra}` : pluralize(entry.count, 'file');
  }

  function transferStatus(entry: TransferEntry): string {
    switch (entry.state) {
      case 'saved':
        return `Saved to ${entry.dest || 'Downloads'}`;
      case 'discarded':
        return 'Discarded';
      case 'saving':
        return 'Saving…';
      case 'discarding':
        return 'Discarding…';
      default:
        return 'Waiting for you to save or discard';
    }
  }

  const toneClass = (tone: ActivityTone) => `tone-${tone}`;
</script>

<Card title="Activity">
  {#if activity.entries.length === 0}
    <p class="muted empty">Nothing yet. Received files and session events show up here.</p>
  {:else}
    <ul class="feed">
      {#each activity.entries as entry (entry.id)}
        <li class="item">
          {#if entry.kind === 'transfer'}
            <span class="badge tone-info"><Icon name="file" /></span>
            <div class="text">
              <span class="title">{pluralize(entry.count, 'file')} received from {entry.from}</span>
              <span class="sub">{fileSummary(entry)} · {formatClock(entry.at)}</span>
              <span class="sub" class:done={entry.state === 'saved'}>{transferStatus(entry)}</span>
            </div>
            {#if entry.state === 'pending' || entry.state === 'saving' || entry.state === 'discarding'}
              <div class="actions">
                <button
                  type="button"
                  class="btn btn-sm"
                  disabled={entry.state !== 'pending'}
                  aria-label="Discard {pluralize(entry.count, 'received file')}"
                  onclick={() => activity.discard(entry.id)}>Discard</button
                >
                <button
                  type="button"
                  class="btn btn-sm btn-primary"
                  disabled={entry.state !== 'pending'}
                  onclick={() => activity.save(entry.id)}
                >
                  {#if entry.state === 'saving'}<span class="spinner" aria-hidden="true"></span>{/if}Save to Downloads
                </button>
              </div>
            {/if}
          {:else}
            <span class="badge {toneClass(entry.tone)}"><Icon name={ICONS[entry.icon]} /></span>
            <div class="text">
              <span class="title">{entry.title}</span>
              <span class="sub">{entry.detail ? `${entry.detail} · ` : ''}{formatClock(entry.at)}</span>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</Card>

<style>
  .empty {
    font-size: 13px;
  }

  .feed {
    display: flex;
    flex-direction: column;
    max-height: 420px;
    overflow-y: auto;
  }

  .item {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    padding: 12px 0;
    border-bottom: 1px solid var(--line);
  }

  .item:last-child {
    border-bottom: 0;
  }

  .badge {
    width: 32px;
    height: 32px;
    flex: none;
    border-radius: var(--radius);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .tone-info {
    background: var(--info-soft);
    color: var(--info);
  }

  .tone-accent {
    background: var(--accent-soft);
    color: var(--accent);
  }

  .tone-muted {
    background: var(--panel2);
    color: var(--muted);
  }

  .tone-warning {
    background: var(--warning-soft);
    color: var(--warning);
  }

  .text {
    flex: 1 1 200px;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .title {
    font-weight: 500;
  }

  .sub {
    font-size: 12px;
    color: var(--muted);
    overflow-wrap: anywhere;
  }

  .sub.done {
    color: var(--accent);
  }

  .actions {
    display: flex;
    gap: 8px;
  }
</style>
