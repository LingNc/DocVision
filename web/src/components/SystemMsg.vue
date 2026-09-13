<script setup lang="ts">
// 系统消息（不带图的 user 行 = harness 任务提示）：安静样式，长提示默认只
// 露 8 行可「展开全文」；会话开头投喂的原图作为附件段收在同一个块里。
import { computed, onMounted, ref } from 'vue'
import { state, storeGet, storeSet, SYSTEM_PREVIEW_LINES } from '../state'
import { estOf, firstLine } from '../legacy/sidebar'
import { foldLabel } from '../legacy/richtext'
import { attachNote, type StreamItem } from '../legacy/stream'
import FoldText from './FoldText.vue'
import ImageStrip from './ImageStrip.vue'
import MdBody from './MdBody.vue'

const props = defineProps<{ item: Extract<StreamItem, { type: 'system' }> }>()

const line = computed(() => props.item.line)
const raw = computed(() => String(line.value.text || ''))
const expanded = ref(storeGet('text.sys.' + state.current.id + '.' + line.value.n) === '1')
const lines = computed(() => raw.value.split('\n'))
const long = computed(() => lines.value.length > SYSTEM_PREVIEW_LINES)
const label = computed(() => foldLabel(expanded.value, lines.value.length, raw.value.length, estOf(line.value).text))

function toggle() {
  expanded.value = !expanded.value
  storeSet('text.sys.' + state.current.id + '.' + line.value.n, expanded.value ? '1' : '0')
}

const root = ref<HTMLElement | null>(null)
onMounted(() => {
  // 任务块是它行号与归并图片轮行号的锚点。
  if (line.value.n !== undefined) state.anchors[line.value.n] = root.value as HTMLElement
})
</script>

<template>
  <section ref="root" class="msg msg-system" title="这一轮是 user 角色发出的任务提示（系统性质，不是人打的字）">
    <div class="sys-line">
      <span class="sys-badge">系统</span>
      <span class="sys-meta">user 轮</span>
      <span class="line-summary">{{ firstLine(line.text) }}</span>
    </div>
    <div v-if="!raw" class="note">（无正文）</div>
    <template v-else>
      <div
        class="sys-scroll"
        :class="{ folded: long && !expanded }"
        :style="{ maxHeight: (long && !expanded) ? (SYSTEM_PREVIEW_LINES * 24) + 'px' : 'var(--code-scroll-h)' }"
      >
        <!-- 正文按 Markdown 开关渲染（与消息正文同一个渲染器），折叠只由这一层负责 -->
        <MdBody v-if="state.markdown" :text="raw" class="md-body sys-md" />
        <pre v-else class="body-text sys-text">{{ raw }}</pre>
      </div>
      <button v-if="long" type="button" class="text-toggle" @click="toggle">{{ label }}</button>
    </template>

    <template v-for="a in item.attachments" :key="a.line.n">
      <div class="io-divider" />
      <div class="io-section attach-section">
        <div class="io-label">附件（user 轮）</div>
        <div class="attach-body">
          <!-- 任务行自己的图：正文已在任务块里，附件段只放图，不抄第二遍 -->
          <FoldText v-if="a.line.text && a.line.n !== a.attr.taskLineN" :text="String(a.line.text || '')"
            :mem-key="'imgtext.' + a.line.n" :tokens="estOf(a.line).text" />
          <ImageStrip :images="a.line.images" />
        </div>
        <div class="attach-note">{{ attachNote(a) }}</div>
      </div>
    </template>
  </section>
</template>
