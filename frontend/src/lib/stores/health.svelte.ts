import { emptyHealth, emptyLogAnalysis } from '../api/normalize';
import type { Api, HealthAlert, HealthStatus, LogAnalysis } from '../api/types';
import { errorMessage } from '../utils';

export interface AlertEntry extends HealthAlert {
  id: number;
  at: Date;
}

const MAX_ALERTS = 10;

export class HealthStore {
  status = $state<HealthStatus>({ ...emptyHealth });
  alerts = $state<AlertEntry[]>([]);
  logs = $state<string[]>([]);
  analysis = $state<LogAnalysis>({ ...emptyLogAnalysis });
  metrics = $state<Record<string, number>>({});
  diagnosticsLoading = $state(false);
  diagnosticsError = $state('');

  readonly #api: Api;
  #seq = 0;

  constructor(api: Api) {
    this.#api = api;
  }

  async load(): Promise<void> {
    try {
      this.status = await this.#api.GetHealthStatus();
    } catch {
      // Health is best effort.
    }
  }

  addAlert(alert: HealthAlert): AlertEntry {
    const entry: AlertEntry = { ...alert, id: ++this.#seq, at: new Date() };
    this.alerts = [entry, ...this.alerts].slice(0, MAX_ALERTS);
    return entry;
  }

  dismissAlert(id: number): void {
    this.alerts = this.alerts.filter((alert) => alert.id !== id);
  }

  async loadDiagnostics(): Promise<void> {
    this.diagnosticsLoading = true;
    this.diagnosticsError = '';
    const results = await Promise.allSettled([
      this.#api.GetRecentLogs(),
      this.#api.GetLogAnalysis(),
      this.#api.GetLoadMetrics(),
      this.#api.GetHealthStatus(),
    ]);
    const [logs, analysis, metrics, health] = results;
    if (logs.status === 'fulfilled') this.logs = logs.value;
    if (analysis.status === 'fulfilled') this.analysis = analysis.value;
    if (metrics.status === 'fulfilled') this.metrics = metrics.value;
    if (health.status === 'fulfilled') this.status = health.value;
    const failed = results.find((result): result is PromiseRejectedResult => result.status === 'rejected');
    if (failed) this.diagnosticsError = errorMessage(failed.reason, 'Some diagnostics could not be loaded.');
    this.diagnosticsLoading = false;
  }
}
