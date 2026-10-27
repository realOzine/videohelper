// One video: its header and actions, the pipeline in the order it runs (polled while busy), the player and a search inside it.
import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { call, get, post, type Moment, type Video as V } from '../api.ts';
import { addStrings, t, useLang } from '../i18n.ts';
import { CHANNELS, channelState, pipeline, stageKind, tc, type Stage } from '../lib.ts';
import { Empty, newProject, panel, panelBase, PanelHead, Shell, useNotice, useLibrary } from '../Shell.tsx';
import { btn, btnDanger, btnPrimary, Ch, cx, field, Icon, Pill, select, TcBadge, Thumb } from '../ui.tsx';
import { busy, stageLabel, tracks, when } from './format.ts';

addStrings({ 'v.reindex': '重新索引' }, { 'v.reindex': 'Rebuild index' });

const params = new URLSearchParams(location.search);
const id = params.get('id') || '';
const startAt = Number(params.get('t')) || 0;

function Cell({ s }: { s: Stage }) {
  const st = s.status === 'queued' ? 'pending' : s.status;
  const label = s.status === 'running' ? (s.progress != null ? t('run.pct', { n: Math.round(s.progress * 100) }) : t('run'))
    : s.status === 'done' ? t('done.at', { t: when(s.updated_at) }) : s.status === 'failed' ? t('fail.n', { n: s.attempt })
      : s.status === 'queued' ? t('queued') : stageKind(s.stage) === 'speech' ? t('pend.asr') : t('pend.all');
  return (
    <div className={cx('grid min-w-0 content-start gap-1 rounded-lg px-3.5 py-3 transition-colors duration-300', st === 'failed' ? 'bg-red-soft' : st === 'running' ? 'bg-ink-3 ring-1 ring-red/35 ring-inset' : 'bg-ink-3')}>
      <span className="flex items-center gap-2 font-semibold">
        <span className={cx('grid size-4 flex-none place-items-center rounded-full text-ink [&>svg]:size-2.5',
          st === 'done' && 'bg-pos', st === 'failed' && 'bg-red-text', st === 'running' && 'animate-spin border-2 border-red border-r-red-track', st === 'pending' && 'border-2 border-line')}>
          {st === 'done' ? <Icon.check /> : st === 'failed' ? <Icon.x /> : null}
        </span>
        {stageLabel(s)}
      </span>
      <span className={cx('text-[13px]', st === 'failed' ? 'text-neg-ink' : st === 'running' ? 'text-fg' : 'text-fg-soft')}>{label}</span>
      {s.progress != null && s.status === 'running' && (
        <span className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-red-track"><i className="block h-full origin-left rounded-full bg-red transition-transform duration-[600ms] ease-out" style={{ transform: `scaleX(${s.progress})` }} /></span>
      )}
      {s.error && <span className="text-[13px] text-neg-ink">{s.error}</span>}
    </div>
  );
}

export default function Video() {
  useLang();
  const { notice, fail } = useNotice();
  const { ws, reload } = useLibrary(fail);
  const [data, setData] = useState<{ video: V; stages: Stage[] } | null>(null);
  const [url, setUrl] = useState('');
  const [added, setAdded] = useState('');
  const [found, setFound] = useState<Moment[] | null>(null);
  const timer = useRef(0), opening = useRef(false), vid = useRef<HTMLVideoElement>(null);

  const load = useCallback(async () => {
    clearTimeout(timer.current);
    const d = await get<{ video: V; stages: Stage[] }>(`/v1/videos/${id}`);
    setData(d);
    if (busy(d.video.status)) timer.current = window.setTimeout(() => load().catch(fail), 3000);
  }, [fail]);
  useEffect(() => {
    if (!/^[1-9]\d*$/.test(id)) { fail(new Error(t('v.missing'))); return; }
    load().catch(fail);
    return () => clearTimeout(timer.current);
  }, [load, fail]);

  // the player opens once transcoding has produced the normalized video, which is when a poster exists
  const v = data?.video;
  useEffect(() => {
    if (!v || opening.current || !(v.thumbnail || v.status === 'ready')) return;
    opening.current = true;
    get<{ url: string }>(`/v1/videos/${id}/playback`).then((r) => setUrl(r.url)).catch((err) => { opening.current = false; fail(err); });
  }, [v, fail]);

  const retry = async (b: HTMLButtonElement) => {
    b.disabled = true;
    try { await post(`/v1/videos/${id}/retry`); await load(); reload().catch(fail); } catch (err) { fail(err); } finally { b.disabled = false; }
  };
  const remove = async () => {
    if (!v || !confirm(t('v.confirmdelete', { t: v.title }))) return;
    try { await call('DELETE', `/v1/videos/${id}`); location.href = 'library.html'; } catch (err) { fail(err); }
  };
  const addTo = async (pid: string) => {
    const p = ws?.projects.find((x) => x.id === pid);
    if (!p) return;
    try { await call('PUT', `/v1/projects/${p.id}/videos/${id}`); setAdded(t('v.added2', { p: p.name })); reload().catch(fail); } catch (err) { fail(err); }
  };
  const searchIn = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const q = String(new FormData(e.currentTarget).get('vq') || '').trim();
    if (!q) return;
    try { setFound((await post<{ moments: Moment[] }>('/v1/search', { query: q, video_ids: [id], limit: 20 })).moments); } catch (err) { fail(err); }
  };
  const jump = (ms: number) => { const el = vid.current; if (!el) return; el.currentTime = ms / 1000; el.play().catch(fail); };

  const stages = data?.stages ?? [];
  const failed = stages.find((s) => s.status === 'failed');
  const done = stages.filter((s) => s.status === 'done').length;
  const p = pipeline(stages), can = channelState(stages);
  const latest = stages.reduce((x, s) => (s.updated_at > x ? s.updated_at : x), v?.updated_at || '');
  const CH = Object.fromEntries(CHANNELS.map((c) => [c, t(`ch.${c}`)]));

  return (
    <Shell page="video" ws={ws} notice={notice} onNewProject={() => newProject(reload, fail)}>
      <p className="mb-3"><a className="inline-flex items-center gap-0.5 font-medium text-fg-soft underline-offset-[3px] hover:underline" href="library.html">{t('nav.library')}<Icon.chevron className="size-3" /></a></p>
      {v && (
        <div>
          <section aria-label={t('a.video')} className={cx(panelBase, 'mb-6 flex items-center gap-4 p-3.5 max-md:flex-wrap')}>
            <Thumb thumb={v.thumbnail} label={t('frame.of', { t: v.title })} className="w-28" />
            <div className="min-w-0">
              <h1 className="flex flex-wrap items-center gap-2.5 text-xl leading-[1.3] font-semibold">{v.title} <Pill status={v.status} /></h1>
              <div className="mt-1 text-[13px] text-fg-soft">{v.duration_ms ? `${tc(v.duration_ms)} · ` : ''}{t(`source.${v.source_kind}`)} · {tracks(v)} · {t('v.added', { w: when(v.created_at) })}</div>
            </div>
            <div className="ml-auto flex flex-wrap gap-2 max-md:ml-0 max-md:w-full">
              {(v.status === 'failed' || v.status === 'cancelled' || (v.status === 'ready' && failed)) && <button type="button" className={btnPrimary} onClick={(e) => retry(e.currentTarget)}>{t('retry')}</button>}
              {v.status === 'ready' && !failed && <button type="button" className={btn} onClick={(e) => retry(e.currentTarget)}>{t('v.reindex')}</button>}
              {busy(v.status) && <button type="button" className={btn} onClick={() => post(`/v1/videos/${id}/cancel`).then(() => load()).catch(fail)}>{t('desktop.cancel')}</button>}
              {v.status === 'deleted' && <button type="button" className={btnPrimary} onClick={() => post(`/v1/videos/${id}/restore`).then(() => load()).then(() => reload()).catch(fail)}>{t('desktop.restore')}</button>}
              {!!ws?.projects.length && (
                <>
                  <label htmlFor="addp" className="sr-only">{t('v.addproject')}</label>
                  <select id="addp" className={select} value="" onChange={(e) => addTo(e.target.value)}>
                    <option value="">{t('v.addproject')}</option>
                    {ws.projects.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
                  </select>
                </>
              )}
              <button type="button" className={btnDanger} onClick={remove}>{t('v.delete')}</button>
            </div>
          </section>

          <section aria-labelledby="ph" className={cx(panel, 'mb-6')}>
            <header className="mb-4 flex items-start justify-between gap-3 max-xs:flex-col max-xs:gap-1">
              <div>
                <h2 id="ph" className="text-lg leading-[1.3] font-semibold">{t('h.progress')}</h2>
                <p className="mt-1 text-[15px] text-fg-soft">{v.status === 'ready' ? t('v.find.ready')
                  : failed ? t('v.find.failed2', { s: stageLabel(failed), n: failed.attempt, e: failed.error || v.error || '' })
                    : v.status === 'uploading' ? t('v.find.uploading') : t('v.find.indexing2', { a: done, b: stages.length })}</p>
              </div>
              <span className="text-xs whitespace-nowrap text-faint">{added || `${t('v.done', { a: done, b: stages.length })} · ${t('v.updated', { t: when(latest) })}`}</span>
            </header>
            <div className="grid grid-cols-[1fr_1fr_3fr_1fr] items-start gap-3 max-xl:grid-cols-2 max-xs:grid-cols-1">
              {p.before.map((s) => <Cell key={s.stage} s={s} />)}
              <div className="grid gap-2 max-xl:col-span-2 max-xs:col-span-1">
                {p.lanes.map(([c, l]) => <div key={c} className={cx('grid gap-2', l.length > 1 ? 'grid-cols-2 max-xs:grid-cols-1' : 'grid-cols-1')}>{l.map((s) => <Cell key={s.stage} s={s} />)}</div>)}
              </div>
              {p.after.map((s) => <Cell key={s.stage} s={s} />)}
            </div>
          </section>

          <div className="grid grid-cols-12 items-start gap-6">
            <section aria-label={t('a.player')} className="col-span-8 min-w-0 overflow-hidden rounded-2xl border border-ink-3 bg-ink-2 max-md:col-span-12">
              <div className="relative aspect-video bg-black">
                {url ? <video ref={vid} src={url} controls playsInline preload="metadata" className="block size-full"
                  onError={(e) => fail(new Error(t('err.playback', { code: e.currentTarget.error?.code ?? 0 })))}
                  onLoadedMetadata={(e) => { if (startAt) e.currentTarget.currentTime = startAt / 1000; }} />
                  : <p className="absolute inset-0 grid place-items-center p-4 text-center text-fg-soft">{t('v.noplay')}</p>}
              </div>
            </section>
            <div className="col-span-4 grid content-start gap-6 max-md:col-span-12">
              <section aria-labelledby="sh" className={panel}>
                <PanelHead id="sh" title={t('h.vsearch')} />
                <form className="flex gap-2" role="search" onSubmit={searchIn}>
                  <label htmlFor="vq" className="sr-only">{t('h.vsearch')}</label>
                  <input id="vq" name="vq" type="search" className={cx(field, 'h-8 min-w-0 flex-1')} placeholder={t('top.placeholder')} />
                  <button type="submit" className={btn}>{t('b.search')}</button>
                </form>
                {found && (
                  <ol className="-mx-2 mt-3 grid gap-1" aria-live="polite">
                    {found.length ? found.map((m, i) => (
                      <li key={i}>
                        <button type="button" onClick={() => jump(m.jump_ms)} className="group flex w-full gap-3.5 rounded-[10px] p-2 text-left transition-colors duration-150 hover:bg-ink-3">
                          <Thumb thumb={m.thumbnail} label={t('frame.of', { t: v.title })} className="w-[120px] max-md:w-24"><TcBadge>{tc(m.jump_ms)}</TcBadge></Thumb>
                          <span className="grid min-w-0 content-start gap-[3px]">
                            <span className="text-[13px] text-fg-soft">{tc(m.start_ms, true)} – {tc(m.end_ms, true)}</span>
                            <span className="flex flex-wrap gap-x-2.5 gap-y-1">{m.channels.map((h) => <Ch key={h.channel} c={h.channel}>{t('rank', { ch: CH[h.channel], n: h.rank })}</Ch>)}</span>
                            {m.snippet && <span className="text-[13px] text-fg-soft">“{m.snippet}”</span>}
                          </span>
                        </button>
                      </li>
                    )) : <li><Empty>{t('vs.none')}</Empty></li>}
                  </ol>
                )}
                <div className="mt-3 grid">
                  {CHANNELS.map((c) => {
                    const [text, tone] = c !== 'visual' && v.has_audio === false ? [t('can.noaudio'), 'bg-ink-3 text-fg']
                      : ({ done: [t('can.yes'), 'bg-pos-bg text-pos-ink'], failed: [t('can.failed'), 'bg-red-soft text-neg-ink'], none: ['—', 'bg-ink-3 text-fg'] } as Record<string, string[]>)[can[c]] || [t('can.wait'), 'bg-ink-3 text-fg'];
                    return (
                      <div key={c} className="flex items-center justify-between gap-3 border-t border-ink-3 py-[9px] first:border-t-0">
                        <Ch c={c} className="text-sm text-fg">{CH[c]}</Ch>
                        <span className={cx('inline-flex h-[22px] items-center rounded-full px-2 text-xs font-medium', tone)}>{text}</span>
                      </div>
                    );
                  })}
                </div>
              </section>
              <section aria-labelledby="ih" className={panel}>
                <PanelHead id="ih" title={t('h.info')} />
                <dl className="grid">
                  {[[t('kv.dur'), v.duration_ms ? tc(v.duration_ms) : '—'], [t('kv.src'), t(`source.${v.source_kind}`)], [t('kv.tracks'), tracks(v)], [t('kv.added'), when(v.created_at)]].map(([k, val]) => (
                    <div key={k} className="flex justify-between gap-3 border-t border-ink-3 py-2.5 first:border-t-0"><dt className="text-fg-soft">{k}</dt><dd className="text-right">{val}</dd></div>
                  ))}
                </dl>
              </section>
            </div>
          </div>
        </div>
      )}
    </Shell>
  );
}
