<script lang="ts">
  import { getAppState, SETTINGS_SECTIONS, type Screen } from './stores/app.svelte';
  import Icon, { type IconName } from './ui/Icon.svelte';
  import ThisPcCard from './ThisPcCard.svelte';

  interface Props {
    version: string;
  }

  let { version }: Props = $props();

  const app = getAppState();
  const conn = app.connection;

  const ITEMS: ReadonlyArray<{ id: Screen; label: string; icon: IconName }> = [
    { id: 'session', label: 'Session', icon: 'session' },
    { id: 'devices', label: 'Devices', icon: 'devices' },
    { id: 'settings', label: 'Settings', icon: 'settings' },
  ];
</script>

<aside class="sidebar">
  <div class="brand">
    <span class="brand-mark"><Icon name="logo" size={18} strokeWidth={2.2} /></span>
    <span class="brand-text">
      <span class="brand-name">MultiSnek</span>
      <span class="brand-version">{version}{app.isPreview ? ' · preview' : ''}</span>
    </span>
  </div>

  <nav aria-label="Primary" class="nav">
    <ul class="nav-list">
      {#each ITEMS as item (item.id)}
        {@const current = app.screen === item.id}
        <li>
          <button
            type="button"
            class="nav-item"
            class:current
            aria-current={current ? 'page' : undefined}
            onclick={() => app.navigate(item.id)}
          >
            <Icon name={item.icon} size={18} />
            <span class="nav-label">{item.label}</span>
            {#if item.id === 'session' && conn.session.connected}
              <span class="nav-dot" title="Connected" aria-hidden="true"></span>
              <span class="sr-only">(connected)</span>
            {:else if item.id === 'devices' && conn.peers.length > 0}
              <span class="nav-count mono" aria-hidden="true">{conn.onlineCount}/{conn.peers.length}</span>
              <span class="sr-only">({conn.onlineCount} of {conn.peers.length} online)</span>
            {/if}
          </button>
          {#if item.id === 'settings' && current}
            <ul class="subnav" aria-label="Settings sections">
              {#each SETTINGS_SECTIONS as section (section.id)}
                {@const subCurrent = app.settingsSection === section.id}
                <li>
                  <button
                    type="button"
                    class="subnav-item"
                    class:current={subCurrent}
                    aria-current={subCurrent ? 'true' : undefined}
                    onclick={() => app.navigate('settings', section.id)}>{section.label}</button
                  >
                </li>
              {/each}
            </ul>
          {/if}
        </li>
      {/each}
    </ul>
  </nav>

  <div class="spacer"></div>
  <ThisPcCard />
</aside>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 20px 14px;
    background: var(--panel);
    border-right: 1px solid var(--line);
    overflow-y: auto;
    min-height: 0;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 4px 8px 18px;
  }

  .brand-mark {
    width: 30px;
    height: 30px;
    flex: none;
    border-radius: 8px;
    background: var(--accent);
    color: var(--on-accent);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .brand-text {
    display: flex;
    flex-direction: column;
  }

  .brand-name {
    font-weight: 650;
    font-size: 15px;
  }

  .brand-version {
    font-size: 12px;
    color: var(--muted);
  }

  .nav-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .nav-item {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 40px;
    padding: 0 10px;
    border: 0;
    border-radius: var(--radius);
    background: transparent;
    color: var(--muted);
    text-align: left;
  }

  .nav-item:hover {
    background: var(--panel2);
    color: var(--text);
  }

  .nav-item.current {
    background: var(--accent-soft);
    color: var(--text);
    font-weight: 600;
  }

  .nav-label {
    flex: 1;
  }

  .nav-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
  }

  .nav-count {
    font-size: 12px;
  }

  .subnav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 4px 0 4px 28px;
  }

  .subnav-item {
    width: 100%;
    text-align: left;
    min-height: 34px;
    padding: 0 10px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--muted);
    font-size: 13px;
  }

  .subnav-item:hover {
    background: var(--panel2);
    color: var(--text);
  }

  .subnav-item.current {
    background: var(--panel2);
    color: var(--text);
    font-weight: 600;
  }

  .spacer {
    flex: 1;
    min-height: 12px;
  }

  @media (max-width: 760px) {
    .sidebar {
      flex-direction: row;
      flex-wrap: wrap;
      align-items: center;
      gap: 8px 12px;
      padding: 10px 16px;
      border-right: 0;
      border-bottom: 1px solid var(--line);
      overflow: visible;
    }

    .brand {
      padding: 0;
    }

    .nav {
      flex: 1 1 auto;
    }

    .nav-list {
      flex-direction: row;
      flex-wrap: wrap;
    }

    .nav-list > li {
      position: relative;
    }

    .subnav {
      flex-direction: row;
      flex-wrap: wrap;
      padding: 4px 0 0;
      position: static;
    }

    .spacer {
      display: none;
    }
  }
</style>
