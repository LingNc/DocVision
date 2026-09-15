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

const model = computed(() => streamModel())

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
        <AssistantMsg v-if="item.type === 'assistant'" :item="item" />
        <SystemMsg v-else-if="item.type === 'system'" :item="item" />
        <ImageTurn v-else-if="item.type === 'imageTurn'" :item="item" />
        <ResultMsg v-else-if="item.type === 'result'" :item="item" />
        <section v-else class="msg msg-other">
          <FoldText :text="String(item.line.text || '(无正文)')" :mem-key="'other.' + item.line.n"
            :preview-lines="LONG_TEXT_LINES" :tokens="estOf(item.line).text" />
        </section>
      </template>
      <div v-if="!model.items.length && !state.partial" class="empty">{{ emptyText }}</div>
      <!-- P7：流式尾——当前会话正在生成的那条消息的实时快照 -->
      <PartialTail v-if="state.current && state.current.live && state.partial" :partial="state.partial" />
    </div>
  </div>
</template>

<style scoped>
/* ============================ 对话（DSH 的消息形态） ============================ */

.timeline {
 flex: 1; overflow-y: auto; padding: 16px 24px 80px; 
}

.stream {
 max-width: var(--content-w); margin: 0 auto; display: flex; flex-direction: column; 
}

/* 回合边界**只有一种画法**：8px + 0.5px 虚线 + 8px（--divider-gap）。虚线只出现在
   section 之间（也就是回合之间），同一条消息里的工具行之间永远只是 --row-gap。 */
.stream > .msg + .msg {
  margin-top: var(--divider-gap);
  padding-top: var(--divider-gap);
  border-top: .5px dashed var(--border);
}

.empty {
 padding: 56px 24px; color: var(--dim); text-align: center; 
}

/* 原 160 号规则里的 msg-other 半边（msg-system 半边在 SystemMsg.vue） */
.msg-other .body-text { color: var(--muted); font-size: 13px; }

@media (max-width: 1023px) {
  .timeline { padding: 12px 12px 60px; }
}
</style>
