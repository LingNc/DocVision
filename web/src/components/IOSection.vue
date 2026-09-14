<script setup lang="ts">
// 输入/输出段（旧页 ioSection 的组件化）：io-label + 机器文本滚动区；
// 精确匹配的输出图片（ImageStrip + 归属小字）经默认插槽贴在正文下方——
// 插槽内容包进 .io-extra 并钉在网格第二列（内容列）：曾直接当网格子元素
// 自动流到第二行第一列（标签列），图片贴最左、把 max-content 的标签列撑到
// 图片宽度、「输出」文字被挤进右侧窄条（T18）。
import { useSlots } from 'vue'
import MachineScrollBox from './MachineScrollBox.vue'

const slots = useSlots()

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
    <div v-if="slots.default" class="io-extra"><slot /></div>
  </div>
</template>
