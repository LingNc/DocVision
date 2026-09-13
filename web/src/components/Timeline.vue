<script setup lang="ts">
// 对话时间线容器：渲染本体在 legacy/timeline.ts（命令式 DOM——图片归属要
// 跨节点手术，虚拟 DOM 不适合表达这种引用）。这里只负责容器与"什么时候
// 重渲"：会话切换 / 正文与行数据变化 / Markdown 与仅看工具调用开关。
import { watch } from 'vue'
import { state } from '../state'
import { renderTimeline } from '../legacy/timeline'

watch(
  // 数组多源形式（逐元素比较）：state.current 每次轮询都被换成新对象，
  // getter 返回新数组的写法会因引用不同每 2 秒误触发一次整流重渲。
  [
    () => (state.current ? state.current.id : ''),
    () => state.lines.length,
    () => state.markdown,
    () => state.onlyTools,
    () => state.unit,
  ],
  () => { renderTimeline() },
)
</script>

<template>
  <div id="timeline" class="timeline" :class="{ hidden: state.view !== 'chat' }"></div>
</template>
