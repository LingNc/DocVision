<script setup lang="ts">
// 块级 Markdown 落点：renderMarkdown 的 DOM API 插入（块解析产元素，
// 从不拼 HTML；```json 代码块由 mdCodeBlock 顺带做 JSON 高亮）。
import { onMounted, ref, watch } from 'vue'
import { renderMarkdown } from '../legacy/richtext'

const props = defineProps<{ text: string }>()
const host = ref<HTMLElement | null>(null)

function render() {
  const el = host.value
  if (!el) return
  el.textContent = ''
  el.appendChild(renderMarkdown(props.text))
}

onMounted(render)
watch(() => props.text, render)
</script>

<template>
  <div ref="host"></div>
</template>
