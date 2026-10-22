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

续跑 images 阶段时，`Already done: N | to process: M` 表示本轮仍需 process 的图；`classify` 只统计其中**尚无分类记录**的图，若全部已分类则显示 `[classify M/M] 100% … done`，随后 `process` 对本轮实际待处理任务独立计数（T112）。

**整组核验（T111/T118，仅档位1）**：classify 后、process 前做最终结构分组。本地完全一致内容/保守视觉相似度 + **小图标类的居中重合**给出**初始相似桶**（T130：与组内闸门同一判据，避免桶里 90 张、闸门能认 100 张这种错位；大插图/照片仍只走逐对相似度）（`candidates:manifest.md` 按桶列出，`candidates:similarity_advisory.json` 给 id/初始桶/是否像素一致/分类/内容摘要），并把桶**预填进 `groups.json`** 当作起点；桶只是参考，AI 可以拆桶，也可以跨桶合并。会话工具是 `read_file` / `grep` / `bash`（沙箱内可跑 Python，工作区挂载为 `/work`）/ `view_image` / `compare_images` / `motif_groups` / `submit`。**没有联系图**：判断必须逐张打开 `originals/NNNNNN.*`，未裁剪看过才算证据；要并排看细节用 `compare_images`（一次 2–4 张模型自己点名的图，进同一张合成图、等比不拉伸、格上标 id，回执附每张像素尺寸与长宽比——**并排的每一张都算「看过」**（都是完整原图等比缩放，模型确实看见了；写 `seen`、满足 submit 的逐张门禁，T132））。dedupe 收尾会把最终结论写进 `<proj>/source/dedupe/`：`groups.json`（组、代表图、成员、成员对代表图的重合度、类别、来源、逐字节副本 + 输入与规则指纹）、`report.md`（一页汇总，图是相对链接）与 `review.html`（**看图页**，T150/T154：每组一张卡片、代表图标 `★`、成员缩略图是 `thumbs/` 下的文件链接（不内嵌 base64）、分组默认收起可一键展开、点开在页内灯箱看大图，独立图另起一段——分组对不对直接看这一页，也能从会话预览页「整组核验」阶段行的「复查」入口打开），外加扫描中间量 `scan.json`（分桶 + 逐对明细，T153：同一批图续跑键命中就跳过桶扫描与逐对指标，判据未动）。每个分组必须给出 `key`（该组最核心的代表图），缺 key 时 `submit` 直接打回；**process 只重画 `key` 这一张，组内其余位置全部替换成它的成品**（各自原图的 md LINK 保留，共享一份 `source/figures/motif_*.tex`）。**逐字节相同的图只送一份**：候选池先按文件内容（`sha256`）收缩，每份不同内容留一张代表图进会话，其余副本不进会话（看一份等于看全部，字节相同不存在看错的可能），核验后**跟着代表图的结论走**——代表图被合并就跟随同一个 leader，代表图独立就跟随代表图本身，process 因此不会把同一张图重画 N 遍。会话分配由 `latex.dedupe_group_session_over`（默认 50：某桶超过 50 张就独占一个会话）与 `latex.dedupe_batch_images`（默认 150：较小的桶可以互相合并进同一会话，单会话不超过 150 张）控制，`0` 表示整池一个会话（旧行为）；这些桶会话之间互不依赖，按 `latex.dedupe_concurrency`（未写时 2，`0`/`1` = 串行）并发，最后的整理会话仍等它们全部结束才开始。没命中任何相似桶的图不塞进这些会话：正常书里绝大多数图彼此无关，它们先算「未分配」，最后只用一个整理会话核对——代码先用同一套本地相似度把未分配图与各已确认组的**关键图**配对，只有真配上的候选才进会话（互不相似就完全不开这个会话），所以整理会话只看关键图、不重看整组。组内合并按**居中重合**判据（T129）：成员按前景外接框按内容框居中贴进 S×S 软墨量画布（S = 两者内容框长边的较大者，24–512），比较时允许按 `latex.dedupe_align_shift`·S（默认 0.10）平移，逐级由粗到细搜到最细一级，判定分取最细一级的 Σmin/Σmax；与**组中心**（平均重合度最高的成员）≥ `latex.dedupe_align_min`（默认 0.60）的成员合并，其余保持独立（**例外** T152：离中心不够、但与组内某个**已经达标的成员**仍 ≥ 0.60 的随组带入——同一张图换分辨率/长宽比与中心的分数会明显偏低；只比一轮、不拿带入的成员当跳板、也不当代表图，所以组内最低分可能低于门槛，`report.md` 会加脚注），代表图优先用模型点名的 `key`（要求它自己达标且内容框面积 ≥ 合格成员最大面积的 90%），否则取合格成员里内容框最大的一张；大图/无前景的图退回逐对相似度（`MotifSimilarTo`）。**重合到 0.75 以上的矢量小图标不再过会话（T131）**：内容框 ≤ 20000 像素、彼此重合度 ≥ `latex.dedupe_icon_auto`（未写时为 0.75，`0` = 关闭）的图标先按连通片成组，取代表图后逐成员对代表图复核，达标的直接本地合并、不进核验会话；0.60–0.75 之间的以及非矢量图照旧逐张核验（对代表图不达标的成员退回池子）。代表图取「与组中心重合度 ≥ 0.95 的成员里内容框最大」的那张，因此既典型又覆盖全。安全口径仍是**宁可漏、不可误判**：低于门槛的成员即使被 AI 并进组也保持独立（上述带入是唯一的例外，且锚在"某个成员已达标"这个证据上）（多画一张的代价可以接受，丢掉真实差异不行）；代表图没产出成品时，跟随它的图会被解除跟随并在下次运行单独处理，绝不让图从书里消失。CLI 在扫描开始即显示 `[dedupe]`，核验时显示会话进度、失败数与耗时，核验结束后再打一行小结（核验会话/整理会话各几个、合并几组几张、其余几张独立重画）——会话进度行的 `N/M` 只是步数，不是结论。
平台侧能力用 `models.<条目>.context_limit` / `max_images` / `max_tokens` 声明（0=不限制），会话侧 `latex.sessions.dedupe.*` 声明该会话需求，实际取两者中较小的正值；`max_images` 超限时请求前自动把最旧图片替换成可重新 `view_image` 的文字提示。不能确认同图的条目仍独立作图；带蓝色题号的裁图不得与纯 z 标识合并。等价关系及原图摘要写回各自的 `source/progress_items/<md>/*.json`（`equivalent_to`、`dedupe_source_hash` 等），中断重跑可继续。已确认的代表图仅开启一次作图会话，其余引用保留原图 LINK 与原位置，生成一份 `source/figures/motif_*.tex` 原始代码片段，在 Markdown/章节 TeX 各处 `\input` 引用；图像发生变化或核验失败则不复用。核验会话转录在 `source/sessions/motif_dedupe_*.jsonl`，Web 会话预览「整组核验」阶段可查看。逐步的完整流程（含居中重合度的算法细节与实测数字）见 [dedupe 全流程](dedupe-algorithm.md)。档位2不启用此步骤。


1. **样式分析 AI**（`models.style`）：拥有独立虚拟工作区（`latex_project/<项目名>/work/style/`），用 `list_source_pages` 取原书页索引（源 PDF / 页范围 / 全局页号 / OCR 推导的章节起点），再用 `view_pdf {path:"source:<file>.pdf", page:N}` **按需渲染** MinerU 保留的原始扫描页（支持裁剪放大）、`view_image` 看提取图、`read_file`/`grep` 读解析文本，分析全书样式后用 `write_file`/`edit_file` 增量起草（`example.tex` 的图**直接用 md 里已有的 LaTeX**：`<!-- DOCVISION-VECTOR -->` 下面那个 ```latex 围栏就是这本书真正的图代码，复制它比重新画一张更省心智、也更贴合实际成品），再 `submit_style` **按工作区路径**提交 `book.cls` + 使用手册 + 案例（`{cls, manual, example, extra[], report}`：`extra` 可带上类需要的任何附属文件——helper `.sty`、TikZ 样式、字体表——会连同示例一起进试编译；`report` 写进 `style/REPORT.md`；示例自动试编译，失败回炉）。`submit_style` **只收工作区路径**，没有内联内容兼容（工具形态不该对模型保留历史包袱，且"路径还是内容"的启发式曾把一行 `## Vector figure style` 误判为文件名）；参数里带换行内容会被拒绝并提示先 `write_file` 再交路径。字体通过 `list_fonts` 查看；缺字体时在提交报告与手册里列出清单，由用户按文件名手动放进 `paths.fonts`（编译环境已注入 `TEXINPUTS`/`OSFONTDIR`，无需装系统字体）
2. **章节划分 AI**（`models.chapter`）：`grep`/`read_file`/`bash` + `buffer.md` 工作记忆缓冲区（分析结论增量写入、跨轮保留），按行号划分章节（结构化提交，全覆盖校验）
   **逐图校验（`latex.figure_check`，默认关闭）**：打开后每次 `submit` 都再用一个**能看图**的模型把"重画图 ↔ 原图"比一遍（两张图同时进会话，工具只有 `submit`）；不一致就把问题清单打回**同一个作图会话**继续修（上下文还在），最多 `max_rounds` 轮（默认 2），仍不合格记警告照常交付。校验会话转录落在 `work/sessions/figure_check_<图>.jsonl`。
3. **转换 AI** 并发逐章转 `.tex`：每章在**自己的临时工作区** `work/temp/conv_<章>/work` 里干活——里面已经铺好 cls/`.sty`/手册/`example.tex` 的**副本**（随便改随便试，真样式包不受影响）、插图（`images/`、`figures/`）、该编译的 wrapper 与**待提交的两个路径**（`<章>.tex` + `<章>/`）；探针、实验 `.tex`、PDF 都留在临时工作区里，**只有提交的那两个路径会被拷进 `work/chapters/`**（分片文件夹整棵替换，历史残留与探针不会混进书里）；别人的成品只能经只读通道 `project:converted/`（其他章节已提交的 `.tex`）与 `project:reports/`（其他章节的工作汇报）参考；首条消息给出**工作区地图**（`work:`/`project:`/`source:` 各是什么）与两处权威输入路径——章节文本 `project:chapters/<章>.md`（**从它转换**：它是解析好的纯文本，读起来最省；但它是 OCR 来的、**可能出错**——遇到读不通的字、公式、图表就以**原书为准**，用 `doc_search`/`list_source_pages`/`view_pdf source:` 翻原页核对；版面问题一律以原书为准）与 class 手册 `project:style/manual.md`（不再整段内联）；工具集含 `bash`（沙箱内、与其它工具同一套挂载点）与 `compile`（**编译包了 `\\documentclass` 的 wrapper**；`compile {path:"chapters/<章>/probe.tex"}` 可单独编译工作区里任何一个 `.tex` 做验证，探针文件不得留在产物里；章节 `compile` 也接受 `engine`/`passes`/`args` 现场指定引擎与额外参数，回执只报 `COMPILE OK` + 产物名 + 页数或错误日志）；章节提示词只写"怎么用"与转换规则，工具参数交给工具自己的 schema（system 9.3K→4.9K、首条消息 1.2K→0.17K，模型不用把同一件事读两遍）；矢量图已在 images 阶段以 latex 代码块内嵌进章节 md，转换时按手册的 `## Vector figure style` 二次加工（保留结构、几何与全部标签）；raster 原图走 includegraphics → 每章提交后由 **核对会话**（`latex.checker_model`，须支持看图）比对产物与原书：它跑成一个**只读会话**——视图里有章节 md、提交的 `.tex`、`parts/` 分片、**本章编译出的 `<章>.pdf`**、转换会话的 `report.md` / `notes.md`（当作"说法"核实，不是事实）以及原书 `source:` part PDF；工具 `read_file`/`grep`/`view_pdf`/`list_source_pages`/`doc_search`/`submit`，**轮次随层级加宽**（T204：最里层 `sessions.checker.max_tool_rounds` 默认 50，每往上一层 +`max_tool_rounds_per_level`（默认 20）→ 50/70/90/110；也可以改成按层翻倍 `max_tool_rounds_factor`），写不了任何东西；**每条问题必须指明位置**（md 段落或原书页 ↔ `.tex` 何处），看不见的差异不报——OCR 噪声、只在原页上的公式/表格/插图、浮动体位置都算正常；md 只是**OCR 参考、不是权威**——原页与 md 不一致时以原页为准，`.tex` 跟着原页走（T203）；重复上一轮的问题前要先回原页复核，自己看错了就撤回（T172：旧行为只给 md 与 tex 两个文本，看不到产物与原书页，于是把正常差异报成缺失，转换会话被逼着改坏本来正确的内容）；**硬性问题回同一个转换会话最多 `latex.checker_rounds` 轮**（上下文还在，最省 token），仍不过就**作废产物、换一个全新会话重来**：顶层默认 6 次、每递归一层 -1、**下限 3**（`checker_rounds` 与最大层数不匹配时起步告警），每次尝试一条独立转录（`convert_<章>.try2.jsonl`…）；checker 两次都拿不到结论**判不通过**（不再降级为通过），余额不足这类致命错误一次就停。**提交门禁**：`compile` 成功会在工作区 `.gate/compile.json` 记下「主 `.tex` + `parts/` 内容摘要 + 产物 sha256 + 页数」，`view_pdf` 渲染这份产物时记下页码；`submit` 要求摘要与当前文件一致（编译后又改过就得重编）、产物指纹一致、且至少看过 `min(2, 总页数)` 页——"没编译"或"编译了没看"都会被拒，并告知下一步该做什么；**还要和原书对过**（T168：`view_pdf` 打开 `source:<原书 part>.pdf` 的页记进 `.gate/source.json`，至少 2–4 页不同的原书页，产物越长要求越多——脚本机械转换最容易在"产物看着没问题、和原书一比就露馅"的地方偷懒），**且内容覆盖率达标**（`pdftotext` 抽出产物文本，与章节 md 的正文去代码块/公式/注释/图片后做 CJK 三字组比对，低于 85% **第一次打回**并列出缺得最多的几段原文，同一章再交仍不达标则放行（OCR 差异大的书可能永远到不了阈值；放行时缺口连同 `report.coverage_note` 的说明写进本报告的未解问题、向上合并）；实测正常完成的章节 0.97–0.995，把产物文本砍掉 1/4 会掉到 0.63–0.76；CJK 太少（纯公式）不判定、`pdftotext` 缺失时跳过）。覆盖率同时写进日志（`内容覆盖率 97.0%（md 正文 → PDF 文本）`），低于 92% 再提醒一次；**章节太大先拆**（T169：`latex.max_chapter_lines`，默认 3000 行，0 关闭）——首条消息直接把"这一章 N 行"告诉会话，超过上限又还没拆分过的节点 `submit` 会被打回，要求先 `request_chapter_split` 拆成 2–16 段（子会话各转各的、各自过 checker，父会话只负责整合与核对），已到最深层不再强制
**P34 递归细分**：转换会话遇到章节过大或无法可靠处理时，可用 `request_chapter_split` 按语义边界申请完整、连续、无重叠的 Markdown 行段（每次 2–16 段，最多三层细分）。子章节会话在对应父工作区的 `_subdivision/` 子树内并行运行，各阶段的**活动 API 会话**并发上限互相独立（T179）：转换会话共用 `latex.convert_concurrency`（未写沿用 `latex.concurrency`；这个数同时是**批次宽度**，即同时开这么多章转换）、核对会话共用 `latex.checker_concurrency`，其它阶段用 `latex.concurrency`；两者不放在一起算，核对不会因为转换排队而等着。要限制真正打到同一个模型上的请求数用 `models.<名>.max_concurrency`（会话层按实际下发的模型名封顶，转换与核对落到同一模型时共用这个额度）。父转换在等待子树期间释放槽位，只有执行转换或 checker 请求时才占槽，避免父节点等待占满并发名额。子节点尚未取得槽时在会话预览显示“等待并发槽”，父节点等子节点时显示“等待子会话”。每个子章节必须通过明确提交 `pass` 的 checker。父会话先读 `_subdivision/children-summary.md`（每个子章节自己报告的问题原文 + "自报 issues" 计数 + 已继承的问题清单，**必读但不要求修复**：能顺手修的写 `report.resolved` 划掉，没修的由机制原样向上——见 `docs/issue.md` T166）、再按源顺序读取 manifest 与子产物，整合后在隔离暂存目录提交核对；**提交产物必须自包含**（`\input`/`\includegraphics` 不许指向 `_subdivision/…` 这类临时路径，`submit` 当场打回）；父级 checker 通过才晋级到上一级章节结果，任一子级失败或缺少 checker 结论都会阻止晋级。细分节点保留输入、按完整章节 ID 命名的转换/checker 转录、工作报告及各层工作区，供失败诊断与续跑。checker transcript 只追加保存；重跑时使用当前 `latex.checker_model` 与模型配置，恢复同一 checker transcript 的历史对话，未完成核对不会误变成 pass。

4. 汇总为多文件 `.tex` 项目 → 编译全书 PDF（失败进入**修复会话**：读文件/改文件/重编译 + 字体工具核对样式）→ 成功后进入**终审会话**逐页核对成品 PDF 并整理（`latex.compile.final_review`，默认开启；转录在 `work/sessions/final_review.jsonl`，WebUI 可见、计成本）→ 生成单文件 `standalone.tex`；编译前幂等注入 hyperref + 逐章 `\pdfbookmark` 保底书签（T49，成品 PDF 有书签/跳转，书签文字与层级取划分 md 首个标题行（# 数=书的真实层级）；main.tex 已带书签时视为终审会话按书结构接管、不再注入基线）并做**资源盘点**（T44：cls/figures/images/章节就位计数 + 逐条核对 \includegraphics 引用，缺失告警不中止）；交付把整棵 build 树复制到 `out/`，`book.pdf` 只是 `main.pdf` 的别名，并另复制到**项目根外侧** `<latex_project>/<项目名>.pdf`（T42）。终审成功后它的树内修改（章节修订 + 新增顶层 .tex + main.tex 钩子）**回写** `work/chapters/` 与 `work/final_review/` 并记内容指纹（`state.sha256`）；续跑时指纹一致（convert 后没动过）直接沿用、**跳过终审会话**，不一致才重审（旧转录归档 `final_review_prev.jsonl`）——build/ 每次重建，不回写的话每次续跑都要把几十轮终审从头再烧。修复会话转录在 `work/sessions/book_fix.jsonl`
5. **风格修复大循环（T43）**：任何章节的工作汇报报告 cls/手册问题（不再要求多数派）即进入：整合会话把各章汇报整合成**不重复的问题清单**（固定格式 `submit_problems`）→ 风格修复会话（**复用原样式会话上下文**，信息不丢失；缓存失效只是成本）修 cls/手册并编译 example 验证 → 影响评估会话按问题**划分章节块**（在工作区 `work:blocks.json` 里 write/edit 划分、可反复修改后 `submit_blocks` 无参提交，也可内联传参；样式问题常是全局性的，评估会话可 grep 全部章节 .tex 圈定真实影响面；T70 起与整合会话同一套工作区提交通路）→ 每块一个并行**分块修复会话**（临时工作区里对块内章节做定位修改、wrapper 编译验证，逐章汇报是否解决，绝不重做）；仍有问题进入下一轮 style-fN+1，上限 `latex.style_fix.max_rounds`（默认 3，负数=关闭）；块会话失败的章节退回单章增量修复，再失败删产物重转换。各会话可用 `latex.style_fix.*_model` 独立配模型（T70，模板默认 extends 自原角色条目 = 行为不变）。各会话转录：`work/sessions/stylefix_integrate.jsonl` / `stylefix_style_r<N>.jsonl` / `stylefix_assess_r<N>.jsonl` / `stylefix-blk-r<N>-<i>.jsonl`。整合会话有自己的虚拟工作区 `work/stylefix_integrate/`（各章汇报按 `reports/<章>.md` 落盘，配 read/grep/write/edit，T64）。T64 重构起问题清单是一块共享存储 `work/style_fix/problems.json` + `problems` 工具——整合会话独占写权限（逐条 add/update/delete，无打勾），下游会话（样式/评估/分块）只能 list/get/search 与 check；清单不再塞进任何会话的提示词。提交门由代码核对存储：样式修复会话要求全部问题打勾、分块会话要求本块问题打勾，缺一个就 REJECTED（T66 的 `checked_problem_ids` 与 T67 的 `list.md` 随之退役）；每个会话另有 `goal` 工具（`plan.md` 计划清单，增删改勾，压缩后随时找回进度），提交前计划必须清空；分块会话可 `view_pdf` 看编译产物（T68）

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
| `work/views/checker_<章>/` | checker 会话的**只读视图**：`<章>.md`、`<章>.tex`、`parts/`、编译产物 `<章>.pdf`、转换会话的 `report.md` 与 `notes.md`（后两者存在时才有） |
| `work/pdfview/` | 原书 PDF 最小视图（`source` 挂载点，`<part>.pdf` 软链到 MinerU 的 `*_origin.pdf`） |
| `work/reports/` | 每章转换会话的工作汇报 `<章>.md`（转换期间，只读通道 `project:reports/`） |
| `work/sessions/` | 会话转录（`chapters.jsonl`、`convert_<章>.jsonl`、`checker_<章>.jsonl`、`style_fix_<章>.jsonl`、`figure_check_<图>.jsonl`）；重试产生 `<名>.try2.jsonl`/`.try3.jsonl`…，预览页把它们聚成「历史 N」；矢量图另有 `sessions/vector_<图>.work` 持久工作区 |
| `work/temp/` | 临时工作区（拆章沙箱、样式/反馈 scratch、每章转换工作区 `conv_<章>/work`），默认用完即删 |
| `build/` | 全书的构建树（每次 clean 重建：cls/手册/案例 + chapters + figures + main.tex），同时是修复会话与终审会话的工作区 |
| `out/` | 交付产物：整棵 build 树（跳过 .aux/.log/.toc/.synctex 等中间文件），`book.pdf` 是 `main.pdf` 的别名，另有 `standalone.tex` |
| `doc_index/doc_index.json` | 只读块索引（`doc_search` 的检索库，style 阶段之前构建；图片条目带 md 里 DOCVISION 注释的类型/描述/原文：`marker`（styled-text/vector/image）、`label`（描述）、`content`（styled-text 的印刷原文 / raster 的解释 / **矢量图的 LaTeX 本体**）；链接（`img` 与档位1 md 里的 `LINK:`）**一律指向原图**，`text` 是厂商图注，**厂商没给图注时用我们自己的内容兜底**（styled-text 的印刷原文 / raster 的解释 / vector 的图标签，不用 LaTeX 本体）） |
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
docvision latex --only chapter_002             # 单章重跑（开新尝试，旧尝试留档；自动只跑 convert）
#   重新核对没有命令行等价物：它的对象是已提交产物，在预览页对应核对行上点 ↻
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

### 单章重试（手动重跑）

某一章转得不对（缺内容、公式错、图丢了），不必重跑整本书：

```bash
docvision latex --only chapter_002                      # 只重跑这一章
docvision latex --only chapter_002,chapter_005           # 一次重跑几章
docvision latex --only chapter_004_child_01 --project 概率论-2026
```

`--only` 不指定 `--step` 时自动只跑 `convert`（重试一章只可能发生在转换阶段）。
语义只有一条：**重试 = 这一章开一个新尝试**。

- 旧的尝试整棵挪进 `<项目>/work/attempts/<章>/try<N>/`（含已提交的 `.tex`、分片、`work/`、`result/`、`integrated-stage/`、旧 `report.md`，问题清单写 `checker-issues.md`），**不删**；
- 新会话写 `convert_<章>.try<N+1>.jsonl`，转录里记一条 `t=meta kind=manual_retry`（原因 + 尝试号），预览页的「历史 N」里两条都在、能来回切；
- 提交过的 `<章>.tex` 会被移走，否则转换阶段会按「已有产物，跳过」直接略过这一章。

手动干预有四种，页面上按角色给按钮：

- **转换重试**（转换行 ↻ / 详情栏「重试这一章」，命令行 `--only`）：重转这一章，开一个新尝试；
- **继续跑**（详情栏「继续跑（不新建尝试）」）：接着这一章**最近一次尝试**干——不新建尝试、不归档、不动已提交的产物，**不发任何 user 内容**（历史原样续行，和中断续跑同一条规矩），只是让那条会话真的再跑一轮——哪怕它上次已 submit，也不会被"收账即结束"跳过。"会话跑歪了/报错了、我先手动改了工作区里的东西，想让它从这儿收尾"用它；
- **从某条记录之后继续（作图会话）**（「轨迹」表每行的 ⤓，对「矢量图」同样可用）：历史截到这一条、完整旧转录留档成 `.tryN.jsonl`（回滚不丢对话）、成品清掉，然后只重跑这一张——不发任何内容（历史原样续行）；类别与 dedupe 分组不动。注意作图这一侧只回滚**对话**，作图 scratch 工作区还是最后一次的样子（成品按图清，不按轮次快照文件树）；
- **重画这张图**（「矢量图」的作图会话详情栏）：只重做这一张——旧作图会话留档成 `.tryN.jsonl`（仍在「矢量图」里看得见），清掉这张图的成品重跑一遍；**类别、dedupe 分组与代表图都不动**，重画只换画法。是某张代表图的副本（dedupe 跟随）会被拒绝，让你去点代表图那张。画完下游（样式/划分/转换/组装）需要重跑才看得到新图；
- **从某条记录之后继续**（轨迹表每行的 ⤓，转换会话与核对会话都行）：把这一章的转录截到这条消息、后面的轮次丢掉，抄成**一条新尝试**接着干；源转录一字不动（原记录仍在「历史」里）。转换会话会**连工作区一起退回那一刻**（P55）：每章的工作区在 `<项目>/work/rounds/<章>/repo.git`（工作区之外的裸仓库，模型看不见）里每落一条消息提交一次，切点按转录行号找回那一张快照，`reset --hard` + `clean -fdx`——改过的文件恢复、后来新建的文件与编译产物清掉，已提交的产物/分片/报告挪进 `work/attempts/<章>/try<源尝试>/`。**兼容老会话**（只有一种模式：能退就退、退不了就只退对话）：P55 之前产生的会话没有快照、宿主没有 `git`、递归子章节不拍快照——这时降级成**只回退对话**，工作区与已提交产物都保持最后一次的样子（与旧版行为一致，也不会为一个老会话建空仓库）；排队后页面的提示会直接写明"⚠ 这条会话没有工作区快照，只会回退对话，工作区里后来改动的内容会保留"，要连文件一起干净重来请用「重试这一章」。核对会话的 ⤓ 只换核对会话：产物与只读视图一个字不动，旧结论留一份 `.tex.checker.try<N>`；
- **删除这条记录**（详情栏）：删掉 `work/sessions/` 下的那一条转录；带 `.tryN` 的连 `work/attempts/<章>/try<N>/` 一起删。90 秒内有更新或存在 `.partial`（可能还在写）会被拒绝；
- **重新核对**（核对行 ↻ / 详情栏「重新核对这一章」）：**产物一个字不动**，只换核对会话与只读视图重新核对一遍已提交的 `.tex` 与编译产物——旧结论挪成 `<章>.tex.checker.try<N>`，新会话写 `checker_<章>.try<N+1>.jsonl`。它只给结论，**不会**自动打回转换会话；核出问题就再重试转换那一章。没有已提交产物的章节不给这个按钮（先重试转换）；递归子章节的产物在父节点的临时树里（`work/temp/conv_<根章>/work/<父章>/children/<子>.tex`、或自己那一支的 `node/result|integrated-stage/<子>.tex`），整合前也能重核（T201）。

粒度是**顶层章节**：细分出来的子章节（`chapter_003_child_01`）不单独重试——它由父章节的细分树产出，重跑它得连同父节点的整合一起走，直接重试父章节即可（`--only` 点名子章节会明确报错）。

单章重试**只做转换这一层**：不进样式反馈回路（那会拿全书报告改 cls、再分块重转整本书，是"整书重跑"该做的决定），也不会自动重跑 `assemble`——想在整书 PDF 里看新章，再跑一次 `docvision latex --step assemble`。

这些干预都可以从预览页点出来（章节行悬浮出的 ↻、右侧详情栏的按钮）。预览页是**只读观察者**，所以按钮不执行命令，只往 `<项目>/work/retry/` 写一个请求文件（删除记录走 `/api/drop`，只删转录）：

- 正在跑的 `docvision latex` 里有一条 **watcher** 跑在整个生命周期里（2 秒一跳）：样式/划分/转换/组装**任何阶段**都收，真正排进各自的并发槽位（转换与核对是两套池子）；同一章正在跑就**不消费**、留在队列里等它收尾，排上之后章节行显示「待重试」；
- 前置条件没到（样式还没就绪、这一章还没有产物/转录）也留队，只在日志里提示一次；
- 进程没在跑时请求留在磁盘上，下次启动先处理——按下那一下不会白点；
- 处理完请求文件改名成 `.done.json` 留档；同一章排队多条也只重跑一次；
- 想完全关掉这些写入口，用 `docvision sessions --no-retry`。

需要本地 LaTeX 工具链：TeX 发行版（xelatex）+ poppler-utils（pdftoppm）。全书多文件构建优先用 `latexmk`（缺失时退回两遍编译）；矢量图转 SVG 按 `dvisvgm → pdftocairo → mutool → inkscape` 依次尝试，任一可用即可；会话 bash 沙箱用 bubblewrap（缺失时自动回退普通 shell 并记警告）。`docvision init` 会自动检查 xelatex / pdftoppm / mmdc 并给出安装提示。

## 清理与重跑（辅助脚本）

仓库根目录有两个 bash 脚本，都默认**先整目录备份**（`cp -a` 到同级的 `<项目名>_记录_<时间戳>/`）、默认先打印计划、要 `-y` 确认（`-n` 只预览）；检测到 `docvision latex` 正在运行会拒绝执行（`-f` 强来）：

- `clean_project.sh <项目目录>` —— 清回**只剩分类结果**（连样式包）：删 dedupe/process 的字段与下游产物、`build/`、`chapters/`、`out/`、`source/figures|tikz`、`source/dedupe`、`source/sessions`，保留 `source/progress_items` 的 classify 结果（逐图请求，删了要整本重烧）。`--with-style` 连样式包一起清，`--keep-sessions` 保留 `source/sessions`。
- `clean_convert.sh <项目目录>` —— 清回**章节划分刚做完、还没逐章转换**：删 `work/chapters/`（转换成品）、`work/reports/`（每章报告）、`work/temp/`（章节工作区、递归子树、`.gate`、issues store）、`work/sessions/{convert_*,checker_*}.jsonl`，以及转换之后的一切——`build/`、`out/`、交付副本 `<项目名>.pdf`、`work/style_fix|stylefix_assess|stylefix_integrate|final_review|views|pdfview`（**不删 `work/media/`**：那是保留下来的 `work/style_session.jsonl` 的图片附件目录，删了样式会话就只剩断图，T199）、`work/sessions/{book_fix,final_review,style_fix_*}.jsonl`；把 `progress.json` 的 `convert`/`assemble` 归空（`style`/`images`/`chapters` 保持 done），所以重跑时分类/样式/划分全部跳过、**直接从逐章转换开始**（每章重新开会话，旧转录也清掉，不会拿"已有产物，跳过"糊过去）。保留 `chapters/*.md`、`work/sessions/chapters.jsonl`、`style/`、`work/style/`、`work/style_session.jsonl`、`source/`（含分类结果与 dedupe 结论）、`doc_index/`。`--keep-convert` 是反口径：保留转换成果（含转录与 `work/temp/`），只清它之后的阶段，重跑从风格修复大循环接上；此时脚本会点出 `work/chapters` 里指向 `_subdivision/…` 的断链（旧二进制产物，汇编会失败）。

## 风格分析的数据来源（档位1）

样式分析 AI 直接查看 **MinerU 保留的原始扫描页面**（`mineru_output/<主题>_part*/*_origin.pdf` 以软链形式暴露为只读 `source` 挂载点下的 `work/pdfview/<part>.pdf`，模型看不到整个 `mineru_output` 树；用 `list_source_pages` 取页索引与章节起点、`view_pdf {path:"source:<file>.pdf", page:N}` 按需 pdftoppm 渲染整页 PNG），支持按页浏览、按百分比裁剪、放大查看细节——这才是真实排版（标题格式、页眉页脚、字号行距）。原始 PDF 缺失时退化为仅用提取图片 + 解析 md 分析，并输出日志提示。

## 原始文档检索（档位1 会话，只读）

构建时把 MinerU 中间产物（`mineru_output/<主题>_part*/` 的 content_list/layout/origin.pdf）加工为只读块索引（`<项目工作区>/doc_index/doc_index.json`，在 style 阶段之前构建），样式/转换/修复/终审会话都可用：

- `doc_search {query}`：按关键词 / 图片文件名 / 页码检索块索引（文本片段、图表标题、公式 LaTeX、bbox），返回**全局页号 pN** 与 bbox，并直接给出精确路径 `source page: view_pdf {path:"source:<file>.pdf", page:N}`（part 级定位，不依赖全局页号累加）；
- `list_source_pages {page?}`：无参数列出「源 PDF → 页范围 → 全局页号」表 + 从 OCR 版面推导的章节起点；带 `{page:N}` 时列出该全局页的正文片段与该页抽出的图片文件名（随后可直接 `view_image`）；
- `view_pdf {path:"source:<file>.pdf", page, left/top/right/bottom, zoom}`：渲染原书某页（或按百分比裁剪/放大）查看真实排版，回执附带该页与裁剪区域的 **mm 尺寸**——与"看自己编译出的 PDF"是**同一个工具**（`zoom_width` 是 `zoom` 的别名，渲染时按目标宽度从 PDF 重渲染，真放大）。

**`list_source_pages` / `doc_search` 的内容来自 OCR 索引，不是我们生成的 md**：条目、页码、bbox、文本与表格/公式内容都取自 MinerU 的 `*_content_list.json`（版面识别结果），章节起点由版面里的标题类块推导；只有图片块的"这是什么图"补充说明来自 images 阶段写进 `source/*.md` 的 DOCVISION 注释（见下条）。md 正文只是这些块的**加工产物**，索引从不回头读它——所以改 md 不会改变索引，重新跑 images 阶段才会刷新 `doc_index/`。

**图片条目带上类型标记、描述与原文（来自 md 里的 DOCVISION 注释）**：MinerU 对没有 caption 的图片块只给一个文件名，于是索引构建时会把 images 阶段写进 `source/*.md` 的机器注释按**图片文件名**接回对应的图片块——`Marker`（`styled-text`/`vector`/`image`）、`Label`（注释里的描述，如 `mind-map diagram`）与 `Content`（STYLED-TEXT 的 `CONTENT:` 原文 / RASTER 的 `DESCRIBE:` 解释文本）。因此 `doc_search` 既可按文件名、也可按"图里写了什么"检索（例如按"知识导图""mind-map"找到那张思维导图），命中与 `list_source_pages` 页详情里都会显示 `[vector] mind-map diagram` 这样的短标签与内容摘要；厂商没给图注（`image_caption`/`image_footnote` 都空）的图块，`text` 会用**我们**写在这张图上的内容兜底（styled-text 的印刷原文 / raster 的解释文本 / vector 的图标签）——这样按"图里写了什么"检索、以及 `list_source_pages` 的页详情都不会看到空条目；厂商本来给了图注的，`text` 保持厂商原文、我们的解释仍在 `content` 里，两边都可检索且来源不混。链接（`img`/`LINK`）三种类型**都指向原图**（`images/<主题>/<sha>.jpg`），从不指向生成的 `figures/*.svg`——那只是**档位2 正文**里的嵌入方式。没有注释的图片块照旧只有文件名（字段留空，不报错），旧版 `doc_index.json` 仍可读取。

样式会话、章节转换会话与全书修复/终审会话都带这套检索工具（终审/修复会话还会同时搜构建树）。

MinerU 产物缺失时自动降级（不注册工具，仅记录日志），不影响主流程。
