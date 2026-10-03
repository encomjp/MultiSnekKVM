// Device identity, peers, the live session and network context (Tailscale,
// local adapters), plus the actions that change them.

import { emptyDevice, emptySession, emptyTailscale } from '../api/normalize';
import type {
  Api,
  ConnectionInterface,
  DeviceInfo,
  LastPeer,
  Peer,
  SessionStatus,
  TailscaleStatus,
} from '../api/types';
import { errorMessage, isPeerOnline, orderedRoutes, routeLabel, shortFingerprint } from '../utils';
import type { ActivityStore } from './activity.svelte';
import type { ToastStore } from './toasts.svelte';

export type PeerBusy = 'untrust' | 'remove';

export class ConnectionStore {
  device = $state<DeviceInfo>({ ...emptyDevice });
  peers = $state<Peer[]>([]);
  session = $state<SessionStatus>({ ...emptySession });
  lastPeer = $state<LastPeer | null>(null);
  tailscale = $state<TailscaleStatus>({ ...emptyTailscale });
  interfaces = $state<ConnectionInterface[]>([]);
  interfacesLoading = $state(false);

  /** Per-peer route the user picked (peer id → address). */
  routeChoice = $state<Record<string, string>>({});
  /** Address currently being connected to, if any. */
  connecting = $state('');
  disconnecting = $state(false);
  reconnecting = $state(false);
  busyPeers = $state<Record<string, PeerBusy>>({});

  activePeer = $derived(
    this.session.connected ? (this.peers.find((peer) => peer.id === this.session.peerID) ?? null) : null,
  );
  onlineCount = $derived(this.peers.filter(isPeerOnline).length);
  isController = $derived(this.session.connected && this.session.role === 'controller');
  isControlled = $derived(this.session.connected && this.session.role === 'controlled');

  readonly #api: Api;
  readonly #toasts: ToastStore;
  readonly #activity: ActivityStore;

  constructor(api: Api, toasts: ToastStore, activity: ActivityStore) {
    this.#api = api;
    this.#toasts = toasts;
    this.#activity = activity;
  }

  async load(): Promise<void> {
    const [device, peers, session, tailscale, lastPeer] = await Promise.allSettled([
      this.#api.GetDevice(),
      this.#api.GetPeers(),
      this.#api.GetSession(),
      this.#api.GetTailscaleStatus(),
      this.#api.GetLastPeer(),
    ]);
    if (device.status === 'fulfilled') this.device = device.value;
    if (peers.status === 'fulfilled') this.peers = peers.value;
    if (session.status === 'fulfilled') this.session = session.value;
    if (tailscale.status === 'fulfilled') this.tailscale = tailscale.value;
    if (lastPeer.status === 'fulfilled') this.lastPeer = lastPeer.value;
    void this.refreshInterfaces();
  }

  async refreshPeers(): Promise<void> {
    try {
      this.peers = await this.#api.GetPeers();
    } catch {
      // The next peers-updated event will catch up.
    }
  }

  /** Session events are only emitted on change; resync after our own actions. */
  async refreshSession(): Promise<void> {
    try {
      this.applySession(await this.#api.GetSession());
    } catch {
      // The next session-updated event will catch up.
    }
  }

  async refreshLastPeer(): Promise<void> {
    try {
      this.lastPeer = await this.#api.GetLastPeer();
    } catch {
      // Keep the previous value.
    }
  }

  async refreshInterfaces(): Promise<void> {
    this.interfacesLoading = true;
    try {
      this.interfaces = await this.#api.GetConnectionInterfaces();
    } catch (error) {
      this.interfaces = [];
      this.#toasts.warning(errorMessage(error, "Couldn't read this PC's network adapters."));
    } finally {
      this.interfacesLoading = false;
    }
  }

  /** Apply a session update from the backend and log transitions. */
  applySession(next: SessionStatus): void {
    const previous = this.session;
    this.session = next;
    if (previous.connected === next.connected && previous.peerID === next.peerID) return;

    this.connecting = '';
    this.disconnecting = false;
    if (next.connected) {
      const peer = this.peers.find((item) => item.id === next.peerID);
      const route = next.route ? routeLabel(next.route, next.remoteAddress) : '';
      const others = peer
        ? orderedRoutes(peer.routes.filter((r) => r !== next.route && r !== 'manual')).map((r) =>
            routeLabel(r, peer.addresses.find((a) => peer.addressKinds[a] === r)),
          )
        : [];
      const title = route ? `Connected over ${route}` : `Connected to ${next.peerName}`;
      const detail = [
        next.peerName,
        others.length ? `preferred over ${others.join(' and ')}` : '',
        peer?.fingerprint ? `certificate ${shortFingerprint(peer.fingerprint)}` : '',
      ]
        .filter(Boolean)
        .join(' · ');
      this.#activity.addEvent('link', 'accent', title, detail);
    } else if (previous.connected) {
      this.#activity.addEvent('unlink', 'muted', `Disconnected from ${previous.peerName || 'peer'}`);
      void this.refreshLastPeer();
    }
  }

  /** The address to use for a peer: the user's pick, else the backend's best. */
  addressFor(peer: Peer): string {
    const chosen = this.routeChoice[peer.id];
    return chosen && peer.addresses.includes(chosen) ? chosen : peer.address;
  }

  chooseRoute(peer: Peer, address: string): void {
    this.routeChoice = { ...this.routeChoice, [peer.id]: address };
  }

  peerByAddress(address: string): Peer | undefined {
    return this.peers.find((peer) => peer.address === address || peer.addresses.includes(address));
  }

  /** Connect to a trusted peer. Returns true when the backend accepted. */
  async connect(peer: Peer): Promise<boolean> {
    const address = this.addressFor(peer);
    if (!address || this.connecting) return false;
    this.connecting = address;
    try {
      await this.#api.Connect(address);
      await Promise.all([this.refreshPeers(), this.refreshSession()]);
      return true;
    } catch (error) {
      const message = errorMessage(error, 'Connection failed.');
      this.#toasts.error(
        /not trusted/i.test(message) ? `${peer.name || address} isn't paired yet. Use Pair & connect.` : message,
      );
      return false;
    } finally {
      this.connecting = '';
    }
  }

  /** First-time pairing with the remote PIN. Throws with a readable message. */
  async pair(peer: Peer, address: string, pin: string): Promise<void> {
    this.connecting = address;
    try {
      await this.#api.TrustPeer(address, pin);
      await Promise.all([this.refreshPeers(), this.refreshSession()]);
      this.#toasts.success(`Paired with ${peer.name || address}.`);
    } catch (error) {
      throw new Error(errorMessage(error, 'Pairing failed. Check the PIN and try again.'));
    } finally {
      this.connecting = '';
    }
  }

  async disconnect(): Promise<void> {
    if (this.disconnecting) return;
    this.disconnecting = true;
    try {
      await this.#api.Disconnect();
      await this.refreshSession();
    } catch (error) {
      this.#toasts.error(errorMessage(error, "Couldn't disconnect."));
    } finally {
      this.disconnecting = false;
    }
  }

  async reconnect(): Promise<boolean> {
    if (this.reconnecting) return false;
    this.reconnecting = true;
    try {
      await this.#api.Reconnect();
      await Promise.all([this.refreshPeers(), this.refreshSession()]);
      return true;
    } catch (error) {
      const message = errorMessage(error, 'Reconnect failed.');
      this.#toasts.error(
        /not trusted/i.test(message) ? 'That device is no longer paired. Pair it again from Devices.' : message,
      );
      return false;
    } finally {
      this.reconnecting = false;
    }
  }

  /** Add a manual peer. Resolves to an error message, or '' on success. */
  async addPeer(address: string): Promise<string> {
    const trimmed = address.trim();
    if (!trimmed) return 'Enter an IP address, hostname or bt:// address.';
    try {
      await this.#api.AddPeer(trimmed);
      await this.refreshPeers();
      return '';
    } catch (error) {
      return errorMessage(error, "Couldn't add that device.");
    }
  }

  async removePeer(peer: Peer): Promise<void> {
    this.busyPeers = { ...this.busyPeers, [peer.id]: 'remove' };
    try {
      await this.#api.RemovePeer(peer.address);
      await this.refreshPeers();
      if (this.lastPeer?.id === peer.id) await this.refreshLastPeer();
      this.#toasts.info(`Removed ${peer.name || peer.address}.`);
    } catch (error) {
      this.#toasts.error(errorMessage(error, "Couldn't remove that device."));
    } finally {
      this.#clearBusy(peer.id);
    }
  }

  async untrust(peer: Peer): Promise<void> {
    this.busyPeers = { ...this.busyPeers, [peer.id]: 'untrust' };
    try {
      await this.#api.UntrustPeer(peer.id);
      await this.refreshPeers();
      if (this.lastPeer?.id === peer.id) await this.refreshLastPeer();
      this.#toasts.info(`${peer.name || peer.address} is no longer paired. Use Pair & connect to pair again.`);
    } catch (error) {
      this.#toasts.error(errorMessage(error, "Couldn't unpair that device."));
    } finally {
      this.#clearBusy(peer.id);
    }
  }

  async sendFiles(): Promise<void> {
    try {
      await this.#api.PickAndSendFiles();
    } catch (error) {
      const message = errorMessage(error, "Couldn't send files.");
      // Cancelling the picker is not an error worth reporting.
      if (!/cancel/i.test(message)) this.#toasts.error(message);
    }
  }

  #clearBusy(id: string) {
    const next = { ...this.busyPeers };
    delete next[id];
    this.busyPeers = next;
  }
}
