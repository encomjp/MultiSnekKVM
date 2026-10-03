export const emptySession = { connected: false, controlling: false, peerName: '', peerID: '', role: '', latencyMs: -1, audioLatencyMs: -1, jitterMs: -1 };

export const emptyHealth = { healthy: true, reconnecting: false, subsystems: [], uptime: 0 };

export const emptyTailscale = {
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

export const routeRank = { usb4: 0, 'usb-bridge': 1, ethernet: 2, lan: 3, wifi: 4, network: 5, tailscale: 6, bluetooth: 7, manual: 8 };
