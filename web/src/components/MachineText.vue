<script setup lang="ts">
// 等宽高亮文本落点（机器文本：JSON 四色 / 终端 / diff / LaTeX 日志）：
// 内容经 machineBlock 用 DOM API 插入（词法扫描产 span，从不拼 HTML）；
// 大内容退纯文本的 note 也由 machineBlock 一并产出。
// tag 用于让宿主元素名与旧页一致（div / span / details 内的落点）。
import { onMounted, ref, watch } from 'vue'
import { machineBlock } from '../legacy/richtext'

const props = withDefaults(defineProps<{ text: string; cls?: string; tag?: string }>(), { tag: 'div' })
const host = ref<HTMLElement | null>(null)

function render() {
  const el = host.value
  if (!el) return
  el.textContent = ''
  el.appendChild(machineBlock(props.text, props.cls))
}

onMounted(render)
watch(() => [props.text, props.cls], render)
</script>

<template>
  <component :is="tag" ref="host"></component>
</template>
