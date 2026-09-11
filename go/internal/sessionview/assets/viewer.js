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
  var LONG_TEXT_LINES = 20;
  // 系统消息（user 轮的任务提示）默认只露这么多行，其余折叠起来（可展开全文）。
  var SYSTEM_PREVIEW_LINES = 8;
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
    mdToggle: document.getElementById('md-toggle'),
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

  // 归属到「本会话任务」的图片轮，如果排在任务**前面**就先记在这里（见 renderLine）。
  var pendingTaskImages = {};

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
    // 消息正文的 Markdown 预览开关：默认开启，关掉回到纯文本 pre-wrap。
    markdown: true,
    // 侧栏 / 详情栏的宽度偏好：0 = 折叠（侧栏折成 56px 轨道，详情栏关掉）
    sidebar: SIDEBAR_DEFAULT,
    details: 0,
    narrowExpanded: false,
    narrow: false,
    // 组展开状态：key -> true（用户折叠过）/ false（用户展开过）。
    // **缺省即收起**：只有记忆里明确记着展开过、或这一组装着当前选中的会话才展开。
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

  /* 字符数的短写（给"内容过大"的提示用：200 KB 以上就按 KB 报）。 */
  function fmtChars(n) {
    if (!n) { return '0 字符'; }
    if (n < 1024) { return n + ' 字符'; }
    return (n / 1024).toFixed(1) + ' KB';
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
   *   collapsed = { "proj:<项目>": true|false, "stage:<项目>/<阶段>": true|false }
   * true = 用户手动折起来过，false = 用户手动展开过，**没有记录 = 默认收起**。
   * 两种选择都显式记下来：这样"当前会话所在组自动展开"既能生效，又不会把
   * 用户手动折起来的那个组在下次切换会话时又顶开。
   */
  function loadState() {
    state.collapsed = storeJSON('collapsed', {});
    state.overflow = storeJSON('overflow', {});
    state.markdown = storeGet('markdown') !== '0';
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

  // 记忆里到底有没有这一组的记录？（true/false 都算，缺省不算——缺省是"收起"）
  function hasCollapseMemory(key) {
    return Object.prototype.hasOwnProperty.call(state.collapsed, key);
  }

  function setCollapsed(key, collapsed) {
    state.collapsed[key] = !!collapsed;
    storeSet('collapsed', JSON.stringify(state.collapsed));
  }

  /*
   * 一个组要不要展开：
   *   · 记忆里有记录 → 就按记忆（用户折过就折着，用户展开过就开着）；
   *   · 记忆里没记录 → 默认**收起**，只有这一组装着当前选中的会话才展开
   *     （否则用户看不到"我现在在哪"）。
   * 过滤命中时由调用方强制展开（frozen），不进这里。
   */
  function groupWantOpen(key, holdsCurrent) {
    if (hasCollapseMemory(key)) { return !isCollapsed(key); }
    return !!holdsCurrent;
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

  /* 程序化地开/关一个已有的组：只动 open 属性，不重建、也不写回记忆。 */
  function setGroupOpen(node, open) {
    if (node.open === open) { return; }
    node.open = open;
    node.dataset.open = open ? '1' : '0';
  }

  /*
   * 切换会话时**只改 open、不重建侧栏**：当前选中会话所在的项目组与阶段组
   * 自动展开（记忆里明确折过它的除外），其余组回到"记忆 / 默认收起"。
   * 走这里而不是重建 DOM，是为了不打断滚动位置、悬停状态与用户刚点的折叠。
   */
  function syncGroupOpen() {
    if (state.filter) { return; }  // 过滤时全部强制展开（frozen），别跟它抢
    var cur = state.current;
    var proj = cur ? (cur.project || projectOf(cur.id)) : '';
    var stage = cur ? (cur.stage || 'session') : '';
    var nodes = refs.list.querySelectorAll('.proj-group, .stage-group');
    Array.prototype.forEach.call(nodes, function (node) {
      var name = node.getAttribute('data-project') || '';
      var stg = node.getAttribute('data-stage');
      var holdsGroup = !!proj && name === proj && (stg === null || stg === stage);
      var key = stg === null ? groupKey('proj', name) : groupKey('stage', name + '/' + stg);
      setGroupOpen(node, groupWantOpen(key, holdsGroup));
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

  /* ---------- JSON 高亮（词法扫描，不嵌套解析） ---------- */

  /*
   * JSON 高亮：把一串 JSON 文本渲染成
   *   span.k（键名）· span.s（字符串）· span.n（数字）· span.b（true/false/null）
   * 类名与配色沿用样式表里既有的 .k/.s/.n/.b 与 --key/--str/--num/--bool，
   * 浅色/深色两套自动生效，不新增配色体系。
   *
   * 只做**一层正则扫描**（不解析、不递归），并且一律用 createTextNode /
   * createElement 落树：值里带 `<script>` 也只是文本。超过阈值就退回纯文本——
   * 高亮是给人看的，为几十万字符的 payload 卡住整页不值。
   */
  var JSON_HL_MAX_CHARS = 200 * 1024;
  var JSON_HL_MAX_LINES = 4000;
  // 机器内容超过这么多行就认为"限高装不下"，给「展开全文」按钮（19px 行高 × 18 行
  // ≈ 340px，正好是 --code-scroll-h；这里略保守一点）。
  var IO_FOLD_LINES = 16;

  /* 文本是 JSON 就返回两空格缩进后的规范化文本，否则 null（非 JSON 不碰）。 */
  function jsonPretty(raw) {
    var s = String(raw === undefined || raw === null ? '' : raw).trim();
    if (!s || (s.charAt(0) !== '{' && s.charAt(0) !== '[')) { return null; }
    try { return JSON.stringify(JSON.parse(s), null, 2); } catch (err) { return null; }
  }

  function countLines(text) { return String(text || '').split('\n').length; }

  function jsonTooBig(text) {
    return text.length > JSON_HL_MAX_CHARS || countLines(text) > JSON_HL_MAX_LINES;
  }

  /* 词法着色：键名 / 字符串 / 字面量 / 数字，其余原样进文本节点。 */
  function appendJSONSpans(parent, text) {
    var re = /("(?:\\.|[^"\\])*")\s*:|("(?:\\.|[^"\\])*")|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?)/g;
    var last = 0;
    var m;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) { parent.appendChild(document.createTextNode(text.slice(last, m.index))); }
      var cls = m[1] !== undefined ? 'k' : m[2] !== undefined ? 's' : m[3] !== undefined ? 'b' : 'n';
      parent.appendChild(el('span', cls, m[0]));
      last = m.index + m[0].length;
    }
    if (last < text.length) { parent.appendChild(document.createTextNode(text.slice(last))); }
  }

  /*
   * 非 JSON 的机器文本（终端输出 / diff / 编译日志）的着色。全部只产出
   * span + 文本节点（先转义再构树在这里就是 createTextNode），规则尽量少：
   *   · 整行：`$ `/`# ` 命令行、diff 的 `+`/`-`/`@@`/文件头、LaTeX 的 `!` 错误行、
   *     Overfull/Underfull 警告；
   *   · 词级：error/FAIL/warning/OK/PASS… 状态词，URL 与文件路径（弱强调）。
   * 配色只用既有的 --err/--warn/--ok/--accent/--caption，不新增色板。
   */
  function consoleLineClass(line) {
    if (/^\s*(\$|#)\s+\S/.test(line)) { return 'cmd'; }
    if (/^\+\+\+|^---\s/.test(line)) { return 'diffhead'; }
    if (/^@@/.test(line)) { return 'hunk'; }
    if (/^\+/.test(line)) { return 'add'; }
    if (/^-/.test(line)) { return 'del'; }
    if (/^\s*!/.test(line)) { return 'bad'; }
    if (/Overfull|Underfull/.test(line)) { return 'warn'; }
    return '';
  }

  var CONSOLE_TOKENS = /(https?:\/\/[^\s"'<>]+)|((?:\.{0,2}\/|\/)[\w.\-]+\/[\w.\-/]*[\w.\-])|(\b(?:ERROR|Error|error|FAILED|FAIL|Failure|failed|FATAL|Fatal)\b)|(\b(?:WARNING|Warning|warning|WARN|Warn|OVERFULL|Overfull|UNDERFULL|Underfull)\b)|(\b(?:OK|PASS|PASSED|COMPILE OK|SUCCESS|Success|done)\b)/g;

  function appendConsoleLine(parent, line) {
    var whole = consoleLineClass(line);
    if (whole) { parent.appendChild(el('span', whole, line)); return; }
    var last = 0;
    var m;
    CONSOLE_TOKENS.lastIndex = 0;
    while ((m = CONSOLE_TOKENS.exec(line)) !== null) {
      if (m.index > last) { parent.appendChild(document.createTextNode(line.slice(last, m.index))); }
      var cls = m[1] || m[2] ? 'path' : m[3] ? 'bad' : m[4] ? 'warn' : 'good';
      parent.appendChild(el('span', cls, m[0]));
      last = m.index + m[0].length;
      if (m[0] === '') { break; }
    }
    if (last < line.length) { parent.appendChild(document.createTextNode(line.slice(last))); }
  }

  function appendConsoleSpans(parent, text) {
    String(text === undefined || text === null ? '' : text).split('\n').forEach(function (line, i) {
      if (i) { parent.appendChild(document.createTextNode('\n')); }
      appendConsoleLine(parent, line);
    });
  }

  /*
   * 把一段机器文本放进 parent（<pre> 之类）：JSON 就做 JSON 高亮，否则按终端
   * 输出着色；两者都过阈值就退回纯文本。返回 { highlighted, note }。
   */
  function highlightMachine(parent, raw) {
    var text = String(raw === undefined || raw === null ? '' : raw);
    var pretty = jsonPretty(text);
    if (pretty === null) {
      if (jsonTooBig(text)) {
        parent.textContent = text;
        return {
          highlighted: false,
          note: '内容过大（' + fmtChars(text.length) + '），已按纯文本显示，不做高亮'
        };
      }
      appendConsoleSpans(parent, text);
      return { highlighted: true, note: '' };
    }
    if (jsonTooBig(pretty)) {
      parent.textContent = pretty;
      return {
        highlighted: false,
        note: '内容过大（' + fmtChars(pretty.length) + '），已按纯文本显示，不做 JSON 高亮'
      };
    }
    appendJSONSpans(parent, pretty);
    return { highlighted: true, note: '' };
  }

  /* 不带折叠的机器文本块（详情栏的 parameters、轨迹的输入输出）。 */
  function machineBlock(text, cls) {
    var frag = document.createDocumentFragment();
    var pre = el('pre', cls || 'code');
    var res = highlightMachine(pre, text);
    frag.appendChild(pre);
    if (res.note) { frag.appendChild(el('div', 'note', res.note)); }
    return frag;
  }

  /*
   * 工具输入输出卡片的正文：**与思考块同一套**——固定高度内滚（--code-scroll-h，
   * 与 reasoning-scroll 同值），内容超出限高时给「展开全文（N 行 / M 字符）」，
   * 点掉高度限制直接看全文，再点「收起」（按钮文案与折叠记忆键都沿用既有的那套）。
   * 高亮与 Markdown 开关无关：这里永远高亮，关掉 Markdown 也一样。
   */
  function machineScroll(text, key, extraClass) {
    var wrap = el('div', 'text-wrap');
    var raw = String(text === undefined || text === null ? '' : text);
    var pretty = jsonPretty(raw);
    var body = pretty === null ? raw : pretty;
    var big = jsonTooBig(body);
    var lines = body.split('\n');
    var scroll = el('div', 'io-scroll' + (extraClass ? ' ' + extraClass : ''));
    var pre = el('pre', 'io-text');
    var expanded = storeGet('text.' + key) === '1';
    var long = lines.length > IO_FOLD_LINES;
    var paint = function () {
      clear(pre);
      if (big) { pre.textContent = body; }
      else if (pretty !== null) { appendJSONSpans(pre, body); }
      else { appendConsoleSpans(pre, body); }
      scroll.classList.toggle('open', expanded);
    };
    paint();
    scroll.appendChild(pre);
    wrap.appendChild(scroll);
    if (big) {
      wrap.appendChild(el('div', 'note', '内容过大（' + fmtChars(body.length) + '），按纯文本显示，不做高亮'));
    }
    if (long) {
      var toggle = el('button', 'text-toggle');
      toggle.type = 'button';
      var label = function () {
        toggle.textContent = foldLabel(expanded, lines.length, body.length);
      };
      label();
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

  /* ---------- Markdown（自带的小渲染器，无外部依赖） ---------- */

  /*
   * 消息正文支持 Markdown 预览：**自己写的小渲染器**，不引任何库、不加任何外链。
   *
   * 安全边界：整棵树只用 DOM API 造（createElement / createTextNode），**从不碰
   * innerHTML**——转录里的 `<script>`、`<img onerror=…>` 落树时就是一个文本节点，
   * 浏览器不会把它当标签解析，页面里因此"原样显示为文本"；链接另经 mdSafeURL()
   * 过滤协议，`javascript:` / `data:` 一律不生成 href。
   *
   * 支持范围（够用为准）：标题 `#`~`######`、粗体/斜体/删除线、行内代码、围栏
   * 代码块（带语言标签与复制按钮）、有序/无序列表（一级嵌套）、引用、水平线、
   * 链接、`|` 表格、段落与软换行。
   */

  // 链接协议白名单：http(s)/mailto/锚点/相对路径放行，其余带协议的（javascript:、
  // data:、vbscript:…）不生成可点击地址——只留文字。
  function mdSafeURL(url) {
    var s = String(url === undefined || url === null ? '' : url).trim();
    if (!s) { return ''; }
    if (/^(https?:|mailto:|#|\/|\.\/|\.\.\/)/i.test(s)) { return s; }
    if (/^[a-z][a-z0-9+.-]*:/i.test(s)) { return ''; }
    return s;
  }

  function mdFence(line) {
    var m = /^\s{0,3}(`{3,}|~{3,})\s*([^\s`]*)\s*$/.exec(line);
    return m ? { mark: m[1].charAt(0), lang: m[2] || '' } : null;
  }

  function mdListMarker(line) {
    var m = /^([ \t]*)([-*+]|\d{1,3}[.)])\s+(.*)$/.exec(line);
    if (!m) { return null; }
    return {
      indent: m[1].replace(/\t/g, '  ').length,
      ordered: /\d/.test(m[2]),
      text: m[3]
    };
  }

  var MD_HR = /^(?:\*\s*){3,}$|^(?:-\s*){3,}$|^(?:_\s*){3,}$/;
  var MD_HEADING = /^(#{1,6})\s+(.*?)\s*#*\s*$/;

  // 表格分隔行（`|---|:--:|` 之类）；单独一行 `---` 是水平线，不走这里。
  function mdSeparatorRow(line) {
    var s = String(line || '').trim();
    if (s.indexOf('-') < 0 || s.indexOf('|') < 0) { return false; }
    return /^\|?[\s:|-]+\|?$/.test(s);
  }

  function mdSplitRow(line) {
    var s = String(line || '').trim().replace(/^\|/, '').replace(/\|$/, '');
    return s.split('|').map(function (c) { return c.trim(); });
  }

  // 这一行会不会开启一个新块？（段落收集时用来判断在哪里停下）
  function mdBlockStart(line, next) {
    var t = String(line || '').trim();
    if (!t) { return true; }
    if (mdFence(line) || MD_HEADING.test(t) || MD_HR.test(t)) { return true; }
    if (/^\s{0,3}>/.test(line) || mdListMarker(line)) { return true; }
    return t.indexOf('|') >= 0 && !!next && mdSeparatorRow(next) && mdSplitRow(t).length > 1;
  }

  /*
   * 行内：先认行内代码（里面一律字面量），再链接、粗体、删除线、斜体。
   * 一律用 textContent / createTextNode 落树，任何位置都不会产生元素。
   */
  var MD_INLINE = /(`+)([^`]*?)\1|\[([^\]]*)\]\(([^)\s]*)\)|\*\*([^*]+)\*\*|__([^_]+)__|~~([^~]+)~~|\*([^*\n]+)\*|_([^_\n]+)_/;

  function mdInline(parent, text, depth) {
    if ((depth || 0) > 6) { parent.appendChild(document.createTextNode(String(text || ''))); return; }
    var rest = String(text === undefined || text === null ? '' : text);
    var guard = 0;
    while (rest && guard++ < 800) {
      var m = MD_INLINE.exec(rest);
      if (!m) { break; }
      if (m.index > 0) { parent.appendChild(document.createTextNode(rest.slice(0, m.index))); }
      rest = rest.slice(m.index + m[0].length);
      var node;
      if (m[1] !== undefined) {
        // 行内代码：内容原样进文本节点（含 < > & ，不会被当成标签）
        node = el('code', 'md-inline-code', m[2]);
      } else if (m[3] !== undefined) {
        node = el('a', 'md-link');
        var href = mdSafeURL(m[4]);
        if (href) {
          node.setAttribute('href', href);
          node.setAttribute('target', '_blank');
          node.setAttribute('rel', 'noopener noreferrer');
        } else {
          node.title = '链接协议不受支持，只显示文字';
        }
        mdInline(node, m[3], (depth || 0) + 1);
      } else if (m[5] !== undefined || m[6] !== undefined) {
        node = el('strong');
        mdInline(node, m[5] !== undefined ? m[5] : m[6], (depth || 0) + 1);
      } else if (m[7] !== undefined) {
        node = el('del');
        mdInline(node, m[7], (depth || 0) + 1);
      } else {
        node = el('em');
        mdInline(node, m[8] !== undefined ? m[8] : m[9], (depth || 0) + 1);
      }
      parent.appendChild(node);
    }
    if (rest) { parent.appendChild(document.createTextNode(rest)); }
  }

  /* 块内的软换行按可见换行处理（转录里一行就是一行，不合并成空格）。 */
  function mdInlineLines(parent, text) {
    String(text === undefined || text === null ? '' : text).split('\n').forEach(function (part, i) {
      if (i) { parent.appendChild(el('br', 'md-br')); }
      mdInline(parent, part, 0);
    });
  }

  /*
   * 围栏代码块：顶部 banner（语言名 + 复制按钮）+ 等宽正文，圆角用 --radius，
   * 内容 11px/19px、单块最大高度内滚——与工具 IO 卡片同一套做法与同一套 token。
   */
  function mdCodeBlock(code, lang) {
    var wrap = el('div', 'md-code-block');
    var head = el('div', 'md-code-head');
    head.appendChild(el('span', 'md-code-lang', lang || 'text'));
    head.appendChild(copyButton(code));
    wrap.appendChild(head);
    var body = el('pre', 'md-code-body');
    var inner = el('code');
    // 代码块内容也走机器文本渲染：```json 做 JSON 高亮，```bash / ```diff / ```log
    // 按终端与 diff 着色，其余纯文本。
    var res = highlightMachine(inner, code);
    body.appendChild(inner);
    wrap.appendChild(body);
    if (res.note) { wrap.appendChild(el('div', 'note', res.note)); }
    return wrap;
  }

  function mdTable(header, rows) {
    var wrap = el('div', 'md-table-wrap');
    var table = el('table', 'md-table');
    var thead = el('thead');
    var hrow = el('tr');
    header.forEach(function (cell) {
      var th = el('th');
      mdInline(th, cell, 0);
      hrow.appendChild(th);
    });
    thead.appendChild(hrow);
    table.appendChild(thead);
    var tbody = el('tbody');
    rows.forEach(function (cells) {
      var tr = el('tr');
      for (var c = 0; c < header.length; c++) {
        var td = el('td');
        mdInline(td, cells[c] === undefined ? '' : cells[c], 0);
        tr.appendChild(td);
      }
      tbody.appendChild(tr);
    });
    table.appendChild(tbody);
    wrap.appendChild(table);
    return wrap;
  }

  function mdListItem(item) {
    var li = el('li', 'md-item');
    mdInlineLines(li, item.text);
    return li;
  }

  /*
   * 块级解析：围栏代码块 → 标题 → 水平线 → 表格 → 引用 → 列表 → 段落。
   * 返回 DocumentFragment（调用方直接 appendChild 即可）。
   */
  function renderMarkdown(text) {
    var frag = document.createDocumentFragment();
    var lines = String(text === undefined || text === null ? '' : text).replace(/\r\n?/g, '\n').split('\n');
    var i = 0;

    while (i < lines.length) {
      var line = lines[i];
      var trimmed = line.trim();
      if (!trimmed) { i++; continue; }

      // 围栏代码块（收尾围栏用同一个字符、长度不限；没闭合就吃到文末）
      var fence = mdFence(line);
      if (fence) {
        var code = [];
        i++;
        var close = new RegExp('^\\s{0,3}' + (fence.mark === '`' ? '`' : '~') + '{3,}\\s*$');
        while (i < lines.length && !close.test(lines[i])) { code.push(lines[i]); i++; }
        if (i < lines.length) { i++; }
        frag.appendChild(mdCodeBlock(code.join('\n'), fence.lang));
        continue;
      }

      var h = MD_HEADING.exec(trimmed);
      if (h) {
        var level = h[1].length;
        var heading = el('h' + level, 'md-h md-h' + level);
        mdInlineLines(heading, h[2]);
        frag.appendChild(heading);
        i++;
        continue;
      }

      if (MD_HR.test(trimmed)) {
        frag.appendChild(el('hr', 'md-hr'));
        i++;
        continue;
      }

      if (trimmed.indexOf('|') >= 0 && i + 1 < lines.length && mdSeparatorRow(lines[i + 1])) {
        var header = mdSplitRow(trimmed);
        if (header.length > 1) {
          var rows = [];
          i += 2;
          while (i < lines.length && lines[i].trim() && lines[i].indexOf('|') >= 0) {
            rows.push(mdSplitRow(lines[i]));
            i++;
          }
          frag.appendChild(mdTable(header, rows));
          continue;
        }
      }

      if (/^\s{0,3}>/.test(line)) {
        var quote = [];
        while (i < lines.length && /^\s{0,3}>/.test(lines[i])) {
          quote.push(lines[i].replace(/^\s{0,3}>\s?/, ''));
          i++;
        }
        var bq = el('blockquote', 'md-quote');
        bq.appendChild(renderMarkdown(quote.join('\n')));
        frag.appendChild(bq);
        continue;
      }

      if (mdListMarker(line)) {
        var items = [];
        while (i < lines.length) {
          var mk = mdListMarker(lines[i]);
          if (mk) { items.push(mk); i++; continue; }
          // 列表项的续行（懒续行）：非空、且不是新块开头，就并进上一条。
          if (items.length && lines[i].trim() && i + 1 <= lines.length &&
              !mdBlockStart(lines[i], lines[i + 1])) {
            items[items.length - 1].text += '\n' + lines[i].trim();
            i++;
            continue;
          }
          break;
        }
        var baseIndent = items[0].indent;
        var list = null;
        var listOrdered = false;
        var lastLi = null;
        var sub = null;
        items.forEach(function (it) {
          if (!list || (it.indent <= baseIndent && it.ordered !== listOrdered)) {
            list = el(it.ordered ? 'ol' : 'ul', 'md-list');
            listOrdered = it.ordered;
            lastLi = null;
            sub = null;
            frag.appendChild(list);
          }
          if (it.indent > baseIndent && lastLi) {
            // 一级嵌套：挂在上一顶层项里面
            if (!sub) {
              sub = el(it.ordered ? 'ol' : 'ul', 'md-list md-sub');
              lastLi.appendChild(sub);
            }
            sub.appendChild(mdListItem(it));
            return;
          }
          sub = null;
          lastLi = mdListItem(it);
          list.appendChild(lastLi);
        });
        continue;
      }

      // 段落：吃到空行或下一个块的开头
      var buf = [];
      while (i < lines.length && !mdBlockStart(lines[i], lines[i + 1])) { buf.push(lines[i]); i++; }
      if (!buf.length) { buf.push(lines[i]); i++; }
      var para = el('p', 'md-p');
      mdInlineLines(para, buf.join('\n'));
      frag.appendChild(para);
    }
    return frag;
  }

  /*
   * 正文块 = Markdown 开关打开时走渲染器，关掉回到原来的纯文本 pre-wrap。
   * 两条路径共用同一个折叠记忆键（`text.<key>`），切开关不会丢掉用户的展开选择。
   */
  function bodyBlock(text, previewLines, key, extraClass) {
    if (!state.markdown) { return collapsibleText(text, previewLines, key, extraClass); }
    return markdownText(text, previewLines, key, extraClass);
  }

  /*
   * Markdown 正文：行数超过 previewLines 时先夹住（max-height + 渐隐遮罩），
   * 展开/收起沿用与纯文本路径同一套按钮与记忆键。
   */
  function markdownText(text, previewLines, key, extraClass) {
    var wrap = el('div', 'text-wrap');
    var raw = String(text === undefined || text === null ? '' : text);
    var body = el('div', 'md-body' + (extraClass ? ' ' + extraClass : ''));
    body.appendChild(renderMarkdown(raw));
    var lines = raw.split('\n');
    var long = lines.length > previewLines;
    var expanded = storeGet('text.' + key) === '1';
    var paint = function () {
      var clamped = long && !expanded;
      body.classList.toggle('clamped', clamped);
      body.style.maxHeight = clamped ? (previewLines * 24) + 'px' : '';
    };
    paint();
    wrap.appendChild(body);
    if (long) {
      var toggle = el('button', 'text-toggle');
      var label = function () {
        toggle.textContent = foldLabel(expanded, lines.length, raw.length);
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
    // 图片名三档（图注 / 短写 / 完整哈希）与 doc_index 路径全部可检索。
    var parts = [s.title, s.label, s.id, s.name, s.project, s.stage, s.stageTitle,
      s.imageName, s.imageType, s.imageCaption, s.imageShort, s.imageFile, s.imageLabel];
    if (s.page) { parts.push('p' + s.page, 'p.' + s.page, '页' + s.page, String(s.page)); }
    if (s.imageOrder) { parts.push('#' + s.imageOrder, '第' + s.imageOrder + '张', String(s.imageOrder)); }
    return parts.filter(Boolean).join(' ').toLowerCase();
  }

  /*
   * 逐图会话显示哪个名字：**有图注/标签就用它**，没有就用**短写文件名**
   * （`bfeafce8.jpg`）——MinerU 按内容哈希命名图片，64 位哈希当标题没人看得懂。
   * 口径与 Go 侧 imageSessionTitle 一致（--list 与侧栏因此显示同一个名字）；
   * 完整文件名与 `images/<书>/<file>` 路径进行的悬浮说明，一个信息都不丢。
   */
  function imageDisplayName(s) {
    return s.imageCaption || s.imageLabel || s.imageShort || s.imageName;
  }

  function sessionTitleOf(s) {
    if (s.imageName && (s.page || s.imageOrder || s.imageShort || s.imageLabel || s.imageCaption)) {
      // 逐图会话的标题用**图注 / 图片名**，页与序号在从属信息里（侧栏只有一行）。
      return (s.stageTitle || '矢量图') + ' · ' + imageDisplayName(s);
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
      // 图片身份：行里显示短名，**完整**文件名、哈希与 doc_index 路径在这里。
      tip.push('来源图片: ' + imageDisplayName(s) + (s.imageType ? ('（' + s.imageType + '）') : '') +
        (s.page ? ('\n第 ' + s.page + ' 页') : '') + (s.imageOrder ? ('\n书内第 ' + s.imageOrder + ' 张') : '') +
        (s.imageCaption ? ('\n图注: ' + s.imageCaption) : ''));
      if (s.imageFile) { tip.push('图片文件: ' + s.imageFile); }
      if (s.imageName) { tip.push('图片哈希: ' + s.imageName); }
      if (s.imagePath) { tip.push('图片路径: ' + s.imagePath); }
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

  /* 图片身份小标签的悬浮说明：短名之外把完整文件名 / 哈希 / doc_index 路径给全。 */
  function imageTipText(s) {
    var bits = [imageDisplayName(s)];
    if (s.imageFile && s.imageFile !== imageDisplayName(s)) { bits.push('文件 ' + s.imageFile); }
    if (s.imageName) { bits.push('哈希 ' + s.imageName); }
    if (s.imagePath) { bits.push(s.imagePath); }
    if (s.page) { bits.push('第 ' + s.page + ' 页'); }
    if (s.imageOrder) { bits.push('书内第 ' + s.imageOrder + ' 张'); }
    if (s.imageType) { bits.push(s.imageType); }
    return bits.join('\n');
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
    if (s.imageName) {
      var chip = el('span', 'row-chip image-chip', imageChipText(s));
      chip.title = imageTipText(s);  // 小标签上也带完整文件名 / 哈希 / 路径
      row.appendChild(chip);
    }
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
    wrap.setAttribute('data-project', g.name);
    var key = groupKey('proj', g.name);
    var cur = state.current;
    var curProj = cur ? (cur.project || projectOf(cur.id)) : '';
    var curStage = cur ? (cur.stage || 'session') : '';
    // **默认收起**：只有记忆里明确展开过、或这一组装着当前选中的会话才展开；
    // 过滤命中的组强制展开（frozen，不写回记忆）。
    var holdsCurrent = !!curProj && curProj === g.name;
    bindCollapse(wrap, key, g.matched ? true : groupWantOpen(key, holdsCurrent), !!g.matched);

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
        det.setAttribute('data-project', g.name);
        det.setAttribute('data-stage', stg);
        var skey = groupKey('stage', g.name + '/' + stg);
        // 阶段组同样默认收起；装着当前会话的那个阶段才自动展开。
        bindCollapse(det, skey, groupWantOpen(skey, holdsCurrent && curStage === stg), false);

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
    syncGroupOpen();  // 当前会话所在的项目组 / 阶段组自动展开（只改 open，不重建）
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
      cur.imageName ? ['图片', imageDisplayName(cur), imageTipText(cur), true] : null,
      cur.imageFile ? ['图片文件', cur.imageFile, cur.imageName, true] : null,
      cur.imagePath ? ['图片路径', cur.imagePath, null, true] : null,
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
      // 注入给模型的工具 parameters 就是 JSON：**高亮**（用户点名这里少了高亮）。
      pbody.appendChild(machineBlock(String(tool.parameters), 'code'));
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
    var promptText = String(line.text || '（这条 meta 行没有正文）');
    if (jsonPretty(promptText) !== null) {
      // 提示词整段就是 JSON（少数会话会把配置当提示词发）→ 直接 JSON 高亮。
      scroll.appendChild(machineBlock(promptText, 'body-text prompt-text'));
    } else if (state.markdown) {
      // 系统提示词也按 Markdown 预览（与消息正文同一套渲染器与开关）；
      // 里面的 ```json 代码块由 mdCodeBlock 顺带做 JSON 高亮。
      var promptMD = el('div', 'md-body prompt-md');
      promptMD.appendChild(renderMarkdown(promptText));
      scroll.appendChild(promptMD);
    } else {
      scroll.appendChild(machineBlock(promptText, 'body-text prompt-text'));
    }
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

  /*
   * Long transcripts are common (a user turn can be thousands of characters),
   * so only the preview lines are put in the DOM until the reader expands it.
   */
  /*
   * 折叠/展开按钮的唯一文案来源：思考块、工具输入输出、系统消息（user 轮任务提示）
   * 三处**逐字一致**，不另造说法。
   */
  function foldLabel(expanded, lines, chars) {
    return expanded ? '收起' : '展开全文（' + lines + ' 行 / ' + chars + ' 字符）';
  }

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
        toggle.textContent = foldLabel(expanded, lines.length, String(text).length);
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

  /*
   * 展开后的"命令行首行"强调：JSON 高亮之外，把这次调用真正执行的那一行
   * （bash 的 command、compile/read/write 的 path、grep 的 pattern）加一个
   * `$ ` 前导提示符并加重——输入侧只做这一点，别比输出更花。
   */
  function toolPromptLine(name, argsText) {
    var obj = null;
    try { obj = JSON.parse(String(argsText || '{}')); } catch (e) { obj = null; }
    if (!obj || typeof obj !== 'object') { return ''; }
    var n = String(name || '').toLowerCase();
    var v = '';
    if (n === 'bash' || n === 'python') { v = obj.command || obj.code || ''; }
    else if (n.indexOf('grep') === 0 || n === 'doc_search' || n.indexOf('search') >= 0) {
      v = [obj.pattern || obj.query || '', obj.path || ''].filter(Boolean).join('  ');
    } else if (n === 'compile' || n.indexOf('write') === 0 || n.indexOf('edit') === 0 ||
               n.indexOf('read') === 0 || n === 'view_pdf' || n === 'view_image') {
      v = obj.path || '';
    }
    v = String(v).split('\n')[0].trim();
    return v.length > 160 ? v.slice(0, 160) + '…' : v;
  }

  function cmdPreview(cmd) {
    var div = el('div', 'io-cmd');
    div.appendChild(el('span', 'cmd-prompt', '$ '));
    div.appendChild(el('span', 'cmd-line', cmd));
    return div;
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
    // 输入输出都走同一套机器文本渲染：是 JSON 就做 JSON 高亮（k/s/n/b），
    // 否则按终端输出/diff/日志着色；超过限高给「展开全文」，与思考块同一套。
    var wrap = machineScroll(text, key);
    if (isError) {
      var pre = wrap.querySelector('.io-text');
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
    var cmdLine = toolPromptLine(name, argsText);
    if (cmdLine) { card.appendChild(cmdPreview(cmdLine)); }
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

  // 折叠行的尾巴：`输入 → 输出 字符数 · ok/error`（再加附件张数）。
  // 一行里就能看出这次调用吃了多少、回了多少、成没成、带了几张图，不必展开。
  function updateCallTail(node) {
    if (!node.tail) { return; }
    var tail = node.argsChars + ' 字符';
    if (node.result) {
      var text = String(node.result.line.text || '');
      tail = node.argsChars + ' → ' + text.length + ' 字符' +
        (node.result.status === 'error' ? ' · error' : node.result.status === 'ok' ? ' · ok' : '');
    }
    if (node.attachments) { tail += ' · 附件 ' + node.attachments + ' 张（user 轮）'; }
    node.tail.textContent = tail;
  }

  // 把一条工具回执补进它对应的调用卡片（同一次调用 = **一行**；配不上的回执
  // 另起一行，见 standaloneResult）。回执与调用合并不只是省地方：折叠态那一行
  // 就写完了 `工具名 · 摘要 · 输入→输出 字符数 · ok/error`。
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
    updateCallTail(node);
    return node;
  }

  /*
   * 图片轮（带 images 的 user 行）作为**附件**并进对应调用/回执的卡片：
   * 与「输入 / 输出」同属一次调用，展开才看到缩略图（点图进灯箱）。
   */
  function attachImages(host, line, attr) {
    var imgs = line.images || [];
    if (!imgs.length) { return host; }
    // 宿主可以是调用卡片（node.card）也可以是系统消息块（任务提示）：图片归属
    // 决定挂哪儿——工具图片回执挂那次调用，会话开头投喂的原图挂任务。
    var box = host.card || host;
    box.appendChild(el('div', 'io-divider'));
    var section = el('div', 'io-section');
    section.appendChild(el('div', 'io-label', '附件（user 轮）'));
    var body = el('div', 'attach-body');
    if (line.text) { body.appendChild(bodyBlock(line.text, LONG_TEXT_LINES, 'imgtext.' + line.n)); }
    body.appendChild(imageStrip(line));
    section.appendChild(body);
    box.appendChild(section);
    section.appendChild(el('div', 'attach-note',
      '这一轮是 user 轮发出的（' + (attr && attr.kind === 'task' ? '会话开头的原图投喂，作为任务的输入' : '工具的输入/附件') +
      '）· ' + attributionText(attr)));
    host.attachments = (host.attachments || 0) + imgs.length;
    if (host.details) { updateCallTail(host); }
    return host;
  }

  function standaloneResult(line) {
    var info = state.calls.get(line.tool_call_id);
    var name = info ? info.name : '(未配对的工具回执)';
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
    if (!info) {
      d.title = line.tool_call_id
        ? '未找到配对的工具调用：id ' + line.tool_call_id
        : '这条回执行没有 tool_call_id，无法与调用配对';
    }
    return d;
  }

  /* ---------- 图片轮 / 任务块 ---------- */

  // 图片引用（file://media/<sha>.jpg 之类）的**文件名**：长哈希只留前 8 位显示，
  // 完整名字在缩略图的 title 与 alt 上（沿用 Go 侧"哈希当不了名字"的同一口径）。
  function refBaseName(ref) {
    var s = String(ref || '').split('?')[0];
    var i = Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\'));
    return i >= 0 ? s.slice(i + 1) : s;
  }

  function shortFileName(name) {
    var s = String(name || '');
    var dot = s.lastIndexOf('.');
    var stem = dot > 0 ? s.slice(0, dot) : s;
    var ext = dot > 0 ? s.slice(dot) : '';
    if (/^[0-9a-f]{32,}$/i.test(stem)) { return stem.slice(0, 8) + ext; }
    return s;
  }

  // 作图任务把原图印刷尺寸写在正文里（ORIGINAL FIGURE SIZE: 36.9mm x 20.1mm）
  function originalFigureSize(text) {
    var m = /ORIGINAL\s+FIGURE\s+SIZE\s*:\s*([0-9.]+\s*mm\s*[x×]\s*[0-9.]+\s*mm)/i.exec(String(text || ''));
    return m ? m[1].replace(/\s+/g, ' ') : '';
  }

  // 图片轮的一行摘要：几张 + 原图尺寸（取得到就写）+ 文件名
  function imageTurnSummary(line) {
    var imgs = (line && line.images) || [];
    var bits = [imgs.length + ' 张图片'];
    var size = originalFigureSize(line && line.text);
    if (size) { bits.push(size); }
    var names = imgs.map(function (r) { return shortFileName(refBaseName(r)); });
    if (names.length) {
      bits.push(names.slice(0, 2).join('、') + (names.length > 2 ? ' 等 ' + names.length + ' 个文件' : ''));
    }
    return bits.join(' · ');
  }

  /*
   * 图片轮该归到哪次调用：往前找**最近的调用或回执**（assistant 的 tool_calls、
   * 或已配对的 tool 回执行；同批图片轮继续往前找）。中间隔着任务提示或普通助手
   * 正文就说明它们不是一回事——配不上就让它自己成行（左对齐、同样默认折叠）。
   */
  /*
   * 带图 user 轮的**工具归属**（第八条）。wire 上图片只能走 user 消息（tool 消息的
   * content 只能是文本），所以归属只能从**句柄文本 + 顺序**里推断，绝不新增字段：
   *   · 句柄以 `Tool image output` 开头（工具图片回执的固定前缀，旧转录同样是它）：
   *       句柄里有 `(call <id>)`  → 精确匹配那一次调用（新转录会写明归属）；
   *       只有 `from <tool>`      → 该轮里同名、且还没被认领的那一次调用；
   *       两者都没有（旧转录）    → 该轮里还没被认领的那一次调用（纯顺序推断）；
   *   · 不是句柄（会话开头"投喂原图"的那一轮）→ 归属到本会话**第一条任务**
   *     （第一条不带图的 user 行）的输入附件，不塞给任何工具。
   * 每条结果都带 how（依据），轨迹页据此标"精确匹配 / 由顺序推断"，让人能核对。
   */
  var IMAGE_HANDLE_RE = /^Tool image output\b/i;
  var IMAGE_CALL_RE = /\(call\s+([A-Za-z0-9_.:-]+)\)/;
  var IMAGE_FROM_RE = /\bfrom\s+([A-Za-z0-9_.:-]+)/i;

  // 归属结果按"会话 + 行数"缓存：渲染是增量的，行数变了就重算一遍。
  function imageAttributions() {
    var sig = (state.current ? state.current.id : '') + ':' + state.lines.length;
    if (state.imgAttr && state.imgAttr.sig === sig) { return state.imgAttr.map; }

    var map = {};
    var claims = {};       // callId → 已被几张图认领
    var roundCalls = [];   // 最近一条带 tool_calls 的助手消息发起的调用
    var firstTask = 0;     // 本会话第一条任务（不带图的 user 行）
    state.lines.forEach(function (l) {
      if (!l || l.bad || (l.t && l.t !== 'msg')) { return; }
      if (l.role === 'user' && !(l.images && l.images.length) && !firstTask) { firstTask = l.n; }
    });

    state.lines.forEach(function (line) {
      if (!line || line.bad || (line.t && line.t !== 'msg')) { return; }
      if (line.role === 'assistant') {
        var calls = line.tool_calls || [];
        if (calls.length) {
          roundCalls = calls.map(function (c) {
            return { id: c.id, name: (c.function || {}).name || '', lineN: line.n };
          });
        }
        return;
      }
      if (line.role !== 'user' || !(line.images && line.images.length)) { return; }

      var text = String(line.text || '').trim();
      var entry = { kind: 'none', how: '', callId: '', name: '', lineN: line.n, taskLineN: firstTask };
      if (IMAGE_HANDLE_RE.test(text)) {
        var idm = IMAGE_CALL_RE.exec(text);
        var frm = IMAGE_FROM_RE.exec(text);
        entry.name = frm ? frm[1] : '';
        var pick = null;
        if (idm) {
          roundCalls.forEach(function (c) { if (!pick && c.id === idm[1]) { pick = c; } });
          if (!pick && state.callNodes && state.callNodes[idm[1]]) {
            pick = { id: idm[1], name: entry.name, lineN: 0 };
          }
          if (pick) { entry.how = 'call-id'; }
        }
        if (!pick && entry.name) {
          roundCalls.forEach(function (c) {
            if (!pick && c.name === entry.name && !claims[c.id]) { pick = c; entry.how = 'tool-name'; }
          });
        }
        if (!pick) {
          roundCalls.forEach(function (c) {
            if (!pick && !claims[c.id]) { pick = c; entry.how = 'order'; }
          });
        }
        if (pick) {
          claims[pick.id] = (claims[pick.id] || 0) + 1;
          entry.kind = 'call';
          entry.callId = pick.id;
          if (!entry.name) { entry.name = pick.name; }
        }
      } else if (firstTask) {
        // 会话开头投喂原图的轮：归到第一条任务（它是那次作图的输入），不塞给工具。
        entry.kind = 'task';
        entry.how = 'task';
      }
      map[line.n] = entry;
    });

    state.imgAttr = { sig: sig, map: map };
    return map;
  }

  // 归属依据的中文说明（对话页的附件脚注与轨迹页共用同一套说法）。
  function attributionText(attr) {
    if (!attr) { return ''; }
    switch (attr.how) {
      case 'call-id':
        return '归属：call ' + attr.callId + (attr.name ? '（' + attr.name + '）' : '');
      case 'tool-name':
        return '归属：call ' + attr.callId + '（按工具名 ' + attr.name + ' 匹配）';
      case 'order':
        return '归属：由顺序推断（本轮的 call ' + attr.callId + '）';
      case 'task':
        return '归属：本会话任务（这一段是投喂给任务的原图）';
      default:
        return '归属：未识别';
    }
  }

  /*
   * 图片轮自己的折叠行（配不上调用时）：与工具行同一套词汇——16px 插槽 +
   * 名称 + 圆点 + 一行摘要 + 右侧张数；**默认折叠**，点开才看具体图片。
   * 名字里点明这是 **user 轮**发出的（它就是给模型的输入，不是"用户的话"）。
   */
  function imageTurnRow(line, attr) {
    var imgs = line.images || [];
    var key = 'image.' + state.current.id + '.' + line.n;
    var d = disclosureLine('disclosure-image', '图片（user 轮）', imageTurnSummary(line),
      imgs.length + ' 张', storeGet(key) === '1');
    var body = el('div', 'image-body');
    if (line.text) { body.appendChild(bodyBlock(line.text, LONG_TEXT_LINES, 'imgtext.' + line.n)); }
    body.appendChild(imageStrip(line));
    if (attr) { body.appendChild(el('div', 'attach-note', attributionText(attr))); }
    d.appendChild(body);
    d.title = '这一轮是 user 轮发出的（把图片投给模型），不是人打的字\n' +
      IMAGE_WIRE_TITLE + '\n' + attributionText(attr) + '\n' + imageTurnSummary(line);
    d.addEventListener('toggle', function () { storeSet(key, d.open ? '1' : '0'); });
    return d;
  }

  // 图片轮做成一条独立消息行（左对齐，永远不靠右）。
  function imageTurnSection(line, attr) {
    var wrap = el('section', 'msg msg-image');
    wrap.appendChild(imageTurnRow(line, attr));
    return wrap;
  }

  /*
   * 不带图的 user 行 = harness 自己发的任务提示，按**系统消息**呈现：
   *   · 角色标签是「系统」+ 次级标签「user 轮」（这一轮确实是 user 角色发出的）；
   *   · 安静样式（弱色、无气泡），正文仍然是正文；
   *   · **与思考/工具展开同一套折叠**：默认只露前 SYSTEM_PREVIEW_LINES 行（渐隐），
   *     点「展开全文（N 行 / M 字符）」看全文，展开后限高内滚、可「收起」——
   *     几千字的任务提示否则会把一屏占满。
   */
  function systemTurnSection(line) {
    var msg = el('section', 'msg msg-system');
    var head = el('div', 'sys-line');
    head.appendChild(el('span', 'sys-badge', '系统'));
    head.appendChild(el('span', 'sys-meta', 'user 轮'));
    head.appendChild(el('span', 'line-summary', firstLine(line.text)));
    msg.appendChild(head);

    var raw = String(line.text || '');
    if (!raw) {
      msg.appendChild(el('div', 'note', '（无正文）'));
      return msg;
    }
    var key = 'sys.' + state.current.id + '.' + line.n;
    var lines = raw.split('\n');
    var long = lines.length > SYSTEM_PREVIEW_LINES;
    var expanded = storeGet('text.' + key) === '1';
    var scroll = el('div', 'sys-scroll');
    // 正文按 Markdown 开关渲染（与消息正文同一个渲染器），但**折叠只由这一层
    // 负责**：不套 bodyBlock 的第二层折叠，否则会冒出两个「展开全文」按钮。
    if (state.markdown) {
      var md = el('div', 'md-body sys-md');
      md.appendChild(renderMarkdown(raw));
      scroll.appendChild(md);
    } else {
      scroll.appendChild(el('pre', 'body-text sys-text', raw));
    }
    var paint = function () {
      scroll.classList.toggle('folded', long && !expanded);
      scroll.style.maxHeight = (long && !expanded)
        ? (SYSTEM_PREVIEW_LINES * 24) + 'px'
        : 'var(--code-scroll-h)';
    };
    paint();
    msg.appendChild(scroll);
    if (long) {
      var toggle = el('button', 'text-toggle');
      toggle.type = 'button';
      var label = function () {
        toggle.textContent = foldLabel(expanded, lines.length, raw.length);
      };
      label();
      toggle.addEventListener('click', function () {
        expanded = !expanded;
        storeSet('text.' + key, expanded ? '1' : '0');
        paint();
        label();
      });
      msg.appendChild(toggle);
    }
    msg.title = '这一轮是 user 角色发出的任务提示（系统性质，不是人打的字）';
    return msg;
  }

  function anchor(node, line) {
    if (line && line.n !== undefined) { state.anchors[line.n] = node; }
    return node;
  }

  /* Returns the element for one transcript line, or null when it is skipped. */
  function renderLine(line, idx) {
    if (line.bad) {
      state.badLines++;
      updateBanner();
      return null;
    }
    // meta 行不是消息：它由详情栏的元信息块渲染，绝不进消息序列。
    if (line.t && line.t !== 'msg') { return null; }
    if (!line.role) { return null; }

    var isToolCall = line.role === 'assistant' && line.tool_calls && line.tool_calls.length > 0;
    // 带图的 user 行是"把图片投给模型"的那一轮（图片投喂 / 工具回执），
    // 不是用户的话，所以「仅看工具调用」里也要看得见它。
    var isImageTurn = line.role === 'user' && !!(line.images && line.images.length);
    if (state.onlyTools && line.role !== 'tool' && !isToolCall && !isImageTurn) { return null; }

      if (line.role === 'user') {
        /*
         * 两条路都不靠右对齐（右侧气泡那套已经去掉）：
         *   · **带图**的 user 行是图片投喂/工具图片回执 → 按归属挂到那一次调用
         *     （会话开头的原图则挂到第一条任务）上，配不上才单独一行折叠行；
         *   · **不带图**的 user 行是 harness 自己发的长任务提示 → 按**系统消息**
         *     呈现（安静样式 + 「系统 · user 轮」标签），默认只露一小段，可展开。
         */
        if (isImageTurn) {
          var attr = imageAttributions()[line.n] || { kind: 'none', how: '', callId: '', taskLineN: 0 };
          if (attr.kind === 'call' && state.callNodes[attr.callId]) {
            var callNode = state.callNodes[attr.callId];
            attachImages(callNode, line, attr);
            return anchor(callNode.details, line);
          }
          if (attr.kind === 'task' && attr.taskLineN) {
            var taskNode = state.anchors[attr.taskLineN];
            if (taskNode) {
              attachImages(taskNode, line, attr);
              return anchor(taskNode, line);
            }
            // 会话开头的原图轮排在任务**前面**：这时任务节点还没建出来，先记账，
            // 等任务行渲染时再挂上去（否则只能退化成一条孤立的图片行）。
            pendingTaskImages[attr.taskLineN] = pendingTaskImages[attr.taskLineN] || [];
            pendingTaskImages[attr.taskLineN].push({ line: line, attr: attr });
            return null;
          }
          return anchor(imageTurnSection(line, attr), line);
        }
        var msg = systemTurnSection(line);
        var waiting = pendingTaskImages[line.n];
        if (waiting && waiting.length) {
          waiting.forEach(function (w) { attachImages(msg, w.line, w.attr); });
          delete pendingTaskImages[line.n];
        }
        return anchor(msg, line);
      }

    if (line.role === 'tool') {
      // 已配对的调用行已经带了输出，这一行就并入那张卡片（节点已在 DOM 里，
      // 这里返回 null）：**一次调用只占一行**，不另起一行"结果"。
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
      if (line.text) { msg.appendChild(bodyBlock(line.text, LONG_TEXT_LINES, 'asst.' + line.n)); }
      (line.tool_calls || []).forEach(function (call) {
        msg.appendChild(toolDisclosure(call).details);
      });
      if (!line.text && !line.reasoning && !(line.tool_calls || []).length) {
        msg.appendChild(el('div', 'note', '（空消息）'));
      }
      return anchor(msg, line);
    }

    var other = el('section', 'msg msg-other');
    other.appendChild(bodyBlock(line.text || '(无正文)', LONG_TEXT_LINES, 'other.' + line.n));
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
    pendingTaskImages = {};
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
    state.lines.forEach(function (line, idx) {
      var node = renderLine(line, idx);
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
    // 增量追加时行号要接着已渲染的部分数：图片轮配对要用 state.lines 里的下标。
    var base = state.lines.length - lines.length;
    lines.forEach(function (line, idx) {
      var node = renderLine(line, base + idx);
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
  // 带图 user 轮的 wire 真相：OpenAI 兼容 schema 里 tool 消息的 content 只能是
  // 文本，图片只能作为 user 消息的多段内容回灌，所以"工具图片回执"在转录里表现为
  // 紧随工具回执行的一条**带图 user 轮**（见 internal/session/session.go）。
  var IMAGE_WIRE_TITLE = 'user 消息承载图片（tool 消息的 content 只能文本，OpenAI 兼容 schema 限制）' +
    ' · text + image_url(data:image/jpeg;base64,…)';

  // 图片占位标记：工具回执正文里如果写了这一笔（调试日志里的形状），说明图片在
  // 紧随其后的 user 轮里。我们自己的转录通常只有紧邻关系，所以两种都认。
  var IMAGE_PLACEHOLDER = /\[\s*image\b|\[\s*图片|图片见|image omitted/i;

  function nextMsgLine(lines, idx) {
    for (var i = idx + 1; i < lines.length; i++) {
      if (lines[i] && !lines[i].bad && lines[i].t === 'msg') { return lines[i]; }
    }
    return null;
  }

  function trajectoryRows() {
    var rows = [];
    var callOf = {};
    // 哪些带图 user 轮的图片是**上一行工具回执**投出来的（两行互相提示，但不合并）。
    var imageAfterTool = {};
    state.lines.forEach(function (l) {
      if (!l || l.bad || l.t !== 'msg') { return; }
      if (l.role === 'assistant') {
        (l.tool_calls || []).forEach(function (c) {
          var fn = c.function || {};
          callOf[c.id] = { name: fn.name || '?', ts: l.ts || '', n: l.n, args: String(fn.arguments || '') };
        });
      }
    });

    state.lines.forEach(function (line, idx) {
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

      /*
       * 步骤分类与对话页一一对应：
       *   · 带图的 user 轮在类型列照旧是「用户」，摘要里写明 `图片 ×N` 并可展开看图；
       *   · 点行跳回对话里**合并后的那一块**（对话是合并的，轨迹是真实的）。
       */
      if (line.role === 'user' && line.images && line.images.length) {
        var attr = imageAttributions()[line.n] || { kind: 'none', how: '', callId: '', taskLineN: 0 };
        var fromTool = !!imageAfterTool[line.n];
        // 跳回对话里"合并后的那一块"：归到调用就跳那次调用，归到任务就跳任务行，
        // 都没认出来才跳自己这一行。
        var jumpTo = attr.kind === 'call' && attr.callId && state.callNodes[attr.callId]
          ? state.callNodes[attr.callId].lineN
          : (attr.kind === 'task' && attr.taskLineN ? attr.taskLineN : line.n);
        rows.push({
          kind: 'user', tag: '用户', name: '用户',
          summary: (fromTool ? '接上一行工具回执 · ' : '') + '图片 ×' + line.images.length + ' · ' +
            (firstLine(line.text) || '（无正文）') + ' · ' + attributionText(attr),
          title: IMAGE_WIRE_TITLE + '\n' + attributionText(attr) +
            (attr.how === 'call-id' ? '（句柄里写了 call id，属于精确匹配）' : '（旧转录没有 call id，按顺序推断；新转录会写上归属）') +
            (fromTool ? '\n这一轮的图片就是上一行工具回执投出来的（同一件事的两段 wire 表达，所以两行不合并）' : ''),
          chars: String(line.text || '').length,
          status: '', jump: jumpTo,
          images: line.images,
          detail: { user: String(line.text || '') || '（这一轮没有正文）' }
        });
      } else if (line.role === 'user') {
        // 会话开头的原图投喂轮归到这条任务上：轨迹里也点明"它带着附件"。
        var fed = 0;
        var attrs = imageAttributions();
        Object.keys(attrs).forEach(function (n) {
          if (attrs[n].kind === 'task' && attrs[n].taskLineN === line.n) { fed++; }
        });
        rows.push({
          kind: 'user', tag: '用户', name: '用户',
          summary: firstLine(line.text) + (fed ? ' · 附件 图片 ×' + fed : ''),
          title: fed
            ? '会话开头的原图投喂轮归到了这条任务（对话页里它们收在同一个块里）'
            : '点击跳到对话里对应的那条消息',
          chars: String(line.text || '').length,
          status: '', jump: line.n,
          detail: { user: String(line.text || '') }
        });
      } else if (line.role === 'assistant') {
        if (line.reasoning) {
          rows.push({
            kind: 'think', tag: '思考', name: 'reasoning',
            summary: firstLine(line.reasoning), chars: line.reasoning.length,
            status: '', jump: line.n, detail: { thinking: line.reasoning }
          });
        }
        if (line.text) {
          rows.push({
            kind: 'msg', tag: '助手', name: 'AI',
            summary: firstLine(line.text), chars: line.text.length,
            status: '', jump: line.n, detail: { message: String(line.text) }
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
      } else if (line.role === 'tool') {
        // 工具结果**独立成行**（轨迹就是让人看清真实结构的）：配对得上的用调用名，
        // 配不上的注明，不并进调用行。
        var info = line.tool_call_id ? callOf[line.tool_call_id] : null;
        var text = String(line.text || '');
        var after = nextMsgLine(state.lines, idx);
        var imageNext = !!(after && after.role === 'user' && after.images && after.images.length);
        if (imageNext) { imageAfterTool[after.n] = true; }
        var hint = imageNext
          ? (IMAGE_PLACEHOLDER.test(text) ? ' · 图片见下一行用户轮' : ' · 图片在下一行用户轮里')
          : '';
        rows.push({
          kind: 'result', tag: '结果',
          name: info ? info.name : '(未配对的工具回执)',
          summary: firstLine(text) + hint, chars: text.length,
          title: imageNext
            ? '这一行是工具回执：tool 消息的 content 只能是文本，随行的图片被回灌在紧随其后的 user 轮里（两行是同一件事，保持两行不合并）'
            : '点击跳到对话里对应的那条消息',
          status: classifyResult(text), jump: line.n,
          time: callDuration(info, line),
          detail: { output: text }
        });
      }
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

  // 轨迹的类型 chip 按**转录里的真实角色**列（对话页是合并后的呈现，两者语义不同）。
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
      var text = String(row.detail[k] || '');
      // 输入 / 输出 / 用量行与对话页同源：JSON 高亮，其余纯文本。
      if (k === 'input' || k === 'request' || k === 'output') {
        section.appendChild(machineBlock(text, 'code'));
      } else {
        var pre = el('pre', 'code', text);
        section.appendChild(pre);
      }
      var actions = el('div', 'row-actions');
      actions.appendChild(copyButton(text));
      section.appendChild(actions);
      body.appendChild(section);
    });
    // 图片轮的展开区里也要能看到图（点图进灯箱，与对话页同一个灯箱）。
    if (row.images && row.images.length) {
      body.appendChild(imageStrip({ images: row.images }));
    }
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
      tr.title = row.title || '点击跳到对话里对应的那条消息';

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
  /*
   * Markdown 开关：默认开启，关掉回到纯文本 pre-wrap（两条路径共用同一套折叠
   * 记忆键）。切换后重画消息流——详情栏里的系统提示词也一起重画。
   */
  function applyMarkdownToggle() {
    setToggle(refs.mdToggle, state.markdown);
    if (refs.mdToggle) {
      refs.mdToggle.title = state.markdown
        ? '消息正文按 Markdown 渲染（标题 / 列表 / 代码块 / 表格），点击回到纯文本'
        : '消息正文按纯文本显示（pre-wrap），点击改用 Markdown 渲染';
    }
  }
  if (refs.mdToggle) {
    refs.mdToggle.addEventListener('click', function () {
      state.markdown = !state.markdown;
      storeSet('markdown', state.markdown ? '1' : '0');
      applyMarkdownToggle();
      renderTimeline();
    });
  }
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
    applyMarkdownToggle();
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
