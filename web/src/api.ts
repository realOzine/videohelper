// The API address and token live in this browser only; the token travels in the Authorization header, never a cookie.
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
export interface Settings { base?: string; token?: string }
export type { Stage };

const KEY = 'vh-api';

export function settings(): Settings {
  try { return JSON.parse(localStorage.getItem(KEY) || '{}') || {}; } catch { return {}; }
}
export const saveSettings = (s: Settings) => localStorage.setItem(KEY, JSON.stringify(s));
export const signOut = () => localStorage.removeItem(KEY);

export class ApiError extends Error {
  status: number; code: string;
  constructor(status: number, code: string, message: string) { super(message); this.status = status; this.code = code; }
}

// Leaves for the sign-in page and never settles, so the page stops where it was.
function toSignIn(): Promise<never> {
  location.href = `signin.html?next=${encodeURIComponent(location.pathname.split('/').pop() + location.search)}`;
  return new Promise(() => {});
}

// `given` settings are being tried on the sign-in page: a 401 is then reported instead of redirected.
export async function call<T = unknown>(method: string, path: string, body?: unknown, given?: Settings): Promise<T> {
  const s = given || settings();
  if (!given && (!s.base || !s.token)) return toSignIn();
  let res: Response;
  try {
    res = await fetch((s.base || '').replace(/\/+$/, '') + path, {
      method, body: body === undefined ? undefined : JSON.stringify(body),
      headers: { Authorization: `Bearer ${s.token}`, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
    });
  } catch {
    throw new ApiError(0, 'network', '');
  }
  const text = await res.text();
  let data: { error?: { code?: string; message?: string } } | null = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* not JSON: reported below by status */ }
  if (res.status === 401 && !given) return toSignIn();
  if (!res.ok) throw new ApiError(res.status, data?.error?.code || 'internal', data?.error?.message || `HTTP ${res.status}`);
  return data as T;
}

export const get = <T>(path: string) => call<T>('GET', path);
export const post = <T>(path: string, body?: unknown) => call<T>('POST', path, body);
