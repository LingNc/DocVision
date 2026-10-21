<script setup lang="ts">
// 「历史」页：当前会话所在 img2text 调用链（成员数 > 1）的全部成员列表。
// 数据来自纯函数 chainOfSession（legacy/sidebar.ts），computed 按
// state.sessions / state.current 引用缓存，轮询换新数组时才重扫一遍。
// 排序与侧栏链一致（sortChainMembers：新 → 旧）；点行切会话（selectSession）。
import { computed, ref } from 'vue'
import { state, relTime } from '../state'
import { chainOfSession, chainMemberTag, plainTitle, sessionTitleOf } from '../legacy/sidebar'
import { dropSession, refreshIndex, selectSession } from '../data'
import { canDropOf } from '../legacy/actions'
import { checkerRounds } from '../legacy/rounds'
import { jumpToLine } from '../legacy/trajectory'
import InlineMD from './InlineMD.vue'

const members = computed(() => chainOfSession(state.sessions, state.current ? (state.current as any).id : null))

function endStateText(s: any): string {
  if (s.live) return '运行中'
  if (s.endState === 'done') return '已完成'
  if (s.endState === 'error') return '错误'
  return '未完成'
}

function endStateCls(s: any): string {
  if (s.live) return 'live'
  if (s.endState === 'done') return 'done'
  if (s.endState === 'error') return 'error'
  return 'pend'
}

function onRowClick(s: any) {
  selectSession(s.id)
}

/* P56：历史里每一行给两个行内动作——「设为当前」（下次继续跑就接着它）与
 * 「删除这一条」（转错的尝试有出口，整条从历史里消失、归档目录一起删）。
 * 删除要二次确认：这里用行内确认条，不用 window.confirm。 */
/* T188：核对会话一次尝试里被打回几轮，全挤在同一条转录里（磁盘格式不动）。
 * 历史里按 user 轮拆成子条目，点一轮就跳到那一轮的起始行——"第 1 轮为什么被
 * 拒"不用再翻整条转录。 */
const rounds = computed(() => {
  const s: any = state.current
  if (!s || s.chapterRole !== 'checker' || !Array.isArray(state.lines)) return []
  const r = checkerRounds(state.lines as any[])
  return r.length > 1 ? r : []
})

const dropAsk = ref<any>(null)
const dropNote = ref('')

function isCurrent(s: any): boolean {
  return !!state.current && state.current.id === s.id
}

function askDrop(s: any) {
  if (!canDropOf(s)) return
  dropAsk.value = s
}

function doDrop() {
  const s = dropAsk.value
  if (!s) return
  dropAsk.value = null
  dropNote.value = '正在删除 ' + s.name + '…'
  dropSession(String(s.project), String(s.path))
    .then((res: any) => {
      dropNote.value = '已删除 ' + s.name + ((res && res.archive) ? '（含 ' + res.archive + '）' : '')
      return refreshIndex()
    })
    .catch((err: any) => { dropNote.value = '删除失败：' + ((err && err.message) || err) })
}
</script>

<template>
  <div id="history" class="history" :class="{ hidden: state.view !== 'history' }">
    <div v-if="!members" class="history-empty">当前会话不属于任何调用链 / 重试链</div>
    <div v-else class="history-list">
      <div class="history-head">同一件事的全部会话（{{ members.length }}，新 → 旧）</div>
      <div v-if="dropAsk" class="history-confirm" role="alert">
        <span>删掉「{{ dropAsk.name }}」这条记录？转录会从历史里消失，作废尝试的归档目录也会一起删。</span>
        <button class="history-act act-danger" type="button" @click="doDrop">确认删除</button>
        <button class="history-act" type="button" @click="dropAsk = null">取消</button>
      </div>
      <div v-if="dropNote" class="history-note">{{ dropNote }}</div>
      <div v-if="rounds.length" class="history-rounds">
        <div class="history-rounds-head">这条核对转录里的 {{ rounds.length }} 轮（新 → 旧同序，点一轮跳到那一轮的起点）</div>
        <button
          v-for="r in rounds"
          :key="'r' + r.round"
          class="history-round"
          type="button"
          :title="r.summary || '跳到第 ' + r.round + ' 轮'"
          @click="jumpToLine(r.from)"
        >
          <span class="row-chip chain-tag history-tag">轮 {{ r.round }}</span>
          <span class="history-round-state" :class="r.verdict === 'pass' ? 'ok' : r.verdict === 'issues' ? 'bad' : ''">
            {{ r.verdict === 'pass' ? '通过' : r.verdict === 'issues' ? '拒绝 ' + r.issues + ' 条' : '无结论' }}
          </span>
          <span class="history-round-from">第 {{ r.from }} 行 · {{ r.lines }} 行</span>
        </button>
      </div>
      <div
        v-for="s in members"
        :key="s.id"
        class="history-row"
        :class="{ active: isCurrent(s) }"
        role="button"
        tabindex="0"
        :title="s.id"
        @click="onRowClick(s)"
        @keydown.enter="onRowClick(s)"
      >
        <span class="row-chip chain-tag history-tag">{{ chainMemberTag(s) }}</span>
        <InlineMD tag="span" class="history-title" :text="sessionTitleOf(s)" :plain="plainTitle(s)"></InlineMD>
        <span v-if="isCurrent(s)" class="history-cur">当前</span>
        <span class="history-state" :class="endStateCls(s)">{{ endStateText(s) }}</span>
        <span class="history-time">{{ relTime(s.mtime) }}</span>
        <span class="history-actions">
          <button v-if="!isCurrent(s)" class="history-act" type="button" title="把这条设为当前会话（继续跑/重试都以它为准）" @click.stop="onRowClick(s)">设为当前</button>
          <button v-if="canDropOf(s)" class="history-act act-danger" type="button" title="删掉这条转录（作废尝试的归档一并删除）" @click.stop="askDrop(s)">删除</button>
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.history-rounds { margin: 0 4px 10px; }

.history-rounds-head { font-size: 11px; color: var(--caption); margin-bottom: 4px; }

.history-round {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 2px 8px;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text);
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.history-round:hover { background: var(--hover, #f3f4f6); }

.history-round-state { font-size: 11px; }

.history-round-state.ok { color: #15803d; }

.history-round-state.bad { color: #b45309; }

.history-round-from { margin-left: auto; font-size: 11px; color: var(--caption); }

.history-confirm {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin: 0 4px 8px;
  padding: 8px 10px;
  border: 1px solid color-mix(in srgb, #dc2626 40%, var(--line, #d0d7de));
  border-radius: var(--radius-sm);
  font-size: 12px;
  color: var(--caption);
}

.history-note {
  margin: 0 4px 8px;
  font-size: 12px;
  color: var(--caption);
}

.history-cur {
  font-size: 11px;
  color: var(--accent, #3b82f6);
  border: 1px solid color-mix(in srgb, var(--accent, #3b82f6) 45%, var(--line, #d0d7de));
  border-radius: 4px;
  padding: 0 4px;
  line-height: 16px;
}

.history-actions {
  display: flex;
  gap: 6px;
  margin-left: auto;
  opacity: 0;
  transition: opacity .12s;
}

.history-row:hover .history-actions,
.history-row:focus-within .history-actions {
  opacity: 1;
}

.history-act {
  border: 1px solid var(--line, #d0d7de);
  background: transparent;
  color: var(--text);
  border-radius: 5px;
  padding: 1px 8px;
  font-size: 11px;
  line-height: 18px;
  cursor: pointer;
}

.history-act.act-danger {
  border-color: color-mix(in srgb, #dc2626 50%, var(--line, #d0d7de));
  color: #dc2626;
}

.history {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 12px 28px 20px 20px;
}

.history.hidden {
  display: none;
}

.history-empty {
  padding: 16px 4px;
  color: var(--caption);
  font-size: 13px;
}

.history-head {
  padding: 0 4px 8px;
  color: var(--caption);
  font-size: 12px;
}

.history-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 36px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background .12s;
}

.history-row:hover {
  background: var(--hover);
}

.history-row.active {
  background: var(--active);
}

.history-tag {
  flex: none;
  color: var(--caption);
  font-size: 11px;
  white-space: nowrap;
  border: .5px solid var(--border);
  border-radius: 999px;
  padding: 0 6px;
  line-height: 16px;
}

.history-title {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  line-height: 20px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.history-row.active .history-title {
  font-weight: 500;
}

.history-state {
  flex: none;
  font-size: 12px;
  line-height: 20px;
  color: var(--caption);
}

.history-state.live {
  color: var(--ok);
}

.history-state.done {
  color: var(--ok);
}

.history-state.error {
  color: var(--err);
}

.history-time {
  flex: none;
  color: var(--caption);
  font-size: 12px;
  line-height: 20px;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 1023px) {
  .history { padding: 10px 16px 16px 12px; }
}
</style>
