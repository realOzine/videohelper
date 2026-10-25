// The workbench pages, rendered from the /v1 API. They are static files: any host can serve them,
// and the API admits this origin through server.cors_origins.
import { call, get, post, saveSettings, settings, signOut } from './api.js';
import { CHANNELS, channelState, eachLimit, esc, partRange, pipeline, safeNext, stageKind, tc, ticks, tile } from './lib.js';

const PAGE = document.querySelector('.app').dataset.page;
const _ = window.I18N.t;
const $ = (s, el = document) => el.querySelector(s);
const params = new URLSearchParams(location.search);
const CH = Object.fromEntries(CHANNELS.map((c) => [c, _(`ch.${c}`)]));
const icon = {
  search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="11" cy="11" r="6.5"/><path d="M20 20l-4-4"/></svg>',
  library: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="6" width="18" height="13" rx="2"/><path d="M6 3h12"/><path d="M10.5 10v5l4-2.5z" fill="currentColor"/></svg>',
  folder: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>',
  play: '<svg viewBox="0 0 24 24"><path d="M7 4.5v15l12.5-7.5z" fill="currentColor"/></svg>',
  pause: '<svg viewBox="0 0 24 24"><path d="M7 4.5h3.5v15H7zM13.5 4.5H17v15h-3.5z" fill="currentColor"/></svg>',
  check: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.2" stroke-linecap="round"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>',
  x: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.2" stroke-linecap="round"><path d="M7 7l10 10M17 7L7 17"/></svg>',
  menu: '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M4 7h16M4 12h16M4 17h16"/></svg>',
};
// the mark: a frame with a playhead through it, the playhead in the accent
const mark = '<svg viewBox="0 0 26 26" fill="none" aria-hidden="true"><rect x="1.5" y="5.5" width="23" height="15" rx="3.5" stroke="currentColor" stroke-width="1.8"/><path d="M15 2v22" stroke="#f43d3f" stroke-width="2.4" stroke-linecap="round"/></svg>';
const TONE = { ready: 'pos', failed: 'neg', indexing: 'accent' };
const pill = (status) => `<span class="pill ${TONE[status] === 'accent' ? '' : TONE[status] || ''}"${TONE[status] === 'accent' ? ' style="background:var(--pill-accent-bg);color:var(--pill-accent-ink)"' : ''}>${_(`st.${status}`)}</span>`;
const thumb = (v, extra = '') => `<span class="thumb">${v.thumbnail ? tile(v.thumbnail, _('frame.of', { t: v.title })) : ''}${extra}</span>`;
const tracks = (v) => (v.has_video == null ? _('tr.pending') : v.has_audio ? _('tr.both') : _('tr.visualonly'));
const busy = (status) => ['uploading', 'importing', 'indexing'].includes(status);
const stageLabel = (s) => { const k = `stage.${stageKind(s.stage)}`, t = _(k); return t === k ? s.stage : t; };

function when(iso) {
  const d = new Date(iso), now = new Date(), hm = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
  if (d.toDateString() === now.toDateString()) return `${_('today')} ${hm}`;
  if (d.toDateString() === new Date(now - 864e5).toDateString()) return `${_('yesterday')} ${hm}`;
  return d.toLocaleDateString('en-CA');
}

function fail(err) {
  const n = $('#notice');
  n.textContent = err.code === 'network' ? _('err.network', { base: settings().base }) : err.message;
  n.hidden = false;
}

async function newProject() {
  const name = prompt(_('p.name'));
  if (!name || !name.trim()) return;
  try { await post('/v1/projects', { name: name.trim() }); location.reload(); } catch (err) { fail(err); }
}

// Sidebar and top bar for every signed-in page; resolves to the workspace's projects.
async function shell(page, query = '') {
  $('#topbar').innerHTML = `
    <button class="btn ghost menu-toggle" type="button" aria-label="${_('top.menu')}">${icon.menu}</button>
    <form role="search" action="search.html">${icon.search}<label class="sr-only" for="q" style="position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)">${_('top.searchlabel')}</label>
      <input id="q" name="q" type="search" value="${esc(query)}" placeholder="${_('top.placeholder')}"><kbd>/</kbd></form>
    <div class="langsw" role="group" aria-label="语言 Language"><button type="button" data-lang="zh">中文</button><button type="button" data-lang="en">EN</button></div>
    <a class="switcher" href="signin.html" title="${_('top.account')}"><span class="avatar">${_('top.me')}</span><span id="ws"></span></a>`;
  $('.menu-toggle').addEventListener('click', () => $('#sidebar').classList.toggle('open'));
  document.addEventListener('keydown', (e) => {
    if (e.key === '/' && !e.target.closest('input, select, textarea')) { e.preventDefault(); $('#q').focus(); $('#q').select(); }
  });
  const [{ projects }, me, failed] = await Promise.all([get('/v1/projects'), get('/v1/me'), get('/v1/videos?status=failed&limit=100')]);
  const n = failed.videos.length;
  $('#ws').textContent = _('top.ws', { id: me.workspace_id });
  $('#sidebar').innerHTML = `
    <a class="brand" href="search.html">${mark}VideoHelper</a>
    <a class="nav-item" href="search.html"${page === 'search' ? ' aria-current="page"' : ''}>${icon.search}${_('nav.search')}</a>
    <a class="nav-item" href="library.html"${page === 'library' ? ' aria-current="page"' : ''}>${icon.library}${_('nav.library')}${n ? `<span class="count" aria-label="${_('nav.failed.aria', { n })}">${_('nav.failed', { n })}</span>` : ''}</a>
    <div class="group-label">${_('nav.projects')}</div>
    ${projects.map((p) => `<a class="nav-item" href="library.html?project=${p.id}"${params.get('project') === p.id ? ' aria-current="page"' : ''}>${icon.folder}${esc(p.name)}<span class="count" style="color:var(--muted)">${p.video_count}</span></a>`).join('')}
    <button class="nav-item" id="newproj-side" type="button" style="border:0;background:none;cursor:pointer;text-align:left">${icon.folder.replace('<path', '<path d="M12 10v6M9 13h6" /><path')}${_('nav.newproject')}</button>`;
  $('#newproj-side').addEventListener('click', newProject);
  return projects;
}

// ---------- search ----------
// The hit lanes are the scrubber: one row per channel across the whole video, the playhead runs through all three.
async function searchPage() {
  const q = (params.get('q') || '').trim(), project = params.get('project') || '';
  const projects = await shell('search', q);
  const scope = $('#scope');
  scope.insertAdjacentHTML('beforeend', projects.map((p) => `<option value="${p.id}">${esc(p.name)}</option>`).join(''));
  scope.value = project;
  scope.addEventListener('change', () => { location.search = new URLSearchParams({ q, ...(scope.value && { project: scope.value }) }); });
  $('#qh').textContent = q ? `“${q}”` : _('s.title');
  if (!q) { $('#sub').textContent = _('s.empty'); $('#found').hidden = true; return; }
  const { moments: ms, skipped_channels: skipped } = await post('/v1/search', { query: q, project_id: project || undefined, limit: 50 });
  const note = skipped && skipped.length ? ` ${_('s.skipped', { chs: skipped.map((c) => CH[c] || c).join(_('and')) })}` : '';
  if (!ms.length) { $('#sub').textContent = _('s.none') + note; $('#found').hidden = true; return; }
  const best = ms[0];
  $('#sub').innerHTML = _('s.sub', { n: ms.length, v: new Set(ms.map((m) => m.video_id)).size, title: esc(best.video_title), tc: tc(best.start_ms), chs: best.channels.map((h) => CH[h.channel]).join(_('and')) }) + esc(note);
  $('#count').textContent = _('s.count', { n: ms.length });
  const list = $('#results'), plot = $('#plot'), frame = $('#frame'), playBtn = $('#play');
  list.innerHTML = ms.map((m, i) => `<li><button class="result" type="button" data-i="${i}" aria-pressed="false">
      ${thumb({ title: m.video_title, thumbnail: m.thumbnail }, `<span class="tc">${tc(m.jump_ms)}</span>`)}
      <span class="txt"><span class="t">${esc(m.video_title)}</span><span class="r">${tc(m.start_ms, true)} – ${tc(m.end_ms, true)}</span>
      <span class="chs">${m.channels.map((h) => `<span class="ch ${h.channel}">${_('rank', { ch: CH[h.channel], n: h.rank })}</span>`).join('')}</span></span>
    </button></li>`).join('');
  const buttons = [...list.querySelectorAll('.result')];
  frame.innerHTML = '<video id="vid" playsinline preload="metadata"></video>';
  const vid = $('#vid'), loaded = new Map(), st = { picked: -1, vid: null, dur: 1 };
  // one video detail and playback URL per video, fetched when a result from it is first picked
  const load = (id) => {
    if (!loaded.has(id)) loaded.set(id, Promise.all([get(`/v1/videos/${id}`), get(`/v1/videos/${id}/playback`)]).then(([d, p]) => ({ v: d.video, url: p.url })));
    return loaded.get(id);
  };
  const seek = (ms) => { const go = () => { vid.currentTime = ms / 1000; }; if (vid.readyState >= 1) go(); else vid.addEventListener('loadedmetadata', go, { once: true }); setT(ms); };

  function buildVideo(v) {
    const mine = ms.filter((x) => x.video_id === v.id);
    st.dur = v.duration_ms || Math.max(...mine.map((x) => x.end_ms));
    const pct = (x) => `${(x / st.dur * 100).toFixed(3)}%`;
    $('#labs').innerHTML = Object.values(CH).map((l) => `<span class="lab">${l}</span>`).join('');
    plot.innerHTML = CHANNELS.map((c) => {
      const none = c !== 'visual' && v.has_audio === false ? `<span class="none">${_('s.noaudio')}</span>` : '';
      return `<span class="lane">${none}${mine.filter((x) => x.channels.some((h) => h.channel === c)).map((x) => `<span class="hit" data-i="${ms.indexOf(x)}" style="left:${pct(x.start_ms)};width:${pct(x.end_ms - x.start_ms)}" title="${tc(x.start_ms)} – ${tc(x.end_ms)}"></span>`).join('')}</span>`;
    }).join('') + `<span class="axis">${ticks(st.dur).map((t) => `<span style="left:${pct(t)}">${tc(t)}</span>`).join('')}</span><span class="ph" id="ph"><i></i></span>`;
    plot.setAttribute('aria-valuemax', String(Math.round(st.dur / 1000)));
    $('#lanecap').textContent = _('s.lanecap', { n: mine.length });
  }
  function setT(t) {
    const at = Math.max(0, Math.min(st.dur, t));
    if (!$('#ph')) return;
    $('#ph').style.transform = `translateX(${(at / st.dur * 100).toFixed(3)}%)`;
    $('#time').innerHTML = `<b>${tc(at)}</b> / ${tc(st.dur)}`;
    plot.setAttribute('aria-valuenow', String(Math.round(at / 1000)));
    plot.setAttribute('aria-valuetext', _('s.valuetext', { a: tc(at), b: tc(st.dur) }));
  }
  async function select(i, jump = true) {
    const m = ms[i];
    st.picked = i;
    buttons.forEach((b) => b.setAttribute('aria-pressed', String(+b.dataset.i === i)));
    $('#mtitle').textContent = m.video_title;
    $('#mmeta').textContent = `${tc(m.start_ms, true)} – ${tc(m.end_ms, true)}${_('s.meta', { secs: Math.round((m.end_ms - m.start_ms) / 1000), tc: tc(m.jump_ms, true) })}`;
    $('#why').innerHTML = m.channels.map((h) => `<span class="ch ${h.channel}"><span>${_('rank.b', { ch: CH[h.channel], n: h.rank })}</span></span>`).join('');
    $('#quote').hidden = !m.snippet;
    $('#quote').textContent = m.snippet ? `“${m.snippet}”` : '';
    $('#pos').textContent = _('s.pos', { n: i + 1 });
    $('#open').href = `video.html?id=${m.video_id}&t=${m.jump_ms}`;
    let entry;
    try { entry = await load(m.video_id); } catch (err) { loaded.delete(m.video_id); return fail(err); }
    if (st.picked !== i) return;
    if (st.vid !== m.video_id) { st.vid = m.video_id; vid.src = entry.url; buildVideo(entry.v); }
    plot.querySelectorAll('.hit').forEach((h) => h.classList.toggle('on', +h.dataset.i === i));
    if (jump) seek(m.jump_ms);
  }
  const step = (d) => select((st.picked + d + ms.length) % ms.length);
  list.addEventListener('click', (e) => { const b = e.target.closest('.result'); if (b) select(+b.dataset.i); });
  list.addEventListener('keydown', (e) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    e.preventDefault();
    const i = (st.picked + (e.key === 'ArrowDown' ? 1 : ms.length - 1)) % ms.length;
    select(i); buttons[i].focus();
  });
  $('#prev').addEventListener('click', () => step(-1));
  $('#next').addEventListener('click', () => step(1));
  const toggle = () => (vid.paused ? vid.play().catch(fail) : vid.pause());
  document.addEventListener('keydown', (e) => {
    if (e.target.closest('input, select, textarea') || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'j') step(1); else if (e.key === 'k') step(-1);
    else if (e.key === ' ' && !e.target.closest('button, a')) { e.preventDefault(); toggle(); }
  });
  // the playhead follows the real video: every frame while playing, once after each seek
  const follow = () => { setT(vid.currentTime * 1000); if (!vid.paused) requestAnimationFrame(follow); };
  const showPlaying = () => {
    playBtn.setAttribute('aria-pressed', String(!vid.paused));
    playBtn.setAttribute('aria-label', vid.paused ? _('play') : _('pause'));
    playBtn.innerHTML = vid.paused ? icon.play : icon.pause;
  };
  vid.addEventListener('play', () => { showPlaying(); requestAnimationFrame(follow); });
  vid.addEventListener('pause', showPlaying);
  vid.addEventListener('seeked', follow);
  playBtn.addEventListener('click', toggle);
  // seeking: a click or drag on the lanes moves the playhead; landing inside a hit selects that moment
  const seekAt = (e) => {
    const r = plot.getBoundingClientRect(), t = (Math.max(0, Math.min(r.width, e.clientX - r.left)) / r.width) * st.dur;
    seek(t);
    const hit = ms.findIndex((x) => x.video_id === st.vid && t >= x.start_ms && t < x.end_ms);
    if (hit >= 0 && hit !== st.picked) select(hit, false);
  };
  plot.addEventListener('pointerdown', (e) => { plot.setPointerCapture(e.pointerId); seekAt(e); });
  plot.addEventListener('pointermove', (e) => { if (plot.hasPointerCapture(e.pointerId)) seekAt(e); });
  plot.addEventListener('keydown', (e) => {
    const d = { ArrowLeft: -5000, ArrowRight: 5000, PageDown: -60000, PageUp: 60000 }[e.key];
    const t = d ? vid.currentTime * 1000 + d : e.key === 'Home' ? 0 : e.key === 'End' ? st.dur : null;
    if (t != null) { e.preventDefault(); seek(Math.max(0, Math.min(st.dur, t))); }
  });
  select(0);
}

// ---------- library ----------
async function libraryPage() {
  const status = params.get('status') || '', project = params.get('project') || '';
  const projects = await shell('library');
  const filter = (extra) => `library.html?${new URLSearchParams({ ...(project && { project }), ...extra })}`;
  $('#tabs').innerHTML = [['', _('tab.all')], ...['indexing', 'ready', 'failed', 'uploading'].map((s) => [s, _(`st.${s}`)])]
    .map(([s, label]) => `<a href="${filter(s ? { status: s } : {})}" role="tab" aria-selected="${s === status}">${label}</a>`).join('');
  const sel = $('#proj');
  sel.insertAdjacentHTML('beforeend', projects.map((p) => `<option value="${p.id}">${esc(p.name)}</option>`).join(''));
  sel.value = project;
  sel.addEventListener('change', () => { location.search = new URLSearchParams({ ...(status && { status }), ...(sel.value && { project: sel.value }) }); });
  $('#newproj').addEventListener('click', newProject);

  let rows = [], cursor = null, timer = 0;
  const query = (extra) => new URLSearchParams({ limit: '50', ...(status && { status }), ...(project && { project_id: project }), ...extra });
  function drawRows() {
    $('#total').textContent = _('l.total', { n: rows.length + (cursor ? '+' : '') });
    $('#more').hidden = !cursor;
    $('#rows').innerHTML = rows.length ? rows.map((v) => `<tr><td class="vidc"><a class="vid" href="video.html?id=${v.id}">${thumb(v)}<span><span class="t">${esc(v.title)}</span><span class="sub">${tracks(v)}</span></span></a></td>
        <td>${pill(v.status)}${v.status === 'failed' ? `<span class="sub">${esc(v.error || '')}<button class="act" type="button" data-retry="${v.id}">${_('retry')}</button></span>` : ''}</td>
        <td class="n num">${v.duration_ms ? tc(v.duration_ms) : '<span style="color:var(--faint)">—</span>'}</td>
        <td class="n num opt">${when(v.created_at)}</td></tr>`).join('')
      : `<tr><td colspan="4"><div class="empty">${_('l.empty')}</div></td></tr>`;
  }
  async function more() {
    const r = await get(`/v1/videos?${query(cursor ? { cursor } : {})}`);
    rows = rows.concat(r.videos); cursor = r.next_cursor; drawRows();
  }
  // re-reads the first page and the attention panel; repeats while anything on screen is still processing
  async function refresh() {
    clearTimeout(timer);
    const [first, failed, indexing] = await Promise.all([get(`/v1/videos?${query({})}`), get('/v1/videos?status=failed&limit=5'), get('/v1/videos?status=indexing&limit=5')]);
    rows = first.videos; cursor = first.next_cursor; drawRows();
    $('#sub').textContent = _('l.sub2', { i: indexing.videos.length + (indexing.next_cursor ? '+' : ''), f: failed.videos.length + (failed.next_cursor ? '+' : '') });
    const attention = failed.videos.concat(indexing.videos);
    $('#attention').innerHTML = attention.length ? attention.map((v) => `<div class="row"><a class="vid" href="video.html?id=${v.id}" style="flex:1;min-width:0">${thumb(v)}<span style="min-width:0"><span class="t" style="display:block">${esc(v.title)}</span><span class="meta" style="color:var(--muted);font-size:13px">${esc(v.status === 'failed' ? v.error || _('st.failed') : tracks(v))}</span></span></a>
        ${v.status === 'failed' ? `<button class="btn" type="button" data-retry="${v.id}">${_('retry')}</button>` : pill(v.status)}</div>`).join('')
      : `<div class="empty">${_('att.none')}</div>`;
    if (rows.concat(attention).some((v) => busy(v.status))) timer = setTimeout(() => refresh().catch(fail), 5000);
  }
  $('#more').addEventListener('click', () => more().catch(fail));
  document.addEventListener('click', async (e) => {
    const b = e.target.closest('[data-retry]');
    if (!b) return;
    b.disabled = true;
    try { await post(`/v1/videos/${b.dataset.retry}/retry`); await refresh(); } catch (err) { b.disabled = false; fail(err); }
  });

  // uploads run in this page: parts go straight to storage, then complete hands the video to the pipeline
  const ups = [];
  const drawUploads = () => {
    $('#uploads').innerHTML = ups.length ? ups.map((u) => {
      const pct = u.total ? u.done / u.total : 0;
      const right = u.state === 'done' ? _('l.submitted') : u.state === 'fail' ? _('u.failed', { e: esc(u.error) }) : `${Math.round(pct * 100)}%`;
      return `<div class="upload-row"><span class="name">${esc(u.name)}</span><span class="n">${right}</span>
        <div class="bar"><span>${_('parts', { a: u.done, b: u.total || '…' })}</span><span class="track${u.state === 'done' ? ' done' : ''}"><i style="transform:scaleX(${pct.toFixed(3)})"></i></span><span>${(u.size / 1048576).toFixed(0)} MB</span></div></div>`;
    }).join('') : `<div class="empty">${_('u.none')}</div>`;
  };
  async function upload(file) {
    const u = { name: file.name, size: file.size, done: 0, total: 0, state: 'run' };
    ups.unshift(u); drawUploads();
    try {
      const { video, upload: plan } = await post('/v1/uploads', { filename: file.name, size_bytes: file.size, content_type: file.type || 'application/octet-stream', project_id: project || undefined });
      u.total = plan.parts.length; drawUploads();
      const parts = [];
      await eachLimit(plan.parts, 4, async (p) => {
        const [a, b] = partRange(p.n, plan.part_size, file.size);
        const res = await fetch(p.url, { method: 'PUT', body: file.slice(a, b) });
        const etag = res.headers.get('ETag');
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        if (!etag) throw new Error(_('u.noetag'));
        parts.push({ n: p.n, etag }); u.done++; drawUploads();
      });
      await post(`/v1/videos/${video.id}/complete`, { upload_id: plan.upload_id, parts: parts.sort((x, y) => x.n - y.n) });
      u.state = 'done';
    } catch (err) {
      u.state = 'fail'; u.error = err.code === 'network' ? _('err.network', { base: settings().base }) : err.message;
    }
    drawUploads();
    if (u.state === 'done') refresh().catch(fail);
  }
  $('#upload').addEventListener('click', () => $('#file').click());
  $('#file').addEventListener('change', () => { [...$('#file').files].forEach(upload); $('#file').value = ''; });
  // closing the page stops the parts in flight
  window.addEventListener('beforeunload', (e) => { if (ups.some((u) => u.state === 'run')) e.preventDefault(); });
  drawUploads();
  await refresh();
}

// ---------- detail ----------
async function videoPage() {
  const id = params.get('id');
  const projects = await shell('library');
  if (!/^[1-9]\d*$/.test(id || '')) return fail({ message: _('v.missing') });
  let timer = 0, playing = false, shown = '', v;
  const cell = (s) => {
    const label = s.status === 'running' ? (s.progress != null ? _('run.pct', { n: Math.round(s.progress * 100) }) : _('run'))
      : s.status === 'done' ? _('done.at', { t: when(s.updated_at) }) : s.status === 'failed' ? _('fail.n', { n: s.attempt })
        : s.status === 'queued' ? _('queued') : stageKind(s.stage) === 'speech' ? _('pend.asr') : _('pend.all');
    const m = s.status === 'done' ? icon.check : s.status === 'failed' ? icon.x : '';
    return `<div class="st ${s.status === 'queued' ? 'pending' : s.status}"><span class="h"><span class="mark ${s.status === 'queued' ? 'pending' : s.status}">${m}</span>${esc(stageLabel(s))}</span><span class="k">${esc(s.stage)}</span><span class="s">${label}</span>
      ${s.progress != null && s.status === 'running' ? `<span class="track" style="margin-top:6px"><i style="transform:scaleX(${s.progress})"></i></span>` : ''}
      ${s.error ? `<span class="s">${esc(s.error)}</span>` : ''}</div>`;
  };
  function render(stages) {
    const failed = stages.find((s) => s.status === 'failed'), retryable = v.status === 'failed' || (v.status === 'ready' && failed);
    // the header holds controls, so polling redraws it only when what it shows changes
    const head = [v.status, v.title, v.duration_ms, v.has_audio, !!v.thumbnail, !!retryable].join('|');
    if (head !== shown) {
      shown = head;
      $('#objhead').innerHTML = `${thumb(v)}<div style="min-width:0"><h1>${esc(v.title)} ${pill(v.status)}</h1>
      <div class="meta">${v.duration_ms ? `${tc(v.duration_ms)} · ` : ''}${_('v.upload')} · ${tracks(v)} · ${_('v.added', { w: when(v.created_at) })}</div></div>
      <div class="actions">${retryable ? `<button class="btn primary" id="retry" type="button">${_('retry')}</button>` : ''}
        <label style="position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)" for="addp">${_('v.addproject')}</label>
        <select class="input" id="addp"${projects.length ? '' : ' hidden'}><option value="">${_('v.addproject')}</option>${projects.map((p) => `<option value="${p.id}">${esc(p.name)}</option>`).join('')}</select>
        <button class="btn danger" id="del" type="button">${_('v.delete')}</button></div>`;
    }
    const done = stages.filter((s) => s.status === 'done').length, p = pipeline(stages);
    $('#pfinding').textContent = v.status === 'ready' ? _('v.find.ready')
      : failed ? _('v.find.failed2', { s: stageLabel(failed), n: failed.attempt, e: failed.error || v.error || '' })
        : v.status === 'uploading' ? _('v.find.uploading') : _('v.find.indexing2', { a: done, b: stages.length });
    const latest = stages.reduce((t, s) => (s.updated_at > t ? s.updated_at : t), v.updated_at);
    $('#pdone').textContent = `${_('v.done', { a: done, b: stages.length })} · ${_('v.updated', { t: when(latest) })}`;
    $('#pipe').innerHTML = `${p.before.map(cell).join('')}
      <div class="par">${p.lanes.map(([, l]) => `<div class="lane-row${l.length === 1 ? ' solo' : ''}">${l.map(cell).join('')}</div>`).join('')}</div>
      ${p.after.map(cell).join('')}`;
    const can = channelState(stages);
    $('#can').innerHTML = CHANNELS.map((c) => {
      const [text, tone] = c !== 'visual' && v.has_audio === false ? [_('can.noaudio'), ''] : { done: [_('can.yes'), 'pos'], failed: [_('can.failed'), 'neg'], none: ['—', ''] }[can[c]] || [_('can.wait'), ''];
      return `<div><span class="ch ${c}" style="font-size:14px;color:var(--text)">${CH[c]}</span><span class="pill ${tone}">${text}</span></div>`;
    }).join('');
    $('#kv').innerHTML = [[_('kv.dur'), v.duration_ms ? tc(v.duration_ms) : '—'], [_('kv.src'), _('v.upload')], [_('kv.tracks'), tracks(v)], [_('kv.added'), when(v.created_at)]]
      .map(([k, val]) => `<div><dt>${k}</dt><dd>${esc(val)}</dd></div>`).join('');
  }
  // the player opens once transcoding has produced the normalized video, which is when a poster exists
  async function openPlayer() {
    if (playing || !(v.thumbnail || v.status === 'ready')) return;
    playing = true;
    try {
      const { url } = await get(`/v1/videos/${id}/playback`);
      $('#frame').innerHTML = `<video id="vid" controls playsinline preload="metadata" src="${esc(url)}"></video>`;
      if (params.get('t')) $('#vid').addEventListener('loadedmetadata', (e) => { e.target.currentTime = +params.get('t') / 1000; }, { once: true });
    } catch (err) { playing = false; fail(err); }
  }
  async function load() {
    clearTimeout(timer);
    const data = await get(`/v1/videos/${id}`);
    v = data.video;
    render(data.stages);
    openPlayer();
    if (busy(v.status)) timer = setTimeout(() => load().catch(fail), 3000);
  }
  $('#frame').innerHTML = `<p class="empty" style="position:absolute;inset:0;border:0">${_('v.noplay')}</p>`;
  $('#objhead').addEventListener('click', async (e) => {
    if (e.target.id === 'retry') {
      e.target.disabled = true;
      try { await post(`/v1/videos/${id}/retry`); await load(); } catch (err) { e.target.disabled = false; fail(err); }
    } else if (e.target.id === 'del' && confirm(_('v.confirmdelete', { t: v.title }))) {
      try { await call('DELETE', `/v1/videos/${id}`); location.href = 'library.html'; } catch (err) { fail(err); }
    }
  });
  $('#objhead').addEventListener('change', async (e) => {
    if (e.target.id !== 'addp' || !e.target.value) return;
    const p = projects.find((x) => x.id === e.target.value);
    try { await call('PUT', `/v1/projects/${p.id}/videos/${id}`); $('#pdone').textContent = _('v.added2', { p: p.name }); } catch (err) { fail(err); }
    e.target.value = '';
  });
  $('#vsearch').addEventListener('submit', async (e) => {
    e.preventDefault();
    const q = $('#vq').value.trim();
    if (!q) return;
    try {
      const { moments } = await post('/v1/search', { query: q, video_ids: [id], limit: 20 });
      $('#vres').innerHTML = moments.length ? moments.map((m) => `<li><button class="result" type="button" data-t="${m.jump_ms}">
          ${thumb({ title: v.title, thumbnail: m.thumbnail }, `<span class="tc">${tc(m.jump_ms)}</span>`)}
          <span class="txt"><span class="r">${tc(m.start_ms, true)} – ${tc(m.end_ms, true)}</span>
          <span class="chs">${m.channels.map((h) => `<span class="ch ${h.channel}">${_('rank', { ch: CH[h.channel], n: h.rank })}</span>`).join('')}</span>
          ${m.snippet ? `<span class="r">“${esc(m.snippet)}”</span>` : ''}</span></button></li>`).join('')
        : `<li><div class="empty">${_('vs.none')}</div></li>`;
    } catch (err) { fail(err); }
  });
  $('#vres').addEventListener('click', (e) => {
    const b = e.target.closest('[data-t]'), vid = $('#vid');
    if (!b || !vid) return;
    vid.currentTime = +b.dataset.t / 1000;
    vid.play().catch(() => {});
  });
  await load();
}

// ---------- sign-in ----------
function signinPage() {
  const s = settings(), state = $('#state');
  $('#base').value = s.base || 'http://127.0.0.1:18000';
  $('#out').addEventListener('click', () => { signOut(); $('#token').value = ''; state.textContent = _('si.out'); });
  $('#si').addEventListener('submit', async (e) => {
    e.preventDefault();
    $('#notice').hidden = true;
    const next = { base: $('#base').value.trim(), token: $('#token').value.trim() };
    if (!/^https?:\/\/[^/?#]+/.test(next.base)) return fail({ message: _('si.badbase') });
    try {
      await call('GET', '/v1/me', undefined, next);
      saveSettings(next);
      location.href = safeNext(params.get('next'));
    } catch (err) {
      fail(err.status === 401 ? { message: _('si.bad') } : err.code === 'network' ? { message: _('err.network', { base: next.base }) } : err);
    }
  });
}

// a language switch re-renders the whole screen from the API, in the other language
window.I18N.on(() => location.reload());
window.I18N.apply();
({ search: searchPage, library: libraryPage, video: videoPage, signin: signinPage })[PAGE]()?.catch?.(fail);
