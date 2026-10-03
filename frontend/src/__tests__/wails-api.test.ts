import { afterEach, describe, expect, it, vi } from 'vitest';
import { getApi } from '../lib/api';
import { createWailsApi } from '../lib/api/wails';

afterEach(() => {
  delete window.go;
  delete window.runtime;
});

describe('Wails API wrapper', () => {
  it('falls back to the preview api without a backend', () => {
    expect(getApi().kind).toBe('preview');
  });

  it('calls the bound Go methods and normalizes nil values', async () => {
    const App = {
      GetPeers: vi.fn().mockResolvedValue([{ id: 'a', address: '10.0.0.1:24831', addresses: null, routes: null }]),
      GetLastPeer: vi.fn().mockResolvedValue({}),
      Connect: vi.fn().mockResolvedValue(undefined),
      SaveReceivedFiles: vi.fn().mockResolvedValue({ dest: 'D:\\Downloads', saved: null }),
    };
    window.go = { app: { App } };
    const api = getApi();
    expect(api.kind).toBe('wails');

    const peers = await api.GetPeers();
    expect(peers[0].addresses).toEqual(['10.0.0.1:24831']);
    expect(peers[0].routes).toEqual([]);
    expect(await api.GetLastPeer()).toBeNull();
    await api.Connect('10.0.0.1:24831');
    expect(App.Connect).toHaveBeenCalledWith('10.0.0.1:24831');
    expect(await api.SaveReceivedFiles('C:\\t')).toEqual({ dest: 'D:\\Downloads', saved: [] });
  });

  it('subscribes to runtime events with normalized payloads', () => {
    const off = vi.fn();
    let handler: ((...data: unknown[]) => void) | undefined;
    window.runtime = {
      EventsOnMultiple: vi.fn((_name: string, cb: (...data: unknown[]) => void) => {
        handler = cb;
        return off;
      }),
    };
    const api = createWailsApi();
    const received = vi.fn();
    const unsubscribe = api.on('file-received', received);
    handler?.({ count: 1, names: null, tempDir: 'C:\\t' });
    expect(received).toHaveBeenCalledWith({ count: 1, names: [], tempDir: 'C:\\t' });
    unsubscribe();
    expect(off).toHaveBeenCalled();
  });

  it('is a no-op subscription when the runtime is missing', () => {
    const api = createWailsApi();
    expect(() => api.on('peers-updated', () => {})()).not.toThrow();
  });
});
