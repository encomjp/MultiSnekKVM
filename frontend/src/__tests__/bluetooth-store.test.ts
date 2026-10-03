import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPreviewApi } from '../lib/api';
import { normalizeBluetooth } from '../lib/api/normalize';
import { BluetoothStore } from '../lib/stores/bluetooth.svelte';

function deferred() {
  let resolve!: () => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('normalizeBluetooth', () => {
  it('fills defaults and drops devices without an address', () => {
    expect(normalizeBluetooth(null)).toEqual({
      available: false,
      enabled: false,
      listening: false,
      scanning: false,
      devices: [],
      lastScan: 0,
    });
    const status = normalizeBluetooth({
      available: true,
      enabled: true,
      error: 'radio off',
      devices: [{ address: 'bt://AA:BB:CC:DD:EE:FF', deviceId: 'x', name: 'X' }, { name: 'no address' }],
      lastScan: 12,
    });
    expect(status.devices).toEqual([{ address: 'bt://AA:BB:CC:DD:EE:FF', deviceId: 'x', name: 'X' }]);
    expect(status.error).toBe('radio off');
    expect(status.lastScan).toBe(12);
  });
});

describe('BluetoothStore', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('loads the status', async () => {
    const store = new BluetoothStore(createPreviewApi());
    expect(store.status.available).toBe(false);
    await store.load();
    expect(store.loaded).toBe(true);
    expect(store.status).toMatchObject({ available: true, enabled: true, listening: true });
    expect(store.status.devices.map((d) => d.name)).toEqual(['Studio PC', 'Travel Laptop']);
  });

  it('stays unavailable when the backend call fails', async () => {
    const api = createPreviewApi();
    vi.spyOn(api, 'GetBluetoothStatus').mockRejectedValue(new Error('not bound'));
    const store = new BluetoothStore(api);
    await store.load();
    expect(store.loaded).toBe(true);
    expect(store.status.available).toBe(false);
  });

  it('refresh shows scanning at once and ends when the backend says so', async () => {
    const api = createPreviewApi();
    const store = new BluetoothStore(api);
    await store.load();
    api.on('bluetooth-updated', (status) => store.apply(status));

    const refreshing = store.refresh();
    expect(store.scanning).toBe(true);
    await refreshing;
    expect(store.status.scanning).toBe(true);

    await vi.advanceTimersByTimeAsync(900);
    expect(store.status.scanning).toBe(false);
    expect(store.scanning).toBe(false);
    expect(store.status.lastScan).toBeGreaterThan(0);
  });

  it('ignores refresh while scanning, disabled or unavailable', async () => {
    const api = createPreviewApi();
    const spy = vi.spyOn(api, 'RefreshBluetooth');
    const store = new BluetoothStore(api);
    await store.refresh();
    expect(spy).not.toHaveBeenCalled();
    await store.load();
    store.apply({ ...store.status, scanning: true });
    await store.refresh();
    expect(spy).not.toHaveBeenCalled();
    store.apply({ ...store.status, scanning: false, enabled: false });
    await store.refresh();
    expect(spy).not.toHaveBeenCalled();
  });

  it('reports a failed refresh and clears scanning', async () => {
    const api = createPreviewApi();
    vi.spyOn(api, 'RefreshBluetooth').mockRejectedValue(new Error('radio busy'));
    const onError = vi.fn();
    const store = new BluetoothStore(api, onError);
    await store.load();
    await store.refresh();
    expect(store.scanning).toBe(false);
    expect(onError).toHaveBeenCalledWith('radio busy');
  });

  it('applies bluetooth-updated events from the backend', async () => {
    const api = createPreviewApi();
    const store = new BluetoothStore(api);
    await store.load();
    api.on('bluetooth-updated', (status) => store.apply(status));
    api.emit('bluetooth-updated', {
      available: true,
      enabled: true,
      listening: true,
      scanning: false,
      devices: [{ address: 'bt://AA:BB:CC:DD:EE:FF', deviceId: 'lab', name: 'Lab PC' }],
      lastScan: 99,
    });
    expect(store.status.devices).toEqual([{ address: 'bt://AA:BB:CC:DD:EE:FF', deviceId: 'lab', name: 'Lab PC' }]);
    expect(store.status.lastScan).toBe(99);
  });

  it('disabling is optimistic and tracks pending', async () => {
    const api = createPreviewApi();
    const gate = deferred();
    vi.spyOn(api, 'SetBluetoothEnabled').mockReturnValue(gate.promise);
    const store = new BluetoothStore(api);
    await store.load();

    const saving = store.setEnabled(false);
    expect(store.status.enabled).toBe(false);
    expect(store.pending).toBe(true);
    gate.resolve();
    await expect(saving).resolves.toBe(true);
    expect(store.pending).toBe(false);
    expect(store.error).toBe('');
  });

  it('reverts the switch and surfaces the error when saving fails', async () => {
    const api = createPreviewApi();
    vi.spyOn(api, 'SetBluetoothEnabled').mockRejectedValue(new Error('adapter missing'));
    const onError = vi.fn();
    const store = new BluetoothStore(api, onError);
    await store.load();

    await expect(store.setEnabled(false)).resolves.toBe(false);
    expect(store.status.enabled).toBe(true);
    expect(store.status.devices).toHaveLength(2);
    expect(store.pending).toBe(false);
    expect(store.error).toBe('adapter missing');
    expect(onError).toHaveBeenCalledWith('adapter missing');
  });

  it('works end to end with the preview backend: off clears devices, on restores them', async () => {
    const api = createPreviewApi();
    const store = new BluetoothStore(api);
    await store.load();
    api.on('bluetooth-updated', (status) => store.apply(status));

    await store.setEnabled(false);
    expect(store.status).toMatchObject({ enabled: false, listening: false, devices: [] });
    await store.setEnabled(true);
    expect(store.status).toMatchObject({ enabled: true, listening: true });
    expect(store.status.devices).toHaveLength(2);
  });
});
