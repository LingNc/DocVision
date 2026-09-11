# Go 命令行说明

> 返回 [README](../README.md)（文档索引见 README「文档」一节）。


```
docvision workflow    运行完整流水线，或 --step 指定单步（split|mineru|organize|img2text|analyze）
docvision split       分割 PDF/DOCX（单文件或 --all 目录模式）
docvision mineru      调用 MinerU API 解析文件
docvision organize    整理解析结果
docvision img2text    AI 图片转文本（--test 测试模式）
docvision latex       LaTeX 输出（可直接传 PDF/DOCX 自动补前置流程，或传 md 名只处理指定文件）
docvision verify      AI 核对输出与原图（默认关闭；只能显式运行，不参与自动流程）
docvision sessions    会话预览：把 AI 会话转录渲染成可浏览页面（静态导出 / 本地实时服务）
docvision analyze     分析日志（--progress 仅进度，--all 汇总历史，--logfile 指定日志）
docvision splitlog    按线程 ID 拆分日志（--logfile / --output-dir）
docvision init        生成配置模板
docvision install     安装到系统路径（/usr/local/bin，无权限时回退 ~/.local/bin）并初始化 ~/.docvision
docvision setup       编辑配置文件，保存时校验语法与配置项
docvision uninstall   从系统路径移除已安装的 docvision
```

## 切分缓存（split manifest）

`split` 会在 `split_files/` 里为每个源文件写一份 `<源文件名>_split_manifest.json`，记录源文件大小/修改时间、切分参数（页数/大小上限）与每个分片的文件名、页码区间、字节数；下次运行只要这些全部对得上就直接跳过昂贵的 pdfcpu 解析与切分，输出 `[跳过] <文件>: 命中 split manifest (N 个部分)，跳过 pdfcpu 解析`。DOCX 同理，另有一种 `mode=passthrough` 的 manifest 记录"页数够小、无需 LibreOffice 转换"。任何一处对不上（源文件改了、分片缺失或大小不符、参数变了、manifest 损坏）都按缓存未命中原样重切，不会用陈旧结果；源文件被切分并归档进 `done/` 后，manifest 仍留在 `split_files/`。

> 缓存键里的源文件路径**分平台无关**：同一个文件在 Windows 上是 `files\书.pdf`、在 Linux 上是 `files/书.pdf`，两边写的 manifest 现在互相认（反斜杠/斜杠、`.`、重复斜杠都折叠成同一种写法）。在此之前缓存键是原样字符串，于是同一份 Samba 目录在 Windows 和 Linux 上轮流跑时**每个平台都会把对方的缓存全部作废**、整库重新切分一遍——看起来就像"两个平台的 img2text 不一样"，其实切分与 img2text 代码完全相同（`runtime.GOOS` 只出现在测试、`install` 与浏览器打开这几处）。

## img2text 嵌入格式

> **可还原**：嵌入发生前，原版 markdown（图片引用完整）会自动留存到
> `finally/progress_items/<文件名>/original.md`（每个文件只存首次版本），
> 需要回滚时直接复制回去即可。

AI 结果带 `[IMG_TYPE: <类型>]` 标签，写入 `finally/` 的 markdown 时按类型选择嵌入方式：

| 类型 | 嵌入方式 |
| --- | --- |
| text / latex（数学公式）/ table / code | **直接嵌入正文**（无任何包装标记，方便后续 AI 检索阅读） |
| mermaid / latex | 代码块直接嵌入（latex 代码块经过 LaTeX 编译校验，失败自动回炉修复） |
| 其余视觉类型（截图/照片/复杂图等） | `[Image]( 可读描述 )` |

latex 代码块校验由 `tools.latex.validation`（off/auto/strict，默认 auto）与 `tools.latex.engine`（默认 xelatex，自动回退 pdflatex/lualatex）控制；`[IMG_TYPE:]` 标签本身仍保留在进度数据中用于统计与断点续传。

## latex 档位2 文本图嵌入

`docvision latex` 的 text 类图片**只提取图片里的可见文本**（专用提示词：公式→LaTeX 数学、表格→Markdown 表格、无文字→保留原图），不会把 AI 的描述性文字写进 markdown：

| 档位 / 情况 | 嵌入方式 |
| --- | --- |
| 档位2（任何文本图，含 styled） | 提取到的文本**直接嵌入正文**，不保留样式、不加标记（档位2 只做"图→文字 / 图→LaTeX"） |
| 档位1 · 普通文本图 | 提取到的文本直接嵌入正文 |
| 档位1 · 样式化文本图（styled） | 注释**首行即闭合**：`<!-- DOCVISION-STYLED-TEXT: <描述> -->`；注释**外**各占一行 `CONTENT: <图片原文>` 与 `LINK: [styled-text](images/…)`。转换会话按手册重排 CONTENT 或直接 includegraphics LINK，注释与字段都不得进入 `.tex` |
| 档位1 · raster 原图 | `<!-- DOCVISION-IMAGE: <label> -->`（首行闭合）+ 注释外一行 `DESCRIBE: <解释文本>` + 注释外一行 `LINK: [image](images/…)`；档位1 在 process 阶段**总是**为 raster 图生成解释（复用 img2text 提取，每张一次视觉调用，不依赖 `insert_image_description`——那个开关只作用于档位2），提取不到文本时只写 LINK 行 |
| 提取不到文本 | 档位2 保留原图链接；档位1 只写 `LINK: [image](…)`，都不丢内容 |

档位1 的矢量图用同一套骨架：`<!-- DOCVISION-VECTOR: <label> -->`（首行闭合）独立一行，注释外一行 `LINK: [vector](images/…)` 位于 latex 围栏上方；注释与字段同样不得进入 `.tex`。fallback（矢量转换失败）保持原图并在档位1 就地标注 `<!-- DOCVISION-ERROR: … -->`。原图没能就位时注释会带上 `| 原图未就位（见日志：复制原图失败）`，**不会**把注释与 LINK 一起丢掉（`doc_index` 靠这些注释回填图片条目）。

## 日志分析（analyze）

工具调用统计同时解析 img2text 的 `[ToolCall]` 行与 latex 会话的 `[tool:<名称>] ok/error` 行，并按线程归属到对应会话；错误分类识别 `IMG_*` 与 `SESSION_*` 两类哨兵。

## img2text 测试模式

```bash
./build/docvision img2text --test                  # 随机 10 张
./build/docvision img2text --test --number 5       # 随机 5 张
./build/docvision img2text --test --seed 42        # 固定随机种子
```

## analyze 选项

```bash
./build/docvision analyze                          # 分析最新日志
./build/docvision analyze --progress               # 仅显示进度摘要
./build/docvision analyze --all                    # 汇总所有历史日志
./build/docvision analyze --threads                # 显示线程详细统计
./build/docvision analyze --percentiles 90,95,99   # 自定义百分位
./build/docvision analyze -o report.csv            # 导出 CSV
./build/docvision analyze -r 1                     # 分析上一次的日志（0=最新, 1=上一次...）
./build/docvision analyze -r 1-3                   # 分析上一次到前3次的范围
./build/docvision analyze -l +2d                   # 分析最近2天内的日志
./build/docvision analyze -l 2026Y9M1D-2026Y9M2D   # 按日期范围（含整天）
./build/docvision analyze -l 2026-09-01_15:30-     # 从某时刻到现在的日志
# analyze 默认即输出“本轮写入 finally 的文件及成功率统计”，分析前会列出选中的日志
```

## 会话预览（docvision sessions）

把项目里所有 AI 会话的 JSONL 转录渲染成像聊天软件一样的可浏览页面。扫描范围是根目录下**任意层级**的 `*.jsonl`：`work/style_session.jsonl`（样式）、`work/sessions/chapters.jsonl`（章节划分）、`work/sessions/convert_<章>.jsonl`（单章转换）、`source/sessions/vector_<书名>__<sha256>__<图>.jsonl`（档位2 矢量图）等。

扫描根下可以并存多个项目，因此左侧会话列表**按项目分组**：一个项目 = 一个可折叠集合，标题带会话数，点标题折叠/展开。分组按**磁盘上的工程结构**判定，而不是死板地取路径第一段：

- **调用过程一眼可读**：工具栏的「仅看工具调用」只留工具调用与结果（排掉对话与思考），适合顺着调用链看一个会话干了什么；每条工具调用卡片按**工具家族**着色（`bash`/`compile` 橙、`write_*` 蓝、`read_*`/`view_*` 绿、`grep`/`doc_search`/`list_*` 紫、`submit` 红），折叠状态下也带**一行摘要**（命令首行、`path`、`pattern`、查询词，超 90 字符截断），展开才是完整 JSON（带复制按钮）；工具结果卡片与调用按 `#序号` 配对并标出状态（`ok`/`error`/普通）。这一套样式对照 DSH 自己的会话界面做的（左侧树 + 中间调用卡片）。
- **侧栏是三层：项目 → 流程阶段 → 会话**。项目组头显示书名与进展（`progress.json` 里各阶段状态：`images ✓ · style ✓ · chapters done · convert running`）；项目内再按阶段分子组（`矢量图` / `样式` / `章节划分` / `章节转换` / `章节核对` / `样式修复` / `逐图校验`），每个阶段组头带会话数与该阶段在 `progress.json` 里的状态（完成打 ✓），可各自展开折叠。
- **逐图（矢量图）会话按图片身份命名与检索**：转录文件名是哈希，页面会把它还原成"哪张图、在哪一页、第几张、什么类型"——行标题用图注/图片名，行内附 `第 12 页 · 第 3 张 · image · abc123def456` 小标签（来自 `doc_index.json` 的图片条目），排序按书里的图片顺序而不是修改时间。搜索框对这些字段同样生效：`p12`／`第12页`／`#3`／`第3张`／`abc123`／`vector`／`table` 都能定位。
- `latex_project/<书名>/work/sessions/convert_01.jsonl` → 组名 `latex_project/<书名>`。多项目布局下每本书一个工作区，书名目录里有 `.docvision_project.json`／`progress.json`／`work/`／`source/` 之类的东西，因此它自己就是一个组——侧栏标题显示**书名**，输出根 `latex_project/` 弱化成灰色前缀；否则同一个输出根下的所有书会挤成一个组，「这本书的会话在哪」就看不出来了。
- `latex_project/work/style_session.jsonl` → 组名 `latex_project`，并标一句「**旧版单项目**」：该输出根**本身**就是工程（`work/`、`progress.json` 直接挂在它下面），是引入多项目布局之前的形态（见上文「兼容旧版单项目布局」）。
- 工程内部的结构名（`work`、`source`、`style`、`sessions`、`temp`…）永远不当组名；直接躺在扫描根下的转录、或用 `--dir` 指向某个工程本身时，一律归到「（根目录）」。

默认全部展开；过滤时只留下有命中的组并自动展开（命中组标注「过滤命中」）；折叠状态记在页面内存里，点「刷新」重新扫描后仍保持（不落盘，重开页面回到全部展开）。

```bash
docvision sessions                           # 扫描当前目录，生成 <当前目录>/sessions.html
docvision sessions --dir /path/to/PDF2MD      # 指定扫描根目录
docvision sessions --list                     # 只在终端列出会话（阶段/消息数/提示词/用量/大小/修改时间/路径）
docvision sessions --serve                    # 本地实时预览 http://127.0.0.1:8848/
docvision sessions --serve --addr 127.0.0.1:9000
docvision sessions --out /tmp/sessions.html   # 自定义静态导出路径
docvision sessions --cost                     # 只打印按阶段的用量/费用报告
```

页面本身也会显示费用：侧栏每个会话的用量摘要末尾、底部合计、以及会话指标卡里的「费用」瓷砖（都读索引 JSON 里的 `cost` 字段，由配置里的价格算好；没配价格的会话整块不显示金额）。

`--cost` 按**阶段**汇总（`convert` / `checker` / `style-fix` / `style` / `vector` …）：会话数、请求数、输入与输出 tokens、加权缓存命中率、金额与平均每次请求，最贵的排前面。价格来自配置里的 `models.<条目>.price`（元/百万 tokens，分**未命中缓存输入 / 命中缓存输入 / 输出**三段），没配价格的模型**不显示金额**（显示"未配价"并提示金额只是下界，绝不用 ¥0 冒充免费）。`docvision latex` 跑完也会自动打印同一份表，并额外按书的规模给出**每张图 / 每页成本**（图片数取 `progress_items/`，页数取交付的 `out/book.pdf`）。

想在**跑 latex 的同时**看，不必另开终端：把 `preview.enabled` 打开（默认关闭），`docvision latex` 启动时会自己拉起同一份只读服务并把确切 URL 打进日志（地址/端口用 `preview.host`/`preview.port`，端口 `0` = 由内核挑；根目录取该档位的输出根 `<latex_project>` / `<latex_output>`），运行结束后随进程退出。详见 `docs/config.md` 的 `preview` 三项。

两种模式的区别：

| 模式 | 产物 | 刷新方式 | 适合 |
| --- | --- | --- | --- |
| 静态导出（默认） | 一个自包含 HTML：数据（`<script type="application/json" id="dsh-data">`）、样式、脚本全部内嵌，图片按相对路径引用 | 打开即定格，页面显示「静态快照 · 生成于 …」，**不做任何轮询** | 留存/转发某次运行的会话记录，`file://` 直接打开 |
| 实时服务 `--serve` | 本地只读 HTTP 服务，默认只监听 `127.0.0.1:8848` | 页面每 2 秒轮询 `/api/index`，当前会话 size 变了才用 `from=<已拉取行数>` 增量追加（保持滚动位置，开「自动跟随」则滚到底部），显示「实时」徽标 | 边跑 latex/转换边盯会话 |

两种模式共用同一套前端资源（`go/internal/sessionview/assets/`，`//go:embed` 内嵌，无 CDN、无构建步骤、纯原生 JS）：页面启动时若存在 `#dsh-data` 就用内嵌数据，否则走 `/api/*` 轮询。

页面能看什么：默认是**白天模式**（白底深字，正文对比度 ≥ 4.5:1），工具栏右侧的「🌙 深色 / ☀️ 浅色」按钮在浅色与深色之间切换，选择记在 `localStorage`（键 `dsh.sessionview.theme`；`file://` 打开时若浏览器禁用存储，静默回退为默认的白天模式）。配色全部走 CSS 变量，切换只改根元素上的 `data-theme` 属性；代码、JSON、思考内容一律等宽字体，两种主题下都保证清晰。

左侧是会话列表，**按项目分组**（见上）：每个会话行有阶段标签徽标、消息数、大小、相对时间、活跃圆点，以及项目内的子目录与「含系统提示词快照」提示；顶部始终显示扫描根路径。右侧是消息时间线——**最前面是「系统提示词（本次运行快照，不参与回放）」卡片**（见下）、用户/任务卡片（长文本可折叠，`images` 渲染成缩略图、点击放大、Esc 关闭）、AI 正文、**默认折叠的思考过程**（摘要行写明「思考过程 · N 字符」，展开后是等宽、低对比、保留原始换行的整段文本，长思考在容器内滚动不撑爆页面）、**工具调用**（工具名 + 格式化高亮的参数 JSON）、与调用按 `tool_call_id` 配对编号着色的**工具结果**（默认只显示前 8 行，可展开全文/复制；含 `error`/`REJECTED`/`文件不存在` 与 `ok (` 用不同颜色）。工具栏分组排列：状态徽标（静态快照/实时）、显示选项（「自动跟随」实时模式默认开、「折叠全部思考」作用于时间线上所有消息、「仅看工具调用」）、外观（主题切换）。每条消息显示序号与所在行号（转录每行都带写入时间戳 `ts`，卡片上的时刻按它显示）。

**系统提示词快照（`t=meta` 原子信息行）**：转录里除消息外还会写入一条 `{"t":"meta",…}` 行，记录**本次运行模型被交代了什么**——`model`、`session_label`、`system_sha`（提示词短哈希）、完整系统提示词 `text`，以及随行的工具定义（`tools[].name/description/parameters`）。它**只记录、永不回放**（`LoadTranscript` 只认 `t=="msg"`，续跑语义与「系统提示词每次运行重新渲染」的原则都不变），也**不计入消息数**。页面把它渲染成时间线最前面的可折叠卡片：摘要行写明「系统提示词（本次运行快照，不参与回放）」+ 模型 + 会话标签 + 短哈希 + 字符数，展开后等宽显示提示词全文（带「复制」按钮，长提示词在卡片内滚动），下方列出工具名与描述，每个工具的 `parameters` JSON 收在二级折叠里（默认收起）。同一转录有多条 meta 行时（同一会话多次运行、提示词变化）**只显示最后一条**并注明「共 N 条，显示最新」；侧栏会话行显示「含系统提示词快照」，工具栏显示提示词字符数与工具定义数。**老转录（没有 meta 行）照旧正常显示**，不会报错也不显示空卡片。

### 会话指标（token / 缓存 / 时延）

面板顶部（系统提示词卡片之后）是**「会话指标」卡片**，数据来自转录里的 `t="usage"` 行（每次 API 请求一条，见 [sessions.md](sessions.md) 的"会话转录"一节），**纯前端按请求加权求和**：

| 指标 | 口径 |
| --- | --- |
| 输入 / 输出 tokens | 各请求 `prompt_tokens` / `completion_tokens` 之和（括号里另给思考 `reasoning_tokens`） |
| 缓存命中 | `Σcached_tokens / Σprompt_tokens`——即前缀缓存把多少输入 token 变成了缓存价 |
| 平均首字 | `Σttft_ms / 请求数`；`ttft_ms` 是"发出请求 → 收到第一个流式增量"（思考中的第一个增量也算），非流式请求没有这个量，它等于整次耗时 |
| 输出速度 | `Σ输出 tokens / Σ(耗时 − 首字延迟)`，**生成时间**作分母，所以排队与思考等待不会被算成生成速度 |
| 平均耗时 | `Σduration_ms / 请求数`（墙钟，含思考） |
| 会话跨度 | 首末两条用量行的时间戳之差 |
| 每次请求明细 | 可展开表格：回合（`compact`/`nudge` 会标出来）、时刻、首字、耗时、输入、缓存%、输出、tok/s、结束原因 |

侧栏每个会话行显示一行用量摘要（输入/输出/缓存%/首字），侧栏底部给出**所有会话的合计**（请求数、输入/输出 tokens、整体缓存命中率）；工具栏也重复当前会话的关键几项。**没有 `t="usage"` 行的旧转录不显示任何指标**（不显示 0，避免把"没记录"误读成"实测为 0"）。`docvision sessions --list` 的「用量」列同理，无记录时是 `-`。

只读接口（供页面使用，也可自己 curl）：`GET /`、`GET /api/index`（`Scan` 结果 + 每个会话的 project/meta 摘要/ size/mtime + `stats` 用量聚合）、`GET /api/session?id=<相对路径>&from=<行号>`（返回 `lines` 与 `nextFrom`，meta 行与 usage 行原样包含在内，usage 行带解析好的 `stats`）、`GET /media/...`、`GET /file/...`（只提供根目录内的文件，`id` 必须命中扫描结果否则 404，`..` 越界一律 400，目录不列举）。无法解析的行会被跳过并在页面顶部提示「跳过 N 行坏数据」。
