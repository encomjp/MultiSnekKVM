import type { ConnectionInterface, HealthStatus, LastPeer, Peer } from './api/types';

/** Route preference used by the backend: lower is better. */
export const ROUTE_RANK: Readonly<Record<string, number>> = {
  usb4: 0,
  'usb-bridge': 1,
  ethernet: 2,
  lan: 3,
  wifi: 4,
  network: 5,
  tailscale: 6,
  bluetooth: 7,
  manual: 8,
};

/** Key order for picking an address out of GetLastPeer's map. */
export const LAST_PEER_KEYS = [
  'usb4',
  'usb-bridge',
  'ethernet',
  'lan',
  'wifi',
  'network',
  'tailscale',
  'bluetooth',
  'manual',
  'address',
] as const;

/** True for direct Bluetooth endpoints such as "bt://AA:BB:CC:DD:EE:FF" (no port). */
export function isBluetoothAddress(address: string | undefined | null): boolean {
  return !!address && /^bt:[/][/]/i.test(address);
}

/**
 * Label for a route kind. Pass the address when known: the "bluetooth" kind on
 * an IP address is a Bluetooth PAN (tethering that shows up as a network
 * adapter), while on a bt:// address it is the direct Bluetooth link.
 */
export function routeLabel(route: string | undefined | null, address?: string | null): string {
  switch (route) {
    case 'usb4':
      return 'USB4 / Thunderbolt';
    case 'usb-bridge':
      return 'USB network bridge';
    case 'ethernet':
      return 'Ethernet';
    case 'wifi':
      return 'Wi-Fi';
    case 'bluetooth':
      return address && !isBluetoothAddress(address) ? 'Bluetooth PAN' : 'Bluetooth';
    case 'network':
      return 'Network';
    case 'lan':
      return 'LAN';
    case 'tailscale':
      return 'Tailnet';
    case 'manual':
      return 'Manual';
    case undefined:
    case null:
    case '':
      return 'Auto';
    default:
      return route;
  }
}

export function interfaceLabel(kind: ConnectionInterface['kind']): string {
  if (kind === 'network') return 'Network adapter';
  // Adapters are always IP: Bluetooth here means PAN, not the direct link.
  if (kind === 'bluetooth') return 'Bluetooth PAN';
  return routeLabel(kind);
}

/** Sort route kinds best-first using the backend's preference order. */
export function orderedRoutes(routes: readonly string[]): string[] {
  return [...routes].sort((a, b) => {
    const diff = (ROUTE_RANK[a] ?? 9) - (ROUTE_RANK[b] ?? 9);
    return diff !== 0 ? diff : a.localeCompare(b);
  });
}

/** Strip the port (and IPv6 brackets) from host:port. bt:// addresses have no port and are returned as-is. */
export function hostOf(address: string): string {
  if (isBluetoothAddress(address)) return address;
  if (address.startsWith('[')) {
    const end = address.indexOf(']');
    return end > 0 ? address.slice(1, end) : address;
  }
  const colons = address.split(':').length - 1;
  return colons === 1 ? address.replace(/:\d+$/, '') : address;
}

function ipv4Parts(host: string): number[] | null {
  const parts = host.split('.');
  if (parts.length !== 4) return null;
  const nums = parts.map((part) => (/^\d{1,3}$/.test(part) ? Number(part) : NaN));
  return nums.every((n) => Number.isInteger(n) && n >= 0 && n <= 255) ? nums : null;
}

/**
 * Classify an endpoint by its IP when the backend did not tag it. Link-local
 * addresses usually mean a direct USB4 link and 100.64/10 is Tailscale's CGNAT
 * range; anything else is a generic network address. We deliberately do not
 * guess Wi-Fi vs. Ethernet vs. Bluetooth PAN from the IP.
 */
export function endpointLabel(address: string): string {
  if (isBluetoothAddress(address)) return 'Bluetooth';
  const host = hostOf(address);
  const parts = ipv4Parts(host);
  if (parts) {
    if (parts[0] === 169 && parts[1] === 254) return 'Direct link';
    if (parts[0] === 100 && parts[1] >= 64 && parts[1] <= 127) return 'Tailnet / CGNAT';
  }
  if (host.toLowerCase().startsWith('fe80:')) return 'Direct link';
  return 'Network';
}

/** Human label for one of a peer's addresses. */
export function addressLabel(peer: Pick<Peer, 'addressKinds'>, address: string): string {
  const kind = peer.addressKinds[address];
  return kind ? routeLabel(kind, address) : endpointLabel(address);
}

export interface RouteOption {
  address: string;
  kind: string;
  label: string;
  best: boolean;
}

/** A peer's addresses as selectable routes, recommended first. */
export function peerRoutes(peer: Peer): RouteOption[] {
  const addresses = peer.addresses.length ? peer.addresses : peer.address ? [peer.address] : [];
  return addresses
    .map((address) => ({
      address,
      kind: peer.addressKinds[address] || '',
      label: addressLabel(peer, address),
      best: address === peer.address,
    }))
    .sort((a, b) => {
      if (a.best !== b.best) return a.best ? -1 : 1;
      return (ROUTE_RANK[a.kind] ?? 9) - (ROUTE_RANK[b.kind] ?? 9);
    });
}

export function lastPeerAddress(lastPeer: LastPeer | null | undefined): string {
  if (!lastPeer) return '';
  for (const key of LAST_PEER_KEYS) {
    const value = lastPeer[key];
    if (value) return value;
  }
  return '';
}

/** The route kind the last-peer address came from, if known. */
export function lastPeerRoute(lastPeer: LastPeer | null | undefined): string {
  if (!lastPeer) return '';
  return LAST_PEER_KEYS.find((key) => key !== 'address' && !!lastPeer[key]) ?? '';
}

export function formatLatency(ms: number | null | undefined): string {
  if (ms == null || ms < 0) return 'Measuring…';
  if (ms === 0) return '<1 ms';
  return `${Number.isInteger(ms) ? ms : ms.toFixed(1)} ms`;
}

export type LatencyQuality = 'excellent' | 'good' | 'fair' | 'poor' | 'unknown';

export function latencyQuality(ms: number | null | undefined): LatencyQuality {
  if (ms == null || ms < 0) return 'unknown';
  if (ms <= 5) return 'excellent';
  if (ms <= 20) return 'good';
  if (ms <= 60) return 'fair';
  return 'poor';
}

const QUALITY_LABELS: Record<LatencyQuality, string> = {
  excellent: 'Excellent',
  good: 'Good',
  fair: 'Fair',
  poor: 'Poor',
  unknown: 'Measuring',
};

export function latencyQualityLabel(quality: LatencyQuality): string {
  return QUALITY_LABELS[quality];
}

export function jitterLabel(ms: number | null | undefined): string {
  if (ms == null || ms < 0) return 'Measuring';
  if (ms <= 2) return 'Stable';
  if (ms <= 10) return 'Some variation';
  return 'Unstable';
}

export function shortFingerprint(fingerprint: string | undefined | null): string {
  if (!fingerprint) return 'pending';
  const clean = fingerprint.replace(/[^0-9a-f]/gi, '').toUpperCase();
  if (clean.length <= 8) return clean.match(/.{1,2}/g)?.join(':') ?? clean;
  return `${clean.slice(0, 2)}:${clean.slice(2, 4)}:…:${clean.slice(-2)}`;
}

export function groupedFingerprint(fingerprint: string | undefined | null): string {
  if (!fingerprint) return '';
  const clean = fingerprint.replace(/[^0-9a-f]/gi, '').toUpperCase();
  return clean.match(/.{1,4}/g)?.join(' ') ?? clean;
}

export function formatPin(pin: string | undefined | null): string {
  const digits = (pin ?? '').replace(/\D/g, '');
  return digits.length === 6 ? `${digits.slice(0, 3)} ${digits.slice(3)}` : digits;
}

export function sanitizePin(value: string): string {
  return value.replace(/\D/g, '').slice(0, 6);
}

export function timeAgo(timestamp: number, now = Date.now() / 1000): string {
  if (!timestamp) return 'never';
  const elapsed = Math.max(0, Math.floor(now - timestamp));
  if (elapsed < 10) return 'just now';
  if (elapsed < 60) return `${elapsed} s ago`;
  if (elapsed < 3600) return `${Math.floor(elapsed / 60)} min ago`;
  if (elapsed < 86400) {
    const hours = Math.floor(elapsed / 3600);
    return `${hours} hour${hours === 1 ? '' : 's'} ago`;
  }
  const days = Math.floor(elapsed / 86400);
  return `${days} day${days === 1 ? '' : 's'} ago`;
}

export function formatDuration(seconds: number): string {
  if (!seconds || seconds < 0) return '0 s';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h} h ${m} min`;
  if (m > 0) return `${m} min`;
  return `${Math.floor(seconds)} s`;
}

export function formatClock(date: Date): string {
  return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
}

export function isPeerOnline(peer: Pick<Peer, 'status'>): boolean {
  return peer.status === 'online';
}

export function isManualPeer(peer: Pick<Peer, 'source' | 'routes'>): boolean {
  return peer.source === 'manual' || peer.routes.includes('manual');
}

export type HealthTone = 'ok' | 'warn' | 'bad';

export function healthSummary(health: HealthStatus): { label: string; tone: HealthTone } {
  if (health.reconnecting) return { label: 'Reconnecting…', tone: 'warn' };
  const unhealthy = health.subsystems.filter((s) => !s.healthy);
  if (unhealthy.length > 0) {
    return { label: `Degraded: ${unhealthy.map((s) => s.name).join(', ')}`, tone: 'bad' };
  }
  if (!health.healthy) return { label: 'Degraded', tone: 'bad' };
  return { label: 'All normal', tone: 'ok' };
}

export function errorMessage(error: unknown, fallback: string): string {
  if (typeof error === 'string' && error.trim()) return error.trim();
  if (error instanceof Error && error.message.trim()) return error.message.trim();
  return fallback;
}

export function pluralize(count: number, singular: string, plural = `${singular}s`): string {
  return `${count} ${count === 1 ? singular : plural}`;
}
