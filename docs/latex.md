# LaTeX 输出（docvision latex）

> 返回 [README](../README.md)（文档索引见 README「文档」一节）。


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

## 档位1 目录布局（`latex_project/<项目名>/`）

每本书一个工作区：`<latex_project>/<项目名>/`（项目名 = 主题名或 `--project`；旧版单项目布局时就是 `latex_project/` 本身，下表同）。下表所有路径都相对于**项目工作区**：

| 目录 | 用途 |
| --- | --- |
| `source/` | images 阶段整理的 md 输入（另有 `source/progress_items/` 逐图进度，以及 `source/images/<书>/` —— 见下条） |
| `source/images/<书名>/` | 该书**全部被引用的插图**，按 md 里的相对路径就位（逐文件软链到全局 `paths.images_dir`，目标是**绝对路径**，失败则复制）。md 用 `images/<书>/<sha>.jpg` 这种相对引用，所以章节工作区、`assemble` 的 build 树、样式会话的 `view_image` 全都靠这棵树解析——曾因这里没铺图而出现"样式会话拿着 md 里的路径却报文件不存在"。软链目标一律绝对：`os.Symlink` 原样存目标串、内核按**软链所在目录**解析，配置里的相对 `paths.images_dir` 会造出读不到的死链。每次 images 阶段开头与 `assemble` 前都会**自愈**（能读到的不动，死链重建） |
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
| `doc_index/doc_index.json` | 只读块索引（`doc_search` 的检索库，style 阶段之前构建；图片条目带 md 里 DOCVISION 注释的类型/描述/原文：`marker`（styled-text/vector/image）、`label`（描述）、`content`（styled-text 的印刷原文 / raster 的解释 / **矢量图的 LaTeX 本体**）） |
| `watermark_memory.json` | 水印检测工作记忆（`latex.remove_watermark` 开启时生成；档位1 落在本项目工作区，档位2 同理落在 `<latex_output>/<项目名>/`） |
| `progress.json` | 逐阶段断点进度 |
| `.docvision_project.json` | 项目自述（项目名 + 主题名来源，仅新建项目写入；同名去重依据；旧版单项目目录不写） |

> 多项目：`latex_project/` 下可以并存 `测试-概率论/`、`线性代数/` …，各自拥有上表全部内容，互不覆盖。项目名默认取主题名（主 md 主文件名），`--project <名字>` 可覆盖。
>
> 档位2（`paths.latex_output`，默认 `finally_latex/`）的工作区同样是 `<latex_output>/<项目名>/`，其中：会话文件 `sessions/vector_<图>.jsonl`（转录）与 `sessions/vector_<图>.work/`（持久工作区），逐图进度 `progress_items/`，另有整理后的 md 与 `figures/`、`images/`、`tikz/`。

章节划分 AI 本身没有写目录：它只在自己沙箱里的 `book.md` + `buffer.md` 上工作（bash + grep/read_file/edit_file），通过结构化 submit 提交切分方案，由代码落盘到 `chapters/`。
## AI 核对（verify，默认关闭）

`docvision verify` 用配置的视觉校验模型（`verify.verifier_model`）逐项核对每张图与其嵌入内容，生成 `verify_report.md`（问题 + 修改意见，不改动输出）。核对只能**显式运行**——`verify.enabled` 不再驱动任何自动流程：

```bash
docvision verify
```

## 用法

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

## 风格分析的数据来源（档位1）

样式分析 AI 直接查看 **MinerU 保留的原始扫描页面**（`mineru_output/<主题>_part*/*_origin.pdf` 以软链形式暴露为只读 `source` 挂载点下的 `work/pdfview/<part>.pdf`，模型看不到整个 `mineru_output` 树；用 `list_source_pages` 取页索引与章节起点、`view_pdf {path:"source:<file>.pdf", page:N}` 按需 pdftoppm 渲染整页 PNG），支持按页浏览、按百分比裁剪、放大查看细节——这才是真实排版（标题格式、页眉页脚、字号行距）。原始 PDF 缺失时退化为仅用提取图片 + 解析 md 分析，并输出日志提示。

## 原始文档检索（档位1 会话，只读）

构建时把 MinerU 中间产物（`mineru_output/<主题>_part*/` 的 content_list/layout/origin.pdf）加工为只读块索引（`<项目工作区>/doc_index/doc_index.json`，在 style 阶段之前构建），样式/转换/修复/终审会话都可用：

- `doc_search {query}`：按关键词 / 图片文件名 / 页码检索块索引（文本片段、图表标题、公式 LaTeX、bbox），返回**全局页号 pN** 与 bbox，并直接给出精确路径 `source page: view_pdf {path:"source:<file>.pdf", page:N}`（part 级定位，不依赖全局页号累加）；
- `list_source_pages {page?}`：无参数列出「源 PDF → 页范围 → 全局页号」表 + 从 OCR 版面推导的章节起点；带 `{page:N}` 时列出该全局页的正文片段与该页抽出的图片文件名（随后可直接 `view_image`）；
- `view_pdf {path:"source:<file>.pdf", page, left/top/right/bottom, zoom}`：渲染原书某页（或按百分比裁剪/放大）查看真实排版，回执附带该页与裁剪区域的 **mm 尺寸**——与"看自己编译出的 PDF"是**同一个工具**（`zoom_width` 是 `zoom` 的别名，渲染时按目标宽度从 PDF 重渲染，真放大）。

**`list_source_pages` / `doc_search` 的内容来自 OCR 索引，不是我们生成的 md**：条目、页码、bbox、文本与表格/公式内容都取自 MinerU 的 `*_content_list.json`（版面识别结果），章节起点由版面里的标题类块推导；只有图片块的"这是什么图"补充说明来自 images 阶段写进 `source/*.md` 的 DOCVISION 注释（见下条）。md 正文只是这些块的**加工产物**，索引从不回头读它——所以改 md 不会改变索引，重新跑 images 阶段才会刷新 `doc_index/`。

**图片条目带上类型标记、描述与原文（来自 md 里的 DOCVISION 注释）**：MinerU 对没有 caption 的图片块只给一个文件名，于是索引构建时会把 images 阶段写进 `source/*.md` 的机器注释按**图片文件名**接回对应的图片块——`Marker`（`styled-text`/`vector`/`image`）、`Label`（注释里的描述，如 `mind-map diagram`）与 `Content`（STYLED-TEXT 的 `CONTENT:` 原文 / RASTER 的 `DESCRIBE:` 解释文本）。因此 `doc_search` 既可按文件名、也可按"图里写了什么"检索（例如按"知识导图""mind-map"找到那张思维导图），命中与 `list_source_pages` 页详情里都会显示 `[vector] mind-map diagram` 这样的短标签与内容摘要；没有注释的图片块照旧只有文件名（字段留空，不报错），旧版 `doc_index.json` 仍可读取。

样式会话、章节转换会话与全书修复/终审会话都带这套检索工具（终审/修复会话还会同时搜构建树）。

MinerU 产物缺失时自动降级（不注册工具，仅记录日志），不影响主流程。
