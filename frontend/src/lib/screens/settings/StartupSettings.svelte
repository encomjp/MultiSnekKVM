<script lang="ts">
  import { getAppState } from '../../stores/app.svelte';
  import type { ThemePreference } from '../../stores/theme.svelte';
  import Card from '../../ui/Card.svelte';
  import SegmentedControl, { type SegmentOption } from '../../ui/SegmentedControl.svelte';
  import Toggle from '../../ui/Toggle.svelte';

  const app = getAppState();
  const settings = app.settings;

  const THEME_OPTIONS: ReadonlyArray<SegmentOption<ThemePreference>> = [
    { value: 'dark', label: 'Dark' },
    { value: 'light', label: 'Light' },
    { value: 'system', label: 'System' },
  ];
</script>

<div class="stack">
  <Card title="Startup">
    <Toggle
      label="Start with Windows"
      description="Launch MultiSnek when you sign in"
      checked={settings.values.autostart}
      pending={settings.status.autostart.pending}
      error={settings.status.autostart.error}
      onChange={(value) => settings.set('autostart', value)}
    />
    <Toggle
      label="Start minimized"
      description="Open quietly to the system tray"
      checked={settings.values.startMinimized}
      pending={settings.status.startMinimized.pending}
      error={settings.status.startMinimized.error}
      onChange={(value) => settings.set('startMinimized', value)}
    />
    <Toggle
      label="Auto-reconnect"
      description="Resume the last session after the link drops or the PC wakes"
      checked={settings.values.autoReconnect}
      pending={settings.status.autoReconnect.pending}
      error={settings.status.autoReconnect.error}
      onChange={(value) => settings.set('autoReconnect', value)}
    />
  </Card>

  <Card title="Appearance" description="Stored on this PC only.">
    <SegmentedControl
      label="Theme"
      options={THEME_OPTIONS}
      value={app.theme.preference}
      onChange={(value) => app.theme.set(value)}
    />
  </Card>
</div>

<style>
  .stack {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
</style>
