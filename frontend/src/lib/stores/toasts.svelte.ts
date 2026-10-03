export type ToastTone = 'info' | 'success' | 'warning' | 'error';

export interface ToastAction {
  label: string;
  run: () => void;
}

export interface Toast {
  id: number;
  tone: ToastTone;
  message: string;
  action?: ToastAction;
}

export interface ToastOptions {
  /** Milliseconds before auto-dismiss; 0 keeps the toast until dismissed. */
  timeout?: number;
  action?: ToastAction;
}

const DEFAULT_TIMEOUT: Record<ToastTone, number> = {
  info: 4500,
  success: 4000,
  warning: 8000,
  error: 8000,
};

const MAX_TOASTS = 4;

export class ToastStore {
  items = $state<Toast[]>([]);
  #seq = 0;
  #timers = new Map<number, ReturnType<typeof setTimeout>>();

  push(message: string, tone: ToastTone = 'info', options: ToastOptions = {}): number {
    const id = ++this.#seq;
    const toast: Toast = { id, tone, message, ...(options.action ? { action: options.action } : {}) };
    const next = [...this.items, toast];
    // Keep the region short; drop the oldest first.
    while (next.length > MAX_TOASTS) {
      const dropped = next.shift();
      if (dropped) this.#clearTimer(dropped.id);
    }
    this.items = next;
    const timeout = options.timeout ?? DEFAULT_TIMEOUT[tone];
    if (timeout > 0) {
      this.#timers.set(
        id,
        setTimeout(() => this.dismiss(id), timeout),
      );
    }
    return id;
  }

  info(message: string, options?: ToastOptions) {
    return this.push(message, 'info', options);
  }

  success(message: string, options?: ToastOptions) {
    return this.push(message, 'success', options);
  }

  warning(message: string, options?: ToastOptions) {
    return this.push(message, 'warning', options);
  }

  error(message: string, options?: ToastOptions) {
    return this.push(message, 'error', options);
  }

  dismiss(id: number) {
    this.#clearTimer(id);
    this.items = this.items.filter((toast) => toast.id !== id);
  }

  dispose() {
    for (const timer of this.#timers.values()) clearTimeout(timer);
    this.#timers.clear();
    this.items = [];
  }

  #clearTimer(id: number) {
    const timer = this.#timers.get(id);
    if (timer) clearTimeout(timer);
    this.#timers.delete(id);
  }
}
