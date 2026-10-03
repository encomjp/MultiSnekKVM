// In-memory Api used when the Wails backend is absent: `vite dev` in a
// browser, and the test suite. It behaves like a small fake backend and
// emits the same events the Go side does, so the UI has one code path.

import { emptySession, DEFAULT_PORT } from './normalize';
import { lastPeerAddress } from '../utils';
import type {
  Api,
  AudioDevice,
  AudioMode,
  AudioProfile,
  AudioTiming,
  AudioTransport,
  ConnectionInterface,
  DeviceInfo,
  EdgeSide,
  EventMap,
  EventName,
  ExitHotkey,
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

export interface PreviewApi extends Api {
  readonly kind: 'preview';
  /** Emit a backend event, as the Go side would. */
  emit<E extends EventName>(event: E, payload: EventMap[E]): void;
}

export interface PreviewState {
  device: DeviceInfo;
  peers: Peer[];
  session: SessionStatus;
  lastPeer: LastPeer | null;
  tailscale: TailscaleStatus;
  interfaces: ConnectionInterface[];
  health: HealthStatus;
  logs: string[];
  autoReconnect: boolean;
  edgeSide: EdgeSide;
  sensitivity: number;
  exitHotkey: ExitHotkey;
  monitors: MonitorInfo[];
  triggerZone: TriggerZone | null;
  returnAnchor: ReturnAnchor | null;
  autostart: boolean;
  startMinimized: boolean;
  audioMode: AudioMode;
  audioTiming: AudioTiming;
  audioTransport: AudioTransport;
  audioProfile: AudioProfile;
  micMode: MicMode;
  muteSource: boolean;
  audioDevices: AudioDevice[];
  captureDeviceID: string;
  playbackDeviceID: string;
  micDeviceID: string;
  micPlaybackDeviceID: string;
}

const MINUTE = 60;
const DAY = 86400;

export function createPreviewState(now = Math.floor(Date.now() / 1000)): PreviewState {
  return {
    device: {
      id: 'desk-main-84f3a192',
      name: 'Desk Main',
      fingerprint: '8A3CCF8D9A10F442B18E6C21F0C4AB8D61E0E9C2',
      pairingCode: '482911',
      port: DEFAULT_PORT,
    },
    peers: [
      {
        id: 'studio-pc',
        name: 'Studio PC',
        address: '169.254.22.15:24831',
        addresses: ['169.254.22.15:24831', '192.168.0.42:24831', '100.92.10.17:24831'],
        addressKinds: {
          '169.254.22.15:24831': 'usb4',
          '192.168.0.42:24831': 'ethernet',
          '100.92.10.17:24831': 'tailscale',
        },
        fingerprint: '4FA2C19B8DD44A89A0FE31C19D68D2F17E05B69C',
        source: 'hybrid',
        routes: ['usb4', 'ethernet', 'tailscale'],
        preferredRoute: 'usb4',
        trusted: true,
        status: 'online',
        lastSeen: now - 8,
      },
      {
        id: 'travel-laptop',
        name: 'Travel Laptop',
        address: '192.168.0.57:24831',
        addresses: ['192.168.0.57:24831', '100.88.3.14:24831'],
        addressKinds: { '192.168.0.57:24831': 'wifi', '100.88.3.14:24831': 'tailscale' },
        fingerprint: 'F8D127A45B7E9A44C4D88D1A0FA45E2031B7C0DE',
        source: 'hybrid',
        routes: ['wifi', 'tailscale'],
        preferredRoute: 'wifi',
        trusted: true,
        status: 'online',
        lastSeen: now - 22,
      },
      {
        id: 'gaming-rig',
        name: 'Gaming Rig',
        address: '192.168.0.88:24831',
        addresses: ['192.168.0.88:24831'],
        addressKinds: {},
        fingerprint: '',
        source: 'manual',
        routes: ['manual'],
        preferredRoute: 'manual',
        trusted: false,
        status: 'added',
        lastSeen: now - 2 * DAY,
      },
    ],
    session: { ...emptySession },
    lastPeer: {
      id: 'travel-laptop',
      name: 'Travel Laptop',
      wifi: '192.168.0.57:24831',
      tailscale: '100.88.3.14:24831',
    },
    tailscale: {
      available: true,
      connected: true,
      backendState: 'Running',
      selfName: 'desk-main',
      tailnet: 'multisnek.ts.net',
      selfIPs: ['100.64.3.2'],
      peerCount: 7,
      targetCount: 2,
      lastSync: now - 25,
      lastError: '',
    },
    interfaces: [
      { name: 'Ethernet 3', description: 'USB4 P2P Network Adapter', kind: 'usb4', addresses: ['169.254.22.16'] },
      { name: 'Ethernet', description: 'Intel(R) Ethernet Controller I226-V', kind: 'ethernet', addresses: ['192.168.0.10'] },
      { name: 'Wi-Fi', description: 'Intel(R) Wi-Fi 7 BE200 320MHz', kind: 'wifi', addresses: ['192.168.0.11'] },
      { name: 'Tailscale', description: 'Tailscale Tunnel', kind: 'network', addresses: ['100.64.3.2'] },
    ],
    health: {
      healthy: true,
      reconnecting: false,
      subsystems: [
        { name: 'Transport', healthy: true, detail: 'TLS 1.3' },
        { name: 'Input hook', healthy: true, detail: 'active' },
        { name: 'Audio', healthy: true, detail: 'idle' },
        { name: 'Clipboard', healthy: true, detail: 'synced' },
        { name: 'Discovery', healthy: true, detail: 'LAN + tailnet' },
      ],
      uptime: 3 * 3600 + 14 * MINUTE,
      goroutines: 31,
      goroutineDelta: 0,
    },
    logs: [
      '2026-10-03T09:12:14Z INFO discovery: 2 peers reachable on LAN',
      '2026-10-03T09:12:18Z INFO tailscale: backend running, 7 nodes visible',
      '2026-10-03T09:12:20Z INFO link: USB4 adapter "Ethernet 3" up at 169.254.22.16',
      '2026-10-03T09:12:24Z INFO session: ready for hand-off on right edge',
      '2026-10-03T09:13:02Z WARN audio: capture device changed, restarting loopback',
    ],
    autoReconnect: true,
    edgeSide: 'right',
    sensitivity: 1,
    exitHotkey: { modifiers: 1 | 4, vkCode: 0x24 },
    monitors: [
      { id: 'display1', name: '\\\\.\\DISPLAY1', x: 0, y: 0, width: 3440, height: 1440, isPrimary: true },
      { id: 'display2', name: '\\\\.\\DISPLAY2', x: -2560, y: 0, width: 2560, height: 1440, isPrimary: false },
    ],
    triggerZone: { monitorID: 'display1', side: 'right', startPct: 0.2, endPct: 0.7 },
    returnAnchor: { monitorID: 'display1', xPct: 0.88, yPct: 0.5 },
    autostart: false,
    startMinimized: false,
    audioMode: 'remote',
    audioTiming: 'switched',
    audioTransport: 'auto',
    audioProfile: 'balanced',
    micMode: 'off',
    muteSource: true,
    audioDevices: [
      { id: 'speakers', name: 'Studio Display Speakers', flow: 'render' },
      { id: 'headset', name: 'Desk Headset', flow: 'render' },
      { id: 'usb-mic', name: 'USB Microphone', flow: 'capture' },
      { id: 'headset-mic', name: 'Desk Headset Microphone', flow: 'capture' },
    ],
    captureDeviceID: '',
    playbackDeviceID: 'speakers',
    micDeviceID: '',
    micPlaybackDeviceID: '',
  };
}

const LATENCY_BY_ROUTE: Record<string, number> = {
  usb4: 2,
  'usb-bridge': 3,
  ethernet: 3,
  lan: 4,
  wifi: 9,
  network: 6,
  tailscale: 19,
  bluetooth: 28,
  manual: 5,
};

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function withPort(address: string): string {
  if (address.startsWith('[')) return address.includes(']:') ? address : `${address}:${DEFAULT_PORT}`;
  return /:\d+$/.test(address) ? address : `${address}:${DEFAULT_PORT}`;
}

export function createPreviewApi(initial: Partial<PreviewState> = {}): PreviewApi {
  const state: PreviewState = { ...createPreviewState(), ...clone(initial) };
  const listeners = new Map<EventName, Set<(payload: never) => void>>();
  const receivedDirs = new Map<string, string[]>();

  function emit<E extends EventName>(event: E, payload: EventMap[E]) {
    for (const listener of listeners.get(event) ?? []) {
      (listener as (value: EventMap[E]) => void)(clone(payload));
    }
  }

  const findPeer = (address: string) =>
    state.peers.find((peer) => peer.address === address || peer.addresses.includes(address));

  const publishPeers = () => emit('peers-updated', state.peers);
  const publishSession = () => emit('session-updated', state.session);

  function startSession(peer: Peer, address: string) {
    const route = peer.addressKinds[address] || peer.preferredRoute || 'manual';
    const latency = LATENCY_BY_ROUTE[route] ?? 5;
    peer.status = 'online';
    peer.lastSeen = Math.floor(Date.now() / 1000);
    state.session = {
      route,
      remoteAddress: address,
      connected: true,
      controlling: true,
      peerName: peer.name || address,
      peerID: peer.id,
      role: 'controller',
      latencyMs: latency,
      audioLatencyMs: latency + 40,
      jitterMs: Math.max(1, Math.round(latency / 4)),
    };
    state.lastPeer = { id: peer.id, name: peer.name, [route]: address };
    state.health = {
      ...state.health,
      subsystems: state.health.subsystems.map((s) =>
        s.name === 'Audio' ? { ...s, detail: state.audioMode === 'off' ? 'off' : 'streaming' } : s,
      ),
    };
    publishPeers();
    publishSession();
    emit('health-updated', state.health);
  }

  const resolved = <T>(value: T) => Promise.resolve(clone(value));

  const api: PreviewApi = {
    kind: 'preview',
    emit,

    on(event, callback) {
      let set = listeners.get(event);
      if (!set) {
        set = new Set();
        listeners.set(event, set);
      }
      const listener = callback as (payload: never) => void;
      set.add(listener);
      return () => set.delete(listener);
    },

    GetDevice: () => resolved(state.device),
    GetPeers: () => resolved(state.peers),
    GetSession: () => resolved(state.session),
    GetTailscaleStatus: () => resolved(state.tailscale),
    GetConnectionInterfaces: () => resolved(state.interfaces),
    GetLastPeer: () => resolved(state.lastPeer),

    async AddPeer(raw) {
      const trimmed = raw.trim();
      if (!trimmed) throw new Error('Enter an IP address or hostname.');
      const address = withPort(trimmed);
      if (findPeer(address)) throw new Error(`${trimmed} is already in your device list.`);
      state.peers = [
        ...state.peers,
        {
          id: `manual-${address.replace(/[^a-z0-9]+/gi, '-').toLowerCase()}`,
          name: trimmed.replace(/:\d+$/, ''),
          address,
          addresses: [address],
          addressKinds: {},
          fingerprint: '',
          source: 'manual',
          routes: ['manual'],
          preferredRoute: 'manual',
          trusted: false,
          status: 'added',
          lastSeen: 0,
        },
      ];
      publishPeers();
    },

    async RemovePeer(address) {
      const peer = findPeer(address);
      state.peers = state.peers.filter((item) => item !== peer);
      if (peer && state.lastPeer?.id === peer.id) state.lastPeer = null;
      publishPeers();
    },

    async Connect(address) {
      const peer = findPeer(address);
      if (!peer) throw new Error(`No known device at ${address}.`);
      if (!peer.trusted) throw new Error('peer is not trusted; pair with its PIN first');
      if (state.session.connected) throw new Error(`Already connected to ${state.session.peerName}.`);
      startSession(peer, address);
    },

    async Disconnect() {
      state.session = { ...emptySession };
      publishSession();
    },

    async Reconnect() {
      const address = lastPeerAddress(state.lastPeer);
      if (!address) throw new Error('No previous device to reconnect to.');
      await api.Connect(address);
    },

    async TrustPeer(address, pin) {
      if (!/^\d{6}$/.test(pin)) throw new Error('The PIN must be 6 digits.');
      const peer = findPeer(address);
      if (!peer) throw new Error(`No known device at ${address}.`);
      if (state.session.connected) throw new Error(`Disconnect from ${state.session.peerName} first.`);
      peer.trusted = true;
      peer.fingerprint ||= 'D019EE1A42A11B4AC8931A0C22E41F9A5B60C7E1';
      startSession(peer, address);
    },

    async UntrustPeer(peerID) {
      const peer = state.peers.find((item) => item.id === peerID);
      if (!peer) throw new Error('Unknown device.');
      peer.trusted = false;
      if (peer.status === 'trusted') peer.status = 'added';
      if (state.lastPeer?.id === peerID) state.lastPeer = null;
      publishPeers();
    },

    GetAutoReconnect: () => resolved(state.autoReconnect),
    SetAutoReconnect: async (enabled) => void (state.autoReconnect = enabled),

    GetHealthStatus: () => resolved(state.health),
    GetRecentLogs: () => resolved(state.logs),
    GetLogAnalysis: () =>
      resolved<LogAnalysis>({
        events: [{ level: 'warn', pattern: 'audio: capture device changed', count: 1, sample: state.logs[4] ?? '' }],
        totalErrors: 0,
        windowLines: state.logs.length,
        analyzedAt: new Date().toISOString(),
      }),
    GetLoadMetrics: () =>
      resolved<Record<string, number>>({ framesIn: 18240, framesOut: 17652, droppedFrames: 0, audioUnderruns: 1 }),

    GetEdgeSide: () => resolved(state.edgeSide),
    SetEdgeSide: async (side) => void (state.edgeSide = side),
    GetSensitivity: () => resolved(state.sensitivity),
    SetSensitivity: async (value) => void (state.sensitivity = value),
    GetExitHotkey: () => resolved(state.exitHotkey),
    SetExitHotkey: async (modifiers, vkCode) => void (state.exitHotkey = { modifiers, vkCode }),
    GetLocalMonitors: () => resolved(state.monitors),
    GetTriggerZone: () => resolved(state.triggerZone),
    SetTriggerZone: async (monitorID, side, startPct, endPct) =>
      void (state.triggerZone = { monitorID, side, startPct, endPct }),
    ClearTriggerZone: async () => void (state.triggerZone = null),
    GetReturnAnchor: () => resolved(state.returnAnchor),
    SetReturnAnchor: async (monitorID, xPct, yPct) => void (state.returnAnchor = { monitorID, xPct, yPct }),
    ClearReturnAnchor: async () => void (state.returnAnchor = null),

    GetAutostart: () => resolved(state.autostart),
    SetAutostart: async (enabled) => void (state.autostart = enabled),
    GetStartMinimized: () => resolved(state.startMinimized),
    SetStartMinimized: async (enabled) => void (state.startMinimized = enabled),

    GetAudioMode: () => resolved(state.audioMode),
    SetAudioMode: async (mode) => void (state.audioMode = mode),
    GetAudioTiming: () => resolved(state.audioTiming),
    SetAudioTiming: async (timing) => void (state.audioTiming = timing),
    GetAudioTransport: () => resolved(state.audioTransport),
    SetAudioTransport: async (transport) => void (state.audioTransport = transport),
    GetAudioProfile: () => resolved(state.audioProfile),
    SetAudioProfile: async (profile) => void (state.audioProfile = profile),
    GetMicMode: () => resolved(state.micMode),
    SetMicMode: async (mode) => void (state.micMode = mode),
    GetMuteSource: () => resolved(state.muteSource),
    SetMuteSource: async (enabled) => void (state.muteSource = enabled),
    GetAudioDevices: () => resolved(state.audioDevices),
    GetCaptureDeviceID: () => resolved(state.captureDeviceID),
    SetCaptureDeviceID: async (id) => void (state.captureDeviceID = id),
    GetPlaybackDeviceID: () => resolved(state.playbackDeviceID),
    SetPlaybackDeviceID: async (id) => void (state.playbackDeviceID = id),
    GetMicDeviceID: () => resolved(state.micDeviceID),
    SetMicDeviceID: async (id) => void (state.micDeviceID = id),
    GetMicPlaybackDeviceID: () => resolved(state.micPlaybackDeviceID),
    SetMicPlaybackDeviceID: async (id) => void (state.micPlaybackDeviceID = id),

    async PickAndSendFiles() {
      if (!state.session.connected) throw new Error('not connected');
    },
    async SendFiles() {
      if (!state.session.connected) throw new Error('not connected');
    },
    async SaveReceivedFiles(tempDir) {
      const names = receivedDirs.get(tempDir) ?? [];
      receivedDirs.delete(tempDir);
      return { dest: 'C:\\Users\\you\\Downloads', saved: names };
    },
    DiscardReceivedFiles: async (tempDir) => void receivedDirs.delete(tempDir),
  };

  // Remember names so SaveReceivedFiles can report them.
  api.on('file-received', (event) => receivedDirs.set(event.tempDir, event.names));
  return api;
}
