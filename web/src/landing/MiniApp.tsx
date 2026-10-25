// The mini workbench on the landing page: search the sample library, play a hit, open the library, watch processing.
import { useEffect, useRef, useState } from 'react';
import { t } from '../i18n.ts';
import { cx, Icon, Mark } from '../ui.tsx';
import { CLIP, SETS } from './data.ts';
import { media, rk, sec, span, title } from './util.ts';

type St = 'done' | 'run' | 'wait' | 'bad';
const ORDER = ['probe', 'prep', 'visual', 'asr', 'audio', 'speech', 'fin'] as const;
const PLAN = (): Record<string, St> => ({ probe: 'done', prep: 'done', visual: 'run', asr: 'run', audio: 'done', speech: 'wait', fin: 'wait' });
const PILL: Record<string, [string, string]> = { ready: ['bg-pos-bg text-pos-ink', 'st.ready'], run: ['bg-red-soft text-neg-ink', 'st.run'], bad: ['bg-red-soft text-red-text', 'st.bad'] };
const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
const side = 'flex h-9 items-center justify-between gap-2 rounded-md px-2.5 text-left text-fg-soft transition-colors duration-150 hover:bg-ink-3 hover:text-fg aria-[current=page]:bg-ink-3 aria-[current=page]:font-semibold aria-[current=page]:text-fg';

export default function MiniApp({ l }: { l: 0 | 1 }) {
  const [cur, setCur] = useState(0), [sel, setSel] = useState(0), [view, setView] = useState<'search' | 'lib' | 'proc'>('search');
  const [q, setQ] = useState(SETS[0].q[l]), [miss, setMiss] = useState('');
  const [lib, setLib] = useState<Record<string, string>>({ jelly: 'ready', reef: 'ready', surf: 'ready', rocket: 'ready', earth: 'ready', iss: 'run', sintel: 'ready', bunny: 'bad' });
  const [proc, setProc] = useState({ id: 'iss', st: PLAN(), pv: 0.64 });
  const [now, setNow] = useState(0);
  const av = useRef<HTMLVideoElement>(null);
  const set = SETS[cur], [id, chs, a, b] = set.rows[sel], len = CLIP[id][3];

  useEffect(() => { setQ(SETS[cur].q[l]); }, [l, cur]);
  // a new hit seeks the clip to its start and plays it
  useEffect(() => {
    const v = av.current;
    if (!v) return;
    const go = () => { try { v.currentTime = a; } catch { /* not seekable yet */ } if (!reduce) v.play().catch(() => {}); };
    if (v.readyState >= 1) go(); else v.addEventListener('loadedmetadata', go, { once: true });
  }, [id, a, view]);

  // shortcut: simulates the processing a real upload goes through; the full workbench polls GET /v1/videos/{id}
  const running = view === 'proc' && ORDER.some((k) => proc.st[k] !== 'done') && proc.st.asr !== 'bad';
  useEffect(() => {
    if (!running) return;
    const timer = setInterval(() => setProc((p) => {
      const st = { ...p.st };
      let pv = p.pv;
      if (st.visual === 'run') { pv = Math.min(1, pv + 0.06); if (pv >= 1) st.visual = 'done'; }
      else if (st.asr === 'run') { st.asr = 'done'; st.speech = 'run'; }
      else if (st.speech === 'run') { st.speech = 'done'; st.fin = 'run'; }
      else if (st.fin === 'run') { st.fin = 'done'; setLib((x) => ({ ...x, [p.id]: 'ready' })); }
      return { ...p, st, pv };
    }), 700);
    return () => clearInterval(timer);
  }, [running]);

  const pick = (i: number, s = 0) => { setCur(i); setSel(s); setMiss(''); };
  const search = (e: React.FormEvent) => {
    e.preventDefault();
    const v = q.trim().toLowerCase();
    const best = SETS.map((s, i) => ({ i, n: s.keys.filter((k) => v.includes(k)).length })).sort((x, y) => y.n - x.n)[0];
    if (best.n) pick(best.i); else setMiss(t('app.nomatch', { q: q.trim() }));
  };
  const open = (cid: string) => {
    if (lib[cid] === 'ready') {
      const i = Math.max(0, SETS.findIndex((x) => x.rows[0][0] === cid));
      pick(i, Math.max(0, SETS[i].rows.findIndex((r) => r[0] === cid))); setView('search'); return;
    }
    const st = PLAN();
    if (lib[cid] === 'bad') { st.visual = 'done'; st.asr = 'bad'; }
    setProc({ id: cid, st, pv: lib[cid] === 'bad' ? 1 : 0.64 }); setView('proc');
  };
  const busyCount = Object.values(lib).filter((s) => s !== 'ready').length;
  const done = ORDER.filter((k) => proc.st[k] === 'done').length;

  return (
    <div className="relative grid h-[640px] grid-cols-[190px_minmax(0,1fr)] overflow-hidden rounded-t-2xl bg-ink text-sm leading-5 text-fg shadow-[0_30px_80px_rgba(0,0,0,0.55)] max-xl:h-auto max-xl:grid-cols-1">
      <aside className="flex flex-col gap-0.5 bg-ink-2 px-2.5 py-4 max-xl:flex-row max-xl:flex-wrap max-xl:items-center max-xl:p-2.5">
        <div className="flex items-center gap-2 px-2 pt-0.5 pb-3.5 font-semibold max-sm:hidden"><Mark className="size-5" />VideoHelper</div>
        <button type="button" className={side} aria-current={view === 'search' ? 'page' : undefined} onClick={() => setView('search')}>{t('app.search')}</button>
        <button type="button" className={side} aria-current={view === 'lib' ? 'page' : undefined} onClick={() => setView('lib')}>{t('app.lib')}<span className="text-fg-soft">{Object.keys(CLIP).length}</span></button>
        <button type="button" className={side} aria-current={view === 'proc' ? 'page' : undefined} onClick={() => setView('proc')}>{t('app.proc')}{!!busyCount && <span className="text-[13px] font-semibold text-red">{busyCount}</span>}</button>
        <div className="mx-2.5 mt-4 mb-1.5 text-[13px] text-fg-soft max-xl:hidden">{t('app.projects')}</div>
        {['p1', 'p2', 'p3'].map((p) => <button key={p} type="button" className={cx(side, 'max-xl:hidden')} onClick={() => setView('lib')}>{t(`app.${p}`)}</button>)}
      </aside>
      <div className="flex min-w-0 flex-col overflow-hidden">
        <div className="flex items-center gap-2.5 border-b border-ink-3 px-4 py-3">
          <form id="qf" role="search" onSubmit={search} className="flex h-9 flex-1 items-center gap-2 rounded-lg bg-ink-2 px-3">
            <Icon.search className="size-4 flex-none text-fg-soft" />
            <label htmlFor="aq" className="sr-only">{t('app.qlabel')}</label>
            <input id="aq" type="search" autoComplete="off" value={q} onChange={(e) => setQ(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-0" />
          </form>
          <button type="submit" form="qf" className="h-9 rounded-lg bg-fg px-3.5 font-semibold text-ink transition-colors hover:bg-white">{t('app.go')}</button>
        </div>
        {view === 'search' && (
          <div className="flex flex-wrap gap-1.5 px-4 pt-2.5">
            {SETS.map((s, i) => (
              <button key={i} type="button" aria-pressed={i === cur} onClick={() => pick(i)}
                className="h-[30px] rounded-full bg-ink-3 px-2.5 text-[13px] transition-colors duration-150 hover:bg-ink-4 aria-pressed:bg-red aria-pressed:text-on-paper">{s.q[l]}</button>
            ))}
          </div>
        )}
        <div className="min-h-0 flex-1 overflow-auto px-4 pt-3.5 pb-4">
          {view === 'search' && (
            <div className="grid h-full grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)] gap-3.5 max-xl:h-auto max-xl:grid-cols-1">
              <div className="flex min-w-0 flex-col gap-2.5">
                <div className="relative aspect-video overflow-hidden rounded-lg bg-black">
                  <video ref={av} src={media(`${id}.mp4`)} poster={media(`${id}.jpg`)} aria-label={title(id, l)} muted loop playsInline autoPlay={!reduce} preload="metadata"
                    className="size-full object-cover" onTimeUpdate={(e) => setNow(e.currentTarget.currentTime)} />
                  <span className="absolute top-2.5 left-2.5 rounded-[4px] bg-ink px-2 py-0.5 text-[13px]">{sec(CLIP[id][2] + now)}</span>
                </div>
                <div className="flex justify-between gap-2.5"><b className="font-semibold">{title(id, l)}</b><span className="text-fg-soft">{span(id, a, b)}</span></div>
                <div className="grid grid-cols-[38px_1fr] items-center gap-x-2.5 gap-y-1.5 text-[13px] text-fg-soft">
                  {(['visual', 'speech', 'audio'] as const).map((c) => (
                    <LaneRow key={c} c={c} on={chs.some(([k]) => k === c)} a={a} b={b} len={len} now={now}
                      onSeek={(x) => { if (av.current) av.current.currentTime = Math.max(0, Math.min(len - 0.05, x * len)); }} />
                  ))}
                </div>
              </div>
              <div>
                <div className="mb-2 text-fg-soft">{miss || t('app.count', { n: set.rows.length })}</div>
                <div className="grid content-start gap-1.5">
                  {set.rows.map(([rid, rchs, ra, rb], i) => (
                    <button key={rid + i} type="button" aria-pressed={i === sel} onClick={() => setSel(i)}
                      className="grid grid-cols-[96px_1fr] items-center gap-2.5 rounded-lg p-1.5 text-left transition-colors duration-150 hover:bg-ink-2 aria-pressed:bg-ink-3">
                      <span className="relative aspect-[16/10] overflow-hidden rounded-[5px] bg-black"><img src={media(`${rid}.jpg`)} width={720} height={405} alt="" className="size-full object-cover" /></span>
                      <span><b className="block font-semibold">{title(rid, l)}</b><span className="text-fg-soft">{span(rid, ra, rb)}</span><br />
                        <span className="text-[13px] font-semibold text-red">{rchs.map(([c, n]) => rk(c, n)).join(' · ')}</span></span>
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}
          {view === 'lib' && (
            <div className="grid grid-cols-4 gap-3 max-xl:grid-cols-2">
              {Object.keys(CLIP).map((cid) => (
                <button key={cid} type="button" onClick={() => open(cid)} className="group text-left">
                  <span className="relative block aspect-[16/10] overflow-hidden rounded-lg bg-black">
                    <img src={media(`${cid}.jpg`)} width={720} height={405} alt="" className="size-full object-cover transition-transform duration-300 ease-out group-hover:scale-[1.03]" />
                    <span className={cx('absolute top-2 left-2 inline-flex h-[22px] items-center rounded-full px-2 text-xs font-semibold whitespace-nowrap', PILL[lib[cid]][0])}>{t(PILL[lib[cid]][1])}</span>
                  </span>
                  <b className="mt-2 block font-semibold">{title(cid, l)}</b>
                  <span className="text-fg-soft">{t(`app.${CLIP[cid][4]}`)} · {sec(CLIP[cid][3])}</span>
                </button>
              ))}
            </div>
          )}
          {view === 'proc' && (
            <div className="grid grid-cols-[260px_1fr] gap-[18px] max-xl:grid-cols-1">
              <div>
                <div className="relative aspect-[16/10] overflow-hidden rounded-lg bg-black"><img src={media(`${proc.id}.jpg`)} width={720} height={405} alt={title(proc.id, l)} className="size-full object-cover" /></div>
                <p className="mt-2.5 font-semibold">{title(proc.id, l)}</p>
                <p className="text-fg-soft">{done === 7 ? t('proc.ok') : proc.st.asr === 'bad' ? t('proc.bad') : t('proc.sub', { n: done })}</p>
              </div>
              <div className="grid content-start gap-2">
                {ORDER.map((k) => {
                  const v = proc.st[k];
                  return (
                    <div key={k} className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-1.5 rounded-lg bg-ink-2 px-3 py-2.5">
                      <span>{t(`stage.${k}`)}</span>
                      {v === 'bad' ? (
                        <button type="button" className="h-7 rounded-full bg-fg px-2.5 text-[13px] font-semibold text-ink transition-colors hover:bg-white"
                          onClick={() => { setProc((p) => ({ ...p, st: { ...p.st, asr: 'run' } })); setLib((x) => ({ ...x, [proc.id]: 'run' })); }}>{t('retry')}</button>
                      ) : <span className="text-[13px] text-fg-soft">{v === 'run' ? (k === 'visual' ? `${Math.round(proc.pv * 100)}%` : t('running')) : t(v === 'done' ? 'done' : 'wait')}</span>}
                      {v === 'run' && <span className="col-span-2 h-[5px] overflow-hidden rounded-[3px] bg-ink-4"><i className="block h-full origin-left bg-red transition-transform duration-500 ease-out" style={{ transform: `scaleX(${k === 'visual' ? proc.pv : 0.35})` }} /></span>}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function LaneRow({ c, on, a, b, len, now, onSeek }: { c: string; on: boolean; a: number; b: number; len: number; now: number; onSeek: (x: number) => void }) {
  return (
    <>
      <span>{t(`ch.${c}`)}</span>
      <span className="relative h-4 cursor-pointer rounded-[3px] bg-ink-3" onClick={(e) => { const r = e.currentTarget.getBoundingClientRect(); onSeek((e.clientX - r.left) / r.width); }}>
        {on && <i className="absolute inset-y-0 min-w-[5px] rounded-sm bg-red" style={{ left: `${a / len * 100}%`, width: `${(b - a) / len * 100}%` }} />}
        {c === 'visual' && <i className="absolute top-[3px] bottom-[3px] min-w-[5px] rounded-sm bg-hit" style={{ left: `${((a + 5) % len) / len * 100}%`, width: `${2.5 / len * 100}%` }} />}
        <span className="pointer-events-none absolute -top-1 -bottom-1 w-0.5 bg-fg transition-[left] duration-[120ms] ease-linear" style={{ left: `${(now / len * 100).toFixed(2)}%` }} />
      </span>
    </>
  );
}
