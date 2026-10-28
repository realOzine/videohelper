// The desktop frame: a ruled T of sidebar and top bar on the ink ground, the canvas beside them.
import { useCallback, useEffect, useRef, useState, type MouseEventHandler, type ReactNode } from 'react';
import { get, post, type Project, type Video, type RuntimeState } from './api.ts';
import { t, useLang } from './i18n.ts';
import './strings.ts';
import { usePageQuery } from './pageQuery.ts';
import { runtimeMessageKey } from './feedback.ts';
import { cx, ErrorDetails, errorSummary, errText, Icon, LangSwitch, Logo } from './ui.tsx';

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
  const [notice, setNotice] = useState<unknown>(null);
  const fail = useCallback((err: unknown) => setNotice(() => err), []);
  return { notice, fail };
}

const navItem = 'desktop-nav';

export function Shell({ page, query = '', ws, onNewProject, notice, children, onQueryLink }: {
  page: 'search' | 'library' | 'video'; query?: string; ws: LibraryState | null; onNewProject: () => void; notice: unknown; children: ReactNode; onQueryLink?: MouseEventHandler<HTMLDivElement>;
}) {
  const lang = useLang();
  const navDialog = useRef<HTMLDialogElement>(null), navButton = useRef<HTMLButtonElement>(null);
  const project = usePageQuery().get('project');
  useEffect(() => {
    const wide = window.matchMedia('(min-width: 60rem)');
    const closeOnWide = () => { if (wide.matches) navDialog.current?.close(); };
    wide.addEventListener('change', closeOnWide);
    return () => wide.removeEventListener('change', closeOnWide);
  }, []);
  useEffect(() => { document.title = t(`page.title.${page === 'video' ? 'video' : page}`); }, [page, lang]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (navDialog.current?.open || e.key !== '/' || (e.target as HTMLElement).closest('input, select, textarea')) return;
      e.preventDefault();
      const q = document.getElementById('q') as HTMLInputElement;
      q.focus(); q.select();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);
  const here = page === 'video' ? 'library' : page;

  const navigation = () => <>
        <div className="mb-5 flex h-[92px] flex-none items-center justify-between px-2">
          <a href="search.html"><Logo /></a>
          <button type="button" autoFocus className="grid size-8 place-items-center text-fg-soft lg:hidden" aria-label={t('a.closenav')} onClick={() => navDialog.current?.close()}><Icon.x className="size-4" /></button>
        </div>
        <a className={navItem} href="search.html" aria-current={here === 'search' ? 'page' : undefined}><Icon.search />{t('nav.search')}</a>
        <a className={navItem} href="library.html" aria-current={here === 'library' && !project ? 'page' : undefined}>
          <Icon.library />{t('nav.library')}
          {!!ws?.failed && <span className="nav-failed ml-auto text-xs font-medium" aria-label={t('nav.failed.aria', { n: ws.failed })}>{t('nav.failed', { n: ws.failed })}</span>}
        </a>
        <div className="mx-3 mt-7 mb-1.5 text-[11px] font-medium tracking-[0.12em] text-faint uppercase">{t('nav.projects')}</div>
        {ws?.projects.map((p) => (
          <a key={p.id} className={navItem} href={`library.html?project=${p.id}`} aria-current={project === p.id ? 'page' : undefined}>
            <Icon.folder /><span className="truncate">{p.name}</span><span className="nav-count ml-auto text-xs">{p.video_count}</span>
          </a>
        ))}
        <button type="button" className={cx(navItem, 'text-left')} onClick={() => { navDialog.current?.close(); onNewProject(); }}><Icon.folder plus />{t('nav.newproject')}</button>
  </>;

  return (
    <div onClick={(e) => { onQueryLink?.(e); if (e.defaultPrevented) navDialog.current?.close(); }} className="grid min-h-svh grid-cols-[240px_minmax(0,1fr)] grid-rows-[76px_minmax(0,1fr)] text-sm leading-5 max-lg:grid-cols-[minmax(0,1fr)] max-lg:grid-rows-[68px_auto]">
      <nav aria-label={t('a.nav')} className="sticky top-0 row-span-2 flex h-svh flex-col gap-1.5 overflow-auto border-r border-ink-3 bg-ink-2/45 px-4 pb-5 max-lg:hidden">{navigation()}</nav>
      <dialog id="mobile-nav" ref={navDialog} aria-label={t('a.nav')} onClose={() => navButton.current?.focus()} className="fixed inset-0 m-0 h-dvh max-h-none w-full max-w-none overflow-y-auto border-0 bg-ink px-4 pb-5 text-fg backdrop:bg-ink/60 lg:hidden">
        <nav className="flex min-h-full flex-col gap-1.5">{navigation()}</nav>
      </dialog>

      <header className="col-start-2 flex min-w-0 items-center gap-4 border-b border-ink-3 px-8 max-lg:col-start-1 max-lg:px-4">
        <button ref={navButton} type="button" className="hidden size-8 flex-none place-items-center text-fg max-lg:grid" aria-label={t('top.menu')} aria-haspopup="dialog" aria-controls="mobile-nav" onClick={() => navDialog.current?.showModal()}><Icon.menu className="size-[18px]" /></button>
        <form role="search" action="search.html" className="flex h-11 w-[min(640px,100%)] items-center gap-3 rounded-full border border-line bg-ink-2 pr-3.5 pl-4 transition-colors duration-150 focus-within:border-fg-soft max-xs:w-auto max-xs:min-w-0 max-xs:flex-1">
          {project && <input type="hidden" name="project" value={project} />}
          <Icon.search className="size-4 flex-none text-fg-soft" />
          <label htmlFor="q" className="sr-only">{t('top.searchlabel')}</label>
          <input id="q" name="q" type="search" defaultValue={query} placeholder={t('top.placeholder')} className="h-full min-w-0 flex-1 bg-transparent outline-0 placeholder:text-faint" />
          <kbd className="rounded bg-ink-3 px-1.5 font-sans text-xs text-fg-soft max-xs:hidden">/</kbd>
        </form>
        <LangSwitch className="ml-auto flex flex-none gap-0.5 rounded-full border border-ink-3 p-[3px]" button="h-[30px] min-w-10 whitespace-nowrap rounded-full px-2.5 font-medium text-fg-soft transition-colors aria-pressed:bg-ink-3 aria-pressed:text-fg" />

      </header>

      <main className="col-start-2 min-w-0 px-8 pt-9 pb-12 max-lg:col-start-1 max-lg:px-4 max-lg:pt-5 max-lg:pb-10">
        <RuntimeStatus />
        {notice != null && notice !== '' && <section role="alert" className="mb-6 flex flex-wrap items-start gap-3 rounded-[10px] bg-red-soft px-4 py-3 text-neg-ink"><div className="min-w-0 flex-1"><ErrorDetails summary={errorSummary(notice)} detail={errText(notice)} /></div><button type="button" className="font-medium text-fg underline underline-offset-[3px]" onClick={() => location.reload()}>{t('retry')}</button></section>}
        {children}
      </main>
    </div>
  );
}

export function TitleRow({ title, sub, children }: { title: ReactNode; sub?: ReactNode; children?: ReactNode }) {
  return (
    <div className="mb-7 flex items-end justify-between gap-5 max-lg:flex-wrap max-lg:items-start">
      <div className="min-w-0">
        <h1 className="disp text-[30px] leading-[1.3] max-lg:text-2xl">{title}</h1>
        {sub && <p className="mt-2 max-w-[72ch] text-sm leading-relaxed text-fg-soft">{sub}</p>}
      </div>
      {children && <div className="flex flex-none gap-2 max-xs:w-full max-xs:[&>*]:flex-1">{children}</div>}
    </div>
  );
}

export const panelBase = 'min-w-0 rounded-2xl border border-ink-3 bg-ink-2';
export const panel = `${panelBase} p-5`;
export function PanelHead({ title, id, children }: { title: ReactNode; id?: string; children?: ReactNode }) {
  return (
    <header className="mb-4 flex flex-wrap items-center justify-between gap-3">
      <h2 id={id} className="text-lg leading-[1.3] font-semibold">{title}</h2>
      {children}
    </header>
  );
}
export const Meta = ({ children }: { children: ReactNode }) => <span className="text-xs text-faint">{children}</span>;
export const Empty = ({ children }: { children: ReactNode }) => (
  <div className="grid min-h-28 w-full place-items-center rounded-xl border border-dashed border-line bg-ink/40 p-5 text-center text-[13px] leading-relaxed text-fg-soft">{children}</div>
);

// New project by name; the sidebar refreshes in place.
export async function newProject(reload: () => Promise<void>, fail: (err: unknown) => void) {
  const name = prompt(t('p.name'));
  if (!name || !name.trim()) return;
  try { await post('/v1/projects', { name: name.trim() }); await reload(); } catch (err) { fail(err); }
}

function RuntimeStatus() {
  const [state, setState] = useState<RuntimeState | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [retrying, setRetrying] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    let timer = 0;
    const poll = async () => {
      try {
        const next = await get<RuntimeState>('/v1/desktop/runtime', controller.signal);
        if (!controller.signal.aborted) { setState(next); setError(null); }
      } catch (err) { if (!controller.signal.aborted) setError(err); }
      finally { if (!controller.signal.aborted) timer = window.setTimeout(poll, 2000); }
    };
    poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, []);
  const failed = Boolean(state?.error || error || state?.stage === 'failed');
  const retry = async () => {
    setRetrying(true);
    try { await post('/v1/desktop/runtime/retry'); }
    catch (err) { setError(err); }
    finally { setRetrying(false); }
  };
  return <section aria-live="polite" className="mb-6 rounded-xl border border-ink-3 bg-ink-2/60 px-4 py-3">
    <div className="flex flex-wrap items-center gap-2 text-[13px]">
      <span className={cx('size-1.5 flex-none rounded-full', failed ? 'bg-red' : state?.ready ? 'bg-pos' : 'animate-beat bg-fg-soft')} />
      <span className="font-medium">{failed ? t('desktop.failed') : state?.ready ? t('desktop.ready') : t('desktop.preparing')}</span>
      {state?.device && <span title={state.reason} className="rounded bg-ink-3 px-1.5 font-mono text-xs text-fg-soft">{state.device.toUpperCase()}</span>}
      {state?.error && <button type="button" disabled={retrying} className="ml-auto text-fg underline underline-offset-[3px] disabled:opacity-50" onClick={retry}>{t('retry')}</button>}
    </div>
    {failed ? <div className="mt-2"><ErrorDetails summary={error ? errorSummary(error) : t('desktop.failureHelp')} detail={state?.error || (error ? errText(error) : undefined)} /></div>
      : !state?.ready && <p className="mt-2 text-xs leading-relaxed text-fg-soft">{t(runtimeMessageKey(state?.stage))}</p>}
    {state?.fraction != null && !state.ready && !failed && <progress aria-label={t('desktop.preparing')} className="mt-2 h-1 w-full accent-red" value={state.fraction} max={1} />}
  </section>;
}
