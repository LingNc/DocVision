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

/* 同一章的角色混在一条链里，行首给角色，编号才不会被误读成"同一件事的第几次"。 */
const ROLE_LABEL: Record<string, string> = { convert: '转换', checker: '核对', continue: '继续', vector: '作图' }
function roleLabel(s: any): string {
  return ROLE_LABEL[s && s.chapterRole] || ''
}

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

/* T194：把"正在看"和"最新一次尝试"分开标——用户原来的困惑是那个「当前」小标签
 * 到底是"页面正在显示"还是"流水线现在用的这一版"。现在：
 *   正在看 = 这一行就是中栏正在显示的会话（点行切换）；
 *   最新   = 这条链里尝试号最大的那一次（真正的最新产物）。
 * 「设为当前」按钮去掉：点行本身就是切换，按钮名字反而让人以为会改流水线状态。 */
function maxAttempt(members: any[]): number {
  let n = 0
  ;(members || []).forEach((s) => { const a = Number(s.chapterAttempt) || 1; if (a > n) n = a })
  return n
}

/* 「最新」只看**还在工作区里的**那几条：归档里的同号转录来自更早一次父尝试，
 * 它的号可能反而更大（父尝试作废后子章节又从 #1 重来），不能拿它当"最新"。 */
const newest = computed(() => {
  const live = (members.value || []).filter((s: any) => !s.archived)
  return maxAttempt(live.length ? live : (members.value || []))
})

/* 链里混着归档转录时给一句解释，否则「旧#1 / 归档」两枚标还是有人会当成重复行。 */
const hasArchived = computed(() => (members.value || []).some((s: any) => !!s.archived))

/* T194：转换与同一轮的核对是**一件事的两个角色**（用户口径："这俩是一体的吧，
 * 如果切换回去就都切换过去"）。历史列表按角色分链，所以从这里给一条直达对方
 * 那一轮的入口——不必去树里找那个子行。 */
function pairOf(s: any): any {
  if (!s || !s.chapterId) return null
  const attempt = Number(s.chapterAttempt) || 1
  const same = (o: any) =>
    o && o.chapterId === s.chapterId &&
    (o.project || '') === (s.project || '') &&
    (Number(o.chapterAttempt) || 1) === attempt &&
    (o.chapterRole || '') !== (s.chapterRole || '')
  const all = (state.sessions || []).filter(same)
  // 同号转录可能不止一份（一次来自归档、一次来自工作区）：先找"同源"的那条，
  // 别把这次的转换配到旧父尝试的核对上。
  return all.find((o: any) => !!o.archived === !!s.archived) || all[0] || null
}

function pairLabel(s: any): string {
  return (s && s.chapterRole === 'checker') ? '同轮转换' : '同轮核对'
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
      <div class="history-head">
          这一章的全部会话（{{ members.length }}，转换与核对都在这里，新 → 旧）
          <span v-if="hasArchived" class="history-old-note" title="带「旧」的是更早一次父尝试留下的转录（那次尝试已作废并搬进 work/attempts/），和这一次的同号行是两段不同的对话">· 带「旧」的是更早一次父尝试（已作废）留下的</span>
        </div>
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
        <span v-if="roleLabel(s)" class="history-role">{{ roleLabel(s) }}</span>
        <span class="row-chip chain-tag history-tag">{{ chainMemberTag(s) }}</span>
        <InlineMD tag="span" class="history-title" :text="sessionTitleOf(s)" :plain="plainTitle(s)"></InlineMD>
        <span v-if="isCurrent(s)" class="history-cur">正在看</span>
        <span v-if="(Number(s.chapterAttempt) || 1) === newest" class="history-latest">最新</span>
        <span v-if="s.archived" class="history-arch" title="这份转录在 work/attempts 归档目录里（工作树那份已被搬走），不是当前工作区">归档</span>
        <span class="history-state" :class="endStateCls(s)">{{ endStateText(s) }}</span>
        <span class="history-time">{{ relTime(s.mtime) }}</span>
        <span class="history-actions">
          <button v-if="!isCurrent(s)" class="history-act" type="button" title="在中栏显示这条会话（只是切视图，不改流水线状态）" @click.stop="onRowClick(s)">查看</button>
          <button
            v-if="pairOf(s) && state.current.id !== pairOf(s).id"
            class="history-act"
            type="button"
            :title="'切到同一轮（' + chainMemberTag(s) + '）的' + (s.chapterRole === 'checker' ? '转换' : '核对') + '会话：' + pairOf(s).id"
            @click.stop="onRowClick(pairOf(s))"
          >{{ pairLabel(s) }}</button>
          <button v-if="canDropOf(s)" class="history-act act-danger" type="button" title="删掉这条转录（作废尝试的归档一并删除）" @click.stop="askDrop(s)">删除</button>
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.history-old-note { color: var(--caption); font-weight: 400; }

.history-role { flex: none; font-size: 11px; color: var(--caption); min-width: 22px; }

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

.history-latest {
  font-size: 11px;
  color: var(--ok, #15803d);
  border: 1px solid color-mix(in srgb, var(--ok, #15803d) 40%, var(--line, #d0d7de));
  border-radius: 4px;
  padding: 0 4px;
  line-height: 16px;
}

.history-arch {
  font-size: 11px;
  color: var(--caption);
  border: 1px dashed var(--line, #d0d7de);
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
