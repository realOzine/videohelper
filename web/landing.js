// VideoHelper landing v7. The sample library is eight real clips (open movies, ocean, space). The hero viewer scrubs one
// under the pointer; the mini workbench searches, plays, opens the library and runs processing. No network calls.
(() => {
  const $ = (s, el = document) => el.querySelector(s);
  const I = window.I18N, t = I.t;
  const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
  const esc = (v) => String(v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
  const L = () => (I.lang === 'en' ? 1 : 0);
  const tc = (s) => { const m = Math.floor(s / 60), r = s - m * 60; return `${String(m).padStart(2, '0')}:${(r < 10 ? '0' : '') + r.toFixed(1)}`; }; // taste-skill-ignore: a timecode

  I.add({
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
    'ft.credits': '示例视频（均为裁剪后的无声片段）：<a href="https://commons.wikimedia.org/wiki/File:Sintel_trailer-1080p.webm">Sintel</a>（Durian Open Movie Project，CC BY 3.0）；<a href="https://commons.wikimedia.org/wiki/File:Big_buck_bunny_montage.webm">Big Buck Bunny</a>（Blender Foundation，CC BY 3.0）；<a href="https://commons.wikimedia.org/wiki/File:Phantom_Jellyfish_Off_of_the_Melchior_Islands.webm">Phantom Jellyfish</a>（MasterfulNerd，CC BY 4.0）；<a href="https://commons.wikimedia.org/wiki/File:Coordinated-vigilance-provides-evidence-for-direct-reciprocity-in-coral-reef-fishes-srep14556-s2.webm">Coral reef fishes</a>（Brandl S、Bellwood D，CC BY 4.0）；<a href="https://commons.wikimedia.org/wiki/File:Surfing_in_Tel_Aviv_MVI_3675.webm">Surfing in Tel Aviv</a>（Deror Avi，CC BY-SA 4.0）；<a href="https://images.nasa.gov/">NASA</a> 的火箭发射与国际空间站画面（公有领域）。',
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
    'ft.credits': 'Sample videos (trimmed, silent): <a href="https://commons.wikimedia.org/wiki/File:Sintel_trailer-1080p.webm">Sintel</a> (Durian Open Movie Project, CC BY 3.0); <a href="https://commons.wikimedia.org/wiki/File:Big_buck_bunny_montage.webm">Big Buck Bunny</a> (Blender Foundation, CC BY 3.0); <a href="https://commons.wikimedia.org/wiki/File:Phantom_Jellyfish_Off_of_the_Melchior_Islands.webm">Phantom Jellyfish</a> (MasterfulNerd, CC BY 4.0); <a href="https://commons.wikimedia.org/wiki/File:Coordinated-vigilance-provides-evidence-for-direct-reciprocity-in-coral-reef-fishes-srep14556-s2.webm">Coral reef fishes</a> (Brandl S, Bellwood D, CC BY 4.0); <a href="https://commons.wikimedia.org/wiki/File:Surfing_in_Tel_Aviv_MVI_3675.webm">Surfing in Tel Aviv</a> (Deror Avi, CC BY-SA 4.0); rocket launch and ISS footage from <a href="https://images.nasa.gov/">NASA</a> (public domain).',
  });

  // ---------- the sample library: [title zh, title en, start in source (s), length (s), project] ----------
  const CLIP = {
    jelly: ['幻影水母', 'Phantom jellyfish', 0, 8, 'p1'], reef: ['珊瑚礁鱼群', 'Coral reef fishes', 0, 8, 'p1'], surf: ['特拉维夫冲浪', 'Surfing in Tel Aviv', 0, 8, 'p1'],
    rocket: ['火箭夜间发射', 'Night rocket launch', 1.5, 5.5, 'p2'], earth: ['空间站俯瞰地球', 'Earth from the ISS', 33, 8, 'p2'], iss: ['空间站与地球', 'The station over Earth', 72, 8, 'p2'],
    sintel: ['Sintel 预告片', 'Sintel trailer', 24, 7, 'p3'], bunny: ['Big Buck Bunny', 'Big Buck Bunny', 0, 6, 'p3'],
  };
  // a query → ranked moments: [clip, [channel, rank]…, hit start, hit end] (seconds inside the clip, on the 2.5 s grid)
  const SETS = [
    { q: ['海里慢慢漂着的水母', 'a jellyfish drifting in the sea'], keys: ['水母', '漂', 'jelly', 'drift'], rows: [['jelly', [['visual', 1]], 0, 5], ['reef', [['visual', 3]], 2.5, 5], ['surf', [['visual', 6]], 5, 7.5]] },
    { q: ['夜里火箭升空', 'a rocket lifting off at night'], keys: ['火箭', '升空', '发射', 'rocket', 'launch'], rows: [['rocket', [['visual', 1], ['audio', 2]], 0, 2.5], ['iss', [['visual', 4]], 2.5, 5], ['earth', [['visual', 7]], 0, 2.5]] },
    { q: ['从太空看地球的云', 'clouds seen from space'], keys: ['太空', '地球', '云', 'space', 'earth', 'cloud'], rows: [['earth', [['visual', 1]], 0, 5], ['iss', [['visual', 2]], 0, 2.5], ['rocket', [['visual', 8]], 2.5, 5]] },
    { q: ['很多人在海上冲浪', 'people surfing'], keys: ['冲浪', '浪', 'surf', 'wave'], rows: [['surf', [['visual', 1], ['audio', 3]], 0, 5], ['reef', [['visual', 5]], 5, 7.5], ['jelly', [['visual', 9]], 5, 7.5]] },
    { q: ['森林里的大胖兔子', 'a big rabbit in the forest'], keys: ['兔', '森林', 'rabbit', 'bunny', 'forest'], rows: [['bunny', [['visual', 1]], 0, 2.5], ['sintel', [['visual', 6]], 2.5, 5]] },
    { q: ['夕阳下的古城废墟', 'ruins at sunset'], keys: ['古城', '废墟', '夕阳', '沙漠', 'ruin', 'sunset', 'desert', 'city'], rows: [['sintel', [['visual', 1]], 0, 5], ['earth', [['visual', 9]], 5, 7.5]] },
  ];
  const title = (id) => CLIP[id][L()];
  const span = (id, a = 0, b = CLIP[id][3]) => `${tc(CLIP[id][2] + a)} – ${tc(CLIP[id][2] + b)}`;
  const rk = (c, n) => t('rank', { ch: t(`ch.${c}`), n });

  // ---------- hero viewer: the strip reads the pointer and scrubs the real clip ----------
  const hv = $('#hv'), strip = $('#hstrip'), htc = $('#htc');
  const HLEN = CLIP.reef[3], HIT = [2.5, 5];
  strip.innerHTML = Array.from({ length: 8 }, (_, i) => `<span style="background-image:url(assets/reef-strip.jpg);background-size:800% 100%;background-position:${(i * 100 / 7).toFixed(3)}% 0"></span>`).join('')
    + `<span class="hit" style="left:${HIT[0] / HLEN * 100}%;width:${(HIT[1] - HIT[0]) / HLEN * 100}%"></span><span class="head" id="hhead"></span>`;
  const head = $('#hhead');
  let scrubbing = false;
  const showAt = (sec) => { head.style.left = `${(sec / HLEN * 100).toFixed(2)}%`; htc.textContent = tc(sec); strip.setAttribute('aria-valuenow', sec.toFixed(1)); strip.setAttribute('aria-valuetext', tc(sec)); };
  const seek = (x) => { const r = strip.getBoundingClientRect(); const sec = Math.max(0, Math.min(HLEN - 0.05, (x - r.left) / r.width * HLEN)); try { hv.currentTime = sec; } catch { /* not seekable yet */ } showAt(sec); };
  strip.addEventListener('pointermove', (e) => { scrubbing = true; hv.pause(); seek(e.clientX); });
  strip.addEventListener('pointerdown', (e) => { strip.setPointerCapture(e.pointerId); seek(e.clientX); });
  strip.addEventListener('pointerleave', () => { scrubbing = false; if (!reduce) hv.play().catch(() => {}); });
  strip.addEventListener('keydown', (e) => { const d = { ArrowLeft: -0.5, ArrowRight: 0.5 }[e.key]; if (d) { e.preventDefault(); hv.pause(); const s = Math.max(0, Math.min(HLEN - 0.05, hv.currentTime + d)); hv.currentTime = s; showAt(s); } });
  hv.addEventListener('timeupdate', () => { if (!scrubbing) showAt(hv.currentTime); });
  if (reduce) document.querySelectorAll('video').forEach((v) => { v.removeAttribute('autoplay'); v.pause(); });

  // ---------- the mini workbench ----------
  const app = $('#app'), av = $('#av'), aq = $('#aq');
  const LIB = { jelly: 'ready', reef: 'ready', surf: 'ready', rocket: 'ready', earth: 'ready', iss: 'run', sintel: 'ready', bunny: 'bad' };
  const PLAN = () => ({ probe: 'done', prep: 'done', visual: 'run', asr: 'run', audio: 'done', speech: 'wait', fin: 'wait' });
  const proc = { id: 'iss', st: PLAN(), pv: 0.64, timer: null };
  let cur = SETS[0], sel = 0, view = 'search';
  function show(v) {
    view = v;
    app.querySelectorAll('aside [data-view]').forEach((b) => (b.dataset.view === v && !b.classList.contains('proj') ? b.setAttribute('aria-current', 'page') : b.removeAttribute('aria-current')));
    ['search', 'lib', 'proc'].forEach((k) => { $(`#v-${k}`).hidden = k !== v; });
    $('#chips').hidden = v !== 'search';
    if (v === 'lib') renderLib();
    if (v === 'proc') renderProc();
  }
  app.querySelector('aside').addEventListener('click', (e) => { const b = e.target.closest('[data-view]'); if (b) show(b.dataset.view); });
  function renderChips() { $('#chips').innerHTML = SETS.map((s, i) => `<button type="button" data-i="${i}" aria-pressed="${s === cur}">${esc(s.q[L()])}</button>`).join(''); }
  $('#chips').addEventListener('click', (e) => { const b = e.target.closest('button'); if (!b) return; cur = SETS[+b.dataset.i]; aq.value = cur.q[L()]; sel = 0; renderSearch(); });
  $('#qf').addEventListener('submit', (e) => {
    e.preventDefault();
    const v = aq.value.trim().toLowerCase();
    const best = SETS.map((s) => ({ s, n: s.keys.filter((k) => v.includes(k)).length })).sort((a, b) => b.n - a.n)[0];
    if (best.n) { cur = best.s; sel = 0; renderSearch(); } else { $('#acount').textContent = t('app.nomatch', { q: aq.value.trim() }); }
  });
  function renderSearch() {
    renderChips();
    $('#acount').textContent = t('app.count', { n: cur.rows.length });
    $('#ares').innerHTML = cur.rows.map(([id, chs, a, b], i) => `<button type="button" data-i="${i}" aria-pressed="${i === sel}"><span class="film"><img src="assets/${id}.jpg" width="720" height="405" alt=""></span>
      <span><b>${esc(title(id))}</b><span class="m">${span(id, a, b)}</span><br><span class="rk">${chs.map(([c, n]) => rk(c, n)).join(' · ')}</span></span></button>`).join('');
    play();
  }
  $('#ares').addEventListener('click', (e) => { const b = e.target.closest('button'); if (b) { sel = +b.dataset.i; $('#ares').querySelectorAll('button').forEach((x) => x.setAttribute('aria-pressed', String(x === b))); play(); } });
  function play() {
    const [id, chs, a, b] = cur.rows[sel], len = CLIP[id][3];
    if (!av.src.endsWith(`assets/${id}.mp4`)) { av.src = `assets/${id}.mp4`; av.poster = `assets/${id}.jpg`; }
    av.setAttribute('aria-label', title(id));
    const go = () => { try { av.currentTime = a; } catch { /* not seekable yet */ } if (!reduce) av.play().catch(() => {}); };
    if (av.readyState >= 1) go(); else av.addEventListener('loadedmetadata', go, { once: true });
    $('#atitle').textContent = title(id);
    $('#arange').textContent = span(id, a, b);
    const has = (c) => chs.some(([k]) => k === c);
    $('#alanes').innerHTML = ['visual', 'speech', 'audio'].map((c) => `<span>${t(`ch.${c}`)}</span><span class="ln" data-c="${c}">${has(c) ? `<i class="on" style="left:${a / len * 100}%;width:${(b - a) / len * 100}%"></i>` : ''}${c === 'visual' ? `<i style="left:${((a + 5) % len) / len * 100}%;width:${2.5 / len * 100}%"></i>` : ''}<span class="ph"></span></span>`).join('');
  }
  $('#alanes').addEventListener('click', (e) => { const ln = e.target.closest('.ln'); if (!ln) return; const r = ln.getBoundingClientRect(); const id = cur.rows[sel][0]; av.currentTime = Math.max(0, Math.min(CLIP[id][3] - 0.05, (e.clientX - r.left) / r.width * CLIP[id][3])); });
  av.addEventListener('timeupdate', () => {
    const id = cur.rows[sel][0], len = CLIP[id][3];
    $('#atc').textContent = tc(CLIP[id][2] + av.currentTime);
    app.querySelectorAll('#alanes .ph').forEach((p) => { p.style.left = `${(av.currentTime / len * 100).toFixed(2)}%`; });
  });
  function renderLib() {
    const ST = { ready: ['ok', 'st.ready'], run: ['run', 'st.run'], bad: ['bad', 'st.bad'] };
    $('#alib').innerHTML = Object.keys(CLIP).map((id) => `<button type="button" data-id="${id}"><span class="film"><img src="assets/${id}.jpg" width="720" height="405" alt=""><span class="pill ${ST[LIB[id]][0]}">${t(ST[LIB[id]][1])}</span></span><b>${esc(title(id))}</b><span class="m">${t(`app.${CLIP[id][4]}`)} · ${tc(CLIP[id][3])}</span></button>`).join('');
    $('#proccnt').textContent = Object.values(LIB).filter((s) => s !== 'ready').length || '';
  }
  $('#alib').addEventListener('click', (e) => {
    const b = e.target.closest('button'); if (!b) return; const id = b.dataset.id;
    if (LIB[id] === 'ready') { const s = SETS.find((x) => x.rows[0][0] === id) || SETS[0]; cur = s; sel = s.rows.findIndex((r) => r[0] === id); if (sel < 0) sel = 0; aq.value = cur.q[L()]; renderSearch(); show('search'); return; }
    proc.id = id; proc.st = PLAN(); proc.pv = 0.64;
    if (LIB[id] === 'bad') { proc.st.visual = 'done'; proc.st.asr = 'bad'; proc.pv = 1; }
    show('proc');
  });
  const ORDER = ['probe', 'prep', 'visual', 'asr', 'audio', 'speech', 'fin'];
  function renderProc() {
    const s = proc.st, done = ORDER.filter((k) => s[k] === 'done').length;
    $('#pimg').src = `assets/${proc.id}.jpg`; $('#pimg').alt = title(proc.id);
    $('#ptitle').textContent = title(proc.id);
    $('#psub').textContent = done === 7 ? t('proc.ok') : s.asr === 'bad' ? t('proc.bad') : t('proc.sub', { n: done });
    $('#pstg').innerHTML = ORDER.map((k) => {
      const v = s[k], label = v === 'run' ? (k === 'visual' ? `${Math.round(proc.pv * 100)}%` : t('running')) : v === 'bad' ? `<button class="retry" type="button">${t('retry')}</button>` : t(v === 'done' ? 'done' : 'wait');
      return `<div><span>${t(`stage.${k}`)}</span><span class="${v === 'bad' ? 'bad' : 'k'}">${label}</span>${v === 'run' ? `<span class="bar2"><i style="transform:scaleX(${k === 'visual' ? proc.pv : 0.35})"></i></span>` : ''}</div>`;
    }).join('');
    clearInterval(proc.timer);
    if (done < 7 && s.asr !== 'bad') proc.timer = setInterval(tick, 700);
  }
  // shortcut: simulates the processing a real upload goes through; the full workbench polls GET /v1/videos/{id}
  function tick() {
    const s = proc.st;
    if (s.visual === 'run') { proc.pv = Math.min(1, proc.pv + 0.06); if (proc.pv >= 1) s.visual = 'done'; }
    else if (s.asr === 'run') { s.asr = 'done'; s.speech = 'run'; }
    else if (s.speech === 'run') { s.speech = 'done'; s.fin = 'run'; }
    else if (s.fin === 'run') { s.fin = 'done'; LIB[proc.id] = 'ready'; }
    if (view === 'proc') renderProc(); else clearInterval(proc.timer);
    renderLibCount();
  }
  const renderLibCount = () => { $('#proccnt').textContent = Object.values(LIB).filter((v) => v !== 'ready').length || ''; };
  $('#pstg').addEventListener('click', (e) => { if (e.target.closest('.retry')) { proc.st.asr = 'run'; LIB[proc.id] = 'run'; renderProc(); } });

  // ---------- the ticker: sample sentences from the library ----------
  const renderTicker = () => { const items = SETS.map((s) => `<li>${esc(s.q[L()])}</li>`).join(''); $('#ticker').innerHTML = items + items; };

  // ---------- one panel, notes in both margins ----------
  const NOTES = [
    ['frame', '画面', '每 2.5 秒一格编码，搜到的那一格就是这一帧。', 'The frame', 'Picture is encoded every 2.5 seconds; the matching cell is this frame.'],
    ['range', '时间段', '结果是一段时间，标出起点、终点和从哪里开始播放。', 'The span', 'A result is a span: where it starts, ends and begins playing.'],
    ['lanes', '三路命中', '画面、台词、声音各自命中在哪，一眼看完。', 'Three channels', 'Where picture, speech and sound each matched, at a glance.'],
    ['stages', '处理进度', '画面编码完成就能先按画面搜，不用等转录。', 'Processing', 'Picture search opens before transcription finishes.'],
    ['find', '在视频里搜', '只在这个视频里找，比如「火箭离开发射台的那一刻」。', 'Search inside', 'Search one video only, like “the moment it leaves the pad”.'],
    ['tools', '跳转与整理', '从命中处播放，或者加入项目留着以后找。', 'Jump and file', 'Play from the hit, or add it to a project for later.'],
  ];
  function renderPanel() {
    const l = L(), o = l * 2;
    $('#panel').innerHTML = `<div data-k="frame" class="film"><img src="assets/rocket.jpg" width="720" height="405" alt="${l ? 'A rocket lighting the night sky at launch' : '火箭夜间发射时照亮天空'}"></div>
      <div data-k="range" class="hd"><b>${esc(title('rocket'))}</b><span class="m">${span('rocket', 0, 2.5)}</span></div><div class="m">${rk('visual', 1)} · ${rk('audio', 2)}</div>
      <div data-k="lanes" class="blk"><h4>${l ? 'Where it matched' : '命中位置'}</h4><div class="ln3">
        <span>${t('ch.visual')}</span><span class="t"><i class="on" style="left:6%"></i><i style="left:62%"></i></span>
        <span>${t('ch.speech')}</span><span class="t"></span>
        <span>${t('ch.audio')}</span><span class="t"><i class="on" style="left:8%"></i><i style="left:40%"></i></span></div></div>
      <div data-k="stages" class="blk"><h4>${l ? 'Processing' : '处理进度'}</h4><div class="stgx">
        ${[['stage.visual'], ['stage.asr'], ['stage.audio']].map(([k]) => `<div><span>${t(k)}</span><span class="ok">${t('done')}</span></div>`).join('')}</div></div>
      <div data-k="find" class="blk"><h4>${l ? 'Search this video' : '在这个视频里搜索'}</h4><div class="say"><b>${tc(1.5)}</b><span>${l ? 'the moment it leaves the pad' : '火箭离开发射台的那一刻'}</span></div></div>
      <div data-k="tools" class="tools"><button type="button" class="p">${l ? 'Play from 00:01.5' : '从 00:01.5 播放'}</button><button type="button">${l ? 'Add to project' : '加入项目'}</button></div>`;
    const note = (n) => `<div class="note" tabindex="0" data-k="${n[0]}"><h3>${esc(n[1 + o])}</h3><p>${esc(n[2 + o])}</p></div>`;
    $('#nl').innerHTML = NOTES.filter((_, i) => i % 2 === 0).map(note).join('');
    $('#nr').innerHTML = NOTES.filter((_, i) => i % 2 === 1).map(note).join('');
    align();
  }
  function align() {
    if (matchMedia('(max-width: 960px)').matches) return;
    document.querySelectorAll('.note').forEach((n) => { const part = $('#panel').querySelector(`[data-k="${n.dataset.k}"]`); if (part) n.style.top = `${part.offsetTop + 16}px`; });
  }
  new ResizeObserver(() => align()).observe($('#panel'));
  const panel = $('#panel');
  const lightPart = (k) => { panel.classList.toggle('dim', !!k); panel.querySelectorAll('[data-k]').forEach((el) => el.classList.toggle('lit', el.dataset.k === k)); document.querySelectorAll('.note').forEach((n) => n.classList.toggle('on', n.dataset.k === k)); };
  ['nl', 'nr'].forEach((id) => {
    const box = $(`#${id}`);
    box.addEventListener('pointerover', (e) => { const n = e.target.closest('.note'); if (n) lightPart(n.dataset.k); });
    box.addEventListener('pointerleave', () => lightPart(null));
    box.addEventListener('focusin', (e) => { const n = e.target.closest('.note'); if (n) lightPart(n.dataset.k); });
    box.addEventListener('focusout', () => lightPart(null));
  });

  // ---------- drag through the stages ----------
  const STAGES = [
    ['上传', '先，把视频交给它。', '浏览器按 16 MB 分片直接传到存储，不限大小。', 'Upload', 'First, hand it the video.', 'The browser sends 16 MB parts straight to storage, with no size limit.'],
    ['预处理', '然后，切成一格一格。', '转成 720p 的标准视频，并生成缩略帧。画面和声音每 2.5 秒一格。', 'Prepare', 'Then it is cut into cells.', 'A 720p working copy and its thumbnails; picture and sound in 2.5-second cells.'],
    ['编码', '接着，三路同时编码。', '画面和声音按格编码，台词先转成句子再编码。', 'Encode', 'Next, three channels encode at once.', 'Picture and sound by the cell; speech becomes sentences first.'],
    ['搜索', '最后，一句话找到它。', '三路结果按名次合并成片段，点一下从那一秒播放。', 'Search', 'Finally, one sentence finds it.', 'The three ranks merge into spans; one click plays from that second.'],
  ];
  const range = $('#stage'), how = $('#how'), stops = $('#stops');
  let shown = -1;
  function frag(i) {
    const l = L();
    const pic = `<div class="film"><img src="assets/surf.jpg" width="720" height="405" alt="${l ? 'Surfers waiting for a wave' : '海上等浪的冲浪者'}"></div>`;
    if (i === 0) return `${pic}<div class="m">${l ? 'Uploading' : '正在上传'}</div><b>surf_tel_aviv.mp4</b><div class="bar"><i style="transform:scaleX(.41)"></i></div><div class="m">${l ? '3 / 7 parts · 104 MB · 41%' : '3 / 7 分片 · 104 MB · 41%'}</div>`;
    if (i === 1) return `<div class="m">${l ? 'Thumbnail frames · 720p' : '缩略帧 · 720p'}</div><div class="cells">${Array.from({ length: 12 }, (_, k) => `<span style="background-image:url(assets/${k < 8 ? 'surf' : 'jelly'}-strip.jpg);background-size:800% 100%;background-position:${((k % 8) * 100 / 7).toFixed(2)}% 0"></span>`).join('')}</div>`;
    if (i === 2) return `${pic}${[[t('ch.visual'), 0.64], [t('ch.speech'), 0.3], [t('ch.audio'), 1]].map(([a, p]) => `<div class="m">${a} · ${Math.round(p * 100)}%</div><div class="bar"><i style="transform:scaleX(${p})"></i></div>`).join('')}`;
    return `${pic}<b>${esc(title('surf'))}</b><div class="m">${span('surf', 0, 5)} · ${rk('visual', 1)}</div>`;
  }
  function stage(i) {
    if (i === shown) return;
    const first = shown < 0; shown = i; range.value = String(i);
    range.style.setProperty('--pass', `${i / 3 * 100}%`);
    const s = STAGES[i], o = L() * 3;
    range.setAttribute('aria-valuetext', `${s[o]}: ${s[1 + o]}`);
    stops.querySelectorAll('li').forEach((li, k) => (k === i ? li.setAttribute('aria-current', 'step') : li.removeAttribute('aria-current')));
    const swap = () => { $('#howh').textContent = s[1 + o]; $('#howd').textContent = s[2 + o]; $('#frag').innerHTML = frag(i); how.classList.remove('fading'); };
    if (first || reduce) return swap();
    how.classList.add('fading'); setTimeout(swap, 200);
  }
  range.addEventListener('input', () => stage(+range.value));
  stops.addEventListener('click', (e) => { const b = e.target.closest('button'); if (b) stage(+b.dataset.i); });

  // ---------- faq ----------
  const FAQ = [
    ['视频会存到哪里？', '存在你工作区自己的存储空间里，工作区之间完全隔开。浏览器把文件分片直接传到存储，不经过服务器中转。', 'Where are my videos stored?', 'In your workspace’s own storage, separated from every other workspace. The browser uploads parts straight to storage.'],
    ['单个视频最大能传多大？', '不设上限，只受存储服务单个文件 5 TB 的限制。大文件按 16 MB 一片并行上传。', 'How large can one video be?', 'No limit of ours, only storage’s 5 TB per file. Large files go up in parallel 16 MB parts.'],
    ['为什么结果是一段时间？', '你要找的是那几秒。每个结果写明从第几秒到第几秒、从哪里开始播放，以及哪几路命中。', 'Why is a result a span of time?', 'You are after those seconds. Each result gives start, end, where playback begins and which channels matched.'],
    ['搜索会把内容发给大模型吗？', '不会。搜索只用编码模型和固定的排名算法，不经过对话模型，同一句话每次结果一样。', 'Does search send my content to a chat model?', 'No. It uses embedding models and a fixed ranking method, so the same sentence always gives the same results.'],
    ['英文视频和声音搜索怎么样？', '台词识别目前以中文为主。声音检索用 CLAP，英文描述效果更好，中文还在评估。', 'How do English and sound search do?', 'Speech recognition is tuned for Chinese today. Sound search uses CLAP and works best with English descriptions.'],
    ['上传后多久能搜？', '取决于视频长度和机器。画面编码一完成就能先按画面搜，每一步进度都在视频详情里实时显示。', 'How soon can I search?', 'It depends on length and hardware. Picture search opens when picture encoding is done; every step shows live.'],
  ];
  function renderFaq() {
    const o = L() * 2;
    $('#faq').innerHTML = `<div class="row rise"><h2 class="disp" id="faqh">${L() ? 'Questions' : '常见问题'}</h2></div>`
      + FAQ.map((f, i) => `<div class="row rise" style="--d:${(i % 3) * 60}ms"><h3>${esc(f[o])}</h3><p>${esc(f[o + 1])}</p></div>`).join('');
  }

  const io = new IntersectionObserver((es) => es.forEach((e) => { if (e.isIntersecting) { e.target.classList.add('in'); io.unobserve(e.target); } }), { rootMargin: '0px 0px -6% 0px' });
  function renderAll() {
    I.apply();
    $('#hsrc').textContent = `${title('reef')} · ${span('reef')}`;
    renderTicker(); aq.value = cur.q[L()]; renderSearch(); if (view !== 'search') show(view);
    renderPanel(); renderFaq();
    stops.innerHTML = STAGES.map((s, i) => `<li><button type="button" data-i="${i}">${s[L() * 3]}</button></li>`).join('');
    const k = shown < 0 ? 0 : shown; shown = -1; stage(k);
    document.querySelectorAll('.rise:not(.in)').forEach((el) => io.observe(el));
  }
  I.on(renderAll);
  renderAll(); renderLibCount();
  requestAnimationFrame(() => requestAnimationFrame(() => $('.hero').classList.add('in')));
})();
