// Defaults and normalizers for backend payloads. Go encodes nil slices and
// maps as null and omits `omitempty` fields, so every value coming over the
// bridge passes through one of these before it reaches a store.

import type {
  AudioDevice,
  AudioMode,
  AudioProfile,
  AudioTiming,
  AudioTransport,
  ConnectionInterface,
  DeviceInfo,
  EdgeSide,
  ExitHotkey,
  FileReceivedEvent,
  HealthAlert,
  HealthStatus,
  LastPeer,
  LogAnalysis,
  MicMode,
  MonitorInfo,
  Peer,
  ReturnAnchor,
  SessionStatus,
  TailscaleStatus,
  TriggerZone,
} from './types';

const str = (value: unknown, fallback = ''): string => (typeof value === 'string' ? value : fallback);
const num = (value: unknown, fallback = 0): number =>
  typeof value === 'number' && Number.isFinite(value) ? value : fallback;
const bool = (value: unknown): boolean => value === true;
const strList = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : [];
const asRecord = (value: unknown): Record<string, unknown> =>
  value && typeof value === 'object' ? (value as Record<string, unknown>) : {};

export const DEFAULT_PORT = 24831;

export const emptyDevice: DeviceInfo = { id: '', name: '', fingerprint: '', port: DEFAULT_PORT };

export const emptySession: SessionStatus = {
  connected: false,
  controlling: false,
  peerName: '',
  peerID: '',
  role: '',
  latencyMs: -1,
  audioLatencyMs: -1,
  jitterMs: -1,
};

export const emptyHealth: HealthStatus = {
  healthy: true,
  reconnecting: false,
  subsystems: [],
  uptime: 0,
  goroutines: 0,
  goroutineDelta: 0,
};

export const emptyTailscale: TailscaleStatus = {
  available: false,
  connected: false,
  backendState: '',
  selfName: '',
  tailnet: '',
  selfIPs: [],
  peerCount: 0,
  targetCount: 0,
  lastSync: 0,
  lastError: '',
};

export const emptyLogAnalysis: LogAnalysis = { events: [], totalErrors: 0, windowLines: 0, analyzedAt: '' };

export function normalizeDevice(raw: unknown): DeviceInfo {
  const r = asRecord(raw);
  const pairingCode = str(r.pairingCode);
  return {
    id: str(r.id),
    name: str(r.name),
    fingerprint: str(r.fingerprint),
    ...(pairingCode ? { pairingCode } : {}),
    port: num(r.port, DEFAULT_PORT) || DEFAULT_PORT,
  };
}

export function normalizePeer(raw: unknown): Peer {
  const r = asRecord(raw);
  const address = str(r.address);
  const addresses = strList(r.addresses);
  const kinds: Record<string, string> = {};
  for (const [key, value] of Object.entries(asRecord(r.addressKinds))) {
    if (typeof value === 'string') kinds[key] = value;
  }
  return {
    id: str(r.id) || address,
    name: str(r.name),
    address,
    addresses: addresses.length ? addresses : address ? [address] : [],
    addressKinds: kinds,
    fingerprint: str(r.fingerprint),
    source: str(r.source),
    routes: strList(r.routes),
    preferredRoute: str(r.preferredRoute),
    trusted: bool(r.trusted),
    status: str(r.status, 'added'),
    lastSeen: num(r.lastSeen),
  };
}

export function normalizePeers(raw: unknown): Peer[] {
  return Array.isArray(raw) ? raw.map(normalizePeer) : [];
}

export function normalizeSession(raw: unknown): SessionStatus {
  const r = asRecord(raw);
  const route = str(r.route);
  const remoteAddress = str(r.remoteAddress);
  return {
    ...(route ? { route } : {}),
    ...(remoteAddress ? { remoteAddress } : {}),
    connected: bool(r.connected),
    controlling: bool(r.controlling),
    peerName: str(r.peerName),
    peerID: str(r.peerID),
    role: str(r.role),
    latencyMs: num(r.latencyMs, -1),
    audioLatencyMs: num(r.audioLatencyMs, -1),
    jitterMs: num(r.jitterMs, -1),
  };
}

export function normalizeTailscale(raw: unknown): TailscaleStatus {
  const r = asRecord(raw);
  return {
    available: bool(r.available),
    connected: bool(r.connected),
    backendState: str(r.backendState),
    selfName: str(r.selfName),
    tailnet: str(r.tailnet),
    selfIPs: strList(r.selfIPs),
    peerCount: num(r.peerCount),
    targetCount: num(r.targetCount),
    lastSync: num(r.lastSync),
    lastError: str(r.lastError),
  };
}

export function normalizeInterfaces(raw: unknown): ConnectionInterface[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((item) => {
    const r = asRecord(item);
    const description = str(r.description);
    return {
      name: str(r.name),
      ...(description ? { description } : {}),
      kind: str(r.kind, 'network'),
      addresses: strList(r.addresses),
    };
  });
}

export function normalizeLastPeer(raw: unknown): LastPeer | null {
  const out: LastPeer = {};
  for (const [key, value] of Object.entries(asRecord(raw))) {
    if (typeof value === 'string' && value) out[key] = value;
  }
  return Object.keys(out).length ? out : null;
}

export function normalizeHealth(raw: unknown): HealthStatus {
  const r = asRecord(raw);
  const subsystems = Array.isArray(r.subsystems)
    ? r.subsystems.map((item) => {
        const s = asRecord(item);
        return { name: str(s.name), healthy: s.healthy !== false, detail: str(s.detail) };
      })
    : [];
  return {
    healthy: r.healthy !== false,
    reconnecting: bool(r.reconnecting),
    subsystems,
    uptime: num(r.uptime),
    goroutines: num(r.goroutines),
    goroutineDelta: num(r.goroutineDelta),
  };
}

export function normalizeHealthAlert(raw: unknown): HealthAlert {
  const r = asRecord(raw);
  return { subsystem: str(r.subsystem, 'system'), message: str(r.message, 'A subsystem reported a problem.') };
}

export function normalizeFileReceived(raw: unknown): FileReceivedEvent {
  const r = asRecord(raw);
  const names = strList(r.names);
  return { count: num(r.count, names.length), names, tempDir: str(r.tempDir) };
}

export function normalizeLogAnalysis(raw: unknown): LogAnalysis {
  const r = asRecord(raw);
  const events = Array.isArray(r.events)
    ? r.events.map((item) => {
        const e = asRecord(item);
        return { level: str(e.level), pattern: str(e.pattern), count: num(e.count), sample: str(e.sample) };
      })
    : [];
  return { events, totalErrors: num(r.totalErrors), windowLines: num(r.windowLines), analyzedAt: str(r.analyzedAt) };
}

export function normalizeNumberMap(raw: unknown): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [key, value] of Object.entries(asRecord(raw))) {
    if (typeof value === 'number' && Number.isFinite(value)) out[key] = value;
  }
  return out;
}

const EDGE_SIDES: readonly EdgeSide[] = ['left', 'right', 'top', 'bottom'];

export function normalizeEdgeSide(raw: unknown): EdgeSide {
  return EDGE_SIDES.includes(raw as EdgeSide) ? (raw as EdgeSide) : 'right';
}

export function normalizeHotkey(raw: unknown): ExitHotkey {
  const r = asRecord(raw);
  return { modifiers: num(r.modifiers), vkCode: num(r.vkCode) };
}

export function normalizeMonitors(raw: unknown): MonitorInfo[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((item) => {
    const r = asRecord(item);
    return {
      id: str(r.id),
      name: str(r.name),
      x: num(r.x),
      y: num(r.y),
      width: num(r.width),
      height: num(r.height),
      isPrimary: bool(r.isPrimary),
    };
  });
}

export function normalizeTriggerZone(raw: unknown): TriggerZone | null {
  const r = asRecord(raw);
  const monitorID = str(r.monitorID);
  if (!monitorID) return null;
  const startPct = num(r.startPct, 0);
  const endPct = num(r.endPct, 1);
  return {
    monitorID,
    side: normalizeEdgeSide(r.side),
    startPct,
    // The backend treats endPct <= 0 as unset.
    endPct: endPct > 0 ? endPct : 1,
  };
}

export function normalizeReturnAnchor(raw: unknown): ReturnAnchor | null {
  const r = asRecord(raw);
  const monitorID = str(r.monitorID);
  if (!monitorID) return null;
  return { monitorID, xPct: num(r.xPct, 0.5), yPct: num(r.yPct, 0.5) };
}

export function normalizeAudioDevices(raw: unknown): AudioDevice[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((item) => {
    const r = asRecord(item);
    return { id: str(r.id), name: str(r.name), flow: str(r.flow) };
  });
}

function oneOf<T extends string>(allowed: readonly T[], fallback: T) {
  return (raw: unknown): T => (allowed.includes(raw as T) ? (raw as T) : fallback);
}

export const normalizeAudioMode = oneOf<AudioMode>(['off', 'remote', 'local'], 'off');
export const normalizeAudioTiming = oneOf<AudioTiming>(['always', 'switched'], 'always');
export const normalizeAudioTransport = oneOf<AudioTransport>(['auto', 'pcm', 'opus'], 'auto');
export const normalizeAudioProfile = oneOf<AudioProfile>(['low-latency', 'balanced', 'music'], 'balanced');
export const normalizeMicMode = oneOf<MicMode>(['off', 'send', 'receive'], 'off');

