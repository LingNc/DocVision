<script setup lang="ts">
import { computed } from 'vue'
import { state } from '../state'
import { chapterActivityLabel, chapterNodeHasRows, chapterNodeKey, chapterWantOpen, fixRowLabel, fixRowTitle, fixTargetId, fixTranscriptTag, rescueRowLabel, rescueRowTitle, sessionTip, sessionTitleOf, setCollapsed, usageChipText } from '../legacy/sidebar'
import InlineMD from './InlineMD.vue'
import { chapterPendingRetry, refreshIndex, requestChapterRetry } from '../data'
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
  // P60：看的是修复会话时，它所在的那一支也要自动露出（修复行是子行）。
  if ((node.fixes || []).some((f: any) => f && f.id === currentId.value)) return true
  // P61：复盘行同理——点开一条复盘会话，它挂在的那一章要自己展开。
  if ((node.rescues || []).some((r: any) => r && r.id === currentId.value)) return true
  return node.children.some(nodeContainsCurrent)
}

/* 记忆键带 scope（项目 + 阶段），否则不同书里同名的 chapter_001 会共用状态。 */
function nodeKey(node: ChapterTreeNode): string {
  return chapterNodeKey(props.scope, node.id)
}

function isOpen(node: ChapterTreeNode): boolean {
  return chapterWantOpen(props.scope, node.id, nodeContainsCurrent(node))
}

/* 核对会话作为**子行**出现，前提是行头已经是转换会话；没有转换会话的节点
 * （只做过重新核对）行头本身就是那条核对会话，再在下面重复一遍就成了
 * 「核对 核对 · chapter_002」两行同一个会话。
 * P60 第五步：修复子行同理——没有修复的节点不能因为这条判据多出一个展开箭头；
 * 反过来，修复行也不能因为这一行没有核对（或核对不显示）就跟着消失，所以
 * 「有没有子行」的三种来源互不依赖（判据收在 sidebar.ts 的 chapterNodeHasRows）。 */
function hasBody(node: ChapterTreeNode): boolean {
  return chapterNodeHasRows(node)
}

/*
 * P60：修复行的状态只说两件事——活着 / 归档。不套 chapterActivityLabel：那是
 * 转换与核对共用的文案表（"转换完成待核对"/"核对完成待处理"），修复既不是转换
 * 也不在等核对，硬套会给出不相干的话。
 */
function fixStateText(session: any): string {
  return session && session.live ? '运行中' : ''
}

/* P61：复盘行只说"这条复盘会话还在写吗"。结论（retry/truncate/fail）不是会话
 * 状态而是**目标**会话的状态（已经由目标行的 chapter_activity 显示成 queued /
 * 复盘处理中 / 会话错误），在这里再抄一遍只会把两件事搅在一起。 */
function rescueStateText(session: any): string {
  return session && session.live ? '运行中' : ''
}

/* 转录名里的 `.tryN` 只有当它**不是**这一轮时才有信息量：轮次已经写在
 * 「修复 child_02 #1」里了，再挂一个 `.try1` 只是把同一个数字写两遍。 */
function fixTryTag(session: any, round: number): string {
  const tag = fixTranscriptTag(session)
  const m = /^\.try(\d+)$/.exec(tag)
  if (m && Number(m[1]) === (Number(round) || 1)) return ''
  return tag
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

/* 同一句状态在两种会话里意思不同（T178），文案的唯一来源是
 * chapterActivityLabel：转换跑完是「转换完成待核对」，核对跑完才是
 * 「核对完成待处理」；转换又在排队而这一章已有核对结论时，它等的是重转换
 * 的并发槽。 */
function activityText(session: any, role: 'convert' | 'checker', node?: ChapterTreeNode): string {
  // 转换行要参考同一行核对的结论（T181）：核对已经"发现问题"时，转换行不能再
  // 挂着"待核对"。
  const chk = node?.checker
  const peer = role === 'convert' && chk ? String(chk.chapterActivity || '') : ''
  return chapterActivityLabel(session?.chapterActivity, role, peer, !!session?.live)
}

function nodeActivity(node: ChapterTreeNode): string {
  return activityText(primary(node), node.convert ? 'convert' : 'checker', node)
}

/* P53：手动重试。页面只往项目的 work/retry/ 排一个请求文件，正在跑的
 * docvision latex 在转换阶段的安全点消费它；没在跑就留到下次启动。
 * 章节已经在队列里时按钮收起、换成「待重试」角标。 */
function nodeProject(node: ChapterTreeNode): string {
  const s = primary(node) || node.convert || node.checker
  return (s && s.project) || ''
}

/* 只有顶层章节能单独重试：子章节由父章节的细分树产出，重跑它得连同父节点
 * 的整合一起走（那是"重试父章节"），所以按钮不给子章节。 */
function canRetryNode(node: ChapterTreeNode): boolean {
  return !!nodeProject(node) && !state.staticMode && node.id.indexOf('_child_') < 0
}

function nodePendingRetry(node: ChapterTreeNode, role = 'convert'): boolean {
  const project = nodeProject(node)
  return !!project && chapterPendingRetry(project, node.id, role)
}

/* role: convert = 重转这一章（开新尝试），checker = 只重新核对已提交的产物。 */
function retryNode(node: ChapterTreeNode, role: string, ev: MouseEvent): void {
  ev.stopPropagation()
  const project = nodeProject(node)
  if (!project) return
  const what = role === 'checker' ? '重新核对' : '重试'
  state.retryNote = '正在提交 ' + node.id + ' 的' + what + '请求…'
  requestChapterRetry(project, node.id, role, '侧栏' + what)
    .then((res: any) => {
      state.retryNote = node.id + ' ' + what + '请求已排队（' + ((res && res.note) || '等待转换阶段处理') + '）'
      return refreshIndex()
    })
    .catch((err: any) => {
      state.retryNote = node.id + ' ' + what + '请求失败：' + ((err && err.message) || err)
    })
}

/* 只有"运行中"染色（T180）：原来只有核对行有 .live 规则，转换行跑着时那句
 * 「运行中」是灰的，看着像没在跑。按文案判而不是按 live 判——排队等槽的
 * 会话也是 live，它不该是绿的。 */
function stateTone(label: string): string {
  return label === '运行中' ? 'run' : ''
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
        <span v-if="nodeActivity(node)" class="chapter-state" :class="stateTone(nodeActivity(node))">{{ nodeActivity(node) }}</span>
        <span v-if="usageChipText(primary(node))" class="chapter-usage">{{ usageChipText(primary(node)) }}</span>
        <span v-if="primary(node)?.archived" class="chapter-arch" title="这份转录只在 work/attempts 归档目录里（作废尝试搬走时留下的副本），不是当前工作区">归档</span>
        <span v-if="nodePendingRetry(node)" class="chapter-retry-pending" title="已在重试队列里：转换阶段的下一个安全点会重跑这一章">待重试</span>
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
            <span v-if="activityText(node.checker, 'checker', node)" class="chapter-state" :class="stateTone(activityText(node.checker, 'checker', node))">{{ activityText(node.checker, 'checker', node) }}</span>
            <span v-if="usageChipText(node.checker)" class="chapter-usage">{{ usageChipText(node.checker) }}</span>
            <span v-if="node.checker.archived" class="chapter-arch" title="这份转录只在 work/attempts 归档目录里（作废尝试搬走时留下的副本），不是当前工作区">归档</span>
            <span v-if="nodePendingRetry(node, 'checker')" class="chapter-retry-pending" title="已在重新核对队列里：转换阶段的下一个安全点会重跑核对会话（产物不动）">待重试</span>
            <button
              v-else-if="canRetryNode(node)"
              class="chapter-retry icon-btn"
              type="button"
              :title="'重新核对这一章 ' + node.id + '：只重跑核对会话、产物不动，旧的核对转录留在历史里'"
              @click="retryNode(node, 'checker', $event)"
            >↻</button>
          </button>
          <button class="chapter-info" type="button" title="打开核对会话详情" @click="emit('info', node.checker)">ⓘ</button>
        </div>
        <!-- P60 第五步：修复会话子行是这一行选中那次转换下面的**独立子行**，
             不在核对块里——核对行在不在（还没核对、或核对尝试号比转换小而不
             显示）都不影响它。没有修复的节点一条也不渲染：不留占位、不撑高度。
             标签写清**修的是哪一章的第几轮**（`修复 child_02 #1`）——同一行下面
             可能挂着不同子章节的修复，只写「修复 N」看不出归属；结局（通过 /
             未通过 / 运行中）在行 title 里（fixRowTitle）。 -->
        <div
          v-if="(node.fixes || []).length"
          v-for="(fx, i) in node.fixes"
          :key="'fix:' + fx.id + ':' + i"
          class="chapter-fix"
          :class="{ active: currentId === fx.id }"
        >
          <button
            class="fix-main session-row"
            type="button"
            role="treeitem"
            :data-id="fx.id"
            :title="fixRowTitle(fx)"
            @click="emit('select', fx)"
          >
            <span class="fix-marker" :title="'修复会话针对的章节：' + (fixTargetId(fx, node.id) || '—')">{{ fixRowLabel(fx, node.id, i + 1) }}</span>
            <span v-if="fixTryTag(fx, i + 1)" class="fix-try">{{ fixTryTag(fx, i + 1) }}</span>
            <span v-if="fixStateText(fx)" class="chapter-state" :class="stateTone(fixStateText(fx))">{{ fixStateText(fx) }}</span>
            <span v-if="fx.archived" class="chapter-arch" title="这份转录只在 work/attempts 归档目录里（作废尝试搬走时留下的副本），不是当前工作区">归档</span>
          </button>
        </div>
        <!-- P61：复盘会话**与核对行同层**（和修复行一样是这一行的直接子行，
             左边距取同一个 26px）。标签是「复盘 #N」——N 是它复盘的那一条转换
             的尝试号（转录名里的 `.tryN`），不是行序号；结局与"复盘的是哪一条
             产物"在行 title 里。没有复盘的节点一条也不渲染，布局与加这条规则
             之前逐字节相同。 -->
        <div
          v-if="(node.rescues || []).length"
          v-for="(rs, i) in node.rescues"
          :key="'rescue:' + rs.id + ':' + i"
          class="chapter-rescue"
          :class="{ active: currentId === rs.id }"
        >
          <button
            class="rescue-main session-row"
            type="button"
            role="treeitem"
            :data-id="rs.id"
            :title="rescueRowTitle(rs)"
            @click="emit('select', rs)"
          >
            <span class="rescue-marker">{{ rescueRowLabel(rs) }}</span>
            <span v-if="rescueStateText(rs)" class="chapter-state" :class="stateTone(rescueStateText(rs))">{{ rescueStateText(rs) }}</span>
            <span v-if="rs.archived" class="chapter-arch" title="这份转录只在 work/attempts 归档目录里（作废尝试搬走时留下的副本），不是当前工作区">归档</span>
          </button>
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
/* P53：重试按钮与「待重试」角标。按钮平时半透明，行悬浮/选中时才明显——
 * 章节行本身就是"点一下选中"，按钮不能抢走整行的点击。 */
.chapter-retry { flex: 0 0 auto; opacity: 0; transition: opacity .12s; font-size: 12px; padding: 0 4px; }
.chapter-node-head:hover .chapter-retry, .chapter-node-head.active .chapter-retry { opacity: .75; }
.chapter-retry:hover { opacity: 1; }
.chapter-retry-pending { flex: 0 0 auto; font-size: 11px; color: var(--amber, #9a6700); }
.chapter-state.run { color: var(--green, #1a7f37); }
.chapter-arch { flex: 0 0 auto; font-size: 11px; color: var(--muted, #6e7781); border: 1px dashed var(--line, #c9d1d9); border-radius: 4px; padding: 0 3px; line-height: 15px; }
.chapter-info { flex: 0 0 22px; border: 0; background: transparent; color: var(--muted, #6e7781); cursor: pointer; }
/* P60 第六步：修复行与核对行**同层**，不再缩进一级。
 * 一次修复会话的复核结果写的是 `checker_<child>.tryN.jsonl`、并被并进本节点
 * 自己那条核对行（同一个锚点），所以并没有"修复的核对行"这一层；修复行与核对
 * 行都是转换行的直接子行，左边距取同一个值（26px，含 12px 的
 * `.chapter-node-body` 内边距在里面）。行序也是这一层：核对行在前，修复行按
 * 旧→新跟在后面（排序在 sidebar.ts 的 fixesForNode）。
 * 只动横向留白，行高仍是 `var(--row-h)`：没有修复的节点一条也不渲染，布局
 * 与加这条规则之前逐字节相同。 */
.chapter-fix { display: flex; align-items: center; padding: 0 7px 0 26px; }
.fix-main { min-width: 0; flex: 1; display: flex; align-items: center; gap: 6px; min-height: var(--row-h); border: 0; background: transparent; color: inherit; text-align: left; padding: 0; cursor: pointer; border-radius: 4px; }
.chapter-fix:hover { background: var(--hover, #f3f4f6); }
.chapter-fix.active { background: var(--active, #eaeef2); }
.fix-marker { flex: 0 0 auto; color: var(--muted, #6e7781); font-size: 11px; }
.fix-try { flex: 0 0 auto; color: var(--muted, #6e7781); font-size: 11px; }
/* P61：复盘行与核对/修复行**同层同高**（同一处 26px 左边距、同一个 --row-h），
 * 只把标记文案染成弱色，与"这只是一条附注"的语义一致。 */
.chapter-rescue { display: flex; align-items: center; padding: 0 7px 0 26px; }
.rescue-main { min-width: 0; flex: 1; display: flex; align-items: center; gap: 6px; min-height: var(--row-h); border: 0; background: transparent; color: inherit; text-align: left; padding: 0; cursor: pointer; border-radius: 4px; }
.chapter-rescue:hover { background: var(--hover, #f3f4f6); }
.chapter-rescue.active { background: var(--active, #eaeef2); }
.rescue-marker { flex: 0 0 auto; color: var(--muted, #6e7781); font-size: 11px; }
</style>
