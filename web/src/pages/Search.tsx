// Search, seen two ways. The gallery lays the moments out one row per video; picking one opens the detail, where the
// hit lanes are the scrubber (one row per channel, the playhead through all three). Back returns to the gallery.
import { useCallback, useEffect, useRef, useState } from 'react';
import { get, post, type Moment, type Video } from '../api.ts';
import { t, useLang } from '../i18n.ts';
import { byVideo, CHANNELS, defaultView, modelName, modelPrefs, searchFields, tc, ticks, type ModelPrefs } from '../lib.ts';
import { newProject, panel, PanelHead, Shell, TitleRow, useNotice, useLibrary } from '../Shell.tsx';
import { btn, btnPrimary, Ch, cx, errText, Icon, Rich, select, TcBadge, Thumb } from '../ui.tsx';

interface Entry { id: string; v: Video; url: string; dur: number }
const emptyMoments: Moment[] = [];
interface ChannelModels {
  channel: string; default_space_ids: string[]; default_weight: number;
  models: { space_id: string; status: string; eligible_videos: number; indexed_videos: number }[];
}
// the viewer's model choice applies to every search in this browser until reset (a per-viewer convenience)
const PREFS = 'vh-models';
const prefs = ((): ModelPrefs => {
  try { const p = JSON.parse(localStorage.getItem(PREFS) || 'null'); if (p?.spaces && p?.weights) return p; } catch { /* blocked storage: defaults */ }
  return { spaces: {}, weights: {} };
})();
const custom = Object.keys(prefs.spaces).length + Object.keys(prefs.weights).length > 0;
type View = 'dir' | 'detail';
const params = new URLSearchParams(location.search);
const q = (params.get('q') || '').trim(), project = params.get('project') || '';
const viewOf = () => { const v = new URLSearchParams(location.search).get('view'); return v === 'dir' || v === 'detail' ? v : null; };
// the view and moment live in the URL, so the browser's Back leaves the detail for the gallery
const urlWith = (extra: Record<string, string>) => `?${new URLSearchParams({ q, ...(project && { project }), ...extra })}`;

export default function Search() {
  useLang();
  const { notice, fail } = useNotice();
  const { ws, reload } = useLibrary(fail);
  const [found, setFound] = useState<{ ms: Moment[]; skipped: string[] } | null>(null);
  const [picked, setPicked] = useState(-1);
  const [entry, setEntry] = useState<Entry | null>(null);
  const [playing, setPlaying] = useState(false);
  const [view, setView] = useState<View | null>(viewOf);
  const [fromDir, setFromDir] = useState<boolean>(!!history.state?.fromDir);
  const [last, setLast] = useState(-1);
  const [meta, setMeta] = useState<Record<string, Video>>({});
  const vid = useRef<HTMLVideoElement>(null), plot = useRef<HTMLSpanElement>(null), ph = useRef<HTMLSpanElement>(null), time = useRef<HTMLSpanElement>(null);
  const loaded = useRef(new Map<string, Promise<Entry>>()), pickedRef = useRef(-1), entryRef = useRef<Entry | null>(null), pending = useRef<number | null>(null);
  const ms = found?.ms ?? emptyMoments;

  useEffect(() => {
    if (!q) return;
    post<{ moments: Moment[]; skipped_channels: string[] }>('/v1/search', { query: q, project_id: project || undefined, limit: 50, ...searchFields(prefs) })
      .then((r) => setFound({ ms: r.moments, skipped: r.skipped_channels || [] }))
      .catch((err) => fail(custom ? new Error(t('mdl.failed', { msg: errText(err) })) : err));
  }, [fail]);

  // the playhead and clock follow the real video by direct DOM writes, so playback does not re-render the list
  const setT = useCallback((at: number, glide = false) => {
    const e = entryRef.current;
    if (!e || !ph.current || !time.current) return;
    const x = Math.max(0, Math.min(e.dur, at));
    ph.current.style.transition = glide ? 'transform 420ms var(--ease-out)' : 'none';
    ph.current.style.transform = `translateX(${(x / e.dur * 100).toFixed(3)}%)`;
    time.current.innerHTML = `<b>${tc(x)}</b> / ${tc(e.dur)}`;
    plot.current?.setAttribute('aria-valuenow', String(Math.round(x / 1000)));
    plot.current?.setAttribute('aria-valuetext', t('s.valuetext', { a: tc(x), b: tc(e.dur) }));
  }, []);
  const seek = useCallback((at: number, glide = false) => {
    const v = vid.current;
    if (v && v.readyState >= 1) v.currentTime = at / 1000; else pending.current = at;
    setT(at, glide);
  }, [setT]);

  const load = useCallback((id: string) => {
    if (!loaded.current.has(id)) {
      loaded.current.set(id, Promise.all([get<{ video: Video }>(`/v1/videos/${id}`), get<{ url: string }>(`/v1/videos/${id}/playback`)]).then(([d, p]) => ({
        id, v: d.video, url: p.url, dur: d.video.duration_ms || Math.max(...ms.filter((m) => m.video_id === id).map((m) => m.end_ms)),
      })));
    }
    return loaded.current.get(id)!;
  }, [ms]);

  const choose = useCallback(async (i: number, jump = true) => {
    const m = ms[i];
    pickedRef.current = i; setPicked(i);
    let e: Entry;
    try { e = await load(m.video_id); } catch (err) { loaded.current.delete(m.video_id); return fail(err); }
    if (pickedRef.current !== i) return;
    entryRef.current = e; setEntry(e);
    if (jump) requestAnimationFrame(() => seek(m.jump_ms, true));
  }, [ms, load, seek, fail]);

  useEffect(() => { if (ms.length) choose(Math.min(Number(params.get('m')) || 0, ms.length - 1)); }, [ms, choose]);
  // a new search opens on the gallery when it reaches several videos, on the detail when it reaches one
  useEffect(() => { if (found) setView((v) => v ?? defaultView(found.ms)); }, [found]);
  // the gallery's rows need each video's length to place the moment on it
  useEffect(() => {
    if (!ms.length) return;
    const ids = [...new Set(ms.map((x) => x.video_id))];
    Promise.all(ids.map((id) => get<{ video: Video }>(`/v1/videos/${id}`).then((d) => [id, d.video] as const).catch(() => null)))
      .then((rows) => setMeta(Object.fromEntries(rows.filter((r) => r !== null))));
  }, [ms]);
  useEffect(() => {
    const onPop = () => {
      const st = history.state || {};
      setView(viewOf() ?? defaultView(ms));
      setFromDir(!!st.fromDir);
      if (typeof st.m === 'number') choose(st.m);
      if (viewOf() !== 'detail') requestAnimationFrame(() => scrollTo(0, st.scroll || 0));
    };
    addEventListener('popstate', onPop);
    return () => removeEventListener('popstate', onPop);
  }, [ms, choose]);

  // a card opens the detail as a new history entry; the gallery's entry keeps its scroll position
  const open = (i: number) => {
    history.replaceState({ ...history.state, scroll: scrollY }, '');
    history.pushState({ m: i, fromDir: true }, '', urlWith({ view: 'detail', m: String(i) }));
    setView('detail'); setFromDir(true); setLast(i); choose(i);
    scrollTo(0, 0);
  };
  // switching by hand changes this search's view only, without a history entry
  const switchTo = (v: View) => {
    history.replaceState({ m: pickedRef.current }, '', urlWith({ view: v, ...(v === 'detail' && { m: String(Math.max(0, pickedRef.current)) }) }));
    setView(v); setFromDir(false);
  };

  const step = useCallback((d: number) => choose((pickedRef.current + d + ms.length) % ms.length), [choose, ms.length]);
  const toggle = useCallback(() => { const v = vid.current; if (v) (v.paused ? v.play().catch(fail) : v.pause()); }, [fail]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement;
      if (!ms.length || view !== 'detail' || el.closest('input, select, textarea') || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === 'j') step(1); else if (e.key === 'k') step(-1);
      else if (e.key === ' ' && !el.closest('button, a')) { e.preventDefault(); toggle(); }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [ms.length, view, step, toggle]);

  // seeking: a click or drag on the lanes moves the playhead; landing inside a hit selects that moment
  const seekAt = (e: React.PointerEvent) => {
    const r = plot.current!.getBoundingClientRect(), en = entryRef.current;
    if (!en) return;
    const at = (Math.max(0, Math.min(r.width, e.clientX - r.left)) / r.width) * en.dur;
    seek(at);
    const hit = ms.findIndex((x) => x.video_id === en.id && at >= x.start_ms && at < x.end_ms);
    if (hit >= 0 && hit !== pickedRef.current) choose(hit, false);
  };
  const onPlotKey = (e: React.KeyboardEvent) => {
    const en = entryRef.current, v = vid.current;
    if (!en || !v) return;
    const d = ({ ArrowLeft: -5000, ArrowRight: 5000, PageDown: -60000, PageUp: 60000 } as Record<string, number>)[e.key];
    const at = d ? v.currentTime * 1000 + d : e.key === 'Home' ? 0 : e.key === 'End' ? en.dur : null;
    if (at != null) { e.preventDefault(); seek(Math.max(0, Math.min(en.dur, at))); }
  };
  const follow = () => { const v = vid.current; if (!v) return; setT(v.currentTime * 1000); if (!v.paused) requestAnimationFrame(follow); };

  const m = picked >= 0 ? ms[picked] : null;
  const mine = entry ? ms.filter((x) => x.video_id === entry.id) : [];
  const pct = (x: number) => `${(x / (entry?.dur || 1) * 100).toFixed(3)}%`;
  const CH = Object.fromEntries(CHANNELS.map((c) => [c, t(`ch.${c}`)]));
  const note = found?.skipped.length ? ` ${t('s.skipped', { chs: found.skipped.map((c) => CH[c] || c).join(t('and')) })}` : '';
  const best = ms[0];
  const groups = byVideo(ms);

  return (
    <Shell page="search" query={q} ws={ws} notice={notice} onNewProject={() => newProject(reload, fail)}>
      <TitleRow title={q ? `“${q}”` : t('s.title')} sub={!q ? t('s.empty') : !found ? ' ' : !ms.length ? t('s.none') + note : (
        <><Rich k="s.sub" vars={{ n: ms.length, v: new Set(ms.map((x) => x.video_id)).size, title: best.video_title, tc: tc(best.start_ms), chs: best.channels.map((h) => CH[h.channel]).join(t('and')) }} />{note}</>
      )}>
        {q && <Models />}
        {ms.length > 0 && (
          <div role="group" aria-label={t('a.views')} className="flex gap-0.5 rounded-lg bg-ink-2 p-[3px]">
            {(['dir', 'detail'] as const).map((v) => (
              <button key={v} type="button" aria-pressed={view === v} onClick={() => switchTo(v)}
                className="inline-flex h-[26px] items-center justify-center gap-1.5 rounded-md px-2.5 text-[13px] font-medium text-fg-soft transition-colors hover:text-fg aria-pressed:bg-ink-3 aria-pressed:text-fg">
                {v === 'dir' ? <Icon.grid className="size-3.5" /> : <Icon.detail className="size-3.5" />}{t(v === 'dir' ? 'v.dir' : 'v.det')}
              </button>
            ))}
          </div>
        )}
        <label htmlFor="scope" className="sr-only">{t('h.scope')}</label>
        <select id="scope" className={select} value={project} onChange={(e) => { location.search = String(new URLSearchParams({ q, ...(e.target.value && { project: e.target.value }) })); }}>
          <option value="">{t('opt.all')}</option>
          {ws?.projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </TitleRow>

      {ms.length > 0 && view === 'dir' && (
        <div className="grid animate-enter gap-10">
          {groups.map((g) => {
            const v = meta[g.id], dur = v?.duration_ms;
            return (
              <section key={g.id} aria-labelledby={`g${g.id}`}>
                <header className="mb-3.5 flex flex-wrap items-baseline gap-x-3 gap-y-1 border-b border-ink-3 pb-2.5">
                  <h2 id={`g${g.id}`} className="text-[17px] font-semibold">{g.items[0].m.video_title}</h2>
                  {dur ? <span className="text-fg-soft">{tc(dur)}</span> : null}
                  <span className="inline-flex h-5 items-center rounded-full bg-ink-3 px-[7px] text-xs">{t(g.items.length === 1 ? 's.hit' : 's.hits', { n: g.items.length })}</span>
                  <a className="ml-auto inline-flex items-center gap-0.5 font-medium text-fg-soft transition-colors hover:text-fg" href={`video.html?id=${g.id}`}>{t('s.openvideo')}<Icon.chevron className="size-3" /></a>
                </header>
                <div className="grid grid-cols-4 gap-6 max-xl:grid-cols-3 max-md:grid-cols-2 max-xs:grid-cols-1">
                  {g.items.map(({ m: x, i }) => (
                    <button key={i} type="button" onClick={() => open(i)} className="group -m-2 flex flex-col justify-start rounded-xl p-2 text-left transition-colors duration-200 hover:bg-ink-2">
                      <span className={cx('block overflow-hidden rounded-lg', i === last && 'ring-2 ring-red')}>
                        <Thumb thumb={x.thumbnail} label={t('frame.of', { t: x.video_title })} className="w-full">
                          <span className="absolute inset-0 grid place-items-center opacity-0 transition-opacity duration-200 group-hover:opacity-100"><span className="grid size-11 place-items-center rounded-full bg-ink/70"><Icon.play className="size-4 text-fg" /></span></span>
                          <span className="absolute right-1.5 bottom-2 rounded-[4px] bg-ink/80 px-1.5 text-xs leading-5 text-white">{tc(x.jump_ms)}</span>
                          {dur ? <span className="absolute inset-x-0 bottom-0 h-[3px] bg-ink/60"><i className="absolute inset-y-0 min-w-[3px] bg-red" style={{ left: `${(x.start_ms / dur) * 100}%`, width: `${((x.end_ms - x.start_ms) / dur) * 100}%` }} /></span> : null}
                        </Thumb>
                      </span>
                      <span className="mt-2.5 flex justify-between gap-2"><span className="font-semibold">{tc(x.start_ms, true)} – {tc(x.end_ms, true)}</span><span className="text-xs text-faint">{t('s.nth', { n: i + 1 })}</span></span>
                      <span className="mt-1 flex flex-wrap gap-x-2.5 gap-y-1">{x.channels.map((h) => <Ch key={h.channel} c={h.channel}>{t('rank', { ch: CH[h.channel], n: h.rank })}</Ch>)}</span>
                      {x.snippet && <span className="mt-1.5 block truncate text-[13px] text-fg-soft">“{x.snippet}”</span>}
                    </button>
                  ))}
                </div>
              </section>
            );
          })}
        </div>
      )}

      {ms.length > 0 && view === 'detail' && fromDir && (
        <p className="-mt-1.5 mb-3.5">
          <button type="button" onClick={() => history.back()} className="inline-flex items-center gap-1 font-medium text-fg-soft transition-colors hover:text-fg">
            <Icon.chevron className="size-3 rotate-180" />{t('s.back', { n: groups.length })}
          </button>
        </p>
      )}

      {ms.length > 0 && view === 'detail' && (
        <div className="grid animate-enter grid-cols-12 items-start gap-6">
          <section aria-label={t('a.player')} className="col-span-7 min-w-0 overflow-hidden rounded-[10px] bg-ink-2 shadow-edge max-md:col-span-12">
            <div className="relative aspect-video bg-black">
              <video ref={vid} src={entry?.url} playsInline preload="metadata" className="block size-full"
                onLoadedMetadata={(e) => { if (pending.current != null) { e.currentTarget.currentTime = pending.current / 1000; pending.current = null; } }}
                onPlay={() => { setPlaying(true); requestAnimationFrame(follow); }} onPause={() => setPlaying(false)} onSeeked={follow} />
            </div>
            <div className="flex min-h-14 flex-wrap items-center gap-3.5 border-b border-ink-3 px-4 py-2">
              <button type="button" onClick={toggle} aria-pressed={playing} aria-label={playing ? t('pause') : t('play')} aria-keyshortcuts="Space"
                className="grid size-8 flex-none place-items-center rounded-full bg-red-fill text-white transition-[transform,background-color] duration-150 ease-out hover:bg-red active:scale-[.92]">
                {playing ? <Icon.pause className="size-3.5" /> : <Icon.play className="size-3.5" />}
              </button>
              <span ref={time} className="whitespace-nowrap text-fg-soft [&_b]:font-medium [&_b]:text-fg" />
              <span className="flex-1" />
              <div className="flex gap-2">
                <button type="button" className={btn} onClick={() => step(-1)} aria-keyshortcuts="K">{t('b.prev')} <kbd className="ml-0.5 font-sans text-[11px] text-faint max-xs:hidden">K</kbd></button>
                <button type="button" className={btn} onClick={() => step(1)} aria-keyshortcuts="J">{t('b.next')} <kbd className="ml-0.5 font-sans text-[11px] text-faint max-xs:hidden">J</kbd></button>
              </div>
            </div>
            <div className="border-b border-ink-3 px-5 pt-4 pb-[18px]">
              <div className="mb-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
                <h3 className="font-medium">{t('h.lanes')}</h3>
                <span className="text-xs text-faint">{t(mine.length === 1 ? 's.lanecap1' : 's.lanecap', { n: mine.length })}</span>
              </div>
              <div className="grid grid-cols-[40px_minmax(0,1fr)] gap-3">
                <span className="grid auto-rows-[18px] items-center gap-1.5" aria-hidden>
                  {CHANNELS.map((c) => <span key={c} className="text-xs leading-[18px] text-fg-soft">{CH[c]}</span>)}
                </span>
                <span ref={plot} role="slider" tabIndex={0} aria-label={t('a.plot')} aria-valuemin={0} aria-valuemax={Math.round((entry?.dur || 0) / 1000)}
                  className="relative grid cursor-pointer touch-none auto-rows-[18px] items-center gap-1.5 rounded focus-visible:outline-offset-[6px]"
                  onPointerDown={(e) => { e.currentTarget.setPointerCapture(e.pointerId); seekAt(e); }}
                  onPointerMove={(e) => { if (e.currentTarget.hasPointerCapture(e.pointerId)) seekAt(e); }} onKeyDown={onPlotKey}>
                  {CHANNELS.map((c) => (
                    <span key={c} className="relative h-[18px] rounded bg-ink-3">
                      {c !== 'visual' && entry?.v.has_audio === false && <span className="absolute top-0 left-2 text-[11px] leading-[18px] text-faint">{t('s.noaudio')}</span>}
                      {mine.filter((x) => x.channels.some((h) => h.channel === c)).map((x) => {
                        const on = ms.indexOf(x) === picked;
                        return <span key={ms.indexOf(x)} title={`${tc(x.start_ms)} – ${tc(x.end_ms)}`} style={{ left: pct(x.start_ms), width: pct(x.end_ms - x.start_ms) }}
                          className={cx('absolute min-w-1 rounded-sm transition-[background-color,top,bottom] duration-200 ease-out', on ? 'top-0 bottom-0 bg-red' : 'top-[3px] bottom-[3px] bg-hit hover:bg-fg')} />;
                      })}
                    </span>
                  ))}
                  <span className="relative block h-[18px] text-[11px] text-fg-soft">
                    {ticks(entry?.dur || 0).map((x) => <span key={x} className="absolute top-0.5 -translate-x-1/2 whitespace-nowrap first:translate-x-0" style={{ left: pct(x) }}>{tc(x)}</span>)}
                  </span>
                  <span ref={ph} className="pointer-events-none absolute inset-x-0 -top-[5px] h-[72px]">
                    <i className="absolute inset-y-0 -left-px w-0.5 rounded-[1px] bg-red before:absolute before:-top-[3px] before:-left-1 before:size-2.5 before:rounded-full before:bg-red before:shadow-[0_0_0_2px_var(--color-ink-2)] before:content-['']" />
                  </span>
                </span>
              </div>
            </div>
            {m && (
              <div className="grid gap-[18px] p-5">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <h2 className="text-lg font-semibold">{m.video_title}</h2>
                    <p className="mt-0.5 text-[13px] text-fg-soft">{tc(m.start_ms, true)} – {tc(m.end_ms, true)}{t('s.meta', { secs: Math.round((m.end_ms - m.start_ms) / 1000), tc: tc(m.jump_ms, true) })}</p>
                    <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1.5">
                      {m.channels.map((h) => <Ch key={h.channel} c={h.channel}><Rich k="rank.b" vars={{ ch: CH[h.channel], n: h.rank }} /></Ch>)}
                    </div>
                  </div>
                  <a className={btnPrimary} href={`video.html?id=${m.video_id}&t=${m.jump_ms}`}>{t('b.open')}</a>
                </div>
                {m.snippet && <p className="rounded-md bg-ink-3 px-3.5 py-3 text-[15px] leading-6">“{m.snippet}”</p>}
              </div>
            )}
          </section>

          <section aria-labelledby="count" className={cx(panel, 'col-span-5 max-md:col-span-12')}>
            <PanelHead id="count" title={t('s.count', { n: ms.length })}>
              <span className="text-xs text-faint">{t('s.pos', { n: picked + 1 })} · {t('meta.byrel')}</span>
            </PanelHead>
            <ol className="-mx-2 grid gap-1" onKeyDown={(e) => {
              if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
              e.preventDefault();
              const i = (picked + (e.key === 'ArrowDown' ? 1 : ms.length - 1)) % ms.length;
              choose(i); (e.currentTarget.querySelectorAll('button')[i] as HTMLButtonElement).focus();
            }}>
              {ms.map((x, i) => (
                <li key={i}>
                  <button type="button" aria-pressed={i === picked} onClick={() => choose(i)}
                    className="group flex w-full gap-3.5 rounded-[10px] p-2 text-left transition-colors duration-150 hover:bg-ink-3 aria-pressed:bg-ink-3">
                    <Thumb thumb={x.thumbnail} label={t('frame.of', { t: x.video_title })} className="w-[120px] max-md:w-24"><TcBadge>{tc(x.jump_ms)}</TcBadge></Thumb>
                    <span className="grid min-w-0 content-start gap-[3px]">
                      <span className="truncate font-semibold">{x.video_title}</span>
                      <span className="text-[13px] text-fg-soft">{tc(x.start_ms, true)} – {tc(x.end_ms, true)}</span>
                      <span className="flex flex-wrap gap-x-2.5 gap-y-1">{x.channels.map((h) => <Ch key={h.channel} c={h.channel}>{t('rank', { ch: CH[h.channel], n: h.rank })}</Ch>)}</span>
                    </span>
                  </button>
                </li>
              ))}
            </ol>
          </section>
        </div>
      )}
    </Shell>
  );
}

// Picks one encoding model per channel from the catalog for this scope, and whether each channel takes part.
// Only models that have finished encoding the scope can be picked (the server refuses the rest); applying reloads the search.
function Models() {
  const box = useRef<HTMLDialogElement>(null);
  const [chs, setChs] = useState<ChannelModels[] | null>(null);
  const [err, setErr] = useState('');
  const [pick, setPick] = useState<Record<string, string>>({});
  const [on, setOn] = useState<Record<string, boolean>>({});
  const show = () => {
    box.current!.showModal(); setErr('');
    post<{ channels: ChannelModels[] }>('/v1/search/models', project ? { project_id: project } : {}).then(({ channels }) => {
      setChs(channels.sort((a, b) => CHANNELS.indexOf(a.channel as never) - CHANNELS.indexOf(b.channel as never)));
      // a kept choice this scope cannot serve falls back to the default, so applying repairs it
      const usable = (c: ChannelModels, id?: string) => c.models.some((md) => md.space_id === id && (md.status === 'ready' || c.default_space_ids[0] === id));
      setPick(Object.fromEntries(channels.map((c) => [c.channel, usable(c, prefs.spaces[c.channel]) ? prefs.spaces[c.channel] : c.default_space_ids[0] ?? ''])));
      setOn(Object.fromEntries(channels.map((c) => [c.channel, c.channel in prefs.weights ? prefs.weights[c.channel] > 0 : c.default_weight > 0])));
    }).catch((e) => setErr(errText(e)));
  };
  const save = (p: ModelPrefs | null) => {
    try { if (p) localStorage.setItem(PREFS, JSON.stringify(p)); else localStorage.removeItem(PREFS); } catch { /* blocked storage: nothing to keep */ }
    location.reload();
  };
  return (
    <>
      <button type="button" className={btn} aria-haspopup="dialog" onClick={show}>
        {t('mdl.btn')}{custom && <span className="text-red-text">· {t('mdl.custom')}</span>}
      </button>
      <dialog ref={box} aria-labelledby="mdlh" className="m-auto w-[min(640px,calc(100%-32px))] rounded-[14px] bg-ink-2 text-fg shadow-edge backdrop:bg-black/60">
        <form className="p-6 max-xs:p-4" onSubmit={(e) => { e.preventDefault(); if (chs) save(modelPrefs(chs, pick, on)); }}>
          <h2 id="mdlh" tabIndex={-1} autoFocus className="text-lg font-semibold outline-none">{t('mdl.title')}</h2>
          <p className="mt-1 text-[13px] text-fg-soft">{t('mdl.sub')}</p>
          {err ? <p className="mt-5 text-neg-ink">{err}</p> : !chs ? <p className="mt-5 text-fg-soft">{t('mdl.loading')}</p> : (
            <div className="mt-5 grid gap-3">
              {chs.map((c) => (
                <fieldset key={c.channel} className="min-w-0 rounded-[10px] bg-ink-3/50 p-4 max-xs:p-3">
                  <legend className="sr-only">{t(`ch.${c.channel}`)}</legend>
                  <div className="flex items-center justify-between gap-3">
                    <Ch c={c.channel} className="font-semibold">{t(`ch.${c.channel}`)}</Ch>
                    <label className="inline-flex cursor-pointer items-center gap-2 text-[13px] text-fg-soft">
                      <input type="checkbox" className="size-4 accent-red" checked={!!on[c.channel]} onChange={(e) => setOn({ ...on, [c.channel]: e.target.checked })} />{t('mdl.use')}
                    </label>
                  </div>
                  {!c.models.length ? <p className="mt-2 text-[13px] text-faint">{t('mdl.none')}</p> : (
                    <div className={cx('mt-3 grid gap-1', !on[c.channel] && 'opacity-50')}>
                      {c.models.map((md) => {
                        const dflt = c.default_space_ids[0] === md.space_id, ok = dflt || md.status === 'ready';
                        const multiple = c.models.length > 1, Row = multiple ? 'label' : 'div';
                        return (
                          <Row key={md.space_id} className={cx('flex flex-wrap items-center gap-x-3 gap-y-0.5 rounded-md px-3 py-2 transition-colors max-xs:px-2', !ok ? 'text-faint' : multiple && 'cursor-pointer hover:bg-ink-3', pick[c.channel] === md.space_id && 'bg-ink-3')}>
                            {multiple && <input type="radio" className="accent-red" name={`m-${c.channel}`} checked={pick[c.channel] === md.space_id} disabled={!ok} onChange={() => setPick({ ...pick, [c.channel]: md.space_id })} />}
                            <span className="min-w-0 flex-1 truncate text-[13px] font-medium">{modelName(md.space_id)}</span>
                            {dflt && <span className="rounded bg-ink-4 px-1.5 text-[11px] leading-[18px] text-fg-soft">{t('mdl.default')}</span>}
                            <span className={cx('text-xs whitespace-nowrap max-xs:w-full max-xs:pl-6', md.status === 'ready' ? 'text-pos-ink' : 'text-fg-soft')}>{t(`mst.${md.status}`)} · {t('mdl.count', { a: md.indexed_videos, b: md.eligible_videos })}</span>
                          </Row>
                        );
                      })}
                    </div>
                  )}
                </fieldset>
              ))}
            </div>
          )}
          <div className="mt-6 flex flex-wrap items-center gap-2">
            {custom && <button type="button" className={btn} onClick={() => save(null)}>{t('mdl.reset')}</button>}
            <span className="flex-1" />
            <button type="button" className={btn} onClick={() => box.current!.close()}>{t('mdl.cancel')}</button>
            <button type="submit" className={btnPrimary} disabled={!chs}>{t('mdl.apply')}</button>
          </div>
        </form>
      </dialog>
    </>
  );
}
