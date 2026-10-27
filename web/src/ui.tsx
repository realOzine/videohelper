// Small pieces shared by every page: icons, the mark, pills, thumbnails, buttons, rich strings, entrances.
import { useEffect, useRef, type CSSProperties, type ReactNode } from 'react';
import { ApiError, settings, type Thumbnail } from './api.ts';
import { setLang, t, useLang } from './i18n.ts';
import { marks } from './lib.ts';

export const cx = (...parts: (string | false | null | undefined)[]) => parts.filter(Boolean).join(' ');

const svg = { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeLinecap: 'round' as const, 'aria-hidden': true };
export const Icon = {
  search: (p: { className?: string }) => <svg {...svg} strokeWidth={1.6} className={p.className}><circle cx="11" cy="11" r="6.5" /><path d="M20 20l-4-4" /></svg>,
  library: (p: { className?: string }) => <svg {...svg} strokeWidth={1.5} className={p.className}><rect x="3" y="6" width="18" height="13" rx="2" /><path d="M6 3h12" /><path d="M10.5 10v5l4-2.5z" fill="currentColor" /></svg>,
  folder: (p: { className?: string; plus?: boolean }) => <svg {...svg} strokeWidth={1.5} className={p.className}>{p.plus && <path d="M12 10v6M9 13h6" />}<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" /></svg>,
  play: (p: { className?: string }) => <svg viewBox="0 0 24 24" aria-hidden className={p.className}><path d="M7 4.5v15l12.5-7.5z" fill="currentColor" /></svg>,
  pause: (p: { className?: string }) => <svg viewBox="0 0 24 24" aria-hidden className={p.className}><path d="M7 4.5h3.5v15H7zM13.5 4.5H17v15h-3.5z" fill="currentColor" /></svg>,
  check: (p: { className?: string }) => <svg {...svg} strokeWidth={3.2} className={p.className}><path d="M5 12.5l4.5 4.5L19 7.5" /></svg>,
  x: (p: { className?: string }) => <svg {...svg} strokeWidth={3.2} className={p.className}><path d="M7 7l10 10M17 7L7 17" /></svg>,
  menu: (p: { className?: string }) => <svg {...svg} strokeWidth={1.6} className={p.className}><path d="M4 7h16M4 12h16M4 17h16" /></svg>,
  arrow: (p: { className?: string }) => <svg {...svg} strokeWidth={2.2} className={p.className}><path d="M7 17 17 7M9 7h8v8" /></svg>,
  grid: (p: { className?: string }) => <svg {...svg} strokeWidth={1.7} className={p.className}><rect x="3" y="4" width="8" height="7" rx="1.5" /><rect x="13" y="4" width="8" height="7" rx="1.5" /><rect x="3" y="13" width="8" height="7" rx="1.5" /><rect x="13" y="13" width="8" height="7" rx="1.5" /></svg>,
  detail: (p: { className?: string }) => <svg {...svg} strokeWidth={1.7} className={p.className}><rect x="3" y="4" width="11" height="16" rx="1.5" /><path d="M17 5h4M17 10h4M17 15h4M17 19h4" /></svg>,
  chevron: (p: { className?: string }) => <svg {...svg} strokeWidth={1.5} viewBox="0 0 12 12" className={p.className}><path d="M4.5 2.5 8 6l-3.5 3.5" strokeLinejoin="round" /></svg>,
};

// the mark (Design/Icon, small reversed cut for 24 px and under): three tracks, picture, speech and sound, in the text
// colour, and the red playhead through the moment all three meet
export const Mark = ({ className = 'size-[26px]' }: { className?: string }) => (
  <svg viewBox="0 0 256 256" aria-hidden className={cx('flex-none', className)}>
    <g fill="currentColor">
      <rect x="17" y="33" width="78" height="46" rx="23" /><rect x="169" y="33" width="46" height="46" rx="23" />
      <rect x="49" y="105" width="46" height="46" rx="23" /><rect x="169" y="105" width="70" height="46" rx="23" />
      <rect x="33" y="177" width="62" height="46" rx="23" /><rect x="169" y="177" width="54" height="46" rx="23" />
    </g>
    <rect x="117" y="9" width="30" height="238" rx="15" fill="#f43d3f" />
  </svg>
);

// ready in green, failed in red; work in progress stays neutral with a beating red dot, so it never reads as an error
const TONE: Record<string, string> = { ready: 'bg-pos-bg text-pos-ink', failed: 'bg-red-soft text-neg-ink' };
export const Pill = ({ status, className }: { status: string; className?: string }) => (
  <span className={cx('inline-flex h-[22px] items-center gap-1.5 whitespace-nowrap rounded-full px-2 font-sans text-xs font-medium normal-case tracking-normal', TONE[status] || 'bg-ink-3 text-fg', className)}>
    {!TONE[status] && <i className="size-1.5 animate-beat rounded-full bg-red" />}
    {t(`st.${status}`)}
  </span>
);

// One tile of a thumbnail sprite, cropped by the viewBox and scaled to fill its box.
export function Thumb({ thumb, label = '', className, children }: { thumb: Thumbnail | null; label?: string; className?: string; children?: ReactNode }) {
  return (
    <span className={cx('relative block aspect-video flex-none overflow-hidden rounded-[4px]', thumb ? 'bg-black' : 'grid place-items-center bg-ink-3', className)}>
      {!thumb && <Icon.library className="size-[38%] max-w-6 text-faint/40" />}
      {thumb && (
        <svg viewBox={`${thumb.x} ${thumb.y} ${thumb.w} ${thumb.h}`} preserveAspectRatio="xMidYMid slice" role="img" aria-label={label} className="size-full transition-transform duration-[420ms] ease-out group-hover:scale-105">
          <image href={thumb.url} />
        </svg>
      )}
      {children}
    </span>
  );
}
export const TcBadge = ({ children }: { children: ReactNode }) => (
  <span className="absolute right-1 bottom-1 rounded-[4px] bg-ink/80 px-[5px] text-[11px] leading-[18px] text-white">{children}</span>
);

// className replaces the default size and colour rather than adding to them (see the note on btn below)
export const Ch = ({ c, children, className = 'text-xs text-fg-soft' }: { c: string; children: ReactNode; className?: string }) => (
  <span className={cx('ch', `ch-${c}`, className)}>{children}</span>
);

// Dictionary strings with <b>/<code> render as elements; everything else is text, so nothing is injected.
export function Rich({ k, vars }: { k: string; vars?: Record<string, string | number> }) {
  return <>{marks(t(k, vars)).map((m, i) => (m.tag === 'b' ? <b key={i} className="font-medium text-fg">{m.text}</b> : m.tag === 'code' ? <code key={i} className="font-mono text-[0.92em]">{m.text}</code> : m.text))}</>;
}

// Utilities that set the same property must not meet in one class list (the stylesheet's order decides, not the list's),
// so each variant carries its own colours and the height is chosen by the caller.
const btnBase = 'inline-flex items-center justify-center gap-2 whitespace-nowrap border px-3 font-medium transition-[background-color,transform] duration-150 ease-out active:translate-y-px disabled:pointer-events-none disabled:opacity-50';
export const btn = cx(btnBase, 'h-8 border-line bg-transparent text-fg hover:bg-ink-3');
export const btnPrimary = cx(btnBase, 'chamfer-sm h-8 border-transparent bg-red-fill text-white hover:bg-red');
export const btnDanger = cx(btnBase, 'h-8 border-red-line bg-transparent text-red-text hover:bg-red-soft');
export const field = 'rounded-md border border-line bg-ink-2 px-3 text-fg placeholder:text-faint';
export const input = cx(field, 'h-[38px]');
export const select = 'h-8 rounded-md border border-line bg-ink-2 pr-7 pl-2.5 text-fg';
export const srOnly = 'sr-only';

export function LangSwitch({ className, button }: { className?: string; button?: string }) {
  const lang = useLang();
  return (
    <div className={className} role="group" aria-label="语言 Language">
      {(['zh', 'en'] as const).map((l) => (
        <button key={l} type="button" aria-pressed={lang === l} onClick={() => setLang(l)} className={button}>{l === 'zh' ? '中文' : 'EN'}</button>
      ))}
    </div>
  );
}

export const errText = (err: unknown) =>
  err instanceof ApiError && err.code === 'network' ? t('err.network', { base: settings().base || '' }) : err instanceof Error ? err.message : String(err);

type RiseTag = 'div' | 'h2' | 'p';
// Rises into place once it scrolls into view; reduced motion shows it at once (index.css).
export function Rise({ as: Tag = 'div', delay = 0, className, style, ...rest }: { as?: RiseTag; delay?: number; className?: string; style?: CSSProperties; id?: string; children?: ReactNode }) {
  const ref = useRef<HTMLElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const io = new IntersectionObserver(([e]) => { if (e.isIntersecting) { el.classList.add('in'); io.disconnect(); } }, { rootMargin: '0px 0px -6% 0px' });
    io.observe(el);
    return () => io.disconnect();
  }, []);
  return <Tag ref={ref as never} className={cx('rise', className)} style={{ ...style, '--d': `${delay}ms` } as CSSProperties} {...rest} />;
}
