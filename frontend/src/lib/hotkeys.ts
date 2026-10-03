import type { ExitHotkey } from './api/types';

export const MOD_CTRL = 1;
export const MOD_ALT = 2;
export const MOD_SHIFT = 4;
export const MOD_WIN = 8;

/** Display names for Windows virtual-key codes. */
const VK_NAMES: Record<number, string> = {
  0x08: 'Backspace',
  0x09: 'Tab',
  0x0d: 'Enter',
  0x13: 'Pause',
  0x1b: 'Esc',
  0x20: 'Space',
  0x21: 'Page Up',
  0x22: 'Page Down',
  0x23: 'End',
  0x24: 'Home',
  0x25: 'Left',
  0x26: 'Up',
  0x27: 'Right',
  0x28: 'Down',
  0x2c: 'Print Screen',
  0x2d: 'Insert',
  0x2e: 'Delete',
  0x60: 'Num 0',
  0x61: 'Num 1',
  0x62: 'Num 2',
  0x63: 'Num 3',
  0x64: 'Num 4',
  0x65: 'Num 5',
  0x66: 'Num 6',
  0x67: 'Num 7',
  0x68: 'Num 8',
  0x69: 'Num 9',
  0x6a: 'Num *',
  0x6b: 'Num +',
  0x6d: 'Num -',
  0x6e: 'Num .',
  0x6f: 'Num /',
  0x91: 'Scroll Lock',
  0xba: ';',
  0xbb: '=',
  0xbc: ',',
  0xbd: '-',
  0xbe: '.',
  0xbf: '/',
  0xc0: '`',
  0xdb: '[',
  0xdc: '\\',
  0xdd: ']',
  0xde: "'",
};

export function vkName(vk: number): string {
  if (VK_NAMES[vk]) return VK_NAMES[vk];
  if ((vk >= 0x30 && vk <= 0x39) || (vk >= 0x41 && vk <= 0x5a)) return String.fromCharCode(vk);
  if (vk >= 0x70 && vk <= 0x87) return `F${vk - 0x6f}`;
  return `VK ${vk.toString(16).toUpperCase().padStart(2, '0')}`;
}

export function modifierNames(modifiers: number): string[] {
  const names: string[] = [];
  if (modifiers & MOD_CTRL) names.push('Ctrl');
  if (modifiers & MOD_ALT) names.push('Alt');
  if (modifiers & MOD_SHIFT) names.push('Shift');
  if (modifiers & MOD_WIN) names.push('Win');
  return names;
}

/** Keys to render as <kbd> chips. vkCode 0 is the backend default, Esc. */
export function hotkeyKeys(hotkey: ExitHotkey): string[] {
  if (!hotkey.vkCode) return ['Esc'];
  return [...modifierNames(hotkey.modifiers), vkName(hotkey.vkCode)];
}

export function hotkeyLabel(hotkey: ExitHotkey): string {
  return hotkeyKeys(hotkey).join(' + ');
}

export type CaptureResult =
  | { kind: 'cancel' }
  | { kind: 'ignore' }
  | { kind: 'invalid'; message: string }
  | { kind: 'ok'; hotkey: ExitHotkey };

const MODIFIER_KEYS = new Set(['Control', 'Alt', 'Shift', 'Meta', 'OS', 'AltGraph']);

/**
 * Turn a keydown into an exit hotkey. WebView2 reports Windows VK codes in
 * KeyboardEvent.keyCode, which is what the backend expects.
 */
export function captureHotkey(event: Pick<KeyboardEvent, 'key' | 'keyCode' | 'ctrlKey' | 'altKey' | 'shiftKey' | 'metaKey'>): CaptureResult {
  if (event.key === 'Escape') return { kind: 'cancel' };
  if (MODIFIER_KEYS.has(event.key)) return { kind: 'ignore' };

  let modifiers = 0;
  if (event.ctrlKey) modifiers |= MOD_CTRL;
  if (event.altKey) modifiers |= MOD_ALT;
  if (event.shiftKey) modifiers |= MOD_SHIFT;
  if (event.metaKey) modifiers |= MOD_WIN;

  if ((modifiers & (MOD_CTRL | MOD_ALT)) === (MOD_CTRL | MOD_ALT)) {
    return { kind: 'invalid', message: 'Ctrl + Alt is AltGr on many keyboard layouts. Pick another combination.' };
  }
  const vk = event.keyCode;
  const isFunctionKey = vk >= 0x70 && vk <= 0x87;
  if (modifiers === 0 && !isFunctionKey) {
    return {
      kind: 'invalid',
      message: 'A key without a modifier would interrupt typing on the other PC. Add Ctrl, Alt or Shift, or use F1–F24.',
    };
  }
  if (!vk) return { kind: 'ignore' };
  return { kind: 'ok', hotkey: { modifiers, vkCode: vk } };
}
