// Landing-page strings and the sample library the mini workbench runs on. No network calls.
import { addStrings } from '../i18n.ts';

addStrings({
    'page.title': 'VideoHelper', 'wall.label': '示例视频',
    'hero.h1a': '在你的视频里', 'hero.h1b': '找到那一刻',
    'hero.lede': 'VideoHelper 是视频内容搜索工具：描述一个画面、一句台词或一种声音，它在你的视频里找到对应的那几秒，点一下就从那里播放。',
    'hero.btn': '现在就试试', 'hero.note': '时间码精确到 0.1 秒 · 画面、台词、声音三路一起找', 'hero.q': '珊瑚礁边的鱼群', 'hero.vid': '珊瑚礁边游动的鱼群', 'hero.strip': '片段时间线，移动指针查看画面',
    'ch.visual': '画面', 'ch.speech': '台词', 'ch.audio': '声音', rank: '{ch} 第 {n} 名',
    'view.h': '拖一下就停在\n命中的那一秒', 'view.p': '每个结果都是一段时间。红线标出命中的几秒，在缩略帧上拖动就能逐帧预览；下面三格告诉你是画面、台词还是声音命中的。',
    'app.search': '搜索', 'app.lib': '资料库', 'app.proc': '处理中', 'app.qlabel': '描述画面、台词或声音', 'app.go': '搜索', 'app.count': '{n} 个片段 · 按相关度排序', 'app.nomatch': '示例库里没找到「{q}」，试试下面这几句',
    'demo.views': '结果视图', 'demo.dir': '目录', 'demo.det': '详情', 'demo.back': '返回目录', 'demo.hits': '命中 {n} 处', 'demo.hit': '命中 {n} 处', 'demo.more': '还有 {n} 个视频 · 切到「详情」看全部片段', 'demo.more1': '还有 {n} 个视频 · 切到「详情」看全部片段', 'demo.ruler': '演示时间尺',
    'done': '完成', 'stage.visual': '画面编码', 'stage.asr': '语音转录', 'stage.audio': '声音编码',
    'pl.h': '一个视频的\n全部线索', 'how.label': '处理步骤',
    'cta.q1': '在上面的迷你版里', 'cta.q2': '先试一句话', 'cta.demo': '回到迷你版试试', 'cta.after': '迷你版用的是开放许可的示例视频，出处见页脚。',
    'ft.what': 'VideoHelper · 在视频里找画面', 'ft.nav': '页脚', 'ft.demo': '演示', 'ft.faq': '常见问题',
}, {
    'page.title': 'VideoHelper', 'wall.label': 'Sample videos',
    'hero.h1a': 'In your videos', 'hero.h1b': 'find the moment',
    'hero.lede': 'VideoHelper searches inside video: describe a scene, a line someone said or a sound, and it finds those seconds in your videos. One click plays from there.',
    'hero.btn': 'Try it now', 'hero.note': 'Timecodes to a tenth of a second · picture, speech and sound together', 'hero.q': 'fish along the coral reef', 'hero.vid': 'Fish swimming along a coral reef', 'hero.strip': 'Clip timeline: move the pointer to see the frame',
    'ch.visual': 'Picture', 'ch.speech': 'Speech', 'ch.audio': 'Sound', rank: '{ch} #{n}',
    'view.h': 'Scrub to the second it matched', 'view.p': 'Every result is a span of time. The red line marks the seconds that matched; drag across the frames to preview them, and the three tiles below say whether picture, speech or sound matched.',
    'app.search': 'Search', 'app.lib': 'Library', 'app.proc': 'Processing', 'app.qlabel': 'Describe a scene, a line or a sound', 'app.go': 'Search', 'app.count': '{n} moments · by relevance', 'app.nomatch': 'Nothing for “{q}” in the sample library; try one of these',
    'demo.views': 'Result view', 'demo.dir': 'Gallery', 'demo.det': 'Detail', 'demo.back': 'Back to gallery', 'demo.hits': '{n} hits', 'demo.hit': '{n} hit', 'demo.more': '{n} more videos · switch to Detail for every moment', 'demo.more1': '{n} more video · switch to Detail for every moment', 'demo.ruler': 'Demo timeline',
    'done': 'Done', 'stage.visual': 'Picture encoding', 'stage.asr': 'Transcription', 'stage.audio': 'Sound encoding',
    'pl.h': 'Everything one video can tell you', 'how.label': 'Processing steps',
    'cta.q1': 'Try one sentence', 'cta.q2': 'in the demo above', 'cta.demo': 'Try the demo', 'cta.after': 'The demo runs on openly licensed sample clips; credits are in the footer.',
    'ft.what': 'VideoHelper · find moments in video', 'ft.nav': 'Footer', 'ft.demo': 'Demo', 'ft.faq': 'Questions',
});

// the sample library: [title zh, title en, start in source (s), length (s), project]
export const CLIP: Record<string, [string, string, number, number, string]> = {
    jelly: ['幻影水母', 'Phantom jellyfish', 0, 8, 'p1'], reef: ['珊瑚礁鱼群', 'Coral reef fishes', 0, 8, 'p1'], surf: ['特拉维夫冲浪', 'Surfing in Tel Aviv', 0, 8, 'p1'],
    rocket: ['火箭夜间发射', 'Night rocket launch', 1.5, 5.5, 'p2'], earth: ['空间站俯瞰地球', 'Earth from the ISS', 33, 8, 'p2'], iss: ['空间站与地球', 'The station over Earth', 72, 8, 'p2'],
    sintel: ['Sintel 预告片', 'Sintel trailer', 24, 6.6, 'p3'], bunny: ['Big Buck Bunny', 'Big Buck Bunny', 0, 6, 'p3'],
  };
export type Hit = [string, [string, number][], number, number];
// a query → ranked moments: [clip, [channel, rank]…, hit start, hit end] (seconds inside the clip, on the 2.5 s grid)
export const SETS: { q: [string, string]; keys: string[]; rows: Hit[] }[] = [
    { q: ['海里慢慢漂着的水母', 'a jellyfish drifting in the sea'], keys: ['水母', '漂', 'jelly', 'drift'], rows: [['jelly', [['visual', 1]], 0, 5], ['reef', [['visual', 3]], 2.5, 5], ['surf', [['visual', 6]], 5, 7.5]] },
    { q: ['夜里火箭升空', 'a rocket lifting off at night'], keys: ['火箭', '升空', '发射', 'rocket', 'launch'], rows: [['rocket', [['visual', 1], ['audio', 2]], 0, 2.5], ['iss', [['visual', 4]], 2.5, 5], ['earth', [['visual', 7]], 0, 2.5]] },
    { q: ['从太空看地球的云', 'clouds seen from space'], keys: ['太空', '地球', '云', 'space', 'earth', 'cloud'], rows: [['earth', [['visual', 1]], 0, 5], ['iss', [['visual', 2]], 0, 2.5], ['rocket', [['visual', 8]], 2.5, 5]] },
    { q: ['很多人在海上冲浪', 'people surfing'], keys: ['冲浪', '浪', 'surf', 'wave'], rows: [['surf', [['visual', 1], ['audio', 3]], 0, 5], ['reef', [['visual', 5]], 5, 7.5], ['jelly', [['visual', 9]], 5, 7.5]] },
    { q: ['森林里的大胖兔子', 'a big rabbit in the forest'], keys: ['兔', '森林', 'rabbit', 'bunny', 'forest'], rows: [['bunny', [['visual', 1]], 0, 2.5], ['sintel', [['visual', 6]], 2.5, 5]] },
    { q: ['夕阳下的古城废墟', 'ruins at sunset'], keys: ['古城', '废墟', '夕阳', '沙漠', 'ruin', 'sunset', 'desert', 'city'], rows: [['sintel', [['visual', 1]], 0, 5], ['earth', [['visual', 9]], 5, 7.5]] },
  ];
// The hero demo's results per query (same order as SETS): videos, each with its moments.
// Moment: clip (a file in media/), timecode label, span label, [channel, rank]…, where it sits in the video (%, %), crop.
export interface DemoMoment { clip: string; tc: string; range: string; chs: [string, number][]; at: number; len: number; pos: string }
export interface DemoVideo { title: [string, string]; dur: string; hits: DemoMoment[] }
const m = (clip: string, tc: string, range: string, chs: [string, number][], at: number, len: number, pos = '50% 50%'): DemoMoment => ({ clip, tc, range, chs, at, len, pos });
export const DEMO: DemoVideo[][] = [
  [{ title: ['幻影水母合集', 'Phantom jellyfish reel'], dur: '24:30', hits: [m('jelly', '08:21', '08:20 – 09:05', [['visual', 1], ['speech', 2]], 34, 3), m('jelly', '14:41', '14:40 – 15:05', [['visual', 3], ['audio', 2]], 60, 2, '35% 40%')] },
    { title: ['珊瑚礁鱼群', 'Coral reef fishes'], dur: '58:00', hits: [m('reef', '12:32', '12:30 – 13:40', [['speech', 1], ['visual', 2]], 21, 2), m('reef', '22:11', '22:10 – 22:30', [['speech', 3]], 38, 1, '30% 60%'), m('reef', '45:00', '45:00 – 45:12', [['visual', 6]], 77, 1, '70% 40%')] },
    { title: ['特拉维夫冲浪', 'Surfing in Tel Aviv'], dur: '12:44', hits: [m('surf', '00:20', '00:20 – 00:25', [['visual', 7]], 3, 1)] }],
  [{ title: ['夜间火箭发射', 'Night rocket launch'], dur: '05:30', hits: [m('rocket', '00:01', '00:01 – 00:04', [['visual', 1], ['audio', 2]], 4, 10), m('rocket', '00:04', '00:04 – 00:06', [['visual', 3]], 14, 6, '70% 30%')] },
    { title: ['空间站与地球', 'The station over Earth'], dur: '08:00', hits: [m('iss', '01:10', '01:10 – 01:15', [['visual', 4]], 15, 2)] },
    { title: ['空间站俯瞰地球', 'Earth from the ISS'], dur: '08:05', hits: [m('earth', '00:33', '00:33 – 00:38', [['visual', 7]], 7, 2)] }],
  [{ title: ['空间站俯瞰地球', 'Earth from the ISS'], dur: '08:05', hits: [m('earth', '00:33', '00:33 – 00:40', [['visual', 1]], 7, 2), m('earth', '01:05', '01:05 – 01:12', [['visual', 3]], 13, 2, '30% 70%'), m('earth', '03:02', '03:02 – 03:20', [['visual', 5]], 37, 4, '70% 30%')] },
    { title: ['空间站与地球', 'The station over Earth'], dur: '08:00', hits: [m('iss', '01:12', '01:12 – 01:20', [['visual', 2]], 15, 2)] }],
  [{ title: ['特拉维夫冲浪', 'Surfing in Tel Aviv'], dur: '12:44', hits: [m('surf', '00:00', '00:00 – 00:05', [['visual', 1], ['audio', 3]], 0, 1), m('surf', '00:30', '00:30 – 00:35', [['visual', 2]], 4, 1, '30% 40%'), m('surf', '06:10', '06:10 – 06:20', [['visual', 4]], 48, 2, '70% 60%')] },
    { title: ['珊瑚礁鱼群', 'Coral reef fishes'], dur: '58:00', hits: [m('reef', '05:05', '05:05 – 05:10', [['visual', 5]], 9, 1)] }],
  [{ title: ['Big Buck Bunny 片段', 'Big Buck Bunny cuts'], dur: '09:56', hits: [m('bunny', '00:00', '00:00 – 00:05', [['visual', 1]], 0, 2), m('bunny', '00:04', '00:04 – 00:08', [['visual', 2], ['speech', 3]], 1, 2, '30% 50%'), m('bunny', '03:40', '03:40 – 03:48', [['visual', 4]], 37, 2, '70% 50%')] },
    { title: ['Sintel 预告片', 'Sintel trailer'], dur: '00:52', hits: [m('sintel', '00:26', '00:26 – 00:30', [['visual', 6]], 50, 8)] }],
  [{ title: ['Sintel 预告片', 'Sintel trailer'], dur: '00:52', hits: [m('sintel', '00:24', '00:24 – 00:29', [['visual', 1]], 46, 9), m('sintel', '00:28', '00:28 – 00:32', [['visual', 3]], 54, 8, '30% 40%')] },
    { title: ['空间站俯瞰地球', 'Earth from the ISS'], dur: '08:05', hits: [m('earth', '00:38', '00:38 – 00:43', [['visual', 9]], 8, 1)] }],
];
// [part of the panel, title zh, text zh, title en, text en]
export const NOTES = [
    ['frame', '画面', '每 2.5 秒一格编码，搜到的那一格就是这一帧。', 'The frame', 'Picture is encoded every 2.5 seconds; the matching cell is this frame.'],
    ['range', '时间段', '结果是一段时间，标出起点、终点和从哪里开始播放。', 'The span', 'A result is a span: where it starts, ends and begins playing.'],
    ['lanes', '三路命中', '画面、台词、声音各自命中在哪，一眼看完。', 'Three channels', 'Where picture, speech and sound each matched, at a glance.'],
    ['stages', '处理进度', '画面编码完成就能先按画面搜，不用等转录。', 'Processing', 'Picture search opens before transcription finishes.'],
    ['find', '在视频里搜', '只在这个视频里找，比如「火箭离开发射台的那一刻」。', 'Search inside', 'Search one video only, like “the moment it leaves the pad”.'],
    ['tools', '跳转与整理', '从命中处播放，或者加入项目留着以后找。', 'Jump and file', 'Play from the hit, or add it to a project for later.'],
  ];
// [stop zh, title zh, detail zh, stop en, title en, detail en]
export const STAGES = [
    ['上传', '先把视频\u200b交给它', '浏览器按 16 MB 分片直接传到存储，不限大小。', 'Upload', 'Start by handing it the video', 'The browser sends 16 MB parts straight to storage, with no size limit.'],
    ['预处理', '然后\u200b切成一格一格', '转成 720p 的标准视频，并生成缩略帧。画面和声音每 2.5 秒一格。', 'Prepare', 'Then it is cut into cells', 'A 720p working copy and its thumbnails; picture and sound in 2.5-second cells.'],
    ['编码', '接着\u200b三路同时编码', '画面和声音按格编码，台词先转成句子再编码。', 'Encode', 'Three channels encode at once', 'Picture and sound by the cell; speech becomes sentences first.'],
    ['搜索', '最后\u200b一句话找到它', '三路结果按名次合并成片段，点一下从那一秒播放。', 'Search', 'One sentence finds the moment', 'The three ranks merge into spans; one click plays from that second.'],
  ];
// [question zh, answer zh, question en, answer en]
export const FAQ = [
    ['视频会存到哪里？', '存在你工作区自己的存储空间里，工作区之间完全隔开。浏览器把文件分片直接传到存储，不经过服务器中转。', 'Where are my videos stored?', 'In your workspace’s own storage, separated from every other workspace. The browser uploads parts straight to storage.'],
    ['单个视频最大能传多大？', '不设上限，只受存储服务单个文件 5 TB 的限制。大文件按 16 MB 一片并行上传。', 'How large can one video be?', 'No limit of ours, only storage’s 5 TB per file. Large files go up in parallel 16 MB parts.'],
    ['为什么结果是一段时间？', '你要找的是那几秒。每个结果写明从第几秒到第几秒、从哪里开始播放，以及哪几路命中。', 'Why is a result a span of time?', 'You are after those seconds. Each result gives start, end, where playback begins and which channels matched.'],
    ['搜索会把内容发给大模型吗？', '不会。搜索只用编码模型和固定的排名算法，不经过对话模型，同一句话每次结果一样。', 'Does search send my content to a chat model?', 'No. It uses embedding models and a fixed ranking method, so the same sentence always gives the same results.'],
    ['英文视频和声音搜索怎么样？', '台词识别目前以中文为主。声音检索用 CLAP，英文描述效果更好，中文还在评估。', 'How do English and sound search do?', 'Speech recognition is tuned for Chinese today. Sound search uses CLAP and works best with English descriptions.'],
    ['上传后多久能搜？', '取决于视频长度和机器。画面编码一完成就能先按画面搜，每一步进度都在视频详情里实时显示。', 'How soon can I search?', 'It depends on length and hardware. Picture search opens when picture encoding is done; every step shows live.'],
  ];
