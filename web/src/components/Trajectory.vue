<script setup lang="ts">
// 「轨迹」页整表组件：工具栏筛选 chips + 事件表 + 就地展开。
// 行模型仍来自 legacy/trajectory.ts 的 trajectoryRows()（照搬旧页语义），
// 渲染全部走模板——trajKinds/trajOpen 是响应式的，筛选与展开不再需要
// 手动重渲（迁移期命令式版本的收尾）。
import { computed } from 'vue'
import { state } from '../state'
import { countValue, fmtDur, unitLabel } from '../legacy/sidebar'
import { jumpToLine, TRAJ_KINDS, trajectoryRows } from '../legacy/trajectory'
import MachineText from './MachineText.vue'
import CopyBtn from './CopyBtn.vue'
import ImageStrip from './ImageStrip.vue'

const rows = computed(() => (state.current ? trajectoryRows() : []))
const visible = computed(() => rows.value.filter(trajVisible))

/* 类型 chip 按转录里的真实角色列（对话页是合并后的呈现，两者语义不同）。 */
const kinds = computed(() =>
  TRAJ_KINDS
    .map((k) => ({ ...k, count: rows.value.filter((r) => r.kind === k.id).length }))
    .filter((k) => k.count > 0),
)

const allPressed = computed(() => Object.keys(state.trajKinds).length === 0)

function trajVisible(row: any): boolean {
  const picked = Object.keys(state.trajKinds).filter((k) => state.trajKinds[k])
  if (!picked.length) return true
  return picked.indexOf(row.kind) >= 0
}

function pick(k: string) {
  if (state.trajKinds[k]) delete state.trajKinds[k]
  else state.trajKinds[k] = true
}

function sizeCell(row: any): string {
  return state.unit === 'char'
    ? (row.chars ? String(row.chars) : '—')
    : (row.tokens ? countValue(row.chars, row.tokens) : '—')
}

function statusCell(row: any): { cls: string; text: string } {
  if (row.status === 'error') return { cls: 'error', text: '✗ error' }
  if (row.status === 'ok') return { cls: 'ok', text: '✓ ok' }
  return { cls: 'plain', text: (row.status === 'plain' || !row.status) ? '—' : row.status }
}

/* 展开区段落标题的唯一来源（与命令式版本的 labels 一致）。 */
const DETAIL_LABELS: Record<string, string> = {
  prompt: '系统提示词', thinking: '思考', user: '用户消息', message: '助手消息',
  input: '输入', output: '输出', request: '用量行',
}

/* 输入 / 输出 / 用量行与对话页同源：JSON 高亮，其余纯文本。 */
function detailKind(key: string): 'code' | 'plain' {
  return key === 'input' || key === 'request' || key === 'output' ? 'code' : 'plain'
}

function rowKey(row: any, idx: number): string {
  return 'r' + idx + ':' + (row.jump || row.name)
}
</script>

<template>
  <div id="trajectory" class="trajectory" :class="{ hidden: state.view === 'chat' }">
    <div class="traj-toolbar">
      <div class="traj-toolbar-inner">
        <div class="traj-filters">
          <button type="button" class="traj-chip" :aria-pressed="allPressed ? 'true' : 'false'"
            @click="state.trajKinds = {}">全部</button>
          <button v-for="k in kinds" :key="k.id" type="button" class="traj-chip"
            :title="'只看 / 不看「' + k.label + '」'"
            :aria-pressed="state.trajKinds[k.id] ? 'true' : 'false'"
            @click="pick(k.id)">{{ k.label }} {{ k.count }}</button>
        </div>
        <span class="traj-count">{{ visible.length }} / {{ rows.length }} 步</span>
      </div>
    </div>
    <div class="traj-scroll">
      <div v-if="!visible.length" class="traj-empty">
        {{ rows.length ? '当前筛选没有匹配的步骤' : (state.current ? '这个会话还没有步骤' : '左侧选择一个会话后，这里列出它的全部步骤。') }}
      </div>
      <table v-else class="traj-table">
        <colgroup>
          <col class="col-n"><col class="col-kind"><col class="col-name"><col>
          <col class="col-status"><col class="col-size"><col class="col-time">
        </colgroup>
        <thead>
          <tr>
            <th class="num-head">#</th><th>类型</th><th>名称</th><th>摘要</th><th>状态</th>
            <th class="num-head">{{ unitLabel() }}</th><th class="num-head">耗时</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="(row, idx) in visible" :key="rowKey(row, idx)">
            <tr class="traj-row" :data-kind="row.kind" :data-error="row.status === 'error' ? 'true' : undefined"
              :title="row.title || '点击跳到对话里对应的那条消息'"
              @click="row.jump ? jumpToLine(row.jump) : (state.trajOpen[rowKey(row, idx)] = !state.trajOpen[rowKey(row, idx)])">
              <td class="traj-num">
                <button type="button" class="traj-disclose" title="展开完整输入输出"
                  @click.stop="state.trajOpen[rowKey(row, idx)] = !state.trajOpen[rowKey(row, idx)]">{{ state.trajOpen[rowKey(row, idx)] ? '▾' : '▸' }}</button>{{ row.jump ? String(row.jump) : '—' }}
              </td>
              <td><span class="kind-tag" :class="row.status === 'error' ? 'kind-error' : 'kind-' + row.kind">{{ row.tag }}</span></td>
              <td class="traj-name">{{ row.name }}</td>
              <td class="traj-summary" :title="row.summary || ''">{{ row.summary || '—' }}</td>
              <td :class="'traj-status ' + statusCell(row).cls">{{ statusCell(row).text }}</td>
              <td class="traj-num-cell">{{ sizeCell(row) }}</td>
              <td class="traj-num-cell">{{ row.time ? fmtDur(row.time) : '—' }}</td>
            </tr>
            <tr v-if="state.trajOpen[rowKey(row, idx)]" class="traj-detail">
              <td :colspan="7">
                <div class="traj-detail-inner">
                  <div v-for="(text, key) in row.detail || {}" :key="key">
                    <div class="traj-detail-title">{{ DETAIL_LABELS[key] || key }}</div>
                    <MachineText v-if="detailKind(key) === 'code'" :text="String(text || '')" cls="code" />
                    <pre v-else class="code">{{ String(text || '') }}</pre>
                    <div class="row-actions"><CopyBtn :text="String(text || '')" /></div>
                  </div>
                  <ImageStrip v-if="row.images && row.images.length" :images="row.images" />
                </div>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
/* ============================ 轨迹（DSH Trajectory 的表） ============================ */

.trajectory {
 flex: 1; min-height: 0; display: flex; flex-direction: column; overflow: hidden; 
}

.traj-toolbar {
  flex: none;
  position: sticky;
  top: 0;
  z-index: 4;
  height: 32px;
  border-bottom: .5px solid var(--border);
  background: var(--bg);
}

.traj-toolbar-inner {
 display: flex; align-items: center; gap: 8px; height: 100%; padding: 0 12px; 
}

.traj-filters {
 display: flex; align-items: center; gap: 2px; flex: 1; min-width: 0; overflow: hidden; 
}

.traj-chip {
  flex: none;
  height: 20px;
  padding: 0 7px;
  border: 0;
  border-radius: 3px;
  background: transparent;
  color: var(--caption);
  font: inherit;
  font-size: 12px;
  line-height: 20px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.traj-chip:hover {
 color: var(--text); background: var(--hover); 
}

.traj-chip[aria-pressed="true"] {
 color: var(--accent); background: var(--accent-soft); 
}

.traj-count {
 flex: none; margin-left: auto; color: var(--caption); font-size: 12px; font-variant-numeric: tabular-nums; 
}

.traj-scroll {
 flex: 1; min-height: 0; overflow: auto; 
}

.traj-table {
 table-layout: fixed; width: 100%; border-spacing: 0; font-size: 12px; color: var(--text); 
}

.traj-table th {
  position: sticky;
  top: 0;
  z-index: 3;
  height: 30px;
  padding: 0 8px;
  border-bottom: .5px solid var(--border);
  background: var(--sidebar-bg);
  color: var(--dim);
  font-weight: 500;
  font-size: 12px;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  user-select: none;
}

.traj-table td {
  height: 30px;
  padding: 0 8px;
  border-bottom: .5px solid var(--hairline);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.traj-table tbody tr.traj-row {
 cursor: pointer; position: relative; 
}

.traj-table tbody tr.traj-row:hover {
 background: var(--hover); 
}

.traj-table tbody tr.traj-row.selected {
 background: var(--active); 
}

.traj-table tbody tr.traj-row[data-error="true"] td:first-child {
 box-shadow: inset 3px 0 0 var(--err); 
}

.traj-table tbody tr.traj-row.selected td:first-child {
 box-shadow: inset 3px 0 0 var(--accent); 
}

.col-n {
 width: 46px; 
}

.col-kind {
 width: 78px; 
}

.col-name {
 width: 150px; 
}

.col-status {
 width: 66px; 
}

.col-size {
 width: 72px; 
}

.col-time {
 width: 72px; 
}

/* 数字列的表头与数值一起右对齐（#、字符、耗时） */
.traj-table th.num-head {
 text-align: right; 
}

.traj-num {
 color: var(--caption); font-family: var(--mono); font-size: 11px; text-align: right; 
}

.traj-name {
 font-family: var(--mono); font-size: 12px; color: var(--muted); 
}

.traj-summary {
 color: var(--muted); 
}

.traj-table tbody tr[data-kind="tool"] .traj-summary,
.traj-table tbody tr[data-kind="result"] .traj-summary {
 font-family: var(--mono); font-size: 11.5px; 
}

.traj-table tbody tr[data-kind="think"] .traj-summary {
 color: var(--dim); font-style: normal; 
}

.traj-num-cell {
 text-align: right; color: var(--caption); font-variant-numeric: tabular-nums; font-family: var(--mono); font-size: 11px; 
}

.traj-status {
 font-size: 11.5px; 
}

.traj-status.ok {
 color: var(--ok); 
}

.traj-status.error {
 color: var(--err); 
}

.traj-status.plain {
 color: var(--caption); 
}

.traj-disclose {
  width: 18px;
  height: 18px;
  margin-right: 4px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--caption);
  font: inherit;
  font-size: 10px;
  line-height: 18px;
  cursor: pointer;
  padding: 0;
}

.traj-disclose:hover {
 background: var(--hover); color: var(--text); 
}

.traj-jump {
  border: 0;
  background: transparent;
  color: var(--accent);
  font: inherit;
  font-size: 11.5px;
  cursor: pointer;
  padding: 0;
  opacity: 0;
}

.traj-table tbody tr.traj-row:hover .traj-jump {
 opacity: 1; 
}

/* 种类标签（照 DSH 的 kindTag：10px / 650 字重 / 4px 圆角 / 19px 高） */
.kind-tag {
  display: inline-flex;
  align-items: center;
  height: 19px;
  padding: 0 5px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 650;
  letter-spacing: .035em;
  line-height: 16px;
  user-select: none;
  white-space: nowrap;
}

.kind-user {
 color: var(--kind-user-fg); background: var(--kind-user-bg); 
}

.kind-msg {
 color: var(--kind-msg-fg); background: var(--kind-msg-bg); 
}

.kind-think {
 color: var(--kind-think-fg); background: var(--kind-think-bg); 
}

.kind-tool {
 color: var(--kind-tool-fg); background: var(--kind-tool-bg); 
}

.kind-error {
 color: var(--kind-err-fg); background: var(--kind-err-bg); 
}

.kind-meta {
 color: var(--kind-meta-fg); background: var(--kind-meta-bg); 
}

.kind-result {
 color: var(--kind-tool-fg); background: var(--kind-tool-bg); 
}

.kind-usage {
 color: var(--kind-usage-fg); background: var(--kind-usage-bg); 
}

.traj-detail > td {
 height: auto; padding: 0; white-space: normal; overflow: visible; background: var(--surface-2); 
}

.traj-detail-inner {
 padding: 8px 8px 12px 54px; display: flex; flex-direction: column; gap: 8px; 
}

.traj-detail-title {
 color: var(--caption); font-size: 11px; 
}

.traj-empty {
 padding: 40px 24px; color: var(--dim); text-align: center; 
}

@media (max-width: 1023px) {
  .col-name { width: 110px; }
  .col-status { width: 56px; }
  .col-size { width: 58px; }
  .col-time { width: 58px; }
}
</style>
