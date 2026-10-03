import { createPreviewApi } from './preview';
import { createWailsApi, hasWailsBackend } from './wails';
import type { Api } from './types';

export type * from './types';
export { createPreviewApi, type PreviewApi } from './preview';

/** The Wails backend when running inside the app, otherwise the in-memory preview. */
export function getApi(): Api {
  return hasWailsBackend() ? createWailsApi() : createPreviewApi();
}
