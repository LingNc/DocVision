<script setup lang="ts">
// 迁移纪律：本组件（以及后续所有组件）不写任何样式——视觉全部来自
// 整卷逐字搬迁的旧页样式表（src/styles/viewer.css）。模板结构照
// go/internal/sessionview/assets/viewer.html 的骨架逐节点复刻。
// 块 1（三栏骨架）进行中：先落静态骨架与主题开关，侧栏/正文/详情内容
// 在后续块里逐块填充。
import { ref } from 'vue'

const theme = ref(document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light')

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  document.documentElement.setAttribute('data-theme', theme.value)
  try {
    window.localStorage.setItem('dsh.sessionview.theme', theme.value)
  } catch {
    /* file:// 下可能禁用 localStorage：静默跳过 */
  }
}
</script>

<template>
  <div id="frame" class="frame">
    <aside id="sidebar-col" class="sidebar-col">
      <div class="side-head">
        <span class="side-title">工作区</span>
        <button id="refresh" class="icon-btn" type="button" title="重新扫描会话">⟳</button>
      </div>
      <div class="side-search">
        <input id="search" type="search" placeholder="过滤：会话名 / 阶段 / 项目…" autocomplete="off">
      </div>
      <div class="side-list-wrap">
        <div id="session-list" class="session-list" role="tree" aria-label="会话列表"></div>
        <div class="list-fade" aria-hidden="true"></div>
      </div>
      <div id="side-totals" class="side-totals hidden"></div>
      <div class="side-status">
        <div id="root-path" class="root-path" title="扫描根目录">—</div>
        <div id="side-foot" class="side-foot"></div>
      </div>
    </aside>

    <div id="handle-sidebar" class="handle" data-side="sidebar" role="separator" aria-orientation="vertical" aria-label="调整侧栏宽度"></div>

    <main id="center-col" class="center-col">
      <header id="center-header" class="center-header">
        <div class="title-row">
          <button id="side-toggle" class="icon-btn" type="button" title="折叠 / 展开侧栏" aria-label="折叠或展开侧栏">▤</button>
          <nav id="crumbs" class="crumbs" aria-label="面包屑"></nav>
          <div id="header-actions" class="header-actions">
            <span id="mode-badge" class="badge badge-muted">—</span>
          </div>
          <button id="details-toggle" class="icon-btn" type="button" title="详情面板（元信息 / 指标）" aria-label="展开或收起详情面板">ⓘ</button>
        </div>
        <div class="tabs-row">
          <div class="tabs" role="tablist" aria-label="视图">
            <button id="tab-chat" class="tab tab-active" type="button" role="tab" aria-selected="true" data-view="chat">对话</button>
            <button id="tab-traj" class="tab" type="button" role="tab" aria-selected="false" data-view="trajectory">轨迹</button>
          </div>
          <div class="tab-tools" role="group" aria-label="显示选项">
            <button id="follow" class="tab-toggle" type="button" aria-pressed="false" title="新消息到达时自动滚动到底部">自动跟随</button>
            <button id="collapse-thinking" class="tab-toggle" type="button" aria-pressed="false" title="把所有消息的思考过程折叠起来">折叠全部思考</button>
            <button id="only-tools" class="tab-toggle" type="button" aria-pressed="false" title="只显示工具调用与工具结果">仅看工具调用</button>
            <button id="md-toggle" class="tab-toggle" type="button" aria-pressed="true" title="消息正文按 Markdown 渲染（标题 / 列表 / 代码块 / 表格），点击回到纯文本">Markdown</button>
            <button id="unit-toggle" class="tab-toggle" type="button" aria-pressed="true" title="计数按 token 显示（本地估算，带 ≈），点击改为字符">token</button>
            <button id="theme-toggle" class="tab-toggle" type="button" title="切换浅色/深色主题" @click="toggleTheme">{{ theme === 'dark' ? '☀ 浅色' : '🌙 深色' }}</button>
          </div>
        </div>
      </header>
      <div id="banner" class="banner hidden"></div>
      <div class="view-area">
        <div id="timeline" class="timeline"></div>
        <div id="trajectory" class="trajectory hidden"></div>
      </div>
    </main>

    <div id="handle-details" class="handle" data-side="details" role="separator" aria-orientation="vertical" aria-label="调整详情栏宽度"></div>

    <aside id="details-col" class="details-col" aria-label="详情">
      <div class="details-head">
        <span class="details-title">详情</span>
        <button id="details-close" class="icon-btn" type="button" title="关闭详情面板" aria-label="关闭详情面板">✕</button>
      </div>
      <div id="details-body" class="details-body"></div>
    </aside>
  </div>
  <div id="lightbox" class="lightbox hidden">
    <img id="lightbox-img" alt="">
    <div class="lightbox-hint">点击空白处或按 Esc 关闭</div>
  </div>
</template>
