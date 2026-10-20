<script setup lang="ts">
import { computed } from 'vue'
import { state } from '../state'
import { chapterNodeKey, chapterWantOpen, sessionTip, sessionTitleOf, setCollapsed, usageChipText } from '../legacy/sidebar'
import InlineMD from './InlineMD.vue'
import type { ChapterTreeNode } from '../legacy/sidebar'

/*
 * 递归章节树（P34）。
 *
 * 展开状态是**每个节点自己**的记忆，与"正在看哪个会话"解耦：
 *  - 见过用户点击的节点按记忆（state.collapsed，落盘 localStorage）；
 *  - 没记忆的节点默认只在"装着当前会话"时展开（自动露出你在看的那一章）；
 *  - 选择会话不会改动任何其它节点的展开状态。
 *
 * 旧实现用 <details :open="nodeContainsCurrent(node)">：`:open` 是响应式
 * binding，每次重渲染（含每秒轮询）都会把"不含当前会话"的节点重新赋成
 * closed，于是点一下别处、甚至在别的会话里等一轮轮询，刚拉开的树就自己收
 * 回去了；而且 <summary> 的点击同时触发"选中会话"和"展开"，一次点击干两件
 * 事，展开还得靠那个 12px 的小三角。
 */
defineOptions({ name: 'ChapterSessionTree' })
const props = defineProps<{ nodes: ChapterTreeNode[]; depth?: number; scope: string }>()
const emit = defineEmits<{ select: [session: any]; info: [session: any] }>()

const currentId = computed(() => state.current && state.current.id)

function nodeContainsCurrent(node: ChapterTreeNode): boolean {
  if (node.convert?.id === currentId.value || node.checker?.id === currentId.value) return true
  return node.children.some(nodeContainsCurrent)
}

/* 记忆键带 scope（项目 + 阶段），否则不同书里同名的 chapter_001 会共用状态。 */
function nodeKey(node: ChapterTreeNode): string {
  return chapterNodeKey(props.scope, node.id)
}

function isOpen(node: ChapterTreeNode): boolean {
  return chapterWantOpen(props.scope, node.id, nodeContainsCurrent(node))
}

function hasBody(node: ChapterTreeNode): boolean {
  return !!node.checker || node.children.length > 0
}

/* 展开/收起只切自己这一棵，不选中、不影响任何其它节点。 */
function toggle(node: ChapterTreeNode, ev: MouseEvent): void {
  ev.stopPropagation()
  setCollapsed(nodeKey(node), isOpen(node))
}

function selectNode(node: ChapterTreeNode): void {
  const s = primary(node)
  if (s) emit('select', s)
}

function activityText(session: any): string {
  const labels: Record<string, string> = {
    queued: '等待并发槽', running: '运行中', waiting_children: '等待子会话',
    checking_result: '核对完成待处理', pass: '已通过', issues: '发现问题', error: '会话错误',
    // T171：转录已写 run_end 却停在 running 的尝试被放弃了（换会话重来或中断），
    // 不能再显示"运行中"，否则同一章会出现两条"运行中"。
    interrupted: '已中断',
  }
  return labels[session?.chapterActivity] || (session?.live ? '运行中' : '')
}

function primary(node: ChapterTreeNode): any {
  return node.convert || node.checker
}

/* 只有子节点、既没有 convert 也没有 checker 的节点也要有名字：取自己的 id。
 * sessionTitleOf 不认 undefined，别把空值喂进去。 */
function nodeTitle(node: ChapterTreeNode): string {
  const s = primary(node)
  return (s && sessionTitleOf(s)) || node.id
}
</script>

<template>
  <div class="chapter-tree" :style="{ '--chapter-depth': depth || 0 }" role="tree">
    <div
      v-for="node in nodes"
      :key="node.id"
      class="chapter-node"
      :class="{ open: isOpen(node) }"
      role="treeitem"
      :aria-expanded="isOpen(node)"
    >
      <div
        class="chapter-node-head session-row"
        :class="{ active: !!primary(node) && primary(node).id === currentId }"
        :data-id="primary(node)?.id"
        :title="primary(node) ? sessionTip(primary(node)) : node.id"
        @click="selectNode(node)"
      >
        <button
          v-if="hasBody(node)"
          class="chapter-toggle"
          type="button"
          :aria-expanded="isOpen(node)"
          :aria-label="(isOpen(node) ? '收起 ' : '展开 ') + nodeTitle(node) + ' 的子会话'"
          :title="isOpen(node) ? '收起子会话' : '展开子会话'"
          @click="toggle(node, $event)"
        ><span class="caret" aria-hidden="true"></span></button>
        <span v-else class="chapter-toggle-spacer" aria-hidden="true"></span>
        <InlineMD class="chapter-name row-title" tag="span" :text="nodeTitle(node)" plain />
        <span v-if="activityText(primary(node))" class="chapter-state">{{ activityText(primary(node)) }}</span>
        <span v-if="usageChipText(primary(node))" class="chapter-usage">{{ usageChipText(primary(node)) }}</span>
      </div>
      <div v-if="hasBody(node) && isOpen(node)" class="chapter-node-body">
        <div
          v-if="node.checker"
          class="chapter-checker"
          :class="{ active: currentId === node.checker.id, live: node.checker.live }"
        >
          <button class="checker-main session-row" type="button" role="treeitem" :data-id="node.checker.id" :title="sessionTip(node.checker)" @click="emit('select', node.checker)">
            <span class="checker-marker">核对</span>
            <InlineMD class="checker-name row-title" tag="span" :text="sessionTitleOf(node.checker)" plain />
            <span v-if="activityText(node.checker)" class="chapter-state">{{ activityText(node.checker) }}</span>
            <span v-if="usageChipText(node.checker)" class="chapter-usage">{{ usageChipText(node.checker) }}</span>
          </button>
          <button class="chapter-info" type="button" title="打开核对会话详情" @click="emit('info', node.checker)">ⓘ</button>
        </div>
        <ChapterSessionTree
          v-if="node.children.length"
          :nodes="node.children" :depth="(depth || 0) + 1" :scope="scope"
          @select="emit('select', $event)" @info="emit('info', $event)"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.chapter-tree { border-left: 1px solid var(--line, #c9d1d9); margin-left: 13px; padding-left: 8px; }
.chapter-node { min-width: 0; }
.chapter-node-head { display: flex; align-items: center; gap: 4px; min-height: 29px; cursor: pointer; padding: 0 7px 0 0; color: var(--text, #24292f); }
/* 展开按钮：整行高的 26px 命中区（原先只有 12px 的小三角），点它只切展开。 */
.chapter-toggle {
  flex: 0 0 26px; align-self: stretch; min-height: 26px;
  display: inline-flex; align-items: center; justify-content: center;
  border: 0; background: transparent; color: var(--muted, #6e7781);
  padding: 0; border-radius: 5px; cursor: pointer;
}
.chapter-toggle:hover { background: var(--hover, #f3f4f6); color: var(--text, #24292f); }
.chapter-toggle:focus-visible { outline: 2px solid var(--accent, #0969da); outline-offset: -1px; }
.chapter-toggle-spacer { flex: 0 0 26px; }
.caret {
  width: 0; height: 0;
  border-left: 5px solid currentColor;
  border-top: 4px solid transparent;
  border-bottom: 4px solid transparent;
  transition: transform .15s ease;
}
.chapter-node.open > .chapter-node-head .caret { transform: rotate(90deg); }
.chapter-name, .checker-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }
.chapter-node-head:hover, .chapter-checker:hover { background: var(--hover, #f3f4f6); }
.chapter-node-head.active, .chapter-checker.active { background: var(--active, #eaeef2); }
.chapter-node-body { padding-left: 12px; }
.chapter-checker { width: 100%; display: flex; align-items: center; gap: 6px; min-height: 28px; color: var(--text, #24292f); padding: 0 7px 0 26px; }
.checker-main { min-width: 0; flex: 1; display: flex; align-items: center; gap: 6px; min-height: 28px; border: 0; background: transparent; color: inherit; text-align: left; padding: 0; cursor: pointer; }
.checker-marker { flex: 0 0 auto; color: var(--muted, #6e7781); font-size: 11px; }
.chapter-state, .chapter-usage { flex: 0 0 auto; color: var(--muted, #6e7781); font-size: 11px; }
.chapter-checker.live .chapter-state { color: var(--green, #1a7f37); }
.chapter-info { flex: 0 0 22px; border: 0; background: transparent; color: var(--muted, #6e7781); cursor: pointer; }
</style>
