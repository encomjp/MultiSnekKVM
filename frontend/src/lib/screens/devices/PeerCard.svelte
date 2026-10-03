<script lang="ts">
  import type { Peer } from '../../api/types';
  import { getAppState } from '../../stores/app.svelte';
  import {
    addressLabel,
    hostOf,
    isManualPeer,
    isPeerOnline,
    orderedRoutes,
    peerRoutes,
    routeLabel,
    shortFingerprint,
    timeAgo,
  } from '../../utils';
  import Icon from '../../ui/Icon.svelte';
  import StatusDot from '../../ui/StatusDot.svelte';

  interface Props {
    peer: Peer;
  }

  let { peer }: Props = $props();

  const app = getAppState();
  const conn = app.connection;
  const uid = $props.id();

  let menuOpen = $state(false);
  let menuWrap: HTMLDivElement | undefined = $state();
  let menuButton: HTMLButtonElement | undefined = $state();

  const name = $derived(peer.name || hostOf(peer.address) || 'Unknown device');
  const active = $derived(conn.session.connected && conn.session.peerID === peer.id);
  const online = $derived(isPeerOnline(peer));
  const manual = $derived(isManualPeer(peer));
  const routes = $derived(peerRoutes(peer));
  const selected = $derived(conn.addressFor(peer));
  const busy = $derived(conn.busyPeers[peer.id]);
  const connectingHere = $derived(!!conn.connecting && routes.some((r) => r.address === conn.connecting));
  const otherSession = $derived(conn.session.connected && !active);

  const subline = $derived.by(() => {
    const parts: string[] = [peer.trusted ? 'Paired' : 'Not paired'];
    if (manual) parts.unshift('Added manually');
    if (active && peer.fingerprint) {
      parts.push(`certificate ${shortFingerprint(peer.fingerprint)}`);
      return parts.join(' · ');
    }
    return parts.join(' · ');
  });

  const routeSummary = $derived.by(() => {
    const kind = peer.addressKinds[selected] || '';
    const primary = addressLabel(peer, selected);
    const others = orderedRoutes(peer.routes.filter((r) => r !== kind && r !== 'manual')).map((r) =>
      routeLabel(
        r,
        peer.addresses.find((a) => peer.addressKinds[a] === r),
      ),
    );
    return { primary, host: hostOf(selected), others };
  });

  function onWindowPointer(event: PointerEvent) {
    if (menuOpen && menuWrap && !menuWrap.contains(event.target as Node)) menuOpen = false;
  }

  function onMenuKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && menuOpen) {
      event.stopPropagation();
      menuOpen = false;
      menuButton?.focus();
    }
  }

  function runMenu(action: () => void) {
    menuOpen = false;
    action();
  }

  async function copyAddress() {
    try {
      await navigator.clipboard?.writeText(selected);
      app.toasts.success(`Copied ${selected}.`);
    } catch {
      app.toasts.error("Couldn't copy to the clipboard.");
    }
  }
</script>

<svelte:window onpointerdown={onWindowPointer} />

<li class="peer" class:active class:manual-untrusted={manual && !peer.trusted} aria-labelledby="{uid}-name">
  <div class="peer-head">
    <span class="peer-icon" class:on={active}>
      <Icon name={peer.source === 'tailscale' ? 'laptop' : 'devices'} size={20} />
    </span>
    <div class="peer-text">
      <span class="peer-name">
        <span id="{uid}-name">{name}</span>
        {#if active}
          <span class="pill">Connected</span>
        {:else if online}
          <span class="state"><StatusDot size={6} />Online</span>
        {:else}
          <span class="state">Offline · last seen {timeAgo(peer.lastSeen)}</span>
        {/if}
      </span>
      <span class="peer-sub">
        {subline}
        {#if !active}
          · {routeSummary.primary} <span class="mono">{routes.length > 1 ? routeSummary.host : selected}</span>
          {#if routeSummary.others.length}· also on {routeSummary.others.join(', ')}{/if}
        {/if}
      </span>
    </div>

    <div class="peer-actions">
      {#if active}
        <button type="button" class="btn btn-danger" onclick={() => conn.disconnect()} disabled={conn.disconnecting}>
          {conn.disconnecting ? 'Disconnecting…' : 'Disconnect'}
        </button>
      {:else if peer.trusted}
        <button
          type="button"
          class="btn"
          class:btn-primary={online && !otherSession}
          disabled={otherSession || !!conn.connecting}
          title={otherSession ? `Disconnect from ${conn.session.peerName} first` : undefined}
          onclick={() => conn.connect(peer).then((ok) => ok && app.navigate('session'))}
        >
          {#if connectingHere}<span class="spinner" aria-hidden="true"></span>Connecting…{:else}Connect{/if}
        </button>
      {:else}
        <button
          type="button"
          class="btn btn-accent-outline"
          disabled={otherSession || !!conn.connecting}
          title={otherSession ? `Disconnect from ${conn.session.peerName} first` : undefined}
          onclick={() => app.openPairing(peer, selected)}>Pair &amp; connect</button
        >
      {/if}

      {#if manual}
        <button
          type="button"
          class="btn btn-ghost"
          disabled={!!busy || active}
          aria-label="Remove {name}"
          onclick={() => conn.removePeer(peer)}>{busy === 'remove' ? 'Removing…' : 'Remove'}</button
        >
      {/if}

      {#if peer.trusted}
        <div class="menu-wrap" bind:this={menuWrap} onkeydown={onMenuKeydown} role="presentation">
          <button
            type="button"
            class="btn btn-icon"
            bind:this={menuButton}
            aria-label="More actions for {name}"
            aria-expanded={menuOpen}
            aria-controls="{uid}-menu"
            onclick={() => (menuOpen = !menuOpen)}
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" />
            </svg>
          </button>
          {#if menuOpen}
            <div class="menu" id="{uid}-menu">
              <button type="button" class="menu-item" onclick={() => runMenu(copyAddress)}>
                <Icon name="copy" />Copy address
              </button>
              <button
                type="button"
                class="menu-item danger"
                disabled={active || !!busy}
                onclick={() => runMenu(() => conn.untrust(peer))}
              >
                <Icon name="unlink" />Untrust
              </button>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  {#if routes.length > 1}
    <fieldset class="routes">
      <legend class="routes-legend">
        Routes · {active ? 'switching takes effect on next connect' : 'the best route is used unless you pick another'}
      </legend>
      {#each routes as route (route.address)}
        <label class="route" class:checked={route.address === selected}>
          <input
            type="radio"
            name="{uid}-route"
            value={route.address}
            checked={route.address === selected}
            disabled={connectingHere}
            onchange={() => conn.chooseRoute(peer, route.address)}
          />
          <span class="route-label">{route.label}</span>
          <span class="route-addr mono">{route.address}</span>
          {#if route.best}<span class="best">Best</span>{/if}
        </label>
      {/each}
    </fieldset>
  {/if}
</li>

<style>
  .peer {
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius-lg);
    padding: 16px 18px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .peer.active {
    border-color: var(--accent);
  }

  .peer.manual-untrusted {
    border-style: dashed;
  }

  .peer-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
  }

  .peer-icon {
    width: 40px;
    height: 40px;
    flex: none;
    border-radius: var(--radius-md);
    background: var(--panel2);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .peer-icon.on {
    background: var(--accent-soft);
    color: var(--accent);
  }

  .peer-text {
    flex: 1 1 200px;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .peer-name {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    font-weight: 600;
    font-size: 15px;
  }

  .state {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 12px;
    font-weight: 500;
    color: var(--muted);
  }

  .peer-sub {
    font-size: 12px;
    color: var(--muted);
    overflow-wrap: anywhere;
  }

  .peer-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .menu-wrap {
    position: relative;
  }

  .menu {
    position: absolute;
    right: 0;
    top: calc(100% + 6px);
    z-index: 20;
    min-width: 180px;
    padding: 6px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
  }

  .menu-item {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 36px;
    padding: 0 10px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    text-align: left;
  }

  .menu-item:hover:not(:disabled) {
    background: var(--panel2);
  }

  .menu-item.danger {
    color: var(--danger);
  }

  .menu-item:disabled {
    color: var(--muted);
  }

  .routes {
    border: 0;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    min-width: 0;
  }

  .routes-legend {
    font-size: 12px;
    color: var(--muted);
    padding: 0;
    margin-bottom: 6px;
  }

  .route {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    min-height: 44px;
    padding: 0 12px;
    border-radius: var(--radius);
    border: 1px solid var(--line);
    background: var(--panel2);
    cursor: pointer;
  }

  .route:hover {
    border-color: var(--line-strong);
  }

  .route.checked {
    border-color: var(--accent);
    background: var(--accent-soft);
  }

  .route input {
    width: 18px;
    height: 18px;
    margin: 0;
  }

  .route-label {
    flex: 1 1 140px;
    font-weight: 500;
  }

  .route-addr {
    font-size: 12px;
    color: var(--muted);
  }

  .best {
    font-size: 11px;
    font-weight: 600;
    color: var(--accent);
    border: 1px solid var(--accent);
    padding: 0 6px;
    border-radius: 999px;
  }
</style>
