// The desktop frame: a title bar under the window controls carries the navigation, or an optional sidebar does.
import { useCallback, useEffect, useRef, useState, useSyncExternalStore, type MouseEventHandler, type ReactNode } from 'react';
import { get, post, type Project, type Video, type RuntimeState } from './api.ts';
import { t, useLang } from './i18n.ts';
import './strings.ts';
import { usePageQuery } from './pageQuery.ts';
import { runtimeMessageKey } from './feedback.ts';
import { cx, ErrorDetails, errorSummary, errText, Icon, Mark } from './ui.tsx';

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

const SIDEBAR = 'vh-sidebar';
const current = (on: boolean) => (on ? 'page' as const : undefined);

export function Shell({ page, query = '', ws, onNewProject, notice, children, onQueryLink }: {
  page: 'search' | 'library' | 'video' | 'settings'; query?: string; ws: LibraryState | null; onNewProject: () => void; notice: unknown; children: ReactNode; onQueryLink?: MouseEventHandler<HTMLDivElement>;
}) {
  const lang = useLang();
  const project = usePageQuery().get('project');
  // The sidebar is a choice kept in this app; narrow windows always use the title bar.
  const [side, setSide] = useState(() => { try { return localStorage.getItem(SIDEBAR) === '1'; } catch { return false; } });
  const toggleSide = () => {
    try { localStorage.setItem(SIDEBAR, side ? '0' : '1'); } catch { /* not remembered, still switched */ }
    setSide(!side);
  };
  useEffect(() => { document.title = t(`page.title.${page}`); }, [page, lang]);
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
  const failed = !!ws?.failed && <span className="fail-count" aria-label={t('nav.failed.aria', { n: ws.failed })}>{ws.failed}</span>;
  const crumb = (here === 'settings' ? [t('nav.settings')] : here === 'search' ? [t('nav.search'), query] : [t('nav.library'), ws?.projects.find((p) => p.id === project)?.name]).filter(Boolean);

  return (
    <div onClick={onQueryLink} className="flex min-h-svh text-sm leading-5">
      {side && (
        <aside aria-label={t('a.nav')} className="sticky top-0 hidden h-svh w-60 flex-none flex-col gap-0.5 overflow-y-auto border-r border-ink-3 bg-ink-2/45 px-3 pb-3 lg:flex">
          <div className="flex h-14 flex-none items-center pl-[calc(var(--mygo-titlebar-inset-left,0px)+4px)] [--app-region:drag]">
            <a href="search.html" aria-label="VideoHelper" className="group grid size-8 place-items-center"><Mark className="size-[26px] group-hover:animate-wiggle" /></a>
          </div>
          <div className="mx-2.5 mt-4 mb-1.5 text-xs text-faint">{t('nav.browse')}</div>
          <a className="side-item" href="search.html" aria-current={current(here === 'search')}><Icon.search />{t('nav.search')}</a>
          <a className="side-item" href="library.html" aria-current={current(here === 'library' && !project)}><Icon.library />{t('nav.library')}{failed}</a>
          <div className="mx-2.5 mt-5 mb-1.5 text-xs text-faint">{t('nav.projects')}</div>
          <ProjectItems ws={ws} project={project} onNewProject={onNewProject} />
          <div className="mt-auto border-t border-ink-3 pt-2"><RuntimeDot /></div>
        </aside>
      )}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className={cx('sticky top-0 z-20 grid h-14 grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-4 border-b border-ink-3 bg-ink pr-4 pl-[calc(var(--mygo-titlebar-inset-left,0px)+16px)] [--app-region:drag]', side && 'lg:pl-4')}>
          <div className="flex min-w-0 items-center gap-2">
            <a href="search.html" aria-label="VideoHelper" className={cx('group grid size-8 flex-none place-items-center', side && 'lg:hidden')}><Mark className="size-[26px] group-hover:animate-wiggle" /></a>
            <button type="button" aria-label={t('a.sidebar')} aria-pressed={side} onClick={toggleSide} className="hidden size-8 flex-none place-items-center rounded-lg text-fg-soft transition-colors hover:bg-ink-2 hover:text-fg lg:grid"><Icon.sidebar className="size-[18px]" /></button>
            {side && <span className="ml-1 hidden min-w-0 items-center gap-2 text-[15px] whitespace-nowrap text-faint lg:flex">
              {crumb.map((c, i) => i ? <span key={i} className="flex min-w-0 gap-2"><span aria-hidden>/</span><b className="truncate font-medium text-fg">{c}</b></span> : <span key={i}>{c}</span>)}
            </span>}
          </div>
          <nav aria-label={t('a.nav')} className={cx('flex items-center gap-1', side && 'lg:hidden')}>
            <a className="top-tab" href="search.html" aria-current={current(here === 'search')}>{t('nav.search')}</a>
            <a className="top-tab" href="library.html" aria-current={current(here === 'library' && !project)}>{t('nav.library')}{failed}</a>
            <ProjectMenu ws={ws} project={project} onNewProject={onNewProject} />
          </nav>
          <div className="flex min-w-0 items-center justify-end gap-2.5">
            <form role="search" action="search.html" className="flex h-8 w-60 min-w-0 items-center gap-2 rounded-full border border-edge bg-ink-2 pr-2 pl-3 transition-colors focus-within:border-fg-soft max-lg:w-36 max-sm:hidden">
              {project && <input type="hidden" name="project" value={project} />}
              <Icon.search className="size-3.5 flex-none text-faint" />
              <label htmlFor="q" className="sr-only">{t('top.searchlabel')}</label>
              <input id="q" name="q" type="search" defaultValue={query} placeholder={t('top.placeholder')} className="h-full min-w-0 flex-1 bg-transparent text-[13px] outline-0 placeholder:text-faint" />
              <kbd className="rounded bg-ink-3 px-1.5 font-sans text-[11px] text-fg-soft max-lg:hidden">/</kbd>
            </form>
            <RuntimeDot className={cx(side && 'lg:hidden')} />
            <a href="settings.html" aria-label={t('nav.settings')} title={t('nav.settings')} aria-current={current(page === 'settings')}
              className="group grid size-8 flex-none place-items-center rounded-lg text-fg-soft transition-colors hover:bg-ink-2 hover:text-fg aria-[current=page]:bg-ink-3 aria-[current=page]:text-fg">
              <Icon.gear className="size-[18px] transition-transform duration-[400ms] ease-out group-hover:rotate-45" />
            </a>
          </div>
        </header>

        <main className="min-w-0 px-8 pt-8 pb-12 max-lg:px-4 max-lg:pt-5 max-lg:pb-10">
          <RuntimeBanner />
          {notice != null && notice !== '' && <section role="alert" className="mb-6 flex flex-wrap items-start gap-3 rounded-[10px] bg-red-soft px-4 py-3 text-neg-ink"><div className="min-w-0 flex-1"><ErrorDetails summary={errorSummary(notice)} detail={errText(notice)} /></div><button type="button" className="font-medium text-fg underline underline-offset-[3px]" onClick={() => location.reload()}>{t('retry')}</button></section>}
          {children}
        </main>
      </div>
    </div>
  );
}

function ProjectItems({ ws, project, onNewProject, className }: { ws: LibraryState | null; project: string | null; onNewProject: () => void; className?: string }) {
  return <>
    {ws?.projects.map((p) => (
      <a key={p.id} className={cx('side-item', className)} href={`library.html?project=${p.id}`} aria-current={current(project === p.id)}>
        <Icon.folder /><span className="truncate">{p.name}</span><span className="ml-auto text-xs font-normal text-faint">{p.video_count}</span>
      </a>
    ))}
    <button type="button" className={cx('side-item', className)} onClick={onNewProject}><Icon.folder plus />{t('nav.newproject')}</button>
  </>;
}

// The title bar's project list opens below its tab and closes on Escape or a click elsewhere.
function ProjectMenu(props: { ws: LibraryState | null; project: string | null; onNewProject: () => void }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => { if (e instanceof KeyboardEvent ? e.key === 'Escape' : !box.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener('pointerdown', close); document.addEventListener('keydown', close);
    return () => { document.removeEventListener('pointerdown', close); document.removeEventListener('keydown', close); };
  }, [open]);
  return (
    <div ref={box} className="relative">
      <button type="button" className="top-tab" aria-expanded={open} aria-current={current(!!props.project)} onClick={() => setOpen(!open)}>
        {t('nav.projects')}<Icon.chevron className={cx('size-3 transition-transform', open ? '-rotate-90' : 'rotate-90')} />
      </button>
      {open && (
        <div className="absolute top-11 left-0 z-30 grid w-56 gap-0.5 rounded-xl border border-edge bg-ink-2 p-1.5 shadow-[0_16px_40px_rgb(0_0_0/0.45)] [--app-region:no-drag]">
          <ProjectItems {...props} className="before:hidden" onNewProject={() => { setOpen(false); props.onNewProject(); }} />
        </div>
      )}
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

export const panelBase = 'min-w-0 rounded-2xl border border-edge bg-ink-2';
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

// One poll of the local runtime, shared by the title bar, the banner and the settings page.
let runtime: { state: RuntimeState | null; error: unknown } = { state: null, error: null };
const runtimeSubs = new Set<() => void>();
const notifyRuntime = () => runtimeSubs.forEach((fn) => fn());
let stopRuntime = () => {};
function subscribeRuntime(fn: () => void) {
  runtimeSubs.add(fn);
  if (runtimeSubs.size === 1) {
    const controller = new AbortController();
    let timer = 0;
    const poll = async () => {
      try {
        const next = await get<RuntimeState>('/v1/desktop/runtime', controller.signal);
        if (!controller.signal.aborted) runtime = { state: next, error: null };
      } catch (err) { if (!controller.signal.aborted) runtime = { ...runtime, error: err }; }
      finally { if (!controller.signal.aborted) { notifyRuntime(); timer = window.setTimeout(poll, 2000); } }
    };
    poll();
    stopRuntime = () => { controller.abort(); clearTimeout(timer); };
  }
  return () => { runtimeSubs.delete(fn); if (!runtimeSubs.size) stopRuntime(); };
}
export function useRuntime() {
  const { state, error } = useSyncExternalStore(subscribeRuntime, () => runtime);
  const failed = Boolean(state?.error || error || state?.stage === 'failed');
  return { state, error, failed, ready: !failed && !!state?.ready };
}
export async function retryRuntime() {
  try { await post('/v1/desktop/runtime/retry'); } catch (err) { runtime = { ...runtime, error: err }; notifyRuntime(); }
}

// Ready is a green dot in the title bar; it opens the settings page, which has the details.
function RuntimeDot({ className }: { className?: string }) {
  const { state, failed, ready } = useRuntime();
  const label = failed ? t('desktop.failed') : ready ? t('desktop.readyShort') : t('desktop.preparing');
  return (
    <a href="settings.html" title={state?.reason || label} className={cx('inline-flex h-8 min-w-0 flex-none items-center gap-2 rounded-full px-2.5 text-[13px] whitespace-nowrap text-fg-soft transition-colors hover:bg-ink-2 hover:text-fg', className)}>
      <i className={cx('size-[7px] flex-none rounded-full', failed ? 'bg-red' : ready ? 'bg-pos' : 'animate-beat bg-fg-soft')} />
      <span className="truncate max-lg:sr-only">{label}</span>
      {state?.device && <code className="font-mono text-[11px] text-faint max-lg:hidden">{state.device.toUpperCase()}</code>}
    </a>
  );
}

// Preparing or failing, the runtime opens a band above the page with its progress or its error.
function RuntimeBanner() {
  const { state, error, failed, ready } = useRuntime();
  const [retrying, setRetrying] = useState(false);
  if (ready || (!state && !error)) return null;
  const retry = async () => { setRetrying(true); await retryRuntime(); setRetrying(false); };
  return <section aria-live="polite" className="mb-6 rounded-xl border border-edge bg-ink-2/60 px-4 py-3">
    <div className="flex flex-wrap items-center gap-2 text-[13px]">
      <span className={cx('size-1.5 flex-none rounded-full', failed ? 'bg-red' : 'animate-beat bg-fg-soft')} />
      <span className="font-medium">{failed ? t('desktop.failed') : t('desktop.preparing')}</span>
      {state?.device && <span title={state.reason} className="rounded bg-ink-3 px-1.5 font-mono text-xs text-fg-soft">{state.device.toUpperCase()}</span>}
      {state?.error && <button type="button" disabled={retrying} className="ml-auto text-fg underline underline-offset-[3px] disabled:opacity-50" onClick={retry}>{t('retry')}</button>}
    </div>
    {failed ? <div className="mt-2"><ErrorDetails summary={error ? errorSummary(error) : t('desktop.failureHelp')} detail={state?.error || (error ? errText(error) : undefined)} /></div>
      : <p className="mt-2 text-xs leading-relaxed text-fg-soft">{t(runtimeMessageKey(state?.stage))}</p>}
    {state?.fraction != null && !failed && <progress aria-label={t('desktop.preparing')} className="mt-2 h-1 w-full accent-red" value={state.fraction} max={1} />}
  </section>;
}
