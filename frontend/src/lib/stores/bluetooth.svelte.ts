// Direct Bluetooth link between paired PCs: scan status, the PCs found
// running MultiSnek, and the enable switch. Like the settings store, the
// switch updates optimistically with its own pending/error state and reverts
// on a failed save. Status updates arrive through `bluetooth-updated`.

import { emptyBluetooth, normalizeBluetooth } from '../api/normalize';
import type { Api, BluetoothStatus } from '../api/types';
import { errorMessage } from '../utils';

export type BluetoothErrorHandler = (message: string) => void;

export class BluetoothStore {
  status = $state<BluetoothStatus>({ ...emptyBluetooth });
  loaded = $state(false);
  /** Saving the enable switch. */
  pending = $state(false);
  error = $state('');
  /** A refresh request is in flight (the backend's own `scanning` flag is separate). */
  refreshing = $state(false);

  readonly #api: Api;
  readonly #onError: BluetoothErrorHandler | undefined;
  #generation = 0;

  constructor(api: Api, onError?: BluetoothErrorHandler) {
    this.#api = api;
    this.#onError = onError;
  }

  /** True while a scan is running or a refresh has been requested. */
  get scanning(): boolean {
    return this.status.scanning || this.refreshing;
  }

  async load(): Promise<void> {
    try {
      this.apply(await this.#api.GetBluetoothStatus());
    } catch {
      // Keep the "not available" default; the backend may not support Bluetooth.
    }
    this.loaded = true;
  }

  /** Apply a status from the backend (event or poll). */
  apply(next: BluetoothStatus): void {
    this.status = normalizeBluetooth(next);
  }

  /** Start an immediate scan. Progress follows via events. */
  async refresh(): Promise<void> {
    if (this.scanning || !this.status.available || !this.status.enabled) return;
    this.refreshing = true;
    // Show the busy state at once; the backend event confirms it.
    this.status = { ...this.status, scanning: true };
    try {
      await this.#api.RefreshBluetooth();
    } catch (error) {
      this.status = { ...this.status, scanning: false };
      this.#onError?.(errorMessage(error, "Couldn't scan for Bluetooth devices."));
    } finally {
      this.refreshing = false;
    }
  }

  /** Enable or disable Bluetooth connections. Returns true on success. */
  async setEnabled(enabled: boolean): Promise<boolean> {
    const previous = this.status;
    const generation = ++this.#generation;
    this.status = enabled ? { ...previous, enabled: true } : { ...previous, enabled: false, scanning: false };
    this.pending = true;
    this.error = '';
    try {
      await this.#api.SetBluetoothEnabled(enabled);
      if (this.#generation === generation) this.pending = false;
      return true;
    } catch (error) {
      const message = errorMessage(error, "Couldn't change the Bluetooth setting.");
      if (this.#generation === generation) {
        this.status = previous;
        this.pending = false;
        this.error = message;
      }
      this.#onError?.(message);
      return false;
    }
  }
}
