<script setup lang="ts">
// 等宽限高内滚（旧页 machineScroll 的落点）：JSON 四色 / 终端 / diff / 日志
// 高亮 + 超限「展开全文」，记忆键 text.<key>；data-error 标红错误输出。
// 内容经 machineScroll 用 DOM API 插入（词法扫描产 span，从不拼 HTML）。
import { onMounted, ref, watch } from 'vue'
import { machineScrollInto } from '../legacy/richtext'

const props = withDefaults(defineProps<{
  text: string
  memKey: string
  tokens?: number
  error?: boolean
}>(), {})

const host = ref<HTMLElement | null>(null)

function render() {
  const el = host.value
  if (!el) return
  el.textContent = ''
  machineScrollInto(el, props.text, props.memKey, undefined, props.tokens)
  if (props.error) {
    const pre = el.querySelector('.io-text')
    if (pre) pre.setAttribute('data-error', 'true')
  }
}

onMounted(render)
watch(() => [props.text, props.memKey, props.tokens, props.error], render)
</script>

<template>
  <div ref="host" class="text-wrap"></div>
</template>
