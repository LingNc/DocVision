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

1. **样式分析 AI**（`models.style`）：拥有独立虚拟工作区（`latex_project/work/style/`），用 `list_source_pages` 取原书页索引（源 PDF / 页范围 / 全局页号 / OCR 推导的章节起点），再用 `view_pdf {path:"source:<file>.pdf", page:N}` **按需渲染** MinerU 保留的原始扫描页（支持裁剪放大）、`view_image` 看提取图、`read_file`/`grep` 读解析文本，分析全书样式后用 `write_file`/`edit_file` 增量起草并 `submit_style` 提交 `book.cls` + 使用手册 + 案例（自动试编译，失败回炉）。字体通过 `list_fonts` 查看；缺字体时在提交报告与手册里列出清单，由用户按文件名手动放进 `paths.fonts`（编译环境已注入 `TEXINPUTS`/`OSFONTDIR`，无需装系统字体）
2. **章节划分 AI**（`models.chapter`）：`grep`/`read_file`/`bash` + `buffer.md` 工作记忆缓冲区（分析结论增量写入、跨轮保留），按行号划分章节（结构化提交，全覆盖校验）
3. **转换 AI** 并发逐章转 `.tex`：每章有**私有工作视图**（只能读写自己那一章），别人的成品只能经只读通道 `project:converted/`（其他章节已提交的 `.tex`）与 `project:reports/`（其他章节的工作汇报）参考；矢量图已在 images 阶段以 latex 代码块内嵌进章节 md，转换时按手册的 `## Vector figure style` 二次加工（保留结构、几何与全部标签）；raster 原图走 includegraphics → 每章提交后由 **核对 AI**（`models.checker`，小文本模型即可）比对原章 md 与产物，**硬性问题回同一个转换会话最多 3 轮**（上下文还在，最省 token），3 轮仍未过才作废该章 `.tex`、资源目录与转录，用全新会话重转换一次
4. 汇总为多文件 `.tex` 项目 → 编译全书 PDF（失败进入**修复会话**：读文件/改文件/重编译 + 字体工具核对样式）→ 成功后进入**终审会话**逐页核对成品 PDF 并整理（`latex.compile.final_review`，默认开启）→ 生成单文件 `standalone.tex`；交付把整棵 build 树复制到 `out/`，`book.pdf` 只是 `main.pdf` 的别名
5. 多数章节的工作汇报报告 cls/手册问题时，打回原样式会话修正（上限 2 轮）；样式包更新后只对「报问题」或「新 cls 下编译不过」的章节并发跑**样式修复子会话**（增量 `edit_file` 适配新 cls/手册，不重新转换），修复失败才退回整章重转换

### AI 会话基础设施

- **模型注册表** `models:`：每个专用 AI（classifier/drawing/style/chapter/convert/checker/verifier）可单独配置 base_url / api_key / model / request_body / stream / thinking / reasoning_effort / tool_stream
- **流式请求（默认开启）**：`models.*.stream: true`（默认）时走 SSE 流式接收，长思考/长输出期间持续有进展，不会长时间静默；厂商不支持流式时自动回退一次非流式请求。流式模式下用 `api_stream_idle_timeout`（默认取 `api_timeout`）判定"卡住"，而不是整次请求超时
- **思考控制**：`thinking: {type: enabled|disabled}` 与 `reasoning_effort: max|xhigh|high|medium|low|minimal|none` 都是请求体**顶层字段**（不要写进 `request_body.extra_body`），按 `models` 条目配置，空值继承 `models.text`；GLM 的保留式思考写 `thinking.clear_thinking: false`（历史 assistant 轮的思维链完整回传，提升缓存命中）
- **会话管理**：每个会话独立上下文窗口（`sessions.*.context_limit`，默认 128K，可设 64K/256K），达到 `compaction_at`（默认 0.85）阈值自动压缩，**两级**：先做**本地裁剪**（零成本，不调用模型）——超长工具结果只留头尾（`sessions.*.prune_tool_chars`，默认 8192 字符），较早的图片换成文字占位（`sessions.*.keep_images`，默认保留最近 3 张；占位写明"这里曾附过 N 张图，需要时重新 view_image"，所以"看过什么"不会丢）；仍超阈值才花一次 **AI 摘要**：**系统提示词、原始任务、最近 8 条消息原样保留**，中间部分由 AI 写成一篇续跑笔记替换（摘要请求是"在原会话末尾追加一条指令"的增量请求，因此仍能命中厂商前缀缓存）；token 估算现在把系统提示词与工具定义一并计入（原来漏算，实测低估 5 万+ tokens）；`sessions.checker` 未设置的字段继承 `sessions.convert`
- **矢量图落盘**：TikZ 编译成功后依次尝试 `dvisvgm → pdftocairo → mutool → inkscape` 转 SVG 内嵌（dvisvgm 3.6 处理 PDF 需要 Ghostscript < 10.01 或 mutool，缺条件时自动走 pdftocairo）；全部失败才回退 PNG/PDF 链接——档位2 只记日志与 `progress.json`，档位1 另在 markdown 就地标注 `<!-- DOCVISION-ERROR: … -->`
- **图片查看工具**：`image_context` 只给文本上下文与前后引用（不看像素），`view_image` 才看图片（支持百分比裁剪与放大）；两者都接受**裸文件名**——`view_image` 以本文档的图片目录为根直接拼接，不做搜索，名字错了就报错
- **编译与看图分离**：`compile {path:"figure.tex"}` 只返回编译日志、产物名与页数，看图统一用 `view_pdf {path:"standalone.pdf", page:1}`（裁剪 + zoom 直接从 PDF 高分辨率重渲染，真放大，不是拉伸像素）
- **可分离工具**：会话工具按需注册（编译、提交确认、grep、bash 沙箱、受限文件读写、PDF/图片查看等）
- **断点续传**：档位2 逐图进度、档位1 逐阶段进度（`progress.json`）
- **工具轮次软限制**：`max_tool_rounds` 限制的是**assistant 轮次**（一轮里发多少个 tool_call 都只算 1 次）。用满 `sessions.*.tool_rounds_warn_ratio`（默认 0.7）后，每轮往会话里更新一条提醒（"已用 N/M 轮，还剩 K 轮，请合理使用并尽快提交"）；到达 `max_tool_rounds` 后**还能再用 `sessions.*.tool_rounds_grace` 轮**（默认 20，负值=不留宽限），此后才真正禁用工具、逼最终文本。提醒是**原地替换**同一条消息，不膨胀历史也不破坏前缀缓存。
- **看图软预算**：`tools.view.image_max`（默认 30）与 `tools.view.pdf_max`（默认 25）是**每会话**的软预算：用满 `tools.view.warn_ratio`（默认 0.7）起，每次 `view_image`/`view_pdf` 的结果里附带"已用 N/30，仅剩 K 次"（30×0.7=21 → 从第 21 次起提醒；取整用向上取整），超出后提示"预算已用尽，请尽快完成并提交"——**只提醒，不拦截调用**（0=默认值，负值=不限）。
- **原图尺寸测量**：每次 `view_image` 都会实时算出原图的**印刷尺寸**并回给模型——从位图回溯到 MinerU 解析目录（`content_list.json` 的 bbox + `layout.json` 的页尺寸），显示标准为 **mm 优先**：`ORIGINAL FIGURE SIZE: 36.9mm x 20.2mm on the page (about 21% of the page width); bitmap 284x156px, effective resolution 195 dpi, aspect 1.82:1`（高度按位图自身比例换算，因为 MinerU 的块 bbox 不紧贴图）。测不出解析目录时退化为只给宽高比。
- **会话转录（JSONL）**：每条消息实时追加为一行 JSON（含 `reasoning_content` 思维链——GLM 保留式思考要求历史思维链完整回传，也是前缀缓存的前提），图片以 `file://media/<hash>.<ext>` 引用（base64 不入转录）；矢量图会话（`<outDir>/sessions/vector_<图>.jsonl`）、样式会话（`work/style_session.jsonl`）、章节划分（`work/sessions/chapters.jsonl`）、单章转换（`work/sessions/convert_<章>.jsonl`）都接入——进程被杀或网络断连后，下次运行自动从转录恢复上下文续跑，不重烧 token（恢复时**重新挂上系统提示词**、只回放最近一次压缩之后的消息）；成功会话的转录默认清理（`latex.keep_session_records` 可保留）

### 会话沙箱与虚拟工作区（挂载表）

每个 AI 会话有独立的命名空间（挂载表），结构化工具与 `bash` 看到**完全同一套路径**，只在需要时挂对应的树——模型拿不到整个 `mineru_output`、转录或构建产物：

| 挂载点 | 内容 | 权限 |
| --- | --- | --- |
| `work` | 会话自己的可写工作区——作图/拆章会话=专用 scratch；样式会话=`work/style`；转换与样式修复会话=`work/views/chapter_<章>`（只含自己那一章）；修复/终审会话=构建树 `build/` | 可写 |
| `project` | 项目只读窄视图：`source`/`style`/`chapters`/`converted`/`reports`；`work/sessions` 转录、`work/temp`、`doc_index/`、`pages/`、`build/`、`out/` 都不在其中 | 只读 |
| `source` | 本书原书 PDF 的最小视图（`work/pdfview/<part>.pdf`，只含本书的 `*_origin.pdf`） | 只读 |
| `build` | 只有转换与样式修复会话额外挂载的**编译 scratch**——它是这两个会话表里的**第一个**挂载点，也就是默认挂载点，所以它们的相对路径（含 `compile` 产物与 `standalone.pdf`）都落在 scratch 里 | 可写 |

挂载表的**第一个挂载点即默认挂载点**：写 `x.tex` 等价于写 `<默认挂载点>:x.tex`。

路径语法：`path`（默认挂载点）、`name:path`（如 `project:style/manual.md`）、`/name/path`（bash 侧同路径）；越界 `../` 与只读挂载点写入会被直接拒绝，未知挂载点报错并列出可用挂载点。

`tools.bash.sandbox`（默认 true）把会话 `bash` 包进 **bubblewrap**：沙箱里只存在上表的树（`/work`、`/project`、`/source`），宿主真实路径一律不存在，`--unshare-net` 断网、`/tmp` 为私有 tmpfs；bwrap 缺失时自动回退普通 shell 并记警告。

档位1 的临时工作区统一落在 `latex_project/work/temp/<名称>`（拆章沙箱、样式/反馈 scratch、每章编译 scratch），不再藏进 `/tmp`；`latex.keep_temp_dirs` 可保留以便事后检查。

### 调试日志

```bash
docvision latex --debug        # 或配置 options.log_level: "debug"
docvision latex --trace        # 或配置 options.log_level: "trace"（更细）
docvision img2text --debug     # img2text 同样支持
docvision verify --debug
docvision latex --verbose      # 详细控制台输出（默认仅显示进度行，5-10s 自动刷新一次；进度行只在控制台，日志文件里看并发与耗时）
```

日志等级：`info`（默认，进度与警告/错误）< `debug`（每轮请求/响应摘要、提示词、工具调用、**最终接收内容**、编译结果与警告、Mermaid/LaTeX 校验结论）< `trace`（再加流式分片进展行等噪音）。全部写入日志文件（`[DEBUG]`/`[TRACE]` 前缀，控制台输出不受影响），每次请求/响应记录：使用的模型、`stream`/`max_tokens`/`temperature`/`thinking`/`reasoning_effort` 实际取值、消息数与上下文估算、耗时、finish_reason、输出与思维链字符数、provider 返回的 token 用量（含 `reasoning_tokens`）。LaTeX 编译只记 `OK|FAILED + 耗时 + warnings=N + 警告清单`（失败时附给 AI 的错误原文），完整编译日志不会写入。可在日志里完整回放某个会话的推理与工具使用过程。

默认控制台输出与 img2text 一致：每个阶段只显示一行实时进度（如 `[classify 12/345] 3.48% (failed: 0, running: 2)`；process 行还带 `done/errors/fallback/raster` 计数），逐图明细写入日志文件；`--verbose` 恢复逐图控制台输出。

`--debug`（或 `options.log_level: debug`）下，临时工作目录与会话转录**必定保留**（`latex.keep_temp_dirs` / `latex.keep_session_records` 打开也保留），因此一次完整运行事后可以逐会话复现。

### 档位1 目录布局（latex_project/）

| 目录 | 用途 |
| --- | --- |
| `source/` | images 阶段整理的 md 输入（另有 `source/progress_items/` 逐图进度） |
| `pages/` | 原始扫描页渲染缓存（仅水印采样使用；会话看图走 `view_pdf` 现场渲染，不缓存） |
| `style/` | 样式分析产物（book.cls / manual.md / example.tex） |
| `work/style/` | 样式分析 AI 的虚拟工作区（write_file 增量起草；转录 `work/style_session.jsonl`） |
| `chapters/` | 章节划分 AI 产出的 chapter_00N.md |
| `work/chapters/` | 转换 AI 的正式产物：`chapter_00N.tex` + 每章资源目录 `chapter_00N/` |
| `work/views/project/` | `project` 挂载点（只读窄视图：source / style / chapters / converted / reports） |
| `work/views/chapter_<章>/` | 单章转换会话的**私有工作视图**（只含自己那章的 .tex 与资源目录） |
| `work/pdfview/` | 原书 PDF 最小视图（`source` 挂载点，`<part>.pdf` 软链到 MinerU 的 `*_origin.pdf`） |
| `work/reports/` | 每章转换会话的工作汇报 `<章>.md`（转换期间，只读通道 `project:reports/`） |
| `work/sessions/` | 会话转录（`chapters.jsonl`、`convert_<章>.jsonl`）；矢量图另有 `sessions/vector_<图>.work` 持久工作区 |
| `work/temp/` | 临时工作区（拆章沙箱、样式/反馈 scratch、每章编译 scratch），默认用完即删 |
| `build/` | 全书的构建树（每次 clean 重建：cls/手册/案例 + chapters + figures + main.tex），同时是修复会话与终审会话的工作区 |
| `out/` | 交付产物：整棵 build 树（跳过 .aux/.log/.toc/.synctex 等中间文件），`book.pdf` 是 `main.pdf` 的别名，另有 `standalone.tex` |
| `doc_index/doc_index.json` | 只读块索引（`doc_search` 的检索库，style 阶段之前构建） |
| `watermark_memory.json` | 水印检测工作记忆（`latex.remove_watermark` 开启时生成，两档位共享） |
| `progress.json` | 逐阶段断点进度 |

> 档位2（`paths.latex_output`，默认 `finally_latex/`）的会话文件：`sessions/vector_<图>.jsonl`（转录）与 `sessions/vector_<图>.work/`（持久工作区），另有逐图进度 `progress_items/`。

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

docvision verify                               # AI 核对报告
```

> 说明：`docvision latex` 是**自助式**的——位置参数支持 `files/` 里的 PDF/DOCX（自动复制进 `files/` 并在项目内补跑 split→mineru→organize）和 md 名（复制进 `files/` 后整理进 `output/`），**目录与不带扩展名的裸文件名不受支持**；前置流程跑完它自己接日志分析，不需要先手动跑 workflow。
> `latex` 与 `verify` 都**不是** workflow 步骤（`workflow --step` 只接受 split|mineru|organize|img2text|analyze）。需要"中间文件隔离在 `~/.docvision/jobs/` 的一次性作业目录"时，用 `docvision workflow <文件或目录>`。

输出目录：档位2 → `finally_latex/`；档位1 → `latex_project/`（含 out/book.pdf 与 standalone.tex），与 img2text 的 `finally/` 互不干扰。

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

### 原始文档检索（档位1 会话，只读）

构建时把 MinerU 中间产物（`mineru_output/<主题>_part*/` 的 content_list/layout/origin.pdf）加工为只读块索引（`latex_project/doc_index/doc_index.json`，在 style 阶段之前构建），样式/转换/修复/终审会话都可用：

- `doc_search {query}`：按关键词 / 图片文件名 / 页码检索块索引（文本片段、图表标题、公式 LaTeX、bbox），返回**全局页号 pN** 与 bbox，并直接给出精确路径 `source page: view_pdf {path:"source:<file>.pdf", page:N}`（part 级定位，不依赖全局页号累加）；
- `list_source_pages {page?}`：无参数列出「源 PDF → 页范围 → 全局页号」表 + 从 OCR 版面推导的章节起点；带 `{page:N}` 时列出该全局页的正文片段与该页抽出的图片文件名（随后可直接 `view_image`）；
- `view_pdf {path:"source:<file>.pdf", page, left/top/right/bottom, zoom}`：渲染原书某页（或按百分比裁剪/放大）查看真实排版——与"看自己编译出的 PDF"是**同一个工具**（`zoom_width` 是 `zoom` 的别名，渲染时按目标宽度从 PDF 重渲染，真放大）。

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
| `latex.sessions.{drawing,style,chapter,convert,checker}.*.max_tool_rounds` | 会话工具轮数上限；**代码无内置默认，省略即 0 = 不限制**（随附模板示例写 128）。checker 继承时 0 表示"继承 convert"，-1 才是无限 | 0（不限制） |
| `latex.sessions.*.context_limit` | 会话上下文窗口（tokens），达到阈值自动 AI 压缩 | 131072 |
| `latex.sessions.*.max_tokens` | 该会话**单次请求最大输出**（不含厂商单独计费的思维链预算）：drawing 16384、style 内置 32768、其余 16384（随附模板示例把 drawing/style 写成 131072） | 16384 / style 32768 |
| `latex.sessions.*.temperature` | 该会话采样温度（同 `models.<name>.temperature`，会话配置优先） | 未设置 |
| `latex.sessions.*.compaction_at` | 触发自动压缩的窗口占用比例（0-1），不写默认 0.85（所有会话一致，不是只继承 drawing） | 0.85 |
| `latex.concurrency` | 档位1/档位2 的会话级并发（classify / 档位2 process / 档位1 逐章 convert / style-fix 共用；style、chapters、assemble、终审是单会话） | 3 |
| `latex.insert_image_description` | 档位2 专用：raster 保留原图时是否嵌入 AI 解释文本（`[Image]( … )`）；档位1 不受它控制（总是生成解释块） | false |
| `latex.chapter_granularity` | 档位1 章节拆分粒度：small=按小节拆分 / large=按大章整体拆分 | small |
| `latex.keep_temp_dirs` | 保留 `<项目>/work/temp/` 下的临时工作目录（拆章沙箱、编译 scratch、矢量图工作区） | false |
| `latex.keep_session_records` | 保留成功会话的 JSONL 转录（与上一项独立）；debug 日志下两项都必定保留 | false |
| `latex.compile.engine` | 编译引擎（pdflatex / xelatex / lualatex） | xelatex |
| `latex.compile.timeout` | 单次编译超时（秒） | 120 |
| `latex.compile.raster_command` | PDF 栅格化工具 | pdftoppm |
| `latex.compile.raster_dpi` | 矢量图 PDF 栅格化为 PNG 的分辨率（SVG 不可用时的回退产物）（随附模板示例写 220） | 110 |
| `latex.compile.max_fix_rounds` | 编译→核对→修复整体循环上限（修复会话与终审会话共用）（随附模板示例写 40） | 8 |
| `latex.compile.final_review` | 全书编译成功后是否再跑终审会话（汇总/整理/重编译）；未通过只告警、照常交付 | true |
| `verify.enabled` | AI 核对开关标记（默认关闭；核对只能显式运行 `docvision verify`） | false |
| `verify.verifier_model` | `models:` 中支持图像的校验模型代号 | "verifier" |
| `tools.view.image_max` | `view_image` 每会话软预算（只提醒不拦截；负值不限） | 30 |
| `tools.view.pdf_max` | `view_pdf` 每会话软预算（同上） | 25 |
| `tools.view.warn_ratio` | 用满该比例起提醒"仅剩 N 次" | 0.7 |
| `latex.sessions.*.tool_rounds_warn_ratio` | 工具轮次提醒起点比例 | 0.7 |
| `latex.sessions.*.tool_rounds_grace` | 达到 `max_tool_rounds` 后仍可用的额外轮次（负值=无宽限） | 20 |
| `latex.sessions.*.prune_tool_chars` | 本地裁剪：超长工具结果的保留字符数（负值=不裁剪） | 8192 |
| `latex.sessions.*.keep_images` | 本地裁剪时保留的最近图片数（负值=全保留） | 3 |
| `verify.concurrency` | 核对并发数 | 2 |
| `verify.report_file` | 核对报告文件名 | verify_report.md |
| `paths.latex_output` | 档位2 LaTeX 输出目录 | ./finally_latex |
| `paths.latex_project` | 档位1 全书工作目录 | ./latex_project |
| `paths.fonts` | AI 字体目录：缺字体时按样式会话报告的清单手动放入，编译环境注入 `TEXINPUTS`/`OSFONTDIR`（无下载工具） | ./fonts |
| `img2text.model` | 基础流程模型（models: 注册表代号，默认 text） | text |
| `latex.checker_model` | 每章核对模型（独立小模型；会话调优未配置的字段继承 convert） | "checker" |
| `latex.remove_watermark` | 水印处理：true 时样式/转换/核对 AI 会检测并排除水印 | false |
| `paths.logs_dir` | img2text 处理日志目录（`img2text_*.log` + `img2text_error_*.log`） | `./logs` |
| `paths.done_dir` | 分割完成后源文件被归档到的目录；空字符串或与 `input_dir` 相同会报错 | `<input_dir>/done` |

> `latex.remove_watermark` 开启后流程开始时先做一次水印检测：全览页渲染 + markdown 重复图片统计，结果缓存为 `latex_project/watermark_memory.json` 并作为工作记忆注入后续所有会话；水印图片引用直接剔除不再处理。

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
finally_latex/          档位2 LaTeX 输出（paths.latex_output）：md + figures/ + images/ + progress_items/ + sessions/
latex_project/          档位1 全书工作目录：source/ style/ chapters/ work/ build/ out/ 及 progress.json
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
