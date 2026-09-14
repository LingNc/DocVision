<script setup lang="ts">
// P7 流式尾：当前会话正在生成的那条消息的实时快照（<转录>.partial sidecar，
// 会话进程流式期间节流覆写、完整消息落盘即删）。纯呈现组件——phase 徽标 +
// 累积文本尾部 + 新鲜度；文本用与思考块一致的等宽安静样式，不用 Markdown
// 解析（半截 Markdown 渲染会闪）。
import { computed } from 'vue'
import type { PartialInfo } from '../legacy/types'

const props = defineProps<{ partial: PartialInfo }>()

const phaseLabel = computed(() => (props.partial.phase === 'reasoning' ? '思考中' : '输出中'))
const age = computed(() => {
  const sec = Math.max(0, Math.round((Date.now() - props.partial.ts) / 1000))
  return sec < 5 ? '刚刚' : sec + ' 秒前'
})
const text = computed(() => {
  const t = props.partial.text || ''
  return t.length > 4000 ? '…' + t.slice(-4000) : t
})
</script>

<template>
  <section class="msg partial-tail" :data-phase="partial.phase">
    <div class="pt-head">
      <span class="pt-spinner" aria-hidden="true"></span>
      <span class="pt-label">{{ phaseLabel }}</span>
      <span class="pt-age">{{ age }}</span>
    </div>
    <pre class="code pt-text">{{ text }}<span class="pt-caret" aria-hidden="true">▍</span></pre>
  </section>
</template>

<style scoped>
.partial-tail {
  border: .5px solid var(--border-strong);
  border-radius: var(--radius-sm);
  padding: 8px 10px;
  background: var(--hover);
}

.pt-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}

.pt-label {
  font-weight: 600;
  font-size: 12px;
  color: var(--ok);
}

.pt-age {
  font-size: 11px;
  color: var(--caption);
}

.pt-spinner {
  width: 10px;
  height: 10px;
  border: 1.5px solid var(--ok);
  border-top-color: transparent;
  border-radius: 50%;
  animation: pt-spin .8s linear infinite;
}

@keyframes pt-spin {
  to { transform: rotate(360deg); }
}

.pt-text {
  margin: 0;
  max-height: 220px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.pt-caret {
  color: var(--ok);
  animation: pt-blink 1s steps(2) infinite;
}

@keyframes pt-blink {
  50% { opacity: 0; }
}
</style>
