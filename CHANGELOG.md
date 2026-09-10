# Changelog

## [Unreleased]

### Changed

- **会话工具统一（读文件 / 编译 / 原书页）**：
  - `read_file` 成为唯一读文件工具：`{path, start_line?, end_line?}`（整文件或带行号的窗口），`AltRoots` 支持只读附加根——style 工作区既能读自己写的 cls/manual/example，也能读项目 md/chapters；删除重复的 `read_md`、`read_lines` 与章节专用 md-grep。
  - `compile` 统一为**只收路径**的通用编译工具（`CompileTexTool`）：`{path?, engine?, passes?, bib?, shell_escape?, args?, timeout?}`，多文件 `\input` 工程可直接编译，`engine:"latexmk"` 走完整多遍构建（参考文献/toc/refs），成功返回产物 PDF 名+页数并提示用 `view_pdf` 检视。tikz 的 `compile_preview` 改为 `compile {path:"figure.tex"}`：模型先 `write_file` 再按路径编译，绝不内联代码；工具把 body 包装成 `standalone.tex` 编译，预览图仍走 `view_image`/`view_pdf`。
  - style 会话新增 `compile {path:"example.tex"}`（cls 同目录直接命中，可先自查再 submit）与 `read_file`；`list_pages`/`view_page` 改名 `list_source_pages`/`view_source_page`（原书扫描页，与"看自己编译出的 PDF"的 `view_pdf` 区分开），提示词同步说明。
  - convert 会话新增 `view_image`（看 md 中引用的原图），并把 `source/images`、`source/figures` 符号链接进预览编译的 scratch，章节里的图片引用不再因缺资源而编译失败。
  - assemble 的修复会话升级为完整工作区会话（read/write/edit/grep/bash/compile/view_pdf/view_image/list_fonts，`write_file` 允许任意文本扩展名），全书编译改用 `CompileFull`（有 latexmk 用 latexmk，否则两遍）。
  - 新增配置 `latex.bash_max_output`（会话 bash 返回给模型的字符上限，默认 5000），`default.yaml`/`config.example.yaml` 与 `setup` 校验同步。
  - process 进度行新增 `raster: N` 计数（档位1 保留原图并生成解释的图数量），`[raster]` 日志附 `described=true|false`——档位1 控制台可直接看出"矢量 vs 原图"比例。

- **章节提交（文件夹）与样式回环定向重做**：
  - convert 会话的写权限收敛为「自己的主文件 `chapters/<base>.tex` + 自己的资源目录 `chapters/<base>/`」（`WriteWorkFileTool.Prefixes` 白名单 + `RejectDocumentclass` 仍禁止 `\documentclass`），`write_file` 改为必带 path；提交即整棵章节树，assemble 复制整棵 `work/chapters`（保留多文件层级，只有顶层 `*.tex` 作为 `\input`），复杂层级/内嵌资源不再被丢掉。
  - 样式反馈回路改为**定向重做**：不再丢弃全部章节，只挑出「工作汇报报问题」或「新 cls 下编译失败」的章节，为它们并发跑 style-fix 子会话（`styleFixSystemPrompt`：读现有 .tex，用 edit_file 增量适配新 cls/manual，保留全部内容、绝不重新转换；编译通过且 submit 才算成功），失败者才删产物 + 转录、退回整章重转换。
  - 样式反馈会话可直接 `read_file {path:"work/chapters/<name>.tex"}` 查看真实提交（不只是工作汇报）。
  - assemble 修复会话新增 `read_file`（AltRoots=项目根，可读原 md/chapters/style）与 `list_source_pages`/`view_source_page`。
  - 删除单文件专用 `WriteFileTool`（与 `WriteWorkFileTool` 重复）。

### Changed

- **档位2 产物纯净（可直接当 markdown 读）**：档位2 输出不含任何 `<!-- DOCVISION-* -->` 注释——矢量图一律嵌入 SVG（`![label](figures/*.svg)`，SVG 缺失退 PNG/PDF 链接），移除了「`insert_image_description` 打开时嵌 latex 代码块」的档位2 分支；text/styled 一律只嵌纯文本；raster 默认嵌原图（该开关默认 false）。矢量回退与 SVG 转换失败在档位2 只写日志与 progress.json，不再就地插注释（档位1 的注释骨架不变，注释也不进 .tex）。
- **checker 反馈改为同一会话三轮 + 兜底重转换**：checker 发现硬性问题后直接反馈给**原来的** convert 会话（上下文仍在，最省 token、修正更准），最多 3 轮；3 轮仍未通过才作废该章 `.tex`、资源目录与转录，用全新会话重转换一次（新增 `maxCheckerRounds`，重转换带 `retry` 标志避免无限递归）。
- **assemble 容错与整树交付**：全书编译失败不再算阶段失败——修复会话在构建树里编译、看报错、迭代，超过 `max_fix_rounds` 只告警；有 PDF 才进入终审会话（核对成品/整理目录/重新构建，submit = 定稿）；交付改为把整棵 build 树（跳过 .aux/.log/.toc/.synctex 等中间文件）复制到 `out/`，目录结构以终审会话实际产出为准，`book.pdf` 是 `main.pdf` 的别名。
- **虚拟工作区挂载表（VFS）**：新增 `internal/latex/vfs.go`——每个会话一个命名空间，`Mount{Name,Dir,Writable}` 挂载表 + `VFS.Resolve(path, write)`。路径语法：`chapters/x.tex`（默认挂载点，通常 `work`）、`project:chapters/x.md`、`/project/chapters/x.md`；越界 `../` 拒绝、只读挂载点写入拒绝、未知挂载点报错并列出可用挂载点。`read_file` 新增 `Mounts` 字段（优先于 Root/AltRoots），回显统一为 `挂载点:路径`；`grep` 新增 `AltRoots`（多根检索，附加根命中加 `label/` 前缀）；`write_file`/`edit_file` 接受 `work:` 前缀、拒绝其它挂载点。终审/修复会话的 grep 现在可同时搜索构建树与项目原始 md。
- **终审会话（全书汇总/整理）**：全书编译成功后不再直接交付，而是总是进入 `finalReview` 会话——构建树 `build/` 同时是样式包基础工作区（cls/sty/manual.md/example.tex 一并复制），工具集与修复会话共享 `bookSessionTools`（read_file 可读构建树 + 项目根、write_file/edit_file/grep/bash、compile {engine:"latexmk"}、view_pdf、view_image、list_fonts、list_source_pages/view_source_page、submit）；它逐页检视成品 PDF（封面/目录/章节顺序与完整性/页码/图表位置与溢出/孤页/overfull box）并做最小 edit_file 修正后重新构建，轮数上限 `latex.compile.max_fix_rounds`。开关 `latex.compile.final_review`（默认 true）；终审未通过只告警，已编译全书照常交付。
- **raster 图档位1 统一解释块**：档位1 的 raster 图在 process 阶段总是生成解释文本（复用 img2text 文本提取，每张一次视觉调用），嵌入格式与 STYLED/VECTOR 同骨架——`<!-- DOCVISION-IMAGE: <label> -->` + `DESCRIBE: <解释>` + `LINK: [image](…)`，无文本时只有 LINK 行；`latex.insert_image_description` 仅控制档位2（开=嵌入 `[Image]( content )`，关=纯 `![image]` 原图引用）

- **档位1 嵌入注释最终骨架（首行闭合 + 字段在外 + `[class]` 链接）**：HTML 注释第一行即闭合 `<!-- DOCVISION-<TYPE>: <描述> -->`，CONTENT/LINK 一律在注释**外面**、各自独立一行；LINK 用链接形式（非图片）且括号前带 class 名：`[styled-text](…)`、`[vector](…)`。STYLED-TEXT = 注释 + CONTENT 原文 + LINK；VECTOR = 注释 + latex 围栏上方 LINK；RASTER 有 AI 描述 = `<!-- DOCVISION-IMAGE: <描述> -->` + 原图 `![image](…)`（图仍是图片形式），无描述保持纯图；fallback 保持原图 + `<!– DOCVISION-ERROR –>`。style/convert 提示词标记说明、embed 测试同步

- **项目字体目录接入编译**：`Compiler` 注入 `TEXINPUTS`/`OSFONTDIR` 指向 `paths.fonts`（默认 `./fonts`）——cls 里按文件名直接引用用户放入的字体文件，无需安装系统字体；latex 运行时自动创建 `fonts/README.md` 说明缺失字体的放置方法与命名约定；移除 `install_font` 工具（下载字体不必要且有版权风险），缺字体改为样式会话在 submit_style 报告与 manual.md 中列出清单、由用户手动下载
- **档位1 嵌入注释统一骨架**：所有机器注释统一为首行 `<!-- DOCVISION-<TYPE>: <描述>` + 字段行（CONTENT:/LINK:）+ 独立闭合行 ` -->`；矢量图注释为 `DOCVISION-VECTOR: <label>` + `LINK: ![](原图)`（替代单行 ORIG-IMAGE），与 STYLED-TEXT 同构

- **会话工具大升级（增量编辑 / 检索 / PDF 查看 / 可配 bash）**：新工具文件 `tools_work.go`——`edit_file`（任意文本文件精确 find/replace，支持 `replace_all` 与 `append:true` 追加；回答"追加能否用编辑实现"：能）；`grep`（工作区递归检索，返回行号+相对路径）；`WorkBashTool`（**timeout 由 AI 参数决定**默认 30s 上限 300s，cwd=会话工作区根，输出上限 5000 字符）；`view_pdf`（按页渲染工作区 PDF，裁剪/放大与 view_image 同一套参数，**始终从 PDF 重渲染**因此放大清晰，返回 page N/M）。接入：tikz（write_file/edit_file/grep + 持久工作区）、style（edit_file/grep/view_pdf）、convert（edit_file 仅自己的 .tex/grep 全项目/view_pdf 编译草稿）、chapters（buffer.md 工作记忆 + read_file + bash）
- **submit 按文件路径引用（process）**：tikz 的 submit 优先接受 `{path:"figure.tex"}`——模型先把最终代码 write_file 进工作区再按路径提交，不再强制整段重输出代码（省 token）
- **process 持久工作区**：tikz 工作区改为 `sessions/vector_<图>.work`（不处理完不删除，中断后下次续上；成功提交后连同转录一起清理）
- **档位1 嵌入携带原图链接**：latex 代码块前新增机器注释 `<!-- DOCVISION-ORIG-IMAGE: ![](images/…) -->` 指向原图，转换会话可回看源图/定位原始页面，注释不得进入 .tex
- **编译与预览分离信息**：compile/compile_preview 成功时返回产物 PDF 文件名与页数并提示 view_pdf 检视；失败仍返回错误日志
- **chapters 工作记忆缓冲区**：章节划分会话获得 `buffer.md`（edit_file 增量写入分析结论，大部头跨轮保留）+ read_file + 可配超时 bash；划分会话也接入 JSONL 转录（中断续上，完成清理）
- **GLM 官方推荐参数**：`models.<name>.tool_stream`（工具参数随流返回，顶层字段，SSE 组装器原生兼容）；`thinking.clear_thinking` 直接写进 thinking map；运行配置 glm-5.3-flash 按推荐设 temperature 1、`thinking: {type: enabled, clear_thinking: false}`、`tool_stream: true`；debug 请求摘要增加 `tool_stream=` / `clear_thinking=`
- **思维链回传（缓存修复）**：chatstream 组装 assistant 消息时保留 `reasoning_content`，ChatMessage 逐字节存回历史并随请求重发——GLM 保留式思考要求完整回传，且请求前缀逐字节一致是 prompt cache 命中的前提（此前思维链每轮丢弃）

- **会话 JSONL 转录与断点续传**：新增 `internal/session/transcript.go`——每条会话消息实时追加为一行 JSON（append-only，一行一条；图片 base64 不入转录，落 `media/` 目录以 `file://media/<hash>.<ext>` 引用，加载时还原为 data URL）。三处会话接入：tikz 矢量会话（`<outDir>/sessions/vector_<图>.jsonl`）、样式会话（`work/style_session.jsonl`，替代旧单文件 `.json`，旧文件自动迁移）、单章转换会话（`work/sessions/convert_<章>.jsonl`）。进程被杀 / 网络断连后，下次运行自动从转录恢复完整上下文续跑（含图片），不再从零重烧 token；章节产物已存在时清理对应转录
- **未提交提醒（tikz）**：作图会话结束时若模型尚未调用 submit（此前有会话未提交就结束的先例），自动补发一次 "You have NOT called submit yet…" 提醒并给追加轮次
- **档位1 版面重排规则**：转换系统提示新增 Layout reconstruction 段——原始 PDF 中横向排列/组合形式的内容（如一行 6 张 venn 图）在线性 markdown 中退化为连续图片引用；转换会话须对连续多图/图文交替区段 doc_search + view_page 查原版面，用 subfigure/minipage 等复现横排/网格，不再线性堆叠
- **编译预览可放大细看（复用 view_image）**：`compile_preview` 的每张预览图都留存为会话内的 `preview-<n>.png`（并保存对应 `preview-<n>.pdf`），`view_image {path:"preview.png"}` 取最新、`preview-<n>.png` 取历史版本，配合 `left/top/right/bottom` 百分比裁剪与 `zoom` 目标宽度即可细看小字号标签/箭头/重叠；`zoom` 时**直接从 PDF 以更高分辨率重渲染**（pdftoppm `-scale-to-x`，上限 6000px），而不是把已有像素拉大，因此放大是真清晰。未编译、名称写错、序号越界都会返回明确提示
- **classify 起始进度行**：`[classify 0/N] 0.00% (failed: 0)` 在进入分类阶段立即打印（此前要等第一张分类完成才出现），与 process 阶段一致
- **日志等级 info / debug / trace**：`options.log_level` 新增 `trace`，命令新增 `--trace`。debug 只保留每轮请求/响应摘要、提示词、工具调用与**最终接收内容**；流式分片进展行等噪音降到 trace。`options.log_level` 取值错误由 `setup` 校验
- **PDF→SVG 多后端回退**：`dvisvgm → pdftocairo → mutool → inkscape` 依次尝试，成功后记录所用后端；全部失败时 ERROR 日志列出每个后端的具体原因（不再是裸 exit status）
- **LaTeX 编译警告反馈**：编译结果新增 `Warnings` / `WarningCount` / `WarningSummary`（Overfull/Underfull box、LaTeX/Package/Class Warning，去重并限长），随 compile_preview / compile / recompile 的工具结果一并返回给 AI；debug 日志新增 `[compile:<preview|chapter|book>] OK|FAILED (耗时) warnings=N` + 警告清单 + 失败时给 AI 的错误原文（完整编译日志仍然不进上下文/日志）
- **Mermaid / LaTeX 校验结果进 debug 日志**：`[validate:mermaid]` / `[validate:latex]` 记录 has_blocks / valid / available / 精简后的错误
- **作图禁止重叠/拥挤**：latex 作图提示词新增硬规则与提交前检查项（标签不得压线/互相遮挡、节点不得重叠或越界，空间不足就整体放大/增间距/按比例缩小字号），img2text 提示词同步
- **图片工具可直接用文件名**：`view_image` 以**当前文档的图片目录为根**直接拼接（`foo.jpg`、`subject/foo.jpg`、`images/subject/foo.jpg` 都落到同一路径），**不做任何搜索**——找不到就报错（说明名字写错了），避免跨文档误命中；`image_context` 同样接受裸文件名（按唯一基名匹配，歧义报错）。提示词只保留一句"直接用文件名"，删掉冗余的路径说明与重复的重叠规则（重叠要求只留在 Rules 一处，省 token）

### Fixed

- **上次失败的图片重跑被整个跳过**：档位1 的 images 阶段此前有 phase 级 done 标记（`progress.json` 的 `images: "done"`），重跑全书时该阶段直接跳过——上次 API 失败留下的 fallback（可重试）图片永远不再重试，控制台看起来"直接完成"。现在 images 阶段不再做 phase 级跳过：`RunImages` 自身就是增量的（done 跳过、fallback 重试、未分类补跑），phase 标记仅作展示
- **进度行 skip 计数语义模糊**：`[classify]/[process]/img2text` 进度行去掉 `skip: N`；断点续传时进度直接从已完成数起跳（如 `[process 5/13] 38.46%`，与 `Already done: 5` 呼应），运行中的任务数仍由 `running: N` 实时显示
- **多工具调用触发 HTTP 400**：返回图片的工具（view_image/compile_preview 等）此前把"图片 user 轮"插在多个 tool 响应之间，违反 OpenAI 协议（tool_calls 之后必须紧跟对应的 tool 消息），导致会话直接失败（`insufficient tool messages following tool_calls message`）。现在先连续追加全部 tool 响应，图片轮统一放在工具块之后；工具预算耗尽后模型仍返回 tool_calls 时也会执行并补齐响应
- **流式回退误触发**：HTTP 400 不再视为"不支持流式"（畸形对话同样是 400），只在错误提到 stream/unsupported 或 404/405/415/422 时回退一次非流式，避免把重试预算浪费在同一个坏请求上
- **档位2 文本图嵌入 AI 描述**：text 类图片此前走 img2text 的通用"描述图片"提示词，会把"该图像是一个标题或图标…"这类描述写进 markdown（即使 `insert_image_description` 关闭）。现在改用专用"只提取可见文本"提示词（公式→LaTeX 数学、表格→Markdown 表格、无文字→`[NO_TEXT]` 并保留原图），描述不再进入正文
- **样式化文本图按档位区分**：档位2 恢复"直接嵌入原文"（不保留样式、不加任何标记，与 v1.4 行为一致）；只有档位1 用一条 HTML 注释携带样式与原文：
  `<!-- DOCVISION-STYLED-TEXT: <样式说明>` / `CONTENT: <图片原文>` / `LINK: ![styled-text](images/…)` / ` -->`，转换会话按手册重排 CONTENT 或直接 includegraphics LINK，整条注释不得进入 .tex
- **日志分析工具调用数恒为 0**：`Session.ToolCalls` 此前从未赋值；现在解析会话引擎的 `[tool:<name>] ok|error` 与 img2text 的 `[ToolCall]` 两种格式，并按线程归属到对应会话
- **错误分类 unknown**：新增 `SESSION_*` 归类（`SESSION_API_ERROR`→api_error，其余→session_error）
- **进度摘要出现负数**：进度条目多于当前 md 引用时（改名/删除的书目、旧格式条目）剩余按 0 计并加注说明
- **日志措辞易误解**：交互式编译失败记 `[compile:preview] compile error (0.8s) → 已返回 AI 修复`（此前写 FAILED，容易被当成会话失败）；工具调用错误记 `[tool:x] error (已返回模型，会话继续)`
- **矢量图 SVG 转换必然失败**：dvisvgm 3.6 处理 PDF 需要 Ghostscript < 10.01 或 mutool，而本机 Ghostscript 10.05.1 不受支持（`ERROR: To process PDF files, either Ghostscript < 10.01.0 or mutool is required`），导致所有矢量图降级为 PNG/PDF 链接并记 ERROR；现由 pdftocairo 等后端兜底
- **`image_locate` 与 `image_context` 功能重叠**：删除 `image_locate`（其行号/前后引用/±行差信息并入 `image_context` 输出），作图会话只保留 `image_context`（文本上下文）与 `view_image`（看像素）两个职责清晰的工具

### Added（前一轮）

- **流式接收（默认开启）**：新增 `models.<name>.stream`（不写即 true），AI 请求改用 SSE 流式接收（`internal/chatstream` 统一组装 content/reasoning/tool_calls/usage），长思考/长输出期间持续有进展不再静默；流式模式自动带 `stream_options.include_usage`；厂商不支持流式（HTTP 400/404/405/415/422 或提示 stream 不支持）时自动回退一次非流式请求
- **流式超时语义**：非流式仍是 `api_connect_timeout + api_timeout`（默认 60+400s）整次请求上限；流式不再设总上限，改用 `models.<name>.api_stream_idle_timeout`（默认取 `api_timeout`）判定"两个数据块之间的最大间隔"，长但不间断的流不会被杀
- **思考控制配置**：`models.<name>.thinking`（顶层 `{type: enabled|disabled}`，GLM-4.5+/DeepSeek）与 `models.<name>.reasoning_effort`（顶层 max/xhigh/high/medium/low/minimal/none，GLM-5.2+），随 `request_body` 之后合并到请求体顶层，显式配置优先；`setup` 严格校验取值
- **调试日志增强**：每次请求/响应记录实际生效的模型、stream、max_tokens、temperature、thinking、reasoning_effort、消息数/上下文估算、耗时、finish_reason、输出与思维链字符数、provider token 用量（含 `reasoning_tokens`）；流式期间每 10s 输出进展行；img2text / verify 也支持 `--debug` 与 `options.log_level: debug`（此前只有 latex 命令生效）
- **`models.<name>.max_tokens` / `temperature` 兜底生效**：会话或单次调用未指定时使用该模型的取值（此前这两个键被继承但无人读取）
- 会话单次最大输出说明：`latex.sessions.*.max_tokens` 为单次请求 `max_tokens`（不含厂商单独计费的思维链预算），style 会话内置默认 32768（不再由 book.go 硬编码覆盖用户配置）

### Fixed

- `view_image` 首次调用失败：图片引用按 markdown 原文 `images/<主题>/x.jpg` 传入，而工具根目录通常是 images 目录本身，导致"文件不存在"；现在两种形式（带/不带 `images/` 前缀）都能解析，错误信息也提示可省略前缀
- `latex.sessions.checker` 的调优此前被忽略（`_ = LatexSession("checker")`）：现在真正生效，且未配置字段继承 `convert` 块；checker 请求也不再发送 `max_tokens: 0`
- 配置版本 5→6（新增 models 层 stream/thinking/reasoning_effort/api_stream_idle_timeout 等键）

- **作图不确定兜底规则**：latex 作图提示词新增 Core rule（不确定时先用 image_context/view_image 裁剪放大核实，仍不确定则以 `% [?]` 注释标记并只画确定内容，禁止臆造）；submit 时程序检测 `[?]` 标记并记警告（建议人工复核）
- **版本规范化**：版本号进入 1.5.0 开发线（当前 v1.5.0-beta.1），历史 v1.3.0/v1.4.0 重打为 beta 标签以示测试态；后续开发构建自动显示 v1.5.0-beta.1-N-gxxxx
- **img2text 嵌入按类型细分**：math/formula 用 markdown 数学定界符包裹（单行短式 `$…$`，整块/含环境 `$$…$$`），code 用带语言标注的围栏代码块，table 保持 Markdown/HTML 表格原样，latex（含 TikZ）统一 ```latex 围栏（已围栏的直通、裸 TikZ 补围栏），其余视觉类型照旧 `[Image]( 描述 )`；提示词同步要求带语言标注的代码块
- **档位1 样式化文本图处理**：classify 新增 `styled`/`style_note` 标记（艺术字/彩色/装饰等纯文本无法表达的视觉样式）；嵌入时打 `<!-- DOCVISION-STYLED-TEXT: … -->` 记号并保留原图链接与提取文本，转换会话拿到 cls 后按手册重排或保留原图，标记注释不得进入 .tex
- **全书图形风格统一（方案A，转换期二次加工）**：样式手册固定新增 `## Vector figure style` section（调色板 \definecolor、节点/箭头/线宽、caption 约定）；转换提示词允许（并要求编译验证）按手册对内嵌 latex 围栏图代码二次加工，跨章一致性由手册 section + 逐章 checker 保障
- **档位1 转换前置检查（preflight）**：进入并发转换前校验 style cls/manual.md 存在、source 图片全部处理完毕（done/fallback），有问题直接停止并提示先补图片处理
- **latex.chapter_granularity（默认 small）**：档位1 章节拆分粒度可选 small（按小节拆分为自洽单元）/ large（整章单文件）；配置版本 4→5
- **进度条目自动迁移**：旧版本写成的 `done + original_kept` 回退条目加载时迁移为 `fallback`，下次运行自动重试矢量转换；dvisvgm 失败（svg_failed）且 figure PDF 尚存的条目直接补跑 dvisvgm（无需重跑 AI 会话）

- **latex 会话日志分析**：档位2 图片处理逐图输出 ▶ START / ✓ DONE [IMG_TYPE: …] / ✗ FAILED 标记（与 img2text 同格式），`docvision latex` 结束后的日志分析可正常解析本次会话数与耗时；进度摘要改读 latex 输出目录（档位1: latex_project/source，档位2: latex.output_dir）的 progress_items（总计/已完成/回退/未完成），不再误读 img2text 历史进度
- **矢量回退可重试 + ERROR 标注**：TikZ/SVG 转换失败按 ERROR 记录，图片保留原图并就地标注 `<!-- DOCVISION-ERROR: … -->`（可搜索定位）；回退条目状态记为 `fallback`，下次运行自动重试（已完成的保持不动）；进度行区分为 成功/失败/回退；dvisvgm 失败降级 PNG/PDF 链接同样 ERROR + 标注
- **逐章工作汇报**：转换会话 submit 必须携带 `report`（pass/issues + 问题 + 建议），固定格式实时写入 `latex_project/work/reports/<章>.md`；转换提示词要求提交前用 doc_search/view_page 抽查原 PDF 核对 cls/手册符合性
- **cls/手册反馈回路**：多数章节汇报样式问题时打回原样式会话修正（样式会话上下文实时持久化到 `latex_project/work/style_session.json`，复用同一上下文不开新会话）；样式包更新并试编译通过后丢弃全部章节 .tex，用全新上下文重新并发转换（最多打回 2 轮）
- `session.Session.SetMessages`：恢复持久化会话上下文

### Fixed
- latex 档位2 重试进度：fallback 重试条目不再被 classify 阶段虚报计数（已带分类结果的直接进 process）；process 阶段开始时先打印 `[process 0/N]` 起始进度行，长会话处理期间进度可见
- latex 前置 Organize Files 汇总（[4/4]）按选中书目过滤：单书 latex 流程不再罗列全树全部 md 与图片目录
- processPhase 的 panic 恢复原为普通语句（recover 不生效，会击穿整个进程），改为 defer 内调用

- **档位1 原始文档检索**：转换会话新增只读 `doc_search`（MinerU content_list 加工的块索引，关键词/图片名/页码检索，返回全局页号与 bbox）并可复用 `view_page` 按需渲染原始 PDF 页（与样式阶段共享缓存）；MinerU 产物缺失时自动降级

- `tools:` 独立配置块（v3）：get_more_context / image_context 的上下文参数与 mermaid/tikz 校验参数从 img2text/options 迁出，全流程共用；新增 `image_locate` 工具（返回前后图片引用的行号与 ±行数差，轻量定位后再按需扩展）

### Changed

- img2text 作图规则放宽：不再限定 TikZ——Mermaid 处理其擅长的图型，其余（几何、函数/坐标图、复杂表格、混合结构）可使用 TikZ/pgfplots/tabular 等任意 LaTeX 方式
- `--version` 输出版权（绫袅 LingNc）与仓库链接；Makefile VERSION 自动取 git describe（不再固定 dev）
- 构建产物 `logs/` 移出版本库并加入 .gitignore

- 水印工作记忆：remove_watermark 开启时，流程最开始做一次性检测（view_page 全览页渲染 + 全文档重复图片引用统计 + markdown 采样），AI 判定水印文本形态与被裁剪成图的水印/广告图引用，结果缓存为 latex_project/watermark_memory.json（两档位共享）；之后作为小工作记忆注入 classify/img2text 文本提取/作图/style/convert/checker 全部会话，水印图片引用在分类阶段直接预剔除（absorbed），不再重复处理

- 档位2 跨页图表拼接：作图会话新增 `image_context`（查任意图片的上下文与前后图片引用）与 `view_image`（查看图片）工具；提示词引导识别跨页续片（重复表头/"续表"/边缘截断等），一次绘制合并图并在 submit 声明 `merges`；被吸收的续片引用在重建 markdown 时自动删除且不再重复处理

- latex 档位2 控制台进度：默认每个阶段显示一行实时进度（classify/process），逐图明细只写日志文件；`--verbose` 恢复详细输出

- `latex.remove_watermark`（默认 false）：开启后样式分析 AI 会在使用手册中标注水印模式并指示排除，转换 AI 跳过水印内容，核对 AI 不把水印缺失报为问题

- 档位1 样式分析虚拟工作区（`latex_project/work/style/`）：write_file 增量起草，submit_style 可引用文件而非全量重发
- 字体管理：`paths.fonts` 目录 + `list_fonts`/`install_font` 工具（样式分析与终审修复会话可用），缺失字体标注替换方法
- 每章核对 AI：`latex.checker_model`（默认用 convert_model，可为小文本模型），逐章比对产物与原 md，问题回炉一轮，遗留记录 `.checker` 备注
- img2text 支持 TikZ：提示词新增 tikz 类型，`options.tikz_validation`/`options.tikz_engine` 用 LaTeX 编译校验（失败自动回炉修复）

### Changed

- **配置 v2（不兼容清理）**：新增 `config_version: 2`，版本不符时启动警告并要求参考模板更新；移除全部向后兼容层——顶层 `ai:` 块、`resolveAIReference`、`options.*`/`img2text.*` 中的 api_timeout/api_connect_timeout/api_max_retries/rate_limit_retries（统一收敛到 models 层）、废弃的 `latex.output_dir`/`latex.project_dir`；`models.text` 成为强制基础条目（setup 校验必填）
- checker 会话不再继承 convert 的会话配置（独立可调）；模板 `checker_model: "checker"` + `models.checker: Qwen/Qwen3.6-27B`（小文本模型）

### Changed

- **档位1 图片内嵌**：全书流程 images 阶段改用 inline 模式——矢量图以 tikz 代码块直接内嵌进 markdown，转换 AI 将代码原样粘贴进 .tex（不再生成/引用 figures/*.pdf 资源）；raster 保留原图引用由 includegraphics 处理；SVG 转换仅在档位2 执行

- API 请求控制参数（api_timeout/api_connect_timeout/api_max_retries/rate_limit_retries）从 img2text/options 迁移到 models 层：任何 models 条目可设置，留空继承 models.text，最终回退代码内置默认（400s/60s/3/100）；旧位置仍解析兼容。session 会话客户端与 img2text 客户端统一使用同一套参数，重试均带指数退避
- rate_limit_retries 默认从 0（无限+代码安全上限 100）改为直接取安全上限值 100
- 结构化 JSON 输出请求（图片分类、章节核对、校验报告）统一附加 response_format={type: json_object}，保证返回一定是 JSON

- latex md 参数不再硬拒绝：output/ 中的 md 直接选用，其他位置的 md 复制进 files/ 后整理进 output/（用户路径里带 output 不会误伤）
- img2text 嵌入前自动留存原版 markdown 到 `finally/progress_items/<文件名>/original.md`（每文件仅首次），嵌入结果可随时还原

- img2text 输出嵌入格式按类型分流：纯文本/数学公式/表格/代码直接嵌入正文（无包装标记），mermaid/tikz 代码块直接嵌入，其余视觉类型用 `[Image]( 描述 )`——不再使用 `<!-- IMG -->`/`[AI]` 包装（**注意：v1.3.0 及之前的 finally/ 输出格式不变，仅新生成内容使用新格式**）
- `docvision latex` 自助化：无参数即从 `files/` 跑全流程（自动跳过已处理）；单文件传 files/ 中的 PDF/DOCX；自备 md 也放 files/；禁止拿 output/ 产物当输入；结束后自动接日志分析
- 移除 `workflow --step latex/verify`（latex 本身就是完整工作流）
- 档位1 原始页面改为 `view_page` 工具**按需渲染**（AI 看哪页渲染哪页并缓存），不再全量预渲染
- 会话工具轮数：模板默认 128；显式 `0` = 真正不限制（无安全上限）
- `paths:` 新增 latex_output/latex_project/fonts；img2text 独立配置块（model 用 models: 代号）

- `docvision latex` 自助化：位置参数为 PDF/DOCX/目录时自动补跑前置流程（split→mineru→organize，隔离于 ~/.docvision/jobs 作业目录）再进入 LaTeX，无需先手动跑 workflow；md 名字参数直接处理 output/ 中对应文件；不带参数批量处理全部 md
- 档位1 样式分析改用 MinerU 保留的原始扫描页面（`*_origin.pdf` 自动渲染为整页 PNG 并缓存到 latex_project/pages/），新增 list_pages/view（裁剪+放大）工具；原始 PDF 缺失时退化为提取图片分析并日志提示

- 配置精简：移除模板中的顶层 `ai:` 块，所有模型统一在 `models:` 注册表配置；`models.text` 成为 img2text 等基础流程的默认模型（必填），其余条目空字段自动继承；旧配置的 `ai:` 块（含 ai.model 引用注册表名）保持完全兼容

- 配置整合：`ai.model` 可直接引用 `models:` 注册表名（如 `text`），base_url/api_key/request_body 从注册表继承，凭据只需维护一份；img2text 等基础流程同样生效
- 会话工具轮数 `max_tool_rounds` 默认 0 = 不限制（代码内安全上限兜底）
- `docvision latex` 支持位置参数只处理指定 markdown 文件；不带参数则批量处理 `output/` 全部文件
- `docvision workflow --step latex`：自动前置 split→mineru→organize 再进入 LaTeX 流程并接日志分析；`--step verify` 同理（受 `verify.enabled` 控制）
- analyze/logfind/rangesel 兼容 `latex_*.log`：`docvision analyze` 可直接分析 LaTeX 管线日志
- `docvision init` 生成模板后自动检查环境（xelatex/pdftoppm/mmdc）并给出安装提示

### Added

#### LaTeX 输出（两档位）
- `docvision latex`：档位2（默认）图片矢量化——分类 AI 逐图标记 text/vector/raster；text 纯内容嵌入文本流（去除 [AI]/[IMG_TYPE] 标记）；vector 由作图 AI 会话写 TikZ，自动编译+栅格化 PNG 视觉核对+确认提交，编译 PDF 以矢量图嵌入；raster 保留原图（可选嵌入 AI 解释文本），矢量失败自动回退并告警
- `docvision latex --level 1`：全书 LaTeX——样式分析 AI（图像裁剪放大/读 md 工具）产出 cls+结构化使用手册+案例并自动试编译；章节划分 AI（grep + 最小 bash 沙箱，虚拟单文件系统）行号划分并全覆盖校验；转换 AI 并发逐章转 .tex（虚拟文件目录，只读他人/只写自己）；汇总多文件 .tex 编译全书 PDF（修复会话兜底）+ 单文件 standalone.tex
- AI 会话基础设施 `internal/session`：可复用多轮会话引擎，可分离工具注册（compile_preview/submit/grep/bash 沙箱/受限读写），可配置上下文窗口（默认 128K，支持 64K/256K 等），达到阈值自动 AI 压缩会话历史（保留关键决策与成果，丢弃草稿/工具噪音）
- 模型注册表 `models:`：每个专用 AI（classifier/drawing/style/chapter/convert/verifier）独立配置 base_url/api_key/model/request_body，空字段继承顶层 ai
- `docvision verify`：AI 核对每张图与嵌入内容（含 TikZ 渲染预览对比），输出问题与修改意见报告（`verify.enabled` 默认关闭，先上日程）
- 断点续传：档位2逐图进度（progress_items），档位1逐阶段进度（progress.json）
- latex 会话日志独立落盘 logs/latex_*.log

### Fixed
- `--config` 传入相对子目录路径时 chdir 后解析错位（改用绝对路径）


## [v1.1.0] - 2026-09-03

### Added
- `install` / `uninstall`：一键安装到 `/usr/local/bin`（无权限回退 `~/.local/bin`）并初始化 `~/.docvision/`；卸载保留配置
- `setup`：自动选择编辑器（`$DOCVISION_EDITOR`/`$EDITOR` 优先，回退 vim → nano → vi）编辑生效配置；保存时严格校验（YAML 语法、未知配置项、类型、必填项、占位符），不通过可回车重编或 q 退出
- 配置查找顺序：`--config` > 当前目录 `config.yaml` > `~/.docvision/config.yaml`（缺失自动从模板创建），任意目录可直接使用
- `workflow <path>` 临时模式：处理任意位置的 PDF/DOCX 文件或目录，中间产物存于 `~/.docvision/jobs/<名称>-<时间戳>/`，仅将最终 `.md` 输出到源文件所在目录

### Changed
- GitHub Actions release 工作流支持 `workflow_dispatch` 手动补发任意标签

## [v1.0.1] - 2026-09-03

### Added
- `analyze -r/--round`：按轮次选择日志（0=最新，1=上一次…，支持 `1-3` 范围）
- `analyze -l/--last`：按时间范围选择日志（`+2d`、`2026Y9M1D-2026Y9M2D`、`2026-09-01_15:30-` 等）
- analyze 默认输出“本轮写入 finally 的文件及成功率统计”

### Fixed
- 日志解析改为按“结束时间 − 耗时 ≈ 开始时间”匹配结果归属，不再依赖线程号顺序：旧日志中线程号复用导致的假“未完成”统计全部纠正（实测 1958 任务：1941 成功 / 17 失败 / 0 未完成）
- img2text 线程号改为独占分配（tid 池），修复任务复用进行中的 tid 导致日志交错
- AI 工具调用消息的 tool_call 类型规范化，修复部分 API 网关报 HTTP 400 (Input should be 'function')
- `finally` 最终文件跳过内容无变化的重复写入


## [v1.0.0] - 2026-08-11

### Added
- 新增"大变"里程碑：split 后的源文件归档到 `done/` 目录，整体工作流闭环
- 支持在同一个工具会话内连续修复 Mermaid，保留上下文并允许后续继续调用工具
- Mermaid 修复次数支持 `0` 表示无限制（受代码内安全上限保护）

### Changed
- Mermaid 默认修复次数从 1 提升到 3
- Mermaid 修复提示词清洗报错栈：剔除 puppeteer 内部堆栈以减少噪音，超长堆栈截断到 16KB

### Fixed
- Mermaid 错误分类与修复提示词加固：常见语法/渲染错误分类更精准，提示词更明确修复方向
- organize 图片路径重写改为严格幂等，修复包含特殊字符的 subject 目录图片缺失问题

### Refactored
- 指纹边车文件后缀从 `.md.fp` 改为 `.fp`，避免与 Markdown 文件混淆

## [v0.3.0] - 2026-08-10

### Added
- DOCX 分割清单（manifest）持久化快速路径：未变化的输入直接复用历史清单，跳过重复计算
- 处理日志与最终输出分离：`img2text` 主日志和错误日志落到独立的 `logs/` 目录，便于归档和分析

### Fixed
- img2text 进度恢复去重：断点续传场景下进度条目不再重复写入
- 主日志选择逻辑统一：analyze / splitlog 在多个候选日志中按规则稳定选择主日志

### Changed
- Python 遗留实现归档到 `legacy/python/`，仅作历史参考，不再维护
- 文档对齐当前 Go 实现：README / 配置示例与 Go 行为同步，移除过时说明

## [v0.2.0] - 2026-08-10

### Added
- Mermaid 输出验证与修复：识别无效 Mermaid 块并触发修复流程
- `docvision init` 时询问是否通过 npm 安装 Mermaid CLI，便于开箱启用 Mermaid 验证
- 任务识别与日志分析优化建议：在分析阶段给出更具操作性的改进提示
- PDF 分割清单（manifest）持久化快速路径：未变化的输入直接复用历史清单，跳过重复计算
- organize 步骤支持增量、幂等的整理运行：重复执行不会产生重复产物

### Fixed
- MinerU 上传超时与路径处理加固：上传空闲超时更稳定，长路径 / 异常路径场景下不再中断
- analyze 工作流使用当前 img2text 日志，而非陈旧日志

## [v0.1.2] - 2026-06-24

### Added
- 工作流步骤开始时显示初始进度提示，提升用户体验

### Fixed
- 进度显示：在工作流步骤开始时显示初始进度，避免用户长时间看不到任何输出

## [v0.1.1] - 2026-06-23

### Added
- DOCX 分割支持：使用与 PDF 相同的页数和大小限制
- DOCX 转 PDF 再分割：通过 LibreOffice 转换为 PDF 获取精确页数后再按页分割
- 无需分割时直接复制 DOCX 文件，避免不必要的 XML 操作

### Fixed
- DOCX 页数检测：从段落启发式改为读取 `docProps/app.xml` 中的实际页数
- DOCX 段落启发式页数估算：从每页 20 段降低为 10 段，更贴近实际排版
- PDF 文件查找：空目录警告改为打印日志并返回 nil，不再中断流程

## [v0.1.0] - 2026-06-04

### Added

#### Go 重写
- 全新 Go 实现，与 Python 版本功能完全一致
- 单二进制文件，无运行时依赖
- 交叉编译支持：Linux/macOS/Windows（amd64 + arm64）
- 8 个子命令：`workflow`、`split`、`mineru`、`organize`、`img2text`、`analyze`、`splitlog`、`init`
- `docvision init` 生成配置模板（`default.yaml` 嵌入二进制）
- Makefile：`make build`、`make release`、`make test`、`make clean`
- DOCX 分割支持：按段落分组，使用与 PDF 相同的页数和大小限制

#### 核心功能
- PDF 分割：按页数 + 文件大小二进制搜索，跳过已分割文件，`--force` 强制重分
- MinerU API：分块上传、空闲超时检测、指数退避轮询、任务断点续传、并发处理
- 文件整理：自动合并分片、按主题组织图片子目录、重写 Markdown 图片路径
- AI 图片转文本：OpenAI 兼容接口、工具调用增量上下文扩展、格式自修复、并发工作池
- 日志分析：状态机解析、8 类错误分类、百分位统计、CSV 导出、进度摘要
- 配置：`request_body` 字段直接注入 API 请求体，支持不同厂商的思考模式配置

#### 工作流模式
- `docvision workflow` 串联全部 5 步，`--step` 单步执行
- img2text 步骤安静模式：控制台仅显示进度百分比，完整日志写入文件

#### CI/CD
- GitHub Actions：推送 `v*.0` 标签自动测试、交叉编译、创建 Release

### Changed
- 配置格式：`ai.enable_thinking` 改为 `ai.request_body`，支持通用请求体注入
- Python `img2text.py` 同步支持新 `request_body` 配置格式（向后兼容 `enable_thinking`）
