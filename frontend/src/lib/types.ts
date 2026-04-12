export interface DeviceInfo {
  id: string;
  name: string;
  fingerprint: string;
  pairingCode?: string;
  port: number;
}

export interface Session {
  connected: boolean;
  controlling: boolean;
  peerName: string;
  peerID: string;
  role: string;
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

export interface Peer {
  id: string;
  name: string;
  address: string;
  addresses?: string[];
  fingerprint: string;
  port: number;
  source?: string;
  status?: string;
  trusted?: boolean;
  routes?: string[];
  preferredRoute?: string;
  lastSeen?: number;
}

export interface AudioDevice {
  id: string;
  name: string;
  flow: string;
}

export interface HealthSubsystem {
  name: string;
  healthy: boolean;
  detail: string;
}

export interface HealthStatus {
  healthy: boolean;
  reconnecting: boolean;
  subsystems: HealthSubsystem[];
  uptime?: number;
}

export interface LastPeerInfo {
  id?: string;
  name?: string;
  address?: string;
  lan?: string;
  tailscale?: string;
  manual?: string;
  [key: string]: string | undefined;
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
