<script>
  import { shortId, shortFingerprint, copyToClipboard } from './utils';

  export let device;

  let copiedToken = '';
  let copiedTimer = null;

  async function copyValue(token, value) {
    try {
      await copyToClipboard(value);
      copiedToken = token;
      clearTimeout(copiedTimer);
      copiedTimer = setTimeout(() => { copiedToken = ''; copiedTimer = null; }, 1600);
    } catch (_) {}
  }
</script>

<article class="panel overview-card">
  <div class="panel-heading">
    <div>
      <span class="eyebrow">Node identity</span>
      <h3>Local workstation</h3>
    </div>
    <span class="status-dot status-dot-ready"></span>
  </div>

  <dl class="detail-grid">
    <div>
      <dt>Device ID</dt>
      <dd class="mono selectable">{shortId(device.id)}</dd>
    </div>
    <div>
      <dt>Port</dt>
      <dd>{device.port || 24831}</dd>
    </div>
    <div class="detail-span">
      <dt>Fingerprint</dt>
      <dd class="detail-action-row">
        <span class="mono selectable">{shortFingerprint(device.fingerprint)}</span>
        <button class="inline-copy" on:click={() => copyValue('local-fingerprint', device.fingerprint)}>
          {copiedToken === 'local-fingerprint' ? 'Copied' : 'Copy'}
        </button>
      </dd>
    </div>
  </dl>
</article>

<style>
  .overview-card { padding: 1.1rem; }
</style>
