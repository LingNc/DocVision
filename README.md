# DocVision - 文视

PDF 转 Markdown 自动化工作流。通过 MinerU API 解析 PDF，再用 AI 将文档中的图片转换为文本描述，最终输出结构化的 Markdown 文件。

提供 **Go** 实现（推荐）；历史 **Python** 实现已归档至 `legacy/python/`，仅供参考且不再维护。

## 工作流程

```
PDF 文件 -> 分割 -> MinerU API 解析 -> 整理文件 -> AI 图片转文本 -> 日志分析
```

`docvision workflow` 共 5 个步骤：

1. **split** - 将大文件按页数和文件大小限制分割（PDF 和 DOCX，MinerU 限制单次最多 200 页）
2. **mineru** - 调用 MinerU API 解析文件，支持并发、断点续传、上传进度显示
3. **organize** - 整理解析结果，合并分片，按引用收集图片到按主题子目录
4. **img2text** - 用 AI 模型识别图片内容并转为文本，支持并发和上下文增量扩展（工具调用）
5. **analyze** - 分析处理日志（img2text 与 latex 日志均支持），统计耗时、成功率、进度

另有两条**独立运行**的命令，不属于 workflow 步骤（`workflow --step` 不接受这两个名字）：

- **latex** - LaTeX 输出（图片矢量化 / 全书转换，见下文；自带前置流程与日志分析）
- **verify** - AI 核对图片与嵌入内容（默认关闭，`verify.enabled`；只能显式运行 `docvision verify`）

## 快速开始

### 配置

```bash
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，填入：

- MinerU API token（从 [mineru.net](https://mineru.net) 获取）
- AI 模型：统一在 `models:` 注册表配置，其中 `models.text` 为 img2text 等基础流程的默认模型（必填）；每个专用 AI（classifier/drawing/style/chapter/convert/checker/verifier）可单独配置 base_url/api_key/model/request_body/stream/thinking/reasoning_effort/tool_stream，空字段自动继承默认条目（`config_version: 6`，版本不符会提示更新配置文件）

### Go 版本（推荐）

```bash
cd go
make build

# 一键安装到 /usr/local/bin（无权限时回退 ~/.local/bin），
# 并初始化 ~/.docvision/config.yaml；之后任意目录可用 docvision
./build/docvision install

# 编辑生效的配置（当前目录 config.yaml 优先，否则 ~/.docvision/config.yaml），
# 自动选择 vim/nano；保存时校验语法与配置项，有问题可回车重编或 q 退出
docvision setup

# 处理任意位置的 PDF/DOCX（单文件或整个目录）——临时模式：
# 中间文件保存在 ~/.docvision/jobs/，最终 .md 输出到源文件所在目录
docvision workflow /path/to/paper.pdf
docvision workflow /path/to/pdf-dir/

# 项目模式（老用法不变）：在项目目录里放 config.yaml + files/，
# workflow/split/mineru/organize/img2text/analyze 照旧
cd 项目目录 && docvision workflow

# 初始化配置模板；会询问是否安装 Mermaid CLI
./build/docvision init

# 运行完整工作流
./build/docvision workflow

# 仅运行某个步骤
./build/docvision split --all
./build/docvision mineru
./build/docvision organize
./build/docvision img2text
./build/docvision analyze

# 指定配置文件
./build/docvision workflow -c my.yaml
```

交叉编译：

```bash
make release   # 输出到 go/release/（共 5 个产物：Linux/macOS 各 amd64+arm64，Windows 仅 amd64）
```

### Mermaid 验证工具（推荐安装）

如果 AI 输出 Mermaid 图，推荐安装 Mermaid CLI：

```bash
# 需要先安装 Node.js/npm
npm install -g @mermaid-js/mermaid-cli
```

`docvision init` 生成配置文件后会询问是否通过 npm 安装 Mermaid CLI；只有确认后才执行 `npm install -g @mermaid-js/mermaid-cli`，不会自动安装 Node.js/npm。

- `tools.mermaid.validation: auto`：找不到 `mmdc` 时给出安装提示，并跳过本次验证；
- `tools.mermaid.validation: strict`：找不到 `mmdc` 时明确报错，需安装后再继续；
- `tools.mermaid.validation: off`：禁用 Mermaid 验证。

### Python 版本

历史 Python 实现已归档至 `legacy/python/`，不再维护，建议使用 Go 版本。

将 PDF 或 DOCX 文件放入 `files/` 目录，运行工作流即可。

## Go 命令行说明

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

### img2text 嵌入格式

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

### latex 档位2 文本图嵌入

`docvision latex` 的 text 类图片**只提取图片里的可见文本**（专用提示词：公式→LaTeX 数学、表格→Markdown 表格、无文字→保留原图），不会把 AI 的描述性文字写进 markdown：

| 档位 / 情况 | 嵌入方式 |
| --- | --- |
| 档位2（任何文本图，含 styled） | 提取到的文本**直接嵌入正文**，不保留样式、不加标记（档位2 只做"图→文字 / 图→LaTeX"） |
| 档位1 · 普通文本图 | 提取到的文本直接嵌入正文 |
| 档位1 · 样式化文本图（styled） | 注释**首行即闭合**：`<!-- DOCVISION-STYLED-TEXT: <描述> -->`；注释**外**各占一行 `CONTENT: <图片原文>` 与 `LINK: [styled-text](images/…)`。转换会话按手册重排 CONTENT 或直接 includegraphics LINK，注释与字段都不得进入 `.tex` |
| 档位1 · raster 原图 | `<!-- DOCVISION-IMAGE: <label> -->`（首行闭合）+ 注释外一行 `DESCRIBE: <解释文本>` + 注释外一行 `LINK: [image](images/…)`；档位1 在 process 阶段**总是**为 raster 图生成解释（复用 img2text 提取，每张一次视觉调用，不依赖 `insert_image_description`——那个开关只作用于档位2），提取不到文本时只写 LINK 行 |
| 提取不到文本 | 档位2 保留原图链接；档位1 只写 `LINK: [image](…)`，都不丢内容 |

档位1 的矢量图用同一套骨架：`<!-- DOCVISION-VECTOR: <label> -->`（首行闭合）独立一行，注释外一行 `LINK: [vector](images/…)` 位于 latex 围栏上方；注释与字段同样不得进入 `.tex`。fallback（矢量转换失败）保持原图并在档位1 就地标注 `<!-- DOCVISION-ERROR: … -->`。

### 日志分析（analyze）

工具调用统计同时解析 img2text 的 `[ToolCall]` 行与 latex 会话的 `[tool:<名称>] ok/error` 行，并按线程归属到对应会话；错误分类识别 `IMG_*` 与 `SESSION_*` 两类哨兵。

### img2text 测试模式

```bash
./build/docvision img2text --test                  # 随机 10 张
./build/docvision img2text --test --number 5       # 随机 5 张
./build/docvision img2text --test --seed 42        # 固定随机种子
```

### analyze 选项

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

### 会话预览（docvision sessions）

把项目里所有 AI 会话的 JSONL 转录渲染成像聊天软件一样的可浏览页面。扫描范围是根目录下**任意层级**的 `*.jsonl`：`work/style_session.jsonl`（样式）、`work/sessions/chapters.jsonl`（章节划分）、`work/sessions/convert_<章>.jsonl`（单章转换）、`source/sessions/vector_<书名>__<sha256>__<图>.jsonl`（档位2 矢量图）等。

扫描根下可以并存多个项目，因此左侧会话列表**按项目分组**：一个项目 = 一个可折叠集合，标题带会话数，点标题折叠/展开。分组按**磁盘上的工程结构**判定，而不是死板地取路径第一段：

- `latex_project/<书名>/work/sessions/convert_01.jsonl` → 组名 `latex_project/<书名>`。多项目布局下每本书一个工作区，书名目录里有 `.docvision_project.json`／`progress.json`／`work/`／`source/` 之类的东西，因此它自己就是一个组——侧栏标题显示**书名**，输出根 `latex_project/` 弱化成灰色前缀；否则同一个输出根下的所有书会挤成一个组，「这本书的会话在哪」就看不出来了。
- `latex_project/work/style_session.jsonl` → 组名 `latex_project`，并标一句「**旧版单项目**」：该输出根**本身**就是工程（`work/`、`progress.json` 直接挂在它下面），是引入多项目布局之前的形态（见上文「兼容旧版单项目布局」）。
- 工程内部的结构名（`work`、`source`、`style`、`sessions`、`temp`…）永远不当组名；直接躺在扫描根下的转录、或用 `--dir` 指向某个工程本身时，一律归到「（根目录）」。

默认全部展开；过滤时只留下有命中的组并自动展开（命中组标注「过滤命中」）；折叠状态记在页面内存里，点「刷新」重新扫描后仍保持（不落盘，重开页面回到全部展开）。

```bash
docvision sessions                           # 扫描当前目录，生成 <当前目录>/sessions.html
docvision sessions --dir /path/to/PDF2MD      # 指定扫描根目录
docvision sessions --list                     # 只在终端列出会话（阶段/消息数/大小/修改时间/路径）
docvision sessions --serve                    # 本地实时预览 http://127.0.0.1:8848/
docvision sessions --serve --addr 127.0.0.1:9000
docvision sessions --out /tmp/sessions.html   # 自定义静态导出路径
```

两种模式的区别：

| 模式 | 产物 | 刷新方式 | 适合 |
| --- | --- | --- | --- |
| 静态导出（默认） | 一个自包含 HTML：数据（`<script type="application/json" id="dsh-data">`）、样式、脚本全部内嵌，图片按相对路径引用 | 打开即定格，页面显示「静态快照 · 生成于 …」，**不做任何轮询** | 留存/转发某次运行的会话记录，`file://` 直接打开 |
| 实时服务 `--serve` | 本地只读 HTTP 服务，默认只监听 `127.0.0.1:8848` | 页面每 2 秒轮询 `/api/index`，当前会话 size 变了才用 `from=<已拉取行数>` 增量追加（保持滚动位置，开「自动跟随」则滚到底部），显示「实时」徽标 | 边跑 latex/转换边盯会话 |

两种模式共用同一套前端资源（`go/internal/sessionview/assets/`，`//go:embed` 内嵌，无 CDN、无构建步骤、纯原生 JS）：页面启动时若存在 `#dsh-data` 就用内嵌数据，否则走 `/api/*` 轮询。

页面能看什么：默认是**白天模式**（白底深字，正文对比度 ≥ 4.5:1），工具栏右侧的「🌙 深色 / ☀️ 浅色」按钮在浅色与深色之间切换，选择记在 `localStorage`（键 `dsh.sessionview.theme`；`file://` 打开时若浏览器禁用存储，静默回退为默认的白天模式）。配色全部走 CSS 变量，切换只改根元素上的 `data-theme` 属性；代码、JSON、思考内容一律等宽字体，两种主题下都保证清晰。

左侧是会话列表，**按项目分组**（见上）：每个会话行有阶段标签徽标、消息数、大小、相对时间、活跃圆点，以及项目内的子目录与「含系统提示词快照」提示；顶部始终显示扫描根路径。右侧是消息时间线——**最前面是「系统提示词（本次运行快照，不参与回放）」卡片**（见下）、用户/任务卡片（长文本可折叠，`images` 渲染成缩略图、点击放大、Esc 关闭）、AI 正文、**默认折叠的思考过程**（摘要行写明「思考过程 · N 字符」，展开后是等宽、低对比、保留原始换行的整段文本，长思考在容器内滚动不撑爆页面）、**工具调用**（工具名 + 格式化高亮的参数 JSON）、与调用按 `tool_call_id` 配对编号着色的**工具结果**（默认只显示前 8 行，可展开全文/复制；含 `error`/`REJECTED`/`文件不存在` 与 `ok (` 用不同颜色）。工具栏分组排列：状态徽标（静态快照/实时）、显示选项（「自动跟随」实时模式默认开、「折叠全部思考」作用于时间线上所有消息、「仅看工具调用」）、外观（主题切换）。转录不记录每条消息的时间戳，因此每条消息显示序号与所在行号，时间只有侧栏的相对时间与工具栏的「最后写入」。

**系统提示词快照（`t=meta` 原子信息行）**：转录里除消息外还会写入一条 `{"t":"meta",…}` 行，记录**本次运行模型被交代了什么**——`model`、`session_label`、`system_sha`（提示词短哈希）、完整系统提示词 `text`，以及随行的工具定义（`tools[].name/description/parameters`）。它**只记录、永不回放**（`LoadTranscript` 只认 `t=="msg"`，续跑语义与「系统提示词每次运行重新渲染」的原则都不变），也**不计入消息数**。页面把它渲染成时间线最前面的可折叠卡片：摘要行写明「系统提示词（本次运行快照，不参与回放）」+ 模型 + 会话标签 + 短哈希 + 字符数，展开后等宽显示提示词全文（带「复制」按钮，长提示词在卡片内滚动），下方列出工具名与描述，每个工具的 `parameters` JSON 收在二级折叠里（默认收起）。同一转录有多条 meta 行时（同一会话多次运行、提示词变化）**只显示最后一条**并注明「共 N 条，显示最新」；侧栏会话行显示「含系统提示词快照」，工具栏显示提示词字符数与工具定义数。**老转录（没有 meta 行）照旧正常显示**，不会报错也不显示空卡片。

只读接口（供页面使用，也可自己 curl）：`GET /`、`GET /api/index`（`Scan` 结果 + 每个会话的 project/meta 摘要/ size/mtime）、`GET /api/session?id=<相对路径>&from=<行号>`（返回 `lines` 与 `nextFrom`，meta 行原样包含在内）、`GET /media/...`、`GET /file/...`（只提供根目录内的文件，`id` 必须命中扫描结果否则 404，`..` 越界一律 400，目录不列举）。无法解析的行会被跳过并在页面顶部提示「跳过 N 行坏数据」。


## LaTeX 输出（docvision latex）

基于 `output/` 中 MinerU 整理后的 Markdown 构建 LaTeX 输出，两档位：

**档位 2（默认，`latex.level: 2`）—— 图片矢量化**，产物是"可以直接当 markdown 读"的干净文本

1. 专用**分类 AI**（`models.classifier`，需视觉能力）逐图标记：
   - `text`：样式化文本（如美化题号）→ 走"只提取图中可见文本"的专用提示词（公式→LaTeX、表格→Markdown 表格，无文字则保留原图），纯文本嵌入正文（无 `[AI]`/`[IMG_TYPE]` 标记）
   - `vector`：函数图像/立体结构/流程图等可矢量重绘图形 → 进入作图流程
   - `raster`：照片/截图等 → 保留原图链接（`insert_image_description: true` 时才嵌入可读解释文本）
2. **作图 AI**（`models.drawing`）在独立会话中重绘矢量图（TikZ/pgfplots/tabular 等任意 LaTeX 方式）→ `write_file` 写代码 → `compile {path:"figure.tex"}` 编译（只回编译日志、产物名与页数）→ `view_pdf {path:"standalone.pdf", page:1}` 看自己编译出的 PDF 做**视觉核对** → 迭代修正 → `submit` 确认提交。会话附带 `image_context`/`view_image` 工具：跨页拆分的长表/大图可查看相邻图片及其上下文，**一次绘制合并图**并在 submit 时声明吸收的续片（续片引用自动删除、不再重复处理）
3. 编译产物转 SVG 后以 `![label](figures/*.svg)` 嵌入 Markdown（SVG 不可用时退 PNG/PDF 链接）；矢量转换失败时回退保留原图并输出警告日志
4. 档位2 输出**不含任何 `<!-- DOCVISION-* -->` 注释**，矢量回退与 SVG 失败只记日志与 `progress.json`

**档位 1（`latex.level: 1`）—— 全书 LaTeX**

1. **样式分析 AI**（`models.style`）：拥有独立虚拟工作区（`latex_project/<项目名>/work/style/`），用 `list_source_pages` 取原书页索引（源 PDF / 页范围 / 全局页号 / OCR 推导的章节起点），再用 `view_pdf {path:"source:<file>.pdf", page:N}` **按需渲染** MinerU 保留的原始扫描页（支持裁剪放大）、`view_image` 看提取图、`read_file`/`grep` 读解析文本，分析全书样式后用 `write_file`/`edit_file` 增量起草（`example.tex` 的图**直接用 md 里已有的 LaTeX**：`<!-- DOCVISION-VECTOR -->` 下面那个 ```latex 围栏就是这本书真正的图代码，复制它比重新画一张更省心智、也更贴合实际成品），再 `submit_style` **按工作区路径**提交 `book.cls` + 使用手册 + 案例（`{cls, manual, example, extra[], report}`：`extra` 可带上类需要的任何附属文件——helper `.sty`、TikZ 样式、字体表——会连同示例一起进试编译；`report` 写进 `style/REPORT.md`；示例自动试编译，失败回炉）。`submit_style` **只收工作区路径**，没有内联内容兼容（工具形态不该对模型保留历史包袱，且"路径还是内容"的启发式曾把一行 `## Vector figure style` 误判为文件名）；参数里带换行内容会被拒绝并提示先 `write_file` 再交路径。字体通过 `list_fonts` 查看；缺字体时在提交报告与手册里列出清单，由用户按文件名手动放进 `paths.fonts`（编译环境已注入 `TEXINPUTS`/`OSFONTDIR`，无需装系统字体）
2. **章节划分 AI**（`models.chapter`）：`grep`/`read_file`/`bash` + `buffer.md` 工作记忆缓冲区（分析结论增量写入、跨轮保留），按行号划分章节（结构化提交，全覆盖校验）
3. **转换 AI** 并发逐章转 `.tex`：每章在**自己的临时工作区** `work/temp/conv_<章>/work` 里干活——里面已经铺好 cls/`.sty`/手册/`example.tex` 的**副本**（随便改随便试，真样式包不受影响）、插图（`images/`、`figures/`）、该编译的 wrapper 与**待提交的两个路径**（`<章>.tex` + `<章>/`）；探针、实验 `.tex`、PDF 都留在临时工作区里，**只有提交的那两个路径会被拷进 `work/chapters/`**（分片文件夹整棵替换，历史残留与探针不会混进书里）；别人的成品只能经只读通道 `project:converted/`（其他章节已提交的 `.tex`）与 `project:reports/`（其他章节的工作汇报）参考；首条消息给出**工作区地图**（`work:`/`project:`/`source:` 各是什么）与两处权威输入路径——章节文本 `project:chapters/<章>.md`（**从它转换**：它是解析好的纯文本，读起来最省；但它是 OCR 来的、**可能出错**——遇到读不通的字、公式、图表就以**原书为准**，用 `doc_search`/`list_source_pages`/`view_pdf source:` 翻原页核对；版面问题一律以原书为准）与 class 手册 `project:style/manual.md`（不再整段内联）；工具集含 `bash`（沙箱内、与其它工具同一套挂载点）与 `compile`（**编译包了 `\\documentclass` 的 wrapper**；`compile {path:"chapters/<章>/probe.tex"}` 可单独编译工作区里任何一个 `.tex` 做验证，探针文件不得留在产物里；章节 `compile` 也接受 `engine`/`passes`/`args` 现场指定引擎与额外参数，回执只报 `COMPILE OK` + 产物名 + 页数或错误日志）；章节提示词只写"怎么用"与转换规则，工具参数交给工具自己的 schema（system 9.3K→4.9K、首条消息 1.2K→0.17K，模型不用把同一件事读两遍）；矢量图已在 images 阶段以 latex 代码块内嵌进章节 md，转换时按手册的 `## Vector figure style` 二次加工（保留结构、几何与全部标签）；raster 原图走 includegraphics → 每章提交后由 **核对会话**（`models.checker`，小文本模型即可）比对原章 md 与产物：它跑成一个**极简的只读会话**——挂载表里只有两个文件（章节 md + 提交的 `.tex`）与一个文件夹（`parts/` 分片），工具只有 `read_file`/`grep`/`submit`，默认 50 轮，写不了任何东西；**硬性问题回同一个转换会话最多 3 轮**（上下文还在，最省 token），3 轮仍未过才作废该章 `.tex`、资源目录与转录，用全新会话重转换一次
4. 汇总为多文件 `.tex` 项目 → 编译全书 PDF（失败进入**修复会话**：读文件/改文件/重编译 + 字体工具核对样式）→ 成功后进入**终审会话**逐页核对成品 PDF 并整理（`latex.compile.final_review`，默认开启）→ 生成单文件 `standalone.tex`；交付把整棵 build 树复制到 `out/`，`book.pdf` 只是 `main.pdf` 的别名
5. 多数章节的工作汇报报告 cls/手册问题时，打回原样式会话修正（上限 2 轮）；样式包更新后只对「报问题」或「新 cls 下编译不过」的章节并发跑**样式修复子会话**（同样在 `work/temp/conv_<章>/work` 里，先把该章已转换的产物拷进去，增量 `edit_file` 适配新 cls/手册，提交同样只交那两个路径，不重新转换），修复失败才退回整章重转换

### AI 会话基础设施

- **模型注册表** `models:`：每个专用 AI（classifier/drawing/style/chapter/convert/checker/verifier）可单独配置 base_url / api_key / model / request_body / stream / thinking / reasoning_effort / tool_stream
- **流式请求（默认开启）**：`models.*.stream: true`（默认）时走 SSE 流式接收，长思考/长输出期间持续有进展，不会长时间静默；厂商不支持流式时自动回退一次非流式请求。流式模式下用 `api_stream_idle_timeout`（默认取 `api_timeout`）判定"卡住"，而不是整次请求超时
- **思考控制**：`thinking: {type: enabled|disabled}` 与 `reasoning_effort: max|xhigh|high|medium|low|minimal|none` 都是请求体**顶层字段**（不要写进 `request_body.extra_body`），按 `models` 条目配置，空值继承 `models.text`；GLM 的保留式思考写 `thinking.clear_thinking: false`（历史 assistant 轮的思维链完整回传，提升缓存命中）
- **会话管理**：每个会话独立上下文窗口（`sessions.*.context_limit`，默认 128K，可设 64K/256K），达到 `compaction_at`（默认 0.85）阈值自动压缩，**两级**：先做**本地裁剪**（零成本，不调用模型）——超长工具结果留"前半 + 后 1/4"（`sessions.*.prune_tool_chars`，默认 4096 字符；实测真实会话里最长的工具结果 6183 字符、p99 5046，8K 阈值从不触发，只有 `read_file` 整文件（硬上限 64KB）与 `bash`（`tools.bash.max_output`，默认 5000）才可能很长），较早的图片换成文字占位（`sessions.*.keep_images`，默认保留最近 3 张；占位写明"这里曾附过 N 张图，需要时重新 view_image"，所以"看过什么"不会丢）；仍超阈值才花一次 **AI 摘要**：**系统提示词、原始任务、最近 8 条消息原样保留**，中间部分由 AI 写成一篇续跑笔记替换（摘要请求是"在原会话末尾追加一条指令"的增量请求，因此仍能命中厂商前缀缓存）；token 估算现在把系统提示词与工具定义一并计入（原来漏算，实测低估 5 万+ tokens）；**阈值检查在每一次模型请求之前都做**——这一条曾经是真实缺陷：守卫原本只在 `Run()` 入口调一次，而 latex 的这些会话"一次 `Run()` 就是整个会话"（初始任务进去以后几百轮都在同一个循环里），于是守卫只见过"系统提示词 + 第一条消息"，**一次都没触发过**：实测一轮运行里 prompt 涨到 323,966 tokens（窗口 128K、阈值 0.85）、转录里从来没有 `=== COMPRESSED SESSION CONTEXT` 行，费用一路升到账户余额不足；现在压缩在循环内每轮检查，并在**普通日志**（不需要 `--debug`）里打出上下文增长：占用到 50%、80% 各一条 `[context] 估算 N tokens = 窗口(M) 的 P%`，压缩本身打 `[compact] 本地裁剪…` / `[compact] history compacted…`；另外"压完仍超阈值"（窗口极小、或尾部本身就很长）不会每轮重复花钱摘要——只有在上次压缩之后又涨了 20% 才再压一次。**本地裁剪只在接近窗口时才发生**（不是每轮都裁）：窗口很大时即便有 5 万字符的工具结果也一个字都不动，历史里"早先看过的图"也不会被换成占位；裁剪后仍超阈值才进入 AI 摘要，因此"裁过就不用了"的担心不成立；压缩后续跑从**最后一次压缩检查点**开始（`LoadTranscript` 丢弃最后一条 `COMPRESSED SESSION CONTEXT` 之前的消息，并在写摘要后补写"原始任务 + 最近 8 条"，回放与内存一致）；`sessions.checker` 未设置的字段继承 `sessions.convert`（唯一例外：`max_tool_rounds` 默认 50）
- **矢量图落盘**：TikZ 编译成功后依次尝试 `dvisvgm → pdftocairo → mutool → inkscape` 转 SVG 内嵌（dvisvgm 3.6 处理 PDF 需要 Ghostscript < 10.01 或 mutool，缺条件时自动走 pdftocairo）；全部失败才回退 PNG/PDF 链接——档位2 只记日志与 `progress.json`，档位1 另在 markdown 就地标注 `<!-- DOCVISION-ERROR: … -->`
- **图片查看工具**：`image_context` 只给文本上下文与前后引用（不看像素），`view_image` 才看图片（支持百分比裁剪与放大）；两者都接受**裸文件名**——`view_image` 以本文档的图片目录为根直接拼接，不做搜索，名字错了就报错
- **编译与看图分离**：`compile {path:"figure.tex"}` 只返回编译日志、产物名与页数，看图统一用 `view_pdf {path:"standalone.pdf", page:1}`（裁剪 + zoom 直接从 PDF 高分辨率重渲染，真放大，不是拉伸像素）
- **可分离工具**：会话工具按需注册（编译、提交确认、grep、bash 沙箱、受限文件读写、PDF/图片查看等）
- **断点续传**：档位2 逐图进度、档位1 逐阶段进度（`progress.json`）
- **工具轮次软限制**：`max_tool_rounds` 限制的是**assistant 轮次**（一轮里发多少个 tool_call 都只算 1 次）。用满 `sessions.*.tool_rounds_warn_ratio`（默认 0.7）后，每轮往会话里更新一条提醒（"已用 N/M 轮，还剩 K 轮，请合理使用并尽快提交"）；到达 `max_tool_rounds` 后**还能再用 `sessions.*.tool_rounds_grace` 轮**（默认 20，负值=不留宽限），此后才真正禁用工具、逼最终文本。提醒是**原地替换**同一条消息，不膨胀历史也不破坏前缀缓存。
- **看图软预算**（按"对象"计数，长文档不吃亏；矢量图会话在**首次提示词里就一次性告知**额度，不再每次看图都重复提醒）：`tools.view.image_max`（默认 30）是**同一张图片文件**的软上限，`tools.view.pdf_max`（默认 25）是**同一个 PDF 的每一页**的软上限——所以一本书里每页各有 25 次额度，而不是整个会话共用一个池子。用满 `tools.view.warn_ratio`（默认 0.7，向上取整）起，每次 `view_image`/`view_pdf` 的结果里附带"已用 N/30，仅剩 K 次"（30×0.7=21 → 从第 21 次起提醒），超出后提示"预算已用尽，请尽快完成并提交"——**只提醒，不拦截调用**（0=默认值，负值=不限）。
- **原图尺寸测量**：每次 `view_image` 都会实时算出原图的**印刷尺寸**并回给模型——位图先解析软链、再按**文件名**匹配 `paths.mineru_output` 下 MinerU 解析（`content_list.json` 的 bbox + `layout.json` 的页尺寸；位图被 images 阶段拷进项目后名字不变，靠文件名就能接回解析），显示标准为 **mm 优先**：`ORIGINAL FIGURE SIZE: 36.9mm x 20.2mm on the page (about 21% of the page width); bitmap 284x156px, effective resolution 195 dpi, aspect 1.82:1`（高度按位图自身比例换算，因为 MinerU 的块 bbox 不紧贴图）。**裁剪之后还会给出当前裁剪区域的尺寸**（`this crop is 18.4mm x 10.1mm on the page`），因为"现在看的是多大一块"正是复现细节时需要的；真的找不到解析（比如位图不在本次解析里）才退化为只给宽高比。
- **PDF 尺寸**：`view_pdf` 回执给出**该页真实尺寸**与**当前裁剪区域的尺寸**（都是 mm），看原书页时它就是真实书本尺寸（可直接用来确定 `geometry` 的纸张），看自己编出来的 PDF 时就是成品的实际尺寸——两边同单位才谈得上"是否符合要求"。`compile` 成功回执里的产出尺寸同样用 mm（`Drawn size: 104.2pt x 66.4pt (36.8mm x 23.4mm, aspect 1.57:1)`）。
- **会话转录（JSONL）**：每条消息实时追加为一行 JSON（含 `reasoning_content` 思维链——GLM 保留式思考要求历史思维链完整回传，也是前缀缓存的前提），图片以 `file://media/<hash>.<ext>` 引用（base64 不入转录）。**转录里没有 system 行，也没有工具的 JSON Schema**——这是设计如此，不是丢失：系统提示词由运行时从 `internal/prompts` 模板重新渲染（含水印开关、挂载说明等本次运行才确定的内容），工具定义由运行时按会话类型拼装，两者都不落盘；不过每个会话开头会写一条 **`t=meta` 元信息行**（`{"t":"meta","kind":"system","session_label":…,"model":…,"system_sha":…,"text":"<完整系统提示词>","tools":[{name,description,parameters}…]}`），把**本次运行模型被交代了什么**（系统提示词全文 + 当时发给 API 的工具定义）原样记下来——它**只用于查看、永不参与回放**（`LoadTranscript` 只认 `t=="msg"`，续跑语义与"系统提示词由运行时重新渲染"的原则都不变），同一份提示词重复挂载时按哈希去重不会写第二条，提示词变了（例如换了模板）才会追加一条；`docvision sessions` 的页面会把它显示成可折叠卡片；要更细的逐轮请求（含 token 用量、流式进展）仍看 `--debug` 的 `[DEBUG]` 摘要；矢量图会话（`<outDir>/sessions/vector_<图>.jsonl`）、样式会话（`work/style_session.jsonl`）、章节划分（`work/sessions/chapters.jsonl`）、单章转换（`work/sessions/convert_<章>.jsonl`）都接入——进程被杀或网络断连后，下次运行自动从转录恢复上下文续跑，不重烧 token（恢复时**重新挂上系统提示词**、只回放最近一次压缩之后的消息——压缩刻意保留的"原始任务 + 最近 8 条"会在写完摘要后补写一遍，否则回放截断会把它们一起丢掉）；成功会话的转录默认清理（`latex.keep_session_records` 可保留）

### 提示词集中管理（`internal/prompts`）

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

### 会话沙箱与虚拟工作区（挂载表）

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

**Python 环境**（`tools.python`）：会话 bash 跑在 `--unshare-net` 的沙箱里，**会话自己装不了任何包**（2026-09-11 实测：样式会话遇到 `ModuleNotFoundError: No module named 'PIL'` 后花约 10 轮自己手写了一个 PGM 解析器）。所以环境由宿主提供——按配置准备 `system`/`venv`/`conda` 解释器并把它的**运行所需目录只读绑定**进沙箱（解释器自身目录、`sys.prefix`、软链链上的每一跳、`ldd` 列出的共享库目录、site-packages；Homebrew 的 venv 就因为 `bin/python3 → .linuxbrew/opt/... → Cellar` 这条链少一环而 exec 失败，bash 的 PATH 搜索会**静默跳到下一个 `/usr/bin/python3`**，于是"装好的环境"根本没生效），`PATH` 指向它；缺模块时宿主侧（有网）自动 `pip install` 并让模型重试；安装失败则按缺字体同样的办法留下 `<项目>/work/python/requirements.txt` + `README.md`。沙箱内用 `python3` 即可，无需 Activation。

档位1 的临时工作区统一落在**本项目工作区**里的 `work/temp/<名称>`（即 `latex_project/<项目名>/work/temp/…`；拆章沙箱、样式/反馈 scratch、每章转换/修复工作区 `conv_<章>/work`），不再藏进 `/tmp`；`latex.keep_temp_dirs` 可保留以便事后检查。

### 调试日志

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

默认控制台输出与 img2text 一致：每个阶段只显示一行实时进度（如 `[classify 12/345] 3.48% (failed: 0, running: 2)`；process 行还带 `done/errors/fallback/raster` 计数），逐图明细写入日志文件；`--verbose` 恢复逐图控制台输出。

`--debug`（或 `options.log_level: debug`）下，临时工作目录与会话转录**必定保留**（`latex.keep_temp_dirs` / `latex.keep_session_records` 打开也保留），因此一次完整运行事后可以逐会话复现。

### 档位1 目录布局（`latex_project/<项目名>/`）

每本书一个工作区：`<latex_project>/<项目名>/`（项目名 = 主题名或 `--project`；旧版单项目布局时就是 `latex_project/` 本身，下表同）。下表所有路径都相对于**项目工作区**：

| 目录 | 用途 |
| --- | --- |
| `source/` | images 阶段整理的 md 输入（另有 `source/progress_items/` 逐图进度） |
| `pages/` | 原始扫描页渲染缓存（仅水印采样使用；会话看图走 `view_pdf` 现场渲染，不缓存） |
| `style/` | 样式分析产物（book.cls / manual.md / example.tex） |
| `work/style/` | 样式分析 AI 的虚拟工作区（write_file 增量起草；转录 `work/style_session.jsonl`） |
| `chapters/` | 章节划分 AI 产出的 chapter_00N.md |
| `work/chapters/` | 转换 AI 的正式产物：`chapter_00N.tex` + 每章资源目录 `chapter_00N/` |
| `work/views/project/` | `project` 挂载点（只读窄视图：source / style / chapters / converted / reports） |
| `work/views/checker_<章>/` | checker 会话的**只读视图**：`<章>.md`、`<章>.tex`、`parts/`（正好两个文件 + 一个文件夹） |
| `work/pdfview/` | 原书 PDF 最小视图（`source` 挂载点，`<part>.pdf` 软链到 MinerU 的 `*_origin.pdf`） |
| `work/reports/` | 每章转换会话的工作汇报 `<章>.md`（转换期间，只读通道 `project:reports/`） |
| `work/sessions/` | 会话转录（`chapters.jsonl`、`convert_<章>.jsonl`）；矢量图另有 `sessions/vector_<图>.work` 持久工作区 |
| `work/temp/` | 临时工作区（拆章沙箱、样式/反馈 scratch、每章转换工作区 `conv_<章>/work`），默认用完即删 |
| `build/` | 全书的构建树（每次 clean 重建：cls/手册/案例 + chapters + figures + main.tex），同时是修复会话与终审会话的工作区 |
| `out/` | 交付产物：整棵 build 树（跳过 .aux/.log/.toc/.synctex 等中间文件），`book.pdf` 是 `main.pdf` 的别名，另有 `standalone.tex` |
| `doc_index/doc_index.json` | 只读块索引（`doc_search` 的检索库，style 阶段之前构建；图片条目带 md 里 DOCVISION 注释的类型/描述/原文） |
| `watermark_memory.json` | 水印检测工作记忆（`latex.remove_watermark` 开启时生成；档位1 落在本项目工作区，档位2 同理落在 `<latex_output>/<项目名>/`） |
| `progress.json` | 逐阶段断点进度 |
| `.docvision_project.json` | 项目自述（项目名 + 主题名来源，仅新建项目写入；同名去重依据；旧版单项目目录不写） |

> 多项目：`latex_project/` 下可以并存 `测试-概率论/`、`线性代数/` …，各自拥有上表全部内容，互不覆盖。项目名默认取主题名（主 md 主文件名），`--project <名字>` 可覆盖。
>
> 档位2（`paths.latex_output`，默认 `finally_latex/`）的工作区同样是 `<latex_output>/<项目名>/`，其中：会话文件 `sessions/vector_<图>.jsonl`（转录）与 `sessions/vector_<图>.work/`（持久工作区），逐图进度 `progress_items/`，另有整理后的 md 与 `figures/`、`images/`、`tikz/`。

章节划分 AI 本身没有写目录：它只在自己沙箱里的 `book.md` + `buffer.md` 上工作（bash + grep/read_file/edit_file），通过结构化 submit 提交切分方案，由代码落盘到 `chapters/`。
### AI 核对（verify，默认关闭）

`docvision verify` 用配置的视觉校验模型（`verify.verifier_model`）逐项核对每张图与其嵌入内容，生成 `verify_report.md`（问题 + 修改意见，不改动输出）。核对只能**显式运行**——`verify.enabled` 不再驱动任何自动流程：

```bash
docvision verify
```

### 用法

```bash
# ★ 最常用：一条命令从原始文档到 LaTeX（自动补跑 分割→MinerU→整理）
docvision latex 书.pdf
docvision latex 书.docx

# output/ 已经有解析结果时：直接对某个 md 继续 LaTeX（跳过前置）
docvision latex 编译原理课程设计报告样例.md
docvision latex book1.md book2.md

# 批量：处理 output/ 下全部 markdown
# （裸命令会先自动跑前置 split→mineru→organize（跳过已处理），再处理 output/ 全部 md）
docvision latex

# 档位与阶段控制
docvision latex --level 1 书.pdf               # 档位1 全书转换
docvision latex --level 2 --test --number 5    # 抽样测试
docvision latex --level 1 --step style 书.md   # 仅某个阶段
#   --step 取值：档位1 images|style|chapters|convert|assemble；档位2 classify|process
docvision latex --source-dir 其它md目录 书.md  # 覆盖输入 markdown 目录（paths.output_dir）
docvision latex --all                          # 日志分析汇总全部历史

# 多项目：一个 latex_project/ 下并存多本书，各自一个工作区
docvision latex --level 1 线性代数.pdf         # 默认项目名 = 主题名（主 md 名，同 images/<主题>/）
docvision latex --level 1 --project 概率论-2026 书.md   # 显式指定项目名
#   工作区：档位1 <latex_project>/<项目名>/，档位2 <latex_output>/<项目名>/
#   --project 名字会被安全化（去路径分隔符/首尾空白与点，中文保留）

docvision verify                               # AI 核对报告
docvision verify --project 概率论-2026         # 多项目时指定核对哪个项目
```

> 说明：`docvision latex` 是**自助式**的——位置参数支持 `files/` 里的 PDF/DOCX（自动复制进 `files/` 并在项目内补跑 split→mineru→organize）和 md 名（复制进 `files/` 后整理进 `output/`），**目录与不带扩展名的裸文件名不受支持**；前置流程跑完它自己接日志分析，不需要先手动跑 workflow。
> `latex` 与 `verify` 都**不是** workflow 步骤（`workflow --step` 只接受 split|mineru|organize|img2text|analyze）。需要"中间文件隔离在 `~/.docvision/jobs/` 的一次性作业目录"时，用 `docvision workflow <文件或目录>`。

输出目录（**多项目**）：一个输出根下每本书一个工作区——档位1 `latex_project/<项目名>/`（含 out/book.pdf 与 standalone.tex），档位2 `finally_latex/<项目名>/`，与 img2text 的 `finally/` 互不干扰。项目名默认取这本书的**主题名**（主 markdown 的主文件名，也就是 `images/<主题>/` 用的那个名字，例如 `测试-概率论`），可用 `--project <名字>` 覆盖。

> **兼容旧版单项目布局**：若输出根**本身**已经是工程（含 `work/`、`source/`、`style/`、`chapters/`、`build/`、`out/`、`doc_index/`、`progress.json`、`progress_items/` 任意一项），且**未**指定 `--project`，则继续沿用该目录（既有工程原地可跑、内容一个字节都不动），日志里会写一条提示："检测到旧版单项目布局 …，未指定 --project，继续沿用 …；要在同一输出根下新建独立项目请用 `--project <名字>`"。也就是说：**要开第二本书，显式给 `--project`**。想把旧工程改造成多项目布局，只需把它整体挪进输出根并以书名命名（进度、会话、doc_index 全部随之保留）：
>
> ```bash
> mv latex_project 测试-概率论 && mkdir latex_project && mv 测试-概率论 latex_project/
> # 之后 docvision latex 测试-概率论.pdf 会复用 latex_project/测试-概率论/（断点续传照常）
> ```

`docvision verify` 也按项目工作区定位：`--project <名字>` 显式指定；不指定时，输出根本身是旧版单项目工程就用它，否则用其中**唯一**的子项目（有多个会报错并列出名字，避免核对错书）。默认读写的 `progress_items/` 与 `verify_report.md` 都在该项目工作区内。

需要本地 LaTeX 工具链：TeX 发行版（xelatex）+ poppler-utils（pdftoppm）。全书多文件构建优先用 `latexmk`（缺失时退回两遍编译）；矢量图转 SVG 按 `dvisvgm → pdftocairo → mutool → inkscape` 依次尝试，任一可用即可；会话 bash 沙箱用 bubblewrap（缺失时自动回退普通 shell 并记警告）。`docvision init` 会自动检查 xelatex / pdftoppm / mmdc 并给出安装提示。

### 风格分析的数据来源（档位1）

样式分析 AI 直接查看 **MinerU 保留的原始扫描页面**（`mineru_output/<主题>_part*/*_origin.pdf` 以软链形式暴露为只读 `source` 挂载点下的 `work/pdfview/<part>.pdf`，模型看不到整个 `mineru_output` 树；用 `list_source_pages` 取页索引与章节起点、`view_pdf {path:"source:<file>.pdf", page:N}` 按需 pdftoppm 渲染整页 PNG），支持按页浏览、按百分比裁剪、放大查看细节——这才是真实排版（标题格式、页眉页脚、字号行距）。原始 PDF 缺失时退化为仅用提取图片 + 解析 md 分析，并输出日志提示。

## Python 脚本独立使用

历史 Python 脚本已归档至 `legacy/python/`，独立使用方式请参阅该目录下的 `README.md`。

## 主要配置说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `mineru.max_pages_per_part` | 单个文件分片最大页数（PDF 和 DOCX） | 200 |
| `mineru.max_size_mb` | 单个文件分片最大大小（MB） | 200 |
| `mineru.max_concurrent` | MinerU API 并发数 | 5 |
| `mineru.upload_timeout` | 上传空闲超时（秒） | 300 |
| `mineru.log_poll_interval` | 控制台日志输出间隔（秒） | 3 |
| `mineru.progress_threshold` | 页数变化阈值，达到此值立即刷新输出 | 80 |

### 工具配置（tools:）

各流程共用的工具参数（get_more_context / image_context / 校验工具）：

| 键 | 说明 | 默认 |
| --- | --- | --- |
| `tools.context.initial_up` / `initial_down` | 图片初始上下文行数（上/下） | 10 / 5 |
| `tools.context.max_up` / `max_down` | 上下文可扩展上限（行） | 50 / 50 |
| `tools.context.max_calls` | 单图最大上下文扩展请求次数 | 5 |
| `tools.mermaid.validation` | Mermaid 校验模式（off/auto/strict） | auto |
| `tools.mermaid.command` | Mermaid CLI 命令 | mmdc |
| `tools.mermaid.fix_attempts` | Mermaid 独立修正次数（0=无限，受安全上限保护） | 3 |
| `tools.mermaid.timeout` | 单次 Mermaid 校验超时（秒） | 30 |
| `tools.latex.validation` | latex 代码块编译校验（off/auto/strict） | auto |
| `tools.latex.engine` | latex 校验引擎（缺 pdflatex/lualatex 自动回退） | xelatex |
| `tools.bash.sandbox` | 会话 bash 是否用 bubblewrap 内核沙箱（沙箱里只有本会话的挂载表；缺 bwrap 自动回退并记警告） | true |
| `tools.bash.max_output` | 会话 bash 工具返回给模型的字符上限 | 5000 |
| `tools.python.enabled` | 是否给会话 bash 提供 Python 环境 | true |
| `tools.python.mode` | 环境形态：`system` / `venv` / `conda`（venv 会按 `env_dir` 自动创建） | system |
| `tools.python.interpreter` | 解释器路径（留空 = `env_dir/bin/python3` 或 PATH 里的 `python3`） | 空 |
| `tools.python.env_dir` | venv 目录（不存在则宿主侧自动创建）或 conda 环境前缀 | 空 |
| `tools.python.conda_env` | conda 环境名（`env_dir` 为空时用它；留空或 `base`/`root` = conda 的 **base** 环境） | 空 |
| `tools.python.packages` | 启动前确保可导入的模块（缺则宿主侧安装） | 空 |
| `tools.python.auto_install` | 会话 bash 报 `ModuleNotFoundError` 时由宿主侧自动 `pip install` 并提示重试 | true |
| `tools.python.install_timeout` | 单次 `pip install` 超时（秒） | 300 |

### 原始文档检索（档位1 会话，只读）

构建时把 MinerU 中间产物（`mineru_output/<主题>_part*/` 的 content_list/layout/origin.pdf）加工为只读块索引（`<项目工作区>/doc_index/doc_index.json`，在 style 阶段之前构建），样式/转换/修复/终审会话都可用：

- `doc_search {query}`：按关键词 / 图片文件名 / 页码检索块索引（文本片段、图表标题、公式 LaTeX、bbox），返回**全局页号 pN** 与 bbox，并直接给出精确路径 `source page: view_pdf {path:"source:<file>.pdf", page:N}`（part 级定位，不依赖全局页号累加）；
- `list_source_pages {page?}`：无参数列出「源 PDF → 页范围 → 全局页号」表 + 从 OCR 版面推导的章节起点；带 `{page:N}` 时列出该全局页的正文片段与该页抽出的图片文件名（随后可直接 `view_image`）；
- `view_pdf {path:"source:<file>.pdf", page, left/top/right/bottom, zoom}`：渲染原书某页（或按百分比裁剪/放大）查看真实排版，回执附带该页与裁剪区域的 **mm 尺寸**——与"看自己编译出的 PDF"是**同一个工具**（`zoom_width` 是 `zoom` 的别名，渲染时按目标宽度从 PDF 重渲染，真放大）。

**图片条目带上类型标记、描述与原文（来自 md 里的 DOCVISION 注释）**：MinerU 对没有 caption 的图片块只给一个文件名，于是索引构建时会把 images 阶段写进 `source/*.md` 的机器注释按**图片文件名**接回对应的图片块——`Marker`（`styled-text`/`vector`/`image`）、`Label`（注释里的描述，如 `mind-map diagram`）与 `Content`（STYLED-TEXT 的 `CONTENT:` 原文 / RASTER 的 `DESCRIBE:` 解释文本）。因此 `doc_search` 既可按文件名、也可按"图里写了什么"检索（例如按"知识导图""mind-map"找到那张思维导图），命中与 `list_source_pages` 页详情里都会显示 `[vector] mind-map diagram` 这样的短标签与内容摘要；没有注释的图片块照旧只有文件名（字段留空，不报错），旧版 `doc_index.json` 仍可读取。

样式会话、章节转换会话与全书修复/终审会话都带这套检索工具（终审/修复会话还会同时搜构建树）。

MinerU 产物缺失时自动降级（不注册工具，仅记录日志），不影响主流程。

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `models.text.request_body` | 注入 API 请求体的额外参数（如 enable_thinking），原样合并到请求体顶层 | 见示例 |
| `options.log_level` | 日志等级：info / debug / trace（debug 记请求响应摘要与最终内容，trace 再加流式分片） | info |
| `models.text.stream` | 是否流式接收（SSE）；不写即 true。厂商不支持时自动回退一次非流式 | true |
| `models.text.api_stream_idle_timeout` | 流式模式下两个数据块之间的最大间隔（秒），超时判定卡住；0 取 `api_timeout` | 0 |
| `models.text.thinking` | 顶层 `thinking` 对象：`type` 取 enabled 或 disabled（GLM-4.5+/DeepSeek）。**不要放进 `extra_body`** | 未设置 |
| `models.text.reasoning_effort` | 顶层 `reasoning_effort`（GLM-5.2+，thinking 开启时生效）：max/xhigh/high/medium/low/minimal/none | 未设置 |
| `models.text.api_timeout` | **非流式**整次请求（连接+读取）总超时（秒）；流式模式改用 idle 超时；所有模型条目可覆盖，留空继承 text | 400 |
| `models.text.api_connect_timeout` | 连接/首字节等待超时（秒） | 60 |
| `models.text.api_max_retries` | 非限流错误重试次数（指数退避 2s/4s/8s…封顶 30s） | 3 |
| `models.text.rate_limit_retries` | 429 限流重试上限（指数退避封顶 60s；原为 0=无限+代码上限 100，现默认直接取上限值） | 100 |
| `img2text.concurrency` | AI 图片转文本并发数（原 `options.concurrency`，两处等价） | 10 |
| `img2text.format_fix_attempts` | 格式修复重试次数（0 禁用，1 表示重试一次）；**只能写在 `img2text` 下**，`options.format_fix_attempts` 已不生效 | 1 |
| `img2text.max_tokens` | img2text 单次请求最大输出（`max_tokens`）（原 `options.max_tokens`，两处等价） | 65536 |
| `img2text.temperature` | img2text 采样温度 | 0.10 |
| `img2text.output_language` | AI 描述输出语言（"Chinese"/"English"） | Chinese |
| `models.text` | **必填**：img2text 等基础流程的默认模型（base_url/api_key/model/request_body/stream/thinking…） | - |
| `models.<name>` | 每个专用 AI 的独立配置，空字段继承 `models.text`；`max_tokens`/`temperature` 作为该模型未指定时的兜底；GLM 可用 `tool_stream: true`（工具参数随流返回）与 `thinking.clear_thinking: false`（保留式思考） | - |
| `latex.level` | LaTeX 档位（2=图片矢量化，1=全书转换） | 2 |
| `latex.sessions.{drawing,style,chapter,convert,checker}.*.max_tool_rounds` | 会话工具轮数上限；**代码无内置默认，省略即 0 = 不限制**（随附模板示例写 128）。checker 是唯一例外：不写就是 **50**（不继承 convert 的轮数），显式写 -1 才是无限 | 0（不限制；checker 50） |
| `latex.sessions.*.context_limit` | 会话上下文窗口（tokens），达到阈值自动 AI 压缩 | 131072 |
| `latex.sessions.*.max_tokens` | 该会话**单次请求最大输出**（不含厂商单独计费的思维链预算）：drawing 16384、style 内置 32768、其余 16384（随附模板示例把 drawing/style 写成 131072） | 16384 / style 32768 |
| `latex.sessions.*.temperature` | 该会话采样温度（同 `models.<name>.temperature`，会话配置优先） | 未设置 |
| `latex.sessions.*.compaction_at` | 触发自动压缩的窗口占用比例（0-1），不写默认 0.85（所有会话一致，不是只继承 drawing） | 0.85 |
| `latex.concurrency` | 档位1/档位2 的会话级并发（classify / 档位2 process / 档位1 逐章 convert / style-fix 共用；style、chapters、assemble、终审是单会话） | 3 |
| `latex.insert_image_description` | 档位2 专用：raster 保留原图时是否嵌入 AI 解释文本（`[Image]( … )`）；档位1 不受它控制（总是生成解释块） | false |
| `latex.chapter_granularity` | 档位1 章节拆分粒度：small=按小节拆分 / large=按大章整体拆分 | small |
| `latex.keep_temp_dirs` | 保留 `<项目>/work/temp/` 下的临时工作目录（拆章沙箱、逐章转换/修复工作区 `conv_<章>`、矢量图工作区） | false |
| `latex.keep_session_records` | 保留成功会话的 JSONL 转录（与上一项独立）；debug 日志下两项都必定保留 | false |
| `latex.compile.engine` | 编译引擎（pdflatex / xelatex / lualatex） | xelatex |
| `latex.compile.timeout` | 单次编译超时（秒） | 120 |
| `latex.compile.raster_command` | PDF 栅格化工具 | pdftoppm |
| `latex.compile.raster_dpi` | 矢量图 PDF 栅格化为 PNG 的分辨率（SVG 不可用时的回退产物）（随附模板示例写 220） | 110 |
| `latex.compile.max_fix_rounds` | 编译→核对→修复整体循环上限（修复会话与终审会话共用）（随附模板示例写 40） | 8 |
| `latex.compile.final_review` | 全书编译成功后是否再跑终审会话（汇总/整理/重编译）；未通过只告警、照常交付 | true |
| `verify.enabled` | AI 核对开关标记（默认关闭；核对只能显式运行 `docvision verify`） | false |
| `verify.verifier_model` | `models:` 中支持图像的校验模型代号 | "verifier" |
| `tools.view.image_max` | 同一张图片文件的 `view_image` 软预算（只提醒不拦截；负值不限） | 30 |
| `tools.view.pdf_max` | 同一个 PDF **每页**的 `view_pdf` 软预算（同上） | 25 |
| `tools.view.warn_ratio` | 用满该比例起提醒"仅剩 N 次" | 0.7 |
| `latex.sessions.*.tool_rounds_warn_ratio` | 工具轮次提醒起点比例 | 0.7 |
| `latex.sessions.*.tool_rounds_grace` | 达到 `max_tool_rounds` 后仍可用的额外轮次（负值=无宽限） | 20 |
| `latex.sessions.*.prune_tool_chars` | 本地裁剪阈值：超过该长度的工具结果裁成"前半 + 后 1/4"（负值=不裁剪） | 4096 |
| `latex.sessions.*.keep_images` | 本地裁剪时保留的最近图片数（负值=全保留） | 3 |
| `verify.concurrency` | 核对并发数 | 2 |
| `verify.report_file` | 核对报告文件名 | verify_report.md |
| `paths.latex_output` | 档位2 输出根：每本书的工作区是 `<latex_output>/<项目名>/`（旧版单项目布局则沿用根目录本身） | ./finally_latex |
| `paths.latex_project` | 档位1 输出根：每本书的工作区是 `<latex_project>/<项目名>/`（旧版单项目布局则沿用根目录本身） | ./latex_project |
| `paths.fonts` | AI 字体目录：缺字体时按样式会话报告的清单手动放入，编译环境注入 `TEXINPUTS`/`OSFONTDIR`（无下载工具） | ./fonts |
| `img2text.model` | 基础流程模型（models: 注册表代号，默认 text） | text |
| `latex.checker_model` | 每章核对模型（独立小模型）。核对本身是一个极简会话：只读两个文件（章节 md + 提交的 .tex）与一个文件夹（分片），工具只有 read_file/grep/submit，默认 50 轮；调优里未配置的字段继承 convert（轮数除外） | "checker" |
| `latex.remove_watermark` | 水印处理：true 时样式/转换/核对 AI 会检测并排除水印 | false |
| `paths.logs_dir` | img2text 处理日志目录（`img2text_*.log` + `img2text_error_*.log`） | `./logs` |
| `paths.done_dir` | 分割完成后源文件被归档到的目录；空字符串或与 `input_dir` 相同会报错 | `<input_dir>/done` |

> `latex.remove_watermark` 开启后流程开始时先做一次水印检测：全览页渲染 + markdown 重复图片统计，结果缓存在本项目工作区的 `watermark_memory.json`（档位1 `<latex_project>/<项目名>/`，档位2 `<latex_output>/<项目名>/`）并作为工作记忆注入后续所有会话；水印图片引用直接剔除不再处理。

完整配置见 `config.example.yaml`。

## 目录结构

```
files/                  源 PDF/Office/图片文件
files/done/             分割完成后归档的源文件（成功 split 后从 files/ 移入）
split_files/            分割后的 PDF/DOCX
mineru_output/          MinerU API 返回的解析结果
output/                 合并后的 Markdown 和引用的图片
output/images/{主题}/   按主题组织的图片
finally/                AI 处理后的最终 Markdown
finally/progress_items/ AI 处理进度记录（断点续传）
logs/                   img2text 处理日志（img2text_*.log + img2text_error_*.log）
finally_latex/{项目名}/    档位2 LaTeX 输出（paths.latex_output）：md + figures/ + images/ + progress_items/ + sessions/
latex_project/{项目名}/    档位1 全书工作区：source/ style/ chapters/ work/ build/ out/ 及 progress.json
fonts/                  AI 字体目录（paths.fonts）：缺字体时按样式会话报告的清单手动放入
```

> `files/done/` 在 SplitAll 模式下自动维护：每次 split 成功的源文件会被 `os.Rename` 到这里；DOCX 直通文件（页数低于阈值）保留在 `files/`，等下次评估。
> `--force` 会在 split 前把 `done/` 中同名的源文件移回 `files/` 再处理。
> 旧的 `*.pdf.done` / `*.docx.done` 标记会在 SplitAll 开始时被迁移到 `done/` 并去掉后缀。
> `os.Rename` 跨文件系统会失败（EXDEV），此时仅打印 warning，不会中断 split；如需跨盘归档请在 `paths.done_dir` 选择同盘路径。
> `logs/` 是 T7 新增目录，专门放 `img2text` 处理期间生成的主日志和错误日志。
> `finally/` 仍保存最终 Markdown 和 `progress_items/` 断点续传记录；
> 分析 / 拆分工具默认从 `logs/` 读取，并回退到 `finally/` 以兼容旧日志。

## CI/CD

推送 `v*.*.*` 标签时，GitHub Actions 自动：

1. 运行测试（`go test -v -race ./...`）
2. 交叉编译 5 个平台二进制
3. 创建 GitHub Release 并上传产物（Release 说明取 `CHANGELOG.md` 中该标签的小节）

> 带 `-` 的标签（如 `v1.5.0-beta.3`）属于预发布，工作流会跳过发布任务，不会创建 Release；需要发布时用不带 `-` 的版本号。

```bash
git tag v1.5.0
git push origin v1.5.0
```

## 许可证

MIT License - Copyright (c) 2026 LingNc
