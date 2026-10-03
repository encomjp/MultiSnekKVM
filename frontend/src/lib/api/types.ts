// Domain types shared by the API layer, stores and components. They mirror
// the JSON the Go App binding returns (see wailsjs/go/models.ts) but are
// plain interfaces with nullable / omitempty fields normalized.

export type RouteKind =
  | 'usb4'
  | 'usb-bridge'
  | 'ethernet'
  | 'lan'
  | 'wifi'
  | 'network'
  | 'tailscale'
  | 'bluetooth'
  | 'manual';

export type InterfaceKind = 'usb4' | 'usb-bridge' | 'ethernet' | 'wifi' | 'bluetooth' | 'network';

export type EdgeSide = 'left' | 'right' | 'top' | 'bottom';
export type AudioMode = 'off' | 'remote' | 'local';
export type AudioTiming = 'always' | 'switched';
export type AudioTransport = 'auto' | 'pcm' | 'opus';
export type AudioProfile = 'low-latency' | 'balanced' | 'music';
export type MicMode = 'off' | 'send' | 'receive';
export type PeerStatus = 'online' | 'added' | 'trusted';

export interface DeviceInfo {
  id: string;
  name: string;
  fingerprint: string;
  pairingCode?: string;
  port: number;
}

export interface Peer {
  id: string;
  name: string;
  address: string;
  addresses: string[];
  addressKinds: Record<string, string>;
  fingerprint: string;
  source: string;
  routes: string[];
  preferredRoute: string;
  trusted: boolean;
  status: PeerStatus | string;
  lastSeen: number;
}

export interface SessionStatus {
  route?: string;
  remoteAddress?: string;
  connected: boolean;
  controlling: boolean;
  peerName: string;
  peerID: string;
  role: 'controller' | 'controlled' | string;
  /** Milliseconds; -1 means unknown. */
  latencyMs: number;
  audioLatencyMs: number;
  jitterMs: number;
}

export interface TailscaleStatus {
  available: boolean;
  connected: boolean;
  backendState: string;
  selfName: string;
  tailnet: string;
  selfIPs: string[];
  peerCount: number;
  targetCount: number;
  lastSync: number;
  lastError: string;
}

export interface BluetoothDevice {
  /** "bt://AA:BB:CC:DD:EE:FF" */
  address: string;
  deviceId: string;
  name: string;
}

export interface BluetoothStatus {
  available: boolean;
  enabled: boolean;
  listening: boolean;
  scanning: boolean;
  error?: string;
  /** Paired PCs found running MultiSnek. */
  devices: BluetoothDevice[];
  /** Unix seconds; 0 = never. */
  lastScan: number;
}

export interface ConnectionInterface {
  name: string;
  description?: string;
  kind: InterfaceKind | string;
  addresses: string[];
}

/** {id, name, <routeKind>: address} as returned by GetLastPeer. */
export type LastPeer = Record<string, string>;

export interface HealthSubsystem {
  name: string;
  healthy: boolean;
  detail: string;
}

export interface HealthStatus {
  healthy: boolean;
  reconnecting: boolean;
  subsystems: HealthSubsystem[];
  uptime: number;
  goroutines: number;
  goroutineDelta: number;
}

export interface HealthAlert {
  subsystem: string;
  message: string;
}

export interface LogEvent {
  level: string;
  pattern: string;
  count: number;
  sample: string;
}

export interface LogAnalysis {
  events: LogEvent[];
  totalErrors: number;
  windowLines: number;
  analyzedAt: string;
}

export interface ExitHotkey {
  /** Bit mask: 1 = Ctrl, 2 = Alt, 4 = Shift, 8 = Win. */
  modifiers: number;
  /** Windows virtual-key code; 0 means the default (Esc). */
  vkCode: number;
}

export interface MonitorInfo {
  id: string;
  name: string;
  x: number;
  y: number;
  width: number;
  height: number;
  isPrimary: boolean;
}

export interface TriggerZone {
  monitorID: string;
  side: EdgeSide;
  startPct: number;
  endPct: number;
}

export interface ReturnAnchor {
  monitorID: string;
  xPct: number;
  yPct: number;
}

export interface AudioDevice {
  id: string;
  name: string;
  flow: 'render' | 'capture' | string;
}

export interface SaveReceivedFilesResult {
  dest: string;
  saved: string[];
}

export interface FileReceivedEvent {
  count: number;
  names: string[];
  tempDir: string;
}

export interface EventMap {
  'device-updated': DeviceInfo;
  'peers-updated': Peer[];
  'session-updated': SessionStatus;
  'tailscale-updated': TailscaleStatus;
  'bluetooth-updated': BluetoothStatus;
  'health-updated': HealthStatus;
  'health-alert': HealthAlert;
  'file-received': FileReceivedEvent;
}

export type EventName = keyof EventMap;
export type Unsubscribe = () => void;

/**
 * Everything the UI needs from the backend. The Wails implementation wraps
 * the generated bindings; the preview implementation keeps state in memory
 * for browser development and tests.
 */
export interface Api {
  readonly kind: 'wails' | 'preview';
  on<E extends EventName>(event: E, callback: (payload: EventMap[E]) => void): Unsubscribe;

  // Device, peers, session
  GetDevice(): Promise<DeviceInfo>;
  GetPeers(): Promise<Peer[]>;
  GetSession(): Promise<SessionStatus>;
  GetTailscaleStatus(): Promise<TailscaleStatus>;
  GetConnectionInterfaces(): Promise<ConnectionInterface[]>;
  GetLastPeer(): Promise<LastPeer | null>;

  // Bluetooth (direct link between paired PCs)
  GetBluetoothStatus(): Promise<BluetoothStatus>;
  SetBluetoothEnabled(enabled: boolean): Promise<void>;
  /** Triggers an immediate scan; progress arrives via `bluetooth-updated`. */
  RefreshBluetooth(): Promise<void>;

  // Connection & trust
  AddPeer(address: string): Promise<void>;
  RemovePeer(address: string): Promise<void>;
  Connect(address: string): Promise<void>;
  Disconnect(): Promise<void>;
  Reconnect(): Promise<void>;
  TrustPeer(address: string, pin: string): Promise<void>;
  UntrustPeer(peerID: string): Promise<void>;
  GetAutoReconnect(): Promise<boolean>;
  SetAutoReconnect(enabled: boolean): Promise<void>;

  // Diagnostics
  GetHealthStatus(): Promise<HealthStatus>;
  GetRecentLogs(): Promise<string[]>;
  GetLogAnalysis(): Promise<LogAnalysis>;
  GetLoadMetrics(): Promise<Record<string, number>>;

  // Input
  GetEdgeSide(): Promise<EdgeSide>;
  SetEdgeSide(side: EdgeSide): Promise<void>;
  GetSensitivity(): Promise<number>;
  SetSensitivity(value: number): Promise<void>;
  GetExitHotkey(): Promise<ExitHotkey>;
  SetExitHotkey(modifiers: number, vkCode: number): Promise<void>;
  GetLocalMonitors(): Promise<MonitorInfo[]>;
  GetTriggerZone(): Promise<TriggerZone | null>;
  SetTriggerZone(monitorID: string, side: EdgeSide, startPct: number, endPct: number): Promise<void>;
  ClearTriggerZone(): Promise<void>;
  GetReturnAnchor(): Promise<ReturnAnchor | null>;
  SetReturnAnchor(monitorID: string, xPct: number, yPct: number): Promise<void>;
  ClearReturnAnchor(): Promise<void>;

  // System
  GetAutostart(): Promise<boolean>;
  SetAutostart(enabled: boolean): Promise<void>;
  GetStartMinimized(): Promise<boolean>;
  SetStartMinimized(enabled: boolean): Promise<void>;

  // Audio
  GetAudioMode(): Promise<AudioMode>;
  SetAudioMode(mode: AudioMode): Promise<void>;
  GetAudioTiming(): Promise<AudioTiming>;
  SetAudioTiming(timing: AudioTiming): Promise<void>;
  GetAudioTransport(): Promise<AudioTransport>;
  SetAudioTransport(transport: AudioTransport): Promise<void>;
  GetAudioProfile(): Promise<AudioProfile>;
  SetAudioProfile(profile: AudioProfile): Promise<void>;
  GetMicMode(): Promise<MicMode>;
  SetMicMode(mode: MicMode): Promise<void>;
  GetMuteSource(): Promise<boolean>;
  SetMuteSource(enabled: boolean): Promise<void>;
  GetAudioDevices(): Promise<AudioDevice[]>;
  GetCaptureDeviceID(): Promise<string>;
  SetCaptureDeviceID(id: string): Promise<void>;
  GetPlaybackDeviceID(): Promise<string>;
  SetPlaybackDeviceID(id: string): Promise<void>;
  GetMicDeviceID(): Promise<string>;
  SetMicDeviceID(id: string): Promise<void>;
  GetMicPlaybackDeviceID(): Promise<string>;
  SetMicPlaybackDeviceID(id: string): Promise<void>;

  // Files
  PickAndSendFiles(): Promise<void>;
  SendFiles(paths: string[]): Promise<void>;
  SaveReceivedFiles(tempDir: string): Promise<SaveReceivedFilesResult>;
  DiscardReceivedFiles(tempDir: string): Promise<void>;
}
