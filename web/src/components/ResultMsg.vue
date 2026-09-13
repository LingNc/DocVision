<script setup lang="ts">
// 未配对的工具回执（找不到 tool_call_id 对应的调用）：一次调用一行的
// 例外，单独成行并注明配不上的原因。
import { computed, onMounted, ref } from 'vue'
import { state } from '../state'
import { countText, estOf, firstLine } from '../legacy/sidebar'
import { classifyResult } from '../legacy/timeline'
import type { StreamItem } from '../legacy/stream'
import Disclosure from './Disclosure.vue'
import IOSection from './IOSection.vue'
import CopyBtn from './CopyBtn.vue'

const props = defineProps<{ item: Extract<StreamItem, { type: 'result' }> }>()

const line = computed(() => props.item.line)
const text = computed(() => String(line.value.text || ''))
const status = computed(() => classifyResult(text.value))

const root = ref<HTMLElement | null>(null)
onMounted(() => {
  if (line.value.n !== undefined) state.anchors[line.value.n] = root.value as HTMLElement
})
</script>

<template>
  <section ref="root" class="msg msg-tool">
    <Disclosure
      :cls="'disclosure-result status-' + status"
      name="(未配对的工具回执)"
      :summary="firstLine(text)"
      :tail="countText(text.length, estOf(line).text) +
        (status === 'error' ? ' · error' : status === 'ok' ? ' · ok' : '')"
      :title="!item.paired ? (line.tool_call_id
        ? '未找到配对的工具调用：id ' + line.tool_call_id
        : '这条回执行没有 tool_call_id，无法与调用配对') : undefined"
    >
      <div class="io-card">
        <IOSection label="输出" :text="text" :error="status === 'error'"
          :mem-key="'result.' + line.n" :tokens="estOf(line).text" />
        <div class="io-actions"><CopyBtn :text="text" /></div>
      </div>
    </Disclosure>
  </section>
</template>
