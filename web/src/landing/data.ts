// Landing-page strings and the sample library the mini workbench runs on. No network calls.
import { addStrings } from '../i18n.ts';

addStrings({
    'page.title': 'VideoHelper', 'nav.demo': '打开完整工作台', 'wall.label': '示例视频',
    'hero.h1a': '在你的视频里，', 'hero.h1b': '找到那一刻。',
    'hero.lede': 'VideoHelper 是视频内容搜索工具：描述一个画面、一句台词或一种声音，它在你的视频里找到对应的那几秒，点一下就从那里播放。',
    'hero.btn': '现在就试试', 'hero.note': '时间码精确到 0.1 秒 · 画面、台词、声音三路一起找', 'hero.q': '珊瑚礁边的鱼群', 'hero.vid': '珊瑚礁边游动的鱼群', 'hero.strip': '片段时间线，移动指针查看画面',
    'ch.visual': '画面', 'ch.speech': '台词', 'ch.audio': '声音', rank: '{ch} 第 {n} 名',
    'try.h': '亲手试试工作台', 'try.p': '这是工作台的缩小版，能点、能搜、能播放。换一句描述，或者去资料库里点开一个视频。',
    'app.search': '搜索', 'app.lib': '资料库', 'app.proc': '处理中', 'app.projects': '项目', 'app.p1': '海洋', 'app.p2': '太空', 'app.p3': '动画',
    'app.qlabel': '描述画面、台词或声音', 'app.go': '搜索', 'app.count': '{n} 个片段 · 按相关度排序', 'app.nomatch': '示例库里没找到「{q}」，试试下面这几句', 'app.this': '这个视频里的命中',
    'st.ready': '可搜索', 'st.run': '处理中', 'st.bad': '失败', 'retry': '重试', 'done': '完成', 'wait': '等待', 'running': '进行中',
    'stage.probe': '读取媒体信息', 'stage.prep': '转码与缩略图', 'stage.visual': '画面编码', 'stage.asr': '语音转录', 'stage.audio': '声音编码', 'stage.speech': '台词编码', 'stage.fin': '完成',
    'proc.sub': '{n} / 7 完成 · 画面编码完成就能先按画面搜', 'proc.ok': '全部完成，可以搜索了', 'proc.bad': '语音转录失败，点重试只重跑没完成的部分',
    'pl.h': '一个视频的全部线索', 'how.label': '处理步骤',
    'cta.q1': '先传一段视频，', 'cta.q2': '还是先看演示？', 'cta.up': '上传视频', 'cta.or': '或', 'cta.demo': '打开完整工作台', 'cta.after': '工作台需要登录；上面的迷你版用的是示例视频。',
    'ft.what': 'VideoHelper · 在视频里找画面', 'ft.nav': '页脚', 'ft.search': '搜索', 'ft.library': '资料库', 'ft.faq': '常见问题',
}, {
    'page.title': 'VideoHelper', 'nav.demo': 'Open the full workbench', 'wall.label': 'Sample videos',
    'hero.h1a': 'In your videos,', 'hero.h1b': 'find the moment.',
    'hero.lede': 'VideoHelper searches inside video: describe a scene, a line someone said or a sound, and it finds those seconds in your videos. One click plays from there.',
    'hero.btn': 'Try it now', 'hero.note': 'Timecodes to a tenth of a second · picture, speech and sound together', 'hero.q': 'fish along the coral reef', 'hero.vid': 'Fish swimming along a coral reef', 'hero.strip': 'Clip timeline: move the pointer to see the frame',
    'ch.visual': 'Picture', 'ch.speech': 'Speech', 'ch.audio': 'Sound', rank: '{ch} #{n}',
    'try.h': 'Try the workbench', 'try.p': 'A small copy of the real workbench: click, search, play. Change the sentence or open a video from the library.',
    'app.search': 'Search', 'app.lib': 'Library', 'app.proc': 'Processing', 'app.projects': 'Projects', 'app.p1': 'Ocean', 'app.p2': 'Space', 'app.p3': 'Animation',
    'app.qlabel': 'Describe a scene, a line or a sound', 'app.go': 'Search', 'app.count': '{n} moments · by relevance', 'app.nomatch': 'Nothing for “{q}” in the sample library; try one of these', 'app.this': 'Hits in this video',
    'st.ready': 'Searchable', 'st.run': 'Processing', 'st.bad': 'Failed', 'retry': 'Retry', 'done': 'Done', 'wait': 'Waiting', 'running': 'Running',
    'stage.probe': 'Read media info', 'stage.prep': 'Transcode and thumbnails', 'stage.visual': 'Picture encoding', 'stage.asr': 'Transcription', 'stage.audio': 'Sound encoding', 'stage.speech': 'Speech encoding', 'stage.fin': 'Finish',
    'proc.sub': '{n} / 7 done · picture search opens when picture encoding is done', 'proc.ok': 'All done: ready to search', 'proc.bad': 'Transcription failed; a retry reruns only what is left',
    'pl.h': 'Everything one video can tell you', 'how.label': 'Processing steps',
    'cta.q1': 'Upload a video,', 'cta.q2': 'or see the demo?', 'cta.up': 'Upload a video', 'cta.or': 'or', 'cta.demo': 'Open the full workbench', 'cta.after': 'The workbench needs sign-in; the mini version above runs on sample clips.',
    'ft.what': 'VideoHelper · find moments in video', 'ft.nav': 'Footer', 'ft.search': 'Search', 'ft.library': 'Library', 'ft.faq': 'Questions',
});

// the sample library: [title zh, title en, start in source (s), length (s), project]
export const CLIP: Record<string, [string, string, number, number, string]> = {
    jelly: ['幻影水母', 'Phantom jellyfish', 0, 8, 'p1'], reef: ['珊瑚礁鱼群', 'Coral reef fishes', 0, 8, 'p1'], surf: ['特拉维夫冲浪', 'Surfing in Tel Aviv', 0, 8, 'p1'],
    rocket: ['火箭夜间发射', 'Night rocket launch', 1.5, 5.5, 'p2'], earth: ['空间站俯瞰地球', 'Earth from the ISS', 33, 8, 'p2'], iss: ['空间站与地球', 'The station over Earth', 72, 8, 'p2'],
    sintel: ['Sintel 预告片', 'Sintel trailer', 24, 7, 'p3'], bunny: ['Big Buck Bunny', 'Big Buck Bunny', 0, 6, 'p3'],
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
    ['上传', '先，把视频交给它。', '浏览器按 16 MB 分片直接传到存储，不限大小。', 'Upload', 'First, hand it the video.', 'The browser sends 16 MB parts straight to storage, with no size limit.'],
    ['预处理', '然后，切成一格一格。', '转成 720p 的标准视频，并生成缩略帧。画面和声音每 2.5 秒一格。', 'Prepare', 'Then it is cut into cells.', 'A 720p working copy and its thumbnails; picture and sound in 2.5-second cells.'],
    ['编码', '接着，三路同时编码。', '画面和声音按格编码，台词先转成句子再编码。', 'Encode', 'Next, three channels encode at once.', 'Picture and sound by the cell; speech becomes sentences first.'],
    ['搜索', '最后，一句话找到它。', '三路结果按名次合并成片段，点一下从那一秒播放。', 'Search', 'Finally, one sentence finds it.', 'The three ranks merge into spans; one click plays from that second.'],
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
