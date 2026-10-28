// Library: imports copy into managed storage; processing continues in the desktop process.
import { useEffect, useReducer, useRef, useState } from 'react';
import { get, pickFiles, post, type LocalFile, type Video } from '../api.ts';
import { t, useLang } from '../i18n.ts';
import { tc } from '../lib.ts';
import { Empty, newProject, panel, PanelHead, Shell, TitleRow, useNotice, useLibrary } from '../Shell.tsx';
import { btn, btnPrimary, cx, ErrorDetails, errText, Icon, input, Pill, select, Thumb } from '../ui.tsx';
import { busy, tracks, when } from './format.ts';
import { followQueryLink, setPageQuery, usePageQuery } from '../pageQuery.ts';
import { libraryEmptyKey } from '../feedback.ts';
import { tabIndexForKey } from '../tabs.ts';

interface Upload { key: number; name: string; size: number; done: number; total: number; state: 'run' | 'done' | 'fail' | 'cancelled'; controller: AbortController; error?: unknown }
interface Page { videos: Video[]; next_cursor: string | null }
const rule = 'border-t border-ink-3 first:border-t-0';

export default function Library() {
  useLang();
  const params = usePageQuery();
  const status = params.get('status') || '', project = params.get('project') || '';
  const query = (extra: Record<string, string>) => new URLSearchParams({ limit: '50', ...(status && { status }), ...(project && { project_id: project }), ...extra });
  const filter = (extra: Record<string, string>) => `library.html?${new URLSearchParams({ ...(project && { project }), ...extra })}`;
  const { notice, fail } = useNotice();
  const { ws, reload } = useLibrary(fail);
  const [rows, setRows] = useState<Video[] | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [attention, setAttention] = useState<{ failed: Page; indexing: Page } | null>(null);
  const ups = useRef<Upload[]>([]), requests = useRef<AbortController | null>(null);
  const [revision, refresh] = useReducer((n: number) => n + 1, 0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [sourceURL, setSourceURL] = useState(''), [addingURL, setAddingURL] = useState(false);
  const [, bump] = useReducer((n: number) => n + 1, 0);

  // re-reads the first page and the attention panel; repeats while anything on screen is still processing
  useEffect(() => {
    const controller = new AbortController(), { signal } = controller;
    requests.current = controller;
    let timer = 0;
    setRows(null); setCursor(null); setLoadingMore(false);
    const poll = async () => {
      try {
        const [first, failed, indexing] = await Promise.all([get<Page>(`/v1/videos?${query({})}`, signal), get<Page>('/v1/videos?status=failed&limit=5', signal), get<Page>('/v1/videos?status=indexing&limit=5', signal)]);
        if (signal.aborted) return;
        setRows(first.videos); setCursor(first.next_cursor); setAttention({ failed, indexing });
        if ([...first.videos, ...failed.videos, ...indexing.videos].some((v) => busy(v.status))) timer = window.setTimeout(poll, 5000);
      } catch (err) { if (!signal.aborted) fail(err); }
    };
    poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [status, project, revision, fail]);

  const more = async () => {
    const signal = requests.current?.signal;
    if (!signal || signal.aborted || loadingMore || !cursor) return;
    setLoadingMore(true);
    try {
      const r = await get<Page>(`/v1/videos?${query({ cursor })}`, signal);
      if (!signal.aborted) { setRows((old) => [...(old || []), ...r.videos]); setCursor(r.next_cursor); }
    } catch (err) { if (!signal.aborted) fail(err); }
    finally { if (!signal.aborted) setLoadingMore(false); }
  };
  const retry = async (id: string, b: HTMLButtonElement) => {
    b.disabled = true;
    try { await post(`/v1/videos/${id}/retry`); refresh(); reload().catch(fail); } catch (err) { b.disabled = false; fail(err); }
  };

  async function upload(f: LocalFile) {
    const u: Upload = { key: Math.random(), name: f.name, size: f.size, done: 0, total: 1, state: 'run', controller: new AbortController() };
    ups.current = [u, ...ups.current]; bump();
    try {
      await post('/v1/imports/file', { path: f.path, project_id: project || undefined }, u.controller.signal);
      u.done = 1; u.state = 'done';
    } catch (err) {
      u.state = u.controller.signal.aborted ? 'cancelled' : 'fail'; u.error = err;
    }
    bump();
    if (u.state === 'done') { refresh(); reload().catch(fail); }
  }
  const chooseFiles = async () => {
    try { for (const f of await pickFiles()) await upload(f); } catch (err) { fail(err); }
  };
  const importURL = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault(); setAddingURL(true);
    try {
      await post('/v1/imports/url', { url: sourceURL.trim(), project_id: project || undefined });
      setSourceURL(''); refresh(); reload().catch(fail);
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
    <Shell page="library" ws={ws} notice={notice} onNewProject={() => newProject(reload, fail)} onQueryLink={followQueryLink}>
      <TitleRow title={t('nav.library')} sub={attention ? t('l.sub2', { i: more2(attention.indexing), f: more2(attention.failed) }) : ' '}>
        <button type="button" className={btn.replace('h-8', 'h-10')} onClick={() => newProject(reload, fail)}><Icon.folder plus />{t('b.newproject')}</button>
        <button type="button" className={btnPrimary.replace('h-8', 'h-10')} onClick={chooseFiles}><Icon.import />{t('b.upload')}</button>

      </TitleRow>

      <form className={cx(panel, 'mb-5 flex flex-wrap items-end gap-3')} onSubmit={importURL}>
        <label className="grid min-w-0 flex-1 basis-56 gap-2"><span className="text-xs text-fg-soft">{t('desktop.url')}</span>
          <input className={cx(input, 'min-w-0 w-full')} type="url" required value={sourceURL} onChange={(e) => setSourceURL(e.target.value)} placeholder="https://…" />
        </label>
        <button className={btn.replace('h-8', 'h-[38px]')} type="submit" disabled={addingURL}><Icon.arrow />{addingURL ? t('desktop.adding') : t('desktop.importURL')}</button>
      </form>
      <div className="mb-7 grid grid-cols-1 gap-5 xl:grid-cols-12">
        <section aria-labelledby="uh" className={cx(panel, 'xl:col-span-7')}>
          <PanelHead id="uh" title={t('h.uploading')}><span className="text-xs text-faint">{t('meta.direct')}</span></PanelHead>
          {ups.current.length ? ups.current.map((u) => {
            const pct = u.total ? u.done / u.total : 0;
            return (
              <div key={u.key} className={cx('grid min-w-0 gap-y-1 py-2.5', rule)}>
                <span title={u.name} className="truncate font-medium">{u.name}</span>
                <div className="min-w-0 text-fg-soft">{u.state === 'fail' ? <ErrorDetails summary={t('u.failed')} detail={u.error == null ? undefined : errText(u.error)} /> : t(u.state === 'done' ? 'desktop.copied' : u.state === 'cancelled' ? 'desktop.cancelled' : 'desktop.copying')}</div>
                <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 text-xs text-fg-soft">
                  <span>{t('desktop.managedCopy')}</span>
                  <span className={cx('h-1.5 overflow-hidden rounded-full', u.state === 'done' ? 'bg-pos-bg' : 'bg-red-track')}>
                    <i className={cx('block h-full origin-left rounded-full transition-transform duration-[600ms] ease-out', u.state === 'done' ? 'bg-pos' : 'bg-red')} style={{ transform: `scaleX(${pct})` }} />
                  </span>
                  <span>{(u.size / 1048576).toFixed(0)} MB</span>
                  {u.state === 'run' && <button className="text-fg underline" type="button" onClick={() => u.controller.abort()}>{t('desktop.cancel')}</button>}
                </div>
              </div>
            );
          }) : <Empty>{t('u.none')}</Empty>}
          <p className="mt-3 text-xs leading-relaxed text-faint">{t('l.copyResults')}</p>
        </section>
        <section aria-labelledby="ah" className={cx(panel, 'xl:col-span-5')}>
          <PanelHead id="ah" title={t('h.attention')}>
            <a className="inline-flex items-center gap-0.5 font-medium underline-offset-[3px] hover:underline" href={filter({ status: 'indexing' })}>{t('b.viewindexing')}<Icon.chevron className="size-3" /></a>
          </PanelHead>
          <div className="grid">
            {!attention ? null : att.length ? att.map((v) => (
              <div key={v.id} className={cx('flex min-h-11 min-w-0 items-center gap-3 py-2.5', rule)}>
                <div className="min-w-0 flex-1">
                  <a className="group flex min-w-0 items-center gap-3" href={`video.html?id=${v.id}`}>
                    <Thumb thumb={v.thumbnail} label={t('frame.of', { t: v.title })} className="w-20" />
                    <span className="min-w-0 flex-1"><span title={v.title} className="block truncate font-semibold">{v.title}</span><span className="text-[13px] text-fg-soft">{tracks(v)}</span></span>
                  </a>
                  {v.status === 'failed' && <div className="mt-2"><ErrorDetails summary={t('l.processingFailed')} detail={v.error} /></div>}
                </div>
                {v.status === 'failed' ? <button type="button" className={btn} onClick={(e) => retry(v.id, e.currentTarget)}>{t('retry')}</button> : <Pill status={v.status} />}
              </div>
            )) : <Empty>{t('att.none')}</Empty>}
          </div>
        </section>
      </div>

      <div role="tablist" aria-label={t('l.statusFilter')} onKeyDown={(e) => {
        if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
        const tabs = Array.from(e.currentTarget.querySelectorAll<HTMLAnchorElement>('[role="tab"]'));
        const next = tabIndexForKey(e.key, tabs.indexOf(e.target as HTMLAnchorElement), tabs.length);
        if (next == null) return;
        e.preventDefault(); tabs[next].focus(); tabs[next].click();
      }} className="mb-5 flex w-fit max-w-full gap-1 overflow-x-auto rounded-full border border-ink-3 bg-ink-2 p-1 whitespace-nowrap">
        {[['', t('tab.all')], ...['importing', 'indexing', 'ready', 'failed', 'cancelled', 'deleted'].map((s) => [s, t(`st.${s}`)])].map(([s, label]) => (
          <a key={s} id={`library-filter-${s || 'all'}`} role="tab" href={filter(s ? { status: s } : {})} aria-selected={s === status} aria-controls="library-videos" tabIndex={s === status ? 0 : -1}
            className="library-tab">{label}</a>
        ))}
      </div>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <label htmlFor="proj" className="sr-only">{t('h.projfilter')}</label>
        <select id="proj" className={cx(select, 'min-w-0 max-w-full')} value={project} onChange={(e) => setPageQuery(new URLSearchParams({ ...(status && { status }), ...(e.target.value && { project: e.target.value }) }))}>
          <option value="">{t('opt.allproj')}</option>
          {ws?.projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
        <span className="flex-1" />
        <span className="text-fg-soft">{rows ? t('l.total', { n: rows.length + (cursor ? '+' : '') }) : ''}</span>
        <span className="text-xs text-faint max-xs:hidden">{t('meta.sort')}</span>
      </div>
      <div id="library-videos" role="tabpanel" aria-labelledby={`library-filter-${status || 'all'}`} tabIndex={0} aria-busy={rows === null} className="@container overflow-x-auto rounded-2xl border border-ink-3 bg-ink-2/40 px-4">
        <table className="w-full table-fixed border-collapse">
          <thead className="@max-[42rem]:hidden">
            <tr className="[&>th]:h-9 [&>th]:border-b [&>th]:border-ink-3 [&>th]:px-3 [&>th]:text-left [&>th]:text-xs [&>th]:font-normal [&>th]:whitespace-nowrap [&>th]:text-fg-soft">
              <th className="w-[44%]">{t('th.video')}</th><th className="w-[30%]">{t('th.status')}</th><th className="w-[12%] text-right!">{t('th.dur')}</th><th className="w-[14%] text-right!">{t('th.added')}</th>
            </tr>
          </thead>
          <tbody>
            {rows === null && <tr><td colSpan={4} className="py-3"><Empty>{t('l.loading')}</Empty></td></tr>}
            {rows && !rows.length && <tr><td colSpan={4} className="py-3"><Empty>{t(libraryEmptyKey(project, status), { status: t(`st.${status}`) })}</Empty></td></tr>}
            {rows?.map((v) => (
              <tr key={v.id} className="border-b border-ink-3 transition-colors duration-150 last:border-b-0 hover:bg-ink-3/50 @max-[42rem]:grid @max-[42rem]:grid-cols-[minmax(0,1fr)_auto] @max-[42rem]:items-start @max-[42rem]:gap-x-3 @max-[42rem]:gap-y-2 @max-[42rem]:py-3 [&>td]:px-3 [&>td]:py-[12.5px] [&>td]:align-middle @max-[42rem]:[&>td]:p-0">
                <td className="min-w-0 @max-[42rem]:col-span-2"><a className="group flex min-w-0 items-center gap-3" href={`video.html?id=${v.id}`}>
                  <Thumb thumb={v.thumbnail} label={t('frame.of', { t: v.title })} className="w-20" />
                  <span className="min-w-0 flex-1"><span title={v.title} className="block truncate font-semibold">{v.title}</span><span className="block text-xs text-fg-soft">{tracks(v)}</span></span>
                </a></td>
                <td className="min-w-0 [overflow-wrap:anywhere]"><Pill status={v.status} />{v.status === 'deleted' && <button type="button" className={cx(btn, 'mt-1')} onClick={() => post(`/v1/videos/${v.id}/restore`).then(() => refresh()).then(() => reload()).catch(fail)}>{t('desktop.restore')}</button>}{(v.status === 'failed' || v.status === 'cancelled') && <div className="mt-2 text-xs text-fg-soft">{v.status === 'failed' && <ErrorDetails summary={t('l.processingFailed')} detail={v.error} />}<button type="button" className="mt-2 font-medium text-fg underline underline-offset-[3px]" onClick={(e) => retry(v.id, e.currentTarget)}>{t('retry')}</button></div>}</td>
                <td className="text-right @max-[42rem]:col-start-2">{v.duration_ms ? tc(v.duration_ms) : <span className="text-faint">—</span>}</td>
                <td className="text-right @max-[42rem]:hidden">{when(v.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {cursor && <p className="mt-4 text-center"><button type="button" className={btn} disabled={loadingMore} onClick={more}>{loadingMore ? t('l.loading') : t('b.more')}</button></p>}
    </Shell>
  );
}
