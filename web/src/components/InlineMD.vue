<script setup lang="ts">
// 行内 Markdown 落点（旧页 nameNode 的 Vue 形态）：内容经 renderInlineMarkdown
// 用 DOM API 插入，从不拼 HTML。名字里没有标记时结果与纯文本一致。
import { onMounted, ref, watch } from 'vue'
import { renderInlineMarkdown } from '../legacy/markdown'

// plain：按纯文本插入、不走行内 markdown（章节会话的标题是流水线标识符
// `chapter_004_child_01`，渲染 markdown 会把下划线当斜体吃掉——T176）。
const props = defineProps<{ tag?: string; text: string; plain?: boolean }>()
const host = ref<HTMLElement | null>(null)

function render() {
  const el = host.value
  if (!el) return
  el.textContent = ''
  el.appendChild(props.plain ? document.createTextNode(props.text) : renderInlineMarkdown(props.text))
}

onMounted(render)
watch(() => [props.text, props.plain], render)
</script>

<template>
  <component :is="props.tag || 'span'" ref="host"></component>
</template>
