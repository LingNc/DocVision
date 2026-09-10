'use strict';
/*
 * DocVision session viewer.
 *
 * One script serves both modes. When the page carries a #dsh-data block the
 * snapshot is complete and nothing is polled; otherwise the page is talking to
 * the local read-only server and refreshes itself every couple of seconds.
 *
 * Transcript content is always inserted as text (never as HTML), so a tool
 * result containing markup stays visible instead of becoming markup.
 *
 * Three things the transcript now lets the page show honestly: the newest
 * t="meta" line (what the model was told: system prompt + tool definitions),
 * the project a session belongs to (the sidebar groups sessions by it), and a
 * light/dark theme that defaults to daylight.
 */
(function () {
  var POLL_MS = 2000;
  var PREVIEW_LINES = 8;
  var LONG_TEXT_LINES = 20;
  var STORE_PREFIX = 'dsh.sessionview.';
  var THEME_KEY = 'theme';
  var DEFAULT_THEME = 'light';
  var ROOT_PROJECT = '（根目录）';

  var dataEl = document.getElementById('dsh-data');
  var DATA = null;
  if (dataEl) {
    try { DATA = JSON.parse(dataEl.textContent); } catch (err) { DATA = null; }
  }
  var MODE = DATA ? 'static' : 'live';

  var refs = {
    refresh: document.getElementById('refresh'),
    rootPath: document.getElementById('root-path'),
    search: document.getElementById('search'),
    list: document.getElementById('session-list'),
    foot: document.getElementById('side-foot'),
    title: document.getElementById('session-title'),
    sessionBadge: document.getElementById('session-badge'),
    sub: document.getElementById('session-sub'),
    badge: document.getElementById('mode-badge'),
    follow: document.getElementById('follow'),
    collapseThinking: document.getElementById('collapse-thinking'),
    onlyTools: document.getElementById('only-tools'),
    themeToggle: document.getElementById('theme-toggle'),
    banner: document.getElementById('banner'),
    timeline: document.getElementById('timeline'),
    lightbox: document.getElementById('lightbox'),
    lightboxImg: document.getElementById('lightbox-img')
  };

  var state = {
    root: '',
    generated: '',
    sessions: [],
    current: null,
    lines: [],
    nextFrom: 0,
    curSize: -1,
    curMtime: 0,
    filter: '',
    follow: MODE === 'live',
    forceCollapse: false,
    onlyTools: false,
    msgSeq: 0,
    toolSeq: 0,
    calls: new Map(),
    badLines: 0,
    polling: false,
    theme: DEFAULT_THEME,
    // 项目折叠状态只存在内存里：刷新（重新扫描）后保持，页面重开回到"全部展开"。
    collapsed: {},
    meta: null
  };

  /* ---------- small helpers ---------- */

  function el(tag, cls, text) {
    var node = document.createElement(tag);
    if (cls) { node.className = cls; }
    if (text !== undefined && text !== null) { node.textContent = text; }
    return node;
  }

  function fmtSize(bytes) {
    if (!bytes && bytes !== 0) { return '—'; }
    if (bytes < 1024) { return bytes + ' B'; }
    if (bytes < 1024 * 1024) { return (bytes / 1024).toFixed(1) + ' KB'; }
    return (bytes / 1024 / 1024).toFixed(2) + ' MB';
  }

  function fmtClock(value) {
    var d = new Date(value);
    if (isNaN(d.getTime())) { return '—'; }
    var pad = function (n) { return (n < 10 ? '0' : '') + n; };
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' +
      pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
  }

  function relTime(value) {
    var t = new Date(value).getTime();
    if (isNaN(t)) { return '—'; }
    var secs = Math.round((Date.now() - t) / 1000);
    if (secs < 5) { return '刚刚'; }
    if (secs < 60) { return secs + ' 秒前'; }
    if (secs < 3600) { return Math.floor(secs / 60) + ' 分钟前'; }
    if (secs < 86400) { return Math.floor(secs / 3600) + ' 小时前'; }
    return Math.floor(secs / 86400) + ' 天前';
  }

  function joinPath() {
    var parts = [];
    for (var i = 0; i < arguments.length; i++) {
      var p = String(arguments[i] || '').replace(/^\/+|\/+$/g, '');
      if (p) { parts.push(p); }
    }
    return parts.join('/');
  }

  function sessionDir(id) {
    var i = String(id || '').lastIndexOf('/');
    return i < 0 ? '' : String(id).slice(0, i);
  }

  // 项目 = 相对路径的第一段；根目录直挂的转录归到"（根目录）"。
  function projectOf(id) {
    var i = String(id || '').indexOf('/');
    return i <= 0 ? ROOT_PROJECT : String(id).slice(0, i);
  }

  // 项目内的子目录（work/sessions 之类），让人一眼看出这个会话是什么阶段落的盘。
  function subPathOf(id) {
    var parts = String(id || '').split('/');
    parts.pop();
    parts.shift();
    return parts.join('/');
  }

  function mediaURL(ref) {
    var tail = String(ref || '').replace(/^file:\/\//, '');
    var rel = joinPath(sessionDir(state.current ? state.current.id : ''), tail);
    if (MODE === 'static') { return (DATA.mediaRoot || '') + rel; }
    return '/media/' + rel;
  }

  function storeGet(key) {
    try { return window.localStorage.getItem(STORE_PREFIX + key); } catch (err) { return null; }
  }

  function storeSet(key, value) {
    try { window.localStorage.setItem(STORE_PREFIX + key, value); } catch (err) { /* file:// may refuse */ }
  }

  function clear(node) {
    while (node.firstChild) { node.removeChild(node.firstChild); }
  }

  function shortSHA(value) {
    var s = String(value || '');
    if (!s) { return '—'; }
    return s.length > 12 ? s.slice(0, 12) : s;
  }

  /*
   * The wire format keeps the provider's own field name (reasoning_content);
   * normalising it once here means the render code reads one spelling only.
   */
  function normalizeLine(line) {
    if (line && line.reasoning === undefined && line.reasoning_content !== undefined) {
      line.reasoning = line.reasoning_content;
    }
    return line;
  }

  function normalizeLines(lines) {
    return (lines || []).map(normalizeLine);
  }

  /* ---------- theme ---------- */

  function applyTheme(theme) {
    state.theme = theme === 'dark' ? 'dark' : DEFAULT_THEME;
    document.documentElement.setAttribute('data-theme', state.theme);
    if (refs.themeToggle) {
      // 按钮上写的是"点一下会变成什么"，不是当前状态。
      refs.themeToggle.textContent = state.theme === 'dark' ? '☀️ 浅色' : '🌙 深色';
      refs.themeToggle.title = state.theme === 'dark' ? '切换为白天模式（浅色，默认）' : '切换为夜间模式（深色）';
      refs.themeToggle.setAttribute('aria-pressed', state.theme === 'dark' ? 'true' : 'false');
    }
  }

  function storedTheme() {
    var saved = storeGet(THEME_KEY);
    return saved === 'dark' || saved === 'light' ? saved : DEFAULT_THEME;
  }

  function toggleTheme() {
    var next = state.theme === 'dark' ? 'light' : 'dark';
    applyTheme(next);
    storeSet(THEME_KEY, next);
  }

  /* ---------- sidebar ---------- */

  function sessionHaystack(s) {
    return [s.title, s.label, s.id, s.name, s.project].join(' ').toLowerCase();
  }

  function metaOf(session) {
    return (session && session.meta && session.meta.count) ? session.meta : null;
  }

  /*
   * 侧栏按项目分组：每个项目是一个可折叠集合（标题 = 项目名 + 会话数）。
   * 过滤时只保留有命中的组并强制展开，命中信息写进组的摘要行。
   */
  function buildGroups() {
    var q = state.filter;
    var groups = [];
    var byName = {};
    state.sessions.forEach(function (s) {
      var name = s.project || projectOf(s.id);
      var g = byName[name];
      if (!g) {
        g = byName[name] = { name: name, items: [], live: 0, matched: false };
        groups.push(g);
      }
      if (q && sessionHaystack(s).indexOf(q) < 0) { return; }
      g.items.push(s);
      if (s.live) { g.live++; }
    });
    if (q) {
      groups = groups.filter(function (g) { return g.items.length > 0; });
      groups.forEach(function (g) { g.matched = true; });
    }
    return groups;
  }

  function sessionRow(s) {
    var row = el('button', 'session-row');
    row.type = 'button';
    if (state.current && state.current.id === s.id) { row.classList.add('active'); }

    var top = el('div', 'row-top');
    top.appendChild(el('span', 'dot' + (s.live ? ' live' : '')));
    top.appendChild(el('span', 'row-title', s.title || s.label || s.name));
    top.appendChild(el('span', 'badge', s.label || ''));
    row.appendChild(top);

    var meta = el('div', 'row-meta');
    meta.appendChild(el('span', null, s.messages + ' 条消息 · ' + fmtSize(s.size) + ' · ' + relTime(s.mtime)));
    var m = metaOf(s);
    if (m) {
      var chip = el('span', 'row-chip meta-chip', '含系统提示词快照');
      chip.title = '这个转录有 ' + m.count + ' 条 t=meta 元信息行，最新一条 ' +
        (m.promptChars || 0) + ' 字符' + (m.model ? '（模型 ' + m.model + '）' : '') +
        ((m.tools) ? '，含 ' + m.tools + ' 个工具定义' : '');
      meta.appendChild(chip);
    }
    var sub = subPathOf(s.id);
    if (sub) {
      var subChip = el('span', 'row-chip row-sub', sub);
      subChip.title = s.id;
      meta.appendChild(subChip);
    }
    row.appendChild(meta);
    row.addEventListener('click', function () { selectSession(s.id); });
    return row;
  }

  function renderSessions() {
    clear(refs.list);
    var groups = buildGroups();
    var shown = 0;
    groups.forEach(function (g) {
      var wrap = document.createElement('details');
      wrap.className = 'proj-group';
      // 默认全部展开；过滤命中的组强制展开，其余按记忆的折叠状态恢复。
      wrap.open = g.matched ? true : !state.collapsed[g.name];

      var head = el('summary', 'proj-head');
      head.appendChild(el('span', 'proj-name', g.name));
      head.appendChild(el('span', 'proj-count', g.items.length + ' 个会话'));
      if (g.live) { head.appendChild(el('span', 'dot live')); }
      if (g.matched) { head.appendChild(el('span', 'proj-note', '过滤命中')); }
      wrap.appendChild(head);

      var body = el('div', 'proj-body');
      g.items.forEach(function (s) { body.appendChild(sessionRow(s)); shown++; });
      wrap.appendChild(body);

      wrap.addEventListener('toggle', function () {
        if (g.matched) { return; } // 过滤强制展开时不要把状态写回记忆
        state.collapsed[g.name] = !wrap.open;
      });
      refs.list.appendChild(wrap);
    });
    if (!shown) {
      refs.list.appendChild(el('div', 'empty',
        state.sessions.length ? '没有匹配的会话' : '没有找到 *.jsonl 会话转录'));
    }
    var live = state.sessions.filter(function (s) { return s.live; }).length;
    refs.foot.textContent = groups.length + ' 个项目 · ' + state.sessions.length + ' 个会话' +
      (live ? ' · ' + live + ' 个活跃' : '') + (state.filter ? ' · 匹配 ' + shown : '');
  }

  function updateRootLabel() {
    refs.rootPath.textContent = state.root || '—';
  }

  /* ---------- toolbar ---------- */

  function renderToolbar() {
    if (!state.current) {
      refs.title.textContent = '未选择会话';
      refs.sub.textContent = '';
      if (refs.sessionBadge) {
        refs.sessionBadge.textContent = '';
        refs.sessionBadge.classList.add('hidden');
      }
      return;
    }
    refs.title.textContent = state.current.title || state.current.label || state.current.name;
    if (refs.sessionBadge) {
      var label = state.current.label || '';
      refs.sessionBadge.textContent = label;
      refs.sessionBadge.classList.toggle('hidden', !label);
    }
    var parts = [state.current.messages + ' 条消息', state.lines.length + ' 行', fmtSize(state.current.size)];
    var m = metaState();
    if (m) {
      parts.push('系统提示词 ' + m.promptChars + ' 字符' +
        (m.count > 1 ? '（共 ' + m.count + ' 条 meta，显示最新）' : '') +
        (m.line.tools && m.line.tools.length ? ' · 工具定义 ' + m.line.tools.length : ''));
    }
    if (state.badLines) { parts.push('坏行 ' + state.badLines); }
    if (state.current.mtime) { parts.push('最后写入 ' + fmtClock(state.current.mtime)); }
    refs.sub.textContent = parts.join(' · ');
  }

  function renderBadge() {
    if (MODE === 'static') {
      refs.badge.textContent = '静态快照 · 生成于 ' + fmtClock(state.generated || DATA.generated);
      refs.badge.className = 'badge';
    } else {
      refs.badge.textContent = state.polling ? '实时' : '实时（已断开）';
      refs.badge.className = 'badge badge-live';
    }
  }

  function setBanner(text) {
    if (!text) {
      refs.banner.classList.add('hidden');
      refs.banner.textContent = '';
      return;
    }
    refs.banner.textContent = text;
    refs.banner.classList.remove('hidden');
  }

  function updateBanner() {
    if (state.badLines > 0) {
      setBanner('已跳过 ' + state.badLines + ' 行坏数据（无法解析为 JSON，可能是一次写入中途读到的不完整行）');
    } else {
      setBanner('');
    }
  }

  /* ---------- meta line (系统提示词快照) ---------- */

  function metaLines() {
    var found = [];
    state.lines.forEach(function (l) { if (l.t === 'meta') { found.push(l); } });
    return found;
  }

  /*
   * 一个转录可能有多条 meta 行（同一会话多次运行、提示词变化时会追加），
   * 只显示最后一条，并注明总条数。总条数取扫描结果与已加载行的较大值：
   * 扫描是权威的，增量加载时也不会因为只拉到一部分而少报。
   */
  function metaState() {
    var metas = metaLines();
    if (!metas.length && !metaOf(state.current)) { return null; }
    var line = metas.length ? metas[metas.length - 1] : null;
    if (!line) { return null; }
    var scanned = metaOf(state.current);
    return {
      line: line,
      count: Math.max(metas.length, scanned ? scanned.count : 0),
      promptChars: String(line.text || '').length
    };
  }

  /* Pretty-print a JSON blob by walking text nodes, never by building HTML. */
  function prettyJSON(raw) {
    if (typeof raw !== 'string' || !raw.trim()) { return ''; }
    try { return JSON.stringify(JSON.parse(raw), null, 2); } catch (err) { return raw; }
  }

  function appendHighlighted(parent, text) {
    var re = /("(?:\\.|[^"\\])*")\s*:|("(?:\\.|[^"\\])*")|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?)/g;
    var last = 0;
    var m;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) { parent.appendChild(document.createTextNode(text.slice(last, m.index))); }
      var cls = m[1] !== undefined ? 'key' : m[2] !== undefined ? 'str' : m[3] !== undefined ? 'bool' : 'num';
      parent.appendChild(el('span', cls, m[0]));
      last = m.index + m[0].length;
    }
    if (last < text.length) { parent.appendChild(document.createTextNode(text.slice(last))); }
  }

  function copyButton(text) {
    var btn = el('button', 'text-toggle', '复制');
    btn.type = 'button';
    btn.addEventListener('click', function () {
      var done = function () { btn.textContent = '已复制'; setTimeout(function () { btn.textContent = '复制'; }, 1200); };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, function () { fallbackCopy(text) && done(); });
      } else if (fallbackCopy(text)) {
        done();
      }
    });
    return btn;
  }

  function fallbackCopy(text) {
    try {
      var area = document.createElement('textarea');
      area.value = text;
      area.style.position = 'fixed';
      area.style.opacity = '0';
      document.body.appendChild(area);
      area.select();
      var ok = document.execCommand('copy');
      document.body.removeChild(area);
      return ok;
    } catch (err) {
      return false;
    }
  }

  /* 工具定义：名字 + 描述；parameters 的 JSON 收在二级折叠里（默认收起）。 */
  function toolSchemaBlock(tool, index) {
    var d = document.createElement('details');
    d.className = 'tool-schema';
    var head = el('summary', 'schema-head');
    head.appendChild(el('span', 'schema-index', '#' + (index + 1)));
    head.appendChild(el('span', 'schema-name', tool.name || '(未命名工具)'));
    if (tool.description) {
      head.appendChild(el('span', 'schema-meta', '描述 ' + String(tool.description).length + ' 字符'));
    }
    if (tool.parameters) {
      head.appendChild(el('span', 'schema-meta', 'schema ' + String(tool.parameters).length + ' 字符'));
    }
    d.appendChild(head);

    var body = el('div', 'schema-body');
    if (tool.description) {
      body.appendChild(el('pre', 'body-text schema-desc', tool.description));
    }
    if (tool.parameters) {
      var pd = document.createElement('details');
      pd.className = 'schema-params';
      var ph = el('summary', 'schema-head');
      ph.appendChild(el('span', 'schema-name', 'parameters'));
      ph.appendChild(el('span', 'schema-meta', 'JSON · 默认收起'));
      pd.appendChild(ph);
      var pbody = el('div', 'schema-body');
      var pre = el('pre', 'code');
      appendHighlighted(pre, prettyJSON(tool.parameters));
      pbody.appendChild(pre);
      var actions = el('div', 'row-actions');
      actions.appendChild(copyButton(String(tool.parameters)));
      pbody.appendChild(actions);
      pd.appendChild(pbody);
      body.appendChild(pd);
    } else {
      body.appendChild(el('div', 'note', '（这条工具定义没有记录 parameters）'));
    }
    d.appendChild(body);
    return d;
  }

  function metaCardKey(line) {
    return 'meta.' + (state.current ? state.current.id : '') + '.' + (line.system_sha || line.n);
  }

  function metaCard() {
    var m = metaState();
    if (!m) { state.meta = null; return null; }
    state.meta = m;
    var line = m.line;

    var card = document.createElement('details');
    card.className = 'block meta-card';
    // 默认**收起**：摘要行已经写明模型/会话/哈希/字符数，一次点击才展开
    // 全文——系统提示词动辄几万字符，默认展开会把会话时间线整个顶下去。
    // 展开状态按会话 + 提示词哈希记忆（用户展开过一次后保持）。
    card.open = storeGet(metaCardKey(line)) === '1';

    var head = el('summary', 'block-head');
    head.appendChild(el('span', 'block-name', '系统提示词（本次运行快照，不参与回放）'));
    head.appendChild(el('span', 'block-meta', '模型 ' + (line.model || '—')));
    if (line.session_label) { head.appendChild(el('span', 'block-meta', '会话 ' + line.session_label)); }
    head.appendChild(el('span', 'block-meta', 'sha ' + shortSHA(line.system_sha)));
    head.appendChild(el('span', 'block-meta', m.promptChars + ' 字符'));
    if (line.kind) { head.appendChild(el('span', 'block-meta', 'kind ' + line.kind)); }
    if (m.count > 1) {
      var note = el('span', 'block-meta', '共 ' + m.count + ' 条，显示最新');
      note.title = '同一个转录里有 ' + m.count + ' 条 meta 行（多次运行 / 提示词变化各一条），这里显示最后一条。';
      head.appendChild(note);
    }
    card.appendChild(head);

    var body = el('div', 'block-body');
    var scroll = el('div', 'prompt-scroll');
    scroll.appendChild(el('pre', 'body-text prompt-text', String(line.text || '（这条 meta 行没有正文）')));
    body.appendChild(scroll);
    var actions = el('div', 'row-actions');
    actions.appendChild(copyButton(String(line.text || '')));
    body.appendChild(actions);

    var tools = line.tools || [];
    var toolsWrap = el('div', 'meta-tools');
    toolsWrap.appendChild(el('div', 'meta-tools-head',
      tools.length ? '工具定义 ' + tools.length + ' 个（parameters 的 JSON 默认收起）' : '工具定义 0 个'));
    if (!tools.length) {
      toolsWrap.appendChild(el('div', 'note', '这条 meta 行没有记录工具定义。'));
    }
    tools.forEach(function (t, i) { toolsWrap.appendChild(toolSchemaBlock(t, i)); });
    body.appendChild(toolsWrap);
    card.appendChild(body);

    card.addEventListener('toggle', function () {
      storeSet(metaCardKey(line), card.open ? '1' : '0');
    });
    return card;
  }

  var EMPTY_TEXTS = {
    none: '左侧选择一个会话开始浏览。',
    noMessages: '这个会话还没有可显示的消息',
    onlyTools: '这个会话没有工具调用记录'
  };

  /* 卡片永远在最前面：时间线重建时插到第一个节点，增量到达时也挪到最前。 */
  function renderMetaCard() {
    var old = refs.timeline.querySelector('.meta-card');
    if (old && old.parentNode) { old.parentNode.removeChild(old); }
    var card = metaCard();
    if (!card) { return 0; }
    refs.timeline.insertBefore(card, refs.timeline.firstChild);
    return 1;
  }

  /* ---------- message rendering ---------- */

  function roleName(role) {
    if (role === 'user') { return '用户'; }
    if (role === 'assistant') { return 'AI'; }
    if (role === 'tool') { return '工具结果'; }
    if (role === 'system') { return '系统'; }
    return role || '未知';
  }

  function messageHead(line) {
    var head = el('div', 'msg-head');
    head.appendChild(el('span', 'msg-seq', '#' + (++state.msgSeq)));
    head.appendChild(el('span', 'msg-role', roleName(line.role)));
    head.appendChild(el('span', 'msg-line', '行 ' + line.n));
    if (line.reasoning) { head.appendChild(el('span', 'msg-chip', '思考 ' + line.reasoning.length + ' 字符')); }
    if (line.tool_calls && line.tool_calls.length) { head.appendChild(el('span', 'msg-chip', '工具 ' + line.tool_calls.length + ' 次')); }
    if (line.images && line.images.length) { head.appendChild(el('span', 'msg-chip', '图片 ' + line.images.length)); }
    return head;
  }

  /*
   * Long transcripts are common (a user turn can be thousands of characters),
   * so only the preview lines are put in the DOM until the reader expands it.
   */
  function collapsibleText(text, previewLines, key, extraClass) {
    var wrap = el('div', 'text-wrap');
    var lines = String(text === undefined || text === null ? '' : text).split('\n');
    var long = lines.length > previewLines;
    var pre = el('pre', 'body-text' + (extraClass ? ' ' + extraClass : ''));
    var expanded = storeGet('text.' + key) === '1';
    var paint = function () {
      pre.textContent = (expanded || !long) ? lines.join('\n') : lines.slice(0, previewLines).join('\n');
      pre.classList.toggle('clamped', long && !expanded);
    };
    paint();
    wrap.appendChild(pre);
    if (long) {
      var toggle = el('button', 'text-toggle');
      var label = function () {
        toggle.textContent = expanded
          ? '收起'
          : '展开全文（' + lines.length + ' 行 / ' + String(text).length + ' 字符）';
      };
      label();
      toggle.type = 'button';
      toggle.addEventListener('click', function () {
        expanded = !expanded;
        storeSet('text.' + key, expanded ? '1' : '0');
        paint();
        label();
      });
      wrap.appendChild(toggle);
    }
    return wrap;
  }

  function imageStrip(line) {
    var strip = el('div', 'images');
    (line.images || []).forEach(function (ref) {
      var url = mediaURL(ref);
      var img = el('img', 'thumb');
      img.src = url;
      img.alt = ref;
      img.loading = 'lazy';
      img.title = ref;
      img.addEventListener('click', function () { openLightbox(url, ref); });
      strip.appendChild(img);
    });
    return strip;
  }

  /*
   * 思考过程默认折叠；展开后是等宽、低对比、保留原始换行的整段文本，
   * 长思考靠容器自身的 max-height + overflow 滚动，不撑爆页面。
   */
  function reasoningBlock(line) {
    var details = document.createElement('details');
    details.className = 'block thinking';
    var remembered = storeGet('thinking.' + state.current.id + '.' + line.n) === '1';
    details.open = !state.forceCollapse && remembered;
    var head = el('summary', 'block-head');
    head.appendChild(el('span', 'block-name', '思考过程'));
    head.appendChild(el('span', 'block-meta', '· ' + line.reasoning.length + ' 字符'));
    var rows = line.reasoning.split('\n').length;
    if (rows > 1) { head.appendChild(el('span', 'block-meta', rows + ' 行')); }
    details.appendChild(head);
    var body = el('div', 'block-body');
    var scroll = el('div', 'reasoning-scroll');
    scroll.appendChild(el('pre', 'body-text reasoning-text', line.reasoning));
    body.appendChild(scroll);
    details.appendChild(body);
    details.addEventListener('toggle', function () {
      if (state.forceCollapse) { return; }
      storeSet('thinking.' + state.current.id + '.' + line.n, details.open ? '1' : '0');
    });
    return details;
  }

  function toolCallBlock(call) {
    var fn = call.function || {};
    var name = fn.name || '(未命名工具)';
    state.toolSeq++;
    state.calls.set(call.id, { name: name, seq: state.toolSeq });

    var details = document.createElement('details');
    details.className = 'block tool-call';
    details.open = storeGet('call.' + state.current.id + '.' + call.id) === '1';
    var head = el('summary', 'block-head');
    head.appendChild(el('span', 'call-tag', '#' + state.toolSeq));
    head.appendChild(el('span', 'block-name', name));
    var args = prettyJSON(String(fn.arguments || ''));
    head.appendChild(el('span', 'block-meta', '参数 ' + String(fn.arguments || '').length + ' 字符'));
    details.appendChild(head);

    var body = el('div', 'block-body');
    var code = el('pre', 'code');
    appendHighlighted(code, args || '(无参数)');
    body.appendChild(code);
    var actions = el('div', 'row-actions');
    actions.appendChild(copyButton(String(fn.arguments || '')));
    body.appendChild(actions);
    details.appendChild(body);
    details.addEventListener('toggle', function () {
      storeSet('call.' + state.current.id + '.' + call.id, details.open ? '1' : '0');
    });
    return details;
  }

  function classifyResult(text) {
    var s = String(text || '');
    if (/REJECTED|文件不存在|失败|error|not found|traceback/i.test(s)) { return 'error'; }
    if (/\bok\s*\(/.test(s)) { return 'ok'; }
    return 'plain';
  }

  function toolResultBlock(line) {
    var info = state.calls.get(line.tool_call_id);
    var name = info ? info.name : '(未配对的工具调用)';
    var text = String(line.text || '');
    var status = classifyResult(text);

    var details = document.createElement('details');
    details.className = 'block tool-result status-' + status;
    var head = el('summary', 'block-head');
    if (info) { head.appendChild(el('span', 'call-tag', '#' + info.seq)); }
    head.appendChild(el('span', 'block-name', name));
    head.appendChild(el('span', 'block-meta',
      text.length + ' 字符' + (status === 'error' ? ' · error' : status === 'ok' ? ' · ok' : '')));
    if (!info && line.tool_call_id) { head.appendChild(el('span', 'block-meta', 'id ' + line.tool_call_id)); }
    details.appendChild(head);

    var body = el('div', 'block-body');
    body.appendChild(collapsibleText(text, PREVIEW_LINES, 'result.' + line.n, 'result-text'));
    var actions = el('div', 'row-actions');
    actions.appendChild(copyButton(text));
    body.appendChild(actions);
    details.appendChild(body);
    return details;
  }

  /* Returns the element for one transcript line, or null when it is skipped. */
  function renderLine(line) {
    if (line.bad) {
      state.badLines++;
      updateBanner();
      return null;
    }
    // meta 行不是消息：它由时间线最前面的那张卡片渲染，绝不进消息序列。
    if (line.t && line.t !== 'msg') { return null; }
    if (!line.role) { return null; }

    var isToolCall = line.role === 'assistant' && line.tool_calls && line.tool_calls.length > 0;
    if (state.onlyTools && line.role !== 'tool' && !isToolCall) { return null; }

    var msg = el('section', 'msg role-' + line.role);
    msg.appendChild(messageHead(line));

    if (line.role === 'user') {
      var card = el('div', 'card user-card');
      card.appendChild(el('div', 'card-title', '用户/任务'));
      if (line.text) { card.appendChild(collapsibleText(line.text, LONG_TEXT_LINES, 'user.' + line.n)); }
      if (line.images && line.images.length) { card.appendChild(imageStrip(line)); }
      msg.appendChild(card);
      return msg;
    }

    if (line.role === 'assistant') {
      if (line.reasoning) { msg.appendChild(reasoningBlock(line)); }
      if (line.text) {
        var body = el('div', 'card');
        body.appendChild(collapsibleText(line.text, LONG_TEXT_LINES, 'asst.' + line.n));
        msg.appendChild(body);
      }
      (line.tool_calls || []).forEach(function (call) { msg.appendChild(toolCallBlock(call)); });
      if (!line.text && !line.reasoning && !(line.tool_calls || []).length) {
        msg.appendChild(el('div', 'card note', '(空消息)'));
      }
      return msg;
    }

    if (line.role === 'tool') {
      msg.appendChild(toolResultBlock(line));
      return msg;
    }

    var other = el('div', 'card');
    other.appendChild(collapsibleText(line.text || '(无正文)', LONG_TEXT_LINES, 'other.' + line.n));
    msg.appendChild(other);
    return msg;
  }

  function timelineEmptyText() {
    if (state.onlyTools) { return EMPTY_TEXTS.onlyTools; }
    if (!state.current) { return EMPTY_TEXTS.none; }
    return EMPTY_TEXTS.noMessages;
  }

  function renderTimeline() {
    clear(refs.timeline);
    state.msgSeq = 0;
    state.toolSeq = 0;
    state.badLines = 0;
    state.calls = new Map();
    var rendered = 0;
    state.lines.forEach(function (line) {
      var node = renderLine(line);
      if (node) { refs.timeline.appendChild(node); rendered++; }
    });
    rendered += renderMetaCard();
    if (!rendered) {
      refs.timeline.appendChild(el('div', 'empty', timelineEmptyText()));
    }
    updateBanner();
    renderToolbar();
  }

  function appendLines(lines) {
    if (!lines || !lines.length) { return; }
    var hasMeta = false;
    lines.forEach(function (line) {
      state.lines.push(line);
      if (line.t === 'meta') { hasMeta = true; }
    });
    var placeholder = refs.timeline.querySelector('.empty');
    lines.forEach(function (line) {
      var node = renderLine(line);
      if (node) { refs.timeline.appendChild(node); }
    });
    if (hasMeta) { renderMetaCard(); }
    if (placeholder && placeholder.parentNode && refs.timeline.children.length > 1) {
      refs.timeline.removeChild(placeholder);
    }
    updateBanner();
    renderToolbar();
    if (state.follow) { scrollToBottom(); }
  }

  function scrollToBottom() {
    refs.timeline.scrollTop = refs.timeline.scrollHeight;
  }

  /* ---------- data plumbing ---------- */

  function staticSession(id) {
    var found = null;
    (DATA.sessions || []).forEach(function (s) { if (s.id === id) { found = s; } });
    return found;
  }

  function selectSession(id) {
    var s = null;
    state.sessions.forEach(function (x) { if (x.id === id) { s = x; } });
    state.current = s;
    state.lines = [];
    state.nextFrom = 0;
    state.curSize = -1;
    state.curMtime = 0;
    state.meta = null;
    renderSessions();
    if (!s) { renderTimeline(); return; }

    if (MODE === 'static') {
      var embedded = staticSession(id);
      state.lines = normalizeLines(embedded && embedded.lines);
      state.nextFrom = state.lines.length;
      state.curSize = s.size;
      state.curMtime = new Date(s.mtime).getTime();
      renderTimeline();
      scrollToBottom();
      return;
    }
    renderTimeline();
    pullSession(true).then(function () { scrollToBottom(); });
  }

  function pullSession(reset) {
    var s = state.current;
    if (!s) { return Promise.resolve(); }
    var from = reset ? 0 : state.nextFrom;
    var url = '/api/session?id=' + encodeURIComponent(s.id) + '&from=' + from;
    return fetch(url, { cache: 'no-store' }).then(function (res) {
      if (!res.ok) { throw new Error('HTTP ' + res.status); }
      return res.json();
    }).then(function (payload) {
      if (!reset && payload.nextFrom < state.nextFrom) {
        // The transcript was replaced (a chapter was retried), so the lines we
        // hold no longer belong to the file on disk.
        state.lines = [];
        state.nextFrom = 0;
        return pullSession(true);
      }
      state.nextFrom = payload.nextFrom;
      state.curSize = payload.size;
      if (reset) {
        state.lines = normalizeLines(payload.lines);
        renderTimeline();
        if (state.follow) { scrollToBottom(); }
      } else {
        appendLines(normalizeLines(payload.lines));
      }
    }).catch(function (err) {
      setBanner('拉取会话失败：' + err.message);
    });
  }

  function applyIndex(payload) {
    state.sessions = payload.sessions || [];
    state.root = payload.root || state.root;
    state.generated = payload.generated || state.generated;
    renderSessions();
    updateRootLabel();
  }

  function refreshIndex() {
    if (MODE === 'static') {
      renderSessions();
      updateRootLabel();
      return Promise.resolve();
    }
    return fetch('/api/index', { cache: 'no-store' }).then(function (res) {
      if (!res.ok) { throw new Error('HTTP ' + res.status); }
      return res.json();
    }).then(function (payload) {
      applyIndex(payload);
      state.polling = true;
      renderBadge();
      var current = null;
      state.sessions.forEach(function (s) { if (state.current && s.id === state.current.id) { current = s; } });
      if (!current) { return; }
      state.current = current;
      var mtime = new Date(current.mtime).getTime();
      var changed = current.size !== state.curSize || mtime !== state.curMtime;
      if (changed && state.curSize >= 0) {
        state.curMtime = mtime;
        return pullSession(false);
      }
      state.curSize = current.size;
      state.curMtime = mtime;
      renderToolbar();
    }).catch(function () {
      state.polling = false;
      renderBadge();
    });
  }

  /* ---------- lightbox ---------- */

  function openLightbox(url, alt) {
    refs.lightboxImg.src = url;
    refs.lightboxImg.alt = alt || '';
    refs.lightbox.classList.remove('hidden');
  }

  function closeLightbox() {
    refs.lightbox.classList.add('hidden');
    refs.lightboxImg.removeAttribute('src');
  }

  /* ---------- wiring ---------- */

  refs.refresh.addEventListener('click', function () { refreshIndex(); });
  refs.search.addEventListener('input', function () {
    state.filter = refs.search.value.trim().toLowerCase();
    renderSessions();
  });
  refs.follow.addEventListener('change', function () {
    state.follow = refs.follow.checked;
    if (state.follow) { scrollToBottom(); }
  });
  refs.collapseThinking.addEventListener('change', function () {
    state.forceCollapse = refs.collapseThinking.checked;
    if (state.forceCollapse) {
      // 作用于时间线上所有消息（含增量追加进来的）。
      Array.prototype.forEach.call(refs.timeline.querySelectorAll('details.thinking'), function (d) { d.open = false; });
    }
  });
  refs.onlyTools.addEventListener('change', function () {
    state.onlyTools = refs.onlyTools.checked;
    renderTimeline();
  });
  if (refs.themeToggle) { refs.themeToggle.addEventListener('click', toggleTheme); }
  refs.timeline.addEventListener('scroll', function () {
    var near = refs.timeline.scrollHeight - refs.timeline.scrollTop - refs.timeline.clientHeight < 40;
    if (!near && state.follow) {
      state.follow = false;
      refs.follow.checked = false;
    }
  });
  refs.lightbox.addEventListener('click', closeLightbox);
  document.addEventListener('keydown', function (ev) {
    if (ev.key === 'Escape') { closeLightbox(); }
  });

  function boot() {
    applyTheme(storedTheme());
    refs.follow.checked = state.follow;
    if (MODE === 'static') {
      state.root = DATA.root || '';
      state.generated = DATA.generated || '';
      state.sessions = (DATA.sessions || []).map(function (s) {
        return {
          id: s.id, label: s.label, title: s.title, name: s.name, messages: s.messages,
          size: s.size, mtime: s.mtime, live: s.live, project: s.project, meta: s.meta
        };
      });
      renderSessions();
      updateRootLabel();
      renderBadge();
      renderTimeline();
      if (state.sessions.length) { selectSession(state.sessions[0].id); }
      return;
    }
    renderBadge();
    refreshIndex().then(function () {
      if (state.sessions.length && !state.current) {
        var live = null;
        state.sessions.forEach(function (s) { if (!live && s.live) { live = s; } });
        selectSession((live || state.sessions[0]).id);
      }
    });
    window.setInterval(function () {
      if (!document.hidden) { refreshIndex(); }
    }, POLL_MS);
  }

  boot();
})();
