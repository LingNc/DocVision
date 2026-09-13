<script setup lang="ts">
// 消息流开头的一行极简摘要：条数 / 输入输出 / 缓存 / 首字 / 费用 / 提示词
// 快照 + 「详情 ›」开关。指标与元信息都在右侧详情栏，这里只点到为止。
import { computed } from 'vue'
import { state, layout, toggleDetails } from '../state'
import { aggregate, countText, fmtCost, fmtDur, fmtTokens, usageLines } from '../legacy/sidebar'
import { metaState } from '../legacy/timeline'

const bits = computed<string[]>(() => {
  if (!state.current) return []
  const out: string[] = [state.current.messages + ' 条消息']
  const st = aggregate(usageLines(state.lines))
  if (st) {
    out.push('输入 ' + fmtTokens(st.promptTokens) + ' / 输出 ' + fmtTokens(st.completionTokens))
    if (st.promptTokens) out.push('缓存 ' + st.cacheHitPct.toFixed(0) + '%')
    if (st.avgTtftMs) out.push('首字 ' + fmtDur(st.avgTtftMs))
    const money = fmtCost(state.current.cost, false)
    if (money) out.push(money)
  }
  const m = metaState()
  if (m) out.push('提示词快照 ' + countText(m.promptChars, m.promptTokenEst))
  return out
})

const detailsBtn = computed(() => (layout.cols.details > 0 ? '收起详情' : '详情 ›'))
</script>

<template>
  <div class="stream-summary">
    <template v-for="(b, i) in bits" :key="i">
      <span v-if="i" class="dot-sep" />
      <span>{{ b }}</span>
    </template>
    <button type="button" @click="toggleDetails">{{ detailsBtn }}</button>
  </div>
</template>
