// Composition root: creates every store for one Api, wires backend events
// into them, and exposes navigation state. Components read it from context.

import { getContext, setContext } from 'svelte';
import type { Api, Peer, Unsubscribe } from '../api/types';
import { pluralize } from '../utils';
import { ActivityStore } from './activity.svelte';
import { ConnectionStore } from './connection.svelte';
import { HealthStore } from './health.svelte';
import { SettingsStore } from './settings.svelte';
import { ThemeStore } from './theme.svelte';
import { ToastStore } from './toasts.svelte';

export type Screen = 'session' | 'devices' | 'settings';
export type SettingsSection = 'input' | 'audio' | 'startup' | 'security' | 'diagnostics';

export const SETTINGS_SECTIONS: ReadonlyArray<{ id: SettingsSection; label: string }> = [
  { id: 'input', label: 'Input & screens' },
  { id: 'audio', label: 'Audio' },
  { id: 'startup', label: 'Startup' },
  { id: 'security', label: 'Security' },
  { id: 'diagnostics', label: 'Diagnostics' },
];

export interface PairingRequest {
  peer: Peer;
  address: string;
}

export class AppState {
  readonly api: Api;
  readonly toasts = new ToastStore();
  readonly theme = new ThemeStore();
  readonly activity: ActivityStore;
  readonly connection: ConnectionStore;
  readonly settings: SettingsStore;
  readonly health: HealthStore;

  screen = $state<Screen>('session');
  settingsSection = $state<SettingsSection>('input');
  pairing = $state<PairingRequest | null>(null);
  ready = $state(false);

  #unsubscribe: Unsubscribe[] = [];

  constructor(api: Api) {
    this.api = api;
    this.activity = new ActivityStore(api, this.toasts);
    this.connection = new ConnectionStore(api, this.toasts, this.activity);
    this.health = new HealthStore(api);
    this.settings = new SettingsStore(api, (_key, label, message) => {
      this.toasts.error(`Couldn't save the ${label}: ${message}`);
    });
  }

  get isPreview(): boolean {
    return this.api.kind === 'preview';
  }

  async start(): Promise<void> {
    this.theme.start();
    this.#subscribe();
    await Promise.all([this.connection.load(), this.settings.load(), this.health.load()]);
    this.ready = true;
  }

  stop(): void {
    for (const off of this.#unsubscribe) {
      try {
        off();
      } catch {
        // Ignore teardown errors from the runtime.
      }
    }
    this.#unsubscribe = [];
    this.theme.stop();
    this.toasts.dispose();
  }

  navigate(screen: Screen, section?: SettingsSection): void {
    this.screen = screen;
    if (section) this.settingsSection = section;
  }

  openPairing(peer: Peer, address = this.connection.addressFor(peer)): void {
    this.pairing = { peer, address };
  }

  closePairing(): void {
    this.pairing = null;
  }

  #subscribe(): void {
    const { api, connection, health, activity, toasts } = this;
    this.#unsubscribe.push(
      api.on('device-updated', (device) => (connection.device = device)),
      api.on('peers-updated', (peers) => (connection.peers = peers)),
      api.on('session-updated', (session) => connection.applySession(session)),
      api.on('tailscale-updated', (status) => (connection.tailscale = status)),
      api.on('health-updated', (status) => (health.status = status)),
      api.on('health-alert', (alert) => {
        health.addAlert(alert);
        activity.addEvent('alert', 'warning', `${alert.subsystem}: needs attention`, alert.message);
        toasts.warning(`${alert.subsystem}: ${alert.message}`, {
          action: { label: 'View', run: () => this.navigate('session') },
        });
      }),
      api.on('file-received', (event) => {
        if (!event.count && !event.names.length) return;
        const from = connection.session.peerName || 'the other PC';
        const entry = activity.addTransfer(event, from);
        toasts.info(`${pluralize(entry.count, 'file')} received from ${from}. Save or discard them in Activity.`, {
          timeout: 8000,
          action: { label: 'Review', run: () => this.navigate('session') },
        });
      }),
    );
  }
}

const KEY = Symbol('multisnek-app');

export function setAppState(state: AppState): AppState {
  return setContext(KEY, state);
}

export function getAppState(): AppState {
  const state = getContext<AppState | undefined>(KEY);
  if (!state) throw new Error('AppState is not available; render inside <App>.');
  return state;
}
