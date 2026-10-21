# DocVision 批次历史（AGENTS.md 的细节档案）

本文件是 `AGENTS.md` 的**细节档案**：AGENTS.md 只保留"现在是什么状态 + 规则"，
这里存放**第一批～第二十七批的原始记录**（每次改了什么都写了，含真实缺陷、
证据数字与设计理由）。需要查"为什么这么设计""某次事故的根因"时读这里；
日常开发只读 AGENTS.md 即可。

维护约定：新增一批就**追加**一节（`v1.5+（第 N 批）`），并从 AGENTS.md 的
"现状要点"里更新受影响的条目；AGENTS.md 不复制批次细节。

---

## 项目总览（早期原文，v1.2～v1.5 各批次前后累积写成）

- **DocVision（本目录主体）**：PDF→Markdown 自动化工作流（Go）。v1.2 起新增 LaTeX 输出两档位（`docvision latex`，图片矢量化 / 全书 LaTeX）、AI 会话基础设施（`go/internal/session`：可分离工具、上下文窗口与自动压缩）、模型注册表（`config.yaml models:`）、AI 核对（`verify.enabled` 默认关闭）。v1.3+ latex 自带完整工作流（files/ 输入 → 自动跳过前置 → 按档位处理 → 日志分析；原始扫描页 view_page 按需渲染；样式虚拟工作区；fonts 字体管理；每章 checker 小模型核对；转换会话只读检索原始文档（doc_search 块索引 + view_page 原页渲染，缓存共享）），workflow 已不再包含 latex/verify 步骤。v1.4 起 img2text 嵌入按类型分流（文本/公式/表格直接嵌入，mermaid/tikz 代码块，视觉类型 [Image]( 描述 )），不再使用 <!-- IMG -->/[AI] 包装。v1.4+ latex 日志带 img2text 同格式 ▶ START/✓ DONE/✗ FAILED 会话标记（analyze/splitlog 通用）；矢量回退记 ERROR 并在 markdown 就地 `<!-- DOCVISION-ERROR -->` 标注、状态 fallback 下次自动重试；档位1 转换会话 submit 必带工作汇报（实时写 work/reports/<章>.md，固定格式），多数章节报 cls/手册问题时打回原样式会话（上下文持久化于 work/style_session.json）修正后全新上下文重新并发转换（上限 2 轮）。v1.4+ img2text 嵌入进一步按类型细分（math `$`/`$$`、code 带语言围栏、table 原样、latex ```latex 围栏）；档位1 classify 标记 styled 样式化文本图（嵌入打 DOCVISION-STYLED-TEXT 记号保留原图+文本，转换期按手册重排），样式手册固定 `## Vector figure style` section、转换会话按其对图代码二次加工保证全书图形风格统一；转换前置 preflight 检查（cls/manual.md/图片全部完成）；`latex.chapter_granularity`（默认 small）控制章节拆分粒度；旧版回退进度条目自动迁移为可重试 fallback。v1.5+ AI 请求默认走 SSE 流式接收（`models.<name>.stream`，不写即 true；`internal/chatstream` 统一组装 content/reasoning/tool_calls/usage，流式无总超时、用 `api_stream_idle_timeout` 判定静默卡死，厂商不支持时自动回退一次非流式）；思考控制为请求体**顶层**字段 `models.<name>.thinking`（`{type: enabled|disabled}`）与 `models.<name>.reasoning_effort`（max/xhigh/high/medium/low/minimal/none），不要写进 `request_body.extra_body`（那是 Python SDK 概念，wire 上会被忽略）；debug 模式（`--debug` 或 `options.log_level: debug`，img2text/verify/latex 均生效）记录每轮请求的模型/stream/max_tokens/temperature/thinking/reasoning_effort、耗时、finish_reason、输出与思维链字符数、token 用量（含 reasoning_tokens）及流式进展行；`latex.sessions.checker` 未配置字段继承 `sessions.convert`（0=继承、-1=无限工具轮），style 会话内置 max_tokens 默认 32768；`view_image` 同时接受带/不带 `images/` 前缀的引用。开发约定：分步开发并适时提交 git；新增配置项必须同步 `config.go` 默认值、`default.yaml`/`config.example.yaml` 模板与 `setup` 严格校验。v1.5+（第二批）：日志分三级 `options.log_level: info|debug|trace`（`--debug`/`--trace` 覆盖；debug 记请求响应摘要/提示词/工具调用/最终接收内容/编译结果与警告/Mermaid 与 LaTeX 校验结论，trace 才记流式分片进展行，见 `logger.LevelInfo/Debug/Trace`）；PDF→SVG 走 `dvisvgm → pdftocairo → mutool → inkscape` 回退链（`ConvertPDFToSVG`，本机 dvisvgm 3.6 因 Ghostscript 10.05.1 不受支持必然失败，实际由 pdftocairo 兜底），全部失败才 ERROR + markdown 标注；`CompileResult.Warnings/WarningCount/WarningSummary` 把编译警告随工具结果回给 AI（完整编译日志不入日志/上下文）；作图提示词含"禁止重叠/拥挤"硬规则；图片工具职责分离（`image_context`=文本上下文+前后引用，`view_image`=看像素+裁剪放大，已删除重叠的 `image_locate`）；`view_image` 以会话文档图片目录（`ViewImageTool.Subject`）为根**直接拼接**，不做任何搜索、找不到即报错（AI 直接给 md 中的图片名/引用即可，路径解析是软件侧的事）；提示词只在 Rules 处写一次"禁止重叠/拥挤"，不重复；compile_preview 的预览图留存为会话内 `preview-<n>.png` + 对应 `preview-<n>.pdf`，view_image 支持虚拟名 `preview.png`（最新）/`preview-<n>.png`（历史）并用 pdftoppm `-scale-to-x` 从 PDF 高分辨率重渲染实现真放大；classify 阶段进入即打印 `[classify 0/N]` 起始进度行。v1.5+（第三批）：会话工具返回图片时"图片 user 轮"必须放在全部 tool 响应之后（否则违反 OpenAI 协议报 400 insufficient tool messages）；HTTP 400 不再触发流式→非流式回退（仅 stream 关键词/404/405/415/422）；档位2 text 类图片改用 `img2text.ExtractTextOnly` 只提取可见文本（公式→LaTeX、表格→Markdown、无文字→`[NO_TEXT]` 保留原图），不再嵌入 AI 描述；样式化文本图按档位区分：档位2 直接嵌入原文（不保留样式、无标记），档位1 用单条注释 `<!-- DOCVISION-STYLED-TEXT: <样式>` + `CONTENT: <原文>` + `LINK: <原图>` + ` -->`（转换会话按手册重排或 includegraphics，注释不得进入 .tex）；analyze 解析 `[tool:<名>] ok|error` 统计工具调用并归类 `SESSION_*` 错误；交互式编译失败日志写 `compile error … → 已返回 AI 修复`。v1.5+（第四批）：会话历史以 JSONL 转录持久化（`internal/session/transcript.go`，append-only 一行一条消息；base64 图片不入转录、落 `media/` 目录以 `file://media/<hash>.<ext>` 引用，加载时还原 data URL），`Session.SetTranscript` 挂载后每条消息实时落盘；三处接入：tikz 矢量会话（`<outDir>/sessions/vector_<图>.jsonl`，中断续跑）、样式会话（`work/style_session.jsonl` 替代旧 `.json`，旧文件自动迁移）、单章转换会话（`work/sessions/convert_<章>.jsonl`，产物已存在时清理）——进程被杀/网络断连后下次运行自动恢复上下文续跑，不重烧 token；tikz 会话结束未 submit 时自动补发一次提交提醒；[book] images 阶段不再做 phase 级 done 跳过（RunImages 自身增量：done 跳过、fallback 重试），否则上次失败图重跑会被整体跳过；classify/process/img2text 进度行去掉 `skip: N`、断点续传从已完成数起跳（done0 计入总数）；档位1 转换提示词含 Layout reconstruction 规则：连续多图/图文交替（横向排列如 6 张 venn 图一行）须 doc_search+view_page 查原版面并用 subfigure/minipage 复现，不得线性堆叠。v1.5+（第五批）：会话工具大升级——`tools_work.go` 提供通用 `edit_file`（精确 find/replace、replace_all、append:true 追加；增量编辑不再整文件重写）、`grep`（工作区递归）、`WorkBashTool`（timeout 由 AI 参数决定默认30s上限300s、cwd=工作区根、输出上限5000字符）、`view_pdf`（按页渲染+裁剪/放大，始终从 PDF 重渲染，返回 page N/M）；compile 成功返回产物 PDF 名+页数；tikz 矢量会话用持久工作区 `sessions/vector_<图>.work`（不处理完不删、中断可续上，成功后清理）且带 write_file/edit_file/grep，submit 支持按工作区路径引用（{path:"figure.tex"}）不再强制整段重输出；style 会话带 edit_file/grep/view_pdf；convert 会话带 edit_file（仅自己的 .tex）/grep（全项目）/view_pdf；chapters 划分会话带 buffer.md 工作记忆缓冲区（edit_file 追增跨轮保留）+ read_file + bash + 转录（中断续上、完成清理）；档位1 latex 代码块前新增 `<!-- DOCVISION-ORIG-IMAGE: ![](images/…) -->` 指向原图（嵌入携带原图链接），转换会话用它回看源图、注释不得进入 .tex；思维链修复：chatstream 组装 assistant 消息保留 `reasoning_content`，ChatMessage 逐字节回传历史——GLM 保留式思考（`thinking.clear_thinking: false`）要求完整回传且是缓存命中的前提；新增 `models.<name>.tool_stream`（GLM 工具流式输出顶层字段），GLM 官方推荐 temperature 1。v1.5+（第六批）：档位1 md 机器注释统一骨架——首行 `<!-- DOCVISION-<TYPE>: <描述>`（TYPE ∈ STYLED-TEXT/VECTOR），下方字段行（CONTENT:/LINK:），闭合 ` -->` 独立一行；矢量图为 `DOCVISION-VECTOR: <label>` + `LINK: ![](原图)`，替代单行 ORIG-IMAGE；移除 install_font（下载字体不必要且有版权风险）：缺字体时样式会话在 submit_style 报告与 manual.md 列出缺失清单，用户手动下载放进 fonts/ 目录（文件名按清单），编译环境通过 TEXINPUTS/OSFONTDIR 指向 paths.fonts 使 cls 按文件名直接引用（无需装系统），latex 运行时自动写 fonts/README.md 说明放法与命名。v1.5+（第七批）：档位1 md 机器注释最终骨架定为「注释首行即闭合」——`<!-- DOCVISION-<TYPE>: <描述> -->` 行尾直接 `-->`，CONTENT:/LINK: 字段一律在注释**外面**各自独立一行，LINK 用 `[class](path)` 链接形式（class ∈ styled-text/vector）：STYLED-TEXT = 注释 + `CONTENT:` 原文 + `LINK: [styled-text](…)`；VECTOR = 注释 + latex 围栏上方 `LINK: [vector](…)`；RASTER 档位1 = `<!-- DOCVISION-IMAGE: <label> -->` + `DESCRIBE:` 解释块 + `LINK: [image](…)`——档位1 在 process 阶段**总是**为 raster 图生成解释文本（复用 img2text 提取，每张一次视觉调用，不依赖 insert_image_description 开关；无文本则只有 LINK 行），档位2 由 `latex.insert_image_description` 开关控制（开=嵌入 `[Image]( content )`，关=纯 `![image]`）；fallback 保持原图 + 就地 `<!– DOCVISION-ERROR –>`。v1.5+（第八批）：会话工具统一——`read_file` 成为唯一读文件工具（`{path, start_line?, end_line?}`：整文件或带行号窗口；`AltRoots` 支持只读附加根，如 style 工作区同时可读项目文件），删除 `read_md`/`read_lines`/章节 md-grep 三个重复工具；`compile` 统一为**只收路径**的通用编译工具（`CompileTexTool`：`{path?, engine?, passes?, bib?, shell_escape?, args?, timeout?}`，多文件 `\input` 工程可用，`engine:"latexmk"` 走完整多遍构建，成功返回产物名+页数），tikz 的 `compile_preview` 改为 `compile {path:"figure.tex"}`（先 write_file 再按路径编译、绝不内联代码；包装为 `standalone.tex` 编译，预览仍走 `view_image`/`view_pdf`）；style 会话新增 `compile {path:"example.tex"}` 与 `read_file`（读自己写的 cls/tex 与项目文件），`list_pages`/`view_page` 改名 `list_source_pages`/`view_source_page`（原书扫描页，与"看自己编译出的 PDF"的 `view_pdf` 明确区分）；convert 会话新增 `view_image`（看 md 中引用的原图），预览编译把 `source/images`、`source/figures` 符号链接进 scratch（章节里的图片引用可编译通过）；assemble 的修复会话升级为完整工作区会话（read/write/edit/grep/bash/compile/view_pdf/view_image/list_fonts），全书编译改用 `CompileFull`（有 latexmk 用 latexmk，否则两遍）；新增 `latex.bash_max_output`（会话 bash 返回字符上限，默认 5000）；process 进度行新增 `raster: N` 计数（档位1 保留原图并生成解释的图数量，与 errors/fallback 并列），`[raster]` 日志附 `described=true|false`；提示词同步全部工具名与骨架。v1.5+（第九批）：提交/回环/汇总升级——convert 会话写权限改为「自己的主文件 `chapters/<base>.tex` + 自己的资源目录 `chapters/<base>/`」（`WriteWorkFileTool.Prefixes` + `RejectDocumentclass`，`write_file` 必带 path），提交即整棵章节树；assemble 复制整棵 `work/chapters`（保留多文件层级，仅顶层 `*.tex` 作为 `\input`）；样式反馈回路改为**定向重做**：只对「工作汇报报问题」或「新 cls 下编译失败」的章节并发跑 style-fix 子会话（`styleFixSystemPrompt`：增量 edit_file 适配新 cls/manual、保留全部内容、绝不重新转换；失败才退回整章重转换），反馈会话可直接 `read_file` 读 `work/chapters` 里的真实提交；assemble 修复会话补 `read_file`（AltRoots 项目根，可读原 md/chapters）+ `list_source_pages`/`view_source_page`；删除单文件专用 `WriteFileTool`。v1.5+（第十批）：终审会话——全书编译成功后**总是**进入 `finalReview`（`latex.compile.final_review` 默认 true，轮数上限复用 `max_fix_rounds`）：构建树同时是样式包基础工作区（cls/sty/manual.md/example.tex 一并复制进 build/），工具集与修复会话共享 `bookSessionTools`（read_file[buildDir + AltRoots 项目根]/write_file/edit_file/grep/bash/compile[latexmk]/view_pdf/view_image/list_fonts/list_source_pages/view_source_page/submit），逐页核对成品 PDF（封面/目录/章节顺序/页码/图表位置/孤页/overfull）并做最小 edit_file 修正后重新构建；终审未通过只告警、已编译全书照常交付；`standalone.tex` 生成前重新 glob 章节（终审可能重命名/整理文件）。v1.5+（第十一批）：虚拟工作区（挂载表）——新增 `vfs.go`（`Mount{Name,Dir,Writable}` + `VFS.Resolve`）：每个会话一个命名空间，路径语法 `path`（默认挂载点，通常 `work`）、`name:path`、`/name/path`，越界 `../` 拒绝、只读挂载点写入拒绝、未知挂载点报错并列出可用挂载点；`read_file` 支持 `Mounts` 字段（优先于 Root/AltRoots）并统一用挂载点标签回显（`project:ch1.md`），`grep` 支持 `AltRoots` 多根检索（附加根命中加 `label/` 前缀），`write_file`/`edit_file` 接受 `work:` 前缀并拒绝其它挂载点；终审/修复会话的 grep 现在可同时搜构建树与项目原 md。`docvision latex --help` 档位1 说明同步为 images/style/chapters/convert/feedback/assemble 全流程。v1.5+（第十二批）：档位2 产物纯净 + checker 三轮反馈 + assemble 容错交付——档位2 输出必须是可以直接当 markdown 读的干净文本：**不含任何 `<!-- DOCVISION-* -->` 注释**，矢量图一律嵌入 SVG（`![label](figures/*.svg)`，SVG 不可用退 PNG/PDF，**不再有 latex 代码块分支**），text/styled 只嵌纯文本，raster 默认嵌原图（`insert_image_description` 默认 false，开启才嵌 `[Image]( … )`）；矢量回退与 SVG 失败在档位2 只记日志/progress.json，不再就地写注释（档位1 保持注释标记）；checker 硬性问题直接回给**同一个** convert 会话（上下文还在、最省 token），最多 `maxCheckerRounds=3` 轮，仍不过才作废该章 `.tex`+资源目录+转录、用全新会话重转换一次（`convertOneChapter(...,retry bool)` 防递归）；assemble 不再把"编译失败"当成阶段失败（会话需要靠编译看报错，修复会话在构建树里迭代，超上限只告警），有 PDF 才进终审会话，交付改为把**整棵 build 树**（跳过 .aux/.log/.toc/.synctex 等中间文件）复制到 `out/`——目录结构随终审会话实际产出（书不同可以不同），`book.pdf` 只是 `main.pdf` 的别名。v1.5+（第十三批）：查看工具统一 + 原书页索引升级 + 编译纯化——**删除 `view_source_page`**：原书 PDF 改为以只读挂载点 `source`（= `paths.mineru_output`）进入会话命名空间，用**同一个 `view_pdf`** 查看（`view_pdf {path:"source:<part>/<x>_origin.pdf", page:<局部页>}`，裁剪/缩放参数与看自己编的 PDF 完全一致，`zoom_width` 作为 `zoom` 别名保留）；`ViewPDFTool` 支持 `Mounts`（`bookMounts(workDir)` = work 可写 + source 只读）；**删除 `list_images`**（样式会话根是 work/style、没有 bash，才需要它，现在改用原书页索引 + bash）；`list_source_pages` 升级为真正的原书索引：无参数时列出「源 PDF → 页范围 → 全局页号」表 + **从 OCR 版面推导的章节起点**（`derivedSections`：命名/编号标题启发式；书没有目录时也能用），`{page:N}` 时列出该全局页的正文片段 + **该页抽出的图片文件名**（随后可直接 `view_image`）——这样"按文字找页 → 按页找图/文本"闭环成立；样式会话（含反馈）新增 `bash`（cwd=work/style：列目录、diff 草稿、wc/grep、pdftotext 等）与 `doc_search`（文本→原页），提示词改为「list_source_pages/doc_search/read_file(project:source/<md>.md)/view_pdf(source:…)/view_image」；原始文档索引提前到 style 阶段之前构建（`buildDocIndexQuiet` 移到 images 之后）；**编译纯化**：`CompileFigureTool` 不再栅格化 + 不再回贴预览图（删除 `previewEntry`/`addPreview`/`preview-<n>.png`/`preview.png` 虚拟名与 `ViewImageTool.Previews`/`zoomFromPDF`），`compile {path:"figure.tex"}` 只返回日志 + 产物名 + 页数，看图统一走 `view_pdf {path:"standalone.pdf"}`；`view_image` 只服务原图；保留内部 `renderSourcePage`（水印采样仍用按需渲染 + `pages/` 缓存）。v1.5+（第十四批）：会话沙箱 bash + 最小原书视图 + 每会话挂载表——`latex.bash_sandbox`（默认 true）把会话 bash 包进 **bubblewrap**：沙箱里只有该会话挂载表里的树，且以挂载点名出现在根下（`/work` 可写、`/project`、`/source` 只读，其余包括宿主真实路径全部不存在，`--unshare-net` 断网、`/tmp` 为私有 tmpfs），从此 `work:x.tex` = `/work/x.tex`、`project:source/book.md` = `/project/source/book.md`、`source:<file>.pdf` = `/source/<file>.pdf`——结构化工具与 bash 的路径空间完全一致；bwrap 缺失时自动回退普通 shell 并记警告；新增 `Runner.sessionMounts(kind, workDir)` 作为**唯一**的"会话能看什么"定义（kind ∈ tikz/style/chapters/convert/book：tikz 与 chapters 只有 work；style/convert/book 额外有 project（项目只读根）与 source）；**原书 PDF 最小视图**：新增 `pdfview.go` —— `buildPDFView` 把**本书**的 `*_origin.pdf` 以干净名字软链到 `<proj>/work/pdfview/<part>.pdf`，只有这些文件进 source 挂载点（整个 mineru_output 树绝不暴露给会话），页表由视图推导；`list_source_pages` 改为按视图名输出（`source:1000题数二-解析册_part1.pdf` + 页范围 + 全局页号 + OCR 推导的章节起点 + 逐页文本/图片清单）；`view_pdf` 用一个工具看自己的产物与原书（`source:<name>.pdf`）；convert 的编译 scratch 作为 `build` 挂载点（默认），style-fix/修复/终审同理；`buildPDFViewQuiet` 在 style 之前构建（与 `buildDocIndexQuiet` 一起）。v1.5+（第十五批）：project 挂载点最小化——上一批只收窄了 `source`（原书 PDF），`project` 仍是**整个项目根**（含 `work/sessions/*.jsonl` 转录、`work/reports`、`doc_index.json`、`pages/` 渲染缓存 15M+、`build/`、`out/` 成品书），模型既会被大文件挤爆上下文、`grep` 也会扫进别人会话的记录。新增 `ensureProjectView(proj)`（`pdfview.go`）：把 `project` 挂载点换成**最小只读视图** `<proj>/work/views/project/{source,style,chapters}`（只对存在的目录软链，幂等、可反复调用——`style/` 要等样式阶段提交后才出现），`sessionMounts` 用它、`Runner.projectRoot()` 供结构化工具（`read_file`/`grep` 的 AltRoots 或 Root）使用——bash 的 `/project` 与工具的 `project:` 看到**完全同一棵窄树**，路径后缀不变（`project:source/book.md`、`project:chapters/x.md`、`project:style/manual.md`），而 `work/`、`pages/`、`build/`、`out/`、`doc_index/` 在两侧都不存在；bwrap 参数生成同步支持"视图目录里的软链目录"（逐个目标 `--ro-bind`，不再跳过目录）。v1.5+（第十六批）：转换会话的"参考通道"与自留地边界——`project` 窄视图新增两个只读软链 `converted/`（→ `work/chapters`，其他章节**已转换的 .tex** + 其资源目录）与 `reports/`（→ `work/reports`，其他章节的工作汇报），让 convert 能"看但不依赖"别人的成果；会话转录 `work/sessions/*.jsonl` 仍然**任何通道都不暴露**；路径仍是 `project:converted/chapter_02.tex` / bash `/project/converted/...`。同时把"只能写自己"补齐：`EditWorkFileTool` 新增 `Prefixes` 白名单（convert 与 style-fix 只允许 `chapters/<章>.tex` 与 `chapters/<章>/`，改别的章节直接 REJECTED——之前只有 `write_file` 有白名单、`edit_file` 能改到任意文件）。另修两处真实缺陷：① 章节"附件"编译不通——`chapterScratch` 之前只软链了 `images`/`figures`，没有 `chapters/`，于是提示词要求的 `\input{chapters/<章>/xxx}` 在会话内 `compile` 必然失败（最终 assemble 却正常），现在把整棵 `work/chapters` 也软链进 scratch；② `doc_search` 打印的 "(part local pN)" 用的是 MinerU 的 0-based `page_idx`，与 `list_source_pages`/`view_pdf` 的 1-based 局部页差 1，现已改为 1-based；convert 提示词里残留的旧挂载名（`/style/manual.md`、`/chapters/<file>`、`/current/chapter.md`）也一并改成 `project:style/manual.md`、`project:chapters/<file>`。v1.5+（第十七批）：part 级页码定位 + 临时目录进项目 + 保留开关 + 逐章私有工作视图——① **part 级定位**：`DocEntry` 一直有 `Part`（MinerU 分块目录），现在 `pdfViewFile` 也记录 `Part`，新增 `FileByPart`/`LocatePart(part, localPage)`：定位不再依赖"全局页号累加"，因此**即使 OCR 索引与真实 PDF 页数不一致也不会漂**；`list_source_pages {page:N}` 优先用索引里该页的 part 定位（输出会注明 "located through the OCR page index: part X, page N"），索引里没有该页时才退回累计偏移；`doc_search` 每条命中直接附带精确路径 `source page: view_pdf {path:"source:<file>.pdf", page:N}`（模型不必自己换算）；新增构建期交叉校验 `pdfView.alignmentProblems`（每 part 索引页数 vs 真实页数，不一致记 `[pdfview] 页数不一致` 警告）。② **临时目录进项目**：`Runner.tempDir(proj, name)` 统一在 `<proj>/work/temp/<name>` 建临时工作区（拆章沙箱 `chapters`、样式 scratch `style`/`style-feedback`、每章编译 scratch `conv_<章>`/`convfix_<章>`/`convchk_<章>`），不再藏在 `/tmp`，日志里会打印保留路径。③ **保留开关**：新增 `latex.keep_temp_dirs`（默认 false：用完即删）与 `latex.keep_session_records`（默认 false：成功会话的 JSONL 转录删除；有人要看全部会话记录就打开）——两项独立；**debug 日志（`--debug` / `log_level: debug`）下两者必定保留**（`Runner.keepTemp/keepRecords` 对 `log.DebugEnabled()` 取或）；转录删除统一走 `Runner.keepSessionFile`（拆章/单章/反馈/矢量图四处）。④ **逐章私有工作视图**：`ensureChapterView(proj, base)` 生成 `<proj>/work/views/chapter_<章>/chapters/`，里面只有该章的 `chapter_<章>.tex`（软链，写入即落到正式 `work/chapters/`）与资源目录软链 → convert/style-fix 的 `work` 挂载点与 write/edit 根都换成它，"自己的操作内容"终于名副其实（别人的成品只能通过只读通道 `project:converted/`、`project:reports/` 看）。⑤ 核对修正：style-feedback 复用会话的 `read_file`/`grep` 之前仍指向整个 `proj`（绕过窄视图），已改为 `r.projectRoot()`。v1.5+（第二十批）：软限制 + 两级压缩 + 原图尺度测量 + 缓存亲和——① **看图软预算**：`tools.view.image_max`（默认 30）/ `pdf_max`（25）/ `warn_ratio`（0.7，向上取整，30×0.7=21 从第 21 次起提醒）按**对象**计数——`image_max` 按图片文件、`pdf_max` 按"文件+页"（每页各有 25 次，长书不用整会话共用一个池子），只把"已用 N/总数、仅剩 K 次"写进工具回执，**从不拦截**调用（负值=不限）；原先的硬拦截（`MaxViews`）删除。② **原图绝对尺度测量**（`imagescale.go`）：从位图回溯到 MinerU 解析目录（`content_list.json` bbox + `layout.json` page_size），算出印刷尺寸 mm、有效 dpi、占版面宽度比例、宽高比，显示标准 **mm 优先**（`ORIGINAL FIGURE SIZE: 36.9mm x 20.2mm on the page (about 21% of the page width); bitmap 284x156px, effective resolution 195 dpi, aspect 1.82:1`），高度按位图比例换算（MinerU bbox 不紧贴图）；每次 `view_image` 结果与作图会话初始消息都带上，测不出解析目录时退化为只给宽高比。③ **工具轮次两级软限制**：`sessions.*.tool_rounds_warn_ratio`（默认 0.7）起每轮**原地替换**一条提醒消息（不膨胀历史、不破坏前缀缓存），到达 `max_tool_rounds` 后仍有 `tool_rounds_grace`（默认 20，负=无宽限）轮可调用工具，之后才禁用（`tool_choice:"none"` 但**保留 tools**）。④ **两级压缩**：`maybeCompact` 先本地裁剪（`prune_tool_chars` 默认 **4096**：工具结果裁成"前半 + 后 1/4"——实测 325 条真实工具结果最长 6183 字符、p99 5046、无一超过 8192，8K 阈值等于失效（真正可能很长的只有 `read_file` 整文件、`bash`、长 `doc_search` 命中）；`keep_images` 默认 3：更早的图片换文字占位并注明"需要时重新查看"），仍超阈值才花 AI 摘要（保 system+任务+最近 8 条）；`EstimatedTokens` 补上系统提示词与工具定义（原来低估 5 万+ tokens）。⑤ **debug/trace 只写日志文件**（`logger.writeFileOnly`）：此前 `logger.write` 连 debug 一起打印控制台，导致终端里会话日志与进度行交错覆盖；`--debug`/`--trace` 帮助文本同步改口径。⑥ **前缀缓存亲和**：每个会话带稳定 `user` 字段（`docvision-<label>-T<tid>`，同一个会话所有请求同值）供网关做渠道亲和（厂商缓存按上游 key 分，换渠道即 0 命中），`[cache-probe]` 日志同时打印 user。⑦ 新增配置项三处同步（`config.go` 默认值 + `default.yaml` + `config.example.yaml`）并加模板解析测试。v1.5+（第十九批）：会话恢复/压缩/看图真实性修复 + 作图比例——① **续跑不再丢系统提示词**：转录 JSONL 里没有 system 行，`SetMessages` 整体覆盖导致恢复出的会话没有系统提示；现在总是把本会话系统提示放回队首（旧 system 行替换不重复）。② **思维链入转录**：`transcriptLine` 增 `reasoning_content`，Append/Load 双向搬运（GLM 保留式思考要求完整回传，也是前缀缓存前提）。③ **压缩重做**：`compact()` 改为「系统提示词 + 原始任务 + 最近 8 条消息原样保留、只压中间段」，摘要请求变成**在原会话末尾追加一条指令的增量请求**（复用前缀命中缓存，不再把整段含 base64 图片的会话塞进一条新 user 消息全额计费）；`LoadTranscript` 回放时丢弃最近一次 `=== COMPRESSED SESSION CONTEXT` 之前的旧消息。④ **进度与并发可观测**：classify/process 进度行加 5s ticker（原来只在任务完成时重绘，长时间停在 `running: 0`），计数器读写同锁（原读侧无锁=数据竞争）；classify/process/convert 各写一条 `并发 N（latex.concurrency）` 日志行（进度行只在控制台，日志文件里看不到并发）。⑤ **作图比例**：删掉提示词里"宁可画大一点"的反向引导，新增硬规则（与原图同宽高比 ±5%、线宽随图形缩放、禁止整图 `\resizebox`、不得画满画布）；作图会话初始消息带原图位图宽高比，`compile` 回执报告产出 PDF 的 pt 尺寸与宽高比（实测原图占页宽 21%、成品 62%、比例 1.82→1.33）。⑥ **看图预算**：矢量图会话 `view_image` ≤12 次、`view_pdf` 每页 ≤10 次（原事故：284×156px 小图被放大查看 115 次），`view_image.zoom` 上限对齐 `view_pdf`（6000px），回执写明"与已看过的一致就停止查看并提交"。⑦ **`doc_search` bbox 单位纠错**：bbox 是 MinerU 版面坐标（约 2× 页宽 pts），不是 PDF points；工具命中直接给出 `left/top/right/bottom` 裁剪百分比。⑧ **提示词占位符渲染**：所有内置提示词统一走 `renderPrompt`（`{OUTPUT_LANG}`=`options.output_language`、`{MAX_ROUNDS}`=会话工具预算），新增守卫测试。⑨ **缓存前缀稳定**：`max_tool_rounds` 用尽后不再摘掉整个 `tools` 块（厂商按 (system, tools, messages) 拼前缀，摘掉工具块使 prompt 少 1752 token 并让其后全部位移、缓存归零），只设 `tool_choice:"none"`；`Usage` 增 `CachedTokens()`（`prompt_tokens_details.cached_tokens`）并在用量行输出 `cached=N(%)`，debug 下每请求多打一行 `[cache-probe] body=… head_sha=… tools=N tool_choice=… messages=…`（哈希变=我方改了前缀；哈希不变而 cached=0=网关换了上游渠道，厂商缓存按上游 key 不跨渠道）。v1.5+（第十八批）：文档全面同步 + 会话 bash 配置归位——① 用户手册与帮助文本按代码事实定点修补：`README.md`（LaTeX 两档位真实流程、`write_file→compile→view_pdf→submit`、档位2 SVG 纯净产物、档位1 私有工作视图/只读参考通道/checker 三轮同会话/终审会话与整树交付、嵌入骨架、目录布局、配置表默认值取**代码默认**）、`docvision latex --help` 重写、`docvision verify --help` 更正（核对只显式运行，`verify.enabled` 只是提示）；全局清理历史遗留名（`view_page`/`list_images`/`install_font`/`compile_preview`/`preview.png`/`read_md`/`mermaid_validation`/`workflow --step latex`/`options.format_fix_attempts`）。② 配置项归位：会话 bash 的两项从 `latex.bash_sandbox`/`latex.bash_max_output` 迁到 **`tools.bash.sandbox`/`tools.bash.max_output`**（与 `tools.mermaid`/`tools.latex` 同级，因为它们是**工具**配置而非档位配置）；旧键仍生效但启动打印迁移提示（`Config.UsesDeprecatedBashKeys`），解析顺序 tools.bash > latex.bash_* > 内置默认（sandbox=true、max_output=5000），`setDefaults` **不再**把默认值写进旧字段（否则会掩盖显式的新键）。③ 修真实缺陷：`docvision latex --all` 一直是死代码——代码读 `GetBool("all")` 但从未注册该标志，显式传会 `unknown flag`；现已注册并出现在 `--help`。④ 模板一致性：`verify.enabled`、`paths.fonts`、`insert_image_description`、`raster_dpi` 注释改为与代码一致（raster_dpi 只影响矢量图 PNG 回退产物）。⑤ 规则板块新增文档同步/CHANGELOG 归属/配置项一致性/发布线纪律/文档审计五条。v1.5+（第二十一批）：提示词集中管理——内置提示词不再散落在各会话源码里，统一收进 **`go/internal/prompts`**：`templates/*.md`（19 段：12 段系统提示词 + 7 段各会话首次用户提示词）经 `//go:embed` 编进二进制，调用点只有两种写法——纯静态 `prompts.Must(prompts.FigureSystem)`、带动态数据 `prompts.Render(prompts.ConvertUser, map[string]string{"MANUAL": manual, …})`，占位符替换逻辑只有一份 `prompts.Fill`（latex 的 `renderPrompt` 也改为调用它）。注册表为每个模板声明④占位符清单（`Vars`：`{OUTPUT_LANG}`/`{MAX_ROUNDS}`/`{MAX_TOOL_CALLS}`/`{MANUAL}`/`{CONTEXT}` 等）、⑤`MustMention`（该会话必须提到的工具名）、⑥`RetiredNames`（退役工具名黑名单）；`prompts.Get` 读取模板时吃掉尾部换行，于是"编辑器加不加结尾换行"不会改变发给模型的正文。搬迁方式是**抽取而非手抄**：临时工具用 go/ast 从旧常量/旧 `fmt.Sprintf` 取出文本（含 `strings.Join([]string{…},"\n")` 逐元素判定静态/动态、把 `%d/%s` 逐个换成 `{占位符}` 并按序接管参数表达式），逐字节校验后才提交；首次用户提示词只搬**静态文案**，与状态同生共死的碎片（轮次预算提醒、图片占位说明、压缩指令、续跑/重试提示、checker 反馈回写、img2text 的 mermaid/tikz 修复消息、水印指引）仍留在原处，各工具自身的 `Definition()` 文案也不动。新增 4 条守护测试：① 模板 ↔ 注册表**双向**一致（死模板/错文件名都失败）；② 按声明占位符全量渲染后不得残留 `{...}`（历史事故：`{OUTPUT_LANG}`/`{MAX_ROUNDS}` 原样发给模型，因为 tikz 会话直接传了裸常量）；③ 模板锁定的工具名必须在代码 `Name()` 里真实存在、且代码里的工具必须在某处模板被提到（`internal/latex/prompt_tools_test.go` 扫描包内工具名）；④ 退役名字不得复活（`read_md`/`view_page`/`view_source_page`/`list_images`/`install_font`/`compile_preview`/`image_locate`/`mermaid_validation`/`preview.png`/`format_fix_attempts`）。动机是三类历史事故：同一规则写在两个会话里互相矛盾（"禁止重叠/拥挤" vs "画大一点"）、工具已删除而提示词仍在教（模型于是调用不存在的工具）、占位符漏渲染直达模型——收进一处后由测试而非人工巡检拦截。规则板块新增提示词维护条目。
v1.5+（第二十二批）：虚拟工作区真实修复 + 终端输出 + 会话预览——① **视图软链绝对化**（`pdfview.go` 新增 `linkAbs`）：`buildPDFView`/`ensureProjectView`/`ensureChapterView`/章节 scratch 之前用裸 `os.Symlink(相对源, link)`，而配置默认是相对路径（`./latex_project`）→ 内核按**链接所在目录**解析 → `<proj>/work/pdfview/*.pdf` 与 `<proj>/work/views/project/{source,style,chapters}` 全部悬空；表现是 `list_source_pages`/`doc_search`（走内存视图）正常、一落盘就 `文件不存在: source:<part>.pdf` / `project:source/<md>.md`，模型连续三轮猜名字。② **沙箱 bash 修活**（`WorkBashTool.sandboxArgs`）：bwrap 的 `--bind/--ro-bind` 源与 `--chdir` 之前原样传挂载目录，而 bwrap 自己解析源路径、默认配置又是相对的 → 每个会话的 `bash` 都以 `bwrap: Can't find source path …` 失败（模型失去 `ls`/`grep`），现已一律 `filepath.Abs`。③ **终端不再外泄/错行**：`RunImages` 结束处硬编码 `SetQuiet(false)` 抹掉了 `RunBook` 的静默设置（全仓库仅此一处），导致其后所有阶段的 info 级 `[tool:x] ok` 直冲控制台；新增 `logger.Quiet()`（保存-恢复）与 `Logger.SetLiveLine(render)`（控制台日志行前先换行结束进度行、写后重绘），进度行只在 stdout 是字符设备时用 `\r` 重绘（管道/重定向改为按节拍整行输出）并加 `\x1b[K` 清行。④ **找不到文件可诊断**：`compile` 不再在编译前删掉上一次的 `standalone.pdf`（一次失败即抹掉唯一产物，随后 `view_pdf standalone.pdf` 只回"文件不存在"），失败/成功回执分别说明旧产物是否还在、可用产物到底是哪个（`figure.tex` 只是正文；不存在 `figure.pdf`）；`read_file`/`view_pdf` 的"文件不存在"附带邻居列表（`suggestInDir`/`suggestNear`）与可用挂载点；`VFS.Resolve` 容忍混写 `source:/source/x.pdf`（bash 路径空间本就是 `/name/path`）。⑤ **绘图提示词收敛 + 看图预算前置**：上一版把比例/线宽写成硬规则、加"看图纪律与预算"后提示词从 4300 → 6055 字符，实测同一张思维导图由 0909 的 13 轮（首个工具调用 `view_image`）退化为 21 轮（首个工具调用 `write_file`）；现改为「整体比例/印刷尺寸**和原图差不多**」「线宽/字号**和原图差不多**」，删掉"最多看几次""看完就提交"的劝退句（**不要**再加"先看图再写代码"这类新约束——模型本来就会先看图，加了反而把会话推向"为看图而看图"；工作流第 1 步与首次用户提示词都保持 0909 的简单写法），预算额度（`tools.view.image_max`/`pdf_max`）以 `{VIEW_BUDGET}` 占位符进首次用户提示词（`tikz.go` 填值，只报两个数字），回执提醒改为**每对象一次**（`viewBudgetNoteOnce`，超限只报"已用尽"）；style 提示词里并列两种路径写法处补"任选其一、不要拼接"。⑥ **样式反馈会话的系统提示词**（真实缺陷）：反馈轮 `NewSession(..., "", tools, ...)` 以为"系统提示在持久化消息里"，但 JSONL 转录**从不写 system 行**，于是打回那一轮完全没有系统提示；抽出 `Runner.styleSystemPrompt()` 供样式会话与反馈会话共用。⑦ **转录不再重复**：`saveSessionContext` 在已有实时转录时整段再写一遍（转录里出现重复历史 + 夹一条 system），现在 `Session.HasTranscript()` 为真即跳过，仅在无转录时兜底；旧 `.json` 续跑迁移时先把历史补写进新 JSONL（否则下次续跑丢历史）。⑧ **会话预览**（新包 `go/internal/sessionview` + `docvision sessions`）：把任意层级 `*.jsonl` 转录（`work/style_session.jsonl`、`work/sessions/convert_<章>.jsonl`、`source/sessions/vector_*.jsonl`）渲染成 DSH 风格深色页面——左侧会话列表（阶段标签/消息数/相对时间/活跃圆点/过滤），右侧时间线（思考过程可折叠、工具调用与按 `tool_call_id` 配对的工具结果分色、图片缩略图点击放大），静态模式 `WriteStaticHTML`（数据内嵌、`file://` 直接打开、不轮询）与实时模式 `Serve`（`127.0.0.1:8848` 只读、`/api/index` + `/api/session?from=N` 增量、2 秒轮询）共用同一份原生 JS 资源（`//go:embed`、无 CDN、无构建步骤）；页面按转录里真实有的东西显示，不假装有 system 提示词/工具 Schema/逐条时间戳。

v1.5+（第二十三批）：doc_index 图片条目带上内容——档位1 的块索引一直只读 MinerU 的 `content_list.json`，而 MinerU 对没有 caption 的图片块只给文件名：索引里那条 image 条目的 `text` 是空串（实测 242 条里 8 条 image、其中 4 条空，正好是 md 里有 DOCVISION 注释的那 4 张），于是 `doc_search "知识导图"`、`doc_search images/<主题>/<hash>.jpg` 都搜不到它们，`list_source_pages {page:N}` 的页详情里这些图既无文本也无描述，会话只能 `view_image` 一张张看。现在构建索引时把 images 阶段写进 `<proj>/source/*.md` 的 DOCVISION 注释按**图片文件名**（`filepath.Base`，`images/<sha>.jpg` ↔ `images/<主题>/<sha>.jpg`）接回图片块：`DocEntry` 新增 `omitempty` 的 `Marker`（`styled-text`/`vector`/`image`）、`Label`（注释里的描述）与 `Content`（STYLED-TEXT 的 `CONTENT:` 原文 / RASTER 的 `DESCRIBE:` 解释）；新增 `parseMdMarkers(mdDir, mdNames)` 逐行扫描（无正则、4MB 行上限；容忍注释缺失、字段换序、多行 `DESCRIBE:`、CRLF、"`-->` 独立成行"的旧骨架与几十行的长 `CONTENT:`——窗口以最后一条字段行起算），`buildDocIndex(mineruOutput, sourceMDs, markerDir, outPath)` 多收注释目录、`buildDocIndexQuiet` 传 `<proj>/source`（缺失时回退输入 md 目录）并在日志里记"带 DOCVISION 注释: N"；`doc_search` 的 haystack/命中输出与 `list_source_pages` 页详情的图片清单都显示 `[vector] mind-map diagram` + `content:`/`description:` 摘要（沿用 `snippet` 截断），两个工具的 `Definition` 同步。旧 `doc_index.json` 仍可反序列化（新字段 omitempty、索引每次运行重建）；档位2 产物按设计不含任何注释，因此本改动只惠及档位1。

v1.5+（第二十四批）：转录自述 + 多项目布局 + 预览页改版——① **系统提示词快照（`t=meta` 元信息行）**：JSONL 转录一直**不写 system 行**（系统提示词由 `internal/prompts` 模板每次运行重新渲染，含本次运行才确定的水印开关/挂载说明；工具定义由运行时按会话类型拼装），代价是"看转录不知道模型被交代了什么"。现在 `Session.SetTranscript` 挂载转录时先写一条 `{"t":"meta","kind":"system","session_label":…,"model":…,"system_sha":…,"text":"<完整系统提示词>","tools":[{name,description,parameters}…]}`：**只记录、永不回放**（`LoadTranscript` 只认 `t=="msg"`，续跑语义与"提示词重新渲染"原则都不变），同一份提示词按 sha256 前 8 字节去重（重复挂载不重复写、换了模板才追加一条），系统提示词为空时打 WARNING（历史事故：样式反馈会话曾以空提示词启动，转录里看不出来）；新增 `TestTranscriptMetaLine`（去重、回放只看到消息、改提示词后追加）；实测 3 个真实工具的快照单行 2549 字节、39.6 KB 提示词 → 单行 42.7 KB 且回放仍只看到消息。② **多项目布局**（用户要求"一个 `latex_project/` 下并存多本书"）：此前输出根**本身**就是工作区（`<root>/work|source|style|chapters|build|out|doc_index|progress.json`），书 A 跑完再跑书 B 会互相覆盖、会话与进度串在一起。新增 `internal/latex/project.go` 把"本次运行的工作区在哪"收成**唯一一处**判定（`resolveProjectDir`，`ProjectLayout{Root,Dir,Name,Legacy,Note}`，`Note` 就是那条 info 日志文本、测试也用它断言分支）：优先级 ① 显式 `--project <名字>` **无条件**用 `<root>/<名字>/`（即使根目录里有旧版工程——这就是"在这台机器上开第二本书"的出口）；② 未指定且 `<root>` 命中旧布局标记（`work/source/style/chapters/build/out/doc_index/progress.json/progress_items`——`chapters/style/build/out` 确实直接挂在旧工程根下、不在 `work/` 里，而 `progress_items` 专为档位2，不认它用户已有的 `finally_latex/` 会被判成空目录而全量重跑）→ 沿用 `<root>` 本身并记 info 日志"检测到旧版单项目布局…要在同一输出根下新建独立项目请用 `--project <名字>`"、**不写标记也不动既有文件**；③ 否则用 `<root>/<项目名>/`。项目名默认取本书主题名（`projectSourceName`＝`subjectOf(filepath.Base(main))`，与 `images/<主题>/` 同名），经 `SanitizeProjectName` 安全化（去路径分隔符、截断 60 字、与结构名冲突则退回主题名），`--project` 名字不可用时退回自动命名并说明。工作区里写 `.docvision_project.json` 自述（项目名+主题名来源），同名目录属于**另一本**书时退 `<名字>-2`，无标记目录（手建的/从旧布局整体搬来的）照旧复用（幂等、断点续传不受影响）。唯一调用者是 `Runner.useProjectDir`（打日志 → 非 Legacy 才 MkdirAll+写标记 → 设 `r.projDir`），`ProjectDir()` 供 CLI 定位日志分析目录、`workRoot()` 供水印记忆与 `pages/` 缓存；`r.projDir` 在 images 阶段**之前**就已设好；`RunImages` 档位2 独立运行时解析（档位1 嵌套 `OutDir=<proj>/source` 时不再解析）。`--project` 同时透传给 `RunBook`/`RunImages`，`cli_latex.go` 的日志分析段改用 `runner.ProjectDir()`（不再直连 `paths.latex_output`）；`docvision verify` 按项目工作区定位（`resolveExistingProjectDir`：显式 `--project` 不存在则报错列出已有项目；否则 root 本身是旧工程就用它，再否则只有唯一子项目才自动选中、多个报错列出名字），顺带把代码里读而 `--help` 里缺的 `verify --progress-dir/--report` 两个旗标补上；`default.yaml`/`config.example.yaml` 的 `latex_output`/`latex_project` 注释同步改为"输出根 + 每本书工作区 `<root>/<项目名>/`"。测试：`project_test.go` 10 条覆盖四条分支与名字安全化（并做变异验证：改坏分支顺序/安全化/日志文案 → 10/2/9 条转红）。③ **会话预览页改版**：默认**白天模式**（配色走 CSS 变量、切主题只改根属性 + `localStorage`，`file://` 下静默回退白天，首帧前应用避免闪色）；左侧会话列表改为**按项目分组**（`SessionInfo.Project` = 相对路径第一段，根目录直挂归「（根目录）」，`latex_project/<书名>/` 归 `latex_project`）：每个项目一个可折叠集合（组名+会话数+活跃圆点），默认全展开、过滤时只留有命中的组；右侧把 `t=meta` 行渲染成时间线**最前面**的可折叠卡片（标题「系统提示词（本次运行快照，不参与回放）」+ 模型/会话标签/短哈希/字符数 + 等宽全文 + 复制按钮 + 工具名与描述、`parameters` 二级折叠；多条 meta 只显示最新并注明"共 N 条"；**默认收起**——几万字符的提示词默认展开会把时间线顶下去），且 meta 行不进消息序列、不计消息数；思考内容默认折叠（摘要写"思考过程 · N 字符"）；`--list` 终端表格新增「项目」「提示词」两列。测试：`sessionview` 6 条；另用合成工程 + headless Chromium 验证了白天默认、两个项目分组、meta 卡片与四条布局分支。

④ **进度行空行修复**（用户实测 `[classify 8/8] …` 与 `[process 2/8] …` 之间空了一行）：终端里同时有两个"实时行"写器——阶段进度行 `liveProgress`（classify/process 各一个）与会话实时行（`book.go livePhaseLine`，每个 AI 会话一条）——会话实时行结束时用 `\r\x1b[K` 清掉当前行，随后阶段进度行 `Close()` 只补一个 `\n` 就留下空行。现在 `liveProgress` 记住最近画出去的一行（`last`）：`Close()` 在终端里重画最终一行再换行（谁清过都不留白），管道里最后一次 `render()` 本就是整行、`Close()` 不再重复；`render()` 按 `stdoutIsTerminal()` 分流（终端 `\r\x1b[K` + 原地覆写，管道整行输出、不再往日志灌裸 CR），`newLiveProgressTTY(text, tty)` 供测试注入；会话实时行在管道下也不再多补一个 `\n`。测试：`liveprogress_test.go` 的 `TestLiveProgressTTYCloseRepaints`（终端下只换一次行）与 `TestLiveProgressPipeNoTrailingBlank`（管道两阶段衔接只有两行、空进度行不输出）。
⑤ **会话预览分组跟上多项目布局**：②的组名原本取"相对路径第一段"，多项目布局下同一个输出根里的所有书会挤成一组。现在 `scanner.projectGroupFor(root, rel)` **看磁盘**：`<seg1>/<seg2>` 是工作区（标记 = `.docvision_project.json`/`progress.json`/`progress_items`/`work`/`source`/`style`/`chapters`/`doc_index`）且 `<seg2>` 不是结构名（work/source/style/chapters/sessions/temp/views/pages/build/out/reports/media/figures/images 等） → 组名 `seg1/seg2`（`SessionInfo.ProjectLegacy=false`）；否则组名 `seg1`，`ProjectLegacy` = `seg1` 本身是不是工作区（旧版单项目输出根，侧栏标「旧版单项目」）；首段是结构名或转录直接挂在扫描根下（含 `--dir <工程>` 直接扫描工程本身）→ 「（根目录）」。工作区判定按目录缓存（`scanner.ws`，实时模式 2 秒轮询不重复 stat）。前端：组标题拆成 `proj-prefix`（输出根，灰、等宽、小字）+ `proj-title`（书名，粗体），旧版组加 `proj-note`「旧版单项目」chip；**静态模式必须把 `projectLegacy` 一起挑进 `state.sessions`**（那边是按字段白名单 map 的，漏字段会让静态快照少一块 UI——本次就漏过）。测试：`TestProjectGroupMultiProjectLayout`、`TestProjectGroupScannedInsideWorkspace`、资源测试新增前缀/标记/静态字段守卫；`docvision sessions --list` 的「项目」列与 README/`--help` 同步。

v1.5+（第二十五批）：看图尺寸真正可用（mm）——用户实测 `view_image` 只回 `ORIGINAL BITMAP: 320x178px`，从来看不到 mm，且裁剪后不知道"现在看的是多大一块"、看 PDF 时也没有物理尺寸，无法判断写出来的 LaTeX 是否符合要求。① **测量在生产里一直只剩 px**（真实缺陷）：第二十批的测量是"从位图向上回溯找 `*content_list.json`"，但 images 阶段把位图**拷贝**进了项目（`<proj>/source/images/<主题>/<sha>.jpg`），向上永远到不了 `mineru_output`，于是每次运行都静默退化成宽高比兜底——`ORIGINAL FIGURE SIZE` 从未在真实运行里出现过（旧布局同样如此，只有测试和手工验证时"看着是好的"）。现在 `NewRunner` 调 `SetImageParseRoot(cfg.Paths.MineruOutput)`：测量先 `EvalSymlinks`（视图软链直接指进解析）再按**图片文件名**命中一次性构建的解析索引（`<root>/*/content_list.json` 的 bbox + 同 part `layout.json` 的页尺寸 → mm/dpi/占页宽比例；MinerU 图片名就是内容哈希，拷贝保留文件名，因此文件名是可靠键）；解析根变化时索引失效**并清空测量缓存**（否则先前"没找到"的否定结论会粘住，这是测试里实际踩到的），找不到才退回宽高比。② **裁剪后给当前尺寸**：`ImageMeasure.Crop(left,top,right,bottom)` 按同一 mm/像素比例换算（宽度信 bbox、形状信位图，与整图同口径），`view_image` 裁剪时追加 `this crop is 18.4mm x 10.1mm on the page`（未裁剪不出这行）。③ **PDF 也给 mm**：新增 `pdfPageSizeMM(pdf, page)`（`pdfinfo -f N -l N` 取**该页**尺寸）与 `parsePDFInfoSizes`，`view_pdf` 回执变成 `… width 1280px, page 182.0mm x 257.0mm, this crop 91.0mm x 128.5mm`——看原书页就是真实书本尺寸（定 `geometry` 用），看自己编的 PDF 就是成品实际尺寸；`compile` 回执的产出尺寸由 cm 改 **mm**，与 `ORIGINAL FIGURE SIZE` 同单位。**`pdfinfo` 的逐页行是 `Page    2 size:`（中间补白）**，原来的前缀匹配会静默漏掉每一页尺寸——现按字段切分。④ 提示词只补口径不补约束：转换会话 `view_pdf` 说明加"回执给页面与裁剪的 mm，用它定复现尺寸"，作图会话的尺寸规则加"单位是 mm，与原图测量同单位"。测试：`TestMeasureImageCopiedIntoProject`（拷贝进项目 + 指定解析根 → 必得 mm 与裁剪尺寸；不告知解析根时不得假阳性）、`TestViewImageReceiptReportsCropSize`、`TestViewPDFReceiptReportsMillimetres`（xelatex 真出 100x150mm 的 PDF）、`TestPDFPageSizeMMPerPage`（逐页尺寸 + 不存在文件不编数字）。

v1.5+（第二十六批）：一次日志审计后的真实缺陷修复（用户提供 2026-09-10/11 一整轮真实运行日志 + 工程：3 章只成功 1 章，两个会话因额度耗尽失败，`assemble`/终审从未运行）——① **章节 `compile` 一直编译裸 `\input` 分片而不是 wrapper**（**87/87 次章节编译全部失败**的根因）：`chapterScratch` 写出了 `<章>_wrapper.tex`（`\documentclass{book}` + `\input{<章>.tex}`）、runner 自己的三处核对也编译 wrapper，唯独交给会话的 `CompileChapterTool.MainFile = base + ".tex"` → 每个会话从头到尾都在编译没有前导的分片，报错是 `Undefined control sequence`（首个 `\section`）与被模型误判成"类加载失败"的 `The font size command \normalsize is not defined`（模型自己把 `\begin{document}` 写进分片后触发），于是 133/80/98 轮的盲目编译-修改循环、87 次失败编译、写了 27+2 个探针文件（还各写两遍：第二遍把内容覆盖成 `% (scratch probe removed)`，因为没有删除工具），最后两个会话在额度耗尽后失败，`chapter_003.md` 的工作汇报把这次环境缺陷误记成"cls 加载失败"；现在 `CompileChapterTool` 区分 `MainFile`（会话自己的分片）与 `WrapperFile`（真正编译的 wrapper），失败回执标明编译的是哪个文件，回归测试 `TestCompileChapterToolUsesWrapper` 同时钉住"编译 wrapper 成功 / 只编译分片必然失败"两个方向。② **一个会话一张挂载表**：convert 的 `read_file`/`grep` 各自用 `Root` 拼出一个**名叫 `work` 却指向项目视图**的挂载点，`write_file`/`edit_file` 的 `work` 是本章私有视图，bash 的 `/work` 又是第三个含义，而提示词教的 `project:chapters/<file>` 在 `read_file` 里报「未知挂载点 "project"（可用: work(rw)）」——三个会话的**第一次工具调用全部失败**，随后 28 次 `read_file` 错误、9 次未知挂载点、`grep` 把 `project:style` 当文件名拼成 `…/project/project:style: 没有那个文件或目录`；现在 `GrepTool` 新增 `Mounts` 并走 `VFS.Resolve`（`name:path`/`/name/path` 都认，裸路径遍历全部挂载点），`ReadFileTool` 挂载表模式下裸路径先可写后只读，convert/style-fix 的工具与 bash 共用 `sessionMounts` 的同一张表。③ **`compile` 支持 `{path}`**：可单独编译工作区里任意 `.tex`（探针、`\input` 分片），不必再污染主文件。④ **会话 bash 的 `/tmp` 从"每次调用一个新 tmpfs"改为"本会话私有的持久目录"**（`WorkBashTool.TmpDir` ← `Runner.sessionBashTemp` 建在 `<proj>/work/temp/bash_<会话>`，`--bind` 到 `/tmp` 并设 `TMPDIR/TMP/TEMP`；会话结束删除、keep_temp_dirs/debug 保留）——原行为让"上一次 bash 写的文件"下一次必然消失（`/tmp/p10-10.pgm` 两次 `FileNotFoundError`、`t5.tex` 折腾 10 轮，会话自己到第 185 轮才悟出 `per-call scratch`）。⑤ **`submit_style` 改为按工作区路径提交 + `extra[]` + `report`**：此前内联回吐 47KB（cls 18KB/example 17KB/manual 11KB，提交前还把 cls 重读一遍以便粘贴），且提示词承诺的"报告"字段根本不存在；现在路径读盘（**第二十七批已取消内联兼容**）、`extra` 里的附属文件（helper `.sty`、TikZ 样式、字体表）落盘进 `style/` 并一起进示例编译 scratch、`report` 写 `style/REPORT.md`；形如 `*.cls/*.md/*.tex/*.sty` 的短字符串不再被当成内联内容。⑥ **提示词**：转换会话**不再内联 11k 字符手册**（改指 `project:style/manual.md`，会话第一步即读），首条消息新增**工作区地图**并明确"章节文本在 `project:chapters/<章>.md`，从它转换、原书 PDF 只用来看版面"；`## Tools` 重写为 `## Workspace and tools`（含 **新增的 `bash`** 与 `compile {path}`）；样式修复会话同样改为按路径读手册 + 同一张工作区地图。⑦ **`truncateStr` 按字节截断会切断 UTF-8 字符**（21,658 字的 chapter_002 只递过去 1,527 字、断在 TikZ 样式表中间，标题却写 "first 2000 chars"）：改为不切断字符，新增 `truncateRunes` 给"字符"语义用，章节预览改用它。⑧ **退避时长溢出成负数**（`-2562047h47m16.854775808s` = `math.MinInt64`，237 次限速等待里 26 次为负 = `time.Sleep` 不等待）：新增 `backoffWait`（先钳位移再乘、越界取上限）；**账户级错误不再当限流重试**（`余额不足/无可用资源包/配额/insufficient/quota/billing` → 立即 `[SESSION_INSUFFICIENT_BALANCE]` 返回，此前在冻结的请求体上空转 3h31m/239 次直到 `rate_limit_retries: 100` 用完）。⑨ **实时行在终端每秒刷新"已用"**（用户实测时间像不动；管道下仍 10 秒整行输出），`[cache-probe]` 的 `tool_choice=%!q(<nil>)` 改为显示 `auto`。⑩ 6 条回归测试（wrapper 编译、挂载表统一、持久 `/tmp`、按路径 `submit_style`+extra、字符截断、退避与账户错误判定）。

v1.5+（第二十六批·续）：会话 bash 的 Python 环境（用户①，用户选定"配置文件指定解释器 + 宿主侧自动 pip 安装"）——沙箱 `--unshare-net`，会话装不了包（实测样式会话为 `No module named 'PIL'` 手写过 PGM 解析器）。新增 `tools.python`（`enabled`/`mode: system|venv|conda`/`interpreter`/`env_dir`/`conda_env`/`packages`/`auto_install`/`install_timeout`）+ `internal/latex/pythonenv.go`（`Prepare()` 建 venv 并确保模块可导入、`BindArgs()`/`EnvVars()` 让环境在沙箱内可用、`NoteFor(output)` 解析 `ModuleNotFoundError` → 宿主侧 `pip install`（system 加 `--user`）→ 提示重试，失败写 `<项目>/work/python/requirements.txt` + `README.md`）。**坑全在"绑定"上**：解释器不一定在 `/usr`（本机 `~/.linuxbrew/bin/python3`）；venv 的 `bin/python3` 经 `python3.14 → ~/.linuxbrew/opt/… → Cellar` 多跳软链，少绑任一跳就 ENOENT，而 **bash 的 PATH 搜索遇 ENOENT 会静默跳到下一个 python3**（沙箱里于是跑 `/usr/bin/python3`，`sys.prefix=/usr`，装好的包全不在）；共享库也在 prefix 外（`~/.linuxbrew/lib/libpython3.14.so.1.0` 还是指向 Cellar 的软链）。因此 `extraDirs()` 绑"解释器目录 + sys.prefix + 软链链每跳 + ldd 每个库的字面与实际目录 + site-packages"，字面路径不改写（bwrap 不解软链）。6 条测试含 2 条**真沙箱端到端**（配置的绝对解释器真跑起来、真建 venv 且沙箱内 `sys.prefix` 就是它）。配置项三处同步 + `validatePaths` 校验；工具描述自带环境状态行，提示词无需改动。

v1.5+（第二十七批）：压缩真的会触发 + 输出与提示词收敛（用户 9-15 点）——① **`maybeCompact` 每轮生效**（真实缺陷：它只在 `Run()` 入口调一次，而 latex 会话"一次 `Run()` 就是整个会话"，守卫整个生命周期只见过"系统提示词+第一条消息"，实测 prompt 324,178 tokens / 窗口 128K、转录里从无 `=== COMPRESSED SESSION CONTEXT`、最终账户余额不足）。现在阈值检查在请求循环内（每次请求前），配套：普通日志在 50%/80% 打 `[context] 估算 …`（这类 bug 一直看不见就是因为请求大小只在 debug 里）；压完仍超阈值时只有相对上次压缩涨 20% 才再压（避免每轮重复付费摘要）；3 条回归测试（循环内真发生压缩、窗口大时绝不裁剪、续跑只从最后检查点回放）。② **本地裁剪只在接近窗口时发生**（不是每轮实时裁）——已用测试钉住；`keep_images` 只把更早的图换成"曾附过 N 张图"占位。③ **conda 默认 base**（`mode=conda` 全空即 base，`conda_env: base/root` 等价；命名环境先 `<base>/envs/<name>` 再 `conda env list --json`）。④ **`submit_style` 只收路径**，删掉内联兼容（用户：别给 AI 保留它不知道的旧形态）与"路径还是内容"的启发式。⑤ **章节提示词瘦身**（system 9.3K→4.9K、user 1.2K→0.17K，规则重排 1-8，工具参数交给 schema）+ 章节 `compile` 只报"OK+产物+页数 / 错误日志"并新增可选 `engine`/`passes`/`args`。⑥ **终端乱行**：`phaseNote` 改走 `logger.PrintConsole`（原来直接写 stdout，会贴在会话实时行上）；convert 进度行手写的 `"\r%s %d/%d] …"` 多一个 `]` 且管道里裸 CR 连成一行，改用 `liveProgress`；两处实时行 ticker 关闭时汇合（在途一帧不得画在清行之后）。⑦ **style 复用已有图的 LaTeX**（md 里 `<!-- DOCVISION-VECTOR -->` 下的 ```latex 围栏就是本项目真实图代码，example.tex 直接复制，不重新作图）。


v1.5+（第二十八批）：章节会话的临时工作区 + 只交两个路径 + checker 改极简只读会话（用户 16-17 点）——用户问："章节转换会话所在的沙箱应该是 temp 里的临时工作区吧？并且应该包含之前的 cls、操作手册还有 example 文件，他可以在这里面随便操作；提交的时候除了之前那些内容，还需要提交这两个路径——一个 .tex 文件路径、一个文件夹路径，有正式感给 AI，就是他要提交这些里面一般不会有其他内容了。对于 checker 也弄一个非常简单的会话吧，提交的时候就提交现在的情况，默认限制 50 轮就行，只有两个只读文件和一个只读文件夹，工具给基本的即可，能读文件搜索就可以。"

① **章节转换会话的 `work:` 从"正式树的私有视图"改成"自己的临时工作区"**：此前 `ensureChapterView(proj, base)` 铺出 `work/views/chapter_<章>/chapters/`，里面是**指向 `work/chapters/` 的软链**——会话写的每一个探针/实验文件都直接落在正式章节树里（2026-09-11 那轮 `chapter_003/` 里积了 27 个 `probe*.tex`，有的内容还被第二遍覆盖成 `% (scratch probe removed)`），而"能编译"另靠一个与挂载表无关的 `work/temp/conv_<章>` scratch（`chapterScratch`：拷 cls + 软链 images/figures + 写 wrapper），两者对不上：会话在视图里写文件、编译工具却从另一棵树里拷。现在 `Runner.chapterWorkTree(proj, clsName, base, resume, seed)` 一次铺好会话真正需要的整棵树 `work/temp/conv_<章>/work`：`*.cls` + `*.sty` + `manual.md` + `example.tex` **复制**进去（随便改随便试，真样式包不受影响）、`images/`/`figures/` 软链（失败退复制）、wrapper `<章>_wrapper.tex`、以及**预建好的待提交路径** `<章>.tex`（还没写）与空文件夹 `<章>/`。`CompileChapterTool` 随之简化：字段 `Dir` 就是会话工作树（原来的 `Scratch`/`SourcePath`/`WorkDir` 三个目录概念合成一个），默认编译树里的 wrapper；`{path}` 编译树里任意 `.tex`（写成 `<name>_wrapper.tex`，`\input{<相对路径>}` 因此也支持子目录探针——这条是被 `TestCompileChapterToolUsesWrapper` 的探针用例抓出来的）。中断续跑用 `resume=true` 复用同一棵树（不重铺、上次写的 `.tex` 还在）；`keep_temp_dirs`/debug 仍保留备查。样式修复会话（`fixChapterStyle`）改用同一套：建树时用 `seed=work/chapters` 把该章已转换的产物拷进去，会话改完同样交两个路径。

② **`submit` 交两个路径，"交什么书里就只有什么"**：新增 `SubmitChapterTool{WorkRoot, SubmitRoot, Base, ReportPath, ReportOptional}`，参数 `{path, dir, report, notes}`。硬要求：`path` 的 basename 必须是 `<章>.tex`、`dir` 的 basename 必须是 `<章>`（空文件夹也要建，`NOT FOUND` 时提示怎么建）、`report` 必填（`ReportOptional` 只给样式修复会话）。校验链：空参数 → 缺 report → 名字不对 → `resolveInside` 越界 → 文件不存在 → 文件夹不存在/不是目录 → 主文件含 `\documentclass`（wrapper 已提供前导，两处都加载类会炸），全部按具体原因拒绝。落盘共用 `placeChapterFiles(tree, submitRoot, base)`：**只拷这两个路径**，分片文件夹先 `RemoveAll` 再整棵复制（历史残留/探针文件不再留在书里），回执 `SUBMITTED. chapters/<章>.tex and chapters/<章>/ (N files) are in the book.`。会话没显式提交（回合用尽）时的兜底也走同一条路：先在工作树里编译 wrapper，通过才落盘并记 `未显式提交，但编译通过，予以采纳`。转换提示词的 `## Workspace` 因此重写（工作区清单 + "那两个路径就是提交" + 实验文件不进书），`## Submit` 只讲交什么。**提示词去重**（用户复查要求）：清单里不再重复 Read first 已点名的 `manual.md`/`example.tex`，`project:style/` 与 `source/images` 条目删掉（内容已并入 Workspace 的只读清单），"THESE TWO PATHS ARE THE SUBMISSION" 与 Submit 段重复的表述、"scratch 留在后面"的重复句、规则 3 里与 Read first 逐字重复的"原页三步"都收敛成一处（5,769 字符）。

③ **checker 从"一次性 API 调用"改成极简只读会话**：原先 `checkChapter` 把章节 md 与 `.tex` 各截断 20,000 字符塞进一次 `response_format=json_object` 请求，看不到 `work/chapters/<章>/` 里的分片、不能 grep、解析失败或请求失败一律"视为通过"。现在它起一个真会话：`ensureCheckerView` 铺 `work/views/checker_<章>/`，里面**正好两个文件 + 一个文件夹**（`<章>.md`、提交的 `<章>.tex`、`parts/` ← 该章分片目录），挂载表只有这一个**只读** `check:` 挂载点（`Mount{Name:"check", Dir:view}`，不带 `Writable`），工具只有 `read_file`/`grep`/`submit`。结论通过 `SubmitDoneTool{RequireReport:true}` 交回（该工具新增 `RequireReport`/`Status`/`Issues`/`Suggestions` 字段：必须给报告但不落盘，原始字段供 runner 判定），`status=issues` 时把 issues 原样回**同一个**转换会话（最多 3 轮，逻辑不变），`pass`/没交结论/会话报错仍按通过（编译与终审是硬门槛，日志里写明"视为通过"）。提示词新增 `latex_checker.system.md`/`latex_checker.user.md`（注册表 `MustMention: read_file/grep/submit`）。轮数：`latex.sessions.checker.max_tool_rounds` 默认 **50**——`LatexSession("checker")` 里这是**唯一不继承 convert 的字段**（`explicitRounds == 0` 时取 `checkerDefaultToolRounds`，显式写 `-1` 仍表示无限），`default.yaml`/`config.example.yaml` 写明、README 配置表与压缩段都标注了这个例外；`TestCheckerDefaultRounds` + `TestCheckerSessionInheritsConvert`（改成断言 50）钉住。

④ **测试与回执**：新增 `TestChapterWorkTreeIsSelfContained`（类/手册/示例副本齐全、非样式文件不进树、改坏副本不动真包、`resume` 时上次产物还在）、`TestSubmitChapterToolPlacesExactlyTwoPaths`（7 种非法提交各自的具体拒绝语、`\documentclass` 拒绝、只落两个路径、分片文件夹整棵替换、旧残留清掉、草稿不进书、报告落盘、重复提交成功）、`TestCheckerViewHoldsTwoFilesOneFolder`（视图内容恰好 `[<章>.md, <章>.tex, parts]`）、`TestCheckerSessionFeedsBackRealProblems`（假 HTTP 模型服务驱动真会话：先 `read_file` 再 `submit` → issues 原样带回；pass / 没交结论 → 通过；issues 无正文 → 用兜底文案挡住）、`TestCheckerReadOnlyMountRejectsWrite`（工具集就是 `[read_file grep]`）、`TestCheckerDefaultRounds`；`TestChapterViewIsPrivate` 随死代码 `ensureChapterView` 一起删除（`ensureChapterView` 已无调用点，`build:` 挂载点对转换/修复会话也不再存在，README 挂载表相应标注"已不再使用"）。



v1.5+（第二十九批）：限流降 20 + raster 图说清 + 进度行只算本次运行 + 部署配置审计 + 打 v1.5.0-beta.4——用户三点："这个速率限制和余额限制还不一样。可以降到 20 吧"／"需要嵌入的图像怎么办？raster 的图像这边转换章节里面是放在章节的文件夹里面的吗？"／"img2text 这边继续的时候，按照这次的算，不要按照总的加上 done，只看这次的 to process"（实测输出 `Already done: 8666 | To process: 396` 紧跟 `[8874/9062] 97.93% (done: 8874, errors: 0, warns: 0, running: 10)`），另加"看一下我的部署工作区的 config 是否更新到了最新版"。

① **限流重试 100 → 20**：账户级错误（余额/配额）早在第二十六批就改成直接判定 `[SESSION_INSUFFICIENT_BALANCE]` 立即返回、**不走重试**，所以 `rate_limit_retries` 只管真 429；20 次指数退避（封顶 60s）足够自恢复，也让真被限流的运行早点暴露而不是安静熬几小时。改动三处：`session.NewClient` 里 `rateLimit <= 0` 的兜底、`internal/config/default.yaml`、`config.example.yaml`，README 配置表默认值列同步（并写明"余额/配额类不消耗重试"），现场 `config.yaml` 第 147 行显式写的 100 也改成 20（先备份 `config.yaml.bak-rate`）。

② **raster 图的位置（回答"放在章节文件夹里吗"）**：图的真身一直在 `source/images/<书名>/<sha>.jpg`，章节 `.tex` 按 markdown 原路径 `\includegraphics{images/<书名>/<sha>.jpg}` 引用，**既不进章节文件夹也不进提交**；会话里能编译是因为工作树的 `images/`、`figures/` 软链到 `source/` 且 wrapper 带 `\graphicspath{{figures/}}`，`assemble` 建 build 树时把 `source/{figures,images}` 原样复制（相对路径一致），所以引用一路有效到 `out/`。`<章>/` 文件夹的用途是真正需要 `\input` 的分片（超长表格等）。checker 的只读视图里**没有图**（就是"两个文件 + 一个文件夹"），因此 `latex_checker.system.md` 新增一段：图不在视图内、引用图是正常的、**不要报"图缺失/读不到"**（引用有效性由章节自己的 `compile` 保证），并把"坏 LaTeX"里的措辞收紧为"`\includegraphics{}` 空参数才算坏"；`latex_convert.system.md` 也写明"按 markdown 原路径引用、**绝不**把图片拷进提交"。新增真实验证 `TestChapterWorkTreeResolvesRasterImages`（真造 PNG、真 xelatex 编译带 `\includegraphics` 的章节通过、提交后章节文件夹里没有 images）。

③ **进度行只算本次运行**：`img2text.runWorkers` 与 `latex` 档位1 的 `classifyPhase`/`processPhase` 此前把"断点续传已完成的数"（`skipped`/`done0`）混进进度线的分子分母（`total = len(pending)+done0`、`doneCount` 从 `done0` 起跳），于是续跑一开就显示接近 100%，用户看着困惑。现在 `total = len(pending)`、`done = 0` 起，断点基数只出现在 `Already done: N | To process: M` 那一行；渲染抽成 `img2text.progressLine(processed,total,ok,errors,warns,running)`、`latex.classifyProgressText`、`latex.processProgressText` 三个纯函数。回归测试：`TestProgressLineCountsThisRunOnly`（格式与 0/0 不除零）、`TestResumedRunProgressIgnoresAlreadyDone`（真跑 `Run`：1 张历史已完成 + 1 张待处理 + mock 模型服务，抓 stdout 断言出现 `Already done: 1 | To process: 1`、`[1/1] 100.00%`，且不出现 `[2/2]`）、`TestPhaseProgressCountsThisRunOnly`（含 ok 为负时钳 0）。

④ **部署配置审计（只读比对脚本）**：把 `/home/share/samba-share/PDF2MD/config.yaml` 与 `internal/config/default.yaml`、`config.example.yaml` 逐键展开对比——`config_version: 6` 与程序要求一致；**模板里的键部署一个不缺**（没有需要补的新配置项）；部署"多"出来的键全部合法（`latex.sessions.*.max_tokens`、`models.*.thinking` 是自由 map，`clear_thinking`/`type` 写在里面、`tool_stream`/`reasoning_effort`/`verifier.temperature` 都是现役字段），没有死键要清理；值差异属于用户自己的选择（`level: 1`、`concurrency: 5`、`raster_dpi: 180`、`tools.python.mode: conda`、模型与 key、超时放宽），其中两处是模板默认值漂移、已提示用户自行决定：`models.text|checker|classifier.model` 部署仍是 `Qwen/Qwen3.5-27B`（模板已升 `3.6`）、`latex.compile.max_fix_rounds` 部署 8（模板/现行 40）。

⑤ **发布**：`v1.5.0-beta.4`（2026-09-11 打标签，覆盖第十八～二十九批）；CHANGELOG 的 `[Unreleased]` 就地改名为该版本小节（节日期取标签创建日期），AGENTS.md 发布线段落同步。

v1.5+（第三十批）：一次真实运行的三个问题（用户 2026-09-11 18:40 跑档位1 的终端输出 + 日志）——① 进度行重复：`[classify 8/8] …` 与 `[process 8/8] …` 各出现两遍；② 8/8 张图全部 fallback（`[process 8/8] 100.00% (done: 0, errors: 0, fallback: 8, raster: 0, running: 0)`），用户问"为什么全 fallback 了你看看日志"；③ 同批把 v1.5.0-beta.4 打上。

① **重复行的真正来源**：`classifyPhase`/`processPhase` 在 `wg.Wait()` 之后自己 `progress(); fmt.Fprintln(os.Stdout)` 定格，紧接着 deferred `liveProgress.Close()` 又 `\r\x1b[K{line}\n` 重画同一行——终端里就是同一行出现两次（管道里则是"整行 + 空行"）。定格与换行现在**只由 `Close()` 负责**（终端重画+换行；管道里上一次 render 已整行输出过就不再打），另外 `render()` 在管道模式下对**连续相同**的状态不再重复整行。测试 `TestLiveProgressNoDuplicateFinalLine`（含终端侧"必须重画、且只换一次行"的反向断言）。

② **8/8 fallback 的根因**：错误日志里的 400 是 `Failed to deserialize the JSON body into the target type: ***.type: unknown variant \`disable\`, expected one of \`adaptive\`, \`enabled\`, \`disabled\``——现场 `models.drawing.thinking.type: disable`。这个字段原样进请求体顶层，服务端因此拒掉**每一个**绘图会话请求；`[vector]` 的失败分支只把它记成"TikZ 未通过，保留原图"，于是 8 张图各耗 90s（含 2/4/8/16/30s 退避）后全部保留原图，控制台显示 `errors: 0, fallback: 8`，看起来像提示词/编译器问题。修法三层：**(a) 启动校验** `config.validateThinkingTypes`（笔误 disable/enable/off/on/false/true/no/yes/none 自动纠正 + stderr 告警；其余未知取值直接报错，附模型名与合法值），现场配置也已改正并备份 `config.yaml.bak-thinking`；**(b) 日志与计数分清因果**：`isSessionAPIError()` 判定 `SESSION_*`/`api error:`/`会话错误` → 打"会话/接口错误（不是 TikZ 问题）"并计入 `errors`，只有 TikZ 校验失败才算 `fallback`；**(c) 早停**：连续 3 张接口/会话错误且**无一成功**时判定为环境问题（配置/额度/端点），停止开始新任务并明确告知"剩余保持未处理，修好后重跑会跳过已完成"，不再逐张空转。测试 `TestThinkingTypeTypoIsRepairedAndWarns`、`TestThinkingTypeUnknownIsFatal`、`TestThinkingTypeValidValuesKeepWorking`、`TestIsSessionAPIError`、`TestProcessAbortsOnRepeatedAPIErrors`。

③ 顺带确认：`Already done: 8 | To process: 0` 之外的偏好没变；`config_version: 6` 与模板键一致（见第二十九批的配置审计）。

④ **4xx 不再当 transient 重试**（用户追问"那为什么 style 的会话能用？"时顺着日志看出来的第二个浪费）：`thinking` 是**按 models 条目**发的顶层字段——`models.style.thinking.type: enabled` 合法、style 会话 34 次请求全部正常（日志里 `thinking=enabled` × 34，就是"33 轮"那次会话），而 `models.drawing.thinking.type: disable` 非法、8 次绘图请求全被 400（日志里 `thinking=disable` × 8，同一个模型名）。可这 8 个 400 被 `CallWithRetry` 的通用分支当成 transient，按 `api_max_retries: 5` 退避 2/4/8/16/30s 重发——每张图 ~90s、8 张十几分钟，日志里只有一串 `[APIRetry] 等待重试`。现在 4xx（400/401/403/404/422…，排除 408/429）判为**不可重试**，一次到位返回 `[SESSION_API_ERROR: HTTP 400 (不可重试): …]`；余额/配额类错误不再依赖状态码（400/402/429 都认），统一立即 `[SESSION_INSUFFICIENT_BALANCE]`；404/422 仍保留"流式失败 → 非流式降级一次"的既有一致行为（测试里钉住是 2 次而非 5+ 次）。测试 `TestClientDoesNotRetryClientErrors`、`TestClientRetriesServerErrors`、`TestClientBalanceErrorIsNeverRetried`、`TestHTTPStatusCodeParsing`。

---

## 发布线与 CHANGELOG（原文）

**发布线与 CHANGELOG**：v1.5.0 测试线，当前标签 **`v1.5.0-beta.4`**。各标签**实际**覆盖范围（按提交可达性判定，非按文档批次号）：`v1.2.0`＝LaTeX 首版（两档位/会话基础设施/模型注册表/verify，未单独打标签），`v1.3.0-beta`＝自助化 + 样式虚拟工作区 + 字体 + 每章 checker + view_page 按需渲染，`v1.4.0-beta`＝img2text 按类型嵌入 + TikZ 编译校验（仅一个提交），`v1.5.0-beta.1`＝嵌入类型细分/styled 标记/风格统一/preflight/chapter_granularity/水印工作记忆/跨页图表拼接/doc_search/tools 块/配置 v2 等，`v1.5.0-beta.2`＝第一～三批（流式接收+thinking、日志三级 trace、SVG 多后端、编译警告反馈、多工具图片轮 400、文本图只提取文本、预览图、classify 起始行），`v1.5.0-beta.3`＝**第四～十七批**（JSONL 转录起，至 part 级页码定位/临时目录与保留开关/逐章私有工作视图），`v1.5.0-beta.4`＝**第十八～二十九批**。CHANGELOG 已按标签分节（`v1.2.0`（未打标签，附注说明）/`v1.3.0-beta`/`v1.4.0-beta`/`v1.5.0-beta.1`/`.2`/`.3` + 历史各节）：条目按**引入该条的提交**归属到对应标签，节日期取标签创建日期（脚本用 `git log -S` 逐条回溯 + 提交区间映射生成，可复核）；`v1.5.0-beta.2` 及更早标签保持不动。运行目录：`/home/share/samba-share/PDF2MD`（config.yaml 与 docvision 二进制随代码更新）。

## 第三十一批：conda 静默降级、包名映射、项目树铺图（2026-09-11，用户三问）

用户三个问题：① 看看 conda 环境（清华源好像没了，换成别的源）② 样式会话看图到底读没读到像素、为什么给它路径却读不到 ③ `list_source_pages` 的内容是从 md 来的还是 OCR 索引来的。

### ① Python/conda：根因不是镜像，是"配了 conda 却在用 Homebrew python"

现场证据（`latex_project/测试-概率论/work/python/README.md` + 日志）：

```
[python] 会话 bash 使用: /home/lingnc/.linuxbrew/bin/python3
[python] 安装失败: fitz exit status 1 error: externally-managed-environment
[python] 安装失败: PIL exit status 1 error: externally-managed-environment
```

宿主上 conda **是装了的**（`/home/lingnc/miniconda3`，base 里 `PIL 11.1.0`、`fitz` 都能 import），但它只被 `~/.bashrc` 的 `conda init` 挂进交互 shell；服务方式启动的 DocVision PATH 里没有 conda → `conda info --base` 失败 → `mode: conda` **静默降级**成 PATH 上的 Homebrew python 3.14，而那是 PEP 668 externally-managed，`pip install --user` 一律被拒。会话因此拿不到 PIL/fitz，只能 `pdftoppm` + 自己想办法（更早一轮还手写过 PGM 解析器）。

镜像另说：`~/.config/pip/pip.conf` 指向清华（`https://pypi.tuna.tsinghua.edu.cn/simple`）；实测六个源（清华/阿里/中科大/腾讯/华为/官方）`pip download six` **全部成功**，清华本身没坏——坏的是上面那条降级链。按用户要求先换成了阿里云（备份 `~/.config/pip/pip.conf.bak-tuna`），随后用户改主意：**就用清华源**——宿主 pip.conf 已从该备份还原成清华，部署配置的 `pip_index_url` 也写清华。并新增配置项 `tools.python.pip_index_url` 让镜像**写进配置**（宿主 pip.conf 对 DocVision 不可见，镜像坏了只能靠猜）。

三处代码修复（`internal/latex/pythonenv.go`）：

1. `findCondaBin()`：PATH → `$CONDA_EXE` → `~/miniconda3`、`~/anaconda3`、`~/miniforge3`、`~/mambaforge`、`~/miniconda`、`~/anaconda`、`/opt/conda`、`/opt/miniconda3`、`/opt/anaconda3`、`/usr/local/miniconda3|anaconda3` 的 `bin/conda`、`condabin/conda`；`conda info --base` 失败时退回二进制自身的 prefix。
2. `Prepare()` 里 mode=conda 而解析为空 → **明确警告**（"配置了 mode=conda 但没找到 conda…将回退到 PATH 上的 python3"），不再静默。
3. `pipPackageName()` 映射：`PIL`→`Pillow`、`PIL.Image`→`Pillow`、`fitz`→`PyMuPDF`、`cv2`→`opencv-python`、`yaml`→`PyYAML`、`sklearn`→`scikit-learn`、`bs4`→`beautifulsoup4`、`dateutil`→`python-dateutil`、`dotenv`→`python-dotenv`、`serial`→`pyserial`、`OpenSSL`→`pyOpenSSL`、`Cryptodome`/`Crypto`→`pycryptodome`、`pkg_resources`→`setuptools`、`google.protobuf`→`protobuf`、`mpl_toolkits`→`matplotlib`、`docx`→`python-docx`、`pptx`→`python-pptx`、`fpdf`→`fpdf2`（实测 `pip download PIL` = `ERROR: No matching distribution found for PIL`）。点号名先整名匹配再退根模块。
4. PEP 668 兜底：`needsBreakSystemPackages()` 命中 `externally-managed-environment` → 自动带 `--break-system-packages` 重试一次。

测试：`TestPipPackageNameMapping`、`TestPythonEnvBreakSystemPackagesRetry`、`TestCondaFoundOutsidePATH`（假 HOME + 无 conda 的 PATH → 仍解析出 `<home>/miniconda3`；完全找不到时前缀为空，供上层打警告）、`TestPythonEnvPipIndexURL`；旧用例 `TestPythonEnvAutoInstallsMissingModule` 的断言从 `pip install --user PIL` 改为 `--user Pillow`。

### ② 样式会话看图：路径解析没错，是**项目树里根本没有图片**

日志（`logs/latex_20260911_184017.log`）：

```
[18:45:59][T01] [session:style] tool call: view_image {"path": "images/测试-概率论/b6f6f41…jpg", "zoom": 900}
[18:45:59][T01] [style] [tool:view_image] error: 文件不存在: images/测试-概率论/b6f6f41…jpg
```

模型给的就是 md 里的引用路径（完全正确），`ViewImageTool{Root: <proj>/source, Subject: "images"}` 解析成 `<proj>/source/images/测试-概率论/<sha>.jpg`——而**那个目录是空的**。全盘 `find` 显示图片只在 `output/images/测试-概率论/`（全局 `paths.images_dir`）以及旧项目 `latex_project_0909/source/images/测试-概率论/`（真副本，不同 inode，且那个项目的 8 张图当年是**嵌入**分支复制过去的）里。

精确根因（比"没人铺图"更窄）：`copyOriginalImage` 只在**嵌入**分支被调用——`rasterBlock()`（档位1 的 IMAGE 块 / 档位2 的原图引用）与 TikZ 内嵌（`embedBlock` 的 `ClassVector`+`TikzCode` 分支）；而"矢量转换失败 → 保留原图"的 fallback 分支（`runner.go` 的 `rp.p.Status == "fallback"`）只是**保留原引用**再补一条 `<!-- DOCVISION-ERROR … -->`，**从不复制文件**。本次运行 8 张图全是 `thinking.type: disable` 导致的 400 失败、状态全是 fallback（`Already done: 0 | to process: 8`），于是 8 个 `![](images/测试-概率论/<sha>.jpg)` 引用全都指向项目树里不存在的文件：样式会话 `view_image` 报"文件不存在"、章节工作区与 build 树同样无图。

影响面比"样式会话看不了图"更大：`chapterWorkTree` 把 `<proj>/source/images` 软链成工作区的 `images/`，`assemble` 把 `<proj>/source/{images,figures}` 拷进 build 树——两处都指向同一个空目录，所以**保留栅格图的章节在会话内编译与最终成书时都拿不到图**。

修复：images 阶段（`RunImages` 收集完任务后）调用新增的 `linkProjectImages(outDir, imagesDir, tasks)`，把每条被引用插图按 **md 相对路径**铺到 `<outDir>/images/<书>/<file>`：优先**逐文件软链**到实体（`assemble.copyDir` 用 `filepath.Walk` 不跟随目录软链，整目录软链会破坏 build 树；逐文件链接会被 `copyFile` 正常读出），链接失败退回复制；幂等（`isSymlink` 连断链也算已存在）；源缺失只计数不报错。日志一行 `图片已铺到项目 source 树: N 个`。同时 `ViewImageTool.missingHint()` 在找不到文件时回报"目录里实际有什么、你要找的同名文件在哪个可用路径"，让模型当轮自我纠正（曾经的教训是模型以为"这个路径里没有图片"而放弃）。

测试：`TestProjectImagesMaterializedNextToMarkdown`（铺图 + 幂等 + 缺源计数）、`TestProjectImagesSurviveAssembleCopy`（build 树里必须是真文件）、`TestViewImageResolvesProjectPathAndHints`（md 路径可解析 + 错名/错目录的提示含真实文件名）；`TestChapterWorkTreeResolvesRasterImages` 从"测试自己手写 PNG"改成走真实的 `linkProjectImages`（修复前这条链是断的）。

### ③ `list_source_pages` 的数据来源

读代码确认（不涉及改动，答复用户）：工具本身只输出 `pdfView`（原书 PDF 列表 + 全局页码）与 `DocIndex` 的内容；`DocIndex` 由 `buildDocIndex` 从 MinerU 中间产物 `*_content_list.json` 构建（每个 text/image/table/equation 块的局部页码、bbox、文本或 caption），章节起点 `derivedSections` 由版面里的标题类块推导——**都不是从我们生成的 md 正文抽的**。唯一来自 md 的是图片块的补充说明：images 阶段写进 `source/*.md` 的 `DOCVISION-*` 注释按图片文件名接回对应图片块（`Marker`/`Label`/`Content`）。所以改 md 不会刷新索引，重跑 images 阶段才会。

### 第四问（同批）：`view_image`「只给文件名」这条老路子在项目级会话里失效

用户问："之前做的好像是给文件名就可以防止路径问题？也就是说现在这两种读取方法并存吧"。

核查结论：**并存是对的，但有一半是假的**。构造点共 6 处（`grep 'ViewImageTool{'`）：

| 会话 | Root / Subject | 裸文件名 | md 引用路径 |
| --- | --- | --- | --- |
| 作图（tikz，`tikz.go:73`） | 全局 `images_dir` / `<书>` | ✅ 可用（`Subject/<file>`） | ✅ |
| 样式（`book.go:389`、`1422`） | `<proj>/source` / `images` | ❌ 拼成 `images/<file>` | ✅（铺图后） |
| 转换（`book.go:953`）、定向修复（`1309`） | `<proj>/source` / `images` | ❌ 同上 | ✅ |
| assemble/终审（`assemble.go:152`） | build 树 / 无 Subject | ❌ | ✅ |

也就是说：**提示词说"给文件名"，工具只认路径**（`latex_style.system.md` 原文 "use the file name that list_source_pages prints"，而 `list_source_pages` 打印的就是裸文件名；`latex_style.user.md` 也写 "view_image takes the file name"）。作图会话能用，是因为它的 `Subject` 恰好是书目录。

修复：`ViewImageTool` 新增 `BareSearch`，在 `Root+Subject` 内做**深度受限（≤3 层、跳过隐藏目录、不跟随目录软链）的唯一匹配**——唯一命中即用，同名多份报错并列出候选（`文件名 X 不唯一，请给完整路径: images/书甲/x.jpg, images/书乙/x.jpg`），完全不命中才走原来的提示信息。开启于 5 处项目级会话（样式 ×2、转换、修复、assemble），作图会话保持原样（跨书搜索在那里是错误行为，`TestViewImageResolveNoSearchAcrossSubjects` 的契约不变）。

**提示词：一度改多了，被用户打回**。改完 BareSearch 我顺手把 `latex_style.system.md`（两处）与 `latex_style.user.md` 的说法扩写成"两种写法都行"，用户反馈："提示词又开始变多了…style 里面的提示词写的挺清晰的，我觉得没有什么问题。模型自己会明白什么意思，他输入文件名或者路径我们这边不都接受吗？…少即是多。"于是**逐字还原**这三个文件（`git checkout e7a595c^ --`，与改动前完全一致），只保留 `latex_fix.system.md` 里**一行最小修正**：原来只有一句 `view_image: look at image resources`（没说该怎么给路径），改成 `view_image {path}: look at one image — give its full path (images/<book>/<file>) or just the file name.`。结论写进规则：工具同时接受多种写法时**不在提示词里罗列**。

测试：`TestViewImageBareNameResolvesUnderImagesRoot`（裸名/`images/<书>/<file>`/`<书>/<file>` 三种写法指向同一文件；未开启 BareSearch 时不搜索）、`TestViewImageBareNameAmbiguousIsRejected`（同名两份拒绝并列出候选、完整路径仍可用）、`TestViewImageBareNameWorksInBuildTree`（build 树 `images/<书>/<file>` 两级深度也能唯一匹配，根目录文件仍直接命中）。

`pip 源`：用户先把清华换成阿里云，同批又改主意"就用清华源"——宿主 `~/.config/pip/pip.conf` 已从备份 `.bak-tuna` 还原成清华，部署配置 `tools.python.pip_index_url` 也写清华，模板/README 的示例同步为清华。

### 第五问（同批）：README 太复杂太长，改成"介绍 + 构建 + 使用引导"，细节进 docs

用户："README 也太复杂和长了吧。详细的无需在 README 中，在我们 docs 中专门介绍每个不同部分详细内容即可。比如配置文件什么的。README 主要做介绍和引导构建和使用。"

做法（**拆分=逐字搬迁**，不重写）：

| 新文件 | 内容（原 README 行号） | 行数 |
| --- | --- | --- |
| `docs/commands.md` | `## Go 命令行说明`（106-124）、`### img2text 嵌入格式`、`### latex 档位2 文本图嵌入`、`### 日志分析`、`### img2text 测试模式`、`### analyze 选项`、`### 会话预览`（125-222） | 118 |
| `docs/config.md` | `## 主要配置说明`、`### 工具配置（tools:）`（417-454）+ 配置项全表（471-531，原挂在一个不相关的 `###` 下，现补 `## 配置项参考（含默认值）`） | 104 |
| `docs/latex.md` | `## LaTeX 输出`（223-244）、`### 档位1 目录布局`、`### AI 核对`、`### 用法`、`### 风格分析的数据来源`（321-412）、`### 原始文档检索`（455-469） | 132 |
| `docs/sessions.md` | `### AI 会话基础设施`、`### 提示词集中管理`、`### 会话沙箱与虚拟工作区`、`### 调试日志`（245-320） | 78 |
| `docs/dev.md` | `## 目录结构`（533-557）、`## CI/CD`（558-572）、`## Python 脚本独立使用`（413-416） | 46 |

README 575 → 147 行：保留简介、工作流程（5 步 + latex/verify 两条独立命令）、新增「环境要求」、安装与构建（make build / install / setup / init / release / Mermaid）、快速开始（配置最小 YAML + 四条常用命令）、命令一览表、输出与目录、文档索引、CI/CD、许可证；每处细节都带一行链接指向对应 docs。

**核对方式**（脚本）：取 `git show HEAD:README.md` 的每一非空行，检查是否仍逐字存在于 README 或某个 `docs/*.md`——结果 435 行里只有 17 行"非标题行"缺失，全部是安装段/快速开始段/ Mermaid 段的重写（事实逐条保留：`make release` 5 产物、`init` 询问装 Mermaid CLI、`auto/strict/off` 三种校验模式、`-c` 指定配置文件、Python 版本已归档），其余 29 行是标题层级变化。顺手修正两处过期/不准确：README 原没有 Go 版本要求（写 `go.mod` 的 1.25）、我起初把 CI 写成 `.github/workflows/ci.yml`（实际只有 `release.yml`：标签触发、预发布跳过 Release）。

新增规则（AGENTS.md）：README 只写"是什么/怎么装/怎么跑/有哪些命令/去哪看细节"，**配置全表、命令细节、流程内幕、会话与沙箱机制一律进 `docs/`**；新增超过 ~10 行或"查资料"性质的内容一律写进 docs，README 不重复同一件事；搬迁必须逐字并做"原文每一行是否仍存在"的核对。

### 第六问（同批）：CI/CD 为什么 1.1 之后没有构建 + "win 和 linux 的 img2text 不一样吗"

**① 为什么没有 Release**（两问合一，都是"标签没推 + 预发布被跳过"）：

- 远端实际只有 8 个标签：`git ls-remote --tags origin` → `v0.1.0 v0.1.1 v0.1.2 v0.2.0 v0.3.0 v1.0.0 v1.0.1 v1.1.0`，**`v1.3.0-beta`/`v1.4.0-beta`/`v1.5.0-beta.1~4` 从来没推上去**（`v1.2.0` 也从未打过标签）。工作流只在 `push: tags: v*.*.*` 触发，推 master 不构建 → 这些版本自然没有任何 Actions 运行。
- 就算推了，`9d27b5c` 加的任务级 `if: !contains(..., '-')` 会**整个跳过**含 `-` 的标签 → beta 线永远不发产物。
- `v1.1.0` 其实**发了** Release（`gh release list` → v0.1.0 / v1.1.0 / v1.0.1），但 GitHub 把"最后发布"的 `v1.0.1` 标成了 **Latest**，看起来就像"停在 1.0.1"。

修法：删掉跳过条件，含 `-` 的标签照常测试+交叉编译+创建 Release，但 `prerelease: true` / `make_latest: false`；正式标签显式 `make_latest: true`（顺带纠正"Latest 归属"这个坑）。手动补发老标签用 `workflow_dispatch`（它跑当前分支的工作流逻辑，能给旧标签补发预发布）：`gh workflow run release.yml -f tag=v1.5.0-beta.4`。文档同步 `docs/dev.md` 的 CI/CD 段 + README 一行 + AGENTS 发布线纪律。

**②"win 和 linux 的 img2text 不一样吗"**：不是。

- 用户贴的两段都是 **Step 1 切分**的输出（img2text 根本没跑），差别在**状态**不在平台：Linux 那次 `files/` 里有 8 个 PDF（多出 `27考研红宝书` 与 `数据结构` 两本，随后被归档进 `files/done/`），Windows 那次只有 6 个；缓存命中情况也不同。
- 代码里**没有平台分支**：`runtime.GOOS` 只出现在 `img2text/mermaid_test.go`、`cmd/docvision/main_test.go`、`install.go`、`sessionview/serve.go`（测试/安装/开浏览器），切分与 img2text 完全共用一套代码。
- 真正的坑是**切分缓存的键含平台写法**：manifest 落盘的 `source_path` 是调用方给的字符串原样——部署目录里 Windows 写下的 `split_files/2010-26年数一真题套卷[解析]_split_manifest.json`（mtime 09-11 20:12、权限 `-rw-rw-r--`＝Samba 客户端指纹）里是 `files\2010-26年数一真题套卷[解析].pdf`，而 Linux 写的 `测试-概率论_split_manifest.json` 是 `files/测试-概率论.pdf`；`Matches()` 严格比较字符串 → 同一份 Samba 目录在两边轮流跑时**每个平台都会把对方的缓存全部作废**，整库重新 pdfcpu 切分（大书十几分钟 + 重新上传），看起来就像"两个平台行为不一样"。源文件大小/毫秒级 mtime 两边完全一致（实测 `size=184496581`、`mtime_ns=1788440981906804900` 都吻合），只有分隔符不同。
- 修法：`normalizeSourcePath()`（`\`→`/` 后再 `path.Clean`，**不**用 `filepath.ToSlash`——它在 POSIX 上是 no-op，读 Windows 写下的键会漏）用于比较与落盘；旧 manifest 照旧命中。用部署目录里的真文件验证：`source_path = "files\\2010-26年数一真题套卷[解析].pdf" → 归一化 "files/2010-26年数一真题套卷[解析].pdf"`，POSIX 查询命中、`VerifyAgainstDisk = true` ✓。

**规模比想象的大**：部署目录 `split_files/` 里 60 份 manifest，**36 份是 Windows 写的**（`source_path` 带反斜杠）、24 份是 POSIX 写的——也就是说这台机器上两边来回切了很多次，每次切换都会把对方写下的缓存整体作废、重新 pdfcpu 切分一遍。修复后逐一检查：**POSIX 路径查询命中且分片校验通过 60/60，未命中 0**（36 份 Windows 写的全部转成命中）。测试：`TestManifestMatchesAcrossPathSeparators`（双向 + `./` + 重复斜杠 + 不同文件）、`TestManifestNormalizesSourcePathOnWrite`、`TestWindowsWrittenManifestHitsOnPOSIX`（走 `LoadManifest` 的真实落盘往返）。

**实盘补发时又踩到两个真实缺陷**（都以 Actions 日志/注解为证）：

1. `gh workflow run release.yml -f tag=v1.5.0-beta.4` 的第一次运行（run 34599166588）**测试、构建、CHANGELOG 抽取全成功，只有 `Create GitHub Release` 失败**，注解 `⚠️ GitHub Releases requires a tag`：`softprops/action-gh-release` 默认从 `github.ref` 取标签名，而手动运行的 ref 是 `refs/heads/master` → 必须显式 `tag_name: ${{ env.RELEASE_TAG }}`。没有这一行，手动补发这条路根本走不通（且失败发生在构建之后，5 个产物白烧一遍）。
2. `setup-go` 注解 `Restore cache failed: Dependencies file is not found`：`go.sum` 在 `go/` 子目录而不是仓库根，加 `cache-dependency-path: go/go.sum` 才真正命中缓存。

修复后重跑，`v1.5.0-beta.4` 预发布按预期生成（5 平台产物 + CHANGELOG 小节正文、`prerelease=true`）。另外执行了 `gh release edit v1.1.0 --latest`：把 Latest 徽章从"最后发布的 v1.0.1"纠正回语义更新的 v1.1.0。

## 第三十二批：会话用量与时间戳、预览页指标、发布只由标签驱动

**用户原话（三问）**：
1. 「我发现在 process 的最新会话中调用 image content 为空是什么意思？“{}” 就是这个。」
2. 「还有一些基本指标比如每个绘画缓存命中率，输入多少 token 输出多少 token，用时多久什么的。token 的输出速度，首字延迟平均等等。在 html 也应该有一点体现。jsonl 不知道有没有保存相关的信息。应该是时间戳？这样可以直接算出来吧？」
3. 「CIDI 还是随着发布根据标签构建吧，不要你手动构建了。不然污染这个 Action 的历史记录。」

### 第一问：`image_context` 的 `{}` 是**参数**，不是结果

先把部署目录 66 份转录全扫一遍：`image_context` 被调用 28 次，其中 **24 次 `arguments` 是 `{}`**（其余是 `{"image":…,"up":6,"down":6}` 之类）。`{}` 表示**一个参数都没传**——这是合法的：`ImageContextTool.Execute` 在 `args` 为空时用 `target = t.CurrentImg`、`up/down = 10`（默认窗口）。

原始转录证据（`latex_project/测试-概率论_0910_glm/work/views/project/source/sessions/vector_…__Venn_diagram.jsonl`，行 3–5）：

```
行3 assistant tool_calls: view_image{"path":"images/…/b6f6…jpg","zoom":1280} + image_context{}
行4 tool (call_00_…): Image images/…/b6f6….jpg … attached. ORIGINAL FIGURE SIZE: 41.5mm x 22.8mm …
行5 tool (call_01_…): image 7 of 8 in this document: images/…/b6f6….jpg
                       ## PREVIOUS image ref: images/…/2e69….jpg (line 272, -5 lines from this image)
                       … ## THIS image context (up 10 / down 10 lines; request agai…
```

即：`{}` 的调用**拿到了完整上下文窗口**，功能正常。这条统计里另外还看到 `list_source_pages` 12 次、`list_fonts` 3 次、`compile` 87 次也是空参数（同样都是"全部用默认值"的合法写法）。

顺带查到 **3 条空结果**（`text` 为 `None`）：全在 `测试-概率论_0910_glm/work/style_session.jsonl`（行 55/168/180），调用都是 `bash`，命令以 `| grep -E "Overfull" | head -3` 结尾——**命令本身没输出**，所以结果为空。不是工具故障。

> 我第一遍的扫描脚本把字段名写成了 `content`（转录里是 `text`），于是把"有多工具调用的一轮"里配对的 `view_image` 结果误读成空串——脚本 bug，不是产品缺陷；改用 `text` 并按下标取紧随其后的 tool 行后即得上面的原文。

### 第二问：指标——JSONL 之前确实**什么都没有**

改造前转录只有 `t=meta`（系统提示词快照）与 `t=msg`，**既没有时间戳、也没有任何 token 用量**；用量只出现在 `--debug` 日志的 `[DEBUG] … prompt=… completion=… cached=…(N%)` 行里，按会话聚合得靠人肉 grep。所以这一批做了两件事：

**① 写侧（`internal/session`）**
- `transcriptLine` 增 `ts`（RFC3339 毫秒），`Append` 每条消息都带。
- 新增 `t="usage"` 行 + `AppendUsage(UsageRecord)`：`model/stream/round/kind/prompt_tokens/cached_tokens/completion_tokens/reasoning_tokens/duration_ms/ttft_ms/finish_reason`。
- `ChatResponse` 增 `TTFT`：`readStream` 在**第一个** `OnContent`/`OnReasoning` 回调里记 `time.Since(start)`（思考里的第一个增量也算），非流式在 `decode` 里令 `TTFT = Elapsed`（一个 JSON body 无法再分首字）。
- 三处请求都记：主循环（`kind=""`）、空回复后的强制文本请求（`kind="nudge"`，session.go 里 604 行的第二次调用）、上下文压缩摘要（`kind="compact"`）——**漏掉后两者，token 统计与缓存命中率都会偏低**。
- `t="usage"` 与 `t=meta` 一样**永不参与回放**：`LoadTranscript` 只认 `t=="msg"`，续跑语义零变化（新测试 `TestTranscriptWritesTimestampsAndUsage` 里明确断言"回放忽略 usage 行"）。

**② 读侧（`internal/sessionview` + 前端）**
- `UsageStats`（前后端同一套口径）：`CacheHitPct = Σcached/Σprompt`、`AvgTTFTMS = Σttft/请求数`、`OutputTPS = Σcompletion / Σ(duration−ttft)`（用**生成时间**作分母，排队与思考等待不算生成速度）、`SpanMS = 末次−首次 ts`。后端在 `Scan` 的**同一趟单遍扫描**里聚合（`transcriptStat.addUsage`），所以侧栏不需要为每行指标多读一次文件；`/api/index` 与静态导出都带 `stats`，`/api/session` 的 usage 行也带解析好的 `stats`。
- 页面：时间线上系统提示词卡片之后是**「会话指标」卡片**（tiles + 可展开的"每次请求明细"表，`compact`/`nudge` 会在回合列标出），侧栏每行一条用量摘要、侧栏底部**全部会话合计**（加权缓存命中率）、工具栏重复关键几项；`--list` 新增「用量」列。
- **旧转录不显示指标**（`Stats` 为 nil 而不是 0）——0 会被读成"实测为 0"。对应测试 `TestScanWithoutUsageHasNoStats`。

**验证**（真实二进制 + 真实数据，非仅单测）：
- `go test ./...` 全绿；新增 `TestRunRecordsUsageAndTTFT` 用 httptest 起一个"先睡 60ms 再送第一个增量"的 SSE mock，跑完整 `Session.Run` 后断言转录里的 usage 行 `ttft_ms ∈ [40, duration_ms]` 且 `cached_tokens/reasoning_tokens/model/stream/finish_reason` 全部落盘——即 **TTFT 是真测出来的，不是补的**。
- 造一份 6 请求的转录 + 一份无用量旧转录，用**静态导出**喂给一个 Node 里的极简 DOM 影子实现跑真实 `viewer.js`：侧栏合计 `合计 6 次请求 · 输入 183k / 输出 3.5k tokens · 缓存命中 80%`、指标卡片 tiles + 6 行明细（`#2 · compact` / `#2 · nudge` 正常标出）、旧转录那张卡片**不生成**且页面不报错。手算核对：prompt 合计 183000、cached 146000 → 79.8%≈80%；completion 3450、Σ(dur−ttft)=19950ms → 172.9 tok/s ✓。
- 实时服务：`curl /api/index` 与 `/api/session` 都返回带口径说明的 `stats`（`cacheHitPct: 80`、`outputTps: 214.28…`）。
- 改动前先确认**没有别的消费者**按行解析转录：`grep '\.jsonl'` 的其它命中（`internal/latex` 的 tikz/checker/book）都只是拼路径，不解析行内容。

### 第三问：发布只由标签驱动

- `.github/workflows/release.yml` **删除 `workflow_dispatch`**（含其 `inputs.tag`），`RELEASE_TAG` 只取 `github.ref_name`；`docs/dev.md` 的 CI/CD 段改为"只推标签"，README 那句"（或手动 `gh workflow run`）"同步删掉。手动跑过的 run **删除**了失败那一条（`gh run delete 34599166588`，`⚠️ GitHub Releases requires a tag` 那次），Action 历史里只留真实发布。
- 上一批的 beta.4 补发用的是手动 run，产物与预发布状态都正确（`gh release list`：`v1.5.0-beta.4 Pre-release`、`v1.1.0 Latest`）——从这一批起，之后的版本一律"打标签 → 推送"。


## 第三十三批（续）：会话压缩从未触发、样式会话工具集不一致、样式修复无转录

用户在第三十三条反馈里问"会话压缩好像没有触发？""style 重新启用之后…工具 bash 坏了，文件不可读取"。三条都能在真实运行（`logs/latex_20260911_201722.log` + `latex_project/测试-概率论/`）里定位到，证据如下。

### ① 压缩从未触发（真缺陷，代价最大）

- 普通日志里 `[context]` 只出现 **1 行**：`[21:06:49][T03] [convert:chapter_003] [context] 估算 66427 tokens = 窗口(131072) 的 50%`——连 80% 那条都没到过。
- **所有转录里 `=== COMPRESSED SESSION CONTEXT` 计数为 0**（`grep -rl` 扫 `latex_project/测试-概率论/` 无命中），即整个运行一次压缩都没有。
- 同一批 `--debug` 行里厂商实测 prompt 规模：`style-feedback 243,533`、`style 206,886`、`convert:chapter_003 132,063`、`convert:chapter_002 110,842`——**全都超过了配置的 131,072 窗口**，阈值 0.85×131072=111,411 却按本地估算判断，估算最高只报 66,427。
- 估算为什么低：`messageTokens` 只累加 `Content`，历史回传的 `reasoning_content`（GLM 保留式思考，逐字节回传）与 `tool_calls` 的参数 JSON（写文件的整段 TikZ/tex 都在里面）**完全不计**；厂商侧还有图片按像素编码、每轮包装等本地看不见的开销。
- 第二个记账缺陷：`compact()` 在"任务与最近 8 条之间没有可摘要内容"时静默 `return nil`，`maybeCompact` 却无条件 `s.Compactions++`、`s.lastCompactTokens = CurrentTokens()`——短会话会记下一次根本没发生的压缩。

修法：估算补上 `ReasoningContent` 与 `ToolCalls`；阈值改用 `CurrentTokens()` = max(本地估算, 标定估算, 厂商实测 `prompt_tokens`)，标定系数由每次响应的 `prompt_tokens ÷ estBeforeRequest` 得出并夹在 1~10；`recordUsage` 里的标定放在 transcript 判空**之前**（没有转录的会话同样要判阈值）；只有真的压缩过（`compact()` 返回 true）才清实测值/标定并记账；`[context]` 行附上厂商实测值。回归测试 4 条，其中 `TestCompactionUsesVendorPromptTokens` 复刻现场：本地估算几千、厂商实测 130k，必须压缩。

### ② 样式反馈会话缺 bash（用户"工具 bash 坏了"）

`[21:12:04]`、`[21:12:45]` 各一条 `[T01] [style-feedback] [tool:bash] unknown tool`。反馈轮 `styleFeedbackPhase` 复用 `work/style_session.jsonl` 的历史，而那段历史里全是样式会话用 `bash` 探测字体/包/编译的回合；反馈会话的工具表是**手抄的第二份**（代码里还写着"工具集与原样式会话一致"的注释），漏了 `WorkBashTool` → 模型照着历史调用，拿到的是"unknown tool"。现在两处共用 `styleSessionTools(proj, workDir, sourceDir, tag, bashTmp, submit)`，parity 由构造方式保证；反馈轮用独立的 `bash_style_feedback` 临时目录。

### ③ 样式修复会话没有转录

`work/sessions/` 里只有 `chapters.jsonl`、`convert_<章>.jsonl`、`checker_<章>.jsonl`，**没有任何 style_fix 转录**，`work/temp/` 下也没有——而日志里 `[style-fix:chapter_002]` 从 21:17 跑到 21:20、49 轮。`fixChapterStyle` 从不 `SetTranscript`，于是逐章样式修复这段工作**在 `docvision sessions` 里根本不出现**，用户第 11 条"用全跑完的会话分析问题"到这里直接断掉，中断重跑也只能从零开始。现在写 `work/sessions/style_fix_<章>.jsonl`。

### ④ 顺带：目录被当文件读只会回一句 EISDIR

`[21:17:21] [style-fix:chapter_002] [tool:read_file] error … read latex_project/测试-概率论/work/temp/conv_chapter_002/work: is a directory`——原始 Go 错误原样丢给模型，用户看到的就是"文件不可读取"。现在回报"是目录不是文件"并列出该目录真实条目（复用 `suggestInDir`）；`resolve` 里的同类提示不再写"用 bash ls"（checker/style-fix 没有 bash）。2 条测试。

### 关于 grep 工具（用户第 12 条）

`grep` **不是**沙箱里的 shell：`GrepTool`（`internal/latex/tools_work.go`）自己按会话挂载表解析路径（`project:style`、`/work/x.tex`、裸相对路径遍历全部挂载点），再在**宿主**上 `exec.Command("grep", "-rIn", …)`。所以它用的是宿主的 grep 二进制、但跑在沙箱之外、看得到全部挂载点——与 `bash` 里那条 `grep` 的可见范围不同。提示词无需改动（按用户要求未动）。

### ⑤ 控制台进度：从"一条会被覆盖的状态行"改成"每人一行的实时块"

用户第三十三条前三点（进度行重叠、convert 要按子会话多行显示、style-fix 与 style-feedback 来回跳）是同一个根因：

- `logger` 只有一个 `live func()` 槽位，`SetLiveLine` **谁最后装谁显示**。并发的子会话（逐章 style-fix、convert、feedback）各自 `livePhaseLine(label)` 装自己的渲染函数，于是终端的同一行每秒被不同所有者覆盖——用户看到的 `[style-feedback] 轮次 33 · 工具调用 49 · 已用 7m06s` 与 `[style-fix] …` 来回跳、两边时间都在涨，正是"两个所有者各带自己的 start 时间抢同一行"。
- 阶段进度行 `liveProgress`（classify/process/convert 聚合）**另起一套**：自己 `fmt.Fprintf(os.Stdout, "\r\x1b[K…")`，是终端光标的第二个所有者，所以它能盖掉会话行（用户："[convert] 0/4 0.00% … 覆盖了上面的 chapters 行"）。
- 日志行前只补一个 `\n` 再重画：进度行被**永久留在滚动区**，终端里同一行内容出现两次（"重叠"的另一种形态）。

现在 `logger` 提供行式 API：`LiveRow(id)` → `Set/Update/Remove/Finalize`，内部维护 `liveRows`（按创建顺序）与 `liveDrawn`（已画行数），重绘是"上移 N 行 → 清行 → 重画"，日志行与 `PrintConsole` 先擦块、写在块原来的位置、再把块画在下面；管道/重定向下不玩光标，按**变化**追加整行（同一状态不重复，延续第三十批的修复）。`liveProgress` 变成块里的一行（不再自己写 stdout），`livePhaseRow(id,label)` 供 style/chapters/style-feedback/final-review 以及新增的**逐章 convert / 逐章 checker / 逐章 style-fix** 使用。`Finalize` 负责把最后一帧定格成普通行（终端里必打，管道里只在没打过时打），阶段行因此"消失后不留空行、也不重复"。测试：`TestLivePanelNeverOverlapsOwnerRows`（把字节流回放进一个终端模型，断言日志行完整、跑完的行消失、在跑的并排、同一行不出现两个实时行）、`TestLiveBlockKeepsRowsIndependent`（两行互不改动、摘掉的行不会因迟到更新复活）、以及改写后的 `TestLiveProgressTTYCloseRepaints`/`TestLiveProgressPipeNoTrailingBlank`/`TestLiveProgressNoDuplicateFinalLine`/`TestPhaseNoteDoesNotOverwriteLiveLine`。

### ⑥ 模型价格与费用报告（含两个真实缺陷）

用户第 4 条：配置里加模型价格（输入/输出/缓存），按阶段统计用量、命中率、价格、规模，算平均每图/每页成本。

- 配置：`models.<条目>.price: {input, cached, output, currency}`，单位元/百万 tokens。`cached` 不填按 `input` 计（未知折扣不能凭空打折）；`currency` 只在配了价格时才补默认 `¥`。价格全 0 = 未配置 → **整块不显示金额**，因为 `¥0` 会被读成"这次没花钱"。
- 汇总口径：`internal/sessionview/cost.go` 按 `Σcached/Σprompt` 算加权命中率，金额分三段（未命中输入 / 命中输入 / 输出）；`StageCosts` 按阶段分组（`convert`/`checker`/`style-fix`/`style`/`chapters`/`vector`），贵的排前面。数字全部来自转录里的 `t="usage"` 行 → 无需运行期埋点，事后重算结果一致。
- 入口：`docvision sessions --cost`（阶段表）、`--list` 新增"成本"列、预览页侧栏摘要/合计/指标卡「费用」瓷砖；`docvision latex` 跑完自动打印同一份表，并用 `progress_items/` 的图片数与交付 `out/book.pdf` 的页数算出**每张图 / 每页成本**。
- 缺陷 1（页面/报告一直显示"未配价"）：`UsageStats` 的聚合 `addUsage` **漏了 `Model: rec.Model`** —— 费用报告按 t="usage" 行里的厂商模型名查价格表，少了这个字段就永远查无此价，价格配了也一个都用不上。回归测试 `TestUsageAggregateKeepsModelName`。
- 缺陷 2（分组错位）：`LabelFor` 不认 `checker_` / `style_fix_` 前缀，这两类会话在预览页、`--list` 与费用表里都掉进"会话:<文件名>"，按阶段统计会把它们算成"其他"。现在分别是 `checker:<章>`、`style-fix:<章>`（标题"核对 · <章>"/"样式修复 · <章>"）。
- 实测（合成项目 + 临时价格表 input 1 / cached 0.1 / output 4 元/百万）：`--cost` 输出 `style ¥0.20（3 请求，输入 630k/缓存 590k/输出 24k）`、`convert ¥0.15`、合计 `¥0.44`，手算 (630k−590k)×1 + 590k×0.1 + 24k×4 = 195000 微元 = ¥0.195 ✓。
- 另注：用户现场那 46 份转录里 `t="usage"` 行数为 **0** —— 因为 0911 20:17 那次运行用的二进制早于 20:47 引入用量行的提交，不是缺陷；下次运行起就有数据。

### ⑦ 逐图校验（默认关闭）

用户第 9 条：新增默认关闭的逐图校验——生成图与原图（或多页图）比对，一致通过，否则打回上一次 submit 的会话继续。

- 为什么值得做：作图会话**只看得到自己的渲染**，编译干净不等于画对（缺坐标轴标签、版式被重排、内容跑出画布都编译得过去）。只有跟原图比才看得出来。
- 配置 `latex.figure_check: {enabled: false, model: "verifier", max_rounds: 2}`：默认关闭（每张图多一次视觉调用）；`model` 必须能看图，缺该条目时**退回作图模型并在日志里说明**（否则"开了却从没跑过"会变成谜）。
- 流程：`submit` 确认 → 用 `state.lastPDF` **重新栅格化**成校验用 PNG → 校验会话（原图 + 重画图两张图，工具只有 `submit`，转录 `work/sessions/figure_check_<图>.jsonl`，实时行 `figure-check:<图>`）→ 判 issues 就把问题清单打回**同一个作图会话**（`figureCheckFeedback`：只改这几点、编译、再看一眼、重新 submit）→ 下一轮再比。会话跨轮复用，所以第 2 轮看得见自己第 1 轮抱怨过什么。
- 兜底口径与 checker 一致（fail-open）：模型跑飞、不交结论、栅格化失败、打回失败一律**视为通过**并记警告——校验是额外的一张网，不是让整本书失败的新理由。轮次用尽仍不合格：记警告、照常交付。会话没改动图形（`finalCode` 不变）时提前停止，不空烧轮次。
- 提示词新增 `latex_figurecheck.system.md` / `latex_figurecheck.user.md`（注册表含 Vars/MustMention，守护测试通过）；明确"字体/线宽/颜色差异不算问题"——重画本来就是不同字体，判"pass"的门槛写在提示词里。
- 测试：`TestFigureCheckSendsBothImagesAndReadsVerdict`（假模型服务：第 1 轮 issues → 不通过并带回问题清单；第 2 轮 pass → 通过；断言请求里**两个 image part** 且后续轮次说明"同一比较的第 N 轮"）、`TestFigureCheckFailsOpenWithoutVerdict`、`TestFigureCheckFeedbackQuotesProblems`、config 侧 `TestFigureCheckDefaultsAreOffButSane`。

### ⑧ 会话预览页：项目 → 流程阶段 → 会话，逐图会话按图片身份命名

用户第 8 条：左侧按项目 + 流程阶段组织会话，可展开、显示当前进展/完成情况；process 阶段按图片名/PDF 页/顺序/图片类型命名与检索。

- 侧栏三层：项目组（书名 + `progress.json` 各阶段状态行）→ 阶段子组（`矢量图`/`样式`/`章节划分`/`章节转换`/`章节核对`/`样式修复`/`逐图校验`，组头带会话数、阶段状态 ✓、live 点）→ 会话行；`<details>` 各自折叠，过滤命中强制展开（沿用原有记忆逻辑）。
- 逐图会话的"图片身份"来自 `doc_index.json`：转录文件名 `vector_<书>__<图片名>__<标签>` 的**倒数第二段**就是图片名（按 `__` 切，不能按 `_`——标签里带下划线），拿它去匹配 doc_index 图片条目的 `img` 文件名（MinerU 的图片名就是内容哈希）→ 得到页码、书内序号、类型、图注。行标题用图注，行内标签 `第 N 页 · 第 N 张 · 类型 · 哈希前 12 位`，排序按书内图片顺序。
- 搜索串加入 `imageName/imageType/imageCaption` 以及 `p12`/`p.12`/`页12`/`#3`/`第3张`/裸数字，用户要的四种检索方式（图片名/PDF 页/顺序/类型）都能用。
- 数据层：`SessionInfo` 新增 `stage`/`stageTitle`/`imageName`/`imageOrder`/`page`/`imageType`/`imageCaption`/`projectStages`；`enrichSessions` 在扫描末尾统一补齐（`loadProjectFacts` 读 `progress.json` + `doc_index/doc_index.json`，按项目目录缓存；`projectDirFor` 从转录相对路径推出项目目录）。静态导出与实时服务共用同一份 JSON，无需改 API 形状。
- 真缺陷（本批自查发现并修掉）：上一批给侧栏行加费用显示时，`statsSummary(st, session)` 里传了一个**在 `sessionRow(s)` 作用域里不存在的标识符** `session`（应为 `s`）——费用列在侧栏等于永远为空。已改回 `s`。
- 测试：`TestEnrichSessionsAddsStageProgressAndImageIdentity`（真实目录结构：progress.json + doc_index + 四种转录名 → 断言 stage/页/序号/类型/图注/项目进展，以及阶段分组按流程顺序）、`TestVectorImageBaseParsing`（含带下划线标签与不足三段的情况）。

### ⑨ 会话预览页：调用卡片按工具家族着色 + 一行摘要（对照 DSH 会话界面）

用户第 6 条：UI 参考 DSH 整体界面（左侧项目、中间调用样式、CSS、查看调用过程）。

- 现状盘点：预览页本来就是"左侧项目树 + 中间消息/调用时间线 + 指标卡片 + 静态导出/实时服务"，"仅看工具调用"开关与 `#序号` 配对的调用/结果卡片也早就有；本批按 DSH 的会话界面补上最影响"看调用过程"的两点。
- `toolFamily(name)`：shell（`bash`/`compile`/`python`）/ write（`write_*`）/ view（`read_*`/`view_*`/`image_context`）/ search（`grep`/`doc_search`/`list_*`/`*search*`）/ submit / other；卡片左侧色条 + 工具名按家族着色（CSS `fam-*`）。
- `toolSummary(name, argsText)`：折叠状态显示一行摘要——`bash`/`python` 取命令**首行**并压掉空白，`compile`/`write_*`/`read_*`/`view_*` 取 `path`，`grep`/`doc_search` 取 `pattern`/`query`，`submit` 取 `path`/`status`，其余工具兜底取参数里第一个非空字符串；超 90 字符截断加省略号，标题悬停看全文。
- 证据：把这两个函数从 `viewer.js` 按大括号配对**原样抽出**跑 node 校验——7 个工具名的家族判定全对、`{"command":"ls -la\nrm -rf x"}` → `"ls -la"`、坏 JSON → 空摘要（不抛错）、200 字符命令 → 91 字符（90 + 省略号）。
- 未做：不追求逐像素复刻 DSH（那是另一套前端）；如需要具体某个 DSH 组件的等价物，按组件名单独提。

### ⑩ 配置版本 6 → 7 + 部署配置补上新块（用户复查发现的漏项）

用户问"config 变化版本升级了吗？我的部署那边的 config 新增这些新的内容了吗？"——两处都没做，是本批的真实漏项。

- **漏项一：没 bump `config_version`**。本批新增了三个配置块（顶层 `preview.*`、`latex.figure_check.*`、`models.<条目>.price.*`），但 `CurrentConfigVersion` 仍是 6。按仓库既有约定（v2→v3 加 `tools` 块、v3→v4 `tikz`→`latex` 改名、v4→v5 嵌入规则、v5→v6 流式/thinking）新增配置块就该 bump：版本号是用户发现"模板过期、有新选项没跟上"的**唯一信号**。现改为 7，并同步 `default.yaml`/`config.example.yaml`/README 示例行/测试夹具/`docs/config.md`（新增 `config_version` 行）；AGENTS.md 规则板块补上"新增配置块必须同时 bump"这条纪律（含本批教训）。
- **漏项二：部署目录的 `config.yaml` 没有任何新块**。核对部署配置：`config_version: 6`、无 `preview:`、无 `figure_check:`、无 `price:`。现按 AGENTS.md 的"运行目录 config.yaml 与二进制随代码更新"补齐：备份 `config.yaml.bak-<时间戳>` 后，`config_version` 改 7、顶层插入 `preview:`（关闭/127.0.0.1/8848）、`latex:` 末尾插入 `figure_check:`（关闭/verifier/2 轮）、8 个 `models` 条目各补 `price:` 占位（全 0 = 未配置）。补丁用文本级插入（不经过 YAML round-trip，避免注释与顺序被重写），全程不打印任何密钥值。
- **顺带修一处真实缺陷**（这次核对时暴露）：单价"存在但全是 0"时，报告写"配置里没有任何 models.*.price"（措辞不实），且合计行打印 `0.00`、每图/每页打印 `0.0000`——`0.00` 会被读成"几乎没花钱"，真实含义是"没配价、算不出来"。现在 `currency` 为空（没有任何一次请求命中已配置单价）时，金额列/合计/每次请求/每图每页一律写 `-` 并说明原因；提示语改成"没有**可用**的 models.*.price（没写，或单价全是 0）"。新增 `TestCostReportShowsNoMoneyWhenUnpriced`（未配价输出不含 `0.00`、必含"未配价"与全零说明；配价后照常显示 `¥0.12`）。
- 验证：部署二进制 `v1.5.0-beta.4-28-g53669ef` 用补好的配置加载**无版本告警**；`sessions --cost` 输出全部为"未配价 / -"，无 ¥0.00；`go vet ./...` 干净、`go test ./...` 全绿。

### ⑪ doc_index 的 text：厂商没图注时用我们自己的内容兜底

用户复查第 13 条的答复时指出两点：（a）"vector → figures/*.svg；raster → 保留原图链接"这句表述有问题，"都应该是原图链接"；（b）"text 是否是对应的内容"，raster 在我们生成的 md 里就是解释内容，这边就该放解释。

- **（a）是我在回复里把两件事混成了一句话**：`figures/*.svg` 是**档位2 正文**的嵌入方式（档位2 的产物就是"用重画结果替换矢量图"）；而**链接**（档位1 md 里的 `LINK: [class](path)` 与 `doc_index.json` 的 `img`）三种类型**一直**都走 `copyOriginalImage` 指向原图 `images/<主题>/<sha>.jpg`，从不是生成物——已核对 `runner.go` 三处 LINK 构造（styled-text/vector/image）与 `docindex.go` 的 `markerKey`（按原图文件名 join）。文档同步补一句"链接一律指向原图，`figures/*.svg` 只是档位2 正文的嵌入"。
- **（b）是真缺陷**：`DocEntry.Text` 只取厂商的 `image_caption`/`image_footnote`（表格取 `table_caption` / 正文摘要），厂商没给就留空——于是"这张图对应什么内容"在 `text` 上是空的，我们的解释只躺在 `content` 里。现在 `applyMarkers` 里加兜底 `markerFallbackText`：`text` 为空时用我们写在图上的内容填——styled-text → 印刷原文、raster → 生成的解释、vector → **图标签**（不塞 LaTeX 本体，整段代码放"图注"位置会把检索结果淹没）；厂商给了图注的不动，我们的内容仍在 `content`，两边都可检索、来源不混。
- 测试 `TestDocIndexTextFallsBackToOurOwnContent`（四种图块：无图注 raster/styled-text/vector 各取对应内容、有厂商图注的不被顶掉且 content 仍在；顺带断言三种类型的 `Img` 都是 `images/` 下的原图、绝不是 `.svg`；兜底后的 text 仍可被 `Search` 命中）。既有 marker 测试全部保持通过。

### ⑫ 会话预览页按 DSH 的界面重做视觉（+ 静态快照丢字段的真缺陷）

用户反馈："我们的界面我的意思就是让你仿照 DSH 的界面做，就是现在这个 AI 的界面样式。之前那个太难看了，并且显示的时候很多没有那么自然内容。并且我刚刚说的很多界面 ui 上的描述这边在原有的基础上不好落实。"

**先说找到了什么真缺陷**：用户说"UI 上的描述落实不了"不是主观感受，而是**静态导出把新字段丢了**——`viewer.js` 的静态模式把每个会话的字段**逐个手写列举**进 `state.sessions`（注释里甚至已经写着"这里漏一个字段，页面就会静默少一块 UI（曾漏掉 projectLegacy）"），于是第七～十批加进去的 `stage`/`stageTitle`/`projectStages`/`imageOrder`/`imageName`/`page`/`cost` 在**静态页里全都不显示**：侧栏阶段分组、书级进展行、逐图会话的"第 N 页 · 第 M 张 · 类型 · 哈希"、费用列统统缺失，而 `--serve` 实时模式正常。用户平时看的就是静态页。修法：改成"**只减字段**"——`Object.keys(s)` 整份带走、只把 `lines` 大块丢掉，并把这一写法钉进 `TestViewerAssetsThemeAndMeta`（既断言新写法在，也断言手写白名单不再回来）。

**视觉重做**：`viewer.css` 整份重写（399 行，浅深两套 token），取向是"扁平、安静、信息优先"：

- 分层只用极浅底色差 + 1px 边框，去掉重描边与厚阴影；卡片圆角统一 10px、行内小元素 6px。
- **助手消息不再套卡片**：正文直接排（13.5px/1.68），只有机器内容进等宽块；用户消息是唯一带浅底色的一侧（并去掉卡片里重复的"用户/任务"标题——上方角色行已经说了是谁）。
- 角色行改成极窄的一行标签（`#序号 · 用户/AI/工具 · 行 N · 思考/工具/图片`），"谁在说话"靠它和缩进区分，而不是成片彩色块。
- 工具调用/结果：无边框行 + 折叠，左侧一条 2px 轨道按**工具家族**着色（`bash`/`compile` 橙、`write_*` 蓝、`read_*`/`view_*` 绿、`grep`/`doc_search`/`list_*` 紫、`submit` 红），折叠态带一行摘要（命令首行/`path`/`pattern`/查询词），结果按 `ok`/`error` 给轨道染色。
- 侧栏：三层全改成无边框行（悬停浅底、选中淡蓝底），阶段组头带该阶段状态、书级进展单独一行、逐图会话的身份小标签照旧；底部合计与脚注压成两行。
- 指标卡改成瓦片网格（`repeat(auto-fit, minmax(140px, 1fr))`，格线用 1px 底色差而不是边框）。

**校验方式（这次的证据）**：装了无头 chromium，直接把静态页渲染成 PNG 回头看——`chromium --headless=new --no-sandbox --user-data-dir=… --screenshot=… file://…/page.html`，浅色与深色各截一张（`data-theme="dark"` 注入副本）。这一步立刻暴露了另一个我自己的错误：我手写的**预览数据用了转录的原始行形状**（`{"t":"msg","message":{"role":…}}`），而页面真正吃的是 Go 侧加工后的 `Line`（`{n,t,role,text,tool_calls,tool_call_id,reasoning_content,stats}`）——用错形状时主区域显示"这个会话还没有可显示的消息"，看起来像页面坏了，其实是**我的预览数据不对**。按真实形状重做预览数据后才看到真正的页面。

### ⑬ 会话预览页改成 DSH 的**组织方式**（三栏 + 页签 + 轨迹表 + 详情栏），并修掉"侧栏被刷新冲掉"

用户看到 ⑫ 的成品后直接否掉了它："感觉还不如之前的 css 好看，这还是之前的样式。完全没有和正常的 dsh 一样的界面。并且左边那个展开总是被刷新然后自动展开。默认展开。**我要的不是样式。是结构界面组织方式。和 dsh 一样高效。更具展现力**。"

这一句把上一批的错判说清楚了：⑫ 改的是**配色与卡片外观**（CSS 层面），而用户要的是**信息怎么摆**（结构层面）——DSH 之所以"高效"，是因为它把三层信息分流到三个栏位、把过程性内容收成一行、给出一条可筛选的事件表，而不是因为它用了什么灰阶。于是这一批去读 DSH 自己的前端源码（`~/.nvm/…/@deepseek-ai/dsh/node_modules/@deepseek-ai/dsh-client-ui-*/lib/client.js`，把其中的 `css` 字符串抽出来当参照），逐项对齐它的布局常量与行词汇。

**① 先复现用户报的缺陷（"左边那个展开总是被刷新然后自动展开"）——是真缺陷，而且有两处**

- `--serve` 每 2 秒轮询 `/api/index`，`applyIndex()` → `refreshList()` 无条件调 `renderSessions()`：`clear()` 掉整个 `#session-list` 再重建。手动折叠的项目组/阶段组**每次都被还原成展开**，滚动位置被弹回顶部，鼠标悬停状态被清掉。
- 更隐蔽的第二处：列表签名里混进了 `Object.keys(state.collapsed)` 与 `Object.keys(state.overflow)`——**视图状态写进了数据签名**，于是"点一下组头折叠"本身就把签名改掉，下一轮询必然重建一次。就算重建能还原状态，这一次无谓重建也会吃掉滚动位置与悬停。
- 修法：签名只统计**会改变行集合或可见文案**的东西（会话 id 顺序 + 消息数 + 费用 + 请求数 + 项目的 `progress.json` 快照 + 过滤词）；签名不变走 `patchList()` —— 只改相对时间、活跃圆点、用量 chip、悬浮说明与选中高亮，**一个 DOM 节点都不新建/搬动**；签名变了才重建，重建后按落盘的 `state.collapsed` 还原各组展开状态并恢复 `scrollTop`（`refs.list.scrollTop = scroll`）。`mtime`/`live` 也移出签名：它们每 2 秒都可能变（`live` 会跨过 60 秒的 `LiveWindow` 边界），唯一的可见表现就是行尾时间与活跃圆点，补丁即可。

**② 用 CDP 做端到端验证（不是"看代码觉得对"）**

起 `docvision sessions --serve`，用 headless chromium 带 `--remote-debugging-port` 打开实时页面，再用 node 的内置 `WebSocket` 直连 CDP 跑 `Runtime.evaluate`。探针做四件事：选中一条会话 → 折叠最后一个阶段组 → 把侧栏滚到底部 → 静默观察 3 个轮询周期 → 从**外部**往 jsonl 追加一行 usage（逼下一次轮询重建）→ 再观察。

实测（脚本 `temp/dsh-ref/live-test.sh` + `live-probe.mjs`，跑完即删）：

| 断言 | 结果 |
| --- | --- |
| 折叠一个阶段组 | `data-open="0"`，落盘 `{"stage:latex_project/book_alpha/checker":true}` |
| 静默 3 个轮询周期（6.5s） | `sameRowNodes: true`、`sameStructure: true`（挖掉相对时间后的 innerHTML 逐字相同） |
| 静默期的折叠状态 / 滚动位置 | 仍折叠（`"0"`），`scrollTop` 仍 317 |
| 外部追加一行 usage | 下一轮询（`waitedPolls: 1`）检测到重建 |
| 重建后的折叠状态 / 滚动位置 / 落盘 | 仍折叠（`"0"`），`scrollTop` 仍 317，落盘值不变 |
| 「更多会话」溢出按钮 | 19 条会话的阶段只列 8 条 + 1 个按钮 |

这一步顺带逼出三个**只有真跑才会发现**的问题（都不是"看代码觉得对"能发现的）：

1. **详情栏渲染直接抛异常**：`kvList` 里写成 `el('dd' + (p[3] ? ' mono' : ''), …)`，`createElement('dd mono')` 抛 `InvalidCharacterError`。它是 `toggleDetails()` 里 `renderDetails()` 的第一句，异常一抛，后面的 `applyLayout()` 就再也执行不到——**点 ⓘ 完全没反应**，而页面其它部分看起来完全正常。截图里"详情栏一直不出现"就是这么来的。
2. **窄窗口点开详情栏静默失败**：1180px 下 `computeColumns` 的让步链算出详情栏放不下（280+300+640 > 1180），于是 `state.details` 被设成 400 但实际宽度 0。DSH 也是这个行为（照搬），但"点了没反应"是坏的：现在放不下时顺手把侧栏折成 56px 轨道再解一次（1180−56−300 = 824 ≥ 640，放得下）。
3. **截图本身不可信**：浅色/深色/详情三张 PNG 的字节数**完全一样**。原因是共用一个 `--user-data-dir`，`localStorage` 里上一轮存下的 `dsh.sessionview.theme='light'` 覆盖了注入的 `data-theme="dark"`，而 `layout.details` 也在变体之间串味。改成每个变体一个 profile 目录、并给深色变体加一段 `localStorage.getItem` 垫片后才拿到可信的对照图。**这条教训值得单记：用截图做验证时，先确认截的确实是你要的那张图。**

**③ 结构改造（逐项对照 DSH 的常量与行词汇）**

- `viewer.html`：三栏骨架（`#sidebar-col` + `#handle-sidebar` + `#center-col` + `#handle-details` + `#details-col`），中栏表头是"面包屑行 + 页签行"，页签行右端挂四个开关按钮。
- `viewer.css`：`.frame` 用 `grid-template-columns` 布三栏（`transition` 见 DSH 的做法），分隔条 8px、`margin-left:-4px`、`cursor: col-resize`、悬停显一条 3px 圆角条；侧栏折叠态只留插槽（这版专门修了折叠态里 chip / 阶段状态 / 根路径**溢出到轨道外**的毛病）；`.tabs { gap: 36px }`、`.tab::after` 2px 下划线、`.crumb { max-width: 220px }`；`.stream { max-width: clamp(680px, 64%, 920px) }`；`.msg-user .bubble` 22px 圆角 / 10px 16px 内边距 / 最宽 82%；`.io-card` 0.5px 边框 + 代码底色 + 12px 圆角 + 每段内滚动 260px；`.traj-table` 固定表格布局、表头 30px 吸顶、`.kind-tag` 19px 高。
- `viewer.js`：`computeColumns()` 是 DSH 让步链的**逐字实现**（纯函数，无迟滞，重新变宽自动恢复）；`wireHandle()` 用 pointer capture 拖拽、双击复位；`renderTrajectory()` / `jumpToLine()` 做轨迹表与"点行跳回对话"；`renderDetails()` / `detailsSessionBlock()` / `detailsStatsBlock()` / `metaCard()` 把元信息与指标搬进右侧；`bindCollapse()` 用 `node.dataset.open` 区分"程序化设置 open"与"用户点击"，只有后者才写回记忆（否则每 2 秒的重建会通过 `toggle` 事件把用户的折叠选择改坏）。

**④ 有意保留的偏离（必须在报告里说清）**

- DSH 的侧栏只有"项目 → 会话"两层，本项目有"项目 → 流程阶段 → 会话"三层（阶段分组是既有功能，不能为了像 DSH 而删）。折中是三层**都用 DSH 的行词汇**（34/28/32px、`padding: 0 8px`、圆角 8px、16px 固定插槽），并**取消按层级递增的缩进**——层级靠插槽内容与底色区分。
- DSH 的会话行只有"标题 + 时间"，没有 chip。本项目要求"侧栏每个会话行有用量摘要 + 费用"，折中是保留两张**最小** chip（`14.5k / 6.2k` 用量、`p7·#3·table` 图片身份），完整口径（含"含系统提示词快照"那句）进整行的 `title`。
- 每次请求明细表原本有 8 列（回合/首字/耗时/输入/缓存/输出/速度/结尾），在 300–520px 的详情栏里挤成换行；改成 5 列（回合/首字/耗时/输入·缓存/输出），模型、tok/s、结束原因、时刻移进行悬浮说明。

**⑤ 验证与测试**

- `gofmt -l .` 干净、`go vet ./...` 干净、`go test ./...` 全绿。
- 测试同步更新（**没有为了过测试删功能**）：`TestViewerAssetsThemeAndMeta` 里四处因结构改变而失效的断言改成新形态（思考折叠行的 `open` 表达式、meta 卡片改挂详情栏、分组折叠改走 `bindCollapse`），并新增 `TestViewerMatchesDSHStructure` 钉住三栏骨架、分隔条与布局常量、面包屑/页签、轨迹表与跳转、详情栏、消息形态、以及**轮询守卫**（`if (sig === state.listSig && refs.list.childElementCount) { patchList(); return; }`；签名函数体内不得出现 `state.collapsed`/`state.overflow`/`s.mtime`/`s.live`；重建后必须有 `refs.list.scrollTop = scroll`）。
- 截图矩阵（浅色/深色 × 对话/轨迹/详情/展开工具卡/展开轨迹行/类型筛选/窄窗口轨道/展开元信息）逐张回看，`ui-preview/` 与 `temp/dsh-ref/` 用完即删。

## 第三十四批：会话预览页十条要求（左对齐 / Markdown / 高亮 / 统一字号 / 折叠 / 系统消息 / 轨迹页保真 / 图片归属）

用户在这一批给的是一串**逐条验收意见**（① 侧栏默认收起 ② Markdown 预览 ③ 图片名短写 ④ 消息全左对齐 + 合并 ⑤ JSON 高亮 ⑥ 工具展开区照思考块 ⑦ 高亮补齐 + 字号统一 ⑧ 输入输出字号一致 ⑨ 任务提示折叠 ⑩ 按系统消息呈现 ⑪ 轨迹页保持真实），外加一条**追加的 wire 事实**（工具图片只能走 user 消息）与第 8 轮的"图片归属"。十条要求**一起交付**，全部落在 `go/internal/sessionview`（页面）与 `go/internal/session`（归属句柄）两处，没有新配置项、没有新依赖（页面仍然是 `//go:embed` 的纯原生 JS，无 CDN、无构建步骤、无第三方库）。

### ① 侧栏三层默认收起（用户第 1 条）

三层（项目 → 阶段 → 会话）此前是 `open` 写死在 HTML 里的"默认全展开"，重开页面永远全开，用户要的是"记住我的选择、默认不展开"。改为三态记忆（`collapsed` 键：`'1'` 手动展开过 / `'0'` 手动收起过 / 缺失 没碰过）：只看"手动展开过"的组才开；**切换会话时**装着当前会话的那一组自动展开（这是"点完能看见"的必要让步），但只要手动收起过就尊重选择。关键是**记忆键不能进列表签名**——签名是实时轮询的最小修补守卫（`if (sig === state.listSig …) { patchList(); return; }`），把 `state.collapsed`/`state.overflow` 放进签名，点一下组头就会让整份侧栏作废重建，等于把"不刷掉用户操作"修复再推翻一次；测试里钉住 `listSignature()` 函数体内不得出现这两个字段。

另一个已知行为记在这里：**切换会话会把"自动展开过"的组收回去**（没记忆就是收起）——这是"默认全部收起"的必然结果，不是 bug。

### ② Markdown 预览（用户第 2 条）

`renderMarkdown(text) -> DocumentFragment`：自己写的小渲染器（标题/粗体/斜体/删除线/行内代码/围栏代码块 + 语言标签 + 复制按钮/有序无序列表/引用/水平线/链接/`|` 表格/段落与软换行），**不使用任何库，也从不拼 HTML**——只 `createTextNode` + `document.createElement`。页签行右端新增 `Markdown` 开关（默认开，落盘 `dsh.sessionview.markdown`），应用到助手正文、user 轮正文、系统提示词快照；思考与工具输入输出**保持等宽纯文本**（它们是机器内容）。

两个刻意的偏离（要写清楚）：① **没有独立的"先转义再解析"步骤**——DOM-only 渲染里 `<script>` 落树就是文本，等价且更安全；先转义再解析反而会把 URL 里的 `&`/引号弄坏。测试同时钉住"源码里没有 `.innerHTML`"与真实 chromium DOM（`<script>` 出现在系统提示词里时仍是文本）。② 链接协议只放行 `http(s)`/`mailto`/锚点/相对路径，一律 `target="_blank" rel="noopener noreferrer"`。

页签行的开关与语法高亮**互不相干**：关掉 Markdown 只是回到纯文本，JSON/终端着色照旧（这条也有测试）。

### ③ 逐图会话按图片短写命名（用户第 3 条）

`internal/sessionview/stages.go` 的取名规则改为**图注 → 转录文件名里的可读标签 → 前 8 位内容哈希 + 原扩展名**（`imageShortKeep = 8`，同一本书里撞车退化到 `imageShortFallbackKeep = 12`，再撞才写全名），**64 位哈希永远不当标题**；完整名 + `images/<书名>/<文件>` 路径进行的悬浮说明。命名在 Go 侧算一次、`--list` 与页面共用（`SessionInfo.ImageLabel`），搜索对短名、完整哈希、图注、页码、序号、类型都命中。注意 `vectorImageParts` 只认 `vector_<book>__<imagebase>__<label>.jsonl` 三段式 stem——夹具若用 `vector_<hash>.jsonl` 这种裸哈希名，短名逻辑**理应什么都不做**（不是失败）。

### ④ 消息全左对齐 + 一次调用一行（用户第 4 条）

右侧浅蓝气泡那套**整段删除**（`.msg-user`/`--user-bubble` 相关 CSS 与 `role` 分支一起清掉）：不用右侧对齐是因为在这个工具里 user 轮不是人打的字（见 ⑩）。`role:"user"` 分两条路：带图 → 图片投喂/工具图片回执（见第 8 轮的归属）；不带图 → 系统消息。工具调用改为**一次调用一行**（`工具名 · 摘要 · 输入→输出 字符数 · ok/error`），回执按 `tool_call_id` **合并**进同一张卡片的三段（输入/输出/附件），配不上的回执单独成行并注明「未配对的工具回执」。

### ⑤⑦ 高亮（用户第 5、7 条）

JSON：`prettyJSON()`（先 `JSON.parse` 再 2 空格 `stringify`，解析失败就返回 null → 保持原文）+ `appendJSONSpans` 正则产出 `span.k/s/n/b`（复用既有的 `--key/--str/--num/--bool` 四个 token），应用到工具输入、工具回执、元信息里的 `parameters`、系统提示词里的 JSON 片段、轨迹展开正文。终端：`consoleLineClass()` 逐行判定 + `CONSOLE_TOKENS` 逐 token 判定——行首命令行（`$ `/`# `）加粗、diff 的 `+`/`-`/`@@`、`error`/`FAIL` 红、`warning` 黄、`OK`/`PASS`/`COMPILE OK` 绿、LaTeX 的 `!` 错误行红、`Overfull`/`Underfull` 黄、路径与 URL 弱强调；输入侧首行给 `$ ` 前导提示符（命令行优先，但**不比输出更亮**）。超过 200 KB 或 4000 行**退回纯文本 + 一句提示**（`JSON_HL_MAX_CHARS`/`JSON_HL_MAX_LINES`）。踩到的坑：早期的高亮函数吐出 `key/str/bool/num` 类名，而 CSS 定义的是 `.k/.s/.n/.b`，于是"高亮看起来没生效"——测试现在直接断言旧类名不存在。

### ⑥⑧⑨ 折叠与字号统一（用户第 6、8、9 条）

- **一套字号刻度**：`:root { --code-font: 11px; --code-line: 19px }`，工具输入、工具输出、思考正文、JSON 高亮、行内 `code`、代码块全部 `var(--code-font)`/`var(--code-line)`；只有次级说明（`25 → 138 字符`、`ok`/`error`）留 12px。测试断言这些选择器都引用同一个 token（而不是各写各的 `11px`）。
- **同一套折叠**：`.io-scroll`/`.reasoning-scroll`/`.prompt-scroll` 共用 `--code-scroll-h: 340px`，`foldLabel(expanded, lines, chars)` 是**「展开全文（N 行 / M 字符）」/「收起」文案的唯一来源**，思考块、工具输入输出、系统消息块、Markdown 长文本全用它；`IO_FOLD_LINES = 16`（16 × 19px ≈ 304px < 340px）保证按钮在触到限高**之前**就出现。
- **系统消息一层折叠**：`systemTurnSection` 最初套了 `bodyBlock()`，于是出现**两个**「展开全文」按钮（外层 `sys-scroll` 一层、内层正文一层）；现在系统消息自己渲染正文（Markdown 或 `pre`），只留一层折叠，默认露 `SYSTEM_PREVIEW_LINES = 8` 行 + 渐隐。

### ⑩ 按系统消息呈现（用户第 10 条）

不带图的 user 行改成安静的**系统消息**：`.sys-line` 里 `span.sys-badge`「系统」+ `span.sys-meta`「user 轮」+ 一行摘要，行的 `title` 写明"这一轮是 user 角色发出的任务提示（系统性质，不是人打的字）"。对话页里**不再出现"用户"这个词**（`>任务<`、`.bubble`、`.msg-task` 也随之消失，测试逐条断言）。

### ⑪ 轨迹页保持真实（用户第 11 条，推翻第 5 条的一部分）

上一批为了让轨迹页"一眼可读"，把工具调用与工具结果**合并成一行**了；用户要的是轨迹页当"事件的真实清单"。于是 `TRAJ_KINDS` 回到转录里的真实角色（`用户/助手/思考/工具/结果/元信息/用量`），工具调用与工具结果**各占一行**，类型 chip 的条数按真实行数统计。唯一增量：带图 user 轮标 `图片 ×N` + 归属依据（可点开看图），点行跳回对话里**合并后**的那一块——"对话页是整理的呈现、轨迹页是真实的结构"这个分工写进了 `docs/commands.md`。

### 追加：工具图片只能走 user 消息（wire 事实）

`tool` 消息的 content 只能是文本，`user` 消息又没有放 `tool_call_id` 的位置——所以"图片属于哪次调用"只能**写在文本里**。这正是第 8 轮归属句柄的依据，也解释了为什么转录里会冒出"带图 user 行"这种看起来奇怪的形态。

### 第 8 轮：图片归属（`internal/session` + 查看页）

- **生成侧**（`session.go`）：`visionTurn` 带上 `callID`/`name`，两条回灌路径合并成 `appendToolImage`，句柄文本由 `toolImageHandleText()` 生成：`Tool image output from <工具> (call <id>) (for your visual review):`（前缀 `toolImageTextPrefix` 固定不变——查看页靠它认旧转录；工具名/call id 缺失时对应部分省略）。`toolImageContent()` 只产生 **text + image_url 两段**，测试显式断言 user 消息里**不得出现 `tool_call_id`**（wire 兼容性）。测试：`TestToolImageTurnDoesNotBreakToolBlock` 追加句柄与 wire 断言、新增 `TestToolImageHandleText`（三种形态 + data URI）。
- **查看侧**（`viewer.js`）：`imageAttributions()` 是**纯函数**（一次遍历 `state.lines`，结果按"会话 id + 行数"缓存并在会话切换时失效），规则是——句柄带 `(call <id>)` → 精确匹配那次调用；只有 `from <tool>` → 该轮里同名且未被认领的调用；只有前缀（旧转录）→ 该轮里**尚未被认领**的调用（按顺序）；不是句柄 → 会话开头投喂原图的轮，归到**本会话第一条任务**；推断不出来 → `kind: 'none'`，单独成一条折叠行。`attributionText()` 把依据翻成人话（`归属：call <id>` / `归属：由顺序推断（本轮的 call <id>）` / `归属：本会话任务` / `归属：未识别`），对话页的附件脚注与轨迹页共用。
- **两个实现细节**：① `attachImages(host, line, attr)` 的宿主既可以是工具卡片（`host.card`）也可以是**系统消息块**（任务），后者让"投喂原图 → 第一条任务"这条归属真的能落地；② 会话开头的原图轮**排在任务行前面**，渲染到它时任务节点还不存在，于是先记进 `pendingTaskImages[任务行号]`，等任务行渲染时再挂上去（每次整表重渲染前清空）。若不这么做，这条归属就只能退化成孤立的图片行。
- **轨迹页**：带图 user 行的摘要末尾直接写归属依据，`title` 里写明协议原因与"精确 vs 顺序"的区别；`tool` 回执与紧随其后的带图 user 轮**互相提示**（「图片见下一行用户轮」/「接上一行工具回执」）但仍是两行；跳转目标 = 归属到的那次调用（或任务行）。
- **测试**（`sessionview`）：新增 `attribution_browser_test.go`，用 headless chromium（`--headless=new --no-sandbox --user-data-dir=<可写目录>`，没有 chromium 的机器 `t.Skip`）跑**真实渲染**：① 旧转录（句柄只有前缀）的图片回执必须落进那次调用的 `.io-card` 里且带「归属：由顺序推断（本轮的 call call_1）」；② 会话首图必须落进 `.msg msg-system` 且「归属：本会话任务」、**不得出现**「归属：call」；③ 轨迹页同时出现「由顺序推断」与「本会话任务」，且 `kind-result` 行存在（调用/结果未合并）；④ 句柄带 `(call call_b)` 时必须精确落到 `view_call_b`（同一轮里顺序推断会给出 `call_a`，所以这条能真正区分两种路径）。断言前会剥掉内联 `<style>`/`<script>`——否则 dump 出来的源码文本会让断言假通过。

### 追加修复：任务提示**自带原图**的轮被判成"未识别"（真实矢量图会话）

真实数据把第 8 轮的归属规则打出一个洞：`source/sessions/vector_<书>__<哈希>__<图>.jsonl` 的**第 2 行**是 `role:"user"` + 1 张原图 + 任务正文（`Redraw the attached image as TikZ.\n\nORIGINAL FIGURE SIZE: …`）——**会话任务自己带着原图**，既不是"纯图片行"，也不以 `Tool image output` 开头。而 `imageAttributions()` 里 `firstTask` 只认"**不带图**的 user 行"，这类会话（同一本书的矢量图会话通常全程只有这一条 user 正文）压根没有任务行可选，整行掉到 `kind:'none'`：页面标「归属：未识别」、`本会话任务` 一次都不出现。用只读导出 + headless chromium 数出来的真数据：36 个矢量图会话里 **35 个各中一条**（每个会话 `归属：未识别` 可见 1 处；DOM 里出现 2 次是因为同一行的可见脚注与折叠行的 `title` 各写了一次，**不是**图例/说明文字），并伴随 35 条孤立的图片行。

改法（`viewer.js` 同一个函数，不动算法骨架）：不以 `Tool image output` 开头的带图 user 轮一律算"任务自己的图"，`entry.taskLineN = text ? line.n : (firstTask || line.n)`——**自带任务正文的**（真实矢量图会话就是这种）归到它**自己**那条任务行；**纯图片行**（排在任务提示前面的那种）仍挂到本会话第一条任务上，`pendingTaskImages` 那条路原样保留。呈现上加一个分支：`attr.taskLineN === line.n` 时这一行照**系统消息/任务块**的样式渲染（`systemTurnSection` + `attachImages`），图片作为**该任务的附件**（缩略图可点开灯箱、默认折在任务块里），绝不塞进任何工具调用；附件段顺手不再把任务正文重复抄一遍（正文已经在任务块里了）。

**顺带查出的第二处「该归属却落到未识别」**：句柄"还没被认领的那次调用"用的是**跨轮累计**的 `claims` 表，而续跑/重放的转录里同一个 call id 会出现两次（样式会话 `style_session.jsonl` 实测 133 个重复 id）——重放段的图片发现"本轮调用全被前面的段认领光了"，只能标未识别。改为**每轮清零**（认领本来就是"这一轮里哪次调用还没拿到图"）。真数据统计（`docvision sessions --dir .` 只读导出 /home/share/samba-share/PDF2MD，headless chromium 逐会话点开、剥掉内联脚本后数 DOM）：

| 类型 | 会话数 | 未识别（前 → 后） | 本会话任务（前 → 后） | 由顺序推断（前 → 后） |
| --- | --- | --- | --- | --- |
| 矢量图会话 | 36 | 35 → 0 | 1 → 36 | 308 → 308 |
| 样式会话 | 5 | 33 → 0 | 0 → 0 | 99 → 132 |
| 转换会话 | 7 | 0 → 0 | 0 → 0 | 97 → 97 |
| 核对会话 | 4 | 0 → 0（无图片轮） | 0 → 0 | 0 → 0 |
| 章节划分 | 2 | 0 → 0（无图片轮） | 0 → 0 | 0 → 0 |

（`归属：call <id>` 在真数据里 0 次：现有转录的句柄都还是旧式"只有前缀"，精确匹配那条路由 chromium 用例 `TestImageHandleAttributionIsPreciseWhenCallIDPresent` 钉住。逐图校验会话在这份真数据里不存在——`latex.figure_check` 默认关闭，没跑过。）测试：`attribution_browser_test.go` 新增 `TestTaskImageTurnStaysWithTaskInChromium`（照真实矢量图会话第 2 行的形状：任务行带图 → 出现 `归属：本会话任务`、不得出现 `归属：未识别`，图片必须落在 `.msg msg-system` 的附件段里、不得落进 `.io-card`）与 `TestTaskFeedBeforeTaskAndOldHandleInChromium`（会话开头**纯图片行**排在任务之前 → 仍归本会话任务；旧式 `Tool image output (for your visual review):` → 仍归「由顺序推断」）。

### 追加：图片归属改 FIFO + 附件段排版 + 行高间距 token + 缩略图预览 + 精确匹配即工具输出（用户追加五条）

用户在真机截图上看 `view_pdf` 卡片时又提了五条（A 归属改 FIFO / B 附件段排版 / C 工具行间距统一 / D 看图调用下面的缩略图预览 / E 精确匹配时不要拆成"输出 + 附件"），同一批做完。

**A. 归属从"就近往前找"改成按转录顺序 FIFO**。旧规则只看"最近一条带 `tool_calls` 的助手消息"，因此会把图片挂到**根本不产图的调用**头上：在 `/home/share/samba-share/PDF2MD` 只读导出的 54 个会话上逐条模拟两种算法，573 条图片行里配到调用的 537 条中有 **10 条两种算法给出不同答案**，而这 10 条全部是旧算法错、FIFO 对——旧算法把图片挂给了同轮的 `bash` / `edit_file` / `grep` / `write_file` / `list_fonts`（最典型的是 `latex_project/测试-概率论_0910/source/sessions/vector_…__mind-map_diagram.jsonl` 第 2 行：一条 `write_file` + 4 次 `view_image`，4 张图被整体错位一格，第一张挂到 `write_file`、其余挂到前三次 view 上）。新规则：先把"会产生图片的调用"按出现顺序排成队列，再按顺序扫带图 user 行，每条还没归属的图片行领走队列里**最早那个还没被认领、且行号排在它前面**的调用（一对一）。**两条证据都用**：有回执时以回执为准（`Image <路径> … attached.` / `PDF page <文件> … attached.`）——真数据里有 **37 次失败的看图调用**（26 次 `view_pdf` + 8 次 `view_image` 回执是 `TOOL ERROR`、3 次 `REJECTED`），它们没产图，按工具名入队就会把后面那条图认错；**没有回执**（被压缩截断）才退回工具名 `view_image`/`view_pdf` 推定。队列元素是**每一次调用**而不是 call id（重放转录里同一 id 出现两次）。`(call <id>)` 句柄仍精确匹配优先，带正文的图片行仍是任务自己的图（FIFO 不许抢）。真数据结果：**537 条图片行 ↔ 537 次产图调用正好一对一、0 条未认领、0 条未识别**；`归属：call <id>` 仍是 0（现有转录的句柄全是旧式），跨轮配对 0 次（所以"本轮"这个措辞在真数据上仍然准确）。只读导出 + headless chromium 逐会话点开统计的最终结果：

| 类型 | 会话数 | 未识别 | 本会话任务 | 由顺序推断 | 任务块带附件 | 调用卡片带附件 | 孤立图片行 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 矢量图会话 | 36 | 0 | 36 | 308 | 36 | 308 | 0 |
| 样式会话 | 5 | 0 | 0 | 132 | 0 | 132 | 0 |
| 转换会话 | 7 | 0 | 0 | 97 | 0 | 97 | 0 |
| 核对会话 | 4 | 0 | 0 | 0（无图片轮） | 0 | 0 | 0 |
| 章节划分 | 2 | 0 | 0 | 0（无图片轮） | 0 | 0 | 0 |

**B. 附件段排版：图片被挤、说明文字跑到右边、空一大片**。根因是一个具体的选择器冲突：附件段复用了「输入 / 输出」的 `.io-section`，而它是 `grid-template-columns: max-content minmax(0, 1fr)` 的两列网格；`attachImages()` 往这一段里追加了**三个**子元素（标签、图片容器、归属脚注），第三个只能落到第二行第一列，把 `max-content` 那一列撑成**脚注那么宽**——于是说明文字被顶到右边、图片列被压成一小条，剩下的空白被当成"卡片空了一大片"。修法：附件段换成单列块 `.io-section.attach-section`（`display: flex; flex-direction: column; align-items: flex-start`，标签 `position: static`），DOM 顺序固定为**说明文字 → 图片（各自成行）→ 归属脚注（块末）**；图片尺寸也归到 token：`.images img { max-width: min(100%, var(--attach-img-w)); max-height: var(--attach-img-h) }`（`520px × 420px`，原来是 `160px` 缩略图）。实测（`convert_chapter_003` 精确匹配卡片的几何）：标签 / 说明 / 图片 / 脚注左边缘同在 `x=532.5`，`1400×2057` 的整页渲染被收到 `281×420`（按原比例顶到高度上限），脚注在最后一行、不参与图片布局。

**C. 工具行间距不一致**。先用 headless chromium 量了真页面（`convert_chapter_003`，183 个折叠行）：行高本来就统一（全部 24px，不因回执/附件/ok-error 而变），不一致的是**间距**——同一条消息里的相邻行 6px、跨消息的行却是落在 `section` 之间，量出来有 8/24/48 三种值，而且**没有任何回合分隔标识**。真根因是另一处代码缺陷：`renderLine()` 在"带图 user 行归到某次调用"时 `return anchor(callNode.details, line)`，调用方拿到这个 `<details>` 就 `appendChild` 到 `.stream`——**卡片被从它所在的助手消息 section 里搬走了**（`convert_chapter_003` 里 40 个带附件的行全中），于是它既没有 6px 的同消息间距、也套不上回合分隔。修法：只登记锚点、`return null`（源码断言钉住）。间距统一成一组 token：`--row-h: 24px`（折叠行高，`min-height: var(--row-h)`，摘要行 `.stream-summary` 同用）、`--row-gap: 6px`（`.msg` 的行间距）、`--divider-gap: 8px`（`.stream > .msg + .msg` 的 `margin-top`/`padding-top`，中间一条 `.5px dashed` 回合分隔线；第一条消息上面没有线）。改后实测：行高唯一值 24px、同消息行间距唯一值 6px、section 间距唯一值 8px（+1px 虚线），`.stream` 的直接子节点不再出现裸 `<details>`；折叠态与"全部展开"态量出来的行高与分隔线完全一致。

**D. 看图类调用下面的缩略图预览行**（新功能，用户原话"显示 view 的图片预览图，跟在对应工具调用下面，直接跟…一轮调用了多次 view，就在最后一个 view 下面显示并排的缩略图，按照顺序"）。`addPreview(host, line)` 把缩略图行插在工具行 `<details>` 的**兄弟位置**（`insertBefore(strip, host.details.nextSibling)`）——这是"折叠态就可见"的唯一办法（`<details>` 的折叠会藏掉卡片里的内容）；宿主行由 `previewHost()` 给出：同一轮里**最后一个**能产图的看图调用（`imageAttributions()` 顺带产出 `candRound`/`roundLast` 两张表），所以一轮多次 view 只在最后一行下面出现一条缩略图行；缩略图按转录行号重排后再画（`strip.__items.sort`）。真数据截图确认：`convert_chapter_003` 里连续两条 `view_pdf` 的一轮，两张缩略图并排挂在**后一条**下面；尺寸实测全部 ≤ `240×108`（竖版整页 73×108、横版裁切 240×24…240×72），一律保持原比例、8px 间距、20px 缩进（与工具行名称对齐）、点图进灯箱。

**E. 精确匹配时图片就是那次调用的输出**。句柄带 `(call <id>)` 时不再另立「附件（user 轮）」段：`attachResult()` 给「输出」段加 `out-section` 标记，图片（`imageStrip`）与一行小字归属说明直接追加进该段的 `.text-wrap`（在滚动区**之外**，所以长输出的内滚不会把图片卷进去），顺序是**输出正文 → 图片 → `归属：call <id>（<工具>，精确匹配）`**，脚注不参与图片布局（同一列里的块级兄弟）。没匹配上（FIFO 推断 / 未识别）的卡片仍走「附件（user 轮）」单列段 + 推断脚注，两种呈现一眼可辨；轨迹页的归属依据不变。真数据里 `(call <id>)` 句柄 0 条，所以这条用**由真实转录改写句柄**的合成工程（`convert_chapter_003` 的 38 条句柄改成新式、留 2 条旧的作对照，图片与回执都是真的）跑截图与 DOM 断言：38 张精确匹配卡片全部没有 `attach-section`、图片在 `.out-section` 内、脚注含"精确匹配"；2 张旧式卡片全部保留附件段且脚注在段末。

### 验证与测试

- `gofmt -l .` 干净、`go vet ./...` 干净、`go test ./...` 全绿（`internal/sessionview` 7.4s，含 8 条 chromium 用例）。
- 本批追加的 chromium 用例（`attribution_browser_test.go`，全部在真实渲染上断言，先剥掉内联 `<style>`/`<script>`）：`TestViewPreviewStripFollowsLastViewRowInChromium`（一轮两次 view → 只有一条缩略图行、挂在这一轮**最后一个** view 行下、两张按 a.png→b.png 顺序、是 `<details>` 的兄弟节点；不产图的 `bash` 卡片不得带附件）、`TestExactHandleRendersImageAsToolOutputInChromium`（精确匹配 → 卡片里**没有**「附件（user 轮）」、图片在 `.out-section` 内且顺序是正文→图片→脚注、脚注写「精确匹配」）、`TestFifoHandleKeepsAttachmentSectionLayoutInChromium`（旧句柄 → 仍有 `attach-section`、标签/图片/脚注顺序正确、图片不算输出）、`TestToolRowGeometryComesFromOneTokenSetInChromium`（注入测量脚本等图片加载完再量：行高唯一 24、同消息行间距唯一 6、回合分隔线唯一 `dashed/8px/8px` 且第一条消息无线、折叠态与全展开态行高一致、缩略图 ≤240×108 与附件图 ≤520×420 且保持原比例、竖版图正好顶到高度上限）、`TestSpacingTokensAreDeclaredOnce`（CSS 里这组 token 只声明一次，且不允许再出现硬编码的旧行高/图片尺寸）。夹具里的图片用 `image/png` 现场生成 800×1200 / 1400×400 的真 PNG（原来写的 `"jpeg"` 文本文件其实根本加载不出来，量不到真实尺寸），并且放在**会话目录**下的 `media/` 里——页面按 `sessionDir(id) + ref` 解析路径。
- `go/internal/sessionview` 测试新增/改写：`TestViewerJSONHighlighting`、`TestViewerToolCardsScrollAndExpandLikeThinking`、`TestViewerConsoleAndDiffHighlighting`、`TestViewerCodeTypographyUsesOneToken`（辅助函数 `jsFunc`/`cssRule` 直接取 JS 函数体与 CSS 规则来断言，不再只做整文件 `Contains`）、`TestViewerMergesToolCallsAndImageTurns`（归属规则与轨迹结构改成新形态）。
- 端到端夹具（`.dsh-check/`，临时、交付前删除）：合成工程 + `--list` + 静态导出 + 5 张截图（浅色/浅色展开/轨迹/深色/深色展开）逐张回看，30→34 条 DOM 断言全过（断言前剥掉内联 `<style>`/`<script>`，并且先注入点击脚本选中目标会话——页面默认选中的是字母序第一个会话，不点就验错对象）。

### 有意保留 / 未做

- 侧栏"切换会话会收起此前自动展开的组"（见 ① ）是默认收起策略的必然结果，没有加"自动展开过就记住"这种更复杂的记忆。
- 工具输出的整块 error 底色（`.io-text[data-error]`）保持不变：per-line 高亮用 span 覆盖颜色，diff 的 `+`/`-` 仍然分明；没有把整块红底改成"只在有高亮时不着色"，避免改变既有语义。
- 没有引入任何前端依赖/构建步骤；Markdown 渲染器与高亮器都是页面自带的纯函数。

### 同一批追加（后半）：img2text 回退到只出 Mermaid + 失败可诊断 + analyze 不再把 warning 当失败

用户原话："还把 img2text 调整回之前的那种吧，不要让他产出 latex 的了。"以及"warning 也算失败吗？好像这边有的有问题？处理的他弄出来的是 latex 格式而不是 mermaid 但是这边解析的时候有问题？"

**根因**：`3c9102e` 给 img2text 加了"用 latex 代码块画矢量图（TikZ/pgfplots）"，模型照做，而 img2text 侧解析不了这种块。

**真实运行证据**（只读 `logs/img2text_error_20260912_012048.log`）：81 张图、42 成功、39 张报废；进度行 `errors: 0, warns: 39`（本来就分列）。161 条 WARNING 里 117 条为 `Mermaid validation failed (1/3..3/3): tikz block 1 failed: … This is XeTeX, Version 3.141592653-2.6-0.999998 …`；78 条 ERROR 里 39 条 `[IMG_MERMAID_INVALID]`、39 条 `Skipped invalid response for … will retry next run.`，另有 4 条 `Unexpected prefix before '[IMG_TYPE:'`。

**A. 提示词逐字回退（不是凭记忆重写）**：从 `git show 3c9102e~1:go/internal/img2text/processor.go` 取旧 `systemPromptTemplate` 原文（当时内联在 Go 里，`go/internal/prompts` 是更晚的 `83e7b57` 才建的），逐字比对后写回 `templates/img2text.system.md`；与回退前只差 `3c9102e`/`641752d` 为 LaTeX 加的两处（4b 的 TikZ 绘图、"代码块带语言标注"），"不许重叠/拥挤"保留但限定为 Mermaid 图。注册表新增 `ForbidMention: []string{"tikz", "pgfplots", "latex code block", "latex vector"}` 与提示词守卫第 5 条。

**B. img2text 路径上的 LaTeX/TikZ 通路全部摘掉**：`CallAIWithTools` 的校验闭包不再按 `opts.LatexValidation`/`opts.LatexEngine` 分流到 `ValidateTikZ`，只剩 Mermaid 校验；新增 `leftoverDrawingBlock`（`diag.go`，识别 ```tikz/```pgfplots/```latex 围栏），命中即按**无效响应**处理（`sentinelInvalid` + `StatusRetry`，跳过、下轮重试）并在日志里打印图片路径、期望格式摘要与截断后的模型原文；`embedBlockFor` 去掉 `typ == "tikz"` 分支与 body 里的围栏判断，只剩 text/latex(数学)/table/code 直嵌、mermaid 代码块、其余 `[Image]( … )`。随后的收尾把无调用方的 `tools.latex.*` 配置块（含 `Options.LatexValidation/Engine` 两个只写不读的载体字段）与 `internal/img2text/tikz.go` 连同其测试一并删除；档位1 作图会话用的是**另一份** `internal/latex/tikz.go`，未动。

**C. 失败可诊断（三个真缺陷）**：
1. 文案名不副实：`Mermaid validation failed (…)` 实际失败的是 TikZ 编译。现按实际校验对象命名并带图片路径——顺带修掉一个真 bug：`imgPathFromIdx` 只 `return "line:<idx>"`（前面拼的 line 变量没用上），现从该行 `![…](images/…)` 解出真实路径，解不出才退回行号。
2. 错误详情截错方向：`truncateOut(out, 400)` 取的是 TeX 输出的**前 400 字节** = 版本横幅。新增 `extractValidationError`：抽 `! 开头` / `l.<数字>` / `Package … Error` / `Emergency stop` / `Runaway argument` 等真错误行（`!` 后紧跟的一行解释也带），排除 `This is XeTeX, Version …`、`entering extended mode`、`Document Class: …`、`preloaded format=` 与 mmdc 的 `Generating …`；一条都抽不到退回**最后 12 行**，仍为空才退回开头 400 字节。
3. 格式不符没原文：`Unexpected prefix before '[IMG_TYPE:'` 与 `Skipped invalid response for …` 现在都附截断原文（rune 安全，800 字符上限 + `共 N 字符`）与期望格式摘要。`ProcessOneImage`/`CallAIWithTools` 因此多返回一个"最后一次原文"（`runResult.rawSnippet`）。

**D. analyze 三分类**：新增 `StatusWarning`；runner 会重试的两类哨兵（`IMG_MERMAID_INVALID`、`IMG_INVALID_FORMAT`）判为警告，其余哨兵仍是失败。`Statistics` 增 `Warning`/`WarningRate`、`RoundFileStat` 增 `Warning`，报告的基础统计、按文件摘要、线程明细、错误分类表（每项标注"警告（下轮重试）"/"失败"）与 CSV 状态列全部同步；进度摘要的"无效条目"注明下轮自动重试并给出可达完成率。合成夹具按真实形状（42 成功 / 39 警告 / 0 失败 = 81）断言 `Failed == 0`——正是修前会打印"失败 39、成功率 51.9%"的那份数据。

### 验证与测试（同批追加）

- `gofmt -l .` 干净、`go vet ./...` 干净、`go test ./...` 全绿。
- 新增用例：`img2text/diag_test.go`（真错误行 vs 版本横幅、无 `!` 时退回**最后**行、rune 安全截断、`leftoverDrawingBlock`、期望格式摘要）、`img2text/processor_latex_reject_test.go`（回 ```tikz → `StatusRetry` + `[IMG_INVALID_FORMAT]` 且**只发一次请求**，日志含图片路径+模型原文+期望格式且**不得**出现 `This is XeTeX`；Mermaid 正常路径仍 `StatusOK`；`BuildSystemPrompt` 不含 tikz/pgfplots 绘图要求）、`prompts/prompts_test.go` 第 5 条守卫、`analyze/parser_test.go`（重试哨兵→warning、`IMG_API_ERROR`→failed、三分类）、`analyze/report_warning_test.go`（真实形状 42/39/0 端到端）。
- **离线验证口径**：真实跑批需要 API，未真跑。替代手段：①读只读日志原文（上述证据数字与 TeX 横幅片段都来自它）；②`httptest` 假服务模拟"回 ```tikz```"，断言请求次数与日志正文；③`extractValidationError` 用**真实日志里那段 XeTeX 输出的形状**（横幅 + `! Undefined control sequence.` + `l.14` + `Emergency stop`）做输入，断言抽到真错误、横幅一字节不留。

### 同一批追加（其三）：图片 token 折算 + 显示单位开关 + `sessions --serve` 读配置

用户三问：① "view 的图像工具调用返回的图片 token 有没有算进用户消息的 token 里？"② "界面里显示的是字符还是 token？"③ "单独跑 `docvision sessions --serve` 好像不用 config 里的配置？"

**① 图片 token：两条路都算，但本地估算漏了大图。** 厂商侧一直算：真实日志里"只多了一张图"的相邻请求 `prompt_tokens` 增量为 292k 像素→440、986k→1285、2.32M→2987（glm-5.3-flash-official），与 `宽×高/750` 吻合，说明 `prompt_tokens` 已含图片。本地侧 `messageTokens` 也计图片，但**每张固定 1100**：对 deepseek-v4.1-flash（实测 ~1050 就饱和）够用，对 glm 的 2.3M 像素大图少算近 2000。改为按尺寸折算：`clamp(宽×高/750, 85, 4096)`，取不到尺寸退回 `1100`；PNG/JPEG/GIF/WebP 头解析用标准库 `image.DecodeConfig`（webp 由 `x/image/webp` 自注册），尺寸结果按路径缓存。**厂商数字一律照抄**（`t="usage"` 行与新加的详情栏瓦片口径分开）。新增 `estimate` 配置块（4 项，`config_version` 7→8）。

**② 显示单位开关。** 行内所有按字数统计的地方（工具行 `输入 → 输出`、`foldLabel()`、思考行、轨迹表计数列与表头、系统提示词快照、`metaState()`）改为走唯一的 `countText(chars, tokens)`：默认 `token` 口径、**必带 `≈`**（本地估算），切到 `字符` 是精确计数；详情栏瓦片是厂商实测值，不带 `≈` 也不受开关影响。token 数全部由 Go 侧算好下发（`Line.est` / `MetaInfo.promptTokenEst` / `ToolSchema.descTokens`），前端不再自算一份。详情栏新增「图片 N 张 ≈ …」瓦片，说明按 `estimate` 规则折算，并注明 `prompt_tokens` 已含图片。

**③ `sessions --serve` 读配置。** 此前 `--addr` 默认写死 `127.0.0.1:8848`、根目录只认 CWD，配置里的 `preview.host/port` 只有 latex 自动预览在用。现在扫描根默认取 `paths.latex_project`（档位2 用 `paths.latex_output`，与 `startPreview` 同源），地址默认取 `preview.host/port`，优先级 `--dir` > 配置 > 当前目录、`--addr` > `--port`（新增）> 配置 > 内置默认；启动打印三行（URL / 目录 / 配置文件），每项后面带来源标签，目录不存在时退回当前目录并写明原因。实测（临时目录 + 真转录副本）：`--list` 根目录显示 `.../latex_project（来源 config paths.latex_project）`，`--serve` 三行如设计，端口 8899 来自配置。

**验证**：`gofmt -l .` 干净、`go vet ./...` 干净、`go test ./...` 全绿；新增 `sessionview/estimate_test.go`（行级估算完整性、1000×800→1066、上下限与兜底）、`sessionview/unit_toggle_browser_test.go`（真实 chromium：默认 `≈ N tokens` → 点一下变 `N 字符`，轨迹表头同步，图片瓦片 `图片 1 张 / ≈ 1.1k`，厂商瓦片 `12k` 不带 ≈）、`sessionview/serve_test.go`（三行横幅内容与来源）、`cmd/docvision/cli_sessions_test.go`（目录/地址优先级与 `--addr` 默认必须留空）。

---

## 第三十五批：图片 token 两种计量方法（可按模型各配一套）+ 详情栏实测/估算分离 + 横幅打印真实绑定地址

用户原话（三件事）：① "不是 token 简化，而是两种计量方法，可以独立的在不同模型上使用。可以自己定义。而不是只用一个"；② 详情栏那张「图片 N 张」瓦片要能看出**实测还是估算**；③ `sessions --serve` 打印的必须是**实际绑定的地址**（配置里 `preview.host: 0.0.0.0` 时横幅却写 `127.0.0.1`）。

### ① 两种计量方法 + 按模型各选一套

上一批把"每张固定 1100"换成"按尺寸折算"（`estimate.image_px_per_token` 等 4 个键），用户指出这等于**只留了一种方法**：同一个模型族里"按张固定计费"与"按像素折算"的端点会同时存在，应该都能选、都能自己定参数、并且**可以按模型分别配**。因此 `estimate` 块改为：

```yaml
estimate:                  # 全局默认
  method: pixels           # fixed / pixels / none
  tokens: 1100             # fixed 的每张固定值；pixels 下取不到尺寸时也用它
  px_per_token: 750
  min_tokens: 85
  max_tokens: 4096
models:
  drawing:
    image_tokens:          # 只覆盖这一条，没写的键继承顶层 estimate
      method: fixed
      tokens: 1050
```

- 旧键 `image_px_per_token`/`image_tokens_min`/`image_tokens_max`/`image_tokens_fallback` **删除**（不留半死的兼容分支），`config_version` 8→9，两个模板、`docs/config.md`、README 版本行同步。
- 解析顺序：`models.<条目>.image_tokens.<键>` → 顶层 `estimate.<键>` → 代码默认；注册时**条目名与 wire 模型 id 都登记**（会话与预览页只知道 wire id），大小写不敏感。
- 默认值仍取 `pixels 750 / 85 / 4096`（与上一版数字一致，老配置的估算不变），`fixed` 的默认 `tokens` 取 1100。依据是这一批重新量的实测：`glm-5.3-flash-official` 上 2.88M 像素页面渲染 ≈3697 token（≈780 px/token，上一批日志里 292k→440 / 986k→1285 / 2.32M→2987 三点同向），`deepseek-v4.1-flash` 上大图饱和 ≈1050/张——两个端点量级差 3~4 倍，一套参数不可能同时准。
- 非法 `method`、负参数、`max_tokens < min_tokens` 由 `semanticChecks` 报错（全局与按模型都查）；上下限写反不再被 `setDefaults` 悄悄纠正（运行期 `Normalized()` 仍会把界限理成有序的，估算不会失控）。

### ② 实测每张多少 token：由用量行的相邻差值推出

上一批记的用量行只有厂商数字，这一批给它加上**本地那一半**：`text_tokens`（请求发出前的文本估算，**不含图片**，由新增的 `Session.promptTextTokens()` 快照）与 `image_count`（该次请求带了几张图，`snapshotRequest()` 在每处发请求前记下）。于是

```
每张实测 = (Δprompt_tokens − Δtext_tokens) / Δ图片数
```

跳过三类步：该步没有新增图片、Δprompt_tokens ≤ 0（本地裁剪或 AI 压缩把内容删掉了，差值量的是删除而不是图片）、旧转录没有本地那一半（`text_tokens` 缺失）。结果取**各步中位数**，不是总计相除——真实网关（new-api 中转）回报的 `prompt_tokens` 在同一步内会漂（同一张 900×1272 页面渲染在不同步里落在 ~630–1800），中位数不会被单步异常带走。页面把步数与张数写进悬浮说明（`实测 2.8k/张：… （1 步 / 1 张）`）。

瓦片因此有两种形态：有样本时 `实测 1.2k/张`（厂商口径、不带 `≈`），没有时 `≈ 1.1k/张`（本地估算），悬浮说明永远写出该会话生效的完整规则（`pixels 750px per token（85–4096）` / `fixed 1100/张`）与本地估算的对照。规则文案与实测值都由 Go 侧算好随 `SessionInfo.estimate` 下发（前端不再自算，`state.estPolicy` 那份页面级副本连同 `/api/index` 的 `estimate` 字段一起删掉——多模型下"一条页面级规则"本身就是错的前提）。

真实数据里**没有任何转录带 `t="usage"` 行**（现存 54 个 `.jsonl` 都是加用量行之前跑的），所以"没有实测样本时只给估算"这条路径是当前默认路径，浏览器用例专门钉住了它。

### ③ 横幅打印实际绑定的地址

`Start()` 改为返回 `Bound{Addr, URL, Browse}`：`Addr` 是 `net.Listen` 回报的地址原样（`net.Listen("tcp","0.0.0.0:0")` 在双栈机器上会给出 `[::]:37703`，这正是要照实打印的东西），`URL` 是能粘进浏览器的（通配绑定渲染成 loopback），`Browse` 只在通配绑定时给出 loopback 地址。横幅：loopback 三行不变；通配绑定四行——第一行 `会话预览: 0.0.0.0:8849（只读服务，Ctrl+C 停止）`、第二行 `浏览 http://127.0.0.1:8849/`，来源标签（`config preview.host/port` / `--port` / `--addr` / `内置默认`）原样保留。latex 自动预览的日志行也一起给出"监听 `<实际地址>`"。

### 顺手修掉的两处文档与代码不一致

1. **旧 estimate 键不再生效时必须出声**：四个旧键改名后没有任何代码读它们，而 `LoadConfig` 用的是不严格的 `yaml.Unmarshal`（严格检查只在 `docvision setup`），所以运行目录那份 `config.yaml`（仍写着 `image_px_per_token: 750` 等四行）会静默按默认值跑。按 `latex.bash_sandbox → tools.bash.sandbox` 的先例加了一行启动迁移提示（`hasRetiredEstimateKeys` 在原始文本里找旧键名）。
2. **`preview.port` 的"0 = 内核挑端口"只对命令行成立**：两个模板与 `docs/config.md` 都写着配置里写 `0` 由内核挑端口，但 `setDefaults` 会把 `0` 补成 8848（`int` 分不出「没写」与「写了 0」）；实际验证时把运行目录配置的副本改成 `port: 0`，横幅打出来的是 `[::]:8848` 而不是随机端口——即文档在骗人。只改措辞（配置写 0 与不写一样取 8848；要内核挑用 `--port 0`），行为未动；真要支持"配置里写 0 由内核挑"需要 presence 检测（`*int` 或自定义 `UnmarshalYAML`），这批没做。

**验证**：`gofmt -l .` 干净、`go vet ./...` 干净、`go test ./...` 全绿（输出见提交说明）。新增/改写的用例：`session/estimate`（两法各自的行为与描述文案、按模型覆盖不影响其它模型、改全局不影响单独配置的模型）、`config/estimate_test.go`（默认值、逐键继承顺序、配置文件读入、非法 method/负参数/上下限写反都是配置错误）、`session/usage_wiring_test.go`（用量行必须落下 `text_tokens`）、`sessionview/measure_test.go`（中位数、按张数摊、裁剪步跳过、无新增图片步跳过、旧用量行跳过、估算偏差步跳过、单步可用）、`sessionview/estimate_test.go`（会话各自的规则与文案下发）、`sessionview/serve_test.go`（通配绑定照实打印 + loopback 浏览行 + 来源标签、端口 0 打印内核端口）、`cmd/docvision/cli_sessions_test.go`（配置 → 估算器的整条链路，条目名与 wire id 都命中）。真实浏览器用例（headless chromium）：`unit_toggle_browser_test.go` 的图片瓦片改为 `图片 1 张 / ≈ 1.1k/张` 并断言悬浮说明写明口径与"没有可用的实测样本"，新增 `TestImageViewTileShowsMeasuredCost`：两条带本地那一半的用量行（10000/9000/0 → 13000/9200/1）让瓦片显示 `实测 2.8k/张`、悬浮说明同时给出 `1 步 / 1 张` 与本地估算 `≈ 1.1k`。

**没有做的事**：`models:` 的复用（同一模型在多处重复整块配置）这一批只做了设计、没有实现——三个方案（YAML anchors / 自研 `extends:` 深合并 / 命名 profile）的对比、风险与迁移路径见交付说明的独立一节；现存 54 个转录都没有用量行，所以实测每张那条路没有真实端到端数据可对照，只有合成夹具与浏览器用例。

---

## 第三十六批：`models.<条目>.extends` 命名基座（深合并）+ 未知键/同名不同价出声 + 用户运行目录配置改写

第三十五批把 `models:` 的复用只做到设计（三个方案的对比），这一批按用户选定的**方案二**实现：命名基座 + `extends:` 深合并，并把它列出的风险一并解决。

### ① 为什么必须在 YAML 节点层合并

`ModelConfig` 除 `Thinking`/`RequestBody`/`ImageTokens`/`Stream`/`ToolStream` 外全是值类型（`int`/`float64`/`string`/`bool`），解码之后「没写这个键」与「写了 `0`/`false`/`""`」是同一个值，任何"逐字段继承"的写法都判不出覆盖。所以 `LoadConfig` 与 `ValidateData` 都先把文档解析成 `yaml.Node` 树，`mergeModelExtends` 在**解码之前**按**条目键**合并，然后才解码：

- 只把子条目**自己写的键**写回树里，没写的键保持"不在树里"——这正是 `ResolveModel` 那套零值继承（第二套规则）能原样继续工作的原因：`models.drawing` 没写 `api_timeout` 时树里也没有这个键，解码后仍是 0，于是照旧往 `models.text` 回落。
- 合并按"能合并的先合并"多趟推进（父条目自己还带 `extends` 时等下一趟），名字排序保证结果与 map 迭代顺序无关；跑不动了就说明有环。
- 合并是**节点级**的：`parent.Content` 的节点被直接挂进子条目。因为解码会为每个条目新建 map，共享节点不会让两个条目共用同一张 map；跨条目的别名只可能来自"把 `models.text` 的 map 头赋给条目"那两处（见 ③）。

### ② 语义表与报错

| 情况 | 结果 |
| --- | --- |
| 子条目没写这个键 | 取基座的值 |
| 子条目写了（任意值） | 整体替换；**显式 `0` / `false` / `""` 也算覆盖** |
| 子条目写了 `null` | 显式清空（基座那个键不生效） |
| 值是列表 | 整体替换，不拼接 |
| 值是 map（`request_body`/`thinking`） | 整块替换，不做半合并 |

加载期硬报错：`models.<名>.extends` 指向不存在的条目（提示"先定义基座名"）、自引用、成环（`a → b → c → a`，打印完整链）、`extends` 写成空值/非字符串。未定义键的定位分两条路：`docvision setup` 的严格路径把 yaml.v3 那句 `line N: field X not found in type config.ModelConfig` 补成真实配置路径（`models.base.thinking_typ, models.drawing.thinking_typ`）；运行期的未知键告警则用反射遍历 `Config` 的 yaml tag 逐层比对节点树，同样列出**写它的条目**与**继承它的条目**。

### ③ 深拷贝（否则一次纠正就污染基座）

两处会**就地写 map**：`validateThinkingTypes` 纠正 `thinking["type"]` 的笔误、`ResolveModel` 把 `fallback.Thinking`/`fallback.RequestBody` 的 map 头直接赋给条目。有了 `extends`，一个基座可能被 4~5 个条目引用，一次纠正就会改掉基座与所有兄弟条目。现在 `validateThinkingTypes` 先把 map 拷一份再写（`cloneAnyMap`），`ResolveModel` 赋的是副本。钉住它的用例：`TestExtendsDoesNotPolluteBase`（改 `models.a.thinking` 后断言 `base`/兄弟 `b`/`text` 都是原值，且两次 `ResolveModel` 的结果互不共享）、`TestValidateThinkingTypesDoesNotPollute`（`thinking.type: disable` 的笔误纠正后基座仍是 `enabled`）。

### ④ 顺手解决的三件事

1. **`extends` 是 `ModelConfig` 的真字段**（`Extends string`）：否则 `setup` 的 `KnownFields(true)` 会把它当未知键、运行期却装作没写。
2. **同名 wire 不同单价**：价格表按厂商报的模型名索引，两个条目发往同一个 wire 名却配了不同费率时，原先是 `map` 迭代顺序随机取一条（同一份配置的费用报告会在两次运行之间变），现在加载期报错并指出两个条目名。
3. **`price` 缺失**（15 个手写零值分支里唯一没被处理的字段）：定成"条目一项费率都没写 ⇒ 继承 `models.text` 的费率；写了任意一项就用自己这一份；全 0 仍是未配置、费用报告整块不显示"（新增 `TestPriceNotInheritedWhenTextHasNone`、改写 `TestModelPricesKeyedByWireModelName` 钉住继承那一半）。
4. **旧 estimate 键的迁移提示改成按**键**匹配**：原来是 `bytes.Contains` 全文字节匹配，文件里一句"取代了 `image_tokens_max`"的注释也会触发迁移告警（改写用户配置时实际踩到）。

### ⑤ 两份模板与运行目录配置

`config.example.yaml` 重写成**教学示例**（用户明确"不要求能直接跑，要体现编辑方法与原理"）：`estimate` 三种方法、`image_tokens` 的逐键优先级链、`extends` 的三种用法（同一模型不同参数 / 跨模型共享 endpoint+key / 角色复用同一条目）与深合并语义、报错行为、`price` 继承规则，逐条短注释；`default.yaml` 保持精简可跑但新键全部出现。两份模板的键面由 `TestConfigTemplateKeyParity` 钉住（值可不同，条目名可不同）。

运行目录 `/home/share/samba-share/PDF2MD/config.yaml` 的改写稿写到 `temp/config_new.yaml`（`temp/` 已 gitignore，只读源目录）：把那四块**逐字相同**的角色条目并成一条 `heavy` 基座 + 四个角色各一行 `extends: "heavy"`，`estimate` 换新键（值与原旧键一一对应：`image_px_per_token`→`px_per_token: 750`、`image_tokens_min`→`min_tokens: 85`、`image_tokens_max`→`max_tokens: 4096`、`image_tokens_fallback`→`tokens: 1100`），其余逐字保留；**255 → 236 行**（净减 19 行：四份逐字相同的角色条目共 60 行并成基座 + 4 行 extends）。**顺带修掉一处原稿的缩进错误**：`figure_check:` 写在 `img2text:` 下（`img2text` 没有这个键——严格校验会报未知键、运行期一直被忽略），改写稿把它移回 `latex:` 段末尾并保持 `enabled: false`（校验通过、运行期行为不变：该功能本来就是关的）。

### 验证与测试

`gofmt -l .` 干净、`go vet ./...` 干净、`go test ./...` 全绿。新增/改写的用例：`config/extends_test.go`（合并语义、显式 0/null、列表与 map 替换、链式、四类报错、**深拷贝不污染基座**、严格校验认 `extends`、被继承块里的未知键定位、`extends` 条目各自带 `image_tokens`、同名同价的四角色形态、无 `extends` 配置在节点合并路径与参照路径上 `reflect.DeepEqual`、无 `extends` 配置的问题清单逐字不变）、`config/knownkeys.go` + `TestUnknownKeyWarningLines`（路径与行号、两个模板零告警）、`config/template_keys_test.go`（两份模板键面一致、版本号一致、都能加载并解析出全部角色、`image_tokens` 逐键覆盖）、`config/runconfig_rewrite_test.go`（改写稿除白名单路径外逐键逐值等于源配置、每个角色解析出的有效模型与图片计量规则一致、严格校验只剩占位符、四个角色恰好各一行 `extends`）。

**没有做的事**：`extends` 不做深层 map 的半合并（`request_body`/`thinking` 整块替换，语义表已写明）；改写稿里那处 `figure_check` 缩进修正改了行为面（虽然 `enabled: false`，但从此会被真正读到）——已在交付说明里单独列出，敢要就删掉那 8 行。

### ⑥ `--list` 的提示词列改成 token 口径（用户追加）

用户："list 列是哪里？但是感觉用 token 好一点。"`docvision sessions --list` 第 4 列量的是会话的**系统提示词快照**，此前打印字符数（`6247 字符`），现在默认打印**同一个本地估算器**算出的估计（`MetaInfo.PromptTokenEst`，与详情栏「提示词快照」同源），形态与页面一致（`≈ 1.5k`）、表头写明单位（「提示词 tokens」）。新增 `--unit token|char`（默认 `token`）切回字符：`--unit char` 的表头是「提示词 字符」、值是精确字符数（与原先一字不差），非法取值报错并列出可选值。测试钉住两种口径的表头与取值、`--list` 其余 8 列在两种单位下逐字段相同、以及纯 ASCII 与含中文假数据下的列起点（rune 与显示宽度两个口径）；实跑真实运行目录 `--dir /home/share/samba-share/PDF2MD/latex_project`（只读）核对两种单位的前 3 行。

### ⑦ 会话名里的 Markdown 也渲染（用户追加）

用户："简单的调整一下，会话名字中出现 markdown 格式也要渲染一下。"名字（转录文件名 / 图注）里的 `` `code` ``／`**粗**`／`_斜_` 此前原样显示。做法是**复用**消息正文那套自写渲染器的行内部分：`mdInline()` 早就在，只是没被单独调用过，所以只加了一个入口 `renderInlineMarkdown(text)`（一次 `mdInline(frag, text, 0)`，**不碰**块级解析，因此永不产生 `<p>/<h*>/<ul>/<br>`），再把手名落点收成一个 `nameNode(tag, cls, name)`——侧栏会话行、中栏面包屑、详情栏「会话」那一行三处共用（`kvList` 的键值对开始接受"已造好的节点"，旧写法行为不变：`undefined` 不再会落成字面量 "undefined"）。名字没有标记时 `mdInline` 找不到匹配就把整串落成一个文本节点，可见结果与改动前逐字相同（浏览器用例断言"1 个子节点 + 0 个元素"）。安全面没变：名字里的 `<`、`&` 仍只是文本，仍只经 `createElement`/`createTextNode` 落树（`<b>x>y&z*单\`` 这种文件名渲染后 `children.length === 0`）。

顺序上刻意不动：短写/截断仍由 `sessionTitleOf()` 先算出**字符串**（图注 → 可读标签 → 前 8 位哈希 + 扩展名），渲染只发生在落点；`sessionHaystack()` 一个字段都没改，所以搜索仍按原始文本（拿 `**粗体**`、反引号本身都能搜到）；`listSignature()`/`patchList()` 都没进渲染代码，浏览器用例刷新两次后给行与 `<strong>` 打过的记号仍在（`1/attached`）——把签名守卫临时改成 `if (false)` 复跑，该断言如期变成 `detached` 失败，证明它不是空转。会话行的悬浮说明改成以**原始**名字开头（标记一个不少）；`--list` 与其它终端输出不渲染。行内 code 直接用共用的 `.md-inline-code`（`--code-font`/`--code-line`），没有新样式。

证据：真实运行目录（`/home/share/samba-share/PDF2MD/latex_project`，只读）46 个会话的名字里**一个带标记的都没有**（唯一的 `_` 出现在 `chapter_003` 这种单下划线里，不构成强调），挑出 `矢量图 · $A\subset B$`、`矢量图 · $A\cup B$ / $A \cap B$ / A与B互斥`、`核对 · chapter_003` 三个真名在 chromium 里渲染，结果都是**单个文本节点、无子元素**——即真实数据逐字不变。带标记的名字只能用夹具：文件名带 `` ` ``/`**` 的 `convert_这是_斜体_与**粗体**与`code`的会话.jsonl`（合法文件名），图注带标记的逐图会话 `vector_book__<64位哈希>__label.jsonl` + `doc_index.json`，Chromium dump 出来的三处落点是同一份结构：`<span class="row-title">转换 · 这是<em>斜体</em>与<strong>粗体</strong>与<code class="md-inline-code">code</code>的会话</span>`（面包屑 / 详情栏同形，`title=` 仍是原始路径）。新增 `sessionname_browser_test.go` 三组用例（渲染+单行+纯文本对照+危险字符+图注+悬浮说明+样式比对+轮询不重建 / 搜索按原文 / 源码契约）。夹具踩到的坑记一笔：文件名里写不出 `/`，所以"闭合标签"形态只能用 `<b>x>y&z*单\``（`</b>` 会被当成目录分隔符，落盘成一个目录）。

### ⑧ LaTeX 数学渲染（用户追加："我说的就是 Latex 的数学公式的渲染，可以完整的支持吧"）

用户例子就是真实数据里的会话名：`矢量图 · $A\subset B$`、`矢量图 · $A\cup B$ / $A \cap B$ / A与B互斥`（来自 doc_index 图注）。选择**自写 LaTeX → MathML**（`viewer.js` 里 `mdMathML()`）：页面不引任何库/外链的硬约束下，MathML Core 由浏览器原生排版，比自绘或塞 KaTeX 都划算。`$…$` 行内、`$$…$$` 独立（`display="block"`），接入点就是行内正则新增的两条分支（放在最后：同一位置上只有它能匹配 `$` 起点，`$a_b$` 不会被 `_…_` 抢走）。
覆盖：分组、`^ _`（大算符自动 `munderover`）、`\frac/\dfrac/\tfrac`、`\sqrt[n]{}`、`\left…\right`、`\overline/\underline/\hat/\vec/\bar/\tilde/\dot/\ddot`、`\text/\mathrm/\mathbf/\mathbb/\mathcal/\mathit/\mathtt/\operatorname`、间距、矩阵族（`matrix/pmatrix/bmatrix/vmatrix/cases/aligned/array`）、希腊字母/关系符/箭头/大算符/常见函数名。
真实缺陷（本批自己踩到并修掉）：① `\sqrt[3]{x}` 用 `parseExpr` 读度数会把 `]` 和后面的组一起吃掉，随后 `parseGroup` 拿到 `null`、`appendChild(null)` 抛错——**一条公式抛错会打断整页渲染**（第 3 条消息整段空白，测试里 `<mroot>/<munderover>` 与 `display="block"` 全缺）；改成按字符直读 `[..]`，并在 `mdInline` 里给数学分支加 try/catch（失败就地退回 `$…$` 原文）。② `\sum_{i=1}^{n}` 的 `^` 接在 `munder` 上时没被认成大算符，接成了 `msup`、看不到上下限 → 增加"底下是 largeop 的 munder/mover 也算大算符"判定。
证据：真实数据 dump 出 `矢量图 · <math class="md-math"><mrow><mi>A</mi><mo stretchy="false">⊂</mo><mi>B</mi></mrow></math>`；新增 `internal/sessionview/math_browser_test.go`（chromium）钉住 `msup/mfrac/mroot/mover/munderover/display="block"/<mtext>\foobar</mtext>`。

### ⑩ 回退 ⑨（用户澄清是误读）

用户看到横幅 `会话预览: [::]:8849` 下方还有一行 `浏览 http://127.0.0.1:8849/`，确认是误读——原写法（Go 双栈通配 + loopback 浏览行）就是要的行为，所以 ⑨ 的 `listenNetwork`（按字面 host 选 tcp4/tcp6）与配套文档一并回退，`preview.host: "0.0.0.0"` 恢复为双栈通配、横幅照实报内核地址。教训：横幅"如实打印内核地址"本身没问题，看到 `[::]` 先看下一行的浏览地址再判断是不是缺陷。

## P5-R1 批（前端迁移重做，`p5/migration-v2` 分支）：旧迁移归档 + 脚手架与 /v2 接线

**背景**：上一轮迁移（另一模型所做，23 个提交）结构与旧页对照全部对上了，但用户判定**观感不对、不符合预期**。经用户决定：这 23 个提交归档到 `archive/p5-attempt1` 分支（**不合并**），master 复位到 `v1.5.0-beta.6`，重做在 `p5/migration-v2` 分支上从零开始。用户明确目标：**效果与旧页一致 + 代码组件化可维护，不要求逐字节复刻**。

**上一轮观感跑偏的根因分析**（对重做的约束）：逐块抽 CSS 时把静态分析误判为"死代码"的规则裁掉了（`.drawer`/`.sheet` 这类类由旧页 JS 动态切换）、CSS 被拆散后层叠顺序变了、布局外壳按想象重画、组件 scoped 样式改写了选择器。所以本轮纪律：**样式表整卷沿用旧页（构建后校验与源一致）、组件一律不写样式**。

**本批改动**：
- git 手术：`archive/p5-attempt1` 保 23 个旧提交；master reset 到 beta.6；`p5/migration-v2` 自 beta.6 切出。清掉了残留的未跟踪 `web/node_modules`。
- `web/` 脚手架：Vue 3.5 + Vite 6 + TS，lib 模式单文件 IIFE（`name: DocVisionViewer`）+ 单文件 CSS 落 `go/internal/sessionview/assets/dist/`；**迁移期关压缩**（`minify:false`/`cssMinify:false`）——曾实测 vite 默认把 44121 字节的 CSS 压成 31.65 kB，破坏"与旧页对照"的前提，且报错栈要行号；`scripts/sync-css.mjs`（构建前从旧页资产拷贝样式表）与 `scripts/check-css.mjs`（构建后校验 `dist/viewer.css` 与源逐字节一致）双守卫。
- `App.vue` 复刻旧页 `viewer.html` 的三栏骨架（含两 `.handle`、表头/页签/六开关、`#lightbox` 常驻），主题开关可用；侧栏/正文/详情内容留空待后续块。
- Go 侧：`sessionview/v2.go` 新增 `/v2`、`/v2/viewer.{js,css}` 路由（页面壳复用旧页的主题预置脚本防闪；产物缺失时 404 文案提示先 npm build）；`serve.go` 路由表加一条前缀分支。旧页路由与静态导出未动。
- `.gitignore` 补 `node_modules/`（原 `*` + `!*.*` 的组合会漏放进 node_modules 里带点的文件——上一轮 node_modules 误入库的根因）。
- 文档：CHANGELOG 把 beta.6 的批次条目从 `[Unreleased]` 归位到新的 `## [v1.5.0-beta.6] - 2026-09-12` 节（随 beta.6 发布的内容此前一直挂在 Unreleased）；AGENTS.md 修正当前标签描述；本节。

**验收证据**：`npm run build`（CSS 44121 字节与源一致）→ `go build` → 8955 端口起服务：`/v2`、`/v2/viewer.js`(205943B)、`/v2/viewer.css` 均 200；无头 chromium dump-dom 见 `#frame`/`#sidebar-col`/`#center-col`/`#details-col`/`#lightbox`/`data-v-app` 齐全；两页 stderr 均无 CONSOLE 报错行。基线截图 `temp/p5run/shot_{old,v2}_base.png`（1600×1000）。

**坑**：① headless chromium 无 `--user-data-dir` 时报 "Failed to create a unique user data directory"，必须显式给；② 服务进程用 `(cmd &)` 起会在 bash 调用结束时被回收，要起一次、同批调用内完成验证；③ `env` 里没有 proxy 变量，首判方向错了——curl 探活优先于猜。

## P5-R1 批（续）：块 2～7 迁移完成，`p5/migration-v2` 分支

承接上文，按块推进至全部迁移完成。每块一个本地提交（df4b862 / 0ab569a / 6b5b8f0 / 7fcd25c / b320053），验收看家工具是 `temp/p5run/` 里的 harness（`both.mjs` 两页同探针 + `pngdiff.mjs` 截图逐像素 diff，均支持隔离 localStorage 的 preExpr）。

**各块内容与验收**：
- **块2 侧栏树 + 数据层**（df4b862）：`legacy/sidebar.ts`（旧页侧栏函数照搬区）、`legacy/data` 层 `data.ts`（refreshIndex/pullSession 增量/selectSession/bootData 2 秒轮询 + pullSeq 防串台）、`Sidebar.vue`（proj-group/stage-group/session-row 全套类名 + `v-collapse` 指令 = 旧页 `bindCollapse`：程序化改 open 不写记忆、用户 toggle 才写回）+ `InlineMD.vue`。验收 150/150 字段一致；**真缺陷**：过滤时项目组未强制展开——旧页是 `g.matched ? true : groupWantOpen(...)`，字面 true 优先于折叠记忆，初版错用 groupWantOpen；侧栏区域 0 像素差。
- **块3 对话正文 + 富文本层**（0ab569a）：`legacy/richtext.ts`（子代理移植 950 行：块级 Markdown/highlightMachine/machineScroll/LaTeX→MathML 转换器；esbuild+DOM stub 冒烟 65 断言）、`legacy/timeline.ts`（renderLine 一族**命令式 DOM 照搬**——图片归属 attachImages 要跨节点手术，虚拟 DOM 不适合表达）、`Timeline.vue` 薄容器。验收：#timeline 范围 31/31 元素全同、交互序列（展开工具卡/仅看工具/Markdown 切换/灯箱 Esc）全链一致、**全页 0 像素差**。两个真缺陷：① `MD_INLINE` 多写 `g` 标志——模块级共享正则 + 递归调用破坏 lastIndex，嵌套行内标记被跳过（旧页特意不带 g），去掉即修；② 详情栏元信息块因探针选择器未限定 `#timeline` 混进计数，定位后确认属块 6。
- **块4 工具卡/思考块交互**：折叠记忆跨重渲、forceCollapse、io 折叠、复制按钮、家族/状态类 20/20 全同，块 3 已覆盖，无需改码。
- **块5 轨迹表**（6b5b8f0）：`legacy/trajectory.ts` 逐函数照搬（trajectoryRows 全类型/callDuration/TRAJ_KINDS 筛选/trajDetailBody/jumpToLine+flash）；state.ts 渲染入口改**注册表**（registerRenderTrajectory/renderDetails 挂点）避免循环 import。104/104 全同 + 轨迹页截图 0 像素差。
- **块6 详情栏**（7fcd25c）：`legacy/details.ts`（会话键值表/指标瓦片双口径/每次请求明细/metaCard+工具 schema）。**真缺陷两处**：① metaCard 摘要漏 `"sha "` 前缀（照搬走样）；② **Vue watch 陷阱**——`watch(() => [a,b,...], cb)` 的 getter 返回新数组只做整体 Object.is 比较，而 `state.current` 每次轮询都被换成新对象 → getter 每 2 秒重跑 → 回调每轮误触发 → 对话流/详情栏整栏重渲把用户展开的折叠冲掉；改**数组多源形式**（逐元素比较）后消除。95/95 全同 + 详情打开态截图 0 像素差。
- **块7 灯箱**（b320053）：旧页 img 不拦冒泡（点图也会关），块 1 模板误加 `@click.stop`，去掉。矢量图会话 16/16 全同。
- **整体验收**：多会话切换（chapters↔style，锚点/高亮/自动展开）、明暗两主题截图、键盘 [ ] 折叠、单位切换、轨迹跳回——20/20 全同；**明暗两主题全页截图均 0 像素差**；两页 25 秒控制台零报错。

**坑与教训**：
1. **旧服务器进程杀不掉**：bash 工具调用结束后 `(cmd &)` 起的进程在沙箱 PID namespace 外，`pkill` 连自己的 shell 一起带走（exit 143）也杀不到它，`ss -tlnp` 看不到 PID——直接换端口（8955 → 8956 → …）比纠缠进程干净。
2. **探针选择器要限定范围**：`document.querySelectorAll('pre.body-text')` 会把详情栏的 prompt-text/schema-desc 混进对话流的计数，凭空多出 6 处"差异"；限定 `#timeline` 后消失。先怀疑自己的探针再怀疑代码。
3. **模块级带 g 的正则是地雷**：移植时照抄旧页要连标志位一起抄——旧页不带 g 是刻意的（mdInline 递归）。
4. **Vue watch 多源**：见块 6，写进 AGENTS.md 陷阱清单。
5. 富文本层（~1500 行纯函数）委派子代理并行移植 + 自写 DOM stub 冒烟，效果良好；时间线/详情栏这种跨节点手术的命令式渲染留主线程逐函数对照。

## P5-R2 批（组件化收尾，直接提交 master）：时间线 / 详情栏 / 轨迹全部 Vue 组件化

背景：P5-R1 已把 `web/` 迁成 Vue + 命令式混合体（时间线/详情栏仍是 legacy 命令式 DOM + 薄容器）。本批把剩下三块命令式渲染全部改成**纯 Vue 组件**，视觉零回归。用户原话："继续web开发…很多界面可以用vue组件就写成组件那样方便维护"。

### 架构定调（四块提交，全部在 master）

- **块A `fed6966`**：`Trajectory.vue` + 基础件 `MachineText`/`CopyBtn`/`ImageStrip`。trajectory.ts 退成纯行模型（`trajectoryRows()` + `jumpToLine()`）。
- **块B `a8384ce`**：`DetailsPanel.vue` + `MdBody.vue`。details.ts 退成纯模型（`sessionKVRows`/`statsModel`/`metaModel`）。
- **块C `a87473b`**：消息流全家桶——`stream.ts` 的 `streamModel()` 把 `state.lines` 一次扫描成条目模型（assistant 带 calls / system / imageTurn / result / other），原来跨节点手术的 `attachResult`/`attachImages`/`pendingTaskImages` 全部变成**条目归属记账**：工具回执与图片行不再操作别人的 DOM，而是记进对应 CallItem 的 `results`/`attachments`/`outImages`/`previews`，由组件模板渲染。组件：`Timeline`/`AssistantMsg`/`SystemMsg`/`ImageTurn`/`ResultMsg`/`ToolCard`/`StreamSummary`/`Disclosure`/`FoldText`/`IOSection`/`MachineScrollBox`。
- **块D**：清死代码（state.ts 的数据层占位 stub、timeline.ts 未外用的命名助手转私有）、文档同步。

关键设计点：

1. **命令式叶子组件的 DOM 契约**：`machineScroll` 拆出 `machineScrollInto(host,…)`，让 `MachineScrollBox` 的根节点**自己充当 `.text-wrap`**——否则组件根 div 会插在 `.io-section` 与 `.text-wrap` 之间，结构探针的类名链立刻多出一段 `>`。视觉虽无差，但 DOM 逐层一致才好验收。
2. **forceCollapse 的真实语义**：旧页「折叠全部思考」开 = 命令式全部合上且**不写记忆**，关 = **什么都不做**（等下次自然重渲才按记忆恢复）。组件化后要在 `watch(forceCollapse)` 里只做单侧动作，不能对称地"关=恢复"。
3. **自动跟随的自动取消**：旧页 timeline 有个 scroll 监听——手动滚离底部 40px 即把跟随开关按灭。这是页眉「自动跟随」按钮唯一会自己变化的路径，图片会话（长转录自动滚动）一触发就露馅，补上后页眉 0 像素差。
4. **展开记忆的恢复时机**：旧页整流重渲（会话/行数/Markdown/仅看工具/单位变化）时按记忆恢复思考块开合；组件化后对应成对这些源 watch 一次 `syncThink`，DOM 直改（用户手点 `<details>`）与记忆写入仍由 Disclosure 的 toggle 事件承担。

### 迁移中抓到的真缺陷（全部由探针/像素差逼出）

1. **IOSection 漏 import `MachineScrollBox`** → 模板渲染成未解析的自定义元素，io 文本内容整块消失（probe3c 145/155 diff），**控制台零报错**（unknown element 静默）——内容缺失先查渲染 HTML 再查 console。
2. **FoldText 的 `str.replace` 误伤**：给 md 分支补 `md-body` 类时，同一写法的 pre 分支（`:class="[extraClass, …]"`）也被整体替换，纯文本态类名混进 `md-body`（probe3d 5 处 diff）。
3. **面包屑会话名多包一层 span**：旧页 `nameNode('span','crumb crumb-current',…)` 的渲染根就是 crumb 本身；Vue 版先 span 包 InlineMD 再塞进 crumb，行内 Markdown 语义不变但 DOM 多一层，图片会话页眉 61×19 像素差。修法：`<InlineMD tag="span" class="crumb crumb-current">`。
4. **缩略图预览行漏渲染**：块C 第一版只在 ToolCard 里渲染了结果/附件，details 的**兄弟节点** preview-strip 忘了（probe7b previewStrips 3→0）。
5. **`str.replace` 全量替换的教训**（同 2）：对出现多次的模板片段做字符串替换前先数出现次数。

### 验收证据

- 结构探针：probe3c（时间线 155 字段）/ probe3d（Markdown 开关 44）/ probe4（思考与折叠交互 20）/ probe5（轨迹 104）/ probe6（详情栏 95）/ probe7b（图片会话灯箱 16）/ probe8（综合验收 20）= **454/454 全一致**。
- 像素差：5 场景（对话 / 轨迹 / 详情打开 / 深色主题 / 图片会话）**全部 0 像素差**（容差 8，最大通道差 0）。
- 控制台：25 秒监听零报错。
- 环境噪声备忘：两页 origin 不同 → localStorage 折叠记忆不同会让侧栏整块像素差，用 `temp/p5run/dumpstore.mjs` 把旧源 6 条界面记忆镜像过去即归零；活跃会话正在写入时侧栏计数会因轮询相位瞬态不同，截图脚本已加"侧栏文本稳定 6 秒"等待。

### P5-R2 收尾用户反馈（两条新口径，随 `web/components` 分支落地）

1. **验收口径放宽**："不需要逐像素一致，看着一致就可以了。功能上一致就可以。做好组件化的。" —— 逐像素 diff 与 DOM 逐层一致降为参考项，硬门槛只剩功能探针 + 控制台零报错；组件化质量是第一优先级。P5-R2 各块当初按逐像素标准验收（0 像素差）不浪费——那是迁移期保真手段，后续开发不再背这个成本。
2. **web 开发走分支**："从之前合并回来的那个节点上对于web的开发这边还是放在一个web分支上去开发吧。" —— 建分支 `web/components`（自 `f999b03` 合并节点分出）；原直接提交在 master 上的组件化四块（fed6966/a8384ce/a87473b/a17a61f）随分支走，**master 退回合并节点**。分支操作记录：先 `git branch web/components` 后误在分支上 `reset --hard f999b03`（指针落错侧），纠正为 `checkout master → reset --hard f999b03 → branch -f web/components a17a61f → checkout web/components`；教训：`git reset` 落点只看"当前在哪个分支"，切分支和动指针要分开核对。

### P5-R2 质量批（`web/components` 分支）：类型层 + 单测 + 锚点接口化

按代码评审结论做三项优化（用户认可"组件化质量优先"口径）：

1. **`legacy/types.ts` 类型层**：`Line`（转录行：n/t/bad/role/text/reasoning/tool_calls/images/est…+ unknown 索引签名）、`ToolCall`、`ImageAttr`（归属三态）、`ImageCallEntry`（FIFO 队列条目）、`Session`、`UsageStats`。stream/timeline/trajectory 的 **42 处 `any` 清零**；`estOf()` 收紧成非可选数字面（缺失按 0，不往下游漏 undefined）；`state.lines`/`state.current`/`callEst`/`imgAttr` 全部带型。
2. **纯函数单测**：vitest（node 环境 + localStorage 打桩），`web/tests/` 23 个用例钉住归属算法的全部规则分支（精确匹配优先/工具名/顺序 FIFO/失败调用不占队/任务不许抢/开头投喂两轮补挂/未识别兜底/轮次索引缓存）与 streamModel（meta/usage 不进流、onlyTools 过滤、回执并卡 lastStatus、配对失败独立行、call-id 无回执退附件、预览条挂轮末 view 卡、callMsgLine）。**23/23 通过**；`package.json` 加 `test` 脚本。
3. **锚点接口化**：行号→DOM 的映射从 `state.anchors`（reactive，会把 DOM 节点包成响应式代理）挪到 state.ts 模块级 `Map` 注册表：`registerAnchor/unregisterAnchor/anchorOf/clearAnchors`；消息组件 onMounted 注册/onBeforeUnmount 注销，ToolCard 对 anchorNs 做差量增删；trajectory `jumpToLine` 改走 `anchorOf()`。行为探针确认：点轨迹行 → 切回对话 → 对应 `msg-assistant` 节点 flash 高亮。

**真实教训（两次）**：`tsc --noEmit` 不检查 `.vue` 的 script 块，vite 构建也不报——AssistantMsg 漏 `onBeforeUnmount` import、ToolCard/AssistantMsg 漏 `registerAnchor` import，都是**运行时才炸**（`ReferenceError: … is not defined`，整个消息流组件挂掉、页面只剩 1 条消息）。第一次是探针 454 字段跑出 219 处 None 才暴露的，单靠"探针里有值"的条目发现不了——所以控制台零报错必须是每轮验收的固定动作。补了一个自查脚本（grep 各文件用到的 state 模块导出 vs import 面板），归入 web 开发流程；最终全绿：454/454 + 控制台零报错 + 5 场景 0 像素差 + 轨迹跳回行为验证（点行 → 切对话 → 目标节点 flash）。

### P8 侧栏仿 DSH 改版 + T12/T13（`web/components` 分支）

**T12（平均首字长浮点）**：`fmtDur` 亚秒分支 `v + 'ms'` 直接漏浮点（用户见 `991.4705882352941ms`）。新旧两页同款修复：`Math.round(v) + 'ms'`。v2 探针 probe5 的 `tiles[3]/value` 从此与旧页（未修）差 1 处——**预期内差异**，453/454。

**T13（旧版单项目徽标误显）**：两层根因，都在"组级 OR"上。
1. Go `projectGroupFor` 对 `<书名>/work/sessions/…`（现代布局最常见落点）走到兜底分支 `Legacy: isProjectWorkspace(root/书名)`——书目录必然是工作区 → 每本书都误挂徽标。修法：`seg[1]∈{work,source} && seg[2]=='sessions'` 的转录落点是现代布局，强制 `legacy=false`；`work/<file>.jsonl`、`sessions/<file>.jsonl` 直接躺着的仍是旧式。测试补「书直接挂扫描根下」用例（原有测试都是 `容器/书名` 两层，恰好漏掉真实部署的单层形态——教训：**测试布局要照真实部署摆**）。
2. 前端 `if (s.projectLegacy) g.legacy = true` 组级 OR：一本书里只要有一笔旧式落点（真实数据里 `<书>/work/style_session.jsonl` 就在 work/ 根下），整组又被标回。语义收窄：徽标只属于**根组**（输出根即工作区）；Go 语义（组目录是工作区根）不动，测试不破。新旧两页同修。

**P8 侧栏改版**（样式仍整卷进 `assets/viewer.css`，新旧页共享——但新类名旧页不引用，不影响旧页观感）：
- **文件夹图标**：项目行槽位换成 DSH 风格内联 SVG 文件夹（展开=打开的文件夹、着色 accent；收起=闭合轮廓），折叠轨道里保留 16px 图标，辨识度比原来的点/箭头好。
- **层级递进**：阶段行缩进 24px、会话行 40px（`sub1`/`sub2` 类，hover 背景仍全宽）。
- **阶段状态三态**：`stageStatusText(stages, stage, live)` —— 会话活着 → 「运行中」（绿、优先于 progress.json）；`done` → 「✓ 完成」（绿）；空值 → 「未开始」（暗）；其他值 → 原文（琥珀）。`.running/.busy/.idle` 配色。
- **会话方块视图**：侧栏头部 ▦/☰ 切换（记忆 `side.blockView`），开启后会话渲染成 20px 方块网格（`session-blocks`），逐图会话块内显示书内序号，活跃=accent 实心、运行中=绿圈；悬浮出 `sessionTip` 全量说明；**不受 8 条 overflow 限制**（方块就是为几十个会话设计的），折叠轨道里隐藏。
- **响应式**：验证 900px（自动折 56px 轨道，文件夹图标保留）与 1280px（详情栏收起、中栏让步）。

验收：探针 453/454（唯一差异=T12 修复本身）；控制台零报错；单测 23/23；截图逐张读图确认（列表/方块/密集 60 会话/深浅主题/900/1280）。新增 `temp/p5run/shotw.mjs`（指定视口宽高截图）。**沙箱注意：bash 每次调用的 /tmp 是独立 tmpfs，跨调用文件一律写工作区。**

### 用户口径更新（P8 批末）：旧页正式冻结

P8 批里把 T12/T13 同步打进了旧页 viewer.js——这是**最后一次**为旧页改代码。用户明确：迁移已完成，旧页不再维护，之后 bug 也不修，一切改动只做 v2。样式表 `assets/viewer.css` 仍整卷共享（sync-css/check-css 纪律不变），新类名旧页不引用即可，无需再为旧页观感让步。旧页继续存在的意义只剩两样：探针对照基准 + 用户还没切过去的入口。

### P11 CSS Vue 化（`web/components` 分支）

**做了什么**：把 1368 行的整卷 `viewer.css` 按归属拆分——全局基础样式进 `web/src/styles/base.css`（token/深色主题/重置/滚动条/通用小件 `.icon-btn`/`.badge`/`.dot`/跨组件共享词汇 `.msg`/`.line-*`/`.disclosure`/`.io-*`/`.code`+高亮色板/`.images`/`.flash`/`.lightbox`/md 正文词汇），组件私有样式进各 SFC 的 `<style scoped>`（App=三栏骨架+中栏头部、Sidebar、Timeline、StreamSummary、SystemMsg、AssistantMsg、ImageTurn、Trajectory、DetailsPanel；FoldText/MdBody 的规则因是共享词汇/作用于 v-html 内容，整块归 base）。旧页资产 `assets/viewer.css` 从此**冻结**（保持 46355 字节原样，旧页继续可用）；`sync-css.mjs`/`check-css.mjs` 退役删除（字节守卫的前提——两页共享同一份样式——已不复存在）。

**为什么共享词汇必须放全局（三个实测陷阱，都由探针/逐元素对比抓出）**：
1. **跨组件类放 scoped 会静默丢样式**：`.line-*`/`.disclosure` 被 7 个组件使用，最初放 FoldText scoped 后，SystemMsg 自己模板里的 `.line-summary` 匹配不上，行高 20→24、系统消息 +8px——探针 probe8 的 `followOff` 因此翻转（详情栏重开后内容多高 48px>40px 阈值，自动跟随被滚离逻辑取消）。教训：**先 grep 类名的组件分布再决定归属**。
2. **`:deep()` 会反转原文件的优先级次序**：md 正文规则经 MdBody scoped 的 `:deep()` 下发后，每条选择器都追加 `[data-v]`，`.md-p`（原 0,1,0）与 `.md-body > *:last-child`（原 0,2,0）被拉平成同分值、按书写顺序决胜 → `.md-p` 的 margin-bottom 8px 复活（`.md-body > *:first-child` 的 `:deep(> …)` 与同元素选择器 `.md-body.clamped` 也一并失配）。而 **v-html 生成的 DOM 根本不带 scope 属性**——md 正文整块退回全局（md-* 类名自命名空间化，无碰撞风险）。
3. **对照测量前先对齐 localStorage**：两个端口 origin 的界面记忆（侧栏/详情栏展开与宽度）让 timeline 宽度差 176px，逐元素高度对比全是环境噪声——"每条消息 +12px"里真差异只有来自陷阱 1/2 的那部分。

**验收**：探针 453/454（唯一差异=T12 的 `tiles[3]/value` 取整，旧参照 8955 是冻结前二进制）；控制台零报错；vitest 23/23；`go test ./internal/sessionview/` 全绿（sessionview 的浏览器测试对页面断言不受影响）；截图目检列表/方块/轨迹/深色主题与 P8 构建一致（dist CSS 58.6KB vs 46.4KB，膨胀来自 scoped 属性复写）。

### T18 预览缩略图收小（`web/components` 分支，P11 后续）

**现象**：view 类调用下的缩略图预览行把折叠流撑出大段空档——横图（如 1300×460）按 `max-width:240px` 约束后仍占 240×86，折叠态的对话里每个 view 调用上下多出 ~86-108px，视觉上「挤压上下文本、图片贴最左」。先对比了 v2（8981）与生产 8849（旧页）的同会话同图：两边几何**逐项相同**（240×86、x=532、缩进 20px、折叠行 24px）——「和 v1 不同」是更早版本的印象，实际是这组 token 从引入起就偏大。

**修法**：只动 v2 的 `base.css` token（P11 后旧页资产冻结，两页不再被同一份样式绑死）——`--preview-h: 108px→72px`、`--preview-w: 240px→160px`。横图 1300×460 → 160×58，竖图最高 72px；缩进 20px（与工具名对齐）保持。灯箱可看原大图，职责不变。

**验收**：探针 453/454（唯一差异仍=T12 取整）；控制台零报错；实测宽图样例 160×58/160×69/143×72。

### T18 修正：真正的根因是 IOSection 插槽掉进网格第一列（`web/components` 分支）

**上一小节的结论错了**：「缩略图收小」不是用户要的——用户澄清：缩略图（108/240）本来就没问题，出问题的是**展开的工具卡片内部**——图片和文字拥挤、图片贴最左边。token 已回滚到 108/240。

**真根因**：旧页把精确匹配的输出图片 append 进 `.text-wrap`（`col.appendChild(imageStrip(line))`，注释原话「text-wrap 里、滚动区之外」）——`.text-wrap` 是 io-section 网格的第二列单元格，图片自然从文字列 x 起排。组件化时 `<ImageStrip>` 经 IOSection 默认插槽成为 `.io-section` 的**直接子元素**，网格自动布局把它排到第二行**第一列**（标签列）：图片贴最左、`max-content` 的第一列被图片宽度撑开（竖图 276px、横图 520px）、「输出」文字被挤进右侧窄条——用户截图里的「文字挤成一条 + 图片在最左」就是它。

**修法（保持 Vue 组件化）**：IOSection 里插槽内容包进 `.io-extra` 容器，`grid-column: 2` 钉回内容列（`.io-extra { display:flex; column; gap:6px }` 进 base.css 的 io 词汇）。插槽用法不变（ToolCard 仍往默认插槽塞 ImageStrip + attach-note），布局语义与旧页一致：图片在输出正文下方、同一列起排、归属脚注跟随。

**验证**：截图目检竖图（1280×1951 → 276×420）与横图（1200×686 → 520×298）卡片——输入 JSON 全宽高亮、输出文本全宽、图片在内容列、脚注跟随；探针 453/454（唯一差异仍=T12 取整）；控制台零报错；vitest 23/23；sessionview go 测试全绿。附：截图时踩了个 CDP 坑——卡片在闭合 `<details>` 里 rect 全 0，必须先 `details.open=true` 再 scrollIntoView 再截视口。

### T22 方块编号 + 完成状态着色（`web/components` 分支）

**需求**（issue.md T22）：章节转换/章节核对的方块没有编号；方块要能显示完成情况——正常=结束、绿色=进行中、红色=错误终止。

**数据源考察**：progress_items/ 只覆盖图片（矢量会话），章节没有逐项进度；progress.json 是档位级的（stage → done/""），粒度太粗。真正的两个信号都在现成数据里：
1. **章号**在转录文件名里：`convert_chapter_001.jsonl` / `checker_chapter_001.jsonl`（`chapter[_-]?0*(\d+)$` 提取；"chapter" 一词保证不误读其他会话名里的哈希/日期数字）。
2. **完成情况**在转录回执里：全部四类 submit 工具的回执都以 `SUBMITTED. ` 开头（tools.go/tools_chapter.go/tools_convert.go），assistant 正常文本只写 "Submitted"——所以 `readStat` 的同一趟流式扫描里加两次**原始行子串检查**（`"role":"tool"` 与 `SUBMITTED`，零额外 JSON 解码）就能得到每会话的结束状态：SawSubmit → done / SawTool&&!SawSubmit → error / 都没有 → pending。真数据验证：glm 项目的两个 convert（COMPILE FAILED / NO MATCHES）正确落红，0911_1 的 8 个只有 meta+user 的 vector 会话正确落暗。

**呈现**（P11 后样式在 Sidebar.vue scoped）：方块文字 = `imageOrder || chapterOrder`；状态类 `err`（红底红字）/ `pend`（opacity .42）只在不 live 时挂——运行中的绿圈优先于一切；正常结束不加类（默认样子就是"已结束"）。悬浮说明补一行状态语义与章号，`sessionHaystack` 补检索词（第N章/chapter N/错误/已提交）。

**验收**：截图目检（glm 章节转换 [红2][红1][暗3]、测试-概率论 章节转换/核对 3 2 1 4）；探针 453/454（唯一差异仍=T12 取整）；控制台零报错；vitest 23/23；sessionview 新增 3 个单测（chapterOrderOf/endStateOf/readStat 回执信号）全绿。

### T19 文件夹图标左遮挡（`web/components` 分支）

**需求**（issue.md T19）：文件夹图标在被选中（展开）时左侧有点遮挡。

**根因**：把 open/closed 两个 SVG 放大到 96px 截图目检——closed 没问题；**open 图标的背板左斜边（`h-12l-1.5 6` 的收尾斜线）与前盖左斜边（`M1.5 13.5l1.6-6` 的起笔斜线）相互交叉**，交叉区在图标左下形成一团乱线，14px 下看就是"左侧被糊住/遮挡"。

**修法**：重画 open 图标的两个 path——背板从右上起笔、左缘走**垂直直线**（`M14.5 8V5.5a1…H2.5a1…v9a1…h2.2`，底部只留短 stub 与前盖衔接），前盖是纯平行四边形（`M4.9 14.5 6.7 7.5h7.7l-1.8 7Z`）整个落在背板内侧；两条斜边不再有任何交叉。closed 图标不动。

**验收**：96px 放大对照（左右侧线条干净、与 closed 图标的 tab 造型一致）+ 侧栏 14px 实测截图；探针 453/454；控制台零报错；vitest 23/23。

### P6 灯箱滚轮缩放（`web/components` 分支）

**需求**（plan.md P6）：点击图片进入的预览界面，增加滚轮缩放。

**实现**：state.ts 灯箱加 `scale/tx/ty`（`translate(tx,ty) scale(s)`、origin 中心）——`zoomLightbox` 以鼠标点为锚（`t' = t + p·(s−s')`，p 为光标相对图像中心偏移），钳制 0.15–8 倍；App.vue 灯箱根接 wheel/pointer/dblclick——按住拖动平移（Pointer Capture），**拖动过吞掉 click**（原「点击关闭」只在微动 <3px 的纯点击上生效，行为向后兼容），双击复位。Esc 关闭沿用。

**CDP 实测**（合成事件 + 数值断言）：锚定误差 <1.5px；35 步放大停在 scale(8)、40 步缩小停在 scale(0.15)；拖拽 translate 精确跟手；拖后不关、微动点击照关、双击复位全过。探针 453/454；控制台零报错；vitest 23/23。

**排错插曲**：断言用的正则没容忍浏览器对 style.transform 的归一化空格（`translate(xpx, ypx)`），前两轮「假失败」——CDP 调试脚本加了 exceptionDetails 输出后定位。

### P9 轨迹点行 → 右侧详情栏看该步（`web/components` 分支）

**需求**（plan.md P9）：类似 DSH，轨迹界面点工具调用行，在右侧边栏看详细信息。

**实现**：行点击从「跳回对话」改为「选中该步」——`state.trajSelected` 存行身份，DetailsPanel 顶部条件渲染「步骤 #N」块（`trajStep` computed：仅轨迹视图 + 有选中时，从 `trajectoryRows()` 取行，detail/images 与轨迹页就地展开同源；类型标签 + 名称 + 各 detail 段（MachineText 代码高亮/纯文本）+ CopyBtn + ImageStrip + × 关闭），会话/指标/元信息块仍在其下（DSH 同款：步骤详情置顶、会话信息跟随）。跳回对话收进行内 `.traj-gochat`（↳，悬浮显形），选中行 `.selected` 高亮；再点同一行取消。

**撞行缺陷（真数据抓到）**：第一版用 `row.jump` 定位步骤——工具行的 jump 是**assistant 消息行号**（call 挂在消息行上），点工具行右栏却显示「助手 AI」；且一条消息多次调用时 jump 还会撞工具行彼此。修法：`TrajRow` 增加**确定性 `rid`**（`kind@jump#序号`，append-only 转录下索引稳定），`trajectoryRows()` 末尾统一赋值；state.trajSelected 存 rid。CDP 复测：点 `list_source_pages` 工具行 → 右栏「工具 list_source_pages · 输入」✓，↳ 跳对话且选中保持 ✓，再点取消 ✓。

**探针同步**：P9 是**有意的新旧行为分叉**（行内多了 ↳ 按钮、行标题换了文案、点行不再跳走）——probe5 的 num 归一化（去 ↳ + trim，浏览器把 style 序列化留尾随空格）、title 文案映射回旧语义；probe8 的跳转改点 `.traj-gochat`（旧页没有该按钮则退回点行本身，两页语义都验「跳回对话」）。全套回到 453/454（唯一已知差异仍 = T12 取整）。

**验收**：截图目检（选中高亮 + 右栏步骤块置顶 + 会话/指标跟随）；CDP 断言（选中/取消/关闭/跳转保持选中）；探针 453/454；控制台零报错；vitest 23/23。

### P7 实时流式状态（`web/components` 分支，Go + web 两侧）

**需求**（plan.md P7）：对话实时进行时能看到流式状态——输出一个字一个字的出、工具调用也是。

**为什么走磁盘 sidecar 而不是内存广播**：转录是整行落盘的 append-only JSONL（`Append(msg)` 在消息完整后才写）；且 `docvision sessions` 预览服务可能是**独立进程**，看不到跑会话进程的内存。所以选 `<转录>.partial` sidecar：会话进程流式期间节流覆写、完整消息落盘即删，任何进程都能读。

**Go 侧**：
1. `chatstream` 已有 `OnContent`/`OnReasoning` delta 回调；`client.readStream` 在两个回调里累积 `strings.Builder`，**~150ms 节流**调 `req.StreamHook(phase, tail)`（尾部截断 16KB）。hook 挂在 `ChatRequest` 上（`json:"-"`）——**每请求作用域**，并发会话共享一个 client 也不会互相覆盖（ 比 client 字段安全）。
2. `TranscriptWriter.WritePartial`（tmp + rename 原子覆写）/ `ClearPartial`；`appendTranscript` 每次追加后清 sidecar（完整消息已落盘、快照过期），`Close` 也清（会话中途消失不能留过期快照）。session.Run 里给每个请求接上 hook。
3. `sessionview` 的 `/api/session` 响应加 `partial` 字段（读 `<path>.partial`，空文本视为无）。
4. 单测 `TestPartialSidecar`：覆写无 tmp 残留、清干净、Close 清。

**Web 侧**：
- `PartialTail.vue`：live 会话消息流末尾的实时卡片——「思考中/输出中」+ spinner + 新鲜度 + 累积文本尾部（>4KB 截头）+ 闪烁光标 `▍`；等宽安静样式、**不做 Markdown 解析**（半截 Markdown 渲染会闪）。data.ts 轮询增量接口时同步 `state.partial`（服务端不返回即置空，卡片自然消失；selectSession 清）。
- 工具运行 spinner：`Disclosure` 加 `running` prop（摘要行绿色「运行中」），ToolCard 传 `!item.lastStatus && live`——没回执且会话 live 才转圈；死会话的悬空调用不转圈。

**验证**：workspace 内 fixture（真数据盘只读，模拟 live 会话：mtime 60s 窗口内 + 手写 .partial）端到端——API 返回 partial ✓、UI「输出中/思考中」卡片 ✓、删 sidecar 后卡片消失 ✓、call_2/call_3（无回执）转圈而 call_1（有回执）不转 ✓；截图目检。探针 453/454（唯一已知差异仍=T12）；控制台零报错；vitest 23/23；go test 全绿。

**排错插曲**：① `pkill -f 'dv3 sessions.*8990'` 匹配到**自己这条命令行**把刚起的服务器杀了（exit 143）——换端口号重起；② python 批量改 props 的静默 no-op（reconstruct 字符串不匹配也不报错）导致 `running?: boolean` 没加进去，第一轮 fixture 测试全 false——用 `edit` 工具按精确文本修。真数据盘 `/home/share` 对本 shell 是只读挂载，测试 fixture 放 workspace。

### T20 + T21 排查与压缩检查点 DSH 化（`web/components` 分支，Go 侧小改）

**T20「image_context 为什么 error」——定性：不是 image_context，也不存在边界报错。**
全量扫描 5 个矢量会话目录（0911_1 / 测试-概率论 / 2026书…）：17 次 `image_context` 调用**全部成功**（回执形如 `image N of M in this document: …`）；代码上 `ImageContextTool` 对首/尾图边界返回 `(none)`（build() 对 i<0 / i>=len(refs) 直接给 "(none)"），只有「目标图在 markdown 里找不到 / 空目标且未绑定当前图」才报错——与「下一张没图了」无关。
用户看到的那条红 error 是 **`view_image`**（`vector_测试-概率论__220c0aa…__geometric_figure.jsonl` 行 94）：模型把 64 位哈希文件名**敲错了**（`…deb97319f5d19c44db` 写成 `…deb97333f5d19c44db`），工具按既有语义如实回报 `TOOL ERROR: 文件不存在 …（请用图片文件名或 markdown 中的引用路径）`——UI 的轨迹/状态列标红是 `classifyResult` 命中「文件不存在」，显示正确，无需修复。

**T21「压缩是不是 user 注入 + DSH 的提示与 XML 包裹」——两点：**
1. 注入角色**就是 user**，且这是对的：system 消息插在会话中间违反多数 provider 的消息序规则、也会打断前缀缓存；DSH 的压缩检查点同样以 user 轮注入。不改。
2. 补齐 DSH 式包裹：`compact()` 的替换注释从裸摘要改为 **marker 行 + `<compacted-summary>` 说明文字（"This is an automatically generated checkpoint… build on it without restating it. Continue the task directly…"）+ `<summary>` 摘要**。首行 `=== COMPRESSED SESSION CONTEXT` marker 不动——`LoadTranscript` 续跑折叠靠 `HasPrefix(marker)` 认注释，新旧两种形态都兼容。新增单测 `TestCheckpointNote`。

### 详情标题样式回归 + 灯箱点击/双击冲突（验收反馈两连修，`web/components`）

**①「详情」标题样式没了**：截图目检 + `getComputedStyle` 定位到 `.details-title` 计算样式全丢。根因是 **P11 scoped 陷阱①的又一例**：`.details-head`/`.details-title` 的规则在拆样式时写进了 **DetailsPanel.vue** 的 scoped 块，但这两个元素由 **App.vue** 渲染——`data-v` 属性对不上，规则整条失配（构建期与控制台都不报错）。修法：规则搬进 App.vue 的 scoped 块，DetailsPanel 里留一行注释指路。验证：computed `600/13px/muted` ✓ + 截图目检 ✓。

**②双击复位 vs 点击退出冲突**：单击立即关灯箱 → 双击的第一击就把灯箱关了，双击复位永远触发不了。修法：**单击延迟 ~260ms 裁决**——窗口内来第二击按双击（复位、保持打开），否则关；拖动过的 pointer 序列吞 click 的判定从「click 时 lbDrag 已被 pointerup 清掉」改成 **pointerup 时定格 `lbSuppressClick`**（顺手钉死拖拽后误关竞态）。CDP 验证：滚轮放大→双击复位且保持打开 ✓→单击 ~620ms 内关 ✓。探针 `probe7b.imgClickKeeps` 一度出现差异（旧页点图立即关=False / 新页 250ms 时仍开=True）——查明是**探针 settle 只有 250ms、赶在 260ms 关闭定时器前**的计时假象，语义（点图冒泡关闭）没变；探针该处等 450ms 后两页对齐（453/454 基线保持，唯一已知差异仍 = T12）。

### T20 补刀：image_context「正常回执被标 error」根因与修复（`web/components`）

用户验收截图：be53f9a 矢量图会话里 image_context 的**结果行内容正常**（`image 4 of 4 in this document: …`）却挂着红 error。定位：`classifyResult` **全文**扫 `/REJECTED|文件不存在|失败|error|not found|traceback/i`，而 image_context 的回执会**引用图片周围的书中正文**——正文里「在第一次失败的条件下」命中「失败」。全仓扫描定量：24 条全文命中的工具回执里 **11 条是这类误报**（首行干净、错误词来自引用正文），其余 13 条是 COMPILE FAILED / TOOL ERROR 等真错误（它们的「Error:」在第二行，靠正文命中才被判出来）。

**修法**：错误判定只看**回执首行**（`s.split('\n',1)[0]`），并把 `COMPILE FAILED` 前缀显式加进标记词（否则编译失败会漏报——vitest 新用例先抓到了这一点）。影响面：streamModel 工具卡、trajectory 结果行、ResultMsg、details 步骤块共用 classifyResult，一处修全修。旧页冻结不动（仍有误报，属预期分叉）。be53f9a 会话 CDP 实测：image_context 结果行 `data-error` 消失、状态恢复「—」；探针 453/454（style_session 探针路径没踩到误报行，无需归一化）、vitest 24/24、控制台零报错。

### P13 缩略图开关（`web/components`，web 迁移线收尾项）

**需求**（plan.md P13）：UI 增加一个功能，选择是否展示缩略图。

**实现**（三行改动，照 Markdown 开关的既有模式）：① `state.showThumbs`（默认 true）+ `loadState` 读 `storeGet('showThumbs') !== '0'`；② App.vue 页签行右端（Markdown 旁）加「缩略图」`tab-toggle`（id `thumb-toggle`、aria-pressed、悬浮说明），点击翻状态并 `storeSet`；③ AssistantMsg 的 preview-strip `v-if` 加 `state.showThumbs &&`——纯响应式跟随，无需手动重渲链。v2-only（旧页冻结不加）。

**验证**：CDP——默认 23 条 preview-strip → 关 0 条（aria-pressed=false、`dsh.sessionview.showThumbs=0` 落盘）→ 开 23 条恢复；**重载后仍 0 条**（loadState 读回记忆），再点恢复 23。截图目检开关位置与高亮态。探针 453/454（默认开，与旧页基准无分叉）、vitest 24/24、控制台零报错。这是 web 迁移线最后一个 plan 项——完成后 `web/components` 非快进合回 master（用户批准）。

### T14 崩溃修复：clientFor 并发写 map（master，post-merge 第一批）

**用户崩溃栈**：`fatal error: concurrent map writes` @ `runner.go:328 clientFor` ← `convertOneChapter`（book.go:887）← convertPhase goroutine。convert 阶段每章一个 goroutine，各章**首次**解析到自己那章的模型（convert_01/02/03…）时同时写 `r.clients`/`r.models`——写写碰撞直接 fatal，整次运行作废。

**修法**：`Runner` 加 `cfgMu sync.Mutex`；**全部 16 处** `r.clients[...]`/`r.models[...]` 裸访问收口成三扇门——`clientFor`（懒创建，锁内）、`modelOf`（读，锁内）、`hasModel`（存在性探测，figure-check 回退用，锁内）。读点也必须收口：Go map 读到一半被并发写同样 fatal。批量替换时正则曾误伤 `r.models[name] = mc` 赋值行与 modelOf 自身（`return r.modelOf(name)` 无限递归）——go vet 立刻抓出，逐一修回；教训：**map 访问批量改写后必须核对函数体内部的自引用**。

**验证**：新增 `TestClientForConcurrentAccess`（8 goroutine × 8 模型名并发 clientFor/modelOf/hasModel，`-race`）通过；全仓 latex/session/sessionview 测试绿。

### T15：img2text 校验失败从「警告」改记「错误」（master，T14 同批）

**用户发现**：img2text 最终文件里，所有被标「警告」的图都没嵌入——但查日志发现它们是 mermaid 校验**未通过**（真错误），却被显示成警告。口径裁定：警告只留给**可自动纠正**的事（如流式降级重试）；校验未通过 = 错误。

**病灶**：runner.go writer 的 `__INVALID_RESPONSE__` 分支（`StatusRetry` 汇聚点——`[IMG_MERMAID_INVALID]`/`[IMG_INVALID_FORMAT]` 都走这）`warnCount++` + `LogWarning`；而每个 goroutine 已经先打过 `✗ FAILED`（error 级）——两级日志一个 error 一个 warning，analyze/CSV 按 warning 归类，口径打架。

**修法**：该分支改 `errorCount++` + `LogError`；**跳过不落进度、下轮重试的语义不变**（不嵌垃圾进最终 md 是对的）。docs/commands.md 的 img2text 状态表同步改「错误（跳过重试）」。验证：img2text 测试全绿。

### T11：档位1 紧凑控制台进度行消失（master，与 T14/T15 同批）

**用户发现**：终端 CLI 下 classify 等阶段一行进度都没有（"之前还有"）。

**根因**：`cd1dec9`（实时进度改每人一行块）在 `paintLiveLocked`/`Finalize` 里加了 `if l.quiet { return }`——而 RunBook 紧凑模式（非 `--verbose`）正是 `SetQuiet(true)`。于是档位1 全程的 LiveRow（classify/process/style/convert/checker 进度行）与阶段终态行全被静音；quiet 的本意是压**明细日志行**（会话轮次/工具调用），不该压进度块本身。档位2 img2text 不受影响（进度行是它自己 `fmt.Fprint` 打的，不走 LiveRow）。

**修法**：去掉两处 quiet 早退——quiet 下 LiveRow 照常渲染、Finalize 终态行照打（滚动缓冲留得住阶段结果）；临时验证测试（管道捕获 stdout）确认 quiet 下进度行与终态行都上屏，随后删除（行为由回归测试兜底）。

### T25：`extends` 支持多重基座（master，T14/T15/T11 同批）

**需求**：`extends: [a, b]` 按顺序后面覆盖前面（用户示例里连写两个 `extends:` 键——YAML 重复键直接报错，所以列表是唯一可行形态）。

**实现**（config/merge.go，加载期 YAML 节点层）：`extendsRef` 升级为 `extendsRefs`（标量=单基座不变；序列=多基座，逐项校验非空条目名）；解析循环的账本改 `map[string][]string`，一个条目在**所有基座都解析完**后才合并；折叠语义 = `acc = p1`，逐个 `applyExtends(copy(pᵢ), acc)`（后面的基座赢），最后 `applyExtends(entry, acc)`（条目自己的键最高）——折叠在 **shallowMapCopy** 上进行（applyExtends 会重写 child.Content，直接拿父条目当 child 会把文档里的基座改掉）。循环检测 `extendsChain` 升级多边版（优先沿未解析的父链走）。knownkeys 的继承归因同步多父。合并语义不变：仍是**条目键级**（嵌套 map 整块替换，TestExtendsDeepMergeSemantics 钉着）。

**测试**：新增列表顺序覆盖（base_url/model/api_timeout/request_body 整块替换）、链式 + 列表混用（`extends: [mid, base]`，base 在后覆盖 mid）、空列表报错、列表含不存在条目报错。config 全套 + 全仓 go test 15 包全绿。config_version 不需要 bump（extends 键已在 v10，只是值形态扩展）。

### T24 审计：img2text `<img src>` 处理与 finally 遗留（master，同批）

**需求**：img2text 的 `<img src>` 引用是否还有处理不到位的；`logs/finally/` 里是否有遗留。

**审计**：`logs/finally/` 扫出 6 个文件、43 个 `<img src="images/<书>/<hash>.jpg"/>` 未替换引用——全部出自 **2026-08-11** 的产物；HTML `<img>` 支持（markdown + HTML 双方言识别、organize 替换）是 **2026-08-29**（4955747）才加的。现行正则对遗留的全部形态（单/双引号、`/>` 自闭合、带其它属性）实测匹配；现行产物（latex_project 各书 out）扫描 **0 遗留**。结论：**现行代码无缺口**，旧文件重跑对应书即可消化。守护测试补双引号自闭合用例。

### T10：收集引用图片「卡很久」（master，同批）

**现象**：`[3/4] 收集引用的图片` 后长时间无输出。**量化现场**：`mineru_output` 191 个目录（167 个含 images），**约 7.5 万张图**；缺失图片触发 `buildImageSourceIndex` 时在 **samba 网络盘**上**串行** `os.ReadDir` 全部目录，期间零输出——不是死了，是看不见的慢。

**修法**：① 索引扫描改**并行**（12 路信号量限流，逐目录 goroutine，互斥锁合并局部 map）；② 扫描中每 2 秒打 `已扫描 N/M 个目录...`（仅真的画过进度才补换行，快扫完不残留空行）；③ 触发扫描前先打印「有 N 张图片缺失，正在扫描 M 个 mineru 目录建索引（并行）」——让等待有解释。T5（逐张核对慢）此前已在 df2ef2c 修过（全部在盘就跳过），本批处理的是「确实缺图时」的索引构建路径。

### T16：mermaid 嵌入统一 `[Image]` 锚 + 围栏闭合防御（master，同批）

**需求**：mermaid 类型的嵌入也应有 `[Image]( … )` 前缀（后面才是 ```mermaid 代码块）；"多多少少还有一点标签没有闭合的问题"。

**实现**（runner.go）：`embedBlockFor` → `embedBlockForRef(result, ref)`——嵌入点把**原始图片引用**（替换循环里 `entry.content[off.Start:off.End]`，右到左替换时切片仍指向原文）传进来；`altOfRef` 从 markdown `![alt](…)` 或 `<img alt="…">` 抽描述（缺失退 "mermaid"）；mermaid 分支输出 `[Image]( 描述 )` 锚行 + 代码块。`ensureClosedFence` 数围栏行（`^\s*```(?:lang)?\s*$`）奇数则补结尾 ```——吞尾围栏的模型输出不再污染后续 markdown。反引号正则里写 ``` 会终止 raw string literal——用解释串拼接（build 立刻抓到）。其余类型嵌入方式不变。

**测试**：TestEmbedBlockFor 更新 + mermaid-with-alt（markdown/html 两种引用）+ 未闭合围栏补齐用例；全仓 15 包绿。

### T17：view_image 回执去冗余（master，同批收官）

**需求**：① 「Redraw it at that size — do NOT scale it up to the page.」不必要；② 预算提醒里的 64 位哈希文件名太长，说"当前这张图"即可。

**实现**：① `ImageMeasure.String()` 只保留尺寸事实（mm/像素/dpi/页宽占比），redraw 指令抽成 `redrawSizeHint` 常量、只由**作图会话**（tikz.go 的 Measure 闭包）追加——style/convert/checker/终审这些只看图的会话不再被这句误导性指令打扰（它们不重画任何东西）。② `view_image` 预算标签 `view_image on <basename>` → `view_image on this image`（去重键仍是完整路径，每图至多提醒一次的语义不变；`view_pdf` 保留页码——那是有信息量的）。

### 预览页 v2 上位：`/` 即 v2，旧页退役（master）

**用户指令**：web 从 v2 挪到根路径，退役老版本 UI。

**路由**（serve.go/v2.go）：`/`、`/index.html`、`/v2`、`/v2/*` 全部走 v2 页（别名零成本保旧书签）；`/viewer.{css,js}` 从 `assets/dist` 伺服；老页 `servePage`/`serveAsset`/`renderPage` 删除；老资产三件套（viewer.html/viewer.css/viewer.js）`git rm`。embed 声明挪进 v2.go。

**静态导出移植**（page.go 重写为 `staticDataBlock` + v2.go `staticPage`）：同一 v2 壳，`<style>`/`<script>` 内嵌 dist 产物 + `#dsh-data`（`SetEscapeHTML(true)` 语义保留）。web 侧：`state.staticMode/mediaRoot`；`bootStatic()` 读 `#dsh-data` 直接装 60 会话数据（不轮询）；`selectSession` 静态分支从内存取行；`mediaURL` 静态分支 = `mediaRoot + rel`（`file://` 没有 `/media` 路由）；`App.vue` 挂载时静态模式不起轮询定时器（否则 file:// 每 2 秒空打 /api）；横幅替代旧徽标声明快照身份。

**测试大扫除**：11 个钉在老 viewer.js/html/css 源码上的"源码契约"测试整函数删除（单元开关/会话名/DSH 结构/主题资产/JSON 高亮等——v2 的等价行为由 vitest 24 例 + Chromium 真渲染测试把守）。存活测试的适配：`TestViewerUnitToggleInChromium` 第二阶段读取挪进 `setTimeout`（Vue 重渲异步，旧页同步重画的前提失效）；缩略图行邻接断言改 `</details>\s*<div[^>]*class="preview-strip[ "]`（scoped CSS 的 data-v 属性在 class 前）；间距 token 测试读 dist 样式、豁免 PartialTail 流式卡的 220px（P7 验收的独立高度）；live 页断言改 v2 壳标识（theme 预置脚本 + `#app`）；`TestViewerSessionName…` 的"轮询不重建"⑩ 块退役（静态 v2 无刷新机制，节点复用由 Vue keyed diff 保证）；搜索探针改逐词 setTimeout 异步链。

**真机验证**（CDP 9333 + 真实 latex_project 60 会话）：`/` 与 `/v2` 均 200、控制台零报错；点会话行 24 消息/35 工具卡/31 图，单位开关即时切换；静态导出（20MB 单文件）file:// 打开 60 会话、横幅「静态快照 · 生成于 …」、图片 **119/119 全部加载**（mediaRoot 相对路径 `../../../../…/latex_project/…` 解析正确）、零报错。go test 15 包 + vitest 24 例全绿。

**坑**：① 反引号正则里写 ``` 会终止 Go raw string（fenceLineRe 用解释串拼接，build 立即抓到）；② Chrome 封禁 8997 端口（ERR_UNSAFE_PORT），探针换 8998；③ bash 工具调用结束会回收 `(cmd &)` 起的进程——服务器要用受管后台任务；④ vue dist 属性顺序是 `data-v` 在 `class` 前，字面量 `class="…">` 匹配会漏。

### T25 补遗：样例配置补多重基座示例 + issue 描述改为列表形式（master）

**用户指出**：① config.example.yaml 没加多重基座的样例；② issue 里的写法（同一键写两次 `extends:`）与实现的列表形式不一致，要改说明。

**改动**：① config.example.yaml 的 extends 注释块（②）补列表形式说明 + models 段加 `vision-heavy: extends: ["gateway-a", "heavy"]` 实例（heavy 链 gateway-b，最终连接走 gateway-b、模型 deepseek-v4.1，gateway-a 只剩 tool_stream 生效——正好演示"后面的基座覆盖前面的"）；default.yaml 注释补一行。② docs/issue.md T25 保留用户原始写法并注明 YAML 重复键不可行、给出列表等价写法；docs/config.md 详述节加列表示例。③ `TestConfigTemplateKeyParity`：extends 的标量/列表是值形态差异不算键面差异（归一化统一记 extends），另加原文守卫"config.example.yaml 必须含 `extends: [`"——新能力在模板里必须有处可学。整份 example.yaml 加载实测零告警（无价格冲突/未知键）。

### P12：Mermaid 升级修复会话（master）

**计划原文**：img2text 普通模型处理复杂图时经历 3 次 mermaid 修复会话仍失败 → 升级：出错的 mermaid 存 submit.md、给虚拟工作区 + 提交功能（类似 latex 流程），工具 write/grep/看原图 + submit（跑 mermaid 语法检查，能过就行）；按设置的错误上限累计编译错误（提交也检查）后清上下文进备选模型（可配置，兜底），出错结果文件保留；会话检查次数上限新设置默认 6；日志整理、analyze 工具调用计入、统计触发后备的会话数。

**实现**：① `tools.mermaid` 新增 `session_rounds`（默认 6，≤0 关闭）/`session_errors`（默认 3）/`fallback_model`（models 条目名，不存在则退回原模型清上下文重来）——现有块加键不 bump `CurrentConfigVersion`（新块才需要）。② `img2text/mermaid_session.go`：工作区 `progress_items/mermaid_fix/<subject>_<图名>/`（每图独立、并发无共享、下轮续用），submit.md 收出错完整响应，工具 `write_file`（唯一可写文件）/`grep`（submit.md+compile_error.log，行号+计数）/`view_image`（原图 base64 回执）/`submit`（ValidateMermaid 全块检查）；错误累计到 `session_errors` 置 escalated，第一段会话结束且 escalated → 第二段全新会话（清上下文、`fallback_model` 的 ModelConfig、工作区文件保留）；submit/检查次数两段合计受 `session_rounds` 约束（submit 工具自守）；两段各留 `session-stage{0,1}.jsonl` 转录。③ 处理器接线：`CallAIWithTools` 新增 `fixSession MermaidFixFunc` 参数，repairBudget 用尽分支先试升级会话、成功返回 StatusOK（修好不跳过），失败/未配置维持旧的 sentinel+StatusRetry；`ProcessOneImage` 新增 `fixCfg *MermaidFixConfig`，每图按 key 拼工作区子目录。④ runner `resolveMermaidFixConfig` 一次性解析（Timeout 默认 30s、备选加载期 ResolveModel）。⑤ analyze/mermaidfix.go：扫文本日志两行标记，进度摘要后打印「mermaid 升级修复会话: 启动 N 次，其中 M 次触发备选模型」；会话文本日志统一 `[ToolCall] <名> · <摘要>` 行（PatternToolCall 照常统计）。⑥ 会话内提示词就地成文（单一使用点，不进 prompts 注册表）。

**测试**：`mermaid_session_test.go` 9 例——假 mmdc（内容含 BROKEN 即失败）下 submit 成功/无块计错/错误累计触发升级/轮次封顶/write+grep/resolve 归一化/关闭开关/401 端点失败路径（快失败，submit.md 现场保留）/runner 解析器（含备选解析成功与失败两态）；`processor_mermaid_repair_test.go` 加场景 9：budget 用尽后钩子收到 (出错响应, 校验错误) 且其结果原样返回 StatusOK。全仓 15 包 ok 0 FAIL；config.example.yaml 加载零告警（dv5 重建后复验）。

### T26：配置文件有问题启动即停（master）

**现场**：config.yaml 的 `models.drawing.extends: nothinking` 引用了不存在的条目，`docvision sessions --serve` 打一行"读取配置失败，本次不显示金额、--dir 退回当前目录"后继续用默认目录/端口服务——坏配置被静默吞掉。

**排查**：全部 `loadConfigWithFlag` 调用点里只有 sessions 软降级（workflow/split/mineru/latex/verify 等本来就 `return err` 硬退）；且"没有配置文件"的场景在 PersistentPreRunE 已被 ResolveConfigPath 自动创建默认配置兜住，软降级实际只在"配置存在但损坏"时触发——这恰恰是最不该继续跑的情况（错误目录/端口/图片折算规则悄悄给误导性结果）。

**修复**：sessions RunE 配置加载失败改为 `return fmt.Errorf("读取配置失败（按 T26 启动即停…）: %w", err)`；清理降级痕迹——`sessionsConfigNote` 收敛单参（"（读取失败）"分支退役）、帮助文本与 docs/commands.md 去掉"无配置则当前目录"。**测试**：`TestSessionsBrokenConfigAborts`（现场同款 extends 错误 → cmd.RunE 必须返回错误）；`TestSessionsListInvalidUnitFailsTheCommand` 随新语义补最小可加载配置 + 裸命令的 --config 旗标桩（否则 T26 先于 --unit 校验触发）。全仓 15 包 ok。

### 修复：预览页轮询定时器双份注册（用户报告"运行启动时侧栏整列空白"）

**现场**：档位1 vector 运行刚启动时，预览页左侧栏整列纯白（连「工作区」头部都没有），刷新恢复、之后不再复现；用户控制台仅有 Adobe Acrobat 扩展 content script 的 `getUserMedia` TypeError 与 Slow-network intervention——页面自身代码零报错。

**排查**（未能在复现环境重现：用户自己的 8849 服务器 + 同一棵树 + 同一份二进制 dist，headless Chrome 下 120s soak/选中会话/改视口/localStorage 边缘键面全部正常，侧栏 840 节点稳定）：代码走查发现 **`bootData()`（data.ts，df4b862 起）与 App.vue onMounted（6e73833 补静态守卫时加的 `if (!state.staticMode)` 分支）各自注册了一份 2s 轮询 `setInterval`**——live 页实际每秒 4 次 `/api/index` 全树扫描。运行刚启动时 samba 冷缓存、扫描慢（用户控制台的 Slow-network intervention 佐证），请求堆积、首份数据迟迟不到，叠加扩展注入干扰，构成最贴合"最开始启动时有、之后没有、刷新恢复"的解释。

**修复**：① 轮询只在 App.vue 注册一份（pollTimer 可清理、页面隐藏跳过），data.ts 的 bootData 只保留首拉与首选中；② Vue 挂全局 `app.config.errorHandler`——今后组件渲染异常会在控制台打 `[app] Vue 错误（info）组件链: …`，消灭"静默白屏无法归因"。验收：web 构建后 dv5 重编译，8998 真机 60s+ 侧栏稳定、控制台零报错；vitest 24/24；sessionview 14.5s 全绿。

### 三项跟进：view_image 冗余句删除 + /v2 别名删除 + 运行中"失败"闪烁修复（用户三问）

**问 1（view_image 额外内容）**：T17 把「Redraw it at that size — do NOT scale it up to the page.」收进作图会话后，用户问是否冗余。核查：`latex_figure.system.md` 硬要求（保持原印刷尺寸/比例、不得放大到整页）+ 首轮 user 提示的 `ORIGINAL_SIZE` 测量行已完整覆盖该指令——**回执每次看图复读一遍确实冗余**（一次作图会话最多 30 次看图 ≈ 450 tokens 噪声），且违背"提示词只减不增"。**删除**该句与 `redrawSizeHint` 常量，回执只保留 ORIGINAL FIGURE SIZE 测量事实（mm/px/dpi）。

**问 2（/v2 没删干净）**：确认 6e73833 留了 `/v2` 旧书签别名。按用户要求彻底删除（serve.go/v2.go 路由收窄到 `/` 与 `/index.html`），实测 `/v2`、`/v2/` → 404，`/`、`/index.html` → 200。AGENTS.md 与 commands.md 同步。

**问 3（会话"运行中/失败"来回跳）**：两层根因——① `LiveWindow = 60s`（mtime 距今 60s 内才算 live），模型一轮思考+限流退避很容易超 60s 不写转录，绿点掉成暗；② 更要命的是 `SawTool && !SawSubmit → endState=error`（"干过活但没交"），**运行中的会话天然就是这个形态**——live 绿脉冲一盖看不出，窗口一过红色就露出来、下一笔写入又变绿，正是用户看到的"显示红色好像失败了过一会就好"。修复：LiveWindow 60s→3min（覆盖慢轮次）；live（含窗口内余晖）期间 error 终态一律压掉（`liveEndState`），done 是真提交保留。sessionview/latex 全绿。

**注**：途中 `gofmt -w .` 把无关文件的 doc-comment 规整出格式噪声（Go 新版行为），已还原——只提交本批真实改动。

### P10 轮次/会话哈希 + T28/T29 续跑收口（用户点单）

**P10（plan.md）——每轮内部哈希 + 每会话哈希**：
- `session/transcript.go`：`transcriptLine` 增 `h`（12 hex SHA-256 over role/正文/call id/调用参数/思维链——内容派生，重放同哈希），`Append` 落盘时计算；`roundHash` 单测钉稳定性与落盘。
- 预览管道：sessionview `Line` 透传 `h`；`transcriptStat` 在同一单遍扫描里算整文件 SHA（12 hex，按 size+mtime 缓存），`SessionInfo.SHA` 下发。
- UI：工具卡尾部 `· h=xxxx`（stream.ts callTail）、轨迹行展开区「轮次哈希」（trajectory.ts roundDetail，5 类行全覆盖）、详情栏会话块「会话哈希」行。旧转录无字段照常显示。

**T28——续跑"几乎重新开一场"**：用户在 [style] 续跑时看到 `轮次 0 · 工具调用 0 · 已用 11.0s`，且实际已 submit 的会话又重新跑了。修复：
- 计数续接：`LoadTranscriptStats`（新）在回放时统计压缩点之后的助手轮数/工具回执数/首尾时间跨度/最后一次已收账 submit 调用（名字+参数），`session.SeedCounters` 回填；`livePhaseRowAt` 让"已用"从「现在 − 历史活跃跨度」起步（进程死亡间隔不计）。
- 已提交重放：`latex/resume.go` `replaySubmit` 用转录里记录的参数重放 submit 工具（submit 工具是对持久工作区的纯状态写入，重放即精确复现提交），style/convert/chapters/tikz 四个续跑点接线——重放成功则跳过 Run 直接走提交后处理（style 进 example 编译段、convert 走 checker 段、chapters 走校验写盘、tikz 直接完成）。

**T29——续跑插内容**：此前 tikz/convert 续跑都把"continuation 提示"当 user 轮插入、tikz 还重附原图。修复：`session.Run` 空参（UserText 空 + 无图）= 纯续行，不追加任何消息；`resumeUserText` 判定——历史以 tool 回执收尾（含合成的悬空占位回执）就不插内容，只有模型自己停了（最后是普通 assistant 文本）才发最小推动。悬空调用（进程死在回执落盘前）由 `LoadTranscriptStats` 合成 `INTERRUPTED` 占位回执，保证重放历史 wire 合法（否则续跑第一次请求 400）。回放从最近压缩检查点开始是 `LoadTranscript` 既有语义（T29 之问的答案），已写入 docs/commands.md。

**验证**：Go 新增 3 测试（roundHash 稳定+落盘、ResumeStats 统计/悬空合成/提交识别、多提交取最后），`go test ./...` 全绿；web 构建 + vitest 24/24；真机（合成转录 + latex_project 实树）：详情栏「会话哈希」出列、/v2 404、控制台零报错（favicon 404 为浏览器默认请求）；轮次哈希 UI 依赖新写入的 h 行，下一次真实运行即显示。

## 第三十九批（2026-09-16）：T27 侧栏空白双根因 + T32 XML 协议残片兼容 + T33 assemble 交付中止

用户跑完《2026年李艳芳预测三套卷数一》（36 章，style/images/chapters/convert 全绿）后报三个问题，本批逐个排查修复。

**T27——侧栏空白，两个独立根因**（用户现象："运行中左侧栏目直接空白"）：
1. `Sidebar.vue` 的 `groups` computed 里调用 `isOverflowOpen(okey)`，但 import 列表只导了 `setOverflowOpen` 漏了 `isOverflowOpen`——vite 不类型检查，构建期与控制台的静默性同"组件未 import"陷阱；某阶段会话数 >8（overflow 分支）即 `ReferenceError`，整个侧栏 computed 崩掉 → 空白。这正是"style 处理完开始下一阶段、会话变多时空白"的现场（convert 36 个会话）。
2. `sessionview.UsageStats.CacheHitPct` 带 `omitempty`，零缓存命中的会话（cachedTokens=0 → pct=0）被从 `/api/index` 省略 → 前端 `statsSummary` 的 `st.cacheHitPct.toFixed(0)` 崩渲染。本次运行 **28/138 个会话**命中（checker 会话全是零缓存）。历史项目全靠命中率 >0 才没炸——潜伏数据形状雷。
修复：① Sidebar.vue 补导入 `isOverflowOpen`；② `CacheHitPct` 语义本就注释为"0 when unknown"，去掉 `omitempty` 恒下发。教训：前端对 API 字段直接 `.toFixed()` 的地方都要警惕 omitempty；侧边栏是单 computed 无错误边界，一个字段崩=整栏空白（errorHandler 只负责归因记录）。

**T32——XML 工具调用残片**：日志实测样本（`logs/latex_20260915_230610.log` 23:48:58，checker_chapter_023 round 1，Qwen3.5-27B，`tool_stream=true`）：模型随 tool_calls 把原始 XML 残片写进 content（`<parameter=path>\ncheck:parts/\n</parameter>\n</function>\n</tool_call>`，恰好 70 字符），真实调用已按 tool_calls 通道正常解析执行（3 个工具全部成功）——残片只是重复，但会进回放历史（污染前缀）与转录（污染 UI）。全库 jsonl 只有这一处。修复（双端）：Go 侧 `session.StripLeakedToolXML`——消息带 tool_calls 时才动手（纯文本回复里讨论协议格式的不碰），删完整 `<tool_call>…</tool_call>` 块 + 成行的协议标签 + 被标签行上下夹住的"标签体内的值"行（`check:parts/`）；在 session.Run 两条工具路径（useTools / 预算耗尽兜底）`NormalizeToolCallTypes` 之后调用，落盘与进历史同时生效。web 侧 `splitProtocolLeak`（legacy/stream.ts，规则与 Go 同构）兜旧转录：残片从正文剥出，AssistantMsg 渲染成默认收起的「XML 协议残片」折叠块（等宽小字，说明"调用已正常解析执行"）。

**T33——`phase assemble: open …/out/REPORT.md: no such file or directory`**：convert 36/36 完成后整本书在最后一步中止。根因：`deliverBook` 先 `RemoveAll(outDir)` 再 `filepath.Walk` 拷贝，但**只在遇到目录项时** `MkdirAll`——顶层文件按字典序排在目录前（`REPORT.md` 大写 R=0x52 < `chapters` 小写 c=0x63），Walk 的第一个顶层项是文件时，`copyFile` 写进一个已被删掉的 `out/` 直接 ENOENT。style 包自带 REPORT.md（每次都有）→ **这是每次都炸的潜伏 bug**：核查历史项目，`测试-概率论_ds4.1` 等全部 out/ 是空的（assemble 从未成功交付过，用户没注意）。修复：`RemoveAll` 后立刻 `MkdirAll(outDir)`，补单测 `TestDeliverBookRecreatesOutDir`（顶层文件 + 目录混合布局复刻现场）。已跑完的书重跑 `docvision latex`（done 阶段自动跳过）即完成交付。

**验证**：Go `go test ./...` 全绿（新增 TestStripLeakedToolXML 4 例、TestDeliverBookRecreatesOutDir）；web vitest 27/27（新增 splitProtocolLeak 3 例）；真机 dv5 挂 latex_project 实树 CDP 探针：修复前复现崩溃（`statsSummary … 'toFixed'` ×5，侧栏 0 组）→ 修复后 7 项目组/23 阶段组渲染、「更多会话（还有 28 个）」overflow 展开正常（isOverflowOpen 路径实走）、checker_chapter_023 会话内「XML 协议残片」折叠块出现且默认收起、正文无污染、控制台零报错。

**文档**：issue.md T27/T32/T33 标 [X]（含根因注记）；CHANGELOG [Unreleased] Added（T32）+ Fixed（T27/T33）；AGENTS.md 命令面/会话基础设施要点同步；docs/commands.md 新增「XML 协议残片（T32）」小节。README 与 --help 无涉未动。

## 第四十批（2026-09-16）：T30 思考泄漏折叠 + T31 缓存命中排查 + T34 XML 残片统一渲染

**T30——`<thinking>` 漏进正文**：用户样本为 style 阶段现场（reasoning 通道开启时也有漏）。修法双端：Go 侧 `session.MoveLeakedThinking`（client.go）——assistant 消息的 `<thinking>…</thinking>`/`<think>…</think>` 整块移到 `ReasoningContent`（UI 渲染成可折叠思考块，GLM 保留式思考回传也走该通道），在 session.Run 主循环与 nudge 响应两处接入，先于 debug 日志（日志与落盘一致）；RE2 无反向引用，两种拼写写成两个分支。web 侧旧转录由 `splitLeaks` 剥出折成「原始思考」块（默认收起）。

**T31——缓存命中"偶尔掉一下/没有命中"**：定量排查（《预测三套卷数一》138 会话 952 次请求）——convert（deepseek-v4.1）：723 命中 / 37 未命中（首轮冷启动 + 少量抖动）；checker（Qwen/Qwen3.5-27B）：**9 命中 / 161 未命中**。三个排除法：① 同会话各轮 head_sha 逐字节一致、36 个 checker 会话共享同一 head（cache-probe 指纹）——我们的前缀完全稳定；② 命中/未命中的平均并发请求数 98.2 vs 100.3——不是并发挤占；③ 同结构请求换模型即高命中——**Provider 差异**（Qwen3.5-27B 部署的前缀缓存近乎不工作，偶发命中一两轮符合缓存容量/LRU 特征）。结论：非会话侧问题，治本只能换缓存友好的模型/服务商。顺手把 `[cache-probe]` 日志加了 head 前 256B 转义预览（`%.256q`），Provider 若声称"你们前缀变了"可 byte 级对质。

**T34——其余 XML 残片"作为 XML 渲染"**：前端 `splitProtocolLeak` 升级为 `splitLeaks`（stream.ts，纯函数）：返回 `{main, blocks:[{kind,text}]}`，kind = thinking | xml。完整 `<tool_call>…</tool_call>` 块不再静默删、保留为可展开块；成行协议标签集扩展 thinking/think 开合标签；**夹值行规则收紧**——只认"上面最近的是开标签（非 `</`）"，修掉了原实现会把残片后面的正文也吃掉的边界缺陷（Go 的 `StripLeakedToolXML` 同步收紧 + 双侧补防回归用例）。AssistantMsg 按 kind 渲染「原始思考」/「XML 协议残片」Disclosure。`splitProtocolLeak` 保留为兼容包装。

**验证**：Go `go test ./...` 全绿（TestMoveLeakedThinking 6 例、TestStripLeakedToolXML 加防回归例）；vitest 31/31（新增 splitLeaks 4 例）；真机 CDP 探针（合成转录含 thinking 泄漏 + XML 残片）：「原始思考」「XML 协议残片」两块并存、默认收起、正文干净、控制台零报错。

**文档**：issue.md T30/T31/T34 标 [X]（T31 记完整排查结论与证据）；CHANGELOG [Unreleased] Added 三条；AGENTS.md 会话基础设施要点改为「正文残片清洗（T30/T32/T34）」；docs/commands.md 的 T32 小节扩写为「正文残片折叠（T30/T32/T34）」。

## 第四十一批（2026-09-16）

用户要求：① 保留式思考折叠块要与正常思考区分开标注；② 做 T35、P14、P15。

**T30 视觉区分——「原始思考」徽标**：Disclosure 组件新增可选 `badge` prop（名称右侧琥珀小胶囊，样式 `--warn` 主题色）。AssistantMsg 的残片块带标注：「原始思考 · 漏进正文」「XML 协议残片 · 协议残片」，与正常「思考」块一眼区分。

**T35——空回复推动请求**：两个子问题分别处理：
1. **提示太长** → `session.Run` 空回复推动文案从 "Provide your final answer now." 精简为 "**Continue.**"（messages 与转录同步）。
2. **「提示之后会话重开、输入变少」→ 查明是显示假象，不是真重开**。证据：对运行目录全部 139 个转录做 round 序列扫描，无一重开；5 个含 nudge 的用量行全在 checker（如 checker_chapter_020：#1 普通 1354 → #1 nudge 563）。nudge 请求 `tool_choice=none` 时厂商（Qwen）不把工具表计入 `prompt_tokens`（差值≈checker 三工具 schema），**历史上下文完整带上**，模型后续正常。处理：预览页请求明细 nudge 行显示「#N·续」、kind 说明改「空回复后的推动请求（同轮续发，未重开会话）」，悬浮说明显式解释 token 变小的缘由。

**P14（本轮范围）——灯箱触屏化 + 折叠轨道美化**：灯箱（App.vue pointer 处理重写为多指跟踪）支持**双指捏合缩放**（双指中点为锚、按初始距离比经 `setLightboxScale` 绝对缩放）、**触屏双击 1×/2.2× 切换**（鼠标双击复位语义不变）、右下角常驻 缩小/百分比/放大/1:1 控件（`@click.stop` 不触发点空白关闭），灯箱 `touch-action:none` 禁浏览器默认手势；折叠侧栏轨道行 28px 纵向呼吸、hover 圆角高亮、当前会话 active 底保留。

**P15——checker 提示词收敛**：`latex_checker.system.md` 末尾加一句——可见回复不是交付物，不要写大段的核对比较分析，读、决定、submit 即可（用户明确要求的提示词修改，保持一行最小增量）。

**验证**：vitest 31/31；`go test ./...`（session/prompts/latex/sessionview/img2text 全 ok）；真机 CDP 探针（合成转录，含 thinking 泄漏 + nudge 用量行）：「原始思考」块带「漏进正文」徽标渲染、正文干净、请求明细出现 `#1·续` 行、控制台零报错。

**文档**：issue.md T35 标 [X]（记完整排查证据）；plan.md P14/P15 标 [X]（P14 注明本轮范围）；CHANGELOG [Unreleased] Added 四条；AGENTS.md 残片要点补徽标、灯箱要点补触屏；docs/commands.md 残片折叠节补徽标说明、灯箱手势、请求明细 nudge「#N·续」说明。

## 第四十二批（2026-09-16）

用户要求：① 上一批 commit 格式与之前不一致，按既有格式（`type(scope): 标题` + 分块正文）重新提交；② 完成 P4（增加 anthropic api 接口）。

**commit 格式修正**：第四十一批（16 文件）`git reset --soft HEAD~1` 后以 `feat(session/web): …` 格式重新提交（cdc2887），与 f4c33e6/78f2d1b 等一致。

**P4——Anthropic Messages API 适配**（`go/internal/session/anthropic.go`，新增）：
- `models.*.<条目>.api: anthropic`（新配置字段，继承规则同 base_url——条目不写继承 models.text）→ 客户端改走 `POST {base_url}/messages`（base_url 指到 /v1），自动带 `x-api-key` + `anthropic-version: 2023-06-01`，不发 Authorization 头。
- 请求翻译：system 消息提升为顶层 `system`（Anthropic 不允许 system 出现在 messages）；assistant 轮的 ReasoningContent/Content/ToolCalls → thinking/text/tool_use 内容块（tool_use 的 input 由 arguments JSON 解码，非法 JSON 原样上交）；tool 回执 → user 轮 tool_result 块；user 多模态 parts → text + image 块（data: URL 拆 base64 source，http URL 走 url source）；OpenAI `user` 路由字段 → `metadata.user_id`；tool_choice "none" → `{type:"none"}`；thinking 配置与 request_body 照旧顶层合并。
- 应答翻译：content blocks → Content/ReasoningContent/ToolCalls；`cache_read_input_tokens` → PromptTokensDetails.CachedTokens、`cache_creation_input_tokens` 计入输入（会话层缓存统计与 [cache-probe] 照常工作）；stop_reason → finish_reason 词汇（tool_use/max_tokens/end_turn → tool_calls/length/stop）。
- **只走非流式**：ChatCompletion 对 anthropic 强制 stream=false，一次 JSON 应答（P7 实时快照这类流式专属能力不适用）；重试/4xx 不重试分类与 OpenAI 线路共用（错误统一为 "HTTP %d: body" 形式）。
- img2text 逐图分析的独立 AIClient 暂仍为 OpenAI 兼容线路（复用 session.Client 是更大重构，留待后续）。

**验证**：新增 `anthropic_test.go` 4 例——端到端（httptest 网关断言路径/双头/无 Authorization/system 提升/工具 input_schema/tool_use/tool_result/图片 base64 source/应答全通道/缓存映射/finish 映射）、tool_choice none、4xx 不重试（401 一次过）、ResolveModel 继承 api；`go test ./...` 全绿；模板键面一致性测试通过（两模板同步加 `api: openai` 行）。

**文档**：config.md 增 `models.text.api` 行；default.yaml/config.example.yaml 同步加 `api:` 行；sessions.md 模型注册表条目补 api；README「AI 模型」一行补 anthropic；plan.md P4 标 [X]；CHANGELOG [Unreleased] Added 一条。

## 第四十三批（2026-09-16）

用户指出 P4 原文是「response/anthropic 两个接口」，上一批只做了 anthropic——补 OpenAI **Responses API**（`/v1/responses`，gpt-5/o 系新协议）。

**实现**（`go/internal/session/responses.go`，新增）：
- `models.*.<条目>.api: responses` → POST {base_url}/responses（Bearer 认证，与 OpenAI 线路同头）。只走非流式；固定 `store:false` + 全量 input 回传（无状态回放，不用服务端存储/previous_response_id 链）。
- 请求翻译：system → 顶层 `instructions`；assistant 轮 → output_text message 项 + reasoning 项（ReasoningContent 原样放回 `encrypted_content`，decode 存回的加密链因此可回放）+ function_call 项（arguments 保持 JSON **字符串**，Responses API 线格式如此）；tool 回执 → function_call_output 项；user 多模态 → input_text + input_image（data: URL 直接收）；工具 → {type:function,name,description,parameters}；tool_choice 直接收 "auto"/"none" 字符串；`max_tokens` → `max_output_tokens`；固定 `include:["reasoning.encrypted_content"]`（加密思考约半小时级有效期，长间隔续跑可能失效——协议固有约束，文档写明）；reasoning_effort/thinking 配置透传为 `reasoning` 顶层字段。
- 应答翻译：output items → Content/ReasoningContent(encrypted 优先，退回 summary 文本)/ToolCalls；`input_tokens_details.cached_tokens` → PromptTokensDetails.CachedTokens（缓存命中统计照常）；status=failed/error → 错误；finish 映射（有 function_call → tool_calls、incomplete_reason=max_output_tokens → length、其余 stop）。
- 重试/4xx 不重试分类与另两条线路共用（错误统一 "HTTP %d: body" 形式）。

**验证**：新增 `responses_test.go` 4 例——端到端（路径/Bearer/instructions/store:false/include/工具形状/function_call 与 function_call_output 往返/图片 input_image/应答全通道含 encrypted 思考回放/缓存映射/finish 映射）、incomplete→length、failed status 报错、reasoning_effort 透传；`go test ./...` 全绿；模板键面一致性通过（两模板 api 注释同步更新）。

**文档**：config.md 的 models.text.api 行扩为三协议（写明加密思考有效期限制）；default.yaml/config.example.yaml 注释更新；sessions.md 模型注册表行补 responses；README「AI 模型」一行补 Responses API；CHANGELOG 上一条 P4 条目更名并扩写；plan.md P4 行补 responses（与 P16 等用户编辑一同留在工作区）。

## 第四十四批（2026-09-16）

### 起因
- issue.md T36：【错误与警告分类统计】把 `mermaid_invalid`/`invalid_format`（及 `empty_response`）打成「警告（下轮重试）」，与 img2text 进度行（T15 起 errors 计数）、`Skipped invalid` 的 error 级日志自相矛盾；要求这些"下轮重试"类算**错误**，警告只留给"程序修正后结果仍可用"的偏离（用户举例：输出带前缀文字、程序剥掉前缀留下标准 `[IMG_TYPE:` 输出——对应 processor 已有的 `Unexpected prefix before '[IMG_TYPE:'` 警告日志）。
- issue.md T37：mermaid 失败升级会话前不再做 3 轮就地修复，**首次输出失败即升级**；升级修复会话（及 debug 下全部逐图会话）要在会话预览 UI 按书分组、可搜索、带图片编号；失败/警告/错误的会话默认保留；全部逐图会话仅 debug 模式开启。

### 先看最新日志再动手（用户要求）
- `logs/img2text_20260916_130807.log`（13989 行）：`Skipped invalid` 7 条全部 error 级（T15 已就位）；`[WARNING]` 73 条主要是 `[ConnRetry]` 36、就地修复轮 `(1/3)(2/3)(3/3)` 23、`[ToolCall] invalid arguments` 1、前缀剥离 0（用户怀疑的纠正类本次未出现）；4 个 `mermaid_fix` 升级工作区（日志含「就地修复轮用尽，启动升级修复会话」→「升级会话未能修复（跳过、下轮重试）；工作区保留」）。结论：T36 只剩 analyze 包口径没翻；T37 的"3 轮"证据就是 `(N/3)` 日志行。

### T36：analyze 错误/警告口径对齐
- `analyze/session.go`：删 `retryableErrorTypes`/`isRetryableError`；`retriesNextRun()`（mermaid_invalid/invalid_format/empty_response）只喂报告的 `失败（跳过、下轮重试）` 标注。`StatusWarning` 语义改写：警告 = ✓ DONE 且会话期间出现过 `[WARNING]` 行（程序自纠正、结果可用）。
- `analyze/parser.go`：`startEvent` 加 `warning` 标记，扫描循环识别 `[WARNING]` 行挂到当前 START；去掉"校验/格式失败降级为 warning"分支（失败一律 `StatusFailed`）；成功关闭带 warning → `StatusWarning`。失败关闭即使带 warning 也仍是失败。
- `analyze/stats.go`：`StatusWarning` 不再进 failures/错误分布（它没有 ErrorType，自纠正成功项）。
- `analyze/report.go`、`roundfiles.go`：警告行改写「输出有偏离但已被程序纠正，结果可用」；分类统计三类标 `失败（跳过、下轮重试）`，其余 `失败`。
- 测试：`parser_test.go`（imgA/imgB 从警告改失败；三分统计重写）、`report_warning_test.go` 重写（构造器加 retryFail 桶：`[WARNING]+✓ DONE` = 自纠正警告；新增/改 4 个用例）。
- **真实日志复核**：`dv5 analyze --logfile img2text_20260916_130807.log` → `empty_response: 24 [失败（跳过、下轮重试）] / mermaid_invalid: 4 [失败（跳过、下轮重试）] / invalid_format: 3 [失败（跳过、下轮重试）] / unknown: 1 [失败]`——与用户贴的期望口径完全一致。

### T37-A：首次校验失败即升级
- `img2text/processor.go` `resolveMermaidRepairBudget`：nil/负值 → **0 轮**（首次失败直接 fixSession 升级，T37 默认）；>0 → N 轮就地修复；0 → 无限（安全上限 100 不变）。删 `mermaidDefaultFixBudget=3`。0 轮时日志改「Mermaid validation failed on first output …」+「[mermaid-fix] 首次校验失败，直接启动升级修复会话」。
- `config.go`：`MermaidFixAttempts` 缺省不再填 3（保持 nil = 新默认）；模板 `fix_attempts: 3` → `-1` 并注释三档语义；docs/config.md 表格与 P12 段落同步。
- 测试：预算 resolver 4 例重写；`TestCallAIWithTools_RepairSucceedsSameSession` 显式 `intPtr(3)`（钉旧路径）；新增 `TestCallAIWithTools_FirstFailureSkipsInPlace`（默认 nil → 只发 1 次请求、status retry、哨兵 mermaid）。
- 注意：`format_fix_attempts`（缺 `[IMG_TYPE:` 前缀的格式修复）不在本次范围，保持 1 次。

### T37-B：img2text 会话进预览（配置 v11）
- `config.go`：`PreviewConfig` 新增 `img2text`/`img2text_all`（新子块键）→ **CurrentConfigVersion 10→11**（回答了用户"版本号是否该升级"：api: 字段级新增不用升，本次 preview 子块必须升）；两模板 config_version: 11 + preview 两个新键；README `config_version: 11` 行；docs/config.md 三行。
- `sessionview/scan_extra.go`（新）：`ScanExtra{Dir,Kind}`；`scanner.extras`；img2text 布局识别——`mermaid_fix/<书>_<图>/session*.jsonl` → 项目 `img2text · <书>`、阶段 `mermaid-fix`（stages.go StageTitles 补「升级修复」+ StageOrder），`sessions/<md>/<图>.jsonl` → 阶段 `img2text`（「逐图分析」）；**组内按图名稳定排序编号**（`#N ·` 标题前缀 + ChapterOrder 喂方块视图；哈希短名 8 位、撞车升 12 位，与矢量图同词汇）。ID 加 `img2text:` 前缀防撞车。
- **两个真实 bug**：① `img2TextEntry.info` 曾指向 append 中途的切片元素，后续 append 扩容 → 指针悬空，编号 pass 改了副本（书B 生效书A 失效）——改按 ID 回指 out 下标；② `newViewerServer` 里 `copy(ex, ex)` 笔误 → scanner 拿到空 extras（本地 ScanWith 单测全过、HTTP 探针 0 条，e2e 才暴露）——`copy(ex, extras)`。
- `serve.go`：`StartWith(root,addr,extras)`/`ServeOptions.Extras`；`serveFile` 逐根尝试主根+附加根——**`resolveUnderRoot` 只查越界不查存在**，主根对任何相对路径都"命中"会 404 掉附加根的 media：改为逐根 open、都不存在才 404、全部越界才 400。media 服务单测钉住。
- 接线：`cli_sessions`（静态导出与 --serve 共用 `sessionsExtras(cfg)`，`preview.img2text` 时附加 `<finally>/progress_items`）+ `cli_latex startPreview`。
- **真实数据探针**（临时端口 18925 + 探针配置）：`img2text · 27政治苏一命题点高清图谱` 组下 4 个升级修复会话，`#1..#4 · 升级修复 · 0fc1091d.jpg` 式标题，`/api/session?id=img2text:...` 200。chdir 到配置目录的既有行为意味着探针配置必须用绝对路径（用户真实配置在运行目录、不受影响）。

### T37-C：debug 全量逐图转录（preview.img2text_all）
- `img2text/recorder.go`（新）：`ExchangeRecorder` 复用 session 引擎 `TranscriptWriter`（同一 JSONL/媒体格式，预览直接渲染）；`Record(req,resp)` 利用 img2text 对话**单调增长**（每轮重放全部历史）的性质，用"已见消息游标"差量持久化 + 回写 assistant 应答与 usage 行——不碰 processor 内部。
- `img2text/client.go`：`AIClient` 加 `record func(req,resp)` + `SetRecorder`；`ChatCompletion` 包装内层实现，成功且带 choices 时回调。
- `img2text/runner.go`：`runWorkers` 加 `recordAll`；逐任务**结构体拷贝** client（http 客户端/配置只读共享）挂 recorder，转录落 `<finally>/progress_items/sessions/<md>/<图>.jsonl`；创建失败只警告不中断分析。
- 测试：`recorder_test.go`（两轮差量：meta+9 行、user 恰 2 次、usage 2 条）。

### 真实缺陷记录
- `copy(ex, ex)` 笔误：纯函数层（ScanWith）测试覆盖不到 HTTP 装配层，必须以真实服务探针兜底。
- 指针悬空的 append 扩容陷阱：同一教训第二次（第一次是 runconfig 时代）——**别保存指向正在 append 的切片的元素指针**。
- 探针配置踩 chdir：PersistentPreRunE chdir 到配置文件所在目录，相对 paths.* 全失效——探针配置一律绝对路径。
- 静态导出路径尚未验证附加根 media 重写（live 服务是主目标；如静态导出要用 img2text 会话再补）。

### 验证
- `go test ./...` 全绿（analyze/config/img2text/sessionview 及全部包）；`TestConfigTemplateKeyParity` 通过（两模板同步 v11 + preview 键 + fix_attempts: -1）。
- analyze 真实日志复核数字见上；预览探针见上。
- 注：运行目录二进制部署仍被 8849 服务占用阻塞（ETXTBSY），等用户停服务后 `cp temp/p5run/dv5`。

## 第四十五批（2026-09-16，晚）

### 起因
- 用户报「T28 依然没修复」+ T39（issue.md:541）：续跑时会话从上一次最后轮次开始算、章节核对里有红色错误结束/没开始的会话却进了下一阶段、**final-review 没有进入会话（WebUI）**、续跑时已完成的部分不显示。

### 先看日志再动手（用户要求）
- 最新 `latex_20260916_184221.log`（1679 行）：18:42 启动 → images Already done → compile:book OK → **直接进 final-review**（round 1、messages=2 全新会话）→ 63 轮提交 → assemble 完成。全程无 style/chapters/convert/checker 行。
- 对照 `latex_20260915_230610.log`：昨晚 23:06 的运行早已跑到 final-review（round 1 全新、72 轮、00:04:52「终审完成，全书页数: 15」）——**日志在终审完成后立刻终止**：没有 `[assemble] 全书编译完成`、没有成本表、`progress.json` 没落 assemble:done（进程死在收尾路上）。昨晚终审改的 4 个章节 + frontmatter.tex + main.tex 钩子全在 build/ 树里，而 `assemblePhase` 每次运行都清空重建 build/——**终审产物从未回写 work/**。
- 证据：`find -name '*.jsonl' -newermt 今天` 在项目下零命中（终审 63 轮没留任何转录）；`work/sessions/` 无 final-review 文件；成本表无 final-review 行（成本表从转录统计）；今晚 `out/main.tex` 比生成模板多 `\input{frontmatter.tex}`，`work/chapters/chapter_001/019/027/033.tex` 与 out 版 diff 非空（终审修订全丢在 build 树里）。
- 结论：「轮次 19」那行是昨晚终端的进度行（昨晚终审在 ~1m04s 时正过 round 19），用户的 T28 痛感实为终审每次续跑都**从零重开**（显示与历史断裂）。

### 修复
1. **finalReview/fixSession 挂转录**（`work/sessions/final_review.jsonl`、`book_fix.jsonl`）：WebUI 可见、进成本表；final-review 用 `livePhaseRowAt` 带起始时间。旧转录经 `archivePrevTranscript` 归档成 `*_prev.jsonl` 保留可查——**不回放历史**：build/ 每次重建，回放对已不存在的编辑是误导（修复会话同理，只归档不重放）。
2. **终审产物回写 + 指纹短路**（新 `finalreview_state.go`）：终审编译通过后 `persistFinalReviewState` 把构建树里的章节 .tex（递归）回写 `work/chapters/`、顶层 .tex（main.tex + frontmatter 等）存 `work/final_review/`，并对「work/chapters + work/final_review」算 SHA-256 指纹写入 `state.sha256`。续跑时 `finalReviewStateCurrent` 重算指纹：一致 → `applyFinalReviewState` 用终审版 main.tex/顶层文件覆盖生成的骨架、**整段跳过终审会话**；不一致（convert 重跑/手改）→ 照常重审。修复会话**不做**回写/短路（它的编辑面太大，且修复结果本来就以"编译通过"即时判定）。
3. **续跑摘要**（book.go）：非 Restart/非 Step 时开头打印「[book] 续跑：已完成 X / Y，只跑剩余阶段」（控制台 + 日志文件各一份，note 原本只上控制台）；每个被跳过的 phase 补一行日志（此前压缩模式下日志里完全看不到跳过）。
4. **阶段词表**：`fix`→「全书修复」、`final-review`→「终审」，进 StageOrder（成本表/侧栏分组）。

### 数据回填
- 用同一 `persistFinalReviewState` 对李艳芳项目以 `out/` 为构建树回填（一次性测试，已删）：`work/final_review/{main,frontmatter,example,standalone}.tex + state.sha256` 就位，`work/chapters/chapter_019.tex` 等 4 个修订已同步——新二进制下次续跑该项目**直接跳过终审**。

### 遗留（如实记录）
- checker 会话失败/未提交**降级为通过**是既有设计（transport 故障不该卡书；编译与终审是硬闸门），`checkChapter` 注释写明了。用户 T39 观察到「红色错误结束却进入下一阶段」即此——本次未改语义，只在回复中向用户说明，要不要改成阻断待用户定。
- 修复会话（fix）无短路：进程死在修复中途→续跑从 scratch 重修（正确性优先，成本有界）。
- 静态 `work/final_review` 里的 example/standalone.tex 一并持久化无害（apply 时覆盖同名文件，内容一致）。

### 验证
- 新增 `finalreview_state_test.go` 3 例（回写→指纹一致→apply 覆盖 main.tex→改章节指纹失效；归档往返）；`go test ./...` 全绿；二进制重建 `temp/p5run/dv5`。
- 真实项目回填后 `finalReviewStateCurrent` = true（下次 assemble 将打印「沿用终审产物，跳过终审会话」）。

### 文档
- docs/latex.md 档位1 第 4 步补终审回写/跳过说明；CHANGELOG [Unreleased] Fixed 一条；AGENTS.md 档位1 流程与续跑语义条目同步；批次历史（本批）。

### 补充（同日，T28 第二轮）

- 用户澄清 T28 真意：终端上每个流程的 `[type] 轮次 N · 工具调用 M · 已用 Xs` 行完成后应**定格保留**，下一个流程另起新行——此前 `fin()` 只把行从实时块摘掉，终态直接消失。
- 实现：`livePhaseRowAt` 的 `fin()` 在 `row.Remove()` 后把定格行 `PrintConsole` 进滚动区（36 个 checker 行随完成逐个定格）。
- 顺藤摸出真 bug：`PrintConsole` 被 `l.quiet` 门禁，而紧凑模式（RunBook `SetQuiet(true)`）下 quiet 恰为 true——**阶段说明行（note/phaseNote）、最终汇总、本批续跑摘要全都从未上过屏**（T11 只把 `Log()` 明细压掉了，门禁却同时吞了专用显示通道）。修复 = 去掉 PrintConsole 的 quiet 门禁（调用点全部是必须显示的行），注释写明 quiet 只压 Log() 明细。
- 验证：`go test ./...` 全绿；人工走查：紧凑模式下 style → chapters → convert → checker×36 → assemble 每行终态留在滚动区，实时块只含当前行。

### 用户问"convert/checker 之后的流程去哪了"（答疑批次）

- 原设计仍在：convert 逐章（问题打回同一会话修）→ checker 逐章核对 → **styleFeedbackLoop** 收集 36 份工作汇报，**多数**报告 cls/手册问题才打回原样式会话修样式并重跑 convert（少数派留给 checker/终审）→ assemble（编译失败→修复会话）→ 终审 → out/。
- 李艳芳这本书的实际情况（日志 22825 行）：36 份汇报、4 份有问题 = 少数派（4×2 ≤ 36）→ 按设计**不打回**，所以没有 style-feedback 会话；全书一次编译通过 → 没有修复会话；终审跑了但**旧二进制不写转录**（上一批已修，待部署）——所以 UI 到 checker 就"没了"。style 会话转录在 `work/style_session.jsonl` 一直在。
- 给用户的关键事实：昨晚+今晚两轮终审（72+63 轮）的转录是真没写过、无法恢复；新二进制部署后终审/修复会话都会有转录进 WebUI。

## 第四十六批（2026-09-17，第一批 issue 清理：T50/T40/T38/T46/T41）

### T50 升级会话「消息为空」根因 + 三层修复（提交 4bca64d）
- **现场**：`finally/progress_items/mermaid_fix/` 4 个升级修复会话的转录全是 7 行骨架——assistant 消息无 content/reasoning/tool_calls，usage 行 `finish_reason=length`、`completion_tokens=0`，两轮全空（第 2 轮还是 T35 之前的旧 nudge 文案），日志报 `empty final answer`。
- **排查**：复刻同结构请求（同模型/工具/流式/tool_stream/enable_thinking/max_tokens=65536/真实系统提示词）探测网关**全部正常**——非确定性，指向厂商网关在高压下（现场并发 ~10、限流退避不断）的退化响应。
- **修复**：① `session.Run` 对退化空响应（无内容/无思考/无工具调用 + completion=0）**原样重发一次**，仍退化才落既有 nudge/报错；有真实输出 token 的空回复不误抢 nudge 路径。② mermaid-fix **会话失败或提醒后未提交也升级备选模型段**（原先只有编译错误累计才升级）。③ 未配置可解析 `fallback_model` 时不再进备选段（零值 ModelConfig 防空跑）。
- **测试**：`degenerate_retry_test.go` 3 例（重试成功/重试后仍退化=3 次请求/helper 边界）+ mermaid 3 例（失败升级/无备选不升级/端到端备选段成功）。

### T40 日志对齐 + 日志分析口径
- `[cost]` 表 `%-11s` 按字节补空格 → 中文阶段名（显示宽 2 倍）整列错位：新增 `padStage` 按显示宽度补齐（CJK 按 2 列），表头/数据行/合计行三处同改 + 单测。
- latex 命令的空分析结果不再说「未解析到图片处理会话」（img2text 口径误导），改为说明"档位流程无逐图会话属正常"并指向 [cost] 表与矢量化汇总。
- 「全书转换没有逐步留痕」核实为**旧二进制现场**（T28 续的冻结行/跳过行已在本代码，待部署）。

### T38 sessions --serve 启动慢
- 实测：banner 即时上屏（新旧二进制同），暖缓存 `--list` 138 会话 5.9s 其中 user 仅 0.5s——**gap 是 samba 网络往返**；`latex_project` 2762 文件、`finally/progress_items` 16788 文件（仅 4 个 extras jsonl）。冷启动慢 = 串行 open/read × latency，无法稳定复现。
- 修复：扫描的转录统计（readStat：消息数+SHA+用量聚合）从 WalkDir 回调内**串行**改为收集候选后 **16 路并发**（每会话带 (size,mtime) 缓存不变，顺序按下标保留）；extras 扫描保持串行（现场仅 4 文件，如实记录）。

### T46 checker 必须拿到结论才收尾
- 旧行为：Run 出错或未提交 → 直接「视为通过」。真实项目审计（见 T41）显示 6/36 章 checker 空转即此路径。
- 新行为：**两轮尝试**——每轮漏 submit 先按 tikz 同款提醒一次；本轮仍无结论（会话错误/提醒后未提交）重开一轮全新核对会话（同一转录续写，一章一个文件）；两轮都失败才降级通过并打 warning。compile 与终审仍是硬关卡，降级语义不变。
- 测试：`checker_test.go` 2 例（退化会话→重试轮成功 / 两轮全退化→降级且请求数 ≤6）。

### T41 checker 提出问题是否解决的审计结论（不改代码）
- 李艳芳项目 36 章 checker 转录逐一解析（submit 参数嵌套 `report.status`）：**30 章 pass、0 章 issues、6 章无结论**（008/014/020/024/028/030）——checker 从未提出过问题，故无"未解决的问题"。
- 6 章无结论的根因 = **T50 同款退化空响应**（ch008 日志实证：round 1 用时 136.5s、content=0/completion=0，旧 nudge 也无工具调用）——已被本批修复覆盖。
- 如实提醒：checker 全 pass 不代表排版质量（它只对照 md 核内容完整性），质量把关实际由终审完成（72 轮发现并修复问题）。

### 验证
- `go test ./...` 全绿；`dv5` 重建（含本会话全部提交）。
- 遗留：T41 的 6 章若要补核对，重跑 convert/checker 阶段即可（增量）；新二进制待用户停 8849 后部署。

## 第四十七批（2026-09-17，第二批：T42/T49/T44）

### T42 最终 PDF 复制到外层
- assemble 交付后把 `out/book.pdf` 另复制到 `<latex_project>/<项目名>.pdf`，日志给路径；失败只告警（交付不受影响）。

### T49 成品 PDF 书签与跳转
- 根因：kyexam.cls 不加载 hyperref，成品 PDF 无书签无跳转。assemble 编译前对 main.tex **幂等注入** hyperref（bookmarksnumbered/bookmarksopen/hidelinks）+ 逐章 `\pdfbookmark`（锚在 `\input{chapters/…}` 前，文字取划分 md 首个标题行、缺失退 base 名；`_ % & # \` 转义）。
- 骨架与终审持久化版 main.tex 都过同一注入（`ensurePDFBookmarks`），已有 hyperref 不重复加、已有锚点不重复加（幂等单测）。
- 端到端验证：真实 kyexam.cls + xelatex 两遍编译，`/Outlines` + `/PageMode /UseOutlines` + UTF-16 书签标题 + GoTo 跳转均在成品 PDF（首轮字节级检查被对象流压缩骗过，qpdf --qdf 解压确认）。
- 注意（如实记录）：`\pdfbookmark` 锚点在 `\input` 前执行，章节若以 \clearpage 起页，书签落在前一页尾——极端情况下跳转偏一页；逐书 cls 行为不同，可后续视实际 PDF 再调。
- 用户定调（同日补充）：书签不应固定为"一章一条"——基线锚点保底，**结构交给终审会话按书的实际情况布置**（final-review 提示词新增第 5 项：检查书签面板、可重分层级/\addcontentsline、替换时删基线锚点）；main.tex 已带任何 \pdfbookmark/\bookmark 时注入器只补 hyperref、不再加基线锚点（`aiOwnsBookmarks` 守卫 + 单测）。

### T44 assemble 资源盘点
- 编译前一行盘点：cls 就位与否、章节/figures/images 文件数、逐章 `.tex` 的 `\includegraphics` 引用在 build 树的解析数（buildDir / figures/ / chapters/ 三个候选位置），缺失逐条告警不中止。
- 测试：preflight 单测（存在引用解析、缺失不解析）。

### 验证
- `go test ./...` 全绿；dv5 重建。文档：CHANGELOG Fixed 三条、docs/latex.md 档位1 第 4 步同步、AGENTS.md 档位1 流程条目同步。

### T49 补充（同日）：书签层级=书的真实层级
- 用户质疑：转换单元只是转换用的拆分，不等于书中同等地位的章节并列，基线书签全平铺不对。核实：划分会话按 markdown 标题边界拆，单元 md 首个标题行带 # 数=真实层级（大章=#、被拆出的小节=##），层级信息其实在产物里、只是注入器没用。
- 修复：`chapterBookmarkTitle` 同时返回标题文字与层级（#→0 层、##→1 层，clamp 0..3），`\pdfbookmark[level]` 按真实层级生成——一个真实大章被 medium 拆成多个单元时，第一单元 0 层、后续单元 1 层，书签树=书的真实结构。终审会话仍可整体接管。

### T49 再补充（同日）：李艳芳扁平书签根因判定 + 编号冲突处理
- 用户问：李艳芳 book.pdf 书签扁平、区分不了题号/选项，是 cls 没写好还是章节划分的原因？判定：**主因 cls**——① 那份 PDF 是旧二进制出的，当时根本没加载 hyperref（一个书签都没有）；② kyexam.cls 的结构命令（\papertitle/\examsection/question 环境）全部自定义、不调 \section，hyperref 自动书签无锚点来源。与章节划分无关——划分只决定顶层粒度，题号环境不是标题、永远不会自动进书签。
- 处理：终审提示词第 5 项补题号级导航建议（习题/试卷类书可在 question 环境里埋 \pdfbookmark 让读者跳到"第 N 题"，由终审按书判断，选项级不建议）。
- 编号冲突：issue.md 出现两个 T49/T50（用户新条目撞号）——新条目重编号为 T51（书签按书结构、大中有小，已由 基线层级+终审接管+T45 medium 覆盖，标 [X][ ]）与 T52（mermaid 升级会话一串问题：无 edit_file/空回复/32 轮上限/校验/UI 混排，未做）。

## 第四十八批（2026-09-17，第三批：T45/T43）

### T45 章节划分三档，默认改中等
- `chapter_granularity` 现有 small/large 两档，新增 **medium（默认）**：提示词要求按这本书自动给出最合理拆分（通常一个顶层章一个文件，明显过大才再拆；绝不拆太细、绝不留超大块、绝不拆碎小节）。
- 同步：config.go 校验/默认值、两份配置模板、docs/config.md。已显式写 `small` 的老配置不受影响（只改缺省值）。

### T43 风格修复大循环（替换多数派打回）
- 旧行为：>50% 章节汇报样式问题才打回原样式会话（上限 2 轮），之后只对报问题的章节跑单章增量修复。用户要求：任何汇报问题都应进入修复，且要 整合→修复→评估影响→分块并行修→循环 的完整闭环。
- 新实现 `stylefix.go`：
  1. **整合会话**（stylefix_integrate）：纯文本会话，把各章汇报整合成不重复问题清单，新工具 `submit_problems`（固定格式：id/title/detail/chapters）。
  2. **风格修复会话**（stylefix_style_r<N>）：**复用原样式会话上下文**（`work/style_session.jsonl`——按用户"衡量一下"的口径选复用：信息不丢失 vs 缓存失效只是成本），提交后写回 cls/手册并编译 example 验证。
  3. **影响评估会话**（stylefix_assess_r<N>）：只读挂载整个项目，可 grep 全部章节 .tex 圈定每个问题的真实影响面（样式问题常全局），`submit_blocks` 交出分块（每块带问题编号与章节范围，同问题的章归一块）。
  4. **分块修复会话**（stylefix-blk-r<N>-<i>，并发=latex.concurrency）：临时工作区（样式包+块内章节+图片软链+每章 wrapper），定位修改绝不重做，compile 验证后 `submit` 逐章汇报 resolved；产物拷回 work/chapters/，汇报回写 work/reports/<章>.md。
  5. 任一章未解决 → style-fN+1 下一轮；上限 `latex.style_fix.max_rounds`（默认 3，负数=关闭）；块会话失败的章退回既有单章 fixChapterStyle，再失败删产物重转换。
- 新配置块 `latex.style_fix.max_rounds` → **配置版本 11→12**（config.go/default.yaml/config.example.yaml/README 示例行/docs/config.md 同步；测试固件 config_version 同步；temp/config_new.yaml 改写固件同步）。
- 新提示词模板 3 个（integrate/assess/block system），注册表 + MustMention 守护。
- 删除：旧 styleFeedbackLoop（多数派门 + 内嵌单章修复循环，约 230 行）与 maxStyleFeedbackRounds 常量；maxCheckerRounds 常量保留（原被误删，已恢复）。
- 测试：stylefix_test.go 4 例（submit_problems/submit_blocks/submit 块结论/collectIssueReports）；全仓 go test 绿。
- 遗留（如实记录）：stylefix 会话无断点续跑重放（中断后整轮重来，成本有界）；整合会话只跑一次、问题清单跨轮复用（修复轮之间问题集假定不变）。

## 第四十九批（2026-09-17，T52 mermaid 升级会话整修）

### 现场证据（finally/progress_items/mermaid_fix/ 4 个升级会话）
- 工具确实只有 write_file/grep/view_image/submit——没有 edit_file，改一处要整篇重写。
- 「submit 总是 error 即使已经没有报错」= 检查预算耗尽后的拒绝（`REJECTED: no check attempts left.`，Rounds=6 用满），文案没说清是预算而非校验失败；真校验记录（compile_error.log 里 mmdc 解析错误）是有的。
- 「UI 里这次的和上次的混在一起」= 同一图的多次升级运行 append 进同一 session-stageN.jsonl，新旧消息混排。
- 「空回复/Provide your final answer now → 第二次才 Continue」= 两层提醒（session 引擎空回复提醒 vs 会话层未提交提醒），根因（厂商退化空响应）已由 T50 修。

### 改动
- **edit_file 工具**（find/replace，唯一匹配才改；多处→AMBIGUOUS、没有→NOT FOUND）；系统/任务/nudge 提示词同步提及。
- **session_rounds 默认 6→32**（用户指定；注意：配置文件里显式写了 6 的不会自动变——runtime config.yaml 需要手改或删行）。
- **转录轮转**：rotateTranscript——旧运行改名 `session-stageN.prev<M>.jsonl`（.prevN 插在 .jsonl 前，预览扫描 *.jsonl 会把它显示为独立会话），保留 3 份。
- **预算耗尽拒绝文案**改说人话（exhausted N/M + 指向 compile_error.log）。
- 预算提醒机制答疑（不改代码）：剩余轮次是一条**原位替换**的 user 消息（reminderIdx），不堆积、前缀缓存友好——不附加在工具回执里是因为回执是历史的一部分、改写历史回执会让模型把过期数字当事实。
- 测试：edit_file 3 例、轮转 3 例、耗尽文案 1 例；全仓 15 包绿；dv5 重建。

## 第五十批（2026-09-17，T47 审计 + T48 绘图语义）

### T47 会话留痕审计（结论：多轮会话全覆盖）
- 代码审计：latex+img2text 共 14 个 NewSession 点，**全部挂 JSONL 转录**（style/chapters/convert/checker/stylefix×4/book_fix/final_review/vector(tikz)/figure_check/mermaid-fix）。vector 作图转录在 `<书>/source/sessions/vector_*.jsonl`，在预览根内、WebUI 可见。
- 无转录的都是**一次性调用而非会话**：classify 分类、img2text 逐图分析（量太大，由 preview.img2text_all debug 开关打开）。mermaid_fix 转录一直写，进 UI 由 preview.img2text 控制。
- 结论：T47 目标已满足，无缺漏要补。

### T48 绘图语义理解
- figure.system 第 2 步与核心规则补一句：先理解图表达什么（立体图形/几何构造/上下文所述关系），用匹配构造画（3D→tikz-3dplot/透视坐标，几何→tkz-euclide），不描摹投影像素。提示词增量 2 句，未动其他。

## 第五十一批（2026-09-19，P18 img2text 进 WebUI 独立板块）

- **Go 数据面**：
  - `SessionInfo.Board`（img2text 来源会话打 "img2text"）；新 API `/api/img2text-progress`（serve.go 路由 + viewerServer.img2textRoot = progress_items 根）。
  - `ScanImg2TextProgress`（img2text_progress.go）：从 progress_items 树推导每书每图状态，**真实布局实测钉住**——`<md>.md/original.md` 是书的 markdown 副本（目录名保留 .md 后缀！第一版把它当文件全踩空）；逐图 json 在 `<md>/`，命名新旧两种（`images_<hash>.jpg.json` / `images_<md>_<hash>.jpg.json`）；mermaid_fix/<md>_<图>/ = 升级工作区。四态：done / **fixed**（有结果且留过修复工作区=修复成功）/ escalated（只有工作区没结果）/ pending（无记录——失败跳过与未跑不可区分，如实报"待处理"）。真实数据验证：80 本书，27政治 64 图 = 60 done + 4 fixed（与 mermaid_fix 4 个工作区精确对上）。
  - `preview.img2text_all` 按用户决定**保留并默认 true**（nil→true，改 *bool）：控制逐图分析转录记录；升级修复会话始终记录不受其管。
- **web（子代理实施，父代理复验）**：Sidebar 头部板块切换（LaTeX/img2text，`side.board` 落盘，无 img2text 数据时隐藏并强制回落 latex）；img2text 板块顶部每书进度概览（分段进度条 done 绿/fixed 青/escalated 琥珀/pending 暗 + 计数）；buildGroups 先按 board 过滤；独立 5s 轮询。vitest 36/36（新增 board.spec 5 条）。
- 验证：go test 15 包绿 + httptest 冒烟（有根/无根两种）；真实 progress_items 扫描数字如上。运行目录的端到端冒烟被 sandbox 拦（sessions 命令要写 sessions.html 静态导出到 latex_project/），留待部署后目检。
- plan.md P18 标 [X][ ]。

## 第五十二批（2026-09-19，T53 升级会话整修 + 调用链聚合）

### 真实缺陷（全部有转录证据）
- **view_image 从没看过原图**：4 个真实升级会话里 view_image 全部回执 `image decode failed: decode image output/images: image: unknown format`——`MermaidFixConfig.ImgPath` 从未按图赋值（fixCfg 全 worker 共享），空路径被 resolveImageFile 解析成 images 目录本身（目录也过存在性检查），decode 才炸。修法：闭包内复制配置按任务填 ImgPath；resolveImageFile 拒绝空路径/目录。
- **成功会话显示红色 error**：mermaid submit 成功回执是 "OK: ..."，侧栏终态判定认规范前缀 "SUBMITTED."（latex 全部工具都用它）——手写工具漂移的代价，已对齐。
- **running 卡 0**：进度行只在结果到达时重印，2 张图跑几分钟就停在 `[0/2] running: 0`——writer 循环 select 化 + 2s 心跳。
- **缺 read_file**：只有 grep 能看内容，补 read_file（全文/行区间，读 submit.md/compile_error.log）。
- **同图多次会话平级罗列**：web 侧按图聚链（归一 `<hash>.<ext>`：sessions 剥 images_/<md>_ 前缀、mermaid_fix 按最后下划线拆），主行=最新、「历史 N」展开器带记忆、方块视图一链一块；归一失败平铺兜底。子代理实施（51 条 vitest 全绿），父代理复验构建。
- 工具复用答疑：mermaid 会话工具是手写简化版（工作区模型不同，未复用 tools_work.go）；本次 SUBMITTED 前缀漂移即其代价，已记录。

## 第五十三批（2026-09-19，T54 进度概览折叠 + 块视图）

- 用户反馈：80 本书的进度概览列表太长。改为整块可折叠（头部「进度概览 + N/M 书完成」聚合行，v-collapse 记忆、默认折叠）；列表支持行式/方块切换（独立记忆 i2t.overviewBlocks），块视图按最重状态着色（escalated>pending>fixed>done）、点块填搜索框跳书。
- 顺手补漏：前端概览条此前只有 done/esc/pend 三段——P18 后端后加的 fixed 第四态前端没跟上，本次补齐（青段 + 「修复 N」计数 + types.ts 字段）。
- 子代理实施（vitest 58/58 全绿含新 7 条），父代理复验 go build。T54 标 [X][ ]。

## 第五十四批（2026-09-19，T55 analyze 三修）

- **`-r 0` 选错日志**（真实证据：用户最新一次是 img2text 运行，analyze 却分析 latex_20260916_184221.log）：logfind/analyze 一律按文件名字符串排序，"img2text_" 字典序恒小于 "latex_"，时间再新也排在前面。修法：按文件名内嵌时间排序（logfind.SortByTime 供 FindLatest/FindAll/WithFallback 与 analyze.sortNewestFirst 共用）。
- **慢**：进度摘要 CheckProgressItems 全量 ReadFile+Unmarshal 1.1 万个 json（samba 盘）。改并发（16 路）字节扫描（"[IMG_TYPE:" 出现且非 __INVALID_RESPONSE__ 即完成），实测 0.2s 出全表。
- **良品率口径**（用户定）：良品 = 完成且无 ERROR 且无 WARNING（警告自纠正成功但影响良品率）；另报错误率（[ERROR] 图占比）与警告张数。ScanLogIssueImages 把原 GetProblematicImages 拆成两集合（同图兼有 ERROR 只算错误），旧接口保留只剩错误集。
- 测试：混合前缀排序钉住、ERROR/WARNING 拆分钉住；真实运行验证 -r 0 选中 img2text_20260919_204549.log。

## 第五十四批补（2026-09-19，进度摘要增量缓存）

- 追问"每次都要全部索引一下吗"：加 `progress_items/.progress_stats.json` 判定缓存（路径→(mtime,size,done)），未变文件复用判定、只读新增/变化的；已消失文件条目清理防胀大；并发度 16→64。实测本机 samba 冷/热均 ~0.27s；用户侧"非常慢"的主因是运行目录还跑着旧串行二进制，部署后生效。

## 第五十五批（2026-09-19，T56/T57 逐图会话显示链整修）

### 真实缺陷（均有真实转录/请求证据）
- **方块爆红**：逐图分析是一次性会话（成功信号 = 答案里的 [IMG_TYPE:），模型偶尔调 getmorecontext（真实转录回执 "Added 10 lines above and 10 lines below"）即满足 SawTool && !SawSubmit 误判 error。补 SawTyped 信号。
- **user 消息显示原始 JSON**：Append 的 multipart 分支只认 []map，recorder 的 json 往返产出 []interface{} → 整块序列化落盘。抽 flattenParts 两态通吃；存量旧转录前端 parseLegacyImageParts 归一（拆文本+图片缩略图，归属钉"本会话任务"）。
- **img2text 会话图片 404（T57）**：mediaURL 用会话 ID 拼路径，img2text: 前缀没剥 → /media/img2text:mermaid_fix/... 不存在。sessionDir 通用剥前缀。
- **历次调用历史没地方看**：逐图转录 NewTranscript 是 append 模式，多次运行混排一个文件。改 prevN 轮转（与 mermaid_fix 同款），web 链解析 sessions/<md>/<图>.prevN.jsonl 归并同图历史。
- **mermaid 预览（T56 追加）**：POST /api/mermaid（mmdc + sha256 缓存于用户缓存目录，实测首渲数秒/缓存 0.6s；坏源码 in-band ok:false）；前端 ```mermaid 块下挂 SVG（DOMParser，不碰 innerHTML），点击编 data URL 进现有灯箱，失败/静态模式回退代码块+一行小字。
- 两子代理分别实施 web 两批（68→80 条 vitest 全绿），父代理复验构建与端点实测。

## 第五十五批（2026-09-19，T56/T57 逐图会话显示链整修）

### 真实缺陷（均有真实转录/请求证据）
- **方块爆红**：逐图分析是一次性会话（成功信号 = 答案里的 [IMG_TYPE:），模型偶尔调 getmorecontext（真实转录回执 "Added 10 lines above and 10 lines below"）即满足 SawTool && !SawSubmit 误判 error。补 SawTyped 信号。
- **user 消息显示原始 JSON**：Append 的 multipart 分支只认 []map，recorder 的 json 往返产出 []interface{} → 整块序列化落盘。抽 flattenParts 两态通吃；存量旧转录前端 parseLegacyImageParts 归一（拆文本+图片缩略图，归属钉"本会话任务"）。
- **img2text 会话图片 404（T57）**：mediaURL 用会话 ID 拼路径，img2text: 前缀没剥 → /media/img2text:mermaid_fix/... 不存在。sessionDir 通用剥前缀。
- **历次调用历史没地方看**：逐图转录 NewTranscript 是 append 模式，多次运行混排一个文件。改 prevN 轮转（与 mermaid_fix 同款），web 链解析 sessions/<md>/<图>.prevN.jsonl 归并同图历史。
- **mermaid 预览（T56 追加）**：POST /api/mermaid（mmdc + sha256 缓存于用户缓存目录，实测首渲数秒/缓存 0.6s；坏源码 in-band ok:false）；前端 mermaid 围栏块下挂 SVG（DOMParser，不碰 innerHTML），点击编 data URL 进现有灯箱，失败/静态模式回退代码块+一行小字。
- 两子代理分别实施 web 两批（68→80 条 vitest 全绿），父代理复验构建与端点实测。

## 第五十五批补（2026-09-19，跨行 $$ 公式不渲染）

- 用户报会话里双 $ 公式原样显示。根因：数学渲染全在行内正则 MD_INLINE，而 mdInlineLines 逐行扫描——跨行 $$…$$ 永远匹配不到。修法：renderMarkdown 块级循环加 collectMathBlock（$$ 起手识别：单行同起同收/多行/未闭合吃到文末；裸 $$$$ 不算），mdBlockStart 同步认 $$（防列表懒续行吞掉）；坏公式 try/catch 回退原文。node 测试环境无 DOM——识别逻辑抽纯函数钉 5 条 vitest。

## 第五十五批补二（2026-09-19，中栏「历史」页签）

- 用户找不到侧栏的链历史入口，提议页签化。实现：chainOfSession(sessions,id) 纯函数（按项目聚链防跨书同哈希混并，>1 成员才成链）+ HistoryPanel.vue + App.vue 第三个页签（computed 按 sessions 引用缓存，watch 标量 getter 避开数组多源陷阱；切到链外会话回落「对话」；历史页内点行切换停留在历史页）。vitest 88/88。子代理实施、父代理复验。

## 第五十六批（2026-09-20，T58/T59）

- **T59 空回复无重试**：逐图分析路径的空响应（0 choices/全空）直接记 EMPTY_RESPONSE 等下轮。在 doCallWithRetryFull 加重试层：isDegenerateResponse 判定（无 choices 或 全空 content 且无工具调用）→ 原样重发 ≤2 次（字节一致保前缀缓存），仍空才算失败。与 T50 会话层自救是同一现象的两次露面。
- **T58 运行中书不可见**：进度板加 running 态——sessions/<md>.md/<图>.jsonl（含 prevN 归一）在活动窗口内被写过且无结果。web：蓝色脉冲段 + 进行中计数 + 方块最优先着色。子代理实施（vitest 90/90），父代理复验。

## 第五十六批补（2026-09-20，latex 看图丢图回归修复）

- 用户报新 latex 会话无缩略图/无分组差异/思考块疑问。查证：① 丢图是 T56（2067210）引入的真实回归——flattenParts 断言 flip（string-map→any-map）与 toolImageContent 产物不符，静默跳过。9-12 老转录有图、9-20 新转录无图即为时间证据。修复：产物改 any-map + 断言双形态兼容；回归测试 TestAppendToolImageTurnStoresImage 钉两种形态（此类断言此前零覆盖才致回归）。② 分组无异常：/api/index 实测该书 7 条会话 stage=vector 全对，单阶段项目按设计平铺显示。③ "思考块"：gpt-5.6-luna 的 reasoning_content 只有加密摘要头（**Planning TikZ creation** 一行）——厂商行为，非格式不兼容，无需 response_format 切换。

## 第五十六批补二（2026-09-20，加密思考摘要显示兼容）

- gpt-5.6-luna 的 reasoning_content 只有 `**Planning TikZ creation**` 式加密摘要头，用户误以为思考块空。isEncryptedSummary 纯函数判定（≤200 字符、无空行多段落、**…** 段为主），思考块标题加 🔒「加密摘要」胶囊；Disclosure 加可选 encrypted prop，其余调用方零变化。子代理实施（vitest 97/97），父代理复验。

## 第五十七批（2026-09-20，P21 页码标记 + P23 发布线改革 + dev 分支）

### P21 img2text md 分页页码标记
- 数据源查实：mineru_output 部件目录的 *_content_list.json 每条目带 page_idx（真数据是字符串形态），图片条目 img_path 可对齐 md 图片引用。
- pagemark.go：锚点扫描（docPartDirs 同款逻辑本地复制，不 import latex）+ 行级对齐（图片行按文件名、文本行去空白互相包含；短锚只认全等；未命中继承上一页）+ 递增处插 `[P. N]` 独立行（围栏内顺延、同位置去重）+ 图片 embed 锚加 `· P.42`。
- 真实数据冒烟（只读）：50 本书 25,621 处标记，非空行覆盖率 47.1%，每本标记数≈书页数（399 页书 370 处）；2 本无产物静默跳过。
- 取舍：embed（字节替换）与标记（行级）分两遍、delta 平移 offset；幂等靠从原始 md 重建 + 字节一致跳过。

### P23 发布线改革
- dev 分支建立（本地领先 origin 的 5 提交即 T56~加密摘要全在 dev；本地 master 回对 origin=beta.8）。
- release.yml：alpha/正式版标签自动发布；dev/beta 标签只构建验证；workflow_dispatch 手动选标签发布。
- 远端：Release v1.5.0-beta.4 已删（标签保留）；现在 Release 只剩 v1.0.1/v1.1.0。
- beta.5/beta.6 CI 失败根因：sessionview 的 chromium 渲染测试在 runner 挂死 10 分钟 → 加 2 分钟超时跳过护栏。
- RULE.md 版本阶梯表 + AGENTS.md 发布线纪律同步；CHANGELOG 跨正式版汇总规则入 RULE。

### P21 用户实测三修（同日第五十七批补）
- **方括号书名 0 锚点**：`[正度纸] 李-范`、`[a4] 大雪深埋` 的 content_list 用 filepath.Glob 找——方括号被当字符类静默失配，整本书 0 标记。改 ReadDir 按后缀挑选（跳 _v2）。
- **目录区标记爆炸 + 页码狂跳**（P.2 后直接 P.243）：目录行"第三章 认识论……029"包含匹配到 body 里同名章节标题锚点，单调指针被拖过全书。加 60 锚前向窗口 + 匹配页码不回退守卫——目录行继承未知页不产标记。复验：闪过 89 处首几页 2,3,4,5,6,7,13…单调；李-范 576 处、大雪深埋 273 处全部正常。
- 用户报的"跳过（内容无变化）"即 0 锚点书的 skip 路径；修复后重跑会写入标记。已写坏的页码文件重跑 img2text 会从 original.md 重建自然清除。

### P21 自检日志 + 正确性抽查（同日）
- 日志升级：`页码标记: N 处 / M 页`（M=该书部件页数上限），越界/回退时附 `⚠ 自检异常`（pageMarkPlan.audit）。
- 正确性抽查（1000题解析册，1882 行文本行 vs content_list 真实 page_idx）：初轮 47% 一致是我的对账脚本太粗——把目录行对到 body 标题（程序已用窗口修掉这个错，脚本没有）；剔除每页重复的页眉/模板行 + 严格唯一匹配后 **91% 一致（671/730）**；剩余 59 行全是"全书重复出现的标题/公式行"（如"第9章 一元函数积分学的计算"在子目录和正文各出现一次），我的脚本取首个锚点判定、程序按阅读顺序指针取的是正确那次——抽查脚本的局限，非程序错误。

## 第五十八批（2026-09-22，T60/T61 web 渲染）

### T60 思考块 Markdown 渲染
- AssistantMsg 思考正文从 `<pre>` 原文改为 MdBody（renderMarkdown）——加密摘要的 `**内容**` 各自成段分行、普通思考的 Markdown 语法生效；颜色/字号设在 `.reasoning-md` 包裹层继承（scoped 够不到 MdBody DOM 节点，已知陷阱）。
- 折叠态标题：加密摘要走新 `encryptedHeadline()`（首个非空行剥 `**`），不再裸露星号；普通思考仍 firstLine。

### T61 LaTeX 括号形式
- MD_INLINE 增加 `\(…\)`（行内，组 12）与 `\[…\]`（显示，组 13）；collectMathBlock/mdBlockStart 认 `\[` 块（单行/多行/未闭合同 $$ 三形态；裸 `$$$$` 与 `\[\]` 退 null 语义保持）。
- 真实数据：某书.md 有 `\(kA\)` 等 2 处此类形式。
- 测试：markdown.spec +6、encrypted-summary.spec +3，全仓 vitest 106 绿；npm build + go build（embed）过。

## 第五十九批（2026-09-22，T62~T68）

- **T62**：book_fix/final_review/stylefix_*（含 stylefix- 连字符变体）在 LabelFor 全落「会话」桶、标题是裸文件名。补映射：book_fix→fix 阶段「全书修复」、final_review→final-review「终审」、stylefix_*→style-fix「样式修复 · …」；web STAGE_ORDER/stageTitleOf 同步。
- **T63**：convert/checker 阶段聚合（stageChapterAggregate）——跑动只列 live 行（并发可见）、全完成收行、阶段头「N 章 · 完成 M · 运行 K · 错误 E」、错误会话保留、搜索不聚合；块视图同规则。
- **T64/T67**：整合会话虚拟工作区 `work/stylefix_integrate/`（reports/<章>.md 落盘 + read/grep/write/edit），整合后代码生成未打钩 list.md；style 会话 tag 统一 stylefix-style-rN。
- **T65**：`latex.style_fix.integrate_model`（空=沿用 style 模型）；config 两模板 + docs/config.md 同步（键在既有块内，无版本 bump）。
- **T66**：块工作区放未打钩 list.md；SubmitBlockFixTool 增加 checked_problem_ids 必填、代码核对全覆盖，缺勾 REJECTED（3 条测试钉住）。
- **T68**：分块修复会话补 ViewPDFTool（book_fix/终审/单章修复原本就有）。
- 验证：go build + config/latex/sessionview 测试全绿；vitest 112 绿（+6）；npm build 过。

### T69 页码注入开关（同日第五十九批补）
- `img2text.page_marks`（默认 true，P21 行为不变）+ `img2text.page_marks_rewrite`（默认 false）。
- runner Final pass：rewrite=false 时已交付的 finally 文件按**原有风格**处理——原来没页码的（hasPageMarks 判定）plan 置 nil 不回注，增量更新照写；删掉 finally 结果重跑才按 page_marks 开关注入。
- 两键 *bool 形态（与 img2text_all 同款），nil = 默认；测试 hasPageMarks（独立行语义）+ 默认值语义。

### T70 修复链会话独立模型 + 评估会话 blocks.json 可编辑提交
- 五个模型键进 `latex.style_fix` 块（stylefix_style/assess/block、bookfix、final_review），空 = 回退原角色模型（style/convert）；`integrate_model` 模板默认 `""` → `"integrate"`。
- models 注册表新增 6 条目（integrate/stylefix-style/stylefix-assess/stylefix-block/bookfix/final-review），全部 `extends` 自原角色 = 行为不变；模板插入时曾把 verifier 的 image_tokens 挤进 final-review（TestTemplatesResolveImageEstimatePerRole 抓住），已复位。
- **真实缺陷（extendAll pass 上限收缩饥饿）**：`mergeModelExtends` 循环上限写 `pass <= len(extendsOf)`，len 随每轮 resolve 收缩——字母序排最前的 bookfix 每轮都赶在父条目 convert 解析前被检查，最后一轮被砍 → 误报「循环继承（bookfix → convert）」。复现链：bookfix → convert → heavy → text（bookfix 字母序在 convert 前，且前面还有 gateway/style 撑高初始计数）。修：上限循环前取定 `totalExtends`；回归测试钉住（误报 + 真环仍报出）。
- 评估/分组会话（stylefix_assess_rN）获得虚拟工作区 `work/stylefix_assess/`：blocks.json 由会话 write/edit、`submit_blocks` 无参从文件读（内联仍兼容）；Workspace 为空时必须内联。blocks.json 最终态回存工作区。测试：TestSubmitBlocksFromWorkspace（文件读/空工作区拒/坏 JSON 拒）。

### T71–T78 终端对齐 / 修复会话提示词 / webui 五连修
- **T71**：费用表表头 `%Ns` 按 rune 数补位，中文表头（会话=4 列）配 `%Nd` ASCII 数据错位。`padLeftWide`（显示宽右对齐）修表头；`padStage` 泛化成 `padWide(left bool)`。
- **T72**：`compile_error.log` 只在 `recordError` 后落盘；全新升级工作区只有 submit.md，但 systemPrompt/taskPrompt 无条件宣称它存在 → 模型照读必报 not found。改 `hasErrorFile()` 条件化提及；grep 工具描述去掉具体文件名列表。
- **T75**：旧口径 `ScanLogIssueImages` 任意日志报过错就永久计坏（修好的也扣），或只扫最新日志漏掉终态问题。新增 `FinalImageVerdicts`：日志按 mtime 升序扫描、每图取**最后一次出现**的块结果为终态；footer 增列「无效（下轮重试）： N 张」。
- **T73**：`mermaidPreviewBlock` 自带 bar+toggle（源码/图表，右上角 icon-btn），双视图同一时刻只显一个；渲染失败自动切源码视图；`mdCodeBlock` 对 mermaid+可用预览时不再渲染源码正文（复制按钮仍在块头）。
- **T74**：`LiveWindow=3min` 余晖让已提交会话仍显运行中。`readStat` 新增 `EndsWithSubmit/EndsWithTyped`（流式扫过时维护：SUBMITTED/[IMG_TYPE: 置位、后续任何 msg 行复位），`liveNow()` = mtime 新鲜 && 未以完成信号收尾。
- **T76（真实缺陷）**：`ExchangeRecorder.seen` 只数请求消息——get_more_context 轮的 tool_calls 助手回复在响应时落盘、又在下一轮请求尾巴处被重写（webui 工具调用显示两次）。游标在追加响应后 +1。老测试 ReconstructsTranscript 期望 9 行改 8 行。
- **T77（真实缺陷）**：`numberImg2Text` 按 项目+阶段 分组编号——升级修复组从 #1 重新起算，链外层徽标取最新成员（修复会话）的号 → 两链同显 #1。改为按**图片**编号（项目内图片排序、同图全体成员同号），短名撞车升级同样按去重图片算。
- **T78**：Go `TitleFor` 对 style-fix 四类 subject（integrate/style_rN/assess_rN/blk-rN-i）给中文点名（`styleFixPhase`）；web `stylefixCompare` 阶段内排序（整合→修复→评估→分块，同环节轮次/块号升序）。fix→final-review 阶段序已在 STAGE_ORDER 里正确（fix=7 < final-review=8），复认无误。

### T79 聚合阶段手动展开入口
- T63 聚合后零行（全完成+无错误）时阶段下没有任何会话行、也无展开入口——用户以为会话丢了。改 Sidebar.vue：aggCollapsed 时渲染「展开全部 N 章（已聚合，点击展开）」按钮，与 overflow 共用记忆键（展开状态落盘）。

### T80/T81 聚合展开修复 + 页码审计降噪
- T80（T79 的真实缺陷）：`aggCollapsed` 翻转 false 后 shown0 落回 agg.rows（空），「展开全部」点了还是空白；方块视图直接读 agg.rows 同病。改 collapsed/expanded 两态显式建模 + blockRows 走完整列表。
- T81：Final pass 每文件都打「页码标记: N 处」审计行，实际写入按字节比对全部跳过——用户读成"每轮全量重打"。内容 unchanged 时抑制审计行（bytes.Equal 预检），只有真写入才报。

### T82 聚合部分可见时的展开入口
- T79/T80 只覆盖「聚合后零行」。真实数据（2026年李艳芳预测三套卷数一_1：checker 30 完成+2 错误+4 无终态）下 agg.rows=2 行错误 → 非零行 → 无展开按钮，用户看到「章节核对下只有 2 行」以为又没显示。改 canExpand = agg.rows.length < items.length（零行与部分行统一），expanded 走完整 items。

### T83 方块视图聚合空白
- 「展开全部」按钮此前只在列表视图（v-else 分支）渲染；方块视图（v-if=blockView）分支只有 .session-blocks 网格——聚合阶段（agg.rows 空/部分）就是空白，用户截图实测。方块视图分支补同一按钮（v-if=blockView && aggCollapsed）；实测：0 块 + 按钮 → 点开 3 块。顺带 convert/checker 加 chapterOrder 升序（块标签曾按 mtime 乱序 2/1/3）。

### T84 缺包检测兜底（真实案例）
- 测试-概率论终审会话：模型 bash 里 `try: import pypdf except ImportError as e: print('no pypdf', e)`——输出只有裸的 "No module named 'pypdf'"，`moduleErrorRe` 要求 `ModuleNotFoundError:`/`ImportError:` 前缀 → 自动安装没触发，模型改用 mutool 绕路。加 moduleErrorBareRe 兜底（误伤面：文本内容恰好含该短语 → 最多多装一个包）。
- 顺带查明该终审会话的真实结局：round 66 工具回执后的下一次 API 请求失败（endState=error），assemble 按设计告警「终审未通过（保留已编译全书）」并照常交付 out/——不是"提前退出"的设计行为。失败原因打在运行终端，转录不含错误行。

### T85 终审/全书修复续跑（真实事故驱动）
- 真实事故：测试-概率论终审跑到 66 轮、正在查书签重复（已发现 bk:chapter1.0 vs chapter.1 重复），round 67 的 API 请求失败 → 会话 error 收场。assemble 按设计告警「终审未通过（保留已编译全书）」照常交付 out/，但下次重跑终审从 0 重烧。
- 改法：新增中断态 `work/final_review/state.partial.sha256`——失败时（Run 返回 error）把当时 build 树的章节/顶层 .tex 回写 work/（与完成态同一套拷贝，指纹落不同文件）；assemble 入口指纹一致 → applyFinalReviewState 恢复顶层编辑 + finalReview/fixSession 带 resume=true 走 LoadTranscriptStats/SetMessages/SeedCounters/replaySubmit（convert 同款），历史以 tool 回执收尾时 resumeUserText 返回空、原样续行不插话术。完成态写入时清掉部分态指纹。测试 TestFinalReviewPartialState 钉住四态（部分态不触发完成跳过/章节变化失效/完成覆盖清部分态/完成态为真）。
- 顺带回填了事故现场：build/ 树还在（三章编辑与 work/ 不一致），用一次性 env-gated 测试把中断态写进 测试-概率论/work/final_review/（已验证指纹与章节同步），用户部署新版后重跑即可续跑那 66 轮。
- 「所有会话续跑」现状盘点：convert/样式/章节划分/矢量图早已续跑；checker 按章粒度 verdict 续跑；style-fix 循环有 blocks.json 等状态文件；本次补上 fix+final_review 两个空档。

### issue.md 条目损坏事故（自查自修）
- T78–T85 条目文本一度消失：给 issue 打勾/加条目的 python 脚本用**裸子串锚点** `s.replace("[X][ ] T78.", …)`——匹配到整行的行首，把原行剩余文本挤到新条目之后；多批滚动后 T78/T79/T81–T84 全变空行、全部文字压进 T85 一行。用户两次察觉（"少了那么多内容"→ 当时误判为 master/dev 分支差；"T84 怎么没有问题"→ 定位根因）。已把各文本拆回各行（`da059e0`），T1–T85 无缺号。**教训：改 issue.md 一律用带行尾/整行的锚点，或直接行级读写。**

### T86 assemble done 掩盖中断终审（T85 盲点）
- 用户实测：部署 T85 后重跑仍"phase assemble already done, skipping"——assemble done 标记正是失败运行（告警交付）写下的，续跑短路在 phase 层就跳过了，走不到 finalReview 的 resume。加 assembleNeedsResume 门：assemble 标 done 但中断态（state.partial.sha256）与当前工作区一致且完成态不成立 → 不跳过、重跑 assemble 续跑终审。测试 TestAssembleNeedsResume 钉四态（无状态/中断/完成/终审关闭）。

### T87 方块视图重复渲染（T83 的 Vue 条件链事故）
- 用户截图：方块视图下块网格和会话列表同时出现。根因：T83 把「展开全部」按钮插在 v-if="blockView" 的块容器与 v-else 的列表容器之间，v-else 失配后列表区无条件渲染。修法：块容器+按钮一起收进 <template v-if>，v-else 保持相邻。教训记档：v-if/v-else 之间不许插任何节点，插按钮要用 template 包裹同分支。
- 验证教训：headless 浏览器探针曾被僵尸 chromium（旧 profile、占同一调试端口）劫持——localStorage 里 side.blockView=1 + overflow 记忆让探针读到的全是旧状态；以后探针前必须确认调试端口上的是当前那个实例（job_list 核对）。

### T88 阶段状态映射 + 块编号
- fix/final-review 是 assemble 阶段内的会话，progress.json 无对应键 → progressKeyFor 落回原名查不到 → 永远「未开始」。映射到 assemble；测试钉 done/idle 两态。
- 块编号此前只认 imageOrder/chapterOrder，style-fix 与装配会话块全空；StageView 构建时按排序位置补 noBy（有号的照旧）。

### T64 重构：问题清单工具化 + goal 计划工具（用户重设计）
- 用户重设计：① 整合会话 submit 不再内联大 problems 数组（问题多时一次输出不全；且转录里那份参数 JSON 还会被模型在总结消息里复述一遍，看着像"重复提交"——实为正常复述，新设计下提交参数不再携带清单，观感问题消失）；② 清单不塞进样式/评估/分块会话提示词，改用 problems 工具（list/get/search/check）随用随查；③ 提交时代码核对勾选——样式会话全勾、分块会话勾本块；④ 新 goal 工具（plan.md 计划清单，增删改勾、有序、持久化，压缩后找回进度），提交前计划必须清空；⑤ 写权限只有整合会话有（add/update/delete，无 check），下游只读+打勾。
- 实现：ProblemStore（work/style_fix/problems.json，带锁——分块会话并行打勾）+ ProblemsTool（权限双模）+ GoalTool（plan.md 读写回环）+ goalGate；SubmitProblemsTool 改无参收账（读存储校验非空+字段齐），SubmitStyleTool 加 Gate 钩子（原样式会话不受影响），SubmitBlockFixTool 勾选核对从参数改为读存储（T66 checked_problem_ids 退役），T67 list.md 退役。提示词三模板 + 注册表 MustMention 同步。测试：TestProblemsTool（权限/增删改查勾/持久化/id 不复用）、TestGoalTool（有序/Pending/plan.md 回读）、TestSubmitProblemsTool/TestSubmitBlockFixRequiresAllChecked 重写为存储语义（含 goal 门）。

### plan.md 入册 + 提示词瘦身（用户口径澄清）
- 用户澄清：goal 工具的设计要求应记进**我们侧管理的 docs/plan.md**（计划档案，与 issue.md 分工，可从会话记录恢复）——runtime 存储用 JSON/md 皆可。plan.md 早已存在（P1–P26，旗标语义见 P17：第一旗 Agent 完成态 ~/X、第二旗开发者审核 T/F——T87 的 [X][T] 来源即此）；补记 P27（problems+goal 工具设计，已随 18dafac 实现）。
- 提示词瘦身（用户：能描述清楚即可，无非必要不加）：integrate 模板删掉 goal 说明与工具操作枚举（工具描述自带）；assess/block 去掉 op 枚举；Go 侧 userText 删掉"提交会被拒"的预告（打回回执本身点名缺失项）。AGENTS 一致性规则把 plan.md 与旗标语义入册。

### T89 mermaid 代码块控件同行
- 用户截图：代码块头部「复制」（带框按钮）与预览块自带一行的「源码」（裸文本钮）上下两行右浮、样式不一。修：mermaidPreviewBlock 接受 head 参数把切换钮挂进代码块头部（顺序 语言 | 源码/图表 | 复制），头部 .md-code-head button 样式天然同套；旧 .md-mermaid-bar 仅在无 head 时兜底。无真实 mermaid 转录可测，伪造单会话项目 + headless 探针实测：头部两钮同样式（1px border/8px radius/同高）、预览块无独立控件行。

### T90 撤掉侧栏会话聚合（用户纠偏）
- 用户问"这个聚合为什么要有？我没说过这个功能"——查实：T63 原文说的是终端进度行（"结束就消失这些实时的信息"），侧栏折叠是实现时的扩大解释。T79/T80/T82/T83/T87/T88/T89 一整串"会话看不到/控件难看"全源于此。撤掉：Sidebar stageViews 不再按 agg.rows 折叠，shown0=items 恒成立，超 8 条回老「更多会话」；方块视图直接铺满 items；agg 只留阶段头汇总文案（stageChapterAggregate 保留，rows 字段不再被侧栏消费）。干净 profile 实测四态全对（列表 3 行 / 36 章 8+更多→36 / 方块 36 块直铺）。
- 探针教训二连：① localStorage 按 origin 记忆——同一 profile 跑两轮探针，上一轮的 blockView/overflow 记忆会污染下一轮"初始状态"（probe10 误判源头）；② 每次探针前先核调试端口上的浏览器身份。

### T91 矢量图 3D 引导强化（真实证据驱动）
- 用户观察：几轮跑下来 3D 立体图不少、却没一个用 3D 包。实测取证：李艳芳_2 与某测试卷里 4 个标记为 3D 的矢量图，figure.tex 全只用了 usetikzlibrary{calc}——模型在手算斜二测投影。根因：提示词里 "3D scenes -> tikz-3dplot / perspective coordinates" 埋在 step 2 长句的括号里，压不住"描投影"的默认习惯。修法：点名真实 API（tdplotsetmaincoords + tdplot_main_coords 坐标系 / perspective 库）并明令禁止 calc 手投 3D。xelatex + tikz-3dplot 冒烟编译通过（texlive 2026 全量自带）。环境包清单：tikz-3dplot / perspective 库 / pgfplots / tkz-euclide 均可用；pst-solides3d 虽装但 PSTricks 走 dvips，与本管线不兼容，别用。

### T92 章节划分三连修 + T93 + P27[F]（真实证据驱动）
- **"Continue 空转"根因**：submit_split 收账后会话不终止，模型视任务已完成回**空消息** → T35 的空回复自救机制发 "Continue." 再烧一轮 → 模型再憋一句废话收尾。修：ToolResult 加 Terminal 字段，submit 类工具成功回执置位、Run 落回执后直接返回。覆盖 submit_split/SubmitChapterTool/SubmitDoneTool（checker/figure-check/assemble×2）/SubmitFigureTool/SubmitBlockFixTool/SubmitProblemsTool/mermaid 修复；SubmitStyleTool 不置位（提交后还有 test-compile，失败要打回续跑）。figure_check 测试同步改成"一次校验一个请求"。
- **"重复 json / 章节双倍"排查**：某测试卷 chapters 会话的 submit args 3204 字符、51 章区间连续无重复（书名"30+5+15"=50 套卷+1 引言=51，数字正确）；buffer.md 只是工作笔记。用户在 webui 看到的"重复"= 同一份清单在 edit_file(buffer) 与 submit args 各出现一次。修：submit_split 改读沙箱 split.json 独立文件（无参提交，path 可覆盖）；内联 chapters 参数保留但只在 replay 通道（旧转录续跑重放 args 用），工具定义不再暴露。提示词同步（chapters system/user）。
- **P27[F] 打回项**：goal 工具铺到 style 主会话 / 章节划分 / 章节转换（各带 plan.md + 提交门 goalGate；stylefix 各会话 T64 已有）。
- **T93**：convert 首条消息删掉章节前 2000 字符内联预览（CHAPTER_PREVIEW 占位符退役）——该 md 是会话必读输入，内联每轮重发。

### T92 续：split 结构化工具（用户补两条讨论结论）
- 用户指出此前讨论的是"要不要给 split.json 配 problems 同款编辑工具"，我跳步用了裸 write_file。补上：SplitTool（tools_chapter.go）ops=list/add/add_many/update/update_many/delete/clear，逐条或批量（全量预检、任一非法整批拒绝、批内自重叠也能查出——add_many 每插一条就重排序再验下一条）；用户补充"50 章不该 50 次调用"→ 批量 op 由此而来。
- 用户第二条："json 裸文件应该可以用 edit 编辑"——不加 write_file/edit_file 封锁，edit_file 直改保留为逃生舱；SplitTool 每次 Execute 先重读文件（loadLocked），外部直改立即可见，乱写 JSON 由 submit_split 终审拒绝兜底。沙箱预置空 split.json。提示词与注册表 MustMention 同步。测试：ops/批量/直改可见性三组。

### 三清单工具瘦身 + chapter_003 未提交采纳说明
- 用户口径：goal/problems/split 别那么复杂——add 统一单批量合一（单条字段 或 items/chapters 数组一次加多条），删掉独立的 add_many/update_many；之前模型建 50 条清单要刷 50 次调用。整批仍是全量预检、任一非法整批不动。
- chapter_003 "报错没提交却完成了"：不是 bug——模型编译干净后漏调 submit 直接收尾，runner 有意的兜底语义"未显式提交但编译通过 → 予以采纳"（convert-check 编译是硬关卡）。会话中报的错误是模型把文件名里的「熵」打成「熜」导致 view_pdf 找不到文件、以及 bash 在只读挂载 mkdir——都被工具干净拒绝，会话绕开后正常完成。
- 用户要求删某测试卷的 stylefix_integrate/stylefix_style_r1 转录（旧二进制产物，2 次 SUBMITTED + Continue 空转）：本机 samba 是只读挂载，删不了，已把确切路径交给用户手动删。

### T94 转换必须显式提交
- 用户口径："这边都要求必须要显式submit，没有的话提前结束就和正常的那个一样 Continue / 提示还没 submit"。删掉 convert 的静默兜底（未提交但编译通过→采纳），改为：Run 后未提交 → 提醒轮（sess.Run 续跑，文案指明"只有显式 submit 才算交"）最多两次 → 仍未提交判失败。与 checker（T46）/tikz 的提醒语义对齐。chapter_003 案例就此定性：模型把「熵」打成「熜」+ bash 只读挂载是过程噪音，真正问题是漏 submit 被兜底掩盖。

### T94 扩展：全会话显式提交 + T95 加密摘要拆行 + issue 重编号
- 用户口径"都应该是需要显式提交的其他会话也是"：抽 Runner.remindSubmit（提醒两轮→false 判失败），铺到 style/chapters（原来是硬失败无提醒）/stylefix 三会话（整合/风格/评估）/assemble 修复与终审。终审的"编译通过予以采纳"静默兜底删除（失败前回写中断态，续跑从现场接着要提交）。修复会话编译通过后同样要补 submit。
- issue.md 用户新条目与既有编号碰撞（T90/T91/T94 各撞一次）→ 重编号为 T95（加密摘要换行，已做）/T96（章节划分文件提交，已实现标 X）/T97（style 覆盖面检查，待办）。
- T95：encryptedSummaryBody（stream.ts）——相邻 **…** 段间补换行、段外短文独立成行，AssistantMsg 展开态用它；折叠态标题逻辑不变。vitest 123 全绿。
- 顺手修过时文案：整合会话 userText "add one by one" → 批量说明。

### goal check 批量（用户截图实测）
- 截图证据：整合会话 goal·check 连打 6 次单条（22→16 tokens/次纯浪费）。check 现收 {n} 或 {ns:[...]}，越界整次拒绝。problems check 本就收 {ids:[...]}；add 三工具都收数组——模型仍逐条 add 是它按读到的汇报顺序边读边加，工具描述里已写明批量形态，提示词只减不增不再加句。
- 截图里第一次 submit_problems 的 error 徽标 = goal 提交门拦截（还有没勾的计划），勾完后第二次提交成功——门按设计工作。

### 三清单工具批量面补齐
- 用户问"能提供多个的都是否正确提供了"→ 审计缺口：problems get/update/delete、goal update/delete、split delete 原本只收单条。全部补齐批量形（ids/ns/items/chapters 数组），批量全量预检、整批原子。测试：goal 批量 add/check、problems 批量 get/update/delete、split 批量 add/delete。

### T98 blocks 工具化 + T99 会话结局落盘 + 存储默认开
- 用户三问：①blocks.json 写入工具化→BlocksTool（同族：批量、每章一块当场拒、edit 逃生舱、submit 终审），评估会话接线+提示词+注册表同步；②终审中断无报错原因——实测转录 164 条消息停在 16:44 的 view_pdf 图片输出（第 63 轮），API/进程死亡从不进转录→AppendNote(run_end) + Run 包装 defer 落结局，web 轨迹单行「run_end · 会话结局」、详情栏「结局」行、endState 见到 run_end error 直接判 error；③keep_session_records 默认 false→true（显式 false 才清理），default.yaml/docs/config.md/代码注释同步，temp_keep 测试适配新默认。

### T100 续跑纯续行 + 终审中断实锤
- run_end 机制立刻见效：终审续跑 2 秒死于 `api error: [SESSION_INSUFFICIENT_BALANCE]`（余额不足）——第一次 16:44 的中断同类，当时没机制只能猜。用户需充值。
- 用户口径"续跑不需要插 user 内容"：resumeUserText 改为非空历史一律返回空（纯续行）。依据：回复中断时死掉的回复不落盘，转录末尾必是 user/tool；assistant 文本收尾 = 模型自己停了，"没提交"由 remindSubmit 两轮提醒兜底（T94 已铺全会话）。

### T101 终审失败原因上行 + 查价按条目名
- 用户 CLI 实测两问：①终审失败只显示"未通过"无原因——原因本来在 LogWarning 滚动行，用户盯的是定格阶段行→phaseNote 直接拼原因（截 160 字符）；②费用表从全配价变成大面积未配价——根因是用户在两次运行之间（15:36）把 gpt-5.6-luna 改名 gpt-6-luna，第一次运行的进程内存里还是旧配置所以 16:44 报表正常，18:11 新进程按新配置查价，1553 条 convert usage 行的厂商名查无此价。修：usage 行带 entry（models 条目名，clientFor 唯一设置点），SessionCost 查价 entry 优先、厂商名兜底；ModelPrices 条目名也作键。旧转录无 entry 字段行为不变（要重配价需给旧厂商名补一个条目）。

### T101 第二版：价格固化进转录（用户否掉 entry 方案）
- 用户定稿：按厂商名计价，且费率在会话写入时冻结进会话（t=meta kind=price：model+input/cached/output/currency，Run 开头写、换模型续跑再写一条）；聚合按模型分桶（ByModel），冻价优先、无冻结行退回当下配置——历史会话价格不随配置漂移。entry 机制（条目名查价）整体撤回。cost 测试改造 + 新增冻价胜出/分桶/部分无价三用例。

### T102 extends 嵌套 map 逐子键深合并
- 用户问 price 复写 input 一条会不会把其余三条打回默认——旧实现正是如此（条目键级整体替换，注释还把"永不半合并"当设计）。改为递归深合并：两侧都是 map 就逐子键合并（任意深度），null 清空支持深层，列表仍整体替换。测试：原两条"整块替换"语义测试翻转为深合并期望 + 新增 price 专项（撞上"同 wire 模型不同价"加载期保护，child 换 wire 名绕过——该保护在冻价时代仍留作旧转录兜底消歧）。

### T103 压缩摘要温度硬编码事故
- 终审死于 `compaction failed: invalid temperature: only 1 is allowed`：摘要请求写死 Temperature=0.1（求"确定的摘要"），>0 不会回落到条目配置，kimi-k2.8 网关只收 1 → 400 不可重试 → 会话失败。修：留 0 走条目配置。排查中发现 runner/verify 两处 Temperature: 0.0 本就是"用配置值"语义（<=0 回落），无需动。

### T104 view_image AltRoots（终审直看原位图）
- 用户要补"终审 view_image 看不到原书位图"的缺口：ViewImageTool 加 AltRoots（label+dir 只读备选根），resolve 在 Root 失败后按 clean / source+clean / source/images+去前缀 三形态试备选根，裸名也做唯一匹配；Root 优先、备选根越界拒绝。bookSessionTools 挂 {project, 窄视图根}，book_fix 与 final_review 同时获得。测试覆盖三写法/优先级/越界。

### T105 多页/多图参数调研（结论：不改）
- 用户问 view_pdf/view_image 要不要加多页/多图参数。真实数据统计（259 会话、4816 工具回合、1291 view 调用）：一轮并发多次看图 316 回合（42%），view_pdf 并发 862 次 ≈ 2.7 页/轮——模型已经靠 wire 层一轮多调用自然批量。加参数 = 同一能力的第二种表达，代价是回执归属 1:1 配平、FIFO、按文件预算全部要改。维持现状。

### T106 meta 快照污染回归
- 用户截图：详情栏"系统提示词"变"（这条 meta 行没有正文）/工具定义 0 个。根因：T99/T101 的 price/run_end 复用 t=meta，readStat 的 addMeta 不按 kind 过滤，最新 meta（price 行）把 system 快照覆盖成空。修：仅 kind=system（或无 kind 旧行）更新快照，Count 仍计全部。回归测试 TestMetaSnapshotIgnoresNonSystemKinds。

### T97 样式覆盖 + 首章试点（pilot）
- 真实产物核查（该书）：cls 已有 42 命令/环境但封面只有颜色无 titlepage 接口，封面图散进 chapter_001 当普通插图。
- 轻量档（提示词最小修正）：盘点清单补封面/前置页/重复结构；cls 必须为每种重复结构定义专门命令/环境；example.tex 复刻全书结构最密的真实一页。
- 重量档（latex.style_pilot 默认开）：chapters 后、convert 前插 pilot——convertOneChapter 试转首章（含 checker），产物铺进 work/style/pilot/（cls 副本同目录可编译），复用样式会话上下文（同 stylefix 复用模式）对照原书修订并再交 submit_style；修订后 example 必须编译过（一次打回机会）；cls/manual 指纹变了→作废首章产物（主转换重转），没变→保留（convertPhase 按产物跳过）。marker 文件 pilot.done 防重复/防失败死循环；任何失败只警告不挡主流程。转录 pilot_style.jsonl。

### T97 pilot v2 重构（用户定稿）
- 用户否定 v1（转真实第一章+复用样式会话）：试点应全部属于样式阶段内部——选材会话（小会话，同章节划分地位）挑全书代表性内容（md 行段+PDF 页，不论完整章节，产 spec.json）→ 拼 sample.md → 试转会话（convert 模型，对照选中页核对，汇报类缺口）→ 修订会话定稿 cls（**新上下文**，用户明确"无需复用之前的会话"）。之后才 chapters/convert。实现：pilot.go 重写，三个独立转录 style_pilot_{select,trial,revise}.jsonl；行段/页码越界裁剪警告；试转与修订各一次编译打回机会；marker 幂等。修 SubmitDoneTool 无 Gate/Set 字段的接线错误；MustMention 只收工具名（spec.json/sample.tex 不是工具）；buildPilotSample 漏 MkdirAll；测试 Runner 需要真 logger（nil log 在 LogWarning 崩溃）。

### T107 预览输出翻倍 + T108 试点模型注册与选材结构化
- T107 证据：转录里 split list 回执 text 字段只存一份（"16 章"出现 1 次）、add 输入 16 条 title 不重复——排除数据层。根因在前端：pullSession 无在飞锁，2s 轮询节拍内响应未回（samba 上 ReadSession 全量读）时第二个拉取以同一 from 发出，先发先落地、后发 seq 仍为最大值不被 pullSeq 守卫丢弃 → 同段行追加两次。修：pullInFlight 锁，reset 不受约束。
- T108：latex.pilot_select/trial/revise_model 三键（空回退 chapter/convert/style）+ models 注册 pilot-* 条目 extends 原角色；CurrentConfigVersion 12→13（两个模板+README 行+docs/config.md+测试夹具 temp/config_new.yaml 同步——后者是 ValidateFile 严格校验钉住的文件，版本不匹配会红）。选材改 pilot_spec 结构化工具（T108，同 split 家族：操作即校验、批量原子、每次 Execute 重读文件防 bash 绕过）。

### T109 无图片书籍中断
- 证据：用户终端——images done (0.0s) → style FAILED "source 目录中没有处理后的 markdown"。根因 rebuildPhase `if len(reps)==0 { continue }`，全书 0 图时 md 永不落盘 proj/source。双重陷阱：坏项目 book progress 里 images=done，续跑跳过 images 阶段，单修 rebuildPhase 也救不回来——RunBook 增加 style 前 ensureSourceMD 兜底补拷。

### T110 试点会话分组与排序
- style_pilot_* 三转录没进 LabelFor 的归类表 → 散「会话」桶；组内默认 mtime 降序，试点修订（最后写完）排在试点选材（最先执行）前。修：Go LabelFor/TitleFor 归 style 阶段并出中文点名（样式 · 试点选材/试转/修订），label_t62_test 加钉；web Sidebar.vue 的 style 组加 stylePilotOrder 执行顺序排序（主样式会话 0 → 选材 1 → 试转 2 → 修订 3），块视图编号随之按执行顺序编。

### T111–T112 该书档位1图片复用调查与续跑进度修复
- 本次仅只读检查 `运行目录`（不是 img2text），未动正在跑的项目：该书 md 388 张图/388 个路径，按原图文件字节 SHA-256 只有 130 组，其中 363 个引用对应 ≤60×70 的小图、这些小图仍有 105 个字节组。实际打开封面、截图、红色菱形 z 的多种裁图（包括 34×45 纯标识和截进蓝色题号 1/3 的 43×43、46×43 图）、数学 3D 图和交换图，确认不是 388 张相同语义的图；只有 z 主图元可复用，数字不可丢。相同字节的 29 张标识仍被判 raster 23/vector 6，证明现有路径级任务让分类不一致。T111 尚未实现：应在档位1 对高频视觉重复图元做候选聚合、确认共同图元与局部差异，绘制一次 + 逐处引用；低信心保留原图，不要直接按小尺寸/近似哈希并整张，也不应改正在跑的 progress_items。
- T112 根因：`classifyPhase` 在从 pending 过滤已分类记录前先创建进度行并记录「待分类 223 张」，续跑全部已分类时残留 `[classify 0/223]`。改成过滤后再创建 liveProgress、为零打印跳过，避免初始化不需要的 classifier；新增 LaTeX 专用回归测试，不改变 process 的分母和已存记录。issue 原 T98–105 落在 T110 后，已按条目整体排序 1–112；用户新补的两条由重复 T106/T107 顺延 T111/T112，原条目内容保留。

### T111 / P31 落地：视觉候选 → 独立核验 → 单图元多引用
- 只读真实书籍二次取样：早期仅检查白底、单连通的保守候选在 363 张小图中检出 **0**（扫描书离散笔画/JPEG 背景所致），不能假称已经去重。补充主色轮廓候选及额外蓝色像素过滤后，用可选 `DOCVISION_MOTIF_CASE` 只读测试得到最大候选组 **336/363**；候选仍不等于等价：旧实现曾逐批传原图给独立视觉 AI 会话辨别相同整张图，题号/多色细节不合并，核验失败原样独立处理。模型 `latex.dedupe_model` / `models.dedupe`（v13→14）；转录 source/sessions/motif_dedupe_*.jsonl 进预览独立阶段。
- 每个原图自己的进度 JSON 记录代表键、源/代表图摘要和核验状态；process 仅对代表图调用绘制会话，确认成品后才给从图落成品；source/figures/motif_*.tex 是裸 TikZ 片段（旧 tikz/*.tex 是完整 standalone 文档，不能直接 input），Markdown 保留每个原图 LINK，在 fenced latex 引用此片段；章节工作区与装配原来已转运 figures/，章节转换模板显式约束保留 input。无原书运行写入；兼容档位2原链。T112 根据用户反馈全分类续跑显示 N/N 100% done，**不添加「（续跑）」后缀**。

### T113–T114 整组去重工具会话与系统提示词元信息修复
- 根因与修正：旧 dedupe 把候选拆成每批10张、把图片直接作为请求附件、没有既有 read_file/grep/view_image/submit 工具和显式提交；现在每个完整候选组只建一个独立会话，挂载只读 `candidates/` 工作区，初始消息仅给 manifest，编号联系图按展示页生成，模型按需看图并由 submit 严格校验已看过的 1-based 成员，任何核验失败均保持独立。336张大组仅按联系图分页展示，不拆上下文。
- Web 侧 `metaState` 原先取最后任意 meta，price/run_end 会覆盖 system 快照；现在 Go/Vue 均只选 `kind=system` 或旧无 kind 行。dedupe 阶段从稳定转录名/标签归类并排在 vector 前；CLI 新增 `[dedupe]` 扫描、整组核验和失败计数进度。dedupe 模型默认回退 `text`，并通过 `LatexSession("dedupe")` 复用现有 drawing tuning。
- 验证证据：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 通过；`go build ./...` 通过；Web `npm test` 129 tests 通过，`npm run build` 通过；新增整组工具会话、336张联系图证据门禁和 system meta 回归测试。某项目按已确认无备份范围清除非 style 生成状态，保留 style、style 试点产物与 `source/images`，脚本指纹校验通过。

### P33 候选池与增量结构分组
- 相似度计算继续只负责初始高召回候选；`motifDedupePhase` 将所有初始候选桶汇总到一个候选池，再交给单一 AI 会话跨桶判断结构分组。AI 不再需要一次性提交几百个 ID。
- 新增 `motif_groups` 结构化工具和候选工作区 `groups.json`：支持 `list/add/update/delete/clear`，每次修改即时校验范围、重复 ID、跨组归属并原子保持旧状态；最终 `submit` 无参数读取 JSON，并对所有已分组成员执行看图证据门禁，失败关闭。

### P33 后续：整组核验显示与失败回退说明
- Web Go/Vue 阶段顺序保持 `dedupe → vector → style`，整组转录固定显示在第一个阶段“整组核验”，不再使用带哈希的逐图观感标题。
- dedupe 会话失败时明确记录“候选保持独立，process 将逐图处理候选图片”；只有显式提交且证据完整的 `equivalent_to` 才会让 process 跳过组内成员并复用代表图。

### T115–T117 / P33 收尾 / P36 全量整组核验与模型级能力限制（配置 v15）
- 真实失败复盘：该书档位1 的 `motif_dedupe_<hash>.jsonl` 末尾是 `t=meta kind=run_end ok=false`，错误为平台 `HTTP 400 invalid_media: too many images in request`——整组核验会话在提交前被打断，`groups.json` 从未生成，388 张全部退回逐图处理。实测当前平台 `qwen3.8-27b`：1/2/4/8 张图片均 200；9、10、12、16、32、64 张全部 400 `too many images in request`；`/models` 不返回 `context_length`/`max_input_tokens`，256K 上下文只能按用户提供的平台声明写配置。
- T116（用户指出的语义缺陷）：旧实现把相似度预筛当作准入条件，388 张里只有 352 张进入 AI，剩余 36 张即使结构相同也永远无法成组。现在 `motifDedupePhase` 的 pool 直接取**全部已分类样本**，本地候选只用于生成 `candidates:manifest.md` 的行内 `similarity_hint=group-N[:exact-pixels]` 与 `candidates:similarity_advisory.json`（`pool_size`/`initial_group`/`exact_content`/`class`/`source_hash`），最终分组完全由 AI 决定；最终组仍要求**全员 vector** 才建立共享（`motifGroupAllVector`），避免把 raster 组误判成可复用图元。回归测试 `TestMotifAdvisoryIsReferenceOnly` 钉住"未命中相似度的图片必须仍出现在全量清单里"。
- 整组核验会话新增沙箱 `bash`（只读挂载为 `/work`，`tools.python` 提供的解释器可用，可做批量统计/排序辅助判断），提示词与 `prompts.DedupeSystem` 的 `MustMention` 同步加入 `bash`。`prepareMotifViewWithAdvisory` 写 advisory JSON 时对缺失 `progress` 的候选做空值保护（真实空指针：`TestVerifyMotifCandidatesIndependentVisionSession` 直接 panic 在 `c.progress.Class`）。
- P36（配置 v15）：`models.<条目>` 新增 `context_limit` / `max_tokens` / `max_images`（全部 0=不限制，且**不跨条目继承**，避免某个视觉平台的图片上限污染其它模型）；`latex.sessions.<会话>` 声明会话需求，`NewSession` 通过 `ModelConfig.ApplyModelLimits` 取两侧较小正值。`maybeCompact` 的隐式 128K 兜底删除（未配置任何上下文即不压缩），`defaultSessionTuning` 不再强塞 `ContextLimit`。会话新增 `pruneModelImages()`：每次请求前若图片总数超过模型 `max_images`，把最旧的带图消息换成保留文本、提示可重新 `view_image` 的文字消息——与 `keep_images`（本地历史保留偏好）是两件事，会话层不新增 `max_images`。新增独立 `latex.sessions.dedupe` 调优块（不再借用 drawing）。
- T117（本次改动暴露的真实缺陷）：默认上下文改为不限制后 `TestCheckerRetriesAfterSessionFailure` 挂死 45s+。根因是"工具预算耗尽后补跑工具以保证 wire 合法"的分支没有上限——模型无视 `tool_choice: none` 时会话永不返回；此前一直被"上下文超阈值→压缩请求失败→会话报错结束"这条路径掩盖。现在最多补跑 2 轮，仍被要求调用工具即返回错误；新增 `TestToolCallsAfterBudgetAreBounded` 钉住请求数上限（预算 2 轮时总请求 ≤5）。
- 运行时配置与清理：`运行目录/config.yaml` 升到 v15，`models.text` 写 `context_limit: 262144` / `max_images: 8` / `max_tokens: 131072`，`models.dedupe` 改为 `extends: "text"`，新增 `latex.sessions.dedupe`（262144 / 131072 / 不限轮次 / `keep_images: -1`）。按要求清空 process 与 dedupe 状态：删除 `source/sessions/motif_dedupe_*` 与 `vector_*`（转录+`.work`）、清空 `build/ chapters/ out/ source/figures/ source/tikz/ work/temp|views|media|pdfview`，388 条图片进度回落 `status: classified` 并删除 `content`/`tikz_code`/`equivalent_to`/`dedupe_*` 等字段，`progress.json` 保持 `style: done`、其余阶段为空；`source/images/`（233 个文件）、`style/`（8 个文件）、`work/style_session.jsonl` 全部保留。
- 验证：`go build ./...` 通过；`GOCACHE=/tmp/dsh-gocache go test ./internal/... -timeout 800s` 全绿（含新增 `TestPruneModelImagesEnforcesProviderCap`、`TestToolCallsAfterBudgetAreBounded`、`TestMotifAdvisoryIsReferenceOnly`、`TestModelLimitsUseSmallestPositiveValue`）。

### T118 / P37 整组核验：去联系图、按桶分会话、每组必须有代表图
- 现场复盘（第二次真实运行，新二进制）：`motif_dedupe_e3d42047cb3a914b` 的 `groups.json` 最终是 `{"groups":[[1..388]]}` 一组；模型只调了 7 次 `view_image`（全是 `sheets/00N.png` 联系图），却通过了"每个成员都看过"的证据门禁；最终因"全员 vector"校验失败，388 张全部退回逐图处理——用户看到的"还在对所有的进行 process 处理"就是这个结果。同一轮里模型顺带看了 6 张原图，说明它并不会主动逐张核对。manifest 与 `similarity_advisory.json` 证明当时跑的确实是新代码（`# Complete image pool` + advisory），问题不在二进制而在设计。
- 用户定的口径（P37）：相似度只做**高召回聚类**，差异过大必须自成一组；每个分组必须由 AI 选出**最核心的代表图**，process 只重画代表图、组内其余位置全部替换为它的成品；**宁可漏（多画一张），绝不误判（合并即丢信息）**；允许整池一组，但同样必须给 key。
- 代码改动：① `prepareMotifViewWithAdvisory` 不再生成 `sheets/*.png`，删掉 `drawMotifID` 与拼图绘制；证据改为 `motifOriginalID` 解析 `originals/NNNNNN.*`，只有逐张未裁剪打开才算看过。② 相似桶**预填 `groups.json`**（`MotifGroup{Key, Members}`，key 留 0），`manifest.md` 按 `### bucket N` 分节列出成员、无提示的图单列"Images with no similarity hint"。③ `motif_groups` 支持 `{group:[ids],key}` / `{groups:[{members,key}]}` / `update {n,group,key}`，兼容旧 `[[ids]]`（无 key，不能提交）；`submit` 先跑 `validateMotifGroupShape`，任何组 key=0 直接打回。④ `motifScopes`：`latex.dedupe_group_session_over`（默认 10）以上的桶各开一个会话，其余小桶与未命中图按 `latex.dedupe_batch_images`（默认 40）打包成整理会话，`0` = 整池一会话（旧行为）。⑤ `applyMotifEquivalenceWithKey` 用 AI 的 key 当唯一 leader；`MotifSimilarTo` 兜底——与 key 相似度不达本地判据的成员即使被 AI 并入也保持独立并告警（先写过一版"逐像素一致"闸门，用户指出"同内容不同像素很常见"，改为复用探测器同一套相似度）。⑥ `clearUnresolvedMotifLinks`：代表图没产出成品时解除跟随并告警，避免跟随者被 process 永久跳过而图从书里消失。⑦ 配置 v16：`dedupe_group_session_over` / `dedupe_batch_images`。
- 提示词：`latex_dedupe.system.md` / `.user.md` 重写为"桶是起点、逐张打开原图、每组给 key、漏比错好、整池一组也要 key"。
- 验证：`go build ./...` 通过；`GOCACHE=/tmp/dsh-gocache go test ./internal/... -timeout 900s` 13/13 包全绿，新增 `TestVerifyMotifCandidatesIndependentVisionSession`（无联系图 + key）、`TestMotifPoolPrefillsBucketsWithoutContactSheets`、`TestMotifSubmitRequiresVisualEvidence`、`TestMotifScopesFollowBucketSize`、`TestMotifMergeGateRejectsDissimilarMembers`；配置夹具升到 `config_version: 16`。
- 第二轮修正（用户校准分配口径）：桶是**例外**而不是常态——"任何一个图大概率和其他图是没关系的"。所以①`dedupe_group_session_over` 默认 **50**（某桶 >50 张独占一个会话），`dedupe_batch_images` 默认 **150**（≤50 的小桶可以互相合并进同一会话，单会话上限 150，例如五六个小桶一起处理）；②未命中任何桶的图**不再塞进桶会话**，先算"未分配"，最后由**一个整理会话**统一归位，且该会话只比对各已确认组的**关键图**（`keys/000001.ext` 即某组代表图，把未分配图加进该 key 的组即可）；③整理会话不无条件开——代码先 `motifConsolidationCandidates` 用同一套本地相似度把未分配图与关键图配对，没有候选就完全不开（正常书里绝大多数图彼此无关，等于零开销）。实现：`motifScope` 增加 `Keys/KeyBase/Groups`，证据 id 支持 `keys/NNNNNN.ext`（`motifScopeID`）；新增 `motifBucketScopes`、`runMotifScope`、`motifConsolidationCandidates`、`motifAppliedGroup`，测试改为 `TestMotifBucketScopesFollowBucketSize` + `TestMotifConsolidationOnlyTakesLocalCandidates`。
- 真实运行复盘（第三次，20:31 起的续跑）：dedupe 只开了 2 个会话，一个 `1e6808206d35d6d7`（336 张、97 轮、350 次工具调用、11m39s）一个 `66cb7114f101df93`（14 轮），**没有汇总会话**，随后 `[process 16/388]` 逐图处理。根因不是证据门禁（用户明确要求"每个都要看，万一有一个不一样"——该门禁保留），而是**相似度的传递性**：`FindVisualMotifCandidates` 用并查集，A~B、B~C 会把 A、C 归到同一个桶，全书小图标链式相连后产生一个 **336 张的"桶"**；它超过 `dedupe_group_session_over` 所以独占一个会话，模型为了满足"每张都要亲眼看"只能把 336 张全看一遍，最后仍把 336 张交成一组（`groups.json` = `[{key:1, members:1..336}]`）——合并闸门按与 key 的相似度把 335 张全退回（同一轮里 `similarity_advisory.json` 也证明本会话只有 1 个桶），`safe<2` 于是整组不合并、`applied` 为空，汇总会话因此**没跑**（Web 上只有 2 个会话就是这个原因），process 只能 388 张逐图。
  修复：新增 `splitChainedMotifBuckets(groups, over)`——逐像素（`Exact`）桶可靠、保持完整；非逐像素桶若超过 `dedupe_group_session_over`，按 detectors 的簇内顺序切成 ≤50 的 seed 锚定子桶（尾部不足 2 张则留在未分配），避免一个桶吞掉半本书；同时补两条日志：组被闸门砍到 <2 张时明确说"整组不合并、其中 N 张保持独立"，以及"本次没有通过校验的分组，跳过整理会话"。测试 `TestSplitChainedMotifBuckets`（336 模糊桶切成 ≤50、120 张逐像素桶不切、成员不丢）。
- 第三次真实运行复盘（20:31 起，388 张）：dedupe 只开 2 个核验会话（336 张桶 + 4 个小组 16 张），**没有整理会话**，随后 `[process 16/388]` 逐图重画。日志给出唯一根因——`[dedupe] 组 [1..336] 含非矢量图，保持独立（只有全部为矢量图的组才复用同一张重画）`：该桶 classify 结果是 raster 159 / vector 155 / text 22，旧校验要求全员 vector，于是用户逐张看过、确认相同的 336 张被整组拒掉；会话 2 的 16 张同样被拒。352 张候选命中、36 张未分配，最终 `applied` 为空，整理会话按设计不开（Web 上只有 2 个会话就是这个原因），process 只能 388 张逐图。
  同日一度误判为"并查集传递性把 336 张并成一个假桶"并加了切桶逻辑（`splitChainedMotifBuckets`），用户指出 336 确实全是同一张图、桶没问题——**该改动已撤回**。按用户新规则改为 T119/P38：组类别取组内占比最大的 classify 结果，并列时 vector 优先；多数非矢量时只合并组内的 vector 成员（真实收益：那 155 张同图从画 155 次变成画 1 次），其余保持独立；相似度闸门与逐张看图证据不变。测试 `TestMotifGroupClassUsesMajority`。

### T120 / P39 两套 progress_items：dedupe 的核验标记写进了没人读的目录（2026-10-17 第三批收尾）
- 用户提问：「我们 program_items 里面不应该是 388 个图来着吗？怎么是 352 是啥意思？」「是不是后面的 process 对他们进行操作了？有修改过？」——现场是该书档位1 项目里**并存两个 `progress_items`**：项目根 352 条、`source/progress_items` 388 条。
- 证据（归档副本 `_记录_20261017_2031` 逐文件比对）：根目录 352 条全部 `dedupe_checked: true`，mtime 分布 **336 条 20:42 / 16 条 20:43**——正好是日志里两个核验会话的成员数（`核验会话 1/3：336 张`、`2/3：16 张`）；`source/progress_items` 388 条里 372 条 `dedupe_checked: false`（20:31 批量清标记）、16 条 `true`（20:43–20:44，被 process 实际重画的那些）。`根 ⊂ source`、差集 36 张 = 日志「36 张未分配图与任何已确认组都不相似，保持独立处理」——36 张没进核验会话，所以从来没写进根目录。**结论：不是 process 改的，是 dedupe 把标记写错了地方。**
- 根因：`runMotifScope` 用 `filepath.Join(filepath.Dir(outDir), "progress_items")` 定位进度目录。档位1 的 `outDir` 已经是 `<proj>/source`（`runner.go` 里 `progDir := filepath.Join(outDir, "progress_items")` 才是 classify/process 读写的那份），再取一层父目录就落到 `<proj>/`。同一次运行内等价关系照样生效（`prog` 是内存共享的 map，`applyMotifArtifacts` 直接看它），但**续跑时 `DedupeChecked` 全丢、整本书会被重新核验**，`preflightConvert` 读的又是 `source/progress_items`，两套目录从此不同步。
- 修复：新增 `motifProgressDir(outDir) = outDir/progress_items` 作为唯一来源，`runMotifScope` 的核验标记与 `applyMotifEquivalenceWithKey` 的等价关系都写回 process 真正读的目录；回归测试 `TestMotifProgressDirIsUnderSource`。
- 运行时纠正：从归档副本恢复 `source/progress_items` 的 388 条（保留 `class`/`label`/`class_reason`/`styled`/`style_note`，`dedupe_*`/`tikz_*`/`figure_*`/`content`/`original_kept` 清空、`status` 回落 `classified`），删掉项目根那份 352 条的错位目录；核对：388 条 ↔ md 引用 388 条零缺零多、`output/images/<书>/` 388 个文件在位，用户重跑时 classify 显示 388/388 已分类、不再重复烧一遍视觉请求。

### T121 / P40 + T122 / P41 整组核验能续跑、检测器特征每图只算一次（2026-10-17 第四批）
- 用户两问：「如果在 dedupe 中断的话，没办法继续之前的会话？就是和其他会话一样直接续上」「那个 388 的整组核验之前有个扫描的过程，会几十秒每次开始都要扫描一次吗？」
- **续跑（T121）**：`verifyMotifScope` 过去是纯"全新开跑"——`sess.Run(RunOptions{UserText: user})`，没有 `LoadTranscriptStats`/`SetMessages`/`SeedCounters`/`replaySubmit`/`resumeUserText`/`resumeLog`（对照 `tikz.go` 的完整写法）。三个断点：① 没有续跑接线；② 即使接线，证据 `seen` 是进程内 `map[int]bool`，重放历史不会重新附图，重放的 submit 必被 `REJECTED: image N has not been inspected` 打回；③ `prepareMotifScopeView` 每次覆写 `groups.json`，会把模型增量维护的分组抹回预填。修复：`motifViewTool` 新增 `dbPath`，看图成功且未裁剪即 `saveSeen()`（`seen.json`，排序后的 id 列表），启动 `loadMotifSeen()` 读回；`groups.json` 只在不存在时写预填；会话按其他阶段同款路径续跑（已 submit 的重放收账直接完成，未提交 `resumeUserText` 返回空串原样续行）。转录身份天然可续：`motif_dedupe_<sha8>.jsonl` 的 id 是成员集哈希，同一批图必然落到同一个文件。证据门禁与"每张都要亲眼看"没有放宽。新增 `TestPrepareMotifScopeViewKeepsResumedGroups`、`TestMotifViewToolPersistsEvidence`、`TestMotifSubmitAcceptsEvidenceFromDisk`。
- **续跑补洞（同批，真实数据暴露）**：部署时发现有正在跑的 `docvision latex`（旧二进制）持续 `view_image` 中 —— 那个进程**不会**写 `seen.json`。用户此刻中断重跑，新二进制回放这份转录时 `seen` 为空、`replaySubmit` 撞闸门 `REJECTED`、`resumeUserText` 又返回空串，模型再提交还是被拒：等于"能续跑"在跨版本场景下并不成立。修复：证据改为双源 —— `motifEvidenceFromMessages(msgs, view, keyOffset)` 从回放出来的工具回执里认未裁剪原图（`^Image (\S+) \(crop 0%,0%-100%,100%, width \d+px\) attached\.`，`keys/` 按 `keyOffset` 换算、裸文件名按工作区 `originals/`+`keys/` 在磁盘定号），与 `loadMotifSeen` 取并集后再 `saveSeen()` 回写；裁剪回执与 `INTERRUPTED:` 占位不产生证据。新增 `TestMotifEvidenceFromTranscriptReceipts`（未裁剪/裁剪/裸名/keys/占位 五类回执 + 用重建出的证据通过 `submit`）。
- **扫描成本（T122）**：真实 388 张图（某项目）实测 `decode=62.0ms detector=20.49s`，候选 5 组、`pixels=5.87M`、363 张 ≤70px（只有它们会走到特征计算）。瓶颈在 `FindVisualMotifCandidates` 的双循环：每对都重新 `motifFeature`/`chromaticMotifFeature`（逐像素扫最多 4 遍，含排序取主色），7.5 万对把同一张图的特征算了上千次。修复：新增 `motifFeatureSet{rgba, rgbaOK, visual, visualOK, chrom, chromOK}` + `computeMotifFeatureSet`，每张图算一次；两两比较改 `motifFeatureSetSimilar`（两份 visualOK 只看 visual，否则看 chrom，严格等价于旧 `visualMotifSimilar`）；精确组与 `isExact` 复用同一份指纹；对外 `MotifSimilarTo` 仍是 `motifFeatureSetSimilar || chromaticFeatureSimilar`（保持旧"detector 或 chrom 兜底"的超集语义）。实测 `detector=0.31s`（66×），`groups=5` 与成员不变。守卫测试 `TestFindVisualMotifCandidatesMatchesBruteForce` 在测试内重写一份"每对重建特征"的朴素参考实现（并查集 + 原样阈值），逐组比对 `Members`/`Exact`。
- 验证：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 14 个包全绿（latex 10.8s）。临时测量测试 `zz_motif_bench_test.go` 用完即删。
- **输出预算把窗口顶爆（同批，真实运行暴露，T123）**：用户重跑 `./docvision latex ./files/某测试书.pdf`（`logs/latex_20261017_221018.log`，`[dedupe:1e6808206d35d6d7]`）时，336 张图的核验会话从 22:10:39 跑到 22:28:01（18 分钟、350 次工具调用在逐个 view_image），随后 22:28:00 那一轮变成 `api error: [SESSION_API_ERROR: HTTP 400 (不可重试)] This model's maximum context length is 262144 tokens. However, you requested 131072 output tokens and your prompt contains at least 131073 input tokens`，会话判失败、`[dedupe] 核验会话失败，该批图片保持独立；process 将逐图处理`——336 张（全书 388 张的 87%）白看，之后 22:30 起连模板请求都变成 `[SESSION_INSUFFICIENT_BALANCE]` / `HTTP 429 quota exceeded`（账户余额耗尽，与本缺陷无关但同轮终止）。根因：厂商按 `prompt + max_tokens` 之和卡窗口，`max_tokens` 固定在配置（131072）、压缩阈值（0.85×262144≈223k）只管输入，于是输入一过 131072 就必爆，而这时本地估算/实测离压缩线还很远，程序什么都不做（不是限流、不是余额、不是重试策略）。修复：`sessionOutputBudget(maxOut, limit, prompt)` + `Session.outputBudget()`，每轮请求前把 `max_tokens` 压到「窗口 − 当前上下文 − 2%（至少 1024）」、下限 512，装得下就一点都不改，第一次让路时打 `[context] 输出预算 A → B：窗口 …、当前上下文 …`；上下文规模沿用压缩口径（本地估算 / 标定 / 厂商实测 `prompt_tokens` 三者最大）。新增 `TestOutputBudgetLeavesRoomForPrompt`（事故参数 131073→125829、无窗口/无预算不动、4k 窗口保底 512、0→262144 全量性质检查、会话级读取 `lastPromptTokens`）。`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 仍全绿（session 6.1s）。

### T124 / P43 逐字节相同的图只核验一次、只画一次（2026-10-17 第五批）

- **现象**：用户重跑时看到第一个会话（386 秒后）`[dedupe 3/3] done (failed: 1)`，而第二个会话把四个相似桶「都放一起」提交成功；用户判断「确实都一样」，并追问是否加了让人变谨慎的提示词。核对结论：提示词自 19:46 未改（`1ba6024`），20:25 那次"整组放一起"的提交其实被代码拒了（含非矢量图 → 0 合并），这次失败也不是谨慎，而是 22:28:00 的 400 爆窗（T123）。
- **真实结构（某项目，全部 388 条图片引用）**：按文件字节算 `sha256` 只有 **130 份不同内容**；逐字节重复 312/388，最大一类 29 份，其次 19/15/15/14/12/11/10/10/9/9/9。两个核验会话：`motif_dedupe_1e6808206d35d6d7` 336 文件 = **90 份内容**（296 张是副本），`motif_dedupe_66cb7114f101df93` 16 文件 = **4 份**（12 张副本）。按内容多数类分：vector 67 / raster 52 / text 11（引用数则是 188/172/28）。**图片文件名里那串 64 位十六进制不是内容哈希**——MinerU 按"图片引用"各发一个名字，同一张图出现 29 次就有 29 个不同名字（`03bd1aeff57887e7…`/`fe172f66b754434b…` 全是 1141 字节、sha256 `64dd12986bac7b4d…`），所以"哈希都不一样"是文件名造成的错觉；内容身份只能自己算（`motifCandidate.sourceHash` 早就是文件字节的 sha256，只是当时没拿它做池收缩）。
- **代价链**："每张都要亲眼看 + 未裁剪"把 336 张（其中 296 张是同一张图的副本）逼成近 300 次 `view_image`，18 分钟、350 次工具调用，最后叠加 T123 的输出预算问题爆在 400 上——同一张图被看 N 遍既没有任何信息增益，还把上下文熬干。
- **修复**：①`motifContentLeaders` 在候选池排序后按 `sourceHash` 收缩，只留 task key 最小的一张当代表图，其余进副本表；相似度检测、桶分配、预填、会话成员、转录身份全部基于收缩后的池（同一份内容特征相同，桶与成员不受影响）。②`motifAdoptCopies` 在核验与整理会话结束后补副本结论：代表图被合并 → 副本跟随同一 leader；代表图独立 → 副本跟随代表图本身，`dedupe_checked`/`dedupe_leader_hash`/`dedupe_source_hash` 一并落盘（字节相同就是同一张图，不是相似度推断）。③三处提前返回（全池一种内容 / 全部已核验 / 会话失败后没有分组）也走同一条补账路径，避免"没开会话就丢副本"。
- **保留不变**：`MotifSimilarTo` 合并闸门、逐张未裁剪证据门禁、组类别取多数（P38）、代表图无成品即解除跟随、相似度只作参考。没有新增提示词，没有新增配置项，`motifDedupeVersion` 不动（内容身份没变，变的是池的组成）。
- **实际数字**：会话 336→90、16→4，全书 388→130；vector 引用 188 → 67 份不同内容（即使完全不合并，最多也只作 67 次图，原先是 188 次）。
- **副作用**：转录身份 = 成员集哈希，成员变少就换文件名，`motif_dedupe_1e6808206d35d6d7` 那份死在 400 上的转录不再复用（它本来也终止在失败上）。
- 验证：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 14 包全绿（latex 9.7s）；新增 `TestMotifContentLeadersCollapseIdenticalBytes`、`TestMotifAdoptCopiesFollowsRepresentativeVerdict`。

### P44 相似度检测上不上 GPU 的评估（同批追问，2026-10-17 第五批）

- **用户提问**：「瓶颈是 O(N²) 里每对都重建两份像素特征，388 张约 7.5 万对这个可能可以用 GPU 进行一下并行加速？」并让排期。
- **前置事实**：这句话描述的是 T122 之前的代码；T122（P41）已把特征改成每图算一次（388 张 20.49s→0.31s），T124（P43）再把候选池按文件字节收缩。某项目本轮实测：**130 份不同内容，`decode=49.6ms detector=213.3ms`**（C(130,2)=8385 对，候选 1 组 90 张）。
- **结论（记在 `docs/plan.md` P44，含触发条件）**：不上 GPU。① 0.2s 不在关键路径（同阶段模型会话以分钟计）；② 工作量是 ≤70px 缩略图上的整数比较，一次 CUDA 初始化 + H2D 拷贝就超过全程耗时；③ 会给零依赖的 Go 单文件二进制硬塞 CUDA/驱动依赖；④ 复杂度在**对数**不在单对成本，GPU 不改 O(N²)。真的变慢时的顺序是 CPU 侧先砍对数（指纹分桶 / 尺寸与墨迹密度预筛 / 早退）再按 goroutine 分片；重新评估的触发条件写死为「检测 >2s 或不同内容数 >2000」。
- 验证：临时测量测试 `zz_reps_bench_test.go` 用完即删，无代码改动。

### T125 / P45 motif_groups 缺「组内增删」与「只换代表图」（2026-10-18 第六批）

- **用户观察**：「这个 motifgroup 的工具功能好像少了一点，比如从某个组中去除某个或者某些，或者向某组中添加某个或者某些？你看他们调用的，好像一直是清空再重写？」核对属实。
- **工具原有 op**：`list` / `add`（只能整组建）/ `update`（整组替换成员）/ `delete` / `clear`——**没有任何组内增删**，`update` 还必须连成员一起重发。
- **转录证据（三条独立会话）**：① 2031 会话 `motif_dedupe_1e6808206d35d6d7` 为了给一组 336 张的图指定 `key`，先发 `{"op":"update","n":1,"key":1}`，被 `REJECTED: update 需要 group 或 members。` 打回；下一轮只好发 `{"op":"update","n":1,"group":[1..336],"key":1}`，把 336 个 ID 原样重打一遍（约 1.3k tokens，且抄漏就是**静默掉成员**）。② 0016 会话 `clear` 之后用 90 次独立 `add` 一个个建组（每调用只放 1 个成员）。③ 本轮 live 会话 00:18–00:26 的调用序列是 `list` → `clear` → `list` → `add{group:[1..51],key:1}` → `clear` → `list`，连自己刚建的 51 张组也一起清掉了。clear+重写不是模型懒，是工具没给别的路。
- **修复**：`motif_groups` 新增两个 op 并放宽一个分支——`remove {n,ids:[...]}`（从第 n 组移出指定 ID，任一 ID 不在该组即整批拒绝；移走 key 则 key 清空并在回执里点名重新指定；移完不足 2 张则该组自动解散、剩余 ID 回到独立状态）、`append {n,ids:[...]}`（把 ID 加进第 n 组，已被别的组占用的 ID 整批拒绝并指出占用的组号，已在组内的算无变化不重复追加）、`update {n,key}`（只给 key 时**成员原样保留**，只换代表图）。三者与既有 op 一致：先整批校验、失败一个字都不写、成功立即落盘并回显全部分组。
- **提示词**：`latex_dedupe.user.md` 里那句 `motif_groups maintains groups.json (list/add/update/delete);` 的 op 括号是过期副本（工具描述才是权威），按「只减不增」删成 `motif_groups maintains groups.json;`（687→662 字节），没有新增任何指导。
- **测试**：新增 `TestMotifGroupsMemberLevelEdits` 覆盖 append 排序与幂等、跨组占用拒绝、remove 保留 key、只换 key 不动成员、移走 key 后回落未指定、整批原子性、剩 1 张自动解散。写测试时**先抓到一个真实缺陷**：解散分支只渲染结果、忘了 `saveLocked`，回执说"已解散"而文件没变——测试立刻红，补上落盘后转绿。
- 验证：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 14 包全绿（latex 13.2s、img2text 12.0s）。

### T126 / P46 0 组提交、clear 与进度行的沉默（2026-10-18 第七批）

- **用户观察**：「他都看了好多了然后最后还看了一下 list 为空还提交了？这个是为啥？」——新跑的会话看完了 90 张，然后清空分组、交回 0 组。
- **现场（转录 `motif_dedupe_ff7046e3467d7552`，390 行）**：`read_file(candidates:manifest.md)` → 90 次 `view_image`（全是未裁剪 `originals/NNNNNN.jpg`，证据一个不缺）→ `motif_groups list` → `clear`（00:18:40）→ `add{group:[1..51],key:1}`（00:22:46 → `OK. 1 个分组 key=1 [1..51]`）→ `clear`（00:23:45，正文写「A broad group was accidentally staged … finish with no merges unless exact identity is established」）→ `list`（00:26:42，空）→ `submit {}`（00:26:48 → `SUBMITTED.`）。工具计数：`read_file` 1、`view_image` 90、`motif_groups` 6、`bash` 1、`grep` 1、`submit` 1。运行日志只有 `[dedupe] 本次没有通过校验的分组，跳过整理会话；process 将逐图处理`，紧接着 `[process] 待处理 130 张`。
- **事实核对**：这 90 张就是同一个菱形 Z 标记，只差缩放/压扁/裁切。按本地探测器同一口径（std 归一化 64×64 灰度，MAD + union-find）复算：`thr=12 → 6 簇 [58,23,4,3,1,1]`、`thr=20 → 1 簇 90 张`、`thr=30 → 1 簇 90 张`。正确结果是 **1 组 90 张**（其中 vector 45 / raster 37 / text 8，按 P38 组类别取多数 → raster，实际只有 45 张 vector 成员共享一份重画）。模型不是没看，是**看了也判不出来**，而且它自己的推理是错的。
- **四条叠加的根因**：① 提示词把 `crop` 写进「任何可见差异都不能合并」，这 90 张正是同一标记的不同裁切；② T118 删掉联系图后没有任何并排比较手段，逐张看图的模型只能靠记忆对比上一张，而"换个比例看"是记忆最不可靠的地方；③ 预填的桶是**一个 90 成员、key 留空的组**，看着像"模型必须为它背书"，而当时 `clear` 是唯一能撤销的动作（T125 落地前没有组内增删），于是清空→重建 51 张→再清空；④ `submit {}` 与 `clear` 的回执都只有一个状态词，`[dedupe 2/2]` 又像"两个会话都跑完了"，130 张逐张重画的后果在日志里没有任何痕迹。
- **用户定的口径（三条方案）**：A 把后果写进回执/小结（做）、B 改提示词里 `crop` 那句（**不做**，原话「可见差异的话这个很难说，因为有的裁剪确实不能合并有的无效裁剪确实可以」）、C 并排比较图（做）。`latex_dedupe.system.md` 里那条 `crop` 规则一个字没动，只把「there are no contact sheets」那句补成「`compare_images` 可以把已看过的 2–4 张并排（不是证据），没有联系图」——这是新增工具必须同步 `MustMention` 的最小改动。
- **代码改动**：① 新文件 `internal/latex/motif_compare.go`：`motifCompareTool{root, files}`，`files` 由 `verifyMotifScope` 按 `originals/NNNNNN.<ext>`（成员）+ `keys/NNNNNN.<ext>`（关键图）拼出，**顺序与 id 完全一致**（`motifScopeID` 的 keyOffset 语义），所以 `motif_groups` 用的 id 就是比较图里标的 id。`Execute` 校验 2–4 个整数 id、越界/重复/非整数一律 `REJECTED`；每格 470×420 内等比缩放（`xdraw.CatmullRom`，小图放大不用 nearest，避免块状化被误读成"笔画不同"），格上 3× 放大标 id、灰框标出真实图幅，2 列布局，白底；回执给出每张 `宽×高px aspect x.xx` 与「每格都缩放到填满画框、图上大小不可比，只有这些数字可比」，并写明这不是证据。② `motifSubmitTool` 增加 `prefill` 字段：0 组时在 `SUBMITTED.` 后追加「0 组：本次一张都不合并，本会话 N 张图全部按独立处理」+（若预填过）「起始预填的 K 个相似桶同样被清空了，代码不会替你恢复」。③ `motif_groups` 的 `clear` 回执改成报数 + 点名 `remove {n,ids}`/`update {n,key}`，空表也照实说「本来就是空的」。④ `motif_dedupe.go` 在会话之后、副本处理之前打一行 `[dedupe] 小结：核验会话 N 个 + 整理会话 M 个；合并 K 组（X 张不同内容共用代表图，其余 Y 张独立重画）`。
- **测试**：新增 `TestMotifCompareImagesRendersPanes`（3 个 id → 2×2 画布尺寸、回执里的像素与长宽比）、`TestMotifCompareImagesRejectsBadIDs`（1 个/5 个/越界/重复/非整数全拒）、`TestMotifCompareImagesIsNotEvidence`（比过两张之后 submit 同一组仍 `REJECTED`，`seen` 依然为空）、`TestMotifSubmitZeroGroupsStatesConsequence`、`TestMotifGroupsClearReportsDropped`。
- 验证：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 14 包全绿（latex 21.6s、img2text 12.0s）；`go vet ./internal/latex/` 干净。

### T127 / P47 桶核验会话要有并发上限（2026-10-18 第八批）

- **用户观察**：「我如果有好多桶的话，前面这些桶就遵循我们之前的规则去并行？」——事实是**没有这条规则**：`motif_dedupe.go` 的桶循环是 `for si, sc := range scopes { r.runMotifScope(...) }`，`motif_dedupe.go`/`motif_session.go` 里连一个 `go func`/`WaitGroup` 都没有，`steps := len(scopes)+1` 只是把步数算成"桶数 + 1"，不代表并发。实测单会话 100 次工具调用、8 分 18 秒，多桶时耗时线性累加。
- **用户口径**：并行，默认上限 2，新增 `latex.dedupe_concurrency`，`0` 可退回串行。
- **代码改动**：① 新增 `runMotifScopes(scopes, limit, run)`：`limit<=1` 走原来的逐个串行，否则信号量限流 + `WaitGroup`；**结果按 scope 下标收回**（每个 goroutine 只写自己的槽位），完成先后不进入 `applied` 顺序——整理会话消费它，同样输入必须开出同样会话。② `runMotifScope` 去掉 `*int failed` 形参，改为返回 `(groups, ok)`；调用方用 `atomic.Int64` 统计失败与完成数，进度行按完成数推进。③ 整理会话保持屏障，仍是串行，并且失败时 `failed++`。④ 日志改成 `核验会话 k/M 开始：N 张图（B 个相似桶，并发上限 L）`。
- **配置语义（关键陷阱）**：`0` 是有效取值（串行），所以"没写"不能像别的键那样在 `setDefaults` 里按零值补默认——必须在**解码前的 YAML 节点层**判断 `latex.dedupe_concurrency` 是否存在（与 `models.*.extends` 同一条理由），缺席才填 2。配置版本 16 → 17，两个模板加键（`dedupe_concurrency: 2` + 注释），`docs/config.md` 补行。
- **测试夹具连带修复**：版本 bump 立刻让 5 个测试文件里写死的 `config_version: 16` 集体过期（`TestExtendsSameWireSamePriceOK`/`TestExtendsIsAKnownField`/`TestRunConfigRewriteStrictValidation`/`TestNoExtendsBehaviourUnchanged` 直接红）。现在夹具一律写 `{{cfgver}}`，由 `cfgVer()` 代入 `CurrentConfigVersion`；`writeConfig`/`writeThinkingCfg` 统一替换，直接走 `ValidateData` 的几处显式套 `cfgVer(...)`。`TestNoExtendsBehaviourUnchanged` 的参照路径补上 `ref.Latex.DedupeConcurrency = 2`（新默认不在 `setDefaults` 里，必须在参照路径显式镜像，测试里写明了原因）。
- **测试**：新增 `TestRunMotifScopesHonoursLimit`（6 个 scope/上限 2 → 峰值并发恰好 2、结果按序）、`TestRunMotifScopesSerialIsInOrder`（limit=1 → 峰值 1、启动顺序=scope 顺序）、`TestRunMotifScopesKeepsGoingAfterFailure`（单个失败只丢自己那格）、`TestRunMotifScopesEmpty`、`TestDedupeConcurrencyDefault`（缺席=2、显式 0=0、3=3、1=1）。
- 验证：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 14 包全绿（latex 15.9s、img2text 12.0s）；`go test ./internal/latex -run 'TestRunMotifScopes|TestMotif' -race` 干净；`go vet ./internal/latex/ ./internal/config/` 干净。

### T128 / P48 对话页要看得见「会话为什么停」（2026-10-18 第九批）

- **用户现场**：贴出轨迹页截图（`元信息 run_end · 会话结局 / 正常结束 / ✓ ok`）后问「有的报错信息为什么没有在这里显示？在网页上，每个会话如果是报错终止的我看不到，只能在那 jsonl 里面？」——同一条消息里还带着另一轮的 `[process ...] ^C` 与一串 `SESSION_INSUFFICIENT_BALANCE` 的 FAILED 行。
- **数据核查（先证伪"没落盘"）**：`curl /api/index` 实测这本书 162 个会话里 **44 个** 带 `runEndError`、`endState=error`；`source/sessions` 下 15 份 jsonl 是 `"ok":false`。抽查 `vector_...__<图注>.jsonl`：`meta system` → `meta price` → `msg user` → `meta run_end {ok:false, error:"api error: [SESSION_INSUFFICIENT_BALANCE]"}`，即"API 一上来就报错、一张图都没处理"的会话形态。
- **真正的缺口**：`web/src/legacy/stream.ts` 里有一句明确的设计——`// meta 行不是消息：它由详情栏的元信息块渲染，绝不进消息序列`（`if (line.t && line.t !== 'msg') return`）。`run_end` 是 meta，于是「对话」页（默认页）对这些会话**只显示一条「系统 · user 轮」**，看起来像个空会话。原因只存在于两处需要主动找的地方：轨迹页末行摘要 `错误终止：…`、右侧详情栏「会话」块的「结局」行。
- **修复**：`Timeline.vue` 在流式尾（`PartialTail`）之后加一条尾巴——`v-if="state.current && !state.current.live && state.current.runEndError"`，两行文案：固定标签「会话结局 · 错误终止」+ 原始原因；红左边框（`--err`）+ 8% 底色，原因用 `--mono`/`--code-font`/`--code-line`、`pre-wrap` + `break-word`（400/429 原始报文一个字不丢）。正常结束不占位、运行中不占位（错误是上一轮的旧账，Resume 成功会把 `RunEndError` 清空）。`legacy/types.ts` 的 `Session` 补 `runEndError?: string`（此前靠索引签名以 `unknown` 读数，`details.ts` 也是这么读的）。
- **没改的东西**：`streamModel()` 的"meta 不进消息流"保持原样（meta 仍归详情栏/轨迹，结局条是独立一段，不是把 meta 塞回消息流）；Go 侧一个字段没动（`RunEndError`/`endState` 早在 T99 就下发了）。
- **验证**：`npm run build` 重建 dist（74.01 kB CSS / 465.32 kB JS）；`npm test` 129 单测全绿；`go build ./...` 通过；`go test ./internal/sessionview/` 通过（15.8s）。**注意：embed 是编译期的，正在跑的 `docvision sessions` 进程要换二进制并重启才看得到新尾巴。**

### T129 / P49 组内合并改用「居中重合」判据（2026-10-18 第十批）

- **用户现场**（接 T126 那一轮）：模型提交 1 组 90 张、key=78，日志却是「组 [...] 里有 69 张与代表图相似度不足，已保持独立」，`小结：合并 1 组（21 张不同内容共用代表图，其余 109 张独立重画）`。用户先问"闸门是怎么构成的"，随后自己给出方案：「这些图应该大家一起放一起然后进行重合，肯定能弄出来一个最理想的图一起居中；如果大家都可以很好的符合就说明我们选的没有什么问题吧」「平移上限一点点向上调，一般用百分比，10/20/30/40/50 都可以试一下」「用 0.6，但要先对 388 张出一份报告和可视化验证」。
- **根因（几何，不是 bug）**：桶是 `MotifSimilarTo` 的**单链**连通（union-find），闸门却是"每张 vs 代表图"的**星形**——在 28×38 像素的小图上，内容框居中后中心差半个像素，硬 mask 的 IoU 就从 0.9 掉到 0.3（实测最差 0.344）。所以 90 张连通、20 张直接相似，闸门只留 21 张。
- **实现**：新增 `go/internal/latex/motif_align.go`：`motifAlignCoverage`（前景外接框 → 32×32 软覆盖网格，格值 = 格内前景面积占比；算墨阈值 R+G+B < 690，比 `foregroundMask` 的 720 紧——纸面 JPEG 噪声会把内容框撑到整张图，实测组内最差重合度 0.647→0.548）、`overlap`（±N 格平移下 Σmin/Σmax 的最大值）、`motifAlignShiftCells`（比例 → 格数，0..16）、`(*Runner).motifAlignFilter`（组中心 = 平均重合度最高的成员；≥ `latex.dedupe_align_min` 合并、其余独立；代表图优先模型 key，要求达标且面积 ≥ 合格成员最大面积 90%，否则取内容框最大者）、`(*Runner).motifMergeAllowed`（整理会话候选判定，同判据，大图退回 `MotifSimilarTo`）。闸门与整理候选都接到它上面；日志新增「居中重合校验：合并 N 张（最差重合度 0.xx），M 张不足保持独立」。
- **配置**：`latex.dedupe_align_min`（默认 0.60）、`latex.dedupe_align_shift`（默认 0.10，内容框边长比例）；**0 是有效取值**，所以"没写"仍在 YAML 节点层判定（与 `dedupe_concurrency`/`extends` 同理由）。版本 17 → 18，两个模板 + `docs/config.md` + README 同步。
- **可视化验证（388 条引用 → 130 份不同内容）**：软覆盖 + ±3 格下，100 张同标记构成一个连通片（90 张会话内 + **10 张落在"未分配"里**），对齐后取平均是一枚干净的菱形 Z（`overlay_mean_100.png`）；会话外那 10 张 `extra_members.png` 肉眼就是同一枚（0.71–0.93）；最差 10 张红蓝合成 `worst_pairs.png` 只有一两像素描边错位（0.647–0.708）；其余簇是 5 张暗色装饰图 / 3 张波形图 / 2 张 3D 坐标图 / 2 张交换图（`other_clusters.png`），没有出现误并。证据与逐张分数在 `temp/motif_overlap/`（不进仓库）。
- **参数扫描**：平移上限 10%/20%/30%/40%/50% 结论**完全一致**（连通片 100、最差 0.647、组外最高 0.494），只有比较次数从 49 涨到 1089；0%（不搜平移）最差跌到 0.439。阈值 0.5→102 张（多并入 `1fd979`/`88d285` 那一对）、0.6/0.7/0.75→100、0.8→95。
- **Go 实现与 Python 的偏差（真实发现）**：同一批图，Go 侧 center=`<sha2>`（与 Python 的中心一致）、中位 0.898、最差 0.602；Python 侧最差 0.647。差在算墨阈值与 JPEG 解码的边界像素——阈值 705 时最差掉到 0.548（低于门槛，会多丢一张），改成 690 后回到 0.602。**最差那张正好压在 0.60 边缘**，注释与 issue 里都写明：代价是多画一张，不会丢内容。
- **测试**：`TestMotifAlignCoverageNormalizesScaleAndPosition`（同一标记半尺寸 + 移位 ≥0.75，不搜平移时明显更差）、`TestMotifAlignOverlapRejectsDifferentShape`（实心块 <0.60）、`TestMotifAlignShiftCells`（0/0.01/0.10/0.25/0.50/0.90）、`TestMotifAlignFilterKeepsAlignedDropsOthers`（并进异形图要被拒、代表图取内容最全者）、`TestMotifAlignFilterFallsBackWithoutCoverage`（1×1 图退回逐对判据）。合成夹具的笔画粗细随半径等比——固定 1px 线条放大 2× 后相对变粗，第一版夹具因此只有 0.279（人造差异，不是判据问题）。
- **验证**：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 14 包全绿（latex 16.6s、img2text 12.0s、sessionview 16.6s）；`go vet ./internal/latex/ ./internal/config/` 干净。

### T130 / P50 相似桶也用「居中重合」判据（2026-10-18 第十批续）

- **用户追问**：看完 300 dpi 的 100 张总览后问「不是用全部 300 的图做的吗？这样的话感觉比之前那个相似度的弄起来更加准确和可控？」——事实是聚类确实在全部 130 份不同内容（= 388 条引用）上做的，但**桶**（谁进同一个会话）仍走旧的单链 `MotifSimilarTo`，于是桶里只有 90 张、**另外 10 张同标记图落在"未分配"**；闸门却是新判据。用户选「桶也改用居中重合（限小图标）」。
- **实现**：`FindVisualMotifCandidatesAligned(samples, alignMin, shiftPct)` —— 逐对相似度之后，再对"拿得到覆盖率的小图标"（`motifAlignCoverage`：前景外接框 ≤ 200×200、整图 ≤ 4M 像素）做一轮 union：重合度 ≥ `dedupe_align_min` 且 **墨迹占比差 ≤ 0.12、长宽比差 ≤ 0.35**（两处廉价预筛，挡掉绝大多数不可能同源的配对）就并进同一个桶。`alignMin <= 0` 退回旧入口；参与比较的小图标 **> 400 张**时整条判据退回旧行为并返回 `-1`，dedupe 阶段打一行 `[WARNING] 小图标超过 400 张，桶退回逐对相似度判据（居中重合太慢）`。
- **实测（该书 130 份内容，含 30 张负样本）**：旧判据 **1 组 `[90]`**，用时 184ms；新判据 **5 组 `[100 3 2 2 2]`**（额外并进 15 对），**最大组 100 张里混入非本组 0 张**，用时 1.36s。3 张/2 张的小组正是波形图、3D 坐标图、交换图那几个簇，与 Python 侧的聚类一致。
- **为什么只限小图标**：32 格软覆盖对稀疏图形没有鉴别力——跨书负样本（其它书的插图）与这个中心的最佳重合度实测 0.72–0.80，如果对任意大图用 0.6 门槛会直接误并；图标类（≤200px 内容框）才在"同源/异源"之间有 0.6 附近的间隔。
- **测试**：`TestFindVisualMotifCandidatesAlignedLinksSquashedVariant`（水平压扁 28% 的同标记：`|Δaspect| > 0.18` 让旧判据拒掉、居中重合并进来；结构不同的方块仍分开；夹具若恰好能过旧判据则 `t.Skip` 说明原因）、`TestFindVisualMotifCandidatesAlignedDisabledMatchesLegacy`。
- **配置**：没有新增键——复用 `latex.dedupe_align_min` / `latex.dedupe_align_shift`（T129 已进配置 v18）。
- 验证：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 14 包全绿（latex 11.0s、img2text 12.0s、sessionview 16.7s）。

### T131 / P51 小图标本地自动归组 + 代表图取"典型里最全"（2026-10-18 第十批续二）

- **用户追问**：看完 100 张 300dpi 总览后问「旧的相似度能否更宽泛一点，尽可能多的把其他的纳入一个桶，然后按照我们 0.6 判据给聚合最终的？这样的话小插图就不用我们的这个 dedupe 的会话了，只有大一点的用这个会话。是这样不？还有就是最标准的覆盖范围全的那个 key 你是怎么选的？应该是选 1.0 的那个作为我们这个组的中心 key」——同时点出「我们这个桶不是才 90 个吗还漏了 10 个超过 60 分的内容？」（那是上一轮旧二进制留下的 `manifest.md`/`groups.json`；T130 已把桶改成 100，见该条）。
- **事实澄清**：桶（T130）确实是"宽召回 + 0.6 收口"，但**所有已分类图仍然全部进会话**——"小图不开会话"没有实现；大图这条路不能放宽（0.6 软覆盖对稀疏图形没鉴别力，跨书负样本 0.72–0.80）。
- **用户选定**：① 「≥0.75 自动定组，0.60–0.75 进会话」；② 代表图「分数 ≥ 0.95 的成员里挑内容框最大」。
- **实现**：新增配置 `latex.dedupe_icon_auto`（未写时 0.75，`0` = 关闭，配置 v19）。`(*Runner).motifAutoIconGroups` 在桶分配之前运行：只取**矢量**图（非矢量不重画，合并只会多一次丢内容的机会）、内容框 ≤ `motifAutoIconMaxArea = 20000` 像素（约 141×141；再大就是插图，自动合并风险不对等）；按 ≥ 阈值连通成片，片内先算中心（平均重合度最高）再在"典型成员"（≥ `motifAlignTypical = 0.95`）里取内容框最大的当代表图，逐成员对代表图复核，**对代表图不达标的成员退回池子交会话**；达标的直接 `applyMotifEquivalenceWithKey` 落盘（`DedupeChecked` 一并置真，续跑走 allChecked 快路径）。自动组也进 `applied`，所以整理会话会把"0.60–0.75 的那批"按 0.6 挂回同一个代表图；`motifAdoptCopies` 仍按完整 leaders 对账，逐字节副本照旧跟随。
- **两处实测发现**：① 一开始按"种子星形"成组（每张与第一张比），100 张同标记被切成 79/11/5/4 四组、等于多画 3 张——改成连通片 + 代表图复核后是 1 组 86 张；② 不设面积上限时，两张内容框 68335 像素的相似插图被自动并成一组（`9c206fd` 那对），加上 20000 上限后它们回到会话。
- **代表图规则（T131 用户口径）**：`motifAlignFilter` 先看"典型成员"（与组中心 ≥ 0.95）里内容框最大的那张；没有这样的成员才退回旧顺序（模型 key 达标且够全 → 合格里最大框 → 组中心）。实测该书：原规则选 `<sha1>`（0.707、框 1102），新规则选 `<sha2>`（1.000、框 1064）——正是用户说的"1.0 的那个作中心"。
- **测试**：`TestMotifAutoIconGroupsOnlyHighOverlapVectors`（0.60–0.75 的压扁版必须留在池子——夹具的压缩系数由度量自己搜出来，避免夹具与阈值脱钩；非矢量与超过面积上限的图不自动合并；`dedupe_icon_auto: 0` 时全部交会话）、`TestMotifAlignFilterPrefersTypicalLargestMember`（内容框最大但形状不同的实心块不许当代表图）。
- 验证：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 14 包全绿（config 0.08s、latex 14.1s）。

### T132 / P52 + T133 / P53 并排比较图算证据、dedupe 结果落盘（2026-10-18 第十一批）

- **T132**：用户口径「四个合并一起的也算看过吧，对于模型来说」。并排的 2–4 张都是完整原图（等比缩放、无裁剪），所以 `compare_images` 现在把并排的每个 id 记进 `seen`、满足 `submit` 的逐张门禁，并与 `view_image` 共用 `saveMotifSeen`/`seen.json`；提示词里那句 "it is not evidence" 改成"并排看等于逐张看过"。上限仍是 4 张、id 由模型点名，不提供整桶联系图（T118）。
- **T133**：用户问「dedupe 的中间结果（重合度扫描）没有输出到某个目录吗？续跑还是要再过一遍，而且每次都不一样」。排查出三条：① 规则改动会让进会话的池子变化，而会话身份是"成员集哈希"，所以 11:15 的 `ff7046e3` 与 11:28 的 `c3cdddf1`/`eadf1649` 是两套会话（11:28 换的是带 T130 桶 + T131 自动归组的二进制，池子 130→77）；11:36/11:37 都是 `轮次 0`（`dedupe_checked` 全真，allChecked 快路径）。② classify 数字 388→364→360 不是丢图（md 引用、图片文件、progress 都还是 388），只是"本次还要处理"的张数在变小。③ 结论只落在会话工作区与每图 progress 里，桶与分数不落盘。
- 实现：`<proj>/source/dedupe/groups.json` + `report.md`，带"输入 + 规则"指纹；写盘失败只警告。测试 `TestWriteMotifArtifactsRecordsFinalGroups`（组/成员/副本/独立/指纹/报告文案）、`TestMotifArtifactFingerprintStable`（顺序无关、规则变则变）。
- 验证：`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 13 包全绿。

### T135 / T138 / T139 dedupe 判据定稿 + 未进会话图挂靠（2026-10-18 第十二批）

- **判据（T138）**：软墨量 `w = 1−通道/255`（不二值化）+ RGB 三通道平均 Σmin/Σmax；画布 `S = clamp(P90(内容框长边), 24, 512)` 按池自适应；内容框**居中贴进 S×S 且不缩放**；网格 `G = 32, 64, …` 逐级翻倍到放得进 S 的最细一级，粗层只把平移搜到位，**判定分取最终（最细）那一级**（用户：「判定层就是逐级搜索的最终结果，不要固定一个 G=多少的层」）。删掉旧的固定 32×32 + 归一化缩放：缩放会抹平尺寸差（同图缩到 0.72× 也能并），细网格才能分开只差一个符号的交换图（内容框 171×390：G=32 得 0.765、G=256 得 0.373）。
- **配置（T138/T139）**：新增 `latex.dedupe_align_grid`（`32` 固定格数或 `0.95S` 画布比例，表示最细一级，`ParseAlignGrid` 夹在 `[8,1024]`）、`dedupe_align_typical`（0.95）、`dedupe_attach_min`（0.60）、`dedupe_icon_max_area`（200000 px²）、`dedupe_bucket_max`（400）；`CurrentConfigVersion` → 20，两份模板与 README/`docs/config.md` 同步。
- **挂靠（T135/T139）**：`motifAttachLeftovers` 在收尾前把未进任何核验会话、`DedupeChecked=false`、无 `equivalent_to`、内容框 ≤ `dedupe_icon_max_area` 的图逐张与已确认组的关键图比居中重合度，最高且 ≥ `dedupe_attach_min` 的组收编，产物标 `source=attached`；每张最多一个锚点（同分按 `applied` 顺序）故不成环，会话结论不覆盖。
- **诊断（T139）**：`pairs.csv` 增 `align_level`/`canvas`/`skip`（`too_large`/`no_content_box`）三列，能直接看出这对用了哪一级网格、多大画布、为什么没走判据。
- **真实数据（该书 130 份不同内容，T139 复测）**：同标记 100 张两两 4950 对 min 0.462 / p10 0.542 / 中位 0.610 / ≥0.60 2851 对 / ≥0.75 232 对（对比 T137 缩放版：中位 0.760、96/100 ≥0.60）；30 张不同内容 435 对 min 0.002 / 中位 0.048 / **max 0.607**；A/B 交换对 0.378。结论：不缩放后同标记分数整体下移，`0.75` 自动档只吃约 4.7% 的对，主力是 0.60–0.75 核验会话 + `attached` 挂靠，且 0.60 门槛正压在不同内容的最大值上——所以自动路径继续由 0.75 把守、误并留在会话里可复核。
- **测试**：重写 4 个依赖缩放不变性的旧 fixture（squash 变体现在明确不并，跨尺寸交给会话/挂靠），新增 S 自适应/位置偏移/回退/挂靠等用例；`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 13 包全绿，`go build ./...` 干净。

### T140 dedupe 扫描提速：内容去重 + 并行判定（2026-10-18 第十三批）

- **问题**：用户看到 `[dedupe] 扫描候选 388/388 · 已用 30s`，问「扫描候选很慢吗？不是在相似度分组之后就是分完桶之后桶内扫描的吗做那个重合度的？」。答案是否——`findAlignedMotifPairs` 跑在**分桶之前**，是一次覆盖全部小图标的全池 O(n²)；桶是并查集的结果，不存在"桶内扫重合度"。这里必须这样：T130 的动机就是逐对相似度把一张连通片切成两半，若把重合度限制在相似度桶内，跨桶的同标记图就永远补不回来。
- **实测拆解（该书 388 条引用，16 核开发机）**：覆盖率 191ms、逐对相似度 0.30s、重合度扫描 **104s**（用户机 30s）。面积闸门后 72390 对、墨迹/长宽比预筛后 68047 对；单对 ~1.9ms，其中**平均 314 次重叠度调用**（单次 6.1µs）——0 级窗口 ±3 格只有 49 次，其余全来自"解顶到窗口边缘就翻倍重搜"（3→6→12→24 格，最远 ±75% S）。
- **修法（判据一条没动）**：① 逐字节副本只算一份（388 引用 → 130 份不同内容，`rgbaFingerprint` 相同结论必然相同，副本已被指纹并进代表所在桶）；② 已在同一桶里的对跳过；③ 达标与否是纯函数，按 CPU 并行算（`motifAlignWorkers = 8`）再按原顺序串行 `union`，合并顺序与并发前一致。**104s → 3.5s**，分组与合并数逐字不变（2 组 / 13 次合并）。
- **没做的事**：把 0 级扩窗夹回 `dedupe_align_shift` 的 ±10% 预算能再快约 6×，但实测 8385 对里 1698 对分数变化（最大 +0.416）、7 对跌破 0.60——那是改判据，校准要重跑，单独一批再说。网格缓存（每图每级前缀和）只占单对成本 ~2%，不值得加复杂度。
- **测试**：新增 `TestFindVisualMotifCandidatesAlignedCopiesStableAcrossRuns`（三份副本 × 三组内容、并行扫描重跑五遍，分组必须逐字一致）；`GOCACHE=/tmp/dsh-gocache go test ./internal/...` 13 包全绿。

### T141–T144 dedupe 收口：复用前置、会话作用域、窗口收窄、全流程文档（2026-10-18 第十四批）

- **T141 续跑先只算哈希**：指纹早写进 `source/dedupe/groups.json`，但 dedupe 仍要解码全池、算完覆盖率与全池重合度扫描（30s 级）才走到 `allChecked` 快路径——续跑开销与真跑几乎一样。现在阶段最前面加一次只算 `sha256`、不解码的预扫（`motifHashedDedupe{Key,Hash}`），按 `v=…;min=…;shift=…;auto=…` + 排序去重后的哈希集算指纹，再逐项校验 `groups.json` 的 `version`/`refs` 条数/指纹/每图结论覆盖是否完整；全对就沿用并打印「输入与规则未变（指纹 …），沿用 …」，任一不符 → 全量重扫（宁可重算，不能不认账）。新增 `TestMotifReuseDedupeValidatesConclusions`（换规则、少一份内容、结论不全、版本不符都必须重算）。
- **T142 会话作用域按原始桶大小**：`motifBucketScopes` 原先用**自动定组之后**的剩余成员数判 `width(g) > dedupe_group_session_over(50)`，该书那个 102 份的桶被 ≥0.75 拿走 78 张后只剩 24 张，于是和 2 张的小桶合进同一会话，本该出现的第二个会话消失（`[dedupe 1/1]` 看着像只核验一次）。现在自动定组**之前**先数一遍 `bucketFull`，`motifBucketScopes(pool, advisory, full)` 用原始大小判，大桶永远独占（剩余 0 就整桶跳过，不开空会话），`full == nil` 保留旧行为供单测与旧调用。实测核验会话 1 个混批 → 2 个（大桶 24 独占 + 小桶 2）。新增 `TestMotifBucketScopeUsesOriginalSize`（原始 60 剩 20 仍独占；原始 6 剩 6 才进批次）。
- **T143 细层窗口收窄（用户纠正）**：用户：「之前说的是每次翻倍变长之后搜最中心的点是在上一层选中的格子中 的 +-2 的格子搜的，就是四个格子吧？我看你写的怎么变成每次翻倍搜索了？」核对属实——0 级全窗之后细层中心确实是「上层解 ×2」，但代码还留着"最优解落在窗口边缘就把窗口翻倍重搜（最多 4 次）"的兜底，那不是约定的算法，正是 T140 里实测的成本主因（单对平均 314 次重叠度计算，0 级只有 49 次）。现在每级**只扫一遍** `win = motifCanonRefine(2)`、中心 = 上层解 ×2，删掉扩窗循环与 `atEdge`；0 级仍是 `round(shiftPct·G)` 全窗（平移预算来自 `dedupe_align_shift`，不属于扩窗）。
- **T143 实测（该书 130 份不同内容 / 7503 对可算分数的对）**：7458 对分数变化，**最大降幅 0.416**（升的最多 +0.014，旧值取自 13:50 的 `pairs.csv`，可能混有更早一版二进制的口径；窗口变小不可能让分数升高）；跌破 0.60 的 40 对、跌破 0.75 的 **0 对**；桶 `[102,2]` → `[100,2]`（少 2 条连回大桶的边）；≥0.75 自动组**逐组不变**（17,11,10,6,6,5,5,5,4,4,3,3,3,2,2,2,2,2）；生产入口 `FindVisualMotifCandidatesAligned` 合并 13 → 11 次、仍 2 组，全池扫描 3.48–3.53s → **2.27s**（约 -35%）。代价是 2 张从大桶落进未分配，由挂靠/会话兜住。
- **T144 全流程文档**：用户要「完整的详细的、从最开始到最终结束的完整过程的详细报告包括里面的算法详细步骤」→ 新增 `docs/dedupe-algorithm.md`（12 步全景、逐步判据与数据结构、居中重合度的算法步骤、阈值总表、实测数字、已知不完美），`docs/dedupe.md`/`docs/latex.md`/`README.md` 各加一行指过去；`docs/dedupe.md` 里 T140 那段"扩窗能再快 6× 但会改判据、未做"已过时，换成 T143 的实测结果。
- **T144 顺带核出的一处代码/文档不一致（未改代码）**：文档写画布 `S` 按池 `clamp(P90(内容框长边), 24, 512)`，`motifCanonCanvas` 也实现了并有单测（P90 用例 62/98），但**生产代码没有调用它**——`motifAlignImage.overlap/overlapDetail` 实际按**每一对**取 `max(a.maxSide, b.maxSide)` 再夹 24–512。本书图标尺寸集中（两种口径都落在 41 附近）所以指标没暴露差异；跨尺寸池子上会有系统性差别。留待用户定：把 `motifCanonCanvas` 接进生产（按池算一次、逐对复用），还是把文档改成实际口径。
- **测试**：`GOCACHE=/tmp/dsh-gocache go build ./...` 干净；`GOCACHE=/tmp/dsh-gocache go test ./internal/... -count=1` 13 包全绿（latex 11.9s、sessionview 16.6s）；`gofmt -l` 对改动的 4 个 Go 文件无输出。

### T145–T149 dedupe 判据纠偏：class 退出合并、复用闸门修通、组间收口（2026-10-18 第十五批）

- **T147 class 又参与合并判决了（用户当场纠正）**：用户：「我们不是说了不看 class 跟 class 没有关系……这一个组如果选好了，这个组就是认为同一张图了，然后按照之前判断的 class 类型选择其中占比最大的就给这个组定上类型（准确说是中心图）……我看你说的 AI 判 22 张同组，但多数 class 是 raster → 只并 7 张 vector。class 竟然还是参与判决了，我的天这都是多久之前说过的了！！！他不参与不参与不影响合并，就当他不存在。」核对属实：`motifAutoIconGroups`（自动定组）从 T134 起已不按 class 过滤，但**会话复核**那条路还留着旧分支——`runMotifScope` 收账时若组内多数不是 `vector`，就把该组交给"只合并组内 vector 成员"的旧逻辑（`motifVectorSubset`/`motifGroupAllVector`）。那次运行：一个 22 张的会话组被判成"多数 raster"，最终只并了 7 张矢量图、其余 15 张打回独立（日志原话「组 [...] 的多数类别是 raster，其中 7 张矢量图合并为一张重画，其余 15 张保持独立」），而其中一张被标成 raster 的成员对该组代表图的重合度是 **1.000**（逐字节同图，只是 classify 给了不同类别）。
- **T147 修法**：删掉整个 class 分支与 `motifVectorSubset`/`motifGroupAllVector`；两条路（自动定组、会话复核）统一成"整组按同一张图合并"，`motifGroupClass(cands)`（组内占比最大的 class）只用来给**中心图**定类型（`motifApplyGroupClass`，日志「中心图按组内占比最大的类别定为 …」），代码注释写清"class 不得作为合并闸门"的原因：同一张图在不同页、不同次 classify 会给出不同类别，混类不代表不是同一张图。预期整书重画数 64 → 48（18 个自动组 + 2 个会话组 + 28 张独立）；18 个自动小组一个不变——那是指标口径问题（见下条），不是 class。新增 `TestMotifGroupClassUsesMajority`、`TestMotifApplyGroupClassSetsCenter`、`TestMotifRasterLeaderKeepsFollowers`。合并语义变了就作废旧结论：`motifDedupeVersion` `visual-motif-v2-tools` → `visual-motif-v3-class-agnostic`，否则上一版写下的 `groups.json` 与逐图 `dedupe_checked` 仍能通过复用闸门（输入、阈值、版本都比得上），续跑继续用被 class 切碎的分组。
- **T148 复用闸门永远命中不了**：那次续跑（14:47）又跑了 ~35s 全量扫描、两个会话记录都重放了一遍，而输入与规则根本没变。根因：`motifReuseDedupe` 要求 `groups.json` 覆盖的**全部**引用都带 `dedupe_checked`，但"没有候选、没进会话"的图永远不会被打上这个标记（它不参与合并，也就没人给它盖章）——T141 的快路径在任何一次真实运行里都不成立。改成两级：有跟随关系的引用验 `DedupeChecked` + 版本 + `dedupe_source_hash`；**独立**引用只验摘要与 `groups.json` 记录一致（一个关系都没有，就没有会过期的旧结论）；覆盖条数检查不变。`TestMotifReuseDedupeValidatesConclusions` 同步改写（清掉组代表图的结论必须重扫；独立图无 `dedupe_checked` 也允许复用）。
- **T146 `pairs.csv` 过程量列**：`overlapDetail` 的返回值从 `(score, g, canvas)` 收成结构体 `motifAlignDetail{Score, Level, Canvas, Levels, ShiftY, ShiftX}`（只有一个调用点：`writeMotifMetrics`），把 `motifCanonScoreShift` 早就算好却被 `_` 丢掉的判定层平移格数与级数写出来；表头变 `a,b,feature_similar,align,align_level,canvas,levels,shift_y,shift_x,skip,same_bucket`，跳过的对三列留 0。新增 `TestMotifMetricsPairsTraceColumns`（列名/列数、算得出分数的对必须带过程量、内容框一致的两张图 `shift` 必须是 0、能算分的对不许带 `skip`）。
- **T145 文档对齐 + 删死代码**：画布 `S` 文档写池级 P90、代码按每一对取 `max(maxSide)`，`motifCanonCanvas` 有实现有单测但**没有生产调用点**——用户在本批定为"改成实际口径"。删掉 `motifCanonCanvas`、只被它用的 `motifCanonPct` 与 `sort` import、`TestMotifCanonCanvas`；`TestMotifCanonScoreSynthetic` 改用 `a.maxSide()` 并注明画布按每一对取；`docs/dedupe.md`/`docs/dedupe-algorithm.md` 的画布定义抄成同一句并写清理由（混尺寸池子里池级 P90 会让小图只占画布一角、分数被系统性压低）。同批把平移口径写明：中心是**逐级累计**的 2×（接上一层自己选中的那一格），不是"相对 0 级的 `2^k·d₀`"——729 对里 693 对两种口径分数不同、最大差 0.072、判定翻转 **0** 对，故只改措辞。
- **为什么 100 份的大桶会裂成 18 个 ≥0.75 的小组（同批核清，不改判据）**：19 个组的关键图两两 `align` 只有 0.496–0.741、中位 **0.615**、**没有任何一对到 0.75**（≥0.70 的 14 对、≥0.65 的 48 对），而组内成员都在 0.75 以上——即"同一枚、同尺寸"的分数很高，"同一枚、跨字号/跨笔画"的分数结构性地到不了自动档，`0.75` 这道闸门在书内就把同一家族切成 18 组。这与 class 无关（class 分支已在 T147 删除），是"不缩放"画布的必然代价：把内容框缩放归一后同一批 19 张两两中位 0.615 → **0.772**、≥0.75 从 0 对变 93/171 对，但那样会连带纳入 11 张本来 <0.35 的独立图，属于判据变更，留待单独一批由用户定。
- **T149 同一个桶被切成十几个组，同一枚图各画一遍（用户口径纠正）**：用户：「我说的那个100的桶，独立出大于0.75的相似度的，是作为不需要进入会话中去看的了，剩下的那些是看需不需要合并入大于0.75的那个分组，而不是独立为一个分组…所以最终的要绘制的图最多31张」——自动定组只表示这几张**不用再看**，不是各自成为一个最终分组。核对代码：`applied` 里的组**从来没有彼此合并过**，`motifAttachLeftovers` 只管"没进过任何会话的图"，会话给出的结论各自成组。实测该书最大桶（100 份不同内容）：自动定组切 18 块、会话再交 1 块，`groups.json` 里 **19 个组、19 个中心图**；这 19 个关键图两两 **171/171 对全部达标**（无跳过、0.496–0.741）、在 0.60 上连成**恰好一个连通片**，本来就是同一张图。单靠"挂靠"补不上：它只处理没进过会话的图。
- **T149 修法**：`applied` 之后加 `motifFoldGroups`（顺序：会话 → 挂靠 → 收口 → 副本跟随 → 落盘）。判据：**只在同一个相似桶内**（桶号复用 `advisory[key].Group`，即 `metrics/buckets.json` 的 `initial_group`；跨桶不折——桶是 0.60 连通的候选集合，出了桶就没有"同一张图"的前提）且两块关键图 `motifMergeAllowed` 才连边，连通片折成一组；中心图在**各块的代表图**里取"与其余代表图平均重合度最高"的那块（`O(块²)`，19 块 = 171 对；比在 85 个成员上做 `O(n²)` 便宜两个数量级），代表图再按组内闸门同一规则换成"与中心 ≥ `dedupe_align_typical` 的成员里内容框最大者"，组类别仍取多数 class 落在中心图上。**成员原样收编**：先按"过新中心闸门"实现过一版，实测 85 个成员只有 75 个对新中心达标、10 张被退回独立（等于同一枚图还是要画两遍）——成员已各自过会话/组内闸门，收口阶段不再重判。产物标 `source=folded`，日志「组间收口：同桶的 N 个组折成 1 组（M 张，中心图 …）」。
- **T149 预期与代价**：该书最终要绘制的图 48 → **31**（1 个折出来的 100 张组 + 2 张 + 28 张独立），与用户"最多 31 张"一致；`motifDedupeVersion` 与 T147 共用一次升级到 `visual-motif-v3`。已知边界：不同内容的两张图最高也能到 0.607（贴着 0.60 门槛），桶内连边理论上可能把一对不同的图串起来——这与建桶同一口径、且每次收口都打日志可核；跨桶那条限制正是为此再收紧一层。新增 `TestMotifFoldGroupsFoldsSameMotif`、`TestMotifFoldGroupsKeepsDistinctApart`、`TestMotifFoldGroupsStaysInsideBucket`。
- **测试/校验**：`GOCACHE=/tmp/dsh-gocache go build ./...` 干净；`gofmt -l` 对改动的 Go 文件无输出；`go test ./internal/latex/ -run 'Motif' -count=1` 与全量 `go test ./internal/... -count=1` 全绿。文档同步：`docs/dedupe-algorithm.md`（画布/平移/会话 class/指标列/复用闸门/已知不完美）、`docs/dedupe.md`（画布、`pairs.csv` 列、组类别）、`docs/issue.md` T145–T149、CHANGELOG `[Unreleased]`、`AGENTS.md` 现状要点。

### T150–T152 dedupe 收尾：看图页、扫描提速、随组带入（2026-10-18 第十六批）

- **T150 分组结论只能读 `groups.json`，看不到"选中的到底是不是同一张图"**：用户：「并且有没有在dedupe里面链接一下选过的图片，我想手动看一下。但是这边我看不了，我感觉这次应该分类的是对着的」。新增 `motif_review.go`，落盘时多写 `<proj>/source/dedupe/review.html`：每组一张卡片、代表图标 `★`、成员按 `groups.json` 顺序排成缩略图网格，缩略图宽 160px、JPEG q=88 内嵌 data URI（离线双击就能看，不依赖服务），点缩略图在新标签打开原图；独立图单独一块。缩略图有解码预算（`motifReviewProbeBudget` 约 2 亿像素），超预算的图退回只给链接。`report.md` 的代表图与独立图同时从"文件名"改成 Markdown 相对链接，两页互为索引。实测 388 条引用 / 130 份不同内容 → `review.html` 452 KB，一次落盘（groups.json + report.md + review.html + metrics）859ms。新增 `TestWriteMotifReviewLinksSelectedImages`。
- **T151 扫描 33s 的瓶颈在指标，不在判据**：用户：「这个扫描候选的时候目前需要33s关键路径是什么？哪里是瓶颈？100多张图都要30多秒有的时候我们这边会有上千张图。」逐段计时（388 引用 / 130 份不同内容）：读+哈希 5ms、解码 388 张 60ms、特征 130 份 206ms、覆盖率 177ms、特征对 4ms、桶扫描 2.2–2.4s、**逐对指标 26.2s**、落盘 0.9s。即 33s 里近 80% 花在给 `metrics/pairs.csv` 的**全部 7503 对**算居中重合度上，而其中 99% 是跨桶、大画布的对（画布 283–512 的单对 12–19ms），对"怎么分桶、怎么分组"没有任何影响——真正决定结论的只有同桶内的对。
  改法（判据一条没动）：① `writeMotifMetrics` 直接复用桶扫描阶段已经算过的分数（`motifPairRef` 只补算没算过的对），不再重算一遍；② 覆盖率与特征各算一次、进同一次遍历；③ 预筛下沉成公共函数，被预筛掉的对记 `skip=prefilter`（以前是算完再丢）；④ 打分并行（上限 `motifAlignWorkers`）、按"一对值多少"排序（小画布/同桶先算）；⑤ 全程一笔成本预算 `motifMetricsCostBudget`（1.2e9 单位），超出记 `skip=pair_cap`——上千张的池子上这段有上限。实测同样 388/130：逐对指标 26.2s → **3.4s**，整段 ~33s → **~7.0s**；`pairs.csv` 仍一对一行（8385 行：有分数 6160、`prefilter` 1343、`too_large` 882），有分数的对中位 0.597、≥0.60 的 2976、≥0.75 的 232。
- **T152 会话确认过的组，复核又把成员退回独立（35 而不是 31）**：用户：「我重新跑了是35张……我感觉这次应该分类的是对着的」。核对：那 4 张都在核验会话的 `motif_groups` 里（`manifest.md` 逐张看过、会话给了 `key`），落盘却是独立图。根因是复核只拿成员与**组中心**比 `align ≥ 0.60`——同一枚图换分辨率/长宽比时它与中心只有 0.52–0.64，而与组内**某个已经达标的成员**是 0.686–0.812；复核把它判成独立，又已经把 `dedupe_checked` 写成了真，收尾的挂靠再也没有机会碰它们（`motifAttachLeftovers` 只处理没核验过的图），于是永远独立。
  修法：`motifAlignFilter` 保留这一轮已经算出的两两分数矩阵（原来算完就丢），离中心不足的成员再与**已经达标的成员**比一次（≥ `dedupe_align_min` 就随组带入）；只比**一轮**、不拿带入的成员当跳板（0.60 是传递的，任由接力会把整条相似链并成一张图），带入的成员不参与代表图挑选（它离中心不够、代表不了这一组），日志附「（其中 N 张离中心不足、与组内成员达标，随组带入）」。判据与阈值都没动（`motifAlignFilter` 只是多一个 `carried` 返回值）。同时把「组内最低分」的口径写进产物：`report.md` 出现低于门槛的成员时加一句脚注——它是**与代表图**的分数，入组要求是"与组里某个已确认成员达标"，组间收口时成员随代表图一起带入、不再重判，逐对证据在 `metrics/pairs.csv`。预期 35 → **31**（1 组 + 2 张 + 28 份独立）。结论语义变了就作废旧结论：`motifDedupeVersion` `visual-motif-v3` → `visual-motif-v4`，否则续跑的复用闸门（版本 + 规则指纹 + 输入摘要）会继续沿用那份 35 张的旧分组、改动等于没生效。新增 `TestMotifAlignFilterCarriesMemberMatchingAnotherMember`（三档尺寸：离中心 0.46、与组内另一档 0.66 的那张要带入且不当代表图）、`TestMotifCarryOverDoesNotRelay`、`TestMotifArtifactReportExplainsBelowMinMember`。
- **测试/校验**：`GOCACHE=/tmp/dsh-gocache go build ./...`、`go vet ./...` 干净；`gofmt -l internal/latex/` 只剩既有的两个历史文件；全量 `go test ./internal/... -count=1` 全绿。文档同步：`docs/issue.md` T150–T152、CHANGELOG `[Unreleased]`（Added/Changed/Fixed 各一条）、`docs/dedupe-algorithm.md`（产物、预筛与预算、复核带入、收口后果、落盘、`skip` 取值、实测表）、`docs/dedupe.md`（复核例外、看图页、`skip` 取值）、`AGENTS.md` 现状要点。

### T153 / T154 dedupe 续跑缓存 + 看图页搬进会话预览（2026-10-18 第十七批）

- **T153 扫描中间量落盘（用户：「接着做那两个还没做的」——已通过的候选之一：续跑不要再重算）**：dedupe 阶段的桶扫描与逐对指标此前每次续跑都重算一遍（同一批 388 条引用 / 130 份不同内容约 7.0s，其中逐对指标 3.4s、桶扫描 2.2–2.4s）。现在把这两段的中间量写成 `<proj>/source/dedupe/scan.json`（新文件 `motif_scan_cache.go`）：`{format, version, key, built_at, merges, buckets:[{members, exact}], pairs:{ref→{s,l,c,n,y,x}}}`——桶结果连同「是否自动定组」一起记，逐对明细记分数/判定层/画布/级数/平移两向（`motifScanPairOf`/`detail()` 双向转换，读回来就是原来那个 `motifAlignDetail`）。键 `motifScanKey(hashes)` = `"v=%s;fmt=%d;min=…;shift=…;auto=…;typical=…;grid=%s;maxarea=%d;bucket=%d\n"` + **排序去重后的内容哈希集** 的 sha256 前 16 位：版本或任何一个判定相关旋钮变了键就变。命中条件不止键相等，还要求池里**每个内容**都在 `pairs`/`buckets` 里有记录（`groupsFor` 遇到缺项返回 nil）——否则等于用半份证据冒充全量。命中即整段跳过并打一行 `[dedupe] 沿用 source/dedupe/scan.json（键 …）：N 个桶、M 对逐对明细，跳过桶扫描与逐对重算`，桶扫描被跳过时 `merges = -1`（日志里那行「合并 N 次」照旧能对上）。读坏/缺项/版本不符一律只当未命中，照常全量重算并覆写。**纯缓存，判据一条没动**：命中与不命中的产物逐字节相同（`TestMotifScanCacheReuseIsOutputNeutral` 钉住），所以 `motifDedupeVersion` 不升；结构变化只升 `motifScanCacheFormat`。`motifArtifactRules`（写进 `groups.json` 的 `rules` 只有 `align_min`/`align_shift`/`icon_auto`）**没有**跟着加宽——另外那几个旋钮由 `motifScanKey` 覆盖，不必动已提交的产物 schema。实测同一批图续跑 **~7.0s → ~1.3s**。新增 `TestMotifScanCacheKeyFollowsCriteria`（改 `align_min`/`grid` 键就变）、`TestMotifScanCacheCorruptIsIgnored`（半截 JSON 当未命中）。
- **T154 看图页搬进会话预览（用户：「生成的那个 dedupe 的 review.html 我觉得可以和那个 web 界面放一起…里面有个按钮可以点就能打开预览这个界面」+「图片不用 base64 嵌入，而是连接」+「每个分组可以收起来或者展开查看，点击能看大图，再点一下可以回去，并且支持缩放」+「文件夹里面 review 也保留」+「目前 review 点击里面图片打不开，并且分组默认展开有的分组比较大，还有就是图片都是 base64」）**：
  - **点图打不开的真因**：旧页 `<a href="data:image/jpeg;base64,…" target="_blank">`——Chrome 明确拒绝**顶层导航到 data: URL**，点了什么都不会发生（右键另存倒是能用，所以一直像"小毛病"）。现在点缩略图走**页内灯箱**，图片本身就是 `/media/.../images/...` 的真文件 URL，右键/复制链接一样能用。
  - **入口**：会话预览服务新增 `/dedupe?project=<项目名>`，**服务的就是磁盘上那一份 `source/dedupe/review.html`**——渲染器只有一份，服务端只把 `<head>` 之后插一行 `<base href="/media/<项目>/source/dedupe/">` 再吐出字节（`injectBaseHref`），页面里的相对链接因此落到既有的 `/media` 路由，不必为预览页另写一版渲染（也避开了 `latex` ⇄ `sessionview` 的循环 import：`cost_report.go` 已经 import 了 sessionview）。`/api/index` 多一个 `dedupeReviews` 列表（只列真有这一页的项目，`dedupeReviewProjects`），侧栏「整组核验」阶段行右侧因此多一枚**常显**的「复查」胶囊（新标签打开；`@click.stop` 阻止冒泡去折叠外层 `<details>`，用 `.stop` 而不是 `.prevent` 才能正常新标签导航）。没有这一页时 404 并说明「整组核验跑完才有」。
  - **页面本身**：缩略图改成落在 `thumbs/<内容哈希前 12 位>.jpg` 的普通 `<img>`（`motifReviewThumbName`，加载时写、不内嵌 base64），`<details class="group">` **默认收起**（收起的组浏览器根本不发图片请求）、顶部给 `全部展开`/`全部收起`；`<img loading="lazy" data-orig>` + `onerror` 退回原图链接；点缩略图开灯箱：滚轮按光标位置缩放（0.9/1/0.9 步长，夹 `[0.1, 8]`）、双指捏合、拖动平移（3px 阈值，`pointercancel` 收尾）、`+`/`−`/复位、再点一下图或点空白或 `Esc` 关闭（照用户「再点一下可以回去」删掉了双击复位）。磁盘上的 `review.html` 照旧保留、离线双击仍能看。
  - **验收证据**（fixture 页：2 组 + 8 张独立、24 条图片引用 / 22 张不同缩略图）：收起状态 **0** 个图片请求（`[0,24,10]`）、点 `全部展开` 后 **24/24** 全 200（`[2,24,24]`）；灯箱开→`100%`→滚轮 `152%`→点击关闭 true→`Esc` 关闭 true；真实项目页（17 个项目 / 45 个阶段行）只有 **1** 枚「复查」、在「整组核验」行、新标签标题 `dedupe 复查`、注入 `<base>`、无破图、不触发外层折叠。
  - **测试**：`TestWriteMotifReviewLinksSelectedImages` 同步钉住新形态（`../images/…`、`thumbs/<12 hex>.jpg`、`id="lb"`、`data-all="open"`、`<details class="group"`、无 `data:image/jpeg;base64,`、无 `<details class="group" open`、`代表图 ★`、`组内最低分`）；新增 `TestServeDedupeReviewPage`（`dedupeReviews == ["bookA"]`、200 `text/html` + 注入的 `<base>`、缩略图与原图各一条 200、未知项目 404、`../etc` 400）。
- **测试/校验**：`go build ./...`、`go vet ./...` 干净；`gofmt -l` 只剩既有的两个历史文件；全量 `go test ./internal/... -count=1` 全绿（latex / sessionview / img2text）；`web/npm test` 129 passed；`npm run build` 重建 `go/internal/sessionview/assets/dist`（`viewer.css` 74.53 kB / `viewer.js` 466.71 kB）后 `go build` 再验证；`node --check` 过页面脚本。文档同步：`docs/issue.md` T153/T154、CHANGELOG `[Unreleased]`（T153 Changed、T154 Added、点图打不开 Fixed）、`docs/dedupe-algorithm.md`（产物、流程图、落盘、续跑四层）、`docs/dedupe.md`（`scan.json`、review.html 形态）、`docs/latex.md`（整组核验落盘）、`docs/commands.md`（会话预览「复查」入口与 `/dedupe` 路由）、`AGENTS.md` 现状要点。

## v1.5+（第三十九批，P34 后续）

- 递归章节会话使用完整稳定 ID 命名转换与 checker transcript；预览扫描兼容旧嵌套 generic 文件名，并恢复父 ID、角色和深度。
- 会话 API 增加章节树字段与 `chapter_activity` 状态。Web 侧把 convert 作为树节点、同 ID checker 放在节点下方，子节点递归；排队、等待子会话、运行、核对和结论状态可见，checker 不重复计入章节聚合。
- 父节点等待子树时释放全局章节 API 槽位；转换、checker 与提醒请求统一通过共享 `latex.concurrency` 槽。strict checker 遇余额/API 错误立即 fail-closed，保留现场供更换 checker 模型续跑；续跑恢复原 transcript 历史、轮次与工具计数并使用当前模型配置。
- 回归覆盖章节身份/父子配对、旧路径兼容、等待元数据、全局槽位、split reminder 抑制、欠费不重试、checker 模型切换后的 transcript continuity；Web 测试覆盖递归树排序和 checker 去重计数。

## v1.5+（第四十批，T156–T158：bash 可写工作区 / responses 流式 / 内存自证）

- **T156 bash 沙箱把会话自己的工作区锁成只读**。`WorkBashTool.sandboxArgs` 原先用「这个挂载目录顶层有没有软链」来决定走"视图目录"分支——有软链就按条目逐个 `--ro-bind`。逐章转换工作区的顶层恰好有 `images`/`figures` 两枚软链（`linkAbs` 指向项目只读资源树），于是 `work:`（本应可写）整棵走进只读分支：`chapter_01.tex`、`manual.md`、`parts/` 都被只读绑定，沙箱里新建文件能写、**改已有文件一律 EROFS**。`write_file` 走 VFS 不经沙箱，于是现场看起来像"只有 bash 坏了"，多个会话在 `OSError: [Errno 30] Read-only file system` 面前直接放弃。
  改法：可写挂载**无条件** `--dir dst --bind mountDir dst`（软链原样保留），再遍历顶层软链，把每个解析目标的**真实绝对路径**补一条 `--ro-bind target target`（去重、跳过 `target == mountDir`、目标不存在就跳过），这样 `images/xxx.jpg` 在沙箱内仍可读、资源树仍不可写；只读视图分支（原书 PDF 视图、项目窄视图）逻辑原样未动。
  验证：`TestSandboxWritableWorkspaceWithLinks`（参数面：必须是 `--bind`、自己的 `.tex` 不得出现 `--ro-bind`、软链目标按真实路径只读绑定）与 `TestBashSandboxEditsOwnWorkspaceWithLinks`（行为面：真 bwrap 下覆盖已有 `.tex` 成功、`cat images/pic.jpg` 读到内容、往软链目标 `echo x >` 仍报只读）。
- **T157 `api: responses` 接上 SSE**。原来该分支 `payload.Stream = false` 强制非流式。现在：`buildResponsesBody` 写 `stream: payload.Stream`；`postResponses(raw, stream)` 按模式选 `streamHTTP`（流式不吃整次超时）；新增 `readResponsesStream` 消费 `response.output_text.delta` / `response.reasoning_summary_text.delta` / `response.reasoning_text.delta` 喂 `StreamHook`（150ms 节流、尾 16KB）与 TTFT，用 `response.completed`/`response.incomplete` 里的完整 response 对象经 `decodeResponsesBody` 收账——与非流式**同一个解码器**，所以转录与用量不随传输方式变化；`response.failed`/`error` 先试解嵌在 `response.error.message` 里的原因再退回事件级字段；`ChatCompletion` 里照 openai 分支补了 4xx/连接错误的一次非流式回退。
  实现细节两个坑：① 行读取 goroutine 在 EOF 后必须用 `msg, open = <-lines` 判关闭——最初写成只判 `err`，channel 关闭后零值空转，`go test` 直接挂死 8 分钟；② 网关忽略 `stream:true` 返回整段 JSON 时不能报「没有 response.completed」，改成把整段 body 攒下来兜底 `decodeResponsesBody`，既有端到端用例因此零改动通过（它们本来就回 JSON）。
  顺带修 `finish_reason`：截断判据原来只认扁平 `incomplete_reason`，补上 REST 的 `incomplete_details.reason=max_output_tokens` → `length`（`length` 是空响应自救的触发条件，漏了就自救不了）。
  测试：`TestResponsesStreaming`（`stream:true`、`Streamed`、TTFT 在 (0, elapsed]、内容、工具调用、usage、`ReasoningChars`、hook 收到增量）、`TestResponsesStreamFailureSurfacesReason`；`TestResponsesEndToEnd` 的 `stream` 断言改为 `true` 并注明同时覆盖"整段 JSON 兜底"。
- **T158 「已杀死」= SIGKILL，改成先留证据**。现场：进程消失、章节转录停在 `waiting_children`、日志无错误行，shell 只打一句「已杀死」（SIGTERM 才是「已终止」）；取证时可用 RAM ~740MB、swap 已用 ~10GB/27GB、`Committed_AS` 超 `CommitLimit`，符合 OOM killer 条件，但 `dmesg`/`journalctl -k` 需要 `adm`，读不到，所以判断只能到"最可能"。SIGKILL 拦不住，于是新增 `internal/latex/memwatch.go`：`RunBook` 一进来就 `startMemWatch(time.Minute)`，立刻打一行 `[mem] 基线`（`VmRSS` + `MemAvailable`/`MemTotal`），此后每 60s 采样并维护 RSS 峰值与可用内存最低值，`defer stop()` 收尾打一行汇总；`memLevel` 三档（`<1GiB 或 <10%` = low，`<256MiB 或 <3%` = critical），low/critical 立即 `LogWarning` 并给"调低 `latex.concurrency`"的可执行建议，critical 明说"随时可能被 SIGKILL、日志不会有错误行"。判据用 `MemAvailable` 而不是 `MemFree`（换出压力下 `MemFree` 仍然好看）。`/proc` 不可读时字段退化为 `?`，绝不因此让流程失败。
  测试：`TestParseMemInfo`、`TestParseRSS`、`TestMemLevel`（阈值边界含 0/unknown）、`TestStartMemWatchLogsBaselineAndStop`（基线与结束两行都真的落盘、`stop()` 不阻塞）。
- **文档同步**：`docs/issue.md` T156–T158；CHANGELOG `[Unreleased]`（T157/T158 Added、T156 Fixed）；`docs/config.md`、`docs/sessions.md` 去掉"后两条线路只走非流式"的旧表述；`go/internal/config/config.go` 的 `API` 字段注释与两份配置模板注释同步；`AGENTS.md` 现状要点更新沙箱口径与内存自证。
- **校验**：`gofmt` 只碰本批文件；`go test ./internal/... -count=1` 全绿（latex 32.1s / session 6.1s / sessionview 17.7s / img2text 12.2s 等，exit 0）。

## v1.5+（第四十一批，T159：续跑回放清洗与输出预算兜底）

- **背景**：用户准备续跑整本书，先问"这些会话是不是要删掉尾巴才能继续"。逐转录核查 187 份，10 份以失败结局，归成三类：**欠费**（`SESSION_INSUFFICIENT_BALANCE`，按设计 fail-closed，充值后续跑即可）；**400 循环**（两例，`invalid_request_error / upstream_request_rejected`，不可重试）；**空回复**（两例，连续两轮 `finish=length`、`completion_tokens` 打满、content 为空 → `empty final answer`）。
- **400 循环的根因**：模型流式输出的工具参数被截断，转录里存下的是不闭合的 JSON（`{"path": "chapter_01.md"` 少右括号）。第一次请求合法（历史里还没有这条坏消息），**第二次请求**把这条 assistant 消息连同坏参数原样回放，网关校验工具调用失败 → 400，会话从此每次续跑都在第一个请求上失败。这不是"要不要删会话"的问题，而是回放没做合法性检查。
- **空回复的根因**：`max_tokens` 是**思考 + 正文**的总预算。开着思考的模型配 8192 时，思考把 8192 全吃掉、正文一个字都不吐；原来的空回复自救只是原样再问一次 `Continue.`，预算没变 → 又是一轮 8192 全空 → 判失败。用户直觉（"配置太小、该调大"）是对的；代码侧要做的只是别让同一个预算再撞一次，并把根因说清楚。
- **改（`internal/session`）**：
  - `replay_sanitize.go`：`sanitizeReplayMessages` 在 `LoadTranscriptStats` 返回前跑一次——坏 JSON 参数按 `{}` 重放并把对应 tool 回执改写成"参数不是合法 JSON，已按空参数重放，请重新调用"；正文为空且无工具调用的 assistant 消息丢弃；其余消息逐条不变。**只改内存历史，磁盘转录一字不动**（事故现场要留原样）。
  - `session.go`：`finish=length` 且正文为空时，重试预算翻倍（`budgetCeiling` 按"窗口 − 当前上下文 − 2% 余量"夹住），并打 `LogWarning` 说明"是预算被思考吃光、该调大会话的 `max_tokens`"。
  - `budget_warn.go`：会话创建时若模型 `thinking.type` 为 enabled/adaptive 且 `max_tokens < 16384`，按「模型 + 预算」去重告警一次，提示调到 32768 以上（几十个章节会话不会刷屏）。
- **测试**：`TestSanitizeReplayFixesMalformedToolArguments`、`TestSanitizeReplayDropsEmptyAssistantMessages`、`TestSanitizeReplayKeepsCleanHistory`、`TestBudgetCeilingIsWindowAware`、`TestNudgeBoostsBudgetAfterLengthTruncation`（断言第二次请求的 `max_tokens` 大于被截断那次）、`TestBudgetWarningForThinkingModel`（含去重守卫）。
- **现场核对**：用只读加载跑过那四份卡住的转录，修复后 `badArgs=0`、`emptyAssistants=0`——续跑第一个请求即合法，**不需要删改任何会话历史**。
- **文档同步**：`docs/issue.md` T159；CHANGELOG `[Unreleased]` Fixed；`docs/config.md`（`max_tokens` 是总预算、思考模型建议 ≥32768）、`docs/sessions.md`（回放清洗与截断兜底）；`AGENTS.md` 现状要点补两句。
- **校验**：`go build ./...`、`go vet ./internal/session`、`gofmt` 干净；`go test ./internal/session ./internal/latex -count=1` 全绿（6.1s / 45.0s）。

## v1.5+（第四十二批，T160：两份配置模板同步）

- **背景**：用户大改 `config.example.yaml` 后问"软件里面那个 default 跟这个 example 差距有点大，是不是之前一直没更新"。核对结果是**两份模板长期各自演化**：键面一直有 `TestConfigTemplateKeyParity` 守着（键集合必须一致），但值、注释、示例条目全可以漂移——当年实测差异 **339 行**：示例新增了 `gateway-a`/`gateway-b`/`heavy`/`vision-heavy` 等命名基座演示 `extends`（单基座、多重基座、链式），`default.yaml` 还是逐个角色写死 `model:` 的老写法；注释一边英文一边中文教学；`verify`/`img2text` 块顺序不同。值差异 11 处：`max_fix_rounds` 8→40、`raster_dpi` 110→220（docs 早已注明是模板示例值）、`sessions.drawing.max_tokens` 16384→131072、`sessions.style.max_tokens` 32768→131072、`preview.img2text` false→true、`mermaid.fix_attempts` -1→2、`mermaid.session_errors` 3→6（这四处此前未注明）、`models.{chapter,heavy,style}.extends` 目标不同，以及 `tools.python.install_timeout` 的 300/30000 之别（单位是秒；曾按数量级笔误改成 300，开发者随后改回 30000 并保留，文档默认值仍写代码默认 300）。
- **改**：① 两份模板同步为同一内容（用户已把示例整体覆盖到 `default.yaml`），`sessions.dedupe.max_tokens` 与 `img2text.max_tokens` 按用户要求统一为 **65536**；② `install_timeout` 按开发者的值保留 30000（`docs/config.md` 默认值列写代码默认 300，并注明模板示例 30000）；③ `docs/config.md` 为 `sessions.*.max_tokens`（模板示例 131072 / dedupe 65536 / img2text 65536）、`mermaid.fix_attempts`（示例 2）、`mermaid.session_errors`（示例 6）、`preview.img2text`（示例 true）补上"随附模板示例写…"的注记——此前只有 `raster_dpi`/`max_fix_rounds` 两行注明过。
- **防复发**：新增 `TestConfigTemplatesInSync`（两份模板逐字节比对，单边修改立刻失败；失败信息里写明"若确需不同，请改成只比值并写进 docs，不要直接删测试"）与 `TestBuiltinTemplateLoads`（**内嵌**的 `ConfigTemplate` 走 `LoadConfigData`：版本号等于 `CurrentConfigVersion`、`models.text.model` 非空、示例条目 `vision-heavy` 的多重基座链能解析出 model）。
- **校验**：`go build ./...`（模板是 `//go:embed` 的，必须重编）；`go test ./internal/... -count=1` 全绿（config 0.08s / latex 19.0s / session 6.1s / sessionview 18.1s 等）。

## v1.5+（第四十三批，T161：递归章节树的展开交互）

- **背景**：用户要改会话预览侧栏里"多子会话"的展开操作——「不要点一下那个会话查看，然后点一下展开的逻辑有点奇怪…左边那个下拉按钮点击的范围可以大一点，点击可以下拉展开，而不是用两次操作，并且可以维持其他的地方的展开保持不变，而不是点一下其他的之前拉开的都收回去了」。
- **根因**：`ChapterSessionTree.vue` 用 `<details :open="nodeContainsCurrent(node)">`。`:open` 是响应式 binding，每次重渲染（每秒轮询、选中任何会话、filter 变化都算）都会把「不含当前会话」的节点重新赋成 closed——所以刚拉开的树会自己收回去，点别处更是全收；展开状态完全没有记忆（侧栏三层分组早有 `state.collapsed`，章节树是漏的）；`<summary>` 一次点击同时触发"选中会话"与"展开"两件事；可点区域只有 12px 的小三角。
- **改**：① 展开状态改成每节点自己的记忆，键 `chapter:<项目>/<阶段>/<节点ID>`（不带 scope 会让不同书/不同阶段的同名 `chapter_001` 串状态），存 `state.collapsed` 并落盘 localStorage；② `<details>` 换受控 `<div class="chapter-node" :class="{open}">` + `v-if` 子树，行点一下只选中、左侧独立按钮点一下只切展开（`@click.stop`），两件事各一次点击就到；③ 展开按钮扩成整行高 26px 的命中区（原 12px 三角），带 hover 与 focus-visible；④ 既无子节点也无 checker 的节点不显示按钮（占位保持缩进对齐）；⑤ checker 行补 26px 左内边距与按钮同一列；⑥ `Sidebar` 两处调用传 `:scope="项目名/阶段"`。新增纯函数 `chapterNodeKey`/`chapterWantOpen`（`web/src/legacy/sidebar.ts`）。
- **踩到的坑**：`state.collapsed` 是 Vue 的 reactive 代理，`hasOwnProperty` 不走 get 陷阱、**建不起响应式依赖**。第一版照 `groupWantOpen` 的 `hasCollapseMemory` 实现，探针里表现为"点展开按钮 → 记忆确实写进 localStorage，但 `open` 类与子树不出现"，要等 `state.current` 之类别的 reactive 变化才显形。改成读一次 `state.collapsed[key]`（不存在的键也会被 track）后立即正常。
- **验证**：`web/tests/chapter-tree.spec.ts` 六条（键带 scope、默认只展开装着当前会话的一支、手动展开一直开着、手动收起优先于自动露出、切一个不影响兄弟、落盘键带 `dsh.sessionview.` 前缀）。另起临时无头 chromium 探针（`web/tmp_tree_harness`，验证完即删、不入库）在真实 DOM 上点了一遍，全部符合预期：点按钮只展开不选中、点行只选中不改展开、两个节点互不影响、切到别的节点会话后前一个仍开着、子节点按钮只切自己、清空记忆后装着当前会话的分支自动露出、模拟轮询重渲染不收回、按钮命中区 26×29px、无子节点的节点没有按钮。`npm test` 135 passed、`npm run build` 后 `go build` + `go test ./internal/...` 全绿。

## v1.5+（第四十四批，T162：进程被「已杀死」时的现场取证）

- **背景**：用户报「这次启动 30s 左右就被杀死了，这是为啥？/tmp 不够用了吗？……我也没办法检测？能否开启一个 debug 模式记录到 log 文件……怎么监控一下？」——终端只有一句「已杀死」，没有任何错误行。
- **现场取证**（只读检查，未改用户机器上的任何文件）：
  - 机器 15.5GiB 内存、**swap 已用 11GiB**（长期压力），`MemAvailable` 6.3GiB；
  - `/tmp` 是 **tmpfs 7.8GiB，已用 3.9GiB**——tmpfs 就是内存，其中 **3.4GiB 是我自己跑测试留下的 `GOCACHE=/tmp/dsh-gocache`**。这是我这边的问题，已立刻删除（顺带清掉 `/tmp/go-build*` 残留），可用内存 6.3→6.6GiB、swap 11→8.6GiB，并改用 `/var/tmp/dsh-gocache`；
  - 运行日志显示这是一次**续跑**：26 章、`latex.concurrency: 5`，一启动就「恢复中断的会话」二十来条（最长 146 条历史消息）——每条恢复都要把转录里的图片重新读成 data URL，这是启动阶段的内存尖峰；
  - 日志里只有一行 `[mem] 基线`（T158 的内存自证），之后再无采样：采样行走 `Trace` 级（info 下不显示）且只进终端日志，进程一被 SIGKILL 就什么都不剩。
  - 结论：不是 `/tmp`「不够用」（7.4GiB 空闲），而是**内存**——15.5GiB 里可用只剩 6GiB 左右、swap 已经在撑着，再叠加 5 路续跑会话同时回放带图历史，被内核 OOM killer 挑中。`已杀死` 是 shell 对 SIGKILL 的汉化，SIGKILL 无法被程序捕获，所以问题不是「没处理信号」而是「没留下证据」。
- **改**：
  1. 内存采样**同时追加到项目内的 `<proj>/work/mem.log`**（时间戳 + `[mem]` 行）。终端被刷掉、窗口关掉、进程被杀，这个文件都还在；
  2. 采样节奏：**前 3 分钟每 10s**（「已杀死」大多发生在起步阶段）、之后每 60s，收尾再一行；
  3. 采样内容补 `VmHWM`（内核记录的峰值，采样间隙也不漏）与 swap 总量/用量；swap 已用 >60% 且 >1GiB 时告警"实际余量比这个数字更小"；
  4. 恢复会话的日志行带**该历史回放的图片张数与字节数**（新增 `session.ReplayWeight`，`map[string]string` 与 `map[string]interface{}` 两种 `image_url` 形态都算），单条 ≥32MiB 额外告警——直接指认哪条会话吃内存；
  5. `--debug`/`--trace` 启动提示写明日志文件路径（新增 `logger.Path()`）；`docs/sessions.md` 新增「内存日志」一节。
- **落在哪**：`go/internal/latex/memwatch.go`（采样/落盘/swap 判据）、`book.go`（采样移到项目目录确定之后，传 `work/mem.log`）、`go/internal/session/session.go`（`ReplayWeight`）、`go/internal/latex/resume.go`（恢复行带图片负载）、`go/internal/logger/logger.go`（`Path()`）、`cmd/docvision/main.go`（调试提示带路径）。
- **验证**：`TestStartMemWatchLogsBaselineAndStop` 扩展为同时校验 `mem.log` 落地；新增 `TestMemSnapshotParsesSwapAndPeak`、`TestReplayWeightCountsImageBytes`；`go build ./...` + 全量 `go test ./internal/...` 绿。
- **给用户的操作建议**（回复里也写了）：`latex.concurrency` 从 5 调到 2–3；跑之前看 `free -h` 的 `可用`，别在 `swap 已用` 很高时开跑；跑起来后 `tail -f <proj>/work/mem.log` 就能看到实时内存曲线。

## v1.5+（第四十五批，T163：会话把宿主进程 kill 掉——沙箱补 PID 命名空间）

- **背景**：用户报「还是已杀死，但是毫无征兆，我看 mem.log 了并没有」——上一批（T162）把死因判成 OOM，这次拿到 `--debug` 日志后直接推翻了。
- **真因（日志原文，同一条 `convert:chapter_009_child_04` 会话）**：
  1. `[21:36:09] tool result: bash  1358006 docvision  ./docvision latex … --debug`——模型在沙箱里跑 `ps`，看到**宿主 DocVision 自己的 PID**；
  2. `[21:36:14] tool call: bash {"command":"kill -STOP 1358006; sleep 1; python /tmp/build.py; …"}`——先把整本书冻住（`work/mem.log` 里 21:36:11→21:37:17 恰好空了一分多钟，与 SIGSTOP 完全吻合），想独占机器跑自己的编译脚本；
  3. `[21:37:39] tool call: bash {"command":"kill -9 1358006; echo ok"}`——日志到此结束，终端留下 shell 报的「已杀死」。
  内存侧一切正常：`mem.log` 里进程 RSS 峰值 0.1GiB、系统可用 6.0GiB 以上，一条低内存告警都没有。所以「毫无征兆」是真的——SIGKILL 由同一用户的子进程发出，程序侧没有任何可捕获的路径（T158/T162 的内存取证本身没错，只是它证明的不是这次的原因）。
- **根因**：bwrap 沙箱只隔离了**文件**（挂载表）与**网络**（`--unshare-net`），**没隔离进程**——与会话共享宿主 PID 命名空间，于是 `ps` 看得见宿主进程、`kill` 能直接对它发信号。
- **改**：① `sandboxArgs` 加 `--unshare-pid`（配已有的 `--proc /proc`，新命名空间里的 `/proc` 只列自己这棵树）；② 按用户要求把 `[mem]` 采样行（含基线与结束）全部降到 `Trace`——普通模式终端不再刷 `[mem]`，只保留一行 `[book] 内存/被杀现场日志: <路径>`（低内存告警仍是 `WARNING`），`work/mem.log` 不受日志等级影响；③ `docs/sessions.md` 沙箱一节写明「文件+网络+进程三个面都要隔离」与这起事故。
- **验证**：`TestSandboxArgsUnsharePID`（参数必须有 `--unshare-pid` 且紧跟 `--proc /proc`）、`TestSandboxCannotSignalHostProcess`（**在本机真起 bwrap**：`ps -eo pid=` 的输出里没有宿主测试进程的 PID，`kill -9 <宿主 PID>` 返回非 0，宿主进程存活）、`TestMemWatchQuietAtInfoLevel`（info 日志里不得出现 `[mem]`，而 `mem.log` 仍有基线与结束行）；既有沙箱集成测试（只读挂载/可写工作区/符号链接/Python 环境/持久 `/tmp`）全绿，全量 `go test ./internal/...` 绿。
- **教训**：沙箱隔离要按「文件 / 网络 / 进程」三个面逐项确认，只验证"宿主路径看不见"是不够的；另外「进程被杀但没有任何错误行」除了 OOM，还要怀疑**同用户的其他进程**——包括模型自己的工具调用。

## v1.5+（第四十六批，P52：只记录一个未来方向，无代码改动）

- 用户提出（原话）：「未来可能会删掉章节划分这一步骤，直接传入整个，由下层觉得太长了拆分更自然且短文件节约」——明确说**只做记录、不实施**。
- 记在 `docs/plan.md` 的 **P52**（`[ ][ ]`，未实现）：取消独立 `chapters` 阶段，把整本书直接交给转换层，转换会话在"一次干不完"时自己调 `request_chapter_split` 就地细分（P34 的递归链路已经具备），父节点整合并过 checker。条目里写清了预期收益（少一次全书会话、拆分点由读内容的那层决定、产物更短更省）与必须先想清楚的点（并发从哪来、失败重试粒度、预览页章节 ID 从"划分产物"变成"递归路径"、`chapters/` 与 `progress.json` 的阶段语义、`chapter_granularity` 下沉成"自拆建议粒度"）。
- 同批次还回答了两个只问不改的问题（结论记在这里，避免下次重复调研）：① 「卡编译」不是资源竞争——`--debug` 日志显示那条转换会话先 `ps | grep '[d]ocvision latex'` 找"另一个进程"，`kill -STOP` 冻住宿主后**它自己的工具回执也卡住了**（30s 超时），它把宿主误当成抢资源的进程再 `kill -9`；② 递归子章节的收集链是完整的：子节点各自 convert + **StrictChecker**（必须明确 pass）、转换报告落 `_subdivision/child_NN/node/reports/conversion.md` 并写进 `children-manifest.md`，父节点整合后自己再过一次整章 checker，任一子节点失败则父节点"禁止提交"并把失败原因往上传；**唯一的弱环节**是"文字意见"不跨层——子 checker 的 issues 只在子节点内迭代 ≤3 轮，过不了就整支失败，"通过"是硬约束而"意见"不汇总给父节点，且整合提示只硬性要求读子 `.tex`/`parts`，报告是"可读"不是"必读"。

## v1.5+（第四十七批，T164/T165：章节转换的"换会话重试 + 编译看图门禁 + 子意见上浮"）

- **背景**：用户要求彻底修正章节转换的质量（原话要点）：checker 顶层重试到 6 次、每层 -1、下限 3、配错要告警；换会话重试要在预览页能看到「历史 N」，「章节转换与下面的小子章节同一策略」；「很多弄好子任务之后回来之后他们并不会看子会话的那个报告」；「很多连编译都没编译连看都没看」。同意作废当次运行、从头重跑。
- **现场取证**（一本 12 章、递归到第三层的书，24 条转录，均为中立计数）：
  1. 8 个已开始的顶层章节：`compile` 次数 1/3/2/1/4/4/10/0——**1 章 0 次编译**；`view_pdf` 次数 1/2/4/0/0/0/3/0——**3 章编译过但没看过**；另有 4 章当时还没开始转换（进程被杀）。
  2. 根章节的 checker 转录里只有 `read_file`/`grep`、**一次 `submit` 都没有**（8 轮、25 次工具调用、38 分钟）——旧兜底「两次没结论也算通过」把它放进了书。
  3. 一条递归子会话在拿到 `Read-only file system`（时间戳早于 T156 的挂载修复）之后开始 `ps`/`kill`、反复重写同一个文件、反复提交——同一会话被打回 3 轮后仍在原上下文里打转；旧代码对**递归节点**是"打回 3 轮就整支失败"，只有顶层才换会话。
  4. 子章节 report 里写过"enumerate/proof 结构对不上、没能干净编译"这类文字意见，但父会话的整合指令只点名读子 `.tex`/`parts`，报告不在必读清单。
- **改**：
  1. **重试预算**：`checkerAttemptBudget(base, depth) = max(3, base-depth)`，`base` = `latex.checker_rounds`（未写 = 6，代码默认也是 6，`0` 视同未写）；`checker_rounds - 3 < 3` 时 `RunBook` 起步打一条 `WARNING`，说明"第 3 层只会吃到下限 3、建议至少设 6"。
  2. **顶层与递归节点同一策略**：同一会话被 checker 打回 `rounds` 轮仍不过 → 作废产物 + **换全新会话**（新转录 `convert_<章>.try2.jsonl`、`try3`…），旧转录保留；次数用尽**判失败**并带最后一次 issues。递归节点不再"一次不过就整支失败"。
  3. **无结论 ≠ 通过**：`checkChapterAt` 两次都没拿到 `submit` 时返回不通过；厂商/网络错误（含空响应重发后仍空、余额不足）单独标注并**不**触发"换会话重转换"（余额不足一次就停）。
  4. **子意见上浮**：新增 `_subdivision/children-summary.md`（每个子章节 report 原文 + "自报 issues" 计数 + 整合者必读清单），整合指令改为"先读这份汇总，能修的修，修不了的写进你自己的 report（可引用、可追加）"；`children-manifest.md` 保留路径清单。
  5. **提交门禁**（T165）：`.gate/compile.json` 记「主 `.tex` + parts 内容摘要 + 产物 sha256 + 页数」，`.gate/view.json` 记看过的页；`submit` 要求摘要与当前文件一致（改过就得重编）、产物指纹一致、至少看过 `min(2, 总页数)` 页，拒绝文案直接指向下一步动作。
  6. **预览页**：会话索引多一个 `chapterAttempt`（`<名>.tryN` 后缀归一，章节 ID/role/父节点/深度都不变），前端 `chapterAttemptChain()` 把同一章各次尝试聚成链——中栏出现「历史 N」页签（与 img2text 调用链同款，`chainMemberTag()` 优先给「第 N 次」），侧栏行带「第 N 次」chip。
- **实现位置**：`go/internal/latex/chaptergate.go`（新）、`book.go`（预算/重试/孩子汇总/门禁接线）、`checker.go`（无结论判失败、致命错误分类）、`runner.go`（`isSessionFatalError`、`submitGate` + 测试旁路 `noChapterGate`）、`tools_convert.go`/`tools_work.go`（compile/view_pdf 打点）、`internal/config`（`latex.checker_rounds` + 两份模板 + `docs/config.md`）、`internal/sessionview`（`chapterAttempt`）、`web/src/legacy/sidebar.ts` + `HistoryPanel.vue` + `Sidebar.vue`、`prompts/templates/latex_convert.system.md`（一句"编译后必须自己看第 1 页与最后一页"、"先读 children-summary"）。
- **验证**：新增 Go 用例 `TestCheckerAttemptBudget`、`TestCheckerBudgetWarning`、`TestAttemptTranscriptPath`、`TestChapterGateRequiresCompileAndView`、`TestChapterIdentityAttemptSuffix`、`TestCheckerWithoutVerdictFailsAfterTwoAttempts`、`TestCheckerRecoversAfterDegenerateResponse`、`TestCheckerDegenerateInBothAttemptsFails`；web 新增 `chapter attempt chain` 三例（聚链/单次不出链/标签）。全量 `go test ./internal/...` 与 `npm test` 绿，dist 重建后 `go build ./...` 通过。
- **给用户的重跑口径**：这一批之前的运行产物作废没问题——门禁会让"没编译/没看过"的章节在 submit 处被挡回，重跑即可；旧项目里已有的 `.gate/` 为空会被要求重新 compile + view_pdf。

## v1.5+（第四十七批补一，T166：问题跨层合并 + 提交产物自包含）

- **背景**：用户接着第四十七批提了两点：① 不要求父章节修复子章节的报告（"大概率是样式的问题"），但**原始问题必须原样继续向上交**；担心"一个章节有多个 issue 挤在一个 submit 里"，也担心父会话"要弄得东西不少、不一定有余力看"，所以合并要由**程序**做；② 追问"父会话为什么看都不看就交了"——之前没有这个现象，怀疑是加了子会话之后才有的，想弄清根因。
- **问题路径（改动前）**：只有 `submit` 的 `report.issues`（一个自由文本），每章落 `work/reports/<章>.md`，风格修复大循环读这些 md。子章节的问题**没有任何机制**能到父章节：需要父会话逐份读报告、再自己抄进自己的报告，任何一步漏掉就丢；多条问题也只能挤成一段话，无法逐条追踪或标记"已修"。
- **改**：
  1. `ChapterIssue`（ID/来源/深度/正文/状态）+ 每节点 store `<work>/.issues/issues.json`；`submit` 的 `report.issues` 收**字符串数组**（推荐）或旧字符串，新增 `report.resolved`（继承条目 ID）。
  2. 合并全在代码里：子节点写 store → `convertChapterChildren` 收齐子节点**未解决**条目写 `_subdivision/issues-inherited.json` → 父节点 submit 时"继承（未解决）+ 自己新报 − resolved"，写回 store 并把未解决清单原样追加到报告 md。顶层报告 = 整棵子树的未解问题全集，风格修复读一份就够。
  3. `children-summary.md` 与整合指令明确**不要求修复**：能顺手修的写 `resolved`，没修的原样向上。
  4. **第 2 问的根因（取证）**：5 个已进入整合阶段的顶层节点里——005 编译 4 次、**全程 0 次 view_pdf** 就提交；007 看过第 1 页与最后一页，但之后又改了 4 轮、编译 4 次、提交 3 次，**那次查看与最终产物已经不是同一份**；008 在整合阶段发现子产物里的 `\input{_subdivision/…}` 在父工作区解析不了，花了整轮 `find` 临时目录、手工 `mkdir`+复制，最后 **0 次编译、0 次查看**，直接以 `report.status=issues` 提交。004/006 当时还停在 `waiting_children`/续跑状态，不算"不看不交"的证据。也就是说：不是模型"不想看"，而是(a) 没有任何机制要求它编译+看，(b) 整合的入口——子产物——本身带着只在临时工作区成立的路径，父节点连编译都编不起来。
  5. 因此新增"提交产物自包含"校验：`\input`/`\include`/`\includegraphics`/`\subfile` 指向 `_subdivision/…`、`node/result/…`、`/temp/…` 的一律在 submit 打回，并说明"把内容整合进 `<章>/` 再交"。嵌套父章节从此不会把断链产物交进书里。
- **实现位置**：`go/internal/latex/chapterissues.go`（新：条目/合并/渲染/报告解析）、`tools_convert.go`（submit 合并 + 自包含校验 + report schema）、`book.go`（继承收集、children-summary 文案、整合指令、工具接线）、`prompts/templates/latex_convert.system.md`（issue 数组、resolved、自包含一句话）、`docs/latex.md`、`docs/issue.md` T166、`CHANGELOG.md`。
- **验证**：`TestMergeChapterIssuesInheritsAndResolves`、`TestReportTextListAcceptsBothShapes`、`TestIssueStoreRoundTrip`、`TestUnportableIncludes`；`go test ./internal/latex` 与全量 `go test ./internal/...` 绿。

## v1.5+（第四十七批补二，T167：清到"章节划分之后、章节转换之前"的清理脚本）

- **需求**：用户要一个和 `clean_project.sh` 同款（带备份、带计划预览）的脚本，"帮我删一下章节转换后面的"，自己稍后运行。第一稿把"章节转换的成果"当成了要保留的东西（默认只清风格修复/汇编/终审），用户跑完立刻纠正：**"我说的要删除章节转换这一章的内容，我等下重新跑来着"**——重跑要从逐章转换开始，所以转换成品、每章报告、逐章与 checker 转录、章节临时工作区都必须一起删。
- **为什么不能只删目录**：`RunBook` 的阶段跳过看 `progress.json`（`style`/`images`/`chapters`/`convert`/`assemble`）。保留转换成品时，即使把 `convert` 标记归空，逐章也会命中"`work/chapters/<章>.tex` 已存在 → 跳过"，实际重跑的只是风格修复；而只删成品不改标记，`convert="done"` 会把整段连同风格反馈回路一起跳过。两件事必须一起做。
- **实现**（`clean_convert.sh`）：
  1. **默认删** `work/chapters/`、`work/reports/`、`work/temp/`（含递归子树、`.gate`、issues store）、`work/sessions/{convert_*,checker_*}.jsonl`，以及转换之后的 `build/`、`out/`、同级 `<项目名>.pdf`、`work/style_fix|stylefix_assess|stylefix_integrate|final_review|views|media|pdfview`、`work/sessions/{book_fix,final_review,style_fix_*}.jsonl`；
  2. `progress.json` 的 `convert`/`assemble` 归空（`style`/`images`/`chapters` 保持），重跑从逐章转换开始、每章重新开会话；
  3. **保留**划分与样式前置成果：`chapters/*.md`、`work/sessions/chapters.jsonl`、`style/`、`work/style/`、`work/style_session.jsonl`、`source/`（分类结果、images 软链、dedupe 结论）、`doc_index/`；
  4. `--keep-convert` 提供反口径（保留转换成果、从风格修复重跑），此时把 `work/chapters` 里指向 `_subdivision/…` 的断链点名（T166 修掉的那类，旧二进制产物，汇编会失败）。
- **交互**：与旧脚本一致——默认 `cp -a` 整目录备份到同级 `<项目名>_记录_<时间戳>/`、先打印计划、`-y` 确认、`-n` 只预览、`--no-backup` 跳过备份、检测到 `docvision latex` 正在运行就拒绝（`-f` 强来）。
- **验证**：`bash -n` 通过；真实项目上 `-n` 确认清单（转换成品、12 条转换转录、4 条 checker 转录、`work/temp` 全在删除列表；`chapters/*.md`、样式包、`source/` 在保留列表）；临时假项目上跑真实删除，核对删/留清单与 `progress.json` 归空结果；`docs/latex.md` 的"清理与重跑（辅助脚本）"登记 `clean_project.sh` 与 `clean_convert.sh`。

## v1.5+（第四十七批补三，T168：必须对照原书 + 内容覆盖率门禁）

- **用户反馈**："有的章节转换会用一个 py 的转换器，然后他也不管最后出来的内容和原来的是否一致。就是没有在他们这一章节逐个 pdf 和原来的核对，他自己的和原来的包括里面的内容。"
- **根因**：T165 的提交门禁只覆盖"编译过 + 看过自己的 PDF"。两者都满足时，一份拿脚本机械包出来的 tex 依然可能漏段、丢表、把公式降级成纯文本——产物内部完全自洽，checker 只对着 md 与 tex 看结构，也发现不了"内容少了"；没有任何环节拿**原书**去比。
- **为什么不做"检测脚本"**：沙箱里 bash/python 是干正事的工具（批量替换、查错、抽样核对都合法），按工具名判定既误伤又容易绕。改成判定**结果**：内容对不对得上、有没有和原书对过，这两件事可机械验证。
- **实现**：
  1. `ViewPDFTool` 记录原书页：工作区之外的 PDF（`source:<原书 part>.pdf`）走 `gate.markSourceViewed` → `.gate/source.json`（文件+页去重）；工作区内的仍按产物哈希记 `view.json`。
  2. `chapterGate.check()` 第二道：至少 `needSourcePages(产物页数)` = min(4, max(2, 页数/3)) 页不同的原书页，否则打回并提示 `list_source_pages`/`doc_search` 定位。
  3. 第三道 `checkCoverage()`：`pdftotext -layout` 抽产物文本，`mdPlainText` 把章节 md 洗成纯正文（去 ``` 代码块、`$$…$$`/`$…$`/`\[…\]`、HTML 注释、图片，保留链接文字与表格文字），两边只保留汉字做三字组交集，覆盖率 < 0.85 **第一次打回**并列出缺得最多的 5 段；**同一章再交仍不达标就放行**（用户口径："第一次打回第二次给通过"——OCR 差异大的书可能永远到不了阈值，硬卡会把整本书卡死），放行时把覆盖率、缺失段与 `report.coverage_note` 写成一条未解问题进本报告，靠 T166 的合并机制向上带；CJK 三字组 < 200（纯公式章节）不判定；`pdftotext` 缺失时跳过（不因环境缺件卡死整本书）。
  4. 提交后把覆盖率写进日志，< 92% 追加一条"建议再对照原文检查漏段"。
  5. 提示词：`latex_convert.system.md` 的 Before submitting 段落补两句（必须对照原书页；不要用脚本机械包 tex）；`book.go` 的整合指令同样要求对照原书。
  6a. 顺带把"自己处理不了怎么办"补完整：`## Recursive subdivision` 原本只写"太大 / 上下文超限就 `request_chapter_split`"，现在把"内容做不可靠（公式太密、OCR 认不出）"也算进触发条件（子会话是独立会话、可以看自己那一份的原书页），并补一句"开到最深仍做不出来时，把缺什么、为什么缺写进 report.issues，不许编造"。
  6. 提示词里"翻原书核对"那句原本是**条件触发**（"whenever something looks doubtful"）：机械包出来的 tex 读起来顺畅，模型不会觉得"有疑问"，于是永远不翻原书，而且没有任何机制核对它翻没翻。改成无条件——每次提交前都要对比本章的几页原书，并点明"整段缺失、表格丢行、插图错序，产物内部看不出来，只有原页能看出来"。
- **阈值标定**：同一批真实产物上 `chapterCoverage` 量到 0.970/0.989/0.991/0.995；用 python 把同一 PDF 文本砍掉 1/4、1/2 后覆盖率掉到 0.63–0.76 / 0.33–0.59。取 0.85。
- **验证**：`TestMdPlainTextDropsNonProse`、`TestCoverageOfDetectsMissingContent`、`TestCoverageOfSkipsFormulaOnlyChapters`、`TestNeedSourcePages`、`TestPathWithin`；`TestChapterGateRequiresCompileAndView` 扩成四段式（无编译 → 只看自己 → 只看 1 页原书 → 2 页原书放行）；`go test ./internal/latex` 与全量 `go test ./internal/...` 绿。

## v1.5+（第四十七批补五，T169：章节规模上限与"太大就先拆"）

- **用户观察**："有可能是这个分配的章节实在是太大了，父会话一个会话处理起来会比较耗时。"
- **量到的规模**：同一批真实项目里，划分阶段给出的 26 个章节 md 从 150 行到 20190 行，总计 133244 行，中位数约 3400 行。也就是说"一章 = 一个会话"这个假设在这类书上根本不成立：读一遍 md、写一遍 tex、编译、看图、核对，几千行的量在一个上下文里做不完，慢与漏都是必然。
- **旧缺口**：递归拆分机制齐备（2–16 段、覆盖全文、≤3 层、子会话各过 checker、父会话整合），但触发条件是模型自评"太大"——没有数字、没有后果；而且会话连自己这章多少行都不知道。
- **改**：首条消息追加 "This chapter is N lines of markdown."，超限时附行动建议（拆 2–16 段、每段约 limit/3–limit/2 行）；`submit` 门禁新增规模关：`lines > limit && !splitRequested && depth < 3` → 打回并要求 `request_chapter_split`（已拆分节点正在整合子章节、最深一层拆不动，二者放行）。上限 `latex.max_chapter_lines`（未写 3000，0 关闭），两份模板 + `docs/config.md` + `docs/latex.md` 同步。
- **验证**：`TestChapterSizeNote`（小章节只报数字 / 超限催拆分并给区间 / 最深一层不催 / 关闭时只报数字）、`TestChapterSizeReject`（未超限 / 超限 / 已拆分 / 到最深 / 关闭五个边界）；`go test ./internal/...` 全绿。

## v1.5+（第四十七批补六，T170：章节行默认打开最新一次尝试）

- **用户反馈**：侧栏某个章节转换的行点开，这一章有多次重试（中栏出现「历史 N」）时，默认选中的应该是最新一次，而不是第一次。
- **根因**：`buildChapterTree` 遍历会话列表逐个赋 `node.convert`，后写覆盖先写；Go 侧列表按 `ModTime` 新→旧排序，所以同一章的多条尝试里最后留下的是 **mtime 最早**的那条 = 被 checker 打回、产物作废的第 1 次。点章节行看到的是废产物，真正的结果要进「历史 N」再点一次。
- **改**：`newerAttempt()` 按 `chapterAttempt` 序号取大（相同才比 mtime），convert / checker 各取最新一次；章节树行加「第 N 次」chip，打开前就知道代表哪次。`chapterAttemptChain()`（历史页签 新 → 旧）不动。
- **验证**：`web/tests/chapter-tree.spec.ts` +5 条（两种入参顺序都选第 3 次、checker 同规则、缺 `chapterAttempt` 时按时间、子节点仍挂父节点）；`npm test` 143 通过；`npm run build` + `go build` 通过（embed 产物已提交）。

## v1.5+（第四十七批补七，T171：重试转录套娃 / 裂成两行 / 陈旧「运行中」）

- **用户反馈**（真实运行中的侧栏截图）：「这个 try2 和 try3 怎么感觉有点混乱……try2 后面又跟了个 try3，并且应该现在在历史中显示的外边了。」截图里同一章两行：`chapter_001 第 2 次`（3.52M / 31k，"运行中"）与 `chapter_001.tr… 第 3 次`（1.00M / 13k，"运行中"）。
- **直接证据**：项目 `work/sessions/` 里真的存在 `convert_chapter_001.try2.try3.jsonl`。
- **根因一（套娃命名）**：重试是递归调用 `convertChapterNode`，`next := node` 复制的是**已改写过 TranscriptPath** 的结构体；进入下一层又调 `attemptTranscriptPath`，对 `…try2.jsonl` 再套一层 → `…try2.try3.jsonl`。T165 引入"每次尝试一条独立转录"时，只验证了"第 2 次"这一层。
- **根因二（裂成两行）**：`chapterIdentity` 的 `^(.*)\.try(\d+)$` 只剥一层，`convert_x.try2.try3` 解析成 ID `x.try2`、第 3 次；于是它不再是 `x` 的尝试，而是侧栏里的另一个章节节点。T170 的"同一章内取最新一次"救不了跨 ID 的情况。
- **根因三（假运行中）**：转换会话只在开跑时写 `chapter_activity=queued/running`，结束后不写终态；第 2 次转录里有 `run_end`、mtime 已是几十分钟前，活动标记却还停在 `running`，侧栏照抄这个标记 → 与真正在跑的第三次并排"运行中"。
- **根因四（修的时候顺着查出来的）**：重跑时 `node.Attempt` 从 1 重新数，与磁盘上已有的 `convert_<章>.try2.jsonl` 撞名；`NewTranscript` 是 `O_APPEND`——两段对话会被追加进同一条转录。
- **改**：① `attemptTranscriptPath` 先剥尽已有 `.tryN` 再套，递归时用原始 `transcriptBase`；② **历史数据一次性改名**：项目内 5 条套娃转录（顶层 1 条 + 递归子章节 4 条，全部是 convert）改成规范名，`chapterIdentity` 保持只剥一层 `.tryN`，不留兼容分支——用户口径："不用给我们历史错误的做兼容，你将我们的历史修改为正确的就行"；③ 拆成两个号——`AttemptNum` 写进文件名、跨运行单调递增（`attemptNumberOnDisk`：最后一次没有 `run_end` 就原位续跑并用**文件本来的路径**，否则新开一号），`Attempt` 只管本次运行的 `checker_rounds` 预算；④ 会话结束写 `checking_result`，checker 写 `pass`/`issues`，失败写 `error`，`sessionview` 兜底把"有 `run_end` 却停在 `running`"折成 `interrupted`（侧栏「已中断」、不算 live）。web 侧补 `interrupted: '已中断'` 文案，dist 重建。
- **验证**：`TestAttemptTranscriptPath` 补两条（已带 `.try2` 再派生 → `.try3`；旧 `try2.try3` → `try4`）；`TestAttemptNumberOnDisk`（空目录 / 中断续跑取真实路径 / 已结束取新号 / 旧套娃取最大 / 别章同前缀不串）；`TestChapterIdentityAttemptSuffix` 补旧套娃与子章节套娃；`TestActivityAfterEnd`、`TestReadStatSawRunEnd`；`go test ./internal/...` 全绿、`npm test` 143 通过、`npm run build` + `go build` 通过。
- **现场**：本批结束时用户那本在跑的书已按授权停掉（旧二进制会继续产出套娃名），待重建二进制后重跑；已被中断的那次尝试会原位续跑，已结束的旧尝试保留原名并在「历史 N」里显示。

## v1.5+（第四十七批补八，T172：checker 看得到产物与原书页，转换会话的说法可随提交交给它）

- **用户反馈**（原话要点）：「checker 的时候有时候会反馈不存在的问题，由于这个 checker 无法看到原文章之类的，会错误的认为一些内容。这边是否应该给 checker 提供查看的工具？还有就是是否应该提供转换章节的 submit 一些能够给 checker 提供信息的工具？因为有的问题是不存在的。」
- **根因**：P28 定的"极简只读会话"（只有 md、tex、parts 三个文本 + `read_file`/`grep`/`submit`）在有产物与原书页可比的时候才安全。实际数据里 md 是 OCR 文本（断字、重排、重复页眉），公式/表格在原页上是图形，插图与表格在排版里是浮动体——这些差异 checker 一律读成"缺失/错位"，转换会话被逼着改坏正确内容，或白耗几轮直至 `checker_rounds` 用尽、整章作废重转。
- **改**（两侧一起做，缺一边都不成立）：
  1. **checker 的可见面**：`ensureCheckerView` 铺好的视图之外，`checkChapterAt` 再把本章编译出的 `<章>.pdf` 软链进去（`check:<章>.pdf`），并把 `r.pdfView` 的 `source:` 原书 part 挂进 `view_pdf` 的挂载表；工具加 `ViewPDFTool` 与 `sourcePageTools()`（`list_source_pages`/`doc_search`）。`read_file`/`grep` 仍只吃 `check:`，不让它去翻二进制 PDF 上下文。
  2. **submit 的信息**：新增 `checkHints{PDF, Report, Notes}`，由 `convertChapterNode` 从 `gate.compiledPDF()`、`node.ReportPath`、`submit.Notes` 组出来，挂成 `check:<章>.pdf` / `check:report.md` / `check:notes.md`（notes 直接写盘）。提示词与首条消息都点明：这些是**说法**，要用产物页与原书页核实。
  3. **提示词重写**（`latex_checker.system.md`）：可见清单改成真实清单；新增"每条问题必须指明位置（md 段落或原书页 ↔ .tex 何处），指不出位置的不算问题"、"不许报"清单（OCR 噪声、只在原页上的公式/表格/插图、浮动体位置、措辞换行、style 意见）、"重复上一轮的问题前先回原页复核，自己看错了就撤回"。注册表 `MustMention` 加 `view_pdf`。
  4. **打回文案**（`book.go` 反馈轮）：补一段"指出的点不是真问题就别改坏正确内容——把证据（文件与行号或原书页码）写进 submit.notes 再交，会带着产物与原书页再核一遍"。
- **验证**：新增 `TestCheckerSeesProductAndHints`（视图里有 `<章>.pdf`/`report.md`/`notes.md`、notes 内容落盘、首条消息与工具表出现 `check:<章>.pdf`、`report.md`、`notes.md`、`view_pdf`）；`checkChapterAt` 的 5 个测试调用点补 `checkHints{}`；`go test ./internal/...` 全绿。
- **未做的（有意）**：没有让 checker 直接看 raster 原图（`view_image`）——插图重画是否像由 `latex.figure_check` 负责，checker 只需要判断题、表、公式、段落有没有丢；也没有把 submit 的 report 当"免检通行证"，否则转换会话可以靠自述绕开核对。

## v1.5+（第四十七批补九，T173：重跑不再凭空多一条空尝试，章节树跟随正在看的尝试）

- **用户反馈**（原话要点）：「我的这些会话怎么都从第二次开始了？是不是错乱了？我看这些第一次都是在等在子会话的返回呢？怎么进入检查阶段了？还有就是整体替换的时候选择某个历史的时候？而且上面存在问题了切换的时候下面的子会话怎么没有跟这边变化，变化那个子会话的历史？……还有对应的核对会话的历史查看，都不太和这次次数相匹配？并且这个次数就不要显示在web上显示中文 第几次了有点碍事。」
- **真实数据**（一个五章进入细分的工作区）：每章除真正干活的第 1 次转录外，多出 `convert_<章>.try2/.try3.jsonl`，内容只有两行（system + `waiting_children`）；第 1 次停在 `run_end ok` + `waiting_children`；`work/temp/conv_<章>/` 下所有文件的 mtime 同一秒，`_subdivision/split.json` 消失、子章节旧转录也全部消失。
- **根因链**：① 父会话在请求拆分那轮就 `run_end`（整合是同一会话后来的 `sess.Run`），所以"上一次有 run_end"并不等于"这一轮结束"；旧规则据此开新尝试；② 新尝试 `resuming=false` → `chapterWorkTreeAt` 直接 `RemoveAll(nodeRoot)`，把 `_subdivision/`（子章节 workspace、`split.json`、子转录）一起删掉 → 子章节从头再转、还留下一条两行空转录；③ 树永远取最新一次（T170），看旧尝试时子会话不动，convert 与 checker 还各取各的；④ 中文「第 N 次」chip 是噪音。
- **改**：① 新增 `attemptNumberForNode(transcriptBase, splitRestored)`——拆分状态可恢复的父节点沿用磁盘上最新尝试（同一轮没做完），其余仍"无 run_end 原位续跑 / 有 run_end 新号"；② `buildChapterTree(items, pinAttempt)` + `pickAttempt(cur, next, pin)`：树跟随当前会话的尝试号，≤pin 取最大、都比 pin 新则取最早，convert/checker 同一个 pin；③ `chapterAttemptTag` 改产 `#N`（第 1 次为空）、`chainMemberTag` 对章节统一 `#N`（历史列表），侧栏行与树行不再显示次数。
- **验证**：`TestAttemptNumberForNode`（5 个分支）；`chapter-tree.spec.ts` 新增 5 条跟随用例并改标签断言；`npm test` 148、`go test ./internal/...` 全绿。
- **善后**：10:56 那次旧逻辑重跑已把 002–006 章的工作树整棵清掉，子章节必须重转一次；修复后同类中断重跑直接从原尝试继续，不再清子树。

## v1.5+（第四十七批补十，T174：中断不算新尝试，作废尝试留档）

- **真实缺陷（T173 遗留）**：① 尝试号规则仍是"上一次有 `run_end` 就换新号"，而父章节请求拆分那轮就会写 `run_end`（整合是同一会话后来的 `sess.Run`），所以"等子会话时被杀"再重跑仍会开新尝试——用户口径：新尝试只该在 checker 跑满轮次后出现；② 换会话时 `chapterWorkTreeAt(resuming=false)` 直接 `RemoveAll(nodeRoot)`，上一轮的 `work/`（含 `_subdivision/` 子章节成品与转录）、`result/` 全被删掉，「历史」只剩对话，看不到那一轮交了什么。
- **改**：`lastAttemptNumber()` 一律返回磁盘最大尝试号（重跑接着最近那条继续），新号只在换会话那一步显式 `next.AttemptNum = node.AttemptNum + 1`；换会话前 `archiveChapterAttempt()` 把上一轮整棵移到 `<proj>/work/attempts/<章>/try<N>/`（work/含子树、result、integrated-stage、已提交 .tex 与分片、`checker-issues.md`），移动失败只警告；开新尝试只清当前层 `work`/`result`/`integrated-stage`/`checker-view`，保留 `sessions/`。
- **验证**：`TestLastAttemptNumber` 4 个分支、`TestArchiveChapterAttempt`（留档到位、旧位置不残留、子转录随子树留档、核对转录留在 sessions）；`go test ./internal/...` 全绿、`npm test` 148。
- **已知代价**：留档把子章节成品一并搬走，新尝试重转子章节时没有"已通过就跳过"的标记；同一次尝试内重跑已通过的子章节仍会重放对话 + 再跑 checker。

## v1.5+（第四十七批补十一，T175：历史切换只钉住正在看的那一条）

- **真实缺陷（T173 遗留）**：`pickAttempt` 让整棵树都按"尝试号 ≤ pin 取最大"重选，点开某章 convert 的 `#2` 时，同书其它章节（重试次数各不相同）也被拉到第 2 次或它们最早的版本。用户口径：每个节点只该显示自己最新的一次，切换历史只影响"我正在看的那一条（和它同一行的角色）"。
- **改**：`buildChapterTree(items, pinId)` 恢复 `newerAttempt` 的"各节点取最新"，只把 `pinId` 那条会话钉进它所在的节点/角色；同一节点的另一角色（convert ↔ checker）优先同一轮、没有同轮则最新；子会话与其它章节不跟着切。删除 `pickAttempt`；`Sidebar.vue` 传当前会话 id（不再按项目名算尝试号）。
- **验证**：`chapter-tree.spec.ts` 跟随用例改写为 4 条（只看那条变、同轮核对、看核对时同行转换切到被核对的那一轮、无 pin 全取最新）；`npm test` 147、`go test ./internal/...` 全绿。

## v1.5+（第四十七批补十二，T176：章节标题按纯文本，不吃下划线）

- **真实缺陷**：会话名统一走行内 Markdown，而章节会话的标题是流水线标识符 `chapter_004_child_01`——下划线被当成强调标记，渲染成斜体并从文本里消失（截图里显示 `chapter004child01chil…`）。
- **改**：`InlineMD` 加 `plain`（纯文本插入）；`plainTitle(s)`（有 `chapterId` 即章节会话）用于章节树两处标题、侧栏四处会话行、历史列表与详情栏「会话」行；其它会话仍走 markdown。
- **验证**：`chapter-tree.spec.ts` 新增 `plainTitle` 用例；`npm test` 148、`go test ./internal/...` 全绿，dist 重建。

## v1.5+（第四十七批补十三，T177：两侧栏的宽度上限按视口算）

- **真实缺陷**：三栏宽度上限是照 DSH 抄来的写死值（侧栏 264–420、详情栏 300–520），大屏上拖到顶就停，详情栏开着时侧栏几乎没有空间。
- **改**：新增 `columnDragMax(viewport, cols, side)`——中栏保底 640、另一列保留当前宽度、再按视口 60% 封顶，结果不低于该列最小宽度；`computeColumns`、拖动分隔条、`toggleDetails` 恢复宽度共用这一条规则。`SIDEBAR_MAX`/`DETAILS_MAX` 删除。
- **效果**：1920 屏、详情栏收起时侧栏可到 1152，详情栏 400 时侧栏可到 880，详情栏可到 1000；窄屏仍是原范围。
- **验证**：新增 `web/tests/layout.spec.ts`（7 条）；`npm test` 155、`go build` + `go test ./internal/...` 全绿。

## v1.5+（第四十七批补十四，T178：章节树状态文案按角色分开）

- **真实缺陷**：转换会话跑完写 `chapter_activity=checking_result`，前端统一译成「核对完成待处理」——核对会话还在「等待并发槽」时，转换行就说"核对完成"，两个阶段说反了。
- **改**：`chapterActivityLabel(state, role, prevVerdict, live)` 作为唯一来源：转换的 `checking_result` = 「转换完成待核对」、核对的 = 「核对完成待处理」、转换在 `queued` 且已有核对结论 = 「核对完成待并发槽」；其余状态不变。
- **验证**：`chapter-tree.spec.ts` 新增 3 条；`npm test` 158、`go build` + `go test ./internal/...` 全绿。

## v1.5+（第四十七批补十五，T179：转换与核对并发独立 + 模型级封顶）

- **用户口径**："核对和转换的并发是独立的，不能放一起算"；"同一个模型的话并发上限和会话的并发相结合"。
- **真实缺陷**：`latex.concurrency` 一个池子被转换、整合、checker 反馈轮、提交提醒共用，核对会话排在慢转换后面白等；没有任何地方按厂商模型限流，两个池子调大后打到同一模型上的请求数无从约束。
- **改**：① `Runner` 拆 `convertSlots`/`checkerSlots`/`chapterSlots`，上限分别来自 `latex.convert_concurrency`/`checker_concurrency`（0 回退 `concurrency`），`LatexConfig.ConvertWorkers/CheckerWorkers/OtherWorkers` 是唯一解析点；子节点本地 sem 改用转换槽位数。② `models.<名>.max_concurrency` + 会话层 `SetModelConcurrency`：`Run` 一开始取该模型槽位，键是实际下发模型名，`Config.ModelConcurrencyLimits()` 用 `ResolveModel` 归并同名条目并取最小值。③ `RunBook` 启动打并发账目，池子之和超过模型上限时告警。
- **验证**：`TestStageWorkers`、`TestModelConcurrencyLimits`、`TestModelSlotLimitsPerModel`（峰值正好等于上限，两模型互不影响，曾因两模型共用同一个 live 计数导致误报 3 > 1，改为按模型分别计数）、`TestModelSlotUnset`；`go build` + `go test ./internal/...` 全绿。

## v1.5+（第四十七批补十六，T180：转换行的「运行中」染绿）

- **用户反馈**："为什么运行中这三个字不是绿色的，只有核对那边的是绿色的？"
- **根因**：Checkers 行的绿来自 `.chapter-checker.live .chapter-state`；转换行是 `.chapter-node-head`，既没有 `live` 类也没有规则，所以状态一直是 muted 灰。
- **改**：`stateTone(label)` 按文案给 `.run` 类（只有「运行中」染绿，排队等槽的 live 会话不染），两行共用；删掉只服务核对行的 `.chapter-checker.live` 规则。
- **验证**：`npm test` 158、dist 重建。

## v1.5+（第四十七批补十七，T181：核对有结论后转换行改口 + 反馈轮补状态）

- **用户反馈**："转换完成之后等待核对，核对完成之后上面是不是该改状态了？如果是发现问题的话，上面不应该显示这个待核对吧？"
- **两个真因**：① `checker` 打回后的反馈轮只调 `sess.Run`，不写 `chapter_activity`，转换行停在上一轮的 `checking_result`；② 前端两行各读各的状态，转换行从不看核对的结论。
- **改**：反馈轮补 `queued`→`running`→`checking_result`；`chapterActivityLabel` 增 `peerVerdict`，转换行按同一行核对的最新状态说话（issues→「核对发现问题」、pass→「已通过」、核对接在跑→「转换完成待核对」）。
- **踩坑**：改函数注释时把块注释的 `*/` 一起吃掉，整段导出被注释掉，vitest 报 `chapterActivityLabel is not a function`（4 条红）；补回结束符即绿。
- **验证**：`npm test` 159、dist 重建、`go test ./internal/latex/` 绿。

## v1.5+（第四十七批补十八，P53/T182：手动重试——页面上重跑某一章）

- **用户需求**："网页上是否应该增加一个手动重试的功能？……在我自己看的时候如果发现有的会话存在问题，可以选择从某个记录处重试之类的功能？然后这个也算在历史里面。"（用选项问过范围，用户选 P1：章节重试 + 队列 + 页面按钮，只 convert）
- **约束**：预览页是**另一个进程的只读观察者**（`serve.go` 只放行 GET/HEAD + `/api/mermaid`），不能让它去 fork 一个会话；真正干活的是 `docvision latex`。于是把"重试"拆成两半：页面只**提请求**，运行中的 latex 在安全点**消费请求**。
- **改**：
  - 新包 `internal/retryq`：`work/retry/req-<纳秒>-<章>.json` 落盘队列（append-only，消费后改名 `.done.json` 留档）；`Pending` 跳过 `.done.json` 与坏文件，`Chapters` 去重，文件名 sanitize 防章节名被当路径。
  - `latex.retry.go`：`forceChapterRetry` 归档旧尝试（复用 `archiveChapterAttempt`）+ 清掉提交的 `.tex`/分片 + 分配新尝试号；`prepareRetries` 消费队列并对 md 不在的请求只警告；`hasPendingRetry` 决定 convert 阶段是否强制跑。
  - `book.go`：重试/`--only` 只做转换这一层——**不进样式反馈回路**（那会拿全书报告改 cls 再分块重转整本书），也不自动 assemble，日志提示再跑 `--step assemble`；`convertPhase` 的并发循环抽成 `convertChapterBatch`（重试补轮要复用同一套并发槽/进度行/失败汇总），阶段头按 `--only`/队列选目标，批次之间最多再补两轮；`runPhase` 对 convert 加 `forceConvert`；`convertOneChapter` 采用 Runner 分配的新尝试号；新尝试转录写一条 `t=meta kind=manual_retry`。
  - `archiveChapterAttempt` 顺手把旧 `report.md` 也挪进 try 目录——新尝试写同一路径，不挪就是被悄悄覆盖。
  - 服务端：`POST /api/retry`（校验项目在扫描根下、章节名 `^chapter_\d+(_child_\d+)*$`、`chapters/<章>.md` 真存在），`/api/index` 增 `retries`；`sessions --no-retry` 关掉写入口。
  - **核对角色（用户追加要求"这个 checker 会话也可以对他重试吧？不只是转换会话"）**：队列请求带 `role`（convert/checker），消费者按角色分流；checker 走 `forceCheckerRetry` + `recheckChapter`——产物一个字不动，只把旧结论挪成 `<章>.tex.checker.try<N>`、换全新核对会话写 `checker_<章>.try<N+1>.jsonl`（只读视图也按尝试号分开，编译产物从 `work/temp/conv_<章>/work/.gate/compile.json` 找回来挂进去，上次的 `notes.md` 继续用），结论照既有语义落盘（通过删 `.checker`、有问题写进去），API 失败不改结论；没有已提交产物的章节服务端直接 400。`/api/index` 增 `retryRoles`，「待重试」挂到对应那一行。
  - **阶段二（用户："针对某些可能有错误的会话进行手动的继续跑并且删除错误的记录之类的，不用等程序重新启动，可以手动重新加入并发队列。可能有的遇到了错误之类的，需要手动调整会话然后从某处继续"）**：① watcher 跑在整个 `RunBook` 生命周期（2s 一跳），消费不再受"转换阶段安全点"限制；② 章节锁 `holdChapter`（阻塞，主流程）/`lockChapter`（TryLock，派发端：忙就不消费、请求留队）+ `retryPrecondition`（样式/产物/转录没就绪也留队，只提示一次）；③ 第三角色 `continue`：`continueChapter` → `convertChapterNow` → `convertOneChapter(retry=true)`，`retry` 用来绕过"已有产物，跳过"、`continueText` 让会话拿到"接着干"那句开场话并强制真跑一轮（否则 replay 收账会把它吞掉），转录里记 `t=meta kind=manual_continue`；④ `/api/drop` 删转录（路径限定在 `<项目>/work/sessions/`、`.jsonl`、90 秒内更新或有 `.partial` 回 409），带 `.tryN` 的连 `work/attempts/<章>/try<N>/` 一起删；⑤ 页面：详情栏按角色给「重试这一章」/「重新核对这一章」/「继续跑（不新建尝试）」/「删除这条记录」。
  - **阶段二之一（用户："从某个错误的地方开始就是从这个错误的最后一条或者前几条我可以自己选择"）**：`retryq.Request` 的 `From int`（转录行号，含该行）+ `Path`（源转录文件名，显式指定而不是取最新——用户可能正在看第 2 次尝试）；`truncateChapterTranscript` 是纯文件部分（校验行号范围、只能切在 `t=msg` 行、源文件不动、新尝试号 = 磁盘上最大 + 1），`continueFromChapter` 之后再 `convertChapterNow` 用 `continueText` 把"后面都丢了、别假设之前改的东西还在"递给会话；服务端校验只有 continue 能带 `from`、源文件必须是本项目 `work/sessions/` 下的 `.jsonl`、行号不超范围；前端在轨迹表每行给「⤓」（hover 显现 + `confirm` 说清后果）。
  - **阶段二之二（用户："可以给矢量图那边加上重跑重试的功能吧……因为修改之后后面都可以变动"；同时定下"整阶段重跑不用做"）**：`ImagesOptions.Only` 让 images 阶段只跑点名的图（done 也算 pending、跳过分类与 dedupe、只跑 process + 重新嵌回 md；点名的键不在扫描范围内直接报错，避免"原始 md 目录不对"被静默吞掉）；`vectorTexBase(mdName, imgPath, label)` 从 `processVectorImage` 里抽出来当唯一命名来源，于是能**反查**：给一条作图转录名找回它的进度记录；`prepareVectorRedraw` = 找回记录 + 拒副本图 + 旧会话改名 `.tryN.jsonl` 留档（就地改名而不是挪进 `work/attempts`——扫描器把根下任何 `*.jsonl` 都当转录，挪走反而变成一条名字古怪的会话）+ 旧工作区挪 `work/attempts/vector_<成品名>/try<N>.work/` + 清成品保类别；`redrawVectorImage` 再 `RunImages(Only:[key])`；`r.vectorMu` 与 images 阶段互斥（都要写 `progress_items` 和 `source/*.md`），派发端 `TryLock` 拿不到就留队；`retryq` 允许"没有章节、只有 Path"的请求（`Pending` 同步放宽，文件名用 `vector_<成品名>` 以便事后辨认），`pendingRetries`/`retryRoles` 跳过空章节条目以免挂到空章节行上。
  - **阶段二之三（用户："矢量图这边允许一下，从某个消息处重试就是，不是完整重新，和章节那样一样"）**：`truncateVectorTranscript` —— 章节的做法是"前缀另写一条 `.tryN` 新尝试"，作图会话没有尝试号管道（tikz 只认 `vector_<成品名>.jsonl` 这一个名字），所以反过来：**前缀留在原地**当续跑起点、**完整的那份抄成 `.tryN.jsonl`** 留档，语义对外一致（历史里两份都在、回滚不丢对话）；只能在消息行上切（meta/usage 行不进消息流）；`prepareVectorRedraw` 多一个 `from` 参数，`from>0` 走截断、`0` 走整张重画，两条路都清成品；续跑那句默认说明改成 `FigureEnv.ResumeNote`（`Runner.vectorResumeNote` 在持 `vectorMu` 时设置），否则模型会以为后面那些被丢掉的轮次还有效；前端轨迹 ⤓ 从"只给章节 convert 会话"放宽到「矢量图」作图会话，`cutFrom` 按会话类型选角色（`vector` 不带章节 / `continue` 带章节），confirm 文案分叉；服务端 `serveVectorRetry` 校验 `from` 行号范围。
  - 前端：章节行悬浮 ↻（排队后换「待重试」角标）、详情栏「重试这一章」+ 一行反馈；**只给顶层章节**——细分出的子章节（`chapter_003_child_01`）不在 `chapters/` 下、由父节点的细分树产出，单独重跑它得连同父节点整合一起走，服务端对子章节回 400 并说明"请重试它所属的顶层章节"，`--only` 点名子章节同样报错。
- **踩坑**：① `.done.json` 也以 `.json` 结尾，第一版 `Pending` 把留档又读成待办（测试抓到）；② 连点两下时第二次仍按磁盘推导出同一个尝试号——磁盘上还没有 `.try2.jsonl`，改用 `max(磁盘, 本次已分配)` 兜住，否则两次归档撞进同一个 `try1`。
- **验证**：`go test ./internal/...` 全绿（新增 retryq 4 条、latex 3 条、sessionview 4 条）；`npm test` 160；dist 重建后 `go build ./...`。
  - **T183**：核对提示词只点名真实存在的"说法"文件——`checkerClaimNote(view)` 独立判断 `report.md`/`notes.md`，两个都没有就整句不提（此前只要 report 在就声称 notes 也在，模型白读一次不存在的文件，还要再解释"它为什么没有"）；`latex_checker.system.md` 同步改成「when present … a file that is not there simply does not exist」。测试 `TestCheckerClaimNoteNamesExistingOnly` 钉住三种组合。

## v1.5+（第四十七批补十九，P53：手动继续/从记录继续到底发不发东西）

- **用户需求**："这边从中间消息继续的话不需要发送任何内容吧？直接发送过去就行吧，模型就会接着回复，无需增加什么东西？就是从某个消息开始？还是说增加一个continue？矢量图和会话转换和核对，都检查。告诉我一下"；随后"还有就是这个plan你怎么排到p3了按照我们plan中的编号排吧"。
- **查下来（三类会话各是什么）**：
  - **章节转换（继续跑 / 从记录继续）**：新尝试转录 = 前 N 行前缀，恢复时 `resumeMsgs` 非空 → `resumeUserText(msgs, note)` **返回空串**（`internal/latex/resume.go`，2026-09-29 用户定的"续跑不插 user 内容"）→ `sess.Run(RunOptions{UserText: ""})` **原样续行，一个字节都不发**。那句 `continueChapterPrompt` / "rolled this chapter back to record #N" **从来没被发出去过**（死代码）。
  - **作图会话（从某条记录继续）**：同一条路——这轮刚加的 `FigureEnv.ResumeNote` 也走 `resumeUserText`，有历史时被同样丢掉，**同样是死代码**。所以用户的理解（"直接发过去，模型接着回复"）就是现状。
  - **核对会话**：**根本没有"从某条记录继续"这个入口**——轨迹 ⤓ 只对 `chapterRole==='convert'` 与 `stage==='vector'` 出现；核对只有「重新核对这一章」= 换一条**全新**核对会话（`checker_<章>.try<N+1>.jsonl`，任务文本是新建会话的首条消息，不是续跑）。
- **改**：把"假装要发"的死代码清掉，让代码与文档都说实话——
  - `Runner.continueText map[string]string` + `setContinueText`/`continueTextOf`/`continueChapterPrompt` → 只剩 `continueFlag map[string]bool` + `setContinueFlag`/`takeContinueFlag`；`convertChapterNow(proj, base)` 去掉 `continueText` 参数。
  - `convertChapterNode` 里继续跑那段改成：`if node.Depth == 0 && r.takeContinueFlag(base) { initial = ""; resumeSkip = false; 记 manual_continue }`——标记的唯一作用是**解除"上次已 submit 就收账即结束"**（否则点一下等于没点），不插内容。
  - 删掉 `FigureEnv.ResumeNote`、`Runner.vectorResumeNote` 与 `prepareVectorRedraw` 里设置它的分支，`tikz.go` 恢复成原来那句默认续跑说明。
  - 文档全部改成事实（`docs/plan.md` P53、`AGENTS.md`、`docs/latex.md`、`docs/commands.md`、`CHANGELOG.md`）：继续跑/从记录继续 = **不发任何 user 内容**，历史原样续行；真跑完没 submit 时 T94 的"没提交"提醒才是唯一会插进去的 user 消息。
- **编号（用户第二问）**：P53 条目内部的 `P1/P2/P3` 分步改称 **「阶段一/阶段二/阶段三」**，并在条目开头写明"这里的「阶段」只是本条目内部的分步，不占 plan 的 P 编号"——plan 里 `P3` 是另一条老计划（把核心工具独立成组件库），内部分步借 P 号会撞车。同批把 `docs/agent-batch-history.md` 的 `P2 之二/之三/之四` 改称 `阶段二之一/之二/之三`、`CHANGELOG` 的 `P53-P2` 改称 `P53 阶段二`、Go 注释里的 `（P2）` 一并改掉（`P27` 是另一条，不动）。
- **P54 补记**：手动插入用户消息要**真发出去**，必须绕过 `resumeUserText`（`sess.Run(RunOptions{UserText: 用户的话})`），且只在这条手动路径上这么做——中断续跑仍保持"不发"。
- **验证**：`go build ./...` 通过；`go test ./internal/latex/ ./internal/sessionview/ ./internal/retryq/` 全绿（`retry_test.go` 的 map 断言换成 flag 语义）；无 web 改动，dist 未动。

## v1.5+（第四十七批补二十，P55 + T184：从某条记录继续，工作区也跟着退、核对也能切）

- **用户两问**：① "核对那边为什么没有加这个从某个记录的地方开始？不行吗？如果是正常的转换会话的话我想他从某次工具调用或者操作之后，后面乱码了我想重新这里跑的话，也是不行的吗？感觉这边你实现的不知道怎么弄得？"② "不是只回滚会话回滚会话的话可能会对模型造成困惑，这边也无需记录就是按照历史模型操作过的过程倒退着返回回去不可以吗？还是说需要额外增加实现部分？"——随后选定：**每轮 git 快照**（工作区外仓库、回退到那一刻），并**同意核对也加**。
- **第一问的答案（先查清再动手）**：转换会话的 ⤓ 每条记录都有（用户/助手/思考/工具回执都是 `t=msg`，工具**调用**行与它所在的 assistant 消息共用行号——从调用行切是"截到那条 assistant 消息之后、回执作废"，从回执行切才是"这次操作跑完之后"）；核对没有是**当时的取舍**（只读判断、重开一条等价，且核对尝试号由 `checker_rounds` 记账），不是做不到。真正的病根是第二问：**工作区没退**，模型一 `read_file` 就发现记忆与文件对不上。
- **第二问的答案（为什么"倒着撤销"不够）**：转录里只有 `write_file` 的**新**内容（覆盖前原文不在任何地方）、`edit_file` 只能反替换一部分、`bash` 完全不可逆（`mv`/`rm`/重定向/跑脚本），半途失败还会把树留在"既不是第 N 条也不是最后"的状态。要真回退只能有**那一轮的树状态**。
- **P55 实现**：
  - `internal/session`：`TranscriptWriter.lines` 计数（打开时按现有文件行数起算、所有写入统一走 `writeLine`——少算一行就会退到错的那一轮）+ `Lines()`；`Session.SetMessageHook(func(lines int))` 在**每条消息成功落盘后**回调（`appendTranscript` 里唯一的 choke point）。
  - `internal/latex/rounds.go`（新文件）：`roundLog` 给一章建一个**工作区之外**的裸仓库 `<proj>/work/rounds/<章>/repo.git`（每次命令显式 `--git-dir`/`--work-tree`，隔离宿主 `~/.gitconfig`、固定提交身份），每落一条消息 `git add -A && commit --allow-empty` 并把 `{line, sha}` 追加进 `journal-try<N>.jsonl`；编译产物（`*.pdf`/`*.aux`/`*.log`/`*.synctex.gz`…）写在仓库的 `info/exclude`（**不进模型工作区**，免得模型去改），不进快照但 `clean -fdx` 会删。
  - `continueFromChapter`：截断之后 `rollbackChapterWork`（按行号取**最近一张 ≤ N 的快照** `reset --hard` + `clean -fdx`）+ `archiveChapterProducts`（把已提交的 `.tex`/分片/`report.md` 挪进 `work/attempts/<章>/try<源尝试>/`——那一刻未必已提交，"未来"的产物留在原地只会把模型带偏）。降级：没有快照（老会话 / 宿主没 `git` / 递归子章节）→ 只回退对话并在日志里说清；单次失败只告警一次。
- **T184 实现**：`truncateCheckerTranscript`（前缀抄成 `checker_<章>.try<N>.jsonl`，号从 try2 起取第一个空号、内存里已分配的号也算进去，切在 usage/meta 行或越界都拒）+ `continueCheckerFrom`（设 `retryCheckerAttempt` → `recheckChapter`，产物与只读视图一字不动，旧结论 `copyFile` 成 `.tex.checker.try<N-1>`）；派发端 checker 带 `from` **不走** `forceCheckerRetry`（它会自己分配下一个号，与截断撞车）；服务端 `role=checker` 允许 `from`，源转录必须正好是这一章的**核对**转录（`transcriptBaseStem` 比对，校验顺序改成"形状 → 存在(404) → 归属(400)"以保住既有语义）；前端 `canCutFrom` 放开 `chapterRole==='checker'`，`cutFrom` 按类型选角色、confirm 文案分叉。
- **踩坑**：① `git init` 不接受 `--work-tree`（`fatal: 不允许 … 而没有指定 --git-dir` 的兄弟错误），裸仓库初始化必须单独一条命令；② 测试里想用 `ensureLogger` 这个不存在的方法——照 `memwatch_test.go` 用真 `logger.NewLogger` + `SetQuiet`；③ 服务端原有的 `convert_chapter_009` 用例期望 404，新加的"源转录必须属于本章"检查若排在存在性之前会变 400，所以顺序调成先查存在。
- **验证**：`GOCACHE=/var/tmp/dsh-gocache go test ./... -count=1` 全绿（新增 `TestRoundLogRollback`/`TestRoundLogMissingJournalDegrades`/`TestTranscriptAttemptParse`/`TestTruncateCheckerTranscript`/`TestPostRetryCheckerFromRecord`）；`npm test` 161 通过；web dist 已重建、`go build ./...` 绿。
  - **T185（兼容老会话）**：用户追问"如果历史没有那个 git 仓库的话就注意处理一下吧，两种模式？bash 的就没办法了……无法回退的话要增加一段话说明？"——定成**一种模式**（能退就退、退不了就只退对话），不做开关：老会话本来就不知道"工作区里后来的改动"与"那一刻"差多少，给它一个"半退"模式只会更困惑。`bash` 在沙箱里能写的只有工作树与它自己的 `/tmp`（会话私有草稿），工作树在快照里、`/tmp` 本来就不该被"退回"。实现：① 退不了时**连产物都不挪**（此前 `continueFromChapter` 无条件 `archiveChapterProducts`，会把已提交的 `.tex` 挪进 `work/attempts/`，而工作区还留着后来的改动——"工作区是新的、产物没了"的半新半旧状态），抽 `applyWorkspaceCut` 收口"退回 + 按结果决定是否挪产物"；② 老会话连空仓库都不建（先只读 journal，没有任何条目直接降级）；③ 页面提前说清：`POST /api/retry` 的回执新增 `workspaceSnapshotCovers` 判断，没覆盖就点名「⚠ 这条会话第 N 行之前没有工作区快照，只会回退对话——工作区里后来改动的内容会保留；要连文件一起干净重来请用「重试这一章」」，`⤓` 的确认框写同一句。测试 `TestApplyWorkspaceCutWithoutSnapshotKeepsProducts`（产物在、工作区不动、不建仓库）、`TestPostRetryFromRecordSnapshotNote`（有/无 journal 两种回执文案）。

## v1.5+（第四十七批补二十一，T186–T188 + P56/P57 立项：核对视图、核对与转换的配对、"总是 fallback"与打回理由审计）

- **用户这几问**：① 重试总按钮放哪了、为什么不放中栏、子章节为什么不能重试、子章节的核对会话没有重试入口、要二次确认与误触恢复（回到之前的会话、删掉重试的会话、历史里选一条作为当前）；② 从某条消息 fork 怎么操作、找不到入口、能不能编辑/删除历史消息再继续；③ "有一个矢量图总是 fallback"要查是谁；④ 父会话二次重试时子会话与核对会话该不该跟着切；⑤ 核对会话看到 `chapter_010_child_03/children/…` 与 `check:` 视角不一致，"很多 checker 都遇到过"；⑥ 查所有重试过的会话为什么被打回，是否误判；⑦ 核对是不是每次覆盖上一次、历史里看不到每次核对。
- **数据在哪**：用户更正——运行时目录是 `~/workspace/kaoyan/docvision`（下面有 `latex_project/`、`config.yaml`、二进制），不是文档里那个旧的 `/home/share/samba-share/PDF2MD`。这次审计全部只读。
- **T186（真缺陷，已修）**：核对视图把整个章节目录挂成 `parts/`，而递归/整合过的章节提交的是 `<章>/parts/children/…` 与 `<章>/figures/…`。留档的 49 条打回理由里 **18 条"`\input` 路径写错"、10 条"插图被漏掉"**，全是视图造成的假问题（例：主 tex 写 `\input{parts/children/x.tex}`、分片里第 15 行就有 `\input{figures/motif_….tex}`，核对却声称"分片里没有"）。修法：目录内容铺到视图根、整目录再挂 `<章>/` 与 `parts/` 两个别名、`<proj>/source/figures|images` **逐条并进去**（章节自带的与项目级的两批都要能读到）。测试 `TestCheckerViewMirrorsChapterLayout` 钉住 6 条路径；`TestCheckerViewHoldsTwoFilesOneFolder` 从"正好三项"改成"该有的必须有"——那条过窄的断言正是这个缺陷的卫兵。
- **T187（已修）**：章节树里核对与转换各取"自己最新"，同轮匹配不到就退回最新。改成"核对结论只对它核对过的那次提交有效"：同轮没有核对就留空、`checker.attempt < convert.attempt` 一律不显示；手工重新核对（号更大）照常显示。web 测试同步改。
- **③ 的答案（真数据）**：`source/progress_items` 里 `status=fallback` 的条目分两类——上游 **HTTP 500 `do_request_failed`**（偶发，重跑可能好），以及老版本写出的 **HTTP 400 `unknown variant \`disable\`**（`thinking.type` 笔误；当前代码已把 `disable` 归一成 `disabled` 并在配置加载时纠正/报错，属于历史事故）。**没有"某一张图恒定失败"的机制**：fallback 是"保留原图"的终态、不进重试；要重画用「重画这张图」，要确认身份看进度记录里的 `key`。
- **⑥ 的答案（真数据）**：那本书留档 49 条打回理由里 28 条属 T186 的假问题（18 路径 + 10 插图），其余是真缺陷（整段解析块缺失、脚注丢失、`♠` 记号丢失、空 `remark` 环境）与"核对多次未提交结论"。所以"很多重试不必要"的判断部分成立——假问题那 28 条本该零成本通过。
- **⑦ 的事实**：核对转录**按转换尝试**已有独立文件（`checker_<章>.try<N>.jsonl`），但**一次尝试内的多轮核对共用一条转录**（追加），`<章>.tex.checker` 只留最新结论 → 历史里看不到逐轮。登记 T188（排期）：逐轮转录或按轮切子条目 + 每轮结论落盘。
- **立项**：P56（中栏重试总入口 + 页面内二次确认 + 撤销请求/删除这次重试/历史行内"设为当前"、删除；子章节重试要连带父节点重新整合）与 P57（对话页每行 fork、fork 后自动切到新会话、删除到某条再继续、与 P54 的"插一句 user 消息"合流）。

- **③ 的精确答案（T189）**：全部项目 `progress_items` 里 `status=fallback` 共 41 条、**class 全是 vector**；36 条是 `[SESSION_INSUFFICIENT_BALANCE]`（额度耗尽、一次就停），5 条是 `open : no such file or directory`（**路径空串**）——而这 5 条**是同一张图**（同一个 `key`），横跨当前项目与最近几次留档运行，正是用户说的"总是 fallback"的那一张。`fallback` 是终态、重跑又被 `open ""` 打回，所以永远停着。待修项见 T189。

## v1.5+（第四十七批补二十二，P56/P57：中栏动作条、撤销、历史行内动作、对话页 fork）

- **用户的四个诉求**：重试总按钮不要放侧栏、要放中栏、要二次确认、误触要能恢复（回到之前那条、删掉重试出来的会话、历史里选一条当当前）；对话页找不到"从某条消息重试"的入口、fork 完要自动切过去。
- **P56（已实现 ①–④）**：新增中栏 `SessionActions`（挂在 `#center-header` 下方），按钮集合按当前会话从 `legacy/actions.ts` 的纯函数算——`retryTargetOf`（convert/checker、非 `_child_`）、`canContinueOf`、`canRedrawOf`、`canDropOf`；**页面内确认条**取代 `window.confirm`（文案由 `confirmTextOf` 单一来源给出）；排上队之后出现「撤销这个请求」与「回到之前那条」，撤销走新的 `POST /api/retry/cancel`（只删还没被消费的 `work/retry/*.json`，`.done.json` 留档不动，幂等，`--no-retry` 时拒绝）；撤不回来（已被消费）就提示去「历史」删那条新会话。详情栏原来的整块按钮与样式删除（避免两处入口打架）。「历史」页每行加「设为当前」「删除」行内动作 + 「当前」标；删除用行内确认条。
- **P57（已实现 ①③）**：fork 的判定与请求体抽到 `legacy/fork.ts`（`canForkFrom`/`forkPrompt`/`forkRequestOf`），提交入口 `web/src/fork.ts` 由轨迹页与**对话页**共用；对话页每条消息右上角悬浮 ⤓。fork 排队成功后 `watchFork()` 记下守望条件，下一次 `/api/index` 里出现同项目同章节同角色的更晚转录就自动 `selectSession`——用户口径"自动切换这个为主会话"。②（删除/编辑历史消息再继续）仍未做。
- **踩到的坑（真缺陷，已修）**：给每条消息外套一层 `.fork-wrap` 后，时间线的回合分隔线规则 `.stream > .msg + .msg` 失效——虚线整片消失。浏览器几何测试（`TestToolRowGeometryComesFromOneTokenSetInChromium`）当场抓住（8px + dashed + 8px 那一条不成立），补了跨 wrapper 的 `.stream > .fork-wrap + .fork-wrap > .msg` 同一条画法。

### 续：P57 ② 的落地方式（"删除某些错误消息让他继续"）

不引入"就地改写转录"的新能力——那要动 append-only 的源文件、还要给每一行重新编号。**等价动作 = 从这条之前 fork**：历史截断点落在前一行，这条及其之后的记录都在新尝试里不存在，源转录一字不动留在「历史」里。所以对话页每条消息多了第二个悬浮按钮 `⤫`（与 `⤓` 同一套判据、同一个排队入口，只是 `from = n-1`），第 1 条之前直接拒绝并说明"要从头重来请用「重试这一章」"。逐条**编辑**某条历史消息（改内容而不是丢弃）仍未做——那需要"改写一条 msg 行 + 保住后面的轮次"，与 fork 的语义不是一回事。

### T189：那张"总是 fallback"的图的真根因（空路径 + 续跑重放）

- **现象**：`status=fallback` 的进度条目里，除了余额不足那批，还有 5 条错误文本是 `open : no such file or directory`——**没有** `会话错误:` 前缀，说明不是会话/接口错误，而是 `RunTikZSession` 自己的 `error` 返回值；而且这 5 条是**同一个 key**（当前项目 + 4 次归档运行），即同一张图反复失败。
- **根因**：交付前 `copyFile(state.lastPDF, dstPDF)`。`state.lastPDF` 只在 compile 工具**本进程**编译成功时赋值；而续跑时 `resumeSubmit` 直接收账（`state.submitted = true`、`resumeSkip = true`），**一次编译都没发生**，`lastPDF` 是空串 → `os.Open("")` → `open : no such file or directory`。这条错误被 `pp.Error = err.Error()` 记进进度、状态写 `fallback`；下次运行 `pending` 又把它排进去（`status != "done"`），重放 → 再次空路径 → 再次 fallback，形成闭环。所以"重画这张图"也没用：重画同样从重放开始。
- **修法**：① 交付前判定 `state.lastPDF` 为空或文件不在，就按 `state.finalCode` 用**同一套** `buildStandalone` + `Compiler.Compile` 补编译一次，成功则用新产物；`finalCode` 也为空才报"没有可交付的图形代码"；② 补编译失败时报"交付前重新编译失败（<fig>）：<最后编译错误>"，不再泄漏系统错误；③ `copyFile` 对空源路径直接报"源路径为空，无法写出 <目标>"（`TestCopyFileRejectsEmptySource` 钉住）。
- **教训**：把"本进程内的临时状态"（`lastPDF`）当成"磁盘上的事实"用，就会在**重放路径**上踩空——重放只恢复逻辑状态（已提交），不恢复过程状态（产物路径）。凡是"交付/收尾"步骤依赖过程状态，都要能从磁盘事实重建。

### T188（B 方案）：核对转录按轮拆成历史子条目

- **为什么不切文件**：`checker_<章>.try<N>.jsonl` 这个名字是"一次尝试"的身份，被 `lastAttemptNumber`、历史聚合（`chapterAttemptChain`）、`/api/drop`（按 `.tryN` 连 work/attempts 一起删）、checker 重放（`replaySubmit`）四处依赖。按轮改名会把这四条同时改动，而用户要的只是"能看第 1 轮的详细拒绝理由"。
- **做法**：纯前端视图层切分。`checkerRounds(lines)` 扫一遍转录：user 行开新轮；submit 回执带 pass → 通过，带 issues/reject/problem → 拒绝并数编号行（`- ` / `1.`）；每轮记起始行号与行数，另存最后一条助手正文的前 120 字作摘要。历史面板只在该会话是 checker 且轮数 > 1 时渲染子列表，点击 `jumpToLine(from)`。
- **边界**：没有 user 行的旧转录整条当一轮（不显示子列表）；行号口径与 `/api/session` 的 `n` 一致，跳转不会串。
