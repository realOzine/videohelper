// Settings: the app's language, the local runtime and the models search uses, as rows of labelled cards.
import { useState, type ReactNode } from 'react';
import { addStrings, t, useLang } from '../i18n.ts';
import { newProject, retryRuntime, Shell, useLibrary, useNotice, useRuntime } from '../Shell.tsx';
import { btn, cx, LangSwitch } from '../ui.tsx';
import { Models, storedPrefs } from './Search.tsx';

addStrings({
  'set.general': '通用', 'set.lang': '语言', 'set.lang.sub': '界面文字的语言，切换后立即生效。',
  'set.runtime': '运行环境', 'set.model': '本地模型', 'set.model.sub': '模型在本机运行；首次准备时可能需要下载模型文件。',
  'set.device': '推理设备', 'set.device.sub': '启动时自动检测。', 'set.retry': '重新准备运行环境', 'set.retry.sub': '运行环境出错时可用；已导入的视频和索引会保留。',
  'set.search': '搜索', 'set.models': '搜索用的模型',
}, {
  'set.general': 'General', 'set.lang': 'Language', 'set.lang.sub': 'The language of the interface; switching applies at once.',
  'set.runtime': 'Runtime', 'set.model': 'Local models', 'set.model.sub': 'Models run on this computer; the first setup may download model files.',
  'set.device': 'Inference device', 'set.device.sub': 'Detected at startup.', 'set.retry': 'Prepare the runtime again', 'set.retry.sub': 'Available when the runtime fails; imported videos and indexes are kept.',
  'set.search': 'Search', 'set.models': 'Models for search',
});

const Section = ({ title, children }: { title: string; children: ReactNode }) => (
  <section className="mt-7 first:mt-0">
    <h2 className="mx-1 mb-2.5 text-xs font-semibold tracking-[0.08em] text-faint">{title}</h2>
    <div className="rounded-2xl border border-edge bg-ink-2 [&>*+*]:border-t [&>*+*]:border-ink-3">{children}</div>
  </section>
);
const Row = ({ title, sub, children }: { title: string; sub?: ReactNode; children?: ReactNode }) => (
  <div className="flex min-h-16 flex-wrap items-center gap-x-4 gap-y-3 px-[18px] py-3">
    <div className="grid min-w-0 flex-[1_1_260px] gap-0.5"><b className="text-[15px] font-medium">{title}</b>{sub && <span className="text-[13px] text-faint">{sub}</span>}</div>
    {children}
  </div>
);

export default function Settings() {
  useLang();
  const { notice, fail } = useNotice();
  const { ws, reload } = useLibrary(fail);
  const { state, failed, ready } = useRuntime();
  const [retrying, setRetrying] = useState(false);
  return (
    <Shell page="settings" ws={ws} notice={notice} onNewProject={() => newProject(reload, fail)}>
      <div className="mx-auto max-w-[760px]">
        <h1 className="disp mb-7 text-[26px]">{t('nav.settings')}</h1>
        <Section title={t('set.general')}>
          <Row title={t('set.lang')} sub={t('set.lang.sub')}>
            <LangSwitch className="flex gap-0.5 rounded-full border border-ink-3 p-[3px]" button="h-[28px] min-w-12 rounded-full px-3 text-[13px] font-medium text-fg-soft transition-colors aria-pressed:bg-ink-3 aria-pressed:text-fg" />
          </Row>
        </Section>
        <Section title={t('set.runtime')}>
          <Row title={t('set.model')} sub={t('set.model.sub')}>
            <span className={cx('inline-flex items-center gap-2 text-[13px]', failed ? 'text-neg-ink' : ready ? 'text-pos-ink' : 'text-fg-soft')}>
              <i className={cx('size-[7px] rounded-full', failed ? 'bg-red' : ready ? 'bg-pos' : 'animate-beat bg-fg-soft')} />
              {failed ? t('desktop.failed') : ready ? t('desktop.readyShort') : t('desktop.preparing')}
            </span>
          </Row>
          <Row title={t('set.device')} sub={state?.reason || t('set.device.sub')}>
            {state?.device && <code className="rounded bg-ink-3 px-2 py-0.5 font-mono text-xs text-fg-soft">{state.device.toUpperCase()}</code>}
          </Row>
          <Row title={t('set.retry')} sub={t('set.retry.sub')}>
            <button type="button" className={btn} disabled={!failed || retrying} onClick={async () => { setRetrying(true); await retryRuntime(); setRetrying(false); }}>{t('retry')}</button>
          </Row>
        </Section>
        <Section title={t('set.search')}>
          <Row title={t('set.models')} sub={t('mdl.sub')}><Models prefs={storedPrefs} /></Row>
        </Section>
      </div>
    </Shell>
  );
}
