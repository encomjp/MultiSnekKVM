export type ThemePreference = 'dark' | 'light' | 'system';
export type ResolvedTheme = 'dark' | 'light';

const STORAGE_KEY = 'multisnek.theme';

function readPreference(): ThemePreference {
  try {
    const stored = globalThis.localStorage?.getItem(STORAGE_KEY);
    if (stored === 'dark' || stored === 'light' || stored === 'system') return stored;
  } catch {
    // Storage can be unavailable (private mode, blocked site data).
  }
  return 'system';
}

function systemTheme(): ResolvedTheme {
  try {
    return globalThis.matchMedia?.('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  } catch {
    return 'dark';
  }
}

export class ThemeStore {
  preference = $state<ThemePreference>(readPreference());
  #system = $state<ResolvedTheme>(systemTheme());
  resolved = $derived<ResolvedTheme>(this.preference === 'system' ? this.#system : this.preference);
  #media: MediaQueryList | null = null;
  #onChange = () => {
    this.#system = systemTheme();
    this.apply();
  };

  /** Start following the OS theme and apply the current one to <html>. */
  start(): void {
    try {
      this.#media = globalThis.matchMedia?.('(prefers-color-scheme: light)') ?? null;
      this.#media?.addEventListener?.('change', this.#onChange);
    } catch {
      this.#media = null;
    }
    this.apply();
  }

  stop(): void {
    this.#media?.removeEventListener?.('change', this.#onChange);
    this.#media = null;
  }

  set(preference: ThemePreference): void {
    this.preference = preference;
    try {
      globalThis.localStorage?.setItem(STORAGE_KEY, preference);
    } catch {
      // Not persisted; the choice still applies for this session.
    }
    this.apply();
  }

  apply(): void {
    if (typeof document === 'undefined') return;
    document.documentElement.dataset.theme = this.resolved;
    document.documentElement.style.colorScheme = this.resolved;
  }
}
