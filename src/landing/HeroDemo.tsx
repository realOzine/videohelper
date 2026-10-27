// The hero's live workbench, staged on a soft ink panel. It runs itself: types a query, shows the gallery,
// points at the best moment and opens it in detail, then the next query. Any touch hands it to the visitor;
// eight quiet seconds later it carries on. With reduced motion it holds still on a detail view.
import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from 'react';
import { t } from '../i18n.ts';
import { cx, Icon, Mark } from '../ui.tsx';
import { DEMO, SETS, type DemoMoment, type DemoVideo } from './data.ts';
import { advance, CYCLE, DIR, phase, take, TYPE, type Clock } from './timeline.ts';
import { media, rk } from './util.ts';

const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
const best = (m: DemoMoment) => Math.min(...m.chs.map((c) => c[1]));
const secs = (x: string) => { const [mm, ss] = x.split(':'); return Number(mm) * 60 + Number(ss); };
const clockOf = (s: number) => { const mm = Math.floor(s / 60), r = s - mm * 60; return `${String(mm).padStart(2, '0')}:${(r < 10 ? '0' : '') + r.toFixed(1)}`; };
const ranks = (m: DemoMoment) => m.chs.map(([c, n]) => rk(c, n)).join(' · ');
const badge = 'pointer-events-none absolute z-10 rounded-[3px] bg-ink/80 px-[5px] text-[11px] leading-[17px]';

type Hit = DemoMoment & { video: DemoVideo };

export default function HeroStage({ l }: { l: 0 | 1 }) {
  const [c, setC] = useState<Clock>(() => ({ q: 0, t: reduce ? DIR : 0, sel: 0, hold: 0 }));
  const [own, setOwn] = useState<string | null>(null);
  const [miss, setMiss] = useState('');
  const [pt, setPt] = useState({ x: 0, y: 0, ok: false });
  const root = useRef<HTMLDivElement>(null), content = useRef<HTMLDivElement>(null), target = useRef<HTMLButtonElement>(null), visible = useRef(true);

  // one tick every 100 ms, only while the stage is on screen and the tab is visible
  useEffect(() => {
    if (reduce) return;
    const io = new IntersectionObserver(([e]) => { visible.current = e.isIntersecting; });
    io.observe(root.current!);
    const timer = setInterval(() => { if (visible.current && !document.hidden) setC((x) => advance(x, 100, Date.now(), DEMO.length)); }, 100);
    return () => { clearInterval(timer); io.disconnect(); };
  }, []);
  useEffect(() => { setOwn(null); setMiss(''); }, [c.q]);
  const grab = (next: Partial<Clock>) => setC((x) => take(x, Date.now(), next));

  const ph = phase(c.t), held = Date.now() < c.hold;
  const set = SETS[c.q], vids = DEMO[c.q];
  const flat: Hit[] = vids.flatMap((v) => v.hits.map((h) => ({ ...h, video: v }))).sort((a, b) => best(a) - best(b));
  const cur = flat[Math.min(c.sel, flat.length - 1)];
  const indexOf = (h: DemoMoment) => flat.findIndex((f) => f.clip === h.clip && f.tc === h.tc);

  // the pointer aims at the best card, measured once the gallery is laid out
  useLayoutEffect(() => {
    if (ph !== 'gallery' || !target.current || !content.current) return;
    const r = target.current.getBoundingClientRect(), box = content.current.getBoundingClientRect();
    setPt({ x: r.left - box.left + r.width * 0.45, y: r.top - box.top + r.height * 0.5, ok: true });
  }, [c.q, ph, l]);
  const moving = !held && !reduce && pt.ok && c.t >= 2600 && c.t < DIR;
  const clicking = moving && c.t >= 3800;
  const box = content.current?.getBoundingClientRect();
  const px = moving ? pt.x : (box?.width ?? 600) - 60, py = moving ? pt.y : (box?.height ?? 360) - 40;

  const p = ph === 'detail' ? Math.max(0, Math.min(1, (c.t - DIR - 200) / (CYCLE - DIR - 600))) : 0;
  const [a, b] = cur.range.split(' – ').map(secs);
  const typing = ph === 'typing' && !held;
  const query = own ?? (typing ? set.q[l].slice(0, Math.ceil((c.t / TYPE) * set.q[l].length)) : set.q[l]);
  const first = Math.min(Math.max(c.q - 1, 0), 2);

  const search = (e: FormEvent) => {
    e.preventDefault();
    const v = (own ?? set.q[l]).trim().toLowerCase();
    const hit = SETS.map((s, i) => ({ i, n: s.keys.filter((k) => v.includes(k)).length })).sort((x, y) => y.n - x.n)[0];
    if (hit.n) { grab({ q: hit.i, t: TYPE + 1000, sel: 0 }); setOwn(null); } else { setMiss(t('app.nomatch', { q: v })); grab({}); }
  };

  return (
    <div ref={root} className="relative flex h-full flex-col overflow-hidden rounded-[36px] bg-stage px-5 pt-5 pb-[84px] shadow-[inset_0_1px_0_rgba(255,255,255,0.03),0_40px_100px_-40px_rgba(0,0,0,0.55)] max-[900px]:rounded-3xl max-[900px]:px-3 max-[900px]:pt-3 max-[900px]:pb-16">
      <div className="scanlines pointer-events-none absolute inset-0" aria-hidden />

      {/* the timeline ruler; its playhead is the demo's own clock */}
      <div className="pointer-events-none absolute inset-x-10 bottom-7 h-11 max-[900px]:inset-x-4 max-[900px]:bottom-4" aria-hidden>
        <div className="ruler absolute inset-x-0 top-0 h-3.5 border-t border-fg/18" />
        <div className="absolute inset-x-0 top-5 flex justify-between text-[11px] leading-[14px] text-fg/40 max-[900px]:[&>*:nth-child(even)]:hidden">{Array.from({ length: 11 }, (_, i) => <span key={i}>{`00:${String(i).padStart(2, '0')}`}</span>)}</div>
        <span className="absolute -top-2.5 -ml-px h-[26px] w-0.5 bg-red" style={{ left: `${(c.t / CYCLE) * 100}%`, transition: c.t < 200 ? 'none' : 'left 100ms linear' }}>
          <i className="absolute -top-1 -left-1 size-2.5 rounded-full bg-red shadow-[0_0_0_3px_var(--color-stage)]" />
        </span>
      </div>

      <div className="@container relative flex min-h-[540px] w-full flex-1 flex-col overflow-hidden rounded-[14px] bg-ink text-sm leading-5 text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.06),0_40px_90px_rgba(0,0,0,0.5),0_0_0_1px_rgba(255,255,255,0.04)] max-[900px]:h-[500px]"
        onPointerDown={() => { if (!held) grab({}); }}>
        <div className="flex h-[46px] flex-none items-center gap-1 border-b border-ink-3 bg-ink-2 px-3">
          <span className="mr-2.5 inline-flex items-center gap-[7px] font-semibold @max-xl:hidden"><Mark className="size-[18px]" />VideoHelper</span>
          <span className="h-[30px] rounded-md bg-ink-3 px-[11px] leading-[30px] font-semibold">{t('app.search')}</span>
          <span className="h-[30px] px-[11px] leading-[30px] text-fg-soft">{t('app.lib')} 8</span>
          <span className="h-[30px] px-[11px] leading-[30px] text-fg-soft">{t('app.proc')} <b className="text-red">2</b></span>
        </div>

        <form role="search" onSubmit={search} className="flex h-[58px] flex-none items-center gap-2 px-3 py-2.5">
          <label className="flex h-[38px] min-w-0 flex-1 items-center gap-2 rounded-lg bg-ink-2 px-2.5">
            <Icon.search className="size-[15px] flex-none text-fg-soft" />
            <span className="sr-only">{t('app.qlabel')}</span>
            <input id="demo-q" value={query} onChange={(e) => { setOwn(e.target.value); grab({}); }} onFocus={() => grab({})} autoComplete="off" className="min-w-0 flex-1 bg-transparent outline-0" />
            {typing && <span className="h-[17px] w-px animate-beat bg-fg" aria-hidden />}
          </label>
          <button type="submit" className="h-[38px] rounded-lg bg-fg px-[13px] font-semibold text-ink transition-colors hover:bg-white">{t('app.go')}</button>
        </form>

        <div className="flex h-10 flex-none items-center gap-1.5 px-3 pb-2.5">
          <div className="flex min-w-0 flex-1 gap-1.5 overflow-hidden @max-xl:[&>*:nth-child(n+3)]:hidden">
            {SETS.map((s, i) => ({ s, i })).slice(first, first + 4).map(({ s, i }) => (
              <button key={i} type="button" onClick={() => grab({ q: i, t: TYPE + 1000, sel: 0 })}
                className={cx('h-7 flex-none rounded-full px-2.5 text-[12.5px] transition-colors duration-200', i === c.q ? 'bg-red-fill font-medium text-white' : 'bg-ink-3 hover:bg-ink-4')}>
                {s.q[l]}
              </button>
            ))}
          </div>
          <div role="group" aria-label={t('demo.views')} className="flex flex-none gap-0.5 rounded-[7px] bg-ink-2 p-0.5">
            {([['gallery', 'demo.dir', TYPE + 1000], ['detail', 'demo.det', DIR]] as const).map(([v, k, at]) => (
              <button key={v} type="button" aria-pressed={(ph === 'detail') === (v === 'detail')} onClick={() => grab({ t: at })}
                className="h-6 rounded-[5px] px-2 text-xs font-medium text-fg-soft transition-colors aria-pressed:bg-ink-3 aria-pressed:text-fg">{t(k)}</button>
            ))}
          </div>
        </div>

        <div ref={content} className="relative min-h-0 flex-1">
          {/* placeholder rows while the query is being typed */}
          <div aria-hidden className={cx('absolute inset-0 px-3 pt-1 transition-opacity duration-300', typing ? 'opacity-100' : 'opacity-0')}>
            <div className="h-3.5 w-36 rounded bg-ink-2" />
            <div className="mt-3 grid grid-cols-4 gap-3 @max-xl:grid-cols-2">{[0, 1, 2, 3].map((i) => <div key={i} className="aspect-video rounded-md bg-ink-2" />)}</div>
            <div className="mt-8 h-3.5 w-28 rounded bg-ink-2" />
          </div>

          {/* gallery: one row per video */}
          <div className={cx('absolute inset-0 overflow-hidden px-3 pt-1 transition-opacity duration-300', ph === 'gallery' ? 'opacity-100' : 'pointer-events-none opacity-0')}>
            {miss && <p className="mb-2 text-fg-soft">{miss}</p>}
            {vids.slice(0, 3).map((v, vi) => {
              let k = vids.slice(0, vi).reduce((n, x) => n + x.hits.length, 0);
              return (
                <div key={v.title[0]} className="mb-3.5">
                  <div className="mb-2 flex items-baseline gap-2"><b className="font-semibold">{v.title[l]}</b><span className="text-xs text-faint">{v.dur}</span><span className="h-[18px] rounded-full bg-ink-3 px-1.5 text-[11px] leading-[18px]">{t(v.hits.length === 1 ? 'demo.hit' : 'demo.hits', { n: v.hits.length })}</span></div>
                  <div className="grid grid-cols-4 gap-3 @max-xl:grid-cols-2 @max-xl:[&>*:nth-child(n+3)]:hidden">
                    {v.hits.slice(0, 4).map((h) => {
                      const gi = indexOf(h), shown = held || reduce || c.t >= TYPE + (k++) * 110, isTarget = gi === 0;
                      const ring = (isTarget && !held && c.t >= 3600) || (held && gi === c.sel);
                      return (
                        <button key={h.tc} ref={isTarget ? target : undefined} type="button" onClick={(e) => { e.stopPropagation(); grab({ t: DIR, sel: gi }); }}
                          className={cx('group block text-left transition-[opacity,transform] duration-[450ms] ease-out', shown ? 'translate-y-0 opacity-100' : 'translate-y-2.5 opacity-0')}>
                          <span className={cx('relative block overflow-hidden rounded-md transition-shadow duration-200', ring && 'shadow-[0_0_0_2px_var(--color-red)]')}>
                            <img src={media(`${h.clip}.jpg`)} alt="" width={720} height={405} style={{ objectPosition: h.pos }}
                              className={cx('block aspect-video w-full object-cover transition-transform duration-300 group-hover:scale-105', isTarget && clicking && 'scale-105')} />
                            <span className={cx(badge, 'right-[5px] bottom-[7px]')}>{h.tc}</span>
                            <span className="absolute inset-x-0 bottom-0 h-[3px] bg-ink/60"><i className="absolute inset-y-0 min-w-[3px] bg-red" style={{ left: `${h.at}%`, width: `${h.len}%` }} /></span>
                          </span>
                          <span className="mt-1.5 block text-xs font-semibold">{h.range}</span>
                          <span className="block text-[11.5px] font-semibold text-red">{ranks(h)}</span>
                        </button>
                      );
                    })}
                  </div>
                </div>
              );
            })}
            {vids.length > 3 && <p className="text-xs text-faint">{t(vids.length === 4 ? 'demo.more1' : 'demo.more', { n: vids.length - 3 })}</p>}
          </div>

          {/* detail: player, lanes and the ranked list */}
          <div className={cx('absolute inset-0 grid grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)] gap-3 px-3 pb-3.5 transition-[opacity,transform] duration-[400ms] ease-out @max-xl:grid-cols-1',
            ph === 'detail' ? 'scale-100 opacity-100' : 'pointer-events-none scale-[.985] opacity-0')}>
            <div className="flex min-w-0 flex-col gap-2">
              <button type="button" onClick={() => grab({ t: TYPE + 1000 })} className="inline-flex h-5 items-center gap-1 self-start text-xs font-medium text-fg-soft transition-colors hover:text-fg">‹ {t('demo.back')}</button>
              <div className="relative overflow-hidden rounded-md bg-black">
                <video key={cur.clip + cur.tc} src={media(`${cur.clip}.mp4`)} poster={media(`${cur.clip}.jpg`)} muted loop playsInline autoPlay={!reduce} preload="metadata"
                  aria-label={cur.video.title[l]} className="block aspect-video w-full object-cover" style={{ objectPosition: cur.pos }} />
                <span className="absolute top-2 left-2 rounded bg-ink px-[7px] py-px text-xs">{clockOf(a + p * (b - a))}</span>
              </div>
              <div className="flex justify-between gap-2"><b className="truncate font-semibold">{cur.video.title[l]}</b><span className="flex-none text-fg-soft">{cur.range}</span></div>
              <div className="grid grid-cols-[auto_1fr] items-center gap-x-2.5 gap-y-1.5 text-[13px] text-fg-soft">
                {(['visual', 'speech', 'audio'] as const).map((ch) => (
                  <Lane key={ch} name={t(`ch.${ch}`)} head={cur.at + p * cur.len}
                    hits={flat.filter((f) => f.video === cur.video && f.chs.some((x) => x[0] === ch)).map((f) => ({ at: f.at, len: f.len, on: f === cur }))} />
                ))}
              </div>
            </div>
            <div className="@max-xl:hidden">
              <div className="mb-1.5 text-fg-soft">{t('app.count', { n: flat.length })}</div>
              <div className="grid gap-1">
                {flat.slice(0, 4).map((f, i) => (
                  <button key={f.clip + f.tc} type="button" aria-pressed={f === cur} onClick={(e) => { e.stopPropagation(); grab({ t: DIR, sel: i }); }}
                    className="grid grid-cols-[80px_1fr] items-center gap-2 rounded-md p-[5px] text-left transition-colors hover:bg-ink-2 aria-pressed:bg-ink-3">
                    <img src={media(`${f.clip}.jpg`)} alt="" width={720} height={405} style={{ objectPosition: f.pos }} className="block aspect-[16/10] w-20 rounded object-cover" />
                    <span className="min-w-0"><b className="block truncate text-[13px] font-semibold">{f.video.title[l]}</b><span className="block text-xs text-fg-soft">{f.range}</span><span className="text-xs font-semibold text-red">{ranks(f)}</span></span>
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* the demo's pointer and its click */}
          <svg aria-hidden width="22" height="22" viewBox="0 0 24 24" className="pointer-events-none absolute z-20 drop-shadow-[0_2px_4px_rgba(0,0,0,0.5)]"
            style={{ left: px, top: py, opacity: moving ? 1 : 0, transform: `scale(${clicking ? 0.82 : 1})`, transformOrigin: '4px 3px', transition: 'left .9s cubic-bezier(.2,.8,.2,1), top .9s cubic-bezier(.2,.8,.2,1), opacity .25s, transform .15s' }}>
            <path d="M4 3l14 8-6.2 1.6L8.8 19z" fill="#f3ede2" stroke="#0f0e15" strokeWidth="1.4" strokeLinejoin="round" />
          </svg>
          <span aria-hidden className="pointer-events-none absolute -mt-2 -ml-2 size-[22px] rounded-full border-2 border-red transition-[opacity,transform] duration-300"
            style={{ left: px, top: py, opacity: clicking ? 1 : 0, transform: `scale(${clicking ? 1.8 : 0.4})` }} />
        </div>
      </div>
    </div>
  );
}

function Lane({ name, hits, head }: { name: string; hits: { at: number; len: number; on: boolean }[]; head: number }) {
  return (
    <>
      <span>{name}</span>
      <span className="relative h-3.5 rounded-[3px] bg-ink-3">
        {hits.map((h, i) => <i key={i} className={cx('absolute min-w-1 rounded-sm', h.on ? 'inset-y-0 bg-red' : 'inset-y-[3px] bg-hit')} style={{ left: `${h.at}%`, width: `${h.len}%` }} />)}
        <span className="absolute -inset-y-[3px] w-0.5 bg-fg transition-[left] duration-100 ease-linear" style={{ left: `${head}%` }} />
      </span>
    </>
  );
}
