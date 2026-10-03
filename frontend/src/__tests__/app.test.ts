import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import App from '../App.svelte';
import { createPreviewApi, type PreviewApi } from '../lib/api';

beforeEach(() => {
  localStorage.clear();
});

async function renderApp(api: PreviewApi = createPreviewApi()) {
  const result = render(App, { props: { api, version: '0.3.0' } });
  await screen.findByRole('heading', { level: 1 });
  // Wait until the initial load has populated the session screen.
  await waitFor(() => expect(screen.getByRole('heading', { level: 1 })).not.toHaveTextContent('No session'));
  return { api, ...result };
}

function nav() {
  return within(screen.getByRole('navigation', { name: 'Primary' }));
}

async function goTo(name: RegExp) {
  await fireEvent.click(nav().getByRole('button', { name }));
}

function peerCard(name: string) {
  return within(screen.getByRole('listitem', { name }));
}

describe('App shell (preview mode)', () => {
  it('renders the Session screen with the last peer', async () => {
    await renderApp();
    expect(screen.getByRole('heading', { level: 1, name: 'Travel Laptop' })).toBeInTheDocument();
    expect(screen.getByText('Not connected')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeEnabled();
    expect(nav().getByRole('button', { name: /^Session/ })).toHaveAttribute('aria-current', 'page');
  });

  it('reveals the local pairing PIN on demand', async () => {
    await renderApp();
    expect(screen.queryByText('482 911')).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Show pairing PIN' }));
    expect(screen.getByText('482 911')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Hide pairing PIN' })).toBeInTheDocument();
  });

  it('navigates to Devices and Settings sections', async () => {
    await renderApp();
    await goTo(/^Devices/);
    expect(screen.getByRole('heading', { level: 1, name: 'Devices' })).toBeInTheDocument();
    expect(nav().getByRole('button', { name: /^Devices/ })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByText('Studio PC')).toBeInTheDocument();

    await goTo(/^Settings/);
    expect(screen.getByRole('heading', { level: 1, name: 'Input & screens' })).toBeInTheDocument();
    await fireEvent.click(nav().getByRole('button', { name: 'Diagnostics' }));
    expect(screen.getByRole('heading', { level: 1, name: 'Diagnostics' })).toBeInTheDocument();
    expect(await screen.findByRole('button', { name: /Copy logs/ })).toBeInTheDocument();
  });

  it('reconnects to the last peer', async () => {
    await renderApp();
    await fireEvent.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(await screen.findByText('Controlling')).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: 'Travel Laptop' })).toBeInTheDocument();
  });
});

describe('Devices', () => {
  it('pairs an untrusted device and lands on a controlling session', async () => {
    const { api } = await renderApp();
    const trust = vi.spyOn(api, 'TrustPeer');
    await goTo(/^Devices/);

    await fireEvent.click(peerCard('Gaming Rig').getByRole('button', { name: 'Pair & connect' }));
    const dialog = screen.getByRole('dialog', { name: /pair with/i });
    const pin = within(dialog).getByLabelText('Pairing PIN');
    expect(pin).toHaveFocus();

    const submit = within(dialog).getByRole('button', { name: 'Pair & connect' });
    expect(submit).toBeDisabled();
    await fireEvent.input(pin, { target: { value: '48-29 11' } });
    expect(pin).toHaveValue('482911');
    await fireEvent.click(submit);

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(trust).toHaveBeenCalledWith('192.168.0.88:24831', '482911');
    expect(await screen.findByRole('heading', { level: 1, name: 'Gaming Rig' })).toBeInTheDocument();
    expect(screen.getByText('Controlling')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
  });

  it('shows a pairing error inside the dialog', async () => {
    const api = createPreviewApi();
    vi.spyOn(api, 'TrustPeer').mockRejectedValue(new Error('PIN rejected by Gaming Rig'));
    await renderApp(api);
    await goTo(/^Devices/);
    await fireEvent.click(peerCard('Gaming Rig').getByRole('button', { name: 'Pair & connect' }));
    const dialog = screen.getByRole('dialog', { name: /pair with/i });
    await fireEvent.input(within(dialog).getByLabelText('Pairing PIN'), { target: { value: '000000' } });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Pair & connect' }));
    expect(await within(dialog).findByText('PIN rejected by Gaming Rig')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('closes the pairing dialog with Escape and restores focus', async () => {
    await renderApp();
    await goTo(/^Devices/);
    const trigger = peerCard('Gaming Rig').getByRole('button', { name: 'Pair & connect' });
    trigger.focus();
    await fireEvent.click(trigger);
    const dialog = screen.getByRole('dialog', { name: /pair with/i });
    await fireEvent.keyDown(dialog, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('untrusting a peer turns it into Pair & connect', async () => {
    const { api } = await renderApp();
    const untrust = vi.spyOn(api, 'UntrustPeer');
    await goTo(/^Devices/);

    expect(peerCard('Travel Laptop').getByRole('button', { name: 'Connect' })).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'More actions for Travel Laptop' }));
    await fireEvent.click(peerCard('Travel Laptop').getByRole('button', { name: 'Untrust' }));

    expect(untrust).toHaveBeenCalledWith('travel-laptop');
    expect(await peerCard('Travel Laptop').findByRole('button', { name: 'Pair & connect' })).toBeInTheDocument();
  });

  it('filters and removes a manual peer', async () => {
    const { api } = await renderApp();
    const remove = vi.spyOn(api, 'RemovePeer');
    await goTo(/^Devices/);

    const manual = screen.getByRole('button', { name: /Manual 1/ });
    await fireEvent.click(manual);
    expect(manual).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('Gaming Rig')).toBeInTheDocument();
    expect(screen.queryByText('Studio PC')).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Remove Gaming Rig' }));
    expect(remove).toHaveBeenCalledWith('192.168.0.88:24831');
    await waitFor(() => expect(screen.queryByText('Gaming Rig')).not.toBeInTheDocument());
  });

  it('adds a manual peer and reports duplicates inline', async () => {
    await renderApp();
    await goTo(/^Devices/);
    const input = screen.getByLabelText('Peer IP address or hostname');
    await fireEvent.input(input, { target: { value: '10.0.0.9' } });
    await fireEvent.click(screen.getByRole('button', { name: /Add device/ }));
    expect(await screen.findByText('10.0.0.9')).toBeInTheDocument();

    await fireEvent.input(input, { target: { value: '10.0.0.9' } });
    await fireEvent.click(screen.getByRole('button', { name: /Add device/ }));
    expect(await screen.findByText(/already in your device list/)).toBeInTheDocument();
  });

  it('connects over the route the user selected', async () => {
    const { api } = await renderApp();
    const connect = vi.spyOn(api, 'Connect');
    await goTo(/^Devices/);

    const studio = peerCard('Studio PC');
    const usb4 = studio.getByRole('radio', { name: /169\.254\.22\.15/ });
    const ethernet = studio.getByRole('radio', { name: /192\.168\.0\.42/ });
    expect(usb4).toBeChecked();

    await fireEvent.click(ethernet);
    expect(ethernet).toBeChecked();
    await fireEvent.click(usb4);
    expect(usb4).toBeChecked();
    await fireEvent.click(studio.getByRole('button', { name: 'Connect' }));

    expect(connect).toHaveBeenCalledWith('169.254.22.15:24831');
    expect(await screen.findByRole('heading', { level: 1, name: 'Studio PC' })).toBeInTheDocument();
  });

  it('uses a non-default route when picked', async () => {
    const { api } = await renderApp();
    const connect = vi.spyOn(api, 'Connect');
    await goTo(/^Devices/);
    const studio = peerCard('Studio PC');
    await fireEvent.click(studio.getByRole('radio', { name: /Ethernet 192\.168\.0\.42/ }));
    await fireEvent.click(studio.getByRole('button', { name: 'Connect' }));
    expect(connect).toHaveBeenCalledWith('192.168.0.42:24831');
  });
});

describe('Settings', () => {
  it('changes audio direction through a radio group', async () => {
    const { api } = await renderApp();
    const setMode = vi.spyOn(api, 'SetAudioMode');
    await goTo(/^Settings/);
    await fireEvent.click(nav().getByRole('button', { name: 'Audio' }));

    const group = within(screen.getByRole('radiogroup', { name: 'Audio direction' }));
    expect(group.getByRole('radio', { name: 'Hear remote' })).toBeChecked();
    await fireEvent.click(group.getByRole('radio', { name: 'Off' }));

    expect(setMode).toHaveBeenCalledWith('off');
    await waitFor(() => expect(group.getByRole('radio', { name: 'Off' })).toBeChecked());
    expect(screen.getByText(/microphone always uses Opus/i)).toBeInTheDocument();
  });

  it('disables edge radios while controlling', async () => {
    const api = createPreviewApi();
    await api.Connect('169.254.22.15:24831');
    render(App, { props: { api } });
    await screen.findByText('Controlling');
    await goTo(/^Settings/);

    const radios = within(screen.getByRole('radiogroup', { name: 'Screen edge' })).getAllByRole('radio');
    expect(radios).toHaveLength(4);
    for (const radio of radios) expect(radio).toBeDisabled();
    expect(screen.getByText(/locked while you control Studio PC/)).toBeInTheDocument();
  });

  it('reverts only the failed setting and reports it', async () => {
    const api = createPreviewApi();
    const setEdge = vi.spyOn(api, 'SetEdgeSide').mockRejectedValue(new Error('backend refused'));
    await renderApp(api);
    await goTo(/^Settings/);

    const group = within(screen.getByRole('radiogroup', { name: 'Screen edge' }));
    await fireEvent.click(group.getByRole('radio', { name: 'Left' }));
    expect(setEdge).toHaveBeenCalledWith('left');

    expect(await screen.findByText(/Couldn't save the screen edge: backend refused/)).toBeInTheDocument();
    await waitFor(() => expect(group.getByRole('radio', { name: 'Right' })).toBeChecked());
    expect(group.getByRole('radio', { name: 'Left' })).not.toBeChecked();
    // No global lock: other controls stay usable.
    expect(group.getByRole('radio', { name: 'Top' })).toBeEnabled();
    expect(screen.getByLabelText('Pointer speed')).toBeEnabled();
  });

  it('records a new exit hotkey with the recorder button', async () => {
    const { api } = await renderApp();
    const setHotkey = vi.spyOn(api, 'SetExitHotkey');
    await goTo(/^Settings/);

    const recorder = screen.getByRole('button', { name: 'Record new…' });
    await fireEvent.click(recorder);
    expect(recorder).toHaveAttribute('aria-pressed', 'true');
    await fireEvent.keyDown(recorder, { key: 'a', keyCode: 0x41 });
    expect(screen.getByText(/would interrupt typing/)).toBeInTheDocument();
    await fireEvent.keyDown(recorder, { key: 'F8', keyCode: 0x77, shiftKey: true });
    expect(setHotkey).toHaveBeenCalledWith(4, 0x77);
  });

  it('persists the theme choice', async () => {
    await renderApp();
    await goTo(/^Settings/);
    await fireEvent.click(nav().getByRole('button', { name: 'Startup' }));
    await fireEvent.click(screen.getByRole('radio', { name: 'Light' }));
    expect(document.documentElement.dataset.theme).toBe('light');
    expect(localStorage.getItem('multisnek.theme')).toBe('light');
  });
});

describe('Backend events', () => {
  it('shows health alerts as a warning toast and in Session health', async () => {
    const { api } = await renderApp();
    api.emit('health-alert', { subsystem: 'log-analysis', message: '12 errors detected in recent logs' });

    const region = await screen.findByLabelText('Notifications');
    await waitFor(() => expect(region).toHaveTextContent('log-analysis: 12 errors detected in recent logs'));
    expect(region).toHaveAttribute('aria-live', 'polite');
    const alerts = within(screen.getByRole('list', { name: 'Health alerts' }));
    expect(alerts.getByText(/12 errors detected/)).toBeInTheDocument();
    expect(region.querySelector('[role="alert"]')).toBeNull();
  });

  it('lists received files in Activity and saves or discards them', async () => {
    const { api } = await renderApp();
    const save = vi.spyOn(api, 'SaveReceivedFiles');
    const discard = vi.spyOn(api, 'DiscardReceivedFiles');

    api.emit('file-received', { count: 2, names: ['mix-final.wav', 'notes.txt'], tempDir: 'C:\\Temp\\rx-1' });
    const activity = within(screen.getByRole('region', { name: 'Activity' }));
    expect(await activity.findByText(/2 files received from/)).toBeInTheDocument();
    expect(screen.getByLabelText('Notifications')).toHaveTextContent(/Save or discard them in Activity/);

    await fireEvent.click(screen.getByRole('button', { name: 'Save to Downloads' }));
    expect(save).toHaveBeenCalledWith('C:\\Temp\\rx-1');
    expect(await screen.findByText(/Saved to C:\\Users\\you\\Downloads/)).toBeInTheDocument();

    api.emit('file-received', { count: 1, names: ['log.zip'], tempDir: 'C:\\Temp\\rx-2' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Discard 1 received file' }));
    expect(discard).toHaveBeenCalledWith('C:\\Temp\\rx-2');
    expect(await screen.findByText('Discarded')).toBeInTheDocument();
  });

  it('toasts when saving received files fails and keeps them pending', async () => {
    const api = createPreviewApi();
    vi.spyOn(api, 'SaveReceivedFiles').mockRejectedValue(new Error('unknown received-files directory'));
    await renderApp(api);
    api.emit('file-received', { count: 1, names: ['a.txt'], tempDir: 'C:\\Temp\\rx-3' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Save to Downloads' }));
    await waitFor(() =>
      expect(screen.getByLabelText('Notifications')).toHaveTextContent('unknown received-files directory'),
    );
    expect(screen.getByRole('button', { name: 'Save to Downloads' })).toBeEnabled();
  });

  it('applies session events from the backend', async () => {
    const { api } = await renderApp();
    api.emit('session-updated', {
      connected: true,
      controlling: false,
      peerName: 'Studio PC',
      peerID: 'studio-pc',
      role: 'controlled',
      latencyMs: 3,
      audioLatencyMs: 40,
      jitterMs: 1,
    });
    expect(await screen.findByText('Being controlled')).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: 'Studio PC' })).toBeInTheDocument();
  });
});
