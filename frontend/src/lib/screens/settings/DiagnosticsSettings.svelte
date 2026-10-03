<script lang="ts">
  import { onMount } from 'svelte';
  import { getAppState } from '../../stores/app.svelte';
  import { formatDuration, healthSummary } from '../../utils';
  import Card from '../../ui/Card.svelte';
  import Icon from '../../ui/Icon.svelte';
  import StatusDot from '../../ui/StatusDot.svelte';

  const app = getAppState();
  const health = app.health;

  onMount(() => {
    void health.loadDiagnostics();
  });

  const summary = $derived(healthSummary(health.status));
  const metrics = $derived(Object.entries(health.metrics).sort(([a], [b]) => a.localeCompare(b)));

  function lineTone(line: string): string {
    if (/\b(ERROR|PANIC|FATAL)\b/.test(line)) return 'error';
    if (/\bWARN(ING)?\b/.test(line)) return 'warn';
    return '';
  }

  function levelTone(level: string): 'bad' | 'warn' | 'info' {
    if (level === 'critical' || level === 'error') return 'bad';
    if (level === 'warn' || level === 'warning') return 'warn';
    return 'info';
  }

  async function copyLogs() {
    try {
      await navigator.clipboard?.writeText(health.logs.join('\n'));
      app.toasts.success('Logs copied to the clipboard.');
    } catch {
      app.toasts.error("Couldn't copy the logs.");
    }
  }

  function analyzedAt(value: string): string {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString();
  }
</script>

<div class="stack">
  {#if health.diagnosticsError}
    <div class="notice warning" role="status"><Icon name="alert" /><span>{health.diagnosticsError}</span></div>
  {/if}

  <div class="pair">
    <Card title="Health">
      {#snippet actions()}
        <span class="summary tone-{summary.tone}">{summary.label}</span>
      {/snippet}
      <ul class="rows">
        {#each health.status.subsystems as subsystem (subsystem.name)}
          <li class="row">
            <StatusDot tone={subsystem.healthy ? 'ok' : 'bad'} label={subsystem.healthy ? 'healthy' : 'unhealthy'} />
            <span class="grow">{subsystem.name}</span>
            <span class="mono muted small">{subsystem.detail}</span>
          </li>
        {/each}
      </ul>
      <dl class="facts">
        <div><dt>Uptime</dt><dd class="mono">{formatDuration(health.status.uptime)}</dd></div>
        <div>
          <dt>Goroutines</dt>
          <dd class="mono">
            {health.status.goroutines}{#if health.status.goroutineDelta}<span class="muted">
                ({health.status.goroutineDelta > 0 ? '+' : ''}{health.status.goroutineDelta})</span
              >{/if}
          </dd>
        </div>
      </dl>
    </Card>

    <Card title="Load">
      {#if metrics.length === 0}
        <p class="muted small">No counters reported.</p>
      {:else}
        <dl class="metrics">
          {#each metrics as [key, value] (key)}
            <div class="metric"><dt class="mono">{key}</dt><dd class="mono">{value.toLocaleString()}</dd></div>
          {/each}
        </dl>
      {/if}
    </Card>
  </div>

  <Card
    title="Log analysis"
    description={health.analysis.windowLines
      ? `${health.analysis.totalErrors} errors in the last ${health.analysis.windowLines} log lines${health.analysis.analyzedAt ? ` · analysed ${analyzedAt(health.analysis.analyzedAt)}` : ''}`
      : 'Recurring patterns found in recent logs.'}
  >
    {#if health.analysis.events.length === 0}
      <p class="muted small">No recurring warnings or errors.</p>
    {:else}
      <ul class="events">
        {#each health.analysis.events as event, index (index)}
          <li class="event">
            <StatusDot tone={levelTone(event.level)} />
            <div class="grow">
              <span class="event-title"><span class="mono">{event.pattern}</span> · {event.count}×</span>
              <span class="mono muted small sample">{event.sample}</span>
            </div>
            <span class="level level-{levelTone(event.level)}">{event.level}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>

  <Card title="Recent logs">
    {#snippet actions()}
      <button type="button" class="btn btn-sm" onclick={() => health.loadDiagnostics()} disabled={health.diagnosticsLoading}>
        <Icon name="refresh" />Refresh
      </button>
      <button type="button" class="btn btn-sm" onclick={copyLogs} disabled={health.logs.length === 0}>
        <Icon name="copy" />Copy logs
      </button>
    {/snippet}
    {#if health.logs.length === 0}
      <p class="muted small">{health.diagnosticsLoading ? 'Loading…' : 'No log lines yet.'}</p>
    {:else}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex (scrollable log must be keyboard-scrollable) -->
      <pre class="logs selectable" tabindex="0" aria-label="Recent log lines">{#each health.logs as line, index (index)}<span class="line {lineTone(line)}">{line}</span>
{/each}</pre>
    {/if}
  </Card>
</div>

<style>
  .stack {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .pair {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 18px;
    align-items: start;
  }

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

  .row,
  .event {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 34px;
    font-size: 13px;
  }

  .event {
    align-items: flex-start;
    padding: 8px 0;
    border-bottom: 1px solid var(--line);
  }

  .event:last-child {
    border-bottom: 0;
  }

  .event :global(.dot) {
    margin-top: 6px;
  }

  .grow {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .sample {
    overflow-wrap: anywhere;
  }

  .level {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .level-bad {
    color: var(--danger);
  }

  .level-warn {
    color: var(--warning);
  }

  .level-info {
    color: var(--info);
  }

  .facts,
  .metrics {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px 18px;
  }

  .metrics {
    grid-template-columns: 1fr;
    gap: 0;
  }

  .metric {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    padding: 6px 0;
    border-bottom: 1px solid var(--line);
    font-size: 13px;
  }

  .metric:last-child {
    border-bottom: 0;
  }

  dt {
    font-size: 12px;
    color: var(--muted);
  }

  .metric dt {
    font-size: 13px;
  }

  dd {
    margin: 2px 0 0;
  }

  .metric dd {
    margin: 0;
  }

  .logs {
    margin: 0;
    max-height: 320px;
    overflow: auto;
    padding: 12px;
    border-radius: var(--radius);
    background: var(--panel2);
    border: 1px solid var(--line);
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.6;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .line.error {
    color: var(--danger);
  }

  .line.warn {
    color: var(--warning);
  }

  @media (max-width: 880px) {
    .pair {
      grid-template-columns: 1fr;
    }
  }
</style>
