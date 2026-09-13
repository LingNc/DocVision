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
