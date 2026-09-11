'use strict';
/*
 * DocVision session viewer.
 *
 * **结构照 DSH（DeepSeek Harness）Web GUI 重做**，不是换配色：
 *   · 三栏 grid 骨架（侧栏 | 中栏 | 详情栏）+ 两条可拖拽分隔条，列宽用 DSH 的
 *     让步链解出来（`computeColumns`：先保住中栏 640px，再压缩详情栏，最后
 *     放弃详情栏）；
 *   · 中栏头部是「面包屑 + 标签页（对话 / 轨迹）」，显示开关收在标签页那行右侧；
 *   · 「轨迹」是 DSH 的 Trajectory——一次会话的全部步骤做成一张表，可筛选、
 *     可展开看完整输入输出、点一行跳回对话里对应的那条消息；
 *   · 元信息（系统提示词 + 工具清单）与指标（tokens/缓存/延迟/速度/费用）搬进
 *     右侧详情栏，消息流里只留一行极简摘要；
 *   · 侧栏行是 DSH 的固定高度两级行（项目 34px / 会话 32px、16px 固定插槽、
 *     没有按层级递增的缩进），列表底部有渐隐遮罩。
 *
 * One script serves both modes. When the page carries a #dsh-data block the
 * snapshot is complete and nothing is polled; otherwise the page is talking to
 * the local read-only server and refreshes itself every couple of seconds.
 *
 * Transcript content is always inserted as text (never as HTML), so a tool
 * result containing markup stays visible instead of becoming markup.
 */
(function () {
  var POLL_MS = 2000;
  var PREVIEW_LINES = 8;
  var LONG_TEXT_LINES = 20;
  var STORE_PREFIX = 'dsh.sessionview.';
  var THEME_KEY = 'theme';
  var DEFAULT_THEME = 'light';
  var ROOT_PROJECT = '（根目录）';

  /*
   * 布局常量全部照抄 DSH 的 client-ui-layout：视口 < 1024px 侧栏自动折成轨道，
   * 侧栏 264–420（默认 280，折叠后 56 的轨道），详情栏 300–520，中栏最小 640。
   */
  var SIDEBAR_AUTO_COLLAPSE = 1024;
  var RAIL_W = 56;
  var SIDEBAR_DEFAULT = 280;
  var SIDEBAR_MIN = 264;
  var SIDEBAR_MAX = 420;
  var DETAILS_DEFAULT = 400;
  var DETAILS_MIN = 300;
  var DETAILS_MAX = 520;
  var CENTER_MIN = 640;
  var OVERFLOW_LIMIT = 8;

  var dataEl = document.getElementById('dsh-data');
  var DATA = null;
  if (dataEl) {
    try { DATA = JSON.parse(dataEl.textContent); } catch (err) { DATA = null; }
  }
  var MODE = DATA ? 'static' : 'live';

  var refs = {
    frame: document.getElementById('frame'),
    sidebarCol: document.getElementById('sidebar-col'),
    handleSidebar: document.getElementById('handle-sidebar'),
    handleDetails: document.getElementById('handle-details'),
    detailsCol: document.getElementById('details-col'),
    detailsBody: document.getElementById('details-body'),
    detailsClose: document.getElementById('details-close'),
    detailsToggle: document.getElementById('details-toggle'),
    sideToggle: document.getElementById('side-toggle'),
    refresh: document.getElementById('refresh'),
    rootPath: document.getElementById('root-path'),
    search: document.getElementById('search'),
    list: document.getElementById('session-list'),
    foot: document.getElementById('side-foot'),
    totals: document.getElementById('side-totals'),
    crumbs: document.getElementById('crumbs'),
    actions: document.getElementById('header-actions'),
    badge: document.getElementById('mode-badge'),
    follow: document.getElementById('follow'),
    collapseThinking: document.getElementById('collapse-thinking'),
    onlyTools: document.getElementById('only-tools'),
    themeToggle: document.getElementById('theme-toggle'),
    tabs: {
      chat: document.getElementById('tab-chat'),
      trajectory: document.getElementById('tab-traj')
    },
    banner: document.getElementById('banner'),
    timeline: document.getElementById('timeline'),
    trajectory: document.getElementById('trajectory'),
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
    callNodes: {},
    anchors: {},
    badLines: 0,
    polling: false,
    theme: DEFAULT_THEME,
    view: 'chat',
    // 侧栏 / 详情栏的宽度偏好：0 = 折叠（侧栏折成 56px 轨道，详情栏关掉）
    sidebar: SIDEBAR_DEFAULT,
    details: 0,
    narrowExpanded: false,
    narrow: false,
    // 组展开状态：key -> true 表示"用户折叠了它"，缺省即展开。
    collapsed: {},
    overflow: {},
    // 侧栏列表签名：签名不变就一行 DOM 都不重建（--serve 每 2 秒轮询的护身符）
    listSig: '',
    meta: null,
    trajKinds: {},
    trajOpen: {}
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

  function storeJSON(key, fallback) {
    var raw = storeGet(key);
    if (!raw) { return fallback; }
    try {
      var v = JSON.parse(raw);
      return (v && typeof v === 'object') ? v : fallback;
    } catch (err) { return fallback; }
  }

  function clear(node) {
    while (node.firstChild) { node.removeChild(node.firstChild); }
  }

  function shortSHA(value) {
    var s = String(value || '');
    if (!s) { return '—'; }
    return s.length > 12 ? s.slice(0, 12) : s;
  }

  function firstLine(text, limit) {
    var s = String(text === undefined || text === null ? '' : text);
    var lines = s.split('\n');
    for (var i = 0; i < lines.length; i++) {
      var t = lines[i].replace(/\s+/g, ' ').trim();
      if (t) {
        var max = limit || 96;
        return t.length > max ? t.slice(0, max) + '…' : t;
      }
    }
    return '';
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

  /* ---------- 指标（t="usage" 行：token / 缓存 / 时延 / 速度） ---------- */

  function fmtTokens(n) {
    n = Number(n) || 0;
    if (n >= 1e6) { return (n / 1e6).toFixed(2) + 'M'; }
    if (n >= 1e3) { return (n / 1e3).toFixed(n >= 1e4 ? 0 : 1) + 'k'; }
    return String(n);
  }

  function fmtDur(ms) {
    ms = Number(ms) || 0;
    if (ms < 1000) { return ms + 'ms'; }
    if (ms < 60000) { return (ms / 1000).toFixed(1) + 's'; }
    var m = Math.floor(ms / 60000);
    var sec = Math.round((ms % 60000) / 1000);
    return m + 'm' + (sec < 10 ? '0' : '') + sec + 's';
  }

  /*
   * 会话指标：把 t="usage" 行按请求加权求和。口径与后端 UsageStats 一致——
   * 缓存命中率 = Σcached/Σprompt，输出速度用"生成时间"（总耗时减首字延迟）
   * 作分母，所以把请求排队与思考等待排除在速度之外。返回 null 表示这个转录
   * 没有用量记录（旧转录），页面显示"无指标"而不是一堆 0（0 会被误读成实测）。
   */
  function usageLines(lines) {
    var out = [];
    (lines || []).forEach(function (l) { if (l && l.t === 'usage' && l.stats) { out.push(l); } });
    return out;
  }

  function aggregate(usages) {
    if (!usages.length) { return null; }
    var agg = {
      requests: 0, promptTokens: 0, cachedTokens: 0, completionTokens: 0, reasoningTokens: 0,
      durationMs: 0, ttftMs: 0, genMs: 0, streamed: false, firstTs: '', lastTs: '', perRequest: usages
    };
    usages.forEach(function (l) {
      var st = l.stats;
      agg.requests += 1;
      agg.promptTokens += Number(st.promptTokens) || 0;
      agg.cachedTokens += Number(st.cachedTokens) || 0;
      agg.completionTokens += Number(st.completionTokens) || 0;
      agg.reasoningTokens += Number(st.reasoningTokens) || 0;
      var dur = Number(st.durationMs) || 0;
      var ttft = Number(st.ttftMs) || 0;
      agg.durationMs += dur;
      agg.ttftMs += ttft;
      agg.genMs += Math.max(dur - ttft, 1);
      if (st.streamed) { agg.streamed = true; }
      var ts = l.ts || '';
      if (ts && (!agg.firstTs || ts < agg.firstTs)) { agg.firstTs = ts; }
      if (ts && ts > agg.lastTs) { agg.lastTs = ts; }
    });
    agg.cacheHitPct = agg.promptTokens ? agg.cachedTokens * 100 / agg.promptTokens : 0;
    agg.avgTtftMs = agg.ttftMs / agg.requests;
    agg.avgDurationMs = agg.durationMs / agg.requests;
    agg.outputTps = agg.genMs ? agg.completionTokens * 1000 / agg.genMs : 0;
    agg.spanMs = 0;
    if (agg.firstTs && agg.lastTs) {
      var t0 = Date.parse(agg.firstTs);
      var t1 = Date.parse(agg.lastTs);
      if (!isNaN(t0) && !isNaN(t1) && t1 > t0) { agg.spanMs = t1 - t0; }
    }
    return agg;
  }

  // 会话列表里的 sessions 项已经带后端聚合好的 stats（不用再读转录）。
  function scanStats(session) {
    var st = session && session.stats;
    if (!st || !st.requests) { return null; }
    return st;
  }

  /*
   * 费用：后端按配置里的 models.*.price 算好放在 session.cost 里（没配价格
   * 就是 null，这里什么都不显示——"¥0"会被读成"这次没花钱"）。金额下界时前
   * 缀 "≥"：有请求的模型没配价格，真实花费只会更高。
   */
  function fmtCost(cost, unpriced) {
    if (!cost) { return ''; }
    return (unpriced ? '≥' : '') + (cost.currency || '') + cost.total.toFixed(2);
  }

  function statsSummary(st, session) {
    if (!st) { return ''; }
    var parts = ['输入 ' + fmtTokens(st.promptTokens) + ' · 输出 ' + fmtTokens(st.completionTokens)];
    if (st.promptTokens) { parts.push('缓存 ' + st.cacheHitPct.toFixed(0) + '%'); }
    if (st.avgTtftMs) { parts.push('首字 ' + fmtDur(st.avgTtftMs)); }
    var money = session ? fmtCost(session.cost, false) : '';
    if (money) { parts.push(money); }
    return parts.join(' · ');
  }

  /* ---------- 持久化：主题 / 布局 / 折叠状态 ---------- */

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

  /*
   * 折叠状态与布局都落 localStorage：
   *   collapsed = { "proj:<项目>": true, "stage:<项目>/<阶段>": true }
   * 缺省即"展开"（用户明确要求默认展开），只有用户手动折叠过才写进去。
   */
  function loadState() {
    state.collapsed = storeJSON('collapsed', {});
    state.overflow = storeJSON('overflow', {});
    // Number(null) === 0，所以要先用 null 判断"有没有存过"——否则首次访问
    // 会被读成"侧栏宽度 0 = 折叠"，页面一打开就是一条轨道。
    var rawSidebar = storeGet('layout.sidebar');
    if (rawSidebar !== null) {
      var sidebar = Number(rawSidebar);
      if (!isNaN(sidebar) && sidebar >= 0) { state.sidebar = sidebar; }
    }
    var rawDetails = storeGet('layout.details');
    var details = rawDetails === null ? 0 : Number(rawDetails);
    state.details = (!isNaN(details) && details > 0) ? details : 0;
    state.narrowExpanded = storeGet('layout.narrowExpanded') === '1';
  }

  function persistLayout() {
    storeSet('layout.sidebar', String(state.sidebar));
    storeSet('layout.details', String(state.details));
    storeSet('layout.narrowExpanded', state.narrowExpanded ? '1' : '0');
  }

  function groupKey(kind, name) { return kind + ':' + name; }

  function isCollapsed(key) { return state.collapsed[key] === true; }

  function setCollapsed(key, collapsed) {
    if (collapsed) { state.collapsed[key] = true; } else { delete state.collapsed[key]; }
    storeSet('collapsed', JSON.stringify(state.collapsed));
  }

  function isOverflowOpen(key) { return state.overflow[key] === true; }

  function setOverflowOpen(key, open) {
    if (open) { state.overflow[key] = true; } else { delete state.overflow[key]; }
    storeSet('overflow', JSON.stringify(state.overflow));
  }

  /*
   * 把 <details> 的 open 状态接到记忆上。程序化改 open 时不会有"真实变化"
   * （dataset.open 已经等于新值），只有用户点击才会走到 setCollapsed——这样
   * 每 2 秒轮询重建 DOM 也不会把用户折叠过的组重新按程序化事件写坏。
   */
  function bindCollapse(node, key, wantOpen, frozen) {
    node.open = wantOpen;
    node.dataset.open = wantOpen ? '1' : '0';
    node.addEventListener('toggle', function () {
      var now = node.open ? '1' : '0';
      if (node.dataset.open === now) { return; }
      node.dataset.open = now;
      if (frozen) { return; }  // 过滤时强制展开，不要把"被强制"当成用户选择写回记忆
      setCollapsed(key, !node.open);
    });
  }

  /* ---------- 三栏布局：DSH 的让步链 + 拖拽分隔条 ---------- */

  var currentCols = { sidebar: SIDEBAR_DEFAULT, center: 0, details: 0 };

  function clampWidth(px, min, max) {
    return Math.min(max, Math.max(min, Math.round(px)));
  }

  /*
   * Solve the three column widths for one viewport frame. Pure: no hysteresis —
   * the output is a function of (viewport, preferences) only, so recovery on
   * re-widening is automatic. 逐字照搬 DSH 的 computeColumns。
   */
  function computeColumns(viewport, sidebar, details) {
    var s = sidebar === 0 ? RAIL_W : clampWidth(sidebar, SIDEBAR_MIN, SIDEBAR_MAX);
    var d0 = details === 0 ? 0 : clampWidth(details, DETAILS_MIN, DETAILS_MAX);
    if (s + d0 + CENTER_MIN <= viewport) {
      return { sidebar: s, center: viewport - s - d0, details: d0 };
    }
    var d1 = d0 === 0 ? 0 : Math.max(DETAILS_MIN, viewport - s - CENTER_MIN);
    if (s + d1 + CENTER_MIN <= viewport) {
      return { sidebar: s, center: CENTER_MIN, details: d1 };
    }
    return { sidebar: s, center: Math.max(0, viewport - s), details: 0 };
  }

  function applyLayout() {
    var viewport = refs.frame.clientWidth || window.innerWidth;
    state.narrow = viewport < SIDEBAR_AUTO_COLLAPSE;
    var sidebarCollapsed = state.narrow ? !state.narrowExpanded : state.sidebar === 0;
    var sidebarPref = sidebarCollapsed ? 0 : (state.sidebar === 0 ? SIDEBAR_DEFAULT : state.sidebar);
    var cols = computeColumns(viewport, sidebarPref, state.details);
    currentCols = cols;

    refs.frame.style.gridTemplateColumns =
      cols.sidebar + 'px minmax(0, 1fr) ' + cols.details + 'px';
    if (sidebarCollapsed) { refs.frame.setAttribute('data-sidebar-collapsed', ''); }
    else { refs.frame.removeAttribute('data-sidebar-collapsed'); }
    if (cols.details === 0) { refs.frame.setAttribute('data-details-collapsed', ''); }
    else { refs.frame.removeAttribute('data-details-collapsed'); }

    refs.handleSidebar.style.left = cols.sidebar + 'px';
    refs.handleDetails.style.left = Math.max(0, viewport - cols.details) + 'px';
    if (cols.details === 0) { refs.handleDetails.setAttribute('data-hidden', 'true'); }
    else { refs.handleDetails.removeAttribute('data-hidden'); }

    if (refs.detailsToggle) {
      refs.detailsToggle.setAttribute('aria-pressed', cols.details > 0 ? 'true' : 'false');
      refs.detailsToggle.title = cols.details > 0 ? '关闭详情面板' : '详情面板（元信息 / 指标）';
    }
    if (refs.sideToggle) {
      refs.sideToggle.setAttribute('aria-pressed', sidebarCollapsed ? 'false' : 'true');
    }
  }

  function toggleSidebar() {
    if (state.narrow) {
      state.narrowExpanded = !state.narrowExpanded;
    } else {
      state.sidebar = state.sidebar === 0 ? SIDEBAR_DEFAULT : 0;
    }
    persistLayout();
    applyLayout();
  }

  function toggleDetails() {
    if (state.details > 0) {
      state.details = 0;
    } else {
      state.details = clampWidth(state.details || DETAILS_DEFAULT, DETAILS_MIN, DETAILS_MAX);
      // 让步链在中栏 640px 保不住时会直接放弃详情栏（DSH 的 computeColumns 就是
      // 这么定的）。点了没反应比"先把侧栏折成轨道腾地方"更糟：窄窗口下用户要的
      // 就是这块面板。所以放不下时顺手把侧栏折了，再解一次。
      var viewport = refs.frame.clientWidth || window.innerWidth;
      var collapsed = state.narrow ? !state.narrowExpanded : state.sidebar === 0;
      var pref = collapsed ? 0 : (state.sidebar === 0 ? SIDEBAR_DEFAULT : state.sidebar);
      if (computeColumns(viewport, pref, state.details).details === 0 && !collapsed) {
        if (state.narrow) { state.narrowExpanded = false; } else { state.sidebar = 0; }
      }
      renderDetails();
    }
    persistLayout();
    applyLayout();
  }

  function detailsVisible() { return currentCols.details > 0; }

  function openDetails() {
    if (state.details === 0) { toggleDetails(); }
  }

  /* 拖拽：8px 命中区的 pointer capture，松手才落盘。 */
  function wireHandle(handle, side) {
    var dragging = false;
    var origin = 0;
    var base = 0;
    handle.addEventListener('pointerdown', function (ev) {
      ev.preventDefault();
      dragging = true;
      origin = ev.clientX;
      base = side === 'sidebar' ? currentCols.sidebar : currentCols.details;
      if (handle.setPointerCapture) { handle.setPointerCapture(ev.pointerId); }
      refs.frame.setAttribute('data-dragging', '');
      handle.setAttribute('data-dragging', 'true');
    });
    handle.addEventListener('pointermove', function (ev) {
      if (!dragging) { return; }
      var dx = ev.clientX - origin;
      if (side === 'sidebar') {
        state.sidebar = clampWidth(base + dx, SIDEBAR_MIN, SIDEBAR_MAX);
        if (state.narrow) { state.narrowExpanded = true; }
      } else {
        state.details = clampWidth(base - dx, DETAILS_MIN, DETAILS_MAX);
      }
      applyLayout();
    });
    var stop = function () {
      if (!dragging) { return; }
      dragging = false;
      refs.frame.removeAttribute('data-dragging');
      handle.removeAttribute('data-dragging');
      persistLayout();
    };
    handle.addEventListener('pointerup', stop);
    handle.addEventListener('pointercancel', stop);
    // 双击分隔条 = 回到默认宽度（DSH 的面板同理）
    handle.addEventListener('dblclick', function () {
      if (side === 'sidebar') { state.sidebar = SIDEBAR_DEFAULT; }
      else { state.details = DETAILS_DEFAULT; }
      persistLayout();
      applyLayout();
    });
  }

  /* ---------- 视图切换（对话 / 轨迹） ---------- */

  function switchView(view) {
    state.view = view === 'trajectory' ? 'trajectory' : 'chat';
    var chat = state.view === 'chat';
    refs.timeline.classList.toggle('hidden', !chat);
    refs.trajectory.classList.toggle('hidden', chat);
    refs.tabs.chat.classList.toggle('tab-active', chat);
    refs.tabs.trajectory.classList.toggle('tab-active', !chat);
    refs.tabs.chat.setAttribute('aria-selected', chat ? 'true' : 'false');
    refs.tabs.trajectory.setAttribute('aria-selected', chat ? 'false' : 'true');
    if (!chat) { renderTrajectory(); }
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

  function metaOf(session) {
    return (session && session.meta && session.meta.count) ? session.meta : null;
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

  /* ---------- 侧栏 ---------- */

  /*
   * 侧栏的第二层：项目 → **流程阶段** → 会话。用户要的是"按项目 + 流程
   * 阶段组织、可展开、显示当前进展/完成情况"，所以阶段组头上既有本阶段
   * 的会话数，也有从 progress.json 读来的阶段状态（✓ 完成 / 进行中 / —）。
   */
  var STAGE_ORDER = ['vector', 'style', 'chapters', 'convert', 'checker', 'style-fix', 'figure-check'];

  function stageRank(stage) {
    var i = STAGE_ORDER.indexOf(stage);
    return i < 0 ? STAGE_ORDER.length : i;
  }

  function stageTitleOf(stage) {
    var map = {
      'vector': '矢量图', 'style': '样式', 'chapters': '章节划分', 'convert': '章节转换',
      'checker': '章节核对', 'style-fix': '样式修复', 'figure-check': '逐图校验'
    };
    return map[stage] || stage || '其他会话';
  }

  // progress.json 里的键是档位级的（images/style/chapters/convert/assemble），
  // 阶段名与它并不一一对应：矢量图属于 images，checker/样式修复属于 convert 之后。
  function progressKeyFor(stage) {
    if (stage === 'vector') { return 'images'; }
    if (stage === 'checker' || stage === 'style-fix') { return 'convert'; }
    return stage;
  }

  function stageStatusText(stages, stage) {
    if (!stages) { return null; }
    var key = progressKeyFor(stage);
    var v = stages[key];
    if (v === undefined || v === null || v === '') { return null; }
    var text = String(v);
    var done = /^(done|ok|true|finished|complete[d]?)$/i.test(text);
    return { text: done ? '✓ 完成' : text, done: done, title: 'progress.json: ' + key + ' = ' + text };
  }

  // 书级进展：progress.json 原样列出各阶段状态（档位1/2 的键略有不同）。
  function projectProgressLine(stages) {
    if (!stages) { return null; }
    var order = ['images', 'style', 'chapters', 'convert', 'assemble'];
    var parts = [];
    order.forEach(function (k) {
      if (stages[k] === undefined) { return; }
      var done = /^(done|ok|true|finished|complete[d]?)$/i.test(String(stages[k]));
      parts.push(k + (done ? ' ✓' : ' ' + stages[k]));
    });
    if (!parts.length) { return null; }
    var line = el('div', 'proj-progress', parts.join(' · '));
    line.title = 'progress.json 里各阶段的当前状态';
    return line;
  }

  function sessionHaystack(s) {
    // 逐图会话要能按**图片名 / PDF 页 / 顺序 / 图片类型**检索，所以这些
    // 字段都进搜索串（页码与序号连 "p12"/"p.12"/"#12"/"12" 都能命中）。
    var parts = [s.title, s.label, s.id, s.name, s.project, s.stage, s.stageTitle,
      s.imageName, s.imageType, s.imageCaption];
    if (s.page) { parts.push('p' + s.page, 'p.' + s.page, '页' + s.page, String(s.page)); }
    if (s.imageOrder) { parts.push('#' + s.imageOrder, '第' + s.imageOrder + '张', String(s.imageOrder)); }
    return parts.filter(Boolean).join(' ').toLowerCase();
  }

  function sessionTitleOf(s) {
    if (s.imageName && (s.page || s.imageOrder)) {
      // 逐图会话的标题用**图注/图片名**，页与序号在从属信息里（侧栏只有一行）。
      return '矢量图 · ' + (s.imageCaption || s.imageName);
    }
    return s.title || s.label || s.name;
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
        // 「输出根/项目名」（多项目布局：<latex_project>/<书名>/…）与
        // 「输出根」（旧版单项目布局，该目录本身就是工作区）在侧栏里
        // 区别对待：前者以书名为标题、容器名弱化成前缀，后者标一句
        // 「旧版单项目」——否则一个输出根下的多本书会全挤在一个组里，
        // 或者把老工程误当成新布局的一本书。
        var cut = name.lastIndexOf('/');
        g = byName[name] = {
          name: name,
          prefix: cut > 0 ? name.slice(0, cut + 1) : '',
          title: cut > 0 ? name.slice(cut + 1) : name,
          legacy: !!s.projectLegacy,
          items: [], live: 0, matched: false
        };
        groups.push(g);
      }
      if (s.projectLegacy) { g.legacy = true; }
      if (q && sessionHaystack(s).indexOf(q) < 0) { return; }
      g.items.push(s);
      if (s.live) { g.live++; }
      // 阶段子组：项目内按流程阶段再分一层（组头带会话数与 progress.json 状态）。
      var stage = s.stage || 'session';
      if (!g.stages) { g.stages = {}; }
      var sg = g.stages[stage];
      if (!sg) {
        sg = g.stages[stage] = { stage: stage, title: stageTitleOf(stage), items: [], live: 0 };
      }
      sg.items.push(s);
      if (s.live) { sg.live++; }
    });
    if (q) {
      groups = groups.filter(function (g) { return g.items.length > 0; });
      groups.forEach(function (g) { g.matched = true; });
    }
    return groups;
  }

  /*
   * 会话行的悬浮说明：**完整**的用量口径、元信息、图片身份与路径。行里放不下
   * （DSH 的行只有标题 + 时间），所以这里承载全部细节，也顺带保证 patchList()
   * 刷新行时口径只有一处实现。
   */
  function sessionTip(s) {
    var tip = [s.id];
    var st = scanStats(s);
    if (st) {
      tip.push(statsSummary(st, s));
      tip.push(st.requests + ' 次 API 请求 · 输入 ' + st.promptTokens +
        ' tokens（其中 ' + st.cachedTokens + ' 命中前缀缓存）· 输出 ' + st.completionTokens +
        (st.reasoningTokens ? '（思考 ' + st.reasoningTokens + '）' : '') +
        ' · 平均耗时 ' + fmtDur(st.avgDurationMs) + ' · 平均首字 ' + fmtDur(st.avgTtftMs) +
        ' · 输出 ' + (st.outputTps || 0).toFixed(1) + ' tok/s');
    }
    var m = metaOf(s);
    if (m) {
      // 元信息（系统提示词快照 + 工具清单）本身搬到了右侧详情栏，这里只留提示。
      tip.push('含系统提示词快照：' + m.count + ' 条 t=meta 元信息行，最新一条 ' +
        (m.promptChars || 0) + ' 字符' + (m.model ? '（模型 ' + m.model + '）' : '') +
        ((m.tools) ? '，含 ' + m.tools + ' 个工具定义' : ''));
    }
    if (s.imageName) {
      tip.push('来源图片: ' + s.imageName + (s.imageType ? ('（' + s.imageType + '）') : '') +
        (s.page ? ('\n第 ' + s.page + ' 页') : '') + (s.imageOrder ? ('\n书内第 ' + s.imageOrder + ' 张') : '') +
        (s.imageCaption ? ('\n图注: ' + s.imageCaption) : ''));
    }
    tip.push('消息 ' + s.messages + ' 条 · ' + fmtSize(s.size) + ' · 最后写入 ' + fmtClock(s.mtime));
    var sub = subPathOf(s.id);
    if (sub) { tip.push('目录 ' + sub); }
    return tip.join('\n');
  }

  function usageChipText(s) {
    var st = scanStats(s);
    return st ? fmtTokens(st.promptTokens) + ' / ' + fmtTokens(st.completionTokens) : '';
  }

  function imageChipText(s) {
    var bits = [];
    if (s.page) { bits.push('p' + s.page); }
    if (s.imageOrder) { bits.push('#' + s.imageOrder); }
    if (s.imageType) { bits.push(s.imageType); }
    return bits.join('·');
  }

  /*
   * 会话行 = DSH 的 sessionRow：32px 高、padding 0 8px、圆角 8px，左边一个
   * 16px 固定插槽放状态点；标题 14px/20px 省略号；次要信息与时间 12px 靠右，
   * hover 时让位给行内操作按钮。**没有按层级递增的缩进。**
   *
   * DSH 的行只有"标题 + 时间"；这里的用量摘要与图片身份是本项目的要求，所以
   * 用最小的 chip 挂在右侧，完整说明（含系统提示词快照、每次请求口径）进 title。
   */
  function sessionRow(s) {
    var row = el('button', 'session-row');
    row.type = 'button';
    row.setAttribute('data-id', s.id);
    row.setAttribute('role', 'treeitem');
    if (state.current && state.current.id === s.id) { row.classList.add('active'); }

    var slot = el('span', 'row-slot');
    slot.appendChild(el('span', 'dot' + (s.live ? ' live' : '')));
    row.appendChild(slot);

    row.appendChild(el('span', 'row-title', sessionTitleOf(s)));

    var usage = usageChipText(s);
    if (usage) { row.appendChild(el('span', 'row-chip usage-chip', usage)); }
    if (s.imageName) { row.appendChild(el('span', 'row-chip image-chip', imageChipText(s))); }
    row.title = sessionTip(s);

    row.appendChild(el('span', 'row-time', relTime(s.mtime)));

    var actions = el('span', 'row-actions');
    var info = el('button', 'icon-btn', 'ⓘ');
    info.type = 'button';
    info.title = '打开详情面板（元信息 / 指标）';
    info.addEventListener('click', function (ev) {
      ev.stopPropagation();
      selectSession(s.id);
      openDetails();
    });
    actions.appendChild(info);
    row.appendChild(actions);

    row.addEventListener('click', function () { selectSession(s.id); });
    return row;
  }

  function buildGroup(g) {
    var wrap = document.createElement('details');
    wrap.className = 'proj-group';
    var key = groupKey('proj', g.name);
    // 默认全部展开；过滤命中的组强制展开，其余按记忆的折叠状态恢复。
    bindCollapse(wrap, key, g.matched ? true : !isCollapsed(key), !!g.matched);

    var head = el('summary', 'proj-row');
    head.title = g.name;
    var slot = el('span', 'row-slot');
    slot.appendChild(el('span', 'row-caret'));
    head.appendChild(slot);

    var name = el('span', 'row-body');
    if (g.prefix) {
      // 容器（输出根）名弱化：一眼看到的是书名，鼠标悬停看到全路径。
      name.appendChild(el('span', 'proj-prefix', g.prefix));
    }
    name.appendChild(el('span', 'proj-title', g.title));
    head.appendChild(name);
    head.appendChild(el('span', 'row-meta', g.items.length + ' 个会话'));
    if (g.legacy) {
      var chip = el('span', 'proj-note', '旧版单项目');
      chip.title = '这个输出根本身就是一个工程（work/、progress.json 等直接挂在它下面），' +
        '是引入多项目布局之前的形态；新布局是「输出根/书名/」。';
      head.appendChild(chip);
    }
    if (g.live) { head.appendChild(el('span', 'dot live')); }
    wrap.appendChild(head);

    var body = document.createElement('div');
    var stages0 = g.items.length ? g.items[0].projectStages : null;
    var prog = projectProgressLine(stages0);
    if (prog) { body.appendChild(prog); }

    // 阶段子组：可展开、显示本阶段会话数 + 该阶段在 progress.json 里的状态。
    var stageKeys = Object.keys(g.stages || {}).sort(function (a, b) {
      var d = stageRank(a) - stageRank(b);
      return d !== 0 ? d : (a < b ? -1 : 1);
    });
    var multi = stageKeys.length > 1;
    stageKeys.forEach(function (stg) {
      var sg = g.stages[stg];
      var items = sg.items.slice().sort(function (x, y) {
        // 逐图会话按"书里的顺序"排，而不是按修改时间。
        if (x.imageOrder && y.imageOrder && x.imageOrder !== y.imageOrder) {
          return x.imageOrder - y.imageOrder;
        }
        return (y.mtime > x.mtime) ? 1 : -1;
      });
      var host = body;
      if (multi) {
        var det = document.createElement('details');
        det.className = 'stage-group';
        var skey = groupKey('stage', g.name + '/' + stg);
        bindCollapse(det, skey, !isCollapsed(skey), false);

        var sh = el('summary', 'stage-row');
        var sslot = el('span', 'row-slot');
        sslot.appendChild(el('span', 'row-caret'));
        sh.appendChild(sslot);
        sh.appendChild(el('span', 'row-body', sg.title));
        sh.appendChild(el('span', 'row-meta', sg.items.length + ' 个会话'));
        var status = stageStatusText(stages0, stg);
        if (status) {
          var pc = el('span', 'stage-progress' + (status.done ? ' done' : ''), status.text);
          pc.title = status.title;
          sh.appendChild(pc);
        }
        if (sg.live) { sh.appendChild(el('span', 'dot live')); }
        det.appendChild(sh);
        var sbody = document.createElement('div');
        det.appendChild(sbody);
        body.appendChild(det);
        host = sbody;
      }
      // 组内会话过多时只显示前 N 条 + 一个「更多会话」按钮。
      var okey = groupKey('overflow', g.name + '/' + stg);
      var needOverflow = items.length > OVERFLOW_LIMIT;
      var openAll = !needOverflow || isOverflowOpen(okey) || !!state.filter;
      var shown = openAll ? items : items.slice(0, OVERFLOW_LIMIT);
      shown.forEach(function (s) { host.appendChild(sessionRow(s)); });
      if (!openAll) {
        var more = el('button', 'session-overflow',
          '更多会话（还有 ' + (items.length - shown.length) + ' 个）');
        more.type = 'button';
        more.addEventListener('click', function () {
          setOverflowOpen(okey, true);
          renderSessions();
        });
        host.appendChild(more);
      }
    });
    wrap.appendChild(body);
    return wrap;
  }

  function findSession(id) {
    var found = null;
    state.sessions.forEach(function (s) { if (s.id === id) { found = s; } });
    return found;
  }

  function renderSessions() {
    var scroll = refs.list.scrollTop;
    clear(refs.list);
    var groups = buildGroups();
    var shown = 0;
    groups.forEach(function (g) {
      refs.list.appendChild(buildGroup(g));
      shown += g.items.length;
    });
    if (!shown) {
      refs.list.appendChild(el('div', 'empty',
        state.sessions.length ? '没有匹配的会话' : '没有找到 *.jsonl 会话转录'));
    }
    var live = state.sessions.filter(function (s) { return s.live; }).length;
    refs.foot.textContent = groups.length + ' 个项目 · ' + state.sessions.length + ' 个会话' +
      (live ? ' · ' + live + ' 个活跃' : '') + (state.filter ? ' · 匹配 ' + shown : '');
    refs.list.scrollTop = scroll;
    state.listSig = listSignature();
  }

  /*
   * 轮询回来的轻量修补：签名没变时**一行 DOM 都不重建**，一个节点都不新建/搬动，
   * 只改会随时间漂移的文案（相对时间、活跃圆点、用量 chip、悬浮说明）与选中高亮。
   * 滚动位置因此天然不动——这就是"左侧展开总被刷新冲掉"的修法。
   */
  function patchList() {
    var rows = refs.list.querySelectorAll('.session-row');
    Array.prototype.forEach.call(rows, function (row) {
      var id = row.getAttribute('data-id');
      var s = findSession(id);
      if (!s) { return; }
      row.classList.toggle('active', !!(state.current && state.current.id === id));
      var t = row.querySelector('.row-time');
      if (t) { t.textContent = relTime(s.mtime); }
      var dot = row.querySelector('.dot');
      if (dot) { dot.classList.toggle('live', !!s.live); }
      var chip = row.querySelector('.usage-chip');
      var text = usageChipText(s);
      if (chip && text && chip.textContent !== text) { chip.textContent = text; }
      row.title = sessionTip(s);
    });
  }

  /*
   * 列表签名：只统计**会改变行集合或其可见文案**的东西——会话 id（顺序即排序）、
   * 消息数、费用、请求数、项目的 progress.json 快照，加上过滤词。
   *
   * 刻意排除三样：
   *  - 折叠 / 溢出记忆的键：那是视图状态，点组头只改 <details> 的 open，不该让整个
   *    列表作废重建（把视图状态混进数据签名，正是"刷新后又自己展开"那类缺陷的温床）。
   *  - mtime / size / live：它们每秒都在动（live 会跨过活跃窗口的边界），但它们唯一
   *    的可见表现是行尾相对时间与活跃圆点，patchList() 直接改写这两处即可。
   *  - 后加的会话靠 id 顺序就能发现：顺序变了签名就变了。
   */
  function listSignature() {
    var parts = [state.filter, state.sessions.length];
    state.sessions.forEach(function (s) {
      parts.push([s.id, s.messages,
        s.cost ? (s.cost.total + (s.cost.currency || '')) : '',
        s.stats ? s.stats.requests : 0,
        s.projectStages ? JSON.stringify(s.projectStages) : ''].join('~'));
    });
    return parts.join('|');
  }

  function refreshList() {
    var sig = listSignature();
    if (sig === state.listSig && refs.list.childElementCount) {
      patchList();
      return;
    }
    renderSessions();
    renderTotals();
  }

  /*
   * 合计：所有会话的输入/输出 token 与平均缓存命中率（按 token 加权）+ 费用。
   * 金额下界用 "≥" 标注（有请求的模型没配价格时真实花费只会更高）。
   */
  function renderTotals() {
    var tot = { requests: 0, prompt: 0, cached: 0, completion: 0, cost: 0, costCurrency: '', unpriced: 0 };
    state.sessions.forEach(function (s) {
      var st = scanStats(s);
      if (!st) { return; }
      tot.requests += st.requests;
      tot.prompt += st.promptTokens;
      tot.cached += st.cachedTokens;
      tot.completion += st.completionTokens;
      if (s.cost) {
        tot.cost += Number(s.cost.total) || 0;
        tot.costCurrency = s.cost.currency || tot.costCurrency;
      } else {
        // 没有 cost 的会话：它的请求都算"未配价"，金额只是下界。
        tot.unpriced += st.requests;
      }
    });
    if (!tot.requests) {
      refs.totals.textContent = '';
      refs.totals.classList.add('hidden');
      return;
    }
    var money = '';
    if (tot.cost > 0 || tot.costCurrency) {
      money = ' · 费用 ' + (tot.unpriced ? '≥' : '') + tot.costCurrency + tot.cost.toFixed(2);
    }
    refs.totals.textContent = '合计 ' + tot.requests + ' 次请求 · 输入 ' + fmtTokens(tot.prompt) +
      ' / 输出 ' + fmtTokens(tot.completion) + ' tokens' +
      (tot.prompt ? ' · 缓存命中 ' + (tot.cached * 100 / tot.prompt).toFixed(0) + '%' : '') + money;
    if (tot.unpriced && money) {
      refs.totals.title = '有 ' + tot.unpriced + ' 次请求的模型没配价格（models.<条目>.price），金额只是下界';
    }
    refs.totals.classList.remove('hidden');
  }

  function updateRootLabel() {
    refs.rootPath.textContent = state.root || '—';
  }

  /* ---------- 中栏头部：面包屑 + 标签页 ---------- */

  function renderHeader() {
    clear(refs.crumbs);
    var cur = state.current;
    if (!cur) {
      refs.crumbs.appendChild(el('span', 'crumb crumb-current', '未选择会话'));
    } else {
      var project = cur.project || projectOf(cur.id);
      var proj = el('button', 'crumb is-link', project);
      proj.type = 'button';
      proj.title = '在侧栏里定位到这个项目';
      proj.addEventListener('click', function () { revealProject(project); });
      refs.crumbs.appendChild(proj);
      refs.crumbs.appendChild(el('span', 'crumb-sep', '›'));
      var name = el('span', 'crumb crumb-current', sessionTitleOf(cur));
      name.title = cur.id;
      refs.crumbs.appendChild(name);
    }
    clear(refs.actions);
    var parts = [];
    if (cur) {
      parts.push(cur.messages + ' 条消息');
      if (state.lines.length) { parts.push(state.lines.length + ' 行'); }
      parts.push(fmtSize(cur.size));
      if (state.badLines) { parts.push('坏行 ' + state.badLines); }
    }
    if (parts.length) { refs.actions.appendChild(el('span', 'badge', parts.join(' · '))); }
    renderBadge();
  }

  function revealProject(project) {
    var wraps = refs.list.querySelectorAll('.proj-group');
    var hit = null;
    Array.prototype.forEach.call(wraps, function (w) {
      var summary = w.querySelector('.proj-row');
      if (!hit && summary && summary.title === project) { hit = w; }
    });
    if (!hit) { return; }
    if (!hit.open) {
      hit.open = true;
      hit.dataset.open = '1';
      setCollapsed(groupKey('proj', project), false);
    }
    hit.scrollIntoView({ block: 'nearest' });
  }

  function renderBadge() {
    if (!refs.badge) { return; }
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

  /* ---------- 右侧详情栏：指标 + 元信息 ---------- */

  function detailBlock(title) {
    var wrap = el('section', 'detail-block');
    if (title) { wrap.appendChild(el('h3', 'detail-block-title', title)); }
    return wrap;
  }

  function kvList(pairs) {
    var dl = el('dl', 'detail-kv');
    pairs.forEach(function (p) {
      if (p === null) { return; }
      var dt = el('dt', null, p[0]);
      var dd = el('dd', p[3] ? 'mono' : null, p[1]);
      if (p[2]) { dd.title = p[2]; }
      dl.appendChild(dt);
      dl.appendChild(dd);
    });
    return dl;
  }

  function detailsSessionBlock() {
    var cur = state.current;
    if (!cur) { return null; }
    var block = detailBlock('会话');
    block.appendChild(kvList([
      ['项目', cur.project || projectOf(cur.id), cur.id],
      ['阶段', cur.stageTitle || cur.stage || '—'],
      ['会话', sessionTitleOf(cur), cur.id, true],
      ['文件', cur.name || '—', cur.path, true],
      ['消息', cur.messages + ' 条 · ' + state.lines.length + ' 行'],
      ['大小', fmtSize(cur.size)],
      ['最后写入', fmtClock(cur.mtime)],
      cur.imageName ? ['图片', cur.imageName, null, true] : null,
      cur.imageName && cur.page ? ['页码', '第 ' + cur.page + ' 页'] : null,
      cur.imageName && cur.imageOrder ? ['顺序', '书内第 ' + cur.imageOrder + ' 张'] : null,
      cur.imageName && cur.imageType ? ['类型', cur.imageType] : null,
      cur.imageName && cur.imageCaption ? ['图注', cur.imageCaption] : null
    ]));
    var sub = subPathOf(cur.id);
    if (sub) { block.appendChild(el('div', 'stats-sub', '目录：' + sub)); }
    return block;
  }

  function detailsStatsBlock() {
    var st = aggregate(usageLines(state.lines));
    if (!st) { return null; }
    var block = detailBlock('指标');
    var meta = el('div', 'stats-sub', st.requests + ' 次请求 · ' + (st.streamed ? '流式' : '非流式') +
      (st.spanMs ? ' · 会话跨度 ' + fmtDur(st.spanMs) : ''));
    block.appendChild(meta);

    var tiles = el('div', 'tiles');
    var tile = function (label, value, title) {
      var t = el('div', 'tile');
      t.appendChild(el('div', 'tile-value', value));
      t.appendChild(el('div', 'tile-label', label));
      if (title) { t.title = title; }
      tiles.appendChild(t);
    };
    tile('输入 tokens', fmtTokens(st.promptTokens),
      st.promptTokens + ' prompt tokens（含缓存命中 ' + st.cachedTokens + '）');
    tile('缓存命中', st.promptTokens ? st.cacheHitPct.toFixed(0) + '%' : '—',
      '前缀缓存命中率 = Σcached_tokens / Σprompt_tokens（供应商未上报时为 —）');
    tile('输出 tokens', fmtTokens(st.completionTokens),
      st.completionTokens + ' completion tokens' + (st.reasoningTokens ? '，其中思考 ' + st.reasoningTokens : ''));
    tile('平均首字', st.avgTtftMs ? fmtDur(st.avgTtftMs) : '—', '每请求"发出→第一个流式增量"的平均耗时');
    tile('输出速度', (st.outputTps || 0).toFixed(1) + ' tok/s',
      '生成速度 = Σ输出 tokens / Σ(请求耗时 − 首字延迟)，不含排队与思考等待');
    tile('平均耗时', fmtDur(st.avgDurationMs), '每请求平均墙钟耗时（含思考与工具执行前后的等待）');
    if (st.reasoningTokens) { tile('思考 tokens', fmtTokens(st.reasoningTokens), 'reasoning_tokens（思考链）'); }
    var sessionCost = state.current && state.current.cost;
    if (sessionCost) {
      tile('费用', fmtCost(sessionCost, false),
        '按配置里的 models.*.price 计算：未命中缓存的输入 × input + 命中缓存的输入 × cached + 输出 × output。' +
        '没配价格的模型不显示金额（¥0 会被读成"没花钱"）。');
    }
    block.appendChild(tiles);

    var details = document.createElement('details');
    details.className = 'stats-details';
    details.open = true;
    var sum = el('summary', 'schema-head');
    sum.appendChild(el('span', 'schema-name', '每次请求明细'));
    sum.appendChild(el('span', 'schema-meta', st.requests + ' 行'));
    details.appendChild(sum);

    var scroll = el('div', 'stats-scroll');
    var table = el('table', 'stats-table');
    var thead = el('tr');
    // 详情栏很窄（300–520px），所以每次请求只留最能说明问题的五列：首字 /
    // 耗时 / 输入（带缓存命中）/ 输出。种类、速度、结尾、模型进行内 tooltip。
    ['回合', '首字', '耗时', '输入·缓存', '输出'].forEach(function (h) {
      thead.appendChild(el('th', null, h));
    });
    table.appendChild(thead);
    st.perRequest.forEach(function (l) {
      var one = l.stats;
      var tr = el('tr');
      tr.className = 'req-row';
      var kindLabel = one.kind === 'compact' ? '上下文压缩摘要请求'
        : one.kind === 'nudge' ? '空回复后的强制文本请求' : '普通对话回合';
      tr.title = (l.ts ? fmtClock(l.ts) + '\n' : '') + kindLabel +
        (one.kind ? '（kind=' + one.kind + '）' : '') +
        (one.model ? '\n模型 ' + one.model : '') +
        '\n输入 ' + one.promptTokens + ' tokens（缓存命中 ' + one.cachedTokens + '）' +
        '\n输出 ' + one.completionTokens + ' tokens' +
        (one.reasoningTokens ? '（其中思考 ' + one.reasoningTokens + '）' : '') +
        '\n输出速度 ' + (one.outputTps || 0).toFixed(1) + ' tok/s' +
        '\n结束原因 ' + (one.finish || '—');
      var cell = function (text) {
        var td = el('td', null, text);
        tr.appendChild(td);
        return td;
      };
      cell(one.round ? '#' + one.round : '—');
      cell(one.ttftMs ? fmtDur(one.ttftMs) : '—');
      cell(one.durationMs ? fmtDur(one.durationMs) : '—');
      cell(fmtTokens(one.promptTokens) +
        (one.promptTokens ? ' · ' + (one.cachedTokens * 100 / one.promptTokens).toFixed(0) + '%' : ''));
      cell(fmtTokens(one.completionTokens));
      table.appendChild(tr);
    });
    scroll.appendChild(table);
    details.appendChild(scroll);
    block.appendChild(details);
    return block;
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
    card.className = 'disclosure meta-card';
    // 默认**收起**：摘要行已经写明模型/会话/哈希/字符数，一次点击才展开
    // 全文——系统提示词动辄几万字符，默认展开会把详情栏整个顶下去。
    // 展开状态按会话 + 提示词哈希记忆（用户展开过一次后保持）。
    card.open = storeGet(metaCardKey(line)) === '1';

    var head = el('summary');
    var slot = el('span', 'line-slot');
    slot.appendChild(el('span', 'line-caret'));
    head.appendChild(slot);
    head.appendChild(el('span', 'line-name', '系统提示词（本次运行快照，不参与回放）'));
    head.appendChild(el('span', 'line-sep'));
    var bits = ['模型 ' + (line.model || '—')];
    if (line.session_label) { bits.push('会话 ' + line.session_label); }
    bits.push('sha ' + shortSHA(line.system_sha));
    bits.push(m.promptChars + ' 字符');
    head.appendChild(el('span', 'line-summary', bits.join(' · ')));
    card.appendChild(head);

    var body = el('div', 'schema-body');
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
    if (m.count > 1) {
      var note = el('div', 'note', '共 ' + m.count + ' 条，显示最新');
      note.title = '同一个转录里有 ' + m.count + ' 条 meta 行（多次运行 / 提示词变化各一条），这里显示最后一条。';
      body.appendChild(note);
    }
    card.appendChild(body);

    card.addEventListener('toggle', function () {
      storeSet(metaCardKey(line), card.open ? '1' : '0');
    });
    return card;
  }

  /*
   * 详情栏 = DSH 的 details 栏：会话信息 + 指标 + 元信息（系统提示词快照与
   * 工具清单）。消息流里因此只剩对话本身，开头最多留一行极简摘要。
   */
  function renderDetails() {
    clear(refs.detailsBody);
    var cur = state.current;
    if (!cur) {
      refs.detailsBody.appendChild(el('div', 'note', '左侧选择一个会话后，这里显示它的指标与元信息。'));
      return;
    }
    var blocks = [detailsSessionBlock(), detailsStatsBlock()];
    var meta = metaCard();
    if (meta) {
      var block = detailBlock('元信息');
      block.appendChild(meta);
      blocks.push(block);
    }
    blocks.forEach(function (b) { if (b) { refs.detailsBody.appendChild(b); } });
    if (!blocks[0] && !blocks[1] && !blocks[2]) {
      refs.detailsBody.appendChild(el('div', 'note', '这个会话没有可显示的详情。'));
    }
  }

  /* ---------- 对话流（DSH 的消息形态） ---------- */

  function roleName(role) {
    if (role === 'user') { return '用户'; }
    if (role === 'assistant') { return 'AI'; }
    if (role === 'tool') { return '工具结果'; }
    if (role === 'system') { return '系统'; }
    return role || '未知';
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
   * 折叠行（思考 / 工具调用）折叠时只占一行：
   *   [16px 图标位（含折叠箭头）] 名称 13px ·(2px 圆点) 一行摘要 13px 省略号
   * 展开后思考缩进 22px；工具调用展开成「输入 / 输出」两段的卡片。
   */
  function disclosureLine(cls, name, summary, tail, open) {
    var details = document.createElement('details');
    details.className = 'disclosure ' + cls;
    details.open = !!open;
    var head = el('summary');
    var slot = el('span', 'line-slot');
    slot.appendChild(el('span', 'line-caret'));
    head.appendChild(slot);
    head.appendChild(el('span', 'line-name', name));
    if (summary) {
      head.appendChild(el('span', 'line-sep'));
      var s = el('span', 'line-summary', summary);
      s.title = summary;
      head.appendChild(s);
    }
    if (tail) { head.appendChild(el('span', 'line-tail', tail)); }
    details.appendChild(head);
    return details;
  }

  function thinkingDisclosure(line) {
    var remembered = storeGet('thinking.' + state.current.id + '.' + line.n) === '1';
    var d = disclosureLine('disclosure-thinking', '思考', firstLine(line.reasoning),
      line.reasoning.length + ' 字符', !state.forceCollapse && remembered);
    var body = el('div', 'thinking-body');
    var scroll = el('div', 'reasoning-scroll');
    scroll.appendChild(el('pre', 'body-text reasoning-text', line.reasoning));
    body.appendChild(scroll);
    d.appendChild(body);
    d.addEventListener('toggle', function () {
      if (state.forceCollapse) { return; }
      storeSet('thinking.' + state.current.id + '.' + line.n, d.open ? '1' : '0');
    });
    return d;
  }

  /*
   * 工具家族：DSH 的调用行一眼能分出"读/写/跑命令/检索"，这里也用同一个
   * 思路——把工具名归成几族，只给工具名上色（不用左侧色条）。
   */
  function toolFamily(name) {
    var n = String(name || '').toLowerCase();
    if (n === 'bash' || n === 'compile' || n === 'python') { return 'shell'; }
    if (n === 'submit') { return 'submit'; }
    if (n.indexOf('write') === 0 || n.indexOf('edit') === 0) { return 'write'; }
    if (n.indexOf('grep') === 0 || n.indexOf('doc_search') === 0 ||
        n.indexOf('list_') === 0 || n.indexOf('search') >= 0) { return 'search'; }
    if (n.indexOf('read') === 0 || n.indexOf('view_') === 0 || n.indexOf('image_context') === 0) { return 'view'; }
    return 'other';
  }

  /*
   * 调用的"一行摘要"：参数里最能说明这次调用在干什么的那一个（命令/path/
   * pattern/query），折叠状态下也能读懂调用过程，不必逐个展开 JSON。
   */
  function toolSummary(name, argsText) {
    var obj = null;
    try { obj = JSON.parse(String(argsText || '{}')); } catch (e) { obj = null; }
    if (!obj || typeof obj !== 'object') { return ''; }
    var n = String(name || '').toLowerCase();
    var pick = function (v) {
      if (typeof v === 'string') { return v; }
      if (v === undefined || v === null) { return ''; }
      try { return JSON.stringify(v); } catch (e) { return ''; }
    };
    var v = '';
    if (n === 'bash' || n === 'python') { v = pick(obj.command || obj.code || obj.script); }
    else if (n === 'compile') { v = pick(obj.path); }
    else if (n.indexOf('grep') === 0 || n.indexOf('search') >= 0 || n === 'doc_search') { v = pick(obj.pattern || obj.query); }
    else if (n.indexOf('write') === 0 || n.indexOf('read') === 0 || n === 'view_pdf' || n === 'view_image') { v = pick(obj.path); }
    else if (n === 'submit') { v = pick(obj.path || obj.status); }
    if (!v) {
      // 兜底：参数里第一个非空字符串（顺序与模型给的参数顺序一致）。
      var keys = Object.keys(obj);
      for (var i = 0; i < keys.length; i++) {
        var cand = pick(obj[keys[i]]);
        if (cand) { v = cand; break; }
      }
    }
    v = String(v).split('\n')[0].replace(/\s+/g, ' ').trim();
    return v.length > 90 ? v.slice(0, 90) + '…' : v;
  }

  function classifyResult(text) {
    var s = String(text || '');
    if (/REJECTED|文件不存在|失败|error|not found|traceback/i.test(s)) { return 'error'; }
    if (/\bok\s*\(/.test(s)) { return 'ok'; }
    return 'plain';
  }

  function ioSection(label, text, isError, key) {
    var section = el('div', 'io-section');
    section.appendChild(el('div', 'io-label', label));
    if (text === undefined || text === null || text === '') {
      section.appendChild(el('div', 'io-empty', '（无内容）'));
      return section;
    }
    var wrap;
    if (key) {
      wrap = collapsibleText(text, PREVIEW_LINES, key, 'io-text');
    } else {
      wrap = el('pre', 'io-text');
      appendHighlighted(wrap, text);
    }
    if (isError) {
      var pre = wrap.tagName === 'PRE' ? wrap : wrap.querySelector('.io-text');
      if (pre) { pre.setAttribute('data-error', 'true'); }
    }
    section.appendChild(wrap);
    return section;
  }

  /*
   * 工具调用行：折叠时一行（图标位 + 工具名 + 圆点 + 摘要），展开时是
   * 「输入 / 输出」两段的卡片（0.5px 边框、代码底色、12px 圆角、260px 内滚）。
   * 输出来自配对的那条 role="tool" 行——两种模式共用同一条路径：直播模式里
   * 结果晚到，就补进同一个节点。
   */
  function toolDisclosure(call) {
    var fn = call.function || {};
    var name = fn.name || '(未命名工具)';
    state.toolSeq++;
    state.calls.set(call.id, { name: name, seq: state.toolSeq });

    var argsText = String(fn.arguments || '');
    var brief = toolSummary(name, fn.arguments);
    var d = disclosureLine('disclosure-tool fam-' + toolFamily(name), name, brief,
      argsText.length + ' 字符', storeGet('call.' + state.current.id + '.' + call.id) === '1');
    d.setAttribute('data-call-id', call.id || '');
    var tail = d.querySelector('.line-tail');

    var card = el('div', 'io-card');
    card.appendChild(ioSection('输入', prettyJSON(argsText) || '(无参数)'));
    var actions = el('div', 'io-actions');
    actions.appendChild(copyButton(argsText));
    card.appendChild(actions);
    d.appendChild(card);
    d.addEventListener('toggle', function () {
      storeSet('call.' + state.current.id + '.' + call.id, d.open ? '1' : '0');
    });

    var node = { details: d, card: card, summary: brief, tail: tail, argsChars: argsText.length, result: null };
    if (call.id) { state.callNodes[call.id] = node; }
    return node;
  }

  // 把一条工具结果补进它对应的调用卡片（或独立成行——孤儿结果不丢）。
  // 折叠行的尾巴换成「输入 → 输出」的字符数 + 状态：一行里就能看出这次调用
  // 吃了多少、回了多少、成没成，不必展开。
  function attachResult(node, line) {
    var text = String(line.text || '');
    var status = classifyResult(text);
    node.result = { line: line, status: status };
    node.card.appendChild(el('div', 'io-divider'));
    node.card.appendChild(ioSection('输出', text, status === 'error', 'result.' + line.n));
    var actions = el('div', 'io-actions');
    actions.appendChild(copyButton(text));
    node.card.appendChild(actions);
    node.details.classList.add('status-' + status);
    if (node.tail) {
      node.tail.textContent = node.argsChars + ' → ' + text.length + ' 字符' +
        (status === 'error' ? ' · error' : status === 'ok' ? ' · ok' : '');
    }
    return node;
  }

  function standaloneResult(line) {
    var info = state.calls.get(line.tool_call_id);
    var name = info ? info.name : '(未配对的工具调用)';
    var text = String(line.text || '');
    var status = classifyResult(text);
    var d = disclosureLine('disclosure-result status-' + status,
      name, firstLine(text), text.length + ' 字符' + (status === 'error' ? ' · error' : status === 'ok' ? ' · ok' : ''));
    var card = el('div', 'io-card');
    card.appendChild(ioSection('输出', text, status === 'error', 'result.' + line.n));
    var actions = el('div', 'io-actions');
    actions.appendChild(copyButton(text));
    card.appendChild(actions);
    d.appendChild(card);
    if (!info && line.tool_call_id) { d.title = '未找到配对的工具调用：id ' + line.tool_call_id; }
    return d;
  }

  function anchor(node, line) {
    if (line && line.n !== undefined) { state.anchors[line.n] = node; }
    return node;
  }

  /* Returns the element for one transcript line, or null when it is skipped. */
  function renderLine(line) {
    if (line.bad) {
      state.badLines++;
      updateBanner();
      return null;
    }
    // meta 行不是消息：它由详情栏的元信息块渲染，绝不进消息序列。
    if (line.t && line.t !== 'msg') { return null; }
    if (!line.role) { return null; }

    var isToolCall = line.role === 'assistant' && line.tool_calls && line.tool_calls.length > 0;
    if (state.onlyTools && line.role !== 'tool' && !isToolCall) { return null; }

    if (line.role === 'user') {
      var userMsg = el('section', 'msg msg-user');
      var bubble = el('div', 'bubble');
      if (line.text) { bubble.appendChild(collapsibleText(line.text, LONG_TEXT_LINES, 'user.' + line.n)); }
      userMsg.appendChild(bubble);
      if (line.images && line.images.length) { userMsg.appendChild(imageStrip(line)); }
      return anchor(userMsg, line);
    }

    if (line.role === 'tool') {
      // 已配对的调用行已经带了输出，这一行就并入那张卡片（节点已在 DOM 里，
      // 这里返回 null，不要把 <details> 从助手消息里搬走）。
      var node = line.tool_call_id ? state.callNodes[line.tool_call_id] : null;
      if (node) {
        attachResult(node, line);
        var lines0 = node.details.getAttribute('data-lines');
        node.details.setAttribute('data-lines', lines0 ? (lines0 + ',' + line.n) : String(line.n));
        anchor(node.details, line);
        return null;
      }
      var standalone = el('section', 'msg msg-tool');
      var sd = standaloneResult(line);
      standalone.appendChild(sd);
      return anchor(standalone, line);
    }

    if (line.role === 'assistant') {
      var msg = el('section', 'msg msg-assistant');
      if (line.reasoning) {
        // 思考行不是独立步骤：轨迹里"思考"这一行跳回的就是这条助手消息。
        msg.appendChild(thinkingDisclosure(line));
      }
      if (line.text) { msg.appendChild(collapsibleText(line.text, LONG_TEXT_LINES, 'asst.' + line.n)); }
      (line.tool_calls || []).forEach(function (call) {
        msg.appendChild(toolDisclosure(call).details);
      });
      if (!line.text && !line.reasoning && !(line.tool_calls || []).length) {
        msg.appendChild(el('div', 'note', '（空消息）'));
      }
      return anchor(msg, line);
    }

    var other = el('section', 'msg msg-other');
    other.appendChild(collapsibleText(line.text || '(无正文)', LONG_TEXT_LINES, 'other.' + line.n));
    return anchor(other, line);
  }

  /*
   * 消息流开头只留一行极简摘要——指标与元信息都在右侧详情栏里，
   * 这里只写"这个会话有多大、花了多少"，点一下就能打开详情。
   */
  function streamSummary() {
    if (!state.current) { return null; }
    var bar = el('div', 'stream-summary');
    var bits = [state.current.messages + ' 条消息'];
    var st = aggregate(usageLines(state.lines));
    if (st) {
      bits.push('输入 ' + fmtTokens(st.promptTokens) + ' / 输出 ' + fmtTokens(st.completionTokens));
      if (st.promptTokens) { bits.push('缓存 ' + st.cacheHitPct.toFixed(0) + '%'); }
      if (st.avgTtftMs) { bits.push('首字 ' + fmtDur(st.avgTtftMs)); }
      var money = fmtCost(state.current.cost, false);
      if (money) { bits.push(money); }
    }
    var m = metaState();
    if (m) { bits.push('提示词快照 ' + m.promptChars + ' 字符'); }
    bits.forEach(function (b, i) {
      if (i) { bar.appendChild(el('span', 'dot-sep')); }
      bar.appendChild(el('span', null, b));
    });
    var btn = el('button', null, detailsVisible() ? '收起详情' : '详情 ›');
    btn.type = 'button';
    btn.addEventListener('click', function () {
      toggleDetails();
      btn.textContent = detailsVisible() ? '收起详情' : '详情 ›';
    });
    bar.appendChild(btn);
    return bar;
  }

  function timelineEmptyText() {
    if (state.onlyTools) { return '这个会话没有工具调用记录'; }
    if (!state.current) { return '左侧选择一个会话开始浏览。'; }
    return '这个会话还没有可显示的消息';
  }

  function streamNode() {
    return refs.timeline.querySelector('.stream');
  }

  function renderTimeline() {
    clear(refs.timeline);
    state.msgSeq = 0;
    state.toolSeq = 0;
    state.badLines = 0;
    state.calls = new Map();
    state.callNodes = {};
    state.anchors = {};

    var stream = el('div', 'stream');
    refs.timeline.appendChild(stream);
    var rendered = 0;
    var summary = streamSummary();
    if (summary) { stream.appendChild(summary); rendered++; }
    state.lines.forEach(function (line) {
      var node = renderLine(line);
      if (node) { stream.appendChild(node); rendered++; }
    });
    if (rendered <= (summary ? 1 : 0)) {
      stream.appendChild(el('div', 'empty', timelineEmptyText()));
    }
    updateBanner();
    renderHeader();
    renderDetails();
    if (state.view === 'trajectory') { renderTrajectory(); }
  }

  function appendLines(lines) {
    if (!lines || !lines.length) { return; }
    var stream = streamNode();
    var hasMeta = false;
    lines.forEach(function (line) {
      state.lines.push(line);
      if (line.t === 'meta') { hasMeta = true; }
    });
    if (!stream) {
      renderTimeline();
      return;
    }
    var placeholder = stream.querySelector('.empty');
    lines.forEach(function (line) {
      var node = renderLine(line);
      if (node) { stream.appendChild(node); }
    });
    if (placeholder && placeholder.parentNode && stream.children.length > 1) {
      stream.removeChild(placeholder);
    }
    updateBanner();
    renderHeader();
    if (hasMeta) { renderDetails(); }
    else if (lines.some(function (l) { return l.t === 'usage'; })) { renderDetails(); }
    if (state.view === 'trajectory') { renderTrajectory(); }
    if (state.follow) { scrollToBottom(); }
  }

  function scrollToBottom() {
    refs.timeline.scrollTop = refs.timeline.scrollHeight;
  }

  /* ---------- 轨迹（DSH Trajectory 的表） ---------- */

  /*
   * 把这次会话的全部步骤摊成一张表：序号 / 类型 / 工具名 / 一行摘要 /
   * 状态 / 字符数 / 耗时。点一行跳到对话里对应的那条消息；左侧的箭头展开
   * 完整的输入输出（不离开这个标签页）。
   */
  function trajectoryRows() {
    var rows = [];
    var callOf = {};
    state.lines.forEach(function (l) {
      if (!l || l.bad) { return; }
      if (l.t === 'msg' && l.role === 'assistant') {
        (l.tool_calls || []).forEach(function (c) {
          var fn = c.function || {};
          callOf[c.id] = { name: fn.name || '?', ts: l.ts || '', n: l.n, args: String(fn.arguments || '') };
        });
      }
    });

    state.lines.forEach(function (line) {
      if (!line || line.bad) { return; }
      if (line.t === 'meta') {
        rows.push({
          kind: 'meta', tag: '元信息', name: line.kind || 'system',
          summary: '模型 ' + (line.model || '—') + ' · 提示词 ' + String(line.text || '').length + ' 字符' +
            ((line.tools || []).length ? ' · 工具 ' + line.tools.length : ''),
          chars: String(line.text || '').length,
          status: '', detail: { prompt: String(line.text || '') }
        });
        return;
      }
      if (line.t === 'usage') {
        var st = line.stats || {};
        rows.push({
          kind: 'usage', tag: '用量', name: '请求' + (st.round ? ' #' + st.round : ''),
          summary: (st.kind || 'chat') + ' · 输入 ' + fmtTokens(st.promptTokens) + '（缓存 ' +
            (st.cachedTokens || 0) + '）· 输出 ' + fmtTokens(st.completionTokens) +
            (st.reasoningTokens ? '（思 ' + fmtTokens(st.reasoningTokens) + '）' : ''),
          chars: '',
          status: st.finish || '',
          time: Number(st.durationMs) || 0,
          detail: { request: JSON.stringify(st, null, 2) }
        });
        return;
      }
      if (line.t !== 'msg' || !line.role) { return; }

      if (line.reasoning) {
        rows.push({
          kind: 'think', tag: '思考', name: 'reasoning',
          summary: firstLine(line.reasoning), chars: line.reasoning.length,
          status: '', jump: line.n, detail: { thinking: line.reasoning }
        });
      }
      if (line.text) {
        var isTool = line.role === 'tool';
        var status = isTool ? classifyResult(line.text) : '';
        rows.push({
          kind: isTool ? 'result' : (line.role === 'user' ? 'user' : 'msg'),
          tag: isTool ? '结果' : (line.role === 'user' ? '用户' : '助手'),
          name: isTool ? ((callOf[line.tool_call_id] && callOf[line.tool_call_id].name) || 'tool') : roleName(line.role),
          summary: firstLine(line.text), chars: line.text.length,
          status: status, jump: line.n,
          time: isTool ? callDuration(callOf[line.tool_call_id], line) : 0,
          detail: isTool ? { output: String(line.text) } : (line.role === 'user' ? { user: String(line.text) } : { message: String(line.text) })
        });
      }
      (line.tool_calls || []).forEach(function (c) {
        var fn = c.function || {};
        var name = fn.name || '(未命名工具)';
        var args = String(fn.arguments || '');
        rows.push({
          kind: 'tool', tag: '工具', name: name,
          summary: toolSummary(name, fn.arguments), chars: args.length,
          status: '', jump: line.n,
          detail: { input: prettyJSON(args) || args }
        });
      });
    });
    return rows;
  }

  // 工具耗时 = 结果行时间戳 − 调用行时间戳（两边都有 ts 才算，不编数字）。
  function callDuration(call, resultLine) {
    if (!call || !call.ts || !resultLine.ts) { return 0; }
    var t0 = Date.parse(call.ts);
    var t1 = Date.parse(resultLine.ts);
    if (isNaN(t0) || isNaN(t1) || t1 < t0) { return 0; }
    return t1 - t0;
  }

  var TRAJ_KINDS = [
    { id: 'user', label: '用户' },
    { id: 'msg', label: '助手' },
    { id: 'think', label: '思考' },
    { id: 'tool', label: '工具' },
    { id: 'result', label: '结果' },
    { id: 'meta', label: '元信息' },
    { id: 'usage', label: '用量' }
  ];

  function trajVisible(row) {
    var picked = Object.keys(state.trajKinds).filter(function (k) { return state.trajKinds[k]; });
    if (!picked.length) { return true; }
    return picked.indexOf(row.kind) >= 0;
  }

  function trajDetailBody(row) {
    var body = el('div', 'traj-detail-inner');
    var labels = { prompt: '系统提示词', thinking: '思考', user: '用户消息', message: '助手消息', input: '输入', output: '输出', request: '用量行' };
    Object.keys(row.detail || {}).forEach(function (k) {
      var section = el('div');
      section.appendChild(el('div', 'traj-detail-title', labels[k] || k));
      var pre = el('pre', 'code');
      var text = String(row.detail[k] || '');
      if (k === 'input' || k === 'request') { appendHighlighted(pre, text); }
      else { pre.textContent = text; }
      section.appendChild(pre);
      var actions = el('div', 'row-actions');
      actions.appendChild(copyButton(text));
      section.appendChild(actions);
      body.appendChild(section);
    });
    return body;
  }

  function renderTrajectory() {
    clear(refs.trajectory);
    if (!state.current) {
      refs.trajectory.appendChild(el('div', 'traj-empty', '左侧选择一个会话后，这里列出它的全部步骤。'));
      return;
    }
    var rows = trajectoryRows();
    var visible = rows.filter(trajVisible);

    var toolbar = el('div', 'traj-toolbar');
    var inner = el('div', 'traj-toolbar-inner');
    var filters = el('div', 'traj-filters');
    var allChip = el('button', 'traj-chip', '全部');
    allChip.type = 'button';
    allChip.setAttribute('aria-pressed', Object.keys(state.trajKinds).length ? 'false' : 'true');
    allChip.addEventListener('click', function () {
      state.trajKinds = {};
      renderTrajectory();
    });
    filters.appendChild(allChip);
    TRAJ_KINDS.forEach(function (k) {
      var count = rows.filter(function (r) { return r.kind === k.id; }).length;
      if (!count) { return; }
      var chip = el('button', 'traj-chip', k.label + ' ' + count);
      chip.type = 'button';
      chip.title = '只看 / 不看「' + k.label + '」';
      chip.setAttribute('aria-pressed', state.trajKinds[k.id] ? 'true' : 'false');
      chip.addEventListener('click', function () {
        if (state.trajKinds[k.id]) { delete state.trajKinds[k.id]; }
        else { state.trajKinds[k.id] = true; }
        renderTrajectory();
      });
      filters.appendChild(chip);
    });
    inner.appendChild(filters);
    inner.appendChild(el('span', 'traj-count', visible.length + ' / ' + rows.length + ' 步'));
    toolbar.appendChild(inner);
    refs.trajectory.appendChild(toolbar);

    var scroll = el('div', 'traj-scroll');
    if (!visible.length) {
      scroll.appendChild(el('div', 'traj-empty', rows.length ? '当前筛选没有匹配的步骤' : '这个会话还没有步骤'));
      refs.trajectory.appendChild(scroll);
      return;
    }

    var table = el('table', 'traj-table');
    var colgroup = document.createElement('colgroup');
    [['col-n'], ['col-kind'], ['col-name'], [], ['col-status'], ['col-size'], ['col-time']].forEach(function (c) {
      var col = document.createElement('col');
      if (c[0]) { col.className = c[0]; }
      colgroup.appendChild(col);
    });
    table.appendChild(colgroup);
    var thead = el('thead');
    var hrow = el('tr');
    ['#', '类型', '名称', '摘要', '状态', '字符', '耗时'].forEach(function (h, i) {
      hrow.appendChild(el('th', (i === 0 || i >= 5) ? 'num-head' : null, h));
    });
    thead.appendChild(hrow);
    table.appendChild(thead);

    var tbody = el('tbody');
    visible.forEach(function (row, idx) {
      var key = 'r' + idx + ':' + (row.jump || row.name);
      var tr = el('tr', 'traj-row');
      tr.setAttribute('data-kind', row.kind);
      if (row.status === 'error') { tr.setAttribute('data-error', 'true'); }
      tr.title = '点击跳到对话里对应的那条消息';

      var tdN = el('td', 'traj-num');
      var discl = el('button', 'traj-disclose', state.trajOpen[key] ? '▾' : '▸');
      discl.type = 'button';
      discl.title = '展开完整输入输出';
      discl.addEventListener('click', function (ev) {
        ev.stopPropagation();
        state.trajOpen[key] = !state.trajOpen[key];
        renderTrajectory();
      });
      tdN.appendChild(discl);
      tdN.appendChild(document.createTextNode(row.jump ? String(row.jump) : '—'));
      tr.appendChild(tdN);

      var tdKind = el('td');
      tdKind.appendChild(el('span', 'kind-tag kind-' + (row.status === 'error' ? 'error' : row.kind), row.tag));
      tr.appendChild(tdKind);

      tr.appendChild(el('td', 'traj-name', row.name));
      var tdSum = el('td', 'traj-summary', row.summary || '—');
      tdSum.title = row.summary || '';
      tr.appendChild(tdSum);
      var statusText = row.status === 'error' ? '✗ error'
        : row.status === 'ok' ? '✓ ok'
          : (row.status === 'plain' || !row.status) ? '—' : row.status;
      tr.appendChild(el('td', 'traj-status ' + (row.status === 'error' ? 'error' : row.status === 'ok' ? 'ok' : 'plain'),
        statusText));
      tr.appendChild(el('td', 'traj-num-cell', row.chars ? String(row.chars) : '—'));
      tr.appendChild(el('td', 'traj-num-cell', row.time ? fmtDur(row.time) : '—'));

      tr.addEventListener('click', function () {
        if (row.jump) { jumpToLine(row.jump); }
        else { state.trajOpen[key] = !state.trajOpen[key]; renderTrajectory(); }
      });
      tbody.appendChild(tr);

      if (state.trajOpen[key]) {
        var dtr = el('tr', 'traj-detail');
        var td = el('td');
        td.colSpan = 7;
        td.appendChild(trajDetailBody(row));
        dtr.appendChild(td);
        tbody.appendChild(dtr);
      }
    });
    table.appendChild(tbody);
    scroll.appendChild(table);
    refs.trajectory.appendChild(scroll);
  }

  /* 轨迹 → 对话：切回对话标签页并滚到那一条，落点短暂高亮。 */
  function jumpToLine(n) {
    var node = state.anchors[n];
    switchView('chat');
    if (!node) { return; }
    node.scrollIntoView({ block: 'center' });
    node.classList.remove('flash');
    // 强制重排，让动画能从头上重放
    void node.offsetWidth;
    node.classList.add('flash');
  }

  /* ---------- data plumbing ---------- */

  function staticSession(id) {
    var found = null;
    (DATA.sessions || []).forEach(function (s) { if (s.id === id) { found = s; } });
    return found;
  }

  function selectSession(id) {
    var s = findSession(id);
    state.current = s;
    state.lines = [];
    state.nextFrom = 0;
    state.curSize = -1;
    state.curMtime = 0;
    state.meta = null;
    state.trajOpen = {};
    patchList();  // 只挪一下高亮，**不重建侧栏**（重建会把展开状态与滚动位置打断）
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
    updateRootLabel();
    // 签名没变 → 一行 DOM 都不重建（只修补相对时间与高亮），
    // 用户手动折叠的项目组 / 阶段组与滚动位置因此不会被每 2 秒的轮询冲掉。
    refreshList();
  }

  function refreshIndex() {
    if (MODE === 'static') {
      refreshList();
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
      var current = findSession(state.current ? state.current.id : '');
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
      renderHeader();
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

  function setToggle(btn, on) {
    if (!btn) { return; }
    btn.setAttribute('aria-pressed', on ? 'true' : 'false');
  }

  refs.refresh.addEventListener('click', function () { refreshIndex(); });
  refs.search.addEventListener('input', function () {
    state.filter = refs.search.value.trim().toLowerCase();
    refreshList();
  });
  refs.follow.addEventListener('click', function () {
    state.follow = !state.follow;
    setToggle(refs.follow, state.follow);
    if (state.follow) { scrollToBottom(); }
  });
  refs.collapseThinking.addEventListener('click', function () {
    state.forceCollapse = !state.forceCollapse;
    setToggle(refs.collapseThinking, state.forceCollapse);
    if (state.forceCollapse) {
      // 作用于消息流里所有思考行（含增量追加进来的）。
      Array.prototype.forEach.call(refs.timeline.querySelectorAll('details.disclosure-thinking'),
        function (d) { d.open = false; });
    }
  });
  refs.onlyTools.addEventListener('click', function () {
    state.onlyTools = !state.onlyTools;
    setToggle(refs.onlyTools, state.onlyTools);
    renderTimeline();
  });
  if (refs.themeToggle) { refs.themeToggle.addEventListener('click', toggleTheme); }
  if (refs.sideToggle) { refs.sideToggle.addEventListener('click', toggleSidebar); }
  if (refs.detailsToggle) { refs.detailsToggle.addEventListener('click', toggleDetails); }
  if (refs.detailsClose) { refs.detailsClose.addEventListener('click', function () { if (state.details > 0) { toggleDetails(); } }); }
  refs.tabs.chat.addEventListener('click', function () { switchView('chat'); });
  refs.tabs.trajectory.addEventListener('click', function () { switchView('trajectory'); });
  refs.timeline.addEventListener('scroll', function () {
    var near = refs.timeline.scrollHeight - refs.timeline.scrollTop - refs.timeline.clientHeight < 40;
    if (!near && state.follow) {
      state.follow = false;
      setToggle(refs.follow, false);
    }
  });
  refs.lightbox.addEventListener('click', closeLightbox);
  document.addEventListener('keydown', function (ev) {
    var tag = ev.target && ev.target.tagName;
    var typing = tag === 'INPUT' || tag === 'TEXTAREA' || (ev.target && ev.target.isContentEditable);
    if (ev.key === 'Escape') { closeLightbox(); }
    if (typing) { return; }
    if (ev.key === '[') { toggleSidebar(); }
    if (ev.key === ']') { toggleDetails(); }
  });

  if (window.ResizeObserver) {
    var raf = null;
    new ResizeObserver(function () {
      if (raf !== null) { return; }
      raf = requestAnimationFrame(function () {
        raf = null;
        applyLayout();
      });
    }).observe(refs.frame);
  } else {
    window.addEventListener('resize', applyLayout);
  }

  function boot() {
    loadState();
    applyTheme(storedTheme());
    wireHandle(refs.handleSidebar, 'sidebar');
    wireHandle(refs.handleDetails, 'details');
    applyLayout();
    setToggle(refs.follow, state.follow);
    setToggle(refs.collapseThinking, false);
    setToggle(refs.onlyTools, false);
    switchView('chat');

    if (MODE === 'static') {
      state.root = DATA.root || '';
      state.generated = DATA.generated || '';
      // 静态模式只丢掉每会话的 lines 大块，其余字段**原样带走**。
      // 这里以前是一个手写白名单：每加一个字段（projectLegacy、stage、
      // projectStages、imageOrder、cost…）都要记得补一行，漏掉页面就静默
      // 少一块 UI——侧栏的阶段分组、书级进展、图片名/页码、费用列在静态
      // 快照里全都不显示，而实时模式（--serve）正常。改成"只减字段"而不是
      // "列举字段"，这类漏项从此不会再有。
      state.sessions = (DATA.sessions || []).map(function (s) {
        var copy = {};
        Object.keys(s).forEach(function (k) {
          if (k !== 'lines') { copy[k] = s[k]; }
        });
        return copy;
      });
      updateRootLabel();
      refreshList();
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
