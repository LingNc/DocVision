<script setup lang="ts">
// 对话时间线容器：渲染本体在 legacy/timeline.ts（命令式 DOM——图片归属要
// 跨节点手术，虚拟 DOM 不适合表达这种引用）。这里只负责容器与"什么时候
// 重渲"：会话切换 / 正文与行数据变化 / Markdown 与仅看工具调用开关。
import { watch } from 'vue'
import { state } from '../state'
import { renderTimeline } from '../legacy/timeline'

watch(
  () => [state.current && state.current.id, state.lines, state.markdown, state.onlyTools, state.unit],
  () => { renderTimeline() },
)
</script>

<template>
  <div id="timeline" class="timeline" :class="{ hidden: state.view !== 'chat' }"></div>
</template>
