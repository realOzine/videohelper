// The desktop frame: a ruled T of sidebar and top bar on the ink ground, the canvas beside them.
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { get, post, type Project, type Video, type RuntimeState } from './api.ts';
import { t, useLang } from './i18n.ts';
import './strings.ts';
import { cx, errText, Icon, LangSwitch, Mark } from './ui.tsx';

export interface LibraryState { projects: Project[]; failed: number }

// The sidebar's projects and failed video count. Pages share it for their own selects.
export function useLibrary(fail: (err: unknown) => void) {
  const [ws, setWs] = useState<LibraryState | null>(null);
  const reload = useCallback(async () => {
    const [{ projects }, failed] = await Promise.all([
      get<{ projects: Project[] }>('/v1/projects'), get<{ videos: Video[] }>('/v1/videos?status=failed&limit=100'),
    ]);
    setWs({ projects, failed: failed.videos.length });
  }, []);
  useEffect(() => { reload().catch(fail); }, [reload, fail]);
  return { ws, reload };
}

// One error line at the top of the canvas.
export function useNotice() {
  const [notice, setNotice] = useState('');
  const fail = useCallback((err: unknown) => setNotice(errText(err)), []);
  return { notice, fail };
}

const navItem = 'flex h-9 items-center gap-2.5 rounded-lg px-2 text-fg-soft transition-colors duration-150 hover:bg-ink-2 hover:text-fg aria-[current=page]:bg-ink-2 aria-[current=page]:font-semibold aria-[current=page]:text-fg aria-[current=page]:shadow-edge [&>svg]:size-4';

export function Shell({ page, query = '', ws, onNewProject, notice, children }: {
  page: 'search' | 'library' | 'video'; query?: string; ws: LibraryState | null; onNewProject: () => void; notice: string; children: ReactNode;
}) {
  const lang = useLang();
  const [open, setOpen] = useState(false);
  const project = new URLSearchParams(location.search).get('project');
  useEffect(() => { document.title = t(`page.title.${page === 'video' ? 'video' : page}`); }, [page, lang]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || (e.target as HTMLElement).closest('input, select, textarea')) return;
      e.preventDefault();
      const q = document.getElementById('q') as HTMLInputElement;
      q.focus(); q.select();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);
  const here = page === 'video' ? 'library' : page;

  return (
    <div className="grid min-h-svh grid-cols-[230px_minmax(0,1fr)] grid-rows-[64px_minmax(0,1fr)] text-sm leading-5 max-md:grid-cols-[minmax(0,1fr)] max-md:grid-rows-[56px_auto]">
      <nav aria-label={t('a.nav')} className={cx('sticky top-0 row-span-2 flex h-svh flex-col gap-1 overflow-auto border-r border-ink-3 px-3 pb-4',
        'max-md:fixed max-md:inset-0 max-md:z-20 max-md:h-full max-md:w-full max-md:bg-ink max-md:transition-transform max-md:duration-200 max-md:ease-out', !open && 'max-md:-translate-x-full')}>
        <div className="-mx-3 mb-3 flex h-16 flex-none items-center justify-between border-b border-ink-3 px-5">
          <a className="flex items-center gap-2.5 text-[17px] font-bold tracking-[-0.01em]" href="search.html"><Mark />VideoHelper</a>
          <button type="button" className="hidden size-8 place-items-center text-fg-soft max-md:grid" aria-label={t('a.closenav')} onClick={() => setOpen(false)}><Icon.x className="size-4" /></button>
        </div>
        <a className={navItem} href="search.html" aria-current={here === 'search' ? 'page' : undefined}><Icon.search />{t('nav.search')}</a>
        <a className={navItem} href="library.html" aria-current={here === 'library' && !project ? 'page' : undefined}>
          <Icon.library />{t('nav.library')}
          {!!ws?.failed && <span className="ml-auto text-xs font-medium text-red-text" aria-label={t('nav.failed.aria', { n: ws.failed })}>{t('nav.failed', { n: ws.failed })}</span>}
        </a>
        <div className="mx-2 mt-5 mb-1.5 text-xs text-faint">{t('nav.projects')}</div>
        {ws?.projects.map((p) => (
          <a key={p.id} className={navItem} href={`library.html?project=${p.id}`} aria-current={project === p.id ? 'page' : undefined}>
            <Icon.folder /><span className="truncate">{p.name}</span><span className="ml-auto text-xs text-faint">{p.video_count}</span>
          </a>
        ))}
        <button type="button" className={cx(navItem, 'text-left')} onClick={onNewProject}><Icon.folder plus />{t('nav.newproject')}</button>
      </nav>

      <header className="col-start-2 flex min-w-0 items-center gap-4 border-b border-ink-3 pl-[29px] max-md:col-start-1 max-md:pl-3">
        <button type="button" className="hidden size-8 flex-none place-items-center text-fg max-md:grid" aria-label={t('top.menu')} onClick={() => setOpen(true)}><Icon.menu className="size-[18px]" /></button>
        <form role="search" action="search.html" className="flex h-9 w-[min(560px,100%)] items-center gap-2 rounded-md bg-ink-2 pr-2.5 pl-3 ring-1 ring-transparent transition-shadow duration-150 focus-within:ring-line max-xs:w-auto max-xs:min-w-0 max-xs:flex-1">
          <Icon.search className="size-4 flex-none text-fg-soft" />
          <label htmlFor="q" className="sr-only">{t('top.searchlabel')}</label>
          <input id="q" name="q" type="search" defaultValue={query} placeholder={t('top.placeholder')} className="h-full min-w-0 flex-1 bg-transparent outline-0 placeholder:text-faint" />
          <kbd className="rounded bg-ink-3 px-1.5 font-sans text-xs text-fg-soft max-xs:hidden">/</kbd>
        </form>
        <LangSwitch className="ml-auto flex gap-0.5 rounded-lg bg-ink-2 p-[3px]" button="h-[30px] min-w-10 whitespace-nowrap rounded-md px-2.5 font-medium text-fg-soft transition-colors aria-pressed:bg-ink-3 aria-pressed:text-fg" />

      </header>

      <main className="col-start-2 min-w-0 px-[29px] pt-7 pb-12 max-md:col-start-1 max-md:px-4 max-md:pt-5 max-md:pb-10">
        <RuntimeStatus />
        {notice && <p role="alert" className="mb-6 flex items-center gap-3 rounded-[10px] bg-red-soft px-4 py-3 text-neg-ink"><span className="flex-1">{notice}</span><button type="button" className="font-medium text-fg underline underline-offset-[3px]" onClick={() => location.reload()}>{t('retry')}</button></p>}
        {children}
      </main>
    </div>
  );
}

export function TitleRow({ title, sub, children }: { title: ReactNode; sub?: ReactNode; children?: ReactNode }) {
  return (
    <div className="mb-5 flex items-end justify-between gap-4 max-md:flex-wrap max-md:items-start">
      <div className="min-w-0">
        <h1 className="disp text-[32px] leading-[1.2] max-md:text-2xl">{title}</h1>
        {sub && <p className="mt-1 text-[15px] text-fg-soft">{sub}</p>}
      </div>
      {children && <div className="flex flex-none gap-2 max-xs:w-full max-xs:[&>*]:flex-1">{children}</div>}
    </div>
  );
}

export const panelBase = 'min-w-0 rounded-[10px] bg-ink-2 shadow-edge';
export const panel = `${panelBase} p-5`;
export function PanelHead({ title, id, children }: { title: ReactNode; id?: string; children?: ReactNode }) {
  return (
    <header className="mb-4 flex items-center justify-between gap-3">
      <h2 id={id} className="text-lg leading-[1.3] font-semibold">{title}</h2>
      {children}
    </header>
  );
}
export const Meta = ({ children }: { children: ReactNode }) => <span className="text-xs text-faint">{children}</span>;
export const Empty = ({ children }: { children: ReactNode }) => (
  <div className="grid min-h-24 w-full place-items-center rounded-lg bg-ink-3/45 p-4 text-center text-fg-soft">{children}</div>
);

// New project by name; the sidebar refreshes in place.
export async function newProject(reload: () => Promise<void>, fail: (err: unknown) => void) {
  const name = prompt(t('p.name'));
  if (!name || !name.trim()) return;
  try { await post('/v1/projects', { name: name.trim() }); await reload(); } catch (err) { fail(err); }
}

function RuntimeStatus() {
  const [state, setState] = useState<RuntimeState | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    let stopped = false;
    const poll = async () => {
      try { const next = await get<RuntimeState>('/v1/desktop/runtime'); if (!stopped) { setState(next); setError(''); } }
      catch (err) { if (!stopped) setError(err instanceof Error ? err.message : String(err)); }
    };
    poll(); const timer = window.setInterval(poll, 2000);
    return () => { stopped = true; clearInterval(timer); };
  }, []);
  return <section aria-live="polite" className="mb-6 rounded-[10px] bg-ink-2 px-4 py-3 shadow-edge">
    <div className="flex flex-wrap items-center gap-2 text-[13px]">
      <span className={cx('size-1.5 rounded-full', state?.ready ? 'bg-pos' : state?.error || error ? 'bg-red' : 'animate-beat bg-red')} />
      <span className="font-medium">{state?.ready ? t('desktop.ready') : t('desktop.preparing')}</span>
      {state?.device && <span title={state.reason} className="rounded bg-ink-3 px-1.5 font-mono text-xs text-fg-soft">{state.device.toUpperCase()}</span>}
      <span className="min-w-0 text-fg-soft">{state?.error || error || (!state?.ready && state?.message)}</span>
      {state?.error && <button type="button" className="ml-auto text-fg underline underline-offset-[3px]" onClick={() => post('/v1/desktop/runtime/retry').catch((err) => setError(errText(err)))}>{t('retry')}</button>}
    </div>
    {state?.fraction != null && !state.ready && <progress aria-label={t('desktop.preparing')} className="mt-2 h-1 w-full accent-red" value={state.fraction} max={1} />}
  </section>;
}
