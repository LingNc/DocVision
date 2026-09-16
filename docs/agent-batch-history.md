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
