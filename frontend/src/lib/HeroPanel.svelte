<script>
  import { sessionStateLabel, preferredRouteLabel } from './utils';

  export let deviceName;
  export let onlinePeersCount;
  export let manualPeerCount;
  export let hybridPeerCount;
  export let lanPeerCount;
  export let tailscalePeerCount;
  export let tailscalePeerCountMesh;
  export let session;
  export let activePeer;
</script>

<section class="hero panel panel-elevated">
  <div class="hero-copy">
    <span class="eyebrow">Operations overview</span>
    <h2>{deviceName || 'Unassigned node'}</h2>
    <p>
      Route-aware desktop handoff with LAN-first discovery, optional Tailscale reachability,
      clipboard sync, and configurable audio forwarding.
    </p>
  </div>

  <div class="hero-metrics">
    <article class="metric-card">
      <span class="metric-label">Reachable peers</span>
      <strong>{onlinePeersCount}</strong>
      <span class="metric-note">{manualPeerCount} manual, {hybridPeerCount} hybrid</span>
    </article>
    <article class="metric-card">
      <span class="metric-label">LAN paths</span>
      <strong>{lanPeerCount}</strong>
      <span class="metric-note">Preferred for lowest latency</span>
    </article>
    <article class="metric-card">
      <span class="metric-label">Tailnet paths</span>
      <strong>{tailscalePeerCount}</strong>
      <span class="metric-note">{tailscalePeerCountMesh} nodes visible in tailnet</span>
    </article>
    <article class="metric-card">
      <span class="metric-label">Session posture</span>
      <strong>{sessionStateLabel(session)}</strong>
      <span class="metric-note">{activePeer ? preferredRouteLabel(activePeer.preferredRoute) : 'Waiting for connect'}</span>
    </article>
  </div>
</section>

<style>
  .hero {
    display: grid;
    grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
    gap: 1rem;
    align-items: end;
  }
  .hero-copy h2 {
    margin-top: 0.35rem;
    font-size: clamp(1.9rem, 3vw, 2.7rem);
    font-weight: 650;
    letter-spacing: -0.05em;
  }
  .hero-copy p {
    color: var(--text-secondary);
    line-height: 1.6;
  }
  .hero-metrics {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.85rem;
  }
  .metric-card {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 1rem;
    border-radius: 1rem;
    background: rgba(255, 255, 255, 0.035);
    border: 1px solid rgba(126, 145, 168, 0.16);
  }
  .metric-label {
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .metric-card strong {
    font-size: 1.45rem;
    letter-spacing: -0.04em;
  }

  @media (max-width: 1180px) {
    .hero { grid-template-columns: 1fr; }
  }
  @media (max-width: 760px) {
    .hero-metrics { grid-template-columns: 1fr; }
  }
</style>
