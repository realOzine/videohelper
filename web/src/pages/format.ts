import type { Video } from '../api.ts';
import { addStrings, t } from '../i18n.ts';
import { CHANNELS, channelState, stageKind, type Stage } from '../lib.ts';

addStrings({
  'v.ready.channels': '处理完成，可按{channels}搜索。',
  'v.ready.unknown': '视频处理完成，通道状态暂不可用。',
  'v.progress.queued': '任务已排队，等待本地模型和处理器就绪。',
  'v.progress.running': '正在处理，全部处理完成后可搜索。',
  'v.progress.copying': '正在复制到资料库，复制完成后开始处理。',
  'v.progress.cancelled': '处理已取消。原视频和已完成的步骤保留，点击重试可继续。',
  'v.progress.failed': '处理未完成。查看下方原因后重试；已完成的步骤会保留。',
  'v.progress.deleted': '视频已删除。恢复后可继续查看或重试。',
  'v.progress.steps': '已完成 {n} 个阶段',
}, {
  'v.ready.channels': 'Processing complete. Search by {channels}.',
  'v.ready.unknown': 'Processing complete. Channel status is currently unavailable.',
  'v.progress.queued': 'Queued, waiting for local models and the processor to be ready.',
  'v.progress.running': 'Processing. Search opens when all processing finishes.',
  'v.progress.copying': 'Copying into the library. Processing starts when the copy finishes.',
  'v.progress.cancelled': 'Processing canceled. The source and finished steps are kept; retry to continue.',
  'v.progress.failed': 'Processing did not finish. Check the reason below and retry; finished steps are kept.',
  'v.progress.deleted': 'This video is deleted. Restore it to view or retry.',
  'v.progress.steps': '{n} steps completed',
});

export const busy = (status: string) => ['uploading', 'importing', 'indexing'].includes(status);
export const tracks = (v: Video) => (v.has_video == null ? t('tr.pending') : v.has_audio ? t('tr.both') : t('tr.visualonly'));
export const stageLabel = (s: Stage) => { const k = `stage.${stageKind(s.stage)}`, label = t(k); return label === k ? s.stage : label; };

export function videoProgress(v: Pick<Video, 'status' | 'has_audio'>, stages: Stage[]): string {
  if (v.status === 'ready') {
    const can = channelState(stages);
    const channels = CHANNELS.filter((c) => can[c] === 'done' && (c === 'visual' || v.has_audio !== false)).map((c) => t(`ch.${c}`));
    return channels.length ? t('v.ready.channels', { channels: channels.join(' / ') }) : t('v.ready.unknown');
  }
  if (['cancelled', 'failed', 'deleted'].includes(v.status)) return t(`v.progress.${v.status}`);
  if (v.status === 'uploading') return t('v.progress.copying');
  return t(stages.some((s) => s.status === 'running') ? 'v.progress.running' : 'v.progress.queued');
}

// today 14:02, yesterday 21:36, or the date
export function when(iso: string) {
  const d = new Date(iso), now = new Date(), hm = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
  if (d.toDateString() === now.toDateString()) return `${t('today')} ${hm}`;
  if (d.toDateString() === new Date(now.getTime() - 864e5).toDateString()) return `${t('yesterday')} ${hm}`;
  return d.toLocaleDateString('en-CA');
}
