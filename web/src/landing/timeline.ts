// The hero demo's clock. One round per query: type it, show the gallery, click the best card, play it in detail.
// Pure, so node --test can check it.

export const TYPE = 1400;   // typing the query
export const DIR = 4400;    // the gallery shows until here; the pointer moves at 2600 and clicks at 3800
export const CYCLE = 10500; // the detail plays until here, then the next query
export const HOLD = 8000;   // after the visitor's last touch the demo waits this long before resuming

export interface Clock { q: number; t: number; sel: number; hold: number }

export type Phase = 'typing' | 'gallery' | 'detail';
export const phase = (t: number): Phase => (t < TYPE ? 'typing' : t < DIR ? 'gallery' : 'detail');

// Advances the clock by `ms`. While the visitor holds it, the gallery stays put and the detail plays to its end
// without moving on; otherwise the round runs and wraps to the next of `queries`.
export function advance(c: Clock, ms: number, now: number, queries: number): Clock {
  if (now < c.hold) return c.t >= DIR ? { ...c, t: Math.min(c.t + ms, CYCLE - 100) } : c;
  const t = c.t + ms;
  return t < CYCLE ? { ...c, t } : { q: (c.q + 1) % queries, t: 0, sel: 0, hold: 0 };
}

// The visitor took over: jump to a phase and hold the demo there.
export const take = (c: Clock, now: number, next: Partial<Clock>): Clock => ({ ...c, ...next, hold: now + HOLD });
