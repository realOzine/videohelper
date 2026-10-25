// Sign-in: until a sign-in service is chosen, paste a token it issued and the API address; the pair stays in this browser.
import { useEffect, useState, type FormEvent } from 'react';
import { ApiError, call, saveSettings, settings, signOut } from '../api.ts';
import { t, useLang } from '../i18n.ts';
import { safeNext } from '../lib.ts';
import '../strings.ts';
import { btn, btnPrimary, cx, field, input, LangSwitch, Mark, Rich } from '../ui.tsx';

export default function SignIn() {
  const lang = useLang();
  const [notice, setNotice] = useState('');
  const [state, setState] = useState('');
  useEffect(() => { document.title = t('page.title.signin'); }, [lang]);

  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setNotice('');
    const f = new FormData(e.currentTarget);
    const next = { base: String(f.get('base')).trim(), token: String(f.get('token')).trim() };
    if (!/^https?:\/\/[^/?#]+/.test(next.base)) return setNotice(t('si.badbase'));
    try {
      await call('GET', '/v1/me', undefined, next);
      saveSettings(next);
      location.href = safeNext(new URLSearchParams(location.search).get('next'));
    } catch (err) {
      setNotice(err instanceof ApiError && err.status === 401 ? t('si.bad') : err instanceof ApiError && err.code === 'network' ? t('err.network', { base: next.base }) : String((err as Error).message));
    }
  };

  return (
    <main className="mx-auto max-w-[600px] px-4 py-12 text-sm leading-5">
      <a className="mb-10 inline-flex items-center gap-2.5 text-[17px] font-bold tracking-[-0.01em]" href="index.html"><Mark />VideoHelper</a>
      <h1 className="disp text-[32px] leading-[1.2]">{t('si.title')}</h1>
      <p className="mt-1 mb-5 text-[15px] text-fg-soft"><Rich k="si.sub" /></p>
      {notice && <p role="alert" className="mb-6 rounded-[10px] bg-red-soft px-4 py-3 text-neg-ink">{notice}</p>}
      <form className="grid gap-4 rounded-[10px] bg-ink-2 p-5" onSubmit={submit}>
        <label className="grid gap-2"><span className="font-medium">{t('si.base')}</span>
          <input className={input} name="base" type="url" required autoComplete="url" defaultValue={settings().base || 'http://127.0.0.1:18000'} />
        </label>
        <label className="grid gap-2"><span className="font-medium">{t('si.token')}</span>
          <textarea className={cx(field, 'resize-y py-2 font-mono text-[13px] leading-normal break-all')} name="token" rows={5} required spellCheck={false} autoComplete="off" />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <button className={btnPrimary} type="submit">{t('si.go')}</button>
          <button className={btn} type="button" onClick={(e) => { signOut(); (e.currentTarget.form!.elements.namedItem('token') as HTMLTextAreaElement).value = ''; setState(t('si.out')); }}>{t('si.signout')}</button>
          <LangSwitch className="ml-auto flex gap-0.5 rounded-lg bg-ink p-[3px]" button="h-[30px] min-w-10 rounded-md px-2.5 font-medium text-fg-soft transition-colors aria-pressed:bg-ink-3 aria-pressed:text-fg" />
        </div>
        {state && <p className="text-xs text-faint" aria-live="polite">{state}</p>}
      </form>
    </main>
  );
}
