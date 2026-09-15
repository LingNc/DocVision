# AI 会话基础设施

> 返回 [README](../README.md)（文档索引见 README「文档」一节）。


- **模型注册表** `models:`：每个专用 AI（classifier/drawing/style/chapter/convert/checker/verifier）可单独配置 base_url / api_key / model / request_body / stream / thinking / reasoning_effort / tool_stream
- **流式请求（默认开启）**：`models.*.stream: true`（默认）时走 SSE 流式接收，长思考/长输出期间持续有进展，不会长时间静默；厂商不支持流式时自动回退一次非流式请求。流式模式下用 `api_stream_idle_timeout`（默认取 `api_timeout`）判定"卡住"，而不是整次请求超时
- **思考控制**：`thinking: {type: enabled|disabled|adaptive}` 与 `reasoning_effort: max|xhigh|high|medium|low|minimal|none` 都是请求体**顶层字段**（不要写进 `request_body.extra_body`），按 `models` 条目配置，空值继承 `models.text`；GLM 的保留式思考写 `thinking.clear_thinking: false`（历史 assistant 轮的思维链完整回传，提升缓存命中）。`type` 写错（如 `disable`）会被服务端**整个请求 400 拒掉**，所以启动时会校验：一眼能认出的笔误（disable/enable/off/on…）自动纠正并在 stderr 告警，其余未知取值直接报错退出
- **会话管理**：每个会话独立上下文窗口（`sessions.*.context_limit`，默认 128K，可设 64K/256K），达到 `compaction_at`（默认 0.85）阈值自动压缩，**两级**：先做**本地裁剪**（零成本，不调用模型）——超长工具结果留"前半 + 后 1/4"（`sessions.*.prune_tool_chars`，默认 4096 字符；实测真实会话里最长的工具结果 6183 字符、p99 5046，8K 阈值从不触发，只有 `read_file` 整文件（硬上限 64KB）与 `bash`（`tools.bash.max_output`，默认 5000）才可能很长），较早的图片换成文字占位（`sessions.*.keep_images`，默认保留最近 3 张；占位写明"这里曾附过 N 张图，需要时重新 view_image"，所以"看过什么"不会丢）；仍超阈值才花一次 **AI 摘要**：**系统提示词、原始任务、最近 8 条消息原样保留**，中间部分由 AI 写成一篇续跑笔记替换（摘要请求是"在原会话末尾追加一条指令"的增量请求，因此仍能命中厂商前缀缓存）；token 估算现在把系统提示词、工具定义、历史回传的 `reasoning_content` 与 `tool_calls` 参数一并计入（原先只数消息正文，实测低估到 2~4 倍），**图片按模型自己那套规则算**（顶层 `estimate.method` 是全局默认，`models.<条目>.image_tokens` 只覆盖那一条）：`pixels` = `宽×高 / px_per_token`（默认 750，夹在 85–4096，取不到尺寸用 `tokens`），`fixed` = 每张固定 `tokens`（默认 1100——实测 deepseek-v4.1-flash 上大图饱和在 ≈1050），`none` = 本地按 0 计；阈值判断**不再只看本地估算**，而是取「本地估算 / 标定后估算 / 厂商实测 `prompt_tokens`」三者最大——标定系数由每次响应的 `prompt_tokens ÷ 请求前估算` 得出（夹在 1~10），厂商实测值是"下一次请求至少这么大"的硬下限，只有真的压缩过才会清掉。这一条也是真实缺陷：2026-09-11 那次运行 `[context]` 只出现 1 行（估算 66,427 = 窗口的 50%）、转录里 **0 条** `COMPRESSED SESSION CONTEXT`，而同一批 debug 行里厂商实测 prompt 已经涨到 132k / 206k / 243k——阈值 0.85×131072 永远够不着，一次都没压过；另外"会话还短、任务与最近 8 条之间没有内容"时不做摘要，这种情况**不再计一次 `Compactions`**（统计里曾混进没发生的压缩）；**阈值检查在每一次模型请求之前都做**——这一条曾经是真实缺陷：守卫原本只在 `Run()` 入口调一次，而 latex 的这些会话"一次 `Run()` 就是整个会话"（初始任务进去以后几百轮都在同一个循环里），于是守卫只见过"系统提示词 + 第一条消息"，**一次都没触发过**：实测一轮运行里 prompt 涨到 323,966 tokens（窗口 128K、阈值 0.85）、转录里从来没有 `=== COMPRESSED SESSION CONTEXT` 行，费用一路升到账户余额不足；现在压缩在循环内每轮检查，并在**普通日志**（不需要 `--debug`）里打出上下文增长：占用到 50%、80% 各一条 `[context] 估算 N tokens = 窗口(M) 的 P%`，压缩本身打 `[compact] 本地裁剪…` / `[compact] history compacted…`；另外"压完仍超阈值"（窗口极小、或尾部本身就很长）不会每轮重复花钱摘要——只有在上次压缩之后又涨了 20% 才再压一次。**本地裁剪只在接近窗口时才发生**（不是每轮都裁）：窗口很大时即便有 5 万字符的工具结果也一个字都不动，历史里"早先看过的图"也不会被换成占位；裁剪后仍超阈值才进入 AI 摘要，因此"裁过就不用了"的担心不成立；压缩后续跑从**最后一次压缩检查点**开始（`LoadTranscript` 丢弃最后一条 `COMPRESSED SESSION CONTEXT` 之前的消息，并在写摘要后补写"原始任务 + 最近 8 条"，回放与内存一致）；`sessions.checker` 未设置的字段继承 `sessions.convert`（唯一例外：`max_tool_rounds` 默认 50）
- **矢量图落盘**：TikZ 编译成功后依次尝试 `dvisvgm → pdftocairo → mutool → inkscape` 转 SVG 内嵌（dvisvgm 3.6 处理 PDF 需要 Ghostscript < 10.01 或 mutool，缺条件时自动走 pdftocairo）；全部失败才回退 PNG/PDF 链接——档位2 只记日志与 `progress.json`，档位1 另在 markdown 就地标注 `<!-- DOCVISION-ERROR: … -->`
- **图片查看工具**：`image_context` 只给文本上下文与前后引用（不看像素），`view_image` 才看图片（支持百分比裁剪与放大）。`view_image` 的 `path` **两种写法并存**：① markdown 里的引用路径 `images/<书>/<file>` ②**只给文件名**——按书为 `Subject` 的作图会话直接拼在书目录下，项目级会话（style/convert/修复/终审）则在图库内做**深度受限的唯一匹配**（同名文件出现两份就报错并列出候选路径，绝不悄悄拿别本书的图）；两种都不命中时报"文件不存在"并列出目录里**真实存在**的文件名
- **编译与看图分离**：`compile {path:"figure.tex"}` 只返回编译日志、产物名与页数，看图统一用 `view_pdf {path:"standalone.pdf", page:1}`（裁剪 + zoom 直接从 PDF 高分辨率重渲染，真放大，不是拉伸像素）
- **工具图片的归属句柄**：工具产出的图片在 wire 上只能走 **user 消息**——`tool` 消息的 content 只能是文本（OpenAI 兼容 schema 里没有放图片的位置，也没有放 `tool_call_id` 的位置；DSH 同样靠"内部模型里图片是 tool-result 块的附件、发请求那一刻才摊平成 text + image_url"来区分），而 `user` 消息又放不下 `tool_call_id`。所以归属写在**文本句柄**里，不动任何 wire 字段：每次图片回灌的 user 轮，其 text 段是 `Tool image output from <工具名> (call <调用 id>) (for your visual review):`（前缀 `Tool image output` 固定不变——`docvision sessions` 正是靠它认旧转录；工具名/call id 缺失时对应部分省略）。这样转录本身就写得清"这张图是哪一次调用投出来的"，而模型的请求内容仍然是一条合法的、只有 text + image_url 两段的 user 消息。**旧转录（只有前缀、没有归属）不会被抛弃**：查看页按顺序把它归到最近一条带 `tool_calls` 的助手消息里、还没被认领的那一次调用上，并在 UI 上标明"由顺序推断"，新转录则标"call <id>"（详见 [commands.md](commands.md) 的「消息形态」一节）。
- **可分离工具**：会话工具按需注册（编译、提交确认、grep、bash 沙箱、受限文件读写、PDF/图片查看等）
- **断点续传**：档位2 逐图进度、档位1 逐阶段进度（`progress.json`）；控制台进度行只算**本次**要处理的部分（`Already done: N | To process: M` 给出基数）
- **实时进度块（多行，每人一行）**：控制台不是"一条会被覆盖的状态行"，而是一块**每人一行的实时区**——阶段进度（`classify`/`process`/`convert` 聚合行）与每个并发子会话各占一行：`[convert:chapter_002] 轮次 12 · 工具调用 25 · 已用 3m04s`、`[checker:chapter_001] …`、`[style-fix:chapter_003] …`。子会话跑完它那一行**消失**，新开始的补在下面，所以 `latex.concurrency: 4` 时能并排看到 4 个子会话各自的轮次/工具数/已用时间。日志行永远先擦掉整块、写在自己那一行、再把块画在下面，终端 scrollback 里不会出现两条内容叠在同一行（旧实现只有一个 live 槽位，并发的样式修复与它的父阶段每秒互相覆盖，看起来就是同一行文字来回跳；日志行前只补一个 `\n` 又把状态行永久留在滚动区，看起来就是"重叠"）。`--console-verbose`（或 stdout 不是终端时按变化追加整行）下不启用实时区。
- **工具轮次软限制**：`max_tool_rounds` 限制的是**assistant 轮次**（一轮里发多少个 tool_call 都只算 1 次）。用满 `sessions.*.tool_rounds_warn_ratio`（默认 0.7）后，每轮往会话里更新一条提醒（"已用 N/M 轮，还剩 K 轮，请合理使用并尽快提交"）；到达 `max_tool_rounds` 后**还能再用 `sessions.*.tool_rounds_grace` 轮**（默认 20，负值=不留宽限），此后才真正禁用工具、逼最终文本。提醒是**原地替换**同一条消息，不膨胀历史也不破坏前缀缓存。
- **看图软预算**（按"对象"计数，长文档不吃亏；矢量图会话在**首次提示词里就一次性告知**额度，不再每次看图都重复提醒）：`tools.view.image_max`（默认 30）是**同一张图片文件**的软上限，`tools.view.pdf_max`（默认 25）是**同一个 PDF 的每一页**的软上限——所以一本书里每页各有 25 次额度，而不是整个会话共用一个池子。用满 `tools.view.warn_ratio`（默认 0.7，向上取整）起，每次 `view_image`/`view_pdf` 的结果里附带"已用 N/30，仅剩 K 次"（30×0.7=21 → 从第 21 次起提醒），超出后提示"预算已用尽，请尽快完成并提交"——**只提醒，不拦截调用**（0=默认值，负值=不限）。
- **原图尺寸测量**：每次 `view_image` 都会实时算出原图的**印刷尺寸**并回给模型——位图先解析软链、再按**文件名**匹配 `paths.mineru_output` 下 MinerU 解析（`content_list.json` 的 bbox + `layout.json` 的页尺寸；位图被 images 阶段拷进项目后名字不变，靠文件名就能接回解析），显示标准为 **mm 优先**：`ORIGINAL FIGURE SIZE: 36.9mm x 20.2mm on the page (about 21% of the page width); bitmap 284x156px, effective resolution 195 dpi, aspect 1.82:1`（高度按位图自身比例换算，因为 MinerU 的块 bbox 不紧贴图）。**裁剪之后还会给出当前裁剪区域的尺寸**（`this crop is 18.4mm x 10.1mm on the page`），因为"现在看的是多大一块"正是复现细节时需要的；真的找不到解析（比如位图不在本次解析里）才退化为只给宽高比。
- **PDF 尺寸**：`view_pdf` 回执给出**该页真实尺寸**与**当前裁剪区域的尺寸**（都是 mm），看原书页时它就是真实书本尺寸（可直接用来确定 `geometry` 的纸张），看自己编出来的 PDF 时就是成品的实际尺寸——两边同单位才谈得上"是否符合要求"。`compile` 成功回执里的产出尺寸同样用 mm（`Drawn size: 104.2pt x 66.4pt (36.8mm x 23.4mm, aspect 1.57:1)`）。
- **会话转录（JSONL）**：每条消息实时追加为一行 JSON（含 `reasoning_content` 思维链——GLM 保留式思考要求历史思维链完整回传，也是前缀缓存的前提），图片以 `file://media/<hash>.<ext>` 引用（base64 不入转录）。**转录里没有 system 行，也没有工具的 JSON Schema**——这是设计如此，不是丢失：系统提示词由运行时从 `internal/prompts` 模板重新渲染（含水印开关、挂载说明等本次运行才确定的内容），工具定义由运行时按会话类型拼装，两者都不落盘；不过每个会话开头会写一条 **`t=meta` 元信息行**（`{"t":"meta","kind":"system","session_label":…,"model":…,"system_sha":…,"text":"<完整系统提示词>","tools":[{name,description,parameters}…]}`），把**本次运行模型被交代了什么**（系统提示词全文 + 当时发给 API 的工具定义）原样记下来——它**只用于查看、永不参与回放**（`LoadTranscript` 只认 `t=="msg"`，续跑语义与"系统提示词由运行时重新渲染"的原则都不变），同一份提示词重复挂载时按哈希去重不会写第二条，提示词变了（例如换了模板）才会追加一条；`docvision sessions` 的页面会把它显示成可折叠卡片；**每行都带写入时间戳 `ts`**（RFC3339 毫秒；早于该字段的旧转录只是没有它，读取方一律按"可能缺失"处理），**每次 API 请求再写一条 `t="usage"` 用量行**（`{"t":"usage","ts":…,"model":…,"stream":…,"round":…,"kind":…,"prompt_tokens":…,"cached_tokens":…,"completion_tokens":…,"reasoning_tokens":…,"duration_ms":…,"ttft_ms":…,"finish_reason":…,"text_tokens":…,"image_count":…}`；`kind` 为空=普通回合，`nudge`=空回复后的强制文本请求，`compact`=上下文压缩摘要请求——**三者的 token 都要计入**，一个压缩过 5 次的会话确实付了那 5 次 prompt 的钱）；它和 `t=meta` 一样**永不参与回放**，只用于查看与统计，所以转录本身就能算出"输入/输出 token、前缀缓存命中率、耗时、首字延迟、输出速度"，不需要翻 `--debug` 日志；`text_tokens`（该次请求发出前的本地文本估算，不含图片）与 `image_count`（该次请求带了几张图）是本地那一半，预览页靠相邻两条用量行的差值反推"每张图片实测多少 token"（`(Δprompt_tokens − Δtext_tokens) / Δ图片数`，取各步中位数，遇到本地裁剪/压缩导致的请求缩小则跳过该步）。要更细的逐轮请求（流式进展、每轮提示词）仍看 `--debug` 的 `[DEBUG]` 摘要；**流式快照 sidecar（P7）**：`<转录>.partial` 与转录并存——会话流式生成期间每 ~150ms 把「当前正在生成的那条消息」的累积文本（尾部截断 16KB）原子覆写进去（tmp + rename，并发读者不会读到半截 JSON）；完整消息 `Append` 落盘的那一刻与 writer `Close` 时都会删除它。预览服务（可能是独立进程）读这个文件即可展示实时输出，转录本体保持 append-only 整行语义不变；矢量图会话（`<outDir>/sessions/vector_<图>.jsonl`）、样式会话（`work/style_session.jsonl`，样式反馈打回复用同一份）、章节划分（`work/sessions/chapters.jsonl`）、单章转换（`work/sessions/convert_<章>.jsonl`）、逐章 checker（`work/sessions/checker_<章>.jsonl`）、**样式修复（`work/sessions/style_fix_<章>.jsonl`）**都接入——进程被杀或网络断连后，下次运行自动从转录恢复上下文续跑，不重烧 token（恢复时**重新挂上系统提示词**、只回放最近一次压缩之后的消息——压缩刻意保留的"原始任务 + 最近 8 条"会在写完摘要后补写一遍，否则回放截断会把它们一起丢掉）；成功会话的转录默认清理（`latex.keep_session_records` 可保留）

## 提示词集中管理（`internal/prompts`）

所有**内置提示词**都放在 `go/internal/prompts/templates/*.md`（用 `//go:embed` 编进二进制），不再散落在各会话的 Go 源码里：

- **系统提示词**（12 段：classifier / figure / style / chapters / convert / style-fix / final-review / fix / verify / watermark / img2text / text-only）与**各会话的首次用户提示词**（7 段）都在模板里；模板用 `{占位符}` 接收动态数据（如 `{OUTPUT_LANG}`、`{MAX_ROUNDS}`、`{MANUAL}`、`{CONTEXT}`）。
- 调用点只有两种写法：纯静态用 `prompts.Must(prompts.FigureSystem)`，带数据用 `prompts.Render(prompts.ConvertUser, map[string]string{"MANUAL": manual})`——占位符替换逻辑只有一份（`prompts.Fill`），不会出现"某个会话忘了替换"的情况。
- 注册表（`prompts.go` 的 `registry`）为每个模板声明三件事：占位符清单、**该会话必须提到的工具名**、以及不得出现的**退役工具名**；`prompts.Templates()` 可自省，供测试与文档使用。
- 仍然留在原地的只有"跟着状态实时累加"的碎片：轮次预算提醒、图片占位说明、压缩指令、续跑/重试提示、checker 反馈回写等（它们与运行时状态同生共死，抽出来反而更难读），以及各工具自身的 `Definition()` 文案。

守护测试（`go/internal/prompts`、`go/internal/latex`）：

1. 模板文件与注册表**双向一致**（磁盘上有未登记的模板、或登记了不存在的模板都失败）；
2. 每个模板**按声明的占位符全量渲染后不得残留 `{...}`**（历史事故：`{OUTPUT_LANG}` 曾原样发给模型）；
3. 模板锁定的**工具名必须在代码里真实存在**，且代码里的工具必须在某处模板被提到（工具改名/新增而提示词没跟上就会失败）；
4. 退役工具名不得复活（`read_md` / `view_page` / `list_images` / `install_font` / `compile_preview` / `preview.png` / `format_fix_attempts` 等——这些名字都曾长期残留在提示词里，模型因此调用不存在的工具）。

> 这样做的原因很实际：同一个规则曾写在两个会话里而互相矛盾（"禁止重叠/拥挤"与"画大一点"）、工具删了提示词还在教、占位符漏渲染直接发给模型——把提示词收进一处之后，这些问题都能被测试而非人工巡检拦住。

## 会话沙箱与虚拟工作区（挂载表）

每个 AI 会话有独立的命名空间（挂载表），结构化工具与 `bash` 看到**完全同一套路径**，只在需要时挂对应的树——模型拿不到整个 `mineru_output`、转录或构建产物：

| 挂载点 | 内容 | 权限 |
| --- | --- | --- |
| `work` | 会话自己的可写工作区——作图/拆章会话=专用 scratch；样式会话=`work/style`；转换与样式修复会话=`work/temp/conv_<章>/work`（**本章的临时工作区**：cls/手册/示例副本 + 插图 + 该编译的 wrapper + 待提交的两个路径，随会话用完即删）；修复/终审会话=构建树 `build/` | 可写 |
| `project` | 项目只读窄视图：`source`/`style`/`chapters`/`converted`/`reports`；`work/sessions` 转录、`work/temp`、`doc_index/`、`pages/`、`build/`、`out/` 都不在其中 | 只读 |
| `source` | 本书原书 PDF 的最小视图（`work/pdfview/<part>.pdf`，只含本书的 `*_origin.pdf`） | 只读 |
| `build` | 已不再使用：转换与样式修复会话的编译就发生在自己的 `work:` 临时工作区里（wrapper 与类都在那儿），全书构建树对修复/终审会话是 `work:` | — |

挂载表的**第一个挂载点即默认挂载点**：写 `x.tex` 等价于写 `<默认挂载点>:x.tex`。

路径语法：`path`（默认挂载点）、`name:path`（如 `project:style/manual.md`）、`/name/path`（bash 侧同路径）；越界 `../` 与只读挂载点写入会被直接拒绝，未知挂载点报错并列出可用挂载点。

`tools.bash.sandbox`（默认 true）把会话 `bash` 包进 **bubblewrap**：沙箱里只存在上表的树（`/work`、`/project`、`/source`），宿主真实路径一律不存在，`--unshare-net` 断网；bwrap 缺失时自动回退普通 shell 并记警告。

沙箱内的 `/tmp` 是**本会话私有的持久临时区**（`<proj>/work/temp/bash_<会话>`，`TMPDIR/TMP/TEMP` 都指向它）：同一个会话的多次 `bash` 调用共享它，会话结束后删除（`latex.keep_temp_dirs` 或 debug 下保留）。**一个会话一张挂载表**——`read_file`、`grep`、`write_file`、`edit_file`、`bash`、`view_pdf` 看到的是同一棵树、同一套挂载点名，所以 `project:chapters/<章>.md` 在哪个工具里都是同一个文件；`grep` 不带 `path` 时遍历全部挂载点（全项目检索），带 `project:style` 这类前缀时只搜该挂载点。

**Python 环境**（`tools.python`）：会话 bash 跑在 `--unshare-net` 的沙箱里，**会话自己装不了任何包**（2026-09-11 实测：样式会话遇到 `ModuleNotFoundError: No module named 'PIL'` 后花约 10 轮自己手写了一个 PGM 解析器）。所以环境由宿主提供——按配置准备 `system`/`venv`/`conda` 解释器并把它的**运行所需目录只读绑定**进沙箱（解释器自身目录、`sys.prefix`、软链链上的每一跳、`ldd` 列出的共享库目录、site-packages；Homebrew 的 venv 就因为 `bin/python3 → .linuxbrew/opt/... → Cellar` 这条链少一环而 exec 失败，bash 的 PATH 搜索会**静默跳到下一个 `/usr/bin/python3`**，于是"装好的环境"根本没生效），`PATH` 指向它；缺模块时宿主侧（有网）自动 `pip install` 并让模型重试（`PIL`→`Pillow`、`fitz`→`PyMuPDF`、`cv2`→`opencv-python` 等**会自动换算成 PyPI 包名**——`pip install PIL` 本身永远装不上）；`mode: conda` 时 conda 可执行文件**不再只靠 PATH**（`~/.bashrc` 里 `conda init` 只对交互 shell 生效，服务方式启动的 DocVision 看不到它，于是"配置写 conda、实际用 Homebrew python"这种静默降级曾让自动安装全灭），会依次尝试 PATH → `$CONDA_EXE` → `~/miniconda3`/`~/anaconda3`/`~/miniforge3`/`/opt/conda` 等常见前缀，找不到就在日志里明确警告并说明回退到了哪个解释器；解释器若是 PEP 668 的 externally-managed（Homebrew/Debian 的 python），`pip install` 会自动改用 `--break-system-packages` 重试一次。安装失败则按缺字体同样的办法留下 `<项目>/work/python/requirements.txt` + `README.md`。沙箱内用 `python3` 即可，无需 Activation。

档位1 的临时工作区统一落在**本项目工作区**里的 `work/temp/<名称>`（即 `latex_project/<项目名>/work/temp/…`；拆章沙箱、样式/反馈 scratch、每章转换/修复工作区 `conv_<章>/work`），不再藏进 `/tmp`；`latex.keep_temp_dirs` 可保留以便事后检查。

## 调试日志

```bash
docvision latex --debug        # 或配置 options.log_level: "debug"
docvision latex --trace        # 或配置 options.log_level: "trace"（更细）
docvision img2text --debug     # img2text 同样支持
docvision verify --debug
docvision latex --verbose      # 详细控制台输出（默认仅显示进度行，5-10s 自动刷新一次；进度行只在控制台，日志文件里看并发与耗时）
#   进度行是终端里的"原地覆写"行：重定向/管道时改为按节拍整行打印（日志里会看到多行进度），
#   阶段结束时保留最后一行状态，不会留下空行
```

日志等级：`info`（默认，进度与警告/错误）< `debug`（每轮请求/响应摘要、提示词、工具调用、**最终接收内容**、编译结果与警告、Mermaid/LaTeX 校验结论）< `trace`（再加流式分片进展行等噪音）。全部写入日志文件（`[DEBUG]`/`[TRACE]` 前缀，控制台输出不受影响），每次请求/响应记录：使用的模型、`stream`/`max_tokens`/`temperature`/`thinking`/`reasoning_effort` 实际取值、消息数与上下文估算、耗时、finish_reason、输出与思维链字符数、provider 返回的 token 用量（含 `reasoning_tokens`）。LaTeX 编译只记 `OK|FAILED + 耗时 + warnings=N + 警告清单`（失败时附给 AI 的错误原文），完整编译日志不会写入。可在日志里完整回放某个会话的推理与工具使用过程。

默认控制台输出与 img2text 一致：每个阶段只显示一行实时进度（如 `[classify 12/345] 3.48% (failed: 0, running: 2)`；process 行还带 `done/errors/fallback/raster` 计数），逐图明细写入日志文件；`--verbose` 恢复逐图控制台输出。**进度行只统计本次运行**：断点续传时"此前已完成多少"只在 `Already done: N | to process: M` 那一行出现，所以续跑的第一行是 `[0/396] 0.00%`，而不是一上来就 97%。

`--debug`（或 `options.log_level: debug`）下，临时工作目录与会话转录**必定保留**（`latex.keep_temp_dirs` / `latex.keep_session_records` 打开也保留），因此一次完整运行事后可以逐会话复现。
