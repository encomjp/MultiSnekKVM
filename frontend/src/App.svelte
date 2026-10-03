<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { version as packageVersion } from '../package.json';
  import { getApi, type Api } from './lib/api';
  import PairingDialog from './lib/PairingDialog.svelte';
  import DevicesScreen from './lib/screens/DevicesScreen.svelte';
  import SessionScreen from './lib/screens/SessionScreen.svelte';
  import SettingsScreen from './lib/screens/SettingsScreen.svelte';
  import Sidebar from './lib/Sidebar.svelte';
  import { AppState, setAppState } from './lib/stores/app.svelte';
  import ToastRegion from './lib/ui/ToastRegion.svelte';

  interface Props {
    /** Backend to use; defaults to Wails when available, else the preview. */
    api?: Api;
    version?: string;
  }

  let { api, version = packageVersion }: Props = $props();

  // The app state lives for the component's lifetime; the api prop is read once.
  // svelte-ignore state_referenced_locally
  const app = setAppState(new AppState(api ?? getApi()));

  let main: HTMLElement | undefined = $state();

  onMount(() => {
    void app.start();
  });

  onDestroy(() => app.stop());

  // Start each screen at the top.
  $effect(() => {
    void app.screen;
    void app.settingsSection;
    main?.scrollTo?.({ top: 0 });
  });
</script>

<div class="shell" inert={!!app.pairing}>
  <Sidebar {version} />
  <main class="main" bind:this={main}>
    {#key app.screen}
      <div class="screen-wrap">
        {#if app.screen === 'session'}
          <SessionScreen />
        {:else if app.screen === 'devices'}
          <DevicesScreen />
        {:else}
          <SettingsScreen />
        {/if}
      </div>
    {/key}
  </main>
</div>

{#if app.pairing}
  <PairingDialog request={app.pairing} />
{/if}

<ToastRegion toasts={app.toasts} />

<style>
  .shell {
    display: grid;
    grid-template-columns: 236px minmax(0, 1fr);
    height: 100vh;
    min-height: 0;
    background: var(--bg);
  }

  .main {
    min-width: 0;
    min-height: 0;
    overflow-y: auto;
    padding: 28px 32px 40px;
  }

  .screen-wrap {
    animation: screen-in 120ms ease-out;
  }

  @keyframes screen-in {
    from {
      opacity: 0;
    }
  }

  @media (max-width: 760px) {
    .shell {
      grid-template-columns: minmax(0, 1fr);
      grid-template-rows: auto minmax(0, 1fr);
    }

    .main {
      padding: 20px 16px 32px;
    }
  }
</style>
