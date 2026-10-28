import { ApiError, type Video } from '../api.ts';
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
  'v.error.network': '本地服务暂时没有响应。请稍后重试；若仍无响应，请重新打开应用。',
  'v.error.not_ready': '本地模型暂不可用。请查看运行环境状态，准备完成后重试。',
  'v.error.not_found': '找不到这个视频或项目。请返回资料库刷新列表。',
  'v.error.invalid': '无法执行这项操作。请检查输入后重试。',
  'v.error.conflict': '视频状态或所选模型已变化。请刷新页面，确认处理和模型状态后重试。',
  'v.error.cancelled': '操作已取消。需要时可重试。',
  'v.error.playback': '无法播放视频。请重新打开视频；若仍失败，可尝试重新索引。',
  'v.error.fetch': '视频下载未完成。请确认链接有效、网络可用后重试。',
  'v.error.probe': '无法读取视频。请确认源文件可正常播放后重试，或重新导入可用的视频。',
  'v.error.prepare_media': '视频转换未完成。请检查本地磁盘空间后重试。',
  'v.error.asr': '语音转写未完成。请确认本地模型已就绪后重试。',
  'v.error.embedding': '视频编码未完成。请确认本地模型已就绪后重试。',
  'v.error.unknown': '操作未完成。请重试；若再次失败，可展开技术详情以排查原因。',
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
  'v.error.network': 'The local service is not responding. Try again shortly; reopen the app if it remains unavailable.',
  'v.error.not_ready': 'Local models are currently unavailable. Check the runtime status and retry when they are ready.',
  'v.error.not_found': 'This video or project could not be found. Return to the library and refresh the list.',
  'v.error.invalid': 'This operation could not be completed. Check your input and try again.',
  'v.error.conflict': 'The video state or selected model has changed. Refresh the page, check processing and model status, then retry.',
  'v.error.cancelled': 'The operation was canceled. Retry when needed.',
  'v.error.playback': 'The video could not be played. Reopen it; if playback still fails, try rebuilding its index.',
  'v.error.fetch': 'The video download did not finish. Check that the link and network are available, then retry.',
  'v.error.probe': 'The video could not be read. Check that the source plays correctly, then retry or import a working video.',
  'v.error.prepare_media': 'Video conversion did not finish. Check free space on the local disk, then retry.',
  'v.error.asr': 'Transcription did not finish. Check that local models are ready, then retry.',
  'v.error.embedding': 'Video encoding did not finish. Check that local models are ready, then retry.',
  'v.error.unknown': 'The operation did not finish. Retry; if it fails again, expand the technical details to investigate.',
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

// Classify by stable API code or the failed pipeline stage, never by parsing
// tool output. Keep the original message intact for expandable diagnostics.
export function videoError(err: unknown, stage?: string): { summary: string; detail?: string } {
  const detail = err instanceof Error ? err.message : err == null ? undefined : String(err);
  const codes = ['network', 'not_ready', 'media_not_ready', 'not_found', 'invalid', 'conflict', 'cancelled', 'playback'];
  let kind = err instanceof ApiError && codes.includes(err.code) ? err.code : 'unknown';
  if (kind === 'media_not_ready') kind = 'not_ready';
  if (kind === 'unknown' && stage) {
    const k = stageKind(stage);
    if (['fetch', 'probe', 'prepare_media', 'asr'].includes(k)) kind = k;
    else if (CHANNELS.includes(k as typeof CHANNELS[number])) kind = 'embedding';
  }
  return { summary: t(`v.error.${kind}`), detail: detail || undefined };
}

// today 14:02, yesterday 21:36, or the date
export function when(iso: string) {
  const d = new Date(iso), now = new Date(), hm = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
  if (d.toDateString() === now.toDateString()) return `${t('today')} ${hm}`;
  if (d.toDateString() === new Date(now.getTime() - 864e5).toDateString()) return `${t('yesterday')} ${hm}`;
  return d.toLocaleDateString('en-CA');
}
