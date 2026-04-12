import { emptyHealth, emptySession, emptyTailscale } from './constants';
import type { AudioDevice, DeviceInfo, HealthStatus, LastPeerInfo, Peer, Session, TailscaleStatus } from './types';

export interface AppApi {
  AddPeer(address: string): Promise<void>;
  Connect(address: string): Promise<void>;
  Disconnect(): Promise<void>;
  GetAudioDevices(): Promise<AudioDevice[]>;
  GetAudioMode(): Promise<string>;
  GetAudioProfile(): Promise<string>;
  GetAudioTiming(): Promise<string>;
  GetAudioTransport(): Promise<string>;
  GetAutoReconnect(): Promise<boolean>;
  GetAutostart(): Promise<boolean>;
  GetCaptureDeviceID(): Promise<string>;
  GetDevice(): Promise<DeviceInfo>;
  GetEdgeSide(): Promise<string>;
  GetHealthStatus(): Promise<HealthStatus>;
  GetLastPeer(): Promise<Record<string, string> | null>;
  GetMicDeviceID(): Promise<string>;
  GetMicMode(): Promise<string>;
  GetMicPlaybackDeviceID(): Promise<string>;
  GetMuteSource(): Promise<boolean>;
  GetPeers(): Promise<Peer[]>;
  GetPlaybackDeviceID(): Promise<string>;
  GetRecentLogs(): Promise<string[]>;
  GetSensitivity(): Promise<number>;
  GetSession(): Promise<Session>;
  GetStartMinimized(): Promise<boolean>;
  GetTailscaleStatus(): Promise<TailscaleStatus>;
  Reconnect(): Promise<void>;
  RemovePeer(address: string): Promise<void>;
  SendFiles(paths: string[]): Promise<void>;
  PickAndSendFiles(): Promise<void>;
  SetAudioMode(mode: string): Promise<void>;
  SetAudioProfile(profile: string): Promise<void>;
  SetAudioTiming(timing: string): Promise<void>;
  SetAudioTransport(mode: string): Promise<void>;
  SetAutoReconnect(enabled: boolean): Promise<void>;
  SetAutostart(enabled: boolean): Promise<void>;
  SetCaptureDeviceID(id: string): Promise<void>;
  SetEdgeSide(side: string): Promise<void>;
  SetMicDeviceID(id: string): Promise<void>;
  SetMicMode(mode: string): Promise<void>;
  SetMicPlaybackDeviceID(id: string): Promise<void>;
  SetMuteSource(enabled: boolean): Promise<void>;
  SetPlaybackDeviceID(id: string): Promise<void>;
  SetSensitivity(value: number): Promise<void>;
  SetStartMinimized(enabled: boolean): Promise<void>;
  TrustPeer(address: string, pairingCode: string): Promise<void>;
  UntrustPeer(peerID: string): Promise<void>;
}

export interface PreviewState {
  device: DeviceInfo;
  peers: Peer[];
  session: Session;
  tailscale: TailscaleStatus;
  edgeSide: string;
  sensitivity: number;
  audioMode: string;
  audioProfile: string;
  audioTransportMode: string;
  audioTiming: string;
  muteSource: boolean;
  micMode: string;
  audioDevices: AudioDevice[];
  captureDeviceID: string;
  playbackDeviceID: string;
  micDeviceID: string;
  micPlaybackDeviceID: string;
  autostart: boolean;
  startMinimized: boolean;
  lastPeer: LastPeerInfo | null;
  reconnecting: boolean;
  autoReconnect: boolean;
  health: HealthStatus;
  previewMode: boolean;
}

const previewTimestamp = Math.floor(Date.now() / 1000);
const previewDevice: DeviceInfo = {
  id: 'desk-main-84f3a192',
  name: 'Desk Main',
  fingerprint: '8A3CCF8D9A10F442B18E6C21F0C4AB8D',
  pairingCode: '482911',
  port: 24831,
};

const previewPeerSeed: Peer[] = [
  {
    id: 'studio-pc',
    name: 'Studio PC',
    address: '192.168.0.42:24831',
    addresses: ['192.168.0.42:24831', '100.92.10.17:24831'],
    fingerprint: 'A7F93C19B8DD4A89A0FE31C19D68D2F1',
    port: 24831,
    source: 'hybrid',
    status: 'online',
    trusted: true,
    routes: ['lan', 'tailscale'],
    preferredRoute: 'lan',
    lastSeen: previewTimestamp - 8,
  },
  {
    id: 'travel-laptop',
    name: 'Travel Laptop',
    address: '100.88.3.14:24831',
    addresses: ['100.88.3.14:24831'],
    fingerprint: 'F8D127A45B7E9A44C4D88D1A0FA45E20',
    port: 24831,
    source: 'tailscale',
    status: 'online',
    trusted: true,
    routes: ['tailscale'],
    preferredRoute: 'tailscale',
    lastSeen: previewTimestamp - 22,
  },
  {
    id: 'gaming-rig',
    name: 'Gaming Rig',
    address: '192.168.0.81:24831',
    addresses: ['192.168.0.81:24831'],
    fingerprint: 'D019EE1A42A11B4AC8931A0C22E41F9A',
    port: 24831,
    source: 'manual',
    status: 'offline',
    trusted: false,
    routes: ['manual'],
    preferredRoute: 'manual',
    lastSeen: previewTimestamp - 3600,
  },
];

export const previewLogs = [
  '2026-03-30T09:12:14Z INFO discovery: 2 peers reachable on LAN',
  '2026-03-30T09:12:18Z INFO tailscale: backend running, 7 nodes visible',
  '2026-03-30T09:12:24Z INFO session: ready for handoff on right edge',
];

const previewAudioDevices: AudioDevice[] = [
  { id: 'default-speakers', name: 'Studio Display Speakers', flow: 'render' },
  { id: 'desk-headset', name: 'Desk Headset', flow: 'render' },
  { id: 'usb-mic', name: 'USB Microphone', flow: 'capture' },
];

function clonePreviewPeers() {
  return previewPeerSeed.map((peer) => ({
    ...peer,
    addresses: [...(peer.addresses || [])],
    routes: [...(peer.routes || [])],
  }));
}

export function createPreviewState(): PreviewState {
  return {
    previewMode: true,
    device: { ...previewDevice },
    peers: clonePreviewPeers(),
    session: { ...emptySession },
    tailscale: {
      ...emptyTailscale,
      available: true,
      connected: true,
      backendState: 'Running',
      selfName: 'desk-main',
      tailnet: 'multisnek.ts.net',
      selfIPs: ['100.92.10.12'],
      peerCount: 7,
      targetCount: 2,
      lastSync: previewTimestamp - 25,
    },
    edgeSide: 'right',
    sensitivity: 1.2,
    audioMode: 'remote',
    audioProfile: 'music',
    audioTransportMode: 'opus',
    audioTiming: 'switched',
    muteSource: true,
    micMode: 'off',
    audioDevices: previewAudioDevices.map((item) => ({ ...item })),
    captureDeviceID: '',
    playbackDeviceID: 'default-speakers',
    micDeviceID: '',
    micPlaybackDeviceID: '',
    autostart: false,
    startMinimized: false,
    lastPeer: { id: 'travel-laptop', name: 'Travel Laptop', tailscale: '100.88.3.14:24831' },
    reconnecting: false,
    autoReconnect: true,
    health: { ...emptyHealth },
  };
}

export function getAppApi(): AppApi | null {
  if (typeof window === 'undefined') return null;

  const appApi = window.go?.app?.App as AppApi | undefined;
  if (appApi) return appApi;

  return (window.go?.main?.App as AppApi | undefined) ?? null;
}

export function getLastPeerAddress(peerInfo: LastPeerInfo | null) {
  if (!peerInfo) return '';
  return peerInfo.lan || peerInfo.tailscale || peerInfo.manual || peerInfo.address || '';
}

export function errorMessage(error: unknown, fallback: string) {
  if (typeof error === 'string' && error.trim()) return error;
  if (error instanceof Error && error.message.trim()) return error.message;
  return fallback;
}

export function sanitizePairingCode(value: string) {
  return (value || '').replace(/\D/g, '').slice(0, 6);
}
