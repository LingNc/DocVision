<script setup lang="ts">
// 「复制」按钮：文案与回退逻辑与旧页 copyButton 一致（clipboard API 优先，
// 失败退 textarea+execCommand；成功置「已复制」1.2 秒后复原）。
import { ref } from 'vue'
import { fallbackCopy } from '../legacy/richtext'

const props = defineProps<{ text: string }>()
const label = ref('复制')
let timer = 0

function done() {
  label.value = '已复制'
  window.clearTimeout(timer)
  timer = window.setTimeout(() => { label.value = '复制' }, 1200)
}

function copy() {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(props.text).then(done, () => { if (fallbackCopy(props.text)) done() })
  } else if (fallbackCopy(props.text)) {
    done()
  }
}
</script>

<template>
  <button type="button" class="text-toggle" @click="copy">{{ label }}</button>
</template>
