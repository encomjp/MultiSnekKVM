// One store for every persisted setting. Each setting saves independently:
// it updates optimistically, tracks its own pending/error state, and on a
// failed save reverts only itself (no global lock, no full reload).

import {
  normalizeAudioDevices,
  normalizeMonitors,
} from '../api/normalize';
import type {
  Api,
  AudioDevice,
  AudioMode,
  AudioProfile,
  AudioTiming,
  AudioTransport,
  EdgeSide,
  ExitHotkey,
  MicMode,
  MonitorInfo,
  ReturnAnchor,
  TriggerZone,
} from '../api/types';
import { errorMessage } from '../utils';

export interface SettingValues {
  edgeSide: EdgeSide;
  sensitivity: number;
  exitHotkey: ExitHotkey;
  triggerZone: TriggerZone | null;
  returnAnchor: ReturnAnchor | null;
  autostart: boolean;
  startMinimized: boolean;
  autoReconnect: boolean;
  audioMode: AudioMode;
  audioTiming: AudioTiming;
  audioTransport: AudioTransport;
  audioProfile: AudioProfile;
  micMode: MicMode;
  muteSource: boolean;
  captureDeviceID: string;
  playbackDeviceID: string;
  micDeviceID: string;
  micPlaybackDeviceID: string;
}

export type SettingKey = keyof SettingValues;

export interface SettingStatus {
  pending: boolean;
  error: string;
}

interface SettingDef<K extends SettingKey> {
  label: string;
  get(api: Api): Promise<SettingValues[K]>;
  set(api: Api, value: SettingValues[K]): Promise<void>;
}

type SettingDefs = { [K in SettingKey]: SettingDef<K> };

export const SETTING_DEFS: SettingDefs = {
  edgeSide: { label: 'screen edge', get: (api) => api.GetEdgeSide(), set: (api, v) => api.SetEdgeSide(v) },
  sensitivity: { label: 'pointer speed', get: (api) => api.GetSensitivity(), set: (api, v) => api.SetSensitivity(v) },
  exitHotkey: {
    label: 'exit hotkey',
    get: (api) => api.GetExitHotkey(),
    set: (api, v) => api.SetExitHotkey(v.modifiers, v.vkCode),
  },
  triggerZone: {
    label: 'hand-off zone',
    get: (api) => api.GetTriggerZone(),
    set: (api, v) => (v ? api.SetTriggerZone(v.monitorID, v.side, v.startPct, v.endPct) : api.ClearTriggerZone()),
  },
  returnAnchor: {
    label: 'return point',
    get: (api) => api.GetReturnAnchor(),
    set: (api, v) => (v ? api.SetReturnAnchor(v.monitorID, v.xPct, v.yPct) : api.ClearReturnAnchor()),
  },
  autostart: { label: 'start with Windows', get: (api) => api.GetAutostart(), set: (api, v) => api.SetAutostart(v) },
  startMinimized: {
    label: 'start minimized',
    get: (api) => api.GetStartMinimized(),
    set: (api, v) => api.SetStartMinimized(v),
  },
  autoReconnect: {
    label: 'auto-reconnect',
    get: (api) => api.GetAutoReconnect(),
    set: (api, v) => api.SetAutoReconnect(v),
  },
  audioMode: { label: 'desktop audio', get: (api) => api.GetAudioMode(), set: (api, v) => api.SetAudioMode(v) },
  audioTiming: { label: 'audio timing', get: (api) => api.GetAudioTiming(), set: (api, v) => api.SetAudioTiming(v) },
  audioTransport: {
    label: 'audio transport',
    get: (api) => api.GetAudioTransport(),
    set: (api, v) => api.SetAudioTransport(v),
  },
  audioProfile: {
    label: 'audio quality',
    get: (api) => api.GetAudioProfile(),
    set: (api, v) => api.SetAudioProfile(v),
  },
  micMode: { label: 'microphone', get: (api) => api.GetMicMode(), set: (api, v) => api.SetMicMode(v) },
  muteSource: { label: 'mute while streaming', get: (api) => api.GetMuteSource(), set: (api, v) => api.SetMuteSource(v) },
  captureDeviceID: {
    label: 'capture device',
    get: (api) => api.GetCaptureDeviceID(),
    set: (api, v) => api.SetCaptureDeviceID(v),
  },
  playbackDeviceID: {
    label: 'playback device',
    get: (api) => api.GetPlaybackDeviceID(),
    set: (api, v) => api.SetPlaybackDeviceID(v),
  },
  micDeviceID: { label: 'microphone device', get: (api) => api.GetMicDeviceID(), set: (api, v) => api.SetMicDeviceID(v) },
  micPlaybackDeviceID: {
    label: 'remote microphone output',
    get: (api) => api.GetMicPlaybackDeviceID(),
    set: (api, v) => api.SetMicPlaybackDeviceID(v),
  },
};

export const SETTING_KEYS = Object.keys(SETTING_DEFS) as SettingKey[];

export const DEFAULT_SETTINGS: SettingValues = {
  edgeSide: 'right',
  sensitivity: 1,
  exitHotkey: { modifiers: 0, vkCode: 0 },
  triggerZone: null,
  returnAnchor: null,
  autostart: false,
  startMinimized: false,
  autoReconnect: true,
  audioMode: 'off',
  audioTiming: 'always',
  audioTransport: 'auto',
  audioProfile: 'balanced',
  micMode: 'off',
  muteSource: false,
  captureDeviceID: '',
  playbackDeviceID: '',
  micDeviceID: '',
  micPlaybackDeviceID: '',
};

function idleStatus(): Record<SettingKey, SettingStatus> {
  const status = {} as Record<SettingKey, SettingStatus>;
  for (const key of SETTING_KEYS) status[key] = { pending: false, error: '' };
  return status;
}

export type SettingErrorHandler = (key: SettingKey, label: string, message: string) => void;

export class SettingsStore {
  values = $state<SettingValues>({ ...DEFAULT_SETTINGS });
  status = $state<Record<SettingKey, SettingStatus>>(idleStatus());
  loaded = $state(false);
  monitors = $state<MonitorInfo[]>([]);
  audioDevices = $state<AudioDevice[]>([]);

  readonly #api: Api;
  readonly #onError: SettingErrorHandler | undefined;
  #generation: Partial<Record<SettingKey, number>> = {};

  constructor(api: Api, onError?: SettingErrorHandler) {
    this.#api = api;
    this.#onError = onError;
  }

  /** Load every setting in parallel. A failing getter keeps its default. */
  async load(): Promise<void> {
    await Promise.all([
      ...SETTING_KEYS.map((key) => this.reload(key)),
      this.refreshMonitors(),
      this.refreshAudioDevices(),
    ]);
    this.loaded = true;
  }

  async reload<K extends SettingKey>(key: K): Promise<void> {
    const def = SETTING_DEFS[key] as SettingDef<K>;
    const generation = this.#generation[key] ?? 0;
    try {
      const value = await def.get(this.#api);
      // Don't clobber a save that started while we were loading.
      if ((this.#generation[key] ?? 0) === generation) this.values[key] = value;
    } catch {
      // Keep the current value; the backend may not support this getter yet.
    }
  }

  /**
   * Save one setting. Returns true on success. On failure the value reverts
   * to what it was before this call, unless a newer save superseded it.
   */
  async set<K extends SettingKey>(key: K, value: SettingValues[K]): Promise<boolean> {
    const def = SETTING_DEFS[key] as SettingDef<K>;
    const previous = this.values[key];
    const generation = (this.#generation[key] ?? 0) + 1;
    this.#generation[key] = generation;

    this.values[key] = value;
    this.status[key] = { pending: true, error: '' };
    try {
      await def.set(this.#api, value);
      if (this.#generation[key] === generation) this.status[key] = { pending: false, error: '' };
      return true;
    } catch (error) {
      const message = errorMessage(error, `Couldn't save the ${def.label}.`);
      if (this.#generation[key] === generation) {
        this.values[key] = previous;
        this.status[key] = { pending: false, error: message };
      }
      this.#onError?.(key, def.label, message);
      return false;
    }
  }

  isPending(key: SettingKey): boolean {
    return this.status[key].pending;
  }

  errorOf(key: SettingKey): string {
    return this.status[key].error;
  }

  async refreshMonitors(): Promise<void> {
    try {
      this.monitors = normalizeMonitors(await this.#api.GetLocalMonitors());
    } catch {
      this.monitors = [];
    }
  }

  async refreshAudioDevices(): Promise<void> {
    try {
      this.audioDevices = normalizeAudioDevices(await this.#api.GetAudioDevices());
    } catch {
      // Keep the last known list.
    }
  }
}
