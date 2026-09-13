<script setup lang="ts">
// 中栏「对话」页：stream 摘要 + 消息条目流。条目来自 legacy/stream.ts 的
// streamModel()（computed 响应式：会话切换 / 行追加 / Markdown / 仅看工具 /
// 折叠思考 / 图片归属全部自动重算，不再有 renderTimeline 手动重渲链）。
import { computed, nextTick, onMounted, watch } from 'vue'
import { state, LONG_TEXT_LINES, clearAnchors } from '../state'
import { estOf } from '../legacy/sidebar'
import { streamModel } from '../legacy/stream'
import StreamSummary from './StreamSummary.vue'
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
      <div v-if="!model.items.length" class="empty">{{ emptyText }}</div>
    </div>
  </div>
</template>
