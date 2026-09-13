<script setup lang="ts">
// 折叠行骨架（思考 / 工具调用 / 图片轮 / 独立回执共用）：
// [16px 图标位（含折叠箭头）] 名称 13px ·(2px 圆点) 一行摘要 13px 省略号 [右侧尾巴]
// 旧页 disclosureLine 的组件化；展开体由默认插槽给出。
defineProps<{
  cls?: string
  name: string
  summary?: string
  tail?: string
  open?: boolean
}>()
</script>

<template>
  <details class="disclosure" :class="cls" :open="!!open">
    <summary>
      <span class="line-slot"><span class="line-caret" /></span>
      <span class="line-name">{{ name }}</span>
      <template v-if="summary">
        <span class="line-sep" />
        <span class="line-summary" :title="summary">{{ summary }}</span>
      </template>
      <span v-if="tail" class="line-tail">{{ tail }}</span>
    </summary>
    <slot />
  </details>
</template>
