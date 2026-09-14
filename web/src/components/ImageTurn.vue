<script setup lang="ts">
// 图片轮单独成行（配不上调用/任务时的归属未识别图片轮，或没有任务可归）：
// 与工具行同一套折叠词汇，默认折叠，点开看图。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { state, storeGet, storeSet, registerAnchor, unregisterAnchor } from '../state'
import { estOf } from '../legacy/sidebar'
import { attributionText } from '../legacy/timeline'
import { imageTurnTitle, type StreamItem } from '../legacy/stream'
import Disclosure from './Disclosure.vue'
import FoldText from './FoldText.vue'
import ImageStrip from './ImageStrip.vue'

const props = defineProps<{ item: Extract<StreamItem, { type: 'imageTurn' }> }>()

const line = computed(() => props.item.line)
const memKey = computed(() => 'image.' + state.current.id + '.' + line.value.n)
const open = ref(storeGet(memKey.value) === '1')
watch(memKey, () => { open.value = storeGet(memKey.value) === '1' })

function onToggle() {
  const d = root.value && (root.value.$el as HTMLDetailsElement)
  storeSet(memKey.value, d && d.open ? '1' : '0')
}

const root = ref<any>(null)
// 行号锚点（轨迹跳回目标）；卸载时注销，防串台。
onMounted(() => {
  if (line.value && root.value?.$el) registerAnchor(line.value.n, root.value.$el)
})
onBeforeUnmount(() => {
  if (line.value && root.value?.$el) unregisterAnchor(line.value.n, root.value.$el)
})
</script>

<template>
  <section class="msg msg-image">
    <Disclosure
      ref="root"
      cls="disclosure-image"
      name="图片（user 轮）"
      :summary="imageTurnSummary(line)"
      :tail="(line.images || []).length + ' 张'"
      :open="open"
      :title="imageTurnTitle(line, item.attr)"
      @toggle="onToggle"
    >
      <div class="image-body">
        <FoldText v-if="line.text" :text="String(line.text)" :mem-key="'imgtext.' + line.n"
          :tokens="estOf(line).text" />
        <ImageStrip :images="line.images" />
        <div class="attach-note">{{ attributionText(item.attr) }}</div>
      </div>
    </Disclosure>
  </section>
</template>

<style scoped>
.image-body {
 padding: 4px 0 6px 22px; display: flex; flex-direction: column; gap: 6px; align-items: flex-start; 
}
</style>
