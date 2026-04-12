import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  normalizeTailscale,
  shortId,
  shortFingerprint,
  timeAgo,
  orderedRoutes,
  routeLabel,
  preferredRouteLabel,
  sessionStateLabel,
  meshStateLabel,
  peerAddresses,
  trustLabel,
  copyToClipboard,
  formatLatency,
  healthSummary,
} from '../lib/utils';

describe('normalizeTailscale', () => {
  it('returns defaults for null input', () => {
    const result = normalizeTailscale(null);
    expect(result.available).toBe(false);
    expect(result.connected).toBe(false);
    expect(result.selfIPs).toEqual([]);
    expect(result.peerCount).toBe(0);
  });

  it('merges partial status over defaults', () => {
    const result = normalizeTailscale({ available: true, peerCount: 5 });
    expect(result.available).toBe(true);
    expect(result.peerCount).toBe(5);
    expect(result.connected).toBe(false);
  });

  it('preserves selfIPs array from input', () => {
    const result = normalizeTailscale({ selfIPs: ['100.64.0.1'] });
    expect(result.selfIPs).toEqual(['100.64.0.1']);
  });

  it('defaults selfIPs to [] when missing from input', () => {
    const result = normalizeTailscale({ available: true });
    expect(result.selfIPs).toEqual([]);
  });
});

describe('shortId', () => {
  it('truncates to 12 characters', () => {
    expect(shortId('abcdef123456789')).toBe('abcdef123456');
  });

  it('returns full id if shorter than 12', () => {
    expect(shortId('abc')).toBe('abc');
  });

  it('returns "unassigned" for falsy input', () => {
    expect(shortId('')).toBe('unassigned');
    expect(shortId(null)).toBe('unassigned');
    expect(shortId(undefined)).toBe('unassigned');
  });
});

describe('shortFingerprint', () => {
  it('formats fingerprint with colon separators', () => {
    const fp = 'AABBCCDDEEFF112233445566';
    const result = shortFingerprint(fp);
    expect(result).toBe('AABB:CCDD:EEFF:1122:3344:5566');
  });

  it('returns "pending" for falsy input', () => {
    expect(shortFingerprint('')).toBe('pending');
    expect(shortFingerprint(null)).toBe('pending');
  });

  it('truncates long fingerprints to 24 chars before formatting', () => {
    const fp = 'AABBCCDDEEFF112233445566EXTRA';
    const result = shortFingerprint(fp);
    expect(result).toBe('AABB:CCDD:EEFF:1122:3344:5566');
  });
});

describe('timeAgo', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-03-26T12:00:00Z'));
  });

  it('returns "never" for falsy timestamp', () => {
    expect(timeAgo(0)).toBe('never');
    expect(timeAgo(null)).toBe('never');
  });

  it('returns "just now" for < 5 seconds ago', () => {
    const now = Math.floor(Date.now() / 1000);
    expect(timeAgo(now - 2)).toBe('just now');
  });

  it('returns seconds for < 60 seconds ago', () => {
    const now = Math.floor(Date.now() / 1000);
    expect(timeAgo(now - 30)).toBe('30s ago');
  });

  it('returns minutes for < 1 hour ago', () => {
    const now = Math.floor(Date.now() / 1000);
    expect(timeAgo(now - 180)).toBe('3m ago');
  });

  it('returns hours for < 1 day ago', () => {
    const now = Math.floor(Date.now() / 1000);
    expect(timeAgo(now - 7200)).toBe('2h ago');
  });

  it('returns days for >= 1 day ago', () => {
    const now = Math.floor(Date.now() / 1000);
    expect(timeAgo(now - 172800)).toBe('2d ago');
  });
});

describe('orderedRoutes', () => {
  it('sorts lan before tailscale before manual', () => {
    expect(orderedRoutes(['manual', 'tailscale', 'lan'])).toEqual(['lan', 'tailscale', 'manual']);
  });

  it('handles empty array', () => {
    expect(orderedRoutes([])).toEqual([]);
  });

  it('handles undefined', () => {
    expect(orderedRoutes(undefined)).toEqual([]);
  });

  it('puts unknown routes last', () => {
    expect(orderedRoutes(['unknown', 'lan'])).toEqual(['lan', 'unknown']);
  });
});

describe('routeLabel', () => {
  it('maps lan to LAN', () => expect(routeLabel('lan')).toBe('LAN'));
  it('maps tailscale to Tailnet', () => expect(routeLabel('tailscale')).toBe('Tailnet'));
  it('maps manual to Manual', () => expect(routeLabel('manual')).toBe('Manual'));
  it('returns unknown routes as-is', () => expect(routeLabel('wireguard')).toBe('wireguard'));
});

describe('preferredRouteLabel', () => {
  it('returns "Auto" for falsy input', () => {
    expect(preferredRouteLabel('')).toBe('Auto');
    expect(preferredRouteLabel(null)).toBe('Auto');
  });

  it('delegates to routeLabel for truthy input', () => {
    expect(preferredRouteLabel('lan')).toBe('LAN');
  });
});

describe('sessionStateLabel', () => {
  it('returns Idle when not connected', () => {
    expect(sessionStateLabel({ connected: false })).toBe('Idle');
  });

  it('returns Controlling when controlling', () => {
    expect(sessionStateLabel({ connected: true, controlling: true })).toBe('Controlling');
  });

  it('returns Controlled when role is controlled', () => {
    expect(sessionStateLabel({ connected: true, controlling: false, role: 'controlled' })).toBe('Controlled');
  });

  it('returns Connected for other connected states', () => {
    expect(sessionStateLabel({ connected: true, controlling: false, role: 'controller' })).toBe('Connected');
  });
});

describe('meshStateLabel', () => {
  it('returns Unavailable when not available', () => {
    expect(meshStateLabel({ available: false })).toBe('Unavailable');
  });

  it('returns Connected when available and connected', () => {
    expect(meshStateLabel({ available: true, connected: true })).toBe('Connected');
  });

  it('returns backendState when available but not connected', () => {
    expect(meshStateLabel({ available: true, connected: false, backendState: 'NeedsLogin' })).toBe('NeedsLogin');
  });

  it('returns Degraded when backendState is empty', () => {
    expect(meshStateLabel({ available: true, connected: false, backendState: '' })).toBe('Degraded');
  });
});

describe('peerAddresses', () => {
  it('joins addresses with comma', () => {
    expect(peerAddresses({ addresses: ['192.168.1.1:24831', '100.64.0.2:24831'] })).toBe('192.168.1.1:24831, 100.64.0.2:24831');
  });

  it('returns empty string for no addresses', () => {
    expect(peerAddresses({ addresses: [] })).toBe('');
    expect(peerAddresses({})).toBe('');
  });
});

describe('trustLabel', () => {
  it('returns "Pinned" for trusted peers', () => {
    expect(trustLabel({ trusted: true })).toBe('Pinned');
  });

  it('returns PIN label for untrusted peers', () => {
    expect(trustLabel({ trusted: false })).toBe('PIN required');
  });

  it('handles null peer', () => {
    expect(trustLabel(null)).toBe('PIN required');
  });
});

describe('copyToClipboard', () => {
  it('does nothing for falsy value', async () => {
    await expect(copyToClipboard('')).resolves.toBeUndefined();
    await expect(copyToClipboard(null)).resolves.toBeUndefined();
  });
});

describe('formatLatency', () => {
  it('returns Measuring... for null', () => {
    expect(formatLatency(null)).toBe('Measuring...');
  });

  it('returns Measuring... for negative values', () => {
    expect(formatLatency(-1)).toBe('Measuring...');
  });

  it('returns <1 ms for zero', () => {
    expect(formatLatency(0)).toBe('<1 ms');
  });

  it('returns ms value for positive numbers', () => {
    expect(formatLatency(42)).toBe('42 ms');
    expect(formatLatency(1)).toBe('1 ms');
    expect(formatLatency(999)).toBe('999 ms');
  });
});

describe('healthSummary', () => {
  it('returns Unknown for null', () => {
    expect(healthSummary(null)).toBe('Unknown');
  });

  it('returns Reconnecting when reconnecting', () => {
    expect(healthSummary({ reconnecting: true, subsystems: [] })).toBe('Reconnecting...');
  });

  it('returns Healthy when all subsystems ok', () => {
    expect(healthSummary({ reconnecting: false, subsystems: [{ name: 'transport', healthy: true }] })).toBe('Healthy');
  });

  it('returns Degraded with unhealthy subsystem names', () => {
    const health = { reconnecting: false, subsystems: [{ name: 'transport', healthy: true }, { name: 'tailscale', healthy: false }] };
    expect(healthSummary(health)).toBe('Degraded: tailscale');
  });
});
