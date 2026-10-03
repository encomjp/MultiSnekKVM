import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import App from '../App.svelte';
import { createPreviewApi, type PreviewApi } from '../lib/api';
import { createWailsApi } from '../lib/api/wails';

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  delete window.go;
  delete window.runtime;
});

async function renderApp(api: PreviewApi = createPreviewApi()) {
  render(App, { props: { api, version: '0.3.0' } });
  await screen.findByRole('heading', { level: 1 });
  await waitFor(() => expect(screen.getByRole('heading', { level: 1 })).not.toHaveTextContent('No session'));
  return api;
}

function nav() {
  return within(screen.getByRole('navigation', { name: 'Primary' }));
}

async function goTo(name: RegExp) {
  await fireEvent.click(nav().getByRole('button', { name }));
}

function card() {
  return within(screen.getByRole('region', { name: 'Bluetooth' }));
}

describe('Devices > Bluetooth card', () => {
  it('shows the ready state and the paired PCs that were found', async () => {
    await renderApp();
    await goTo(/^Devices/);
    const bt = card();
    expect(bt.getByText('Ready · 2 paired PCs running MultiSnek')).toBeInTheDocument();
    const list = bt.getByRole('list', { name: /Paired PCs found over Bluetooth/ });
    expect(within(list).getByText('Studio PC')).toBeInTheDocument();
    expect(within(list).getByText('bt://00:1A:7D:DA:71:13')).toBeInTheDocument();
    expect(within(list).getByText('bt://00:1A:7D:DA:71:27')).toBeInTheDocument();
    expect(bt.getByText(/Pair both PCs in Windows Bluetooth settings first/)).toBeInTheDocument();
    expect(bt.getByRole('switch', { name: 'Use Bluetooth' })).toBeChecked();
  });

  it('refresh scans, disables the button while busy, then settles', async () => {
    await renderApp();
    await goTo(/^Devices/);
    const refresh = card().getByRole('button', { name: /Refresh/ });
    expect(refresh).toBeEnabled();
    expect(refresh).not.toHaveAttribute('aria-busy');

    await fireEvent.click(refresh);
    expect(await card().findByText('Scanning…')).toBeInTheDocument();
    expect(card().getByRole('button', { name: /Refresh/ })).toBeDisabled();
    expect(card().getByRole('button', { name: /Refresh/ })).toHaveAttribute('aria-busy', 'true');

    await waitFor(() => expect(card().getByText(/^Ready · 2 paired PCs/)).toBeInTheDocument(), { timeout: 2000 });
    expect(card().getByRole('button', { name: /Refresh/ })).toBeEnabled();
  });

  it('turning Bluetooth off clears the list and back on restores it', async () => {
    const api = await renderApp();
    const set = vi.spyOn(api, 'SetBluetoothEnabled');
    await goTo(/^Devices/);

    await fireEvent.click(card().getByRole('switch', { name: 'Use Bluetooth' }));
    expect(set).toHaveBeenCalledWith(false);
    expect(await card().findByText('Off')).toBeInTheDocument();
    expect(card().queryByText('Studio PC')).not.toBeInTheDocument();
    expect(card().getByRole('button', { name: /Refresh/ })).toBeDisabled();

    await fireEvent.click(card().getByRole('switch', { name: 'Use Bluetooth' }));
    expect(set).toHaveBeenLastCalledWith(true);
    expect(await card().findByText('Studio PC')).toBeInTheDocument();
  });

  it('reverts the toggle and shows the error when saving fails', async () => {
    const api = createPreviewApi();
    vi.spyOn(api, 'SetBluetoothEnabled').mockRejectedValue(new Error('adapter missing'));
    await renderApp(api);
    await goTo(/^Devices/);
    await fireEvent.click(card().getByRole('switch', { name: 'Use Bluetooth' }));
    expect(await card().findByText('adapter missing')).toBeInTheDocument();
    expect(card().getByRole('switch', { name: 'Use Bluetooth' })).toBeChecked();
  });

  it('explains when Bluetooth is not available, with the error text', async () => {
    const api = createPreviewApi({
      bluetooth: {
        available: false,
        enabled: false,
        listening: false,
        scanning: false,
        error: 'No Bluetooth radio found',
        devices: [],
        lastScan: 0,
      },
    });
    await renderApp(api);
    await goTo(/^Devices/);
    expect(card().getByText('Not available on this PC')).toBeInTheDocument();
    expect(card().getByText('No Bluetooth radio found')).toBeInTheDocument();
    expect(card().getByRole('switch', { name: 'Use Bluetooth' })).toBeDisabled();
    expect(card().getByRole('button', { name: /Refresh/ })).toBeDisabled();
  });

  it('follows bluetooth-updated events', async () => {
    const api = await renderApp();
    await goTo(/^Devices/);
    api.emit('bluetooth-updated', {
      available: true,
      enabled: true,
      listening: true,
      scanning: false,
      devices: [{ address: 'bt://AA:BB:CC:DD:EE:FF', deviceId: 'lab', name: 'Lab PC' }],
      lastScan: 0,
    });
    expect(await card().findByText('Ready · 1 paired PC running MultiSnek')).toBeInTheDocument();
    expect(card().getByText('bt://AA:BB:CC:DD:EE:FF')).toBeInTheDocument();
  });

  it('mentions direct Bluetooth and PAN in the connection guide', async () => {
    await renderApp();
    await goTo(/^Devices/);
    expect(screen.getByText(/Bluetooth \(direct\):/)).toBeInTheDocument();
    expect(screen.getByText(/Bluetooth PAN:/)).toBeInTheDocument();
  });
});

describe('Bluetooth routes in the UI', () => {
  it('lists the direct Bluetooth route last for Travel Laptop with its bt:// address', async () => {
    await renderApp();
    await goTo(/^Devices/);
    const peer = within(screen.getByRole('listitem', { name: 'Travel Laptop' }));
    const radios = peer.getAllByRole('radio');
    expect(radios).toHaveLength(3);
    expect(radios[2]).toHaveAccessibleName(/Bluetooth/);
    expect(radios[2]).toHaveAttribute('value', 'bt://00:1A:7D:DA:71:27');
    expect(peer.getByText('bt://00:1A:7D:DA:71:27')).toBeInTheDocument();
    expect(peer.queryByText(/PAN/)).not.toBeInTheDocument();
  });

  it('connects over Bluetooth and shows the route and address on the Session screen', async () => {
    const api = await renderApp();
    const connect = vi.spyOn(api, 'Connect');
    await goTo(/^Devices/);
    const peer = within(screen.getByRole('listitem', { name: 'Travel Laptop' }));
    await fireEvent.click(peer.getByRole('radio', { name: /Bluetooth/ }));
    await fireEvent.click(peer.getByRole('button', { name: 'Connect' }));
    expect(connect).toHaveBeenCalledWith('bt://00:1A:7D:DA:71:27');

    const details = await screen.findByRole('region', { name: 'Connection details' });
    expect(within(details).getByText('Bluetooth')).toBeInTheDocument();
    expect(within(details).getByText('bt://00:1A:7D:DA:71:27')).toBeInTheDocument();
    expect(within(details).queryByText(/PAN/)).not.toBeInTheDocument();
  });

  it('accepts a bt:// address when adding a device', async () => {
    await renderApp();
    await goTo(/^Devices/);
    await fireEvent.input(screen.getByLabelText('Peer IP address or hostname'), {
      target: { value: 'bt://AA:BB:CC:DD:EE:FF' },
    });
    await fireEvent.click(screen.getByRole('button', { name: /Add device/ }));
    expect((await screen.findAllByText('bt://AA:BB:CC:DD:EE:FF')).length).toBeGreaterThan(0);
  });
});

describe('Settings', () => {
  it('has a Bluetooth connections toggle on the Startup page bound to the same store', async () => {
    const api = await renderApp();
    const set = vi.spyOn(api, 'SetBluetoothEnabled');
    await goTo(/^Settings/);
    await fireEvent.click(nav().getByRole('button', { name: 'Startup' }));
    const toggle = screen.getByRole('switch', { name: /Bluetooth connections/ });
    expect(toggle).toBeChecked();
    await fireEvent.click(toggle);
    expect(set).toHaveBeenCalledWith(false);
    await waitFor(() => expect(screen.getByRole('switch', { name: /Bluetooth connections/ })).not.toBeChecked());

    await goTo(/^Devices/);
    expect(await card().findByText('Off')).toBeInTheDocument();
  });

  it('notes that desktop audio is always Opus over Bluetooth', async () => {
    await renderApp();
    await goTo(/^Settings/);
    await fireEvent.click(nav().getByRole('button', { name: 'Audio' }));
    expect(screen.getByText('Over Bluetooth, desktop audio is always compressed (Opus ≤ 96 kbit/s).')).toBeInTheDocument();
  });
});

describe('Wails Bluetooth bindings', () => {
  it('normalizes the status and calls the bound methods and event', async () => {
    const App = {
      GetBluetoothStatus: vi.fn().mockResolvedValue({ available: true, enabled: true, devices: null, lastScan: 5 }),
      SetBluetoothEnabled: vi.fn().mockResolvedValue(undefined),
      RefreshBluetooth: vi.fn().mockResolvedValue(undefined),
    };
    let handler: ((...data: unknown[]) => void) | undefined;
    window.go = { app: { App } };
    window.runtime = {
      EventsOnMultiple: vi.fn((_name: string, cb: (...data: unknown[]) => void) => {
        handler = cb;
        return () => {};
      }),
    };
    const api = createWailsApi();

    expect(await api.GetBluetoothStatus()).toEqual({
      available: true,
      enabled: true,
      listening: false,
      scanning: false,
      devices: [],
      lastScan: 5,
    });
    await api.SetBluetoothEnabled(false);
    expect(App.SetBluetoothEnabled).toHaveBeenCalledWith(false);
    await api.RefreshBluetooth();
    expect(App.RefreshBluetooth).toHaveBeenCalled();

    const received = vi.fn();
    api.on('bluetooth-updated', received);
    handler?.({ available: true, enabled: true, listening: true, scanning: true, devices: null });
    expect(received).toHaveBeenCalledWith(expect.objectContaining({ scanning: true, devices: [] }));
  });
});
