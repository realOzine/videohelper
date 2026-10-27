import { t } from '../i18n.ts';
import { CLIP } from './data.ts';

export const media = (file: string) => `media/${file}`;
// mm:ss.t from seconds, as the sample clips are measured
export const sec = (s: number) => { const m = Math.floor(s / 60), r = s - m * 60; return `${String(m).padStart(2, '0')}:${(r < 10 ? '0' : '') + r.toFixed(1)}`; };
export const title = (id: string, l: 0 | 1) => CLIP[id][l];
export const span = (id: string, a = 0, b = CLIP[id][3]) => `${sec(CLIP[id][2] + a)} – ${sec(CLIP[id][2] + b)}`;
export const rk = (c: string, n: number) => t('rank', { ch: t(`ch.${c}`), n });
