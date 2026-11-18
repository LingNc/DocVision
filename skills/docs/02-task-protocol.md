# 02 · 任务协议总表

LLM 任务 = 任务包（kernel 下发）+ 工作说明书（agent 读本）+ 验收 spec（kernel 机械检查）。

## 章节划分语义（2026-11-18 用户修正）

- **初次章节划分是独立单层阶段**（可选）：一次性把全书 md 切成章，**不分层**；跳过它则第一个 convert 会话直接拿全书 md。
- **分层细化发生在 convert 阶段内部**：会话觉得上下文处理不了 → 提交拆分提案（行号区间）→ kernel 验证扇出子任务 → **父任务 blocked 等子任务，子任务产物合并回父** → 子会话可以继续向下拆。**递归 3 层**（有初划时 convert 递归 3 层；无初划=多一层，即 4 层）。实现=队列父子指针（`split` 命令+`Unblock`），层数深度由 kernel 按 depth 字段控制（待接）。
- img2text、analyze 属 PREP 侧（见 04 台账），其中 **img2text 是独立任务类型**（见下）。

## T6 img2text（逐图文字化小轮转会话——待实现 SK-P29）

独立于 latex 转换的任务类型：把每张图"文字化"（生成描述/提取内容），为全书检索与 style 素材供料。

- **任务粒度**：一张图一个任务；kernel 维持小轮转（claim/submit 循环）。
- **会话工具三件套**（kernel 提供）：检索上下文（up/down——该图前后段落/doc_index 邻块）、看图（page-render）、提交描述（write verdict）。
- **产物**：`out/description.md`（图的文字化内容）+ kernel 汇总进 doc_index 的 content 字段与 style 素材库。
- **页码/行定位**：md 合并时（Organize）从 docjson 精确定位页码打上标识——每段带页码范围，前文目录给页码定位+行定位。
- 与 draw/classify 的关系：classify 判三态（结构化 verdict），img2text 产自然语言描述——**两任务类型不混**。

## 人类可读编号体系（待实现 SK-P28）

- 给人类的固定 ID：**imgxxx**=原图、**vecxxx**=矢量成品、**pdfpxx**=页码——agent 内部仍用 sha/路径（它对 sha 更熟）。
- 映射表落盘 `skills_ids.json`（id ↔ sha/路径/global 页/bbox）；用户与 agent 讨论时互指 img012 即知是哪张；看图台与手册按此编号展示。

通用任务包结构（claim 返回）：

```json
{
  "task_id": "conv_chapter_003_t2",
  "type": "convert",
  "project": "<项目名>",
  "sandbox": "tasks/conv_chapter_003_t2/",
  "inputs": ["inputs/chapter.md", "inputs/style/manual.md", ...],
  "whitelist_read": ["style/", "converted/", "reports/", "doc_index/"],
  "acceptance": { "...见各类 spec..." },
  "book_profile": "projects/<项目名>/profile.md",
  "lease_seconds": 1800
}
```

通用 submit 语义：kernel 逐条跑验收 spec，全过记 `done`，任一不过返回机器可读错误清单、状态不变。验收只看磁盘事实，不看 agent 自述。

## T1 classify（图片三态分类）

- **任务粒度**：一张图一个任务；kernel 本地预筛（sha256 收缩、图标自动合并区间）后入队。
- **说明书要点**：三态定义（styled-text / vector / raster）与判断证据要求；样式化文本的提取规范。
- **验收 spec**：`verdict.json` 含 `class ∈ {styled_text, vector, raster}`；styled_text 必须附提取内容；vector 必须附"为什么可矢量重绘"一句证据；分类不合法时不记 done（沿用参考实现"失败不降级成 raster"判据）。

## T2 作图（矢量图重绘）

- **任务粒度**：一张 key 图一个任务；dedupe 跟随图不成任务（画 key、跟随成品）。
- **说明书要点**：TikZ/pgfplots 作画规范、编译-自看-迭代循环（用 agent 自己的看图能力看自己编译的 PDF）、样式手册 `## Vector figure style` 的引用方式。
- **验收 spec**：`out/figure.tex` 编译通过；kernel 跑软墨迹对齐（沿用参考实现 compareFigure 算法）给 overlap/scale_dev 数值，低于按图阈值打回；`accept_as_is` 出口要求先交够 N 次（沿用 `accept_min_submits`）。
- **kernel 工具**：渲染 composite、编译、数值比对全在 kernel。

## T3 convert（章节转换）

- **任务粒度**：一章一个任务；过大章节先走拆分（见编排节）。
- **说明书要点**：从章节 md 转换、以原书为准核对 OCR 疑点的 doc_search/view 用法、成对图统一、报告写法。
- **验收 spec**（沿用参考实现提交硬门禁，全部代码执行）：
  1. `out/<章>.tex` + 分片目录就位，`\input`/`\includegraphics` 不指向临时路径（自包含）；
  2. 编译通过且产物 sha256 与提交时一致；
  3. 内容覆盖率 ≥85%（pdftotext + CJK 三字组；第一次打回列缺口，再交仍不达标放行并记 coverage_note）；
  4. 章节行数 ≤上限（未拆分的节点超限直接打回，要求拆分）。

## T4 check（核对，只读）

- **任务粒度**：一章（或其一次尝试）一个核对任务。
- **上下文规则**：核对任务包**不含**转换过程笔记（防"我是这么画的"记忆污染）；无子智能体平台走延迟批量（转换攒批后再派）。
- **说明书要点**：每条问题必须指明位置（md 段落或原书页 ↔ .tex 何处）；看不见的差异不报；md 是 OCR 参考、原书页是权威；重复上一轮问题前须回原页复核，看错要撤回。
- **验收 spec**：`verdict.json` 含 `pass | issues[] | no_verdict`；issues 必须带位置指针；no_verdict 由 kernel 计重试次数（沿用参考实现"绝不因无结论重转整章"判据）。

## T5 复盘回炉（rework / rescue）

- **触发**：核对 issues、冒烟不过、验收连续打回。kernel 从 issues 存储生成 rework 任务，原任务 blocked。
- **粒度**：默认顶层章节（子章节归父）；风格类系统性问题进风格修复循环（整合→修样式→影响评估→分块修复），每轮结论落盘绑指纹，交过的步骤直接跳过（沿用参考实现 T285 机制）。
- **验收 spec**：rework 任务跑所属类的全部 spec + 本次回炉问题逐条复核。

## 延迟核对与回炉队列（无子智能体平台）

转换跑完一批后统一派核对批次（隔批异上下文 = 廉价异上下文）。硬要求：核对发现的问题**不允许靠 agent 记住**，一律 `issue_add` 落盘 → kernel 生成 rework 任务。代价是错误暴露滞后，缓解 = 每阶段首批 3–5 个冒烟。

## 冒烟与放量

每阶段第一批 3–5 个任务试跑 + 验收 + （有核对时）试核，全过才放量。这是"跑 20 小时发现系统性跑偏"的最便宜保险。

## 交接语仪式（每类任务通用）

交还任务前在 notes.md 追加两行：`停在 X` / `下一步 Y`。重入先读这两行。全局状态永不丢，局部状态按需加载。
