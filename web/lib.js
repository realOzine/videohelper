// Pure helpers for the workbench pages: no DOM, no network, so `node --test web/` can load them.

export const esc = (v) => String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);

// mm:ss, or mm:ss.t with tenths; minutes keep counting past the hour, like an editor's timecode
export const tc = (ms, tenths = false) => {
  const d = Math.floor(Math.max(0, ms) / 100), m = Math.floor(d / 600), s = String(Math.floor(d / 10) % 60).padStart(2, '0');
  return `${String(m).padStart(2, '0')}:${s}${tenths ? `.${d % 10}` : ''}`;
};

// The dictionary key of a pipeline stage: probe, prepare_media, asr, finalize, or the channel of an embed stage.
export const stageKind = (stage) => (stage.startsWith('embed:') ? stage.split(':')[1] : stage);

export const CHANNELS = ['visual', 'speech', 'audio'];

// Groups stages the way they run: media steps first, one lane per channel (speech after asr), then the rest.
export function pipeline(stages) {
  const lanes = Object.fromEntries(CHANNELS.map((c) => [c, []])), before = [], after = [];
  for (const s of stages) {
    const k = stageKind(s.stage);
    if (k === 'asr') lanes.speech.unshift(s);
    else if (k in lanes) lanes[k].push(s);
    else (k === 'probe' || k === 'prepare_media' ? before : after).push(s);
  }
  before.sort((a, b) => (a.stage === 'probe' ? -1 : b.stage === 'probe' ? 1 : 0));
  return { before, lanes: Object.entries(lanes).filter(([, l]) => l.length), after };
}

// Whether each channel can be searched yet: done, failed, waiting, or none when the video has no such stage.
export function channelState(stages) {
  return Object.fromEntries(CHANNELS.map((c) => {
    const mine = stages.filter((s) => stageKind(s.stage) === c || (c === 'speech' && s.stage === 'asr'));
    if (!mine.length) return [c, 'none'];
    if (mine.every((s) => s.status === 'done')) return [c, 'done'];
    return [c, mine.some((s) => s.status === 'failed') ? 'failed' : 'waiting'];
  }));
}

// Byte range [start, end) of upload part n (1-based), as the server planned it.
export const partRange = (n, partSize, size) => [(n - 1) * partSize, Math.min(n * partSize, size)];

// Runs fn over items with at most `limit` in flight; the first failure stops new work and rejects.
export async function eachLimit(items, limit, fn) {
  let next = 0;
  const worker = async () => {
    while (next < items.length) {
      const i = next++;
      try { await fn(items[i], i); } catch (err) { next = items.length; throw err; }
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
}

// One tile of a thumbnail sprite, cropped by the viewBox and scaled to fill its box.
export const tile = (t, label = '') => `<svg viewBox="${t.x} ${t.y} ${t.w} ${t.h}" preserveAspectRatio="xMidYMid slice" role="img" aria-label="${esc(label)}"><image href="${esc(t.url)}"/></svg>`;

// Axis ticks for a timeline of `dur` ms: every 10, 5 or 2 minutes, 30 s, or 10 s for clips under a minute.
export function ticks(dur) {
  const step = dur > 1800000 ? 600000 : dur > 600000 ? 300000 : dur > 180000 ? 120000 : dur > 60000 ? 30000 : 10000, out = [];
  for (let t = 0; t <= dur - step / 2; t += step) out.push(t);
  return out;
}

// Only a page name of this app may follow sign-in, never another site.
export const safeNext = (next) => (/^[a-z]+\.html(\?[^#]*)?$/.test(next || '') ? next : 'search.html');
