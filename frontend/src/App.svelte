<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { emptyHealth, emptySession, emptyTailscale } from './lib/constants';
  import { createPreviewState, errorMessage, getAppApi, getLastPeerAddress, previewLogs, sanitizePairingCode } from './lib/appShell';
  import DevicesScreen from './lib/DevicesScreen.svelte';
  import OverviewScreen from './lib/OverviewScreen.svelte';
  import SettingsScreen from './lib/SettingsScreen.svelte';
  import { formatLatency, healthSummary, normalizeTailscale, preferredRouteLabel } from './lib/utils';
  import type { AudioDevice, DeviceInfo, HealthStatus, LastPeerInfo, MonitorInfo, Peer, Session, TailscaleStatus } from './lib/types';

  type PrimaryTab = 'overview' | 'devices' | 'settings';
  type SettingsSection = 'general' | 'audio' | 'advanced';
  type PeerFilter = 'all' | 'online' | 'trusted' | 'manual';

  interface Toast {
    id: number;
    type: 'error' | 'success' | 'info' | 'warning';
    msg: string;
    leaving?: boolean;
  }

  let activePrimaryTab: PrimaryTab = 'overview';
  let activeSettingsSection: SettingsSection = 'general';
  let peerFilter: PeerFilter = 'all';
  let device: DeviceInfo = { id: '', name: '', fingerprint: '', port: 24831 };
  let peers: Peer[] = [];
  let session: Session = { ...emptySession };
  let tailscale: TailscaleStatus = { ...emptyTailscale };
  let newPeerAddr = '';
  let addError = '';
  let connectingAddress = '';
  let previewMode = false;
  let toasts: Toast[] = [];
  let toastSeq = 0;
  let showLogs = false;
  let logLines: string[] = [];
  let removingAddress = '';
  let forgettingPeerID = '';
  let pairingPeer: Peer | null = null;
  let pairingCodeInput = '';
  let pairingBusy = false;
  let pairingError = '';
  let disconnecting = false;
  let busySetting = '';
  let isBooting = true;
  const reducedMotion = typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
  let pairingInputEl: HTMLInputElement | undefined;
  let edgeSide = 'right';
  let sensitivity = 1;
  let audioMode = 'off';
  let audioProfile = 'balanced';
  let audioTransportMode = 'pcm';
  let audioTiming = 'always';
  let muteSource = false;
  let micMode = 'off';
  let audioDevices: AudioDevice[] = [];
  let captureDeviceID = '';
  let playbackDeviceID = '';
  let micDeviceID = '';
  let micPlaybackDeviceID = '';
  let autostart = false;
  let startMinimized = false;
  let lastPeer: LastPeerInfo | null = null;
  let reconnecting = false;
  let autoReconnect = true;
  let health: HealthStatus = { ...emptyHealth };
  let isDarkMode = true;
  let eventUnsubs: Array<() => void> = [];
  let latencyTone: 'good' | 'ok' | 'idle' = 'idle';

  // Exit hotkey
  let exitHotkeyModifiers = 0;
  let exitHotkeyVKCode = 0;

  // Monitor zones
  let localMonitors: MonitorInfo[] = [];
  let triggerMonitorID = '';
  let triggerSide = '';
  let triggerStartPct = 0;
  let triggerEndPct = 1;
  let returnMonitorID = '';
  let returnXPct = 0.5;
  let returnYPct = 0.5;

  // Pending received files — each transfer is tracked independently to prevent orphaned temp dirs.
  type PendingTransfer = { id: number; count: number; names: string[]; tempDir: string };
  let pendingTransfers: PendingTransfer[] = [];
  let savingTransferID: number | null = null;
  let nextTransferID = 0;

  $: activePeer = peers.find((peer) => peer.id === session.peerID) || null;
  $: onlinePeers = peers.filter((peer) => peer.status === 'online');
  $: trustedPeerCount = peers.filter((peer) => peer.trusted).length;
  $: manualPeerCount = peers.filter((peer) => peer.source === 'manual' || (peer.routes || []).includes('manual')).length;
  $: filteredPeers = peers.filter((peer) => {
    if (peerFilter === 'online') return peer.status === 'online';
    if (peerFilter === 'trusted') return !!peer.trusted;
    if (peerFilter === 'manual') return peer.source === 'manual' || (peer.routes || []).includes('manual');
    return true;
  });
  $: isBeingControlled = session.connected && session.role === 'controlled';
  $: isControllerConnected = session.connected && session.role === 'controller';
  $: isConnected = session.connected;
  $: sessionStatusText = !isConnected ? 'Not Connected' : session.controlling ? 'Controlling Remote Computer' : session.role === 'controlled' ? 'Being Controlled' : 'Connected';
  $: sessionStatusSubtext = !isConnected ? (lastPeer?.name ? `Last connected to ${lastPeer.name}` : 'Ready to connect') : session.controlling ? `You have control of ${session.peerName}` : session.role === 'controlled' ? `${session.peerName} has control` : `Connected to ${session.peerName}`;
  $: activeRouteLabel = activePeer ? preferredRouteLabel(activePeer.preferredRoute) : 'Standby';
  $: latencyTone = session.latencyMs > 0 && session.latencyMs < 10 ? 'good' : session.latencyMs >= 10 && session.latencyMs < 50 ? 'ok' : 'idle';
  $: remoteAudioSummary = audioMode === 'remote' ? 'Listening to remote audio' : audioMode === 'local' ? 'Sending this computer audio' : 'Audio off';
  $: micSummary = micMode === 'send' ? 'Sharing microphone' : micMode === 'receive' ? 'Monitoring remote microphone' : 'Microphone off';
  $: pairingPeerLabel = pairingPeer?.name || pairingPeer?.address || 'this device';

  function toast(msg: string, type: Toast['type'] = 'error', ms = 4500) {
    const id = ++toastSeq;
    toasts = [...toasts, { id, type, msg }];
    if (ms > 0) {
      setTimeout(() => dismissToast(id), ms);
    }
  }

  function dismissToast(id: number) {
    toasts = toasts.map((item) => item.id === id ? { ...item, leaving: true } : item);
    setTimeout(() => {
      toasts = toasts.filter((item) => item.id !== id);
    }, 220);
  }

  async function openPairingDialog(address: string) {
    const peer = peers.find((item) => item.address === address || (item.addresses || []).includes(address));
    if (!peer) {
      toast('Unable to find that device for pairing.');
      return;
    }
    pairingPeer = peer;
    pairingCodeInput = '';
    pairingError = '';
    await tick();
    if (pairingInputEl?.isConnected) pairingInputEl.focus();
  }

  function closePairingDialog() {
    if (pairingBusy) return;
    pairingPeer = null;
    pairingCodeInput = '';
    pairingError = '';
  }

  function toggleTheme() {
    isDarkMode = !isDarkMode;
    document.documentElement.setAttribute('data-theme', isDarkMode ? 'dark' : 'light');
    try {
      localStorage.setItem('theme', isDarkMode ? 'dark' : 'light');
    } catch {}
  }

  function loadPreviewState() {
    const preview = createPreviewState();
    previewMode = preview.previewMode;
    device = preview.device;
    peers = preview.peers;
    session = preview.session;
    tailscale = preview.tailscale;
    edgeSide = preview.edgeSide;
    sensitivity = preview.sensitivity;
    audioMode = preview.audioMode;
    audioProfile = preview.audioProfile;
    audioTransportMode = preview.audioTransportMode;
    audioTiming = preview.audioTiming;
    muteSource = preview.muteSource;
    micMode = preview.micMode;
    audioDevices = preview.audioDevices;
    captureDeviceID = preview.captureDeviceID;
    playbackDeviceID = preview.playbackDeviceID;
    micDeviceID = preview.micDeviceID;
    micPlaybackDeviceID = preview.micPlaybackDeviceID;
    autostart = preview.autostart;
    startMinimized = preview.startMinimized;
    lastPeer = preview.lastPeer;
    reconnecting = preview.reconnecting;
    autoReconnect = preview.autoReconnect;
    health = preview.health;
  }

  if (typeof window !== 'undefined' && !getAppApi()) {
    loadPreviewState();
    isBooting = false;
  }

  onMount(() => {
    try {
      const savedTheme = localStorage.getItem('theme');
      if (savedTheme) {
        isDarkMode = savedTheme === 'dark';
      } else {
        isDarkMode = window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false;
      }
    } catch {
      isDarkMode = false;
    }
    document.documentElement.setAttribute('data-theme', isDarkMode ? 'dark' : 'light');

    if (!getAppApi()) {
      loadPreviewState();
      isBooting = false;
      return;
    }

    refreshAll();

    const runtime = window.runtime;
    const toOff = (value: unknown) => typeof value === 'function' ? value as () => void : () => {};
    if (runtime?.EventsOn) {
      eventUnsubs = [
        toOff(runtime.EventsOn('device-updated', (data) => { device = data || device; })),
        toOff(runtime.EventsOn('peers-updated', (data) => { peers = data || []; })),
        toOff(runtime.EventsOn('session-updated', async (data) => {
          const prev = session;
          session = data || { ...emptySession };
          if (prev.connected !== session.connected) {
            connectingAddress = '';
            disconnecting = false;
            if (prev.connected && !session.connected) {
              try {
                const lp = await getAppApi()?.GetLastPeer();
                lastPeer = (lp as LastPeerInfo | null) || null;
              } catch {}
            }
          }
        })),
        toOff(runtime.EventsOn('tailscale-updated', (data) => {
          tailscale = normalizeTailscale(data);
        })),
        toOff(runtime.EventsOn('health-updated', (data) => {
          health = data || { ...emptyHealth };
        })),
        toOff(runtime.EventsOn('file-received', (data) => {
          if (data?.count > 0) {
            pendingTransfers = [...pendingTransfers, {
              id: nextTransferID++,
              count: data.count,
              names: data.names || [],
              tempDir: data.tempDir || '',
            }];
          }
        })),
      ];
    }
  });

  onDestroy(() => {
    for (const off of eventUnsubs) {
      try {
        off();
      } catch {}
    }
    eventUnsubs = [];
  });

  async function refreshAll() {
    const appApi = getAppApi();
    if (!appApi) {
      loadPreviewState();
      isBooting = false;
      return;
    }

    try {
      previewMode = false;
      const [
        nextDevice,
        nextPeers,
        nextSession,
        nextTailscale,
        nextEdgeSide,
        nextSensitivity,
        nextAudioMode,
        nextAudioProfile,
        nextAudioTransportMode,
        nextAudioTiming,
        nextMuteSource,
        nextAutostart,
        nextStartMinimized,
        nextLastPeer,
        nextHealth,
        nextAutoReconnect,
        nextMicMode,
        nextAudioDevices,
        nextCaptureDeviceID,
        nextPlaybackDeviceID,
        nextMicDeviceID,
        nextMicPlaybackDeviceID,
      ] = await Promise.all([
        appApi.GetDevice().catch(() => null),
        appApi.GetPeers().catch(() => null),
        appApi.GetSession().catch(() => null),
        appApi.GetTailscaleStatus().catch(() => null),
        appApi.GetEdgeSide().catch(() => null),
        appApi.GetSensitivity().catch(() => null),
        appApi.GetAudioMode().catch(() => null),
        appApi.GetAudioProfile().catch(() => null),
        appApi.GetAudioTransport().catch(() => null),
        appApi.GetAudioTiming().catch(() => null),
        appApi.GetMuteSource().catch(() => null),
        appApi.GetAutostart().catch(() => null),
        appApi.GetStartMinimized().catch(() => null),
        appApi.GetLastPeer().catch(() => null),
        appApi.GetHealthStatus().catch(() => null),
        appApi.GetAutoReconnect().catch(() => null),
        appApi.GetMicMode().catch(() => null),
        appApi.GetAudioDevices().catch(() => null),
        appApi.GetCaptureDeviceID().catch(() => null),
        appApi.GetPlaybackDeviceID().catch(() => null),
        appApi.GetMicDeviceID().catch(() => null),
        appApi.GetMicPlaybackDeviceID().catch(() => null),
      ]);
      const [nextExitHotkey, nextMonitors, nextTriggerZone, nextReturnAnchor] = await Promise.all([
        appApi.GetExitHotkey().catch(() => null),
        appApi.GetLocalMonitors().catch(() => []),
        appApi.GetTriggerZone().catch(() => null),
        appApi.GetReturnAnchor().catch(() => null),
      ]);

      device = nextDevice || device;
      peers = nextPeers || [];
      session = nextSession || { ...emptySession };
      tailscale = normalizeTailscale(nextTailscale);
      edgeSide = nextEdgeSide || 'right';
      sensitivity = typeof nextSensitivity === 'number' ? nextSensitivity : 1;
      audioMode = nextAudioMode || 'off';
      audioProfile = nextAudioProfile || 'balanced';
      audioTransportMode = nextAudioTransportMode || 'pcm';
      audioTiming = nextAudioTiming || 'always';
      muteSource = !!nextMuteSource;
      micMode = nextMicMode || 'off';
      audioDevices = nextAudioDevices || [];
      captureDeviceID = nextCaptureDeviceID || '';
      playbackDeviceID = nextPlaybackDeviceID || '';
      micDeviceID = nextMicDeviceID || '';
      micPlaybackDeviceID = nextMicPlaybackDeviceID || '';
      autostart = !!nextAutostart;
      startMinimized = !!nextStartMinimized;
      lastPeer = (nextLastPeer as LastPeerInfo | null) || null;
      health = nextHealth || { ...emptyHealth };
      autoReconnect = nextAutoReconnect !== false;
      if (nextExitHotkey) {
        exitHotkeyModifiers = nextExitHotkey.modifiers || 0;
        exitHotkeyVKCode = nextExitHotkey.vkCode || 0;
      }
      localMonitors = nextMonitors || [];
      if (nextTriggerZone?.monitorID) {
        triggerMonitorID = nextTriggerZone.monitorID;
        triggerSide = nextTriggerZone.side || 'right';
        triggerStartPct = nextTriggerZone.startPct ?? 0;
        // Mirror backend startup: treat endPct=0 as invalid, normalize to 1
        triggerEndPct = (nextTriggerZone.endPct > 0) ? nextTriggerZone.endPct : 1;
      } else {
        triggerMonitorID = '';
        triggerSide = '';
        triggerStartPct = 0;
        triggerEndPct = 1;
      }
      if (nextReturnAnchor?.monitorID) {
        returnMonitorID = nextReturnAnchor.monitorID;
        returnXPct = nextReturnAnchor.xPct ?? 0.5;
        returnYPct = nextReturnAnchor.yPct ?? 0.5;
      } else {
        returnMonitorID = '';
        returnXPct = 0.5;
        returnYPct = 0.5;
      }
    } catch (error) {
      toast(errorMessage(error, 'Unable to load state'));
    } finally {
      isBooting = false;
    }
  }

  async function addPeer() {
    const addr = newPeerAddr.trim();
    if (!addr) return;

    addError = '';
    const appApi = getAppApi();
    if (!appApi) {
      if (peers.some((peer) => peer.address === addr)) {
        addError = 'That address is already in the preview list.';
        return;
      }
      peers = [
        ...peers,
        {
          id: `manual-${addr.replace(/[^a-z0-9]+/gi, '-').toLowerCase()}`,
          name: addr.replace(/:\d+$/, ''),
          address: addr,
          addresses: [addr],
          fingerprint: '',
          port: 24831,
          source: 'manual',
          status: 'offline',
          trusted: false,
          routes: ['manual'],
          preferredRoute: 'manual',
          lastSeen: Math.floor(Date.now() / 1000),
        },
      ];
      newPeerAddr = '';
      toast('Added preview endpoint', 'success', 2400);
      return;
    }

    try {
      await appApi.AddPeer(addr);
      peers = await appApi.GetPeers() || [];
      newPeerAddr = '';
    } catch (error) {
      addError = errorMessage(error, 'Failed to add peer');
    }
  }

  async function removePeer(address: string) {
    removingAddress = address;
    const appApi = getAppApi();
    if (!appApi) {
      peers = peers.filter((peer) => peer.address !== address);
      if (lastPeer && getLastPeerAddress(lastPeer) === address) {
        lastPeer = null;
      }
      removingAddress = '';
      toast('Removed preview endpoint', 'info', 2200);
      return;
    }

    try {
      await appApi.RemovePeer(address);
      peers = await appApi.GetPeers() || [];
    } catch (error) {
      toast(errorMessage(error, 'Failed to remove peer'));
    } finally {
      removingAddress = '';
    }
  }

  async function forgetPeerTrust(peer: Peer) {
    forgettingPeerID = peer.id;
    const appApi = getAppApi();
    if (!appApi) {
      peers = peers.map((item) => item.id === peer.id ? { ...item, trusted: false } : item);
      if (lastPeer?.id === peer.id) {
        lastPeer = null;
      }
      forgettingPeerID = '';
      toast(`Removed trust for ${peer.name || peer.address}`, 'info', 2600);
      return;
    }

    try {
      await appApi.UntrustPeer(peer.id);
      peers = await appApi.GetPeers() || [];
      if (lastPeer?.id === peer.id) {
        lastPeer = null;
      }
      toast(`Removed trust for ${peer.name || peer.address}. Use "Pair & Connect" to pair again.`, 'info', 3600);
    } catch (error) {
      toast(errorMessage(error, 'Failed to remove trust for peer'));
    } finally {
      forgettingPeerID = '';
    }
  }

  async function pairAndConnect() {
    if (!pairingPeer) return;

    const address = pairingPeer.address;
    const peerName = pairingPeer.name || address;
    pairingCodeInput = sanitizePairingCode(pairingCodeInput);
    if (pairingCodeInput.length !== 6) {
      toast('Enter the 6-digit PIN shown on the other machine.');
      return;
    }

    pairingBusy = true;
    connectingAddress = address;
    const appApi = getAppApi();
    if (!appApi) {
      peers = peers.map((item) => item.id === pairingPeer.id ? { ...item, trusted: true, status: 'online', lastSeen: Math.floor(Date.now() / 1000) } : item);
      session = { connected: true, controlling: true, peerName, peerID: pairingPeer.id, role: 'controller', latencyMs: pairingPeer.preferredRoute === 'lan' ? 4 : 19, audioLatencyMs: pairingPeer.preferredRoute === 'lan' ? 49 : 55, jitterMs: 2 };
      lastPeer = { id: pairingPeer.id, name: pairingPeer.name, address, [pairingPeer.preferredRoute || 'manual']: address };
      activePrimaryTab = 'overview';
      pairingBusy = false;
      connectingAddress = '';
      closePairingDialog();
      toast(`Paired with ${peerName}`, 'success');
      return;
    }

    try {
      await appApi.TrustPeer(address, pairingCodeInput);
      peers = await appApi.GetPeers() || peers;
      activePrimaryTab = 'overview';
      pairingBusy = false;
      closePairingDialog();
      toast(`Paired with ${peerName}`, 'success');
    } catch (error) {
      pairingError = errorMessage(error, 'Pairing failed');
      toast(pairingError);
    } finally {
      pairingBusy = false;
      connectingAddress = '';
    }
  }

  async function connectToPeer(address: string, needsTrust = false) {
    if (needsTrust) {
      openPairingDialog(address);
      return;
    }

    connectingAddress = address;
    const appApi = getAppApi();
    if (!appApi) {
      const peer = peers.find((item) => item.address === address || (item.addresses || []).includes(address));
      if (!peer) {
        toast('Preview peer not found');
        connectingAddress = '';
        return;
      }
      peers = peers.map((item) => item.id === peer.id ? { ...item, trusted: needsTrust ? true : item.trusted, status: 'online', lastSeen: Math.floor(Date.now() / 1000) } : item);
      session = { connected: true, controlling: true, peerName: peer.name || address, peerID: peer.id, role: 'controller', latencyMs: peer.preferredRoute === 'lan' ? 4 : 19, audioLatencyMs: peer.preferredRoute === 'lan' ? 49 : 55, jitterMs: 2 };
      lastPeer = { id: peer.id, name: peer.name, address, [peer.preferredRoute || 'manual']: address };
      activePrimaryTab = 'overview';
      toast(`Connected to ${peer.name || address}`, 'success');
      connectingAddress = '';
      return;
    }

    try {
      await appApi.Connect(address);
      peers = await appApi.GetPeers() || peers;
      activePrimaryTab = 'overview';
      const peerName = peers.find((peer) => peer.address === address)?.name || address;
      toast(`Connected to ${peerName}`, 'success');
    } catch (error) {
      const message = errorMessage(error, 'Connection failed');
      toast(message.toLowerCase().includes('not trusted') ? 'This peer is not paired yet. Use "Pair & Connect" first.' : message);
    } finally {
      connectingAddress = '';
    }
  }

  async function disconnect() {
    disconnecting = true;
    const appApi = getAppApi();
    if (!appApi) {
      const active = peers.find((peer) => peer.id === session.peerID);
      if (active?.address) {
        lastPeer = { id: active.id, name: active.name, address: active.address, [active.preferredRoute || 'manual']: active.address };
      }
      session = { ...emptySession };
      disconnecting = false;
      toast('Disconnected', 'info');
      return;
    }

    try {
      await appApi.Disconnect();
      connectingAddress = '';
      toast('Disconnected', 'info');
    } catch (error) {
      toast(errorMessage(error, 'Failed to disconnect session'));
    } finally {
      disconnecting = false;
    }
  }

  async function reconnect() {
    reconnecting = true;
    const appApi = getAppApi();
    if (!appApi) {
      const address = getLastPeerAddress(lastPeer);
      if (!address) {
        reconnecting = false;
        toast('No preview peer available to reconnect.');
        return;
      }
      const peer = peers.find((item) => item.address === address || (item.addresses || []).includes(address));
      await connectToPeer(address, !!peer && !peer.trusted);
      reconnecting = false;
      return;
    }

    try {
      await appApi.Reconnect();
      activePrimaryTab = 'overview';
    } catch (error) {
      const message = errorMessage(error, 'Reconnection failed');
      toast(message.toLowerCase().includes('not trusted') ? 'Saved peer is no longer trusted. Reconnect from Devices with "Pair & Connect".' : message);
    } finally {
      reconnecting = false;
    }
  }

  async function sendFiles() {
    const appApi = getAppApi();
    if (!appApi) {
      toast('Not available in preview mode');
      return;
    }
    try {
      await appApi.PickAndSendFiles();
      toast('Sending files', 'success', 2500);
    } catch (error) {
      const msg = errorMessage(error, 'Failed to send files');
      if (!msg.includes('not connected')) {
        toast(msg);
      }
    }
  }

  const settingInflight = new Set<string>();

  async function runSettingUpdate(token: string, action: () => Promise<void>) {
    if (settingInflight.has(token)) return; // drop concurrent same-key calls
    settingInflight.add(token);
    busySetting = token;
    try {
      await action();
    } catch (error) {
      toast(errorMessage(error, 'Failed to update setting'));
      await refreshAll();
    } finally {
      settingInflight.delete(token);
      if (busySetting === token) busySetting = '';
    }
  }

  async function updateEdgeSide(side: string) {
    if (!getAppApi()) { edgeSide = side; return; }
    await runSettingUpdate('edge-side', async () => {
      await getAppApi()?.SetEdgeSide(side);
      edgeSide = side;
    });
  }

  async function updateExitHotkey(modifiers: number, vkCode: number) {
    exitHotkeyModifiers = modifiers;
    exitHotkeyVKCode = vkCode;
    if (!getAppApi()) return;
    await runSettingUpdate('exit-hotkey', async () => {
      await getAppApi()?.SetExitHotkey(modifiers, vkCode);
    });
  }

  async function updateTriggerZone(monitorID: string, side: string, startPct: number, endPct: number) {
    triggerMonitorID = monitorID;
    triggerSide = side;
    triggerStartPct = startPct;
    triggerEndPct = endPct;
    if (!getAppApi()) return;
    await runSettingUpdate('trigger-zone', async () => {
      if (monitorID) {
        await getAppApi()?.SetTriggerZone(monitorID, side, startPct, endPct);
      } else {
        await getAppApi()?.ClearTriggerZone();
      }
    });
  }

  async function updateReturnAnchor(monitorID: string, xPct: number, yPct: number) {
    returnMonitorID = monitorID;
    returnXPct = xPct;
    returnYPct = yPct;
    if (!getAppApi()) return;
    await runSettingUpdate('return-anchor', async () => {
      if (monitorID) {
        await getAppApi()?.SetReturnAnchor(monitorID, xPct, yPct);
      } else {
        await getAppApi()?.ClearReturnAnchor();
      }
    });
  }

  async function saveReceivedFiles(transfer: { id: number; count: number; names: string[]; tempDir: string }) {
    if (!transfer.tempDir || savingTransferID !== null || !getAppApi()) return;
    savingTransferID = transfer.id;
    try {
      const result = await getAppApi()?.SaveReceivedFiles(transfer.tempDir);
      if (result?.dest) {
        pendingTransfers = pendingTransfers.filter(t => t.id !== transfer.id);
        toast(`${result.saved?.length || 0} file(s) saved to ${result.dest}`, 'success', 5000);
      }
    } catch (err) {
      toast(errorMessage(err, 'Failed to save files'));
    } finally {
      savingTransferID = null;
    }
  }

  async function updateSensitivity(value: string) {
    const parsed = parseFloat(value);
    if (Number.isNaN(parsed)) return;
    sensitivity = parsed;
    if (!getAppApi()) return;
    await runSettingUpdate('sensitivity', async () => { await getAppApi()?.SetSensitivity(parsed); });
  }

  async function updateAudioMode(mode: string) {
    if (!getAppApi()) { audioMode = mode; return; }
    await runSettingUpdate('audio-mode', async () => {
      await getAppApi()?.SetAudioMode(mode);
      audioMode = mode;
    });
  }

  async function updateAudioProfile(profile: string) {
    if (!getAppApi()) { audioProfile = profile; return; }
    await runSettingUpdate('audio-profile', async () => {
      await getAppApi()?.SetAudioProfile(profile);
      audioProfile = profile;
    });
  }

  async function updateAudioTransportMode(mode: string) {
    if (!getAppApi()) { audioTransportMode = mode; return; }
    await runSettingUpdate('audio-transport', async () => {
      await getAppApi()?.SetAudioTransport(mode);
      audioTransportMode = mode;
    });
  }

  async function updateAudioTiming(timing: string) {
    if (!getAppApi()) { audioTiming = timing; return; }
    await runSettingUpdate('audio-timing', async () => {
      await getAppApi()?.SetAudioTiming(timing);
      audioTiming = timing;
    });
  }

  async function updateMuteSource(enabled: boolean) {
    if (!getAppApi()) { muteSource = enabled; return; }
    await runSettingUpdate('mute-source', async () => {
      await getAppApi()?.SetMuteSource(enabled);
      muteSource = enabled;
    });
  }

  async function updateMicMode(mode: string) {
    if (!getAppApi()) { micMode = mode; return; }
    await runSettingUpdate('mic-mode', async () => {
      await getAppApi()?.SetMicMode(mode);
      micMode = mode;
    });
  }

  async function updateCaptureDevice(id: string) {
    if (!getAppApi()) { captureDeviceID = id; return; }
    await runSettingUpdate('capture-device', async () => {
      await getAppApi()?.SetCaptureDeviceID(id);
      captureDeviceID = id;
    });
  }

  async function updatePlaybackDevice(id: string) {
    if (!getAppApi()) { playbackDeviceID = id; return; }
    await runSettingUpdate('playback-device', async () => {
      await getAppApi()?.SetPlaybackDeviceID(id);
      playbackDeviceID = id;
    });
  }

  async function updateMicDevice(id: string) {
    if (!getAppApi()) { micDeviceID = id; return; }
    await runSettingUpdate('mic-device', async () => {
      await getAppApi()?.SetMicDeviceID(id);
      micDeviceID = id;
    });
  }

  async function updateMicPlaybackDevice(id: string) {
    if (!getAppApi()) { micPlaybackDeviceID = id; return; }
    await runSettingUpdate('mic-playback-device', async () => {
      await getAppApi()?.SetMicPlaybackDeviceID(id);
      micPlaybackDeviceID = id;
    });
  }

  async function updateAutostart(enabled: boolean) {
    if (!getAppApi()) { autostart = enabled; return; }
    await runSettingUpdate('autostart', async () => {
      await getAppApi()?.SetAutostart(enabled);
      autostart = enabled;
    });
  }

  async function updateStartMinimized(enabled: boolean) {
    if (!getAppApi()) { startMinimized = enabled; return; }
    await runSettingUpdate('start-minimized', async () => {
      await getAppApi()?.SetStartMinimized(enabled);
      startMinimized = enabled;
    });
  }

  async function updateAutoReconnect(enabled: boolean) {
    if (!getAppApi()) { autoReconnect = enabled; return; }
    await runSettingUpdate('auto-reconnect', async () => {
      await getAppApi()?.SetAutoReconnect(enabled);
      autoReconnect = enabled;
    });
  }

  async function loadLogs() {
    const appApi = getAppApi();
    if (!appApi) {
      logLines = [...previewLogs];
      return;
    }
    try {
      logLines = await appApi.GetRecentLogs() || [];
    } catch {
      logLines = [];
    }
  }

  async function toggleLogs() {
    showLogs = !showLogs;
    if (showLogs) {
      await loadLogs();
    }
  }

  async function refreshAudioDevices() {
    const appApi = getAppApi();
    if (!appApi) return;
    try {
      audioDevices = await appApi.GetAudioDevices() || [];
    } catch {}
  }
</script>

<div class="app-layout">
  <header class="top-nav" style="--wails-draggable: drag;">
    <div class="brand" style="--wails-draggable: no-drag">
      <div class="brand-icon">
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
          <path d="M15 10l-4 4l6 6l4 -16l-18 7l4 2l2 6l3 -4" />
        </svg>
      </div>
      <span class="brand-label">MultiSnek</span>
    </div>

    <nav class="nav-links" style="--wails-draggable: no-drag" aria-label="Primary navigation">
      <button type="button" class="nav-item" class:active={activePrimaryTab === 'overview'} on:click={() => activePrimaryTab = 'overview'} aria-pressed={activePrimaryTab === 'overview'}>
        Overview
      </button>
      <button type="button" class="nav-item" class:active={activePrimaryTab === 'devices'} on:click={() => activePrimaryTab = 'devices'} aria-pressed={activePrimaryTab === 'devices'}>
        Devices
      </button>
      <button type="button" class="nav-item" class:active={activePrimaryTab === 'settings'} on:click={() => activePrimaryTab = 'settings'} aria-pressed={activePrimaryTab === 'settings'}>
        Settings
      </button>
    </nav>

    <div class="nav-end" style="--wails-draggable: no-drag">
      <div class="status-dot" class:live={session.connected} class:preview={previewMode} title={previewMode ? 'Preview' : session.connected ? 'Connected' : 'Ready'}></div>
    </div>
  </header>

  <main class="main-content" class:is-overview={activePrimaryTab === 'overview'}>
    {#if isBooting}
      <div class="boot-overlay" aria-busy="true" aria-label="Loading application">
        <span class="boot-spinner" aria-hidden="true"></span>
      </div>
    {:else}
      {#if activePrimaryTab === 'overview'}
        <div class="overview-fill" in:fly={{ y: 10, duration: reducedMotion ? 0 : 200, delay: reducedMotion ? 0 : 40 }} out:fade={{ duration: reducedMotion ? 0 : 100 }}>
          <OverviewScreen
            sessionStatusText={sessionStatusText}
            sessionStatusSubtext={sessionStatusSubtext}
            deviceName={device.name}
            pairingCode={device.pairingCode || ''}
            onlinePeerCount={onlinePeers.length}
            activeRouteLabel={activeRouteLabel}
            latencyLabel={isConnected ? formatLatency(session.latencyMs) : 'Standby'}
            latencyTone={latencyTone}
            sessionConnected={session.connected}
            sessionPeerName={session.peerName}
            latencyMs={session.latencyMs}
            audioLatencyMs={session.audioLatencyMs}
            jitterMs={session.jitterMs}
            reconnecting={reconnecting}
            disconnecting={disconnecting}
            healthReconnecting={health.reconnecting}
            healthHealthy={health.healthy}
            healthSummaryText={healthSummary(health)}
            lastPeerName={lastPeer?.name || ''}
            onReconnect={reconnect}
            onDisconnect={disconnect}
            onSendFiles={sendFiles}
            onBrowseDevices={() => activePrimaryTab = 'devices'}
          />
        </div>
      {/if}

      {#if activePrimaryTab === 'devices'}
        <div class="content-wrapper" in:fly={{ y: 10, duration: reducedMotion ? 0 : 200, delay: reducedMotion ? 0 : 40 }} out:fade={{ duration: reducedMotion ? 0 : 100 }}>
          <DevicesScreen
            bind:peerFilter
            bind:newPeerAddr
            {addError}
            {peers}
            {filteredPeers}
            onlinePeerCount={onlinePeers.length}
            {trustedPeerCount}
            {manualPeerCount}
            sessionConnected={session.connected}
            sessionPeerID={session.peerID}
            {connectingAddress}
            {removingAddress}
            {forgettingPeerID}
            onAdd={addPeer}
            onConnect={connectToPeer}
            onDisconnect={disconnect}
            onRemove={removePeer}
            onForgetTrust={forgetPeerTrust}
          />
        </div>
      {/if}

      {#if activePrimaryTab === 'settings'}
        <div class="content-wrapper" in:fly={{ y: 10, duration: reducedMotion ? 0 : 200, delay: reducedMotion ? 0 : 40 }} out:fade={{ duration: reducedMotion ? 0 : 100 }}>
          <SettingsScreen
            bind:activeSettingsSection
            bind:showLogs
            {busySetting}
            {edgeSide}
            {sensitivity}
            {audioMode}
            {audioProfile}
            {audioTransportMode}
            {audioTiming}
            {muteSource}
            {micMode}
            {audioDevices}
            {captureDeviceID}
            {playbackDeviceID}
            {micDeviceID}
            {micPlaybackDeviceID}
            {autostart}
            {startMinimized}
            {autoReconnect}
            {isBeingControlled}
            {isControllerConnected}
            {tailscale}
            {device}
            {logLines}
            onUpdateEdgeSide={updateEdgeSide}
            onUpdateSensitivity={updateSensitivity}
            onUpdateAudioMode={updateAudioMode}
            onUpdateAudioProfile={updateAudioProfile}
            onUpdateAudioTransportMode={updateAudioTransportMode}
            onUpdateAudioTiming={updateAudioTiming}
            onUpdateMuteSource={updateMuteSource}
            onUpdateMicMode={updateMicMode}
            onUpdateCaptureDevice={updateCaptureDevice}
            onUpdatePlaybackDevice={updatePlaybackDevice}
            onUpdateMicDevice={updateMicDevice}
            onUpdateMicPlaybackDevice={updateMicPlaybackDevice}
            onUpdateAutostart={updateAutostart}
            onUpdateStartMinimized={updateStartMinimized}
            onUpdateAutoReconnect={updateAutoReconnect}
            onToggleLogs={toggleLogs}
            onRefreshAudioDevices={refreshAudioDevices}
            {exitHotkeyModifiers}
            {exitHotkeyVKCode}
            {localMonitors}
            {triggerMonitorID}
            {triggerSide}
            {triggerStartPct}
            {triggerEndPct}
            {returnMonitorID}
            {returnXPct}
            {returnYPct}
            onUpdateExitHotkey={updateExitHotkey}
            onUpdateTriggerZone={updateTriggerZone}
            onUpdateReturnAnchor={updateReturnAnchor}
          />
        </div>
      {/if}
    {/if}
  </main>

  {#if pairingPeer}
    <button class="modal-backdrop" type="button" aria-label="Close pairing dialog" on:click={closePairingDialog}></button>
    <section class="pairing-modal" role="dialog" aria-modal="true" aria-labelledby="pairing-title">
      <div class="pairing-modal-card">
        <span class="eyebrow">First-time pairing</span>
        <h2 id="pairing-title">Pair with {pairingPeerLabel}</h2>
        <p>Enter the 6-digit PIN shown on that machine to trust it and connect.</p>
        <label class="pairing-field" for="pairing-pin-input">
          <span>Pairing PIN</span>
          <input
            id="pairing-pin-input"
            class="input pairing-input mono"
            bind:this={pairingInputEl}
            type="text"
            inputmode="numeric"
            placeholder="000000"
            maxlength="6"
            value={pairingCodeInput}
            on:input={(event) => { pairingCodeInput = sanitizePairingCode(event.currentTarget.value); pairingError = ''; }}
          />
        </label>
        {#if pairingError}
          <p class="pairing-error" role="alert">{pairingError}</p>
        {/if}
        <div class="pairing-actions">
          <button class="btn btn-ghost" type="button" on:click={closePairingDialog} disabled={pairingBusy}>Cancel</button>
          <button class="btn btn-primary" type="button" on:click={pairAndConnect} disabled={pairingBusy || pairingCodeInput.length !== 6}>
            {#if pairingBusy}<span class="btn-spinner"></span>{/if}
            {pairingBusy ? 'Pairing…' : 'Pair & Connect'}
          </button>
        </div>
      </div>
    </section>
  {/if}

  <div class="toast-container" aria-live="polite">
    {#each pendingTransfers as transfer (transfer.id)}
      {@const isSaving = savingTransferID === transfer.id}
      <div class="toast toast-info file-toast" in:fly={{ x: 60, duration: 250 }} out:fly={{ x: 60, duration: 200 }} role="status">
        <span class="toast-icon">📁</span>
        <span class="toast-body">
          {transfer.count} file{transfer.count !== 1 ? 's' : ''} received
          {#if transfer.names.length > 0}
            {@const truncated = transfer.names.slice(0, 3).map(n => n.length > 24 ? n.slice(0, 22) + '…' : n)}
            <span class="file-names">({truncated.join(', ')}{transfer.names.length > 3 ? ', …' : ''})</span>
          {/if}
          — Ctrl+V to paste, or
        </span>
        <button type="button" class="toast-action-btn" on:click={() => saveReceivedFiles(transfer)} disabled={savingTransferID !== null}>{isSaving ? 'Saving…' : 'Save to folder…'}</button>
        <button type="button" class="toast-dismiss" on:click={() => {
          pendingTransfers = pendingTransfers.filter(t => t.id !== transfer.id);
          if (transfer.tempDir) getAppApi()?.DiscardReceivedFiles(transfer.tempDir).catch(() => {});
        }} aria-label="Dismiss" disabled={isSaving}>✕</button>
      </div>
    {/each}
    {#each toasts as t (t.id)}
      <div class="toast toast-{t.type}" class:leaving={t.leaving} in:fly={{ x: 60, duration: 250 }} role="alert">
        <span class="toast-icon">
          {#if t.type === 'error'}⚠{:else if t.type === 'success'}✓{:else if t.type === 'warning'}⚠{:else}ℹ{/if}
        </span>
        <span class="toast-body">{t.msg}</span>
        <button type="button" class="toast-dismiss" on:click={() => dismissToast(t.id)} aria-label="Dismiss">✕</button>
      </div>
    {/each}
  </div>
</div>

<style>
  .app-layout {
    display: grid;
    grid-template-rows: auto 1fr;
    height: 100vh;
    overflow: hidden;
  }

  .top-nav {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    gap: 1rem;
    padding: 0.7rem 1.25rem;
    border-bottom: 1px solid var(--border);
    background: var(--nav-bg);
    backdrop-filter: blur(18px);
    z-index: 20;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    justify-self: start;
  }

  .brand-icon {
    width: 1.75rem;
    height: 1.75rem;
    display: grid;
    place-items: center;
    border-radius: 0.55rem;
    background: linear-gradient(135deg, var(--accent), var(--accent-strong));
    color: white;
    flex-shrink: 0;
  }

  .brand-label {
    font-weight: 700;
    font-size: 0.95rem;
    color: var(--text-strong);
    letter-spacing: -0.02em;
  }

  .nav-links {
    display: flex;
    gap: 0.25rem;
  }

  .nav-item {
    border-radius: 999px;
    border: 1px solid transparent;
    background: transparent;
    color: var(--text-secondary);
    padding: 0.45rem 0.85rem;
    font-weight: 600;
    font-size: 0.88rem;
    white-space: nowrap;
    transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
  }

  .nav-item.active {
    background: var(--panel);
    border-color: var(--border);
    color: var(--text-strong);
  }

  .nav-end {
    justify-self: end;
    display: flex;
    align-items: center;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-muted);
    transition: background 0.3s, box-shadow 0.3s;
  }

  .status-dot.live {
    background: var(--success);
    box-shadow: 0 0 0 3px var(--success-bg);
  }

  .status-dot.preview { background: var(--accent); }

  .main-content {
    overflow-y: auto;
    min-height: 0;
    position: relative;
  }

  .main-content.is-overview {
    overflow: hidden;
  }

  .overview-fill {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .content-wrapper {
    width: min(1120px, 100%);
    margin: 0 auto;
    padding: 1.5rem 1.25rem 2rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(5, 10, 18, 0.58);
  backdrop-filter: blur(6px);
  z-index: 40;
  }

  .pairing-modal {
  position: fixed;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 1.5rem;
  z-index: 41;
  }

  .pairing-modal-card {
  width: min(100%, 28rem);
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 1.35rem;
  border-radius: 1.15rem;
  border: 1px solid var(--accent-border);
  background: var(--panel);
  box-shadow: 0 28px 80px rgba(0, 0, 0, 0.28);
  }

  .pairing-modal-card h2 {
  margin: 0;
  font-size: 1.4rem;
  letter-spacing: -0.03em;
  color: var(--text-strong);
  }

  .pairing-modal-card p {
  margin: 0;
  color: var(--text-secondary);
  line-height: 1.55;
  }

  .pairing-field {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  color: var(--text-secondary);
  font-size: 0.88rem;
  font-weight: 600;
  }

  .pairing-input {
  text-align: center;
  letter-spacing: 0.35em;
  padding-left: 0.35em;
  font-size: 1.35rem;
  }

  .pairing-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  }

  .pairing-error {
  margin: 0;
  padding: 0.5rem 0.65rem;
  border-radius: 0.5rem;
  background: rgba(224, 82, 82, 0.12);
  color: var(--danger, #e05252);
  font-size: 0.85rem;
  line-height: 1.45;
  }

  .boot-overlay {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
  }

  .boot-spinner {
    width: 2rem;
    height: 2rem;
    border-radius: 50%;
    border: 2px solid var(--accent-border);
    border-top-color: var(--accent);
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  @media (prefers-reduced-motion: reduce) {
    .boot-spinner {
      animation: none;
      opacity: 0.7;
    }
  }
</style>
