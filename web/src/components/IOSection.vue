<script setup lang="ts">
// 输入/输出段（旧页 ioSection 的组件化）：io-label + 机器文本滚动区；
// 精确匹配的输出图片（ImageStrip + 归属小字）经默认插槽贴在正文下方。
import MachineScrollBox from './MachineScrollBox.vue'

withDefaults(defineProps<{
  label: string
  text: string
  error?: boolean
  out?: boolean
  memKey?: string
  tokens?: number
}>(), {})
</script>

<template>
  <div class="io-section" :class="{ 'out-section': out }">
    <div class="io-label">{{ label }}</div>
    <div v-if="text === undefined || text === null || text === ''" class="io-empty">（无内容）</div>
    <MachineScrollBox v-else :text="text" :mem-key="memKey || ''" :tokens="tokens" :error="error" />
    <slot />
  </div>
</template>
