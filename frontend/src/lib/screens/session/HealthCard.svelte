<script lang="ts">
  import { getAppState } from '../../stores/app.svelte';
  import { formatClock, healthSummary } from '../../utils';
  import Card from '../../ui/Card.svelte';
  import Icon from '../../ui/Icon.svelte';
  import StatusDot from '../../ui/StatusDot.svelte';

  const app = getAppState();
  const health = app.health;

  const summary = $derived(healthSummary(health.status));
</script>

<Card title="Health">
  {#snippet actions()}
    <span class="summary tone-{summary.tone}">{summary.label}</span>
  {/snippet}

  {#if health.alerts.length > 0}
    <ul class="alerts" aria-label="Health alerts">
      {#each health.alerts as alert (alert.id)}
        <li class="alert">
          <span class="alert-icon"><Icon name="alert" /></span>
          <div class="alert-text">
            <span class="alert-title">{alert.subsystem}</span>
            <span class="alert-msg">{alert.message} · {formatClock(alert.at)}</span>
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-icon btn-sm"
            aria-label="Dismiss {alert.subsystem} alert"
            onclick={() => health.dismissAlert(alert.id)}
          >
            <Icon name="close" />
          </button>
        </li>
      {/each}
    </ul>
  {/if}

  {#if health.status.subsystems.length === 0}
    <p class="muted small">No subsystem reports yet.</p>
  {:else}
    <ul class="subsystems">
      {#each health.status.subsystems as subsystem (subsystem.name)}
        <li class="subsystem">
          <StatusDot tone={subsystem.healthy ? 'ok' : 'bad'} label={subsystem.healthy ? 'healthy' : 'unhealthy'} />
          <span class="name">{subsystem.name}</span>
          <span class="detail mono">{subsystem.detail}</span>
        </li>
      {/each}
    </ul>
  {/if}
  <button type="button" class="link-btn diag-link" onclick={() => app.navigate('settings', 'diagnostics')}>
    Open diagnostics
  </button>
</Card>

<style>
  .summary {
    font-size: 12px;
    font-weight: 500;
  }

  .tone-ok {
    color: var(--accent);
  }

  .tone-warn {
    color: var(--warning);
  }

  .tone-bad {
    color: var(--danger);
  }

  .alerts {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .alert {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 8px 6px 8px 10px;
    border-radius: var(--radius);
    background: var(--warning-soft);
  }

  .alert-icon {
    color: var(--warning);
    display: flex;
    padding-top: 2px;
  }

  .alert-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .alert-title {
    font-weight: 600;
    font-size: 13px;
  }

  .alert-msg {
    font-size: 12px;
    overflow-wrap: anywhere;
  }

  .subsystem {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 34px;
    font-size: 13px;
  }

  .name {
    flex: 1;
  }

  .detail {
    color: var(--muted);
    font-size: 12px;
    text-align: right;
    overflow-wrap: anywhere;
  }

  .diag-link {
    align-self: flex-start;
  }
</style>
