<script lang="ts">
  import type { AudioDevice, MonitorInfo, TailscaleStatus, DeviceInfo } from './types';
  import AdvancedSettingsPanel from './settings/AdvancedSettingsPanel.svelte';
  import AudioSettingsPanel from './settings/AudioSettingsPanel.svelte';
  import GeneralSettingsPanel from './settings/GeneralSettingsPanel.svelte';

  export let activeSettingsSection: 'general' | 'audio' | 'advanced' = 'general';
  export let busySetting = '';
  export let edgeSide = 'right';
  export let sensitivity = 1;
  export let audioMode = 'off';
  export let audioProfile = 'balanced';
  export let audioTransportMode = 'pcm';
  export let audioTiming = 'always';
  export let muteSource = false;
  export let micMode = 'off';
  export let audioDevices: AudioDevice[] = [];
  export let captureDeviceID = '';
  export let playbackDeviceID = '';
  export let micDeviceID = '';
  export let micPlaybackDeviceID = '';
  export let autostart = false;
  export let startMinimized = false;
  export let autoReconnect = true;
  export let isBeingControlled = false;
  export let isControllerConnected = false;
  export let tailscale: TailscaleStatus;
  export let device: DeviceInfo;
  export let showLogs = false;
  export let logLines: string[] = [];
  export let exitHotkeyModifiers = 0;
  export let exitHotkeyVKCode = 0;
  export let localMonitors: MonitorInfo[] = [];
  export let triggerMonitorID = '';
  export let triggerSide = '';
  export let triggerStartPct = 0;
  export let triggerEndPct = 1;
  export let returnMonitorID = '';
  export let returnXPct = 0.5;
  export let returnYPct = 0.5;
  export let onUpdateEdgeSide: (side: string) => void | Promise<void>;
  export let onUpdateSensitivity: (value: string) => void | Promise<void>;
  export let onUpdateAudioMode: (mode: string) => void | Promise<void>;
  export let onUpdateAudioProfile: (profile: string) => void | Promise<void>;
  export let onUpdateAudioTransportMode: (mode: string) => void | Promise<void>;
  export let onUpdateAudioTiming: (timing: string) => void | Promise<void>;
  export let onUpdateMuteSource: (enabled: boolean) => void | Promise<void>;
  export let onUpdateMicMode: (mode: string) => void | Promise<void>;
  export let onUpdateCaptureDevice: (id: string) => void | Promise<void>;
  export let onUpdatePlaybackDevice: (id: string) => void | Promise<void>;
  export let onUpdateMicDevice: (id: string) => void | Promise<void>;
  export let onUpdateMicPlaybackDevice: (id: string) => void | Promise<void>;
  export let onUpdateAutostart: (enabled: boolean) => void | Promise<void>;
  export let onUpdateStartMinimized: (enabled: boolean) => void | Promise<void>;
  export let onUpdateAutoReconnect: (enabled: boolean) => void | Promise<void>;
  export let onToggleLogs: () => void | Promise<void>;
  export let onRefreshAudioDevices: (() => void | Promise<void>) | undefined = undefined;
  export let onUpdateExitHotkey: (modifiers: number, vkCode: number) => void | Promise<void>;
  export let onUpdateTriggerZone: (monitorID: string, side: string, startPct: number, endPct: number) => void | Promise<void>;
  export let onUpdateReturnAnchor: (monitorID: string, xPct: number, yPct: number) => void | Promise<void>;
</script>

<section class="screen" data-screen="settings">
  <div class="settings-tabs" role="tablist">
    <button type="button" class="settings-tab" class:active={activeSettingsSection === 'general'} on:click={() => activeSettingsSection = 'general'}>General</button>
    <button type="button" class="settings-tab" class:active={activeSettingsSection === 'audio'} on:click={() => activeSettingsSection = 'audio'}>Audio</button>
    <button type="button" class="settings-tab" class:active={activeSettingsSection === 'advanced'} on:click={() => activeSettingsSection = 'advanced'}>Advanced</button>
  </div>

  {#if activeSettingsSection === 'general'}
    <GeneralSettingsPanel
      {busySetting}
      {edgeSide}
      {sensitivity}
      {autostart}
      {startMinimized}
      {autoReconnect}
      {isControllerConnected}
      {exitHotkeyModifiers}
      {exitHotkeyVKCode}
      {localMonitors}
      {triggerMonitorID}
      {triggerSide}
      {triggerStartPct}
      {triggerEndPct}
      {returnMonitorID}
      {returnXPct}
      {returnYPct}
      {onUpdateEdgeSide}
      {onUpdateSensitivity}
      {onUpdateAutostart}
      {onUpdateStartMinimized}
      {onUpdateAutoReconnect}
      {onUpdateExitHotkey}
      {onUpdateTriggerZone}
      {onUpdateReturnAnchor}
    />
  {/if}

  {#if activeSettingsSection === 'audio'}
    <AudioSettingsPanel
      {busySetting}
      {audioMode}
      {audioProfile}
      {audioTransportMode}
      {audioTiming}
      {muteSource}
      {micMode}
      {audioDevices}
      {captureDeviceID}
      {playbackDeviceID}
      {micDeviceID}
      {micPlaybackDeviceID}
      {isBeingControlled}
      {onUpdateAudioMode}
      {onUpdateAudioProfile}
      {onUpdateAudioTransportMode}
      {onUpdateAudioTiming}
      {onUpdateMuteSource}
      {onUpdateMicMode}
      {onUpdateCaptureDevice}
      {onUpdatePlaybackDevice}
      {onUpdateMicDevice}
      {onUpdateMicPlaybackDevice}
      {onRefreshAudioDevices}
    />
  {/if}

  {#if activeSettingsSection === 'advanced'}
    <AdvancedSettingsPanel
      {tailscale}
      {device}
      {showLogs}
      {logLines}
      {onToggleLogs}
    />
  {/if}
</section>

<style>
  .screen {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .settings-tabs {
    display: flex;
    gap: 0.35rem;
  }

  .settings-tab {
    border-radius: 999px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-secondary);
    padding: 0.42rem 0.9rem;
    font-weight: 600;
    font-size: 0.88rem;
    transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
  }

  .settings-tab.active {
    border-color: var(--accent-border);
    background: var(--panel-accent);
    color: var(--text-strong);
  }
</style>
