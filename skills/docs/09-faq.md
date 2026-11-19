# FAQ · 常见问题解决表（动态增长，随版本走）

> **用法**：按"现象"检索。每条带版本锚——**`[since xxxx]`** = 该知识/修复引入的版本；
> **`[fixed ≥xxxx]`** = 旧版本才有此症状，升级即消。看到你的 `.skill-version` 早于条目版本：
> 先 `upgrade-check` 升级再照方抓药。**失效条目不删**（划线上标 `[superseded]`），历史可查。
>
> **与 docs/07 的分工**：本表 = **已知症状→即时解法** 的速查；07 = 未知问题的**上报流程**。
> 表里没有你的症状 → 走 07 状态机（start → collect → draft → validate → submit）。

## 追加新条目的格式（维护者/agent 发现新坑后照此补，保持一行一条）

```
| 症状关键词 | 现象 | 诊断 | 解法 | [since 版本] |
```

| # | 症状关键词 | 现象 | 诊断 | 解法 | 版本 |
|---|---|---|---|---|---|
| 1 | MinerU 断点不生效 | prep-mineru-api 重跑又全量上传 | 断点判据是 part 根有无 content_list；半截 auto/ 会误导（旧版判 auto 存在即跳过） | 删残留 auto/ 重跑即续传 | [fixed ≥6a1fc7b] |
| 2 | model_version 400 | prep-mineru-api 报 -10002 | v2 不是合法值（仅 pipeline/vlm/MinerU-HTML） | 默认已改 vlm；config 里别写 v2 | [fixed ≥6a1fc7b] |
| 3 | organize 章节错位 | prep-organize 产出 mineru.md 且只一份；prep-index 报找不到 content_list | part 根输入时命名推导取到父目录名（SK-I17） | 升级；报错现带"试过的 md 名+现存目录"自查 | [fixed ≥0616445] |
| 4 | ctx-up 上文离谱 | ctx-up 返回的段落离目标图很远 | 旧版从头扫描凑数，取到全书开头 | 升级；新版最近 n 块、最近者在前 | [fixed ≥8d399a5] |
| 5 | luadraw 章编译失败 | 报"无 lualatex" | luadraw 需 LuaLaTeX；无则自动走 lua5.4 预编译回退 | 装 lua5.4（或设 LUADRAW_LUA）即可预编译出 TikZ 走正常链；两个都没有才需装 texlive-lualatex | [since e23e4e2] |
| 6 | 队列 .tmp 残留 | book/ 下只有 skills_queue.json.tmp，命令报队列异常 | 进程中断时写入没 rename 完 | 确认无活跃进程后：删 .tmp 与 tasks/ 残留，重铺任务（gen-* 幂等） | [since b81cf0f] |
| 7 | config 被静默重置 | config.yaml mineru 段少写几行，其余键变默认值 | 旧版零值直赋覆盖 | 升级后零值安全；仍建议照 config.example.yaml 全段抄写 | [fixed ≥6a1fc7b] |
| 8 | 引擎链报错 | 编译报 pdflatex 不可用 | 引擎回退链 pdflatex→xelatex→tectonic 逐个试 | 装任一即可；`doctor` 看现有什么 | [since a12d2e9] |
| 9 | 产物结构疑惑 | mineru/ 下没有 auto/ 目录了 | 归一契约=part 根直出（对齐老 DocVision 约定） | 正常，不是错；下游两种布局都兼容 | [since 6a1fc7b] |
| 10 | 想绕过坏环节 | "OCR 不通我自己转录伪造产物接着跑" | **绝对禁止**（docs/07 铁律；伪造产物=伪造进度，历史事故） | 停→查→报（07 状态机）；任务设计已让账本归内核，伪造无意义 | [since 51a9ddb] |
| 11 | 看图缺上下文 | 相邻图的信息没进 ctx-down | 旧版只收纯文字块 | 升级；现收一切带文字的条目（含图片的文字化描述） | [fixed ≥8d399a5] |
| 12 | ctx 上下文形态 | 上下文是一块一块的条目，不是 md 行，也没有上下行号 | 旧参考实现是按 md 行的（行号定位）；现 `dvagent ctx` 已同形态对齐（md 行+行号），块级 ctx-up/down 为底层变体 | [since 下一版] |
| 13 | mermaid 打回 | result=struct 反复不过 | figure.mmd 语法错（内核 mmdc 校验） | 按报错修语法；对齐 docs/luadraw 示例 | [since 1b449d8] |

## 版本对照

| 版本 | 要点 |
|---|---|
| 8d399a5 | ctx-up/down 语义修复 |
| 6452fa6 | 文档引导批（00-start.md） |
| 0616445 | SK-I17 part 根命名修复 |
| e23e4e2 | SK-I16 luadraw 预编译内核 + 置信度/水印 |
| 6a1fc7b | MinerU 上游兼容三修 + part 根归一 |
| 1b449d8 | [若需更早历史见 issue.md 的 SK-I1~I10] |
