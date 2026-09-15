<script setup lang="ts">
// 工具调用卡（旧页 toolDisclosure + attachResult/attachImages 的组件化）：
// 折叠一行（工具名按家族上色 + 摘要 + 输入→输出计数尾巴），展开是
// 「输入 / 输出」卡片；回执与归属图片记在 CallItem 上（见 legacy/stream.ts）。
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { state, storeGet, storeSet, registerAnchor, unregisterAnchor } from '../state'
import { countText, estOf } from '../legacy/sidebar'
import { prettyJSON } from '../legacy/richtext'
import { attachNote, callTail, outImageNote, type CallItem } from '../legacy/stream'
import Disclosure from './Disclosure.vue'
import IOSection from './IOSection.vue'
import FoldText from './FoldText.vue'
import ImageStrip from './ImageStrip.vue'
import CopyBtn from './CopyBtn.vue'

const props = defineProps<{ item: CallItem }>()

// Disclosure 是组件，ref 拿到实例；根 elements 是 <details>。
const details = ref<any>(null)
const open = ref(storeGet(props.item.memKey) === '1')

/* 会话切换时 memKey 变化 → 从记忆恢复展开状态。 */
watch(() => props.item.memKey, () => { open.value = storeGet(props.item.memKey) === '1' })

function onToggle() {
  if (!props.item.id) return
  const d = details.value && (details.value.$el as HTMLDetailsElement)
  storeSet(props.item.memKey, d && d.open ? '1' : '0')
}

/*
 * 归并进这张卡片的行（回执/图片轮）锚点 = 这个 details 元素（点轨迹行跳到
 * 这里）。模型每轮重算、anchorNs 可能增删——记一份已注册集合做差量，
 * 卸载时全部注销。
 */
const d = computed(() => (details.value ? (details.value.$el as HTMLDetailsElement) : null))
const registered = new Set<number>()
watch(() => props.item.anchorNs.join(','), () => {
  const el = d.value
  if (!el) return
  const want = new Set(props.item.anchorNs)
  registered.forEach((n) => { if (!want.has(n)) { unregisterAnchor(n, el); registered.delete(n) } })
  props.item.anchorNs.forEach((n) => {
    if (!registered.has(n)) { registerAnchor(n, el); registered.add(n) }
  })
})
onBeforeUnmount(() => {
  const el = d.value
  if (!el) return
  registered.forEach((n) => unregisterAnchor(n, el))
  registered.clear()
})

const tail = computed(() => callTail(props.item))
/* P7：没有回执且会话 live 才算「运行中」；会话死了的悬空调用不转圈。 */
const live = computed(() => !!state.current?.live)
const prettyArgs = computed(() => prettyJSON(props.item.argsText) || '(无参数)')
</script>

<template>
  <Disclosure
    ref="details"
    :cls="'disclosure-tool fam-' + item.fam + (item.lastStatus ? ' status-' + item.lastStatus : '')"
    :running="!item.lastStatus && live"
    :name="item.name"
    :summary="item.brief"
    :tail="tail"
    :open="open"
    :data-call-id="item.id || ''"
    :data-lines="item.anchorNs.join(',') || undefined"
    @toggle="onToggle"
  >
    <div class="io-card">
      <div v-if="item.cmdLine" class="io-cmd">
        <span class="cmd-prompt">$ </span><span class="cmd-line">{{ item.cmdLine }}</span>
      </div>
      <IOSection label="输入" :text="prettyArgs" :mem-key="'in.' + (item.id || '')" :tokens="item.argsTokens" />
      <div class="io-actions"><CopyBtn :text="item.argsText" /></div>

      <template v-for="(r, ri) in item.results" :key="r.line.n">
        <div class="io-divider" />
        <IOSection label="输出" :text="String(r.line.text || '')"
          :error="r.status === 'error'" :mem-key="'result.' + r.line.n" :tokens="estOf(r.line).text" out>
          <template v-if="ri === 0">
            <template v-for="a in item.outImages" :key="a.line.n">
              <ImageStrip :images="a.line.images" />
              <div class="attach-note">{{ outImageNote(a) }}</div>
            </template>
          </template>
        </IOSection>
        <div class="io-actions"><CopyBtn :text="String(r.line.text || '')" /></div>
      </template>

      <template v-for="a in item.attachments" :key="a.line.n">
        <div class="io-divider" />
        <div class="io-section attach-section">
          <div class="io-label">附件（user 轮）</div>
          <div class="attach-body">
            <FoldText v-if="a.line.text && a.line.n !== a.attr.taskLineN" :text="String(a.line.text || '')"
              :mem-key="'imgtext.' + a.line.n" :preview-lines="FOLD_PREVIEW_LINES" :tokens="estOf(a.line).text" />
            <ImageStrip :images="a.line.images" />
          </div>
          <div class="attach-note">{{ attachNote(a) }}</div>
        </div>
      </template>
    </div>
  </Disclosure>
</template>
