<script setup lang="ts">
// 助手消息：思考块（可选）+ 正文（可选）+ 工具卡（每个 tool_call 一张）。
// 思考不是独立步骤：轨迹里"思考"行跳回的就是这条消息。
import { computed, ref, watch, onMounted } from 'vue'
import { state, storeGet, storeSet, LONG_TEXT_LINES } from '../state'
import { countText, estOf, firstLine } from '../legacy/sidebar'
import { type StreamItem } from '../legacy/stream'
import { openLightbox } from '../state'
import { mediaURL } from '../legacy/timeline'
import Disclosure from './Disclosure.vue'
import FoldText from './FoldText.vue'
import ToolCard from './ToolCard.vue'

/* 看图调用的缩略图预览行：折叠态就可见，是 details 的**兄弟节点**；
 * "一轮多 view"的缩略图按转录行号排序后全部并排挂在最后一个 view 行下。 */
function previewThumbs(call: { previews: any[] }): { ref: string; url: string }[] {
  const out: { ref: string; url: string }[] = []
  call.previews.slice().sort((a: any, b: any) => a.n - b.n).forEach((l: any) => {
    ;(l.images || []).forEach((ref: string) => { out.push({ ref, url: mediaURL(ref) }) })
  })
  return out
}

function show(ref: string, url: string) {
  openLightbox(url, ref)
}

const props = defineProps<{ item: Extract<StreamItem, { type: 'assistant' }> }>()

const line = computed(() => props.item.line)
/*
 * 展开状态记忆。旧页语义：整流重渲（会话/行数/Markdown/仅看工具/单位）时
 * 按记忆恢复；「折叠全部思考」开 = 命令式全部合上且不写记忆，关 = 什么都不
 * 做（等下次自然重渲才按记忆恢复）。
 */
const thinkOpen = ref(false)
const thinkMemKey = computed(() => 'thinking.' + state.current.id + '.' + line.value.n)
function syncThink() {
  thinkOpen.value = storeGet(thinkMemKey.value) === '1'
}
syncThink()
watch(
  [() => state.current && state.current.id, () => line.value && line.value.n,
   () => state.lines.length, () => state.markdown, () => state.onlyTools, () => state.unit],
  syncThink,
)
watch(() => state.forceCollapse, (on) => { if (on) thinkOpen.value = false })

function onThinkToggle() {
  if (state.forceCollapse) return
  const l = line.value
  const d = thinkRef.value && (thinkRef.value.$el as HTMLDetailsElement)
  storeSet('thinking.' + state.current.id + '.' + l.n, d && d.open ? '1' : '0')
}

const thinkRef = ref<any>(null)
const root = ref<HTMLElement | null>(null)
onMounted(() => {
  // 本条助手消息是它行号的锚点（轨迹跳回目标）。
  if (line.value && line.value.n !== undefined) state.anchors[line.value.n] = root.value as HTMLElement
})

const thinkTail = computed(() =>
  line.value && line.value.reasoning ? countText(line.value.reasoning.length, estOf(line.value).reasoning) : '')
</script>

<template>
  <section ref="root" class="msg msg-assistant">
    <Disclosure
      v-if="line.reasoning"
      ref="thinkRef"
      cls="disclosure-thinking"
      name="思考"
      :summary="firstLine(line.reasoning)"
      :tail="thinkTail"
      :open="thinkOpen"
      @toggle="onThinkToggle"
    >
      <div class="thinking-body">
        <div class="reasoning-scroll">
          <pre class="body-text reasoning-text">{{ line.reasoning }}</pre>
        </div>
      </div>
    </Disclosure>
    <FoldText v-if="line.text" :text="String(line.text)" :mem-key="'asst.' + line.n"
      :preview-lines="LONG_TEXT_LINES" :tokens="estOf(line).text" />
    <template v-for="c in item.calls" :key="c.key">
      <ToolCard :item="c" />
      <div v-if="previewThumbs(c).length" class="preview-strip">
        <img v-for="t in previewThumbs(c)" :key="t.ref" class="preview-thumb"
          :src="t.url" :alt="t.ref" loading="lazy" :title="t.ref" @click="show(t.ref, t.url)">
      </div>
    </template>
    <div v-if="item.empty" class="note">（空消息）</div>
  </section>
</template>
