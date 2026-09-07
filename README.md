# DocVision - 文视

PDF 转 Markdown 自动化工作流。通过 MinerU API 解析 PDF，再用 AI 将文档中的图片转换为文本描述，最终输出结构化的 Markdown 文件。

提供 **Go** 实现（推荐）；历史 **Python** 实现已归档至 `legacy/python/`，仅供参考且不再维护。

## 工作流程

```
PDF 文件 -> 分割 -> MinerU API 解析 -> 整理文件 -> AI 图片转文本 -> 日志分析
```

共 5 个步骤：

1. **split** - 将大文件按页数和文件大小限制分割（PDF 和 DOCX，MinerU 限制单次最多 200 页）
2. **mineru** - 调用 MinerU API 解析文件，支持并发、断点续传、上传进度显示
3. **organize** - 整理解析结果，合并分片，按引用收集图片到按主题子目录
4. **img2text** - 用 AI 模型识别图片内容并转为文本，支持并发和上下文增量扩展（工具调用）
5. **latex** - LaTeX 输出（图片矢量化 / 全书转换，见下文；也可独立自助运行）
6. **verify** - AI 核对图片与嵌入内容（默认关闭，`verify.enabled`）
7. **analyze** - 分析处理日志（img2text 与 latex 日志均支持），统计耗时、成功率、进度

## 快速开始

### 配置

```bash
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，填入：

- MinerU API token（从 [mineru.net](https://mineru.net) 获取）
- AI 模型：统一在 `models:` 注册表配置，其中 `models.text` 为 img2text 等基础流程的默认模型（必填）；每个专用 AI（classifier/drawing/style/chapter/convert/checker/verifier）可单独配置 base_url/api_key/model/request_body，空字段自动继承默认条目（兼容旧版顶层 `ai:` 块）

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
make release   # 输出到 go/release/（Linux/macOS/Windows，amd64+arm64）
```

### Mermaid 验证工具（推荐安装）

如果 AI 输出 Mermaid 图，推荐安装 Mermaid CLI：

```bash
# 需要先安装 Node.js/npm
npm install -g @mermaid-js/mermaid-cli
```

`docvision init` 生成配置文件后会询问是否通过 npm 安装 Mermaid CLI；只有确认后才执行 `npm install -g @mermaid-js/mermaid-cli`，不会自动安装 Node.js/npm。

- `mermaid_validation: auto`：找不到 `mmdc` 时给出安装提示，并跳过本次验证；
- `mermaid_validation: strict`：找不到 `mmdc` 时明确报错，需安装后再继续；
- `mermaid_validation: off`：禁用 Mermaid 验证。

### Python 版本

历史 Python 实现已归档至 `legacy/python/`，不再维护，建议使用 Go 版本。

将 PDF 或 DOCX 文件放入 `files/` 目录，运行工作流即可。

## Go 命令行说明

```
docvision workflow    运行完整流水线（默认），或 --step 指定单步
docvision split       分割 PDF/DOCX（单文件或 --all 目录模式）
docvision mineru      调用 MinerU API 解析文件
docvision organize    整理解析结果
docvision img2text    AI 图片转文本（--test 测试模式）
docvision latex       LaTeX 输出（可直接传 PDF/DOCX 自动补前置流程，或传 md 名只处理指定文件）
docvision verify      AI 核对输出与原图（默认关闭）
docvision analyze     分析日志（--progress 仅进度，--all 汇总历史）
docvision splitlog    按线程 ID 拆分日志
docvision init        生成配置模板
```

### img2text 嵌入格式

> **可还原**：嵌入发生前，原版 markdown（图片引用完整）会自动留存到
> `finally/progress_items/<文件名>/original.md`（每个文件只存首次版本），
> 需要回滚时直接复制回去即可。

AI 结果带 `[IMG_TYPE: <类型>]` 标签，写入 `finally/` 的 markdown 时按类型选择嵌入方式：

| 类型 | 嵌入方式 |
| --- | --- |
| text / latex（数学公式）/ table / code | **直接嵌入正文**（无任何包装标记，方便后续 AI 检索阅读） |
| mermaid / tikz | 代码块直接嵌入（tikz 经过 LaTeX 编译校验，失败自动回炉修复） |
| 其余视觉类型（截图/照片/复杂图等） | `[Image]( 可读描述 )` |

tikz 校验由 `options.tikz_validation`（off/auto/strict，默认 auto）与 `options.tikz_engine`（默认 xelatex，自动回退 pdflatex/lualatex）控制；`[IMG_TYPE:]` 标签本身仍保留在进度数据中用于统计与断点续传。

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

**档位 2（默认，`latex.level: 2`）—— 图片矢量化**

1. 专用**分类 AI**（`models.classifier`，需视觉能力）逐图标记：
   - `text`：艺术样式文本（如美化题号）→ 复用图片解释 AI，纯内容嵌入文本流（无 `[AI]`/`[IMG_TYPE]` 标记）
   - `vector`：函数图像/立体结构/流程图等可矢量重绘图形 → 进入作图流程
   - `raster`：照片/截图等 → 保留原图链接（`insert_image_description: true` 时嵌入可读解释文本）
2. **作图 AI**（`models.drawing`）在独立会话中重绘矢量图（TikZ/pgfplots/tabular 等任意 LaTeX 方式）→ `compile_preview` 工具自动编译并栅格化为 PNG 回给模型**视觉核对** → 迭代修正 → `submit` 确认提交。会话附带 `image_context`/`view_image` 工具：跨页拆分的长表/大图可查看相邻图片及其上下文，**一次绘制合并图**并在 submit 时声明吸收的续片（续片引用自动删除、不再重复处理）
3. 编译产物 PDF 按矢量图嵌入 Markdown（`insert_image_description: true` 时嵌入 tikz 代码块）；矢量转换失败时回退保留原图并输出警告日志

**档位 1（`latex.level: 1`）—— 全书 LaTeX**

1. **样式分析 AI**（`models.style`）：拥有独立虚拟工作区（`latex_project/work/style/`），通过 `view_page` 工具**按需渲染** MinerU 保留的原始扫描页（`*_origin.pdf`，调用哪页渲染哪页并缓存，支持裁剪放大）、`list_images`/`view_image` 看提取图、`read_md` 读解析文本，分析全书样式后用 `write_file` 增量起草并 `submit_style` 提交 `book.cls` + 使用手册 + 案例（自动试编译，失败回炉）。字体通过 `list_fonts` 查看、`install_font` 下载到 `paths.fonts`；无法提供的字体会标注替换方法
2. **章节划分 AI**（`models.chapter`）：grep 检索 + 最小 bash 沙箱（虚拟文件系统只有这一个文件），按行号划分章节（结构化提交，全覆盖校验）
3. **转换 AI** 并发逐章转 `.tex`（只读全部章节文件与他人产物，仅可写自己的文件）→ 每章提交后由 **核对 AI**（`models.checker`，小文本模型即可）比对原章 md 与产物，发现问题回炉一轮，遗留问题记录为 `.checker` 备注供终审处理
4. 汇总为多文件 `.tex` 项目 → 编译全书 PDF（失败进入**修复会话**：读文件/改文件/重编译 + 字体工具核对样式）→ 生成单文件 `standalone.tex`

### AI 会话基础设施

- **模型注册表** `models:`：每个专用 AI（classifier/drawing/style/chapter/convert/checker/verifier）可单独配置 base_url / api_key / model / request_body
- **会话管理**：每个会话独立上下文窗口（`sessions.*.context_limit`，默认 128K，可设 64K/256K），达到阈值自动 **AI 压缩**会话（保留关键决策/成果，丢弃草稿与工具噪音）
- **可分离工具**：会话工具按需注册（编译预览、提交确认、grep、bash 沙箱、受限文件读写等）
- **断点续传**：档位2逐图进度、档位1逐阶段进度（progress.json）

### 调试日志

```bash
docvision latex --debug        # 或配置 options.log_level: "debug"
docvision latex --verbose      # 详细控制台输出（默认仅显示进度行）
```

开启后，每一轮 AI 调用的**完整系统提示词、用户提示词、工具调用（名称+参数）、工具结果**都会写入 `logs/latex_*.log`（`[DEBUG]` 前缀，控制台输出不受影响）。可在日志里完整回放某个会话的推理与工具使用过程。

默认控制台输出与 img2text 一致：每个阶段只显示一行实时进度（如 `[classify 12/345] 3.48% (失败: 0)`），逐图明细写入日志文件；`--verbose` 恢复逐图控制台输出。

### 档位1 目录布局（latex_project/）

| 目录 | 用途 |
| --- | --- |
| `source/` | images 阶段整理的 md 输入 |
| `pages/` | 原始扫描页按需渲染缓存（view_page 调用哪页渲染哪页） |
| `style/` | 样式分析产物（book.cls / manual.md / example.tex） |
| `work/style/` | 样式分析 AI 的虚拟工作区（write_file 增量起草） |
| `chapters/` | 章节划分 AI 产出的 chapter_00N.md |
| `work/` | 转换 AI 虚拟根（每个会话只能写 `work/chapters/<章>.tex`） |
| `build/` | 终审编译目录（每次 clean 重建，多文件 .tex + 编译） |
| `out/` | 最终产物：book.pdf、main.tex、standalone.tex |
| `progress.json` | 逐阶段断点进度 |

章节划分 AI 本身没有写目录：它只读全书 md（虚拟 bash 沙箱 + grep/按行读取工具），通过结构化 submit 提交切分方案，由代码落盘到 `chapters/`。
### AI 核对（verify，默认关闭）

`verify.enabled: true` 后，用配置的视觉校验模型逐项核对每张图与其嵌入内容，生成 `verify_report.md`（问题 + 修改意见，不改动输出）。也可显式运行：

```bash
docvision verify
```

### 用法

```bash
# ★ 最常用：一条命令从原始文档到 LaTeX（自动补跑 分割→MinerU→整理）
docvision latex 书.pdf
docvision latex 书.docx
docvision latex 目录/              # 目录下所有 PDF/DOCX

# output/ 已经有解析结果时：直接对某个 md 继续 LaTeX（跳过前置）
docvision latex 编译原理课程设计报告样例.md
docvision latex book1 book2

# 批量：处理 output/ 下全部 markdown
# （前置流程已跑过时，裸命令即批量；未跑过会提示先跑 workflow）
docvision latex

# 档位与阶段控制
docvision latex --level 1 书.pdf               # 档位1 全书转换
docvision latex --level 2 --test --number 5    # 抽样测试
docvision latex --level 1 --step style 书.md   # 仅样式分析阶段

docvision verify                               # AI 核对报告
```

> 说明：`docvision latex` 是**自助式**的——传 PDF/DOCX 会自动隔离在
> `~/.docvision/jobs/` 作业目录里补跑前置流程，不需要先手动跑 workflow。
> 老用户仍可把 latex 作为 workflow 步骤：`docvision workflow --step latex`
> （split→mineru→organize→latex→analyze 一条龙，`--step verify` 同理）。

输出目录：档位2 → `finally_latex/`；档位1 → `latex_project/`（含 out/book.pdf 与 standalone.tex），与 img2text 的 `finally/` 互不干扰。

需要本地 LaTeX 工具链：TeX 发行版（xelatex）+ poppler-utils（pdftoppm）。`docvision init` 会自动检查并给出安装提示。

### 风格分析的数据来源（档位1）

样式分析 AI 直接查看 **MinerU 保留的原始扫描页面**（`mineru_output/<主题>_part*/*_origin.pdf`，自动用 pdftoppm 渲染为整页 PNG 并缓存到 `latex_project/pages/`），支持按页浏览、按百分比裁剪、放大查看细节——这才是真实排版（标题格式、页眉页脚、字号行距）。原始 PDF 缺失时退化为仅用提取图片 + 解析 md 分析，并输出日志提示。

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
| `models.text.request_body` | 注入 API 请求体的额外参数（如 enable_thinking） | 见示例 |
| `options.concurrency` | AI 图片转文本并发数 | 10 |
| `options.max_retries` | AI 请求更多上下文的最大轮数 | 5 |
| `options.max_context_lines_up` | 图片上方初始上下文行数 | 10 |
| `options.max_context_lines_down` | 图片下方初始上下文行数 | 5 |
| `options.api_connect_timeout` | API 连接超时（秒） | 60 |
| `options.api_max_retries` | 非限流错误的 API 重试次数 | 3 |
| `options.rate_limit_retries` | 限流错误重试次数（0 表示无限次，由代码设置上限） | 0 |
| `options.format_fix_attempts` | 格式修复重试次数（0 禁用，1 表示重试一次） | 1 |
| `options.mermaid_validation` | Mermaid 验证模式（off/auto/strict） | auto |
| `options.mermaid_command` | Mermaid CLI 命令 | mmdc |
| `options.mermaid_fix_attempts` | Mermaid 独立修正次数（0 表示无限次，受代码内安全上限保护） | 3 |
| `options.mermaid_timeout` | 单个 Mermaid 验证超时（秒） | 30 |
| `options.max_tokens` | API 调用最大 token 数 | 65536 |
| `models.text` | **必填**：img2text 等基础流程的默认模型（base_url/api_key/model/request_body） | - |
| `models.<name>` | 每个专用 AI 的独立 base_url/api_key/model/request_body，空字段继承 `models.text` | - |
| `latex.level` | LaTeX 档位（2=图片矢量化，1=全书转换） | 2 |
| `latex.sessions.*.max_tool_rounds` | 会话工具轮数上限，0=真正不限制（无安全上限） | 128 |
| `latex.sessions.*.context_limit` | 会话上下文窗口（tokens），达到阈值自动 AI 压缩 | 131072 |
| `verify.enabled` | AI 核对开关（默认关闭） | false |
| `paths.latex_output` | 档位2 LaTeX 输出目录 | ./finally_latex |
| `paths.latex_project` | 档位1 全书工作目录 | ./latex_project |
| `paths.fonts` | AI 字体目录（install_font 可下载字体到此） | ./fonts |
| `img2text.model` | 基础流程模型（models: 注册表代号，默认 text） | text |
| `options.tikz_validation` | img2text tikz 代码块 LaTeX 编译校验（off/auto/strict） | auto |
| `options.tikz_engine` | tikz 校验引擎（缺 pdflatex/lualatex 自动回退） | xelatex |
| `latex.checker_model` | 每章核对模型（留空用 convert_model） | 空 |
| `latex.remove_watermark` | 水印处理：true 时样式/转换/核对 AI 会检测并排除水印 | false |
| `paths.logs_dir` | img2text 处理日志目录（`img2text_*.log` + `img2text_error_*.log`） | `./logs` |
| `paths.done_dir` | 分割完成后源文件被归档到的目录；空字符串或与 `input_dir` 相同会报错 | `<input_dir>/done` |

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

1. 运行测试（`go test -race ./...`）
2. 交叉编译 5 个平台二进制
3. 创建 GitHub Release 并上传产物

```bash
git tag v0.1.2
git push origin v0.1.2
```

## 许可证

MIT License - Copyright (c) 2026 LingNc
