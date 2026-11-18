# 04 · 早期架构资产清点（迁移来源台账）

迁移动作 = **判据入代码、知识入说明书、会话层退役**。逐条抽取、逐字校对，不凭记忆重写（同仓库提示词维护规矩）。本文件是迁移的 checklist，迁一项勾一项。

## 一、判据类 → kernel 验收 spec（第一优先级）

来源：`上游实现/latex`（提交门禁、覆盖率、dedupe 判据）、`上游实现/analyze`、`docs/agent-notes/latex-pipeline.md`（判据、阈值与事故证据）。

| 判据 | 旧位置线索 | 新归宿 |
| --- | --- | --- |
| 覆盖率 ≥85%（pdftotext + CJK 三字组，首打回再交放行记 coverage_note） | book.go 提交门禁 | T3 验收 spec + 覆盖率包 ✅ P83 |
| 提交门禁：编译摘要 + sha256 一致 + 看页计数 + 原书对照 | book.go .gate | T3 验收 spec ✅ P83（编译+sha256 快照） |
| 自包含：`\input`/`\includegraphics` 不指向临时路径 | book.go submit 检查 | T3 验收 spec ✅ P81 |
| dedupe：sha256 收缩、居中重合 + 对齐平移（0.10S）、组闸门 0.60、例外带入、图标自动合并 0.75、代表图选取与跟随/解除跟随 | dedupe-algorithm.md、book.go | kernel 本地算法包 + T2 前置 |
| 章节行数上限、拆分 2–16 段完整连续无重叠、最多三层 | book.go | 队列依赖边验证 ✅ P82 |
| classify 失败不降级 raster，记未分类+错误、断点续跑 | book.go | T1 验收 spec |
| 作图数值门槛：min_overlap / max_scale_dev、accept_as_is 先交够 N 次 | figure check 相关 | T2 验收 spec |
| 核对：问题必须带位置、no_verdict 重试计数、绝不因无结论重转整章 | book.go checker | T4 验收 spec |
| 风格修复循环：problems 共享存储、轮次上限、每轮步骤绑指纹跳过 | style_fix 相关 | T5 验收 spec |
| 资源盘点、书签注入、standalone 内联、终审指纹跳过 | assemble.go | ASSEMBLE/FINAL 阶段 |

## 二、知识/方法类 → 工作说明书（agent/ 目录，per 任务类型）

来源：`上游实现/prompts/templates/*.md`（各会话 system 模板）、`docs/latex.md`、`docs/agent-notes/`。

迁移时的抽取纪律：逐篇过旧模板，"必须/不得"句进验收 spec，"怎么/建议"句进说明书，人设/仪式句丢弃；搬迁逐字校对。**说明书只减不增**（少即是多），工具宽容度提高时不往说明书加解释。

| 说明书 | 素材来源 |
| --- | --- |
| T1 classify 说明书 | classify 会话模板 |
| T2 作图说明书 | 作图会话模板 + 样式手册规范 + P79/P80 绘图统一样式结论 |
| T3 转换说明书 | convert 会话模板 + doc_search/view 用法 + 成对图规则 |
| T4 核对说明书 | checker 会话模板（含"问题带位置/原书权威/复核撤回"） |
| T5 回炉/风格修复说明书 | stylefix 整合/评估/分块模板 + goal/plan 惯例 |
| 冷启动仪式说明 | 本目录 docs/01、02、03 |

## 三、会话层退役清单（只留调试价值）

| 机制 | 处置 |
| --- | --- |
| 上下文窗口管理、两级压缩、输出预算 | 退役（宿主平台自管） |
| JSONL 转录 + 回放恢复 | 退役为可选调试记录，不参与恢复 |
| 用量/费用审计（t=usage） | 退役（可选：kernel 记自己的工具链成本） |
| 虚拟工作区 VFS、挂载表、bubblewrap 沙箱 | 退役；裁剪视图职责 = 任务包白名单路径；验收口兜底 |
| 并发槽位池、父等子等待 | 退役；并发 = 队列租约 + 平台子智能体 |
| 重试分类、退避、余额错误分流 | 部分保留：LLM 侧退役；kernel 工具链侧（MinerU/编译超时）保留进程组硬杀与重试 |

## 四、目录与数据兼容

- 新线项目工作区布局沿用参考实现（`source/`、`progress_items/`、`work/`、`style/`、`chapters/`、`build/`、`out/`），旧文档与调试直觉平移。
- 参考实现 `docs/` 的判据文档（dedupe-algorithm.md、agent-notes/latex-pipeline.md）是本线开发的**规格来源**，迁完前不得删除。
