# skills 线 issue 台账（独立编号 SK-I<N>）

> 与参考实现 `docs/issue.md` 的 T 编号完全隔离——本文件只记 skills 线自己的问题/需求。原参考实现 T328（skills 线开发）已迁移为 SK-I1。

**旗标语义**（同仓库规矩）：第一个 `[ ]`=Agent 完成态（`~` 在做 / `X` 完成），第二个 `[ ]`=开发者审核（`T` 通过 / `F` 打回）。

[X][ ] SK-I1. skills 线开发与判据迁移：LLM 会话外迁到宿主 agent，内核只做确定性执行+验收。
  根因：目标形态=用户只装 skill + MinerU，零 LLM API 配置，放下 PDF 跑到底/中断无损续/换 agent 可续。
  改：分支 `feat/skills`，新增 `skills/`（docs 设计文档+验收标准 A1-A10、kernel/ 十四包、agent/ 手册）；判据逐字移植参考实现（覆盖率 T168/dedupe T130-T151/compareFigure T307-T312/checker T222-T233/书签 T49/盘点 T33/input@path T324）；五类任务闭环+rework；详见本目录 plan.md SK-P1–SK-P26 与参考实现 docs/agent-batch-history.md 第三十七至第六十二批。
  验证：`go test ./internal/...` 12 包全绿（含 poppler/pdftotext 真集成冒烟、A1 全链演练 SK-P24）。

[X][ ] SK-I2. 验收门槛全部程序强制化（用户要求，2026-11-18）：矢量图重合度默认 17%、accept_as_is 必须交够次数才放行、**阈值只允许用户显式调整**（agent 无权自调，用户不提不改）；章节划分语义修正（初划独立单层阶段可有可无；递归细化在 convert 阶段内部，默认 3 层，无初划=4 层）；img2text 是独立任务类型（逐图小轮转会话：上下文上下检索/看图/提交描述，kernel 整理）；人类可读编号 imgxxx/vecxxx/pdfxxx + sha 映射表。
  根因：防模型偷懒与防阈值被 agent 自行放宽；img2text 与 latex 转换是两类任务不能混。
  改：skills/docs/plan.md SK-P27+（config 层）、SK-P28（编号映射）、SK-P29（img2text 类型）、SK-P30（看图台）——四项全部落地。
  验证：config 层单测 + e2e 复跑；2026-11-18 23:4x 复核实物：ids/img2text+mdimg/viewer/final 四包俱在且测试绿，SK-P28–P30 台账"待实现"字样已更正。
