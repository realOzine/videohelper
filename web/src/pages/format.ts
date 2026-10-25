import type { Video } from '../api.ts';
import { t } from '../i18n.ts';
import { stageKind, type Stage } from '../lib.ts';

export const busy = (status: string) => ['uploading', 'importing', 'indexing'].includes(status);
export const tracks = (v: Video) => (v.has_video == null ? t('tr.pending') : v.has_audio ? t('tr.both') : t('tr.visualonly'));
export const stageLabel = (s: Stage) => { const k = `stage.${stageKind(s.stage)}`, label = t(k); return label === k ? s.stage : label; };

// today 14:02, yesterday 21:36, or the date
export function when(iso: string) {
  const d = new Date(iso), now = new Date(), hm = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
  if (d.toDateString() === now.toDateString()) return `${t('today')} ${hm}`;
  if (d.toDateString() === new Date(now.getTime() - 864e5).toDateString()) return `${t('yesterday')} ${hm}`;
  return d.toLocaleDateString('en-CA');
}
