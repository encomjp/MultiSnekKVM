<script lang="ts">
  import { getAppState, SETTINGS_SECTIONS } from '../stores/app.svelte';
  import AudioSettings from './settings/AudioSettings.svelte';
  import DiagnosticsSettings from './settings/DiagnosticsSettings.svelte';
  import InputSettings from './settings/InputSettings.svelte';
  import SecuritySettings from './settings/SecuritySettings.svelte';
  import StartupSettings from './settings/StartupSettings.svelte';

  const app = getAppState();
  const section = $derived(SETTINGS_SECTIONS.find((s) => s.id === app.settingsSection) ?? SETTINGS_SECTIONS[0]);

  const SUBTITLES: Record<string, string> = {
    input: 'Changes save as you make them.',
    audio: 'Desktop audio and microphone between the two PCs. Changes save as you make them.',
    startup: 'How MultiSnek starts and looks.',
    security: 'This PC’s identity and the devices it trusts.',
    diagnostics: 'Health, logs and counters for troubleshooting.',
  };
</script>

<div class="screen settings">
  <header>
    <h1>{section.label}</h1>
    <p class="muted subtitle">{SUBTITLES[section.id]}</p>
  </header>

  {#if section.id === 'input'}
    <InputSettings />
  {:else if section.id === 'audio'}
    <AudioSettings />
  {:else if section.id === 'startup'}
    <StartupSettings />
  {:else if section.id === 'security'}
    <SecuritySettings />
  {:else}
    <DiagnosticsSettings />
  {/if}
</div>

<style>
  .settings {
    max-width: 960px;
  }

  .subtitle {
    margin-top: 4px;
  }
</style>
