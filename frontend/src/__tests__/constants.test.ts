import { describe, it, expect } from 'vitest';
import { emptySession, emptyTailscale, emptyHealth, routeRank } from '../lib/constants';

describe('emptySession', () => {
  it('has all required keys with default values', () => {
    expect(emptySession).toEqual({
      connected: false,
      controlling: false,
      peerName: '',
      peerID: '',
      role: '',
      latencyMs: -1,
      audioLatencyMs: -1,
      jitterMs: -1,
    });
  });
});

describe('emptyHealth', () => {
  it('has all required keys with default values', () => {
    expect(emptyHealth).toEqual({
      healthy: true,
      reconnecting: false,
      subsystems: [],
      uptime: 0,
    });
  });
});

describe('emptyTailscale', () => {
  it('has all required keys with default values', () => {
    expect(emptyTailscale.available).toBe(false);
    expect(emptyTailscale.connected).toBe(false);
    expect(emptyTailscale.selfIPs).toEqual([]);
    expect(emptyTailscale.peerCount).toBe(0);
    expect(emptyTailscale.targetCount).toBe(0);
    expect(emptyTailscale.lastSync).toBe(0);
    expect(emptyTailscale.lastError).toBe('');
  });
});

describe('routeRank', () => {
  it('ranks lan < tailscale < manual', () => {
    expect(routeRank.lan).toBeLessThan(routeRank.tailscale);
    expect(routeRank.tailscale).toBeLessThan(routeRank.manual);
  });
});
