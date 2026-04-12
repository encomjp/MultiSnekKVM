<script>
  import { meshStateLabel, timeAgo } from './utils';

  export let tailscale;
  export let deviceName;

  $: meshHeadline = !tailscale.available
    ? 'Tailscale not detected'
    : tailscale.connected
      ? 'Tailnet relay available'
      : 'Tailscale installed but not connected';

  $: meshSummary = !tailscale.available
    ? 'Install and sign in to Tailscale to extend peer discovery beyond the local subnet.'
    : tailscale.connected
      ? `Discovery is targeting ${tailscale.targetCount} tailnet endpoint${tailscale.targetCount === 1 ? '' : 's'} across ${tailscale.peerCount} known node${tailscale.peerCount === 1 ? '' : 's'}.`
      : 'The local node can see the Tailscale client but it is not currently providing a running tailnet session.';
</script>

<article class="panel overview-card mesh-card">
  <div class="panel-heading">
    <div>
      <span class="eyebrow">Tailscale integration</span>
      <h3>{meshHeadline}</h3>
    </div>
    <span class="status-chip" class:is-mesh={tailscale.connected}>{meshStateLabel(tailscale)}</span>
  </div>

  <p class="panel-copy">{meshSummary}</p>

  <dl class="detail-grid detail-grid-mesh">
    <div>
      <dt>Tailnet</dt>
      <dd>{tailscale.tailnet || 'Not connected'}</dd>
    </div>
    <div>
      <dt>Backend state</dt>
      <dd>{tailscale.backendState || 'Unknown'}</dd>
    </div>
    <div>
      <dt>Node name</dt>
      <dd>{tailscale.selfName || deviceName || 'Unavailable'}</dd>
    </div>
    <div>
      <dt>Last sync</dt>
      <dd>{timeAgo(tailscale.lastSync)}</dd>
    </div>
    <div class="detail-span">
      <dt>Self IPs</dt>
      <dd class="mono">{tailscale.selfIPs.length ? tailscale.selfIPs.join(', ') : 'None'}</dd>
    </div>
  </dl>

  {#if tailscale.lastError}
    <div class="mesh-error">{tailscale.lastError}</div>
  {/if}
</article>

<style>
  .overview-card { padding: 1.1rem; }
  .mesh-card {
    background: linear-gradient(180deg, rgba(14, 25, 40, 0.95), rgba(7, 23, 34, 0.96));
  }
  .detail-grid-mesh { margin-top: 1rem; }
</style>
