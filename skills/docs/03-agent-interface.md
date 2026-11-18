# 03 · agent ↔ kernel 接口

## 总原则

- **控制面走工具调用**（粗粒度、批量、JSON），**数据面走文件路径**（工具不搬运内容，只搬运指针）。
- 大图/长文本永远不进工具参数与回执：kernel 返回路径 + 元数据，agent 用自己的看图/read_file 打开。早期架构的看图预算、max_images 替换 hack 整体退役。
- agent 写产物用它的原生工具；kernel 只在 submit 时验收（路径/sha256/编译/覆盖率）。**验收口是边界，不约束过程。**

## 工具面（五六个通用工具，任务类型差异全在任务包与说明书里）

| 工具 | 语义 |
| --- | --- |
| `status` | 全局蒸馏：项目列表、各阶段 done/pending、失败清单、当前可领任务数。输出 ≤1KB 纯文本/JSON，是冷启动仪式的唯一入口 |
| `claim(type?, n=1..5)` | 领 n 个任务 + 租约；返回自包含任务包（输入路径、说明书路径、验收 spec、白名单、书档案路径） |
| `submit(batch=[{task_id, note_summary, ...}])` | 批量提交；kernel 逐条跑验收，返回逐条机器可读 verdict（pass / REJECTED + 错误清单），磁盘状态原子更新 |
| `note(task_id, text)` | 追加任务笔记（append-only） |
| `profile_update(project, text)` | 追加书级档案（append-only，kernel 中介） |
| `issue_add / issue_resolve` | 写共享问题存储（kernel 据此派 rework） |
| `pause / resume / abort` | 项目级控制：停派新任务（任务边界原子）/ 恢复 / 归档终止 |

不做的：为每类任务设计专用工具（classify_figure、convert_chapter……）。工具稳定，说明书和 spec 可迭代。

## 批量形状

`claim(n=3..5)` 一次返回 n 个自包含任务包 → agent 串行干完（结论写各任务 scratch/notes.md）→ `submit(batch)` 一次交 n 条、一次收回 n 条 verdict。

收益：工具往返 ÷n；每次往返后上下文干净（agent 不需要记住上一批细节，下一批 claim 又是自包含包）。**批量与上下文卫生绑定**：n 默认 3–5，与参考实现"compare 一次 2–4 张、模型自己点名"同一哲学。

## 传输载体：CLI 为主，MCP 为壳

同一个 Go 二进制、两套皮，同一组函数、不加新语义（防双层协议行为漂移）：

- **CLI（v0 即完整）**：`dvagent status/claim/submit/...` 子命令，JSON 输出。任何只有 bash 的 agent 零适配开跑；可脚本化、可人工接管调试。
- **MCP server（v1 再加）**：同一组函数的协议壳，换来 schema 自动发现。

## 长操作形状

编译/渲染秒级，工具同步返回（不引入 job/poll 异步）。真正的长操作（整阶段 run）由队列驱动：agent 发指令后按自己的对话节奏周期性 `status()`，kernel 持续在磁盘更新进度。**交互是拉取式，不维持长连接。**

## 冷启动仪式（每次会话开始或被压缩后，成本几百 token）

1. `status` → kernel 扫磁盘重建全局状态（不读任何对话历史）；
2. 向用户播报："上次到 X，Y 待处理，Z 失败"，问是否继续；
3. 用户确认 → `claim` 下一个任务，开工。

可靠性来源：进度永远可以从制品层确定性重建——换 agent、换平台、压缩多少次，续上的都是同一个状态。

## 自有工具去向（旧会话工具三分法）

| 旧工具 | 去向 |
| --- | --- |
| compare_images（合成并排图） | kernel 保留：回传合成图**路径**，agent 自看。删图片上下文管理 |
| view_pdf（渲染/裁剪/mm 尺寸） | kernel 保留：渲染 PNG 到任务缓存区，回传路径+元数据 |
| doc_search / list_source_pages | kernel 保留：回执瘦身（命中+页号+bbox+一句指针），正文 agent 自读 |
| compile / 覆盖率 / 资源盘点 | kernel 保留，不变 |
| request_chapter_split | 拆三份：验证进 kernel、决策给 agent、排队进队列依赖边（父子指针） |
| submit / submit_style / submit_problems / submit_blocks | 归并统一 submit + 各类验收 spec |
| 会话层（压缩/回放/审计/VFS/sidecar） | 退役；转录 JSONL 降级为可选调试记录 |

规律：早期架构每个会话工具 = "kernel 算法 + 上下文搬运"的混合物；剥掉上下文搬运层，剩下的全归 kernel。
