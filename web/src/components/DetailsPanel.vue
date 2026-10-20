<script setup lang="ts">
// 右侧详情栏整体组件：会话块（键值表）+ 指标块（瓦片 + 每次请求明细表）+
// 元信息卡（系统提示词快照 + 工具 schema）。数据全部来自 legacy/details.ts
// 的纯模型函数（computed 响应式驱动，不再有 renderDetails 手动重渲链）。
import { computed, ref, watch } from 'vue'
import { state, storeSet } from '../state'
import { metaModel, META_COUNT_TIP, sessionKVRows, sessionSubPath, statsModel } from '../legacy/details'
import { trajectoryRows } from '../legacy/trajectory'
import InlineMD from './InlineMD.vue'
import MachineText from './MachineText.vue'
import MdBody from './MdBody.vue'
import CopyBtn from './CopyBtn.vue'
import { chapterPendingRetry, refreshIndex, requestChapterRetry } from '../data'
import ImageStrip from './ImageStrip.vue'

const kvRows = computed(() => sessionKVRows())
const subPath = computed(() => sessionSubPath())
const stats = computed(() => statsModel())
const meta = computed(() => metaModel())

/* 元信息卡展开状态：按会话 + 提示词哈希记忆（用户展开过一次后保持）。 */
const metaOpen = ref(false)
watch(
  () => meta.value?.key,
  () => { metaOpen.value = !!meta.value?.open },
  { immediate: true },
)

function onMetaToggle(ev: Event) {
  const m = meta.value
  if (!m) return
  const open = (ev.target as HTMLDetailsElement).open
  metaOpen.value = open
  storeSet(m.key, open ? '1' : '0')
}

/* P53：手动重试入口。只在"章节转换会话"上给按钮——重跑一章是干净的
 * （每章自己的转录与产物），而样式/汇编/整组核验是屏障阶段，重跑它们不是
 * 一回事，这里不提供。 */
const retryTarget = computed(() => {
  const s: any = state.current
  const role = s && s.chapterRole
  if (!s || !s.chapterId || !s.project) return null
  if (role !== 'convert' && role !== 'checker') return null
  // 子章节由父章节的细分树产出，不单独重跑（重试父章节）。
  if (String(s.chapterId).indexOf('_child_') >= 0) return null
  return { project: String(s.project), chapter: String(s.chapterId), role: String(role) }
})
const retryPending = computed(() => {
  const t = retryTarget.value
  return !!t && chapterPendingRetry(t.project, t.chapter, t.role)
})
const retryIsChecker = computed(() => !!retryTarget.value && retryTarget.value.role === 'checker')
const retryButtonText = computed(() => {
  const t = retryTarget.value
  if (!t) return ''
  if (retryPending.value) return t.role === 'checker' ? '已在重新核对队列' : '已在重试队列'
  return t.role === 'checker' ? '重新核对这一章' : '重试这一章'
})

function doRetry(): void {
  const t = retryTarget.value
  if (!t) return
  const what = t.role === 'checker' ? '重新核对' : '重试'
  state.retryNote = '正在提交 ' + t.chapter + ' 的' + what + '请求…'
  requestChapterRetry(t.project, t.chapter, t.role, '详情栏' + what)
    .then((res: any) => {
      state.retryNote = t.chapter + ' ' + what + '请求已排队（' + ((res && res.note) || '等待转换阶段处理') + '）'
      return refreshIndex()
    })
    .catch((err: any) => {
      state.retryNote = t.chapter + ' ' + what + '请求失败：' + ((err && err.message) || err)
    })
}

/* 切会话时清掉上一章的反馈，免得看着像这一章的。 */
watch(() => state.current && state.current.id, () => { state.retryNote = '' })

const countTip = computed(() =>
  meta.value?.countNote ? META_COUNT_TIP.replace('{n}', meta.value.countNote.replace(/^共 (\d+) 条.*$/, '$1')) : '',
)

/* P9：轨迹页选中的步骤——detail/images 与轨迹页就地展开同源（trajectoryRows）。
 * 仅在轨迹视图显示（对话页有自己的就地展开与行锚点，两处不抢戏）。 */
const TRAJ_STEP_LABELS: Record<string, string> = {
  prompt: '系统提示词', thinking: '思考', user: '用户消息', message: '助手消息',
  input: '输入', output: '输出', request: '用量行',
}
const trajStep = computed(() => {
  if (state.view !== 'trajectory' || state.trajSelected == null || !state.current) return null
  const row = trajectoryRows().find((r: any) => r.rid === state.trajSelected)
  return row ? { n: row.jump as number, tag: row.tag as string, name: row.name as string, detail: (row.detail || {}) as Record<string, unknown>, images: (row.images || []) as string[] } : null
})

function trajDetailKind(key: string): 'code' | 'plain' {
  return key === 'input' || key === 'request' || key === 'output' ? 'code' : 'plain'
}

function closeTrajStep() {
  state.trajSelected = null
}
</script>

<template>
  <div id="details-body" class="details-body">
    <div v-if="!state.current" class="note">左侧选择一个会话后，这里显示它的指标与元信息。</div>
    <template v-else>
      <section v-if="trajStep" class="detail-block traj-step">
        <h3 class="detail-block-title">
          步骤 #{{ trajStep.n }}
          <button type="button" class="icon-btn traj-step-close" title="关闭这一步的详情" @click="closeTrajStep">×</button>
        </h3>
        <div class="traj-step-head">
          <span class="kind-tag">{{ trajStep.tag }}</span>
          <span class="traj-step-name">{{ trajStep.name }}</span>
        </div>
        <div v-for="(text, key) in trajStep.detail" :key="key" class="traj-step-section">
          <div class="traj-detail-title">{{ TRAJ_STEP_LABELS[key] || key }}</div>
          <MachineText v-if="trajDetailKind(key as string) === 'code'" :text="String(text || '')" cls="code" />
          <pre v-else class="code">{{ String(text || '') }}</pre>
          <div class="row-actions"><CopyBtn :text="String(text || '')" /></div>
        </div>
        <ImageStrip v-if="trajStep.images.length" :images="trajStep.images" />
      </section>
      <section class="detail-block">
        <h3 class="detail-block-title">会话</h3>
        <dl class="detail-kv">
          <template v-for="row in kvRows" :key="row.k">
            <dt>{{ row.k }}</dt>
            <dd :class="row.mono ? 'mono' : undefined" :title="row.title || undefined">
              <InlineMD v-if="row.md" :text="row.md" tag="span" class="detail-name" />
              <template v-else>{{ row.v }}</template>
            </dd>
          </template>
        </dl>
        <div v-if="subPath" class="stats-sub">目录：{{ subPath }}</div>
      </section>

      <section v-if="retryTarget && !state.staticMode" class="detail-block">
        <h3 class="detail-block-title">{{ retryIsChecker ? '重新核对' : '手动重试' }}</h3>
        <div class="retry-hint">
          <template v-if="retryIsChecker">
            只重跑<b>核对会话</b>，产物一个字不动：用全新上下文再核对一遍已提交的 .tex 与编译产物，
            旧结论留档在 .checker.try&lt;N&gt;，旧转录留在「历史」里。
          </template>
          <template v-else>
            重跑这一章会开一个<b>新尝试</b>：旧的整棵留档在工作区的 work/attempts/，预览页「历史」里两条都在。
          </template>
          请求只写进项目的 work/retry/ 队列——正在跑的 latex 会在转换阶段的安全点处理，没在跑就留到下次启动。
        </div>
        <button class="retry-btn" type="button" :disabled="retryPending" @click="doRetry">
          {{ retryButtonText }}
        </button>
        <div v-if="state.retryNote" class="retry-note">{{ state.retryNote }}</div>
      </section>

      <section v-if="stats" class="detail-block">
        <h3 class="detail-block-title">指标</h3>
        <div class="stats-sub">{{ stats.sub }}</div>
        <div class="tiles">
          <div v-for="t in stats.tiles" :key="t.label" class="tile" :title="t.title || undefined">
            <div class="tile-value">{{ t.value }}</div>
            <div class="tile-label">{{ t.label }}</div>
          </div>
        </div>
        <details class="stats-details" open>
          <summary class="schema-head">
            <span class="schema-name">每次请求明细</span>
            <span class="schema-meta">{{ stats.rowCount }} 行</span>
          </summary>
          <div class="stats-scroll">
            <table class="stats-table">
              <tr><th v-for="h in stats.cols" :key="h">{{ h }}</th></tr>
              <tr v-for="(r, i) in stats.rows" :key="i" class="req-row" :title="r.title">
                <td v-for="(c, j) in r.cells" :key="j">{{ c }}</td>
              </tr>
            </table>
          </div>
        </details>
      </section>

      <section v-if="meta" class="detail-block">
        <h3 class="detail-block-title">元信息</h3>
        <!-- 默认收起：摘要行已写明模型/会话/哈希/字符数；展开状态按会话+哈希记忆 -->
        <details class="disclosure meta-card" :open="metaOpen" @toggle="onMetaToggle">
          <summary>
            <span class="line-slot"><span class="line-caret" /></span>
            <span class="line-name">系统提示词（本次运行快照，不参与回放）</span>
            <span class="line-sep" />
            <span class="line-summary">{{ meta.bits }}</span>
          </summary>
          <div class="schema-body">
            <div class="prompt-scroll">
              <MachineText v-if="meta.mode === 'json'" :text="meta.promptText" cls="body-text prompt-text" />
              <MdBody v-else-if="meta.mode === 'md'" :text="meta.promptText" class="md-body prompt-md" />
              <MachineText v-else :text="meta.promptText" cls="body-text prompt-text" />
            </div>
            <div class="row-actions"><CopyBtn :text="meta.copyText" /></div>
            <div class="meta-tools">
              <div class="meta-tools-head">{{ meta.toolsHead }}</div>
              <div v-if="!meta.tools.length" class="note">这条 meta 行没有记录工具定义。</div>
              <details v-for="(t, i) in meta.tools" :key="i" class="tool-schema">
                <summary class="schema-head">
                  <span class="schema-index">#{{ i + 1 }}</span>
                  <span class="schema-name">{{ t.name }}</span>
                  <span v-if="t.descCount" class="schema-meta">{{ t.descCount }}</span>
                  <span v-if="t.paramCount" class="schema-meta">{{ t.paramCount }}</span>
                </summary>
                <div class="schema-body">
                  <pre v-if="t.desc" class="body-text schema-desc">{{ t.desc }}</pre>
                  <details v-if="t.params" class="schema-params">
                    <summary class="schema-head">
                      <span class="schema-name">parameters</span>
                      <span class="schema-meta">JSON · 默认收起</span>
                    </summary>
                    <div class="schema-body">
                      <MachineText :text="t.params" cls="code" />
                      <div class="row-actions"><CopyBtn :text="t.params" /></div>
                    </div>
                  </details>
                  <div v-else class="note">（这条工具定义没有记录 parameters）</div>
                </div>
              </details>
            </div>
            <div v-if="meta.countNote" class="note" :title="countTip">{{ meta.countNote }}</div>
          </div>
        </details>
      </section>

      <div v-if="!kvRows.length && !stats && !meta" class="note">这个会话没有可显示的详情。</div>
    </template>
  </div>
</template>

<style scoped>
/* ============================ 详情栏 ============================ */
/* .details-head/.details-title 在 App.vue 渲染——规则放 App 的 scoped 块
 * （P11 陷阱①：scoped 样式跟组件走，元素在别的组件里就匹配不上）。 */

.details-body {
 flex: 1; min-height: 0; overflow-y: auto; padding: 10px 14px 40px; 
}

.details-body .hidden {
 display: none !important; 
}

/* P9：轨迹步骤详情块 */
.traj-step-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 2px 0 8px;
}

.traj-step-name {
  font-weight: 600;
  font-size: 12px;
  color: var(--text);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.traj-step-close {
  margin-left: auto;
}

.traj-step-section {
  margin-bottom: 8px;
}

.detail-block {
 margin: 0 0 18px; 
}

.detail-block-title {
  margin: 0 0 6px;
  color: var(--caption);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: .3px;
  text-transform: none;
}

.detail-kv {
 display: grid; grid-template-columns: 68px minmax(0, 1fr); gap: 2px 10px; font-size: 12px; line-height: 20px; 
}

.detail-kv dt {
 color: var(--caption); 
}

.detail-kv dd {
 margin: 0; color: var(--text); word-break: break-all; 
}

.detail-kv dd.mono {
 font-family: var(--mono); font-size: 11.5px; 
}

.tiles {
 display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1px; background: var(--border); border: .5px solid var(--border); border-radius: var(--radius-sm); overflow: hidden; 
}

.tile {
 background: var(--bg); padding: 7px 9px; min-width: 0; 
}

.tile-value {
 font-size: 14px; font-weight: 600; font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; 
}

.tile-label {
 color: var(--caption); font-size: 10.5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; 
}

.stats-table {
 width: 100%; border-collapse: collapse; font-size: 11px; 
}

.stats-table th, .stats-table td {
 padding: 3px 5px; text-align: right; border-bottom: .5px solid var(--hairline); font-variant-numeric: tabular-nums; white-space: nowrap; 
}

.stats-table th:first-child, .stats-table td:first-child {
 text-align: left; 
}

.stats-table th {
 color: var(--caption); font-weight: 500; 
}

.stats-table tbody tr:hover {
 background: var(--hover); 
}

.stats-scroll {
 max-height: 300px; overflow: auto; 
}

.stats-sub {
 color: var(--caption); font-size: 11px; line-height: 18px; margin-top: 6px; 
}

.meta-card {
 border: 0; background: transparent; 
}

.meta-card > summary {
 padding: 4px 0; 
}

.meta-card .line-name {
 color: var(--muted); 
}

.meta-tools {
 margin-top: 8px; 
}

.meta-tools-head {
 color: var(--caption); font-size: 11px; padding: 2px 0 6px; 
}

.tool-schema {
 border-top: .5px solid var(--hairline); 
}

.tool-schema > summary {
 padding: 5px 0; 
}

.schema-head {
 display: flex; align-items: center; gap: 8px; padding: 4px 0; cursor: pointer; list-style: none; 
}

.schema-head::-webkit-details-marker {
 display: none; 
}

.schema-index {
 color: var(--caption); font-family: var(--mono); font-size: 10.5px; 
}

.schema-name {
 font-family: var(--mono); font-weight: 600; font-size: 11.5px; 
}

.schema-meta {
 color: var(--caption); font-size: 10.5px; 
}

.schema-body {
 padding: 2px 0 8px; 
}

.schema-desc {
 margin: 0 0 6px; color: var(--muted); font-size: 11.5px; line-height: 18px; font-family: var(--sans); 
}

.schema-params {
 margin: 0; 
}

.schema-params > .schema-body {
 padding-top: 4px; 
}

/* 详情栏里的系统提示词：比消息正文小一号，标题仍用正文色 */
.prompt-md {
 font-size: 12.5px; line-height: 20px; color: var(--muted); 
}
.retry-hint { color: var(--muted, #6e7781); font-size: 12px; line-height: 18px; margin-bottom: 8px; }
.retry-btn { font-size: 12px; padding: 3px 10px; border: 1px solid var(--border, #d0d7de); border-radius: 6px; background: var(--bg, #fff); cursor: pointer; }
.retry-btn:hover:not(:disabled) { background: var(--hover, #f3f4f6); }
.retry-btn:disabled { opacity: .6; cursor: default; }
.retry-note { margin-top: 6px; font-size: 12px; color: var(--muted, #6e7781); }
</style>
