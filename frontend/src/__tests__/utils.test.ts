import { describe, expect, it } from 'vitest';
import type { Peer } from '../lib/api/types';
import { normalizePeer, normalizeSession, normalizeTriggerZone } from '../lib/api/normalize';
import { captureHotkey, hotkeyKeys, hotkeyLabel, vkName } from '../lib/hotkeys';
import {
  addressLabel,
  endpointLabel,
  errorMessage,
  formatLatency,
  formatPin,
  healthSummary,
  hostOf,
  interfaceLabel,
  jitterLabel,
  lastPeerAddress,
  lastPeerRoute,
  latencyQuality,
  orderedRoutes,
  peerRoutes,
  routeLabel,
  sanitizePin,
  shortFingerprint,
  timeAgo,
} from '../lib/utils';

function peer(overrides: Partial<Peer> = {}): Peer {
  return normalizePeer({ id: 'p', name: 'P', address: '10.0.0.2:24831', ...overrides });
}

describe('routeLabel', () => {
  it.each([
    ['usb4', 'USB4 / Thunderbolt'],
    ['usb-bridge', 'USB network bridge'],
    ['ethernet', 'Ethernet'],
    ['wifi', 'Wi-Fi'],
    ['bluetooth', 'Bluetooth PAN'],
    ['network', 'Network'],
    ['tailscale', 'Tailnet'],
    ['lan', 'LAN'],
    ['manual', 'Manual'],
    ['', 'Auto'],
    ['carrier-pigeon', 'carrier-pigeon'],
  ])('%s -> %s', (route, label) => {
    expect(routeLabel(route)).toBe(label);
  });

  it('handles missing routes and adapter kinds', () => {
    expect(routeLabel(undefined)).toBe('Auto');
    expect(routeLabel(null)).toBe('Auto');
    expect(interfaceLabel('network')).toBe('Network adapter');
    expect(interfaceLabel('usb4')).toBe('USB4 / Thunderbolt');
  });

  it('orders routes by backend preference', () => {
    expect(orderedRoutes(['manual', 'tailscale', 'wifi', 'usb4', 'bluetooth', 'ethernet', 'lan', 'usb-bridge', 'network'])).toEqual([
      'usb4',
      'usb-bridge',
      'ethernet',
      'lan',
      'wifi',
      'network',
      'tailscale',
      'bluetooth',
      'manual',
    ]);
  });
});

describe('endpoint labels', () => {
  it('classifies addresses by IP range', () => {
    expect(endpointLabel('169.254.22.15:24831')).toBe('Direct link');
    expect(endpointLabel('100.92.10.17:24831')).toBe('Tailnet / CGNAT');
    expect(endpointLabel('100.128.0.1')).toBe('Network');
    expect(endpointLabel('192.168.0.42:24831')).toBe('Network');
    expect(endpointLabel('[fe80::1]:24831')).toBe('Direct link');
    expect(endpointLabel('studio.local:24831')).toBe('Network');
  });

  it('strips ports and IPv6 brackets', () => {
    expect(hostOf('192.168.0.42:24831')).toBe('192.168.0.42');
    expect(hostOf('[fe80::1]:24831')).toBe('fe80::1');
    expect(hostOf('fe80::1')).toBe('fe80::1');
    expect(hostOf('studio')).toBe('studio');
  });

  it('prefers backend address kinds over IP guesses', () => {
    const p = peer({ addressKinds: { '192.168.0.42:24831': 'ethernet' } });
    expect(addressLabel(p, '192.168.0.42:24831')).toBe('Ethernet');
    expect(addressLabel(p, '169.254.1.1:24831')).toBe('Direct link');
  });

  it('lists routes with the recommended address first', () => {
    const p = peer({
      address: '169.254.22.15:24831',
      addresses: ['100.92.10.17:24831', '192.168.0.42:24831', '169.254.22.15:24831'],
      addressKinds: { '169.254.22.15:24831': 'usb4', '192.168.0.42:24831': 'ethernet', '100.92.10.17:24831': 'tailscale' },
    });
    expect(peerRoutes(p).map((r) => [r.label, r.best])).toEqual([
      ['USB4 / Thunderbolt', true],
      ['Ethernet', false],
      ['Tailnet', false],
    ]);
  });
});

describe('last peer', () => {
  it('picks the address by route preference', () => {
    expect(lastPeerAddress({ id: 'x', tailscale: '100.1.1.1', wifi: '192.168.0.5', usb4: '169.254.0.2' })).toBe('169.254.0.2');
    expect(lastPeerAddress({ id: 'x', bluetooth: '10.0.0.1', manual: '10.0.0.2' })).toBe('10.0.0.1');
    expect(lastPeerAddress({ id: 'x', address: '10.0.0.3' })).toBe('10.0.0.3');
    expect(lastPeerAddress(null)).toBe('');
    expect(lastPeerRoute({ id: 'x', wifi: 'a', tailscale: 'b' })).toBe('wifi');
    expect(lastPeerRoute({ id: 'x', address: 'a' })).toBe('');
  });
});

describe('formatting', () => {
  it('formats latency', () => {
    expect(formatLatency(-1)).toBe('Measuring…');
    expect(formatLatency(undefined)).toBe('Measuring…');
    expect(formatLatency(0)).toBe('<1 ms');
    expect(formatLatency(12)).toBe('12 ms');
    expect(formatLatency(1.84)).toBe('1.8 ms');
  });

  it('grades latency and jitter', () => {
    expect(latencyQuality(-1)).toBe('unknown');
    expect(latencyQuality(2)).toBe('excellent');
    expect(latencyQuality(15)).toBe('good');
    expect(latencyQuality(45)).toBe('fair');
    expect(latencyQuality(120)).toBe('poor');
    expect(jitterLabel(-1)).toBe('Measuring');
    expect(jitterLabel(1)).toBe('Stable');
    expect(jitterLabel(30)).toBe('Unstable');
  });

  it('formats PINs and fingerprints', () => {
    expect(formatPin('482911')).toBe('482 911');
    expect(sanitizePin('48-29 11 7')).toBe('482911');
    expect(shortFingerprint('4FA2C19B8DD44A89A0FE31C19D68D2F17E05B69C')).toBe('4F:A2:…:9C');
    expect(shortFingerprint('')).toBe('pending');
  });

  it('formats relative time', () => {
    const now = 1_000_000;
    expect(timeAgo(0, now)).toBe('never');
    expect(timeAgo(now - 3, now)).toBe('just now');
    expect(timeAgo(now - 120, now)).toBe('2 min ago');
    expect(timeAgo(now - 3600, now)).toBe('1 hour ago');
    expect(timeAgo(now - 2 * 86400, now)).toBe('2 days ago');
  });

  it('summarises health', () => {
    const base = { healthy: true, reconnecting: false, subsystems: [], uptime: 0, goroutines: 0, goroutineDelta: 0 };
    expect(healthSummary(base)).toEqual({ label: 'All normal', tone: 'ok' });
    expect(healthSummary({ ...base, reconnecting: true }).tone).toBe('warn');
    expect(healthSummary({ ...base, subsystems: [{ name: 'Audio', healthy: false, detail: '' }] })).toEqual({
      label: 'Degraded: Audio',
      tone: 'bad',
    });
  });

  it('extracts error messages', () => {
    expect(errorMessage('boom', 'fallback')).toBe('boom');
    expect(errorMessage(new Error('bad'), 'fallback')).toBe('bad');
    expect(errorMessage({}, 'fallback')).toBe('fallback');
  });
});

describe('normalizers', () => {
  it('fills nil slices and defaults from Go', () => {
    const p = normalizePeer({ id: 'a', address: '10.0.0.1:24831', addresses: null, routes: null, addressKinds: null });
    expect(p.addresses).toEqual(['10.0.0.1:24831']);
    expect(p.routes).toEqual([]);
    expect(p.addressKinds).toEqual({});
    expect(normalizeSession(null)).toMatchObject({ connected: false, latencyMs: -1, jitterMs: -1 });
    expect(normalizeTriggerZone({})).toBeNull();
    expect(normalizeTriggerZone({ monitorID: 'm', side: 'top', startPct: 0.2, endPct: 0 })).toEqual({
      monitorID: 'm',
      side: 'top',
      startPct: 0.2,
      endPct: 1,
    });
  });
});

describe('hotkeys', () => {
  const key = (overrides: Partial<KeyboardEvent>) =>
    ({ key: 'x', keyCode: 0, ctrlKey: false, altKey: false, shiftKey: false, metaKey: false, ...overrides }) as KeyboardEvent;

  it('names keys and combinations', () => {
    expect(vkName(0x24)).toBe('Home');
    expect(vkName(0x41)).toBe('A');
    expect(vkName(0x77)).toBe('F8');
    expect(hotkeyKeys({ modifiers: 0, vkCode: 0 })).toEqual(['Esc']);
    expect(hotkeyLabel({ modifiers: 1 | 4, vkCode: 0x24 })).toBe('Ctrl + Shift + Home');
  });

  it('captures valid combinations and rejects unsafe ones', () => {
    expect(captureHotkey(key({ key: 'Escape' }))).toEqual({ kind: 'cancel' });
    expect(captureHotkey(key({ key: 'Shift', shiftKey: true })).kind).toBe('ignore');
    expect(captureHotkey(key({ key: 'a', keyCode: 0x41 })).kind).toBe('invalid');
    expect(captureHotkey(key({ key: 'Home', keyCode: 0x24, ctrlKey: true, altKey: true })).kind).toBe('invalid');
    expect(captureHotkey(key({ key: 'F9', keyCode: 0x78 }))).toEqual({ kind: 'ok', hotkey: { modifiers: 0, vkCode: 0x78 } });
    expect(captureHotkey(key({ key: 'Home', keyCode: 0x24, ctrlKey: true, shiftKey: true }))).toEqual({
      kind: 'ok',
      hotkey: { modifiers: 5, vkCode: 0x24 },
    });
  });
});
