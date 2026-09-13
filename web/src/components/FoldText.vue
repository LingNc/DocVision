<script setup lang="ts">
// 折叠正文（旧页 collapsibleText + markdownText = bodyBlock 的组件化）：
// Markdown 开关决定走 md-body 还是纯文本 pre；行数超过 previewLines 先夹住
// （max-height + 渐隐遮罩），展开/收起记忆在 text.<key>，按钮文案唯一来源
// 是 foldLabel。
import { computed, ref } from 'vue'
import { state, storeGet, storeSet, LONG_TEXT_LINES } from '../state'
import { foldLabel } from '../legacy/richtext'
import MdBody from './MdBody.vue'

const props = withDefaults(defineProps<{
  text: string
  /** 记忆键后缀（旧页 key 原样）：'asst.<n>' / 'imgtext.<n>' / … */
  memKey: string
  previewLines?: number
  extraClass?: string
  tokens?: number
}>(), { previewLines: LONG_TEXT_LINES })

const raw = computed(() => String(props.text ?? ''))
const allLines = computed(() => raw.value.split('\n'))
const long = computed(() => allLines.value.length > props.previewLines)
const expanded = ref(storeGet('text.' + props.memKey) === '1')

const label = computed(() => foldLabel(expanded.value, allLines.value.length, raw.value.length, props.tokens))
const shown = computed(() =>
  (expanded.value || !long.value) ? allLines.value.join('\n') : allLines.value.slice(0, props.previewLines).join('\n'))

function toggle() {
  expanded.value = !expanded.value
  storeSet('text.' + props.memKey, expanded.value ? '1' : '0')
}
</script>

<template>
  <div class="text-wrap">
    <MdBody
      v-if="state.markdown"
      :text="raw"
      :class="['md-body', extraClass, { clamped: long && !expanded }]"
      :style="long && !expanded ? { maxHeight: (previewLines * 24) + 'px' } : undefined"
    />
    <pre v-else class="body-text" :class="[extraClass, { clamped: long && !expanded }]">{{ shown }}</pre>
    <button v-if="long" type="button" class="text-toggle" @click="toggle">{{ label }}</button>
  </div>
</template>
