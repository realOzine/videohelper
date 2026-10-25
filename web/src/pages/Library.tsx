// Library: uploads run in this page (parts straight to storage, then complete), the list polls while anything processes.
import { useCallback, useEffect, useReducer, useRef, useState } from 'react';
import { get, post, type Video } from '../api.ts';
import { t, useLang } from '../i18n.ts';
import { eachLimit, partRange, tc } from '../lib.ts';
import { Empty, newProject, panel, PanelHead, Shell, TitleRow, useNotice, useWorkspace } from '../Shell.tsx';
import { btn, btnPrimary, cx, errText, Icon, Pill, select, Thumb } from '../ui.tsx';
import { busy, tracks, when } from './format.ts';

interface Upload { key: number; name: string; size: number; done: number; total: number; state: 'run' | 'done' | 'fail'; error?: string }
interface Page { videos: Video[]; next_cursor: string | null }
const params = new URLSearchParams(location.search);
const status = params.get('status') || '', project = params.get('project') || '';
const query = (extra: Record<string, string>) => new URLSearchParams({ limit: '50', ...(status && { status }), ...(project && { project_id: project }), ...extra });
const filter = (extra: Record<string, string>) => `library.html?${new URLSearchParams({ ...(project && { project }), ...extra })}`;
const rule = 'border-t border-ink-3 first:border-t-0';

export default function Library() {
  useLang();
  const { notice, fail } = useNotice();
  const { ws, reload } = useWorkspace(fail);
  const [rows, setRows] = useState<Video[] | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [attention, setAttention] = useState<{ failed: Page; indexing: Page } | null>(null);
  const ups = useRef<Upload[]>([]), timer = useRef(0), file = useRef<HTMLInputElement>(null);
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

  async function upload(f: File) {
    const u: Upload = { key: Math.random(), name: f.name, size: f.size, done: 0, total: 0, state: 'run' };
    ups.current = [u, ...ups.current]; bump();
    try {
      const { video, upload: plan } = await post<{ video: Video; upload: { upload_id: string; part_size: number; parts: { n: number; url: string }[] } }>('/v1/uploads',
        { filename: f.name, size_bytes: f.size, content_type: f.type || 'application/octet-stream', project_id: project || undefined });
      u.total = plan.parts.length; bump();
      const parts: { n: number; etag: string }[] = [];
      await eachLimit(plan.parts, 4, async (p) => {
        const [a, b] = partRange(p.n, plan.part_size, f.size);
        const res = await fetch(p.url, { method: 'PUT', body: f.slice(a, b) });
        const etag = res.headers.get('ETag');
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        if (!etag) throw new Error(t('u.noetag'));
        parts.push({ n: p.n, etag }); u.done++; bump();
      });
      await post(`/v1/videos/${video.id}/complete`, { upload_id: plan.upload_id, parts: parts.sort((x, y) => x.n - y.n) });
      u.state = 'done';
    } catch (err) {
      u.state = 'fail'; u.error = errText(err);
    }
    bump();
    if (u.state === 'done') refresh().catch(fail);
  }
  // closing the page stops the parts in flight
  useEffect(() => {
    const warn = (e: BeforeUnloadEvent) => { if (ups.current.some((u) => u.state === 'run')) e.preventDefault(); };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, []);

  const att = attention ? [...attention.failed.videos, ...attention.indexing.videos] : [];
  const more2 = (p: Page) => p.videos.length + (p.next_cursor ? '+' : '');

  return (
    <Shell page="library" ws={ws} notice={notice} onNewProject={() => newProject(reload, fail)}>
      <TitleRow title={t('nav.library')} sub={attention ? t('l.sub2', { i: more2(attention.indexing), f: more2(attention.failed) }) : ' '}>
        <button type="button" className={btn} onClick={() => newProject(reload, fail)}>{t('b.newproject')}</button>
        <button type="button" className={btnPrimary.replace('h-8', 'h-9')} onClick={() => file.current?.click()}>{t('b.upload')}</button>
        <input ref={file} type="file" accept="video/*" multiple hidden onChange={(e) => { [...(e.target.files || [])].forEach(upload); e.target.value = ''; }} />
      </TitleRow>

      <div className="mb-6 grid grid-cols-12 gap-6">
        <section aria-labelledby="uh" className={cx(panel, 'col-span-7 max-md:col-span-12')}>
          <PanelHead id="uh" title={t('h.uploading')}><span className="text-xs text-faint">{t('meta.direct')}</span></PanelHead>
          {ups.current.length ? ups.current.map((u) => {
            const pct = u.total ? u.done / u.total : 0;
            return (
              <div key={u.key} className={cx('grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1 py-2.5', rule)}>
                <span className="truncate font-medium">{u.name}</span>
                <span className="text-right text-fg-soft">{u.state === 'done' ? t('l.submitted') : u.state === 'fail' ? t('u.failed', { e: u.error || '' }) : `${Math.round(pct * 100)}%`}</span>
                <div className="col-span-2 grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 text-xs text-fg-soft">
                  <span>{t('parts', { a: u.done, b: u.total || '…' })}</span>
                  <span className={cx('h-1.5 overflow-hidden rounded-full', u.state === 'done' ? 'bg-pos-bg' : 'bg-red-track')}>
                    <i className={cx('block h-full origin-left rounded-full transition-transform duration-[600ms] ease-out', u.state === 'done' ? 'bg-pos' : 'bg-red')} style={{ transform: `scaleX(${pct})` }} />
                  </span>
                  <span>{(u.size / 1048576).toFixed(0)} MB</span>
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
        {[['', t('tab.all')], ...['indexing', 'ready', 'failed', 'uploading'].map((s) => [s, t(`st.${s}`)])].map(([s, label]) => (
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
                <td><Pill status={v.status} />{v.status === 'failed' && <span className="block text-xs text-fg-soft">{v.error}<button type="button" className="ml-2 font-medium text-fg underline underline-offset-[3px]" onClick={(e) => retry(v.id, e.currentTarget)}>{t('retry')}</button></span>}</td>
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
