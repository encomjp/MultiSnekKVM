/// <reference types="svelte" />
/// <reference types="vite/client" />

type WailsMethod = (...args: never[]) => Promise<unknown>;

interface Window {
  /** Injected by Wails: the bound Go methods. Absent in the browser preview. */
  go?: {
    app?: {
      App?: Record<string, WailsMethod>;
    };
  };
  /** Injected by Wails: the JS runtime (events, window control, ...). */
  runtime?: {
    EventsOnMultiple?: (event: string, callback: (...data: unknown[]) => void, maxCallbacks: number) => () => void;
    [key: string]: unknown;
  };
}
