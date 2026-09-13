<script setup lang="ts">
// 行内 Markdown 落点（旧页 nameNode 的 Vue 形态）：内容经 renderInlineMarkdown
// 用 DOM API 插入，从不拼 HTML。名字里没有标记时结果与纯文本一致。
import { onMounted, ref, watch } from 'vue'
import { renderInlineMarkdown } from '../legacy/markdown'

const props = defineProps<{ tag?: string; text: string }>()
const host = ref<HTMLElement | null>(null)

function render() {
  const el = host.value
  if (!el) return
  el.textContent = ''
  el.appendChild(renderInlineMarkdown(props.text))
}

onMounted(render)
watch(() => props.text, render)
</script>

<template>
  <component :is="props.tag || 'span'" ref="host"></component>
</template>
