<script setup lang="ts">
// 系统消息（不带图的 user 行 = harness 任务提示）：安静样式，长提示默认只
// 露 8 行可「展开全文」；会话开头投喂的原图作为附件段收在同一个块里。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { state, storeGet, storeSet, SYSTEM_PREVIEW_LINES, registerAnchor, unregisterAnchor } from '../state'
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
// 任务块是它行号与归并图片轮行号的锚点；卸载时注销，防串台。
onMounted(() => {
  if (line.value && root.value) registerAnchor(line.value.n, root.value)
})
onBeforeUnmount(() => {
  if (line.value && root.value) unregisterAnchor(line.value.n, root.value)
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

<style scoped>
/* 消息全部**左对齐**，没有右侧气泡：在这个工具里 role:"user" 行其实是 harness
   自己发的一轮（见 docs/commands.md 的"消息形态"），所以——
   · 不带图的 user 行 = 系统消息（安静样式 + 「系统 · user 轮」标签，默认只露
     前 8 行，可展开全文并内滚）；
   · 带图的 user 行 = 把图片投给模型的那一轮，作为对应工具的输入/附件收在
     同一次调用的块里（配不上才单独一行折叠行）。 */
.msg-system {
 align-items: stretch; 
}

.sys-line {
 display: flex; align-items: center; gap: 8px; min-width: 0; 
}

.sys-badge {
  flex: none;
  font-size: 11.5px;
  line-height: 18px;
  font-weight: 600;
  letter-spacing: .06em;
  color: var(--caption);
}

.sys-meta {
  flex: none;
  padding: 0 6px;
  border-radius: 4px;
  background: var(--chip);
  color: var(--chip-text);
  font-family: var(--mono);
  font-size: 10.5px;
  line-height: 16px;
}

.sys-line :deep(.line-summary) {
 min-width: 0; color: var(--dim); font-size: 12px; 
}

/* 系统消息正文：默认露前 8 行（渐隐），展开后限高内滚——与思考/工具同一套 */
.sys-scroll {
 overflow: auto; 
}

.sys-scroll.folded {
  overflow: hidden;
  mask-image: linear-gradient(180deg, #000 72%, transparent);
}

.msg-system .body-text {
 color: var(--muted); 
}

/* 原 160 号规则里的 msg-system 半边 */
.msg-system .body-text { color: var(--muted); font-size: 13px; }
</style>
