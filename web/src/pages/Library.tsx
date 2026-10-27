// Library: imports copy into managed storage; processing continues in the desktop process.
import { useCallback, useEffect, useReducer, useRef, useState } from 'react';
import { get, pickFiles, post, type LocalFile, type Video } from '../api.ts';
import { t, useLang } from '../i18n.ts';
import { tc } from '../lib.ts';
import { Empty, newProject, panel, PanelHead, Shell, TitleRow, useNotice, useLibrary } from '../Shell.tsx';
import { btn, btnPrimary, cx, errText, Icon, input, Pill, select, Thumb } from '../ui.tsx';
import { busy, tracks, when } from './format.ts';

interface Upload { key: number; name: string; size: number; done: number; total: number; state: 'run' | 'done' | 'fail'; controller: AbortController; error?: string }
interface Page { videos: Video[]; next_cursor: string | null }
const params = new URLSearchParams(location.search);
const status = params.get('status') || '', project = params.get('project') || '';
const query = (extra: Record<string, string>) => new URLSearchParams({ limit: '50', ...(status && { status }), ...(project && { project_id: project }), ...extra });
const filter = (extra: Record<string, string>) => `library.html?${new URLSearchParams({ ...(project && { project }), ...extra })}`;
const rule = 'border-t border-ink-3 first:border-t-0';

export default function Library() {
  useLang();
  const { notice, fail } = useNotice();
  const { ws, reload } = useLibrary(fail);
  const [rows, setRows] = useState<Video[] | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [attention, setAttention] = useState<{ failed: Page; indexing: Page } | null>(null);
  const ups = useRef<Upload[]>([]), timer = useRef(0);
  const [sourceURL, setSourceURL] = useState(''), [addingURL, setAddingURL] = useState(false);
  const [, bump] = useReducer((n: number) => n + 1, 0);

  // re-reads the first page and the attention panel; repeats while anything on screen is still processing
  const refresh = useCallback(async () => {
    clearTimeout(timer.current);
    const [first, failed, indexing] = await Promise.all([get<Page>(`/v1/videos?${query({})}`), get<Page>('/v1/videos?status=failed&limit=5'), get<Page>('/v1/videos?status=indexing&limit=5')]);
    setRows(first.videos); setCursor(first.next_cursor); setAttention({ failed, indexing });
    if ([...first.videos, ...failed.videos, ...indexing.videos].some((v) => busy(v.status))) timer.current = window.setTimeout(() => refresh().catch(fail), 5000);
  }, [fail]);
  useEffect(() => { refresh().catch(fail); return () => clearTimeout(timer.current); }, [refresh, fail]);

  const more = async () => {
    const r = await get<Page>(`/v1/videos?${query(cursor ? { cursor } : {})}`);
    setRows((old) => [...(old || []), ...r.videos]); setCursor(r.next_cursor);
  };
  const retry = async (id: string, b: HTMLButtonElement) => {
    b.disabled = true;
    try { await post(`/v1/videos/${id}/retry`); await refresh(); reload().catch(fail); } catch (err) { b.disabled = false; fail(err); }
  };

  async function upload(f: LocalFile) {
    const u: Upload = { key: Math.random(), name: f.name, size: f.size, done: 0, total: 1, state: 'run', controller: new AbortController() };
    ups.current = [u, ...ups.current]; bump();
    try {
      await post('/v1/imports/file', { path: f.path, project_id: project || undefined }, u.controller.signal);
      u.done = 1; u.state = 'done';
    } catch (err) {
      u.state = 'fail'; u.error = u.controller.signal.aborted ? t('desktop.cancelled') : errText(err);
    }
    bump();
    if (u.state === 'done') { refresh().catch(fail); reload().catch(fail); }
  }
  const chooseFiles = async () => {
    try { for (const f of await pickFiles()) await upload(f); } catch (err) { fail(err); }
  };
  const importURL = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault(); setAddingURL(true);
    try {
      await post('/v1/imports/url', { url: sourceURL.trim(), project_id: project || undefined });
      setSourceURL(''); await refresh(); reload().catch(fail);
    } catch (err) { fail(err); } finally { setAddingURL(false); }
  };
  // A file copy is canceled on navigation; submitted processing jobs live in Go.
  useEffect(() => {
    const warn = (e: BeforeUnloadEvent) => { if (ups.current.some((u) => u.state === 'run')) e.preventDefault(); };
    window.addEventListener('beforeunload', warn);
    return () => { window.removeEventListener('beforeunload', warn); ups.current.forEach((u) => u.controller.abort()); };
  }, []);

  const att = attention ? [...attention.failed.videos, ...attention.indexing.videos] : [];
  const more2 = (p: Page) => p.videos.length + (p.next_cursor ? '+' : '');

  return (
    <Shell page="library" ws={ws} notice={notice} onNewProject={() => newProject(reload, fail)}>
      <TitleRow title={t('nav.library')} sub={attention ? t('l.sub2', { i: more2(attention.indexing), f: more2(attention.failed) }) : ' '}>
        <button type="button" className={btn} onClick={() => newProject(reload, fail)}>{t('b.newproject')}</button>
        <button type="button" className={btnPrimary.replace('h-8', 'h-9')} onClick={chooseFiles}>{t('b.upload')}</button>

      </TitleRow>

      <form className="mb-6 flex flex-wrap items-end gap-3" onSubmit={importURL}>
        <label className="grid min-w-56 flex-1 gap-2"><span className="text-xs text-fg-soft">{t('desktop.url')}</span>
          <input className={input} type="url" required value={sourceURL} onChange={(e) => setSourceURL(e.target.value)} placeholder="https://…" />
        </label>
        <button className={btn} type="submit" disabled={addingURL}>{addingURL ? t('desktop.adding') : t('desktop.importURL')}</button>
      </form>
      <div className="mb-6 grid grid-cols-12 gap-6">
        <section aria-labelledby="uh" className={cx(panel, 'col-span-7 max-md:col-span-12')}>
          <PanelHead id="uh" title={t('h.uploading')}><span className="text-xs text-faint">{t('meta.direct')}</span></PanelHead>
          {ups.current.length ? ups.current.map((u) => {
            const pct = u.total ? u.done / u.total : 0;
            return (
              <div key={u.key} className={cx('grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 py-2.5', rule)}>
                <span className="truncate font-medium">{u.name}</span>
                <span className="text-right text-fg-soft">{u.state === 'done' ? t('l.submitted') : u.state === 'fail' ? t('u.failed', { e: u.error || '' }) : t('desktop.copying')}</span>
                <div className="col-span-2 grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 text-xs text-fg-soft">
                  <span>{u.state === 'run' ? t('desktop.managedCopy') : t('desktop.copied')}</span>
                  <span className={cx('h-1.5 overflow-hidden rounded-full', u.state === 'done' ? 'bg-pos-bg' : 'bg-red-track')}>
                    <i className={cx('block h-full origin-left rounded-full transition-transform duration-[600ms] ease-out', u.state === 'done' ? 'bg-pos' : 'bg-red')} style={{ transform: `scaleX(${pct})` }} />
                  </span>
                  <span>{(u.size / 1048576).toFixed(0)} MB</span>
                  {u.state === 'run' && <button className="text-fg underline" type="button" onClick={() => u.controller.abort()}>{t('desktop.cancel')}</button>}
                </div>
              </div>
            );
          }) : <Empty>{t('u.none')}</Empty>}
        </section>
        <section aria-labelledby="ah" className={cx(panel, 'col-span-5 max-md:col-span-12')}>
          <PanelHead id="ah" title={t('h.attention')}>
            <a className="inline-flex items-center gap-0.5 font-medium underline-offset-[3px] hover:underline" href={filter({ status: 'indexing' })}>{t('b.viewindexing')}<Icon.chevron className="size-3" /></a>
          </PanelHead>
          <div className="grid">
            {!attention ? null : att.length ? att.map((v) => (
              <div key={v.id} className={cx('flex min-h-11 items-center gap-3 py-2.5', rule)}>
                <a className="group flex min-w-0 flex-1 items-center gap-3" href={`video.html?id=${v.id}`}>
                  <Thumb thumb={v.thumbnail} label={t('frame.of', { t: v.title })} className="w-20" />
                  <span className="min-w-0"><span className="block truncate font-semibold">{v.title}</span><span className="text-[13px] text-fg-soft">{v.status === 'failed' ? v.error || t('st.failed') : tracks(v)}</span></span>
                </a>
                {v.status === 'failed' ? <button type="button" className={btn} onClick={(e) => retry(v.id, e.currentTarget)}>{t('retry')}</button> : <Pill status={v.status} />}
              </div>
            )) : <Empty>{t('att.none')}</Empty>}
          </div>
        </section>
      </div>

      <div role="tablist" className="mb-6 flex gap-6 overflow-x-auto border-b border-ink-3 whitespace-nowrap">
        {[['', t('tab.all')], ...['importing', 'indexing', 'ready', 'failed', 'cancelled', 'deleted'].map((s) => [s, t(`st.${s}`)])].map(([s, label]) => (
          <a key={s} role="tab" href={filter(s ? { status: s } : {})} aria-selected={s === status}
            className="relative py-2.5 text-fg-soft transition-colors duration-150 hover:text-fg aria-selected:text-fg aria-selected:after:absolute aria-selected:after:inset-x-0 aria-selected:after:-bottom-px aria-selected:after:h-0.5 aria-selected:after:bg-fg aria-selected:after:content-['']">{label}</a>
        ))}
      </div>
      <div className="mb-2 flex items-center gap-3">
        <label htmlFor="proj" className="sr-only">{t('h.projfilter')}</label>
        <select id="proj" className={select} value={project} onChange={(e) => { location.search = String(new URLSearchParams({ ...(status && { status }), ...(e.target.value && { project: e.target.value }) })); }}>
          <option value="">{t('opt.allproj')}</option>
          {ws?.projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
        <span className="flex-1" />
        <span className="text-fg-soft">{rows ? t('l.total', { n: rows.length + (cursor ? '+' : '') }) : ''}</span>
        <span className="text-xs text-faint max-xs:hidden">{t('meta.sort')}</span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse">
          <thead className="max-md:hidden">
            <tr className="[&>th]:h-9 [&>th]:border-b [&>th]:border-ink-3 [&>th]:px-3 [&>th]:text-left [&>th]:text-xs [&>th]:font-normal [&>th]:whitespace-nowrap [&>th]:text-fg-soft">
              <th className="w-[42%]">{t('th.video')}</th><th>{t('th.status')}</th><th className="text-right!">{t('th.dur')}</th><th className="text-right!">{t('th.added')}</th>
            </tr>
          </thead>
          <tbody>
            {rows && !rows.length && <tr><td colSpan={4} className="py-3"><Empty>{t('l.empty')}</Empty></td></tr>}
            {rows?.map((v) => (
              <tr key={v.id} className="animate-enter border-b border-ink-3 transition-colors duration-150 last:border-b-0 hover:bg-ink-2/60 max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto] max-md:items-center max-md:gap-x-3 max-md:gap-y-1.5 max-md:py-3 [&>td]:px-3 [&>td]:py-[12.5px] [&>td]:align-middle max-md:[&>td]:p-0">
                <td><a className="group flex min-w-0 items-center gap-3" href={`video.html?id=${v.id}`}>
                  <Thumb thumb={v.thumbnail} label={t('frame.of', { t: v.title })} className="w-20" />
                  <span className="min-w-0"><span className="block truncate font-semibold">{v.title}</span><span className="block text-xs text-fg-soft">{tracks(v)}</span></span>
                </a></td>
                <td><Pill status={v.status} />{v.status === 'deleted' && <button type="button" className={btn} onClick={() => post(`/v1/videos/${v.id}/restore`).then(() => refresh()).then(() => reload()).catch(fail)}>{t('desktop.restore')}</button>}{(v.status === 'failed' || v.status === 'cancelled') && <span className="block text-xs text-fg-soft">{v.error}<button type="button" className="ml-2 font-medium text-fg underline underline-offset-[3px]" onClick={(e) => retry(v.id, e.currentTarget)}>{t('retry')}</button></span>}</td>
                <td className="text-right max-md:col-start-2">{v.duration_ms ? tc(v.duration_ms) : <span className="text-faint">—</span>}</td>
                <td className="text-right max-md:hidden">{when(v.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {cursor && <p className="mt-4 text-center"><button type="button" className={btn} onClick={() => more().catch(fail)}>{t('b.more')}</button></p>}
    </Shell>
  );
}
