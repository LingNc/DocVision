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
  /** P7：调用还没回执且会话 live——摘要行显示「运行中」spinner。 */
  running?: boolean
  /** 名称右侧的小徽标（如正文残片块的来源标注）。 */
  badge?: string
}>()
</script>

<template>
  <details class="disclosure" :class="cls" :open="!!open">
    <summary>
      <span class="line-slot"><span class="line-caret" /></span>
      <span class="line-name">{{ name }}</span>
      <span v-if="badge" class="line-badge">{{ badge }}</span>
      <template v-if="summary">
        <span class="line-sep" />
        <span class="line-summary" :title="summary">{{ summary }}</span>
      </template>
      <span v-if="running" class="line-running" title="这个调用还没有收到回执，正在执行">
        <span class="running-dot" />运行中
      </span>
      <span v-if="tail" class="line-tail">{{ tail }}</span>
    </summary>
    <slot />
  </details>
</template>

<style scoped>
/* 残片徽标：琥珀小胶囊，把「原始思考 / XML 协议残片」和正常思考块一眼区分开。 */
.line-badge {
  flex: none;
  margin-left: 6px;
  padding: 0 6px;
  border-radius: 999px;
  border: .5px solid var(--border);
  background: color-mix(in srgb, var(--warn, #d97706) 12%, transparent);
  color: var(--warn, #b45309);
  font-size: 10.5px;
  line-height: 16px;
  white-space: nowrap;
}
</style>
