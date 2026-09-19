/*
 * 转录行的 TS 类型（组件化质量批）：Go 侧 JSONL 逐行下发的 wire 结构。
 * 字段随行类型（msg/meta/usage）差异很大——**渲染会读的公共字段收紧成
 * 具名可选字段**，其余放开为 unknown 索引签名（meta 的 kind/model、
 * usage 的 stats 等读取得走显式收窄）。此前 stream/timeline/trajectory
 * 里 50 多处 `any` 大多因此而来。
 */

/** 一次工具调用（assistant 行的 tool_calls 条目）。 */
export interface ToolCall {
  id?: string
  type?: string
  function?: { name?: string; arguments?: string }
  [k: string]: unknown
}

/** Go 侧随行下发的本地估算（前端只取用、不自己算，两套公式必然漂移）。 */
export interface LineEst {
  text?: number
  reasoning?: number
  /** 每个工具调用的参数估算，与 tool_calls 一一对应。 */
  calls?: number[]
  images?: number
  imageCount?: number
  [k: string]: unknown
}

/** usage 行的 stats 块（厂商实测口径）。 */
export interface UsageStats {
  round?: number
  kind?: string
  promptTokens?: number
  cachedTokens?: number
  completionTokens?: number
  reasoningTokens?: number
  finish?: string
  durationMs?: number
  [k: string]: unknown
}

/*
 * 一条转录行。`n` 是转录行号（1 起、含非消息行），是跳转/锚点/归属的
 * 全局主键；`t` 区分 wire 类型（msg = 消息行，meta = 元信息——由详情栏
 * 渲染、绝不进消息序列，usage = 用量行）；`bad` = JSON 解析失败的坏行。
 */
export interface Line {
  n: number
  t?: string
  bad?: boolean
  /** msg 行的角色（meta/usage 行没有）。 */
  role?: 'user' | 'assistant' | 'tool' | 'system'
  text?: string
  /** 思考正文（wire 上是 reasoning_content，normalizeLine 归一成 reasoning）。 */
  reasoning?: string
  reasoning_content?: string
  tool_calls?: ToolCall[]
  /** tool 行回指它所属调用的 id。 */
  tool_call_id?: string
  /** 带图 user 轮的图片引用（file://media/<sha>.jpg 之类）。 */
  images?: string[]
  ts?: string
  /** P10：轮次内容哈希（会话写入端落盘的 h，12 hex），报障定位用。 */
  h?: string
  est?: LineEst
  /** meta 行：模型名 / 工具清单 / meta 种类。 */
  kind?: string
  model?: string
  tools?: string[]
  /** usage 行：实测用量块。 */
  stats?: UsageStats
  [k: string]: unknown
}

/*
 * 带图 user 轮的归属结果（timeline.ts 的 imageAttributions）：
 *   · kind 'call' —— 归某次调用，how 区分精确匹配(call-id)/工具名/顺序；
 *   · kind 'task' —— 本会话任务的原图投喂（taskLineN 指任务行）；
 *   · kind 'none' —— 未识别（页面标「归属：未识别」）。
 */
export interface ImageAttr {
  kind: 'call' | 'task' | 'none'
  how: 'call-id' | 'tool-name' | 'order' | 'task' | ''
  callId: string
  /** tool-name 匹配时从句柄里读出的工具名。 */
  name?: string
  lineN: number
  taskLineN: number
}

/** 归属算法内部的"产图调用"队列条目（qpos = 在队列里的位置，-1 = 不在）。 */
export interface ImageCallEntry {
  id: string
  name: string
  /** 调用所在助手行的行号。 */
  lineN: number
  /** 首条回执文本（null = 还没有回执/被压缩截断）。 */
  receipt: string | null
  claimed: boolean
  qpos: number
}

/** 侧栏/详情层的会话条目（列表接口下发；字段多、读取得散，先收公共面）。 */
/** P7：/api/session 的 partial 快照（<转录>.partial sidecar 的内容）。 */
export interface PartialInfo {
  phase: 'content' | 'reasoning'
  text: string
  ts: number
}

export interface Session {
  id: string
  /** P10：转录文件内容哈希（12 hex），详情栏「会话哈希」展示。 */
  sha?: string
  /** 相对路径第一段 = 项目名（projectOf 的结果，接口直接给）。 */
  project?: string
  stage?: string
  stageTitle?: string
  name?: string
  path?: string
  messages?: number
  size?: number | null
  mtime?: string | number
  /** 逐图会话：图片身份与展示字段。 */
  imageName?: string
  imageFile?: string
  imagePath?: string
  imageCaption?: string
  page?: number
  imageOrder?: number
  /** 章节族会话（转换/核对/样式修复）文件名里的章号（convert_chapter_003 → 3）。 */
  chapterOrder?: number
  /** 方块视图的完成信号（T22）：done=交过 / error=干过活没交 / ''=还没开工；运行中由 live 表达。 */
  endState?: 'done' | 'error' | ''
  imageType?: string
  /** P18：板块标记——img2text 来源的会话带 "img2text"，主根（latex）会话无此字段。 */
  board?: string
  [k: string]: unknown
}

/** P18：/api/img2text-progress 的一张图。 */
export interface Img2TextItem {
  name: string
  status: 'done' | 'fixed' | 'escalated' | 'pending' | string
}

/** P18：/api/img2text-progress 的一本书（进度概览区逐书渲染）。 */
export interface Img2TextBook {
  book: string
  total: number
  done: number
  /** fixed = 升级后修好的（有结果且留过修复工作区）；与 done 同属"已完成"。 */
  fixed: number
  escalated: number
  pending: number
  items?: Img2TextItem[]
}
