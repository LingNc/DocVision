<script setup lang="ts">
import { computed } from 'vue'
import { state } from '../state'
import { sessionTip, sessionTitleOf, usageChipText } from '../legacy/sidebar'
import InlineMD from './InlineMD.vue'
import type { ChapterTreeNode } from '../legacy/sidebar'

defineOptions({ name: 'ChapterSessionTree' })
const props = defineProps<{ nodes: ChapterTreeNode[]; depth?: number }>()
const emit = defineEmits<{ select: [session: any]; info: [session: any] }>()

const currentId = computed(() => state.current && state.current.id)

function nodeContainsCurrent(node: ChapterTreeNode): boolean {
  if (node.convert?.id === currentId.value || node.checker?.id === currentId.value) return true
  return node.children.some(nodeContainsCurrent)
}

function activityText(session: any): string {
  const labels: Record<string, string> = {
    queued: '等待并发槽', running: '运行中', waiting_children: '等待子会话',
    checking_result: '核对完成待处理', pass: '已通过', issues: '发现问题', error: '会话错误',
  }
  return labels[session?.chapterActivity] || (session?.live ? '运行中' : '')
}

function primary(node: ChapterTreeNode): any {
  return node.convert || node.checker
}
</script>

<template>
  <div class="chapter-tree" :style="{ '--chapter-depth': depth || 0 }">
    <details v-for="node in nodes" :key="node.id" class="chapter-node" :open="nodeContainsCurrent(node)">
      <summary
        class="chapter-node-head session-row"
        :data-id="primary(node)?.id"
        :title="primary(node) ? sessionTip(primary(node)) : node.id"
        @click="primary(node) && emit('select', primary(node))"
      >
        <span class="chapter-caret" aria-hidden="true"></span>
        <InlineMD class="chapter-name row-title" tag="span" :text="sessionTitleOf(primary(node)) || node.id" />
        <span v-if="activityText(primary(node))" class="chapter-state">{{ activityText(primary(node)) }}</span>
        <span v-if="usageChipText(primary(node))" class="chapter-usage">{{ usageChipText(primary(node)) }}</span>
      </summary>
      <div class="chapter-node-body">
        <div
          v-if="node.checker"
          class="chapter-checker"
          :class="{ active: currentId === node.checker.id, live: node.checker.live }"
        >
          <button class="checker-main session-row" type="button" role="treeitem" :data-id="node.checker.id" :title="sessionTip(node.checker)" @click="emit('select', node.checker)">
            <span class="checker-marker">核对</span>
            <InlineMD class="checker-name row-title" tag="span" :text="sessionTitleOf(node.checker)" />
            <span v-if="activityText(node.checker)" class="chapter-state">{{ activityText(node.checker) }}</span>
            <span v-if="usageChipText(node.checker)" class="chapter-usage">{{ usageChipText(node.checker) }}</span>
          </button>
          <button class="chapter-info" type="button" title="打开核对会话详情" @click="emit('info', node.checker)">ⓘ</button>
        </div>
        <ChapterSessionTree
          v-if="node.children.length"
          :nodes="node.children" :depth="(depth || 0) + 1"
          @select="emit('select', $event)" @info="emit('info', $event)"
        />
      </div>
    </details>
  </div>
</template>

<style scoped>
.chapter-tree { border-left: 1px solid var(--line, #c9d1d9); margin-left: 13px; padding-left: 8px; }
.chapter-node { min-width: 0; }
.chapter-node-head { display: flex; align-items: center; gap: 6px; min-height: 29px; cursor: pointer; list-style: none; padding: 0 7px; color: var(--text, #24292f); }
.chapter-node-head::-webkit-details-marker { display: none; }
.chapter-caret { width: 12px; flex: 0 0 12px; color: var(--muted, #6e7781); }
.chapter-caret::before { content: '▸'; }
.chapter-node[open] > .chapter-node-head .chapter-caret::before { content: '▾'; }
.chapter-name, .checker-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }
.chapter-node-head:hover, .chapter-checker:hover { background: var(--hover, #f3f4f6); }
.chapter-node-body { padding-left: 12px; }
.chapter-checker { width: 100%; display: flex; align-items: center; gap: 6px; min-height: 28px; color: var(--text, #24292f); padding: 0 7px; }
.checker-main { min-width: 0; flex: 1; display: flex; align-items: center; gap: 6px; min-height: 28px; border: 0; background: transparent; color: inherit; text-align: left; padding: 0; cursor: pointer; }
.chapter-checker.active, .checker-main:hover { background: var(--active, #eaeef2); }
.checker-marker { flex: 0 0 auto; color: var(--muted, #6e7781); font-size: 11px; }
.chapter-state, .chapter-usage { flex: 0 0 auto; color: var(--muted, #6e7781); font-size: 11px; }
.chapter-checker.live .chapter-state { color: var(--green, #1a7f37); }
.chapter-info { flex: 0 0 22px; border: 0; background: transparent; color: var(--muted, #6e7781); cursor: pointer; }
</style>
