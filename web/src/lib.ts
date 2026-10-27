// Pure helpers for the workbench: no DOM, no network, so `node --test` loads them directly.

export type Channel = 'visual' | 'speech' | 'audio';
export const CHANNELS: Channel[] = ['visual', 'speech', 'audio'];

export interface Stage { stage: string; version?: string; status: string; attempt: number; required?: boolean; progress?: number | null; error?: string; updated_at: string }

// mm:ss, or mm:ss.t with tenths; minutes keep counting past the hour, like an editor's timecode
export const tc = (ms: number, tenths = false) => {
  const d = Math.floor(Math.max(0, ms) / 100), m = Math.floor(d / 600), s = String(Math.floor(d / 10) % 60).padStart(2, '0');
  return `${String(m).padStart(2, '0')}:${s}${tenths ? `.${d % 10}` : ''}`;
};

// The dictionary key of a pipeline stage: probe, prepare_media, asr, finalize, or the channel of an embed stage.
export const stageKind = (stage: string) => (stage.startsWith('embed:') ? stage.split(':')[1] : stage);

// Groups stages the way they run: media steps first, one lane per channel (speech after asr), then the rest.
export function pipeline<S extends Stage>(stages: S[]) {
  const lanes: Record<string, S[]> = { visual: [], speech: [], audio: [] }, before: S[] = [], after: S[] = [];
  for (const s of stages) {
    const k = stageKind(s.stage);
    if (k === 'asr') lanes.speech.unshift(s);
    else if (k in lanes) lanes[k].push(s);
    else (k === 'probe' || k === 'prepare_media' ? before : after).push(s);
  }
  before.sort((a, b) => (a.stage === 'probe' ? -1 : b.stage === 'probe' ? 1 : 0));
  return { before, lanes: Object.entries(lanes).filter(([, l]) => l.length), after };
}

export type ChannelState = 'done' | 'failed' | 'waiting' | 'none';
// Whether each channel can be searched yet: done, failed, waiting, or none when the video has no such stage.
export function channelState(stages: Stage[]): Record<Channel, ChannelState> {
  const state = (c: Channel): ChannelState => {
    const mine = stages.filter((s) => stageKind(s.stage) === c || (c === 'speech' && s.stage === 'asr'));
    if (!mine.length) return 'none';
    if (mine.every((s) => s.status === 'done')) return 'done';
    return mine.some((s) => s.status === 'failed') ? 'failed' : 'waiting';
  };
  return { visual: state('visual'), speech: state('speech'), audio: state('audio') };
}

// Byte range [start, end) of upload part n (1-based), as the server planned it.
export const partRange = (n: number, partSize: number, size: number) => [(n - 1) * partSize, Math.min(n * partSize, size)];

// Runs fn over items with at most `limit` in flight; the first failure stops new work and rejects.
export async function eachLimit<T>(items: T[], limit: number, fn: (item: T, i: number) => Promise<void>) {
  let next = 0;
  const worker = async () => {
    while (next < items.length) {
      const i = next++;
      try { await fn(items[i], i); } catch (err) { next = items.length; throw err; }
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
}

// Axis ticks for a timeline of `dur` ms: every 10, 5 or 2 minutes, 30 s, or 10 s for clips under a minute.
export function ticks(dur: number) {
  const step = dur > 1800000 ? 600000 : dur > 600000 ? 300000 : dur > 180000 ? 120000 : dur > 60000 ? 30000 : 10000, out: number[] = [];
  for (let t = 0; t <= dur - step / 2; t += step) out.push(t);
  return out;
}

// Moments grouped by video, keeping each moment's index; videos come in the order of their best moment,
// since the API returns moments ranked.
export function byVideo<M extends { video_id: string }>(ms: M[]) {
  const groups = new Map<string, { id: string; items: { m: M; i: number }[] }>();
  ms.forEach((m, i) => {
    const g = groups.get(m.video_id) ?? { id: m.video_id, items: [] };
    g.items.push({ m, i });
    groups.set(m.video_id, g);
  });
  return [...groups.values()];
}

// A search that reaches several videos opens on the gallery; one video opens straight on the detail.
export const defaultView = (ms: { video_id: string }[]): 'dir' | 'detail' => (new Set(ms.map((m) => m.video_id)).size > 1 ? 'dir' : 'detail');

// Only a page name of this app may follow sign-in, never another site.

// Splits a translated string on <b>…</b> and <code>…</code> so markup in the dictionary renders without innerHTML.
export function marks(s: string): { tag: '' | 'b' | 'code'; text: string }[] {
  return s.split(/(<b>.*?<\/b>|<code>.*?<\/code>)/).filter(Boolean).map((part) => {
    const m = /^<(b|code)>(.*)<\/\1>$/.exec(part);
    return m ? { tag: m[1] as 'b' | 'code', text: m[2] } : { tag: '', text: part };
  });
}

// A viewer's model choice for search, kept only where it differs from the server's defaults: one space per channel
// (space_overrides) and a query multiplier where a channel is switched on or off (channel_weights).
export interface ModelPrefs { spaces: Record<string, string>; weights: Record<string, number> }
export function modelPrefs(chs: { channel: string; default_space_ids: string[]; default_weight: number }[], pick: Record<string, string>, on: Record<string, boolean>): ModelPrefs {
  const p: ModelPrefs = { spaces: {}, weights: {} };
  for (const c of chs) {
    if (pick[c.channel] && pick[c.channel] !== c.default_space_ids[0]) p.spaces[c.channel] = pick[c.channel];
    if (on[c.channel] !== c.default_weight > 0) p.weights[c.channel] = on[c.channel] ? 1 : 0;
  }
  return p;
}
export const searchFields = (p: ModelPrefs) => ({
  ...(Object.keys(p.spaces).length && { space_overrides: p.spaces }),
  ...(Object.keys(p.weights).length && { channel_weights: p.weights }),
});
