<script>
  import { sessionStateLabel, meshStateLabel } from './utils';

  export let device;
  export let session;
  export let tailscale;
</script>

<header class="topbar">
  <div class="brand" style="--wails-draggable: drag">
    <div class="brand-mark">MS</div>
    <div>
      <div class="eyebrow">Secure desk switching</div>
      <h1>MultiSnekKVM Control Center</h1>
    </div>
  </div>

  <div class="topbar-pills">
    <span class="pill">Port {device.port || 24831}</span>
    <span class="pill pill-status" class:is-live={session.connected}>{sessionStateLabel(session)}</span>
    <span class="pill pill-status" class:is-mesh={tailscale.connected}>{meshStateLabel(tailscale)}</span>
  </div>
</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 1.15rem 1.5rem;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(180deg, rgba(12, 20, 31, 0.94), rgba(12, 20, 31, 0.8));
    backdrop-filter: blur(20px);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 1rem;
  }
  .brand-mark {
    width: 3rem;
    height: 3rem;
    border-radius: 0.9rem;
    display: grid;
    place-items: center;
    font-size: 0.9rem;
    font-weight: 700;
    letter-spacing: 0.14em;
    color: var(--text-strong);
    background: linear-gradient(135deg, rgba(78, 128, 203, 0.94), rgba(102, 196, 204, 0.62));
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.12), 0 18px 36px rgba(7, 12, 20, 0.28);
  }
  h1 {
    margin-top: 0.18rem;
    font-size: 1.2rem;
    font-weight: 650;
    letter-spacing: -0.03em;
  }
  .topbar-pills {
    display: flex;
    flex-wrap: wrap;
    gap: 0.65rem;
  }

  @media (max-width: 760px) {
    .topbar { flex-direction: column; align-items: flex-start; }
  }
</style>
