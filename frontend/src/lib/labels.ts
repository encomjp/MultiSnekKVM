// Shared copy for setting values, so the Session quick controls and the
// Settings screens always use the same words.

import type { AudioMode, AudioProfile, AudioTiming, AudioTransport, EdgeSide, MicMode } from './api/types';
import type { SegmentOption } from './ui/SegmentedControl.svelte';

export const AUDIO_MODE_OPTIONS: ReadonlyArray<SegmentOption<AudioMode>> = [
  { value: 'off', label: 'Off' },
  { value: 'remote', label: 'Hear remote' },
  { value: 'local', label: 'Send mine' },
];

export const MIC_MODE_OPTIONS: ReadonlyArray<SegmentOption<MicMode>> = [
  { value: 'off', label: 'Off' },
  { value: 'send', label: 'Send' },
  { value: 'receive', label: 'Receive' },
];

export const EDGE_OPTIONS: ReadonlyArray<SegmentOption<EdgeSide>> = [
  { value: 'left', label: 'Left' },
  { value: 'right', label: 'Right' },
  { value: 'top', label: 'Top' },
  { value: 'bottom', label: 'Bottom' },
];

export const AUDIO_TIMING_OPTIONS: ReadonlyArray<SegmentOption<AudioTiming>> = [
  { value: 'always', label: 'Always' },
  { value: 'switched', label: 'Only while on remote' },
];

export const AUDIO_TRANSPORT_OPTIONS: ReadonlyArray<{ value: AudioTransport; label: string }> = [
  { value: 'auto', label: 'Auto (recommended)' },
  { value: 'pcm', label: 'PCM (uncompressed)' },
  { value: 'opus', label: 'Opus (compressed)' },
];

export const AUDIO_PROFILE_OPTIONS: ReadonlyArray<SegmentOption<AudioProfile>> = [
  { value: 'low-latency', label: 'Low latency' },
  { value: 'balanced', label: 'Balanced' },
  { value: 'music', label: 'Music' },
];

export function transportSummary(transport: AudioTransport): string {
  if (transport === 'pcm') return 'PCM';
  if (transport === 'opus') return 'Opus';
  return 'Auto';
}

export function transportHint(transport: AudioTransport): string {
  if (transport === 'pcm') return 'Uncompressed 48 kHz audio. Best on USB4 or wired Ethernet; heavy on Wi-Fi.';
  if (transport === 'opus') return 'Compressed audio that copes with Wi-Fi, Tailscale and Bluetooth links.';
  return 'PCM on direct wired and USB4 links, Opus over Wi-Fi, Tailscale and Bluetooth.';
}

export function profileSummary(profile: AudioProfile): string {
  return AUDIO_PROFILE_OPTIONS.find((option) => option.value === profile)?.label ?? 'Balanced';
}

export function oppositeEdge(side: EdgeSide): EdgeSide {
  return ({ left: 'right', right: 'left', top: 'bottom', bottom: 'top' } as const)[side];
}
