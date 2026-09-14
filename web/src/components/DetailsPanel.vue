<script setup lang="ts">
// 右侧详情栏整体组件：会话块（键值表）+ 指标块（瓦片 + 每次请求明细表）+
// 元信息卡（系统提示词快照 + 工具 schema）。数据全部来自 legacy/details.ts
// 的纯模型函数（computed 响应式驱动，不再有 renderDetails 手动重渲链）。
import { computed, ref, watch } from 'vue'
import { state, storeSet } from '../state'
import { metaModel, META_COUNT_TIP, sessionKVRows, sessionSubPath, statsModel } from '../legacy/details'
import InlineMD from './InlineMD.vue'
import MachineText from './MachineText.vue'
import MdBody from './MdBody.vue'
import CopyBtn from './CopyBtn.vue'

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

const countTip = computed(() =>
  meta.value?.countNote ? META_COUNT_TIP.replace('{n}', meta.value.countNote.replace(/^共 (\d+) 条.*$/, '$1')) : '',
)
</script>

<template>
  <div id="details-body" class="details-body">
    <div v-if="!state.current" class="note">左侧选择一个会话后，这里显示它的指标与元信息。</div>
    <template v-else>
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

.details-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 8px 0 14px;
  border-bottom: .5px solid var(--border);
}

.details-title {
 flex: 1; min-width: 0; font-size: 13px; font-weight: 600; color: var(--muted); letter-spacing: .2px; 
}

.details-body {
 flex: 1; min-height: 0; overflow-y: auto; padding: 10px 14px 40px; 
}

.details-body .hidden {
 display: none !important; 
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
</style>
