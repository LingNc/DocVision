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

- 用户贴的两段都是 **Step 1 切分**的输出（img2text 根本没跑），差别在**状态**不在平台：Linux 那次 `files/` 里有 8 个 PDF（多出两本教辅 PDF，随后被归档进 `files/done/`），Windows 那次只有 6 个；缓存命中情况也不同。
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
- 测试：`chapter_checker_test.go` 2 例（退化会话→重试轮成功 / 两轮全退化→降级且请求数 ≤6）。

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
- 测试：style_fix_test.go 4 例（submit_problems/submit_blocks/submit 块结论/collectIssueReports）；全仓 go test 绿。
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
- **方括号书名 0 锚点**：`[正度纸] 某书甲`、`[a4] 某书乙` 的 content_list 用 filepath.Glob 找——方括号被当字符类静默失配，整本书 0 标记。改 ReadDir 按后缀挑选（跳 _v2）。
- **目录区标记爆炸 + 页码狂跳**（P.2 后直接 P.243）：目录行"第三章 认识论……029"包含匹配到 body 里同名章节标题锚点，单调指针被拖过全书。加 60 锚前向窗口 + 匹配页码不回退守卫——目录行继承未知页不产标记。复验：某书 89 处首几页 2,3,4,5,6,7,13…单调；另两本 576 / 273 处全部正常。
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
- **验证**：`go build ./...` 通过；`go test ./internal/latex/ ./internal/sessionview/ ./internal/retryq/` 全绿（`chapter_retry_test.go` 的 map 断言换成 flag 语义）；无 web 改动，dist 未动。

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

### P56⑤：子章节重试的边界就是"父节点有没有整合"

- **用户给的判据**：① 父节点还在等子会话 → 重跑这一支没有任何问题；② 父节点已经弄完（整合/晋级）→ 不能重跑；③ 已经通过的子章节本来就没人要重跑。真实动机是"会话出错要人工干预"，而那种场景多用 ⤫/继续跑，重试只用于确实要重做这一支。
- **实现**：服务端判据用磁盘事实而不是运行期状态——`work/chapters/<父章>.tex` 存在即视为父已整合（提交产物是整合的终点），这时 400 并指名"重跑父章节 X"；否则要求能找到子章节的输入（`work/chapters/<子章>.md` 或父节点 `conv_<父>*` 临时树里的 `_subdivision/**/<子章>.md`），找不到 404。这样判断与"预览页此刻显示什么状态"无关，重启后行为一致。
- **前端**：`retryTargetOf` 不再对 `_child_` 返回 null；确认文案专门说这条边界。测试从"不给按钮"改成"给按钮 + 文案含边界 + 服务端两条拒绝（父已整合 400 / 找不到输入 404）"。

### T190：历史操作的默认语义——就地改，fork 才复制

- **用户的原话**："对历史操作的时候不要默认 fork，默认是在这上面操作的，fork 是个手动操作的过程。防止每操作一次就增加一个，我可以手动先 fork 一个一样的然后再操作。"
- **为什么原先不对**：`⤫` 复用了"从第 N-1 条之后继续"的排队请求（开新尝试）。语义上它确实等价于"删掉后面再继续"，但**副作用是每次删都产出一条新尝试**，而用户的心智是"我在改这一条会话"。
- **做法**：新增 `POST /api/truncate`（`serveTruncate`）——就地截断，三条安全线：① 90 秒内有更新或有 `.partial` 就 409（和 `/api/drop` 同一条判据，避免改一条正在写的转录）；② 动手前整份备份到 `work/sessions/.trash/<转录>.<时间>.jsonl`；③ 临时文件 + rename，中途失败不留半份。`⤓` 保持 fork 语义不变。
- **边界**：只认 `work/sessions/*.jsonl`（`transcriptNameRe`），`line < 1` 400（要清空就删记录），`line >= 总行数` 回 `dropped: 0` 不动文件。

### T191（否决）：按轮删除为什么不该存在

- 实过一版（`POST /api/prune` + 中栏勾选面板），用户当场否掉，代码已 revert。**否决理由记在这里，避免以后又被提一遍**：
  - 删掉中间某轮时，转录少了那段上下文，而工作区/产物/快照仍是"后面那些轮次叠出来的结果"——两者不再自洽。要修就得连工作区一起回滚到那一轮、再让后面全部重跑，本质上等于重开一条会话，做成"删除按钮"只会让人误判成本。
  - 只有"截掉尾巴"天然安全：尾巴之后没有东西，所以对话、`work/rounds` 快照（P55）、已提交产物与归档能对齐。`⤫`（就地截断）与 `⤓`（复制成新尝试）都属于这一类。
- 顺带确认的格式问题：`docs/plan.md` 是扁平的 `[旗标] P<N>.` 列表，**不在列表项之外挂 `###` 小节**——设计结论要折回对应条目正文里。

### T192：重新核对与"父节点等子会话"解耦 + Glob `**` 的坑

- 用户一眼看出 P56⑤ 的判据挂错位置：*"核对会话的重新重试的那个核对？不需要父会话是否等待子会话合并之类的吧，我看你这边弄一起了？"*——对。
- **语义**：重核不动产物（只换核对会话、旧结论挪 `.checker.tryN`、视图重建），所以"父章节已整合"根本不是它的前置条件；父整合完了照样能重核这一支，结论仍然进 `历史 N`。只有 convert / continue 才要那条父节点判据（它们会改产物与工作区）。
- **两个真 bug**（写测试时暴露）：
  1. `filepath.Glob(join(..., "**", base+".md"))` —— `**` 在 Glob 里只是"一个路径段里的任意字符"，不是递归。父节点正在跑这一支时 md 在 `work/temp/conv_<父>/work/_subdivision/<子>/source/<子>.md`，永远匹配不到 → 误判"父节点既没整合也没有正在跑它的临时树"→404。改成 `filepath.WalkDir` 全树找。
  2. `recheckChapter` 把 md 路径写死成 `chapters/<章>.md`；子章节没这个文件，核对视图就少了 md（拿 tex/parts 去核等于换了判据）。新增 `chapterMarkdownFor()`：顶层直取，子章节在父细分树里找。
- 判据收敛成一句：**改产物的动作**（重转、继续跑）看父节点整合状态；**只看产物的动作**（重新核对）只看"产物在不在"。

### T193：入口与位置的三处硬伤

- 用户连续三条反馈："我章节会话去哪了"（截图里章节行显示成 `核对 · chapter_002`，下面又有一行 `核对 核对 · chapter_002`）、"上面这个位置不太好"（动作条独占一行）、"正常会话的 fork 按钮在哪里 / 对某条消息后面删除在哪里操作？怎么进入这个模式？"。
- **① 树行归属**：`buildChapterTree` 里 T186 的"同一轮没有另一种角色就留空"被对称地用在了 `convert` 上。核对号可以比转换号大（手工重核就是），那时 `node.convert` 被置空 → `primary()` 落到核对会话 → 行头变 `核对 ·`，而 `hasBody` 看到 `node.checker` 又把同一个会话渲染成子行。修法：`convert` 方向不置空（它是"这一章是谁转的"），`checker` 方向保留留空（旧结论不摆到新转换旁边）；`hasBody` 要求 `node.convert && node.checker` 才单列核对行。单测覆盖 pin 与不 pin 两种情况。
- **② 动作条**：拆成 `SessionActions.vue`（顶栏按钮）+ `SessionActionPanel.vue`（确认条 / 回执），共享 `src/actions.ts`。CDP 实测：`#header-actions` 里顺序是「重试这一章 继续跑 删除这条记录 41 条消息 · 62 行 · 66.1 KB」。
- **③ `⤓` / `⤫`**：真 bug ——`.fork-btn { opacity: 0 }` 与 `.fork-acts { opacity: 0 }` 叠加，**永远不可见**（连 hover 都只是把容器提到 1、按钮自己仍是 0）。改成只由容器控制（0.45 / hover 1），并给 `.fork-wrap` 加 `padding-right: 54px` 留出窄栏——原来绝对定位在 `right: 0` 压在行尾 token 计数上。
- 验证：`npm test` 187 通过；用仓库内构建的二进制 + 一份 `/tmp` 里的项目副本起只读预览，CDP 读 DOM（`forkOpacity=0.45`、`elementFromPoint` 命中 `fork-btn`）并截图目检。

### T194：「历史」里的当前/最新/归档，以及两个 `#1` 的真凶

- 用户发来「历史 2」的截图：`#2 当前 · 运行中`、`#1 已完成`，同时说"我点那个就是当前、这个当前的标示不对""设为当前的话那个就乱了""我刚刚对我的一个进行操作，就全乱了，显示两个 #1 #1"。
- **两个 `#1` 不是重复副本，是两段不同的对话共用了一个号**：作废尝试用 `archiveChapterAttempt` 把 `work/`（含 `_subdivision/` 子树）整棵搬进 `work/attempts/<章>/try<N>/`，而扫描只按扩展名收 `.jsonl`，搬家后的转录照样进索引。把真实项目复制一份起服务核对：635 条转录里 **84 条在 `work/attempts/` 下**，按（项目 + 章节 + 角色 + 尝试）分组有 **26 组**与工作树那条同号。**先按"副本"处理，是错的**——逐字节比对（`sha256` + 体积）发现两边完全不同：父尝试作废后这一支重跑，文件名里的尝试号**又从 1 重新数**，所以归档那份是"上一次父尝试里这一章的 #1"、工作树这份是"这一次父尝试里的 #1"。教训：同号 ≠ 同一份，别拿路径规律当内容判据。修法随之改成两层判据：扫描打 `archived`（`SessionInfo.Archived`）；`dedupeChapterAttempts` **只在内容指纹（`sha`）相同时**才去掉同号行（真被扫两遍那种），内容不同则两条都留，归档那条由 `chainMemberTag` 标成 `旧#N`、行上另挂「归档」徽标，历史列表头顶补一句说明。把旧的那条直接藏掉会丢掉一次真实对话与那次作废的证据。
- **「当前」混了三件事**：中栏正在看的、流水线现在用的、最新一次尝试。现在三枚不同的标：`正在看` / `最新`（只数还在工作区里的，归档那条的号可能反而更大）/ `归档`；「设为当前」改名「查看」（点行本来就是切视图，名字让人以为会改重试目标）。顺带确认：**页面没有"归档"这个操作**——归档只由流水线在"核对判不过、作废这次尝试换新会话"时做，用户界面上根本点不出来，所以"我点了一下就变成归档了"这种自述一律先怀疑时间线。
- **成对切换**：转换与同一轮核对是同一件事的两个角色，历史每行加「同轮核对 / 同轮转换」直达对方那一轮；`重新核对`/`重试` 排队成功后复用 P57 的守望条件（同项目同章节同角色的新转录、且 mtime 晚于排队时刻，20 分钟作废）自动切过去。**踩到并修掉的副作用**：`actions.ts` 里"换会话就清空 queued"会在自动切过去的那一刻把「撤销这个请求」收走，等于刚排上就撤不回来——改成"切到的正是这次排队的那一章那一角色就保留"。
- **"点一下 `#2` 就变成 `#1`"的真因是分链**：历史列表原来只列"当前会话那个角色"的尝试——转换有 `#2/#1`，核对只有 `#1`。点一条核对会话，整列换成核对的链，编号全落到 `#1`，看起来就像那条 `#2` 被改名了。用户描述的"对某个 `#2` 点设为当前就变成 `#1`"就是这么来的（CDP 复现：同一次点击前后 `#2 #1 旧#1` → `#1 旧#1`）。修法：`chapterAttemptChain` **不再按角色过滤**，这一章的转换与核对混在一条链里（本来就该如此——"这俩是一体的"），行首标「转换 / 核对」，编号仍取文件名里的尝试号，点任何一行列表都不再变。单测钉住"从任一条进去拿到同一份列表"。
- 顺带：面包屑把章节 ID 交给行内 Markdown，`chapter_004_child_02_child_01` 渲染成 `chapter004child02child01`（下划线被当强调），补 `:plain="plainTitle(state.current)"`，与侧栏/历史一致。
- 验证：`go test ./...` 全绿（新增 `TestScanMarksArchivedCopies`）、`npm test` 194 绿（新增 `t194-attempt-switch.spec.ts` 4 条 + 去重 3 条）；真实项目转录副本 + `dvmine8 sessions --serve` + CDP 复核：重复章节不再有「历史」页签、`#2/#1` 各一条且 `正在看`/`最新`/`归档` 正确、点「同轮核对」切到核对会话、排队后新建转录出现即切换且「撤销这个请求」仍在、撤销后 `work/retry/` 清空并跳回原会话。

### T195 · 转换行别再挂「重试这一章」的悬浮 ↻（2026-10-21）

- 用户截图：鼠标停在转换行上，行尾浮出 ↻（重试这一章）与 ⓘ。要求"不要放这里了"。
- 这是 T193 之后的重复入口：动作条已经在顶栏右侧放同一个「重试这一章」，行尾那个既重复、位置又像状态徽标（一误点就开新尝试、旧产物归档），撤掉。
- 只撤**转换行首**的 ↻；核对子行的 ↻ 是另一种动作（重新核对：产物一字不动），保留。「待重试」徽标（纯状态、不可点）不受影响。
- 验证：`npm test` 195 绿；重建 dist 后产物里 `chapter-retry icon-btn` 只剩 1 处（核对那条），`重试这一章` 文案不再出现在章节树里；`go build ./...` 通过。

### T196 · 递归子章节也能"删掉这条及之后、再接着跑"（2026-10-21）

- 用户的现场：一条三级递归子章节的转换会话因网关 `HTTP 400 System messages are not allowed` 终止，一条同级核对会话被"还没提交"提醒追着走、最后 `submit` 自己报错。他要在页面上"删掉这条及之后、再继续跑"，被告知"这是你之前写的功能"——查下去发现**子章节根本用不了**。
- 三个断点：① `canForkFrom()` 把 `_child_` 一刀切排除，对话页不画 ⤫（可是就地截断只需要转录文件）；② `/api/truncate`、`/api/drop` 只认 `work/sessions/<名字>`，子章节转录在 `work/temp/conv_<父>/work/_subdivision` 下的 `node/sessions/`，请求 404；③ 子章节的 `continue`/`convert` 请求进了 `work/retry/` 就出不来——`retryPrecondition` 找 `chapters/<子>.md` 与 `work/temp/conv_<子>/work` 都找不到，页面只显示"排队中"，看起来像按钮坏了。
- 修法：判据拆成 `canCutFrom()`（就地截断，允许子章节）与 `canForkFrom()`（fork 仍只给顶层）；服务端 `transcriptUnderWork()` 按绝对路径解析并限定 `<项目>/work/` 之下的 `.jsonl`（裸文件名仍走 `work/sessions/`），路径不合法 400、不存在 404；子章节会话的「继续跑」换成它的顶层父章节（`continueTargetOf()`，文案「继续跑父章节」）——父章节接着跑不新建尝试、不归档，重新驱动子树时每一支按自己磁盘上那条转录续跑，于是"截断 + 继续跑父章节"= 从截断处接着跑；派发端收到子章节的 `convert`/`continue` 直接消费并写清怎么做，不再无声滞留。
- 两条现场的归因：转换那条的 400 是网关侧返回的（同一会话此前请求都正常，前缀没变，属于换一次路由再试）；核对那条是模型连着多轮没调 `submit`、被"没提交"提醒追着走，截到第一次提醒之前再继续能让它带着干净上下文重判。
- 验证：`npm test` 197 绿（新增两组单测：子章节可截、继续跑换父章节）；`go test ./...` 全绿（新增 `TestPostTruncateDeepTranscript`：细分树里的转录能就地截断、备份仍落 `work/sessions/.trash/`、工作区外 400、`../` 穿越拒绝）。

### T197 · 核对会话的确认文案与子章节的产物路径（2026-10-21）

- 用户的反馈很短：截图里站在 `核对 · chapter_014_child_04` 上，确认条写着"重试这个子章节？只在父节点还在等子会话时有效……父章节一整合完，服务端就会拒"，他问"核对会话不应该提示这个吧"。确实不对：重核一个字节的产物都不动（T192 已把"父节点是否已整合"这条判据限定在会改产物的动作上），文案却把转换口径套在了核对会话上。
- 根因是判断顺序：`confirmTextOf('retry', s)` 先看 `s.chapterId` 里有没有 `_child_`，命中了就返回转换那套说法，`chapterRole === 'checker'` 那一支根本轮不上。改成**先分角色、再看层级**；核对会话对子章节补一句"重核不改产物、父章节整合完同样可以"。
- 顺着这条又挖出一个真断点：三处"有没有已提交产物"的判据都写死 `work/chapters/<章>.tex`，而递归子章节的产物在它上面某一层的 `work/chapters/<父>/children/<子>.tex`（层数不定，且暂存目录 `children.tmp` 里有同名半成品）。于是子章节的「重新核对」要么 400"这一章还没有提交产物"，要么排队后永远等不到前置条件——按钮看着有、点了不行。现在 `chapterProductsFor()`（latex）与 `chapterTexIn()`（预览服务）按文件名在 `work/chapters` 下解析、跳过 `children.tmp`，`recheckChapter` 与 `/api/retry` 前置条件都用它；子章节的编译产物 PDF 也从细分树的节点工作区（`work/temp/conv_<顶层>/work/_subdivision/**/node/work/.gate/compile.json`）取，核对视图仍带产物页。
- 验证：`npm test` 198 绿；`go test ./...` 全绿（`TestChapterProductsFor` 钉顶层/两层/三层路径与暂存目录排除，`TestPostRetryCheckerChildProduct` 钉"有细分树产物 200、只有 children.tmp 半成品拒绝"）。

### T198 · `convert_concurrency` 调了没用：批次宽度还是 `concurrency`（2026-10-21）

- 用户把 `latex.convert_concurrency` 调到 15，看日志发现启动账目写"转换 15 / 核对 15（独立槽位）/ 其它阶段 5"，但转换阶段自己打"[convert] 26 章，并发 5"，问"实际并发跑到 15 了没有"。
- 根因：`convertChapterBatch()` 的批次宽度写死 `r.cfg.Latex.Concurrency`；T179 加的 `convert_concurrency` 只喂给了会话槽位池（`acquireConvertSlot`）。同一时刻只有 5 个 worker 在跑，池子 15 永远到不了上限——两处数字不一致，用户看到的"调了没反应"就是这个。
- 实测（本次运行 21:50 起，从各转录的 `chapter_activity` running→结论 区间做重叠统计）：转换峰值 6 个会话（21:55:24）、核对峰值 4 个（21:51:31），合计 10，确实没到 15。
- 改：批次宽度改取 `ConvertWorkers()`，并收成 `convertBatchWidth()` 一处（启动账目里的"转换 N"用的是同一个 `ConvertWorkers()`，不会再走岔）；日志行前缀改成"（latex.convert_concurrency）"。子节点信号量本来就是 `ConvertWorkers()`，未动；核对会话是逐章内联跑的，批次宽度上去之后 `checker_concurrency` 才真能到 15。
- 顺带把口径写清（用户问"哪个数管什么"）：`latex.concurrency` = 其它阶段的 worker；`latex.convert_concurrency` = 同时转换的章节数与转换会话槽位；`latex.checker_concurrency` = 核对会话槽位；`models.<名>.max_concurrency` = 真正同时打到同一个模型（wire 名）的请求数，未写 = 不限。
- 验证：新增 `TestConvertBatchWidth`（四种写法都要与 `ConvertWorkers()` 一致）；`go test ./...` 全绿、`go build ./...` 绿。

### T199 · 样式会话的图片为什么又没了：清理脚本删了它的附件目录（2026-10-21）

- 用户发来样式会话的截图，`view_pdf` 缩略图全是断图，问"media 里的图片怎么又没有了？被谁删了？我今天就执行过一次删除了一个核对会话"。
- 先自证清白：`/api/drop`（删一条记录）只删那一条转录 + `work/attempts/<章>/try<N>/`，`/api/truncate` 只截断文件，Go 代码里没有任何一处 `RemoveAll`/`Remove` 落在 `media` 上。再按项目里所有转录的 `file://media/<hash>` 引用逐条核对：5493 张附件里只有 `work/style_session.jsonl` 的 31 张缺失、`work/media/` 整个目录不在——只影响样式会话，与"删了一个核对会话"无关。
- 真凶：会话写图片的目标是**转录同级目录的 `media/`**（`session.NewTranscript` 里 `mediaDir: filepath.Join(filepath.Dir(abs), "media")`），所以 `work/media/` 归属 `work/style_session.jsonl`。而 `clean_convert.sh`（T167）的删除清单里有 `work/media`、保留清单里却有 `work/style_session.jsonl`——脚本把留下来给用户看的会话的附件删了。旁证：项目同级留有 `*_记录_20261020_0737/`（脚本先整目录备份再删），那 31 张正好都在它的 `work/media/` 里，时间戳与删除时间一致。
- 改：`clean_convert.sh` 不删 `work/media`；`clean_project.sh` 只在 `--with-style`（连样式会话一起删）时才删 `work/media`；两处都写清了"media 是转录同级附件目录"的理由；`work/sessions/media` 也不能整体删——被保留的 `chapters.jsonl` 用它。`docs/latex.md` 的脚本章节同步。
- 数据恢复：从 `*_记录_20261020_0737/work/media/` 把缺的 31 张补回项目 `work/media/`（只补缺、不覆盖），复核后附件缺失数归零（存在 5544 / 缺失 0）。
- 验证：`bash -n` 两个脚本；在临时假项目上 `-n` 跑一遍，删除清单不再包含 `work/media`（`work/views`/`work/pdfview` 仍删，那是每次运行重建的）。

### T200 · 侧栏里的"空会话"与"备份项目"：快照日志被当成转录（2026-10-21）

- 用户发两张截图问"有一堆这些会话是什么？"：转换阶段下并列 14 个方块、点开标题是 `journal-try1`、轨迹页空白；侧栏还多出一个挂着 76 个会话的组。
- 查盘：那 14 条是 `work/rounds/<章>/journal-try1.jsonl`——P55 工作区快照的"转录行号 → commit"日志（内容只有 `{"line":N,"commit":"…"}`），项目里正好 14 章有快照，与方块数一致；它跟会话毫无关系，只是扩展名也是 `.jsonl`。76 会话那个组是清理脚本的整目录备份 `<项目名>_记录_20261020_0737/`（先备份再删的产物），整份工程副本被当作独立项目扫了进来。
- 改：`sessionview` 新增 `skipScanDir(p, name)`（主扫描与附加根扫描共用的唯一判据）——`media`、`.git`、`*.git`（`work/rounds/<章>/repo.git` 是裸仓库）、`work/rounds`（限定父目录是 `work`，免得误吞名叫 rounds 的书）、`cleanBackupRe`（`_记录_<8位日期>_<4位时刻>` 结尾）整棵跳过。
- 验证：`TestScanSkipsRoundsJournalsAndCleanBackups`（真转录留、快照日志/裸仓库/备份副本都不出现）、`TestSkipScanDir`（规则边界）；`go test ./...` 全绿；真实输出根 `sessions --list` 1005 条、`journal-try` 命中 0、无 `_记录_` 组。

### T201 · 乱码会话、子章节的产物/输入/转录解析、核对会话的"从记录继续"（2026-10-22）

- 用户三连问：*"有的转换会话我看输出乱码了，我想让他重跑一下。但是会报错。有的会话截断消息可以，但是有的会话不行。比如核对会话"*，紧接着是核对会话的截图 *"就是这个核对会话没办法重新转换"* + *"失败：这一章还没有提交产物，无法重新核对（先重试转换）"*。
- **乱码是"模型把工具调用写成正文"**：转储项目里全部转录（跳过 `.trash`）后发现 10 条会话有同一种形态——assistant 的正文以 `{"paths":["skills://","skills://plugins"]}` 开头，跟着伪 `bash -lc …`，再声明"the chapter workspace tools are not available in this session"。这些消息**没有 `tool_calls`**（工具一次都没执行），后面两轮"必须 submit"提醒收到的是逐字不同的同类垃圾，最后 `run_end ok=true`、章节没有产物。同批用量行：厂商只报 2472/2530 prompt tokens，本地估算 8031/8056（约三分之一）——请求在网关那侧被改小了，模型拿不到完整系统提示与工具表。以前 `observePromptSize` 把 ratio 夹到 ≥1，这种异常完全静默；`remindSubmit` 只会再问两遍。
- **子章节产物在临时树里**：`chapterProductsFor`（Go）与 `chapterTexIn`（预览服务）只认 `work/chapters/`。父节点整合前，子章节产物在 `work/temp/conv_<根章>/work/<父章>/children/<子>.tex` 或自己那一支的 `node/result|integrated-stage/<子>.tex`。真数据里 `chapter_014_child_04` 转换完、核对也跑完了（135 条消息），点「重新核对」只得到"这一章还没有提交产物"。
- **子章节输入的真身是 `input/chapter.md`**：`findChildMarkdown`/`childMarkdownInTempTree` 只找 `<章>.md`，真数据里能命中纯属侥幸——细分树里只有核对视图那份同名副本，视图一清就"找不到章节 markdown"。而且只比文件名会串层级：`chapter_014_child_04` 被认成 child_01 底下那一支的 child_04（真数据里就这样认错过，md 指到了 put 错的输入）。
- **"从记录继续"写死 `work/sessions/`**：`/api/retry` 的 `from` 分支拿 `filepath.Base(req.Path)` 去 `work/sessions/` 找；`truncateCheckerTranscript` 同样。子章节的核对转录在 `node/sessions/`，于是截断（就地改，`/api/truncate` 走的是 `transcriptUnderWork`）能成、⤓ 从记录继续一律 404。页面上 `canForkFrom` 又对所有 `_child_` 一刀切，子章节的核对会话连 ⤓ 都不出现。fork 请求只发文件名，也是同一个坑。
- **子章节重跑没有输入是死路**：输入不在、父节点没整合 → 404"先在父章节里重跑这一支"，用户点"重试"什么也不会发生。
- 改：`session.LooksLikeToolProseLeak`（当时判失败、**T205 已撤成只告警**）；产物/md 解析补齐临时树并按层级链整段比对；`retryPrecondition` 走同一 md 解析、工作区判据认细分树节点；`from` 分支用 `transcriptUnderWork` 解析并按相对路径入队、`truncateCheckerTranscript` 同口径；`canForkFrom` 对核对放开子章节、fork 请求发完整路径；子章节输入不在时改为排队父章节 continue。
- 验证：新增 9 个 Go 测试（检测、临时树产物、前置条件、层级精确、子章节核对 from、子章节 continue 回退、父无转录仍 404）+ 2 个 web 断言（子章节核对可 fork、fork 带完整路径）；`go test ./...` 全绿、`npm test` 199 绿、重建 dist；真实项目只读复核三支子章节的产物/md/前置条件；临时副本上用 HTTP 复核"有临时树产物 → 200 排队重新核对"与"输入不在、父有转录 → 200 改排父 continue"。

### T202 · 截断备份（`.trash`）不再被扫成会话（2026-10-22）

- 用户截图问：*"还有就是截断之后的会话怎么还会在这里显示？在 .trash中的"*——侧栏转换阶段下多出一条 `chapter_022_child_08.jsonl.20261022-122539`。
- 原因：就地截断（⤫）把原文整份备份到 `work/sessions/.trash/<转录>.<时刻>.jsonl`，扩展名还是 `.jsonl`，扫描按扩展名认会话，于是这条备份被列成一个新会话（标题就是那串后缀，点开是截断前的旧对话）。T200 的 `skipScanDir` 补了 `media`/`.git`/`work/rounds`/`<名>_记录_…`，漏了 `.trash`。
- 改：`skipScanDir` 增加 `.trash` 整棵跳过（主扫描与附加根扫描共用）；要看截断前的原文，用 `docvision sessions --dir <项目>/work/sessions/.trash`。
- 验证：扫描测试补两处 `.trash`（`work/sessions/.trash/` 与细分树 `…/work/.trash/`）、规则测试补两条边界；`go test ./...` 全绿；真实输出根 `sessions --list` 1029 条、`grep` 命中 `.trash|jsonl.2026|journal-try` 为 0（截断备份实际只有 1 份，正是截图那条）。

### T203 · 核对会话别把 markdown 当权威（2026-10-22）

- 用户反馈：*"有的这个核对会话好像会太过于依赖markdown的转换内容……他只是ocr的一个转换参考"*，并要求"核对的提示词少即是多，不要增加太大量篇幅"。
- 现象与根因：核对提示词虽已写了"md 是 OCR 来的、看着奇怪去看原页"，但判据仍是"md 有、`.tex` 没有 = 缺内容"，开场又把 md 称作 "the source it was converted from"——于是核对会话拿 md 的差异当事实，报出原书根本没缺的东西，转换会话被逼着改坏正确内容。
- 改（只两处措辞、不新增段落，净增约 30 词）：① md 那一条改成"a reference, not the truth. … where the page and the markdown disagree, the page wins and the .tex may follow it"；② "缺内容"那一类补 "confirmed on the original page, since a markdown-only gap proves nothing"。
- 验证：`go test ./internal/prompts/` 绿；模板仍 27 行。

### T204 · 核对会话的工具轮次随层级加宽（2026-10-22）

- 用户截图 `ROUND BUDGET: 35 of 50 tool rounds used, 15 left`，要求：*"随着层级越高每层增加20轮吧，线性方法增加或者倍数增加，并且在配置文件中可以修改，这边默认是50"*。
- 现状：所有层级的核对会话都是固定 50 轮。但父节点的核对要通读整棵子树（每个孩子都要过一遍），最里层只管自己那一片——父章节核对到 35/50 就被迫收尾，子树里几段内容根本没翻到。
- 改：`sessions.checker.max_tool_rounds` 改指**最里层**的预算；往上每层 +`max_tool_rounds_per_level`（默认 20）；要倍数增长配 `max_tool_rounds_factor`（默认 1=纯线性，2=每层翻倍；per_level 写负数 = 只翻倍不另加）。公式 `rounds = base × factor^levels + per_level × levels`，levels = `maxChapterSplitDepth - depth`，上限 1000 轮。层级从章节 ID 数出来，不动调用链。
- 验证：config 侧 `TestToolRoundsAtLevels` / `TestCheckerSessionDefaultsPerLevel`，latex 侧 `TestChapterDepthOf` / `TestCheckerRoundsScaleWithDepth` / `TestCheckerRoundsDoubling`；模板键面 parity 测试绿；`go test ./...` 全绿。

### T205 · 撤掉"按正文特征判失败"，改回只告警（2026-10-22）

- 用户追问 T201 那条检测：*"确定不会误判吗？是必须要加的吗？我感觉可以人工手动这边重试就行，因为有各种各样的错误。每个厂家都不一样。"*
- 复盘：那条判据是**软判据**——`skills://`、`tools are not available in this session` 只是字符串，模型正常解释"这个会话里没有某个工具"就会被打死，异常形态也不止这一种；而且判失败本来就不是必要的，"这一轮有没有产出"用户看页面就知道。T201 真正有价值的部分是**让人看见**，不是替人判失败。
- 改：判失败撤掉，改成每会话一条警告（含厂商报的 prompt tokens 与本地估算）；T201 另加的"厂商 prompt < 本地估算 60% 告警"也撤掉（各家计数口径不同，单看比例会误报），两个数字并进同一条警告。行为回到 T201 之前。
- 验证：`TestToolProseLeakWarnsButDoesNotFailRun`（退化回复正常返回、不重发、只置位一次）、`TestToolProseLeakWarnsOnce`；`go test ./...` 全绿。

### T206/T207 · 控制台实时行的时序、以及页面「是否在跑」的判据（2026-10-22）

- 用户反馈两件事：*"这个cli界面很多显示的不全面或者有点乱。正在运行的和排队的，还有之前开子会话又回来续上的"* + 一屏实时行；*"web界面对于是否正在运行的显示感觉也有很多问题"*。
- **CLI 三个问题**：① `ResumeStats.Active` 是"首行→末行"的墙上跨度，隔几天续跑的会话（实际只跑四十来分钟）写成 `2628m56s`；② 实时行在拿到并发槽之前就建好，排队 / 父节点等子会话（十几二十分钟）/ 真正在跑都长成 `轮次 0 · 工具调用 0 · 已用 25m46s`；③ 续跑的行从 `0` 开始爬，要等第一次进度回调才跳到真实数字。
- **页面问题（实测定位）**：真实输出根 1044 条会话只有 1 条 live，而同一时刻 10 条转换 + 1 条核对在写（4 个 `.partial`、23 条转录 3 分钟内有写入）。根因是 `activityAfterEnd` 只看"出现过 run_end"，而一条转录里会跑好几轮 Run，上一轮的结局把这一轮的活动标记折成「已中断」，scan 又对 `interrupted` 强制 `live=false`。
- 改：`Active` = 各用量行 `duration_ms` 之和；`fmtDuration` 超 1 小时按 `h`；实时行加状态词（排队/运行/等子会话 N/待核对，只有运行让计时器走，切换时结账累计）；续跑行首标「续跑」并用转录里的轮次/工具数/累计时间初始化；转换与核对接上状态切换（含提交提醒轮、父节点等子会话再整合）。页面侧按**行号**比较 `chapter_activity` 与 `run_end`，run_end 之后又出现消息时那条结局不再计入。
- 验证：9 个新测试（`TestFmtDurationHours`、`TestLiveRowTextStates`、`TestResumeActiveSumsRequestDurations`、`TestResumeActiveWithoutUsageIsZero`、`TestActivityInterruptedOnlyWhenRunEndIsNewer`、`TestRunEndErrorClearedByLaterMessages`、`TestScanLiveWhenRunningActivityIsNewest` + 两个既有测试改口径）；真实数据复扫 live 1 → 11；`go test ./...` 全绿。

### T208 · 「重试这一章」点了没有反应（2026-10-22）

- 用户反馈：*"还有一个17章节我点那个重试该章节也没有用来着，因为它乱码了我想让他重新跑一下"*。
- 查证：服务端正常——同样的 `POST /api/retry`（`chapter_017`、`role=convert`）返回 200，队列文件 5 秒被消费、旧尝试归档 `work/attempts/chapter_017/try1/`、新转录 `convert_chapter_017.try2.jsonl` 带 `manual_retry` 当场开写；而用户点的那次队列里既无待办也无 `.done`，归档目录也不存在 → POST 没发出去。
- 根因：动作条按钮只打开**确认条**，请求在「确认」才发；`actions.ts` 的 `watch(state.current.id)` 在任何会话切换时清空 `pending`——重试排队后视图自动跳到新尝试（T194 的行为）或实时跟随切走，确认条被静默收走。
- 改：`legacy/actions.ts` 新增纯函数 `sameActionTarget`（同项目+同章节+同角色 = 同一件事），确认条只在目标变化时收走；重画/删除仍一切换即收。
- 验证：`session-actions.spec.ts` +3 条，`npm test` 202 绿；`npm run build` 重出 dist。

### T209 · 核对会话"把 submit 写成正文"的空转环路（2026-10-22）

- 用户反馈：*"并且我无法重试或者删除这个checker_4的这个核对会话，因为这个核对会话有问题"*（截图里回执是「失败：HTTP 409」）。
- 三条链一起查：① 删除 409 是因为会话正在写（实测 mtime 距现在 13 秒）——保护正确，但页面把 `http.Error` 的**纯文本**原因丢了（`res.json()` 失败就退回 `HTTP 409`）；② 重试 400 是因为父节点产物要过核对才从暂存区晋级，核对没过时 `work/chapters/<章>.tex` 不存在（鸡生蛋）；③ 环路根因是核对会话把 submit 写成正文（`\submit{ "report": … }`，`finish_reason=stop`、`tool_calls` 空），那段"没结论"的失败文本被当成"核对发现问题"喂回转换会话，转换只能再交一遍同样的产物。实测 13:31→13:48 每 30–100 秒一轮。
- 改：`postJSON` 统一写入口并优先显示服务端说明；提交提醒写明"写进正文不算，要发真的工具调用"；`checkerNoVerdict()` 把"会话自己没给结论"与"章节真有问题"分开，前者跳过反馈轮（判据是 submit 回执这个事实，不是正文形态，与 T205 同口径）。
- 验证：`TestCheckerNoVerdict` + 既有测试，`go test` 四个包全绿、`npm test` 202 绿、`dist` 重出。

### T210/T211 · 拆分父节点的暂存产物、子章节重试的去向（2026-10-22）

- 两条用户反馈连着查：① 拆分父节点的「重新核对」报"还没有提交产物"，而活的核对会话正在读同一份 `.tex`；② 子章节的重试回执说"已排队"，刷新后会话原样不动。
- ① 的产品在 `work/temp/<章>/integrated-stage/` 等核对通过才晋级，判据（`chapterProductsFor`、`chapterTexIn` → `tempProductFor`）只认 `work/chapters/**` 与子树里的 `result|integrated-stage`，不认节点自己这一层——同一份文件两套结论。两处都补上，晋级后仍优先用 `work/chapters/`。
- ② 队列里两条子章节请求被消费成 `.done`，那一支的转录与 `work/attempts/` 一个字节没变：接口侧（T201）放行并排队，派发端（T196）消费掉只写日志，回执与事实相反。改成接口与派发端统一：子章节的重转/继续跑翻译成父章节的「继续跑」，回执点名父章节；父章节无转录时 404。
- 验证：`TestChapterProductsFindsStagedParentProduct`/`TestChapterProductsPrefersPromotedProduct`/`TestChapterTexInFindsStagedParentProduct`/`TestChapterTexInIgnoresEmptyStagingDir`、`TestChildChapterParent`/`TestQueueParentContinueFor`、`TestPostRetryChildBecomesParentContinue`；旧断言跟着新语义改（父已整合/临时树两条由 400/单独排队改为父章节继续跑）。`go test ./internal/...` 全绿。

### T212 · `request_chapter_split` 第一次总被打回（2026-10-22）

- 用户反馈：*"很多会话都会请求两次 request_chapter_split，这个第一次总是报错"*。
- 转录统计（1137 条会话、173 次调用）：21 次第一次被拒，全是 `ranges stop at line N; expected full coverage through line N+1`。根因是行数口径：`len(strings.Split(md, "\n"))` 把末尾换行多算一行，模型按 `wc -l`/`grep -n` 看到的正文行数切分，就差那一行。
- 改：`lastContentLine()` 取最后一条非空行，校验按它收口（`next == lastContent+1` 与 `next == totalLines+1` 都接受）；`read_file` 整文件回执加 `M lines`。真机重放：被拒的那次原始参数（`total=1672 lastContent=1671`）现在直接 `SUBDIVISION REQUESTED: 7 child ranges persisted`。
- 验证：`TestValidateChapterSubdivisionsAllowsTrailingBlankLine` / `TestLastContentLine` + 既有分段测试；`go test ./internal/latex ./internal/session` 绿。

## 第五十八批：未提交收尾的复盘会话（P58，2026-10-22）

用户口径：「我们的模型有时候会在没有反馈的时候就是意外的中间终止就是没有最终 submit……只是发送 not submit 并不能让模型正确的继续，因为很多时候都是模型已经开始输出错误的乱码，或者 json，或者 xml 的工具等等」——所以要一个**只读的复盘会话**来看那条会话的历史，判断"还能救（发提醒）"还是"已退化（从最后一个干净轮次截断）"，它自己的会话存到单独的「错误处理」分类里、默认只在 debug 下存 jsonl。

**设计定稿（用户确认的三点）**：① 模型条目不跟随 checker，而是**单独新建 `models.rescue` 条目**（模板里 `extends: text`，默认与 checker 同底座，方便单独调）；② 记录默认**只写一行审计日志**，`--debug` 才落 jsonl；③ 三步一次做完（retry / truncate / fail 全上，convert 与 checker 都接）。

**做了什么**：
- `internal/latex/rescue.go`：`ReadSessionTool`（按物理行号取窗口：默认末尾 12 条消息、单条截 2000 字符、`from` 往前翻、`count` 到 40、`max_chars: 0` 看全文并回执"要读全文就再给 `max_chars=0`"）、`SessionStatsTool`（轮次数、每轮 `finish_reason`、正文"像工具调用"的行号（`<tool_call`/`<parameter`/`"function":`/`\submit{`/整段裸 JSON）、最长重复 120 字符块、每轮起点行号——**事实**，不下结论）、`RescueSubmitTool`（`verdict`/`from`/`reason`/`hint`，`truncate` 必须给 `from`、`reason` 必填、终态回执）。`rescueReview` 起会话（上下文 `latex.rescue.context_limit` 默认 131072、输出预算与 checker 同配置、轮次 `max_rounds` 默认 12），`rescueBudget` 按「章节/角色」计数（默认 2 次），`rescueAudit` 追加 `work/rescue/log.jsonl`，`rescueQueueTruncate` 排 `role=continue`（checker → `role=checker`）的重试请求（`From`/`Path`/`Source: rescue`）。
- `remindOrRescue`（`book.go`）：`remindSubmit` 变薄壳；convert 调用点带 `rescueTarget{Proj, Base, Role: "convert", TrPath: trPath}`。每次提醒前先复盘：`fail` → 直接失败；`truncate` → 排队并失败收尾（不再在同一坏会话里烧第二轮）；`retry` → 照常提醒，`hint` 拼在提醒末尾。
- checker（`checker.go`）：未提交结论时同样先复盘，`role=checker`，`truncate` 排队后返回「已排队从干净轮次继续核对」。
- 预览（`sessionview`）：「错误处理」阶段（`StageTitles`/`StageOrder` 排最后/`LabelFor` 认 `rescue_` 前缀），复盘转录 `work/sessions/rescue_<章>_<时间>.jsonl` 单独成组。
- 提示词 `latex_rescue.system.md`（三类结论的判据、`truncate` 取**最早**干净点、不得编造行号、必须 `submit`）+ 注册表（`MustMention: read_session/session_stats/submit`）。
- 配置分两处（用户当天调整）：`latex.rescue.*` 只留功能级——`enabled` / `max_per_attempt` / `keep_records` / 模型指针 `model`；**会话参数进 `latex.sessions.rescue`**（`context_limit` 默认 131072、`max_tool_rounds` 默认 12、`max_tokens` 0 = 继承 checker 的预算；没写的字段继承 `sessions.checker`，`LatexSession("rescue")` 里实现）。模型条目 `models.rescue` 写在 **`models.checker` 下面**、`extends: "checker"`（限制不写在模型条目上）。配置版本 20→22，两份模板同步、`temp/config_new.yaml` 固件同步、README/docs/config.md 同步。
  期间一次操作失误：把模板 cp 到了仓库外（`/home/lingnc/workspace/Test/config.example.yaml`），已确认内容与模板一致后删除，并写到正确的 `MinerU_Tools/config.example.yaml`。

**测试**：`rescue_probe_test.go`（尾窗/`from`/全文、超长截断提示、`session_stats` 点名正文工具调用与 `finish_reason`、`submit` 校验与终态、排队请求的 `role=continue`/`From`/`Path`、预算默认 2 次、模型回退链）、`scan_rescue_stage_test.go`（预览阶段归组与排序）。`go test ./internal/...` 全绿。

**已知边界**：递归子章节的**转换**会话被判 `truncate` 时，`role=continue` 会被 T211 的分流折成"继续跑父章节"（`From` 丢弃）——子章节退化目前靠父章节续跑兜底，没有按行号精确截断；子章节的**核对**会话不受影响（`role=checker` 的 `From` 一路可用）。`--debug` 强制落盘复盘转录（`rescueKeepRecords` 读 logger level ≥ 4）。

**上线后第一轮观察（同日 17:17 重启后）**：`work/rescue/log.jsonl` 四条——17:19 `chapter_023_child_02_child_02` 判 `retry`（理由：转换与目标检查都已完成、只差 submit，属"正常提示"那一路，随后确实发了那条提醒）、17:31 两个 `child_04` 子章节判 `fail`（模型两次自述"workspace tools 不可用"；其转录显示 15:01 第一次响应就是 `finish=stop`、正文与工具调用皆空，属厂商空响应后的退化，T205 的"只告警不判死"正是这条链的源头）、17:32 `child_04_child_03` 判 `truncate`（from=44）。**暴露两个问题**：① `--debug` 下复盘转录没有落盘（判定写成 `level >= 4`，而 Debug=1）→ 网页「错误处理」看不到会话，只有审计日志；已修并加测试；② 子章节判 `truncate` 时排出的 `role=continue` 被 T211 分流折成"父章节继续跑"（`req-…-chapter_023.json` 在队列里等章节锁），`From` 丢掉——子章节的精确截断仍缺，见 plan P58 的"已知边界"。

**P58 的第二次观察（用户指出）**：`chapter_023_child_04_child_02` 的转录里只有一条 user 任务 + 一条 "the chapter workspace tools are not available in this session" 的拒绝，用户判定"这不对，工作区明明给了工具，这是幻觉"。查下来是两件事叠在一起：① 15:01:26 的第一次响应 `finish=stop`、**completion 1819 tokens 但正文/思考/工具调用全空**（思考被网关吞掉），T50 的退化判据要求 `completion==0`，于是漏网——会话静默停住；② 进程重启恢复后模型冒出"工具不可用"的幻觉并收尾，复盘判了 `fail`（把整章记成不可能）。修法：会话层遇到"声称工具不可用 + 本轮没有 tool_calls"就**纠正一次**（把真实工具名清单写进 user 消息、要求现在就调，每会话一次，仍拒绝则照常收尾交提醒/复盘）；空回复判据放宽到"正文/思考/工具调用三者全空"（不再要求 completion==0），`finish=length` 仍交给 T159 的预算翻倍路径，两条不抢；复盘提示词补「拒绝不算答案」——只拒绝的会话判 `truncate`（从头重开）而不是 `fail`。测试：`TestToolProseLeakNudgesOnceButDoesNotFailRun`（2 次请求 + 纠正消息里点名列工具）、`TestDegenerateEmptyResponseHelper`（completion>0 的空内容也算退化）、`TestClaimsNoToolAccess`/`TestToolNameList`；`TestDegenerateEmptyResponseGivesUpAfterOneRetry` 改用 `finish=stop`（`length` 归预算翻倍路径）。

**判定口径再调整（同日，用户口径）**：`truncate` 应当是常态而不是例外——"retry 会保留错误，工具问题和会话污染；只有网络中断、截断这类小问题 retry 比较好，会话或输出有问题就截断"。于是把 `latex_rescue.system.md` 的判定段落按这条重写：`truncate` 是默认（坏的一轮 / 工具用错 / 拒绝 / 重复乱码 / 上下文被错误前提污染 / 已经做错的工作），`retry` 只留给"聊天记录一路干净、停住的原因是外部瞬时故障"（断网、API 错误、`finish=length` 截断、进程中断），并加一条硬规则：**拿不准选 truncate**；`truncate` 时若任务之后没有干净点，就截到原始任务那一行（同一个任务、干净上下文重开）。代码不动（判据在提示词里），`docs/sessions.md`/plan P58/CHANGELOG 同步。

**章节树把上一代当成现状（用户报告）**：查 `chapter_004` 的数据——工作树里是这一次的 `convert_chapter_004_child_03.jsonl`（#1），归档 `work/attempts/chapter_004/try1/...` 里是上一代的 `convert_chapter_004_child_03.try2.jsonl`（#2）与它的 `checker...try2`。根章重来一次时子章节尝试号又从 1 数起，而 `newerAttempt` 只比号，于是上一代的 #2（已通过）盖住这一次的 #1（还在排队），核对结论也跟着挂上去——用户看到的「下面都通过了、却还有一个没完成」正是这样来的。修法：`newerAttempt` 先比「是否归档」（工作树永远优先），核对只与同一世代配对；`chainMemberTag` 给核对行加 `核对` 前缀（`旧核对#1`），同号 `#1` 不再分不清。web 单测 205 条全绿、dist 已重建。

## v1.5+（第五十九批补一，T214/T215：子行跟着父行换代 + 预览「重置视图」，2026-10-22）

### T214 · 切换转换会话时子章节行跟着换代

- 现象（P59 落地后的遗留）：钉住上一代（归档）的转换时，父行的转换切过去了，下面子章节行仍在工作树里挑——上一代的子行挂到这一次的父行下面。根因是 `buildChapterTree` 每个节点各自 `newerAttempt`（先比归档标记、再比尝试号），从来没有"子行跟着父行走"这一层。
- 改：先搭父子关系、再从根往下逐层定行。父行先定它显示哪一次转换，子行 `convertInGeneration` 只在**同一代**里挑（父行在工作树 → 只在工作树；父行是归档 → 回到归档），同代里再按原来的 `newerAttempt` 取最新；核对继续走 P59 的锚点规则（锚定同一次转换 + 同世代、mtime 最新）。钉住的那一行照旧不被覆盖（钉住核对时转换由锚点决定）；没有 pin 时与旧行为一致；那一代里没有这个子章节的转录时不留空行、退回该节点最新一次。世代只说工作树 / 归档，与尝试号无关（父子的号本来互不相关，T175 的"子行不按父行号切"仍然成立）。
- 验证：`web/tests/chapter-tree.spec.ts` 新增 4 条（不钉：父/子都取工作树；钉工作树转换：子行留在工作树；钉归档转换：子行回到归档；归档那一代的核对只挂在归档子行下、切回工作树就不显示）。既有 41 条全绿——尤其 T175 那条"钉 convert #2 时子行仍是自己最新一次"（同属工作树，世代没变）。

### T215 · 预览页「重置视图」

- `web/src/state.ts` 新增 `resetViewState()`：纯状态、不碰 DOM、不发请求。清手动选中/pin 与它的行缓冲、轨迹页选中/展开/类型筛选、搜索过滤词，清空 `state.collapsed`/`state.overflow` 并从 localStorage 删掉这两个记忆键，窄屏侧栏回到展开；**主题 / 显示单位 / Markdown / 缩略图 / 自动跟随 / 板块 / 方块视图一律不动**。返回"该重新选中哪一条"：本板块里正在跑的顶层会话（无 `chapterParent` 那条，子章节的世代跟着父章节走，选中它等于钉住一行）→ 正在跑的子章节 → 列表第一条 → 空串。选中与拉行仍走唯一的 `selectSession` 入口（所以它自己不发写请求、不删数据）；`storeRemove()` 与 `storeGet/storeSet/storeJSON` 同款静默降级。
- 侧栏头部刷新键 `⟳` 右边加文字按钮「重置视图」（复用 `.tab-toggle` 词汇），悬浮说明写清"清什么、不碰什么"；56px 轨道态放不下，与搜索框/状态行一样隐藏。
- 验证：`web/tests/reset-view.spec.ts` 4 条（清导航键 + localStorage 两个记忆键、主题/单位/Markdown/缩略图原样留着、回到正在跑的顶层会话而不是子章节、板块之间不串）。
- 本批合计：`npm test` 219 条全绿（原 211 + 4 + 4）、`npm run build` 重建 dist（`go/internal/sessionview/assets/dist/` 已提交）、仓库内 `go build ./...` 通过；`npx tsc --noEmit` 只剩 `data.ts` 与 `legacy/trajectory.ts` 的既有报错（改动文件零新增）。

## v1.5+（第五十九批补二，T217：核对拿不到结论先重开核对，不升级为整章重来，2026-10-23）

### T217 · 核对的"没结论"不再等于"这一章不过"

- 现象：一次转换已经跑完、产物也在，核对会话两次都没有提交结论（模型把结论写成正文、或直接结束回合，提醒也没用），`checkChapterAt` 就把失败文本交给调用方——调用方按"核对发现了问题"的规矩处理，旧尝试整棵归档、这一章从头重转。实测有一章内容本身没有问题，只因为核对始终没提交，整章重跑了一遍，白花钱也拖时间。
- 根因：`checkChapterAt` 里写死 `checkerAttempts = 2`，且"没结论"与"结论说有问题"返回的都是同一种"失败"，上层没有可区分的事实。核对的失败类型其实有两种：**会话自己没产出结论**（跑挂 / 漏 submit，重开一次核对就好）与**核对真的判了这一章有问题**（要回到转换会话修内容）。
- 改：① 循环里记 `sawVerdict`（任一次 `submit.Submitted`）。一次结论都没拿到时预算放宽到 4 次**全新会话**（同一转录路径、同一只读视图，每次重开打一条 warning）；只拿到过一次结论就回到原来的 2 次，`issues` 那一路照旧立刻返回、由调用方按 `checker_rounds` 喂回转换会话。② 文案分开：没结论时是 `核对始终没有给出结论（不是内容问题，共尝试 N 次）…`，旧的 `checker 多次未能提交结论（会话未调用 submit）` 留给"给过结论但没有可用结论"那一路；`checkerNoVerdict()` 同步认得新前缀——不然这串文本又会被当成"章节有问题"送进反馈轮（T209 修掉的就是这个空转）。③ `book.go` / `retry.go` 的升级链**刻意不动**：4 次都拿不到结论时整章仍会作废重来，这一批只把"没结论"这一类失败的概率压下去，不重设计升级路径。
- 验证：`TestCheckerNoVerdictRetriesFourAttempts`（只回正文 → 恰好 4 次会话：8 条请求 = 每轮首请求 + 提交提醒、转录里 4 条 `checker_target` 锚点，返回新文案前缀）、`TestCheckerNoVerdictExtraAttemptCanPass`（前两次只回正文、第 3 次提交 `pass` → 通过，3 条锚点，多出来的轮次确实可用）、`TestCheckerWithoutVerdictFailsAfterFourAttempts`（退化响应连跑 4 轮、请求数 8–12，原先是"两轮 / ≤6"）、`TestCheckerNoVerdict` 补新文案一条；`go test ./internal/latex/` 与 `go test ./internal/...` 全绿。
- 已知边界：P58 的复盘会话判 `fail`/`truncate` 时会**提前返回**（"复盘判定会话无法挽救" / 已排队从干净轮次继续），走不到第 4 次；这两句也不在 `checkerNoVerdict()` 的识别范围里。本轮不动它——动它等于改升级链。另：核对转录是同一条（同一次尝试内四次会话共用一个路径），所以"试了几次"只能从锚点/警告行看出来，页面仍显示一条核对会话。


## v1.5+（P60 第一步：修复会话骨架与 `dispatch_fix` 工具，默认关闭，2026-10-23）

设计口径（见 `docs/plan.md` P60）：父章节重试代价太大，应当按修复任务把工作重新下派给**已经划好的**子章节与工作区，父会话只做整合。这一步只落接口与骨架，**不接进运行路径**。

- `internal/config`：新增 `latex.fix` 块——`enabled`（`*bool`，默认 false）、`max_rounds`（默认 2；0/负数也按 2，不是无限轮）、`model`（空 = 继承 convert），外加 `LatexConfig.FixEnabled()` / `FixMaxRounds()`。两份配置模板逐字节同步（`fix` 块写在 `latex.rescue` 后面）、`config_version` 22 → 23，README 的示例行与 `docs/config.md` 的版本行/新增三行同步。
- `internal/latex/fix.go`：`fixPlan` / `fixPlanChild` / `fixPlanIssue` 与 `WriteFixPlan` / `ReadFixPlan`（同目录临时文件 + rename 原子写；版本、根章名、`child_NN` 形状、字段非空、issue id 不重复都在写与读的同一套校验里）；`DispatchFixTool`（`dispatch_fix`）只**记录计划**并回执计划路径 + issue 数 + 将要下派的子章节数，校验失败当场 `REJECTED` 且不写盘；子章节名对着 `_subdivision/` 下的真实目录核对（优先用调用方给的父工作区，否则按 `work/temp/conv_<根章>*` 找最近的一棵）。`runChildFix` 按最终形态写好：复用子章节自己的 `node/work`、工具集 = 转换会话减去 `request_chapter_split` 再加 `submit`、转录 `node/sessions/fix_<子章>.jsonl`、每轮写一条 `t=meta kind=fix_target`（锚点与核对同口径）、提交后跑该子章节原本的核对（同一只读视图、同一锚点，P59）。守卫顺序：未启用 → 顶层节点 → 无任务 → 工作区/输入不在 → 轮数预算用完，都不凭空重建。
- `internal/prompts`：新增 `latex_childfix.system.md` + 注册表条目（`Vars: MAX_ROUNDS`，`MustMention` 含 `submit` 与转换那套工具）。**名字不能叫 `latex_fix.system.md`**——那个文件名已经是装配阶段的书级编译修复会话（`FixSystem` / `book_fix`），两个用途不能共用一个模板。
- `internal/latex/book.go`：唯一一处运行期接线——`latex.fix.enabled` 为真时给转换会话注册 `dispatch_fix`；默认关闭时工具面一字不变。
- `internal/sessionview`：解析 `t=meta kind=fix_target` 得到 `fixAnchor`/`fixAnchorAttempt`/`fixIssueIDs`，`SessionInfo` 上给出 `chapterRole:"fix"` + `chapterAnchor`/`chapterAnchorAttempt` + `fixIssueIds`（新字段）。**分组、章节树、历史一行没改**（章节身份仍为空、阶段仍落通用「会话」），留给后续的树/历史条目。
- 守护测试的一处调整：`TestLiveLatexToolsAreMentionedSomewhere` 要求"每个工具名都在某个模板里被提到"，而 `dispatch_fix` 是**条件注册**的（只在开关打开时存在）——把它登记进 `conditionalTools` 并注明理由，避免提示词为一个默认不存在的工具留下说明（模型照着提示词调用不存在的工具是历史事故）。
- 测试：`internal/latex/rescue_fix_plan_test.go`（计划往返与落盘形状、非法 child 名、未知版本、字段缺失/重复 id、`dispatch_fix` 的校验矩阵与"校验失败不写盘"、正常下派的分组与回执、默认关闭守卫、轮数预算、顶层拒绝、模型继承）、`internal/config/p60_fix_test.go`（默认值与访问器、两份模板读出来的值、版本号已 bump）、`internal/sessionview/scan_fix_target_test.go`（`fix_target` 解析、角色/锚点/issue 清单、普通转录不受影响）。`go test ./internal/...` 全绿；`go vet ./internal/...`、`gofmt` 干净。
- 本地一份 **gitignored** 的配置改写固件（`temp/config_new.yaml`）随之把 `config_version` 从 22 提到 23，否则它自己的严格校验测试会因版本不匹配而失败；该文件不入库。


## v1.5+（P60 第二步：把 `dispatch_fix` 接进章节转换流程，默认仍然关闭，2026-10-23）

第一步只落骨架、不接运行路径；这一步把调度接上去，开关 `latex.fix.enabled` 依旧默认关闭——关着时转换会话的工具面、系统提示词与请求数都与引入 P60 之前逐字节一致（有测试按请求数钉住）。

- **提示词只按开关追加**：新增模板 `latex_convert_fixdispatch.system`（"把问题子章节还回去"两行：只记录计划、runner 会跑该子章节的修复会话并重核、没列出的子章节不动），由 `Runner.convertSystemPrompt` 在开关打开时接在 `latex_convert.system` 后面；常驻的转换模板一个字没动。因此关着开关的跑法不可能看到一个不存在的工具（历史事故：`view_page`）。
- **为什么需要一次额外对话**：子章节只要有一个没通过自己的核对，`convertChapterChildren` 直接报错、整章不会整合——也就是说走到"子章节转换+核对完"这一步时，**所有子章节都过了核对**，没有任何现成信号告诉父会话哪个孩子还有问题。所以细分父节点分支在整合对话之前先给父会话一次**复查对话**（`reviewChildrenBeforeIntegration`，同样取转换槽位）：读 `children-summary.md` 与有疑问的子章节产物，有问题就调 `dispatch_fix`，没事就停手；这段对话本身失败也只警告，照旧整合。
- **按计划执行**：`applyChildFixPlan` 读 `work/fix/<根章>/plan.json`，按子章节顺序跑 `runChildFix`（各自的工作区、`latex.fix.max_rounds` 轮数预算、修复会话现在也从 `latex.convert_concurrency` 的槽位池取一个）。修完把该子章节的 `.tex` 与分片**重拷**回父章节的 `children/`，重写 `children-manifest.md`/`children-summary.md`，并按子章节的 issue store 重算 `issues-inherited.json`——整合会话读的就是这三样，不刷新等于没修。计划之外、不在计划里的子章节一律不碰。
- **重核用的是"新的一次核对"**：沿用原来那条核对转录会被"转录里已 submit 就重放收账"直接判成上一轮的结论（模型根本不会再核一遍，实测就是秒过）。改成与手动重核同一个口径——写 `checker_<子章>.try<N>.jsonl`，锚点仍指同一次转换（P59），产物与只读视图不变。
- **失败一律降级**：修复会话失败/未提交/修复后重核不过，都只记一行 warning，父会话照旧整合并提交（今天的行为）；这份失败不会让整本书跑挂。修复后核对没过的子章节，父章节整合时看到的是它在磁盘上的最新产物（注释里写明了这一点）。
- 守护测试调回普通规则：`dispatch_fix` 现在真的出现在一个模板里，`conditionalTools` 豁免删除；"关闭时提示词逐字节不变"改由 `TestConvertSystemPromptDispatchOnlyWhenEnabled` 与几个假客户端集成用例钉住。
- 测试（`internal/latex/rescue_fix_dispatch_test.go`，沿用既有 httptest 假客户端风格）：关闭时 7 个请求（父拆分 / 两子转换 / 两子核对 / 父整合 / 父核对）、工具表里没有 `dispatch_fix`、提示词里没有它；打开且计划点名 `child_01` 时只有它多一条修复会话与一次重核（核对次数 1→2），`child_02` 不变，父整合照常，且父章节 `children/` 副本确实换成了修复后的内容、修复转录里有 `fix_target`；修复会话怎么提醒都不提交时整本书仍然整合提交，日志里有那条 warning。

## v1.5+（P60 第三步：章节树显示修复会话子行，2026-10-23）

前两步把修复会话与调度做完了，但预览页一直看不到它们——`fix_target` 只被读成角色/锚点，树与历史一行未动。这一步补上显示，**没有修复转录时行为与之前一致**（默认配置下 `latex.fix.enabled` 仍是 false，本批零运行期差异）。

- **树数据（`web/src/legacy/sidebar.ts`）**：`ChapterTreeNode` 多一个 `fixes: any[]`（没有修复就是空数组，调用方不用判空）。修复会话按**锚点**挂到节点上——锚点等于这一行**选中的那次转换**的转录名才挂，归档世代也要一致，锚定另一次转换的修复不显示（与 P59 核对同一条规则）；没有锚点的老转录退回按章节 ID 相等。节点内按时间**旧 → 新**排（修复是同一处的连续轮次）。修复会话从头到尾不参与该行 convert/checker 的挑选；钉住一条修复会话时，它所在的行按锚点显示那次转换（否则点开修复后行会切回最新一次、修复子行跟着消失）。
- **历史（`chapterAttemptChain`）**：修复会话自己的链 = 同一锚点下的其他修复轮次（新 → 旧）；转换会话的链仍是「只有转换」，核对的链仍是「同一转换名下的各次核对」——修复已被排除在成员之外（它们本来就因为文件名不是 `convert_`/`checker_` 前缀而没有章节身份）。
- **UI（`ChapterSessionTree.vue`）**：核对行下面每条修复一行（24px，走 `--row-h` token；缩进再深一级），标「修复 N」（N = 该节点内的 1-based 顺序）、文件名里的 `.tryN`、运行中/归档状态，点行选会话（与其它行同一个 emit）。状态只说"运行中"——不套 `chapterActivityLabel` 那套转换/核对语义。`hasBody` 把"有修复"也算成可展开，`nodeContainsCurrent` 把修复算进"这一支装着当前会话"，所以从修复会话进来时它自动露出。
- **历史面板（`HistoryPanel.vue`）**：修复行有「修复」角色标签、头部文案是「同一处的修复轮次」。修复转录取不到尝试号，按号比较会让每一行都顶「最新」——修复链改按链序标第一行；`chainMemberTag` 对修复给「修复」+ 文件名里的 `.tryN`（归档前缀仍是「旧」），不编一个假的 `#1`。
- **测试**：`web/tests/chapter-tree.spec.ts` 新增 9 条（同锚两轮按旧 → 新列出且行头仍是转换、锚定另一次转换的修复不挂上来、钉住修复时行按锚点切、归档那一代的修复不挂到工作树、没有修复时是空数组、只有修复会话不凭空造节点、修复自己的链只含同锚点且新 → 旧、转换的链仍只有转换、`.tryN` 与标签不编尝试号）。`npm test` 228 条全绿（原 219 + 9）、`npm run build` 重建 dist（已提交）、仓库内 `go build ./...` 与 `go test ./internal/sessionview/...` 通过（后者也钉住"间距尺寸只认 token"，修复行的 24px 因此走 `--row-h`）。
- **数据形状没对上的一处**：spec 里"锚点为空时退回 `chapterId` 相等"在真实数据上走不到——修复转录文件名是 `fix_<章>`，`chapterIdentity` 只认 `convert_`/`checker_` 前缀，所以扫描给出的 `chapterId`/`chapterParent`/`chapterAttempt` 都是空的（P60 第一步的测试就明确钉着这一点，本步不改后端）。于是树的挂靠主路径只能是锚点，`chapterId` 那条作为旧/外来数据的兜底保留；历史面板里也才有"修复没有尝试号"这一串处理。

## v1.5+（P60 第四步：修复会话纳入章节身份 + 整章重开预算收口，2026-10-23）

第三步的最后一条记录点明了数据形状没对上：修复转录文件名是 `fix_<章>`，而 `chapterIdentity` 只认 `convert_`/`checker_` 前缀，所以扫描出来的 `chapterId`/`chapterParent` 是空的，树的挂靠只能是锚点、历史里还得专门绕开"修复没有尝试号"。这一步把身份补上，并把一直留着的"整章重开预算"落地；`latex.fix.enabled` 仍默认关闭，默认跑法不变。

- **身份（`internal/sessionview`）**：`chapterIdentity` 的前缀表加上 `fix_` —— `chapterId`/`chapterParent`/`chapterDepth` 与 `.tryN` 尝试号自动齐了；`LabelFor` 给出 `fix:<章>`，`enrichSessions` 的既有规则（有章节身份即章节阶段）把它归到 `convert` 组，不再落通用「会话」。`TitleFor` 的 `fix` 分支按 subject 是否像章节 ID 分叉：章节修复是「修复 · <章>」，书级 `book_fix` 仍是「全书修复」（两者原本共用一个阶段名，直接改会把装配阶段那条也一起改掉）。
- **网页（`web/src/legacy/sidebar.ts` / `HistoryPanel.vue`）**：一行未改挂靠逻辑——`buildChapterTree` 先按角色 `fix` 收进修复池（不建节点），`fixesForNode` 仍以**锚点等于这一行选中的转换**为唯一依据，`chapterId` 只是兜底；`chainMemberTag`、`chapterAttemptChain` 对修复仍走锚点分支。唯一改动是历史面板的 `pairOf`（转换 ↔ 同轮核对的直达入口）对修复返回空：`chapterId` 有了值之后，修复行会被当成"非核对"的一半去和同章的转换配对，多出一个跳错地方的「同轮核对」按钮。
- **预算（`internal/latex`）**：`fixBudgetExhausted(work, parentBase, children)` —— 修复打开、且某个子章节的修复转录里已有 `>= latex.fix.max_rounds` 条 `fix_target` 时为真（读不到转录算 0）。命中时 `convertChapterNode` 的"核对不过 → 归档 + 换新会话整章重转"改为保留这一版整合产物（细分父节点照 `placeChapterFiles` 晋级到正式章节目录）、记一行「修复轮数已用完，不再整章重开」的 warning 后正常返回；未解决的问题不在这一层处理，它们本来就走 issues 库与报告向上。开关关着、节点没有子章节、或没有子章节进过修复路径时判据为空，代码路径与之前完全一致。
- 测试：`internal/sessionview/scan_fix_target_test.go` 加两条（`.try2` 文件名推出 ID/父章/深度/尝试号/阶段/标题，扫描行落在 `convert` 阶段组且锚点与角色不受影响），并改掉原先"章节身份必须为空、阶段必须是「会话」"的断言；`internal/latex/rescue_fix_budget_test.go` 四条——判据本体表驱动（关着 / 没进过修复 / 预算还有 / 用光 / 第二个孩子用光），以及用真实 `convertOneChapter` 递归路径 + 假客户端跑的三条端到端：预算用光时不重开（父会话与父核对各一次、无 `work/attempts/` 归档、整合产物仍在、日志有警告）、预算还在时照旧重开（归档出现、多次尝试直到次数用尽）、关着时同样照旧重开且没有任何修复日志。为跑这几条把第二步的假客户端扩成带选项的 `p60RunWith`（父核对可报 issues、可压 `checker_rounds`、可预期失败），并补一条"核对反馈轮重交同一份产物"的回复分支。变异检查：把收口判据短路掉，预算用光那条会退化成"3 次尝试仍未通过 checker"，测试确实红了。
- 验证：`go test ./internal/... -count=1` 全绿（`internal/latex` 17.5s、`internal/sessionview` 17.5s），`go vet ./internal/...` 干净，`gofmt -l` 对改动文件无输出（`internal/latex/stylefix.go` 是既有的未格式化文件，本轮未触碰）；`web` 侧 `npx vitest run` 233 条全绿（新增 5 条），`vite build` 重建 dist 并提交（`HistoryPanel.vue` 的 scoped hash 因此变化）。

## v1.5+（P60 收尾：转换槽位按任务创建顺序派发，2026-10-23）

P59 定下的第二条口径是「当前没有被阻塞的会话，创建早的优先级高」，那一批只落了锚点归组，槽位派发明写「未另改」。这一批把它补上，改动只落在 `latex.convert_concurrency` 那个转换槽位池里；核对池与其它阶段的通用池仍是先到先得。

- **现象**：一次真实运行里，一棵正在等孩子的子树的 15 个孙节点在队列里排了约 25 分钟，期间后来创建的顶层会话轮番拿走全部 10 个槽位，那一支一步没动。等待本身不是问题，问题是**等待的人里没有按创建早晚排序**。
- **根因**：转换池是 `make(chan struct{}, N)`，取槽位就是往 channel 里塞、释放就是收——这是纯粹的先到先得。递归细分下来的子章节与晚创建的顶层章节在同一时刻变成"就绪"，谁先排到谁上；一棵早创建但中途在等孩子的子树，等完回来时排在所有后来者后面。
- **改**：新增 `internal/latex/slots.go`，一个零依赖、只做一件事的优先信号量 `slotSem`：容量 N，`acquire(seq)` 返回释放函数，等待队列按 `(seq, 到达顺序)` 排序，`release` 把槽位**直接交给**队里最小的那个（交棒时 `used` 不变，不给新来者插入的机会）。释放幂等——调用方既有 `defer release()` 也有显式 `release()`，重复释放若照收就会把别人的槽位吞掉。创建戳 `seq` 由 `Runner.nextConvertSeq()` 发（1 起、单调递增，`atomic.Int64`）：顶层章节在**派发循环里、goroutine 启动之前**取（章节顺序 = 创建顺序），子章节在 `convertChapterChildren` 的循环里、启动 goroutine 之前取（顺序按 children，且必然大于父节点的号），重试沿用父节点原来的号（同一件任务，不因为重来一次就变成"新任务"）。修复会话与整合前的那次复查对话用的是**它们所属那棵子树**的号，不是一个此刻新发的号——否则一个早创建、正在等孩子的子树，它的修复会话会被排到所有后来者后面。等自己子树的父节点本来就在等之前把槽位还掉了，所以「被阻塞的会话」根本不在等待队列里，「不占队头」是这么成立的、不需要额外判据。
- **边界**：`acquireConvertSlot(seq int64)` 的调用方没带创建戳（`seq <= 0`）时当场领一个新号，于是排到所有已创建任务之后——宁可保守排后面，也不让它凭空抢队头。`convertOneChapter` 保持原签名（运行期手动重试/测试入口按"此刻创建"取号），批量派发走 `convertOneChapterSeq` 显式带号。`convertChapterChildren` 里那个局部的 `sem` 没有一起改：它只在同一个父节点的兄弟之间限流，兄弟本就是同一批创建的任务，改它没有收益。
- **测试**（`internal/latex/chapter_slots_test.go`，7 条）：容量 1 下三个等待者按 3、1、2 到达仍按 1、2、3 被释放；一次释放只叫醒最小的那个（其余仍在排队、`used` 不变）；晚到的小号照样拿到下一个空槽（"被阻塞后刚就绪"）；容量 3 时第 4 个必须排队、放掉一个只叫醒一个；20 个并发取用者、容量 4 时峰值不超容量且事后无泄漏（再整批取满仍成立）；重复释放幂等；book 级的 `TestConvertSlotUsesCreationStamp` 走真实 `Runner.acquireConvertSlot`，晚创建的先排队也抢不过早创建的后到者，`seq<=0` 的调用方排在两者之后。`go test ./internal/... -count=1` 全绿、`go vet ./internal/...` 干净、`gofmt -l` 对改动文件无输出。
- **没覆盖到的**：没有一条测试在真实递归运行里端到端验证"孙节点按创建顺序上槽位"——那需要可观测的会话启动顺序与可控的槽位争抢，测试里只能用信号量本体 + `Runner` 池的两层直接断言；调度层（派发循环里取号、子章节号大于父节点）靠代码结构与注释约定，不是自动化断言。

## v1.5+（P58 扩展：复盘也覆盖报错终止与"声称没有工具"，且可整段关闭，2026-10-22 第二轮）

P58 第一版的触发点只有一个：`Run` **正常结束**但没 `submit`（`remindOrRescue` 被调用的那一刻）。用户第二轮把口子放宽——**报错终止**的会话（网络/接口瞬时故障、空回复等非致命错误）上下文其实还完整，却被直接记成"这一章失败"，连一次复盘都没有；**"声称工具不可用"**的会话（工具一直在请求里，是幻觉）也只能靠提醒再赌一轮。除此之外，整个功能要能被一个开关整段关掉，关掉时与引入之前逐字节相同。

- **唯一入口**：`maybeRescue(tid, tag, label, target)` 收走开关判断、`latex.rescue.max_per_attempt` 预算、结论处理与全部日志；`remindOrRescue`（convert）与核对会话的提醒路径都调它——核对里原先那份内联的 `fail`/`truncate`/`retry` 分支删掉，两处不再有第二份结论处理（`fail` → `Stop`；`truncate` → 排队 `role=continue` + `Stop`；`retry` → 只把 `hint` 带回去拼进提醒）。
- **报错终止**：新增 `rescueAfterError(sess, …, cause)`，在 `convertChapterNode` 的**初次转换**与**细分父章节整合**两处 `sess.Run` 之后调用。`cause` 非致命时先把一段事实写进 `rescueTarget.Cause`（复盘会话的第一句话变成"这次是报错终止（原因）"，而不是"模型自己收尾了"——`retry` 那一档本来就只为外部瞬时故障准备），然后复用 `remindOrRescue`：`retry`/无结论 → 照旧发提醒续跑一轮（`retry` 成功提交则这一章接着走核对），否则原样返回 `cause`。**致命错误（余额不足 / 无可用资源包）在函数入口就返回**，一次模型调用都不多花。
- **"声称没有工具"**：`session.Session` 新增读访问器 `ToolAccessNudged()`（`toolAccessNudged` 原本只在包内可见）。触发点在 `remindOrRescue`：会话层已经纠过一次"你的工具确实可用"而它仍然收尾时，`target.NoToolNudge` 置位，复盘者的第一段话会读到这条事实（"纠正已经用过、它仍然没提交"）。这不改变任何结论处理——它只是把"这是退化的强信号"告诉复盘者，判 `truncate` 还是 `retry` 仍由它按判据决定。
- **关闭即老行为**：`latex.rescue.enabled: false` 时 `maybeRescue` 第一行返回空结论、`rescueAfterError` 第一行原样返回 `cause`——没有复盘请求、没有复盘转录、没有审计行、不排重试队列；"没提交"仍是两次提醒 + 失败，提醒文案取自新的 `submitReminderText()`（提醒轮与报错续跑共用一个来源，措辞与之前一字不差）。
- **配置**：键面没变（`latex.rescue.enabled`/`max_per_attempt`/`keep_records`/`model` 都在 P58 第一版里），但开关的覆盖面扩大，版本号 23 → 24；两份模板仍逐字节相同、`docs/config.md`（`config_version` 行 + `latex.rescue.enabled` 行的说明）、`README.md` 的版本行、`docs/sessions.md` 的触发点说明同步。
- **测试**（`internal/latex/rescue_triggers_test.go`，沿用包内 `httptest` 假客户端 + `subdivideReply` 风格，跑真实 `convertOneChapter`，断言**请求数与磁盘上留下什么**而不是内部函数被调了几次）：① 报错终止 → 恰好一次复盘、请求数 2、`work/retry/` 里一条 `role=continue` + `from` + `source=rescue`、审计行 verdict=truncate、复盘请求文本里带"ended with an error"；② "声称工具不可用" → 转换请求 2 次（幻觉 + 纠正后一轮）、复盘 1 次且文本里带"claimed that its tools were not available"；③ 429 + 余额不足 → 复盘 0 次、请求 1 次、无队列无审计、错误原样上报；④ `enabled: false` 两条：报错终止请求数 1（立刻上报）、没提交请求数 3（转换 + 两次提醒）、两种都没有复盘转录/审计/队列，提醒文案仍是 "You have NOT submitted yet"；⑤ 不写开关（默认开）→ 复盘两次（`retry` 一次把 hint 拼进提醒、再判 `fail` 停手）、审计两行。变异检查：把 `rescueAfterError` 短路成直接返回 → ① 立刻红；把 `NoToolNudge` 写死 false → ② 红。
- **没覆盖到的**：① 核对会话的**报错终止**仍走它自己的路（`checkChapterAt` 的错误分支本来就会换一条新核对会话重开，与转换的"报错即整章失败"不是一回事；这里只把它的"没提交"分支并入 `maybeRescue`，错误分支一行未改）；② 转换会话的**反馈轮**（核对打回之后那一轮）报错时仍只是 `break` 出来按核对结论处理——它不是"会话收尾"，而是同一章内部的一次重试轮，改它要同时改反馈轮之后的复核时机，本轮不动。

## v1.5+（P59/P60 收尾：核对历史保留老转录、修复行标明章节与轮次、子章节核对视图补项目插图，2026-10-23）

三条互不相干的遗留问题，一批收拾。前两条只动预览页，第三条只动核对视图的项目根来源；默认跑法与 P59/P60 定下的挂靠规则一行未改。

- **老核对转录被历史丢掉（T218）**：`chapterAttemptChain` 的核对分支只保留 `chapterAnchor` 等于自己的成员。锚点是 P58 起才写进转录的，更早落盘的老核对转录没有锚点、一次都匹配不上——用户看到的是"最新一次核对把旧的盖掉了"。成员侧放宽成"没有锚点就保留（没得比）+ 有锚点必须同锚"，`self.chapterAnchor` 为空时仍不过滤（老会话行为一字不差），排序仍是新 → 旧；锚在另一次转换上的核对照旧排除。`web/tests/chapter-tree.spec.ts` 两条：两次有锚点 + 一条老转录时三条都在（从最新那条与从老那条点开结果相同），锚在 `.try2` 的核对在钉住第 1 次转换时仍不出现。
- **修复行看不出"修的是哪一章的第几轮"（T219）**：树行以前只写「修复 N」，而同一行下面可能挂着不同子章节的修复；历史修复链也只有轮数。新增 `chapterIdSuffix` / `fixTargetId` / `fixRowLabel` 三个纯函数做标签唯一来源：章节 ID 只取本层后缀（`child_02`，嵌套保留整段后缀 `child_01_child_02`，顶层照写自己的 ID），归属优先用转录自带的 `chapterId`、老转录退回挂靠行的章节，轮次按渲染顺序从 1 数（树上旧 → 新、历史新 → 旧用 `fixRoundAt`）。章节树行变成 `修复 child_02 #1`（行高仍走 `var(--row-h)`，`.tryN` 只在它不等于轮次时另挂一枚），历史表头写目标章节 ID、每行标「章节后缀 #轮次」并带悬浮说明。测试加 5 条（后缀三形态、标签含归档与老转录兜底、轮次换算、子章节两轮修复 `#1`/`#2`、历史链取目标章节并按新 → 旧标 `#2`/`#1`）。
- **递归子章节的核对视图缺项目级插图（T220）**：`ensureCheckerViewAt` 用 `filepath.Dir(filepath.Dir(chapPath))` 猜项目根——顶层章节正好猜对，子章节的 md 在 `_subdivision/<child>/input/` 下，猜出来的是子节点目录，`linkCheckerAssets` 在那里找不到 `source/figures|images`，项目级插图一个都挂不上（子章节的核对于是把本来正确的引用报成缺失）。改成项目根由调用方显式传入（`ensureCheckerView` / `checkChapterAt` 本来就有它），子章节与顶层用同一个来源。
- **`parts/` 本身没丢，也钉住了**：按用户描述的目录形状铺了一遍（子章节自己的临时工作区 `work/temp/conv_<根>/work/_subdivision/<child>/node/work` 里的 `parts/`、提交契约的 `work:<子章>/`、走真实 `placeChapterFiles` 与节点内的 `checker-view`），`parts/x.tex`、`parts/sub/y.tex`、`<子章>/parts/x.tex` 全部解析得到——视图的 `parts/` 别名（T186 的 `PARTS_DIR`）对递归节点是对的。反面对照也写了：分片只留在**工作区根**、没有进提交的分片文件夹时，视图里 `parts/` 为单位空目录、工作区文件不进视图也不被动过——"本地能编译、提交里没有"属于转换侧布局，不是视图藏文件，这一条刻意不改。
- **验证**：`go test ./internal/... -count=1` 全绿（新增 `TestCheckerViewRecursiveChildHasPartsAliases` / `TestCheckerViewDoesNotLeakWorkTreeParts`）、`go vet ./internal/...` 干净、`gofmt -l` 对改动文件无输出；`web` 侧 `npx vitest run` 240 条全绿（`chapter-tree.spec.ts` 66 条），`vite build` 重建 dist 并提交，`npx tsc --noEmit` 只剩 `data.ts` / `legacy/trajectory.ts` 的既有报错。

## v1.5+（P60 第五步：`dispatch_fix` 同步执行并回执结局、修复行独立于核对行，2026-10-23）

P60 前四步把修复会话、计划文件、调度与预览页的修复子行都接通了，但父会话手里始终没有**结局**：`dispatch_fix` 只写一份计划就返回，修复要等这次复查对话结束、runner 再执行，父会话整合时并不知道哪一个孩子修好了。这一步把"执行 + 回执"收进工具自己，并顺手收拾了两处误判。

- **同步执行（`internal/latex/fix.go`）**：`DispatchFixTool` 新增 `Runner`/`Parent`/`Children`/`ChildDepth`/`Seq`/`Tid`，`Execute` 写完计划后走 `applyPlan` → 新增的 `(*Runner).applyFixPlan`（把原来 `applyChildFixPlan` 的循环抽成共享实现），每个子章节调 `runChildFixAt`——`runChildFix` 保留为薄包装，因此同步路径与 runner 路径的校验、预算、工具集、挂载、转录、重核完全同源。结局用 `fixChildResult`（Child/Fixed/Checked/Verdict/RoundsUsed/RoundsBudget/RoundsLeft/Err）表达，回执 `FIX RESULT` 按子章节列 `FIXED`/`STILL FAILING`、核对结论第一行（`fixVerdictSummary` 截到 160 字符）、`rounds N/M, K left`，并在父会话转录里追加一行 `t=meta kind=fix_result`（`summary = "N/M 修正通过"`）。计划执行后 `MarkFixPlanApplied` 写 `applied`，`applyChildFixPlan` 读到它直接空转；没有消费的计划（没接 Runner 的老流程）仍按原路径执行，执行完也写 `applied`，所以同一份计划不会被执行两次。
- **并发放手是这一步的必要条件**：父会话停在工具调用里时仍占着转换槽与模型并发槽，而子修复会话马上要问同一个模型——`max_concurrency=1` 时会当场互等。转换槽按 `holdSlot` 让给这一轮修复（调用方已占着，不再另取），模型槽由新增的 `Session.PauseModelSlot()` 临时让出、`applyPlan` 结束时取回；`Session.Run` 里的取槽改成经 `holdModelSlot`/`releaseModelSlot` 记账，取回后仍由同一个 `defer` 释放。
- **自包含边界（A）**：`latex_convert_fixdispatch.system.md` 与整合前的复查指令都写明 `figures/`、`images/` 是装配时才供上的项目级资源，引用它们不算断链；只有 `_subdivision/…` 与 `node/result/…` 才是。`prompts_test.go` 钉住模板文本，`rescue_fix_dispatch_test.go` 钉住关闭开关时提示词逐字节不变、打开时这些句子才出现。
- **修复行独立于核对行（C）**：渲染条件只认 `node.fixes`，"有没有子行"的判据从组件内的 `hasBody` 提成 `chapterNodeHasRows`（核对 / 修复 / 子章节三种来源互不依赖），行 title 由 `fixRowTitle` 给出（`sessionTip` + 结局，结局来自修复会话自己的 `chapter_activity`：pass/issues/error/运行中）。**如实记录**：本仓此前的模板里修复行已经与核对块并列（不在 `v-if="node.checker"` 内），所以这一步的实际改动是显式独立块 + 判据收口 + 结局 title + 回归测试；真实跑批里"看不见修复行"若仍复现，剩余的唯一可解释来源是**锚点规则**（修复锚点不等于该行选中的那次转换时按设计不显示，与核对同一条规则），而不是嵌套。
- **测试**：新增 5 条 Go 测试——`TestConvertFixDispatchScopesSelfContainedness`（模板守卫）；`internal/latex/rescue_fix_sync_test.go` 三条（回执含每个子章节的结局与剩余轮数且重核真的跑过、第二次下派吃掉下一轮并在预算上停住不再发请求、同步消费过的计划 runner 侧空转 / 未被消费的计划 runner 照旧执行且执行后也只执行一次）；`TestPauseModelSlotFreesNestedSession`（让出后子会话立刻拿到名额、取回后名额不能同时被第二个会话拿到）。既有 `TestFixDispatchRunsPlannedChildOnly` 追加三条断言（修复请求恰好 1 次、回执里 `FIXED`/`checker: pass`/`rounds 1/5, 4 left`、计划已 `applied`）。`go test ./internal/... -count=1` 全绿、`go vet ./internal/...` 干净、`gofmt -l` 对改动文件无输出（`internal/latex/stylefix.go` 的未格式化是既有状态，未动）。`web` 侧 `npx vitest run` 243 条全绿（`chapter-tree.spec.ts` 69 条，新增 3 条：核对因尝试号不显示时修复行仍在、`chapterNodeHasRows` 三来源、结局 title），`vite build` 重建 dist 并提交。
- **没覆盖到的**：① 没有在真实一本书上把 `models.<名>.max_concurrency` 配成 1 跑一遍端到端死锁场景——现在只有 `session` 层的直接断言，重试/续跑路径下的让出时机靠代码结构约定；② 前端改动只由纯函数测试把守（本仓 `web/tests` 没有组件渲染测试），"修复行在有/无核对时都渲染出来"由 `chapterNodeHasRows` + `node.fixes` 的数据层断言间接保证，没有 DOM 级探针。

## v1.5+（P58/P60 补：复盘会话可丢弃重来并按层级显示、`dispatch_fix` 回执终结本轮，2026-10-23）

用户提了四条互相关联的问题：`dispatch_fix` 回执之后父会话还在空转一轮；复盘会话自己挂了就没人管；复盘开始/结论时界面看不出状态；一条嵌套子章节的转录堆满"还没提交"却从没被复盘过。四项都不改默认开关语义（`latex.rescue.enabled`、`latex.fix.enabled` 的默认值与关闭时的行为一字未动）。

- **A：回执即终态（`internal/latex/fix.go`）**：同步跑完修复的 `FIX RESULT` 回执标 `session.ToolResult.Terminal`（会话层本来就有这个通道，`submit` 用的同一个），`Run` 在回执与工具消息都落盘后直接返回，父会话不再为了"回应这条回执"发一次请求——整章流程里"整合前复查"的请求数从 2 降到 1，`rescue_fix_dispatch_test.go` 用 `reviews == 1` 钉住。"只记计划"那条回执**不**标终态：它出现在还没接上 `Runner` 的一轮（工具从第一次转换轮起就在工具表里），在那儿掐掉一轮会丢掉父会话还没说完的判断。
- **B：复盘会话可丢弃重来（`internal/latex/rescue.go`）**：`rescueReview` 拆成"重试循环 + `rescueReviewOnce`"，一次复盘最多起 `rescueMaxReviewAttempts = 3` 条会话；没结论（报错终止或跑完不 `submit`）就把这条转录挪进 `work/sessions/.trash/`（撞名加序号）后重开，每次一行警告、三次都没结论再记一行"放弃"；`maybeRescue` 在放弃时把这条目标的复盘预算一次用光并返回空结论——于是行为退回引入复盘之前（照常提醒、仍不提交判失败），不是把整章判死。`isRescueTranscript` 在 `maybeRescue` 的第一道闸上挡住 rescue-of-rescue。转录名从时间戳改成 `rescue_<章>[.try<目标尝试号>].jsonl`（与转换/核对/修复同一口径），同名文件在开新会话前先退掉，所以重来与跨轮次都不会把两段对话串进同一个文件（`NewTranscript` 是 `O_APPEND`，这一点必须显式做）。
- **C：状态当场可见**：`markTargetActivity` 把状态写进**目标**转录（有会话本体就走它自己的 writer，避免 P55 的工作区快照行号错位；没有才退到 `appendTranscriptActivity`）。复盘开始写 `rescuing`；判 `truncate` 时先写 `queued`、再 `rescueQueueTruncate`，判 `retry` 时先写 `queued`、提醒轮拿到槽位后再写 `running`。`truncate` 之后那一章仍按失败收尾，所以 `Runner` 上留一个"已排队"标记（`rescueQueued` map + 互斥量），两处失败终态看到它就把 `error` 写成 `queued`——否则界面会被最后那条 `error` 拉回「会话错误」，正是用户要求避免的。Go 侧的活动状态折 live 的开关补上 `rescuing`，web 侧 `CHAPTER_ACTIVITY` 补「复盘处理中」。
- **D：嵌套子章节的复盘**：用户的假设（`convertChapterChildren`/`convertChapterNode` 的嵌套路径没接 `remindOrRescue`）**只对了一半**。普通的嵌套子章节走的仍是同一个 `remindOrRescue`（同一个函数处理所有深度，`splitTool.Requested == false` 那条分支），复盘本来是接上的；真正没有提醒/复盘的只有一条分支——**自己又细分过的节点**，它的整合轮没提交时直接 `return fmt.Errorf("细分父章节整合会话未提交")`，于是外层只看到"子章节转换或 checker 失败"，那条转录停在"整合完请 submit"上。现已并入同一条链路（同样的 `maybeRescue`、同样的 trigger 与结论处理）。另一个叠加因素是 `latex.rescue.keep_records` 默认 `false`：复盘当时可能真的跑过，但转录没落盘，事后只能看见同一会话里的提醒——这也是"从没被复盘过"的观感来源之一。新测试跑真实三层细分树（父 → child_01/child_02 → 孙章节），孙章节正常转换+核对，child_01 的整合轮不提交：恰好一次复盘、`truncate` 之后恰好一条 `role=continue` 的请求指向 `.../_subdivision/child_01/node/sessions/convert_chapter_01_child_01.jsonl`，且这条路径里不再往同一个坏会话发提醒。
- **E：复盘行的层级（`sessionview` + `web`）**：`chapterIdentity` 认 `rescue_` 前缀，且要求剩余部分**整段**是一个章节 ID（`^chapter[_-]?\d+(?:_child_\d+)*$`）——于是带章节身份的复盘落进章节阶段组（树才画得出），老命名的 `rescue_<章>_<时间戳>` 整段不匹配、仍按 P58 留在「错误处理」组；`TitleFor` 给「复盘 · <章>」。web 侧 `ChapterTreeNode` 加 `rescues`，`buildChapterTree` 单独收集复盘会话（与修复同一个池子的写法），`rescuesForNode` 按章节 ID + 世代挂靠、旧 → 新排序（**不按尝试号过滤**：尝试 2 的复盘不能因为这一行切到尝试 3 就消失，标签里写着 `#N`），钉住复盘时按名字里的 `.tryN` 选那一代那一次转换；`chapterAttemptChain` 对复盘一律返回 `null`，也不让它进成员列表；`ChapterSessionTree.vue` 在修复行之后加一组 `.chapter-rescue` 行（同层 26px 左边距、`.rescue-marker` 文案来自 `rescueRowLabel`、行 title 由 `rescueRowTitle` 给出"复盘对象"），`nodeContainsCurrent` 把复盘算进"装着当前会话的那一支"。
- **测试**：Go 侧新增 `rescue_retry_test.go`（4 条：两次无结论后第三次拿到结论且 `.trash` 里两份、三次都无结论时放弃并退回提醒且源码里剩 0 条复盘转录、`truncate` 的 `rescuing → queued` 顺序与 `queued` 终态、`retry` 的 `queued → running` 顺序）、`rescue_nested_test.go`（三层细分树的 D 用例）、`internal/sessionview/scan_rescue_identity_test.go`（3 条：章节身份三形态、扫描后的角色/阶段/标题/JSON 字段、`rescuing` 不算 live）；改了两条既有用例（`rescue_fix_sync_test.go` 的回执改看 `ToolResult.Terminal`、`rescue_fix_plan_test.go` 的"只记计划"回执断言不是终态）。web 侧 `web/tests/chapter-tree.spec.ts` 新增 6 条（复盘挂靠与排序、标签/title、`chapterNodeHasRows` 三来源含复盘、不进尝试链、钉住复盘的选行、`rescuing` 文案与既有状态文案不变）。`go test ./internal/... -count=1` 全绿、`go vet ./internal/...` 干净、`gofmt -l` 对改动文件无输出（`internal/latex/stylefix.go` 的未格式化是既有状态，未动）；`npx vitest run` 250 条全绿，`vite build` 重建 dist 并提交。
- **没覆盖到的**：① 复盘转录的 `.trash` 命名用秒级时间戳 + 撞名序号，跨进程同一秒内多次丢弃只会保留两份（`-2` 之后的会被覆盖），复盘转录本来就是一次性的，没有做更强的保留策略；② 前端没有组件渲染测试，复盘行的落位仍由数据层断言（`node.rescues` + `chapterNodeHasRows`）间接保证；③ "复盘自己连续无结论"只在小模型假客户端下验证，没有在真实厂商的空回复/超时上跑过。

## v1.5+（P58 补：复盘按目标编号成历史、并入章节树并隐藏旧阶段，2026-10-23）

复盘会话此前有两个尾巴：转录名里的 `.tryN` 是"目标转录的尝试号"，于是同一目标第二次复盘会复用文件名、开新会话前先把上一份退掉（等于删掉一段已经写完的复盘记录）；老命名（`rescue_<章>_<时间戳>.jsonl`）拿不到章节身份，只能堆在侧栏最后一组里，既进不了章节树也没有自己的历史。这一批把命名、归属、历史与编号一起收口，预览页的其它行为不动。

- **每次复盘拿自己的一号（`internal/latex/rescue.go`）**：`.tryN` 改成"这条目标被复盘的第几次"，由 `nextAttemptNumber`（`attemptNumberOnDisk` 的最大号 + 1，与转换的 `lastAttemptNumber` 同一口径）在开跑前算出并传进 `rescueReviewOnce`；"上一轮同名先退掉"整段去掉，只有**没拿到结论**的那一次仍退进 `work/sessions/.trash/`。开跑前还往复盘自己的转录写一行 `t=meta kind=rescue_target`（`chapter` / `reviewed_role` / `reviewed_transcript` / `reviewed_attempt`），它是预览页补身份与历史链分目标的唯一依据。
- **老命名按 meta 读时补身份（`internal/sessionview/sessionview.go`）**：`chapterIdentity` 仍只认文件名；`Scan` 在它给不出 ID 且名字是 `rescue_` 前缀时调 `rescueLegacyChapter`——先看 `rescue_target.chapter`，退一步用系统快照里的 `session_label: rescue:<章>`，形状校验通过就得到与带编号转录完全相同的章节身份（父章/深度/角色），标签与标题按规范名 `rescue_<章>.jsonl` 推导；两条都没有的仍落「错误处理」。`SessionInfo` 新增 `reviewedRole` / `reviewedTranscript`。
- **历史与树（`web/src/legacy/sidebar.ts` / `HistoryPanel.vue` / `ChapterSessionTree.vue`）**：`chapterAttemptChain` 对复盘会话返回"同一章 + 同一目标"的复盘（新 → 旧，`rescueTargetKey` = `reviewed_role|reviewed_transcript`，老转录归章节内的未知目标桶）；行标「复盘#N」由 `rescueOrdinal` 按 mtime 数出，树行标签「复盘 #N」同一来源，归档带「旧」；钉住一条复盘时按 `reviewed_transcript` 精确配转换（老转录不钉、不猜号）。历史表头写「复盘了 转换#1」（角色取 meta、轮次取被复盘转录名里的 `.tryN`），行里另给一枚「复盘了 …」小标签；`pairOf` 不再给复盘行配"同轮核对"。
- **验证**：`go test ./internal/... -count=1` 全绿（新增 `rescue_attempts_test.go` 两条：同一目标两次复盘得到 `.jsonl` + `.try2.jsonl` 且两份都留 `run_end` 与目标备注、下一号只看磁盘；`scan_rescue_legacy_meta_test.go` 三条：老命名 + 目标备注补身份、只有会话标签也能补、什么都没有仍留「错误处理」），`go vet ./internal/...` 干净、`gofmt -l` 对改动文件无输出；web 侧 `npx vitest run` 253 条全绿（`chapter-tree.spec.ts` 79 条，含复盘行按目标编号、不同目标各自成行成链、钉住按目标转录），`vite build` 重建 dist 并提交。`npx tsc --noEmit` 只剩 `data.ts` / `legacy/trajectory.ts` 的既有报错。
- **没覆盖到的**：① 文件名里的号只在**章节**内递增（同一章的不同目标共用一段号段，靠 meta 区分目标），界面显示的号按目标重数——两者不一定相等，这是有意的（防覆盖优先）；② 老转录没有 `reviewed_transcript` 时，"被复盘的会话"与目标分桶都是未知，只能按章节 + mtime 推断；③ 前端没有组件渲染测试，表头/行标仍由数据层断言间接保证。

## v1.5+（P62 第二轮：测试文件分组、夹具入库、章节树用例拆分，2026-10-23）

P62 第一轮只做审计（测试文件"只出表、不搬"），这一轮把那张表落地，**行为一字未改**——没有测试逻辑与断言改动，除了夹具路径常量没有生产代码改动。**分组**：`internal/latex` 87 个测试文件里 26 个按六组加前缀——`chapter_` 7（`chaptercoverage`/`chaptergate`/`chapterissues`/`chaptersize`/`checker`/`retry`/`slots`）、`rescue_` 9（`p58_rescue`/`p58_rescue_attempts`/`p58_resume_invariant`/`p60_fix`/`p60_fix_budget`/`p60_fix_dispatch`/`p60_fix_sync`/`p61_nested_rescue`/`p61_rescue_retry`）、`tools_` 2（`vfs`/`sandbox`）、`style_` 4（`stylefix`×3 + `pilot`）、`env_` 4（`docindex`×2 + `pythonenv`×2）；`motif_*` 12、`tools_*` 4、`chapter_subdivision*` 2、`rescue_triggers` 本就在组里，其余 42 个与六组无关（书级装配/编译/成本/交付/提示词/进度/看图/分块/项目/嵌入…）一个都没动；`internal/sessionview` 的 8 条扫描器用例改 `scan_*`（`p58_rescue_stage`/`p58_rescue_legacy_meta`/`p59_checker_anchor`/`p60_fix_target`/`p61_rescue_identity`/`t200_scan`/`t207_activity_order`/`t210_staged_product`），另 19 个非扫描器用例不动（`t201_child_retry` 是重试接口、`label_t62`/`live_t74`/`number_t77` 的号是后缀且与扫描无关、`review_fixes_test.go` 内容属工具与挂载，硬套前缀只会更难认）。文件名不再带 `pNN`/`tNNN`（号留在**文件正文**注释里），文档与注释里 29 处旧路径（文档 26 + 注释 3）同步改（含测试文件之间的一处交叉引用）。**夹具**：两份配置改写稿副本进 `internal/config/testdata/`（`temp/` 原件保留），`runconfig_rewrite_test.go` 改读 `testdata/`、删掉 `flatten` 的 `t.Skipf` 与三处 `os.Stat` 守卫，四条用例现在**总是**跑（原来是"检出里缺文件就跳过"）；`.gitignore` 的 `test*/` 会连 `testdata/` 一起忽略，补了 `!go/internal/config/testdata/`；夹具里的本地网关域名换 `api.example.com`、头部注明样例项目 `example_book`、凭据保持占位符。**拆分**：`web/tests/chapter-tree.spec.ts`（1002 行 / 79 条）拆成 `chapter-tree.spec.ts` 38 条、`chapter-chain.spec.ts` 9 条、`chapter-fix-rescue.spec.ts` 32 条，夹具构造器进 `web/tests/helpers/chapter.ts`（`SESSIONS_DIR`/`archiveDir`/`idWithTry`/`liveId`/`archivedId`/`fixId`/`session`/`parentOf`），各 describe 的本地 helper 保留原签名，只把拼路径那一段换成共享函数（mtime、字段与锚点一字未改）。**验证**：`go test ./internal/... -count=1` 14 个包全绿、`go vet ./internal/latex/ ./internal/sessionview/` 干净；`npx vitest run` **22 个文件 253 条**全过；拆分前后对比 `expect(`/`it(` 行集合，298 行逐条相同。

## v1.5+（P58/P60 补：树上复盘与修复各只留一行，编号只进历史，2026-10-23）

用户看截图指出两件事：树上「复盘 #1 / 复盘 #2」这种号放在外面看不懂，而且修复会话每修一轮就在节点下面多一行「修复 child_02 #1 / #2 / #3」，同一处被修三轮、复盘两次就把树叠成流水账。口径改为**树上每种只留一行，号只在「历史」页签里**。

- **数据层（`web/src/legacy/sidebar.ts`）**：新增 `newestByKey` 与两个可单测的纯函数 `treeFixRows(node)`（按 `fixTargetId` = 子章节/锚点去重，取 mtime 最新一轮）、`treeRescueRows(node)`（按 `rescueTargetKey` = `reviewed_role|reviewed_transcript` 去重，取最新一次）。`node.fixes`/`node.rescues` 池子**不变**——历史页签、钉住选行、`chapterNodeHasRows` 都还要用全部条目；只是树渲染时再过一层。`chapterAttemptChain` 一字未改（仍给全部轮次，新 → 旧，`修复#N`/`复盘#N` 标签只出现在那里）。
- **组件（`ChapterSessionTree.vue`）**：修复行 `v-for="fx in treeFixRows(node)"`、复盘行 `v-for="rs in treeRescueRows(node)"`，标签固定「修复」/「复盘」（归档前缀「旧」由 `kindLabel` 给），去掉了行内的 `#N` 与 `.tryN` 角标；轮数、最新一轮结局、被复盘的会话仍在行 `title`（`fixRowTitle`/`rescueRowTitle`）。不同目标（转换/核对/修复）的复盘仍各占一行，互不遮挡。
- **测试**：`web/tests/chapter-fix-rescue.spec.ts` 新增 describe「树上每种只留一行（编号只进历史）」三条——三轮修复树上只剩最新一轮且历史里三轮都在（新 → 旧）、同一目标两次复盘树上只一行且链里两次都在、复盘了转换与核对两个目标时树上两行且各自历史独立（单条目标返回 `null`）。`npx vitest run` 22 文件 **256 条**全绿，`vite build` 重建 dist 并提交。
- **没覆盖到的**：树行仍然没有组件级渲染测试（落位由 `treeFixRows`/`treeRescueRows` 的数据层断言保证）；「最新一轮」按 mtime 判定，mtime 相同的极端情况下取池子里靠后的那条（`>=` 覆盖）。

## v1.5+（T221：核对退化/放弃/视图失败不再喂反馈轮，2026-10-23）

用户贴出截图：同一条转换会话里，「checker 会话已退化，已排队从干净轮次继续核对」下面紧跟着「The checker reviewed your submitted chapter and found problems that must be fixed」+「Fix them in your workspace … then submit the two paths again」。核对在那一刻**没判定过章节**，第二句让去改内容纯属自相矛盾。先确认这是真缺陷（不是用户跑旧版造成的错觉）：`checkerNoVerdict()` 的识别表漏了退化路径的文案，反馈轮照旧发了出去。

- **识别表（`internal/latex/checker.go`）**：`checkerNoVerdict()` 从三个前缀扩到六个，补上核对"没判定就返回"的三条路——`checker 会话已退化，已排队从干净轮次继续核对`（复盘判 `truncate`）、`checker 复盘判定会话无法挽救：…`（判 `fail`）、`checker 只读视图无法准备`（视图没铺起来，strict 模式判失败）。`convertChapterNode` 的反馈轮守卫本来就是 `!checkerNoVerdict(issues)`，一行调用侧代码没改；命中时多打一行「核对会话没有给出结论，跳过反馈轮」。
- **刻意不动的部分（升级链）**：跳过反馈轮之后仍走 T217 那条路——按 `checker_rounds` 归档这一版、换全新会话重转换，次数用尽判失败。没有改成"留产物 + 强制重核"，因为顶层重跑遇到已有 `<章>.tex` 会走 `book.go` 的「已有产物，跳过」并直接 return（连核对都不跑），留产物等于让下一次运行静默采纳一份没核对过的东西。那条要动得先改重跑跳过的语义，单独定。
- **测试**：`t209_checker_noverdict_test.go` 的识别表补三条字符串（`real` 一侧不动，继续保证内容问题不被误判成"没结论"）；新增 `checker_degrade_test.go`，在 `rescue_triggers_test.go` 的夹具上加了 `checkerNoSubmit` 开关（核对会话跑完只回一段正文、从不调 `submit`），跑真实 `convertOneChapter`：断言转换会话一条反馈消息都没收到、复盘确实跑过、`truncate` 排出的队列请求是 `role=checker`、这一章仍按失败收尾、`stats.convert >= 2`（换会话重转换没被跳过）。把三个前缀临时删掉重跑，用例红在"却收到了第 4 条反馈消息"并打印出用户截图里那一段原文，恢复即绿——这条用例确实复现了现场。
- **验证**：`go build ./...`、`go vet ./internal/...` 干净；`go test ./internal/... -count=1` 14 个包全绿（`internal/latex` 21.5s）；`gofmt -l` 对改动文件无输出（`internal/latex/stylefix.go` 的未格式化是既有状态）。
- **没覆盖到的**：① 这三类"没结论"最后仍会把这一版产物作废，与「已排队从干净轮次继续核对」的措辞并不完全一致——那句话说的是"排队重核"，实际发生了什么见上（保产物会导致下一次运行静默跳过核对，这条取舍要用户定）；② 只在小模型假客户端下验证，没有在真实厂商的空回复/超时上复跑；③ 用户现场跑的是这批提交之前的二进制，旧行为会一直存在到重新构建。

## v1.5+（T222：核对没给结论时只重核、不作废产物整章重转，2026-10-23）

用户接着追问：「所以这个退化是啥意思？为什么要重转？不是重新核对就可以了吗？我还是没懂」——他的直觉正是代码该做的事。核对退化时界面承诺的是「已排队从干净轮次继续核对」，代码做的却是归档这一版产物、删掉、换全新会话整章重转。产物那一刻根本没被任何核对判定过，重转出来的是同一份东西：重做一遍不会更有结论，也没有理由假设新的转换会话能救回一个坏掉的核对模型；确定被消耗掉的只有时间与额度——核对模型持续退化时，这一章会被作废重转最多 `checkerAttempts(depth)` 次（顶层 6），每次再开若干核对会话、每次再触发一次复盘。`rescueQueueTruncate` 排出的那条 `role=checker` 重核请求随之落空：产物已被删，`recheckChapter` 只会回「这一章还没有提交产物，无法重新核对（先重试转换）」。

- **保留产物、只重开核对会话（`internal/latex/book.go`）**：`convertChapterNode` 的反馈轮之后原本直接落进"核对不过"的同一个分支（写 `.checker` → `archiveChapterAttempt` → 删 `.tex` 与分片 → `convertChapterNode(..., true, next)` 换会话重来）。现在先分出一条：`checkerNoVerdict(issues)` 为真时**不动产物**，只重开核对会话核这一份（`checkerNoVerdictRetryCycles = 1`，同一份只读视图、全新会话）。重核拿到结论就按结论走——通过就在原处收账，真指出内容问题才落到原来那条归档重转链路（内容问题该重转，这一支一格没动）。重核仍没结论就以「checker 不可用」收尾：`<章>.tex` 留在原处不归档不删、`.checker` 留那句话、`chapter_activity` 记 `error`，错误文案写明「不是内容问题，N 个核对会话都没下判定，已保留这一版产物未作废——重跑或「重新核对」会接着核这一份」。
- **补上"留产物"的前提：跳过不再无条件放行（`internal/latex/checker.go` + `book.go`）**：T221 当初不敢留产物的理由是真的——顶层重跑遇到已有 `<章>.tex` 会走「已有产物，跳过」并直接 `return nil`，连核对都不跑，留产物等于让下一次运行**静默采纳一份没被判定过的东西**。新增 `chapterRecheckPending()`（读 `texPath+".checker"`，内容是 `checkerNoVerdict` 认得的文案即为真）挂在那条跳过分支上：命中就先走 `recheckChapter`（与页面「重新核对」同一条通路、产物一个字节不动），通过才 `keepSessionFile` + 跳过，没过如实报错并说明"这份产物未被采纳"。于是"保留产物"与"绝不静默采纳未核对产物"两条不再打架，T221 留的那个口子消失。
- **刻意不动**：① 复盘判 `fail`（会话无法挽救）与判 `truncate` 走同一条"重核一次"的路——对产物来说两者处境完全一样（都没被判定过）；② 重核只加一轮，不做无限重试：真坏掉的核对模型再开十次也一样，不如交给人和下一次运行；③ `rescueQueueTruncate` 排的请求保留，现在它核的是留下来的这份产物，跑通一次就把 `.checker` 收掉，下次运行自然"已有产物，跳过"。
- **测试（`internal/latex/checker_degrade_test.go`）**：`TestCheckerDegradeDoesNotFeedConvertSession` 的旧断言 `stats.convert >= 2` 反过来成了 `== 1`（那正是要命的旧行为），并加断言：核对会话 ≥ 2（初核 + 重核）、`work/chapters/<章>.tex` 仍在、`work/attempts/` 空、失败文案含「没有给出结论」、`.checker` 里是"没结论"那类文案。新增 `TestPendingNoVerdictProductIsRecheckedNotSkipped`：夹具加 `preseedNoVerdict` 开关预铺"产物在 + `.checker` 是退化文案"，断言重跑**不跳过**——`stats.convert == 0`、核对跑 1 次、通过后 `.checker` 被收掉。夹具顺手加了 `stats.checker` 计数（原来只数 convert/rescue）。
- **验证**：`go build ./...`、`go vet ./internal/...` 干净、`gofmt` 只报既有的 `stylefix.go`；`go test ./internal/... -count=1` 14 个包全绿。
- **没覆盖到的**：这件事的触发条件是"核对模型持续退化"，而真跑起来第一次重核成功与否取决于模型当时的状态——测试里钉的是**路径**（不重转、保留产物、跳过前先重核），不是"重核一定成功"。真机上仍可能连续两个核对会话都没结论，那时这一章会停在"checker 不可用"+保留产物，重跑或点 ↻ 接着核，这条要看用户现场是否还会撞上。

## v1.5+（T223：复盘的审计日志不再被当成会话，2026-10-23）

用户贴出侧栏截图：章节组下面还有一个阶段名就叫「会话」的分组，里面一条会话标题 `log`、状态「未开始」，问「还有一个 log 会话不知道是什么」。查现场索引（`/api/index`，2600 条会话）只有一条 `stage == "会话"`，它的路径是 `<项目>/work/rescue/log.jsonl`——P58 复盘的审计日志，每行一条复盘结论（章节 / 角色 / `verdict` / 提示词 / 转录行数），592 行、258 KB。它不是任何一次会话，只是文件名以 `.jsonl` 结尾。

- **根因（`internal/sessionview/sessionview.go`）**：`scanner.scan()` 的 walk 只按后缀收候选、只对**目录**有 `skipScanDir`，所以这个文件被当转录读进来；`LabelFor` 的对不上任何已知前缀，退回 `会话:<stem>`，标题 `log`、阶段 `会话`（`stageTitleOf` 拿不到映射就原样返回阶段名）。没有 `msg` 行 → 消息数 0 → 状态「未开始」；复盘每写一行就更新 mtime，看着还像 live。T200（`work/rounds` 快照日志）与 T202（`.trash` 截断备份）是同一类问题的前两次，前两次都是"目录不是会话"，这次是"单个文件不是会话"。
- **改**：新增 `skipScanFile(p, name)`，只跳 `<项目>/work/rescue/log.jsonl`（父目录 `rescue` 且祖父目录 `work` 同时成立才跳），在 walk 里紧跟"是不是 `.jsonl`"之后调用。**没有动侧栏分组逻辑**——分组没错，错的是把一个非会话文件扫了进来。审计日志的写入（`rescue.go` 的 `work/rescue/log.jsonl`、`keep_records`/`--debug` 语义）一字未动；复盘转录 `rescue_<章>[.tryN].jsonl` 住在 `work/sessions/`、名字也不同，不受影响。
- **测试（`internal/sessionview/scan_dirs_test.go`）**：`TestScanSkipsRoundsJournalsAndCleanBackups` 加一条 `work/rescue/log.jsonl`（仍只留真正那条转录，扫描到 1 条）；新增 `TestSkipScanFile` 钉住边界——`work/rescue/log.jsonl` 跳，`work/sessions/log.jsonl`、`rescue/log.jsonl`（项目名恰叫 rescue）、`work/rescue/rescue_chapter_001.jsonl`、`log.jsonl.bak` 都不跳。
- **验证**：`go build ./...`、`gofmt -l`（改动文件无输出）、`go test ./internal/sessionview/ -count=1` 全绿（17.0s）。对真实数据做只读复扫（临时程序调 `sessionview.Scan("…/latex_project")`、跑完即删）：2604 条会话、落「会话」组 **0** 条；修前那个索引里正好是 1 条。
- **没覆盖到的**：① 侧栏那枚方块里的 `1` 是分组的行数徽标，不是会话名——修完这组直接不存在，没有为它单独做什么；② 只用只读复扫验证了索引层，页面渲染没重跑（dist 没改、不需要重建 web）；③ 现场跑的是旧二进制，要重新构建才会看到这一组消失。

## v1.5+（T224：同一章的复盘在树上只占一行，以前的复盘都进它的历史，2026-10-23）

用户贴出侧栏截图：一个章节节点下面竖着六、七行「复盘」，最后一行标「运行中」、上面几行只有「复盘」两个字；点进任何一行只有它自己那次对话。他的原话是「之前的复盘不要这样显示吧，显示在这个复盘会话里面的历史中，不要这样堆在这里，这里只能有一个复盘。并且之前的记录这边都找不到，我希望你帮我迁移一下之前的复盘记录到正确的位置上」。三件事：树上每章只留一行；以前的复盘进那一行的「历史」；那些记录要**真的归位**，不是把行藏起来。

- **根因（旧口径是"一行一个复盘目标"）**：`sidebar.ts` 的 `rescueTargetKey(s)` 用「这一章 + 它复盘的那条转录」（`reviewedTranscript`，来自转录里的 `t=meta kind=rescue_target`）当树的 key，`newestByKey` 再按 key 取最新。P58 之前（以及复盘队列早期）的转录名是 `rescue_<章>_<时间戳>.jsonl`，翻真转录确认里面只有系统提示词、`price` 与 `run_end`——**没有** `rescue_target` 那一行；index 下发的 `reviewedRole`/`reviewedTranscript`/`chapterAnchor`/`archived` 全空、`chapterAttempt` 兜底成 1，于是 key 退化成空串，`newestByKey` 只能退回按 `rs.id` 兜底 → 一份老转录一个 key，同一章复盘过几次就叠几行。雪上加霜的是每行自己又是"只有一次复盘"的链（链也按目标分桶），所以每行都没有「历史」页签：行数堆着，记录谁也看不见。
- **只读摸清现场**：`/api/index`（2621 条会话）里 623 份复盘散在 213 章上，130 章超过一次（最多的 13 次）。取其中一章逐份打开看：12 份老命名 + `rescue_<章>.jsonl` + `rescue_<章>.try2.jsonl`，按旧口径就是 14 行；而审计日志 `work/rescue/log.jsonl` 里对应的记录只有 `chapter`/`role`/`verdict`/`reason`/`from`/`hint`/`model`/`transcript_lines`/`ts`，**没有**被复盘转录的文件名或尝试号（有一份 log 的 `ts` 与某条老转录最后一行 meta 的时间戳逐字相同，但这也只能说明"是那一次"，推不出文件名）。
- **改（`web/src/legacy/sidebar.ts`）**：① 新增 `chapterRescueRows(sessions, rescue)` = 这一章、同项目的**全部**复盘（含更早父尝试留下的归档世代），按 mtime 旧 → 新；② `treeRescueRows(node)` 改成**这一章只留最新一行**（mtime，并列取后面那条，与 `newestByKey` 同口径）；③ `chapterAttemptChain` 的复盘分支不再按目标过滤，直接用 `chapterRescueRows`，新 → 旧；④ `rescueOrdinal` 与 `treeRescueRows` 共用同一份集合，所以树上、历史行、历史链的号一定对得上（`复盘#N` = 这一章的第 N 次，读时推断）；⑤ 删掉 `rescueTargetKey`。
- **改（界面）**：`ChapterSessionTree.vue` 的 P61 注释改成"每章一行"（渲染分支没动，`v-for` 现在恒为 0 或 1 项）；`HistoryPanel.vue` 的链表头从「同一处目标的复盘（复盘了 转换#1，N 次，新 → 旧）」改成「这一章的全部复盘（<章节后缀>，N 次，新 → 旧）」，删掉不再用的 `reviewedHead`、新增 `rescueChapter`；`chainTagTip` 改口「这是这一章的第 N 次复盘：<复盘了谁>」；行内 `复盘#N` 与「复盘了 …」逐行照旧。
- **迁移为什么在读取侧做**：要让老记录"进到正确的位置"，可以改名把老转录变成 `rescue_<章>.tryN.jsonl`，也可以改读法让它们自然归位。选后者。老转录**没有记录它复盘的是哪一条转录**，改名只能编一个猜出来的 `.tryN`——这跟"不编造转录没记录过的元数据"直接冲突；而且只改名字、不补 `rescue_target` 行，界面照样读不到目标。读时归位则一份不丢、磁盘一个字节不改、正在写的那条转录也不会被碰到（摸现场时正好有一次复盘在跑）。`rescueRowTitle` 对老转录继续给「复盘对象：<转录名去掉 rescue_ 前缀>」，不猜角色与轮次。
- **测试（`web/tests/chapter-fix-rescue.spec.ts`）**：改掉三条与新口径冲突的用例——「同一章两个目标各占一行、各算各的号」→「号按全章连着数、树上仍只占一行」；「复盘链只列同一章 + 同一目标」→「列这一章的全部复盘，只有一次时没有页签，别的章不混进来」；「复盘了不同目标各占一行」→「树上只有一行，两次都在它的历史里（并断言行上的 `复盘了 核对#1/转换#1`）」。新增「T224：老复盘转录没有目标备注，也不再一章叠成好几行」——三份无 meta 的老转录 → `treeRescueRows(node).length === 1`、链里三份（新 → 旧）、`rescueOrdinal` = 1/2/3、`reviewedTargetLabel` 给空串。
- **验证**：`cd web && npx vitest run` 22 个文件 257 条全绿；`npm run build` 通过，dist 已重出（`viewer.js` 544.73 kB / `viewer.css` 87.45 kB）。真实数据只读验算：临时写一条一次性 vitest 用例拉 `/api/index` 跑完删掉——目标章 13 份复盘 → `treeRescueRows` 1 行、`chapterAttemptChain` 13 条、序号 1..13 无重复；全库 130 个曾经超过一行的章节节点，改造后每个最多一行。
- **没覆盖到的**：① 老转录的「复盘对象」只能显示它的转录名（`rescue_<章>_<时间戳>` 之类），真正的"复盘的是转换第几次"永远补不出来——这是磁盘上没有的事实，不再尝试猜；② 页面渲染只靠单测 + dist 构建验证，没有在浏览器里重跑截图（dist 已重出，但要重新构建 `docvision` 才会看到）；③ 用户现场跑的是这批提交之前的二进制。

## v1.5+（T226：实时进度块按屏幕高度截断、只重画变化的行，2026-10-23）
- **问题（用户报）**：几十条会话一起跑时控制台"一直在刷新和频闪"，那片进度区比屏幕还高，"滚动到上面就看不到了"，想要"驻留固定屏幕的会话"。
- **根因**：① 块高 = 行数、没有上限；终端覆写靠 `\x1b[<n-1>A` 上移 + 逐行清空，`n` 超过屏幕高度时上移被终端夹在顶部，接着打印的换行只能靠向下滚动腾地方——"擦除"实际是把 scrollback 里的历史推出屏幕。② `refreshLiveLocked()` 先擦后**全量**重画，1 秒一跳的计时器让整块每秒重绘一次；收尾又是 `Remove()` 重画 + `PrintConsole()` 擦/画各一次。
- **改（`go/internal/logger`）**：新增 `termsize_unix.go`（`TIOCGWINSZ`）/`termsize_windows.go`（返回 0）与测试钩子 `TermRowsForTest`；`liveCap() = 终端行数 − 6`（下限 3，查不到按 24 行）；`liveBlockTextsLocked()` 超限只画前 `cap-1` 行 + 末行折叠提示；`liveRowEntry.prio` + `LiveRow.SetWithPriority()` + `liveOrderLocked()`（`sort.SliceStable`，同优先级保持创建顺序）；`paintLiveLocked()` 改增量——逐行与 `painted` 比对只重写变了的行，行数变化才整块重来；`eraseLiveLocked()` 清 `painted`；`LiveRowTexts()` 按显示顺序但列全部行。`Finalize` 保持原语义（摘行 + 定格行 + 重画一次）。
- **改（`go/internal/latex/book.go`）**：新增 `rowStatePriority()`（运行 0 / 等子会话 1 / 待核对 2 / 排队 3），render 走 `SetWithPriority`；阶段收尾从 `row.Remove()` + `PrintConsole(final)` 改成 `row.Finalize(final)`，一次收尾不再两遍整块重画。
- **测试（`internal/logger/livepanel_screen_test.go`，沿用 ANSI 回放屏幕模型）**：`TestLiveBlockBoundedByTerminalHeight`（12 行终端 + 22 条会话：屏上 ≤ 6 行、末行「另有 17 个会话」、两条运行中都在、排队第 20 个不上屏、字节流最大光标上移 ≤ 5）、`TestLiveBlockHiddenRowStillFinalizes`、`TestLiveBlockRepaintsOnlyChangedLines`（a/c 各只写 1 次）、`TestLiveBlockPipeKeepsAllRows`（管道不截断、无折叠提示）。
- **验证**：`go build ./...`、`go test ./internal/logger/ -count=1`、`go test ./internal/latex/ -count=1`（32.5s）全绿；另用一次性可视化用例（30 行终端 + 36 条会话）目检屏幕：运行中/等子会话在前、排队在后、末尾一行折叠提示，跑完删除该用例。
- **口径**：截断只发生在屏幕上——日志文件、`--console-verbose` 与管道输出仍每个会话每一行；被折起来的会话收尾时定格行照样落进滚动区，账不省。

## v1.5+（T227：原书视图就绪顺序、截断真的会生效、转录体检与复读提醒，2026-10-23）
- **问题（用户报）**：转换会话大面积陷在同一个死循环里——`submit` 回「还没有和原书对照过…你已经看了 0 页」、`view_pdf` 回「未知挂载点 "source"」，复盘判截断却从不生效，转录里重复提醒与重复总结越堆越多（「这么多的污染内容，而不是从一个最干净的地方截断？…你检查一下？」→「主要是感觉现在好多会话都陷入到了这个情况，导致几乎都没办法继续推进下去了」→「这些有非常多这种错误的会话，怎么处理一下。对这些转换会话处理一下再继续跑？」）。
- **根因①（顺序）**：`RunBook` 一开始就 `startRetryWatcher`，运行期任何时刻都能派发重试/继续；而 `buildPDFViewQuiet` / `buildDocIndexQuiet` / `checkPageAlignment` 排在**图片阶段之后**。中间被派发的转换会话建出来时 `r.pdfView` 还是 nil → `sessionMounts` 没有 `source` 挂载点 → 门禁仍要求"看过 2–4 页不同的原书页"（`needSourcePages()`）→ 一个不可能满足的条件 → 必然走完"看不了 → 打回 → 复盘 → 判截断 → 再拉起"。日志里**没有任何 `[pdfview]` 行**，说明不是配置或构建问题，是顺序。页计数没被截断清掉：`.gate/source.json` 只在 `node.Depth == 0` 重置运行（P55）时才被清，所以同一句「已经看了 0 页」会被反复撞到。
- **根因②（截断读错目录）**：`truncateChapterTranscript` 把目录写死成 `<项目>/work/sessions`；子章节转录实际在递归节点里（`work/temp/conv_<章>/work/_subdivision/<子章>/node/sessions/`），`os.ReadFile` 直接失败。
- **根因③（请求被改写）**：`dispatchRetries` 把命中子章节的 `convert`/`continue` 请求一律改写成"继续跑父章节 `From=0`"，请求里带的 `From`/`Path` 被丢掉；②③ 的失败只落 `LogWarning`（仅文件），界面照旧显示"已截断继续"。
- **根因④（逐字重复的提醒）**：`remindOrRescue` 两轮提醒在没拿到复盘 `hint` 时是同一句话，模型收到同一句话就照抄上一轮答复 → 同一句 `You have NOT submitted yet…` 与同一段总结在转录里越堆越多。
- **改（`go/internal/latex/book.go`）**：① 视图/索引/页对齐三步挪到 `startRetryWatcher` 之前（注释写明顺序理由）；图片阶段之后只保留"产物出来再补一次"。`buildDocIndexQuiet` 加指纹守卫（`docIndexStampOf` + `docIndexStamp` 字段）。② `convertChapterNode` 在 `node.Attempt == 1` 且工作区存在时先做转录体检，命中就换新的尝试转录接着跑。③ `remindOrRescue` 先算 `reminder`，`reminderInCircles(sess, reminder)` 为真（上一次提醒前后的两条助手正文一字不差）就不发第二次；新增 `assistantSaid`。
- **改（`go/internal/latex/retry.go`）**：`truncateChapterTranscript` 走新增的 `findChapterTranscript`（顶层 `work/sessions/` + `work/temp/conv_*` 下 `_subdivision` 递归找）；新增 `cutChapterTranscriptForRetry` 照请求的 `Path`/`From` 截；`dispatchRetries` 子章节分支先消费请求、再排父章节继续跑；截断/继续失败改 `LogError` + `phaseNote`。
- **改（`go/internal/latex/chaptergate.go` + `runner.go`）**：`chapterGate` 加 `NoSourceMount`（+ `Warn` 回调、只提示一次的 `noSourceNoted`）；`submitGate` 用 `len(r.sourceMounts()) == 0` 判定，会话确实没有 `source` 挂载点时就记一行警告、只要求覆盖检查。
- **新增（`go/internal/latex/repair.go`）**：`repairDerailedTranscript(trPath, workRoot) (path, attempt, why, ok)`。两条硬证据——工具回执里 `未知挂载点 "source"`、同一 N 的「还没有和原书对照过…你已经看了 N 页」拒绝重复 ≥2 次；**只在回放窗口内**（`replayStart` 找最后一条 `=== COMPRESSED SESSION CONTEXT`）、**只在最后一次成功看过原书页之后**（`lastSourceViewIndex`，回执形如 `PDF page source:`）评估，取最早那条证据；有收账的 `SUBMITTED` 一律不动；切点落在窗口起点之前就跳过。截断复用 `cutTranscriptAt`（源转录一字不动、新尝试写在旁边）。
- **测试**：`repair_test.go` 15 条（截断点、收账放过、单次拒绝放过、同 N 才认、不同 N 不认、压缩检查点之前跳过、尾巴只有元信息时到上一段找、之后看过原书页的不动、其它挂载点错误不认、干净未完成的不动、新尝试编号、无工作区跳过）；`rescue_triggers_test.go` 加 `repeatAnswer` 开关与两条口径用例（复读只提醒一次 / 答复每轮都变照发两次）。
- **离线体检（真实工作区，只读 + 一次性写盘后逐条核对）**：625 条最新尝试 → 58 条需截断、567 条干净；58 条逐个核对"行数与源文件前缀逐字节一致"全对，源文件均未被改动；与复盘审计里同一批判 `truncate` 的 195 条交集 **57/58**（切点：AI 偏早 25、偏晚 32、相同 0）。临时驱动用例用完即删，不入库。
- **验证**：`go test ./internal/latex/ -count=1` 全绿、`go build ./...`、`go vet` 干净。
- **口径**：提醒不能一刀切取消——实测 65 条发过第二次提醒的转录里 56 条答复变了、48 条最终真提交；只砍"复读"这一种。截断只退对话：不写 `.gate`、不动工作区与已提交产物。

## v1.5+（T228：截断当场落盘 + 词内下划线，2026-10-24）

- **触发**：用户贴了一份复盘会话转录，问截断结论提交后有没有**立刻**反映到 jsonl 与网页上（「提交截断之后，有没有在 web 界面显示的时候或者实时的就反应在那个 jsonl 上了，而不是等到并发的时候才去截断它…不然用户可能会错误的认为这个没有修复」）；同一份转录里用户打的章节名显示成没有下划线的样子。
- **缺陷 A（延迟截断）**：`maybeRescue` 的 `truncate` 分支只调 `rescueQueueTruncate` → 写一份 `work/retry/*.json`（`Role:"continue"`、`From`、`Path`）；真正的切口在 `dispatchRetries` 里（`cutChapterTranscriptForRetry` / `continueFromChapter`），要等并发槽。证据：复盘审计 `2026-10-23T12:52:02+08:00` 已写 `{"chapter":"…","from":6,…,"verdict":"truncate"}`，进度行也写了"会开一个新尝试"，而 `.try2.jsonl` 直到次日 `00:43`（离线体检）才出现——中间网页「历史」里只有原来那一条跑歪的会话。
- **改 A**：新增 `Runner.cutTranscriptNow(proj, base, role, trPath, from) (dst string, next int, err error)`——纯文件操作 + `retryAttempt` 记账（转换 `cutTranscriptAt`、核对 `truncateCheckerTranscript`）。`rescueQueueTruncate` 改成 `(int, error)`：先切（并 `applyWorkspaceCut` 把工作区也退回那一刻），再排队，请求新增 `retryq.Request.CutAttempt`（`cut_attempt`，omitempty）记住已切好的尝试号。派发端据此分流：`dispatchRetries` 的子章节分支只在 `CutAttempt == 0` 时切；`runRetryTask` 的 `continue` 分支 `From>0 && CutAttempt==0` 才走 `continueFromChapter`（切 + 推进），否则直接 `continueChapter`（续跑最新尝试，`book.go` 的 `retryAttemptOf` 覆盖让同进程内立刻选中新尝试、重启则靠 `lastAttemptNumber`）；`checker` 分支拆出 `continueCheckerAfterCut(proj, base, next)`（`retryCheckerAttempt` + 归档旧 `.checker` + `recheckChapter`），`CutAttempt>0` 时跳过切口。切不动时不装作切了：`CutAttempt` 留 0，退回派发时再切，日志与进度行都写"当场截断失败"。
- **缺陷 B（渲染吃下划线）**：`web/src/legacy/markdown.ts` 的 `MD_INLINE` 里 `__([^_]+)__` / `_([^_\n]+)_` 没有 CommonMark 的词内下划线规则，`chapter_012_child_02_child_03`、`convert_….jsonl` 的第一对下划线被当成斜体。转录原文一直是正确的，纯渲染问题。
- **改 B**：`mdInline` 保持 `MD_INLINE` 分组号不变（`__` 第 6 组、`_` 第 9 组），匹配后过滤：跨切片记录"当前切片前一个字符"，新增导出函数 `intrawordUnderscore(m, before, after)`——只有 `m[6]`/`m[9]` 命中、且任一侧紧贴 `[\p{L}\p{N}_]` 时返回 true，该次匹配回退成字面文本。**故意不用后行断言**：Safari <16.4 在解析阶段就 SyntaxError，整页白屏（模块级正则的 `g` 标志陷阱同源）。
- **测试**：Go 新增 `TestRescueTruncateCutsOnTheSpot`（提交后立刻读到 `.try2.jsonl`、6 行、源转录 10 行未动、请求带 `CutAttempt=2` 与 `work/` 下相对 `Path`）、`TestRescueQueueTruncateFallsBackWhenCutFails`（切点越界 → `attempt==0`、`CutAttempt==0`）；原 `TestRescueQueueTruncateQueuesContinue` 由这两条取代（`rescue_probe_test.go` 的匿名结构补 `json` tag，否则 `cut_attempt` 读不到）。另加 `TestDispatchDoesNotCutTwice`（两条子测试：`CutAttempt=0` 的老请求派发时照切一刀、`CutAttempt=2` 的已切请求不再切第二刀且父章节排上「继续跑」，前一条是后一条的对照——去掉守卫后者会多出 `.try3.jsonl`）；预览页那条"下划线斜体"探针的夹具原本把 `_斜体_` 夹在两个汉字之间（按新规则本该是字面），改用 `-` 划出词边界，保住"下划线斜体仍会渲染"的对照；Web 新增 `web/tests/markdown.spec.ts` 的 `T228 词内下划线不当强调` 5 条（章节名/转录名/`a_b_c` 字面、`_斜体_`/`__加粗__`/`~~删~~`/行内代码照旧、`a*b*c` 星号字面、`**要重跑**` 仍加粗、`intrawordUnderscore` 单元判定），配一个只实现 `createTextNode`/`createElement`/`createDocumentFragment` 的假 DOM（`node.textContent` 对无子节点返回自身 `text`）。
- **验证**：`go test ./internal/latex/ -count=1` 全绿（30.9s）、`go test ./internal/... -count=1` 全绿、`go build ./...`、`go vet ./internal/latex/ ./internal/retryq/ ./internal/sessionview/` 干净、`gofmt -l` 只剩既有的 `stylefix.go`；`cd web && npx vitest run` 22 个文件 262 条全绿，`npm run build` 重出 `viewer.js`（545.31 kB）后再 `go build` 通过。
- **口径**：① 网页 ⤓ 不改成当场写盘——`docvision sessions` 是只读观察者，工作区只归 `docvision latex` 动；回执补了"切口要等它排进槽位时才写盘：页面只读，不提前改你的转录"。② 复盘判 `truncate` 后给父章节排的仍是"继续跑 `From=0`"（T227 口径不变），子章节的截断点跟着请求里的 `Path`/`From` 走。③ 截断仍只动"新尝试 + 工作区回退"，不写 `.gate`、不删任何东西。

## v1.5+（T229/T230：临时实验目录的去向取证；同一章不再并排跑两条核对，跑歪的核对转录不再无限续跑，2026-10-24）

- **触发**：用户先问一份图像对比实验的临时目录为何"凭空消失"（是不是被 Agent 删了）；随后贴出核对页面的现场：「核对 `chapter_004_child_01_child_03` 的会话不会重试吗？我看他的复盘会话一直没有触发？并且这里面混杂了太多的这个错误内容」「我手动进行重新核对，我发现我的重新核对和之前的核对同时在跑，太奇怪了。没有停下来之前的」。
- **T229 取证（不改代码）**：该目录整体在一个未跟踪的 `temp/` 下（`.gitignore` 里、无 git 历史）、回收站为空、`/home` 是 ext4 且无快照，因此**没有任何恢复途径**。对照 P62 的整理记录，结论是它在前一轮"按 tracked 文本零引用判残留"的清理里被当成构建产物删掉——是 Agent 自己的错。教训写成硬规则 `AGENTS.md`「未跟踪数据不许按零引用删」，经过记在 `c140f61`。
- **T230 缺陷 A（并发）**：`convertChapterChildren` 里子节点在自己的 goroutine 里转换与核对，**一把章节锁都没拿**；`convertChapterBatch` 只对顶章 `holdChapter`。派发端 `dispatchRetries` 用 `lockChapter(base)`（TryLock，忙就留队）判"这一章能不能开跑"，于是同名子章节永远是"空闲"——页面的「重新核对」与父节点对同一支的重驱动并排开了两条核对。磁盘上能看到两处各写一条：节点目录 `…/child_03/node/sessions/checker_<章>.try9.jsonl`（319 行／515 198 字节、34 次 `run_end`、74 条章节活动、19 条提交提醒）与顶层 `work/sessions/checker_<章>.try4/5.jsonl`（51／55 行，都是页面「重新核对」在 18:33 前后开出来的）。
- **改 A**：`childBase` 提到 `wg.Add(1)` 之前算好；goroutine 里 `release := r.holdChapter(childBase)` + `defer release()`。语义是"子节点转换/核对期间这一章名归它"：派发端 TryLock 失败 → 请求留在队列里等它收尾，既不并排、也不打断。父子锁名不同、`holdChapter` 阻塞而派发端只 TryLock，所以不会互相等死。
- **T230 缺陷 B（无限续跑）**：`checkChapterAt` 只要转录文件还在就把历史回放给下一个核对会话。一次调用内部"没结论就重开、最多 4 次"（T217）是设计好的预算，但文件跨调用共享：父节点反复重驱动、页面反复点重新核对，都往同一条文件后面追加。T227 的转录体检也救不了这条路——它不是"忘了提醒"，是**提醒了也不提交**（那条 319 行的转录里 19 条提醒，一段结论没交）。
- **改 B**：`checkChapterAt` 进门先 `countCheckerSubmitReminders(loadTranscriptMessages(trPath))`；`>= checkerNoVerdictAttempts`（4，提为包级常量，与"一次调用内部的 4 次预算"共用同一个数字）就 `keepCheckerTranscript` 后如实返回「checker 多次未能提交结论：这条核对转录里已经攒了 N 次提交提醒…不再原地续跑它」，**不建会话、不回放、不追加**。产物仍按 T222 保留；干净会话走 ⤓ 截断或页面「重新核对」（另开一次尝试，提醒数从零算）。判据只看 `user` 消息是否以那句提醒开头，正文里写的 `submit` 不算。
- **T230 缺陷 C（判决落错地方，本轮只记录不改）**：复盘判 `truncate` 后 `truncateCheckerTranscript` 把前缀抄成新尝试写在 `work/sessions/`（T201 的决定：那里不随父节点临时树被清、页面「重新核对」也按同一位置找）；节点驱动的核对路径却是 `<节点>/sessions/checker_<章>.try<转换尝试号>.jsonl`。两条通路读的不是同一个文件，所以对"节点自己那条"的救援注定改不动它读的那条。这一章的审计日志有 18 条结论（核对 8、转换 10），两次核对 `truncate` 判在 17:17（74 行 → 68）与 17:24（84 → 69），而那条 try9 在 18:25 还长到 319 行。改通道映射会把两条通路搅在一起，本轮由 B 的入口守卫兜住"不再回放"。
- **改 C（页面口径）**：`internal/sessionview/retry.go` 的核对回执改成「只重跑核对会话，产物不动；同一章已经有核对在跑时，这一条会等它收尾后再开始——不会并排再跑一条，也不会打断正在跑的那条」。
- **测试**：`checker_degrade_test.go` 新增 `TestCountCheckerSubmitReminders`（正文写 `submit`／非字符串消息／别的提醒文案都不算）与 `TestCheckerTranscriptWithRepeatedRemindersIsNotResumed`（预铺 4 条提醒：一次核对请求都不发、`stats.checker == 0`、产物在、`.checker` 是"没结论"那类文案；把守卫摘掉这条会红，已复验）；`rescue_triggers_test.go` 夹具加 `seedCheckerReminders`；`chapter_subdivision_integration_test.go` 在子节点转换的那一刻探 `r.lockChapter(childBase)`，探到"没被占住"就断言失败（摘掉 A 的锁这条会红，已复验）。
- **验证**：`go test ./internal/... -count=1` 14 个包全绿（latex 29.1s）、`go build ./...`、`go vet ./internal/...` 干净。
- **未改（刻意）**：① 正在跑的那条无法从页面叫停（预览服务只读，也没有"停止会话"通道）——保证的是"不会再有第二条并排跑起来"；② 已经落盘的坏转录一字不动（用户数据）；③ 节点驱动的续跑路径与切割副本落点都不动（见缺陷 C）。

## v1.5+（T231：核对结论写回它自己的转换转录；父节点不再停在「等待子会话」，2026-10-24）

- **触发**：用户贴了四行章节树：「转换 · chapter_022_child_12 会话错误 1.98M / 9.2k」「转换 · chapter_023_child_04 会话错误 2.95M / 12k 就卡在这一个会话上了」「转换 · chapter_024_child_05_child_05 会话错误 761k / 5.5k」「转换 · chapter_024_child_09 等待子会话 295k / 3.8k」，并说「很多父会话都卡在同一个问题上，有一个子会话核对通过会话提交，但是显示会话错误导致父会话一直没办法合并」「子会话全部显示已经通过，但是我看这个会话里面并没有子会话完成的消息」。
- **现场（只读 `/api/index` 与转录尾部）**：`chapter_022_child_12` 转换 a1 是 `error`，核对 a2/a3 在 16:42/16:46 通过、`checker_target` 记的转换尝试号对得上 a1；`chapter_024_child_05_child_05` 转换 a4 是 `error`（`.try4`），核对 a2/a3 在 16:37/16:41 通过、锚点也对得上。这两处是标签缺陷，产物是过的。`chapter_023_child_04` 转换 a3 `error`，核对 a3 从 16:23 起停在 `checking_result`、没有结论——不是显示错。`chapter_024_child_09` 转换 a3 `waiting_children`（children:3，16:20:42）：child_01 17:26 `issues` → 17:53 重转通过并核对通过，child_03 17:15 通过，child_02 转换 17:04 `checking_result`、核对 17:41 `error`——那三行里没有 child_02 的失败，父节点一直在等它。
- **缺陷 A（结论落错转录）**：`recheckChapter` 按章节名前缀找核对转录，先命中 `work/sessions/` 下的顶层文件，也会误命中同前缀的子章节文件（`chapter_024_child_05_child_05` 与 `…_child_05`）；拿到结论后只 `appendTranscriptActivity(trPath, …)`，改的是核对转录，转换转录最后一条活动仍是 `error`。
- **改 A**：`retry.go` 新增 `chapterSessionsDir`／`isChapterTranscriptName`（精确匹配 `convert_<章>[.tryN]`、`checker_<章>[.tryN]`，小写比较）／`isChapterTranscriptDir`／`chapterConvertTranscript`／`latestConvertAttempt`；`recheckChapter` 用会话目录定位转录，尝试号取该目录下最大的一版，结论走 `markChapterVerdict(proj, base, attempt, ok, issues)`——按 `checker_target` 的锚点写回对应的转换转录（`pass`／`issues`／`no_verdict`）。
- **缺陷 B（没结论被记成会话错误）**：核对的几种收尾（入口守卫、会话退化、复盘判救不回、始终不 submit、只读视图没准备好）返回的都是"没给结论"，但转录上写 `error`，转换行于是显示「会话错误」。
- **改 B**：`checkChapterAt` 拆成入口包装 + `runChapterCheck`（原函数体）；包装里 `!ok && checkerNoVerdict(issues) && !strings.HasPrefix(issues, "checker API failure")` 时写 `markCheckerNoVerdict(trPath)`（`chapter_activity: no_verdict`）。写在入口而不是六处返回点，漏一处就会复现。接口失败／余额不足仍写 `error`。`book.go` 里 no-verdict 结算那支把转换侧的 `error` 也换成 `no_verdict`（T222 的"保留产物、只重开核对"口径不变）；页面 `CHAPTER_ACTIVITY` 新增 `no_verdict: '核对无结论'`。
- **缺陷 C（失败状态没人写）**：子节点转换失败只进了 `convertChapterChildren` 的返回值；父转录最后一条活动还是之前的 `waiting_children`，失败的子节点自己也不写状态。
- **改 C**：父节点转换失败时写 `error`（已排队重试则写 `queued`），子节点失败时同样往自己转录写一条终态。
- **缺陷 D（子章节核对通过后没人叫醒父节点）**：父节点的整合只在它自己那一轮驱动里发生；子章节的重试把核对跑过了，父节点不知道，于是一直「等待子会话」。
- **改 D**：`runRetryTask` 的核对分支通过后调 `requeueParentAfterChildPass(proj, req.Project, base)`，内部 `childChapterParent` 取到负责整合的那一段（多层的取最上面一层），排一条父章节的「继续跑」；父章节已有「继续跑」排队时不重复排。
- **测试**：`chapter_verdict_test.go`（新增：结论只写回自己的转换转录、同名子章节的转录不被误改、顶层章节的结论留在顶层）；`checker_degrade_test.go` 改成钉"退化后转换与核对两边都留 `no_verdict`，没有任何一条留 `error`"（这条一开始红——退化的收尾在 `dec.Stop` 那支提前返回，入口包装才收到它，所以标记必须放在入口）；`chapter_subdivision_integration_test.go` 新增 `TestChildConvertFailureRecordsStateOnItsOwnNode`（子节点转换必失败：父节点与失败子节点的收尾状态都不能是 `waiting_children`）；`t211_child_retry_test.go` 新增 `TestChildCheckerPassRequeuesParentContinue`（顶层不排、子章节排到 `chapter_024`、重复调用只排一次）。
- **验证**：`go test ./internal/... -count=1` 14 个包全绿、`go build ./...`、`go vet ./internal/...` 干净；`cd web && npx vitest run` 262 条全绿，`npm run build` 重出 `viewer.js`（545.58 kB）后再 `go build` 通过。
- **未改（刻意）**：① 已经落盘的转录一字不动（用户数据），新记法只对之后的写入生效；② 用户机器上正在跑的进程用的是 T230/T231 之前的二进制，状态不会自己变回来，那些行重新核对一次即可；③ 多层父节点的重排仍排最上面那一层（与 T211 口径一致），不改成逐层叫醒。

## v1.5+（T232：排队不再沉默、核对没结论自动换新会话、复盘不再重复同一切口，2026-10-24）

- **触发**：用户先报「我对好多核对会话重新核对，都没有反应，不会新建一个新的也不会排队，就是没有反应，除非我截断的话倒是可以正确继续运行」，再追问「我们的复盘会话不是用来裁剪的吗处理一些遇到的会话问题，没有正确提交的让他们提交，或者遇到的错误的结束的、裁剪并继续运行的吗？这个好像也没办法正确工作了吗？」，最后把要求说定：「正常来说应该不需要手动操作，这边也有对应的复盘机制可以让会话变好和变干净，但是不会出现卡死的情况」。
- **现场（预览页 `/api/index` 只读快照，3932 条转录、generated 20:17:53）**：live 17 条 = 转换 9 + 修复 1（共用转换池 10 格，正好占满）+ 核对 6（池 20 格）+ 复盘 1，排队 175 条。四五个父章节（013/014/022/024/004）都是 `waiting_children`；chapter_013 那棵树里只剩 `chapter_013_child_02`（19:39:10）与 `chapter_013_child_03_child_03`（19:38:48）两条 `queued`，一直没拿到转换槽位。人工「继续跑」被章节锁当"忙"挡回队列，所以「点了没反应」；截断立刻生效是因为它走核对池。核对一侧：`chapter_004_child_01_child_03` 已跑到 try8，另一支复盘转录到 try12。
- **缺陷 A（等待沉默）**：取转换槽位的路径只在拿到槽位前写一次 `queued`；等 40 分钟期间零输出，日志里连"被挡回"都不一定有（旧二进制的 busy 分支是静默 `continue`），外面无法区分排队与卡死。
- **改 A**：`book.go` 新增 `holdConvertSlot`／`holdSlot`／`slotHeartbeatEvery`（5 分钟）。取槽位期间的心跳把「转换槽位已等 N」写进转录 `chapter_activity`（页面行时间戳随之更新）并打一行 info；三处转换 `acquireConvertSlot`（首轮、整合轮、反馈轮）全部改走它。`retry.go` 的 busy 分支改成 `logBusySkip`（同一请求 5 分钟最多一行，带已等时长与该章最后一次输出的年龄）。
- **缺陷 B（核对没结论只能靠人）**：节点驱动算核对转录时只认 `node.AttemptNum`；一条转录的「没提交」提醒攒满 4 条后 `checkChapterAt` 入口守卫当场拒跑，同一份文件永远回放，父章节每次「继续跑」原地失败。抬尝试号的只有人工「重新核对」与复盘截断两条路，自动这一路把"没结论"当成了章节判决。
- **改 B**：新增 `nextCheckerAttempt(nodeTr, start, maxBumps)`：从当前尝试起找第一条提醒没打满的转录，找到就换新尝试（新文件、无历史可回放、产物不动）。节点核对外面包一层自愈循环 `checkerNoVerdictSelfHeal = 2`：`checkerNoVerdict` 收尾时抬号重试，仍不行才如实报没结论。另补一条同会话提醒：`submit` 交了不可用的 `status` 时先在同会话追加 `checkerStatusReminder`（并计入提醒数），不再直接关会话白烧一次预算。
- **缺陷 C（复盘重复同一切口）**：`maybeRescue` 的 `truncate` 分支每次都从判定给的 `From` 切，没有记忆；同一章因此一轮轮从同一个位置重开（现场 try8／try12）。
- **改 C**：`rescue.go` 新增 `rescueCutAt`（key 是目标转录路径）与 `rescueCutRepeated`；`book.go` 的 `truncate` 分支先问一句，重复同一切口就写「复盘这次仍要截到第 N 条记录，与上次同一个切口：不再重复截断」、判 `fail` 收尾。
- **测试**：新增 `t232_guards_test.go` 三条——打满的转录换到下一条（try1→try2）、三条都打满时保持原号交回守卫、同一切口只拦第二次（换切口/换转录放行）。`rescue_retry_test.go` 与 `rescue_triggers_test.go` 里原来依赖"每次都能切"的四条用例在新守卫下先红后绿：守卫的 key 从章节名改成目标转录路径（跨用例/跨项目不再互相污染）。
- **验证**：`go test ./internal/latex/ -count=1` 全绿、`go build ./...`、`gofmt` 对新改文件干净（`stylefix.go` 是历史遗留，不动）。
- **未改（刻意）**：① 已落盘转录一字不动（用户数据），新记法只对之后的写入生效；② 槽位总数与派发顺序不动（"转换池 10 格被大章节占满"是并发成本，不是本次要改的口径，本次只让等待可见、让退化会话自愈）；③ 正在跑的进程仍用旧二进制，要看到这些行为得换新二进制重跑。

## v1.5+（T233：核对没结论默认换 6 条新会话、复盘自身失败不占目标预算，2026-10-25）

- **触发**：用户三句话——「核对一直没有反馈的时候不是应该重新核对吗？就是这边复盘超过10次都这样吧（默认给6次）」「复盘会话自己出问题不计算在次数自我重试」「重新核对能跑就行最后肯定可以给出核对结论吧，如果重转就造成浪费了。因为这次不是转换会话的问题」。
- **现场**：一章的核对到了 `try8`，同一支的复盘转录到了 `try12`；一条核对会话 `run_end` 只有 107 秒，第一轮 97 个字符的答复是把 `submit {` 写进正文，被提醒后仍然不调用工具——这类收尾以前一次调用内部就够 4 条会话、随后自动重核只给 2 次（`checkerNoVerdictSelfHeal=2`），用尽就交回入口守卫，页面上一直是「核对无结论」，只能人再点「重新核对」。
- **缺陷 A（重核次数写死且太短）**：`checkerNoVerdictRetryCycles`（旧 1）与 `checkerNoVerdictSelfHeal`（旧 2）都是常量，`book.go` 的自愈循环把"再开一次就收手"当成默认；用户现场需要的量级是 6 甚至更多。
- **改 A**：`config.go` 新增 `LatexConfig.CheckerNoVerdictRetryCount`（`checker_no_verdict_retries`）与 `CheckerNoVerdictRetries()`（0/未写 = 6）；`book.go` 的自愈循环改用 `r.cfg.Latex.CheckerNoVerdictRetries()`，每轮抬 `checkAttempt`（至少 2）并把 `checkerTranscript` 换成 `attemptTranscriptPath(nodeTr, checkAttempt)`，日志写明第几次、产物不动。用尽后保留产物、如实报没结论，并 `retryq.Queue` 一条 `role=checker`、`Source=no_verdict` 的重核请求（不重转）。T222 那段"同一条死转录再开一次"的第二循环整段删掉。旧常量留下 `checkerNoVerdictSelfHeal = 6` 只作测试与兜底。
- **缺陷 B（复盘自身失败吃掉目标预算）**：`maybeRescue` 在复盘没有给出结论时直接 `exhaustRescueBudget(key)`，等于"一次失败的复盘 = 这条目标永远别复盘"，下次提醒退回普通路径。
- **改 B**：`rescue.go` 新增 `rescueOwnBad` 计数、`refundRescueBudget(key)`（把 `rescueSeen[key]` 退一格）与 `rescueOwnFailure(key)`；`book.go` 的 `!ok` 分支改成先退款、再记自身失败，累计到 `rescueMaxOwnFailures = 6` 才 `exhaustRescueBudget` 收手，日志分两句写清楚"第几次、不占这条目标的次数"。
- **测试**：新增 `t233_budget_test.go`（`CheckerNoVerdictRetries` 默认 6 / 负数取 6 / 显式覆盖；复盘退款后还能再复盘、自身失败累计到上限）；改 `checker_degrade_test.go` 的「打满提醒不再回放」用例——断言换成"不再原地续跑但会自动换新尝试、次数有上限"；`rescue_retry_test.go` 的「三次没结论就放弃复盘」改成 `TestRescueOwnFailuresDoNotExhaustTargetBudget`（复盘自身失败不吃目标预算，第 4 条复盘照常起得来并给出结论）。
- **配置与文档**：`config.go` + `internal/config/default.yaml` + `config.example.yaml`（两份模板逐字节一致，`cmp` 核过）+ `docs/config.md` 新行；`config_version` 不变（标量键，无新块）。
- **验证**：`go build ./...`、`go vet ./internal/latex/ ./internal/config/`、`GOCACHE=/tmp/dsh-gocache go test ./internal/... -count=1` 十四个包全绿；`gofmt -l` 对新改文件干净（`stylefix.go` 是既有）。
- **未改（刻意）**：① 已经落盘的转录一字不动（用户数据），新行为只对之后的写入生效；② 正在跑的进程用的是旧二进制，要看到这些行为得换新二进制重跑；③ 槽位总数没动（重核走核对池，`checker_concurrency` 20 已够）。

## v1.5+（T234：打回后的重核真的重新问一遍核对模型，2026-10-25）

- **触发**：用户报「我发现有一个会话 明明在第一次已经处理完了并且通过了。为什么现在又被重跑了一次？：「work:chapter_024_child_01_child_04.tex 」」。
- **现场**：这一支的子章节在 18:20:17 与 18:27:47 各核对通过一次（两次都落 `run_end`，第二次在 `checker_…child_04.try2.jsonl`），21:35 父节点的工作区被重置，22:00 它被从头转换，随后重新拆出四条子章节（22:00–23:50）。往上追一层：父节点的核对转录 112 条消息、只有 18:51:21 一次 `submit`、之后一次 `run_end`，却在 18:51:22、18:56:34、19:02:42、19:10:39、19:18:24、19:25:23 记了六次「发现问题」，六次的问题原文逐字相同（同一条脚注缺正文）。
- **缺陷**：`book.go` 的反馈轮把同一个 `checkerTranscript` 再交给 `checkChapterAt`；`runChapterCheck` 从这条转录回放历史，末尾那次 `submit` 被 `replaySubmit` 原样重放、`resumeSkip` 置真，`sess.Run` 整段跳过——模型一次都没被再问，返回的还是上一轮的结论。于是「重核」= 复读：每一轮的修复会话都真的把被点名的行补上（分片 16192→16301→16304→16432→16293→16314 字节，脚注在第 121 行，报告写「Open issues carried upward: 0」），核对一次都没看过它。6 轮耗尽 ⇒ `archiveChapterAttempt` 作废整次尝试 ⇒ 重转该节点与全部子孙，把早已通过的子章节一起扔掉。缺陷在 `Depth 0` 的普通章节同样成立（顶层第一轮的核对转录在 `work/sessions/`，节点推导出来的路径在 `work/temp/conv_<章>/sessions/`，两条路都走同一个 `replaySubmit`）。
- **改**：反馈轮新增局部闭包 `nextCheckerTranscript()`——基准取第一条真在用的核对转录（递归节点是节点目录那条，顶层是 `work/sessions/checker_<章>.jsonl`），号取 `lastAttemptNumber` + 1 且必须大于当前号、至少 2，写回 `checkerAttemptNo` 并用 `attemptTranscriptPath` 开新文件；`checkChapterAt` 的入参换成它。首次核对与 T233 的自动重核路径一字未动。
- **测试**：新增 `t234_recheck_test.go`（`checkerStatuses: {"issues","pass"}`）：这一章必须收下、核对请求 2 次、转换请求 2 次、`work/chapters/chapter_01.tex.checker` 不存在、`work/sessions/checker_chapter_01.try2.jsonl` 存在。把 `checkChapterAt` 的入参改回旧路径，这条 0.96 秒就红，报的正是「共 6 次尝试仍未通过 checker: the marked formula is missing from the submitted chapter」——与现场同一句话。`rescue_triggers_test.go` 的假服务器为此加了按请求取 `status` 的能力。P60 的 `TestFixBudgetExhaustedSkipsChapterReattempt` 原先期望「父章节核对 1 次」，那是重放假象；现在按「首次 + 反馈轮」算（`checker_rounds: 1` 被下限抬到 3，故 4 次），整章不重开的三条断言不变。
- **验证**：`go build ./...`、`go vet ./internal/latex/ ./internal/config/`、`GOCACHE=/tmp/dsh-gocache go test ./internal/... -count=1` 十四个包全绿；`gofmt -l` 对新改文件干净（`stylefix.go` 是既有）。
- **未改（刻意）**：① 已作废的尝试照旧留在 `work/attempts/` 下不删；② 被重转那一支已经烧掉的转换不回来；③ 落盘转录一字不动（用户数据），新行为只对之后的写入生效；④ 整章重开仍会重转子树（"已通过的子章节跨尝试复用"是另一个题目，不在本次范围）。

## v1.5+（T235：切历史尝试时子会话跟着换、归档判据修好、子节点失败终态不再写到老转录，2026-10-25）

- **触发**：用户三问——「转换 · chapter_025_child_11 这个在切换两次历史重跑会话的时候下面的子会话没有跟着变吗？」「我的会话本来是1230个我以为已经固定了，好像有的还在增加是否都是这些，奇怪的重跑的，然后增加子会话的会话？」「还有的是会话错误增加的，比如转换 · chapter_011_child_01 但是我也不知道之前他为什么会话错误，后来判增加了重跑了…是不是上一个版本留下来的会话错误，这次都重试了」。
- **现场（只读快照，4084 条转录）**：`chapter_025_child_11` 有两次 live 转换（第 1 次 300 条消息、14:19:50 判「发现问题」；第 2 次 22 条、21:35:56 在等子会话），第 1 次尝试的整棵子树已挪进 `work/attempts/chapter_025_child_11/try1/`（八个子章节的转换尝试 1–3 全 `pass`）。页面钉住第 1 次时，子行仍取 `work/temp/…` 下那一代（子 01 的 live 转换尝试 3 `pass`、子 02 的尝试 1 在跑）——用户看到的"切了没反应"就是这一半；钉住第 2 次与不钉的结果本来就一样。
- **缺陷 A（世代没传下去）**：钉住哪一次尝试只影响父行自己，子行仍按"最新一次"挑。作废尝试只把**工作树**搬进 `work/attempts/`，它自己的转录还留在 `work/sessions/` 或节点目录下，所以子节点判"这是不是归档世代"时拿到的布尔量是 false——同一父行下两条 live 转录分不出 `try1` 与 `try2`。
- **改 A**：`web/src/legacy/sidebar.ts` 新增 `sessionGeneration`（从转录路径里读 `<项目根>|<章>|tryN`，没有 `/work/attempts/<章>/tryN/` 段时为 `''`）、`childGeneration`（父行这一次该用哪一代：自己归档就取自己的键，否则按"是不是比 live 最大尝试号小、且磁盘上真有那一代的归档"推断）与 `convertInGeneration`（同归档面里先精确匹配世代键、退而取最新）；`buildChapterTree` 先扫一遍全量转录收齐归档世代集合，`resolve(node, parentGen)` 把世代一路传进子节点，修复行与复盘行按同一世代过滤。旧行为（只看 `archived` 布尔）在归档面里仍然分不出代，所以键取路径。
- **缺陷 B（多项目布局下 `archived` 恒为 false）**：`sessionview.go` 的判据是 `strings.HasPrefix(rel, "work/attempts/")`，而 `Scan` 的根是 `latex_project/`、`rel` 实际是 `<项目名>/work/attempts/…`——前缀永不命中。现场快照里 767 条转录在归档目录，`archived` 一条都没标出来（接口里连这个键都不出现），页面的归档标记、`newerAttempt` 的活体优先、修复/复盘按世代挂靠整套失效。
- **改 B**：抽出 `archivedRel(rel)`，前缀与路径中段都认（旧单项目布局照旧）。
- **缺陷 C（子节点失败终态写到第 1 次尝试）**：`convertChapterChildren` 建的 `childNode` 里 `TranscriptPath` 永远是 `convert_<章>.jsonl`（第 1 次尝试），而 `convertChapterNode` 按**值**收这份配置——尝试号推导与 `attemptTranscriptPath` 改写都发生在副本里，回调方手里还是最老那条。`chapter_011_child_01` 的现场就是这样：第 1 次尝试的转录 10-20 21:00:18 已经 `pass`，仍在 10-24 19:46:23 被追加了一条 `chapter_activity: error`（同一个时刻 `.try4.jsonl` 自己也有一条），页面按最新挑行，把这个子章节显示成「会话错误」将近两小时——历史里当然看不到错误，那一代早就通过了。
- **改 C**：`convertChapterNode` 改收 `*chapterNodeConfig`，调用方（三个入口：批量派发、递归子章节、作废后新尝试）读到的就是这一次实际用的转录；失败终态的 `appendTranscriptActivity` 就地写在它上面。
- **测试**：`web/tests/chapter-tree.spec.ts` 新增四条（钉住作废的第 1 次 → 子行回到 `work/attempts/<根章>/try1/…`、核对行同代；钉住第 2 次 → 子行与核对行都取 live；拷不到时退回归档）；把 `resolve` 改回只看 `archived` 的旧写法，恰有一条红（`expected '…/work/sessions/convert_…' to be '…/work/attempts/chapter_050/try1/…'`）。Go 新增 `t235_child_transcript_test.go`（预铺 `.try4.jsonl`，函数返回后调用方路径必须是 `.try4`；在函数头上把指针换回副本即红）与 `TestScanMarksArchivedCopiesInMultiProjectLayout`（`<项目>/work/attempts/…` 必须标 `archived`；把判据改回前缀即红）。
- **数字（回答"会话数为什么还在涨"）**：3.8 小时三次快照对比，新增 175 条、消失 23 条（作废尝试整棵搬进 `work/attempts/` 后在旧路径上消失，不是删除）、净增 152；新增按角色是核对 74、复盘 57、转换 36、修复 8，其中 29 条一出现就在 `work/attempts/` 下；只有 6 个新的章节 ID。涨的是"作废重跑 + 核对自动换新尝试"这两条路径各开的新转录；`1230` 既不是配置也不是计数器里的任何常量（现场 809 个章节 ID、1979 个「章+角色」、3203 个「章+角色+尝试」），所以它不会"固定"。
- **验证**：`GOCACHE=/tmp/dsh-gocache go test ./internal/... -count=1` 十四个包全绿、`go vet ./...`、`go build ./...`、`gofmt -l` 对新改文件干净（`stylefix.go` 等既有文件不动）；`npm test` 22 个文件 266 条全绿；`npm run build` 产物已重建并提交。
- **未改（刻意）**：① 已落盘的转录一字不动（用户数据），包括那条写错地方的「会话错误」——它只是显示口径问题，那一章现在并没有失败，重新核对一次就会写上新的结论；② 那两次重跑也不是那条错误状态引起的（作废路径由核对轮次用尽触发，T234 那条已修）；③ 跨尝试复用已通过的子章节仍是另一个题目，不在本次范围。

## v1.5+（T236：父章节换尝试不再把已通过的子章节一起重跑，2026-10-25）

- **触发**：用户连报四条——「还有我发现一件严重的事，为什么chapter_004又开了第三遍？之前不是就是父会话在等待子会话吗？并且他到#3之后为什么我切换到#2的时候他的子会话不会随着变化？之前子会话都基本上全完成了，这下好像又全重新跑了一遍」「chapter_012也是，怎么又重新开了一个」「13也是，我怎么切换回去，之前子会话都通过了。我想切换回去这次是核对会话出问题了我删一下」「会话17怎么也新增加了一个，好像子会话还没转换完？」。
- **现场（只读核对运行目录）**：① 004 的第 2 次尝试在 00:08 被复盘判截断，排队请求里没有 `cut_attempt`（裁剪当场失败），00:09 的派发按「接着干」开第 3 次尝试并回退工作区——`repo.git` 的 HEAD 落在第 233 行的存档点（23:05），`git clean -fdx` 把 `work/_subdivision/` 整棵删掉，里面是 00:08:50–52 刚通过核对的四个子章节，于是子章节从 00:21 起全部重跑。② 012 同一机制，只是它的截断当场成功、请求带了 `cut_attempt=2`。③ 013 的第 1 次尝试已归档在 `work/attempts/chapter_013/try1/`，六个子章节的转换与核对**全部 pass**（其中两条是它们自己的第 2 次尝试通过的）；第 2 次尝试从 00:09 起、00:40 又派发一次，从此子章节一直在跑，最后一条子会话活动是 08:58。④ 017 的 `convert_chapter_017.try3.jsonl` 不是新的——它 10-22 16:04 就由一次手动重试建好，08:18 只是被追加了一行 `error`：日志里那句是 `继续跑失败: 子章节转换或 checker 失败…child_02: 父章节整合会话失败: api error: [SESSION_CONNECTION_TIMEOUT]; child_03: …`，也就是接口超时，不是换尝试。
- **缺陷 A（回退删子树）**：`rollbackChapterWork` 用 `git reset --hard <存档点>` 加 `git clean -fdx` 清整个 `work/temp/conv_<章>/work`。递归子章节的工作树就在这棵树的 `_subdivision/` 下，而它们是**未跟踪**的，`clean -fdx` 一并删；已经通过核对的子章节产物只有这一份（作废归档只搬当次尝试的顶层），删了就只剩重跑。
- **改 A**：回退前 `stashSubdivision` 把 `_subdivision/` 改名为同级的 `.subdivision-keep`，`reset`+`clean` 之后 `restoreSubdivision` 放回；`reset` 失败时同样放回，放不回则记一行「子章节目录没能放回工作树（暂存在 …），它们会被重新派发」——不再静默丢。
- **缺陷 B（重转必重跑子树）**：细分父章节被重新转换时，`convertChapterChildren` 无条件按 `chapterSubrange` 重写 `input/chapter.md` 并新建每个子节点的转换，即使子章节的输入一个字没变、上一轮的产物就躺在那儿。
- **改 B**：写 `input/chapter.md` 之前先比三件事——将要写入的正文与上一次写入的完全一致、`result/<子章>.tex` 还在、该子章节转录里最后一条 `chapter_activity` 是 `pass`。三条都成立就 `copyChildArtifacts` 把产物拷进父工作区、`inheritChildIssues` 把它的未决问题清单并进来（复用的子章节不能把问题丢掉），然后跳过这一支并记一行「上次已转换并通过核对、输入未变——直接复用产物，不再重跑」。新文件 `internal/latex/childreuse.go` 放 `childPassedEarlier` 与 `lastActivityState`（按行读 JSONL 取最后一条活动，用量行与消息行不算）。
- **测试**：新增 `childreuse_test.go` 两条——（产物 + 最后一次 pass 才算复用；最后一次是 `issues` 不算；之后又 pass 才算；更新的 `.try2` 以 `error` 收尾则不算；用量行、消息行、`run_end` 不影响结论）。新增 `rounds_test.go` 的 `TestRoundLogRollbackKeepsSubdivision`——回退后父章节 `.tex` 回到存档点那一版、后来产生的 `junk.tex` 被清掉、`_subdivision/child_01/node/result/<子章>.tex` 仍在、暂存目录已消失；把 `stashSubdivision`/`restoreSubdivision` 摘掉这条立刻红。
- **验证**：`GOCACHE=/tmp/dsh-gocache go test ./internal/... -count=1` 十四个包全绿（latex 38.5s）、`go vet ./...`、`go build ./...` 干净、`gofmt -l` 对新改文件干净（7 个既有文件不动）。
- **未改（刻意）**：① 已作废的尝试照旧留在 `work/attempts/` 下不删，人工"回到第 N 次尝试"（把归档搬回工作树）本次没做，眼下要回退只能等这一轮停下后手工搬；② 已经白跑掉的那些子章节不回来；③ 落盘转录一字不动（用户数据）；④ 017 那种 `SESSION_CONNECTION_TIMEOUT` 是接口侧失败，不在本次范围。

## v1.5+（P63：切回第 N 次尝试，2026-10-25）

- **触发**：用户先问"之前子会话都通过了，我想切换回去"—，随后一句口径："做一下切回"，中途追加"还有注意切换的时候暂停当前会话"。T236 那一批的"未改（刻意）"里写的正是"人工回到第 N 次尝试本次没做，要回退只能手工搬"，这一批就是它。
- **口径**：切回 = 把 `work/attempts/<章>/try<N>/` 整棵搬回工作区、把现役这一次整棵留档，两个方向对称；搬的清单与 `archiveChapterAttempt` 同一张（节点工作区的 `work`/`result`/`integrated-stage`、提交的 `.tex`、分片目录、报告）。转录也要跟着搬，否则 `lastAttemptNumber` 仍按 `work/sessions` 里最大的 `.tryN` 认"现在这一次"，切回来以后各条路径继续往那个更大的号上写。
- **"暂停"的真实落地方式**：走查了三条可能的叫停入口——`internal/latex/runner.go` 的 `aborted` 只是接口错误的重试计数，`internal/session/session.go` 的 `interrupted` 是"从磁盘续跑"的标志，`continueFlag`/`setContinueFlag` 只是手动继续的消息标记；没有取消通道、没有项目级锁、没有 PID 文件，`lockChapter` 也只是内存里那一格。硬做一次真正的中途取消要动会话循环与工具执行，代价和风险都远大于收益。于是"暂停"= 请求排队 + 只在拿到章节锁（`TryLock`）时才动手：正在跑的会话把手上这一轮走完 → 松锁 → 整棵现役尝试被搬进留档（完整、可续）→ 再切回来 + 「继续跑」就能接着干。日志 `[restore] … 等它跑完这一轮`（同一章 5 分钟内只留一行）与页面回执都照实这么写，不假装已经停下。
- **写保护（唯一能把数据搞坏的路）**：`restoreBusyReason` 看两处——`work/sessions` 下这一章（含 `_child_*`）的转换转录与 `.partial`、以及节点工作区里除 `media/views/pdfview/build/out/images/children.tmp` 之外的条目，最近 2 分钟（`restoreLiveGrace`）有 mtime 就拒。013 这种父章节停在 `waiting_children`、孩子在 `_subdivision/` 里写的现场，如果没有这道守卫，半个工作区会被搬走。
- **不覆盖**：目标已占就拒（`movePath` 报「目标已存在，不搬」），冲突的那一份加 `parked-` 前缀，同名转录不搬、原地留着；`try<attempt>` 目录只在搬空后删。
- **接口**：`retryq.Request` 新增 `Attempt`，`/api/retry` 接受 `role=restore` + `attempt`；四个拒绝分支各有一句话——没写第几次（400）、号不大于 0（400）、留档不存在（404）、子章节（400，并指出该对哪个父章节切）。`role=restore` 不要求章节的 `.md` 还在（它搬的是留档，不是重跑），也不会自动开核对会话。
- **前端**：「历史」页签每一行多一个「切回这次」，判定在 `web/src/legacy/actions.ts` 的 `restoreTargetOf`（顶层章节的转换会话、未归档、尝试号 ≠ 当前这一次——切回去之后被搬走那一次的号比"现在"大，所以同样有这个按钮，能切回来）；确认条写明"现在这一次整棵留档、之后还能切回来""正在跑的会话不会被打断，会等它跑完手上这一轮"。
- **缺陷（自己踩的，留在档案里）**：第一版把 `transcriptAttempt` 的返回值当成了不带前缀的章节名，而它返回的 stem **含** `convert_` 前缀——于是"搬转录"和"写入中守卫"两条路径都在拿 `chapter_004` 比 `convert_chapter_004`，一个转录都没搬、守卫也永远返回空；四条 Go 用例里三条当场红（"转录应该在 …/try3/…"、"应允许切回第 3 次，却被拒"）。改成与 `"convert_"+base` 比较后全绿。
- **测试**：`go/internal/latex/restore_test.go` 四条（双向对调、往返幂等、写入中拒切、四类非法目标）；`go/internal/sessionview/restore_retry_test.go` 两条（排队成功且回执带 `attempt`、"不会被打断"、队列里那一项角色正确；四类拒绝且队列保持空）。`web/tests/session-actions.spec.ts` 新增五条钉住按钮给谁不给谁。
- **验证**：`GOCACHE=/tmp/dsh-gocache go test ./internal/... -count=1` 十四个包全绿、`go vet ./...`、`go build ./...` 干净；`npx vitest run` 22 个文件 271 条全绿；`npm run build` 后 `go build` 再通过（dist 已重建并提交）。
- **未改（刻意）**：① 落盘的转录与产物一个字节都不改写，切回只做搬运；② 核对转录不搬（结论记在它自己那条转录的 meta 里，那是历史事实）；③ 真正的中途取消没做，卡死的会话今天仍然只能等它自己收尾——这是能力边界，不是遗漏。

## v1.5+（T237/T238/T239：接口错误原地重试、截断不再等于重跑、回执不再常驻，2026-10-25）

- **触发**：用户先问「我记得我就添加了13是切回队列，其他的并没有添加到切回。但是其他的所有的都没办法操作了，都显示已经加入切回队列是啥情况？因为我看有好多是因为 会话错误 CONNECTION 这样的」，随后给了口径：「是api问题，不是会话退化相关的问题，不能够区让他们重试，因为如果重新跑的话就会有越来越多的api问题了，这是上游api错误，我们这边应该是重试吧。不然一直就重新跑转换，得不偿失，最后每一个会话都 api错误了」；再补一个具体案例「转换 · chapter_024_child_01」。
- **现场取证一（回执）**：磁盘上 `work/retry/` 里 `role: "restore"` 的请求**一共只有两条**（都在 11:23–11:37 写给同一章、且至今仍是待办），但每一条转换/核对会话下面都写着「已加入切回队列」——回执写在全局 `state.retryNote` 上，没有过期时间、切会话也不清。
- **现场取证二（接口错误变成新尝试）**：`work/rescue/log.jsonl` 1474 条里 `truncate` 1011 / `retry` 430 / `fail` 34；`retry` 里 99 条的理由写着连接问题（这条路本来就在原地重试，是对的），但 `truncate` 里仍有 5 条据此判了截断，切口落在这条健康对话的第 22/86/90/134/196 条记录上；运行日志另有 47 行把 `api error: [SESSION_…]` 记成失败/终止原因（其中 11 条是转换会话「会话报错终止（非致命），交给复盘链路」）。代价：一条子章节的复盘转录已经到 try24，另一章开到第 10 次尝试。运行日志里 `chapter_011` / `chapter_024` 被判「章节失败」的最后一句同样只是 `api error: [SESSION_CONNECTION_TIMEOUT]`——转换、核对、整合都干完了，接口一抖就整章作废。
- **诊断**：`rescueAfterError` 只看"是不是致命错误"，不看错误来源，于是上游抖动与模型跑歪共用同一条「复盘 → 截断 → 开新尝试」的路；而截断的闸门只记「章|角色」，换一次尝试就清零，于是下一刀又落在同一处（一章的切口分别停在第 7、13、19 条记录）。
- **改法一（T237）**：`isSessionUpstreamError` 认 `SESSION_CONNECTION_TIMEOUT`/`SESSION_API_ERROR`/`SESSION_RATE_LIMIT_EXCEEDED`/`SESSION_TIMEOUT`/`connection reset`（带「不可重试」标记的不算），命中就原地重试：排一条 `role=continue`、`source=upstream` 的请求并带上 `not_before`（30 秒起每次翻倍、上限 10 分钟），派发端到点前**不取、不占锁、不耗并发槽**；次数由 `latex.api_error_retries` 决定（默认 6，约 25 分钟），按 `项目|章|角色|转录` 记在内存；用完才如实记失败，不触发复盘、不截断、不占尝试预算。致命错误（余额不足/资源包不可用）仍先短路。
- **改法二（T238）**：`rescueCutRefusal` 三条闸门——保留不足 12 条记录（总数 > 12）、保留比例不足 1/5 且总数 ≥ 60、同一 `项目|章|角色` 在 ±8 条内重复切过同一处；命中就当场判失败并写清理由，不排队、不开新尝试。去重键加上项目根，避免不同项目（以及并行测试）互相污染。
- **改法三（T239）**：`state.ts` 加 `setRetryNote`/`clearRetryNote`（终态 20 秒、进行中 60 秒自动过期），切会话、重置视图、「历史」里换尝试都清；`actions.ts` 九处、`fork.ts` 六处、章节树三处统一走这个入口——那些地方原来是**裸表达式**，顺手接住任何返回值都编不过，所以必须包成语句；`HistoryPanel` 的丢弃/切回问答也按同一个信号清。
- **测试**：新增 `t237_upstream_test.go` 四条（错误分类、退避序列、排队请求带 `continue`+`not_before`、派发端不提前取）；`t232_guards_test.go` 的 `TestRescueCutRefusal` 覆盖三条闸门；嵌套子章节集成用例的假复盘切口从第 1 条改成第 13 条（保住子章节自己那几轮、只丢掉没提交的整合轮），否则新闸门会拒掉它——这是用例不现实，不是代码错。web 新增 `web/tests/retrynote.spec.ts` 四条。
- **验证**：`GOCACHE=/tmp/dsh-gocache go test ./internal/... -count=1` 十四个包全绿、`go vet ./...`、`go build ./...` 干净；`npx vitest run` 23 个文件 275 条全绿；`npm run build` 后 `go build` 再通过（dist 已重建并提交）。
- **未改（刻意）**：① 已经落盘的 47 条历史截断不自动搬回——现场还有章节在写，搬整棵子树必须等它停下来，且旧二进制会接着开新尝试；② 重试计数在内存里，重启进程重新计数；③ 一个会话报错后的"复盘预算"与"接口重试"是两套账，互不占用。
- **补丁（T240，同批收尾）**：用户问「chapter13 我咋切换不回去」，查现场发现排队提示自己在误报——`chapterLastOutputAge` 只扫 `<proj>/work/sessions/` 与 `work/chapters/<章>.tex`，而递归子章节的转录在父章节的临时工作区 `<proj>/work/temp/conv_<父章>/work/_subdivision/…/node/sessions/` 下（P34）。现场父章节那条转录停在 12:56 的 `waiting_children` 再没动过，子树里同一章的子章节却在 15:07 还写着第 8 条核对转录——提示于是每 5 分钟说一次「最后一次输出在 1 小时前——它可能卡住了，等不下去时只能重启进程」，让人去重启一个干得好好的进程。改法：把子树里属于这一章的转录与 `.partial` 也算进"最后一次输出"（按名字过滤、`WalkDir` 条目数封顶 2 万，别拖住 2 秒一次的 tick），加 `topChapterOf` 把子章节名收敛到最外层章节再拼临时工作区路径。测试 `t240_busyage_test.go` 三条。切回请求本身仍要等这一章真正停下来（P63 的硬规矩），这条没动。
- **补丁（T241，同批收尾）**：用户问「chapter14 他怎么重复了这么多次历史，这个每次创建一个新的，并且这次没有改变子会话的情况下，这个会话他不知道他过去做了什么。他不知道他有没有分派子会话，有没有分派修复会话。所以如果要创建新的转换肯定要连带子会话一起转换。但是这边显然就没有。并且这边过去的那个会话就不应该重新创建会话。只要不是核对会话多次打回创建的新会话外，这种多次复盘的我们这不影响，可以一直复盘，我看这个#1才是正确的会话，他里面应该正确的截断一下就能继续跑了」。取证：这一章的转换有 10 条转录（第 1 次 371KB、后面 69–157KB），`work/attempts/<章>/` 一条留档都没有——工作树从没换过代，只有转录被复制了 9 次；`work/rescue/log.jsonl` 里同一章连着 10 条 `verdict=truncate`，切口分别是第 7、13、19、12 条记录；细分工作树 `_subdivision/child_01…child_09` 完好、`children-summary.md` 与 `children-manifest.md` 都在，子会话从没重跑过，问题不在子会话而在转录被切坏了。根因：复盘截断沿用 `cutTranscriptAt`「前缀抄成下一条尝试」，把"退回去重说"记成了"换一次尝试"；而且第一刀切在第 7 条——一条调了 4 个工具、一个回执都还没有的 assistant 消息后面，续跑只能补占位回执，模型于是每次都说「这个会话里工作区工具不可用」，再退化、再被切、再长一条（同一句话也解释了"记不住自己派过子会话"）。改法：新增 `trimTranscriptInPlace`——原地截断，同一条转录、同一个尝试号，页面「历史」不多出一条；丢掉的那一段抄进 `work/rescue/cuts/<章>/`（只作证据、不进历史、不可切回），切口上补一条 `t=meta kind=cut`；新增 `balancedCutFrom` 把切口往前挪到"工具调用都已回执"的那一条（老转录没有 `tool_call_id` 时按 role 一对一销），整段前缀都不平衡就如实拒绝、一个字不动；`applyWorkspaceCutTo`/`archiveChapterProductsTo` 让复盘这一刀的产物挪进 `work/rescue/cuts/<章>/` 而不是 `work/attempts/<章>/tryN`（尝试号没变，那是"作废留档"的位置）；核对会话的截断仍然换尝试（核对一句一尝试、结论按尝试号回写，语义不同）。`rescueQueueTruncate` 的返回值从 `int` 改成 `rescueCutResult{Kept,Dropped,Attempt}`，`book.go` 的进度行据此说清"原地截断、仍是第几次尝试、丢掉几条"。测试：新增 `t241_inplace_cut_test.go` 四条（切口平衡表、原地截断后是 6 条 + 1 条 cut 且不写新尝试、证据留档、切口落在等回执那条上要前移、整段不平衡时拒绝且不动文件），`rescue_probe_test.go` 两条按新语义改写（`TestRescueTruncateTrimsInPlace` 期待"还是第 1 次尝试"）。验证：`go build ./...`、`go vet ./internal/latex/`、`go test ./internal/... -count=1` 全绿（`sessionview` 有一条既有并发 flake，单跑与重跑均绿）。未改：已经长出来的历史一条不删（用户数据），要折回一条需人工确认后搬进证据目录；复盘次数仍不限，T238 三条闸门继续拦"只留几行""反复切同一处"的刀。

## v1.5+（T242：核对交了空清单不再作废整章，2026-10-26）

- **现场**：用户按上一批的建议重建二进制、重启、把 `chapter_013` 切回已经跑完的那一次尝试（第 1 次，子会话都通过、470 个文件已提交），半小时后回来看页面：`#1 转换 · chapter_013` 又被换成了新的转换，且那条新转换十几分钟没有任何输出。原话：「我记得你不是帮我切换到之前归档的那个#1的转换了吗，就是子会话都已经完成的那个。但是这边我点重置视图他还是跳到了新的转换？并且之前那个新的转换好像还在跑吗？」
- **取证**：切回本身是成功的——13:06 现役文件被整棵搬进留档、第 1 次的整棵树搬回原处、转录跟着搬回，随后第 1 次的转换会话原地续跑、`submit` 成功（回执是「chapters/chapter_013.tex 与 chapters/chapter_013/（470 个文件）已在书里」）、`run_end ok`，核对会话接手。13:30 那棵整树又被搬进 `work/attempts/chapter_013/try1/`（`integrated-stage/` 469 个文件、报告、工作区都在，留档完整可切回），同时开出第 2 次尝试的转换转录——5 条记录，只有 meta/queued/running/price 与首条 user 文本，没有一条 assistant 输出。留档里那句核对结论只有 53 字节：`the checker reported problems but gave no description`。
- **根因**：核对 `submit` 交的是 `status: issues`、`issues` 是空串，`checker.go` 给这份留档补了一句占位文案就当结论返回；上层按"打回"处理——先喂反馈轮，反馈轮走完仍不过就作废产物、换一条全新转换会话从头转。一句零信息的话把一棵跑好的产物扔了，页面上看起来就是"切回去没生效、又跳到新的转换"。
- **改法**：`issues` 为空时先在同一条会话里要一次清单（与 T232 问 `status` 同一手法，新增 `checkerIssuesReminderMark`/`checkerIssuesReminder`，转录里留一条提醒，同样计入「这条转录还能不能原地续跑」的判断）；仍然为空就按"这次没结论"收尾——`AppendTranscriptNote("chapter_activity", no_verdict)`、关掉这条会话换一条核对重核，`sawVerdict` 不动、预算仍是 `checkerNoVerdictAttempts`，产物与转换会话一个字不动。全程都要不到结论时文案单列一句「每次都交了 issues 却一条问题都没有写出来」，仍以「核对始终没有给出结论」开头，`checkerNoVerdict` 与 `.checker` 标记照旧认成"没结论"（重跑时先重核这一份，不静默采纳也不作废）。占位文案整条删掉。
- **测试**：新增 `t242_checker_empty_issues_test.go` 两条（同会话补上清单：只有一条核对转录、只发一次提醒、不重转、不归档；补不到：如实报没结论、产物留在原处、`.checker` 属"没结论"那类、转换会话没收到假反馈），`rescue_triggers_test.go` 的假核对加 `checkerEmptyIssues` 选项按请求序号吐空清单；`chapter_checker_test.go` 里原本钉着旧行为的 `issues without text still blocks` 改成两条——`issues without text asks for the list`（第二次补上清单就按清单打回）与 `issues without text is not a verdict`（要过还是空 → 报没结论）。验证：`go build ./...`、`go test ./internal/... -count=1` 全绿。
- **未改**：已经因此被作废的尝试不自动搬回（要切回仍走「切回这次」，P63）；核对真写了问题清单时照旧打回、按 `latex.checker_rounds` 修。

## v1.5+（T243：接口错误重试用满就暂停，2026-10-26）

- **原话**：「程序按「接口错误原地重试」把它重新拉起来（第 1/6 次 这个有点少了这种接口的错误，如果重试次数太多就这次暂停，程序下次重启的时候继续跑或者再web端可以手动继续跑，而不是重试之类的因为重试也没有也是接口错误。这个接口错误默认重试30次吧）」。
- **现场**：一条 `SESSION_CONNECTION_TIMEOUT` 只换来「第 1/6 次原地重试」，队列里那条续跑请求到点也没人接——第二天再看这一章还停在「等待并发槽」，`work/temp/conv_<章>/` 早没了。
- **根因**：① 6 次 / 25 分钟是按"抖几下"估的，一场一两小时的上游中断根本撑不过去，用完还按原样记成失败、整轮中止；② `retryPrecondition` 要求临时工作区还在才派发续跑，而章节工作树一次转换收尾就删（`keep_temp_dirs` 默认关），接口错误那一次收尾正好把它删掉——于是每条续跑请求每次扫到都被判「这一章的工作区已经不在了」，这一章永远排不出去；③ 失败终态先看 `rescueQueuedFor`（上一次排队留下的内存标记）再看暂停，旧标记会把「已暂停」盖回「等待并发槽」。
- **改法**：`latex.api_error_retries` 默认 6 → 30（`DefaultAPIErrorRetries`，30 秒起翻倍、10 分钟封顶，约 4.5 小时；写 0 也取 30），两个配置模板、`docs/config.md`、AGENTS.md 同步。次数用满**不记失败**：`queueUpstreamRetry` 依次删计数、留一行警告、返回 false；`rescueAfterError` 把这条路径翻成 `&pausedError{Base, Retries, Cause}`（`pausedMarker`/`isPaused` 用 `errors.As`，包一层仍认得出）。批量转换 `convertChapterBatch` 单独收集暂停的章节、`convertPhase` 用 `pauseError` 收尾（`pausedChaptersError` 一段话说清哪几章暂停了、怎么续），不再落进「N 个章节转换失败」。转录状态写 `paused`，页面文案「已暂停」；派发端遇到暂停只留一行「上游接口仍不通，这一章保持暂停」，不写「继续跑失败」。`retryPrecondition` 去掉工作区判据（接着跑本来就会以 resume=true 重建工作树）；四处失败终态改成先判 `isPaused` 再判旧排队标记。空清单不再是暂停（`pauseError(nil)` 返回 nil）。
- **测试**：新增 `t243_pause_test.go` 四条——默认 30 且 0 取默认；暂停措辞含「暂停/重启/继续跑」且包一层仍认得；`rescueAfterError` 第一次排队、第二次返回暂停且队列不增、`SESSION_INSUFFICIENT_BALANCE` 仍致命；真跑一章撞两次 502 后状态是 `paused`、队列清空、没有 `work/attempts` 归档、没有 `.tex` 产物。`t237_upstream_test.go` 两处文案与注释按暂停语义改写。web `sidebar.ts` 加 `paused → 已暂停` 与一条单测（23 个文件 276 条全绿，`npm run build` 后 `viewer.js` 一起提交）。`go test ./internal/... -count=1` 全绿（latex 55.99s）。
- **未改**：暂停不自动续跑——要重启程序或在网页点「继续跑」（用户要的就是「这次暂停」）；重试计数仍在内存里，重启后重新给满，正好是"重启继续跑"的语义。

## v1.5+（T244：有切回在等时这一章不再开新活，2026-10-26）

- **原话**：「为啥我切不回去？有没有强制切换的。这边要不没用我发现，我重启也不行」
- **现场**：网页 15:24 发起的那条切回请求（`role=restore`、`attempt 1`）在 `work/retry/` 里躺着，同一分钟日志只有一行「切回请求排队中：先等这一章的会话跑完手上这一轮再搬」，此后一小时零进展。占锁的是这一章自己的细分驱动：它从转换开始就持着父章节的锁，八支子分支还在往下细分、跑核对（179 个 `.tex`、最新写入就落在几分钟前）。重启也没用——重启后这一章没有 `.tex`，又被当成"要转换"重新派发，锁当场被拿走，队里的切回继续等。
- **根因**：P63 的硬规矩是"不打断正在跑的会话"，切回只在派发端 `lockChapter` 抢到锁时才搬；而细分父章节的"正在跑"是整棵子树，这把锁要等最后一个孙子收尾才放。「等它跑完手上这一轮」这句承诺在这种节点上等于几小时，且排队那行日志按请求去重、一辈子只打一次，页面上也没有任何提示。
- **改法**：把"有切回在等"做成转换侧的停止条件。新增 `restore.go` 的 `rootChapterOf`（子章节名折回顶层）与 `(*Runner).pendingRestore`（只认 `restore` 角色、折回同名顶层章节、留档目录在、且目标不是现役那一次——只做这几条便宜判断，不调 `RestorePrecondition` 的写入检查，那要遍历整棵子树）；`restoreWaitingError` + `isRestoreWaiting` 作为第三种结局。接线四处：`convertChapterBatch` 派发前跳过并单独记「等切回」清单（既不进 `failures` 也不进 `pauses`，收尾不再报「全部转换完成」）；`convertChapterNode` 入口直接返回，不建会话、不写转录、不写终态；`convertChapterChildren` 发现切回在等就不再派发剩下的子分支、已跑的跑完收工；`dispatchRetries` 在这一章排着切回时把别的角色（续跑/转换/核对）留在队列里，免得它们抢走刚松开的锁。`logRestoreWait` 改成按 5 分钟重复，带第几次、已等多久、最后一次输出去区分"还在动"与"可能卡住了"。
- **顺带**：同一个按钮点两下会留下第二条同目标请求，它永远排不上、以前每 5 分钟重排一次永不消失；派发端现在发现目标已是现役就把它作废、留一行说明。
- **测试**：新增 `t244_restore_wait_test.go` 五条——只认真能切的请求（空队列/别的角色/无留档/现役本身就是目标都不算，子章节名折回父章节算）；节点入口不建会话、不产工件、措辞里没有"失败"；批次跳过只记「没跑」且请求仍留队；章节忙时派发端一条都不派、两条请求都还在；重复的切回请求被收掉且产物不动。`go test ./internal/...` 只有一条与本批无关的环境用例失败（沙箱里跑 venv，本机 python3 已升 3.14 而 libpython3.14.so 缺失，改动前后一样失败）。
- **未改**：切回仍然不打断正在跑的会话（T244 只是让"等"有个上限，不是强制打断）；已经躺着的那条请求在旧进程上仍要等这一轮收尾，要立刻见效就重启并在启动时用 `--only` 指定别的章节，让这一章整轮都不被碰。

## v1.5+（T245：截断给的行号落在用量行上也能切，2026-10-26）

- **用户报的现象**：日志里两章各撞两次同样的报错——`[rescue] chapter_017 当场原地截断失败，退回派发时再切：第 239 行不是一条消息记录（选了用量/元信息行？）`，紧跟着就是 `[convert] chapter_017 从记录继续失败：第 239 行…`；另一章是第 258 行。每次还白跑一轮父章节的「继续跑」。
- **取证**：转录是"每请求一条用量行"的 JSONL，实测一条真转录 261 条物理记录里只有 119 条 `t=msg`，另有 102 条 `t=meta`、40 条 `t=usage`。`from` 的含义一直是"保留到第几条**物理**行"，复盘会话读到"最后一条好消息是第 237 行"，落笔就填 238——那正是紧跟其后的用量行。四个截断点（`trimTranscriptInPlace`、`cutTranscriptAt`、`truncateCheckerTranscript`、`truncateVectorTranscript`）都硬拒不是 `msg` 的行号，于是同一个数在「当场切 → 派发时切 → 从记录继续」上各失败一次；队列里那条请求被消费之后续跑是静默丢掉的，页面上只剩"这一章一直不动"。（同一份日志里另有一刀是成功的，16:39:50 `已原地截断到第 269 条记录`——说明链路本身没坏，坏的是那个行号。）
- **根因**：提示词与工具说明都写着"最后一条好消息的行号"，没有说错，错在**回放只走 `t=msg`** 而切口是物理行号——模型按"一轮的边界"落笔时天然会指向那一轮的用量行。硬拒这种行号等于把一个常见写法变成死锁，而且死锁点被三条路径各撞一次。
- **改法**：新增 `snapCutLine`——切口挪到最近的一条消息行（先往前找；前面一条消息都没有才往后找到第一条）；只在 `from` 越界（<1 或超过行数）或整条转录一条消息行都没有时照旧报错。四处接线：`trimTranscriptInPlace`（原地截断）与 `truncateCheckerTranscript` 挪过之后打一行警告「切口从第 N 条挪到第 M 条：那一行是用量/元信息行，不进消息流」；`cutTranscriptAt`（派发端切新尝试）与 `truncateVectorTranscript` 静默落位；`cutChapterTranscriptForRetry` 在切之前先 `snapTranscriptFile` 落位，日志与回执报的是真正切到的行号。`RescueSubmitTool` 构造时拿到 `read_session` 的同一份转录（新增 `Lines` 字段），`truncate` 结论当场落位并把原始值留在 `RawFrom`，回执里多一句「line N is a usage/meta line, it is never replayed — the cut lands on the last message line M」，因此排进队列的 `From` 一开始就是能用的数（`CutAttempt` 语义不变，派发端不会切第二刀）。
- **口径**：只做"最近的一条消息行"这一步，不替复盘会话判断该留在哪一轮；提示词一个字没改——`latex_rescue.system.md` 本来指的就是"最后一条好消息"，工具变宽容不该往提示词里加解释。
- **测试**：新增 `t245_cut_snap_test.go` 四条——落位表（用量行往前挪、第 1 行是元信息且前面没有消息时往后挪、越界与 0 仍拒、整条没有消息行仍拒）；事故原形的原地截断（`from` 落在用量行上照样切、`kept=6/dropped=2`、退化段进 `work/rescue/cuts/<章>/`、切口留 `kind=cut`）；submit 自己落位且回执说明原因、没有转录时一个数都不改；派发端落位后新转录 3 行、坏尾巴不跟过来。`chapter_retry_test.go` 两条旧断言按新语义改写（切在用量行现在挪到消息行照切，不再期待拒绝）；`rescue_retry_test.go` / `rescue_triggers_test.go` 里依赖 `from=1` 硬拒的三处断言改成落位后的行号——顺带发现原地截断会把尾巴里的 `rescuing` 标记一起切走（它在最后一条好消息之后），该用例改成要求切口有 `kind=cut`、终态是 `queued` 或 `running`。`go test ./internal/...` 只有那条与本批无关的环境用例失败（沙箱里跑 venv，本机 python3 已升 3.14 而 libpython3.14.so 缺失）。
- **未改**：不判断"该留在哪一轮"；已经因为坏行号被消费掉的排队请求不会自动重放（要续跑就在网页上点一次「继续跑」或重启后重派发）；`balancedCutFrom` 的"工具调用都已回执"仍然在落位之后跑，两条约束互不影响。

## 批次 T246：「等待并发槽」不再永久停留

- **现象**：一章的页面状态永远停在「等待并发槽」，而进程早就没了——用户的原话是「他一直没等到并发槽…其他都终止了也没等到他」。取证实录：该章转录最后两条是 `manual_continue` 与 `chapter_activity=queued`（同一次点击、同一秒写完），此后两天没有任何新状态；章节的 `.tex` 与 `.tex.checker` 都在，重跑走的是「已有产物，跳过」。同一个项目里另有一章停在 `running`、一章停在 `rescuing`，都是同一类残留。
- **根因**：`chapter_activity` 是页面那一行状态的唯一来源，而收口只发生在"会话跑完"的路径上。进程被杀、阶段收尾、请求刚被队列消费就退出，这三种收尾都不经过那里，进行中状态于是永久留存；「已有产物，跳过」这条路持着章节锁、这一章本轮也不会再被驱动，却从不回头看那一行。
- **改法**：新增 `go/internal/latex/activity.go` 的 `settleStaleActivity(trPath, texPath)`——只对 `queued`/`running`/`rescuing` 这三种"只有活着的会话才成立"的状态动手；新状态照磁盘事实写：有 `.tex.checker` → `issues`（有产物但核对没过），只剩产物 → `checking_result`，没有产物 → 什么都不写（那种情况的真相是"还没转出来"，不归这条路管）。调用点只加一处：`book.go` 的「已有产物，跳过」分支，收口之后再按 `latex.keep_session_records` 处置转录。复用已有的 `lastActivityState`（`childreuse.go`），不新写解析。
- **为什么只收口一处**：其余入口（转换、核对、复盘）自己会写终态，多写只会造成两条状态打架；而"跳过"这一支恰好是唯一一处"确定不会再有人写"的地方。
- **测试**：新增 `t246_activity_settle_test.go` 四条——`queued` 且 `.checker` 在 → `issues`、收口后再调不再刷 note；`pass` 之后一个字段都不改；没有产物时什么都不写（`queued` 原样留着）；产物在但 `.checker` 已删、状态停在 `running` → `checking_result`（不凭空造 `pass`）。`go test ./internal/...` 只有那条与本批无关的环境用例失败（`TestPythonEnvVenvRunsInsideRealSandbox`：本机 python3 已升 3.14 而 `libpython3.14.so` 缺失，回退改动同样失败）。
- **未改（另开一条）**：细分父章节把"仍在重试中的子节点"记成失败、以及同一章后来通过后阶段汇总里那条旧失败不消失。这两条要动的是细分驱动的判定与失败清单的生命周期，放在同一个提交里风险太大。取证已经做完：父章节在某次继续跑时把两支还在整合/提醒中的子节点判成失败并写进 `failures`（`book.go` 汇聚处），子节点随后各自通过（核对转录 `pass`），父节点后来重新整合也通过，但阶段汇总里那条失败一直留到运行结束。

## 批次 T247：父章节不再把「还在重试的子节点」判成失败

- **现象**：一份运行日志的结尾是「3 个章节转换失败」，点名的三章里有两章的核对在同一分钟或更早已经是 `pass`；另一份日志里父章节在 18:02 报「子章节转换或 checker 失败；父章节禁止提交: child_03: 细分父章节整合会话未提交（已提醒两次）」，而这两支子节点当时刚被复盘原地截断、已经排进重试队列（转录状态写成 `queued`），几分钟后各自跑通、父章节随后重新整合也通过。用户看到的是「父会话通过了子会话还在运行」。
- **根因一**：子节点这一轮没交上来时，代码先按 `rescueQueuedFor` 判断"它已经排进重试"，把转录状态写成 `queued`，然后**照样**把它塞进 `failures`——状态说"待会儿接着跑"，记账说"它失败"。父章节当场被锁死，而那一支并没有死。
- **根因二**：失败清单是按"哪一章的 goroutine 返回了什么"追加的切片，没有生命周期。重试队列在批次外面把同一章推通了，旧条目仍然留着，阶段汇总于是一边打印"通过"一边报它失败。
- **改法一**：新增 `go/internal/latex/retryingchildren.go` 的 `retryingChildrenError` + `isRetryingChildren`；细分驱动的子节点错误分支里，`state == "queued"` 的记进 `retrying`（不再进 `failures`）。`wg.Wait()` 之后如果只有这类子节点，父章节返回 `retryingChildrenError`——不提交、不写失败。批次派发（`convertChapterBatch`）与 retry 派发（`dispatchRetries` 的 continue 分支）两处都认这个类型，只记一行"子章节还在重试，这次不提交（不记失败）"，并把它并入"这次没跑完、不计失败"的清单（与 T244 的切回同一口径，日志分开写，不混成"有切回请求在等"）。真正跑到死的子节点（提醒用满、复盘没救回来、没排任何重试）照旧记失败。
- **改法二**：`activity.go` 新增 `latestConvertTranscript`（`convert_<章>.jsonl` 与 `.tryN.jsonl` 取 mtime 最新）、`chapterPassedOnDisk`（产物在 + 没有 `.tex.checker` + 最新一条转换转录最后状态是 `pass`，三条同时成立）、`dropHealedFailures`，以及 `Runner.chapterFailure`——阶段汇总前先摘掉"后来已经通过"的章节并记一行日志；两个报错口（首批与补跑批次）都改走它。
- **判据取舍**：只看"产物在"不够（失败前可能留着上一版产物），只看"状态是 pass"也不够（那可能属于另一次尝试），所以三条同时要求；`pass` 时 `.checker` 会被删掉这一既有事实正好当第三条。
- **测试**：新增 `t247_retrying_children_test.go` 三条——`retryingChildrenError` 只被自己认出来（普通失败与 nil 都不算）、章名排序稳定；`dropHealedFailures` 三条样本里只摘掉"后来通过"的那一条，仍有 `.checker` 的与没有产物的照旧算失败；`chapterPassedOnDisk` 三条判据逐个反证（缺产物、留 `.checker`、状态不是 `pass` 都不算）。`go test ./internal/...` 只有那条与本批无关的环境用例失败（`TestPythonEnvVenvRunsInsideRealSandbox`：本机 python3 已升 3.14 而 `libpython3.14.so` 缺失）。
- **未改**：`retrying` 的父章节这一刻不写自己转录的状态（它没开会话），页面靠子节点的 `queued` 与随后 T231 排的那条「继续跑」体现进度；子节点通过后替父章节排「继续跑」的链路本来就有，没有动。

## 批次 T248：章节行选对"现在这一次"尝试

- **现象**：三章的页面行各显示一种旧状态——一章产物由 try1 交出、当天通过，行上却是十天前的 try10「等待并发槽」；一章停在上一代的 try13；一章 try3 已通过，行上却是 try4「运行中」。用户据此以为工作区坏了。
- **根因**：`newerAttempt` 的比较顺序是 archived → 尝试号 → mtime，号大者优先。子章节的尝试号随根章重来**重新从 1 数起**，上一代留下的废尝试号往往比现在这次更大，于是"号大但十天没动"的废尝试压过真正交出产物、当天通过的那一次。P60 加这条本是为了"默认打开最新一次"，但判据选错了维度。
- **改法**：`web/src/legacy/sidebar.ts` 的 `newerAttempt` 改成 mtime 优先、尝试号退为并列时的确定性比较；archived 优先级不变（工作树那条永远优先于归档）。接口字段与数据一个都没动。
- **测试**：`tests/chapter-tree.spec.ts` 新增「尝试号更大但更旧（上一代的废尝试）不压过现在这次」；`npm test` 23 个文件 277 条全绿；`vite build` 重出 `go/internal/sessionview/assets/dist/viewer.js`（产物照规矩提交）；`go build` 通过。
- **未改**：「历史 N」页签仍按尝试号列出全部轮次（那里本来就该按号排）；接口侧 `chapterAttempt` 的含义没动。

## 批次 T251：阶段头按章节报数

- **现象**：页面「章节转换」一行报出「1811 章 · 完成 1370 · 运行 1 · 错误 53」，实际全书只有几十章，用户无法判断还有几章没完。
- **根因**：`stageChapterAggregate` 数的是该阶段下的**转换转录条数**——同一章的历史尝试、上一代归档、返工留下的多条各自算一章。
- **改法**：`web/src/legacy/sidebar.ts` 先按 `chapterId` 收敛，取「这一章现在那一条」（判据与章节树一致：非归档优先、mtime 新者胜），再据此算完成／运行／错误。
- **测试**：`tests/chapter-tree.spec.ts` 加两条（同一章多条历史只算一次且状态取最新；归档不压过现役）；`npm test` 279 条全绿；`vite build` 重出 `viewer.js`；`go build` 通过。
- **未改**：子章节各算一章（它们本来就是独立章节），总数仍会大于顶层章数。

## 批次 T249 / T250：换尝试不搬子树、等子会话不算错误

- **T249 现象**：一章在换新尝试后被"继续跑"恢复成一条「1 条历史消息 · 轮次 0」的空转录，第 1 轮就重新 `request_chapter_split`、重发全部 16 支子章节；当天日志里这一章"复用产物"命中 **0** 次（同书其它章共 13 次）。查树：现役 `work/temp/conv_<章>/work/_subdivision/child_01/node/result/` 是空的，留档 `work/attempts/<章>/try3/work/_subdivision/child_01/node/result/` 里有产物。
- **T249 根因**：`archiveChapterAttempt` 把 `node.NodeRoot/work` 整棵挪走；子节点的产物、转录、已通过结论都在这棵树里。判据写着"换尝试不删 `_subdivision/`"，实现却整棵搬。
- **T249 改法**：新增 `moveKeepingSubdivision`——留档 work 下文件时跳过 `_subdivision/`，其余照旧；子节点产物与转录原地不动，新尝试按 T236 逐支复用。
- **T250 现象**：页面上一批父节点显示「会话错误」，实际在等子节点（107 条转换转录里 9 条最后状态 `error`，其中数个同时是 `waiting_children`）。
- **T250 根因**：细分父章节在子树上出错时一律写终态 `error`（T231 为了不再停在"等待子会话"）；T247 之后"子节点还在重试"不算失败，却仍走写 `error` 的分支。
- **T250 改法**：该处按错误类型分流——`isRetryingChildren` → `waiting_children`，`isPaused` → `paused`，复盘排了队 → `queued`，其余 `error`。
- **测试**：`chapter_gate_test.go` 的留档用例改为新口径（不再要求留档里有 `_subdivision`，改为断言它留在原地、子转录仍在）；`go test ./internal/...` 只有那条与本批无关的环境用例失败。
- **未改**：留档里因此不再有 `_subdivision` 快照；真正跑到死的子节点仍记 `error`。

## 批次 T252：复盘截断残片不再当会话列出

- **现象**：预览页「会话」阶段平铺出几十条带时间戳的条目（`20261027-111514-convert_<章>.jsonl`，3 条消息 / 27.9 KB），每条显示未开始。
- **根因**：这些是复盘截断时抄进 `work/rescue/cuts/<章>/` 的**残片**（`saveCutEvidence`，名字前加 `YYYYMMDD-HHMMSS-`）。扫描器按 `.jsonl` 收文件，把整棵 cuts 也收进来；`chapterIdentity` 又不认时间戳前缀，于是它们既没有章节身份、也不进阶段统计，各自占一行。
- **改法**：`skipScanDir` 整棵跳过 `work/rescue/cuts/`；`chapterIdentity` 先剥 `^\d{8}-\d{6}-` 前缀再推章节身份。
- **测试**：章节身份用例加一条带前缀的路径（断章 ID／父章／深度）；`go test ./internal/sessionview/` 全绿。
- **未改**：cuts 里的残片不删（复盘证据），只是不进会话列表。

## 批次 T253：预览页启动慢——索引缓存落盘

- **现象**：`docvision sessions --serve` 启动要等好几分钟，用户体感"一个只读网页为什么这么慢"。
- **根因**：启动时 `Scan` 把运行时目录下每条转录整文件读一遍算消息数、用量与内容 SHA。实测该目录 **5365 条转录 / 约 7.3 GB**；这套计算每次启动都重来，内存缓存（`scanner.counts`）只在进程活着时有效。
- **改法**：新增磁盘索引缓存 `<扫描根>/.sessionview-cache.json`（带 `version`），键为转录路径、值 `(size, mtime, 统计)`；`Scan` 先装载、命中即不开文件，扫完落盘。转录 append-only，size 或 mtime 任一变化即失效重算。落盘时按本轮 `keep` 集合剪掉已删除条目。写失败只当没写。
- **踩坑**：`countEntry` 的字段当时是小写（未导出），序列化出来是 `{}`——缓存"写成功"其实什么都没记住，用例当场抓到；改为导出字段后正常。
- **测试**：`t253_cache_test.go`——把文件换成同样长度、同样 mtime 的垃圾内容，命中缓存则消息数沿用上次值（未命中会归零）；用例同时断言缓存文件确实写出。
- **实测**：同一棵真实目录，冷启 **3 分 08 秒**（5365 条）→ 热启 **0.64 秒**。
- **未改**：冷启那一次仍要全量读；SHA 仍在这一次里算（之后不再重算）。

## 批次 T254：阶段头只数章节会话

- **现象**：阶段头「852 章 · 完成 824 · 运行 1 · 错误 12」里，点开错误条目能看到 `work/sessions/chapters.jsonl`、`style_pilot_trial.jsonl`、`style_session.jsonl`、`source/sessions/vector_*.jsonl`——这些是阶段级会话，不是章节。
- **根因**：T251 只按 `chapterId` 收敛，没有章节身份的会话各自算一章。
- **改法**：`stageChapterAggregate` 只统计 `chapterId` 非空的会话（角色仍限 convert）。
- **测试**：`tests/chapter-tree.spec.ts` 新增用例（阶段会话即便 `endState=error` 也不进计数）；`npm test` 全绿；`vite build` 重出 `viewer.js`。
- **未改**：真正的章节会话里仍有旧二进制写下的历史 `error` 终态，T250 只保证以后不再写错。

## 批次 T255：预览页 `concurrent map writes` 崩溃

- **现象**：`docvision sessions --serve` 起来就整个进程退出，`error.log` 里 `fatal error: concurrent map writes`，崩在 `scan` 的 WalkDir 回调（写 `s.keep` 那一行），同一份转储里还有多处数据竞争。
- **根因**：两条都来自 T253 的索引缓存。① walk 回调直接往 scanner 字段 map 写；② 预览页**复用同一个 scanner** 做轮询，两个请求的扫描撞在一起就是两个 goroutine 同时写同一个 map。
- **改法**：walk 期间写本地 map，走完持锁交给字段；给 scanner 加 `scanMu`，同一实例上的 `scan` 串行（扫描 0.6 秒级、轮询 2 秒一次，排队无感）。
- **测试**：新增 `t255_concurrent_scan_test.go`——同一个 scanner 8 个 goroutine 并发 scan，`go test -race` 由失败转通过；`go test ./internal/sessionview/ -race` 全绿。

## 批次 T260：通过过的产物不再被旧核对反馈打回

- **现象**：某章的核对已经交了 pass，紧接着日志出现「checker 第 1 轮反馈」，转换会话被喂回上一轮的问题重做 1 小时；重做完再开核对，请求被上游 `HTTP 400 invalid_request_error / upstream_request_rejected` 拒掉，整章记「checker 会话失败」。
- **根因**：同一章会同时留着多条核对转录（不同尝试、不同代）。驱动只看"手上这条会话的结论"，不看这一版产物**已经通过过**，于是旧会话的 issues 又生效。
- **改法**：`internal/latex/checker_pass_marker.go`——通过时写 `<章>.tex.passed`（产物指纹 = 主 .tex + parts 全部文件按路径排序后哈希）；拿回 issues 时比对：指纹一致且记号比那条核对转录更新，就判为旧反馈，不写 `.checker`、不打回，直接按通过收尾；产物变了或记号更旧照旧打回；判 issues 时清记号。
- **测试**：`checker_pass_marker_test.go` 四条（无记号、旧反馈、产物变化、清记号）。

## 批次：矢量图 compare 辅助工具

- **改法**：矢量图作图会话新增纯 Go `compare_figure` 工具。成功编译后，将原图与当前 PDF 首页栅格化结果做软墨迹比较，使用内容 bbox 原点补偿和 origin/centroid 多起点渐进对齐，返回通道重合度、估计位移和颜色 overlay。
- **边界**：工具是可选模型工具，只提供视觉证据，不改变会话提交状态和原有提交流程。
- **同步**：提示词、`docvision latex --help`、README、CHANGELOG 和 P65 计划条目已同步。
- **测试**：`TestCompareFigureImagesFindsTranslation` 与 `TestCompareFigureToolIsOptionalBeforeCompile` 通过；全量 Go 测试的唯一失败是既有 `TestPythonEnvVenvRunsInsideRealSandbox` 环境用例。

## 批次 P64（第二刀）+ T261：图片核对页与 style 模板库

- **P64 图片阶段可核对**：`motif_review.go` 的复查页接上 `progress_items`。每张缩略图下面新增状态词（只认 `status`：已完成／退回原图／失败／已分类待处理／未处理）、「判定记录」链接（`../progress_items/<md>/<img>.json`）与转换产物入口（TikZ/PDF/PNG/SVG，相对 `source/dedupe/`）。新增两段：「等待转换的原图」（未收尾的，含原图与判定记录）与「退回原图的（已收尾但没画出成品）」——这两批以前混在几百张缩略图里找不出来。`writeMotifArtifacts`/`writeMotifReview`/`motifReviewDocOf` 都多带一个 `prog` 参数。
- **进度行双层口径**：`motif_progress.go` 的 `imageStatusCounts`/`countImageStatuses` 从 `progress_items` 汇总全书（Total/Settled/OK/Fallback/Error/Pending/Raster），`process` 与 `dedupe` 的进度行尾部追加「全书 X/N（画好 a、退回原图 b、失败 c、未处理 d）」；`process` 收尾用 `fallbackImageNames` 把退回原图的图名逐个打进日志。续跑时不会再只剩一个 `1/1`。
- **T261 style 模板库**：`latex_style.system.md` 加「先写 STYLE INVENTORY（含 absent 判定）」与「cls 必须是覆盖清单的参数化模板库，章节作者只写内容」两条要求，manual 固定增加 `## Style inventory`、`## Template library` 两节，example 还要用到题干+解答、定理+证明、表格、图注图这几类模板；`latex_style.user.md` 同步改成先列清单再建库。
- **测试**：`t_p64_image_review_test.go` 钉住状态词、判定记录链接、产物链接与两段集合；`TestCountImageStatusesUsesStatusOnly` 钉住"只看 status"（raster/fallback 也算收尾）。`go test ./internal/latex/` 除既有 venv 环境用例外全绿，`./internal/prompts/`、`./internal/sessionview/` 全绿。

补充（同一批，第二个提交）：复查页的「LaTeX 源」与「会话」入口由 `figures/<名>.pdf` 反推（`figureBaseName`），指向 `source/tikz/<名>.tex` 与 `source/sessions/vector_<名>.jsonl`；图注备注带上分类。测试同步断言这两个链接，未画过的图不生成它们。

补充（同一批，第三个提交）：`progress_items` 新增 `fallback_reason`，process 的两个退回分支（工具链不可用、TikZ 校验没过）各写一句原因，复查页「退回原图的」那段逐张显示——以前只有状态，看不出"为什么退"到底是环境问题还是这张图本身。

补充（同一批，第四个提交）：核对页改成**按磁盘现状现渲染**。新增 `internal/reviewpage`（样式、灯箱、卡片、分段、状态词表一份，`StatusSettled`/`StatusQuality`/`StatusLabel`，latex 的 `image_status.go` 改为转发），`internal/sessionview/review_live.go` 读 `groups.json`+`progress_items`+`figures/`+`tikz/`+`sessions/vector_*.jsonl` 造页面：每张图左原图右成品并排（`.shot.orig` / `.shot.conv`，`:has()` 才把卡片放宽到两栏），图注直出类别、状态、代表图跟随关系、退回原因，成品与链接只在文件存在时出现；`/dedupe` 先试现渲染、没有 `groups.json` 才退回静态 `review.html`，侧栏入口的判据也从「有 review.html」放宽到「有 groups.json 或 review.html」。测试：`reviewpage` 单测（并排/缺成品不伪造/状态词表/命名口径）、`sessionview` 的 `TestRenderLiveImageReviewUsesDiskFacts`（状态、并排成品、三类链接、两段集合、统计口径）与 `TestRenderLiveImageReviewNeedsGroups`。

补充（T262，第五个提交）：现场一次"重跑矢量图阶段"的输出把两件事混在了一起——`[process 18/23] (done: 0, errors: 18, fallback: 0)` 与 `全书 27/388（画好 9、退回原图 18、失败 0、未处理 361）`。查下来的口径问题：① 那 18 张全是同一个模型名报 `model_not_found`（上游 404、标了"不可重试"），进的是"接口错误"分支，而 `errors` 与 `fallback` 是两个互斥计数，`fallback` 只统计 TikZ 没过与工具链缺失，于是 18 张"以原图交付"的图显示成 18 个错误、fallback 为 0；② 全书的 `画好 9` 是 4 张位图 + 5 张文字图（`status=done` 但根本不需要画）；③ `未处理 361` 里 356 张是 dedupe 判为同一内容、复用代表图成品的跟随项，只有 23 张代表图是真作业项。改法：进度行按任务结束时的 `status` 分栏（`done`/`fallback`/`raster`），接口错误作为 `fallback` 子集单标 `api-err`；全书盘点拆 `Work`（非跟随代表图 + 文字 + 位图）与 `Followers`，另加 `VectorOK`/`TextRaster` 两栏，`summary()` 只打印非零分栏；`isSessionNonRetryableError`（含"不可重试"与 `model_not_found`）把这类 4xx 判成 `status=error` 并让这一批当场停下、把原因打进日志，不再逐张空转（实测同一次运行白跑 18 张）。测试：`TestCountImageStatusesSplitsWorkAndFollowers`、`TestStatusSummaryDropsZeroBuckets`、`TestNonRetryableSessionError`，`TestPhaseProgressCountsThisRunOnly` 补 `fallback: 18, api-err: 18` 一行。

补充（T263，第六个提交）：接上一批的现场。18 张代表图（模型名 404）失败之后，`progress_items` 里 356 条 `equivalent_to` 全没了，`dedupe_checked` 还是 true、`dedupe_leader_hash` 已空——正是 `clearUnresolvedMotifLinks` 的指纹。判据"代表图不是 done 就解开"对"还没跑完/刚失败"的代表图同样成立，而解开时没放开 `DedupeChecked`，挂靠（`motifAttachLeftovers`）与复用（`motifReuseDedupe`）都会跳过"已核"记录，于是这一组永久散架：作业项从 32 张涨到 388 张，再跑一轮 dedupe 也只挂回 80 条（实测 21 秒的核验会话）。改法：`leader.Status != "done"` 时保留链接，只有"已完成却没有可复用成品"或代表图记录消失才解开，并在解开时把 `DedupeChecked` 一并放开；日志只报这一种情况。测试 `TestFollowerKeepsLinkWhileLeaderUnresolved`（两条分支：fallback 不动链接、done 且无 TikZ 解开并放开记号）。善后：`temp/restore_dedupe_links.py` 按 `groups.json` 把链接写回，演练结果 356 条 = 258 个副本 + 98 个组内成员，与失败前一致。

补充（T264，第七个提交）：用户回看说"打印的东西太多了"——`process`/`dedupe` 每帧尾巴上都挂着 P64 加的全书盘点（`· 全书 9/32 项（文字/位图 9、失败 23）· 跟随代表图 356 张`），`process` 收尾还把 18 个退回原图的图名一行列完。实时行本是每帧重画的变化行，不是汇总行；改成只报这一轮队列，`motifProgress.cumulative` 与 `imageStatusCounts.summary()` 一并删掉（`countImageStatuses` 留着给收尾报数），收尾那句缩成"退回原图共 N 张（见核对页）"，`fallbackImageNames` 删除。计数语义保留 T262 的口径：`errors` 只数 `status=error`，接口错误后保留原图算 `fallback`、不再标 `api-err`。

补充（T265，第八个提交）：用户指出单文件版 `standalone.tex` 里子会话、子子会话的内容没进去。核对交付树：`standalone.tex` 6534 行，仍带着 192 条 `\input{chapters/<章>/children/<子章>.tex}` 与 3 条 `\input{figures/motif_*.tex}`，目标文件都在、只是没展开。根因是 `buildStandaloneTex` 只替换 `main.tex` 里 `\input{chapters/xxx.tex}` 这一层，替换表按 `chapters/*.tex` 的文件名建；二级路径与靠 `\input@path` 指路的裸名字都不在表里。改成从 `main.tex` 出发递归内联（包含它的文件目录 → 构建树根 → `\input@path` 目录三种解析顺序、`..` 与绝对路径拒绝、只有"正在内联中"算环、层数封顶 8、每段留 `% ==== 路径 ====` 出处注释），`assemble` 去掉预扫描章节列表那段（终审可能重排改名，跟着 `main.tex` 走）。测试三条：子—子子—共享片段、`\input@path` 裸名、自引用与缺文件原样保留且重复引用各展开一次。真实树重跑：125277 行、残留 0。

补充（T266，第九个提交）：用户看侧栏说矢量图阶段"不同的 try 能否不同的会话层显示，不然有点乱，开子层级放"。现状是每个转录一块、编号取 `imageOrder`，于是同一张图的 try1/try2/try3 是三个同号方块（46 条会话里二十多个序号各出现两次）。新增纯函数 `groupImageAttempts`（图片身份 = `imageName`，老转录退回去掉 `.tryN` 的 id；尝试号从转录名 `.tryN` 读）与 `imageAttemptNo`/`imageAttemptKey`；侧栏两处模板都加了分组分支（多阶段/单阶段各一套，缩进类不同），父行带「N 次」「有失败」、子行 `tryN · 结局 · 时间`，`v-collapse` 记忆展开、当前会话所在组自动展开（只试过一次的图仍走普通会话行，不套展开壳）；方块视图一图一块 + 次数角标；搜索时不分组（沿用 T53 的取舍：历史尝试要能被搜到）。验证：vitest 5 条 + 真渲染探针 `TestViewerVectorAttemptGroupsInChromium`（2 组、父行 3 次、子行 try3/try2/try1、方块 2 + 角标 1、搜索 4 行平铺），web 全量 286 条绿。

## 附：issue/plan 条目收窄时移出的原文（2026-10-28，T267）

用户口径：条目一律 `[X][ ] T<n>.` / `[X][ ] P<n>.` 一行起头、最多再跟「根因／改／验证」三行；本次只收窄 T110 起、P52 起（Agent 本人写的批次），更早的条目保持用户原文不动。下面是被收窄条目的原文，只挪位不删。

### issue.md（T110 起）

T129
  新增 `TestMotifAlignCoverageNormalizesScaleAndPosition`、`TestMotifAlignOverlapRejectsDifferentShape`、`TestMotifAlignShiftCells`、`TestMotifAlignFilterKeepsAlignedDropsOthers`、`TestMotifAlignFilterFallsBackWithoutCoverage`（合成夹具的笔画粗细随半径等比，避免"1px 线条放大 2× 后相对变粗"这种人造差异）；`TestNoExtendsBehaviourUnchanged` 补两个新键；`temp/config_new.yaml`（本地夹具）版本同步 18。）
T137
  抖动实测（同一对交换图，各自做「放大 5%/12% + 随机裁剪 90%/85% + 噪声 σ4/8」，存于 `temp/motif_overlap/sym/`）：**结果与预期相反**——G=64 时同符号变体只有 0.25–0.47，异符号原图对却有 0.55–0.66；G=256 同符号变体掉到 0.05–0.23。根因：**「不缩放画布」对几何变化零容忍**（5% 缩放就让内容框尺寸变、居中贴上后笔画整体错位），而符号差异在完全同几何时反而只扣到 0.66–0.77。结论：**B 方案（不缩放）被推翻**——它只能在“全池几何完全一致”时可用；正确组合是 **letterbox 归一化（长边→G、保持比例、尺度不变）+ 中等 G（64–96）+ 全量平移搜索**，这样尺度抖动被吃在归一化里、符号差异靠更细的格子拉开，仍由会话兜底。
T138
   ④ **逐级搜索**：`G = 32, 64, …, ≤ S`，0 级全窗 `±0.10·S`（换算成格），其后 `中心 = 2×粗层解`、窗口 `±2` 格，细层解顶到窗口边缘时扩窗重搜；
   ⑤ **判定分 = 逐级搜索最终（最细）那一级的分数**，不额外锁定某一层、没有 `min(最大级, 128)`（用户 2026-10-18：「判定层就是逐级搜索的最终结果，不要固定一个 G=多少的层」）。级数一直做到放得进 S 的最细一级（S=390 → 32/64/128/256，S=70 → 32/64），分数与平移都取最终解；粗层只负责把平移搜到位，不参与判定。
   ⑥ 阈值沿用 `dedupe_align_min=0.60` / `dedupe_icon_auto=0.75`；仍只对内容框 ≤200×200 px 的图生效，放不进 S 的退回逐对相似度并在 `pairs.csv` 标原因。
T140
  ④ **不做的事**：把 0 级扩窗夹回 `dedupe_align_shift` 的 ±10% 预算能再快约 6×，但实测 8385 对里 1698 对分数变化（最大 +0.416）、7 对跌破 0.60——那是改判据不是提速，校准要重跑，留作以后单独一批。网格缓存（每图每级前缀和）只占单对成本的 ~2%，不值得加复杂度。
  `pairs.csv` 与日志口径不变；新增 `TestFindVisualMotifCandidatesAlignedCopiesStableAcrossRuns`（副本 + 并行扫描重跑五遍分组逐字一致）。
T147
  合并语义变了，旧结论就必须作废：`motifDedupeVersion` 从 `visual-motif-v2-tools` 升到 `visual-motif-v3`（与 T149 同一批升级，一次 bump 覆盖两处语义变化）——否则上一版跑出的 `groups.json` 与逐图 `dedupe_checked` 仍会通过复用闸门（输入与阈值都没变），续跑跑的还是被 class 切碎的那份分组。
  新增 `TestMotifGroupClassUsesMajority`、`TestMotifApplyGroupClassSetsCenter`、`TestMotifRasterLeaderKeepsFollowers`。
T149
  预期效果（该书 130 份不同内容、桶 `[100,2]`、未分配 28）：100 份的那个桶折成 **1 组**，最终要绘制的图 = 1 + 2 + 28 = **31 张**（与用户口径一致；T147 修复前是 48，再往前 64）。`groups.json`/`report.md` 里这些组标 `source=folded`；`motifDedupeVersion` 一次升到 `visual-motif-v3`（与 T147 同一批，一次 bump 覆盖两处语义变化），旧结论照旧作废。
  新增 `TestMotifFoldGroupsFoldsSameMotif`（两块同一枚图折成一组、代表图在成员里、其余成员跟随新代表图）、`TestMotifFoldGroupsKeepsDistinctApart`（明显不同的两块不折）、`TestMotifFoldGroupsStaysInsideBucket`（关键图达标但不在同一个桶也不折）。
T152
  结论语义变了就必须作废旧结论：`motifDedupeVersion` 从 `visual-motif-v3` 升到 `visual-motif-v4`，否则续跑的复用闸门（版本 + 规则指纹 + 输入摘要全对就沿用 `groups.json`）会继续端出那份 35 张的旧分组，改动等于没生效。
  预期效果：该书 35 张 → **31 张**（1 个折出来的组 + 2 张 + 28 份独立），与 T149 的用户口径一致。新增 `TestMotifAlignFilterCarriesMemberMatchingAnotherMember`（三档尺寸：离中心 0.46、与组内另一档 0.66 的那张要带入、且不能当代表图）、`TestMotifCarryOverDoesNotRelay`（带进来的不能当跳板）、`TestMotifArtifactReportExplainsBelowMinMember`（低于门槛时必须有那句解释、都达标时不许出现）。
T159
  **根因二（空回复）**：`max_tokens` 是**思考 + 正文**的总预算。开着思考的模型配 8192 时，思考把 8192 全吃掉、正文一个字都不吐（`finish=length`、`completion_tokens=8192`、content 空）。原来的空回复自救只是原样再问一次 `Continue.`，预算没变 → 又是一轮 8192 全空 → `empty final answer` 失败。现场两例连续两轮 8192/8192。
T161
  踩到的坑：`state.collapsed` 是 Vue 的 reactive 代理，`hasOwnProperty` 不走 get 陷阱、**建不起响应式依赖**——照 `groupWantOpen` 的 `hasCollapseMemory` 写法会出现"点按钮记忆写进去了但视图不重绘"（要等别的 reactive 变化才显形）。`chapterWantOpen` 改成读一次 `state.collapsed[key]` 建立依赖。
T163
  证据（`--debug` 日志，同一条会话 `convert:chapter_009_child_04`）：
  所以内存侧一切正常（`mem.log` 显示进程 RSS 峰值 0.1GiB、系统可用 6.0GiB 以上，没有一条低内存告警），也解释了为什么「毫无征兆」：SIGKILL 由同一用户的子进程发出，程序侧没有任何可捕获的路径。
  根因：bwrap 沙箱只做了 `--unshare-net`（网络）与挂载表（文件），**没做 `--unshare-pid`**——会话与宿主共享 PID 命名空间，于是 `ps` 看得见宿主进程、`kill` 也能直接对它发信号。文件与网络被隔离了，进程信号没有。
T164
  现场证据（一本 12 章、递归到第三层的书，共 24 条转录）：
T166
  原路径（改动前）：问题只存在于 `submit` 的 `report.issues`（一个字符串），每章落一份 `work/reports/<章>.md`，风格修复大循环只读这些 md。也就是说：子章节的问题**没有任何机制**能到父章节——父会话得自己读很多份 report、再自己抄进自己的 report，任何一步漏了，问题就在层级之间蒸发；多个问题也只能挤在一段话里，无法逐条追踪。
T167
  现状：`clean_project.sh` 只能清到"只剩分类结果"，章节转换及其之后没有对应口径；而且只删目录不够——阶段跳过看 `progress.json`，`convert`/`assemble` 标着 done 会被直接跳过。
T168
  原因：旧 `submit` 只要求"编译过 + 看过自己那份 PDF"，既不要求与**原书页**对照，也没有任何机制检查"md 里的内容是不是真的进了 PDF"。于是拿 python/sed 把 md 机械包成 tex 也能过关：漏段、丢表、公式降级成文本，产物内部自洽，checker 只看 md/tex 结构也发现不了，整章内容缺失却一路进书。
  阈值标定（同一批真实产物上量的）：正常完成的章节 0.97–0.995；把 PDF 文本砍掉 1/4 掉到 0.63–0.76、砍一半掉到 0.33–0.59，所以 0.85 既抓得住"少转十分之一以上"，也不会被 OCR 噪声与重复页眉误伤；只比 CJK 是因为公式抽成文本不可靠，公式正确性交给编译、checker 与看图。
  提示词原本就写了"OCR 可能不准、有疑问就翻原书核对"，但那是**条件触发**（"whenever something looks doubtful"）——机械包出来的 tex 读起来很顺，模型根本不会觉得"有疑问"，于是永远不翻原书；而且没有任何东西核对"到底翻没翻"。所以这次除了加硬门禁，也把那句改成**无条件、每次提交前都要对比几页原书**，并点明"文本比对看不出公式被降级、表格丢行、插图错序"这一类只有原页才能发现的问题。
  另：`submit` 之后把覆盖率写进日志（`内容覆盖率 97.0%（md 正文 → PDF 文本）`），低于 92% 再提醒一次；提示词新增两句（必须对照原书页；不要用脚本把 md 机械包成 tex），整合指令同样要求对照。
  验证：新增 `TestMdPlainTextDropsNonProse`、`TestCoverageOfDetectsMissingContent`（完整 1.0、丢 40% 后跌破阈值并点名缺失段落）、`TestCoverageOfSkipsFormulaOnlyChapters`、`TestNeedSourcePages`、`TestPathWithin`；`TestChapterGateRequiresCompileAndView` 扩成"编译 → 只看自己 → 只看 1 页原书 → 2 页原书才放行"；在真实章节上跑 `chapterCoverage` 与手工测量一致（0.97–0.995）。
T169
  数据：一份真实项目里划分阶段给出的 26 个章节 md 从 150 行到 20190 行不等（总计 13.3 万行，中位数约 3400 行）。几千行的章节丢给一个转换会话，读、写、核对都做不完——不是模型"愿不愿意"的问题，是规模本身超出了单会话的舒适区。
  旧行为的缺口：递归拆分早就有，但触发条件是模型自评"我觉得太大了"，和"有疑问就翻原书"一样靠不住；而且会话根本不知道自己这一章有多少行（提示词里没有数字），只能凭第一次读文件的感觉猜。
T171
  三条独立缺陷：
  另有第四条，是修的时候顺着查出来的：重跑时 `node.Attempt` 从 1 重新数，与磁盘上已有的 `convert_<章>.try2.jsonl` 撞名，`NewTranscript` 是 append 语义——两段对话会追加进同一条转录。
  验证：`TestAttemptTranscriptPath` 补"已带 .try2 再派生 → 只留 .try3"（写入方向的保险，任何调用都产不出套娃名）；新增 `TestAttemptNumberOnDisk`（空目录 / 中断续跑 / 已结束取新号 / 第 3 次规范名 / 同前缀的别章文件不串）；`TestChapterIdentityAttemptSuffix` 补第 3 次与子章节第 3 次的归组；新增 `TestActivityAfterEnd`、`TestReadStatSawRunEnd`。`go test ./internal/...` 全绿、`npm test` 143 通过。改名后拿真实项目 `sessions --list` 复核：`convert_chapter_001` 的三条（第 1/2/3 次）标签都是 `转换 · chapter_001`。
T172
  根因：checker 的只读视图只有**两个文本**（章节 md、提交的 .tex）与一个 `parts/` 文件夹，工具只有 `read_file`/`grep`/`submit`（P28 定的"极简只读会话"）。它看不见编译产物，也看不见原书页，因此凡是"md 这么写、tex 那么写"的差异只能猜：md 是 OCR 文本（断字、重排、重复页眉）、公式与表格在原页上是图形（md 里只剩噪声）、插图与表格在排版里是浮动体——这些正常差异全被当成"缺失/错位"报回去；转换会话为了让 checker 满意只能去改本来正确的内容，或者白跑几轮反馈，攒够 `checker_rounds` 后整章被作废重转。
T173
  根因：
  验证：Go 新增 `TestAttemptNumberForNode`（空目录 / 拆分可恢复时沿用第 1 次 / 无 `run_end` 原位续跑 / 已结束换新号 / 拆分可恢复时即使上次已结束也沿用）；`web/tests/chapter-tree.spec.ts` 新增 5 条"树跟随正在查看的尝试"（父与子同轮、子节点取 ≤pin 的最近一轮、核对会话同轮、pin 超过所有尝试、无 pin 仍取最新）并把标签断言改成 `#N`；`npm test` 148 通过、`go test ./internal/...` 全绿。
  代价与善后：10:56 那次重跑（旧逻辑）已经把 002–006 章的 `work/temp` 整棵清掉，子章节必须重转一次；本次修复后，同一类中断重跑会直接从原尝试继续，不再清子树、不再多出空尝试。
T174
  问题：T173 只做到"拆分计划还在的父节点沿用原尝试"，规则本身仍是"上一次有 `run_end` 就换新号"——所以进程被杀、网络断连、窗口关掉这类中断在重跑时依然会开一条新尝试（用户口径：这不该算一次尝试，新尝试的唯一来源是 checker 连续 `checker_rounds` 轮仍不过、代码主动作废产物换会话）。另一头，换会话时 `chapterWorkTreeAt(resuming=false)` 用 `RemoveAll(nodeRoot)` 把上一轮的工作区连 `_subdivision/`（子章节的成品与转录）一起删了——「历史」里只剩对话，看不到那一轮到底交了什么。
  已知代价：留档会把子章节的成品与转录一起搬进 `attempts/`，所以新尝试重转子章节时看不到旧成品（子节点目前还没有"已通过就跳过"的标记）；同一次尝试内重跑已通过的子章节仍会重放对话后再跑一遍 checker。
T179
  问题：`latex.concurrency` 是**一个**槽位池，逐章转换、整合、checker 反馈轮、提交提醒全挤在里面——核对会话排在池子里等一个慢转换腾位置属于白等（核对读的是已经编译好的 PDF，跟转换不抢同一份资源）。
  配置面：`latex.convert_concurrency`、`latex.checker_concurrency`、`models.<名>.max_concurrency` 三键（现有块加键，不 bump `config_version`），两份模板 + `docs/config.md` + `docs/latex.md` + `latex --help` 同步。
T188
  补记：落地（B 方案，2026-10-21）：**不改磁盘格式**——核对转录仍是一条 `checker_<章>[.tryN].jsonl`（append-only 不动，命名/重放/`/api/drop`/历史聚合全不受影响），改在**历史面板**里按 **user 轮**把它拆成子条目：`web/src/legacy/rounds.ts` 的 `checkerRounds(lines)` 给出每轮的起始行号、行数、结论（pass / 拒绝 N 条 / 无结论）与一句摘要，历史面板在核对会话下渲染「这条核对转录里的 N 轮」列表，点一轮 `jumpToLine(from)` 直接跳到那一轮的起点。判据只认转录自身内容：user 行开新一轮，submit 回执里带 pass 记通过、带 issues/reject/problem 记拒绝并数编号行。只有一轮时不显示（没有可分的）。单测 `web/tests/rounds.spec.ts`。A 方案（按轮切文件）不做——它把命名、`lastAttemptNumber`、历史聚合、重放全牵动一遍，换来的只是文件名可读性。
T198
  两种配置的分工（回答用户"到底哪个数管什么"）：`latex.concurrency` = 其它阶段（classify/process/style-fix/assemble）的 worker；`latex.convert_concurrency` = 同时转换的章节数与转换会话槽位；`latex.checker_concurrency` = 核对会话槽位；真正同时打到同一个模型的请求数由 `models.<名>.max_concurrency` 封顶（未写 = 不限）。
T199
  恢复：从 `*_记录_20261020_0737/work/media/` 把缺的 31 张补回项目的 `work/media/`（只补缺、不覆盖已有文件），复核后附件缺失数归零。
T201
  验证：`TestLooksLikeToolProseLeak`、`TestTruncateCheckerTranscriptFromSubtree`（相对/绝对路径都行、`work/` 之外拒）、`TestChapterProductsForChildInTempTree`（暂存池、`integrated-stage`、`children.tmp` 不算）、`TestRetryPreconditionChildInTempTree`、`TestFindChildMarkdownDepthExact`、`TestChildMarkdownInTempTreeDepthExact`、`TestPostRetryCheckerChildFromSubtreeTranscript`、`TestPostRetryChildConvertWithoutInputQueuesParent`、`TestPostRetryChildConvertWithoutParentTranscript`；`go test ./...` 全绿，`npm test` 199 绿，重建 dist；真实项目上只读复核 `chapter_014_child_04`/`chapter_014_child_06`/`chapter_007_child_02` 三支：产物与 md 都指向各自那一支、`retryPrecondition` 放行；临时副本上用 HTTP 复核两条路径（子章节有临时树产物 → 200 排队重新核对；子章节输入不在、父有转录 → 200 改排父章节 continue）。
T208
  查证：服务端没有问题——用同样的请求（同项目、`chapter_017`、`role=convert`）直接 `POST /api/retry` 返回 200，队列文件 5 秒内被运行中的 latex 消费、旧尝试归档进 `work/attempts/chapter_017/try1/`、新转录 `convert_chapter_017.try2.jsonl` 当场开始写（`t=meta kind=manual_retry` 在位）。而用户点的那一次，队列目录里既没有待办也没有 `.done` 留档，`work/attempts/chapter_017` 也不存在——说明 POST 根本没发出去。
T209
  ② 提交提醒点名"要真的发一次工具调用"——`remindSubmit` 与 checker 的提醒都写明"把调用/JSON 写进正文不算（`\submit{...}` 也一样），要用工具本身"；
  ③ `checkerNoVerdict()`：核对失败若是"会话自己没给结论"（无 submit 回执 / API 错误），**跳过反馈轮**——那串文本不是章节问题，喂回去只会换来一次同样产物的再提交。判定依据是"没有 submit 回执"这个事实，不是正文长什么样（与 T205 的口径一致）。
T217
  已知边界：P58 复盘会话判 `fail`/`truncate` 时会直接返回（`checker 复盘判定会话无法挽救…` / 已排队续跑），走不到第 4 次；且这两句不在 `checkerNoVerdict()` 的识别范围里。本轮不动它（动它就等于改升级链，见上），观察后再定。→ **T221 已补**：这三类（再加 `checker 只读视图无法准备`）都进了 `checkerNoVerdict()`，反馈轮不再被喂；升级链（作废产物、换会话重转）仍未动。
T221
  注：用户现场跑的是这批提交之前的二进制（旧行为照旧），需重新构建才生效。
T224
  迁移口径（为什么不在磁盘上改名）：要让老记录"进到正确的位置"有两条路——把老转录改名成 `rescue_<章>.tryN.jsonl`，或者改读法让它们自然归位。选后者。老转录**没有记录它复盘的是哪一条转录**：转录里只有系统提示词与 `run_end`，审计日志 `work/rescue/log.jsonl` 里也只有 `role`/`verdict`/`transcript_lines`/`ts`，没有目标文件名、也没有尝试号；强行改名只能编一个猜出来的 `.tryN`，这与"不编造转录没记录过的元数据"直接冲突，而且只改名字不补 `rescue_target` 行，界面照样读不到目标。读时归位的结果是一份不丢、一个字节不改、正在写的转录也不会被碰到。`rescueRowTitle` 对老转录照旧给「复盘对象：<转录名去掉 rescue_ 前缀>」，不猜角色与轮次。
T230
  根因二：核对只要转录文件还在就回放历史，而文件是跨调用共享的，父节点重驱动与页面重核都往后追加，坏尾巴越滚越长。
  根因三：复盘判截断后把前缀抄到 work/sessions 下的新尝试，节点驱动读的是节点目录里那条，两条通路不是同一个文件，所以判了也改不动它读的那条。
T231
  结论先说：022_child_12 与 024_child_05_child_05 是标签缺陷——转换那一行写着会话错误，核对其实在 16:42/16:46 与 16:37/16:41 就通过了（会话里记的转换尝试号对得上），产物是过的。023_child_04 是真的没结论，核对从 16:23 起停在待核对，不是显示错。024_child_09 是真卡住：三个子会话里两个已通过，第三个的核对 17:41 报错，这条报错没写进任何转录，父会话一直在等一个不会来的消息。
  根因二：核对「没给结论」被记成会话错误。核对几种收尾（会话退化、复盘判救不回、始终不 submit、只读视图没准备好）返回的都是没结论，但转录上写的是 error。
  根因三：父节点的失败状态没人写。子节点转换失败只进了返回值，父转录最后一条活动还是之前的「等待子会话」，失败的那个子节点自己也没写状态。
  根因四：子章节核对通过后没有谁去叫醒父节点。父节点的整合只在它自己那一轮驱动里发生。
T232
  现场（预览页 /api/index 只读快照，3932 条转录）：live 17 条 = 转换 9 + 修复 1（共用转换池 10 格，正好占满）+ 核对 6（池 20 格）+ 复盘 1；排队 175 条。父章节不是进程卡住，是它们的子章节在等转换槽位：chapter_013 那棵树里只剩 chapter_013_child_02 与 chapter_013_child_03_child_03 两条 queued，从 19:38–19:39 一直等到 20:17 没拿到槽位。人工请求被章节锁当"忙"挡回队列，于是看起来"点了没反应"；截断之所以立刻生效，是因为它走核对池（20 格只用了 6）。
T235
  先说结论：① 子会话不跟着切换是真缺陷，已修；② 会话数在涨是那几次作废重跑与自动重核在按预期新增转录，不是漏写；③ chapter_011_child_01 的「会话错误」是旧版本写错地方留下的，之后的运行不会再写上去，但已经落盘的那一行按规矩不动（用户自己重新核对一次即可盖过去）。
  取证：只读快照里这个父章节有两次 live 实现（第 1 次 14:19 停下、判定问题；第 2 次 21:35 在等子会话），它的第 1 次尝试整棵子树已经挪进 work/attempts/<章>/try1/（子章节的转换尝试 1–3 全 pass）。页面钉住第 1 次时子行仍全取 live 的那一代；而钉住第 2 次与不钉时的结果本来就一样，用户看到的"没反应"正是这一半。
  根因二：接口里的 archived 字段在多项目布局下恒为 false——判据是 rel 是否以 work/attempts/ 开头，而扫描根是 latex_project/、rel 实际是 <项目名>/work/attempts/...，前缀永不命中，页面上归档标记、活体优先、修复/复盘按世代挂靠整套失效。
  根因三：子节点转换失败的终态写在**第 1 次尝试**的转录上。convertChapterNode 按值收配置，尝试号推导与转录名改写都发生在副本里，调用方手里的路径还是最老的那条；这一行因此在几小时后被打上新时间戳，页面按最新挑行就把**那一代**显示成「会话错误」（那一代其实早就 pass 了）。
  数字：三次快照对比，3.8 小时里新增 175 条转录、消失 23 条（作废尝试整棵搬进 work/attempts/ 后在旧路径上消失，不是删除），净增 152；新增里按角色是核对 74、复盘 57、转换 36、修复 8，其中 29 条一出现就在 work/attempts/ 下；只有 6 个新章节 ID。也就是说涨的是"作废重跑 + 核对自动换新尝试"这两条路径各自开的新转录，1230 这个数从来不是代码里的上限，也没有任何计数器等于它。
T236
  先说结论：004 与 012 是真缺陷（截断把子章节的工作树当垃圾清掉了），已修；013 同一机制（第 1 次尝试归档里 6 个子章节全部 pass，第 2 次从 00:40 起把它们重跑）；017 不是新尝试，是父章节在 08:18 跑「继续跑」时子/孙会话 `SESSION_CONNECTION_TIMEOUT`，那一支的子章节本来就还没转完。
  现场：004 第 2 次尝试 00:08 复盘判截断，排队请求里没有 cut_attempt（裁剪当场失败），00:09 派发端按「接着干」开第 3 次尝试，顺手把工作区按存档点回退——`git reset --hard` 之后再 `git clean -fdx`，`work/_subdivision/` 整棵被删，里面是 00:08:50–52 刚通过核对的子章节；于是子章节从 00:21 起全部重跑。012 同一机制（它的截断当场成功、请求带 cut_attempt=2）。
T244
  顺带收掉一类残留：同一个按钮点两下，第二条切回的目标就是第一轮搬完后的现役那一次，它永远排不上，以前会一直躺在队列里每 5 分钟重排一次、永不消失；现在派发端发现目标已是现役就把它作废并留一行说明。
T260
  根因两条：① 同一章同时留着多条核对转录（不同尝试/不同代），驱动取状态时没有"最新一条已是 pass 就不再打回"的收口；② 核对遇到 400 是按"不可重试 = 会话失败"处理的，而 400 往往只是这一次请求的输入被上游拒（模型侧参数/内容问题），换一个核对模型或收敛一下输入就能继续，不该让整章失败。
  补记：（已修）章节已判 pass，仍被更早那条核对会话的旧 issues 打回重做。
  根因：同一章会同时留着多条核对转录（不同尝试、不同代）。驱动只看"手上这条会话给出的结论"，不看这一版产物是不是**已经通过过**——旧会话一被跑起来，它的 issues 就又生效。

### plan.md（P52 起）

[ ][ ] P52. (2026-10-19 仅记录，未实现) **去掉独立的章节划分阶段**：不再先跑一个 `chapters` 会话把全书 md 切成 N 个章节文件，而是把**整本书直接交给转换层**，由转换会话自己判断"这章太长、我一次干不完"时调用已有的 `request_chapter_split`（结构化工具，子区间当场校验：全覆盖、不重叠、最小行数）就地细分，父节点再整合子产物并过自己的 checker（这条递归链路 P34 已经建好，只是目前由上层先划分、细分只是兜底）。预期收益：① 少一次全书级会话（少一份转录、少一轮 token 与时间）；② 拆分点由真正读内容的那一层决定，比"先盲切再逐章读"更自然，跨节公式/定理表/习题编号的接缝可以按语义切而不是按行数猜；③ 产物更短更省：每个转换会话只背自己那段，长章节一次性喂进去的浪费没有了。要想清楚的点：全书只有一个转换会话时并发从哪来（要么它自己拆成子节点后并发、要么外层按预估体量预切几个"块"再各自细分）、失败重试的粒度（现在一章失败重跑一章，整书入口失败重跑什么）、进度与预览页的面包屑（章节 ID 从"划分产物"变成"递归路径"）、以及 `chapters/` 目录与 `progress.json` 的阶段语义怎么收（保留成派生产物还是彻底取消 `chapters` 阶段）。与既有 `chapter_granularity`（large/medium/small）的关系也要一并想：它现在是"划分会话的建议粒度"，取消划分后应该下沉成"转换会话自拆的建议粒度"。

[X][ ] P53. (2026-10-20 起已实现阶段一、阶段二；阶段三（整阶段重跑）按 2026-10-21 的决定不做。**这里的「阶段」只是本条目内部的分步，不占 plan 的 P 编号——这条计划在 plan 里就叫 P53**） **手动重试**：在预览页看到某条会话不对时，能从那一章直接重跑，并让重跑进「历史」。约束是预览页是**只读观察者**（另一个进程、GET-only），真正干活的是 `docvision latex`，所以按钮不能执行命令、只能提请求：页面 POST `/api/retry` → 往 `<项目>/work/retry/req-<ns>-<章>.json` 排一个文件（append-only、消费后改名 `.done.json` 留档）；运行中的 latex 在转换阶段的安全点（每批章节前后，最多补两轮）消费它，没在跑就留到下次启动——按下那一下不会白点。重试语义只有一条：**这一章开一个新尝试**——旧尝试整棵挪进 `work/attempts/<章>/try<N>/`（含旧 report），提交的 `.tex` 与分片移走（否则转换阶段"已有产物，跳过"），新会话写 `convert_<章>.try<N+1>.jsonl` 并在转录里记一条 `t=meta kind=manual_retry`（原因/尝试号），预览页「历史」里两条都在。阶段一做两个角色、粒度是**顶层章节**：**转换重试**（重转这一章：归档旧产物、清掉提交的 .tex、新会话 `convert_<章>.try<N+1>.jsonl`）与**重新核对**（产物一个字不动，只换核对会话与只读视图，旧结论挪成 `<章>.tex.checker.try<N>`、新会话 `checker_<章>.try<N+1>.jsonl`；只给结论，不会自动打回转换会话——要修就再重试转换）。队列请求带 `role`（convert/checker），消费者按角色分流，`/api/index` 的 `retryRoles` 让「待重试」挂在对应那一行；没有已提交产物的章节拒绝重新核对（细分出的子章节不单独重试——重跑它得连同父节点的整合一起走，只给"重试父章节"；`--only chapter_002,chapter_005` 是同一套语义的命令行入口，自动把 step 收成 convert）；**阶段二已实现（运行期手动干预，2026-10-21 用户提的"针对可能有错误的会话手动继续跑、删掉错的记录、不用等程序重启、手动重新加入并发队列"）**：① 消费不再只在转换阶段的两个安全点——`RunBook` 起一条 watcher（2s 一跳）跑在整个生命周期里，样式/划分/转换/组装**任何阶段**都能收请求，真正进并发槽位（转换/核对各自的池子）；② 加**章节锁**，主流程的并发批阻塞拿锁、派发端 `TryLock`——忙就**不消费**、请求留在队列里等下一次 tick，同一章永远不会跑两遍；前置条件没到（样式未就绪 / 还没产物 / 还没转录）也留队并只提示一次；③ 新增第三种角色 **`continue`（继续跑）**：接着这一章最近一次尝试干——不新建尝试、不归档、不动已提交产物，**不插任何 user 内容**（历史原样续行，与中断续跑同一条规矩，2026-09-29 定的），只解除"上次已 submit 就收账即结束"——点它的意义正是"结果不对、接着改"（`convertChapterNode` 的 `retry` 兼作"人手动要求的，产物在也要跑"）；④ `/api/drop` **删除记录**：删 `work/sessions/` 下的转录（拒绝路径穿越），带 `.tryN` 的连 `work/attempts/<章>/try<N>/` 一起删，90 秒内有更新或存在 `.partial` 的（可能还在写）回 409 拒绝；⑤ 页面按角色给按钮：「重试这一章」（convert）、「重新核对这一章」（checker）、「继续跑（不新建尝试）」（convert 会话）、「删除这条记录」（任何章节会话）。**阶段二之一：从某条记录之后继续（用户："我说的从某个错误的地方开始就是从这个错误的最后一条或者前几条我可以自己选择"）**——轨迹表每行一个「⤓」动作：把这一章的转录截到这条消息（含），后面的轮次丢掉，抄成**一条新尝试**接着干；源转录一字不动（原记录仍在「历史」里、也删得掉），页面用 `confirm` 说清"后面会丢、原记录保留"，服务端校验行号范围、源文件必须在本项目 `work/sessions/` 且是 `.jsonl`、只有「继续跑」能带 `from`。诚实的限制：当时只回滚**对话**，工作区还是最后一次的样子（没有按轮次快照文件树）——这一半后来由 P55 补齐。**阶段二之二：矢量图那一侧的重画（用户 2026-10-21："可以给矢量图那边加上重跑重试的功能……因为修改之后后面都可以变动"）**——作图会话所在的「矢量图」阶段行/详情栏给「重画这张图」：按转录名找回那张图的进度记录，把旧作图会话改名成 `.tryN.jsonl` 留档（仍在「矢量图」里看得见，不删）、旧工作区挪进 `work/attempts/vector_<成品名>/try<N>.work/`，清掉这张图的成品与状态（`tikz_code`/`figure_*`/`error`/`kept`），类别、标签、dedupe 分组与代表图一律不动，然后只让这一张图重跑一遍（`ImagesOptions.Only`：跳过分类与 dedupe 两个阶段，只跑 process + 重新嵌回 md）。与 images 阶段用同一把 `vectorMu` 互斥——两者都写 `source/progress_items` 与 `source/*.md`。**从某条记录之后继续也给了作图会话**（用户 2026-10-21："矢量图这边允许一下，从某个消息处重试就是，不是完整重新，和章节那样一样"）：轨迹表每行的 ⤓ 对「矢量图」同样可用——历史截到那一条（前 N 行留在原地当续跑起点、**完整的旧转录抄成 `.tryN.jsonl` 留档**，回滚不丢对话），成品照旧清掉，然后只重跑这一张（**同样不发任何内容**，历史原样续行；原本想塞一句"历史已回滚"的说明是死代码——`resumeUserText` 在有历史时一律返回空串，已删）。与整张重画的区别只有一个：会话是接着那条截断的历史说的，不是从零开始。**阶段三（style/assemble 这类屏障阶段的整阶段重跑）按用户 2026-10-21 的决定不做**：只做转换会话与矢量图这两侧的重试/重画，"修改之后后面都可以变动"由"改完自己重跑下游"来承担，不在软件里自动触发。

写入口可整体关掉（`docvision sessions --no-retry`）。重试只做转换这一层：**不进样式反馈回路**（那会拿全书报告改 cls、再分块重转整本书，属于"整书重跑"的决定）、也不自动 `assemble`，日志提示"要更新整书 PDF 请再跑一次 --step assemble"。

[ ][ ] P54. (2026-10-21 用户提出，排期中) **手动插入一条用户消息**：重试/继续跑的时候，人手里常常有一句具体指令（"这条线要粗一点"、"这张图的坐标轴刻度换成 0/5/10"、"这一章把示例 3 换成表格"），现在只能排一个"继续跑"、模型自己去猜差在哪。要的是：**把人的这句话作为一条真正的 user 消息递进会话，然后让它接着干**。用户原话："能否支持用户消息插入？有所修改可以。在这边提供一下输入界面。这个可以排期一下，可能比较复杂。因为后面章节转换那边也想加入一下。"——两边都要（作图会话与章节转换会话），复杂度主要在输入界面与会话语义，不在队列。设计要点（待实现）：① 复用 P53 的队列，`retryq.Request` 加 `Text`（`json:"text,omitempty"`），角色仍是 `continue`（章节）与 `vector`（作图）；与 `From` 组合即"从某条记录分叉 + 带一句指令"，不带 `From` 就是"接着最近一次干 + 一句指令"。② **先看清现状**：`resumeUserText(msgs, note)`（`internal/latex/resume.go`）在有历史时**一律返回空串**（2026-09-29 定的"续跑不插 user 内容"），所以手动继续/从记录继续今天**一个字节都不发**。P54 要真发出去必须**绕过这条规矩**：`continue` / `vector` 请求带 `Text` 时直接 `sess.Run(session.RunOptions{UserText: text})`，且只在这条手动路径上这么做——中断续跑仍保持"不发"。③ wire 合法性：会话最后一条是带未回执 tool_call 的 assistant 时，续跑层已会合成占位回执（T159），插入的 user 消息排在其后；要有测试钉住"插入后第一轮请求不 400"。④ 输入界面：详情栏一块可折叠的 textarea +「插入这条消息并继续」按钮（预填上一次输入，按会话记在 `localStorage`；限长 ~2000 字符、拒绝纯空白；按钮旁写清"这条消息会作为一条 user 消息进入这条会话，然后它接着干"）；想插在中间就先去轨迹页点 ⤓（带 `from` 的分叉）。⑤ 边界：预览页仍是只读观察者（只排请求文件）、`--no-retry` 同样关掉这个入口、服务端校验长度与项目归属、live 的会话拒绝（正在写的不许插）。⑥ 预留：同一个 `Text` 以后可以喂给"重画这张图"——现在 `reason` 只进日志与 `t=meta`，模型看不到。

[ ][ ] P55. (2026-10-21 用户提出："不是只回滚会话回滚会话的话可能会对模型造成困惑，这边也无需记录就是按照历史模型操作过的过程倒退着返回回去不可以吗？还是说需要额外增加实现部分？" 已实现) **从某条记录继续时连工作区一起回退**：P53 阶段二之一只截断对话，工作区仍是最后一次的样子——模型记得那一刻的文件，一 `read_file` 却发现后面乱改的内容还在，于是要么顺着改坏的内容继续、要么整段重做，"从这儿重跑"看着就像没用。**纯"按工具调用倒着撤销"做不到**：转录里只有 `write_file` 的**新**内容（覆盖前的原文不存在于任何地方）、`edit_file` 的 old/new 可反替换但只覆盖一部分、`bash` 完全不可逆（`mv`/`rm`/重定向/跑脚本改文件），半途失败还会把工作区留在"既不是第 N 条也不是最后"的状态，比不回退更让模型困惑。所以要的是**那一刻的树状态**，做法是每轮快照：给每章的临时工作区配一个**工作区之外**的裸 git 仓库（`--git-dir=<proj>/work/rounds/<章>/repo.git`、每次命令显式 `--work-tree=<该章 work>`，模型看不见也碰不到，不用给任何工具加钩子——包括 `bash` 的副作用，因为同一棵树都在里面）。`TranscriptWriter` 记物理行号、`Session.SetMessageHook` 在每落一条消息后回调，`roundLog` 立刻 `git add -A && commit` 并把 `行号 → commit` 追加进 `journal-try<N>.jsonl`（按尝试分文件：重跑续写同一条转录时行号接着算）。`continueFromChapter` 在截断之后按行号取**最近一张 ≤ N 的快照** `reset --hard` 再 `clean -fdx`（`-x` 连忽略的编译产物一起删，回退后的树就是那一刻的样子），并把已提交的产物/分片/报告挪进 `work/attempts/<章>/try<源尝试>/`（那一刻未必已提交，未来的 `.tex` 留在原地只会把模型带偏）。`*.pdf`/`*.aux`/`*.log`/`*.synctex.gz` 等产物写进仓库的 `info/exclude`（不是工作区的 `.gitignore`，免得模型去改）：不进快照、回退时照样被 `clean -fdx` 删掉。降级路径（**兼容老会话**，只有一种模式：能退就退、退不了就只退对话，不做开关）：快照缺失（P55 之前产生的会话、宿主没有 `git`、递归子章节 `_child_` 不做快照）→ **只回退对话**，工作区与已提交的产物都保持最后一次的样子（与 P55 之前的行为逐字一致——退不了还把 `.tex` 挪走，就成了"工作区留着后来的改动、产物却没了"的半新半旧状态）；这时连快照仓库都不建（先只读 journal 再决定），日志说清"没有工作区快照"。**页面上提前说清**：排队的回执（`POST /api/retry` 的 `note`）会查这一章对应尝试的 journal 有没有覆盖这一行——没有就在回执里点名"⚠ 这条会话第 N 行之前没有工作区快照，只会回退对话，工作区里后来改动的内容会保留；要连文件一起干净重来请用「重试这一章」"，`⤓` 的确认框也写了同一句。单次提交失败只告警一次、不再每轮重试。顺带的好处是**不需要再往会话里插说明**——历史与工作区自洽，2026-09-29 定的"续跑不发任何内容"正好成立。

[X][ ] P56. (2026-10-21 已实现 ①–⑤) (2026-10-21 用户提出，排期中) **重试的总入口搬到中栏 + 防误触 + 可撤销**：用户原话"重试会话的那个总按钮你放哪里了？……不要放在侧边栏里面，重试会话这个应该放在中间的界面中，并且做好二次确认防止误触，并且提供误触的恢复，就是可以还回到之前的会话，并且可以选择删除重试的会话，提供这样的功能，在历史里面。可以选择某个作为当前要进行的会话。"要点：① 中栏表头（`对话/轨迹/历史` 那一行右侧）放一个与实际会话相关的动作条——转换会话给「重试这一章 / 继续跑 / 从这条记录继续」，核对会话给「重新核对」，作图会话给「重画这张图 / 从这条记录继续」，不再藏在右侧详情栏与侧栏悬浮里；② 二次确认用页面内的确认条（写清"会开新尝试、旧产物归档进 work/attempts、历史里两条都在"），不用 `window.confirm`；③ **误触可撤销**：排上队后给「撤销这个请求」（删掉 `work/retry/req-*.json`，新增 `POST /api/retry/cancel`）与「回到刚才那条会话」（切回原选中项）；请求已经被消费、新尝试已经出现时，给「删除这次重试的会话」（复用 `/api/drop`，带 `.tryN` 的连归档目录一起删）；④ 「历史 N」每一行给行内动作：设为当前（下次继续跑就接着它）、删除这一条；⑤ 顺带回答用户第二个问题——子章节（`_child_`）现在不能重试是因为它由父节点的细分树产出，单独重跑它会与父节点的整合/晋级状态脱节；要做就得把"重跑子树 + 父节点重新整合"当成一个动作（或者在父节点上按子 ID 精确重跑）。这一条要一起设计，不要只把按钮放开。**⑤ 的定论（用户定的语义）**：子章节重试不是禁区，边界只有一条——**父节点还在等子会话时可以随便重跑**（它还没整合，重跑只影响这一支的尝试号），`work/chapters/<父章>.tex` 已在（父章已整合/已晋级）就 400 并提示"要改这一支请重跑父章节"（那时父报告里的结论对应旧子结果）；名字打错、父节点既没整合也没有正在跑它的临时树时 404。用户的实际动机是"会话出错要人工干预"，那种场景多用 ⤫ / 继续跑，重试只用于确实要重做这一支。

[X][ ] P57. (2026-10-21 ①③④：对话页每行 ⤓/⤫、fork 后自动切到新尝试、删掉这条及之后再继续；逐条**编辑**历史消息仍未做) (2026-10-21 用户提出，排期中) **从某条消息 fork 一条会话（含编辑/删除消息后再继续）**：用户原话"重试应该是从这个地方 fork 了一个会话就是变为一个新的重试会话但是从某个地方开始对吧。然后自动切换这个为主会话。但是我在网页中没有找到可以从某个消息的地方开始重试？或者对历史消息的编辑然后从某个地方重试？比如删除某些错误消息让他继续之类的。"P53 阶段的 ⤓ 只出现在「轨迹」页每一行，用户找不到（在对话页看不到入口），要做的：① 「对话」页每条消息行也给 ⤓（与轨迹页同一个动作），确认框写明"从这里 fork"，fork 完成后**自动把新尝试设为当前选中会话**（今天要等下一次轮询，且不会自己切）；② 消息级编辑：删掉某几条消息再继续（等价于"从保留的最后一条分叉"）——先做"删除到这条为止"，真正的逐条编辑/改写排后；③ 与 P54（插入一条 user 消息）合流：fork 时允许附一句指令。语义上都是"截断 + 新尝试 + 可选一句 user 消息"，复用 `retryq.Request.From/Text`。 **② 的定论（用户澄清）**：所谓"编辑历史消息"主要就是**删除**——从某处开始把后面删掉再继续，等价于 fork 一条重试会话，这正是 `⤫` 做的事。所以 P57 到此完整：`⤓` 从这条 fork（开新尝试、源记录保留）、`⤫` 删掉这条及之后（默认**就地截断**这条转录，不新开尝试、原文先备份到 `work/sessions/.trash/`，T190）、排队后自动切到新会话。凡是改历史默认就地改，fork 必须是人手动点的那一下。"只改字不改结构"没有做、也不按"就地改写 append-only 转录"做（会同时踩后续轮次是否算数、`h`/会话指纹要不要重算、已 submit 的尝试能不能伪造通过三条规矩）；将来要做就走"编辑 + 从这条 fork"（`POST /api/edit` 不写源转录，开新尝试时按 `from=line-1` 截断后把改后的内容作为一条新 user 消息起头，顺带吃掉 P54）。

[X][ ] P58. (2026-10-22 用户提出，同日实现) **未提交收尾的复盘会话（rescue）**：用户原话"我们的模型有时候会在没有反馈的时候就是意外的中间终止就是没有最终 submit……只是发送 not submit 并不能让模型正确的继续，因为很多时候都是模型已经开始输出错误的乱码，或者 json，或者 xml 的工具等等。我觉得我们需要当模型在没有 submit 就结束的时候，应该使用这个 checker 的模型对这个会话进行一个核对……判断是否是可以 fallback 通过正常的输入他还没有提交（这个时候可能吃小问题……可以挽救的不是模型输出混乱了，可以继续），而有的时候……很多是 xml 工具残片……还有常见的是模型输出混乱，就是到这一轮次开始无故重复任务/内容/或者说他没办法转换等等……并且难以用程序判定是否正常输出。可以用这个应该用那个 verify 那个模型吧，默认和 checker 一样或者新起一个模型并设定对应的配置（是否开启这个功能，默认开启），从而在 submit 的时候可以提交一个轮次的截断，从某次正常对话的最后开始……注意这个会话最好也要保存一下，但是保存到一个单独的分类页面中，比如错误处理，放在最后……在 debug 模式下才会保存对应的 jsonl 的会话记录。这里面可以提供他们获取该会话的最后一条和前几条，然后可以通过工具继续向前或者（比如通过编号等），提交的时候看是否需要截断，还是正常提示等等。"

**要解决的问题**：会话"跑完但没 submit"时，今天的处理是同一会话里发一条提醒（最多 2 次）。这条路径对**还能救**的会话有效（网络断、这一轮判断错、工具参数写坏），但对**已经退化**的会话无效——模型已经把工具调用写成正文（`\submit{…}`、裸 JSON、`<parameter=…>` XML 残片）、开始无故重复前文、或直接说"我转换不了"，此时再发"你还没提交"它只会再退化一次，白烧 1–2 轮。

**设计（定稿）**：把"提醒"升级成"先复盘、再决定"。会话没提交就结束时，先起一个**只读的复盘会话**（新角色 `rescue`）看这条会话的历史，得出一个**结论**，代码按结论行动：
- `retry`：这条会话是健康的，只是一次性失误/中断 → 照旧发那条（已写清"要真的发一次工具调用"的）提醒，继续。
- `truncate`：已经退化 → 截断到**最后一个干净轮次**再继续：复用 P53「从某条记录之后继续」+ P55「工作区一起回退」那套（前缀抄成新尝试 `.tryN+1`、源转录保留、按行号取最近的工作区快照 `reset --hard` + `clean -fdx`）。
- `fail`：救不回来（反复退化、模型直接拒绝） → 不再提醒，按失败收尾并把模型给的理由写进日志/报告。

**工具与判据**：复盘会话只有三个工具，**没有任何可写挂载**，看到的只是目标会话的转录（只读、按需取窗口）：
- `read_session {from?, count?}`：按转录**行号**取一段消息（默认取最后 12 条、单条截断到 2 KB），可以带 `from` 往前翻——正是用户要的"获取最后一条和前几条，然后可以通过工具继续向前（比如通过编号）"。
- `session_stats`：由代码算好的事实（轮次数、工具调用数、每轮 finish_reason、连续重复内容的最高相似段、出现"工具调用被写成正文"的轮次），让模型不必在一坨文本里做算术；这是**事实**，不是结论，判不判退化仍由模型决定（T205 的口径：不靠猜正文长短判生死）。
- `submit {verdict: retry|truncate|fail, from?, reason, hint?}`：`from` 是"最后一条好记录"的行号，`truncate` 必须给；`hint` 会作为一句补充拼进提醒（`retry` 时用），让模型知道"上一条为什么没交"。
提示词 `prompts/templates/rescue.md`：说明自己是复盘者、三类结论的判据、以及"重复出现的模式（第 3 轮乱、第 4 轮提醒后又好两轮再乱）要从**最早**的那个干净点截断"。

**触发与预算**：只在 `convert` 与 `checker` 会话"Run 正常结束但没提交"这一个点上触发（替掉今天的第一次提醒）；`latex.rescue.enabled` 默认 true，`latex.rescue.max_per_attempt` 默认 2（同一次尝试最多复盘两次），同一轮次同一 `from` 重复提交直接升级成 `fail`，复盘会话自身**不递归**（它没提交就按 fail 收尾，绝不给自己再起复盘）。`latex.rescue.model` 空 = 跟随 checker 的模型（与用户"默认和 checker 一样或者新起一个"一致）。

**记录（用户明确要求）**：复盘会话的转录落在 `<项目>/work/sessions/rescue/rescue_<章>[.tryN]_<时间>.jsonl`，预览页侧栏多一个**排在最后**的阶段组「错误处理」（阶段行标题「错误处理 · 会话复盘」），行上带章节、结论（`retry`/`truncate`/`fail`）与被截断的行号；**默认不保存转录全文**（`latex.rescue.keep_records: false`，`--debug`/`latex.keep_session_records` 打开时才落盘），但**每次复盘都写一行审计记录**到 `<项目>/work/rescue/log.jsonl`（时间、项目、章节、尝试、轮次、结论、`from`、理由、tokens），没开 debug 也能事后查"哪一章被判过什么"。

**为什么这样切**：复盘会话是**只读 + 只能给结论**的，权限上不可能写坏产物；它的结论最坏只造成"一次截断"（源转录与归档尝试都留着，随时能删记录重跑），而不是"静默改坏一章"。它也不改变既有语义：`retry` 就是今天的提醒路径，`truncate` 就是 P53 阶段二之一那条人工按钮路径，只是把"人看出来了"换成"复盘会话看出来了"；`fail` 与今天两次提醒后的失败等价。

**实现分三步**：① 复盘会话骨架 + `retry`/`fail` + 配置 + 审计日志 + 预览「错误处理」组（不动截断）；② `truncate` 接 `continueFromChapter`（含 P55 工作区回退与"没有快照就只退对话"的诚实降级）；③ 核对会话也走同一条路（今天 checker 的提醒逻辑与 convert 的提醒是两处，合并到 `remindOrRescue`）。

**实现结果（2026-10-22）**：三步一次做完。`internal/latex/rescue.go`（三个工具 + 复盘会话 + 审计日志 + 预算 + 模型解析）、`remindOrRescue`（convert 与 checker 两处都接上，样式/划分/装配等阶段不受影响）、`rescueQueueTruncate`（排 `role=continue` 的请求，复用页面 ⤓ 那条通路；checker 排 `role=checker` 走 `continueCheckerFrom`）、预览侧 `rescue` 阶段（`StageTitles` = 错误处理、`StageOrder` 里排最后、`LabelFor` 认 `rescue_` 前缀）、提示词 `latex_rescue.system.md`、配置块 `latex.rescue.*`（开关/预算/记录/模型指针）+ `latex.sessions.rescue`（上下文 128K、轮次 12、其余继承 checker）+ 模型条目 `models.rescue`（写在 `models.checker` 下面、`extends: checker`），配置 v22、`docs/config.md`/`docs/sessions.md`/CHANGELOG 同步。测试钉住：`read_session` 的尾窗/`from`/`max_chars=0`、`session_stats` 点名正文工具调用、`submit` 校验、排队请求的角色与 `From`/`Path`、预算默认 2 次、模型回退链、预览阶段归组。**判定口径（2026-10-22 用户调整）**：`truncate` 是默认——会话本身有问题（坏的一轮、工具用错、拒绝、重复/乱码、上下文被错误前提污染、已经做错的工作）就截断，retry 只会把错误留在上下文里；`retry` 只留给**聊天记录干净、纯外部瞬时故障**（断网、API 错误、`finish=length` 截断、进程中断），拿不准一律 `truncate`。提示词里写死这条优先级。

**触发点扩展（2026-10-22 第二轮，用户口径）**：上面"只在 Run 正常结束但没提交"那一条作废——**报错终止（非致命）**与**"声称没有工具"纠正过一次后仍未提交**也进同一条复盘链路（`rescueAfterError` / `maybeRescue`，同一会话、同一套结论处理、同一行审计记录；致命错误余额不足/无资源包不复盘），`latex.rescue.enabled: false` 时三种触发全部退回引入复盘之前的行为（请求数/文件逐字节不变），配置版本 23 → 24（键面未变，只是开关覆盖面扩大）。

**已知边界**：递归子章节的转换会话如果被判 `truncate`，`role=continue` 会被 T211 的分流折成"继续跑父章节"（`From` 丢弃，等于从会话末尾续跑）——子章节的退化目前靠父章节续跑兜底，没有按行号精确截断；子章节的**核对**会话不受影响（`role=checker` 的 `From` 一路可用）。

**补记（2026-10-23）**：① **复盘会话可丢弃重来**——复盘自己报错终止或跑完没 `submit` 只算"这一次没拿到结论"：退掉它的转录（`work/sessions/.trash/`，撞名加序号）、重开一条，最多 3 条（1 + 2 次重来），每次一行警告、放弃时一行明说；**复盘永不复盘自己**；三次都没结论就退回引入复盘之前的行为（照常提醒、仍不提交判失败），并把这条目标的复盘预算一次用光。转录名同时收成 `rescue_<章>[.try<目标尝试号>].jsonl`（与转换/核对/修复同一口径）；同名旧文件先退掉，两段对话不串一条转录。② **状态可见**——复盘一开始就往**目标**转录写 `chapter_activity: rescuing`（预览页「复盘处理中」，且不算 live），判 `truncate`/`retry` 时先写 `queued` 再排队；`truncate` 之后那一章的失败终态也写成 `queued`（队列马上接手重跑），界面不再跳回「会话错误」。③ **自己又细分过的子章节也有提醒与复盘**——`convertChapterNode` 里唯一没接 `remindOrRescue` 的就是"细分父节点整合轮没提交"那条分支（它直接判失败），现已并入同一条链路；普通嵌套子章节本来就与顶层同一个入口。④ 带章节身份的复盘落进**章节阶段组**（预览页才能挂进树），「错误处理」组只留给老命名的转录。

**补记（2026-10-23，P58 补）**：复盘转录的 `.tryN` 改成"**这条目标被复盘的第几次**"（磁盘上已有的最大号 + 1，第二次起 `.try2`/.try3，绝不覆盖或追加到上一份已收尾的转录，只有没拿到结论的那一次才退进 `.trash/`）；复盘开跑前在自己的转录里写一行 `t=meta kind=rescue_target`（章节 / 被复盘的角色与转录名），预览页据此把它**只**当章节树里的一行「复盘 #N」显示、不再自成一个阶段组，历史页签按"同一章 + 同一目标"聚链（新 → 旧，行标「复盘#N」，表头写明「复盘了 转换#1」）。老命名 `rescue_<章>_<时间戳>.jsonl` 的章节归属（`rescue_target` 的 `chapter`，缺则 `session_label: rescue:<章>`）与 `#1..#N` 编号（按 mtime）都是**读时推断**——不重写、不改名任何已落盘的转录；两条线索都没有的老转录仍留在「错误处理」。**再补（同日，用户口径）**：树上**每种只留一行**——修复按子章节取最新一轮、复盘按目标取最新一次（`fixRowLabel`/`rescueRowLabel` 的 `#N` 不再进树，只在「历史」页签里出现），悬浮说明仍给轮数、最新一轮结局与被复盘的会话；`chapterAttemptChain` 不变（仍给全部轮次，新 → 旧），判据收在 `treeFixRows`/`treeRescueRows` 两个可单测的纯函数里。

### P59 预览：以「转换会话」为唯一锚点组织核对与子会话（已实现）

**用户口径（2026-10-22）**："每个会话的历史和对应的转换、核对的关系很难说清楚，特别是核对的多次轮次——应该以转换会话为唯一的锚点：转换会话决定看到哪一个核对，同一个转换会话下可以有历史的不同核对会话；同理子会话也是。不能因为切换核对会话而改变转换会话或造成状态混乱；这样重试核对也不影响，因为都按最新的来。不要打补丁。"

**问题（实测 `chapter_004`）**：转换与核对各自按**文件名里的尝试号**编号（`convert_<章>.tryN` / `checker_<章>.tryN`），两个号没有关系——
- 手动重新核对只让核对的号加一（`checker_<章>.try2` 核对的仍是 `convert_<章>` 第 1 次），于是「同号=同一轮」的配对规则当场失效；
- 根章重来一次后子章节的尝试号又从 1 数起，归档里上一代的 `convert_<子章>.try2` 号比工作树里这一次的 `#1` 还大，树与历史把两代混在一起（"下面都通过了、还有一个在排队"）。

**模型**：唯一锚点是**转换会话**。
- `anchor(会话)`：核对会话记录它核对的转换转录（新增 `t=meta kind=checker_target {"convert": "convert_<章>[.tryN]", "convert_attempt": N, "convert_sha": "…"}`，第一次核对与重新核对都写）；转换会话的锚点就是它自己。
- 树上每个节点：`convert` = 这一次该章/该子章的转换（工作树优先、归档只作历史）；`checker` = **锚点等于该 convert 的所有核心里最新的那一条**。切换/重核核对只影响 `checker`，永远不改变 `convert`；`pin` 一条核对时，`convert` 由它的锚点决定（不是由号决定）。
- 历史页：以 convert 为行首锚点分组——每个转换尝试一行，它下面的核对（含多次重核）按时间排；同一条链里不再出现"两个 `#1` 分不清"（核对行标 `核对#N`，且 `#N` 只是该角色自己的历史序号，不参与配对）。
- 子会话：锚点规则同样成立——子章节行的父节点由子章节**转换**所属的根尝试决定（世代），与它被核对过几次无关。
- 旧转录（没有 `checker_target`）回退：`checker.chapterAttempt == convert.chapterAttempt` 且归档世代相同，否则把核对接给它所在世代里最后一次转换（只在读索引时推断，不改磁盘）。

**实现落点**：① `internal/latex/checker.go` 建会话后写 `checker_target`（`recheckChapter`/递归子节点同一入口）；② `internal/sessionview/sessionview.go` 读出 `ChapterAnchor` 字段进索引；③ `web/src/legacy/sidebar.ts` 的 `buildChapterTree`/`chapterAttemptChain` 改为按锚点选行与分组，`pin` 不再按号推断；④ 单测钉住：重核后 `convert` 不变、旧世代核对不上树、同锚多核对只显示最新；⑤ 归档世代参与链的身份键（`项目|章节|世代|锚点`）。

**P59 补充（用户 2026-10-22 当天追加的三条口径）**：
- **历史页签保留，但内容要换**：转换会话下面看的是这一章**转换**的历史（各次尝试），核对会话下面看的是**这个转换名下核对的历史**（含多次重核）——不再是"某条核对转录里按 user 轮拆成 轮1…轮11、每轮写『无结论』『第 N 行 · M 行』"这种按消息轮次的条目（用户："这些没用"）。所以 `HistoryPanel` 在章节会话下只列**会话级**条目，删掉按轮拆分的子条目与行号/行数文案；img2text 链保持原样。
- **并发优先级 = 创建顺序**：没被阻塞的会话里，创建早的先拿槽位（等待时间最久的先派活）。不需要按层级/父节点加权，只要队列按创建时间排序、且"被阻塞"（等子树的父节点、等章节锁、等前置条件）的不占队头。
- **落地后要迁移当前工作区**：新索引/新锚点字段是读时推断的，老转录不用改写；重启后按 P59 的新模型显示即可，然后继续跑（并把 `chapter_004` 这一类被饿住的子树放出去）。

**实现结果（2026-10-22）**：锚点落地。`internal/latex/checker.go` 在核对会话建好后写 `t=meta kind=checker_target`（`convert` = 产出这份产物的转换转录文件名、`convert_attempt` = 该转换的尝试号），`checkChapterAt` 新增转换尝试号参数：`convertChapterNode` 直接传节点的 `AttemptNum`，`recheckChapter` 用 `lastAttemptNumber` 从磁盘上的 `convert_<章>[.tryN].jsonl` 反推（递归子章节跟在节点自己的 `sessions/` 下找）；每次重核都写一条，指回同一份转换。`internal/sessionview/sessionview.go` 在扫描时解析这条 meta 行进 `SessionInfo.chapterAnchor` / `chapterAnchorAttempt`。`web/src/legacy/sidebar.ts` 的 `buildChapterTree` 改为**先定转换、再由锚点选核对**（有锚点只认锚点，同锚多次重核取最新；钉住核对时转换由锚点决定；旧转录没有锚点才退回「同尝试号 + 同世代」），`chapterAttemptChain` 由 `orderChapterChain` 排序：每个转换尝试后面跟着它锚定的核对（新 → 旧），再是上一个更老的转换。`web/src/components/HistoryPanel.vue` 的章节链只列会话级条目，删掉按 user 轮拆分的子条目；逐图链不变。测试：Go `TestCheckerWritesConvertAnchor`/`TestScanExposesCheckerAnchor`/`TestScanLeavesAnchorEmptyWithoutNote`，Web 侧新锚点用例（同锚多核对只显示最新、钉住旧核对不改转换、指向别的转换的核对不挂树上、历史按锚点归组）。**本轮范围**：只做锚点归组与历史页内容；「并发优先级 = 创建顺序」的队列派发本来已按 `retryq` 文件名（创建时间）排序、被阻塞的请求不占队头，未另改；「老转录读时推断、无需迁移」照旧成立。

**后续（2026-10-23，T218）**：核对链把**没有锚点的老转录**也算进来（`!m.chapterAnchor || m.chapterAnchor === self.chapterAnchor`）——P59 的锚点是 P58 之后才写的，老转录没有它就是"没得比"，只按锚点全等过滤会让历史看起来像被最新一次覆盖；有锚点的仍只认自己的锚点。

### P60 「修复会话」——在已划分的章节与工作区上按修复任务重新下派（设计）


**用户口径（2026-10-22）**："父章节重试代价太大。应该提供一个工具/机制：等子会话完成后，按修复任务重新下派新任务到已经划分好的章节和工作区上，用新会话继续完成（不用核对会话），减少父会话工作量、提高效率；父会话多次和重试次数一样没成功才重新开始会话。核对也可以保留，但由父会话**定向下发要检查的规则**，让子会话必然能过父会话的门槛；子会话多次不通过，就回到父会话下发的那一步。"

**模型**：
- 新角色 `fix`（修复会话），锚点仍是**它所属的那次转换**（P59）：`t=meta kind=fix_target {"convert": "convert_<章>[.tryN]", "issues": <父会话下发的问题清单 ID>}`。它复用该章**已经划好的子章节与工作区**（`work/temp/conv_<章>/`、`_subdivision/child_*`），只重跑有问题的子章节，不重新划分、不重建工作区。
- 账本：父会话在整理阶段把"哪些子章节、按哪份规则改"写成 `work/fix/<章>/plan.json`（复用 `problems` 工具的结构），再一次性下派给子章节的修复会话；子章节修完各自 submit，父会话只做整合，不再逐条重跑转换。
- 预算：每个子章节的修复轮数上限沿用 `checker_rounds` 的口径；父会话整体失败次数达到与转换重试相同的上限，才整章重开（现在的行为），否则只在原地修。
- 核对：修复会话的核对沿用**同一锚点**的核对会话（不新开核对链）；父会话可以通过修复任务把"必须满足的规则"写进子会话的 `problems`/提示，核对按这些规则判。UI 上修复会话与其核对**都挂在同一次转换下面**（锚点不变），只是行上标「修复」。

**前端**：不做新的顶层页签。中子栏「历史」保持按角色分开（P59）；章节树里在子章节的核对行下面多一层「修复」子行（同锚点、带轮次），点它进入修复会话；核对仍归该转换，历史里按"谁的核对"分组（锚点相同即同组）。另外加一个**重置视图**按钮（在刷新旁）：清掉 pin/选中/展开记忆，回到"当前正在跑"的顶层状态。

**后续（2026-10-23，T219）**：修复子行与历史里的修复链要写清归属——树上行标签是 `修复 child_02 #1`（章节 ID 只取本层后缀，与核对行的章节身份同一口径；轮次按旧 → 新从 1 数），历史表头写出目标章节 ID、每行标「章节后缀 #轮次」（新 → 旧，队尾是最早一轮）。挂靠规则一行未动，仍只由锚点决定。

**实现落点（后端已落地，前端待做）**：`internal/latex/`（fix 角色 + 计划文件 + 复用子章节工作区 + 预算，两步都已接进运行路径）、`sessionview`（`fix_target` 解析已落地、树里的修复子行待做）、`web`（子行渲染待做 + 重置按钮已落地）。先在 issue 里拆条（T214 子会话切换 bug / T215 重置按钮 / T216 修复会话），再逐条落地。

#### P60 接口设计（已定）

第一步（2026-10-23）只把接口定死并落地一个**默认关闭**的骨架，不接进现有跑法；下面这份契约是实现依据，后续步骤按它接线。

**工具（暴露给父会话 = 章节转换会话）**：`dispatch_fix`

```json
{"issues":[{"id":"<issue id or slug>","child":"child_02","where":"...","what":"...","rule":"..."}],"note":"..."}
```

语义：**只记录计划**，不同步执行任何东西（父会话还在跑，同步下派会把它的上下文与并发槽一起占住）。回执写明写好的计划文件路径、一共几条 issue、将要下派几个子章节（按子章节分组：`child_01 (2), child_03 (1)`）。校验：`child` 必须是这次细分树里**真实存在**的目录名（`_subdivision/child_NN`）；`id`/`where`/`what`/`rule` 全部不得为空；`id` 不得重复；没细分过的章节直接拒。任何一条不合格都当场 `REJECTED` 并指出是第几条、哪个字段，**不写盘**（半对的计划会让下派凭空少掉任务）。

**计划文件** `work/fix/<rootChapter>/plan.json`（schema 固定为 v1；同目录临时文件 + `rename` 原子写）：

```json
{"version":1,"created":"<RFC3339>","root":"<根章>","children":[{"child":"child_02","issues":[{"id":"...","what":"...","rule":"...","where":"..."}]}]}
```

`children` 按子章节名排序、组内 issue 保持父会话给出的顺序；读回时不认识的 `version` 直接报错（宁可停下，也不按猜测执行一份外来的计划）。

**修复会话（角色 `fix`）**：每个**有 issue 的子章节一条**，跑在该子章节**自己的节点工作区**（`node/work`；转换时建好，递归节点收尾不删），不重建工作区、不重新划分。工具集 = **转换会话的工具集减去 `request_chapter_split`**，**加上 `submit`**（`goal`/`compile`/`bash`/`view_pdf`/`view_image`/`doc_search`/`list_source_pages` 都在；章节规模那条提交门禁对它不适用——这一段本来就是父章节拆下来的，它也不能再拆）。转录写该子章节的 `node/sessions/fix_<childChapter>.jsonl`，第二轮起接在同一条转录上继续。它 submit 后跑的仍是**该子章节原本的核对**（同一只读视图、同一转换锚点，P59）——不是新开一条核对链。

**预算**：`latex.fix.max_rounds`（int，默认 2）= 一个子章节能拿到的修复轮数；转录里每条 `fix_target` 锚点算一轮。用完就**退回老行为**——父章节按原来的重试路径重来，不再无限下派。

**锚点**：每条修复转录带 `t=meta kind=fix_target {"convert":"convert_<子章>[.tryN]","convert_attempt":N,"issue_ids":[...]}`。`convert` 与该子章节核对里 `checker_target.convert` 是同一个名字，`convert_attempt` 与 P59 同口径（预览页据此把修复会话、以及它之后那次核对都挂到同一次转换下面）。

**配置**：`latex.fix.enabled`（本步默认 **false**）、`latex.fix.max_rounds`（默认 2；0/负数也按 2，不是"无限轮"）、`latex.fix.model`（空 = 继承 convert；会话调优沿用 `latex.sessions.convert`，不新开块）。配置版本 22 → 23。

**第一步的边界**：已落地 `internal/latex/fix.go`（计划类型与读写、`DispatchFixTool`、`runChildFix`）、提示词 `latex_childfix.system.md`（**与装配阶段的 `latex_fix.system.md` 不是一回事**，后者是书级编译修复 `book_fix`）、`sessionview` 把 `fix_target` 读成 `chapterRole:"fix"` + `chapterAnchor`/`chapterAnchorAttempt` + `fixIssueIds`（分组一行不动）、配置项与模板、两份配置模板同步。`dispatch_fix` **只在 `latex.fix.enabled` 打开时注册**给转换会话；`runChildFix` 目前**只有测试会调用**——所以默认配置下本批代码零运行期行为。**还没做**：调度（父会话何时下派、修完怎么整合、失败怎么退回）、章节树里的「修复」子行与历史分组、配置开关的打开验证。这几项留给 P60 的下一步。

**第二步（2026-10-23）已完成**：调度接进 `convertChapterNode` 的细分父节点分支——子章节转换+核对完、父会话整合之前，先给它一次**复查对话**（这一步必须存在：子章节只要有一个没过自己的核对，整章根本不会走到整合，所以"哪个孩子还有问题"只能由父会话读过报告与产物之后回答）。`dispatch_fix` 只在 `latex.fix.enabled` 打开时注册，系统提示词那一段（`latex_convert_fixdispatch.system`）也只在那时追加，关着时提示词与请求数逐字节不变。父会话写下计划后，runner 按计划逐个子章节跑 `runChildFix`（各自的 `_subdivision/<child>/node` 工作区、`latex.fix.max_rounds` 轮数预算、转换槽位池），修完用**新的一次核对转录**（`checker_<子章>.try<N>.jsonl`，锚点仍指同一次转换，P59）重判，再把 `.tex`/分片、`children-manifest.md`/`children-summary.md` 与 `issues-inherited.json` 刷新回父章节，最后照今天的流程整合。计划之外的子章节不碰；任何一步失败只记 warning 并退回原来的整合流程。**还没做**：预览页章节树里的「修复」子行与历史分组（`fix_target` 已被读成角色/锚点，树与历史一行未动）；"父章节整体失败次数到与转换重试同一上限才整章重开"这条预算也还没落地（现在仍是整合不顺就按原路径整章重试）。

**第三步（2026-10-23）已完成**：预览页的章节树把修复会话作为**子行**挂在它修的那一行下面（锚点等于这一行选中的那次转换才挂、旧 → 新逐轮列、标「修复 N」与名字里的 `.tryN`、点行切会话，没有修复就一条也不渲染），修复会话自己的「历史」= 同一锚点下的其他修复轮次（新 → 旧），转换/核对的历史分工照旧（P59 不变）；没有修复转录时树与历史与之前一致。**还留着**：「父章节整体失败次数到与转换重试同一上限才整章重开」这条预算仍未落地（现在仍是整合不顺就按原路径整章重试）。

**第四步（2026-10-23）已完成**：最后两件收尾，都不改默认跑法（`latex.fix.enabled` 仍默认关闭）。

① **修复转录有了章节身份**：`sessionview.chapterIdentity` 把 `fix_` 前缀与 `convert_`/`checker_` 一视同仁——`chapterId`/`chapterParent`/`chapterDepth` 与文件名里的 `.tryN` 尝试号都齐，`LabelFor` 给出 `fix:<章>`，阶段归组随"有章节身份"落进章节那一组（不再是通用「会话」桶，「全书修复」这个阶段名仍只留给书级 `book_fix`），标题是「修复 · <章>」。章节树与历史一行未改：**挂到哪一行仍只由锚点决定**，`chapterId` 只作身份与阶段；网页侧补了单测钉住"身份与锚点指向不同处时锚点说了算"，历史面板也不再拿修复行去和同章的转换/核对配对（它是修复，不是这两个角色中的任何一个）。

② **整章重开的预算收口（此前一直留着的那条）**：新助手 `fixBudgetExhausted`——修复打开、且某个子章节的修复转录里已经攒满 `latex.fix.max_rounds` 条 `fix_target` 时为真。命中时父章节核对再不过也**不再**归档这一版、换全新会话把整章连同所有子章节从头重跑（同一段很贵的对话只会被重放一遍），而是保留这一版整合产物继续往上走——未解决的问题本来就走 issues 库与报告向上传递，一个都不会丢——并记一行「修复轮数已用完，不再整章重开」的警告。开关关着、或没有任何子章节进过修复路径时判据为空，作废 + 重开与之前一字不差。`docs/config.md` 的 `latex.fix.max_rounds` 一行按新口径改写。

测试：`internal/sessionview` 两条（`fix_chapter_004_child_02` 的 ID/父章/角色/尝试号、阶段落在 `convert` 组、标题与书级 `book_fix` 不混）+ 既有 `fix_target` 用例改成新口径；`internal/latex/rescue_fix_budget_test.go` 四条（判据本体表驱动、预算用光不重开且警告与产物都在、预算还在照旧重开并归档、开关关着照旧重开且没有任何修复日志），后三条用真实 `convertOneChapter` 递归路径 + 假客户端跑。**还留着**：手动把 `latex.fix.enabled` 打开跑一本真书的观察（工具面与提示词追加在真实数据上够不够用）。

**收尾（2026-10-23）已完成**：P59 第二条口径（「并发优先级 = 创建顺序」）真正落到转换槽位池上——`latex.convert_concurrency` 的槽位不再是 channel 先到先得，而是按任务**创建时**发的号排队、释放时把槽位直接交给队里创建最早的那个（同号按到达顺序），创建戳在章节/子章节被创建的那一行取、重试沿用原号；父节点等自己的子树时已经还掉槽位，所以被阻塞的节点不在队列里、不占队头。核对池与其它阶段的池子不动。
**第五步（2026-10-23）已完成**：`dispatch_fix` 从"只记录计划"改成**同步执行并回执结局**——按计划逐个子章节走与 runner 相同的一条路径（`runChildFixAt`，与 `runChildFix` 同一份校验/预算/工具集/挂载/重核），回执按子章节给 fixed / 核对结论第一行（截断 160 字符）/ 已用与剩余轮数，并在父会话转录追加一行 `t=meta kind=fix_result`；计划执行后标 `applied`，`applyChildFixPlan` 读到已消费的计划就空转，未消费时仍按原路径兜底且执行后同样标记（同一份计划不会执行两次）。同步执行的前提是并发放手：转换槽由调用方让给这一轮修复（`holdSlot`），模型并发槽由新增的 `Session.PauseModelSlot()` 临时让出、执行完取回。同一批还收紧了两处误判——下派提示词写明 `figures/`、`images/` 是项目级资源、只有 `_subdivision/…` 与 `node/result/…` 算断链（关闭开关时提示词仍逐字节不变）；章节树的修复行只认 `node.fixes`（判据收进 `chapterNodeHasRows`，与核对行、子章节互不依赖），行 title 写出修复结局。测试：`rescue_fix_sync_test.go` 三条 + `TestPauseModelSlotFreesNestedSession` + `TestConvertFixDispatchScopesSelfContainedness`，预览页三条纯函数用例。
**第六步（2026-10-23）已完成**：两处收口——① 章节树里的修复行与核对行**同层**：一次修复的复核写的是 `checker_<child>.tryN.jsonl` 并被并进本节点自己那条核对行（同一个锚点），没有"修复的核对行"这一层，所以修复行是转换行的直接子行、左边距与核对行同值（行高仍 `var(--row-h)`，行序核对在前、修复按旧 → 新，没有修复时布局逐字节不变）；② `latex.fix.enabled` 打开时追加的下派说明多一句「回执已逐个子章节给出结局，回复里不要复述或改写它，直接接着整合」，关闭时提示词仍逐字节不变（`TestConvertSystemPromptDispatchOnlyWhenEnabled` 钉住这句只在打开时出现、且落实在同一行）。
**P60 默认值（用户口径 2026-10-22 晚）**：`latex.fix.enabled` 默认 **true**、`max_rounds` 默认 **5**（写 0/负数也按 5，不是无限轮）。显式 `false` 才退回“整章重转”的老路径，那时工具面、提示词与请求数与引入 P60 之前逐字节一致（有对照测试）。
**补记（2026-10-23）**：① **`dispatch_fix` 的回执即终态**——同步跑完修复的 `FIX RESULT` 回执标 `ToolResult.Terminal`，复查轮在回执落盘后就结束，父会话不再为"回应回执"多发一次模型请求（整章流程的复查请求数 2 → 1）；"只记计划"那条回执**不**标终态（它出现在还没接上 Runner 的一轮，父会话可能还要接着说）。② **复盘行与核对行同层**——`rescue_` 转录按文件名拿到章节身份（`chapterRole: "rescue"`，ID/父章/深度与 `checker_` 同源，`.tryN` 是它复盘的那次转换的尝试号），章节树里是**直接子行**（同一处 26px 左边距、同一 `var(--row-h)`），标签「复盘 #N」+ 归档前缀「旧」，点行切会话、钉住时那一行切到它复盘的那次转换；它永不参选 convert/checker、也不进「历史」尝试链；没有复盘时布局逐字节不变。

### P62 仓库整理（审计与计划）

**编号说明**：用户口述的批次号是 P61，但 P61 已被复盘会话占用（`internal/latex/rescue_*_test.go`、`internal/sessionview/scan_rescue_identity_test.go` 与 `book.go` 里的 `P61/A–D` 注释），按编号递增规则顺延为 P62。

**口径**：只整理、不改行为——功能代码、提示词、配置默认值、测试期望一律不动。已跟踪文件的删除可由 `git log` 回滚，所以只删「零引用 + 从未被删过 + 有替代物」的；未跟踪文件的删除不可回滚，所以只删**可再生的构建产物**与**一次性脚本**，样本数据副本一律留在原地（只列清单）。

**本批已做**

- `web/src/legacy/dom.ts` —— 旧页命令式 DOM 小助手（`el()`/`clear()`）。P11/P5-R2 全组件化之后 `web/src` 的 import 图里没有任何边指向它（44 个模块、41 条被引用，另两个是入口 `main.ts` 与类型声明 `env.d.ts`）。删前删后各跑一次 `npm run build`，内嵌产物 `assets/dist/viewer.{js,css}` **逐字节相同**（同样 88 个模块）——"行为不变"的凭据是哈希没动，不是"看了一遍"。
- `powershell/organize_files.ps1` —— 被 `legacy/python/python/organize_files.py`（README 与 `docs/dev.md` 指向的归档实现）同一套三步流程取代的早期版本，且内含一处本地绝对路径（本仓库不允许）。该目录随之变空而消失；`RULE.md` 推送清理示例里那句 `rm -rf … powershell` 现在走它自己的"文件夹不存在，跳过"分支。
- 上面两个路径的证据相同：`git log --diff-filter=D -- <路径>` 为空（从未被删过），`git grep` 按 basename 与全路径两种写法都是零命中。
- 未跟踪的一次性脚本与临时态（都在 `temp/`）：11 个按 P 号命名的源码改写/探针脚本（`p30.py`～`p138.py`）、一个内联正则探针（`recheck.mjs`）、一份文件名里带本地样本名的重置脚本、一份已被 `build.sh` 取代的部署脚本（`deploy.py`）、一份 9.8 MB 的单条转录拷贝（`session.jsonl`）、两份一次性配置（`models_only.yaml`/`t37cfg.yaml`），以及七个探针/评审残留目录（浏览器探针截图、两张对照图、一个一次性 Go 探针、一个旧二进制副本、一组 e2e 配置与日志、一组 133 MB 的图形对比指标、一个空目录）。
  - **更正（2026-10-24）**：上面那组「133 MB 的图形对比指标」属于误删——它不是再生得了的探针残留，而是用户自己的一次**图像对比实验**：3914 个文件、约 133 MB，顶层有对比脚本（`compare.py`/`compare_vnext.py`/`calibrate_metrics.py`）、指标汇总 CSV 与折线图，另有若干按次分目录的会话转录（`.jsonl`）。当时的判据是"tracked 文本零引用 = 残留"，但 `temp/` 整体在 `.gitignore` 里，删掉**不可恢复**（无 git 历史、回收站为空、`/home` 是 ext4 无快照）。教训写成硬规矩置于 `AGENTS.md` 的「未跟踪数据不许按"零引用"删」：含 `.jsonl`／`.csv`／结果图／实验脚本的未跟踪目录一律当数据，只删可再生的构建产物，真要清先逐项列清单确认。
- 可再生的构建产物：`go/` 下的 Go 构建缓存（1.7 GB，`.gitignore` 已列）、`make build`/`make release` 的输出目录（156 MB；CI 用 `make release` 现场生成，见 `.github/workflows/release.yml`）、`go/temp` 的 e2e 临时目录、两个平台二进制（`go build ./...` 即回）。连同上面的脚本合计约 **2.1 GB**。
- 空目录 `release/`（从未被任何脚本写入；发布产物在 `go/release/`）。

**留待后续（本批只看不动）**

- **样本数据副本约 8.3 GB**（MinerU 解析结果、切分后的 PDF、历年输出与对照数据，全在 `.gitignore` 里）：逐图模型请求换来的，删了不可再生，所以只列清单——删除还是迁出仓库目录由人工定。
- 两个**有文档引用**的探针目录按"被 tracked 文本引用就保留"的规矩留着：浏览器两页对照探针（3.9 GB，`AGENTS.md` 的验收口径写到了它）、重合度实验（5.9 MB，`docs/issue.md` 引）。
- **测试文件重排**：202 个 `_test.go`（1.27 MB，16 个包）+ 20 个 spec（连 `setup.ts` 21 个文件、133 KB）本批**只出表、不搬文件**。可归的三类：① 以 `tNNN_`/`pNN_` 开头、绑定单个 issue 的 30 个（`latex/` 17、`sessionview/` 8、`session/` 3、`config/` 2），其中复盘与修复两条线占 14 个（`p58_*`/`p59_*`/`p60_*`/`p61_*`/`rescue_triggers`）；② `latex/motif_*` 12 个（dedupe 一整套，约占该包测试体积的三成）；③ 两个只有辅助函数、没有任何 `Test` 的文件（`latex/pngtest_helper_test.go`、`split/helpers_test.go`）——Go 的约定本来就该这么放，不算问题。同包内辅助函数重名 0 处（真重名编译就过不去），测试名撞 `RetiredNames` 那 11 个退役名也是 0 处。 **第二轮（2026-10-23）已落地**：`internal/latex` 26 个文件按六组前缀 `git mv`（`chapter_` 7：coverage/gate/issues/size/checker/retry/slots；`rescue_` 9：p58×3、p60×4、p61×2；`tools_` 2：vfs/sandbox；`style_` 4：stylefix×3 + pilot；`env_` 4：docindex×2 + pythonenv×2；`motif_*` 12 个本来就在组里），`internal/sessionview` 8 条扫描器用例改 `scan_*`——共 34 个重命名；文件名不再带 `pNN`/`tNNN`（号留在**文件正文**注释里），文档与注释里 29 处旧路径（文档 26 + 注释 3）同步改。latex 其余 42 个与六组无关的文件（书级装配/编译/成本/交付/提示词/进度/看图/分块/项目/嵌入…）与 sessionview 其余 19 个非扫描器用例一个没动（`t201_child_retry` 是重试接口；`label_t62`/`live_t74`/`number_t77` 的号是后缀且与扫描无关；`review_fixes_test.go` 内容属工具与挂载）。
- **仓库外夹具**：`internal/config/runconfig_rewrite_test.go` 读 `temp/` 下两份配置改写稿，检出里缺文件时自动 `t.Skipf`。**第二轮（2026-10-23）已落地**：两份副本收进 `internal/config/testdata/`（`temp/` 原件保留）、测试改读 `testdata/`、`flatten` 的 `t.Skipf` 与三处 `os.Stat` 守卫删掉（夹具总在，四条用例现在**总是**跑）；夹具里的本地网关域名换 `api.example.com`、头部注明样例项目 `example_book`、凭据保持占位符；`.gitignore` 的 `test*/` 会连 `testdata/` 一起忽略，补了 `!go/internal/config/testdata/`。
- **章节树用例拆分（P62 第二轮已落地）**：`web/tests/chapter-tree.spec.ts`（1002 行 / 79 条）拆成 `chapter-tree.spec.ts` 38 条（树本身的规则：展开记忆、默认取最新一代、只钉住当前行、标题与状态文案、世代与归档）+ `chapter-chain.spec.ts` 9 条（尝试链、角色分离、同号去重、老转录兜底）+ `chapter-fix-rescue.spec.ts` 32 条（修复行、复盘行、标签与结局），夹具构造器抽到 `web/tests/helpers/chapter.ts`；断言与用例标题零改动（拆分前后 `expect(`/`it(` 行集合 298 行逐条相同），`npx vitest run` 仍是 253 条。
- `DocVision.code-workspace`（43 B，单文件夹 `.`）零引用、从未删过，属于"删了也不影响任何东西"的候选，本批不动。
- 文档体积（本批不重写）：最大三节是 `docs/config.md` 的「配置项参考」（144 行）、`docs/commands.md` 的「会话预览」（84 行）、本文件 P60 一节（64 行）。10 份 `docs/*.md` 全部可达——`README.md` 直连 8 份，`dedupe.md` 由 `dedupe-algorithm.md` 连，`agent-batch-history.md`/`issue.md`/`plan.md` 由 `AGENTS.md` 与 `README.md` 连。

**验证**：`GOCACHE=/var/tmp/dsh-gocache go test ./internal/... -count=1` 全绿（14 个包 ok，`splitlog` 无测试文件）、`go build ./...` 通过、`npx vitest run` 20 个文件 250 条全过、`npm run build` 后 dist 零差异。

**第二轮验证（2026-10-23）**：`go test ./internal/... -count=1` 14 个包全绿（`internal/latex`、`internal/sessionview`、`internal/config` 都含改名后的文件）、`go vet ./internal/latex/ ./internal/sessionview/` 干净、`npx vitest run` **22 个文件 253 条**全过（拆出两份新 spec，原文件 79 → 38 条）、`npx tsc --noEmit` 仍只有 `data.ts`/`legacy/trajectory.ts` 两条既有报错；生产代码与 `dist` 未动（除夹具路径常量）。

### P63 切回第 N 次尝试（已实现）

**用户口径（2026-10-25）**："做一下切回"，并在中途追加一条："还有注意切换的时候暂停当前会话"。

**问题**：换尝试一直是单向的。作废的那一次整棵被搬进 `work/attempts/<章>/try<N>/`，新尝试从空工作区重开，旧的只能翻不能回去。页面上一排「第 1/2/3 次」点了只是切视图，流水线用的还是最新那一次——用户遇到的正是最典型的后悔：一次新尝试跑到一半发现"上一版其实是对的"，或者核对会话自己跑坏了、想退回它还没插手那一刻接着干。

**改法**：把它做成一次对称的搬运，`go/internal/latex/restore.go`。现役这一次（第 L 次）整棵搬进 `work/attempts/<章>/tryL/`，`work/attempts/<章>/tryN/` 整棵搬回工作区；搬的清单与 `archiveChapterAttempt` 同一张（节点工作区的三份 + 提交产物 + 分片目录 + 报告），所以"切回再切回来"是幂等的，不会长出第二套布局。转录也跟着搬：`lastAttemptNumber` 是按 `work/sessions` 里最大的 `.tryN` 号认"现在这一次"的，只搬文件树不搬转录，切回来以后各条路径会继续往那个更大的号上写，页面上的「最新」也对不上。

**不做的事**：不重跑任何会话（切回只是把文件摆回原处，要不要接着干由用户按「继续跑 / 重新核对」决定，自动开一条核对会立刻烧掉一轮 token，而用户常常只是想先看一眼那一版）；不动核对转录（结论记在它自己那条转录的 meta 里，那是历史事实）；不覆盖任何东西（目标已占就拒，冲突的那一份加 `parked-` 前缀，同名转录不搬、原地留着）。

**"暂停"是怎么落地的**：这套流水线里没有可叫停的入口——`lockChapter` 只是内存里的章节锁，没有取消通道，也没有 PID 文件。所以切回不打断任何会话：请求照常排队，等这一章的会话把手上这一轮走完、松开章节锁，整棵现役尝试才被搬走；那一次是完整留档的，之后在历史里还能切回来接着跑。守卫是"最近 2 分钟内还有没有写入"（这一章与它整棵细分树里的转换/核对转录、以及节点工作区里除 `media/views/pdfview/build` 之外的条目），有写入就不搬——半个工作区被搬走时子会话还在里面写，是这个功能唯一能把数据搞坏的方式。日志与页面文案都照实说这一点，不假装"已经停下了"。

**接口**：`/api/retry` 增加角色 `restore` 与字段 `attempt`。子章节一律拒（它的尝试号是父尝试之内的号，单独搬一支会把两代子树拼在一起），要切就切父章节。前端入口在「历史」页签的每一行上：「切回这次」，只给顶层章节的转换会话、且尝试号不等于当前这一次（切回去之后，被搬走的那一次号比"现在"大，所以同样有这个按钮，能切回来）。判定收在 `web/src/legacy/actions.ts` 的 `restoreTargetOf` 里，页面只负责确认条与发请求。

**验证**：`go/internal/latex/restore_test.go` 覆盖双向对调、往返幂等、写入中拒切、四类非法目标，`go/internal/sessionview/restore_retry_test.go` 钉住接口的四个拒绝分支与回执文案；`web/tests/session-actions.spec.ts` 钉住按钮给谁不给谁。全量 `go test ./internal/...`、`npx vitest run`、`go vet`、`npm run build` 全绿。

### P64 图片阶段的可核对浏览界面（未做）

[~][ ] P64. 图片阶段（process / dedupe）现在只有数字与一堆平铺的图，核对时找不到"到底哪些图还在等转换"、也看不到单张图的判定依据。要做成一个可以逐个核对的界面，需求逐条如下。

**进度行要显示全量**：现在跑一次就只显示这一次的计数（实测出现 `[process 1/1] 100.00% (done: 0, errors: 0, fallback: 1, ...)`），上一轮已经 done 的一概不显示。要改成"整本书的进度 + 本轮增量"两层：总数、已完成（含历史）、本轮新增、失败、fallback 都列出来，让人一眼知道还差多少、这一轮干了什么。

**`fallback` 要能点开看是谁**：现在只有一个数字。要能点开列出这些图（文件名/哈希前缀 + 落到哪一类 + 用了哪个备选模型/为什么退回），否则这个数字毫无意义。

**要有"待转换原图集合"这一层**：用户反复找不到"最终还在等转换的那批原图"的目录视图。需要按"这一批是要送去转换的集合"分组呈现——分组即最终决策（新画 / 复用代表图 / 保留原图 / 转文本），而不是几百张图片平铺。

**单图核对页（核心）**：点任意一张图，能看到
- 原图本身（可缩放/看印刷尺寸）；
- 它在 `progress_items` 里的那条 JSON 原文（当时怎么判的：class、status、equivalent_to、dedupe_leader_hash、shared_tex 等字段）；
- 转换结果（TikZ/SVG 成品、编译后的 PDF/PNG 预览）；
- 对应的 LaTeX 代码（可复制）；
- 它属于哪个分组、代表图是谁、组内其余成员；
- 是否已完成、走的是哪条会话（点得进那条转录）。

**figure 目录要能看到原图**：现在 figure 目录里没有原图，核对时对不上"这张图长什么样"。

**分组视角**：复查界面应当既有"全部图"也有"按组"两种视图；按组时一组一行（代表图缩略图 + 成员数 + 决策），展开看成员。

**做法（草案）**：数据侧已有 `progress_items` 与 dedupe 结果，新增只读接口 `/api/images`（分组 + 逐图字段）与 `/api/image/<id>`（单图详情 + 会话定位）；前端在图片阶段下加一个「核对」页签，复用现有缩略图与大图组件；分组与逐图的判定字段全部直出磁盘事实，不重新推理。进度行改成两层计数，fallback 列表来自同一次判定的记录。

**未定**：单图核对页是否要允许"就地改判"（把这张图从"转文本"改成"重画"并只重跑这一张）——这属于能改数据的操作，需要单独确认；本计划先只做只读核对。

**进展（P64 第二刀，已完成）**：复查页（`source/dedupe/review.html`，侧栏「整组核验」阶段行的按钮打开同一份页面）接上了 `progress_items`——每张缩略图下面有状态词（已完成／退回原图／失败／已分类待处理／未处理，只认 `status`）、类别、「判定记录」链接（指到 `progress_items/<md>/<img>.json`）与转换产物链接（PDF/PNG/SVG），真画过的图还有「LaTeX 源」（`source/tikz/<名>.tex`）与「会话」（`source/sessions/vector_<名>.jsonl`）两个入口——文件名都能从 `figures/<名>.pdf` 推出来，没画过的图不伪造这些入口；另加两段：「等待转换的原图」（未收尾的，点缩略图看原图）与「退回原图的（已收尾但没画出成品）」。进度行也改成双层：`process`/`dedupe` 尾部报全书盘点（画好／退回原图／失败／未处理），`process` 收尾把退回原图的图名逐个列出，续跑不再只剩 1/1。`fallback` 的退回原因也落到 `progress_items` 的新字段 `fallback_reason`（工具链不可用 / TikZ 没通过校验 / 接口会话错误各一句），复查页在「退回原图的」那段逐张写出。**进展（P64 第四刀，已完成）**：核对页不再只是快照——会话预览页的 `/dedupe?project=…` 现在**按磁盘现状现渲染**（`internal/reviewpage` 是共用渲染器：样式/灯箱/卡片/分段一份，`internal/sessionview/review_live.go` 读 `groups.json` + `progress_items` + `figures/` + `tikz/` + `sessions/`），每张图**左原图、右成品**并排，图注直出类别、状态、代表图跟随关系与退回原因，成品与链接只在文件真存在时出现；磁盘上的 `source/dedupe/review.html` 保留为没有 `groups.json` 时的退路。状态词表也搬到 `internal/reviewpage`（`StatusSettled`/`StatusQuality`/`StatusLabel`），latex 的 `image_status.go` 改为转发，页面与进度行不会再各说各话。仍未做：图片阶段自己的页签（现在走 dedupe 复查页）、"就地改判并只重跑这一张"（单图重画本身已有 `role=vector` 队列，缺的只是核对页上的入口，按你的决定不做）。

**进展（P64 第一刀，已完成）**：完成口径先收成一处——`go/internal/latex/image_status.go` 的 `imageStatusSettled`/`imageStatusQuality`，判定**只看 `progress_items` 的 `status`**（用户口径：raster 那类没真正处理也算完成）。词表：`classified` 未完事、`done` 真做完、`fallback` 退回原图但也算收尾、其余（`error`/空）未完事；另给界面一档 `quality`（ok/degraded/pending），让"退回原图"能和"真画出来"分开显示。用例 `image_status_test.go` 钉住六种输入。后续进度行、分组视图、单图页一律调这两个函数，不再各自判断。

### P65 矢量图 compare 辅助工具（已实现）

矢量图作图会话新增纯 Go `compare_figure` 工具。它只在模型已经成功编译后读取原图和当前 PDF 预览，返回软墨迹重合信息、对齐位移和 overlay 图像，供模型与 `view_pdf`、`view_image` 一起判断；工具本身不修改会话提交状态，也不改变原有提交流程。实现、提示词、用户帮助和合成回归测试已随 `a6c27fb` 提交；聚焦测试通过，全量测试唯一失败项为既有沙箱 Python 环境用例。

## 第六十七批（2026-10-28）：T268——矢量图分组的"最新一次"认反了，方块视图把旧次藏掉

用户原话："可以问题是最新的不是 try2 吧，我这边用的 clean_vector 脚本清理的，这边最新的应该是 try。并且这边显示的也不全面，还不如之前的不加这个角标全部显示的，至少还是正常的现在都不正常了。"

事实链：`clean_vector.sh` 归档时把 `vector_<图>.jsonl` 改名成 `.tryN.jsonl`（N 递增），新一次跑出来的是无后缀的同名文件——所以**现役没有 tryN 后缀**。T266 的排序写成「尝试号大的在前」（`nb - na`），无后缀被当成"第 1 次"排到了归档的 try2/try3 后面，`main` 取排头，于是点开同号方块/分组父行打开的是**归档的旧次**；用户"新跑的我怎么看"就是这么来的。另外方块视图按"一图一块"渲染，`shownGroups` 只出前 8 张图且方块视图里没有溢出按钮，旧次与新图都看不到——用户说"显示的也不全面"。

改法（`web/src/legacy/sidebar.ts` + `web/src/components/Sidebar.vue`）：
- `imageAttemptNo`：无后缀 = 现役（记 0），`.tryN` = 归档代次；新增 `imageAttemptCurrent`。
- 组内排序：现役永远最前，归档的按 N 从大到小（mtime 兜底）；`main` = 现役。列表视图子行标签：现役「当前」，归档「tryN」。
- 方块视图（T268）：回到**一条转录一块**的全量显示——同一张图的各次尝试贴在一起（现役在前），被取代的旧次整块调暗（`.session-block.old`）并挂「旧」角标（`.block-old`），悬浮说明写"旧的一次尝试（已被更新的取代）"。
- 补上方块视图缺失的溢出按钮（分组态「更多图片（还有 N 张）」、平铺态「更多会话（还有 N 个）」）；单阶段分支的链式 `v-else-if` 包成 `<template v-else-if="blockView">` 才能塞进按钮不破坏条件链。

验证：`t266-image-attempts.spec.ts` 6 条（现役=0、平铺顺序）；`TestViewerVectorAttemptGroupsInChromium`（子行 「当前,try3,try2」、方块 4 块、暗标 2、角标 2）；web 全量 287 条、sessionview 全量绿。CHANGELOG 的 Fixed 条目单独提交（文件里还有用户进行中的 compare_figure 草稿，不能混进来）。

## 第六十八批（2026-11-01）：T269——复盘切口不再切开「最后一个完整工具轮」

用户原话："复盘的切口误伤可以补一下"（另一条"拿不到工具清单"是他用的中转站问题，不修，换模型即可）。

现场：一章的细分父节点在复盘判定的退化点之前刚调用过 `request_chapter_split`，复盘给的切口行正好落在那条 assistant 消息上。`balancedCutFrom` 只保证"保留的前缀里工具调用都有回执"，于是这一轮（调用 + 下一行的回执）被整段裁掉——平衡是对的，可续跑回放的历史里"我申请过子章节"这个事实没了，页面也不再显示它拆过章；事主只能从 `work/rescue/cuts/<章>/` 的备份里手工补回两行。

改法：新增 `extendPastToolRound(lines, keep)`，在 `trimTranscriptInPlace` 的平衡检查之后、写盘之前调用——切口那条是 `t=msg` 的工具调用、且紧跟的行（允许 meta/usage 夹层）里把它的全部回执都拿到了，就把切口后移到回执之后；中途遇到下一条 assistant 消息或类型不认识的行就原样不动。丢掉的那一段仍然进 `work/rescue/cuts/`，`dropped` 计数按后移后的切口算。

验证：`internal/latex/t269_cut_keeps_tool_round_test.go` 两条（边界：带 meta 夹层的后移、缺回执不动、非调用不动、双调用只回一条不动；集成：切口落在 `request_chapter_split` 上时保留调用与回执、`dropped` = 退化段行数）。

## 第六十九批（2026-11-01）：T270——Anthropic 线路补 SSE 流式 + 新版 Claude 的 thinking/effort 适配

用户两个问题：① "那个 message 就是 anthropic 的那个格式是不是还不支持 SSE 流式输出?"；② "用这个 claude 的模型的话这边用现在的配置报错：`status_code=400, claude-haiku-5-5 requires adaptive thinking; omit thinking or use ***.type=adaptive and output_config.effort`"。

**根因**：`client.go` 里 anthropic 分支写死 `payload.Stream = false`（P4 当时只做了非流式），所以思考型模型整段静默、`StreamHook` 实时快照在这条线路上根本不会触发；`buildAnthropicBody` 只透传 `thinking`，而 `reasoning_effort` 是 OpenAI 形状的顶层字段，Anthropic 要的是顶层 `output_config.effort`——两个条件凑不齐，新版 Claude 对每个请求都回 400。

**改法**：新增 `internal/session/anthropic_stream.go`。`collectAnthropicStream` 按 Anthropic 的事件模型拼消息（`message_start` 取 input/cache 两个基数、`content_block_start` 认 text/thinking/tool_use、`content_block_delta` 分 text_delta/thinking_delta/input_json_delta、`message_delta` 取 stop_reason 与 output_tokens、`message_stop` 收尾），坏事件跳过不毁整条流，idle 看门狗与 OpenAI 那条流一致（同一份 `api_stream_idle_timeout` 语义），快照节流同为 150ms、只回传尾巴 16KB。`ChatCompletion` 的 anthropic 分支按 `payload.Stream` 走流式，`Content-Type` 不是 `event-stream` 或 4xx 命中"不支持流式"时退回 `decodeAnthropicResponse`。`buildAnthropicBody` 加 `stream:true`，并把 `c.reasoningEffort` 并进顶层 `output_config.effort`（`withOutputEffort`：已有 `output_config` 是对象就保留其它键）。

**验证**：`anthropic_stream_test.go` 三例——事件流全通道（思考/文本/工具分片/usage 105+42/缓存 80/hook 有回调）、网关忽略 `stream:true` 回整段 JSON 仍能解析、`thinking: {type: adaptive}` + `reasoning_effort: high` → 请求体带 `output_config.effort=high` 且不含 `reasoning_effort`、`request_body` 里 `output_config.format` 未被覆盖；`TestAnthropicEndToEnd` 显式钉 `stream: false` 继续覆盖非流式翻译；`./internal/session`、`./internal/config` 绿（latex 两条既有失败照旧：`compare_figure` 门禁那个测试、宿主 python）。

**文档**：`docs/config.md`（`models.text.api`、`thinking`、`reasoning_effort` 三行）、`docs/sessions.md`（模型注册表一行）、`go/internal/config/default.yaml` 与 `config.example.yaml` 的 `api:`/`reasoning_effort:` 注释同步；AGENTS.md 无对应描述，未改。

## 第七十批（2026-11-01）：T271——配置模板补一段能照抄的 Anthropic（Claude）示例

用户问："这个 anthropic 的那个标准的 yaml 中有没有展示这个 anthropic 的 api 怎么配置的？"——答：没有。两份模板里 `api:` 那行只有键注释（`openai（默认）| anthropic | responses`），`gateway-b` 是 OpenAI 兼容网关的例子，谁要换 Claude 都得自己把 `api`/`base_url`（要指到 `/v1`）/`thinking: {type: adaptive}`/effort 四件事拼起来，漏一件就是 T270 那个 400。

**改法**：`go/internal/config/default.yaml` 与 `config.example.yaml` 的 models 段（`gateway-b` 之后、`heavy` 之前）各插一段**注释掉**的 `claude:` 示例——`api: anthropic`、`base_url: .../v1`、`max_tokens: 8192`、`api_timeout: 900`（思考型模型单轮久）、`thinking: {type: adaptive}`、`reasoning_effort: high`（注释里写明它翻译成顶层 `output_config.effort`、老式 `{type: enabled, budget_tokens: N}` 会 400）。注释掉而非放开：模板要能被直接当配置用，一条带占位 key 的活条目只会添乱。

`docs/config.md` 在 `## extends：命名基座` 前新增「线路协议示例：Anthropic（Claude）」一节：最小可用 YAML + 三条易错点（adaptive 与 effort 必须同时在场、不要用 `budget_tokens`、`extends` OpenAI 基座时 `base_url`/`api_key`/`api` 三项都要覆盖）。

**验证**：`TestConfigTemplateKeyParity` 通过（注释不参与键面）；`./internal/config` 全绿；`diff` 两份模板的 models 段为空。无代码改动，`config_version` 不动（没有新增键）。

## 第七十一批（2026-11-01）：T272——`sessions --serve` 启动慢：索引缓存根本没接在这条路上

用户原话："现在启动这个 … `./docvision sessions --serve` … 还是巨慢"。这条在三批前就被问过一次（T253 加了磁盘索引缓存），所以先查"缓存到底有没有被用上"。

**根因**：`.sessionview-cache.json` 只被 `sessionview.Scan` 读写，而 `Scan` 的调用点只有静态导出（`static.go`）与费用报告（`latex/cost_report.go`）；`sessions` 命令本身（`--list`／`--cost`／`--serve`）走的是 `ScanWith`，那条路**从不 load 也从不 save**。于是一本 9.2 GB、6459 条转录的项目每次启动都把每个文件从头读一遍，缓存文件躺在扫描根下（7.5 MB、10 月 27 日写的）却完全没被打开；`--serve` 更冤：它启动时先 `ScanWith` 扫一遍（结果只用于 `--list`/`--cost`，`--serve` 分支直接丢弃），随后预览服务的长驻 scanner 又从头扫一遍——最贵的活干了两遍。

**改法**：① `ScanWith` 与 `Scan` 同源——`loadCountCache` + `saveCountCacheIfDirty`；② 新增 `saveCountCacheIfDirty`：`fileStat` 的缓存未命中路径置 `dirty`，只有真的重算过才落盘，并节流到 30 秒一次（预览页每 2 秒轮询一次，全部命中缓存时不该重写 7 MB 索引）；`saveCountCache` 保持原样，T253/T255 的测试还直接调它；③ `newViewerServer` 建 scanner 时就 `loadCountCache`，`/api/index` 与 `/api/session` 改走新的 `scanAll()`（扫完按需回写）；④ `cli_sessions.go`：`--serve` 且没要 `--list`/`--cost` 时不再预扫。

**实测**（同一项目，仓库内构建的探针二进制，`sessions --list` 重定向到 /dev/null）：冷扫（先挪走缓存）**173.360 秒**；第二次命中缓存 **0.770 秒**。改动前 `--serve` 约等于冷扫 ×2。`--serve` 单独跑一次（临时端口、只读）：banner 立即上屏，第一个 `/api/index` **1.388 秒**、第二个 **0.548 秒**。

**验证**：`internal/sessionview/t272_scan_cache_test.go` 两例——`ScanWith` 第二遍用"同长度、同 mtime 的垃圾内容"替换转录后消息数仍为 2（证明没重读）；`newViewerServer` 的首轮 `scanAll()` 同样命中缓存，且全部命中时缓存文件 mtime 不变（证明没白写）。`./internal/sessionview`、`./cmd/docvision` 全绿。

**文档**：`docs/commands.md` 会话预览一节补一段（缓存文件位置、失效判据、首次扫描要几分钟、删了会重算、轮询不写盘）；AGENTS.md 无对应描述，未改。

## 第七十二批（2026-11-01）：T273——工具名兼容（大小写归一 + 模型级 tool_aliases + 自解释的 unknown 回执）

用户看 haiku 跑的某个子章节会话："这个会话感觉他调用的工具存在一些问题？你看 … 下面的，有很大的问题，我的工具是否应该兼容一下大小写不敏感？作为一个设置项？在这个作为某个模型下面启用？不然他用不了这个"。

**现场**：转录里的调用名是另一套 agent 的工具面——`Bash {"command":"ls -la /work /project /input"}`、`Read {"file_path":"/input/chapter.md"}`、`Grep {"pattern":".","path":"project:"}`、`ToolSearch {"query":"select:Read,Write,Bash"}`、`edit_file`。本会话声明的是 `read_file`/`write_file`/`bash`/`grep`/`compile`/`view_pdf`/`submit`…，`executeTool` 又是严格字符串相等，于是每个调用都拿到一句 `TOOL ERROR: unknown tool Bash`，模型换个名字再猜，几轮之后整章卡死。注意 `Grep` 的 `path` 写的是 `project:`——说明我们的挂载名它知道，只是工具名来自别处（网关/中转站把另一套 harness 的工具清单塞进了请求）。

**改法**：`executeTool` 改成先 `resolveTool`：
1. 精确名（含 `mcp__x__read_file`、`tools/read_file` 取尾段的候选）；
2. `s.modelCfg.ToolAliases`（新配置键 `models.<条目>.tool_aliases`，`{Read: read_file}`）：键与值都过 `normalizedToolName`（小写、去掉 `_- . 空格`），值必须真在本会话的工具里，指向不存在的工具就继续往下找（不静默吞调用）；
3. 归一化后的**唯一**匹配（撞两个就不猜）。
找不到时的回执升级为 `TOOL ERROR: unknown tool X（想找的可能是 read_file）；本会话可用的工具：bash, compile, …`——相似建议用归一化前缀包含或 Levenshtein ≤2，宁缺勿滥（`ToolSearch` 不会瞎建议）。

配置侧：`ModelConfig.ToolAliases` + `ResolveModel` 的整块零值继承（继承的是拷贝，改条目不会改到 `models.text`）、`extends` 走既有的 map 深合并；`CurrentConfigVersion` 24→25，两份模板与 README、`docs/config.md` 同步；模板的 claude 示例里给出 `Read: read_file` / `Edit: write_file` 两行注释。

**验证**：`internal/session/t273_tool_alias_test.go`——11 条名字解析用例（精确/`Bash`/全大写 `SUBMIT`/`read-file`/`read file`/`mcp__docvision__grep`/别名键大小写/别名值大小写/别名指向不存在工具/`ToolSearch`）、有歧义不猜（`read_file` 与 `readfile2` 并存时精确名仍命中）、回执含全部工具名与 `Read → read_file` 建议且无相似时不建议、`BASH` 真能执行；`internal/config/t273_tool_aliases_test.go`——继承整张表 + 拷贝语义。`./internal/session`、`./internal/config` 全绿。

**文档**：`docs/config.md` 新增 `models.text.tool_aliases` 行与 `config_version` 25；`docs/sessions.md` 模型注册表字段表补 `tool_aliases`；AGENTS.md 会话基础设施条与 `docs/agent-notes/session-infra.md` 同步一句现状；两份模板补注释示例。

## 第七十三批（2026-11-01）：T274——`grep` 工具按 Go 正则跑，路径不存在不再冒充 NO MATCHES

用户："我们的 grep 工具是不是有问题，你看转换 · `chapter_002_child_06` 里面他用 grep 不行用 bash 里面的 grep 就好了？"

**现场**（同一份子章节转录）：`grep {"path":"project:converted","pattern":"QuestionHeading|begin\\{question\\}|chapterintro"}` → `NO MATCHES`；`grep {"path":"gaoshuqiyi.cls","pattern":"newcounter\\{question|newenvironment\\{question|workedexample|chapterintro"}` → `grep error: grep: 不匹配的 {`；`grep {"pattern":"。|，|；：", ...}` → `NO MATCHES`。三条都不是"文件里没有"，而是模式语义错位——`grep(1)` 默认 BRE：`|` 是字面量（交替失效，静默零命中）、`\{` 是区间表达式的开头（GNU grep 直接报"不匹配的 {"）。模型随后改用 `bash` 里的 `grep -E` 就正常了，说明它没写错，是工具没按自己宣传的口径跑。

**改法**：`runGrep` 不再 `exec grep -rIn`，改成进程内 `regexp.Compile`：先按 **Go 正则**（工具描述与提示词一直写作 "Go regexp"），编译不过（例如模型照 ERE 写的 `\{`、未闭合括号）退回**纯文本** `strings.Contains`，两种都不报语法错；自己 `filepath.WalkDir` 遍历（跳过 `.git`/`.trash`/`node_modules`、含 NUL 的二进制、>8 MB 的文件），输出仍是 `path:line:text`（行号 1 起），命中数按 `max_matches` 截断并给 `...(N more, 收窄 pattern)`（总数真数，不留"只数到 limit"的假数字）。顺带修 `path` 不存在时被 `continue` 掉、最后回 `NO MATCHES` 的老毛病：单目标时回 `grep error: 路径不存在：project:converted（converted 的同级目录下有：chapter.md, chapters/, style/ …）`，模型一眼能改成真路径。工具描述补一句"编译不过按纯文本匹配"。

**验证**：`internal/latex/t274_grep_test.go`——`question|workedexample` 命中两行（BRE 会零命中）、`。|，|；|：` 命中中文标点行、`\{question\}` 照常命中、未闭合括号退回纯文本、行号 `:3:` 对齐、无匹配返回空串；`path` 不存在时报 `grep error` + `路径不存在` + 列出 `chapter.md`/`chapters/` 且不含 `NO MATCHES`；二进制跳过、20 行命中按 5 截断出 `...(15 more, 收窄 pattern)`。`go test ./internal/...` 全绿（除既有的 `compare_figure` 门禁测试与宿主 libpython 两条）。

**文档**：`docs/sessions.md` 的挂载表一节补 grep 语义（Go 正则 + 纯文本兜底 + 路径不存在报错 + 二进制/8 MB 跳过）；`docs/issue.md` T274；`CHANGELOG.md` Unreleased/Fixed。

## 第七十四批（2026-11-01）：T275——`clean_convert.sh` 补上后来新增的转换伴生状态

用户："我现在想清空一下这个 convert 的过程，我换个模型重新跑一下，之前的那个 clean_convert 还能用吗"。

**审计**：脚本（T167）能被 `bash -n` 过、`-n` 也能跑，但删除清单是手写枚举，**新目录一个都没跟上**：`work/attempts/<章>/try<N>/`（P63 作废尝试留档）、`work/rounds/<章>/{repo.git,journal-tryN.jsonl}`（逐章快照仓库，实测 88 MB）、`work/rescue/{cuts/,log.jsonl}`（复盘切口存档，T241）、`work/fix/<章>/plan.json`、`work/retry/*.done.json`（章节级重试队列，68 条 done + 1 条**未完成**请求）全是转换阶段的伴生状态；转录侧只清 `convert_*`/`checker_*`，`rescue_*.jsonl`（实测 100+ 条）与改名后的 `stylefix*.jsonl` 都不清，`.partial` 快照（`*.jsonl.partial`）也不在 glob 里。清完再跑会留下"页面有尝试/复盘条目、转录和产物已经没了"的错位。

**改法**：`to_delete_convert` 增加 `work/attempts`、`work/rounds`、`work/rescue`、`work/fix`、`work/retry`（`--keep-convert` 时整体保留）；`rescue_*.jsonl` 并进转换分支；三类转换转录与 `style_fix_*`/`stylefix*` 的 glob 末尾都加 `*`，把 `.partial` sidecar 一起带走；头部注释与 `docs/latex.md`「清理与重跑（辅助脚本）」同步。

**验证**：`bash -n` 通过；真实项目 `-n`：新增的 5 个目录与 `rescue_*`/`checker_*`/`convert_*` 共 171 条转录都在计划里（`work/rounds` 88 MB、`work/temp` 162 MB 一并列出），备份路径与保留清单正确；两个假项目各跑一次真删除——默认口径下转换/复盘/修复/快照/队列目录与三类转录（含 `.partial`）全清，`chapters/*.md`、`work/sessions/{chapters,keep}.jsonl`、`work/media`、`work/sessions/media`、`style/`、`doc_index/`、`progress.json` 全留，`convert`/`assemble` 归空；`--keep-convert` 下 `work/chapters`、三类转录、attempts/rounds/rescue/fix/retry 全留，`views`/`pdfview`/`stylefix*`/`book_fix`/`final_review` 照删、`progress.json` 同样归空。

## 第七十五批（2026-11-01）：T276——核对侧复盘截断不再搬走转换产物

用户："我这个 chapter 10 和 11 不知道咋回事？为什么它已经 submit 了然后还是继续了……并且它的 checker 有一次都通过了，这是咋回事他还在继续？"

**现场**（两章都是**细分父章节**：子章节各自过核对后，父会话先「复查子章节」、再「整合并 submit」）：
- `work/sessions/convert_chapter_010.jsonl` 里同一份转录出现 4 轮 `复查 → 整合 → submit`（`SUBMITTED` 在 261、280…），中间夹着 `chapter_activity=pass`（263）与 `no_verdict`（264/265）；`convert_chapter_011.try2.jsonl` 同款（`pass` 在 313、`no_verdict` 在 314）。
- 两章的 `work/chapters/` 里**只有 `.tex.checker`**（内容：「checker 会话已退化，已排队从干净轮次继续核对」），没有 `.tex`——产物不在书里。
- 运行日志时间线：`16:38 chapter_010 内容覆盖率 99.1%`（整合成功）；`17:29:43 [rescue] chapter_010 复盘结论 truncate（from=114）` → 同一秒 `[convert] chapter_010 已提交的产物挪进 work/attempts/chapter_010/try1/`；`17:48` 又是一次（from=56，核对第 3 次尝试）；`18:37:14 [checker] chapter_010 通过`；之后又被重转覆盖。`work/attempts/chapter_{010,011}/try1/` 与 `chapter_012/try1/` 就是被搬走的产物。

**根因**：`rescueQueueTruncate` 的 `role == "checker"` 分支照抄了转换分支——截断核对转录之后仍调 `applyWorkspaceCut`（回退章节工作树 + `archiveChapterProducts` 把 `work/chapters/<章>.tex`、分片目录、报告挪进 `work/attempts/<章>/try<核对尝试号>/`）。核对会话根本没有可写工作区，它的退化只是"这次没给判定"，产物是**转换会话**交的、并且已经过了覆盖率与编译门禁。产物一没，这一章下一轮就"没有产物 → 整章重转"（细分父章节更贵：子章节复用、父章节重走复查+整合+submit），再交给同一个退化的核对会话，再退化、再搬走——环。附带两个污染：归档目录用的是核对转录的尝试号（`try1` = 核对第 1 次，与转换的尝试号串味，切回列表里多出假的转换尝试）；每次重转又把上一轮的 `pass` 标记与 `.passed` 覆盖掉，于是"checker 明明通过过却还在继续"。

**改法**：
1. `rescue.go` 核对分支：只 `cutTranscriptNow` + 排 `role=checker` 的重核请求，**不碰文件**（注释写清为什么）。
2. `retry.go`：`applyWorkspaceCutTo` 与 `archiveChapterProducts` 都加"源转录必须是 `convert_*`"的守卫——非转换转录只回退工作区、不归档，并写一行 `[rescue] 只回退工作区，不动已提交的产物（源转录不是转换会话）`。

**验证**：新增 `internal/latex/t276_checker_cut_test.go`——核对侧截断一个真项目（`.tex` + 分片 + `.checker` + 8 条消息的核对转录）之后：`.tex` 与 `part_01.tex` 仍在、没有创建 `work/attempts/`、队列里恰好一条 `role=checker` 请求；`archiveChapterProducts` 传 `checker_*` 时拒绝（文件还在），传 `convert_*` 时照旧落到 `work/attempts/<章>/try1/<章>.tex`。`go test ./internal/...` 全绿（除既有的 `compare_figure` 门禁与宿主 libpython 两条）。

**文档**：`docs/issue.md` T276；`CHANGELOG.md` Unreleased/Fixed；AGENTS.md 会话基础设施条里"核对会话仍换尝试"那句补上核对侧现状。

## 第七十六批（2026-11-01）：T277——核对会话要能看到产物页

用户："我不确定之前那个 checker 是否是正确的检查和合并了……我看这两个的核对都出现了同一个问题就是 view pdf 的时候看不到"，贴的回执是 `view_pdf check:chapter_010.pdf` → `TOOL ERROR: 文件不存在: check:chapter_010.pdf（该目录可用: chapter_010, chapter_010.md, chapter_010.tex, children, figures, images, parts）`。

**现场**（`work/views/checker_chapter_010_try3` 的目录清单）：
- `chapter_010.tex`、`chapter_010/`、`children/`、`parts/` 全是**软链**，目标指向 `work/temp/conv_chapter_010/integrated-stage/…`——细分父章节的暂存提交根，下一轮一开始就被 `RemoveAll`。视图留着，链全断：`read_file check:chapter_010.tex` 与 `view_pdf check:chapter_010.pdf` 都只会回"文件不存在"，而回执里列出来的名字仍然在（readdir 看得见断链），模型只能反复重试。
- 视图里**没有** `*.pdf`：重核路径 `recheckChapter` 的 `hints.PDF` 来自 `chapterCompiledPDF` → 读上一轮转换工作区的 `.gate/compile.json`；那份是**上一轮会话**编的，产物换过（重整合/切回/手工恢复）就过期，工作区被清掉就彻底没有。
- 提示词还在骗人：`checkerClaimNote` 只要 `report.md`/`notes.md` 在就写 "CLAIMS to verify against the page and this PDF"，于是模型自己去找 `check:<章>.pdf`。截图里那段 `≈32 tokens → ≈61 tokens · error` 的重试就是这么来的。

**改法**：
1. `checker.go` 新增 `linkCopy`，`ensureCheckerViewAt` 里**产物一侧一律复制**（`<章>.tex`、parts 条目、`<章>/`、`parts/` 两份别名），只有 `chapters/<章>.md` 与项目级 `source/figures|images` 继续挂软链（它们稳定）。一次视图就是一次快照，暂存目录之后怎么清都不影响。
2. `retry.go` 新增 `compileChapterForCheck`：用 `chapterScratch` 在 scratch 里按**当前已提交产物**编 wrapper（类文件 + 产物 + 项目 images/figures），PDF 拷到 `work/views/pdf/<章>.pdf` 再挂进视图；`recheckChapter` 先走它，失败才退回旧 gate 的 PDF 并记一行警告。
3. `checkerClaimNote(view, base)`：只有 `view/<章>.pdf` 真的在才提 "this PDF"；没有就只说 "verify against the page"。
4. 没有产物页的那一轮，提示词补一句"本轮没有 check:<章>.pdf 可开，按 markdown/.tex/parts 与原书页判断"，堵住"照着直觉去找 PDF"。
5. 重核通过时补写 `<章>.tex.passed`（与主路径同一口径 `writePassMarker(productFingerprint(...))`）。

**验证**：`t277_checker_view_test.go`——建视图后把 `integrated-stage` 整个删掉，`<章>.tex`、`<章>/children/c1.tex`、`parts/children/c1.tex` 仍读得到；`checkerClaimNote` 无 PDF 时不提 "this PDF"、有 PDF 时提；没有产物时 `compileChapterForCheck` 如实报错。`TestCheckerViewMirrorsChapterLayout`、`TestCheckerViewRecursiveChildHasPartsAliases`、`TestCheckerClaimNoteNamesExistingOnly` 同步更新调用签名后仍绿；`go test ./internal/...` 除既有的 `compare_figure` 门禁与宿主 libpython 两条外全绿。

**给用户的现场处置**（同一批，数据侧只加不删）：把 `work/attempts/chapter_010/try1/`、`chapter_011/try1/` 里那两份 18:37 / 18:50 **通过核对**的产物复制回 `work/chapters/`（`.tex` + 分片目录 + report），独立编译验证 20 页 / 25 页、CJK 三字组覆盖率 96.2% / 96.3%、`\input` 目标齐全、无临时路径引用；`.tex.checker` 保留"没结论"文案，下一轮只会**重核**这两份、不再整章重转。注意不要用 `--only`（`forceChapterRetry` 会把产物归档、强制重转）。

## 第七十七批（2026-11-01）：T278——重核不要撞回旧的退化会话

用户重跑后：`[checker:chapter_010] 轮次 24 · 工具调用 47 · 已用 18m23s`、`[checker:chapter_011] 轮次 30 · 工具调用 49 · 已用 21m18s`，接着 `[rescue] chapter_011 已原地截断到第 122 条记录（第 5 次尝试），排队继续`，最后 `phase convert FAILED: chapter_011 重新核对未通过（这份产物未被采纳）: checker 会话已退化，已排队从干净轮次继续核对`。用户问："这个是不会自动重试了，这个退出是你新加的吗？"

**结论**：T277 起作用了（`work/views/pdf/chapter_010.pdf` 21:17、`chapter_011.pdf` 21:44 都是按当前产物新编的产物页，视图里不再有断链；chapter_010 已经 `重新核对通过` 并写了 `.passed` 21:39）。坏在**重核挑了一条旧会话**：

- `recheckChapter` 的尝试号来自内存计数器 `retryCheckerAttempt[base]`，进程重启后是 0 → `n = max(2, 0) = 2` → 转录名 `checker_<章>.try2.jsonl`；
- 而 `try2` 在磁盘上**已经存在**（上一轮退化打满提醒的那条，89 条历史、`submit` 0 次）→ `checkChapterAt` 见到文件就走"恢复中断的会话"，把那 89 条原样回放 → 第一轮再次退化 → `checker 会话已退化，已排队从干净轮次继续核对`；
- 这条结论落到 T222 的入口守卫上就是硬失败（"没判定不能静默采纳"），于是整轮 convert 判失败、进程退出。硬失败不是这两批新加的，但"核对模型退化"被当成了"这一章不合格"是错的。

**改法**：
1. `retry.go` 新增 `freshCheckerAttemptNo(proj, base)`：`max(磁盘上已有最大尝试号 + 1, 本次运行已抬号 + 1, 2)`，并 `bumpCheckerAttemptNo`，保证下一条转录**磁盘上还不存在**。
2. `recheckChapterAt(proj, base, tid, forceAttempt)`：第一轮在 `forceAttempt >= 2` 时用它（"从第 N 条记录继续核对"已经把前缀抄成那条尝试，必须接着跑），否则用新开号；没给结论就自动换一条全新会话再核，**上限 2 次**（一轮核对十几分钟，三条都没结论就是核对模型的问题）。
3. `book.go` 入口守卫：重核仍无结论时保留产物、`.checker` 写明原因、日志说清"下一轮会再重核"，**返回 nil 不算转换失败**（真正的内容问题照旧 `return fmt.Errorf`）。

**验证**：`t278_recheck_fresh_test.go`——磁盘已有 `checker_<章>.jsonl`、`.try2`、`.try5` 时，`freshCheckerAttemptNo` 返回 6（且该文件不存在），再调返回 7；空项目返回 2。`go test ./internal/...` 除既有的 `compare_figure` 门禁与宿主 libpython 两条外全绿。

**现场**：chapter_010 已通过（`.tex.passed` 21:39）；chapter_011 的产物（1014B 主文件 + 分片目录，独立编译 25 页、覆盖 96.3%）仍在 `work/chapters/`，`.tex.checker` 是"没结论"文案——下一轮重核会从 `try6` 开一条干净会话接着核。

## 第七十八批（2026-11-01）：T279——重入风格修复别再白烧一轮整合会话

用户在 `--step convert` 重跑后贴的控制台：`[convert] 12/12 100.00% (done: 12, errors: 0, running: 0, 0.7s)` → `[style-fix] 工作汇报 12 份，其中 3 份报告 cls/手册问题` → `[stylefix-integrate] 运行 · 轮次 1 · 工具调用 2 · 已用 18.0s`，并说"继续跑的时候会重新跑这个 integrate 的会话可以稍微调整一下……他会重新跑有点奇怪"。

**现场**：12 章产物都在，convert 阶段 0.7 秒全跳过；风格修复的入口 `styleFeedbackLoop` 拿到 3 份"存在问题"的报告后，无条件 `store.Problems = nil` 再跑整合会话 —— 上一轮（22:28）已经整合好的 `work/style_fix/problems.json` 被整份丢掉，于是又跑一遍同样的整合，转录还是 append 到同一个 `stylefix_integrate.jsonl`，页面上那条会话就显得"又从轮次 1 开始、工具调用归零"。

**改法**：
1. `ProblemStore` 增加 `ReportsSHA`，`problems.json` 里多一个 `reports_sha` 字段（`loadProblemStore`/`saveLocked` 双向带上）。
2. `styleFixReportsSHA(issues)`：把"有问题"的报告按名字排序后连内容做 sha256。
3. `reuseStyleProblems(store, sha)`：指纹一致且清单非空才沿用；沿用**只清 `Checked`**，返回副本（清空后的状态写回存储，下一轮按"全部未解决"派活）。
4. 入口分支：沿用 → 日志"工作汇报与上次一致（sha 前 8 位）——沿用上次整合的清单，不重跑整合会话"；不沿用 → 若之前有指纹就先记一行"工作汇报有变化（旧→新）"，再重置清单、跑整合；**指纹只在整合成功后才写**，失败不留"可用清单"的假象。

**验证**：`t279_stylefix_reuse_test.go`——同一批报告换 map 顺序指纹相同；报告改一个字、增删一份都会让指纹变化；指纹一致才沿用、且只清勾选；返回的是副本（改它不动存储）；空清单不继承指纹。`go test ./internal/...` 除既有的 `compare_figure` 门禁与宿主 libpython 两条外全绿。

## 第七十九批（2026-11-02）：T280——风格修复会话要能真的续上

用户："这个样式修复的会话我重新跑不能续上吗？…我重新跑之后他好像是在原来的会话继续还是什么？因为之前有两个问题已经解决了"

**现场**（`--step convert` 重入 + 当天 12:49 再跑一次之后）：
- `work/sessions/stylefix_style_r1.jsonl`：95 条消息，`run_end` 只有一次（11-01 23:44），之后 12:49 又追加了两轮 `problems` 调用就断了——**没有 `submit_style`**；
- `work/sessions/stylefix_integrate.jsonl`：41 条、`run_end` 三次（22:40 / 23:01 / 23:20）——整合会话被重跑了三遍（T279 之后不再重跑）；
- `work/style_fix/problems.json`：3 项，**0 项打勾**，`reports_sha=f46938b8…`；
- `work/style_session.jsonl`：mtime 仍是 11-01 02:56（样式阶段那次），风格修复的对话一个字都没进去。

**根因**：
1. `styleFixStyleSession` 每次都 `loadSessionContext(style_session.jsonl)` 重开，本轮自己的转录从不被读回——打断之后，上一轮"改了 cls 的哪三处、P1/P3 已经好了"这段上下文凭空消失；
2. `saveSessionContext` 在会话已经挂了转录时是 no-op（它只服务没有转录的老路径），所以风格修复的对话也进不了 `style_session.jsonl`；
3. 页面之所以看着"在原来的会话继续"，是因为转录是 append-only 的同一条文件——两轮的内容写在一起，模型那边其实是重开的；
4. 勾选状态：只有**分块修复会话** `submit` 前会 `problems check`。这几次一条块会话都没起过（没有 `stylefix_assess_*`/`stylefix-blk-*` 转录），所以"两个问题已解决"只是对话里的说法，存储里就是 0/3。

**改法**：`styleRoundSeed(proj, round)` 三级取上下文——① 本轮自己的转录（打断续跑）② 最近一轮的转录（新一轮接着上一轮记忆）③ 样式阶段 `style_session.jsonl`（第一次进循环）；取到转录时 `SeedCounters` 续上轮次/工具数、日志写明从哪条转录继续、重入时反馈里加一句 `Continue this round from where it stopped`。取不到任何上下文时如实报错，不拿空上下文硬跑。

**验证**：`t280_style_round_resume_test.go`（第 1 轮接自己那条；第 2 轮先接 r1、有 r2 优先 r2；无上下文返回空）。`go test ./internal/...` 除既有的 `compare_figure` 门禁与宿主 libpython 两条外全绿。

**给用户的说明**：现在重跑会真的接着那一轮说话（转录里 95 条历史 + 续上的轮次/工具数）；勾选仍从 0 开始（沿用清单时按 T279 清掉打勾），由分块修复会话重新核对后打勾——也就是说"两个已解决"要等这一轮核对过才成为记录。

## 第八十批（2026-11-02）：T281——多子层级章节的风格修复分派

用户："在当前多个子层级的这种章节转换下，样式修复中后期的分派那边是否处理的合理？要精确到什么程度？我现在正在跑了分派评估"

**现场**（12 章产物，其中 3 章是细分父章节）：
- `work/chapters/chapter_010.tex` 只有 6 行 `\input{chapter_010/children/chapter_010_child_0N.tex}`，正文全在 `chapter_010/children/**`（6 个 `.tex`，每个还带自己的 `parts` 目录）；另一章 8 个、再一章 22 个 `.tex` 都在 `<章>/` 层；
- `styleFixAssess` 的 `chapterNames` 来自 `filepath.Glob(work/chapters/*.tex)` —— **只有 12 个顶层名**，userText 写着 "Their .tex files are readable at project:work/chapters/<name>.tex"，系统提示词同样；
- `styleFixBlockSession` 的 userText 写 "Your chapters (in work:chapters/): chapter_010"（工作区里其实已经拷了 `<章>/` 整棵，含 children）；
- `BlocksTool`/`SubmitBlocksTool` 不校验章节名，`chapter_010_child_03` 这种子节名能写进块里 → 分块复制时 `work/chapters/chapter_010_child_03.tex` 不存在、静默跳过。

**改法**：
1. `subdividedChapters(proj, names)`：有 `<章>/children/` 目录的就是细分父章节。
2. 评估 userText 追加一段：点名这些章节、说明"顶层 `.tex` 只是 `\input`，正文在 `work/chapters/<章>/children/**`，判它没受影响之前先 grep 那个目录"；系统提示词的 "readable at …<name>.tex" 补成 "…<name>.tex 与其整个 `<name>/` 目录"。
3. 分块 userText 追加 `subdividedBlockNote`：块里有细分章节时列出 `chapters/<章>/children/`；系统提示词同步一句。
4. `BlocksTool`/`SubmitBlocksTool` 都加 `Known`（顶层章节名）校验：未知名拒绝并列出可用名，附一句"子节不单独分块，按父章节名写"。

**边界**：分块单位仍是**顶层章节**——产物拷回 `<章>.tex` + `<章>/`、`<章>_wrapper.tex` 编译都以它为单位；子层级只是这一章的文件布局，不额外拆块。精度要求落在"具体文件 + 具体模式"：块会话必须能指出改的是 `chapters/<章>/children/<子节>.tex` 的哪一处，块本身保持"一次会话能改完的一手章节（≤3–4 章）"。

**验证**：`t281_stylefix_dispatch_test.go`（子节名被拒且给出可用名、父章节名通过、改 blocks.json 再提交同样被拒；`subdividedChapters` 只认带 `children/` 的、无细分章节时不加那句）。`go test ./internal/...` 除既有的 `compare_figure` 门禁与宿主 libpython 两条外全绿。

## 第八十一批（2026-11-02）：T282——分派评估会话加 bash、重跑从干净状态开始

用户："分派评估这边的一些会话应该增加一个 bash 的工具吧？不然很多操作和子文件夹不好看。还有我怎么让这个 [stylefix-assess-r1] 运行 · 轮次 25 · 工具调用 40 · 已用 38m34s^C 重新跑。她用的好像是旧的会话内容是不是我删掉这个 jsonl 就行了"

**现场**：评估会话工具表 = `read_file`/`grep`/`write_file`/`edit_file`/`problems`/`goal`/`blocks`/`submit_blocks`，**没有 bash、没有 ls**；转录 `work/sessions/stylefix_assess_r1.jsonl` 472 KB，最后一步是 13:33 的请求（无 `run_end`），评估工作区里留着 13:14 的 `plan.md`（goal 计划）。

**两件事分开看**：
1. **看不到目录结构**：12 章里 7 章是细分父章节，正文在 `<章>/children/**`，而评估会话只能 grep——它当时是靠反复试正则（`\\\\begin\{statement\}` 之类全部 NO MATCHES）才在 17 轮左右撞到 `chapter_004/children`。给它 bash 是最直接的解法。
2. **"像在旧会话里继续"**：评估会话**从不回放**转录（没有 `SetMessages`），所以旧的 jsonl 对模型没有影响；真正会漏进新一轮的是 `work/stylefix_assess/plan.md`（`NewGoalTool` 会读回）与 `blocks.json`（`submit_blocks` 会读回）。另外转录是 append-only 同一条，页面看到的就是"那条跑了 38 分钟的会话"。

**改法**：
- 评估会话、分块会话都加 `WorkBashTool`（`/project`、`/work` 与 file 工具同一套前缀，`TmpDir` 用 `sessionBashTemp`，沙箱开关跟着 `latex` 配置）；
- `archiveAssessWorkspace`：每轮开始把上一轮的 `plan.md`/`blocks.json` 挪进 `work/stylefix_assess/archive/<时间戳>`（不删），日志一行「评估工作区已重置（… 挪进 archive/）」；
- `nextAssessTranscript`：每轮新开一条转录 `stylefix_assess_r1.jsonl` → `.2.jsonl` → `.3.jsonl`，旧记录留档；
- 两份提示词各补一句 `bash`（`ls`/`find` 看目录布局），`MustMention` 加 `bash` 钉住。

**给用户的重跑食谱**（不用手删 jsonl）：`mv work/sessions/stylefix_assess_r1.jsonl* work/stylefix_assess/` 都可省了——直接重跑，代码会自己归档；真要让页面也干净，把旧转录挪出 `work/sessions/` 即可。

**补记（同日，用户指出不一致 → T283）**：这里原本写的是"上一轮收尾过就另开一条转录、把 plan.md/blocks.json 归档"，用户立刻指出这与前面刚做的"被打断的会话都能续上"自相矛盾："这里面的所有会话应该都是能续上的吧，至于重新跑只能我通过外部去删除，因为这次只是暂时性的删除过去的文件产生的会话不应该算进去"。因此改成**一律能续**：`transcriptResumeState(dir,name,terminalTool)` 只要磁盘上有转录就 `SetMessages` + `SeedCounters` 接着跑；`run_end` 在末尾或末尾交过本会话终态工具只用来标 `finished`（续跑但提醒"旧产物还在，别当新结论交"）；整合、评估、分块三处全部接入，块会话再用 `tempDirKeep` 复用被打断时的工作区（不再被 `tempDir` 重建清掉已改章节）。归档/轮转两个函数与"每轮新开转录"的做法一并删除——**要真正从头再来只能由用户在外面删转录与目录**。上面那段"重跑食谱"随之作废。

**验证**：`t282_assess_bash_test.go`（旧 plan/blocks 被挪走、新一轮 goal 为空、旧文件在 archive/、空目录重跑不报错；转录 `.jsonl`→`.2.jsonl`→`.3.jsonl`）；`./internal/...` 除既有 `compare_figure` 门禁与宿主 libpython 两条外全绿。

## 第八十二批（2026-11-02）：T284——跑完的那一轮不许再被续上

用户："我那个 stylefix 都结束了为什么又给他续上了 … 会话都跑完了。你看看怎么处理一下这后面他新增加的几条消息"

**现场**：`work/sessions/stylefix_style_r1.jsonl` 里 13:06:12 有一条 `submit_style` 与 `run_end`——那一轮**已经交过结论**；但 14:08:53 又被追加了一条 `"Continue this round from where it stopped (it was interrupted before submit_style)"` 加两轮工具调用，控制台显示「轮次 52 · 工具调用 66 · 已用 19.0s」。

**根因**：T280 的 `styleRoundSeed` 只看"转录文件在不在"——跑完的一轮和被打断的一轮在磁盘上长得一模一样，于是 `--step convert` 重入时把交过结论的那一轮当成被打断续了上去，模型重新读一遍清单、重新打勾。

**改法**：
- `transcriptSubmitted(path, terminalTool)`：这条转录里有没有调过本会话的终态工具（`submit_style`/`submit_problems`/`submit_blocks`/`submit`）；
- `sessionAttemptPath(dir, base, terminalTool)`：`<base>.jsonl` 没交过 → 续它；交过 → 试 `<base>.2.jsonl`、`.3.jsonl`……（与转换/核对的尝试号同一条思路）；旧文件原样留档，不覆盖不删；
- 风格修复轮、整合、评估、分块四处统一走它；块会话续跑复用工作区（`tempDirKeep`）、新一次尝试重新铺（`tempDir`）；
- `resumeNote` 去掉"已收尾"分支，日志只报"接着上一次没交 X 的尝试继续（N 条历史）"；
- `styleRoundSeed` 删除（它那套"有就接、还往上一轮串"正是这次事故的来源）。

**数据处置**（用户要求处理那几条多出来的消息）：先把整条转录备份到 `work/sessions/archive/stylefix_style_r1.full-20261102-1409.jsonl`，再把正文截到 176 行（最后一条是 13:06:12 的 `run_end`），14:08–14:09 那几条伪续跑记录移出正文。截断只针对这一个文件、且留了完整备份。

**验证**：`TestSessionAttemptPathResumesOnlyUnsubmitted`（没交过就续同一条）、`TestSessionAttemptPathSkipsFinishedAttempt`（交过 → `.2`；`.2` 没交 → 续 `.2`；别的会话终态工具不算）、`TestTranscriptSubmittedReadsToolCalls`、`TestTempDirKeepReusesInterruptedWorkspace`；`./internal/...` 除既有两条外全绿。

## 第八十三批（2026-11-02）：T285——交过的阶段就该往下走

用户："这个 style r1 他不是已经提交过了吗？为什么还是重新开了一个会话？样式修复 · style_r1.2 交过不应该是下一个阶段了吗？"

**现场**：`--step convert` 重入，控制台「第 1/3 轮风格修复」后起来的是 `stylefix-style-r1`（新会话，轮次 2 · 工具调用 2 · 33 秒后被打断）——T284 把"已提交的一轮"换成了 `.2` 转录，但它仍然是**从头再跑一遍**。

**根因（比 T284 更深一层）**：`styleFeedbackLoop` 每轮三步（风格会话 → 评估 → 分块）无条件全跑；磁盘上只有转录、没有"这一轮这一步已经交付"的产物，所以"跑完"和"被打断"在循环眼里没有区别。

**改法**：
- 轮级产物：`work/style_fix/blocks_r<N>.json`（评估交回来的分块）、`work/style_fix/results_r<N>.json`（每块的 `resolved`/`notes`），两者都带工作汇报指纹 `sha`（`styleFixReportsSHA`），报告一变旧产物自动作废；
- `styleRoundDone`：这一轮的转录（含 `.2`、`.3`）里有 `submit_style` → 跳过风格会话（cls/manual 在提交时就写进 `style/` 了）；
- `loadRoundBlocks` 命中 → 沿用分块，不重跑评估会话；
- `loadBlockResults` 命中的块 → 直接收账写报告（章节副本在提交时就拷回 `work/chapters/` 了），只跑没交过的块；
- 于是重入后的路径是"评估（若没交）→ 分块（只跑缺的）→ 下一轮"，而不是把已完成的一步重做一遍。

**验证**：`TestStyleRoundDoneReadsTranscripts`、`TestRoundBlocksAndResultsReuseBySHA`（同指纹沿用 / 异指纹作废 / 追加写不覆盖已有块）；`./internal/...` 除既有两条外全绿。

## 第八十四批（2026-11-02）：T286——评估会话的视野收窄、路径说清

用户："你检查一下这个 bash 路径不清楚提示词有没有问题"＋"我不清楚你给的这个工作区是否东西实在是太广泛了？太多了？…毕竟这个是分派评估"

**现场**（真实转录）：评估会话先 `ls -R /work/chapters` → `没有那个文件或目录`，再 `ls -la /work; ls -la /project` 才发现：`/work` 是自己的草稿区（当时还是空的）、`/project` 是**整个项目根**（`build/ chapters/ doc_index/ out/ progress.json source/` 全在里面）。浪费两轮，而且它刚看到的 `chapters/` 其实是 Markdown 源，不是要评估的 `.tex`。

**根因**：别的会话都走 `sessionMounts` → `projectRoot()` 的**窄只读视图**（`projectViewEntries`：`source`/`style`/`chapters`/`converted`/`reports`，明确不含 `work/sessions`、`work/attempts`、`work/temp`、`build/`、`out/`），只有评估会话直接 `{Name:"project", Dir: proj}`。再叠上首条消息里"正文在 `work/chapters/<name>/children/**`"这个不带上缀的写法，和它自己的 `/work` 挂载撞名。

**改法**：
- 评估会话 `mounts` 改为 `{project: r.projectRoot()}` + 自己的 `work` 草稿区（后者是唯一可写挂载，sandbox 里只读挂载真的只读）；
- 提示词与首条消息统一 `project:converted/<name>.tex`、`project:converted/<name>/children/**`，并写明 bash 里 `/project` 是视图、`/project/chapters/*.md` 只是 Markdown 源、`/project/style`·`/project/reports` 也是只读、`/work` 只是草稿区（`plan.md`/`blocks.json`）；
- `bash` 工具描述里那个写死的 `work:chapters/a.tex = /work/chapters/a.tex` 例子换成与挂载表无关的说法（每个会话的挂载本来就不同，写死一个会把模型引到不存在的路径）；
- 提示词按用户要求收敛：改完 1361 字节，比改动前（1410）还短，规则 2/4/5 顺带压掉冗余措辞。

**验证**：`t286_prompt_paths_test.go`——`TestAssessPromptNamesBothMounts`（必须点到 `project:converted`、`/project/style`、`/work is only your scratch area`，且不得再出现 `work/chapters`）、`TestSubdividedBlockNoteNamesWorkspace`；`./internal/...` 除既有两条外全绿。

## 第八十五批（2026-11-02）：T287——中文项目里的「重画这张图」

用户："我想重绘制一下这个某两个图，后续可能会用到，这边怎么操作？"＋页面回执 "失败：只能重画某条作图会话（source/sessions/vector_*.jsonl）"

**现场**：点的是「矢量图」里的作图会话（一条 60 条消息、53.4 KB 的 `vector_*.jsonl`），服务端却回"只能重画某条作图会话"。同一项目里有 33 条这样的转录、35 条进度记录全是 `done`。

**根因**：`internal/sessionview/retry.go` 的 `transcriptNameRe = ^[A-Za-z0-9_.-]+\.jsonl$`。作图转录的名字是 `vector_<项目名>__<哈希>__<描述>.jsonl`，中文项目名一出现就不匹配 → 进了"不是转录文件"分支。**同一个正则**还用在 `transcriptUnderWork`（页面的「删除记录」/「从某条记录之后继续」），所以中文项目里那两处也一起坏——只是不点就看不见。

**改法**：`validTranscriptName`——只要求"一个 `*.jsonl` 文件名"：`filepath.Base(name)==name`、不含 `/` `\`、不以点开头、不含控制字符；中文、空格、括号照收。三处调用点统一换掉。

**验证**：`t287_transcript_name_test.go`——`TestValidTranscriptNameAllowsUnicode`（中文/带哈希名字收，`../escape.jsonl`、`a/b.jsonl`、`.hidden.jsonl`、`notes.txt` 仍拒）、`TestPostRetryVectorRoleAcceptsChineseNames`（中文项目 + 中文转录名 → 200，队列里 `role=vector` 且 `Request.Path` 是原名）；`./internal/sessionview/` 17.4s 全绿。

## 第八十六批（2026-11-04）：T288——会话断了就得停

用户："前面样式的整个流程有个会话终止了就直接进入下一个阶段了…下次应该终止而不是继续"

**现场**（另一本书的真实运行）：`work/sessions/style_pilot_select.jsonl` 最后一行 `t=meta kind=run_end ok=false error="api error: [SESSION_API_ERROR: stream error: ... Upstream request failed, please try again later]"`（18:37:05）。日志只留一句「[pilot] 选材失败（跳过试点）」，随后 `progress.json` 里 `style/chapters` 都写成 done、转换开始派发；再往后转换会话集体撞 `SESSION_INSUFFICIENT_BALANCE`（一章 55 行跑了一阵、四章各 7 行当场死），用户只能把下游全删掉重跑。

**根因**：
- `stylePilotPhase` 把三步会话的错误全按「跳过试点」处理（`LogWarning` + `writeMarker`），没法区分"试点这一步内容上没做成"与"会话/接口本身断了"。后者意味着样式包根本没被修订，继续往下走等于拿没磨过的包划分+逐章转换。
- `convertChapterBatch` 只在批跑完后才由 `chapterFailure` 决定整轮失败；批内已经按并发宽度派出去的章节照跑，而且余额这类致命错误并不阻止后续派发——用户看到的就是"一章耗尽额度，后面几章各自开一条转录、写 7 行就死"。

**改法**：
- `stylePilotPhase` 返回 `error`；`pilotContentSkip(err)` 用 `isSessionAPIError`/`isSessionFatalError` 分流：接口/会话级 → `fmt.Errorf("样式试点…中止…: %w")` 中止整轮（不写 marker，下次重跑会重新试点）；内容级（选材不合法、样例拼不出来、未提交）→ 仍旧只警告跳过试点并写 marker。
- `convertChapterBatch` 加致命错误闩锁：goroutine 里遇到 `isSessionFatalError` 记下消息；派发循环在每章之前检查，命中就停止派发、把剩下的章节记成"没开始"；函数多返回一个 `fatal error`，`convertPhase` 两个调用点见到就直接 `return`，未开始的章节在日志里点名。

**验证**：`t288_pilot_abort_test.go`——`TestPilotPhaseAbortsOnSessionError`（假服务端回 500，试点必须返回错误且不写 marker）、`TestPilotPhaseSkipsOnContentFailure`（会话跑完没交选材 → 只跳过、写 marker）；`t244_restore_wait_test.go` 的调用点补第四返回值；`./internal/latex/` 除既有两条外全绿。

## 第八十七批（2026-11-04）：T289——文本图不进分组、图没跑完不进下一阶段

用户："dedupe阶段好像不太需要对text类型的图片进行分组吧？…如果分组处理最后下个阶段会不会导致本来text内容不同给弄到相同的内容了？"+ "为什么矢量图阶段没跑完就跑到下一阶段了？不会自动停下吗？"（附日志：`[process 7/66] 10.61% (done: 3, errors: 4, …)` 紧跟 `[book] === phase: images done (8m52s) ===`）

**① 文本图被分组**：`motifDedupePhase` 的候选收集只挡"没分类 / 已吸收"，`ClassText` 照样进池，于是两张文字不同、版式相近的文本图可能被并成一组；`processPhase` 对跟随者一律跳过（"materialized after leader"），而被跳过的文本图不会有 `Content`，`rebuildPhase` 只能退回原图——文字从书里消失，还白花一次 dedupe 会话。
改：`motifDedupeEligible(class)` 把 `text` 挡在池外并打一行「N 张文本图不参与分组」；`motifDedupeVersion` v4 → v5，让可能含文本成员的旧 `groups.json` 整体失效、重新核验一次（文本图逐张自己提取文字，代价很低）。

**② 图没画完就进下一阶段**：`processPhase` 是 void——逐图失败只累加 `errors/failed` 计数，`RunImages` 收尾只 `return r.rebuildPhase(...)`，于是"接口掐断、只跑了 7/66 张"也被 `runPhase` 记成 `images: done`，style/chapters/convert 全建在缺 59 张图的册子上（用户当场看出来了）。
改：`processPhase` 把中止原因（不可重试的 4xx / 连续接口错误且无一成功）当 error 返回；`RunImages` 在档位1 收尾再用 `unfinishedImageKeys` 核对一遍（`done`/`fallback`、跟随者、被吸收的跨页续片都不算），有没成品的图就报错并点名前 5 个——阶段不记 done，运行停在 images，重跑从这些图继续。

**收尾时的两处回调**（同批，改完重跑测试）：③ 单张图的"不可重试 4xx"以前当场掐断整批（T262），但在"3 张成功、4 张 400"这种现场里它更像是这几张图自己的问题——一撞就停会让一张永远画不出来的图把整本书永久卡住。改成只累加"连续错误"计数（成功一张清零），连续 3 张才判环境/配置问题；④ 于是"没成品"分成两类：`error`（跑了但请求被判不可重试）按原图交付、只在收尾提示，不挡后续阶段；只有 `pending`（压根没跑到）才让 images 阶段报错停下。

**验证**：`t289_images_incomplete_test.go`——`TestMotifDedupeSkipsTextImages`、`TestUnfinishedImageKeys`（error 不算没跑到、但计入没成品）；`./internal/...` 除既有两条（`compare_figure` 门禁、宿主 libpython）外全绿。

## 第八十八批（2026-11-04）：T290——会话视野里的 source 只剩书本身

用户："style里面的bash是不是能看的东西也有点多了？会干扰，那个project里面好像还有前面的会话什么的内容？"

**现场**：样式会话的 `project:` 挂的是 `work/views/project`，其中 `source` 是**整棵** `source` 的软链。而 `source` 同时装着两种东西：书（`*.md`、`images/`、`figures/`、`tikz/`）与这一程的流水账（`sessions/` 逐图作图转录、`dedupe/groups.json` + `similarity_advisory.json`、`progress_items/` 每图一份 JSON、`progress.json`）。`ls /project/source` 就能看到前面那些会话的转录文件名，展开更吵。

**改法**：视图里的 `source` 不再整棵软链，改成真的分栏目录——`*.md` 逐条软链，`images`/`figures`/`tikz` 三个"属于书"的产物目录软链，其余一律不铺（`projectSourceSubdirs`）。两个安全细节：① 旧版本留在这里的整棵 source 软链要先摘掉再建目录，否则"清理多余条目"会穿透进真 source；② 清理只删软链（`Lstat` 判 ModeSymlink），真文件/真目录一律不动。

**验证**：`t290_project_view_test.go`——`TestProjectViewHidesSourceBookkeeping`（造出 sessions/dedupe/progress_items，断言视图里看不到；再把整棵 source 软链塞回去，断言会被替换成分栏视图）；`TestProjectViewIsNarrow` 照旧；`./internal/latex/` 除既有两条外全绿。

## 第八十九批（2026-11-04）：T291——list_fonts 的系统字体一直是空的

用户："我们的系统没有字体吗？这边我看列出的时候虽然文件夹中没有但是系统字体呢？"（附 `list_fonts` 回执：项目目录只有 `README.md`，`System fonts (fc-list):` 后面空白）

**根因**（两个）：
- `Execute` 里先 `cmd.Run()` 判可用、再 `cmd.Output()` 取内容——同一个 `*exec.Cmd` 只能启动一次，第二次返回 `exec: already started`，`out` 恒为空串，于是标题打出来了、列表一行都没有。宿主机其实装了 2647 个字体文件、其中 90 条 CJK 记录（Noto Sans/Serif/Mono CJK SC、WenQuanYi、Droid Sans Fallback…）。
- 项目字体目录只按 `!e.IsDir()` 过滤，`README.md` 被当成"可用字体"列在 `\setCJKmainfont Path=` 该用的那一段里。

**改法**：`fontListText` 采现场（`projectFontFiles` + 一次 `fc-list :lang=zh family file`）交给纯函数 `formatFontReply` 组装；`fcBaseFamilies` 按家族名归并（去尾部字重词、去重、简体优先）、上限 40 行；`fc-list` 不可用时明说"没有系统 CJK 字体、必须 Path= 指项目目录"。回执末尾加一句事实：编译在宿主机跑、能看到这些家族，bash 沙箱里没有字体目录，别在里面 `fc-list`。

**验证**：`t291_list_fonts_test.go` 三条（README 不算字体；`Noto Sans CJK SC Bold` 归并、SC 排最前；fc-list 缺失如实说）+ 本机实测回执从空变 32 个家族。

## 第九十批（2026-11-04）：T292——原书 PDF 视图因为缺 `*_origin.pdf` 整个失效

用户："这次这个风格会话竟然没看原始pdf……都看不了了。"（附转录：`bash {command:"ls -la /source"}` → `ls: 无法访问 '/source': 没有那个文件或目录`，`doc_search` 却还在教它 `view_pdf {path:"source:<file>"}`）

**证据**：运行日志第一行 `[WARNING] [pdfview] 原书 PDF 视图不可用（source 挂载点关闭）: no origin pdfs for …`；该书的 `mineru_output/<subj>_part*/` 里只有 `full.md`/`images`/`layout.json`/`content_list*.json`，没有 `*_origin.pdf`（不是每次 MinerU 解析都会保留原 PDF）。

**根因**：`findOriginPDFs` 只 glob `<part>/*_origin.pdf`，取不到就返回空 → `buildPDFView` 失败 → `r.pdfView = nil` → `sessionMounts` 少一个 `source`、`list_source_pages` 不注册、`doc_search` 的回执照旧提示 `source:`；我们自己切的 part PDF 一直在 `paths.split_dir`，却没被当作兜底。

**改法**：`findOriginPDFs(mineruDir, splitDir, subject) []originPDF`——每个 part 没 origin 就用 `<splitDir>/<part 目录名>.pdf`；返回体带 `Part`（MinerU 目录名，`FileByPart`/`doc_search`/`list_source_pages` 全靠它对页），否则兜底 PDF 的父目录 "split_files" 会冒充 part 名；`buildPDFView`/`buildPageIndex`/`buildDocIndex` 增加 `splitDir` 参数，`viewName` 改为吃 part 名。`doc_search` 在没有视图时改说"本会话看不到原书页、别去开 source:"。

**验证**：真实数据 `buildPDFView` → `[27…__part1.pdf, 27…__part2.pdf]` 共 244 页（200+44），part 名与 `doc_index` 的 part 目录逐字一致，`LocatePart` 通；`t292_source_pdf_fallback_test.go` 三条；`./internal/...` 除既有两条外全绿。

**补记（T292）**：换模型后的首次实跑，原书视图正常（`[pdfview] 原书 PDF 视图就绪 … 244 页`，风格会话连续 `view_pdf source:<part>.pdf` 对照原页），但每次启动多两条 `[pdfview] 页数不一致: part split_files: no view file`——`alignmentProblems` 把 PDF 的父目录当 part 名，兜底 PDF 的父目录是 `split_files`。`pageSrc` 增 `part` 字段（`buildPageIndex`/`buildDocIndex` 填 MinerU 目录名），核对侧只在空值时回退路径推导；`TestPageSrcKeepsMineruPartName` + 真实数据 244 页 0 告警。

## 第九十一批（2026-11-04）：T293——grep 不认 `project:` 前缀，静默回 NO MATCHES

用户："这次的风格会话 grep DOCVISION-VECTOR … 输出 NO MATCHES，这个太奇怪了吧，原书中明明有，是我们的 grep 程序有问题吗？"

**证据**：转录里 `grep {"path":"project:source/<书>.md","pattern":"DOCVISION-VECTOR"}` → `NO MATCHES`；同一路径 `read_file` 返回 597 KB 正文；`bash {ls -la /project/source}` 里那个 md 软链就在；磁盘上该文件含 54 处 `DOCVISION-VECTOR`、无 NUL 字节（不是被"二进制/超大文件"跳过）。

**根因**：`GrepTool` 两条路径解析分支不对称。有整套挂载表的会话（转换/核对）走 `VFS.Resolve`，认 `project:`；只有 `Root`+`AltRoots` 的会话（样式、修复）走的是 `filepath.Join(Root, rel)`——`rel` 还是带前缀的整串，于是 `<work>/project:source/<书>.md` 不存在 → `continue` → 最后一句 `NO MATCHES`。`read_file` 一直在 `resolve` 开头用 `explicitMountOf` 处理前缀，所以它没这个毛病。

**改法**：`grep` 的路径解析统一用 `t.vfs()`：显式挂载点 `Resolve` 精确定位（命中行前缀用 `label:stripMount(rel)`，不再是绝对路径）；无前缀仍先工作区再备选根；新增 `existed` 标记，一个目标都没落地时回 `grep error: 路径不存在…`（T274 的初衷）。

**验证**：`t293_grep_mount_test.go` 五条（显式挂载点命中且前缀正确、缺路径报错、文件在但没命中仍 `NO MATCHES`、无前缀兜底搜备选根、挂载表会话不退化）+ 既有 `TestGrepMissingPathIsAnError`/`TestGrepToolAltRoots` 等全绿。

## 第九十二批（2026-11-04）：T294——试转会话一张原书位图都没铺进去

用户："他说样式包和工作区都没有原始位图是啥意思？这个工作区丢东西了吗？是不是之前修改的 bash 里面的 vfs 导致的？"

**事实**：项目里 `source/images/<项目名>/` 有 89 条图片（0 死链）；`project:` 窄视图照旧铺 `images`/`figures`/`tikz`（T290 只摘掉 sessions/dedupe/progress 这些流水账），所以**没有丢东西**，也不是 bash 挂载表的事。

**会话里实际发生了什么**：试转工作区 `work/style/pilot/work/` 里只有 cls/manual/example/sample.md，**没有 images 目录**。模型先跑了 `find /source /project -type f -iname '*.png' …`（`-type f` 不匹配软链，视图里的图片条目本来就是软链，必然空），又从未调用过 `view_image`（整条转录 0 次），于是把"没有原始位图"写进报告——它看到的工作区确实没有图。

**根因**：`pilotTrialSession` 铺图那一段——`pilotImageRef` 的正则 `\]\((images/[^)\s]+)\)` 在第一个空白处就截断，项目名带空格时只取到 `images/<项目名>`（半个目录名，指向一个目录）；再加上 `copyFile` 只 `os.WriteFile` 不建父目录、返回的 error 被 `_ =` 丢掉，两条错误叠加成"静默一张不铺"。

**改法**：正则改成 `\]\((images/[^)]+)\)`（取到右括号）；铺图收进纯函数 `stagePilotImages`（去重、逐张 `MkdirAll` 父目录、返回失败的相对路径），调用方逐条打 `[pilot] 试转工作区缺这张插图的副本:` 告警；试点系统提示词的工作区清单补一句"引用的位图已复制到 images/"。

**验证**：`t294_pilot_images_test.go` 三条（带空格路径能铺、缺失如实报、无空格路径不退化）；真实 `sample.md` 探针：**14 张全部铺进、0 缺失**；`xelatex` 实测 `\includegraphics{images/<目录带空格>/a.jpg}` 两种写法都能编译，所以不需要给文件名去空格。

## 第九十三批（2026-11-04）：T295——窄视图在会话 bash 里是空的（里层软链全死链）

用户："tikz 我看这次的那个样式修复会话里面他就看不到里面的东西？之前的会话是能看到的"

**证据**：试点修订会话的转录里 `ls /source; ls /project/source/tikz | head; wc -l /project/source/'<书> .md'` → 输出只有两个 part PDF 的路径 + 一行 `/project/source/tikz` + `wc: no such file`。同一会话用 `read_file`/`grep`（进程内、无沙箱）读 md 却完全正常——所以不是文件没了，是**沙箱里**看不见。

**根因**：T290 把 `project:` 视图里的 `source` 从"指向真 `source` 的软链"改成"真目录 + 里层软链（md / images / figures / tikz）"。`sandboxArgs` 对只读视图只按**第一层**判断有没有软链：`source` 是普通目录 → 走 `--ro-bind <view>/source /project/source` 整棵绑定，里层软链的目标都在绑定范围之外 → 沙箱里全是死链。改动前 `source` 本身是软链，整棵真目录被绑进来（里面的 md/tikz 是真文件），所以"之前能看到"；`images/<项目>/<图>` 那种再深一层的软链一直看不到。

**改法**：`linked` 判断补上 `hasNestedLink(dir, 2)`；只读视图改走 `bindViewTree`：真文件 `--ro-bind` 到视图位置、真目录 `--dir` 后递归、软链解析出目标再按同一位置绑，深度 6，`boundTarget` 去重。

**验证**：`t295_view_sandbox_test.go` 两条（三层嵌套软链都落到实绑；无软链的只读目录仍整棵绑定，不拆成上千条参数）；真实视图进真 bwrap 探测：`TIKZ=54 / MD=7304 行 / IMG=89 / FIG=108 / GREP(DOCVISION-VECTOR)=54`，与磁盘逐一相符。

## 第九十四批（2026-11-04）：T296——重跑试点时选材不再整段重做

用户："那个选材可以不用重跑，其他重跑吧。"

**问题**：`pilotSelectSession` 一进来就 `archivePrevTranscript` 开新会话，从不读上次的转录与 `spec.json`；重跑试点（改样式包、换模型）时那四十分钟的选材等于白等——选材本身与样式包无关，读的是 md 与原书页。

**改法**：新增 `pilotSpecFromPrevRun(proj, trPath)`：`spec.json` 在、转录里已有 submit 收账（`replaySubmit` 认磁盘事实，不看进程内存）、且 md 与 pdf 两类条目都非空时，直接返回上次的 `pilotSpec` 并跳过选材会话；半截清单（只有 md 段）或没交过题的照旧重选。跳过时不动转录，新一次选材仍会照常轮转留档。

**验证**：`t296_pilot_select_reuse_test.go` 两条单测（沿用且字段逐一读对；无转录/无 submit 回执/只有 md 段三种情况都老实重选）。

### 补记（第九十三批）：T295 的引入点，以及"以前为什么没暴露"

用户："上一次确实是 bash 那边存在问题吧？但我看之前的会话好像都没有这个问题，应该是那次 bash 删减的时候引入的。"

引入点确认是 **`42e3d82`（T290 收窄 `project:` 视图）**：`sandboxArgs` 对只读视图"只按第一层判断有没有软链"是 `cb5ee9d` 起就有的老逻辑，本身没变；T290 把视图里的 `source` 从"指向真 source 的软链"改成"真目录 + 里层软链"，同一个判定就从"绑真目录"退化成"绑视图目录"，里层全成死链。

真沙箱里两种形状各跑一次（同一本书、同一批文件）：

| 视图形状 | md | tikz | figures | 位图形状 |
|---|---|---|---|---|
| 旧：`source` 是软链（绑真目录） | 7304 行可读 | 54 个 .tex 可读（读得出 8868 字节） | 108 个条目 | 读 0 字节（`images/<项目>/<图>` → 全局 `images_dir`，一直都在绑定范围外） |
| 新：`source` 是视图真目录（整棵绑） | 读不到 | 只剩 1 行报错 | 只剩 1 行报错 | 读 0 字节 |

结论：① bash 的确是从那次收窄起坏的；② 但**位图从来没能在 bash 里读过**（两种形状都是 0 字节），会话要图一直靠 `view_image`（进程内、不过沙箱）——所以更早的会话"看起来没问题"；③ `grep`/`read_file` 也在进程内，从来没受沙箱影响，T293 那个 `NO MATCHES` 是另一条独立的工具侧 bug。

## 第九十五批（T297）：编译回执点名"真实文件名"

真实事故：试点试转会话写 sample.tex 时，11 张插图的 `\includegraphics` 里只有 1 张的 64 位哈希尾巴打错了（`…f33dcc7701…` 写成 `…8f56b40b80…`），latexmk 报 `File ... not found`、`l.227 \end`；会话收到"not found"后自己判断"证书文件名里有 …f56b40…，TeX 里是 …f9…"——两处都有 f9，判断本身就是错的，改完还是错。它已经读到过 `rg --files` 的完整清单，仍然从上下文里"回忆"了哈希。

判断：不是流水线 bug（同一次试转里 10/11 张图能正常 includegraphics、`example.pdf`/`sample.xdv` 都在编），是**长哈希名 + 只回一句 not found** 这个组合的必然代价。

改：新增 `internal/latex/compile_hint.go`——编译失败时用 `File `X' not found` / `can't find file `X'` 取出缺失名，在它的目录里找与 X 公共前缀最长的真实文件（前缀 <8 字符视为不像、目录不存在就沉默、最多 3 条），回执追加：

```
MISSING FILES（名字必须原样照抄，不要手打）：
  images/<书>/<错的名字>
    → 工作区里真实存在的是 images/<书>/<真的名字>（原样照抄这个）
```

三处编译回执都接上：work 会话（试转/修订，`t.Root`）、整章 convert（`t.Dir`）、figure 预览（`t.State.workDir`）。

测试：`t297_compile_hint_test.go` 三条（错尾巴指到真实文件且同一缺失只列一次、无相似候选不乱指、目录不存在不崩）；`./internal/...` 除 `TestCompileFigureToolFromFile` / `TestPythonEnvVenvRunsInsideRealSandbox` 两条既有失败外全绿，`go build ./...` 通过。

## 第九十六批（T298–T301）：续跑从"最后一步"接着走，不重烧已完成步骤

真实事故（现场）：一次运行在样式试点中途被停，重启后续跑。样式阶段看到转录里"已提交"，重放了那条提交以收账；但试点修订会话此前在共用的 `work/style/plan.md` 里加了一条待办未勾选，重放的提交被计划清单门禁当场打回（回执 `REJECTED: 计划清单还有 N 项未完成`）。重放逻辑只看 `Execute` 是否报错——`REJECTED` 是正常回执、不是 error——于是判定"重放成功"，阶段继续，又因为工具状态没真置位而发出"还没提交"的提醒，把样式会话重新拉起来干活（请求体约 8 MB/轮、第 3 轮仍在转）。同一轮里，试转会话的工作区每次都被清空、转录被归档，修订会话也被归档重开：续跑等于把整场试转与修订重烧一遍。

判断：四处独立缺陷叠在一起——① 重放不能只看 error，得看"提交是否真的被收下"；② 两个会话共用一份 goal 清单会互相卡门禁；③ 试转/修订的续跑路径是"清空重来"而不是"接着干"；④ 样式阶段只看转录、不看磁盘上是否已经有落盘的包，会把之后修订会话改过的草稿重放覆盖回 `style/`。

改：
- `replaySubmit` 改为要求回执 `Terminal`（T92 的收账标记）为真才算成功，否则打一行"重放的提交未被收下（回执首行）"按未提交继续；`SubmitStyleTool` 的成功回执补 `Terminal: true`，与其它 submit 工具一致（全仓 9 处重放点都核对过：成功回执只有这一处没带标记）。
- 修订会话改用 `work/style/plan-revise.md` 自己的清单；样式会话仍用 `plan.md`，`goalGate` 不再跨会话互相牵连。
- `pilotTrialSession` 增加 `pilotTrialPrevRun`：转录已收账 + `sample.tex` 在且仍编译通过 → 直接沿用工作区与转录，不 `RemoveAll`、不归档。
- `pilotReviseSession`：转录已提交 → 沿用最终样式包；没提交 → 回放该转录续跑（`SetMessages` + 计数续接），不再归档重开；续跑时不重发任务文本（T29 口径）。
- `stylePhase` 增加 `stylePackageDone`（`style/` 包齐 + `pilot/spec.json` 或 `pilot.done` 在）：成立就沿用磁盘上的包直接返回，不重放导出。

现场处理：把样式转录截回提交收账那一行之后（去掉提醒拉起的多余轮次）、把修订的那条待办从 `plan.md` 挪进 `plan-revise.md`、回退那次被拉起的样式会话对 cls 的修改（恢复修订会话自己的编辑），三个文件原样备份在 `work/style/.bak-revert/`，源转录另有完整拷贝（截断只删多余的尾部轮次）。

测试：`t298_replay_submit_test.go`（打回 false / 收账 true / 无记录 false / 回执首行截短）、`t300_resume_skip_test.go`（样式包判定：包齐+试点在 → 已完成，缺件或试点没开始 → 未完成；无试转转录直接判否）；`./internal/latex/` 全绿，除既有的两条环境相关失败。

补记（提示词侧，T302）：试点修订的提示词有三处说不清——`Revise class.cls` 里的 `class.cls` 是占位符（真类名要由 `classNameOfFile` 从 `style/` 读出来，代码在渲染时才能给），试转的 findings 只说"the trial session's findings"却不给落盘路径（那份文件是试转会话自己起的名，代码不生成），同一张 sample.tex 又分别写成 `work/sample.tex` 与 `pilot/work/sample.tex`。现场修订会话开局连读 `project:pilot/...`、`trial-findings.md`、`class.cls` 三个不存在的路径，四次调用后才自己 `find` 出工作区。改法：修订模板用 `{CLS}` 并要求 findings 在 `pilot/work/pilot-findings.md`（注册表 Vars 加 `CLS`，渲染时传 `classNameOfFile` 的结果，读不到就退回 `class`）；试转模板加一句"把 findings 写进工作区根的 `pilot-findings.md`"，落盘位置从此固定。开头那段列举（`work/sample.tex`）本就是以 `pilot/` 为基准，与 compile 示例的完整路径对得上，留着不动。`go test ./internal/prompts/` 全绿（Vars 与占位符一致的守护测试覆盖到）。

## 第九十七批（T303、T304）：通过之后不再白烧，通了的章要算数

真实事故（现场）：一章的核对在 12:43 交了 pass（转录里最后一个动作就是 `submit` with `status=pass`，通过记号与转换转录的结论同时落盘）。紧接着 12:43:54，队列里**更早排下的一条自动重核请求**（`Source=no_verdict`，从 10:34 起就在等这一章的锁，日志每 5 分钟一行「请求排队中：还有核对在跑（已等 2 小时）」）被派发出去，于是刚通过的章又开了一条全新核对会话。

那条会话没有可核的东西：读两个文件、grep 一遍 parts 之后就开始原地打转，13:03 退化，复盘会话判"截断到第 16 条"——被"同一位置反复切"的闸门拒掉（这一章这个角色 08:41 已经切过第 14 条附近），只能按 no_verdict 收尾，于是"换一条全新的核对尝试 try7 再来一次（第 1/2 次）"；try8 同样退化（13:11，「第 2/2 次」），try9 跑到 13:26。前后三条核对会话 + 三次复盘（其中三次复盘直接吃上游 400 `upstream_request_rejected`），约四十分钟、数十万 token，全部烧在一份**已经通过、一个字没改**的产物上。

判断：不是核对模型退化，是派发端少问了一句磁盘。队列里的自动请求等的是一章的锁，可能等一两个小时；这期间那一章完全可能已经通过。"提交那一刻清队列"不够——请求可能还没派发，也可能正在派发；"派发那一刻问磁盘"才稳。

改：
- 新增 `internal/latex/checker_passed_guard.go` 的 `chapterPassedAndUnchanged(proj, base)`：产物在、没有 `.tex.checker`（没被后一轮打回）、通过记号里的指纹等于当前主 `.tex` + parts 的内容指纹。判据与 T260 的"陈旧反馈"完全一致，只是用在派发侧。`autoRetryRequest(src)` 区分"程序自己提的"（`rescue`/`no_verdict`/`latex`/`upstream`/空）与页面点的（`web`）。
- `prepareCheckerRetries`：先把队列里"已通过 + 自动提"的 checker 请求直接作废（消费 + 一行日志），页面提的照旧进批次。
- `dispatchRetries`：checker/continue 角色且自动提的请求，派发前查一次；子章节跳过这条判据（它的产物通过了，父章节还要整合，那条继续跑仍得跑）。
- `runRetryTask`：拿到章节锁之后再查一次——派发与执行之间差几十毫秒，这一章也可能刚好通过。
- `recheckChapterAt` 的"没结论 → 自动换新尝试"循环：产物已通过时不再换（人工重核一份已通过的产物若得不出结论，就如实报没结论，别再烧两条）。

页面「重新核对」的语义不变：`Source=web` 一律放行，人工在同一份产物上确实还能看出问题、还能打回（T260 的口径）。

测试：`t303_checker_passed_guard_test.go` 两条——① 判定：无记号 / 指纹一致 / 分片改过 / 又被打回，各条路径；② 队列：一条 `no_verdict`、一条 `web`，通过之后前者被作废、后者交给批次（`prepareCheckerRetries` 返回它、两条都被消费）。

补记（T304）：顺着上面这次复盘又查了一遍磁盘，发现那一章"通过了却没算数"：产物停在 `work/temp/conv_<章>/integrated-stage/`（细分父节点提交的暂存区），通过记号也写在那儿，而转换侧"已有产物，跳过"看的却是 `work/chapters/<章>.tex`——重核这条路只写记号、不晋级（晋级动作只写在主核对流程的通过分支里）。也就是说：核对通过的章，重启后会被当成"没有产物"整章重转，起步两小时，而它上一轮已经判过 pass。

改：`recheckChapterAt` 通过分支补一次 `promoteRecheckedProduct`（只在顶层章节做；子章节的产物归父节点整合，单独搬进 `work/chapters/` 会多出一份没人整合的碎片），晋级后再按新路径重算并写下通过记号——否则派发侧的"已通过"判据（T303 那个）认不出搬过去的那一版。现场按同一动作手工补齐那一章（主文件 + 17 个分片文件，记号与产物指纹核对一致）。

测试：`TestPromoteRecheckedProduct`（暂存区产物晋级到 `work/chapters/`，主文件与分片都在，记号跟着搬，`chapterPassedAndUnchanged` 认得出来）。

## 第九十八批（P68–P76）：非扫描版 PDF 与"人可指挥"的流程设想（仅设计存档，未实现）

用户提出一组新想法，要求先原文存档再汇总。本批只动 `docs/plan.md`：新增 P68–P76 九条与一份原文附录（样本身份按仓库规矩替换为「（书名略）」），不改任何代码与行为。

主题是把定位从"扫描书"扩到"任意 PDF"：原生 PDF 的图本来就在里面（位图和矢量都在），默认应当精确取用而不是重画；"扫描还是原生"必须按页/按图逐张判（半扫描半整理的书很常见），且先跑确定性体检（`pdfimages -list`/`pdffonts`/`pdftotext`），只有体检分不清的才进会话；重复结构的图应当"模板一次、逐处调用"，否则逐张重绘既烧 token 又会被样式阶段改第二遍——这会把图片阶段从整批绘制改成按需触发的工具，并牵动阶段顺序（模板先于图）；此外还有外部素材通道、事前/事后的语义调控与定点返工、顶层"领导会话"、样式与原书的确定性对比，以及把会话层换成 dsh/Pi-ai 的设想。P68 是上一批讨论（样式包类文件规范）留下的待裁决项。

## 第九十九批（T305）：图片核对页的入口不该依赖"并组结论"

真实缺陷（用户报）：某个项目整组核验跑完、**一张图都没并组**，于是图片核对页在页面上根本没有入口——侧栏"整组核验"阶段行不挂按钮（那一段其实只剩汇总行或干脆没有），直接开 `/dedupe?project=<项目>` 也是 404。

根因两层，都在同一个错误前提上："这个页面属于 dedupe 的结论"：

1. 页面渲染 `renderLiveImageReview` 一上来就 `os.ReadFile(groups.json)`，读不到直接 `false` → `serveDedupeReview` 退到静态 `review.html`，也没有 → 404（文案还写着"整组核验跑完才有"，可这次正是跑完了）。
2. 索引 `dedupeReviewProjects` 也只看 `groups.json` / `review.html` 两个文件，于是侧栏连"这个项目能看图"都不知道，按钮自然不出现。

而这个页面本身是"看图"：每张图左原图、右成品，点缩略图看大图。它不该以"有没有并组"为前置条件。

改：

- `renderLiveImageReview`：分组结论文档缺失、或文档里**一组一张独立图都没有**（等于没结论）时，按"未分组"列出 `progress_items` 里全部图，并在规则行写明"还没有分组结论"；只有**一张图都没有**（图片阶段还没跑）才返回 false → 404。
- `dedupeReviewProjects`：判据放宽成"有图片判定记录"（`source/progress_items/<md>/<图>.json`），这样一张都没并组的项目也会在索引里报出来。
- 侧栏：入口从"只挂在整组核验阶段行"改成**固定在项目行**（阶段行那个保留），项目名右边一个「看图」小链接，`@click.stop` 不打断折叠。

验证：新增 `TestImageReviewWithoutGroups`——无 `groups.json`、以及"有结论文档但空分组"两种情形都要能打开并列出图片；`go test ./internal/sessionview/` 全绿，`npm test` 287 项通过，`viewer.js`/`viewer.css` 重新构建并提交。

补记（设计侧修正，同日）：P69 与设计报告原先把"原页裁切"当成原生 PDF 的首选取图方式，方向错了——**非扫描版应直接读原对象**（页内嵌位图取 `pdfimages -all` 的原字节、矢量把 Form XObject 导出成独立 PDF），"按裁剪框在原页重裁"只适用于**扫描版**（它本来没有可分离的对象），页级 `\includegraphics[page,trim,clip]` 降为兜底。已同步 `plan.md` P69 与 `docs/plan-report/design-any-pdf.md`。

同一批的清理：把档案与代码里残留的三处样本身份（两本书名与一本教辅名）中性化（`docs/agent-batch-history.md` 两处、`img2text/pagemark.go` 与两个测试的 fixture），只换身份、技术与数字原样保留。

## 第一百批（T306）：分类失败不许冒充"位图"（含现场复盘）

真实缺陷（用户从运行日志里发现）：某台机器上分类请求报错，日志里出现 `images/... -> raster (unclassified)`。该记录既是**最终判定**（`toClassify` 只挑 prog 里没有记录的图，于是再也不会重试）、又让 process 阶段正常跑过（原图当位图嵌进书里），而且和真的被判定为位图的图完全分不开——一次接口错误就是一张/一批本该矢量的图被永久降级，整本书照样编得出来，没有任何地方报警。

改法（不动 raster 的语义：raster 是**判定结果**，不能拿来当异常兜底）：

- `classifyFailure`：失败记 `Class: ""`、`Status: error`、`Error: "<哪一步>: <原因>"`（截断 400 字）。
- `classifyNeeded`：没有判定记录、**或者没有类别**都要重新分类；已有类别的条目不重分类（那条是给 process 用的）。
- 三条失败路径（图片缺失、读图失败、请求失败）走同一个 `fail` 闭包，都留账。
- `classifyPhase` 返回 error：`wg.Wait()` 之后还有失败就 `classifyAbortError` 停下（点名最多 5 张、写明"不是这些图是位图"、说明重跑会从这些图继续）；调用点直接 `return err`，`--step classify` 也一样。
- 核对页多读一个 `error` 字段，显示成"未分类：<原因>"。

现场复盘（同一台机器、同一项目，两次运行）：日志里 97 张图全部 `回复中未找到 JSON 对象`，没有任何 HTTP 错误。远程把整条链路拆开复现，结论如下。

**根因是 base_url 少了 `/v1`，被流式读取器吃成了"空回复"。** 该网关（New API）把 `/chat/completions` 当网页路由，回 **200 + HTML 首页**；而 `latex` 默认 `stream: true`，`readStream` 在 HTML 上收不到一个 `data:` 行，`chatstream.Collect` 不报错、返回的消息内容为空——于是每张图都是"回复里没有 JSON 对象"，而 HTTP 层一切正常。同一地址加 `/v1` 后，用**真提示词 + 真图片**打过去立刻正常：`{"confidence":0.95,...,"kind":"vector"}`，流式与非流式都对得上。补测证实了这条链：把 HTML 直接喂给流式 `decode`，`err=nil`、`content_len=0`。

因此这一批末尾又补了一处代码修正：`readStream` 在"0 个内容块 + Content-Type 不是 `text/event-stream`"时直接报错，明说"端点返回的不是 SSE 数据流，多半是 base_url 少了 /v1"。真正的退化空响应（T50）走的是真 SSE，Content-Type 是 `text/event-stream`，不受这条影响（`TestStreamOnEmptySSEStaysEmptyReply` 钉住）。

顺带查出的第二处：**输出预算与诊断**。512 token 对思考型模型太紧——真提示词下思考内容就 2000 字上下，很容易把预算吃光让 `content` 变空或只剩 `{"`，而错误信息只有"回复中未找到 JSON 对象"。于是 `classifyMaxTokens` 512 → 2048（两次尝试同值）、`classifyReplyError` 区分"被截断 / 回复为空 / 模型说了别的话"并附原话前 200 字、非流式响应也把 `choices[0].finish_reason` 提到顶层（`ChatResponseChoice.FinishReason` + `decode`；此前非流式路径的 `ChatResponse.FinishReason` 永远为空）。

运行时现场（不写进仓库，记在这里备查）：该机器上该项目 35 条 `class=raster && label=unclassified` 的进度记录已整体搬到 `<项目>/work/rollback_T306_20261110/source_progress_items/`，`progress_items` 清空（下次运行从头分类）；`figures`/`tikz`/`chapters`/`build`/`out`/`style` 当时全空，没有下游产物需要回退。

## 第一百零一批（T307）：重画图的提交门槛与"尽力还原"

**用户提的问题**：① 想让 `compare_figure` 有个默认 17% 的门槛，"要提交的话必须达到"；② 发现有些图压根没和原图对齐——不是覆盖率，而是横纵、宽度、图在画布里的位置都没对齐，怀疑是模型随机偏差而非提示词问题；③ 想在网页上给每个矢量图会话单独设阈值，能"打回继续"而不是只能重新跑；④ 还需要一个"实在达不到要求精度但已经尽力还原"的提交选项，否则一直烧 token。

**查下来第一件事得先说清**：代码里**没有** 17% 这个门槛。`submit_figure` 只有一个条件——成功编译之后至少调过一次 `compare_figure`（`tools.go`）；那个 17% 是回执里 `Soft-ink overlap: 17.00%` 这个**显示用**的数字。所以"提高阈值"无从谈起，得先把门槛做出来。

**为什么"没对齐"却能过**（这是 ② 的真正答案，也被记进 P78）：`compareFigureImages` 算分之前做了三件对齐归一化——`fitFigureCanvases` 把候选**按原图宽度等比缩放**、`cropFigureInk` 按墨迹外接框裁掉留白、质心 + 逐级平移搜索取最优。结果刚好把用户关心的三样抹掉：宽度差、画布内位置、横纵比例。它量的是"墨水形状像不像"，所以 17% 也可能"看着对上了"。要卡这三件事，必须另外量**归一化之前**的墨迹外接框。

**改法**：
- `figureInkBox` 单独扫一遍墨迹外接框（不拷像素），在 `fitFigureCanvases` **之前**量候选/原图的宽高比 → `ScaleX/ScaleY/ScaleDev` 进 `figureComparison`，回执里写明 `Ink size vs original: width x0.92, height x0.69 (max deviation 31.0%)`。
- `tikzState` 记"最近一次编译后最好的一次成绩"（`bestOverlap` + 对应 `scaleDevAtBest`/`bestScaleX/Y`），`compile` 成功时清零。
- `SubmitFigureTool` 拿 `MinOverlap`/`MaxScaleDev`：不达标就 REJECTED，回执把"12.30% < 17.00%（差 4.70 个百分点）""宽高尺寸偏差 31.0% > 10.0%（宽的 0.92 倍、高的 0.69 倍）"写全；`accept_as_is:true` 必须带 `note`，放行并在回执里标 best-effort。
- 配置：`latex.figure_check.min_overlap`（0 = 用默认 0.17，负数 = 关掉）、`max_scale_dev`（0 = 用默认 0.10，负数 = 关掉）；两份模板与 `docs/config.md` 同步（不是新块，`config_version` 不 bump）。
- 粘性：`TikZResult`/`imageProgress` 增 `fig_best_overlap`/`fig_accept_as_is`/`fig_accept_note`/`fig_min_overlap`；`--only` 点名重画会清掉接受标记（人说了算）。
- **提高门槛后自动重判**：`requeueBelowThreshold` 在 pending 选择前把"交过、成绩低于当前门槛"的图退回未分类（成绩提交时就记了，不必重画一遍才知道），已接受的和没有成绩的老图不动；退回当场落盘。
- 两处界面共用一个写入口 `POST /api/figure-override`（项目校验走 `projectWorkspace`，`md`+`img` 或作图转录名 `path` 定位——后者与 latex 的成品名规则一致，`vectorTexBaseOf` 是它的镜像实现，因为 latex 已经 import 了本包）：核对页卡片上多一行"阈值 全局 · 上次 12.0% [17] 存阈值 · 尽力还原 / 撤销接受"，实时渲染才挂控件、静态导出不挂；会话页在「重画这张图」旁边同一组控件。`/api/figure-override` 与其它写入口一样受 `--no-retry` 总开关约束。

**留一个说明**：`latex.figure_check.enabled` 仍然管的是"逐图视觉校验"那张额外的网；这次的门槛不看那个开关，它是 `submit` 自己的判据（因为用户要的是"提交前必须达到"）。默认 17%/10% 会对现状生效；某张图被门槛卡住时只要 `accept_as_is` 就有出口，不想让全局门槛生效就把 `min_overlap` 写负数。

**测试**：`TestSubmitFigureThresholdGate`（回执必须带数字与差值）、`TestSubmitFigureThresholdPassesOrDisabled`、`TestSubmitFigureAcceptAsIsNeedsNote`、`TestRequeueBelowThreshold`（只退该退的）、`internal/config` 的 `TestFigureThresholdDefaults`、`TestFigureOverrideSetsThreshold`/`ResolvesTranscriptPath`/`AcceptNeedsNote`/`RejectsBadInput`、`TestReviewPageRendersThresholdControls`；`npm test` 287 项全绿，`go test ./internal/...` 只剩两个既有失败。

**补记（同日，用户追问"已经通过了我改阈值让它继续画怎么办"）**：门槛做好了才发现漏了一环——升级前就画好的图没有 `fig_best_overlap`，改了阈值也判不了，用户只能一张张点「重画」，而「重画」是**从头画**（旧转录改名 `.tryN` 留档），偏偏人想要的是"接着上次那条会话继续改"。补两件：
1. `backfillFigureScores`：启动时对"done + 有 tikz_code + 没有成绩"的图，用**已交付的成品 PNG 与原图**本地比一遍（`compareFigureImages`，不调用模型、不花 token），写回 `fig_best_overlap` 与新的 `fig_scale_dev`；核对页那行就从"上次 --"变成"上次 18.4% · 尺寸偏差 39%"，哪张该收拾一目了然。
2. 退回语义收紧：`requeueBelowThreshold` 增加尺寸判据（`max_scale_dev`），并且**补算出来的成绩默认不参与自动退回**——只有被单独设了 `fig_min_overlap` 的那一张（人明确点了名）才会退。否则升级后第一次跑，全局门槛一比对就是把几十张老图一起退回去重画，纯烧 token。
顺带把"继续画"和"重画"的区别钉清楚：自动退回（`status → classified`、转录原地保留）走的是"恢复中断的会话"，历史原样接着改；页面上点「重画这张图」（`from = 0`）则是旧会话归档、开新会话从头画。要"继续画"就别点重画。

**再补（用户）：`accept_as_is` 得交够次数才算数。** 现场数据很说明问题：升级后第一次跑，有成绩的 12 张里 11 张走了"尽力还原"，成绩落在 8%–18%，其中不少是第一版或第二版就举手。用户要求"只能在 submit 8 次以上才可以被接受"，并把次数放进默认配置。于是 `tikzState.submitAttempts` 每次 submit 都 +1（被门槛打回的也算——那正是"改了一版又交"的证据），`SubmitFigureTool` 在 `accept_as_is` 时要求 `submitAttempts >= latex.figure_check.accept_min_submits`（默认 8；0 = 用默认，负数 = 不限），打回回执直接写"只交过 N 次，至少要真交 8 次"并附上差在哪。用户随后又收紧了一条：**这个出口根本不该出现在工具清单与提示词里**——schema 里写着"needs at least 8 submit attempts"等于一上手就告诉模型有白旗可举，反而更爱投降。于是 `submit` 的 schema 退回只有 `path`/`code`/`merges` 三个参数，`accept_as_is` 与 `note` 一个词都不出现（解析仍然认，模型照抄历史里的调用也还能用）；打回回执也分层：次数没到只说"差在哪 + 这张图目前只交过 N 次"，交够次数后那次打回才把"确实只能做到这儿的话可以 `accept_as_is` 交这一版"说出来。两条守护测试钉住：`TestSubmitSchemaDoesNotAdvertiseAcceptAsIs`（schema 里不许出现 `accept_as_is`/`note`/`best-effort`/`尽力`）、`TestAcceptAsIsHintOnlyAfterEnoughSubmits`（前两次打回不许提、第 3 次才提）。代价是每张图至少 8 轮改图，token 会比之前高——想放宽就配小一点。

**T308（用户问"我点了两次重画，不知道排在哪里了"）**：查下来两条请求都还在 `<proj>/work/retry/` 里，`.tryN` 留档 0 份——**从来没被执行过**。原因在 `dispatchRetries` 的矢量图分支：`vectorMu.TryLock()` 拿不到锁就 `continue`，而 images 阶段为了写同一批 `progress_items` 会整段持锁，于是"图片阶段跑着"和"重画请求排队"这中间的几十分钟里，请求既不动也没有任何日志（同一类问题 T232 在章节侧修过：排队不再沉默，这次漏了矢量图这一支）。修法只是补一行节流日志，请求留队语义本来就对（有 `TestVectorRetryStaysQueuedWhileImagesHoldsLock` 钉住"锁被占时不派发也不吃掉"）。

**T309（用户指着轨迹页那排按钮问"这个就地截断坏了吗"）**：确实坏，而且不是界面问题。线上探针很干脆——对一条作图转录发 `/api/truncate`（`line=0`，解析完就停在参数校验，不改文件）得到 **400「转录不在这个项目的工作区里」**，只给文件名则是 **404「转录不存在」**。原因：**就地截断**（`/api/truncate`）与**删除这条记录**（`/api/drop`）共用 `transcriptUnderWork`，它只接受 `<项目>/work/` 之下的路径（为递归子章节的细分树加的），而作图转录写在 `<项目>/source/sessions/`——这两个按钮对作图会话**从来**没成功过，而前端 `canCutFrom` 是允许 vector 的，于是按钮常亮、点了报错。修法是把解析抽成 `transcriptUnderProject`，同时认 `work/` 与 `source/sessions/`；裸文件名先 `work/sessions/` 再 `source/sessions/`；顺带把删除回执里那句"work/attempts 里的同一次尝试也一起删了"改成只在真有归档时说（作图会话的留档是 `.tryN.jsonl`，摆在同目录）。

**T310（用户："帮我撤销之前的那个重画申请"）**：走页面同一条接口去撤，两次都回 `{"removed":0}` 而队列里明明还有 2 条。原因在 `serveRetryCancel` 的矢量图分支：`filepath.Base(f.Path)` 取的是**队列文件名**（`req-<纳秒>-vector-….json`，`retryq.File.Path`），转录名在嵌入的 `f.Request.Path` 里——两者永远不相等，于是撤销对作图会话**从未生效**，页面还据此说"已经被 latex 消费，撤不回来了"（`actions.ts` 的 `n>0` 分支才提示撤销成功）。修好后回执 `removed:1`、队列立即少一条。当时那两条是直接删队列文件撤掉的（从未被消费，`.done.json` 都没有）。

**T311（用户指着对话页那排按钮问"你有在本地打开网页测试过吗？为什么点这个裁剪按钮没有反应"）**：老实说没有——前面 T309/T310 都是在**服务端**，`curl` 打接口就能证死；这个 bug 在**前端点击处理**里，只有真点一次才看得见。补课：playwright 起 chromium，两边都点了。① 真页面（旧构建）：那个作图会话有 12 个 ⤫ 按钮，点下去**没有确认框、没有 POST、没有提示**——复现。② 本地 fixture + 新构建：第 3 条上点 ⤫ → 确认框「删掉第 3 条及之后（共 2 条保留）」→ `POST /api/truncate` → 提示「已截断：保留 2 条、丢掉 4 条（备份 work/sessions/.trash/…）」→ 文件 6 行变 2 行、备份落盘、控制台零错误。根因是 `dropFromLine` 的 `if (!s || !s.chapterId) return false`：作图会话 `chapterId` 为 null，而按钮是否出现走 `canCutFrom`（允许 vector）——两处判据不一致，点击就在第一行静默返回。顺带确认了"正在写入的会话不给截"这条设计有效（fixture 刚写完时 ⤫ 干脆不渲染）。

**T312（用户追问"你那个宽高测量的是墨迹宽高和原始的墨迹宽高对比？还是 pdf 本身的宽高比和图片的宽高比？别是墨迹宽高和原图那个图片宽高——倒不如对比 pdf 画布本身和原图"）**：不是"墨迹 vs 原图外框"，两边都是墨迹外接框；但比的是**像素**，也就是把 PDF 渲染的每毫米像素数除原扫描的每毫米像素数——绝对值毫无意义（同一张图调 `raster_dpi` 结论就翻），只有两者之比的**宽高比**才 DPI 无关。用户拿到的那句"宽的 0.92 倍、高的 0.69 倍、偏差 31%"里，真正有几何意义的是 0.69/0.92 = 宽高比偏 25%，剩下那一截是任意缩放因子。改法：墨迹占各自画布的比例 × 画布真实尺寸 → 毫米；原图侧用 `MeasureImage`（MinerU 解析出的印刷尺寸，跑在真实项目里可用），候选侧用 `pdfPageSizeMM`（PDF 页真实尺寸）；两边缺任一侧就退到宽高比（报"只能比形状"）。回执三样都给：画布占比 / mm 尺寸 / 宽高比。测试 `t312_figure_size_mm_test.go`：像素差 5 倍但物理相同 → 偏差 0（旧口径会报 400%）；真放大一倍 → 100%；压扁 2:1→4:1 → 100%；无 mm 时退宽高比（同形状放过、3:1→4:1 判出）。

**T314（用户："我截断了没有改变这个图片本身是否完成的状态……不然我截断的意义是什么"）**：核对现场——`work/sessions/.trash/` 下 32 份截断留档，**32/32 对应的进度条目都是 done**；本轮 `Already done: 49 | to process: 48` 里一张截断过的都没有，会续跑的 13 张是更早就中断的会话（名字与 `md+图片名+label` 逐字对得上，末条都是 `tool` 回执）。机制上截断确实会续（`RunTikZSession` 读 `vector_<md>__<hash>__<label>.jsonl`、非空就 `SetMessages`+`SeedCounters`，`resumeUserText` 对非空历史返回空串=不插 user 内容、不重附原图，只有 `from=0` 的「重画这张图」才归档 `.tryN`），问题是**根本没人派它**。修法：截断/删除成功后按转录名里的 64 位图片哈希找回 `source/progress_items/<md>/<file>.json`，`status=classified`、删 `tikz_code`/`figure_*`/`fig_best_overlap`/`fig_scale_dev`/`fig_accept_*`/`error` 等，保留 `class`/`label`/dedupe 字段与 `fig_min_overlap`（按图阈值），回执带 note；dedupe 副本不翻（要重画得对代表图操作）。三条测试钉住：正常翻转、副本跳过、章节转录不碰逐图进度。正在跑的那一轮不会追加（pending 在启动时算好），下次启动就在名单里。

**T315（用户："把「修改阈值」也做成立即生效：当新阈值高于这张图当前成绩时，直接改回待处理"）**：同一类"只写字段不排队"的问题。`/api/figure-override` 只改 `fig_min_overlap`，真正的退回在运行启动时的 `requeueBelowThreshold`（它要 status=done、`fig_best_overlap>0`、没被 `fig_accept_as_is` 接受、门槛高于成绩）。改成写阈值时当场判一次：满足条件就把 `status=classified`、清 `tikz_code`/`figure_*` 等成品字段（`figureRedrawFields`，与 T314 共用），保留阈值与上次成绩（页面还能显示"上次 18.4%"）；回执 `note` + `requeued`，会话页（`res.note`）与核对页（解析 JSON 后显示 `out.note`）都把这句露出来。四条测试钉住边界：抬高退回、低于成绩/用全局不动、接受过的图不动、没成绩的老图不动。现场分布（当时 69 张已完成）：有成绩 56 张、17%–25% 27 张、≥25% 29 张、接受过 4 张、没成绩 13 张。

**T316（用户指着一段作图转录："这边是不是没有说一些什么行为？没有什么特别提示词吗"，随后："处理一下这些错误的…会话输出，裁剪掉…还有对于有的那种一直在重复的，如果降低阈值的话也要实时跟进吧"）**：那段转录的真相——`submit` 收下（17.67%）→（另一个进程 / 另一次派发续跑）→ `replaySubmit` 重放那次提交 → **REJECTED: call compare_figure at least once after a successful compile** → 空初始文本跑一轮 → 空消息触发 "Continue." → 模型写"已提交" → `run_end` → 因为 `state.submitted` 是假的，代码发"你还没提交"的提醒 → 模型直接 submit → 又被打回（这一版就是末尾那行 REJECTED），整张图被记成未提交。根因两处：`replaySubmit` 是靠**重新执行工具**判断"收下没有"，而作图提交的门槛（compile/compare）是进程内状态，新进程必然打回；提醒文案只说 compile，没说 compare。修法：`LoadTranscriptStats` 多给一个 `SubmitAccepted`（最后那条 submit 的回执以 SUBMITTED 开头），`RunTikZSession` 在"回执已收下 + 成品 `tikz/<名>.tex` 与 `figures/<名>.pdf` 都在"时直接 `return TikZResult{Submitted:true, Resumed:true}`，调用方见 `Resumed` 就不覆盖 `tikz_code`/成绩；提醒改成 compile → compare_figure → submit 三步并写明顺序。门槛侧同理两个方向都要当轮生效：`/api/figure-override` 抬门槛标回待处理并写记号 `fig_requeued=threshold`（不再清成品字段——清了就没法撤回），放宽到成绩之下且记号还在就把状态写回 done 并抹掉记号；`SubmitFigureTool.LiveThreshold` 让每次 submit 现读进度里的 `fig_min_overlap`（0=用全局、负数=不卡），所以一直在重复交不过的图降阈值后当轮就能过。现场收拾：一条 379 行的转录被截回 362 行（保留到 compare_figure 的图片回执，丢掉那一段 submit + 空转 + 打回），备份在 `.trash`。测试四条：`SubmitAccepted` 正反例、实时门槛三态、放宽撤回、非本队不撤回。

**T317（用户看着控制台："我那个重绘还有调高阈值的任务为什么没有在图片阶段完成？…那还有我提高阈值的那些不还在排队吗"）**：两条不同的等待。① 三条重画请求确实进了 `work/retry/`，但图片阶段整段持着 `vectorMu`、重画又是逐张串行（`TryLock` 拿不到就跳过），只能等 32 分 52 秒的图片阶段收尾——收尾后第一条 04:50:52 才开跑，队列文件被消费时改名成 `.done.json`，所以日志里"排队中"那三行不是失败，是等锁。② 抬高阈值的那些更彻底：T315/T316 只把状态写成 `classified`，而 `pending` 是启动时算好的，运行中的 process 阶段不会回头再看一遍进度，于是它们的"排队"根本不存在——要么重跑一次，要么手动一张张点重画。这显然不该让人来补：抬门槛本来就是"这张图要重画"。于是 `/api/figure-override` 在翻成 `classified` 之后顺手 `retryq.Queue` 一条 `role=vector`（理由"门槛抬高，重画"，无 `from`，不归档转录），正好接上 watcher 已有的派发路径；反向撤回时再用 `dropVectorRequest` 把还没被接走的那条删掉（已改名成 `.done.json` 的不动）。排请求一律在写进度之后：watcher 可能下一秒就看到它，那时进度里必须已经写着 `classified`。测试两条断言：抬门槛后 `retryq.Pending` 有 1 条、撤回后为 0。

**T318（用户重启后："你再看看，这全启动就 done 了咋办"）**：控制台 `process 14/15 · done:14`，日志里 14 行 `转录里的提交已被收下且成品在位，直接按已完成收账` —— T316 的收账把 T315/T317 刚排出来的重画全吃了。根因很清楚：收账条件只有"转录里最后那条 submit 被收下 + `tikz/*.tex` 与 `figures/*.pdf` 在盘"，**没看成绩够不够这张图现在的门槛**；抬门槛是后发生的，旧提交（17.9%）当然被收下过。更要命的是连锁：翻待处理时（v17 那版）清掉了 `tikz_code`/`figure_pdf`，收账又把它们原样留空，于是 md 里那张图会退回嵌入原图、latex 代码块空着。修三处：① 把 submit 门槛的判据抽成 `figureGatePass`，`figureResumeCredit` 用它判"成绩本来就达标"，不达标不收账、照常续跑；② 收账额外要求进度里 `tikz_code` 与 `figure_pdf` 都在，空的说明记录不完整，交给续跑补；③ `requeueBelowThreshold` 原来要求 `tikz_code != ""`，于是字段被清过的图永远停在"done 且成绩低于自己的门槛"，现在"被单独设过 `fig_min_overlap`"的可以退。现场数字：14 张收账里 9 张成绩低于自己的阈值（17.07%–30.12%，门槛 19%–35%），其中 2 张交付物字段是空的；部署 v20 后下次启动由 `requeueBelowThreshold` 自动退回。

**T319（用户："样式里面对书的比例和什么完全误判了还有书的尺寸感觉问题很大"，紧接着："是不是用 pdf 里面那个 size 误导了？原书里面有那个开版信息，有时候并非用 pdf 本身的尺寸"）**：看一眼产物就明白——`style/*.cls` 里写着 `paperwidth=260mm,paperheight=397mm`，而这本书的版权页明明白白印着"开本 850×1168 1/32"（成品约 146×212mm），扫描页框比成品大了近一倍；而且各 part 的页框本身就不一致（383.8mm 与 397mm 并存），样式会话挑了最大的那一份，还没吭声。根因是样式阶段只把 `view_pdf` 的毫米回执丢给模型自己推纸张，代码侧一个字都没给。修法：样式首条消息先注入"书里自己声明的开本"原文（`trimSizeSnippets` 在 md 正文里捡「开本 …」「成品尺寸 …」以及裸的 `787×1092` 这类规格，最多两条、不替它换算），再注入量出来的 PDF 页框并明确写成**交叉校验**，同时要求版心按页面比例给、别抄绝对毫米——扫描件缩放这件事必须让模型知道。测试：四种正文（版权页/成品尺寸/裸规格/正文无尺寸）与"量不到 PDF 时安静降级"。

**T320（用户："转换 · chapter_004_child_03 里面的矢量图和之前矢量图环节提交的那个不一样？是程序出 bug 了还是模型自己又重绘了一下？"）**：比对三份文本就有答案——`source/*.md` 里的 ```latex 围栏与该图的 `source/tikz/*.tex` 相似度 0.959（只差 wrapper 的提升），而章节 `chapter_004_child_03.tex` 里那块与两者只有 0.319：提交的那份是 `\usepackage{tikz-3dplot}` + `\tdplotsetmaincoords{65}{0}` 的真 3D 立体图，章节里那份是 `x=1mm,y=1mm` 的手摆 2D 图。再翻转录，模型的推理原文写着「I should avoid package dependency by using TikZ for 2D geometry while keeping the 3D figure labels」。所以不是 md 写错、也不是程序把代码搬错——是转换会话自己重画了。往下追一层才是真缺陷：图代码顶端带着 `\usepackage{tikz-3dplot}\standaloneconfig{border=.5pt}` 这种前言行（94 张里 26 张如此，18 张要 `tikz-3dplot`、3 张要 `amsmath`/`etoolbox`），粘进章节正文就是 "Can be used only in preamble"；样式类又不加载 `tikz-3dplot`，模型无路可走才整张重画。修三处：`stripFigurePreamble` 把围栏里的前言行剥掉（`\usetikzlibrary` 保留，正文里能用）；样式阶段扫 `progress_items` 统计宏包/库清单并写进首条消息，要求类自己 `\RequirePackage`/`\usetikzlibrary`；转换提示词补两句（前言行丢掉、缺包是样式问题、不许重画几何）。注意：这条与 T319 是同一轮暴露的两件事——一个把"页面尺寸"当纸张，一个把"缺包"当成"图不对"。

**T321（用户："第一个吧，之后你得找找哪些他修改了……这边样式章节咋办？"）**：用户选择"保留这一版样式、补 cls、只重转被改写的章节"，所以先把漏加载的宏包补进 `style/*.cls`——本书图代码要 `tikz-3dplot`（18 张）、`bm`（1 张），TikZ 库还差 `patterns.meta`、`3d`、`decorations.markings`、`angles`、`quotes`、`intersections`、`positioning` 七个（`arrows.meta`/`patterns`/`calc`/`amsmath`/`etoolbox`/`fontspec` 早已在）。补完用 `example.tex` 试编译：0 错误、16 页。代码侧加一道 `warnMissingFigurePackages`：`preflightConvert` 里比对"图代码要什么"与"cls 加载了什么"，缺的列清单并点明后果，免得下一次又静默退化成重画。扫本轮已产出的 34 个章节（去重后）：**5 个章节的图被改写**——4 个正文子章节（改写块数 4/4、12/16、3/6、4/12）与 1 个附属片段文件（6/6），最差相似度 0.19（等于重画）。

**T322（用户："修复会话里面还有前面的转换章节里面我们这个 tikz 的源码是否可以被这些会话看到？还有修复的时候能看到原始 md 吗？"）**：先查挂载表——`sessionMounts` 只对作图会话与分块（chapters）会话关掉项目视图，转换/核对/评估/分块修复/整合/终审全都挂 `project:`，而 `ensureProjectSourceView` 只铺 md 文件 + `images`/`figures`/`tikz` 三个产物目录（T290 的窄视图），所以**技术与权限上一直看得到**：章节 md 里有 ```latex 围栏（就是该图的权威代码）和 `LINK: [vector](images/…)`，`source/tikz/<名>.tex` 是带 standalone 包装的单图源，`source/images/` 是原图，`source:` 还能开原书 PDF 页对照印刷结果。真正的缺口在提示词：`latex_stylefix.user.md` 的 `project:` 清单写了 style/chapters/converted/reports 却漏掉 `source/`，`latex_stylefix_block.system.md` 干脆一个挂载点都没列（只描述 work 里有什么 + `view_image`），于是会话面对"曲线画不出来/缺 3D 环境"只能自己补图形。两处各补一行（读围栏或 `tikz/` 源 + 原图，只改代码、不许重画几何）。与 T320/T321 是一条线上的第三环：代码供给（谁加载包）、供给检查（有没有漏）、以及改图时该照哪份代码。

**T323（用户："exit error: exit status 127 / bash: 行 1: rg: 未找到命令，这个 rg 是什么？"）**：`rg` 是 ripgrep，一个比 `grep` 快的搜索工具——它不在宿主上（`command -v rg` 为空），沙箱里自然也没有；模型是凭习惯敲的，撞一次 127 换个写法又撞一次，白烧轮次。修法不是往提示词里堆"不许用 rg"，而是让工具回执自己会说话：解析 `line N: <cmd>: command not found`（同时认中文"未找到命令"），按命令给出环境里现成的等价物，并在末尾加一句"不要重复试同一条命令"。这条与 T273（工具名解析：`Bash`→`bash`）是同一类问题的两个面——一个是工具名叫错，一个是命令不存在，都该由回执纠正而不是靠模型猜。

**T324（用户："我在终审会话终止重新运行，他就从全书修复开始，并且这边已经重复一次了……我希望下次我继续应该从终审会话开始"）**：先看事实——`book_fix.jsonl`（18 KB，06:17）与 `book_fix_prev.jsonl`（72 KB，06:14）并存，`out/` 还是 11-10 的旧货，说明修复会话的成果只活在临时构建树里、随 assemble 重建一起蒸发。从上一轮修复转录里挖出最早的编译错误，两个都指向代码：`Option clash for package hyperref`（类里有 hyperref，`ensurePDFBookmarks` 又往 main.tex 注一个带选项的）与 `File 'chapter_001/children/chapter_001_child_01.tex' not found`（子章节按 `chapter_00X/…` 相对路径互相 `\input`，主文件在构建根）。手搭一棵构建树验证：加 `\input@path{{chapters/}}` 后路径错误消失，接着暴露 `Undefined pattern 'Lines[angle=20,…]'`（需要 `patterns.meta`，而图代码自己没声明这个库——`grep pattern=Lines source/*.md` 为 0，是转换会话现场发明的语法）与 `Undefined control sequence \bm`（类里没有 `bm`）。补上后整书重编译 **0 错误、430 页**。代码侧三处：main.tex 注 `\input@path`（幂等）、类已加载 hyperref 就跳过注入、样式消息里给一套基线 TikZ 库。顺带发现类被后来的样式会话重写过（我前一天手工补的库行没了，`bm` 也丢了）——所以这类"类里该有什么"必须由样式阶段的清单要求，不能靠手工补。

**T325（用户："我可以等下让他重跑修复会话，只是我的意思是之后不能再终审停下后下次还是从修复开始内容被删除"）**：用户把话说到了点子上——T324 只是把这本书卡住的三个编译原因拔掉了（路径、hyperref 选项、缺库），一旦换一本书、换一个类，修复会话照样要跑，而"修完的东西活不过一次重启"这件事本身没解决。看代码：只有终审**成功**才 `persistFinalReviewState` 回写 `work/chapters` + `work/final_review`；修复会话**成功**那一支什么都不写，连失败/中断时那套 `persistFinalReviewPartial`（T85）也没接。于是 assemble 下次 `RemoveAll(build)` 一重建，修复成果蒸发、编译再失败、又开一条全新修复会话——同一本书连修两轮的真实事故。改法就一句：修复成功（编译已通过）后同样 `persistFinalReviewPartial`，章节进 `work/chapters`、顶层 `.tex` 进 `work/final_review`、指纹落 `state.partial.sha256`；用中断态而不是完成态，是为了让它只解锁"恢复现场 + 从终审续跑"，绝不被误判成终审已完成（否则会跳过终审）。测试 `TestFixPersistSurvivesRebuild` 把这条链子钉住：回写、指纹匹配、完成态仍为 false、重建树拿到修复版章节与 main.tex。

**T326（用户："这个终审会话怎么卡住1个小时了？在编译"）**：`ps` 里看得清清楚楚——`xelatex` 以 99.9% CPU 跑了 61 分钟，它的父进程 `sh -c …` 只剩 PPID 1，latexmk 早没了；而 `main.log` 一小时前就停在某个字符中间，说明 TeX 在空转、不是慢。三个事实拼起来：① 终审会话自己在 main.tex 里写了一段 `\RenewDocumentCommand` 覆写目录行、并用 `\input` 拉各章 `contents.tex` 的宏（它前后还在反复 edit_file）；② 这类宏一旦参数写成 `##1` 而外层不是定义体，就会在目录展开时无限调用自己——100% CPU、不报错、不写日志；③ 真正的坑在代码侧：`exec.CommandContext` 到点只杀直接子进程，latexmk 拉起的 `sh -c xelatex` 成了孤儿；而 `CombinedOutput` 走管道，孤儿继承了写端，`Wait` 永远等不到 EOF——超时判了 15 分钟，工具调用又陪挂 45 分钟。杀掉 xelatex 后会话立刻恢复（07:44 继续改 main.tex、07:46 编译通过 430 页 0 错误）。改法：`runCmdTimeout` 收口——独立进程组（`Setpgid`，超时整组 SIGKILL）、输出内存缓冲、等待 goroutine、超时后限时 5 秒。测试用 `sh -c "sleep 30 & sleep 30"` 复现"孙子进程继承管道"这个死法，0.5 秒返回且不留孤儿。

**T327（用户："终审会话的工具调用轮次感觉有点少了，可以给256，默认"）**：查下来终审根本没有自己的调优块——`finalReview` 直接 `r.cfg.LatexSession("convert")`，而随附模板给 convert 写的是 128 轮。终审的工作量却是另一回事：逐章通读、核对封面/目录/页码/图表、改版式、再重建全书，128 轮对一本几百页的书常常不够。于是加一个 `latex.sessions.final_review` 块：没配轮次默认 256，其余字段（上下文/输出预算/压缩比/裁剪策略）继续继承 convert——避免出现"只多了一轮次、上下文却掉回 16K"的坑。显式写值照旧、写负数仍是"真正不限制"。

**P79（用户："对于那种在文章中成对的图左右线条和样式一般要求统一，在转换章节的时候"）**：先用真实章节量了一遍，确认"成对"不能靠行距判——一个围栏本身二三十行，相邻两张不同编号的图（图 8.1 与图 8.2）也只隔 31 行。真正的标志是**图注**：同一图号的 (a)/(b) 之间没有各自图注，只有 `(a)`/`(b)` 这种子图标注；不同图号的两张之间必有 `图 8.1`/`图 8.2`。于是规则定为"两个矢量围栏之间无小标题、无图注、相距 ≤60 行"。拿一本八章的书试跑：点出 5 对（chapter_005 两对、chapter_006 三对），而"图 8.1 + 图 8.2"那类没有被算进去。提示词只加一句（同图号的图彼此也要一致），具体是哪几对由代码在首条消息里点名，省得模型自己猜。

**P80 设计（用户："或者说如果有整体的话这边整体的线条可以有个统一性？比如由style会话的cls规定一些绘图的可选配置项"）**：P79 只治"同一图号的并排两半"，根子在于**线宽是作图会话逐图写死的**（每张图按自己那张原书的墨迹量出来），全书没有单一来源。于是设计成三层契约：cls 定义 `dv-*` 样式族与 `\dvfigwire` 等宏（取值来自 images 阶段聚合的 `source/figure_ink.json`，纯本地解析不花 token）；作图会话只写 `draw[dv-wire]` 并在围栏开头留 `\ifdefined\dvfigloaded` 守卫兜底（单图编译时用自己实测值，整书编译时类里的值赢）；convert 只把残留的 `line width=` 映射过去，preflight 出一句警告。分 S1/S2/S3 三步，S1 不动提示词即可验证"类能提供样式"，S3 对已完成的书一次性映射、不强制重画。设计稿在 `docs/plan-report/p80-figure-style-unity.md`。

**T328（用户："这个12776页的pdf这边已经处理了半个小时了没有任何进展，是不是程序写的有问题？"）**：是程序的问题，不是文件的问题。先量文件本身：`mutool info` 4.7 秒、取一页 0.47 秒、取 200 页 0.56 秒、整本 `mutool clean` 8.9 秒——文件健康、A4、204 MB。再看代码：`splitBySizeAndPages` 给每个分片先取一页探测（`TrimFile` 源文件）、再取整片探测、再二分 `log2(maxPages)` 次，每次探测都是**对源文件的全量解析**，最后才写真正的那一片。12776 页按 200 页一份是 64 份，等于对源文件做了两百遍全量解析。实测 pdfcpu 对这份文件取一页的 `TrimFile` 跑了二十几分钟还没回来（两个探测进程最后是我掐掉的），`api.SplitFile` 一次切 7 份也没在十分钟内回来——pdfcpu 在这个体量上就是不可用。改法两条：① 整本**一次读入**按页数切成若干份（`mutool merge` 每份一次调用，或 pdfcpu `SplitFile` 一次切开），② 超体积的分片**在它自己身上**对半再切（小文件，代价可忽略），彻底删掉"回源文件二分探测"这条路（`estimatePartSizeMB`/`splitBySizeAndPages` 一并删除）。`internal/split/splitter.go` 收口后端选择，`DOCVISION_PDF_SPLIT_TOOL` 可强制；日志打后端与用时。同一份文件现在 **58.1 秒**切完 7 份（2000/2000/…/776 页，合计 12776 页、174 MB），此前 >30 分钟零产出。测试：`TestSplitPDFBackendsBothWork`（两种后端各 4 份、页数合计对齐）、`TestSplitPDFRefinesOversizePartsOnTheirOwn`（上限压到一半后就地再切、manifest 页段连续覆盖整本），连同既有 9 条 split 测试全绿。

**P81 设计（用户："增加一个计划，就是这边提供一些现代的latex 3d和平面绘图的支持……首选就是lualatex的luadraw，通过lua代码更直接渲染3d关系和平面图。还有就是一些我们现在用的3dplot，luna tikztools3d，这些是可选使用……并且这个只要装了latex就可以用吧？"）**：先回答那句"装了 LaTeX 就能用吧"——**不是**。实测这台机器的 TeX Live 2026（LuaHBTeX 1.24.0）：`lualatex` 在、`tikz`/`pgfplots`/`tikz-3dplot`/`luacode` 都在，但 **`luadraw.sty` 与 `luadraw.lua` 缺失**；CTAN 上也**没有** `3dplot`、`3dtools` 这两个包（照着写 `\usepackage{3dplot}` 会当场编译失败），那一族的真实名字是 `tikz-3dplot` 与 `pgfplots` 的 3D。luadraw 本身（CTAN 页读到：Lua-based 2D/3D graphs for **LuaLaTeX**，输出用 TikZ 画、可自动存图，收录于 TeX Live 作为 `luadraw`，v3.7 2026-11-05）要求**整本用 LuaLaTeX**编译，并且得先 `tlmgr install luadraw`。管道侧勘明三处：引擎 `latex.compile.engine` 是全局单例（作图会话、章节 compile、整书 build 共用一份，所以"一章 luadraw、别处 xelatex"不可能）；`buildStandalone(body, engineIsXe)` 只在 `engineIsXe` 时补 `ctex`，判定该改成"需要 CJK（xe 或 lua）"，否则 lualatex 下中文图掉字；围栏里的 `\usepackage`/`\usetikzlibrary` 原本就会被 `figurepkgs.go` 抽出来、由 `preflightConvert` 点名样式类漏加载（T321），luadraw 直接走这条既有通路，**判据层零改动**。设计分 S1（引擎与能力探测：缺包就停在开工前并给一行安装命令）/S2（`allow_lua` 默认关、样式包声明后端 + `## Drawing toolkit`、作图提示词只在开关打开时才提 Lua）/S3（`\directlua` 静态 lint 禁 `io`/`os`/`require`——编译期 Lua 够得着文件系统，`-no-shell-escape` 拦不住；图代码用 lualatex 编、数值门槛与 `compare_figure` 全复用）。只写设计与台账，**未动代码**。

**T329（用户："我暂时有将这个 img2text 的流程……现在不是 [Image]() 这样的吗？可以在他的上面增加一行 [url](xxxx/xxxxx)的定位，其位置是 images/书名/xxxx.xxx，就是在输出的 finally 里面只有需要引用的图像才弄这个，那种 text 类的就不用"）**：老嵌入用 `[Image]( 描述 · P.N )` 整条替换掉原图引用，原图路径就只剩 `progress_items/<md>/original.md` 里那一份底稿，交付文件本身认不回来。改法：`embedBlockForRef` 在锚行**上一行**追加 `[url](images/<书>/<文件>)`，位置直接从原引用里取（`linkOfRef`：markdown 取 `](…)` 目标、HTML 取 `src`，容忍 `<>` 包裹与尾随 title），所以链接就是 md 里原本那条，不需要另配路径规则；只有 mermaid 与"其余视觉类型"这两支加，text/latex/math/table/code 直接嵌入正文的不加（用户明确"text 类的就不用"）。无引用的再嵌（空 ref）自然不给链接行。测试 `TestEmbedBlockFor`（视觉类与 mermaid 带链接行、文本类不带）、`TestLinkOfRef`（两种引用形态 + 无链接/title 边界）。

**T330（用户："我发现之前的页码定位算法存在漂移问题所以默认禁用了那个页码定位的算法，这边可以重新加回来，默认开启，这次使用一个正确的算法如下：……"）**：用户转来的是 skills 线 SK-I22 的规格，核心三条——页界在最早拿到逐页结构时钉死、合成时单遍消费 `curPage`、只留一套可见页码。老线（P21）的毛病对得上：标记只在页码**上升**时打（`pageMarkPositions` 只记 `p > prev`），中间对不上任何文本锚的空白页/整页图页被整段跳过；锚点页码与标记虽然同源，但改写走的是"按字节偏移替换 ref、再用每个替换的 delta 把标记位置平移回去"（`embedShift`/`insertPageMarks`），改写一多就得反复对账。改法（`internal/img2text/pagemark.go`+`runner.go`）：Final pass 改成**一遍行级扫描**（`rewriteFinal`）——先按 content_list 对齐得到每行真实页码，然后逐行输出：`pageMarkCoverage` 把"上一个已announce 的页 → 本行页码"之间**每一页**都列出来（缺页补齐、页码抖动不重复不回退），围栏内的标记顺延到围栏后（`fenceBodyFlags`+`markLines`），图片锚的 `· P.N` 与标记共用同一个 `curPage`；`embedShift`/`insertPageMarks`/`shiftFor`/`resolveFencePos`/`lastLineBefore`/`pageAt`/`applyTo` 整批删除，`audit` 多报一项「缺页」（日志自检行变成"越界 / 回退 / 缺页"）。`img2text.page_marks` 默认本来就是 `true`，用户那边关掉是配置层的事，代码侧现在给的是不漂移的版本。测试：`TestPageMarkCoverage`（1→4 补齐 2、3；非递增不重复）、`TestRewriteFinalCoverageFillsBlankPages`（缺页回归，审计 `Missing=0`）、`TestRewriteFinalFenceDeferral`、`TestRewriteFinalEndToEnd`（链接行 + 锚 + 标记同源）、`TestPlanPageMarksWithoutMinerU`；`go test ./internal/img2text/` 全绿，`./internal/...` 只剩既有的 `TestPythonEnvVenvRunsInsideRealSandbox` 环境失败（本机无 venv/libpython）。

**T331（用户："我知道我的意思我们不能从那个layout.json自己生成 md吗？我给你的那个人是怎么说的？"→ 选 2）**：错在我上一轮把"生成 md 时插标记"理解成"必须由 MinerU 生成"。先量可行性：从 `layout.json` 逐页重建 md、复刻 MinerU 的 writer 规则后是 13502/13505 行、678200 vs 678199 字节（99.98%），两处差异（带图注图片块顺序、中文 span 间距）来自版本差——现场 MinerU 是 3.4.4，对规则用的是 1.3.12 的 `ocr_mkcontent.py`，3.4.4 已重构成 `_render_span`/`_join_rendered_span`。所以"自己生成 md"成立，但那是长期跟上游 writer 跑的活靶子，而页界只需要"每页第一个块落在哪一行"。用户选了 2：正文不动、页界落 sidecar。实现：`internal/organize/pages.go`（`buildPageIndex` 按部件累加真实拼接换行算 base、页内首块整串相等优先、整页不中再用 24 字前缀包含兜底、图片块只比文件名；命中即 `ptr=start+1` 保证页序单调），`step2MergeParts` 写 `<书>.pages.json`（无 layout 也照写，表示"这版算过"；命中指纹但缺 sidecar 时重算），`img2text` 的 `planPageMarks` 先读 sidecar 展开每行页码（`Lines` 对不上即判过期），没有才退回 T330 的对齐。真实数据（2 部件 244 页）：219 页直接落点、页号单调 0 回退，未落点的 28 页由"页号连续"补齐。测试 `TestWritePageIndex`、`TestPlanFromPageIndex`。

**T332（用户："你能帮我看一下那个出问题了吗？…step \"split\" failed: 读取 PDF 失败: validatePageContents: page content must be stream dict or array, got: types.Integer"）**：不是切分算法的问题，是那份文档本身不合法——某页的 `/Contents` 指到一个整数，pdfcpu 读取时会校验页面内容流，于是 `api.PageCountFile` 直接拒，连页数都算不出来。现场复现：把该目录下 15 份 PDF 逐个过 `api.PageCountFile`，只有这一份报同一个错（586 页）；同一份文件 `mutool info` 正常给页数、`mutool merge` 切 1-3 页产出 1.5 MB 正常分片。改法：`pageCountFile` 按 pdfcpu → `mutool info` → `pdfinfo` → `qpdf --show-npages` 依次探测，全不中才回传 pdfcpu 原错误并附"装 mutool / 设 `DOCVISION_PDF_SPLIT_TOOL=mutool`"；pdfcpu 切分失败且机器上有 mutool 时就地换后端重切。另外提醒用户：他跑的是 `/home/share/samba-share/PDF2MD` 这套旧部署，里面的二进制早于 T328/T332，代码改完要他自己重新 build。

**T333（用户搬完四册后追问为何最终 md 在输入目录里）**：查代码确认是缺陷不是用法问题——`runAdHocWorkflow` 的 `destDir` 取 `filepath.Dir(abs)`（目录入参取该目录本身），临时模式下 `cfg.Paths.FinallyDir` 又被改写成 `<job>/finally`，配置里的 `paths.finally_dir` 全程没参与交付；于是每跑一册就在输入目录里多一份最终 md，而配置指定的 finally 目录一直是空的（这也解释了用户为什么得手动搬）。改法：`workflowDestDir(configuredFinally, srcDir)` 在 chdir 进 job **之前**把 `paths.finally_dir` 解析成绝对路径，配了就用、没配才回落源目录，交付前 `MkdirAll`。`TestWorkflowDestDir` 钉住三种情形（未配置/相对/绝对）。顺带记：本次四个 job 的 `finally/progress_items/.progress_stats.json` 是"外层单键包一个大 dict"的结构，按外层键合并得 0 条，四个 job 的缓存条目没并进共享文件；影响仅限 img2text 要重新 stat 一遍这些条目（逐项 json 都搬完了，不会重跑任务）。

**T334（用户问"带 [url] 的图片会不会复制到 finally"，然后："那就我希望带有 [url] 的这个对应的图片应该复制到 finally 里面"）**：先答清了现状——图片只在 `organize` 阶段进 `paths.images_dir`，img2text 一行都不搬；`finally/` 里只有 md + `progress_items/`，所以交付 md 的 `[url](images/<书>/<文件>)` 是相对路径、离开 `output/images` 就断。改法：新增 `finalimages.go`，`deliveredURLRe` 只认交付 md 里的 `[url](…)` 行（不顺带搬文本/表格那种直接嵌入的图），`resolveDeliveredImage` 先试 `<images_dir>/<去掉 images/ 后的相对路径>`、再试 `<output_dir>/<原路径>`、最后试 `<images_dir>/<原路径>`；目标 `<finally>/<原路径>`，同大小已存在跳过，绝对路径与含 `..` 的一律忽略，源找不到就跳过（md 里的链接本身还在，不因复制失败丢内容）。测试 `TestCopyDeliveredImages` / `TestDeliveredImageTargets`。

**T335（用户："我以后来外边的程序再去弄这些图的时候就丢失了原有像素图像的尺寸，没办法和 compare-figure 精确算一样的数据"，并选定"存原始量"）**：先确认这是信息丢失而不是算法问题——原图 mm 由 `content_list.json` 的 bbox(pt) 与 `layout.json` 的 page_size(pt) 换算，T334 只搬了像素。估过体积（229 册图片目录）：JSON 全量约 4.4 MB、CSV 约 1.2 MB（CSV 小 3.6 倍），结论是体积可忽略、按便利选，用户最终选"只存原始量"。实现：新包 `internal/imagemeta`（`SetMineruRoot` + `LookupImage`，扫 `<root>/*/_content_list.json` 的 image 条目取 bbox/page_idx，页尺寸从同目录 `layout.json` 取，优先不带 `_v2` 的那份）；`finalimages.go` 复制每张原图时按目标目录攒原始量，收尾只写 `sizes.csv`（file,page,bbox_x0..y1,page_w_pt,page_h_pt，按文件名排序；用户看过体积对比后定 CSV only，JSON 版删掉）；查不到解析的图不写条目。测试 `TestCopyDeliveredImagesWritesSizes`（含"不许再写 sizes.json"）。

**T336（用户："我不管bbox我也不管留白，程序只看到 img.jpg 320x450px，要能换回原始 pdf 的 32pt:45pt"）**：先量清了映射——位图宽 / bbox 宽 = px/pt，50 张样本中位 1.50，且**册内近乎恒定**（1.41–1.42、1.65–1.66、1.20 三档），跨册不同，说明裁剪比例是每册一个常数。于是把 T335 的 bbox 原始量改成纯换算表：注释行 `# scale_px_per_pt`（本册 `px_w/bbox_w` 中位数）+ `file,width_pt,height_pt`，宽高同除一个比例，位图比例保持。期间尝试过的"渲染原页量墨迹"方案（`internal/imagemeta/ink.go`）已按用户口径删除；顺带查清一个坑：`mutool draw -p N` 在这批扫描件上会渲染整本（26 秒 71MB），尾随页号 `… file N` 才是单页（134ms 364KB）。测试 `TestCopyDeliveredImagesWritesSizes`。

**T337（用户："图片引用截断"）**：交付 md 的 `[url](images/<书>/<文件>)` 在第一个 ASCII 空格或半角 `(` 处被切断（现网 6357 处，书名带空格或 `(下)` 的册几乎全坏）。两个解析点同源：`linkOfRef` 的正则 `\]\(\s*<?([^()\s>]+)` 与 `finalimages.go` 的 `deliveredURLRe` 同款坑——把路径当 token 取。改为 `linkTargetOf`：从 `](` 取到行内最后一个 `)`，剥 `<…>` 与尾随标题；`deliveredImageTargets` 改成逐行解析。补测 `images/某书(下)/a1.jpg`、`images/计算机网络（第8版） (谢希仁)/abc.jpg`（原有一条 `images/书/c d.png` 的期望值正是被截断的结果，一并纠正）。图片文件本来就在 `output/images/` 下，重新跑这几册的 img2text 即可回填。

**T338（用户："这边好像不是所有的书都生成了 csv…不重跑图片生成一下所有的 csv"）**：`sizes.csv` 原先只在图片处理出口写，旧册子不会再经过那里。新增 `img2text.WriteSizesForDir`（按盘上位图 + `imagemeta` 索引重算）与 `docvision img2text --sizes-only`（扫 `<finally>/images/*/` 逐册写，不调模型、不读 md）。现场回填 213 册 / 17925 张图；8 个目录因位图在 MinerU 索引里查不到而跳过。回填时发现两个口径问题记在案：① 每册按中位数定标，若同一本书被切成多个 part 且各 part 渲染 dpi 不同，单一比例会让部分行偏大（现场约 10% 行出现「宽度 > 页宽」，14 册比例落在 1.0–2.0 之外）——后续应按 part 定标；② 缺索引的册子只能等重跑。

**P81 实现与 T339（LuaDraw 迁移、缺失尺寸索引复核）**：独立功能分支接入 compare-figure 0.2.1 原生 Go LuaDraw 工具，内嵌 LuaDraw 3.7 运行库、官方 PDF/TeX 手册、Lua 源码和独立 Lua 启动脚本，保留许可证与来源。新绘图会话写 figure.lua，支持官方 luadraw 环境的 TeX；编译先做真实能力探测，LuaLaTeX 可用时优先，不可用时 Lua/texlua 输出 TikZ 再交 XeLaTeX/pdfLaTeX。luadraw_doc 包含 search/read/view/list/find/grep/frequent，跨会话共享索引对象、磁盘索引与高频段落缓存，PDF 预览按会话隔离；保留 Lua/自包含 TeX 编辑源，生成 TikZ 继续供全书排版，不要求全书改引擎。旧成品与旧 TikZ 工作区沿用。验证复杂 2D/3D 的直编和回退、文档检索与热缓存、提交代码和最后编译源一致、失败保留预览。

T339 证据纠正：八个缺 CSV 目录共 965 张交付位图，递归核查原始 content_list 后全部找到。五个目录的方括号被 Glob 误作表达式；两个目录有多份解析而索引只取第一份，实际图片在另一份；一个目录的四张图全是 chart，被 image 类型过滤漏掉。改为按字面枚举目录、合并各份 content_list、按路径+bbox 收图，不再按 image 类型排除。回归测试使用中立目录和文件名。T338 的“缺索引只能重跑”不成立；约 10% 宽度超过页宽是已观察事实，但“part DPI 不同”仍只是未经验证的假设，本批不据此改变尺寸算法，也不宣称绝对 mm 校准已证明。最终已将八份 CSV 写回并逐字节读回核对，共 965 行；编译预览从同一份自包含编辑源构建，生成 TikZ 经现有全书引擎再次编译通过。CLI 构建、帮助与命令测试通过；全量内部测试仅两项既有失败：用户配置示例与内置模板值不同，以及 Python 沙箱缺少 libpython3.14.so.1.0；此前排除这两项的全量测试通过。

**T340/T341（用户要求同步作图上下文上限并彻查 venv 沙箱失败）**：T340 将用户配置示例的 drawing.context_limit=256000 同步进内嵌初始化模板与配置加载缺省值；显式 0 继续保留旧含义，平台窗口更小时取较小值。回归覆盖字段缺席、空 drawing 块、显式 0、自定义值和模型上限；两份模板一致性测试通过。

T341 经真实失败复现、ldd、ELF RUNPATH 和沙箱内 LD_DEBUG=libs 查证：宿主 libpython 存在且能运行，库目录里的文件是相对软链；代码绑定软链所在目录及 EvalSymlinks 的最终物理目录，却遗漏相对软链目标的字面目录。安装前缀的目录别名在沙箱中没有重建，库因此断链；不是需要重装 Python。linkChainDirs 现在包含链终点的字面目录，sharedLibDirs 对依赖库也逐跳收集，再由原有逻辑绑定物理目录。中立路径别名测试钉住该缺陷，真实 system/venv 沙箱回归通过，venv 的 sys.prefix 与 setuptools 导入均正常。诊断脚本和动态加载器原始输出留在未跟踪临时区。最终使用仓库内可写 XDG_CACHE_HOME 运行 `GOCACHE=/tmp/dsh-gocache go test ./internal/...` 全部通过，无跳过筛选；CLI 构建与 cmd/docvision 测试通过，配置示例与内嵌模板逐字节一致。
