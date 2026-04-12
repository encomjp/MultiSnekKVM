<article class="panel sidebar-panel sidebar-panel-accent">
  <span class="eyebrow">Routing notes</span>
  <h3>Discovery policy</h3>
  <ul class="notes-list">
    <li>
      <svg class="note-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
      <span>Inbound control requests are accepted only from peers already pinned in the local trust store.</span>
    </li>
    <li>
      <svg class="note-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><circle cx="5" cy="12" r="2"/><circle cx="19" cy="5" r="2"/><circle cx="19" cy="19" r="2"/><path d="M7 12h10M17 7l-5 5 5 5"/></svg>
      <span>LAN routes are preferred automatically whenever the same peer is reachable both locally and over Tailscale.</span>
    </li>
    <li>
      <svg class="note-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
      <span>Tailscale extends peer discovery by broadcasting to visible tailnet node IPs when the client is running.</span>
    </li>
    <li>
      <svg class="note-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><line x1="12" y1="5" x2="12" y2="19"/><polyline points="19 12 12 19 5 12"/></svg>
      <span>Manual endpoints remain available even when broadcast discovery is blocked by local network policy.</span>
    </li>
  </ul>
</article>

<style>
  .sidebar-panel {
    padding: 1.1rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .sidebar-panel-accent {
    background: linear-gradient(180deg, rgba(16, 29, 46, 0.94), rgba(11, 21, 35, 0.98));
  }
  .notes-list {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .notes-list li {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    font-size: 0.85rem;
    line-height: 1.5;
    color: var(--text-secondary);
  }
  .note-icon {
    width: 14px;
    height: 14px;
    flex-shrink: 0;
    margin-top: 0.15em;
    color: var(--accent);
    opacity: 0.7;
  }
</style>
