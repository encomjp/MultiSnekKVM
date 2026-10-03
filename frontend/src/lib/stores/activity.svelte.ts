// Session activity feed: received file transfers (with Save / Discard) and
// notable session events. Transfers are tracked individually so each temp
// folder is either saved or discarded, never orphaned.

import type { Api, FileReceivedEvent } from '../api/types';
import { errorMessage, pluralize } from '../utils';
import type { ToastStore } from './toasts.svelte';

export type ActivityIcon = 'file' | 'link' | 'unlink' | 'lock' | 'alert';
export type ActivityTone = 'accent' | 'info' | 'muted' | 'warning';

export interface TransferEntry {
  kind: 'transfer';
  id: number;
  at: Date;
  count: number;
  names: string[];
  tempDir: string;
  from: string;
  state: 'pending' | 'saving' | 'discarding' | 'saved' | 'discarded';
  dest?: string;
}

export interface EventEntry {
  kind: 'event';
  id: number;
  at: Date;
  icon: ActivityIcon;
  tone: ActivityTone;
  title: string;
  detail: string;
}

export type ActivityEntry = TransferEntry | EventEntry;

const MAX_ENTRIES = 30;

export class ActivityStore {
  entries = $state<ActivityEntry[]>([]);
  pendingTransfers = $derived(
    this.entries.filter((entry): entry is TransferEntry => entry.kind === 'transfer' && entry.state === 'pending'),
  );

  readonly #api: Api;
  readonly #toasts: ToastStore;
  #seq = 0;

  constructor(api: Api, toasts: ToastStore) {
    this.#api = api;
    this.#toasts = toasts;
  }

  #prepend(entry: ActivityEntry) {
    const next = [entry, ...this.entries];
    // Trim old finished entries but never drop an unresolved transfer.
    while (next.length > MAX_ENTRIES) {
      const index = next.findLastIndex((item) => item.kind === 'event' || item.state === 'saved' || item.state === 'discarded');
      if (index < 0) break;
      next.splice(index, 1);
    }
    this.entries = next;
  }

  addEvent(icon: ActivityIcon, tone: ActivityTone, title: string, detail = ''): void {
    this.#prepend({ kind: 'event', id: ++this.#seq, at: new Date(), icon, tone, title, detail });
  }

  addTransfer(event: FileReceivedEvent, from: string): TransferEntry {
    const entry: TransferEntry = {
      kind: 'transfer',
      id: ++this.#seq,
      at: new Date(),
      count: event.count || event.names.length,
      names: event.names,
      tempDir: event.tempDir,
      from,
      state: 'pending',
    };
    this.#prepend(entry);
    return entry;
  }

  #update(id: number, patch: Partial<TransferEntry>) {
    this.entries = this.entries.map((entry) =>
      entry.kind === 'transfer' && entry.id === id ? { ...entry, ...patch } : entry,
    );
  }

  #find(id: number): TransferEntry | undefined {
    return this.entries.find((entry): entry is TransferEntry => entry.kind === 'transfer' && entry.id === id);
  }

  async save(id: number): Promise<void> {
    const entry = this.#find(id);
    if (!entry || entry.state !== 'pending') return;
    this.#update(id, { state: 'saving' });
    try {
      const result = await this.#api.SaveReceivedFiles(entry.tempDir);
      this.#update(id, { state: 'saved', dest: result.dest });
      const saved = result.saved.length || entry.count;
      this.#toasts.success(`${pluralize(saved, 'file')} saved to ${result.dest || 'Downloads'}.`);
    } catch (error) {
      this.#update(id, { state: 'pending' });
      this.#toasts.error(errorMessage(error, "Couldn't save the received files."));
    }
  }

  async discard(id: number): Promise<void> {
    const entry = this.#find(id);
    if (!entry || entry.state !== 'pending') return;
    this.#update(id, { state: 'discarding' });
    try {
      await this.#api.DiscardReceivedFiles(entry.tempDir);
      this.#update(id, { state: 'discarded' });
    } catch (error) {
      this.#update(id, { state: 'pending' });
      this.#toasts.error(errorMessage(error, "Couldn't discard the received files."));
    }
  }
}
