import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import App from '../App.svelte';
import OverviewScreen from '../lib/OverviewScreen.svelte';

beforeEach(() => {
  localStorage.clear();
  (window as any).go = undefined;
  (window as any).runtime = undefined;
});

describe('App shell', () => {
  it('reveals the local pairing PIN on demand', async () => {
    render(App);

    expect(await screen.findByRole('button', { name: 'Show pairing PIN' })).toBeInTheDocument();
    expect(screen.queryByText('482911')).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Show pairing PIN' }));

    expect(screen.getByText('First-time pairing PIN')).toBeInTheDocument();
    expect(screen.getByText('482911')).toBeInTheDocument();
  });

  it('switches top-level tabs and settings sub-tabs', async () => {
    render(App);

    expect(await screen.findByRole('heading', { name: 'Travel Laptop' })).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Devices' }));
    expect(screen.getByText('Studio PC')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Settings' }));
    expect(screen.getByRole('heading', { name: 'Mouse & screen edge' })).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Audio' }));
    expect(screen.getByRole('heading', { name: 'Playback' })).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Advanced' }));
    expect(screen.getByRole('heading', { name: 'Identity & network' })).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'General' }));
    expect(screen.getByRole('heading', { name: 'Mouse & screen edge' })).toBeInTheDocument();
  });

  it('filters manual devices and removes them in preview mode', async () => {
    render(App);

    await fireEvent.click(await screen.findByRole('button', { name: 'Devices' }));
    await fireEvent.click(screen.getByRole('button', { name: /Manual 1/i }));

    expect(screen.getByText('Gaming Rig')).toBeInTheDocument();
    expect(screen.queryByText('Studio PC')).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await waitFor(() => {
      expect(screen.queryByText('Gaming Rig')).not.toBeInTheDocument();
    });
  });

  it('pairs a manual device with a PIN and returns to overview', async () => {
    render(App);

    await fireEvent.click(await screen.findByRole('button', { name: 'Devices' }));
    await fireEvent.click(screen.getByRole('button', { name: /Manual 1/i }));
    await fireEvent.click(screen.getByRole('button', { name: 'Pair & Connect' }));

    const dialog = screen.getByRole('dialog', { name: /pair with/i });
    await fireEvent.input(screen.getByLabelText('Pairing PIN'), { target: { value: '482911' } });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Pair & Connect' }));

    expect(await screen.findByText('Controlling Remote Computer')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
  });

  it('untrusts a preview peer and returns it to pair-connect state', async () => {
    render(App);

    await fireEvent.click(await screen.findByRole('button', { name: 'Devices' }));
    expect(screen.getAllByRole('button', { name: 'Untrust' }).length).toBe(2);

    await fireEvent.click(screen.getAllByRole('button', { name: 'Untrust' })[0]);

    await waitFor(() => {
      expect(screen.getAllByRole('button', { name: 'Pair & Connect' }).length).toBe(2);
    });
  });

  it('shows and hides conditional audio controls in the audio section', async () => {
    render(App);

    await fireEvent.click(await screen.findByRole('button', { name: 'Settings' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Audio' }));

    expect(screen.getByText('Play through')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('radio', { name: 'Off' }));
    await waitFor(() => {
      expect(screen.queryByText('Play through')).not.toBeInTheDocument();
    });

    await fireEvent.click(screen.getByLabelText('Share microphone'));
    expect(screen.getByText('Default microphone')).toBeInTheDocument();
    expect(screen.getByText('Only active when mouse is on remote screen')).toBeInTheDocument();
  });

  it('locks edge and monitor layout settings while connected as controller', async () => {
    render(App);

    await fireEvent.click(await screen.findByRole('button', { name: 'Devices' }));
    await fireEvent.click(screen.getAllByRole('button', { name: 'Connect' })[0]);
    await fireEvent.click(screen.getByRole('button', { name: 'Settings' }));

    expect(screen.getByText('Disconnect from the current host to change the edge or monitor layout.')).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Left' })).toBeDisabled();
  });
});

describe('Overview screen', () => {
  it('shows reconnect actions and degraded health detail', async () => {
    render(OverviewScreen, {
      props: {
        sessionStatusText: 'Not Connected',
        sessionStatusSubtext: 'Last connected to Travel Laptop',
        deviceName: 'Desk Main',
        pairingCode: '482911',
        onlinePeerCount: 2,
        activeRouteLabel: 'Tailscale',
        latencyLabel: 'Standby',
        latencyTone: 'idle',
        sessionConnected: false,
        sessionPeerName: '',
        reconnecting: false,
        disconnecting: false,
        healthReconnecting: false,
        healthHealthy: false,
        healthSummaryText: 'tailscale: offline',
        lastPeerName: 'Travel Laptop',
        onReconnect: vi.fn(),
        onDisconnect: vi.fn(),
        onBrowseDevices: vi.fn(),
      },
    });

    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Browse devices' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show pairing PIN' })).toBeInTheDocument();
    expect(screen.getByText('tailscale: offline')).toBeInTheDocument();
    expect(screen.getByText('Last connected to Travel Laptop')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Show pairing PIN' }));

    expect(screen.getByText('482911')).toBeInTheDocument();
  });

  it('surfaces reconnect progress copy', () => {
    render(OverviewScreen, {
      props: {
        sessionStatusText: 'Not Connected',
        sessionStatusSubtext: 'Last connected to Travel Laptop',
        deviceName: 'Desk Main',
        pairingCode: '',
        onlinePeerCount: 0,
        activeRouteLabel: 'Standby',
        latencyLabel: 'Standby',
        latencyTone: 'idle',
        sessionConnected: false,
        sessionPeerName: '',
        reconnecting: true,
        disconnecting: false,
        healthReconnecting: true,
        healthHealthy: true,
        healthSummaryText: '',
        lastPeerName: 'Travel Laptop',
        onReconnect: vi.fn(),
        onDisconnect: vi.fn(),
        onBrowseDevices: vi.fn(),
      },
    });

    expect(screen.getByText('Reconnect in progress.')).toBeInTheDocument();
    expect(screen.getByText('Trying the last known device in the background.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connecting…' })).toBeInTheDocument();
  });

  it('shows empty-state guidance when no device is available yet', () => {
    render(OverviewScreen, {
      props: {
        sessionStatusText: 'Not Connected',
        sessionStatusSubtext: 'Ready to connect',
        deviceName: 'Desk Main',
        onlinePeerCount: 0,
        activeRouteLabel: 'Standby',
        latencyLabel: 'Standby',
        latencyTone: 'idle',
        sessionConnected: false,
        sessionPeerName: '',
        reconnecting: false,
        disconnecting: false,
        healthReconnecting: false,
        healthHealthy: true,
        healthSummaryText: '',
        lastPeerName: '',
        onReconnect: vi.fn(),
        onDisconnect: vi.fn(),
        onBrowseDevices: vi.fn(),
      },
    });

    expect(screen.getByText('Add a trusted device to start switching between systems.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add a device' })).toBeInTheDocument();
  });
});
