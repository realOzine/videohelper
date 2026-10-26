// VideoHelper landing: the dark screening room (v6 structure), Cindy palette and type, screen.studio's staged product.
// Sections: hero with the live mini workbench, a ticker of queries, then a stack that pins: the viewer, the annotated
// paper panel and the pipeline drag-through each slide over the last; questions and the closing choice scroll on.
import { useEffect, useLayoutEffect, useRef, useState, type HTMLAttributes, type ReactNode } from 'react';
import { t, useLang } from '../i18n.ts';
import { cx, Icon, LangSwitch, Mark, Rise } from '../ui.tsx';
import { CLIP, FAQ, NOTES, SETS, STAGES } from './data.ts';
import HeroStage from './HeroDemo.tsx';
import { media, rk, sec, span, title } from './util.ts';

const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
const btn = 'chamfer inline-flex h-[52px] items-center justify-center gap-2.5 whitespace-nowrap px-[26px] font-semibold transition-[transform,background-color] duration-150 ease-out active:scale-[.97] [&>svg]:size-[18px] [&>svg]:transition-transform [&>svg]:duration-200 [&>svg]:ease-out hover:[&>svg]:translate-x-0.5 hover:[&>svg]:-translate-y-0.5';
const film = 'overflow-hidden bg-black [&>img]:size-full [&>img]:object-cover [&>video]:size-full [&>video]:object-cover';

export default function Landing() {
  const l = useLang() === 'en' ? 1 : 0;
  const stack = useStack(3);
  return (
    <div className="landing text-(length:--step-body) leading-[1.55]">
      <Hero l={l} />
      <div className="overflow-hidden py-3" aria-hidden>
        <div className="-mx-5 -rotate-[1.2deg] overflow-hidden bg-red-fill text-white">
          <ul className={cx('flex w-max animate-run gap-12 py-3.5', l ? 'font-disp text-[22px] tracking-[0.02em] uppercase' : 'font-cjk text-xl font-black')}>
            {[...SETS, ...SETS].map((s, i) => <li key={i} className="whitespace-nowrap after:ml-12 after:align-middle after:text-[0.6em] after:content-['◆']">{s.q[l]}</li>)}
          </ul>
        </div>
      </div>
      <main>
        <Layer s={stack} i={0} card={false} aria-labelledby="viewh" className="flex min-h-svh items-center bg-ink px-(--m) py-24 max-sm:min-h-0 max-sm:py-20">
          <div className="grid w-full grid-cols-12 items-center gap-x-6 gap-y-10">
            <div className="col-span-5 max-xl:col-span-12">
              <Rise as="h2" id="viewh" className={cx('disp text-[clamp(44px,4.8vw,80px)]', l ? 'max-sm:text-[12vw]' : 'max-sm:text-[9vw]')}>{t('view.h')}</Rise>
              <Rise as="p" delay={80} className="mt-6 max-w-[40ch]">{t('view.p')}</Rise>
            </div>
            <Rise className="col-span-7 grid min-h-[620px] place-items-center rounded-[28px] bg-ink-2 p-10 max-xl:col-span-12 max-xl:min-h-0 max-sm:rounded-[18px] max-sm:p-3">
              <Viewer l={l} />
            </Rise>
          </div>
        </Layer>
        <Layer s={stack} i={1} aria-labelledby="ph" className="relative bg-paper px-(--m) py-40 text-on-paper max-sm:py-24"><Plate l={l} /></Layer>
        <Layer s={stack} i={2} id="how" aria-labelledby="howh" className="relative grid min-h-svh grid-cols-2 bg-ink max-sm:min-h-0 max-sm:grid-cols-1"><How l={l} /></Layer>
        <Layer s={stack} i={3} pin={false} id="faq" aria-labelledby="faqh" className="grid gap-[72px] bg-paper px-(--m) py-40 text-on-paper max-sm:gap-14 max-sm:py-24">
          <Rise className="grid grid-cols-12 items-baseline gap-x-6 max-lg:block"><h2 id="faqh" className={cx('disp col-span-6 text-(length:--step-head)', l ? 'max-sm:text-[12vw]' : 'max-sm:text-[10vw]')}>{l ? 'Questions' : '常见问题'}</h2></Rise>
          {FAQ.map((f, i) => (
            <Rise key={i} delay={(i % 3) * 60} className="grid grid-cols-12 items-baseline gap-x-6 max-lg:block">
              <h3 className="col-span-6 text-(length:--step-lead) leading-[1.15] font-bold max-sm:text-2xl">{f[l * 2]}</h3>
              <p className="col-start-8 col-end-[-1] max-w-[40ch] max-lg:mt-3 max-lg:max-w-none">{f[l * 2 + 1]}</p>
            </Rise>
          ))}
        </Layer>
        <section aria-labelledby="ch1" style={{ zIndex: 4 }} className="relative grid min-h-[86svh] grid-rows-[auto_1fr_auto] bg-red-fill pt-8 pr-(--mr) pb-6 pl-(--m) text-center text-white">
          <h2 id="ch1" className={cx('disp', l ? 'text-(length:--step-display) max-sm:text-[16vw]' : 'text-[clamp(56px,7vw,120px)] max-sm:text-[12vw]')}>{t('cta.q1')}</h2>
          <div className="grid justify-items-center gap-4 self-center">
            <div className="flex items-center gap-10 max-sm:w-[72%] max-sm:flex-col max-sm:gap-3">
              <a className={cx(btn, 'min-w-[200px] bg-ink text-fg hover:bg-[#26252f] max-sm:w-full')} href="library.html">{t('cta.up')}</a>
              <span className="font-semibold">{t('cta.or')}</span>
              <a className={cx(btn, 'min-w-[200px] bg-fg text-ink hover:bg-white max-sm:w-full')} href="search.html">{t('cta.demo')}</a>
            </div>
            <p className="text-(length:--step-small)">{t('cta.after')}</p>
          </div>
          <p className={cx('disp', l ? 'text-(length:--step-display) max-sm:text-[16vw]' : 'text-[clamp(56px,7vw,120px)] max-sm:text-[12vw]')}>{t('cta.q2')}</p>
        </section>
      </main>
      <footer className="pt-[26svh] pr-(--mr) pb-[30px] pl-(--m) text-(length:--step-small) max-sm:pt-[20svh]">
        <div className="grid grid-cols-[1fr_auto_1fr] items-baseline gap-6 max-sm:grid-cols-1 max-sm:gap-2">
          <span className="text-fg-soft">{t('ft.what')}</span>
          <nav aria-label={t('ft.nav')} className="flex gap-6 [&>a]:underline-offset-4 [&>a:hover]:underline">
            <a href="search.html">{t('ft.search')}</a><a href="library.html">{t('ft.library')}</a><a href="#faq">{t('ft.faq')}</a>
          </nav>
          <span className="justify-self-end text-fg-soft max-sm:justify-self-start">© 2026</span>
        </div>
        <p className="mt-[18px] max-w-[64ch] text-fg-soft [&_a]:text-fg [&_a]:underline [&_a]:underline-offset-2"><Credits l={l} /></p>
      </footer>
    </div>
  );
}

function Hero({ l }: { l: 0 | 1 }) {
  const [shown, setShown] = useState(false);
  useEffect(() => { requestAnimationFrame(() => requestAnimationFrame(() => setShown(true))); }, []);
  const line = 'block transition-transform duration-[900ms] ease-out';
  return (
    <header id="top" className="relative grid min-h-[calc(100svh-96px)] grid-cols-[minmax(0,5fr)_minmax(0,7fr)] max-[900px]:min-h-0 max-[900px]:grid-cols-1">
      <a className="absolute top-[30px] left-(--m) z-[2] inline-flex items-center gap-2.5 font-semibold" href="#top"><Mark />VideoHelper</a>
      <div className="absolute top-[26px] right-(--mr) z-[2] flex items-center gap-5 text-(length:--step-small)">
        <a href="search.html" className="underline decoration-fg-soft underline-offset-[5px] max-sm:hidden">{t('nav.demo')}</a>
        <LangSwitch className="flex gap-0.5" button="h-11 min-w-11 whitespace-nowrap rounded-md px-2.5 text-fg-soft transition-colors duration-150 aria-pressed:bg-ink-3 aria-pressed:text-fg" />
      </div>
      <div className="flex flex-col justify-center pt-[88px] pr-8 pb-8 pl-(--m) max-[900px]:px-(--m) max-[900px]:pt-9 max-[900px]:pb-12 max-sm:pt-7">
        <div className="w-fit max-w-full self-center max-[900px]:self-start">
        <h1 className={cx('disp', l ? 'text-[clamp(48px,5vw,88px)] max-[900px]:text-[11vw] max-sm:text-[12.5vw]' : 'text-[clamp(44px,4.86vw,84px)] max-[900px]:text-[8vw] max-sm:text-[11vw]')}>
          <span className="block overflow-hidden pb-[0.04em]"><span className={cx(line, !shown && 'translate-y-[105%]')}>{t('hero.h1a')}</span></span>
          <span className="block overflow-hidden pb-[0.04em] text-red"><span className={cx(line, 'delay-[110ms]', !shown && 'translate-y-[105%]')}>{t('hero.h1b')}</span></span>
        </h1>
        <div className={cx('transition-opacity delay-500 duration-700', shown ? 'opacity-100' : 'opacity-0')}>
          <p className="mt-8 max-w-[44ch]">{t('hero.lede')}</p>
          <div className="mt-11">
            <a className={cx(btn, 'bg-red-fill text-white hover:bg-red')} href="#demo-q" onClick={(e) => { e.preventDefault(); document.getElementById('demo-q')?.focus(); }}>{t('hero.btn')}<Icon.arrow /></a>
            <span className="mt-4 block text-(length:--step-small) whitespace-nowrap text-fg-soft max-sm:whitespace-normal">{t('hero.note')}</span>
          </div>
        </div>
        <ul aria-label={t('wall.label')} className="mt-16 grid max-w-[540px] grid-cols-3 gap-2 max-sm:hidden">
          {[['sintel', 'sintel_trailer.mp4'], ['surf', 'surf_tel_aviv.mp4'], ['rocket', 'rocket_launch.mp4']].map(([id, name]) => (
            <li key={id}><figure>
              <div className={cx(film, 'aspect-[16/10] rounded-[4px]')}><video src={media(`${id}.mp4`)} poster={media(`${id}.jpg`)} muted loop playsInline autoPlay={!reduce} preload="metadata" aria-hidden /></div>
              <figcaption className="mt-1.5 text-(length:--step-small) text-fg-soft">{name}</figcaption>
            </figure></li>
          ))}
        </ul>
        </div>
      </div>
      <div className="pt-[88px] pr-8 pb-8 pl-2 max-[900px]:order-first max-[900px]:px-4 max-[900px]:pt-[84px] max-[900px]:pb-4 max-sm:pt-[72px]">
        <HeroStage l={l} />
      </div>
    </header>
  );
}

// the hero's working viewer: the strip reads the pointer and scrubs the real clip
function Viewer({ l }: { l: 0 | 1 }) {
  const hv = useRef<HTMLVideoElement>(null), scrubbing = useRef(false);
  const [at, setAt] = useState(0);
  const LEN = CLIP.reef[3], HIT = [2.5, 5];
  const seekX = (x: number, el: HTMLElement) => {
    const r = el.getBoundingClientRect(), s = Math.max(0, Math.min(LEN - 0.05, (x - r.left) / r.width * LEN));
    try { hv.current!.currentTime = s; } catch { /* not seekable yet */ }
    setAt(s);
  };
  return (
    <div className="w-[min(560px,100%)] rounded-[14px] bg-ink-3 p-3.5 text-[15px] leading-[22px]">
      <div className={cx(film, 'aspect-[16/10] rounded-lg')}>
        <video ref={hv} src={media('reef.mp4')} poster={media('reef.jpg')} muted loop playsInline autoPlay={!reduce} preload="auto" aria-label={t('hero.vid')}
          onTimeUpdate={(e) => { if (!scrubbing.current) setAt(e.currentTarget.currentTime); }} />
      </div>
      <div className="mx-0.5 mt-3 mb-2 flex items-baseline justify-between gap-3"><b className="font-semibold">{t('hero.q')}</b><span className="font-semibold text-red-text">{sec(at)}</span></div>
      <div role="slider" tabIndex={0} aria-label={t('hero.strip')} aria-valuemin={0} aria-valuemax={LEN} aria-valuenow={Number(at.toFixed(1))} aria-valuetext={sec(at)}
        className="relative grid h-[46px] cursor-crosshair touch-none grid-cols-8 gap-[3px]"
        onPointerMove={(e) => { scrubbing.current = true; hv.current?.pause(); seekX(e.clientX, e.currentTarget); }}
        onPointerDown={(e) => { e.currentTarget.setPointerCapture(e.pointerId); seekX(e.clientX, e.currentTarget); }}
        onPointerLeave={() => { scrubbing.current = false; if (!reduce) hv.current?.play().catch(() => {}); }}
        onKeyDown={(e) => {
          const d = ({ ArrowLeft: -0.5, ArrowRight: 0.5 } as Record<string, number>)[e.key], v = hv.current;
          if (!d || !v) return;
          e.preventDefault(); v.pause();
          const s = Math.max(0, Math.min(LEN - 0.05, v.currentTime + d)); v.currentTime = s; setAt(s);
        }}>
        {Array.from({ length: 8 }, (_, i) => <span key={i} className="rounded-[3px] bg-black bg-no-repeat" style={{ backgroundImage: `url(${media('reef-strip.jpg')})`, backgroundSize: '800% 100%', backgroundPosition: `${(i * 100 / 7).toFixed(3)}% 0` }} />)}
        <span className="absolute -top-[7px] h-1 rounded-sm bg-red" style={{ left: `${HIT[0] / LEN * 100}%`, width: `${(HIT[1] - HIT[0]) / LEN * 100}%` }} />
        <span className="pointer-events-none absolute -top-2 -bottom-2 -ml-px w-0.5 bg-fg" style={{ left: `${(at / LEN * 100).toFixed(2)}%` }} />
      </div>
      <div className="mt-3 grid grid-cols-3 gap-2 max-sm:hidden">
        {([['#1', 'visual', true], ['—', 'speech', false], ['#5', 'audio', false]] as const).map(([n, c, on]) => (
          <div key={c} className="rounded-md bg-ink-4 px-2.5 py-2"><b className={cx('block text-lg font-semibold', on && 'text-red-text')}>{n}</b><span className="text-[15px] text-fg-soft">{t(`ch.${c}`)}</span></div>
        ))}
      </div>
      <p className="mt-2.5 text-fg-soft max-sm:hidden">{title('reef', l)} · {span('reef')}</p>
    </div>
  );
}

// one product panel on paper, a note in the margin for each of its parts; pointing at a note lights its part
function Plate({ l }: { l: 0 | 1 }) {
  const [lit, setLit] = useState<string | null>(null);
  const [tops, setTops] = useState<Record<string, number>>({});
  const panel = useRef<HTMLDivElement>(null), parts = useRef<Record<string, HTMLElement | null>>({});
  useLayoutEffect(() => {
    const el = panel.current;
    if (!el) return;
    const wide = matchMedia('(width >= 60rem)');
    const align = () => setTops(wide.matches ? Object.fromEntries(Object.entries(parts.current).map(([k, p]) => [k, (p?.offsetTop ?? 0) + 16])) : {});
    const ro = new ResizeObserver(align);
    ro.observe(el); wide.addEventListener('change', align);
    return () => { ro.disconnect(); wide.removeEventListener('change', align); };
  }, [l]);
  const part = (k: string, className: string, children: ReactNode) => (
    <div ref={(e) => { parts.current[k] = e; }} className={cx('transition-opacity duration-[240ms]', lit && lit !== k && 'opacity-[.28]', className)}>{children}</div>
  );
  const note = (n: string[], side: 'left' | 'right') => (
    <div key={n[0]} tabIndex={0} onPointerEnter={() => setLit(n[0])} onPointerLeave={() => setLit(null)} onFocus={() => setLit(n[0])} onBlur={() => setLit(null)}
      style={tops[n[0]] != null ? { top: tops[n[0]] } : undefined}
      className={cx('absolute w-[24ch] max-w-full py-1.5 max-lg:static max-lg:w-auto max-lg:text-left', side === 'left' && 'right-0 text-right')}>
      <h3 className={cx('text-lg font-semibold transition-colors', lit === n[0] && 'text-red-deep')}>{n[1 + l * 2]}</h3>
      <p className="mt-1 text-(length:--step-small) leading-normal text-on-paper-soft">{n[2 + l * 2]}</p>
    </div>
  );
  const lane = (c: string, hits: [number, boolean][]) => (
    <><span>{t(`ch.${c}`)}</span><span className="relative h-3.5 rounded-[3px] bg-ink-3">{hits.map(([x, on]) => <i key={x} className={cx('absolute w-1.5 rounded-sm', on ? 'inset-y-0 bg-red' : 'inset-y-0.5 bg-hit')} style={{ left: `${x}%` }} />)}</span></>
  );
  const blk = 'mt-3.5 border-t border-ink-3 pt-3 [&>h4]:mb-2 [&>h4]:text-[15px] [&>h4]:font-medium [&>h4]:text-fg-soft';
  const notes = 'relative col-span-4 min-h-[720px] max-lg:col-span-1 max-lg:grid max-lg:min-h-0 max-lg:grid-cols-2 max-lg:gap-x-5 max-lg:gap-y-4';
  return (
    <>
      <div className="grid grid-cols-12 items-start gap-x-6 max-lg:grid-cols-1">
        <Rise as="h2" id="ph" className="disp col-span-4 text-right text-[clamp(36px,3.4vw,56px)] max-lg:col-span-1 max-lg:mb-6 max-lg:text-left max-lg:text-[34px]">{t('pl.h')}</Rise>
      </div>
      <div className="mt-6 grid grid-cols-12 items-start gap-x-6 max-lg:grid-cols-1">
        <div className={cx(notes, 'max-lg:order-2')}>{NOTES.filter((_, i) => i % 2 === 0).map((n) => note(n, 'left'))}</div>
        <Rise className="col-span-4 col-start-5 w-full max-w-[400px] justify-self-center max-lg:order-1 max-lg:col-span-1 max-lg:col-start-1">
          <div ref={panel} className="relative rounded-[14px] bg-ink p-4 text-[15px] leading-[22px] text-fg">
            {part('frame', cx(film, 'aspect-[16/10] rounded-lg'), <img src={media('rocket.jpg')} width={720} height={405} alt={l ? 'A rocket lighting the night sky at launch' : '火箭夜间发射时照亮天空'} />)}
            {part('range', '', <>
              <div className="mt-3 mb-0.5 flex justify-between gap-2"><b className="font-semibold">{title('rocket', l)}</b><span className="text-fg-soft">{span('rocket', 0, 2.5)}</span></div>
              <div className="text-fg-soft">{rk('visual', 1)} · {rk('audio', 2)}</div>
            </>)}
            {part('lanes', blk, <><h4>{l ? 'Where it matched' : '命中位置'}</h4>
              <div className="grid grid-cols-[44px_1fr] items-center gap-x-2.5 gap-y-1.5 text-fg-soft">{lane('visual', [[6, true], [62, false]])}{lane('speech', [])}{lane('audio', [[8, true], [40, false]])}</div></>)}
            {part('stages', blk, <><h4>{l ? 'Processing' : '处理进度'}</h4>
              {['stage.visual', 'stage.asr', 'stage.audio'].map((k) => <div key={k} className="flex justify-between py-1"><span>{t(k)}</span><span className="text-pos-ink">{t('done')}</span></div>)}</>)}
            {part('find', blk, <><h4>{l ? 'Search this video' : '在这个视频里搜索'}</h4>
              <div className="grid grid-cols-[50px_1fr] gap-2 rounded-md bg-ink-3 px-2 py-1.5"><b className="font-medium text-fg-soft">{sec(1.5)}</b><span>{l ? 'the moment it leaves the pad' : '火箭离开发射台的那一刻'}</span></div></>)}
            {part('tools', 'mt-3.5 flex gap-2', <>
              <button type="button" className="h-[38px] flex-1 rounded-md bg-red-fill font-semibold text-white">{l ? 'Play from 00:01.5' : '从 00:01.5 播放'}</button>
              <button type="button" className="h-[38px] flex-1 rounded-md bg-ink-3">{l ? 'Add to project' : '加入项目'}</button>
            </>)}
          </div>
        </Rise>
        <div className={cx(notes, 'max-lg:order-3')}>{NOTES.filter((_, i) => i % 2 === 1).map((n) => note(n, 'right'))}</div>
      </div>
    </>
  );
}

// drag through the stages: the left half says the step, the right half shows it
function How({ l }: { l: 0 | 1 }) {
  const [i, setI] = useState(0), [shown, setShown] = useState(0), [fading, setFading] = useState(false);
  useEffect(() => {
    if (i === shown) return;
    if (reduce) { setShown(i); return; }
    setFading(true);
    const timer = setTimeout(() => { setShown(i); setFading(false); }, 200);
    return () => clearTimeout(timer);
  }, [i, shown]);
  const s = STAGES[shown], o = l * 3;
  const pic = <div className={cx(film, 'mb-3 aspect-[16/10] rounded-lg')}><img src={media('surf.jpg')} width={720} height={405} alt={l ? 'Surfers waiting for a wave' : '海上等浪的冲浪者'} /></div>;
  const bar = (p: number) => <div className="mt-1.5 mb-2.5 h-1.5 overflow-hidden rounded-[3px] bg-ink-4"><i className="block h-full origin-left bg-red" style={{ transform: `scaleX(${p})` }} /></div>;
  const frag = [
    <>{pic}<div className="text-fg-soft">{l ? 'Uploading' : '正在上传'}</div><b>surf_tel_aviv.mp4</b>{bar(0.41)}<div className="text-fg-soft">{l ? '3 / 7 parts · 104 MB · 41%' : '3 / 7 分片 · 104 MB · 41%'}</div></>,
    <><div className="text-fg-soft">{l ? 'Thumbnail frames · 720p' : '缩略帧 · 720p'}</div>
      <div className="mt-2 grid grid-cols-4 gap-1">{Array.from({ length: 12 }, (_, k) => <span key={k} className="aspect-[4/3] rounded-[3px] bg-black" style={{ backgroundImage: `url(${media(`${k < 8 ? 'surf' : 'jelly'}-strip.jpg`)})`, backgroundSize: '800% 100%', backgroundPosition: `${((k % 8) * 100 / 7).toFixed(2)}% 0` }} />)}</div></>,
    <>{pic}{([['visual', 0.64], ['speech', 0.3], ['audio', 1]] as const).map(([c, p]) => <div key={c}><div className="text-fg-soft">{t(`ch.${c}`)} · {Math.round(p * 100)}%</div>{bar(p)}</div>)}</>,
    <>{pic}<b>{title('surf', l)}</b><div className="text-fg-soft">{span('surf', 0, 5)} · {rk('visual', 1)}</div></>,
  ][shown];
  const fade = cx('transition-opacity duration-200', fading && 'opacity-0');
  return (
    <>
      <div className="bg-ink px-(--m) pt-[72px] pb-[150px] max-sm:pt-7 max-sm:pb-6">
        <h2 id="howh" className={cx('disp max-w-[9em] text-[clamp(44px,4.8vw,80px)]', l ? 'max-sm:text-[12vw]' : 'max-sm:text-[9vw]', fade)}>{s[1 + o]}</h2>
        <p className={cx('mt-6 max-w-[40ch]', fade)}>{s[2 + o]}</p>
      </div>
      <div className="grid place-items-center bg-ink-2 px-7 pt-[72px] pb-[150px] max-sm:order-first max-sm:min-h-[46svh] max-sm:px-4 max-sm:py-7">
        <div className={cx('w-[min(600px,100%)] rounded-[14px] bg-ink-3 p-4 text-[15px] leading-[22px]', fade)}>{frag}</div>
      </div>
      <div className="absolute right-(--mr) bottom-9 left-(--m) max-sm:static max-sm:pr-(--mr) max-sm:pb-6 max-sm:pl-(--m)">
        <label htmlFor="stage" className="sr-only">{t('how.label')}</label>
        <input id="stage" type="range" min={0} max={3} step={1} value={i} onChange={(e) => setI(Number(e.target.value))} aria-valuetext={`${STAGES[i][o]}: ${STAGES[i][1 + o]}`}
          className="stage-range" style={{ '--pass': `${i / 3 * 100}%` } as React.CSSProperties} />
        <ol className="mt-2.5 grid grid-cols-4 text-(length:--step-small) text-fg-soft max-sm:grid-cols-2 max-sm:gap-x-3 max-sm:gap-y-1">
          {STAGES.map((st, k) => (
            <li key={k} aria-current={k === i ? 'step' : undefined} className={cx(k === i && 'text-fg', k === 1 && 'text-center -translate-x-[17%]', k === 2 && 'text-center translate-x-[17%]', k === 3 && 'text-right', 'max-sm:translate-x-0 max-sm:text-left')}>
              <button type="button" className="py-1 transition-colors hover:text-fg" onClick={() => setI(k)}>{st[o]}</button>
            </li>
          ))}
        </ol>
      </div>
    </>
  );
}

const cardEdge = 'sm:rounded-t-[44px] sm:shadow-[0_-20px_120px_-10px_rgba(0,0,0,0.32),0_-1px_0_rgba(255,255,255,0.04)]';
type Stack = ReturnType<typeof useStack>;

// One layer of the stack: it pins at the top (or at its own bottom when taller than the window) and the next layer
// slides over it. Phones scroll normally.
function Layer({ s, i, pin = true, card = true, className, children, ...rest }: { s: Stack; i: number; pin?: boolean; card?: boolean; className?: string; children: ReactNode } & HTMLAttributes<HTMLElement>) {
  return (
    <section ref={(e) => { s.secs.current[i] = e; }} {...rest} style={{ zIndex: i + 1 }} className={cx('relative', pin && 'sm:sticky')}>
      <div ref={(e) => { s.inner.current[i] = e; }} className={cx('origin-top overflow-hidden', card && cardEdge, className)}>{children}</div>
      {pin && <div ref={(e) => { s.dims.current[i] = e; }} aria-hidden className={cx('pointer-events-none absolute inset-0 bg-black opacity-0', card && 'sm:rounded-t-[44px]')} />}
    </section>
  );
}

// Pins the first `n` layers and, as each next layer covers one, shrinks it, dims it and rounds its corners.
// Styles are written straight to the elements on scroll, so scrolling never re-renders the page.
function useStack(n: number) {
  const secs = useRef<(HTMLElement | null)[]>([]), inner = useRef<(HTMLElement | null)[]>([]), dims = useRef<(HTMLElement | null)[]>([]);
  useEffect(() => {
    const wide = matchMedia('(width >= 47.5rem)');
    let raf = 0;
    const layout = () => secs.current.slice(0, n).forEach((el) => { if (el) el.style.top = wide.matches ? `${Math.min(0, innerHeight - el.offsetHeight)}px` : ''; });
    const paint = () => {
      raf = 0;
      for (let i = 0; i < n; i++) {
        const next = secs.current[i + 1], box = inner.current[i], dim = dims.current[i];
        if (!next || !box || !dim) continue;
        const p = wide.matches && !reduce ? Math.max(0, Math.min(1, 1 - next.getBoundingClientRect().top / innerHeight)) : 0;
        box.style.transform = p ? `scale(${1 - 0.06 * p})` : '';
        box.style.borderRadius = p ? `${44 * p}px` : '';
        dim.style.opacity = String(0.4 * p);
      }
    };
    const onScroll = () => { if (!raf) raf = requestAnimationFrame(paint); };
    const ro = new ResizeObserver(() => { layout(); onScroll(); });
    secs.current.forEach((el) => el && ro.observe(el));
    addEventListener('scroll', onScroll, { passive: true });
    addEventListener('resize', onScroll);
    wide.addEventListener('change', layout);
    layout(); paint();
    return () => { ro.disconnect(); removeEventListener('scroll', onScroll); removeEventListener('resize', onScroll); wide.removeEventListener('change', layout); cancelAnimationFrame(raf); };
  }, [n]);
  return { secs, inner, dims };
}

const CREDITS: [string, string, string, string][] = [
  ['Sintel', 'https://commons.wikimedia.org/wiki/File:Sintel_trailer-1080p.webm', 'Durian Open Movie Project，CC BY 3.0', 'Durian Open Movie Project, CC BY 3.0'],
  ['Big Buck Bunny', 'https://commons.wikimedia.org/wiki/File:Big_buck_bunny_montage.webm', 'Blender Foundation，CC BY 3.0', 'Blender Foundation, CC BY 3.0'],
  ['Phantom Jellyfish', 'https://commons.wikimedia.org/wiki/File:Phantom_Jellyfish_Off_of_the_Melchior_Islands.webm', 'MasterfulNerd，CC BY 4.0', 'MasterfulNerd, CC BY 4.0'],
  ['Coral reef fishes', 'https://commons.wikimedia.org/wiki/File:Coordinated-vigilance-provides-evidence-for-direct-reciprocity-in-coral-reef-fishes-srep14556-s2.webm', 'Brandl S、Bellwood D，CC BY 4.0', 'Brandl S, Bellwood D, CC BY 4.0'],
  ['Surfing in Tel Aviv', 'https://commons.wikimedia.org/wiki/File:Surfing_in_Tel_Aviv_MVI_3675.webm', 'Deror Avi，CC BY-SA 4.0', 'Deror Avi, CC BY-SA 4.0'],
];
function Credits({ l }: { l: 0 | 1 }) {
  const nasa = <a href="https://images.nasa.gov/">NASA</a>;
  return l ? (
    <>Sample videos (trimmed, silent): {CREDITS.map(([n, u, , e]) => <span key={n}><a href={u}>{n}</a> ({e}); </span>)}rocket launch and ISS footage from {nasa} (public domain).</>
  ) : (
    <>示例视频（均为裁剪后的无声片段）：{CREDITS.map(([n, u, z]) => <span key={n}><a href={u}>{n}</a>（{z}）；</span>)}{nasa} 的火箭发射与国际空间站画面（公有领域）。</>
  );
}
