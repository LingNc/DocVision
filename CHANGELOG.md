# Changelog

> 版本号即 git 标签（`git tag`）：`v1.5.0-beta.N` 是 1.5.0 测试线，`v1.3.0-beta`/`v1.4.0-beta` 是 v1.3/v1.4 的重打测试标签。
> 每个小节的日期取该标签的创建日期；`v1.2.0` 未单独打标签（日期取该版最后一次提交）。用 `git show <tag>` 可查看对应提交。

## [Unreleased]

### Added

- **转录里的系统提示词快照（`t=meta` 元信息行）**：JSONL 转录一直不写 system 行（系统提示词由 `internal/prompts` 模板**每次运行重新渲染**——水印开关、挂载说明等是本次运行才确定的；工具定义由运行时按会话类型拼装），好处是恢复会话时不会 replay 一份过期提示词，代价是"看转录不知道模型被交代了什么"，只能靠 `--debug` 的请求摘要。现在 `Session.SetTranscript` 挂载转录时先写一条元信息行 `{"t":"meta","kind":"system","session_label":…,"model":…,"system_sha":…,"text":"<完整系统提示词>","tools":[{name,description,parameters}…]}`：**只记录、永不回放**（`LoadTranscript` 只认 `t=="msg"`，续跑语义不变，运行时仍以模板渲染的提示词为准）；系统提示词按 sha256 前 8 字节去重——同一份提示词重复挂载（续跑）不会写第二条，提示词变了（换模板/换版本）才追加一条；系统提示词为空时打 WARNING（历史事故：样式反馈会话曾以空提示词启动，转录里完全看不出来）；工具快照按 `Definition()` 取 `function.name/description/parameters`，就是当时发给 API 的那份。实测：3 个真实工具 + 短提示词 → 单行 2549 字节；39.6 KB 提示词 → 单行 42.7 KB、回放仍只看到消息。新增 `TestTranscriptMetaLine`（去重、回放只认消息、换提示词后追加）。

- **会话预览 `docvision sessions`（新包 `go/internal/sessionview`）**：把项目里所有 AI 会话的 JSONL 转录渲染成可浏览页面。`Scan(root)` 递归扫描任意层级的 `*.jsonl`（`work/style_session.jsonl`、`work/sessions/chapters.jsonl`、`work/sessions/convert_<章>.jsonl`、`source/sessions/vector_<书名>__<sha256>__<图>.jsonl` 等；跳过 `media/` 与 `.git`），按 mtime 倒序返回（活跃会话自然排在前面，同刻按路径定序）：相对路径 id、阶段标签（`style`/`chapters`/`convert:<章>`/`vector:<图>`，矢量图名去掉 sha 段并把 `_` 还原成空格）、中文显示名（样式/章节划分/转换 · <章>/矢量图 · <图>）、消息数（按 `t=="msg"` 计数，按 size+mtime 缓存，2 秒轮询不重复读盘）、字节数、mtime、以及「最近 60 秒内写入」的活跃标志。`ReadSession(path, fromLine)` 按行增量读取并返回 `nextFrom`，前端只取新行；坏行不中断读取（带 `bad` 标记与原始文本），文件末尾「写到一半」的行不计入 `nextFrom`，下次轮询重读而不会永久丢行。
- **两种查看模式，一套前端资源**（`assets/viewer.html|viewer.css|viewer.js`，`//go:embed`，无 CDN、无构建步骤、纯原生 JS）：`WriteStaticHTML(root, outPath, sessions)` 生成自包含 HTML——数据内嵌在 `<script type="application/json" id="dsh-data">`，样式与脚本一并内联（`json.Encoder.SetEscapeHTML` 保证转录里的 `</script>` 不会冲出数据块），图片按相对导出文件算出的路径引用，`file://` 打开即用、不轮询、显示「静态快照 · 生成于 …」；`Serve(root, addr, openBrowser)` 起本地只读服务（默认 `127.0.0.1:8848`，监听成功后才打印确切 URL），提供 `GET /`、`/api/index`（含每个会话的 size+mtime）、`/api/session?id=&from=`（返回 `lines` + `nextFrom`）、`/media/...`、`/file/...`，`id` 必须命中 `Scan` 结果否则 404，路径越界（`..`、`%2e%2e`）一律 400，目录不列举，非 GET/HEAD 返回 405——自建 handler 而不用 `http.ServeMux`，避免它把 `..` 变成 301 重定向而不是明确拒绝。
- **会话预览界面（后改版为黑白双主题 / 按项目分组，见下方 Changed）**：左侧会话列表（阶段标签、消息数、大小、相对时间、活跃圆点、按名字过滤）；右侧消息时间线——用户/任务卡片（长文本折叠，`images` 缩略图点击放大、Esc 关闭）、AI 正文、可折叠的思考过程（带字符数、折叠状态记忆）、工具调用（工具名 + 格式化高亮参数 JSON）与按 `tool_call_id` 配对编号着色的工具结果（默认前 8 行、可展开全文/复制，含 `error`/`REJECTED`/`文件不存在` 与 `ok (` 分色）；工具栏含「自动跟随」（实时模式默认开，手动上滚自动关闭）、「折叠全部思考」、「仅看工具调用」。转录里的系统提示词与工具 JSON Schema 现在由 `t=meta` 元信息行提供（见上），页面据此渲染快照卡片而不"假装没有"；转录不记录每条消息的时间戳，故每条消息显示序号与行号，时间只取侧栏相对时间与工具栏「最后写入」。坏行跳过并在顶部提示「跳过 N 行坏数据」。
- `docvision sessions` 命令（`go/cmd/docvision/cli_sessions.go`）：默认扫描**命令启动时所在的目录**（`--config`/全局配置导致的 chdir 不影响它）并生成 `<根目录>/sessions.html`；`--dir` 指定根、`--out` 指定导出路径、`--list` 在终端列会话（阶段/消息数/大小/修改时间/路径）、`--serve --addr` 起服务并打印 `会话预览: http://127.0.0.1:8848/`（不自动打开浏览器；端口占用时明确提示换 `--addr`）。
- **`doc_index` 图片条目带上内容（DOCVISION 注释回填）**：档位1 的块索引此前只读 MinerU 的 `content_list.json`，而 MinerU 对**没有 caption** 的图片块只给一个文件名——索引里那条 image 条目 `text` 是空串（实测 `latex_project/doc_index/doc_index.json` 242 条里 8 条 image，其中 4 条 text 为空），可是这 4 张图在 images 阶段加工过的 md 里明明已经有内容（`<!-- DOCVISION-STYLED-TEXT/VECTOR/IMAGE: <描述> -->` + `CONTENT:`/`DESCRIBE:` + `LINK: [class](path)`）。后果是 `doc_search "知识导图"`、`doc_search images/<主题>/<hash>.jpg` 都搜不到那几张图，`list_source_pages {page:N}` 的页详情里这些图既无文本也无描述，转换/样式会话只能靠 `view_image` 一张张看。现在：`DocEntry` 新增三个 `omitempty` 字段 `Marker`（`styled-text`/`vector`/`image`）、`Label`（注释里的描述）与 `Content`（STYLED-TEXT 的原文 / RASTER 的解释文本）；新增 `parseMdMarkers(mdDir, mdNames)`（逐行扫描、无正则、容忍注释缺失/字段换序/多行 `DESCRIBE:`/CRLF，`mdNames` 为空即返回空 map），`buildDocIndex` 多收一个注释目录参数并在算完 `Global` 后按**图片文件名**（`filepath.Base`，"images/<sha>.jpg" ↔ "images/<主题>/<sha>.jpg"）回填——匹配不上就留空、不报错；`buildDocIndexQuiet` 传 `<proj>/source`（不存在时回退输入 md 目录），日志增记带注释的条目数。呈现同步：`doc_search` 的检索 haystack 与命中输出、`list_source_pages` 页详情的图片清单都显示 `[vector] mind-map diagram` + 内容摘要，两个工具的 `Definition` 同步说明。旧 `doc_index.json` 仍可反序列化（字段 omitempty、索引每次运行重建）。

- **多项目工作区：工作区改为 `<输出根>/<项目名>/`，同一个 `latex_project/` 下可并存多本书**（用户实测：书 A 跑完再跑书 B，两本书互相覆盖、会话与进度串在一起）。此前 `paths.latex_project`（档位1）与 `paths.latex_output`（档位2）**本身**就是一个项目的工作区（`<root>/work`、`<root>/source`、`<root>/style`、`<root>/chapters`、`<root>/build`、`<root>/out`、`<root>/doc_index`、`<root>/progress.json`），所以一个输出根下只能住一本书；现在工作区是 `<root>/<项目名>/`（`latex_project/测试-概率论/`、`latex_project/线性代数/`、`finally_latex/线性代数/`…）。项目名默认取**主题名**——主 markdown 的主文件名，也就是 `images/<主题>/` 用的那个名字（实测 `测试-概率论`；一次跑多个 md 时取最大的那个，与 style 阶段/原书 PDF 视图同一取法），新增 **`docvision latex --project <名字>`** 覆盖；显式名与自动推导名走**同一套安全化**（`/` 与 Windows 保留字符换成 `-`、连续的 `.` 折叠成 `-`、去控制字符、trim 首尾空白/点/连字符，禁止 `.`/`..`/空串，中文与书名里的连字符原样保留，超长按 60 字截断），若结果撞上 `work`/`source`/`progress.json` 这类旧布局判据名则退回主题名（否则那个项目目录会让下一次运行把输出根误判成旧工程）。**决定项目根的地方只有一处**：新增 `internal/latex/project.go` 的 `resolveProjectDir(root, want, source)`（`Runner.useProjectDir` 是唯一调用者，`Runner.ProjectDir()` 供 CLI 定位日志分析目录），`work/`、`source/`、`style/`、`chapters/`、`build/`、`out/`、`doc_index/`、`progress.json`、`tempDir`、`buildPDFView`/`ensureProjectView`/`ensureChapterView`、水印记忆与 `pages/` 缓存全部由它派生。**旧版单项目布局继续原地可用**（判据：输出根含 `work/ source/ style/ chapters/ build/ out/ doc_index/ progress.json progress_items/` 任意一项；`progress_items/` 是档位2 自己的进度目录，不认它会让既有 `finally_latex/` 被判成空目录而全量重跑）：未指定 `--project` 时沿用输出根并记一条 info 日志「检测到旧版单项目布局 …继续沿用（既有内容不改动、原地可跑）；要在同一输出根下新建独立项目请用 `--project <名字>`，新项目会放在 `<root>/<项目名>/`」，且**不写项目标记、不动既有文件**；**显式 `--project` 时即使输出根里有旧工程也照样新建子目录**——这就是"在同一台机器上开第二本书"的出口。同名去重依据工作区里的 `.docvision_project.json`（项目名 + 主题名来源）：同名目录已属于**另一本**书时新项目退到 `<名字>-2`，无标记的目录（空的、手工建的、刚从旧布局整体搬来的）一律复用，保证同一本书重复运行落在同一工作区、断点续传不受影响。档位2 独立运行（`RunImages`）不再直接写 `latex_output` 根，同样进 `<latex_output>/<项目名>/`（`figures/ images/ tikz/ progress_items/`、逐图进度与 md 重建产物都在项目内），档位1 嵌套调用（`OutDir=<proj>/source`）不重复解析；`docvision verify` 新增 `--project` 并改按项目工作区定位（不指定时：输出根本身是旧版工程就用它，否则用其中**唯一**的子项目，有多个会报错列出名字，`progress_items/` 与 `verify_report.md` 都落在该项目内），顺带把代码里读、`--help` 里却没有的 `verify --progress-dir/--report` 两个旗标补上。实测迁移路径：`mv latex_project 测试-概率论 && mkdir latex_project && mv 测试-概率论 latex_project/`——旧工程（进度、会话转录、doc_index、out/ 全部）被新布局直接复用，不用重跑。

### Changed
- **工具回执与提示词瘦身（少即是多）**：工具回执里"看完就提交""和原图对比结构/标签/重叠/比例"这类说教全部删掉，只留事实——`compile` 成功只报产物名（`standalone.pdf`，含页数与产出尺寸）、失败只说"旧产物还在/尚无产物"，`view_image`/`view_pdf` 只报裁剪/缩放参数与实测尺寸，`image_context` 不再复述跨页续图规则（系统提示词已写）；提示词里凡是"工具箱已声明"的内容不再重复（档位1 转换会话的 `## Tools` 一节从逐条罗列参数压成"只有不显然的几条"，样式会话的 `view_pdf`/`list_source_pages` 描述压成一行），矢量图提示词的"先看图再写代码"新增强调**撤回**（工作流第 1 步与首次用户提示词都还原成 0909 的写法，模型本来就会先看图），{VIEW_BUDGET} 只报两个数字。

- **绘图（矢量）提示词收敛**：上一版新增的"比例/尺寸硬规则（553 字符）+ 线宽硬规则（442）+ 看图纪律 + 看图预算"把提示词从 4300 撑到 6055 字符（+41%），实测同一张图（bfea 思维导图）的会话从 0909 的 13 轮 / 首个工具调用 `view_image`（先看图）变成 21 轮 / 首个工具调用 `write_file`（先写代码），效果反而下降。现在把两条硬规则合并成两句口语要求——「整体比例与印刷尺寸**和原图差不多**」「线宽**和原图差不多**、字在该尺寸下要看得清」，删掉"最多看几次""看完就提交"的劝退措辞，并把工作流第 1 步改成"**先看清原图再写代码**"、首次用户提示词的开场从"Begin: write the TikZ code…"改成"Begin by LOOKING at the attached image…"（0909 版首次工具调用是 `view_image`，现在是 `write_file`，这一句就是诱因）（工具选型、body-only、跨页续图、标签忠实等规则不变）。
- **看图预算改为开头一次性告知**：预算数值（`tools.view.image_max` / `pdf_max`）现在写进矢量图会话的首次用户提示词（模板占位符 `{VIEW_BUDGET}`），不再等用满 `warn_ratio` 后**每一次**看图都在回执尾部重复"只剩 N 次"；提醒改为**每个对象只出现一次**（`viewBudgetNoteOnce`），超限也只说"已用尽"。同时删掉 `view_image` / `view_pdf` / `compile` 回执里"看完就提交"，"stop viewing and call submit"式的催促。


- **提示词集中管理**：19 段内置提示词（12 段系统提示词 + 7 段各会话的首次用户提示词）从散落的 Go 源码搬进 `go/internal/prompts/templates/*.md`（`//go:embed` 编进二进制），调用点统一为 `prompts.Must(Name)`（纯静态）与 `prompts.Render(Name, map[string]string{…})`（带占位符），替换逻辑只有一份 `prompts.Fill`。注册表为每个模板声明占位符清单、必须提到的工具名、不得出现的退役工具名。搬迁逐字节核对（脚本从旧常量/旧 `fmt.Sprintf` 抽取后与原文本比对），提示词自身行为不变。
- **提示词守护测试（4 条）**：① 模板文件 ↔ 注册表双向一致；② 按声明占位符全量渲染后不得残留 `{...}`（历史事故：`{OUTPUT_LANG}`/`{MAX_ROUNDS}` 原样发给模型）；③ 模板锁定的工具名必须在代码里存在、且代码里的工具必须在某处模板被提到；④ 退役工具名不得复活（`read_md`/`view_page`/`list_images`/`install_font`/`compile_preview`/`preview.png`/`format_fix_attempts`）。
- **会话预览的按项目分组改为按工程结构判定（跟上多项目布局）**：组名不再是"相对路径第一段"——多项目布局下每本书一个工作区，那样会让同一个输出根下的所有书挤成一个组，「这本书的会话在哪」就看不出来了。现在 `Scan` 会看磁盘：`latex_project/<书名>/work/sessions/convert_01.jsonl` 归到组 `latex_project/<书名>`（书名目录里有 `.docvision_project.json`／`progress.json`／`work/`／`source/` 之类的东西即认定为工作区），侧栏标题显示**书名**、输出根 `latex_project/` 弱化成灰色前缀；旧版单项目输出根（`work/`、`progress.json` 直接挂在它下面）仍是一组并标注「**旧版单项目**」，不会被误当成新布局的一本书；工程内部的结构名（`work`/`source`/`style`/`sessions`/`temp`/`views`/`pages`/`build`/`out`/`doc_index`/`reports`/`media`/`figures`/`images`/`pdfview`）永远不当组名，扫描根下的散落转录或用 `--dir <工程>` 直接扫描某个工程时归「（根目录）」。工作区判定按目录缓存（实时模式 2 秒轮询不重复 stat）。`docvision sessions --list` 的「项目」列同步显示新组名。新增测试：`TestProjectGroupMultiProjectLayout`（新布局两本书 + 旧版单项目 + 档位2 旧版 + 非工作区目录各归各组，且三本书的组名互不相同）、`TestProjectGroupScannedInsideWorkspace`（直接扫描工程本身时结构名不会被当成组名）、资源测试新增「容器前缀 / 书名主体 / 旧版标记 / 静态模式字段映射含 `projectLegacy`」守卫。

- **会话预览页改版：黑白双主题（默认白天）+ 左侧按项目分组 + 系统提示词快照卡片 + 思考呈现**。① **主题**：配色全部收进 CSS 变量——`:root` 就是浅色（`--bg: #f4f6f9` / 深字，正文对比度 ≥ 4.5:1），深色只是 `html[data-theme="dark"]` 的一层覆盖；工具栏右侧新增「🌙 深色 / ☀️ 浅色」按钮，切换只改根元素上的 `data-theme` 一个属性，选择写进 `localStorage`（`dsh.sessionview.theme`；`file://` 下取不到就静默回退白天），并在 `<head>` 里于首帧渲染前应用以免闪色；代码 / JSON / 思考内容一律等宽字体，浅色下的 JSON 高亮也重新取值（key/str/num/bool 全部 ≥ 5:1）。② **按项目分组**：`SessionInfo` 新增 `Project` 字段（`ProjectFor(rel)` = 相对路径第一段；`latex_project`、`latex_project_0909` 各成一组，`latex_project/<书名>/…` 仍归 `latex_project`，直接躺在扫描根的转录归「（根目录）」）；侧栏改为可折叠的项目组（`<details>`，标题 = 项目名 + 会话数 + 活跃圆点），**默认全部展开**，过滤时只保留有命中的组并强制展开（命中组标注「过滤命中」），折叠状态只存内存（点「刷新」重新扫描后保持，重开页面回到全部展开）；会话行保留阶段徽标/消息数/大小/相对时间/活跃圆点，并补上项目内子目录与「含系统提示词快照」提示，顶部继续显示扫描根路径。③ **系统提示词快照卡片**：转录里新增的 `t=meta` 行现在渲染成时间线**最前面**的一张可折叠卡片——摘要行「系统提示词（本次运行快照，不参与回放）」+ 模型 + `session_label` + `system_sha` 短哈希 + 字符数，展开后等宽显示提示词全文（带「复制」，长提示词在卡片内滚动而非撑爆页面），下方列出工具名与描述、每个工具的 `parameters` JSON 收在二级折叠里（默认收起）；一个转录有多条 meta 行时**只显示最后一条**并注明「共 N 条，显示最新」。为支撑它，`Scan` 新增 `MetaInfo{count,promptChars,model,sessionLabel,systemSha,tools}` 摘要（`readStat` 一趟扫描同时数消息与汇总 meta，最新一条胜出，按 size+mtime 缓存），`Line` 新增 `Kind/SessionLabel/Model/SystemSHA/Tools` 与 `IsMeta()`，`parseLine` 只在 `t=="meta"` 时填充、`renderLine` 把非 `msg` 行排除在消息序列外——因此 **meta 行不计入消息数**，会话行/工具栏改为显示「系统提示词 N 字符」；老转录（没有 meta 行）照旧正常显示，不报错也不显示空卡片。④ **思考呈现**：assistant 的 `reasoning_content` 默认折叠、摘要行写「思考过程 · N 字符」，展开后是等宽、低对比、保留原始换行的整段文本，放在 `max-height: 42vh` 的内部滚动容器里；「折叠全部思考」仍作用于时间线上所有**消息**（含增量追加进来的）；工具结果用左侧色条 + 底色与正文/工具调用分层。⑤ 工具栏整理成三组（状态徽标 · 显示选项 · 外观），会话标题旁新增阶段标签徽标。测试：`TestProjectFor`、`TestScanProjectsAndMeta`、`TestReadSessionMetaLine`、`TestStaticHTMLMetaAndGroups`、`TestViewerAssetsThemeAndMeta`、`TestServeAPIMetaAndProject`（另用 DOM 仿真跑通「默认浅色 → 点击切深色并落 `localStorage` → 项目分组/过滤/折叠记忆 → meta 卡片取最新一条且工具二级折叠默认收起 → 思考默认折叠」整条路径）。

### Changed

- **文档全面同步（本轮）**：`README.md` 按当前代码事实定点修补——「LaTeX 输出」整章（两档位真实行为、会话工具集、checker 三轮同会话 + 兜底重转换、终审会话与整树交付）、命令用法示例（latex 只收显式扩展名、裸命令自带前置、删除 `workflow --step latex/verify`）、档位1 嵌入骨架（注释首行闭合 + 字段在外 + `[class]` 链接、RASTER 的 `DESCRIBE` 块）、新增「会话沙箱与虚拟工作区（挂载表）」「断点续传（JSONL 转录）」小节、目录布局表（`work/pdfview`、`work/views/**`、`work/reports`、`work/sessions`、`work/temp` 等）、配置表默认值与新增键、环境依赖（latexmk / bubblewrap / SVG 后端任一）。
- `docvision latex --help` 重写：档位2 产物纯净（SVG 嵌入、无 `DOCVISION` 注释、text 只提取可见文本）、档位1 逐章私有工作视图 + 只读参考通道 + checker 三轮 + 终审会话、会话挂载表与 bubblewrap 沙箱说明。
- `docvision verify --help` 更正：核对**只在显式运行本命令时执行**（workflow/latex 自动流程都不调用；`verify.enabled` 仅作提示，不影响是否运行）。
- **会话 bash 配置归位到 `tools.bash`**：`latex.bash_sandbox` → `tools.bash.sandbox`、`latex.bash_max_output` → `tools.bash.max_output`（与 `tools.mermaid.*` / `tools.latex.*` 同级——描述的是**工具**本身，不是档位）。解析优先级 `tools.bash.*` > 旧 `latex.bash_*` > 内置默认（sandbox=true / max_output=5000）；旧键仍生效但启动会打印迁移提示；`setDefaults` 不再把默认值写进旧字段（否则会掩盖显式的新键）。

### Fixed
- **进度行之间多出一行空白**（用户实测：`[classify 8/8] 100.00% (failed: 0, running: 0)` 与下一阶段 `[process 2/8] 25.00% …` 之间空了一行）：终端里同时有两个"实时行"在写——阶段进度行（`liveProgress`，classify/process 各一个）与会话实时行（`liveLine`，每个 AI 会话一条）——会话实时行结束时用 `\r\x1b[K` 把当前行清掉（它本来就不该留在屏幕上），随后阶段进度行 `Close()` 只补了一个 `\n`，于是留下一个空行。现在 `liveProgress.Close()` 在终端里**重画最终一行再换行**（`\r\x1b[K<最终进度>\n`，谁清过都不留白），管道/重定向里最后一次 `render()` 本来就是整行输出、`Close()` 不再重复整行；`render()` 也按 stdout 是否为字符设备分流（终端原地覆写 + `\x1b[K` 清残影，管道整行输出，不再往日志里灌裸 CR）；会话实时行在管道模式下也不再额外补一个 `\n`。新增 `TestLiveProgressTTYCloseRepaints`（终端下只换一次行，正是那条空白）与 `TestLiveProgressPipeNoTrailingBlank`（管道下两阶段首尾相接只输出两行、空进度行不输出）。

- **会话预览的两个小缺陷**：① `docvision sessions --out` 传**相对**路径时按进程当前目录解析，而进程在读配置后可能已经 chdir 到 `~/.docvision`，于是相对路径落到那里并因只读报 `mkdir … read-only file system`；现在相对 `--out` 与 `--dir` 一样按**命令启动时**的目录解析。② 侧栏只显示会话名，扫描根下存在多个项目（`latex_project`、`latex_project_0909`…）时不好区分——每行现在带**项目标签**（相对路径第一段），过滤框提示改为"会话名或项目（如 latex_project_0909）"，于是过滤框既是名字搜索也是工作区切换。

- **会话转录的三个真实缺陷**：① **样式反馈轮没有系统提示词**——打回原样式会话时 `NewSession(..., "", tools, …)` 以为"系统提示已在持久化消息里"，但 JSONL 转录**从不写 system 行**，于是那一轮完全失去系统提示（模板、挂载说明、水印要求全丢）；现抽出 `Runner.styleSystemPrompt()` 供样式会话与反馈会话共用。② **转录里历史重复**——`saveSessionContext` 在已有实时转录的情况下把整段会话（含 system）再写一遍，文件里出现重复历史且夹着一条 system 行；现 `Session.HasTranscript()` 为真即跳过，仅在实时转录不可用时兜底；旧版单 JSON 上下文续跑时先把历史补写进新 JSONL，避免下次续跑丢历史。③ **压缩后的续跑丢掉"任务 + 最近 8 条"**——压缩的磁盘形态是「…中间段、最近 8 条、note」（消息实时追加），而回放从最新 note 起截断，恰好把压缩刻意保留的原始任务与最近上下文一起丢掉；现在写完 note 后再补写这两部分，磁盘回放与内存状态一致。
- **进度行与日志行互相覆盖 / info 行外泄终端**（详见下方"虚拟工作区与终端输出"条目）：`RunImages` 硬编码恢复 `SetQuiet(false)` 抹掉了整体静默，`
` 进度行不清行导致日志行叠在同一行。

- **虚拟工作区在磁盘上是坏的（会话反复报"文件不存在"的真因）**：`buildPDFView` / `ensureProjectView` / 逐章视图都用裸 `os.Symlink(src, link)` 建软链，而 `src` 来自配置的相对路径（`./mineru_output`、`./latex_project`）——软链目标是按**链接所在目录**解析的，于是 `<proj>/work/pdfview/<part>.pdf` 与 `<proj>/work/views/project/{source,style,chapters}` 全部**悬空**。表现：`list_source_pages` / `doc_search` 走内存里的视图所以正常，一旦落到文件系统就 `文件不存在: source:<part>.pdf` / `project:source/<md>.md`，模型只好反复猜名字（三个报错、三轮空转）。新增 `linkAbs`（绝对目标 + 幂等替换，真实文件/目录不覆盖）并用于 pdfview、project 视图、逐章视图与章节编译 scratch。
- **沙箱 bash 在默认配置下必然失败**：`WorkBashTool.sandboxArgs` 把挂载目录**原样**交给 bwrap，而 bwrap 自己解析源路径（不经过我们的 CWD），默认配置又是相对路径 → 每个会话的 `bash` 都以 `bwrap: Can't find source path latex_project/work/style` 失败，模型因此失去 `ls`/`grep` 能力并开始猜文件名。现在所有 `--bind`/`--ro-bind` 源与 `--chdir` 一律 `filepath.Abs`。
- **会话内部日志外泄到终端 + 进度行被覆盖**：`RunImages` 在结束处**硬编码** `SetQuiet(false)`，把 `RunBook` 设好的静默状态抹掉（全仓库仅此一处），于是 style/chapters/convert/assemble 全程的 info 级 `[tool:x] ok (N chars result)` 直冲控制台；而进度行用 `
` 重绘且不清行，日志行正好写在光标处 → 终端里两条内容叠在一行、反复换行留下残影（时间戳"倒流"即屏幕残留）。修法：① `RunImages` 保存并恢复原 quiet（新增 `logger.Quiet()`）；② `Logger.SetLiveLine` 让进度行可注册，任何控制台日志行之前先换行、之后重绘进度行；③ 进度行只在 stdout 是字符设备时用 `
` 重绘（管道/重定向/日志捕获下改为按 10s 节拍整行输出），并加 `[K` 清行。
- **`view_pdf` 找不到 PDF 时不给任何线索**：`compile` 曾**在编译前**就删掉上一次的 `standalone.pdf`，一次失败编译即抹掉唯一产物，随后的 `view_pdf {path:"standalone.pdf"}` 只回一句"文件不存在"，模型连试三次（含误写 `Standalone.pdf`）。现在编译失败不再删旧产物，失败回执直接说明"旧 PDF 还在/尚不存在"，成功回执写明"可用产物: standalone.pdf（figure.tex 只是正文，不存在 figure.pdf）"；`read_file` / `view_pdf` 的"文件不存在"一律附带**该目录可用文件清单**与**可用挂载点**（新增 `suggestInDir` / `suggestNear`）。
- **挂载点写法混用被误判越界**：模型把两种语法拼在一起（`source:/source/<part>.pdf`）时，`VFS.Resolve` 剥掉挂载点前缀后把绝对路径交给越界检查，回一句看不懂的"路径越界（绝对路径）"。现在容忍"挂载点前缀 + 同名绝对路径"的冗余写法（含 `source:/source/...`、`/source/source/...`）。


- **看图预算是硬拦截**（上一版实现把超预算直接变成"拒绝调用"）：改为**软预算**——`tools.view.image_max`（默认 30）/ `tools.view.pdf_max`（默认 25）/ `tools.view.warn_ratio`（默认 0.7，向上取整），用满 70% 起每次调用附带"已用 N/30，仅剩 K 次"，超出后提示尽快提交，**从不拦截调用**。计数粒度是**对象**而非整会话：`image_max` 按**图片文件**、`pdf_max` 按**文件+页**——整会话共用一个池子会让"几百页 PDF 的会话"过早耗尽额度（每页各有 25 次更符合直觉，也便于预估）。
- **原图没有绝对尺度**（模型只知道像素，于是把图放大到整页）：新增 `imagescale.go` —— 从位图回溯到 MinerU 解析目录（`content_list.json` 的 bbox + `layout.json` 的 page_size），算出**印刷尺寸（mm）**、有效 dpi、占版面宽度比例，并在每次 `view_image` 结果与作图会话的初始消息里回给模型（显示标准：mm 优先）。实测该测试图：36.9mm x 20.2mm、占页宽 21%、195 dpi、比例 1.82:1（与独立复算一致）。高度按位图自身比例换算（MinerU 块 bbox 不紧贴图，直接取 bbox 高度会得到 2.7:1 的矛盾比例）。
- **工具轮次只有硬上限**：`max_tool_rounds` 达到后立刻禁用工具，长任务常在收尾阶段被砍。现在有两级软限制——用满 `tool_rounds_warn_ratio`（默认 0.7）后每轮提醒剩余轮次；到达上限后仍有 `tool_rounds_grace`（默认 20）轮可调用，之后才禁用；提醒消息**原地替换**同一条，不膨胀历史、不破坏前缀缓存。
- **调试日志刷屏**：`logger.write` 对 debug/trace 也打印控制台，导致 `--debug` 时终端里 style 的会话日志与进度行交错重叠（进度行被覆盖）。现在 debug/trace **只写日志文件**，终端只保留进度行与告警；`--debug`/`--trace` 帮助文本同步说明。
- **压缩只有一级（且先付 AI 的钱）**：改为两级——先本地裁剪（超长工具结果裁成"前半 + 后 1/4"，`sessions.*.prune_tool_chars` 默认 **4096**——原定 8192，实测 325 条真实工具结果最长 6183 字符、p99 5046、**无一超过 8192**，8K 阈值等于失效；较早图片换文字占位并注明"需要时重新查看"，`keep_images` 默认 3），仍超阈值才调用 AI 摘要；token 估算补上系统提示词与工具定义（原来漏算，实测低估 5 万+ tokens）。
- **前缀缓存无法固定上游**：请求现在带稳定的 `user` 字段（`docvision-<会话标签>-T<线程号>`），网关（new-api 等）可用它做渠道亲和，把同一会话固定到同一上游渠道——厂商前缀缓存是按上游 key 分的，换渠道即 0 命中；debug 的 `[cache-probe]` 行同时打印 `user`，便于核对。
- **工具轮耗尽时摘掉整个工具块**：`max_tool_rounds` 用尽后 `req.Tools` 不再发送（只设 `tool_choice:"none"`）——厂商把 (system, tools, messages) 拼成缓存前缀，工具块消失会让其后所有 token 位移、**整个前缀缓存归零**（实测 prompt 37114→35362，差 1752 = 整个工具 schema）。现在工具块恒定发送，是否可调用只由 `tool_choice` 决定。
- **缓存命中不可观测**：`Usage` 新解析 `prompt_tokens_details.cached_tokens` 并在用量行输出 `cached=N(%)`；debug 下每请求多打一行 `[cache-probe] body=… head_sha=… tools=N tool_choice=… messages=…` —— 前缀头哈希变了说明是**我们自己**改了前缀，哈希没变而 `cached=0` 则指向网关把请求路由到了另一个上游（厂商缓存按上游 key/节点，不跨渠道）。
- **续跑丢系统提示词**：转录（JSONL）里没有 system 行，恢复时 `SetMessages` 整体覆盖 → 续跑会话**完全没有系统提示**。现在 `SetMessages` 总是把本会话的系统提示放回队首（转录里若有旧 system 行则替换，不重复）。
- **思维链不入转录**：`transcriptLine` 没有 reasoning 字段，续跑时历史思维链全丢（GLM 保留式思考要求完整回传，也是前缀缓存的前提）。现在转录写入/恢复 `reasoning_content`。
- **压缩会毁掉前缀且花冤枉钱**：旧实现把整段会话（含 base64 图片）塞进**一条全新的 user 消息**去要摘要 → 一个 100k+ token 的新前缀全额计费、图片重发；压缩后历史只剩 system+摘要，最近上下文全丢。现在：压缩请求改为"在原会话末尾追加一条指令"的增量请求（复用前缀、命中缓存），系统提示词 + 原始任务 + 最近 8 条消息原样保留，只压中间段；回放时忽略最近一次压缩标记之前的旧消息。
- **档位2 进度行"永远是 0"**：`[process 0/N] … running: 0` 只在任务完成时重绘（无 ticker），首个会话跑几十分钟期间看起来像计数坏了。现在 classify/process 进度行与 convert 一样带 5s ticker 自动重绘，计数器读写同锁（原来读侧无锁，属数据竞争）。
- **并发不可见**：控制台进度行不进日志文件，事后看日志无法判断当时并发几个会话。现在 classify/process/convert 阶段各写一条 `并发 N（latex.concurrency）` 日志行。
- **作图比例失真**（实测：原图印刷约 3.6×2.0cm、占页宽 21%，成品 10.85×8.15cm、占页宽 62%，宽高比 1.82→1.33）：提示词里"宁可画大一点"的反向引导已删除，改为硬规则（与原图同宽高比 ±5%、线宽必须随图形一起缩放、禁止整图 `\resizebox`），并给作图会话传入原图位图宽高比 + 编译回执报告产出 PDF 的 pt 尺寸/宽高比，模型可以自查。
- **看图空转**：单张图的会话可以无限 `view_image`/`view_pdf`（实测一张 284×156px 小图被看了 115 次，其中 100 次是 2400px 放大——放大不产生新信息）。现在矢量图会话有看图预算（`view_image` 12 次、`view_pdf` 每页 10 次，超预算返回"请提交"），`view_image` 的 `zoom` 上限与 `view_pdf` 对齐（6000px），工具回执明确写"若与已看过的一致就停止查看并提交"。
- **`doc_search` 的 bbox 单位说错了**：提示词教模型"bbox 是 PDF points，除以页宽得百分比"——实际是 MinerU 版面坐标系（约 2× 页宽点数，493×720pt 的页可到 ~986×1440），照做必然溢出 100%。现在工具直接给出可用的裁剪百分比（`left/top/right/bottom`），提示词写明单位不是 points。
- **提示词占位符未被替换**：作图提示词里的 `{OUTPUT_LANG}` 与 `{MAX_ROUNDS}` 原样进入 system prompt（日志里可见 `Respond in the document's language ({OUTPUT_LANG}) for any explanation`）。现在所有内置提示词统一走 `renderPrompt(prompt, tuning, lang)`：`{MAX_ROUNDS}` 取会话工具预算，`{OUTPUT_LANG}` 取 `options.output_language`（默认 Chinese）；新增 `TestBuiltinPromptsAreRendered` 守住所有提示词常量。
- **`docvision latex --all` 缺失标志注册**：代码一直在读 `cmd.Flags().GetBool("all")`（决定日志分析是"只看本次"还是"汇总全部历史"），但从未 `cmd.Flags().Bool("all", ...)` 注册，于是显式传 `--all` 会直接报 `unknown flag: --all`（自 7f3b7d0 起的潜在缺陷，该分支实际是死代码）。现已注册并出现在 `--help` 里。
- 文档与帮助文本里的历史遗留名清理：`view_page` → `view_pdf {path:"source:<file>.pdf", page:N}`、`view_source_page`、`list_images`、`read_md`、`install_font`、`compile_preview` 与 `preview-<n>.png`/`preview.png` 虚拟名、`mermaid_validation` → `tools.mermaid.validation`、`options.format_fix_attempts` → `img2text.format_fix_attempts`、`workflow --step latex/verify`。
- 配置模板注释与代码事实对齐（`config.example.yaml` + `default.yaml`）：`paths.fonts` 不再自动下载字体（按会话报告清单手动放入，编译经 `TEXINPUTS`/`OSFONTDIR`）、`latex.insert_image_description` 仅作用于档位2（档位1 raster 总带 `DESCRIBE`）、`latex.compile.raster_dpi` 只用于矢量图 PNG 回退产物。
- `AGENTS.md` 规则板块新增五条：文档同步（README + `--help` + CHANGELOG 三处同批更新、删名必须全局 grep 清理）、CHANGELOG 归属规则（`git log -S` 回溯到引入提交所在标签、节日期取标签创建日期、分类不得错位）、配置项一致性（默认值三处同步 + 注释不得描述已移除行为）、发布线纪律（不移动既有标签，仅同日未推送的笔误可 `-f`）、大改动后做只读文档审计再定点修补。

## [v1.5.0-beta.3] - 2026-09-10

### Added

- `latex.keep_temp_dirs` / `latex.keep_session_records`（默认 false，debug 日志下必定保留）：临时工作目录与会话转录的保留策略分开配置。
- **part 级页码定位**：`pdfView.LocatePart/FileByPart` + 构建期 `alignmentProblems` 校验；`doc_search` 命中直接给出 `view_pdf {path:"source:<file>.pdf", page:N}`；`list_source_pages` 优先按索引 part 定位。
- 临时工作目录改到 `<proj>/work/temp/`（不再用 `/tmp`），便于事后检查；逐章私有工作视图 `work/views/chapter_<章>/`。
- **参考通道（只读）**：`project:converted/<file>.tex`（其他章节已提交的 .tex + 资源目录）与 `project:reports/<file>.md`（其他章节的工作汇报）；会话转录仍不暴露；`edit_file` 新增 `Prefixes` 白名单（convert/style-fix 只能改自己的章节）。

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

### Fixed

- 章节附件（`chapters/<章>/`）现在会被软链进编译 scratch，`\input{chapters/<章>/xxx}` 在会话内 `compile` 也能通过。
- `doc_search` 的 "(part local pN)" 由 0-based `page_idx` 改为 1-based，与 `list_source_pages`/`view_pdf` 一致。
- **`project` 挂载点最小化**：不再暴露整个项目根，改为最小只读视图 `<proj>/work/views/project/{source,style,chapters}`（`ensureProjectView`）；bash 的 `/project` 与结构化工具的 `project:` 指向同一棵窄树，`work/sessions` 转录、`work/reports`、`doc_index.json`、`pages/`、`build/`、`out/` 两侧都不可见（新增 `TestProjectViewIsNarrow`）。
- **会话 bash 内核沙箱**（`latex.bash_sandbox`，默认 true）：会话 bash 通过 bubblewrap 运行，沙箱内只存在该会话挂载表的树（`/work` 可写、`/project`、`/source` 只读，宿主真实路径不可见，`--unshare-net`，`/tmp` 私有）；结构化工具与 bash 的路径后缀完全一致；bwrap 缺失时回退普通 shell + 警告。新增集成测试验证只读挂载被内核拒绝写入、工作区可写、宿主路径不可见。
- **原书 PDF 最小视图**（`pdfview.go`）：把本书的 `*_origin.pdf` 以干净文件名软链到 `<proj>/work/pdfview/`，只这一份进 `source` 挂载点；`sessionMounts(kind, workDir)` 统一各会话可见范围（tikz/chapters 只有 work；style/convert/book 还有 project + source）。
- **查看工具统一（删除 `view_source_page`）**：原书 PDF 以只读挂载点 `source`（`paths.mineru_output`）进入会话命名空间，用同一个 `view_pdf` 查看（`view_pdf {path:"source:<part>/<x>_origin.pdf", page:N}`），裁剪/缩放参数与看自己编译的 PDF 完全一致；`ViewPDFTool` 新增 `Mounts`（`Runner.bookMounts`：work 可写 + source 只读），`zoom_width` 作为 `zoom` 别名兼容旧用法。
- **`list_source_pages` 升级为原书索引**：无参数 → 源 PDF/页范围/全局页号表 + 从 OCR 版面推导的章节起点（书没有目录也能得到可用目录）；`{page:N}` → 该全局页的正文片段 + 该页抽出的图片文件名（可直接 `view_image`），形成"文本→页→图"闭环。
- **删除 `list_images`**：它只服务样式会话；现在样式/反馈会话改用 `list_source_pages` + 新增的 `bash`（cwd=work/style）与 `doc_search`（文本→原页），提示词同步为 list_source_pages → view_pdf(source:) → view_image 的路径。原始文档索引提前到 style 阶段之前构建。
- **编译纯化**：`compile {path:"figure.tex"}` 不再栅格化、不再回贴预览图（删除 `previewEntry`/`addPreview`/`preview-<n>.png`/`preview.png` 虚拟名、`ViewImageTool.Previews` 与 `zoomFromPDF`），只返回日志 + 产物名 + 页数，看图统一 `view_pdf`；`view_image` 只用于原图。
- 修正样式修复子会话的章节工作根（此前误传样式工作区 `work/style`，实际应为 `proj/work`）。
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
- **上次失败的图片重跑被整个跳过**：档位1 的 images 阶段此前有 phase 级 done 标记（`progress.json` 的 `images: "done"`），重跑全书时该阶段直接跳过——上次 API 失败留下的 fallback（可重试）图片永远不再重试，控制台看起来"直接完成"。现在 images 阶段不再做 phase 级跳过：`RunImages` 自身就是增量的（done 跳过、fallback 重试、未分类补跑），phase 标记仅作展示
- **进度行 skip 计数语义模糊**：`[classify]/[process]/img2text` 进度行去掉 `skip: N`；断点续传时进度直接从已完成数起跳（如 `[process 5/13] 38.46%`，与 `Already done: 5` 呼应），运行中的任务数仍由 `running: N` 实时显示

## [v1.5.0-beta.2] - 2026-09-09

### Added

- **流式接收（默认开启）**：新增 `models.<name>.stream`（不写即 true），AI 请求改用 SSE 流式接收（`internal/chatstream` 统一组装 content/reasoning/tool_calls/usage），长思考/长输出期间持续有进展不再静默；流式模式自动带 `stream_options.include_usage`；厂商不支持流式（HTTP 400/404/405/415/422 或提示 stream 不支持）时自动回退一次非流式请求
- **流式超时语义**：非流式仍是 `api_connect_timeout + api_timeout`（默认 60+400s）整次请求上限；流式不再设总上限，改用 `models.<name>.api_stream_idle_timeout`（默认取 `api_timeout`）判定"两个数据块之间的最大间隔"，长但不间断的流不会被杀
- **思考控制配置**：`models.<name>.thinking`（顶层 `{type: enabled|disabled}`，GLM-4.5+/DeepSeek）与 `models.<name>.reasoning_effort`（顶层 max/xhigh/high/medium/low/minimal/none，GLM-5.2+），随 `request_body` 之后合并到请求体顶层，显式配置优先；`setup` 严格校验取值
- **调试日志增强**：每次请求/响应记录实际生效的模型、stream、max_tokens、temperature、thinking、reasoning_effort、消息数/上下文估算、耗时、finish_reason、输出与思维链字符数、provider token 用量（含 `reasoning_tokens`）；流式期间每 10s 输出进展行；img2text / verify 也支持 `--debug` 与 `options.log_level: debug`（此前只有 latex 命令生效）
- **`models.<name>.max_tokens` / `temperature` 兜底生效**：会话或单次调用未指定时使用该模型的取值（此前这两个键被继承但无人读取）
- 会话单次最大输出说明：`latex.sessions.*.max_tokens` 为单次请求 `max_tokens`（不含厂商单独计费的思维链预算），style 会话内置默认 32768（不再由 book.go 硬编码覆盖用户配置）

### Changed

- **编译预览可放大细看（复用 view_image）**：`compile_preview` 的每张预览图都留存为会话内的 `preview-<n>.png`（并保存对应 `preview-<n>.pdf`），`view_image {path:"preview.png"}` 取最新、`preview-<n>.png` 取历史版本，配合 `left/top/right/bottom` 百分比裁剪与 `zoom` 目标宽度即可细看小字号标签/箭头/重叠；`zoom` 时**直接从 PDF 以更高分辨率重渲染**（pdftoppm `-scale-to-x`，上限 6000px），而不是把已有像素拉大，因此放大是真清晰。未编译、名称写错、序号越界都会返回明确提示
- **classify 起始进度行**：`[classify 0/N] 0.00% (failed: 0)` 在进入分类阶段立即打印（此前要等第一张分类完成才出现），与 process 阶段一致
- **日志等级 info / debug / trace**：`options.log_level` 新增 `trace`，命令新增 `--trace`。debug 只保留每轮请求/响应摘要、提示词、工具调用与**最终接收内容**；流式分片进展行等噪音降到 trace。`options.log_level` 取值错误由 `setup` 校验
- **PDF→SVG 多后端回退**：`dvisvgm → pdftocairo → mutool → inkscape` 依次尝试，成功后记录所用后端；全部失败时 ERROR 日志列出每个后端的具体原因（不再是裸 exit status）
- **LaTeX 编译警告反馈**：编译结果新增 `Warnings` / `WarningCount` / `WarningSummary`（Overfull/Underfull box、LaTeX/Package/Class Warning，去重并限长），随 compile_preview / compile / recompile 的工具结果一并返回给 AI；debug 日志新增 `[compile:<preview|chapter|book>] OK|FAILED (耗时) warnings=N` + 警告清单 + 失败时给 AI 的错误原文（完整编译日志仍然不进上下文/日志）
- **Mermaid / LaTeX 校验结果进 debug 日志**：`[validate:mermaid]` / `[validate:latex]` 记录 has_blocks / valid / available / 精简后的错误
- **作图禁止重叠/拥挤**：latex 作图提示词新增硬规则与提交前检查项（标签不得压线/互相遮挡、节点不得重叠或越界，空间不足就整体放大/增间距/按比例缩小字号），img2text 提示词同步
- **图片工具可直接用文件名**：`view_image` 以**当前文档的图片目录为根**直接拼接（`foo.jpg`、`subject/foo.jpg`、`images/subject/foo.jpg` 都落到同一路径），**不做任何搜索**——找不到就报错（说明名字写错了），避免跨文档误命中；`image_context` 同样接受裸文件名（按唯一基名匹配，歧义报错）。提示词只保留一句"直接用文件名"，删掉冗余的路径说明与重复的重叠规则（重叠要求只留在 Rules 一处，省 token）

### Fixed

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
- `view_image` 首次调用失败：图片引用按 markdown 原文 `images/<主题>/x.jpg` 传入，而工具根目录通常是 images 目录本身，导致"文件不存在"；现在两种形式（带/不带 `images/` 前缀）都能解析，错误信息也提示可省略前缀
- `latex.sessions.checker` 的调优此前被忽略（`_ = LatexSession("checker")`）：现在真正生效，且未配置字段继承 `convert` 块；checker 请求也不再发送 `max_tokens: 0`
- 配置版本 5→6（新增 models 层 stream/thinking/reasoning_effort/api_stream_idle_timeout 等键）

## [v1.5.0-beta.1] - 2026-09-08

### Changed

- img2text 作图规则放宽：不再限定 TikZ——Mermaid 处理其擅长的图型，其余（几何、函数/坐标图、复杂表格、混合结构）可使用 TikZ/pgfplots/tabular 等任意 LaTeX 方式
- `--version` 输出版权（绫袅 LingNc）与仓库链接；Makefile VERSION 自动取 git describe（不再固定 dev）
- 构建产物 `logs/` 移出版本库并加入 .gitignore
- 水印工作记忆：remove_watermark 开启时，流程最开始做一次性检测（view_page 全览页渲染 + 全文档重复图片引用统计 + markdown 采样），AI 判定水印文本形态与被裁剪成图的水印/广告图引用，结果缓存为 latex_project/watermark_memory.json（两档位共享）；之后作为小工作记忆注入 classify/img2text 文本提取/作图/style/convert/checker 全部会话，水印图片引用在分类阶段直接预剔除（absorbed），不再重复处理
- 档位2 跨页图表拼接：作图会话新增 `image_context`（查任意图片的上下文与前后图片引用）与 `view_image`（查看图片）工具；提示词引导识别跨页续片（重复表头/"续表"/边缘截断等），一次绘制合并图并在 submit 声明 `merges`；被吸收的续片引用在重建 markdown 时自动删除且不再重复处理
- latex 档位2 控制台进度：默认每个阶段显示一行实时进度（classify/process），逐图明细只写日志文件；`--verbose` 恢复详细输出
- `latex.remove_watermark`（默认 false）：开启后样式分析 AI 会在使用手册中标注水印模式并指示排除，转换 AI 跳过水印内容，核对 AI 不把水印缺失报为问题
- **配置 v2（不兼容清理）**：新增 `config_version: 2`，版本不符时启动警告并要求参考模板更新；移除全部向后兼容层——顶层 `ai:` 块、`resolveAIReference`、`options.*`/`img2text.*` 中的 api_timeout/api_connect_timeout/api_max_retries/rate_limit_retries（统一收敛到 models 层）、废弃的 `latex.output_dir`/`latex.project_dir`；`models.text` 成为强制基础条目（setup 校验必填）
- checker 会话不再继承 convert 的会话配置（独立可调）；模板 `checker_model: "checker"` + `models.checker: Qwen/Qwen3.6-27B`（小文本模型）
- **档位1 图片内嵌**：全书流程 images 阶段改用 inline 模式——矢量图以 tikz 代码块直接内嵌进 markdown，转换 AI 将代码原样粘贴进 .tex（不再生成/引用 figures/*.pdf 资源）；raster 保留原图引用由 includegraphics 处理；SVG 转换仅在档位2 执行
- API 请求控制参数（api_timeout/api_connect_timeout/api_max_retries/rate_limit_retries）从 img2text/options 迁移到 models 层：任何 models 条目可设置，留空继承 models.text，最终回退代码内置默认（400s/60s/3/100）；旧位置仍解析兼容。session 会话客户端与 img2text 客户端统一使用同一套参数，重试均带指数退避
- rate_limit_retries 默认从 0（无限+代码安全上限 100）改为直接取安全上限值 100
- 结构化 JSON 输出请求（图片分类、章节核对、校验报告）统一附加 response_format={type: json_object}，保证返回一定是 JSON
- latex md 参数不再硬拒绝：output/ 中的 md 直接选用，其他位置的 md 复制进 files/ 后整理进 output/（用户路径里带 output 不会误伤）
- img2text 嵌入前自动留存原版 markdown 到 `finally/progress_items/<文件名>/original.md`（每文件仅首次），嵌入结果可随时还原

### Fixed

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
- latex 档位2 重试进度：fallback 重试条目不再被 classify 阶段虚报计数（已带分类结果的直接进 process）；process 阶段开始时先打印 `[process 0/N]` 起始进度行，长会话处理期间进度可见
- latex 前置 Organize Files 汇总（[4/4]）按选中书目过滤：单书 latex 流程不再罗列全树全部 md 与图片目录
- processPhase 的 panic 恢复原为普通语句（recover 不生效，会击穿整个进程），改为 defer 内调用
- **档位1 原始文档检索**：转换会话新增只读 `doc_search`（MinerU content_list 加工的块索引，关键词/图片名/页码检索，返回全局页号与 bbox）并可复用 `view_page` 按需渲染原始 PDF 页（与样式阶段共享缓存）；MinerU 产物缺失时自动降级
- `tools:` 独立配置块（v3）：get_more_context / image_context 的上下文参数与 mermaid/tikz 校验参数从 img2text/options 迁出，全流程共用；新增 `image_locate` 工具（返回前后图片引用的行号与 ±行数差，轻量定位后再按需扩展）

## [v1.4.0-beta] - 2026-09-07

### Changed

- img2text 支持 TikZ：提示词新增 tikz 类型，`options.tikz_validation`/`options.tikz_engine` 用 LaTeX 编译校验（失败自动回炉修复）
- img2text 输出嵌入格式按类型分流：纯文本/数学公式/表格/代码直接嵌入正文（无包装标记），mermaid/tikz 代码块直接嵌入，其余视觉类型用 `[Image]( 描述 )`——不再使用 `<!-- IMG -->`/`[AI]` 包装（**注意：v1.3.0 及之前的 finally/ 输出格式不变，仅新生成内容使用新格式**）

## [v1.3.0-beta] - 2026-09-07

### Changed

- 档位1 样式分析虚拟工作区（`latex_project/work/style/`）：write_file 增量起草，submit_style 可引用文件而非全量重发
- 字体管理：`paths.fonts` 目录 + `list_fonts`/`install_font` 工具（样式分析与终审修复会话可用），缺失字体标注替换方法
- 每章核对 AI：`latex.checker_model`（默认用 convert_model，可为小文本模型），逐章比对产物与原 md，问题回炉一轮，遗留记录 `.checker` 备注
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

## [v1.2.0] - 2026-09-07

> LaTeX 输出首版（档位 1/2）。该版本未单独打标签，内容包含在 `v1.3.0-beta` 之前的主线提交中。

### Added

- `docvision latex`：档位2（默认）图片矢量化——分类 AI 逐图标记 text/vector/raster；text 纯内容嵌入文本流（去除 [AI]/[IMG_TYPE] 标记）；vector 由作图 AI 会话写 TikZ，自动编译+栅格化 PNG 视觉核对+确认提交，编译 PDF 以矢量图嵌入；raster 保留原图（可选嵌入 AI 解释文本），矢量失败自动回退并告警
- `docvision latex --level 1`：全书 LaTeX——样式分析 AI（图像裁剪放大/读 md 工具）产出 cls+结构化使用手册+案例并自动试编译；章节划分 AI（grep + 最小 bash 沙箱，虚拟单文件系统）行号划分并全覆盖校验；转换 AI 并发逐章转 .tex（虚拟文件目录，只读他人/只写自己）；汇总多文件 .tex 编译全书 PDF（修复会话兜底）+ 单文件 standalone.tex
- AI 会话基础设施 `internal/session`：可复用多轮会话引擎，可分离工具注册（compile_preview/submit/grep/bash 沙箱/受限读写），可配置上下文窗口（默认 128K，支持 64K/256K 等），达到阈值自动 AI 压缩会话历史（保留关键决策与成果，丢弃草稿/工具噪音）
- 模型注册表 `models:`：每个专用 AI（classifier/drawing/style/chapter/convert/verifier）独立配置 base_url/api_key/model/request_body，空字段继承顶层 ai
- `docvision verify`：AI 核对每张图与嵌入内容（含 TikZ 渲染预览对比），输出问题与修改意见报告（`verify.enabled` 默认关闭，先上日程）
- 断点续传：档位2逐图进度（progress_items），档位1逐阶段进度（progress.json）
- latex 会话日志独立落盘 logs/latex_*.log

### Fixed

- `--config` 传入相对子目录路径时 chdir 后解析错位（改用绝对路径）

## [v1.1.0] - 2026-09-06

### Added
- `install` / `uninstall`：一键安装到 `/usr/local/bin`（无权限回退 `~/.local/bin`）并初始化 `~/.docvision/`；卸载保留配置
- `setup`：自动选择编辑器（`$DOCVISION_EDITOR`/`$EDITOR` 优先，回退 vim → nano → vi）编辑生效配置；保存时严格校验（YAML 语法、未知配置项、类型、必填项、占位符），不通过可回车重编或 q 退出
- 配置查找顺序：`--config` > 当前目录 `config.yaml` > `~/.docvision/config.yaml`（缺失自动从模板创建），任意目录可直接使用
- `workflow <path>` 临时模式：处理任意位置的 PDF/DOCX 文件或目录，中间产物存于 `~/.docvision/jobs/<名称>-<时间戳>/`，仅将最终 `.md` 输出到源文件所在目录

### Changed
- GitHub Actions release 工作流支持 `workflow_dispatch` 手动补发任意标签

## [v1.0.1] - 2026-09-06

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
