// The desktop process supplies an ephemeral connection; it is never saved in browser storage.
import type { Stage } from './lib.ts';

export interface Thumbnail { url: string; x: number; y: number; w: number; h: number }
export interface Video {
  id: string; title: string; status: string; source_kind: string; error?: string;
  duration_ms: number | null; has_video: boolean | null; has_audio: boolean | null;
  thumbnail: Thumbnail | null; created_at: string; updated_at: string;
}
export interface Project { id: string; name: string; video_count: number; created_at: string }
export interface Moment {
  video_id: string; video_title: string; start_ms: number; end_ms: number; jump_ms: number; score: number;
  channels: { channel: string; rank: number }[]; snippet: string | null; thumbnail: Thumbnail | null;
}
export type { Stage };

interface Connection { base: string; token: string }
export interface LocalFile { path: string; name: string; size: number }
export interface RuntimeState { stage: string; message: string; device: string; reason?: string; error: string; fraction: number | null; ready: boolean }

declare global {
  interface Window { mygo?: { call<T>(method: string, ...args: unknown[]): Promise<T> } }
}
let connection: Promise<Connection> | undefined;
export function connect(): Promise<Connection> {
  return connection ??= window.mygo ? window.mygo.call<Connection>('Desktop.Bootstrap') : Promise.reject(new Error('Please open VideoHelper from the desktop app.'));
}
export async function pickFiles(): Promise<LocalFile[]> {
  if (!window.mygo) throw new Error('The desktop file picker is unavailable.');
  return window.mygo.call<LocalFile[]>('Desktop.PickFiles');
}

export class ApiError extends Error {
  status: number; code: string;
  constructor(status: number, code: string, message: string) { super(message); this.status = status; this.code = code; }
}

export async function call<T = unknown>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const s = await connect();
  let res: Response;
  try {
    res = await fetch(s.base + path, {
      method, signal, body: body === undefined ? undefined : JSON.stringify(body),
      headers: { 'X-Desktop-Token': s.token, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
    });
  } catch (err) {
    if (signal?.aborted) throw signal.reason;
    throw new ApiError(0, 'network', '');
  }
  const text = await res.text();
  let data: { error?: { code?: string; message?: string } } | null = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* status below reports malformed error responses */ }
  if (!res.ok) throw new ApiError(res.status, data?.error?.code || 'internal', data?.error?.message || `HTTP ${res.status}`);
  return data as T;
}

export const get = <T>(path: string) => call<T>('GET', path);
export const post = <T>(path: string, body?: unknown, signal?: AbortSignal) => call<T>('POST', path, body, signal);
