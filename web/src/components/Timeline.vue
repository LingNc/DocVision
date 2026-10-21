<script setup lang="ts">
// 中栏「对话」页：stream 摘要 + 消息条目流。条目来自 legacy/stream.ts 的
// streamModel()（computed 响应式：会话切换 / 行追加 / Markdown / 仅看工具 /
// 折叠思考 / 图片归属全部自动重算，不再有 renderTimeline 手动重渲链）。
import { computed, nextTick, onMounted, watch } from 'vue'
import { state, LONG_TEXT_LINES, clearAnchors } from '../state'
import { estOf } from '../legacy/sidebar'
import { streamModel } from '../legacy/stream'
import StreamSummary from './StreamSummary.vue'
import PartialTail from './PartialTail.vue'
import AssistantMsg from './AssistantMsg.vue'
import SystemMsg from './SystemMsg.vue'
import ImageTurn from './ImageTurn.vue'
import ResultMsg from './ResultMsg.vue'
import FoldText from './FoldText.vue'
import { dropFromLine, forkFromLine } from '../fork'
import { canForkFrom } from '../legacy/fork'

const model = computed(() => streamModel())

/* P57：对话页每一条消息也能 fork（以前只有「轨迹」页每行有 ⤓，用户在对话页
 * 找不到入口）。判据与轨迹页同一处（legacy/fork.ts）。 */
const canFork = computed(() => canForkFrom(state.current))

const emptyText = computed(() => {
  if (state.onlyTools) return '这个会话没有工具调用记录'
  if (!state.current) return '左侧选择一个会话开始浏览。'
  return '这个会话还没有可显示的消息'
})

/* 手动滚离底部 → 自动取消跟随（旧页 timeline scroll 监听的同一语义）。 */
onMounted(() => {
  const t = document.getElementById('timeline')
  t?.addEventListener('scroll', () => {
    const near = t.scrollHeight - t.scrollTop - t.clientHeight < 40
    if (!near && state.follow) state.follow = false
  })
})

/* 自动跟随：行追加 / 切会话后滚到底部；切会话先清锚点表防串台。 */
watch(
  [() => (state.current ? state.current.id : ''), () => state.lines.length],
  async (_, prev) => {
    if (prev[0] && prev[0] !== (state.current ? state.current.id : '')) {
      clearAnchors()
    }
    if (state.follow) {
      await nextTick()
      const t = document.getElementById('timeline')
      if (t) t.scrollTop = t.scrollHeight
    }
  },
)
</script>

<template>
  <div id="timeline" class="timeline" :class="{ hidden: state.view !== 'chat' }">
    <div class="stream">
      <StreamSummary v-if="state.current" />
      <template v-for="item in model.items" :key="item.key">
        <div class="fork-wrap">
          <AssistantMsg v-if="item.type === 'assistant'" :item="item" />
          <SystemMsg v-else-if="item.type === 'system'" :item="item" />
          <ImageTurn v-else-if="item.type === 'imageTurn'" :item="item" />
          <ResultMsg v-else-if="item.type === 'result'" :item="item" />
          <section v-else class="msg msg-other">
            <FoldText :text="String(item.line.text || '(无正文)')" :mem-key="'other.' + item.line.n"
              :preview-lines="LONG_TEXT_LINES" :tokens="estOf(item.line).text" />
          </section>
          <span v-if="canFork" class="fork-acts">
            <button
              class="fork-btn"
              type="button"
              :title="'复制一份并从这里开始（fork）：开一个新尝试接着干，源记录保留在「历史」里。想就地改就先在目标会话上操作，不必 fork。'"
              @click.stop="forkFromLine(item.line.n, '对话')"
            >⤓</button>
            <button
              class="fork-btn fork-drop"
              type="button"
              :title="'删掉这条及之后的记录并继续（就地截断这条会话，不新开尝试；原文先备份到 work/sessions/.trash/）'"
              @click.stop="dropFromLine(item.line.n, '对话')"
            >⤫</button>
          </span>
        </div>
      </template>
      <div v-if="!model.items.length && !state.partial" class="empty">{{ emptyText }}</div>
      <!-- P7：流式尾——当前会话正在生成的那条消息的实时快照 -->
      <PartialTail v-if="state.current && state.current.live && state.partial" :partial="state.partial" />
      <!-- 会话结局（t=meta kind=run_end，T99）：meta 行按设计不进消息流，于是
           "报错终止"的会话在对话页看起来就是个空会话——原因只能去 jsonl 里找。
           这里把终止原因直接放在对话末尾；正常结束不占位，运行中也不占位。 -->
      <section
        v-if="state.current && !state.current.live && state.current.runEndError"
        class="msg msg-end"
      >
        <div class="end-title">会话结局 · 错误终止</div>
        <div class="end-detail">{{ state.current.runEndError }}</div>
      </section>
    </div>
  </div>
</template>

<style scoped>
/* fork 按钮：悬浮在这一条消息右上角，平时不抢戏（与轨迹页的 ⤓ 同一个动作）。 */
.fork-wrap { position: relative; }

.fork-btn {
  border: 1px solid var(--line, #d0d7de);
  background: var(--bg, #fff);
  color: var(--caption);
  border-radius: 5px;
  padding: 0 5px;
  font-size: 12px;
  line-height: 18px;
  cursor: pointer;
  opacity: 0;
  transition: opacity .12s;
}

.fork-acts {
  position: absolute;
  top: 2px;
  right: 0;
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity .12s;
}

.fork-wrap:hover .fork-acts,
.fork-acts:focus-within {
  opacity: 1;
}

.fork-drop {
  color: #b45309;
  border-color: color-mix(in srgb, #d97706 50%, var(--line, #d0d7de));
}

/* ============================ 对话（DSH 的消息形态） ============================ */

.timeline {
 flex: 1; overflow-y: auto; padding: 16px 24px 80px; 
}

.stream {
 max-width: var(--content-w); margin: 0 auto; display: flex; flex-direction: column; 
}

/* 回合边界**只有一种画法**：8px + 0.5px 虚线 + 8px（--divider-gap）。虚线只出现在
   section 之间（也就是回合之间），同一条消息里的工具行之间永远只是 --row-gap。 */
.stream > .msg + .msg,
/* P57：每条消息外面多了一层 .fork-wrap（装 ⤓ 按钮），分隔线规则要跨过它——
   否则相邻两条消息不再是兄弟，回合之间那条虚线就整片消失。 */
.stream > .fork-wrap + .fork-wrap > .msg {
  margin-top: var(--divider-gap);
  padding-top: var(--divider-gap);
  border-top: .5px dashed var(--border);
}

.empty {
 padding: 56px 24px; color: var(--dim); text-align: center; 
}

/* 原 160 号规则里的 msg-other 半边（msg-system 半边在 SystemMsg.vue） */
.msg-other .body-text { color: var(--muted); font-size: 13px; }

/* 会话结局（run_end ok=false）：一句原因，红边框 + 安静底色，不抢正文。 */
.msg-end {
  border-left: 3px solid var(--err);
  background: color-mix(in srgb, var(--err) 8%, transparent);
  border-radius: 6px;
  padding: 10px 12px;
}
.msg-end .end-title { font-size: 12px; color: var(--err); font-weight: 600; }
.msg-end .end-detail {
  margin-top: 4px; font-family: var(--mono);
  font-size: var(--code-font); line-height: var(--code-line);
  word-break: break-word; white-space: pre-wrap; color: var(--text);
}

@media (max-width: 1023px) {
  .timeline { padding: 12px 12px 60px; }
}
</style>
