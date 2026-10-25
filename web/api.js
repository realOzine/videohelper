// The API address and token live in this browser only; the token travels in the Authorization header, never a cookie.
const KEY = 'vh-api';

export function settings() {
  try { return JSON.parse(localStorage.getItem(KEY)) || {}; } catch { return {}; }
}
export const saveSettings = (s) => localStorage.setItem(KEY, JSON.stringify(s));
export const signOut = () => localStorage.removeItem(KEY);

export class ApiError extends Error {
  constructor(status, code, message) { super(message); this.status = status; this.code = code; }
}

// Leaves for the sign-in page and never settles, so the page stops where it was.
function toSignIn() {
  location.href = `signin.html?next=${encodeURIComponent(location.pathname.split('/').pop() + location.search)}`;
  return new Promise(() => {});
}

// `given` settings are being tried on the sign-in page: a 401 is then reported instead of redirected.
export async function call(method, path, body, given) {
  const s = given || settings();
  if (!given && (!s.base || !s.token)) return toSignIn();
  let res;
  try {
    res = await fetch(s.base.replace(/\/+$/, '') + path, {
      method, body: body === undefined ? undefined : JSON.stringify(body),
      headers: { Authorization: `Bearer ${s.token}`, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
    });
  } catch {
    throw new ApiError(0, 'network', '');
  }
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* not JSON: reported below by status */ }
  if (res.status === 401 && !given) return toSignIn();
  if (!res.ok) throw new ApiError(res.status, data?.error?.code || 'internal', data?.error?.message || `HTTP ${res.status}`);
  return data;
}

export const get = (path) => call('GET', path);
export const post = (path, body = undefined) => call('POST', path, body);
