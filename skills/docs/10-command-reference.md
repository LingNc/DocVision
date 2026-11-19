# 命令与工具参考 · 签名 / 输出 / 新旧语义对照

> **本表由代码抽取生成**（docs/tools/ 抽取器；改代码后重跑刷新，勿手改本表数据区）。
> 生成基准：`main.go` 全部 case + `mcp.go` 工具注册表。版本锚见各节。
> 双皮约定：CLI（dvagent 子命令）与 MCP 工具**同名同函数**（ctx/gen-img2text 等共享 helper）；发现双皮行为不一致即 bug，报 SK 台账。

## 1. CLI 命令全集（49）

### 1.1 任务循环（agent 主用）

| 命令 | 签名 | 输出 |
|---|---|---|
| `status` | （无参/status） | JSON：项目与队列状态总览 |
| `claim` | claim 需要: <项目名> <owner> [n] [types] | JSON：领到任务的完整 Task 对象数组（含 id/type/sandbox/accept）；空=无任务 |
| `claim-wait` | claim-wait 需要: <项目名> <owner> [--interval N] [--timeout N] [--types t1,t2] | JSON：同 claim；超时 {"claimed":0,"timeout":true} |
| `submit` | submit 需要: <项目名> <task_id,...> | JSON：验收结果（errors 数组，空=过；过了自动推进状态） |
| `note` | note 需要: <项目名> <task_id> <文本> | JSON |
| `ctx` | ctx 需要: <项目名> <task_id> [--up N] [--down M] | JSON：{task, md, image_line, range:[起,止], lines:[{line,text}]} |

### 1.2 铺任务与生成（gen-* 家族，幂等）

| 命令 | 签名 | 输出 |
|---|---|---|
| `split` | split 需要: <项目名> <parent_task_id> <segments.json> | JSON：{report} 结构（见源码 out） |
| `gen-classify` | gen-classify 需要: <项目名> [images目录] | JSON：{report}（铺/跳过计数） |
| `gen-convert` | gen-convert 需要: <项目名> [chapters目录] | JSON：{report} 结构（见源码 out） |
| `gen-check` | gen-check 需要: <项目名> | JSON：{report} 结构（见源码 out） |
| `gen-draw` | gen-draw 需要: <项目名> [章md名] | JSON：{report}（Added/VectorStems 等） |
| `gen-img2text` | gen-img2text 需要: <项目名> | JSON：{report}（铺任务数/跳过数） |
| `img2text-collect` | img2text-collect 需要: <项目名> | JSON：{report} 结构（见源码 out） |
| `gen-final` | gen-final 需要: <项目名> | JSON：{report} 结构（见源码 out） |

### 1.3 PREP 无人值守段

| 命令 | 签名 | 输出 |
|---|---|---|
| `prep-split` | prep-split 需要: <项目名> <pdf路径> [每份页数] | JSON（见源码 out） |
| `prep-mineru` | prep-mineru 需要: <项目名> <part.pdf...> | JSON（见源码 out） |
| `prep-organize` | prep-organize 需要: <项目名> | JSON：{md_files:[...]} |
| `prep-index` | prep-index 需要: <项目名> [md名列表（默认 chapters/*.md）] | JSON：{entries, pages, page_srcs} |
| `prep-mineru-api` | prep-mineru-api 需要: <项目名> <part.pdf...> | JSON：按 part 的 {part,status:done|skipped|error, batch} 数组 |
| `img2text-assemble` | img2text-assemble 需要: <项目名> <源md> <输出md> | JSON：{md, replaced, kept, mermaid, image_lines, summary} |

### 1.4 检索 / 查看 / 协作

| 命令 | 签名 | 输出 |
|---|---|---|
| `assemble-verify` | assemble-verify 需要: <项目名> | JSON（见源码 out） |
| `doc-search` | doc-search 需要: <项目名> <query> [max] | JSON（见源码 out） |
| `page-render` | page-render 需要: <项目名> <global页> [x0,y0,x1,y1] | JSON（见源码 out） |
| `ids-list` | ids-list 需要: <项目名> [kind] | JSON（见源码 out） |
| `viewer-build` | viewer-build 需要: <项目名> | JSON（见源码 out） |
| `viewer-apply` | viewer-apply 需要: <项目名> <regroup.json路径> | JSON（见源码 out） |
| `luadraw-search` | luadraw-search 需要关键词：dvagent luadraw-search <关键词...> | JSON：{hits:[{kind,title,tex,line,pdf_page}], count, tip} |

### 1.5 配置 / 记忆 / FAQ / 生命周期

| 命令 | 签名 | 输出 |
|---|---|---|
| `register` | register 需要: <名字> <项目根> | JSON（见源码 out） |
| `activate` | activate 需要: <名字> | JSON（见源码 out） |
| `profile-update` | profile-update 需要: <项目名> <文本> | JSON（见源码 out） |
| `issue-add` | issue-add 需要: <项目名> <task_id> <描述> | JSON（见源码 out） |
| `issue-resolve` | issue-resolve 需要: <项目名> <issue_id> | JSON（见源码 out） |
| `pause` | pause 需要: <项目名> | JSON（见源码 out） |
| `resume` | resume 需要: <项目名> | JSON（见源码 out） |
| `abort` | abort 需要: <项目名> | JSON（见源码 out） |
| `mcp` | 见 --help / 源码 | JSON（见源码 out） |
| `config-get` | config-get 需要: <项目名> | JSON（见源码 out） |
| `config-set` | config-set 需要: <项目名> <key> <value> | JSON（见源码 out） |
| `doctor` | 见 --help / 源码 | JSON：环境检查（工具在位/缺失清单） |
| `memory-put` | memory-put 需要: <项目名> <key>（内容走 stdin） | JSON（见源码 out） |
| `memory-get` | memory-get 需要: <项目名> <key> | JSON（见源码 out） |
| `memory-list` | memory-list 需要: <项目名> [前缀] | JSON：{keys:[...], count} |
| `memory-rm` | memory-rm 需要: <项目名> <key> | JSON（见源码 out） |
| `faq-add` | 见 --help / 源码 | JSON（见源码 out） |
| `faq-search` | faq-search 需要关键词 | JSON：{hits:[行原文], count} |
| `upgrade-check` | 见 --help / 源码 | JSON：{upgrade_needed, behind, skills_changed, local_ref, remote_ref} |
## 2. MCP 工具全集（28）

> 宿主经 MCP 协议暴露给 agent 的工具。与 CLI 同名同函数（双皮同体）。

| 工具 | 参数（必填*） | 说明 |
|---|---|---|
| `status` |  | 全局蒸馏：项目列表+各阶段 done/pending+失败清单（冷启动唯一入口） |
| `claim` | project*, owner*, types | 领 n 个任务+租约，返回自包含任务包 |
| `submit` | project*, task_ids* | 批量提交：逐条跑验收器，返回逐条 verdict |
| `note` | project*, task_id*, text* | 追加任务笔记（append-only） |
| `doc-search` | project*, query* | 全书检索：回蒸馏命中（global页号+bbox+截断） |
| `assemble-verify` | project* | 复核 done 产物自验收以来未被篡改（装配前闸门） |
| `deliver` | project*, cls | 装配交付：漂移闸门→归集→主文件→书签→standalone→盘点 |
| `gen-classify` | project*, images | 扫描图目录铺 classify 任务（含 imgxxx 编号登记） |
| `gen-convert` | project*, chapters | 章节 md 铺 convert 任务（md 拷沙箱作覆盖率对照源） |
| `gen-check` | project* | 为每个 convert 铺 blocked 核对任务（转换 done 自动解锁） |
| `gen-draw` | project* | 按 classify 结论铺 draw 任务（覆盖分组下组员跟随代表） |
| `gen-img2text` | project* | doc_index 图片条目铺 img2text 小轮转任务 |
| `gen-final` | project* | 全书落定后铺终审任务（T7） |
| `style-collect` | project* | styled_text 提取内容汇总进 style 素材池（选材数据面） |
| `memory_put` | project*, key*, content* | 项目共享记忆区写入（memory/，多会话共享；key=相对路径，content=内容） |
| `memory_get` | project*, key* | 项目共享记忆区读取（原始内容） |
| `memory_list` | project*, prefix | 项目共享记忆区列表（可选前缀过滤） |
| `memory_rm` | project*, key* | 项目共享记忆区删除 |
| `img2text-collect` | project* | img2text 描述回填 doc_index Content |
| `ctx-up` | project*, task_id* | img2text 小轮转：取该图前文文字块（上下文检索） |
| `ctx-down` | project*, task_id* | img2text 小轮转：取该图后文文字块 |
| `page-render` | project*, bbox | 渲染原书某页（可 bbox 裁剪）到 work/renders/ |
| `prep-organize` | project* | content_list 拼章节 md（PREP） |
| `prep-index` | project*, md_names | 建 doc_index+页表（PREP） |
| `viewer-build` | project* | 生成静态看图台（分组/对比/调参导出） |
| `ids-list` | project*, kind | 查人类编号映射表（imgxxx/vecxxx） |
| `pause` | project* | 暂停项目（停派新任务） |
| `resume` | project* | 恢复项目 |
## 3. 新旧语义对照（逐项 · 差异均注理由，供复核）

> 旧 = 原 DocVision 参考实现（契约原文存 `agent/prompts-archive/`）。**"同"=语义等价，"异"=有意变更**（理由标注；标 ⚠ 的建议你重点复核）。

| 旧机制 | 现行 | 判定 | 差异与理由 |
|---|---|---|---|
| `get_more_context(↑N, ↓M)` 单工具双方向、md 行定位 | `ctx <项目> <任务id> [--up N] [--down M]` | **同（已逐项对齐）** | 行号定位/双方向/行内容三要素全对（合成工作区实测 [since 5834748]） |
| 模型自由回调全套工具（看图上文/下图/OCR…） | `claim`/`submit` 领任务制 + 手册工具 | 异 ⚠ | 账本权从模型没收给内核状态机（防伪造进度——你的 OCR 伪造事件是动机）；agent 自主性下降是可预期代价 |
| 每图输出 JSON 含 program items 自报进度 | 内核 queue.json 台账 + verdict.json | 异 ⚠ | 同上：进度=内核记账，不信模型自报 |
| textonly 单轮 OCR（一图一轮出文字） | img2text 三态任务（文字化/结构图/保留，内核验收） | 异 ⚠ | 质变：分散决策升为带验收的任务制；OCR 质量由 [IMG_TYPE]+replacement 契约保障 |
| 旧 classify：4 类 + styled 标记 + confidence | 4 类 + styled 独立标记 + confidence | **同（SK-I18 已恢复）** | 曾因误合并为 3 类丢失 styled=false 直通路与 table 独立类，已按原文恢复 |
| 输出头 `[IMG_TYPE:]` | 产物首行强制 + 八道闸 | **同（保留）** | 你拍板保留（[since ea5d4b5]） |
| `latex_chapters` 拆章阶段 | convert 内递归细化 + 行数上限 | 异 | 阶段合并进 convert（初划可有可无语义） |
| `latex_fix` + `stylefix` 三段（assess/block/integrate） | style 四步（写样式/选材/试点/试点修复）+ rework 循环 | 异 ⚠ | 三段式合并为试点闭环；assess 职能由 check 任务承担 |
| `latex_watermark` 阶段 | classify 标 `"watermark":true` + assemble 默认去水印 | 异 | 按你拍板：默认去水印（[since 17c0a71]） |
| `latex_verify` 三对照 QA | 无 | **不实施** | 你拍板：原程序自身未启用过（SK-I4） |
| luadraw | （旧程序无） | 新增 | LuaLaTeX 绘图包引入 + 无 lualatex 两段式预编译回退（[since e23e4e2]） |
| `progress items` 断点续传 | 内核断点判据（part 根 content_list 等）+ 任务幂等 | **同（更强）** | 续传由内核状态决定，不依赖模型输出完整性 |

## 4. 多子智能体并发原语

| 需求 | 机制 | 形态 |
|---|---|---|
| 互斥锁（同任务不被两会话领） | 内核队列 **租约（lease）**：claim 即持锁，过期自动回收（防会话暴毙死锁） | 内核内建，无应用层锁 |
| 任务分发/排队 | 队列先到先得；`claim <项目> <owner> [n] [--types ...]` | 即时 |
| **阻塞等待（条件变量语义）** | `claim-wait <项目> <owner> [--interval N] [--timeout N] [--types ...]` | **轮询式阻塞**：无任务时睡眠 interval（默认 5s）直到领到任务或超时；不占 CPU、可随停 |
| 共享工作区 | `memory/`（四命令）+ tasks/ 沙箱物理隔离 | 自由层无锁，后写覆盖 |
| 通知（唤醒） | **现状=轮询，无推送** | 有意取舍：零外部依赖、可随停、可观测；代价是 interval 级延迟 |

**"条件变量 + 通知"的三个升级选项（待拍板，均未实施）**：
1. **fsnotify 文件监听**：内核写 queue 后 touch 一个 `wake` 文件，claim-wait 用 fsnotify 阻塞——纯 Go 可内嵌，毫秒级唤醒；多机不适用（共享文件系统才有效）
2. **unix socket 控制通道**：`dvagent wait-sock` 挂 socket，内核派发时主动 push——真·通知，单机多进程完美；需要进程间 socket 管理
3. **外置 mq（redis/nats）**：跨机通知——引入外部依赖，与"安装包自足"哲学冲突

当前轮询对 interval≥1s 的场景完全够用；若未来 convert 会话要"毫秒级响应新图任务"再上选项 1（成本最低）。
