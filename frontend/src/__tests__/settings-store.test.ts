import { describe, expect, it, vi } from 'vitest';
import { createPreviewApi } from '../lib/api';
import { DEFAULT_SETTINGS, SettingsStore } from '../lib/stores/settings.svelte';

function deferred() {
  let resolve!: () => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('SettingsStore', () => {
  it('loads every setting and the device lists', async () => {
    const store = new SettingsStore(createPreviewApi());
    expect(store.values).toEqual(DEFAULT_SETTINGS);
    await store.load();
    expect(store.loaded).toBe(true);
    expect(store.values.audioMode).toBe('remote');
    expect(store.values.triggerZone).toEqual({ monitorID: 'display1', side: 'right', startPct: 0.2, endPct: 0.7 });
    expect(store.monitors).toHaveLength(2);
    expect(store.audioDevices.length).toBeGreaterThan(0);
  });

  it('keeps defaults for getters that fail', async () => {
    const api = createPreviewApi();
    vi.spyOn(api, 'GetAudioTransport').mockRejectedValue(new Error('not bound'));
    const store = new SettingsStore(api);
    await store.load();
    expect(store.values.audioTransport).toBe('auto');
    expect(store.values.audioMode).toBe('remote');
  });

  it('saves optimistically and tracks pending per setting', async () => {
    const api = createPreviewApi();
    const gate = deferred();
    vi.spyOn(api, 'SetMicMode').mockReturnValue(gate.promise);
    const store = new SettingsStore(api);

    const saving = store.set('micMode', 'send');
    expect(store.values.micMode).toBe('send');
    expect(store.status.micMode.pending).toBe(true);
    expect(store.status.audioMode.pending).toBe(false);

    gate.resolve();
    await expect(saving).resolves.toBe(true);
    expect(store.status.micMode).toEqual({ pending: false, error: '' });
  });

  it('reverts only the failed setting and reports the error', async () => {
    const api = createPreviewApi();
    vi.spyOn(api, 'SetEdgeSide').mockRejectedValue(new Error('refused'));
    const onError = vi.fn();
    const store = new SettingsStore(api, onError);
    await store.load();
    await store.set('sensitivity', 1.5);

    await expect(store.set('edgeSide', 'left')).resolves.toBe(false);
    expect(store.values.edgeSide).toBe('right');
    expect(store.status.edgeSide).toEqual({ pending: false, error: 'refused' });
    expect(store.values.sensitivity).toBe(1.5);
    expect(onError).toHaveBeenCalledWith('edgeSide', 'screen edge', 'refused');
  });

  it('does not let a stale failure clobber a newer save', async () => {
    const api = createPreviewApi();
    const first = deferred();
    const spy = vi.spyOn(api, 'SetAudioProfile').mockReturnValueOnce(first.promise).mockResolvedValueOnce(undefined);
    const store = new SettingsStore(api);

    const a = store.set('audioProfile', 'music');
    const b = store.set('audioProfile', 'low-latency');
    await b;
    first.reject(new Error('late failure'));
    await a;

    expect(spy).toHaveBeenCalledTimes(2);
    expect(store.values.audioProfile).toBe('low-latency');
    expect(store.status.audioProfile.error).toBe('');
  });

  it('clears zones through the Clear* bindings', async () => {
    const api = createPreviewApi();
    const clearZone = vi.spyOn(api, 'ClearTriggerZone');
    const clearAnchor = vi.spyOn(api, 'ClearReturnAnchor');
    const store = new SettingsStore(api);
    await store.load();
    await store.set('triggerZone', null);
    await store.set('returnAnchor', null);
    expect(clearZone).toHaveBeenCalled();
    expect(clearAnchor).toHaveBeenCalled();
    expect(await api.GetTriggerZone()).toBeNull();
  });
});
