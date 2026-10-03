// Api implementation backed by the Wails bindings in frontend/wailsjs.
// All calls go through the generated wrappers (never window.go directly) and
// every payload is normalized, because Go encodes nil slices as null.

import * as App from '../../../wailsjs/go/app/App';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import {
  normalizeAudioDevices,
  normalizeAudioMode,
  normalizeAudioProfile,
  normalizeAudioTiming,
  normalizeAudioTransport,
  normalizeBluetooth,
  normalizeDevice,
  normalizeEdgeSide,
  normalizeFileReceived,
  normalizeHealth,
  normalizeHealthAlert,
  normalizeHotkey,
  normalizeInterfaces,
  normalizeLastPeer,
  normalizeLogAnalysis,
  normalizeMicMode,
  normalizeMonitors,
  normalizeNumberMap,
  normalizePeers,
  normalizeReturnAnchor,
  normalizeSession,
  normalizeTailscale,
  normalizeTriggerZone,
} from './normalize';
import type { Api, EventMap, EventName } from './types';

const eventNormalizers: { [E in EventName]: (raw: unknown) => EventMap[E] } = {
  'device-updated': normalizeDevice,
  'peers-updated': normalizePeers,
  'session-updated': normalizeSession,
  'tailscale-updated': normalizeTailscale,
  'bluetooth-updated': normalizeBluetooth,
  'health-updated': normalizeHealth,
  'health-alert': normalizeHealthAlert,
  'file-received': normalizeFileReceived,
};

const strings = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : [];

export function hasWailsBackend(): boolean {
  return typeof window !== 'undefined' && !!window.go?.app?.App;
}

export function createWailsApi(): Api {
  return {
    kind: 'wails',

    on(event, callback) {
      if (typeof window === 'undefined' || !window.runtime?.EventsOnMultiple) return () => {};
      const normalize = eventNormalizers[event];
      const off = EventsOn(event, (payload: unknown) => callback(normalize(payload)));
      return typeof off === 'function' ? off : () => {};
    },

    GetDevice: async () => normalizeDevice(await App.GetDevice()),
    GetPeers: async () => normalizePeers(await App.GetPeers()),
    GetSession: async () => normalizeSession(await App.GetSession()),
    GetTailscaleStatus: async () => normalizeTailscale(await App.GetTailscaleStatus()),
    GetConnectionInterfaces: async () => normalizeInterfaces(await App.GetConnectionInterfaces()),
    GetLastPeer: async () => normalizeLastPeer(await App.GetLastPeer()),

    GetBluetoothStatus: async () => normalizeBluetooth(await App.GetBluetoothStatus()),
    SetBluetoothEnabled: (enabled) => App.SetBluetoothEnabled(enabled),
    RefreshBluetooth: () => App.RefreshBluetooth(),

    AddPeer: (address) => App.AddPeer(address),
    RemovePeer: (address) => App.RemovePeer(address),
    Connect: (address) => App.Connect(address),
    Disconnect: () => App.Disconnect(),
    Reconnect: () => App.Reconnect(),
    TrustPeer: (address, pin) => App.TrustPeer(address, pin),
    UntrustPeer: (peerID) => App.UntrustPeer(peerID),
    GetAutoReconnect: async () => (await App.GetAutoReconnect()) !== false,
    SetAutoReconnect: (enabled) => App.SetAutoReconnect(enabled),

    GetHealthStatus: async () => normalizeHealth(await App.GetHealthStatus()),
    GetRecentLogs: async () => strings(await App.GetRecentLogs()),
    GetLogAnalysis: async () => normalizeLogAnalysis(await App.GetLogAnalysis()),
    GetLoadMetrics: async () => normalizeNumberMap(await App.GetLoadMetrics()),

    GetEdgeSide: async () => normalizeEdgeSide(await App.GetEdgeSide()),
    SetEdgeSide: (side) => App.SetEdgeSide(side),
    GetSensitivity: async () => {
      const value = await App.GetSensitivity();
      return typeof value === 'number' && value > 0 ? value : 1;
    },
    SetSensitivity: (value) => App.SetSensitivity(value),
    GetExitHotkey: async () => normalizeHotkey(await App.GetExitHotkey()),
    SetExitHotkey: (modifiers, vkCode) => App.SetExitHotkey(modifiers, vkCode),
    GetLocalMonitors: async () => normalizeMonitors(await App.GetLocalMonitors()),
    GetTriggerZone: async () => normalizeTriggerZone(await App.GetTriggerZone()),
    SetTriggerZone: (monitorID, side, startPct, endPct) => App.SetTriggerZone(monitorID, side, startPct, endPct),
    ClearTriggerZone: () => App.ClearTriggerZone(),
    GetReturnAnchor: async () => normalizeReturnAnchor(await App.GetReturnAnchor()),
    SetReturnAnchor: (monitorID, xPct, yPct) => App.SetReturnAnchor(monitorID, xPct, yPct),
    ClearReturnAnchor: () => App.ClearReturnAnchor(),

    GetAutostart: async () => (await App.GetAutostart()) === true,
    SetAutostart: (enabled) => App.SetAutostart(enabled),
    GetStartMinimized: async () => (await App.GetStartMinimized()) === true,
    SetStartMinimized: (enabled) => App.SetStartMinimized(enabled),

    GetAudioMode: async () => normalizeAudioMode(await App.GetAudioMode()),
    SetAudioMode: (mode) => App.SetAudioMode(mode),
    GetAudioTiming: async () => normalizeAudioTiming(await App.GetAudioTiming()),
    SetAudioTiming: (timing) => App.SetAudioTiming(timing),
    GetAudioTransport: async () => normalizeAudioTransport(await App.GetAudioTransport()),
    SetAudioTransport: (transport) => App.SetAudioTransport(transport),
    GetAudioProfile: async () => normalizeAudioProfile(await App.GetAudioProfile()),
    SetAudioProfile: (profile) => App.SetAudioProfile(profile),
    GetMicMode: async () => normalizeMicMode(await App.GetMicMode()),
    SetMicMode: (mode) => App.SetMicMode(mode),
    GetMuteSource: async () => (await App.GetMuteSource()) === true,
    SetMuteSource: (enabled) => App.SetMuteSource(enabled),
    GetAudioDevices: async () => normalizeAudioDevices(await App.GetAudioDevices()),
    GetCaptureDeviceID: async () => (await App.GetCaptureDeviceID()) || '',
    SetCaptureDeviceID: (id) => App.SetCaptureDeviceID(id),
    GetPlaybackDeviceID: async () => (await App.GetPlaybackDeviceID()) || '',
    SetPlaybackDeviceID: (id) => App.SetPlaybackDeviceID(id),
    GetMicDeviceID: async () => (await App.GetMicDeviceID()) || '',
    SetMicDeviceID: (id) => App.SetMicDeviceID(id),
    GetMicPlaybackDeviceID: async () => (await App.GetMicPlaybackDeviceID()) || '',
    SetMicPlaybackDeviceID: (id) => App.SetMicPlaybackDeviceID(id),

    PickAndSendFiles: () => App.PickAndSendFiles(),
    SendFiles: (paths) => App.SendFiles(paths),
    SaveReceivedFiles: async (tempDir) => {
      const result = await App.SaveReceivedFiles(tempDir);
      return { dest: result?.dest || '', saved: strings(result?.saved) };
    },
    DiscardReceivedFiles: (tempDir) => App.DiscardReceivedFiles(tempDir),
  };
}
