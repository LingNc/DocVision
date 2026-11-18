# skills 线 plan 台账（独立编号 SK-P<N>）


> 与旧线 `docs/plan.md` 的 P 编号完全隔离。SK-P1–SK-P26 由旧线 P81–P106 迁移（2026-11-18，防新旧线混淆）；批次历史档案仍在旧线 docs/agent-batch-history.md 第三十七至第六十二批（只增日志不迁移）。


[ ][ ] SK-P1. skills 线：LLM 会话外迁到宿主 agent 的新架构——skills 内核只做确定性执行+验收（任务队列/租约/验收器/项目注册表/书级档案），用户除 MinerU 外零 API 配置；愿景"放下 PDF 跑到底、中断无损续、换 agent 可续"，验收标准 A1–A10。放 `skills/` 独立演化，不侵入旧线。
  改：新增 `skills/`（README + docs/01架构 02任务协议 03接口 04迁移台账 + `kernel/` Go module + `dvagent` CLI）；本批同步 plan/CHANGELOG/batch-history/AGENTS.md 指针。
  验证：`go test ./internal/...` 5 例绿（幂等入队/租约过期回收/确定性派发/打回回队/依赖解锁）；dvagent 冒烟全链路绿（register→activate→claim→submit 打回→补产物通过→解锁核对→note/profile/status）。


[ ][ ] SK-P2. skills 内核编排层落地：章节拆分验证与扇出（2–16 段、完整、连续、无重叠，父任务 blocked 于子任务）、issues 存储 + 回炉任务生成（原任务挂起等回炉，回炉回原沙箱）、pause/resume/abort 生命周期命令、claim 生命周期闸门。
  改：`skills/kernel` 增 `queue/split.go`（ValidateSplit/FanOutSplit）、`queue/issues.go`（IssueStore/SpawnRework）、`project.Pause`、`dvagent split/issue-add/issue-resolve/pause/resume/abort`；`Task.Reclaimable` 加"有依赖永不可领"判据；修两个冒烟真 bug（submit 打回不解锁依赖、章节行数末尾换行 off-by-one）。
  验证：`go test ./internal/...` 8 例绿（拆分判据五反例 + 扇出解锁 + 回炉挂起）；dvagent 冒烟：坏拆分拒绝/好拆分扇出/子过父解锁/issue-add 回炉/pause 拒派/resume/abort 全链路。


[ ][ ] SK-P3. T3 重型判据落地：覆盖率算法逐字移植旧线（T168：pdftotext+CJK 三字组、阈值 0.85、<200 组跳过、首打回再放行记缺口）、编译检查（pdflatex 双遍+sha256 快照入 verdict）、验收器默认装配重型 Hook（缺工具跳过记 note 不卡死）。
  改：`skills/kernel` 增 `texcheck/`（coverage.go 移植+compile.go）；`verify.NewChecker` 默认装配+覆盖率二判制+out/ 哈希快照；`Verdict` 加 `Notes`；dvagent submit 回传 notes。
  验证：`go test ./internal/...` 14 例绿（覆盖率 5 例：剥离/自对照/删1-4掉阈值+列缺口/<200组跳过/首打回再放行）；pdftotext 真集成冒烟：75% 覆盖率首次打回列缺口段落、二次放行记 coverage_note、哈希快照就位。


[ ][ ] SK-P4. T2 作图判据落地：软墨迹对齐（compareFigure）逐字移植旧线 figure_compare.go——多分辨率渐进对齐、质心起点、尺寸按物理 mm 在归一化前量取（scale_dev 抓"看着对上了其实窄了一截"）、RGB/灰度双通道、证据叠加图（白=共有/品红=原图独有/绿=候选独有）可落盘；仅剥会话工具壳。
  改：`skills/kernel` 增 `figcheck/`（compare.go 移植 566 行 + compare_test.go 5 例）；go.mod 引入 golang.org/x/image（CatmullRom 缩放）。
  验证：`go test ./internal/...` 19 例绿（同图 ≥0.9 相似、mm 口径抓 50% 宽度差、无 mm 退化宽高比判据、空图拒绝）；修掉移植笔误一处（overlay→Overlay 调用点）。


[ ][ ] SK-P5. T1/T4 verdict 模式验收落地：classify 三态校验（styled_text 必附 extracted、vector 必附 reason、class 不合法/缺失一律打回——旧线"失败不降级"判据入代码）；check 校验（result 三选一、issues 每条必带位置指针、no_verdict 打回但由 attempts 上限兜底）。
  改：`skills/kernel` 增 `verify/schema.go`（ClassifyVerdict/CheckVerdict 结构 + 校验）+ `schema_test.go` 8 例；`Run` 第 6 步接入。
  验证：`go test ./internal/...` 27 例绿（classify 5 例 + check 4 例覆盖全部正反路径）。


[ ][ ] SK-P6. dedupe 居中重合判据核心落地：软墨量内容框（不二值化不缩放）、S 自适应按对计算、逐级搜索 32→最细（判定分取最细级）、±10% 平移预算/细层 ±2 格不扩窗，从旧线 motif_canon.go+motif_align.go 逐字移植；附预筛（墨量/长宽比）、代价记账、AlignDetail 过程量、CarryOver 带入不接力（T151）。
  改：`skills/kernel` 增 `dedupe/`（canon.go 230 行 + align.go 230 行 + dedupe_test.go 8 例）；Runner/配置耦合改为包级变量与参数。
  验证：`go test ./internal/...` 36 例绿（自比≈1、方块vs竖条低于 0.60 闸门、预筛拒高宽比差、空白/小内容框退回、画布抬下限、平移换算、级数尾级规则、带入不接力）。


[ ][ ] SK-P7. dedupe 候选桶检测与成组校验落地：sha256 精确收缩（像素指纹桶）+ 形状/颜色 20×20 网格特征 + 色度特征（白占比 0.82/单连通片/三通道主导色）+ 小图标居中重合并桶（T130 同判据，像素相同只比一份、预筛 WorthScore、并行打分串行 union）；SimilarTo 闸门（检测器超集：严格特征失败仍可凭色度合并，但内容无关永不允许）；groups.json 结构校验（normalize/validate/shape，交互 Tool 壳由"agent 直编 JSON + kernel submit 校验"替代）。
  改：`skills/kernel` 增 `dedupe/candidates.go`（500+ 行移植）+ `dedupe/groups.go`（验证逻辑）+ `pipeline_test.go` 5 例。
  验证：`go test ./internal/...` 40 例绿（精确组、平移副本并桶、内容不同拒并、闸门正反、成组校验正反）。


[ ][ ] SK-P8. dedupe 代表图与组收口纯函数落地：ContentLeaders（内容哈希→代表+字节副本）、TypicalMembers/PickRepresentative（典型 0.95 + 内容框最大者当代表，无达标回退中心）、FoldGroups（同桶组折叠：中心=与其余代表平均重合最高，成员原样收编不重筛 T149，跨桶不折）、AutoIconGroups（0.75 无疑问自动定组 T131）、AttachLeftovers（T135：只认已确认 key 作锚、一次挂一组、跳过已确认）。
  改：`skills/kernel` 增 `dedupe/represent.go`（5 纯函数）+ `represent_test.go` 5 例。
  验证：`go test ./internal/...` 45 例绿；修掉移植笔误一处（ContentLeaders 簿记）。


[ ][ ] SK-P9. doc_index 检索索引落地：MinerU content_list → 只读索引（块条目含 bbox/全局页号/图路径）+ DOCVISION 注释回填（窗口 20 行状态机：styled-text 印刷原文 / vector 的 ```latex 代码块进 content / image 描述；无厂商图注时 Text 用我们自己的内容兜底，vector 用标签不用整段代码）+ AND 检索（term 前缀 2.0/包含 1.5/覆盖率打分，字段过滤 text/content/marker/label，ties 按 Seq 文档序）+ EntryOnPage/CropHint（regex 命中文本块后，label 匹配优先，页内唯一图兜底）+ 落盘/重载。
  改：`skills/kernel` 增 `docindex/`（docindex.go 约 700 行移植 + docindex_test.go 5 例）；pdfcpu 页数与 findOriginPDFs 两处外部耦合改为函数注入（PageCounter/OriginPDFsFunc，nil 时跳过）。
  验证：`go test ./internal/...` 50 例绿（构建/检索排序/字段过滤/AND 语义/CropHint 唯一图兜底/marker 回填含跨行与代码块/落盘重载一致性）。


[ ][ ] SK-P10. assemble 纯函数落地第一刀：standalone.tex 递归内联（\input@path{{a/}{b/}} 声明收集进搜索路径、层数保险 8、withinRoot 越界拒绝、onStack 循环防重入）+ PDF 书签注入（T49：hyperref 幂等注入、逐章 \pdfbookmark[层]{标题}{bk:<base>}、AI 已有 \pdfbookmark 只补 hyperref、标题取划分 md 首个标题 # 数-1 clamp 0..3、escapeBookmark 转义）。
  改：`skills/kernel` 增 `assemble/`（standalone.go + bookmarks.go + assemble_test.go 8 例）；deliverBook/copyDir 等 I/O 管道留待装配批次。
  验证：`go test ./internal/...` 58 例绿（递归内联/声明路径解析/越界拒/书签注入幂等/AI 接手不重复/层级计算/转义）。


[ ][ ] SK-P11. checker 无结论判据落地：IsNoVerdict（T222 六类文案前缀：始终没给结论/API failure/会话退化/复盘无法挽救/只读视图无法准备等）+ MaxNoVerdictRetries=6（T233：每次换全新核对尝试、转换不重跑——重转救不了坏核对）+ RecheckPending（产物在但上次核对未判定 → 重跑先重核，绝不静默采纳没被判定过的产物）+ `.checker` 结论文件约定。
  改：`skills/kernel` 增 `checkspec/`（checkspec.go 60 行 + checkspec_test.go 2 例）；checkerAnchorName 不进新架构（新形态锚点=队列任务 id，语义已由 verdict 承担）。
  验证：`go test ./internal/...` 60 例绿（六前缀正反、无文件/有判定/无结论文案/空结论四态）。


[ ][ ] SK-P12. assemble 资源盘点 + input@path 注入落地：PreflightAssemble（旧线 preflightAssemble 改为返回结构化报告——cls 就位/章节计数/figures、images 计数/\includegraphics 逐条解析三候选路径 buildDir·figures·章节目录，缺失逐条列出不中止 T33）+ EnsureInputPath（T324：\input@path{{chapters/}} 幂等注入 main.tex 首行后——子章节按 chapters/ 内相对路径互引，缺它全书只能靠修复会话逐个改路径、每次 assemble 重烧）。
  改：`skills/kernel` 增 `assemble/inventory.go` + assemble_test.go 补 2 例。
  验证：`go test ./internal/...` 62 例绿（盘点计数与缺失引用/input@path 注入幂等）。


[ ][ ] SK-P13. 接线第一刀——classify 任务生成器：gen.GenClassify（扫描图目录→幂等铺 T1 任务，验收 spec 自描述，按文件名排序入队保证确定性）+ dvagent gen-classify 命令；**修真 bug**：queue.Claim 派发排序同时间戳（批量入队）时退化为 map 迭代序——非确定，违反"同样的磁盘状态必然产生同样的派发序列"核心设计不变量，补 id 字典序 tiebreak。
  改：`skills/kernel` 增 `gen/`（tasks.go + tasks_test.go 2 例）；`queue.Claim` 加确定性 tiebreak。
  验证：`go test ./internal/...` 64 例绿（生成器新增/幂等/缺目录零新增 + 派发顺序断言）。


[ ][ ] SK-P14. 接线第二刀——convert 任务生成器：gen.GenConvert（chapters/*.md → 每章一个 convert 任务：章节 md 拷进沙箱 inputs/chapter.md 作覆盖率对照源，验收 spec=主 .tex + require_compile + min_coverage，与 verify.NewChecker 默认 CoverageSource 咬合）+ dvagent gen-convert 命令。
  改：`skills/kernel` 增 `gen/convert.go` + convert_test.go 2 例。
  验证：`go test ./internal/...` 66 例绿（新增/幂等/缺目录/spec 断言/沙箱输入就位）。


[ ][ ] SK-P15. 端到端冒烟（A1 前置）：classify 全链（图目录→任务→claim→agent 交 verdict.json→验收：合法 styled_text done、空 class 打回（不降级）、重载队列冷启动续接状态原样、打回即释放租约可立即重领）+ convert 全链（章节铺队→交 .tex：临时路径引用打回（自包含）、好产物通过且带哈希快照、本机缺 pdflatex/无 PDF 时编译与覆盖率跳过记 note 不卡死——T33/缺工具哲学全链生效）。
  改：`skills/kernel` 增 `gen/e2e_test.go` 2 例。
  验证：`go test ./internal/...` 68 例绿。


[ ][ ] SK-P16. check 任务生成器：gen.GenCheck 枚举队列中现存（未 failed）的 convert 任务，每个铺一个 chk_<章名> 核对任务——blocked 于对应 convert（依赖边进生成器：T4 延迟核对模式的结构表达，转换 pass 后 Unblock 自动解锁核对）+ 只读白名单（convert 沙箱/style/reports）+ check verdict 验收 spec；failed convert 不派核对（没产物可核）。
  改：`skills/kernel` 增 `gen/check.go` + check_test.go 2 例；dvagent 增 `gen-check`。
  验证：`go test ./internal/...` 71 例绿（blocked 不可领/conv 全过解锁/幂等/failed convert 不派）。


[ ][ ] SK-P17. draw 任务生成器：gen.GenDraw 读回队列里 done 的 classify 任务 verdict.json，class=vector 的图铺 drw_<名> 任务（图从 classify 沙箱 inputs 拷进 draw 沙箱 inputs——记忆泡泡自包含；白名单 style/figures/；验收=figure.tex 编译通过，T2 数值判据接线留待 verify figure 批）；styled_text/raster 不派；未验收不驱动；verdict 缺失/损坏计 NoVerdict 异常。
  改：`skills/kernel` 增 `gen/draw.go` + draw_test.go 2 例；dvagent 增 `gen-draw`。
  验证：`go test ./internal/...` 75 例绿（三态分流/自包含拷贝/幂等/未验收不驱动/异常计数）。


[ ][ ] SK-P18. T2 作图数值判据接线（验收闭环最后一块判据）：verify.Run 第 7 步——acceptance.figure_ref 触发：pdftoppm 150dpi 渲染 out/figure.pdf → figcheck.CompareImagesMM（原图 vs 渲染，pdfinfo 取候选画布 pt→mm）→ 重合度 < min_overlap 或 mm 尺寸偏差 > max_scale_dev（T312）打回并附对齐成绩/尺寸句证据；accept_min_submits 交够 N 次 below 阈值放行记 figure_accept_as_is note（旧线判据，不静默）；.kv 内核临时目录用完即删；pdftoppm/pdfinfo 缺失跳过记 note。
  改：`skills/kernel` 增 `verify/figure.go` + figure_test.go 4 例 + testimg 助手。
  验证：`go test ./internal/...` 80 例绿（尺寸解析/无 spec 跳过/缺原图/缺 PDF/.kv 清理）。


[ ][ ] SK-P19. PREP 编排接内核第一刀（A1 前置）：prep.Split（poppler 链：pdfinfo→pdfseparate→pdfunite，PagesPerPart 可配，已存在 part 不重建=幂等，单页临时目录用完即删）+ prep.RunMinerU（命令名/额外参数/Runner 全注入；产物用 glob 定位（*_content_list.json 优先非 v2、layout.json）；缺 content_list 报错不静默——MinerU 版本间布局差异靠 glob 兜底而非猜路径）。
  改：`skills/kernel` 增 `prep/`（split.go + mineru.go + prep_test.go 2 例，含 poppler 真集成测试：reportlab 造 5 页 PDF 拆 2+2+1）。
  验证：`go test ./internal/...` 82 例绿。


[ ][ ] SK-P20. organize 接内核（PREP 收尾）：prep.OrganizePart/Organize——content_list 按块拼 md（text/footnote 原文；image/table/chart→`![caption](images/<sha>)`；equation/interline_equation→`$$…$$`），Organize 把多 part 按序拼接成整本 <stem>.md（stem 从 <名>_part1 去尾缀）；md 是 PREP 可再生产物，幂等覆盖重写；纯文本拼装无判据。
  改：`skills/kernel` 增 `prep/organize.go` + `prep/contentlist.go`（parseContentList 与 RunMinerU 共用）+ organize_test.go 2 例。
  验证：`go test ./internal/...` 84 例绿（块渲染断言/整本顺序拼接/part md + book md 三产出）。


[ ][ ] SK-P21. assemble-verify 落地：verify.CheckDrift——遍历 done 任务最后一次 pass verdict 的 artifact_sha256 快照，重算当前 out/ 哈希：不符=篡改/替换、快照有盘上无=删除、新增单列、无快照标 NoSnapshot；DriftSummary 压缩成一行结论；dvagent assemble-verify <项目> 输出 {clean, issues[], detail[]}（装配前漂移清单为空才准进装配）。
  改：`skills/kernel` 增 `verify/assemble.go` + assemble_test.go 1 例；dvagent 增 `assemble-verify`。
  验证：`go test ./internal/...` 86 例绿（未动零漂移/篡改抓 mismatch/删除报 missing/无快照标记）。


[ ][ ] SK-P22. 检索链闭合（doc_search/view_pdf 等价物落地）：dvagent doc-search（Load doc_index.json → Search 回蒸馏命中：seq/global/part/type/text120/bbox）；dvagent page-render（LoadPageIndex 全书页→原书 PDF 页 → pdftoppm 150dpi → work/renders/，可选 bbox 裁剪 + 坐标系 sanity（bbox 超页面两倍报错）+ 物理尺寸 mm 回执）；Build 落盘页表（<名>_pages.json 随索引）。
  改：`skills/kernel` 增 `prep/render.go`（RenderPage/findPage/cropPNG）+ `docindex/pages.go`（LoadPageIndex）+ render_test.go 2 例；dvagent 增两命令；Build 补页表落盘。
  验证：`go test ./internal/...` 89 例绿（页表映射/真 pdftoppm 渲染 A4≈210mm/bbox 半高裁剪/越界 bbox 报错）。


[ ][ ] SK-P23. agent 侧手册落地：agent/README.md（冷启动仪式：status→播报→claim；五条铁律=状态零保存/看图纪律/产物只在 out/交还两行仪式/回炉靠落盘；命令速查）+ 五类任务手册（T1 三态定义与判据证据/T2 作画-编译-自检循环与数值验收证据用法/T3 转换方法+doc-search/page-render 用法+拆分提案+覆盖率二判制/T4 只读核对+位置指针纪律+no_verdict 语义/T5 rework 沙箱续接+逐条复核+profile-update 本书经验）。
  改：新增 `skills/agent/`（README + manual-classify/draw/convert/check/rework.md）；skills/README 索引更新。
  验证：文档批次，无代码变更；交叉引用核对（手册内命令与 dvagent 实际子命令一致）。


[ ][ ] SK-P24. deliver 交付管道：deliver.Deliver 五步——①漂移闸门（CheckDrift 有 mismatch/missing 即拒装配且不动 build 树；NoSnapshot 只记不拦）；②归集（done conv_* 的 out/*.tex→build/chapters/保持相对结构、done drw_* 的 figure.tex→build/figures/）；③主文件（documentclass[+可选 cls]+按章序 \input）；④input@path+书签注入+standalone 内联落盘；⑤装配后资源盘点回执。dvagent deliver <项目> [cls]。
  改：`skills/kernel` 增 `deliver/`（deliver.go + deliver_test.go 2 例）+ dvagent deliver。
  验证：`go test ./internal/...` 92 例绿（归集/章序/书签=章数/standalone 内联标记/盘点零缺失；漂移闸门拒装配不动树）。


[ ][ ] SK-P25. A1 全链演练（验收标准 A1 的自动化前置）：internal/e2e 一条测试打穿全链——reportlab 造 3 页书 → prep.Split(2页/part) → fake MinerU runner 产 content_list+图 → Organize 拼 md → docindex 建索引+检索断言 → 注册项目（新 workspace）→ GenClassify（模拟 agent 验收循环：vector/styled_text 分流）→ GenDraw（vector 图作画交付）→ GenConvert（自包含 tex）→ GenCheck（自动解锁+pass）→ 队列重载（A2 冷启动）→ deliver（漂移闸门过/书签/standalone）。**修真 bug×3**：GenClassify 漏拷图（自包含原则没贯彻到分类器）、GenCheck 对已 done convert 挂依赖边（顺序生成核对永锁）、Registry.Save 缺 MkdirAll（新 workspace 注册失败）；docindex 兼容 MinerU auto/ 布局。
  改：`skills/kernel` 增 `internal/e2e/`；修 `gen/tasks.go`、`gen/check.go`、`project/registry.go`、`docindex/docindex.go`。
  验证：`go test ./internal/...` 12 包全绿（e2e 通过：5 任务 done、交付 build/ 生成）。


[ ][ ] SK-P26. MCP 壳落地：dvagent mcp——stdio JSON-RPC 2.0（newline-delimited）实现 initialize/tools/list/tools/call；7 工具（status/claim/submit/note/doc-search/assemble-verify/deliver）直接调同一组内核函数（CLI 同构，零行为漂移）；通知类消息（initialized/cancelled）按协议不响应；未知 method -32601、未知工具 isError:true。
  改：`skills/kernel` 增 `cmd/dvagent/mcp.go`（~250 行）+ main.go 注册 mcp 子命令。
  验证：go vet/test 12 包绿 + MCP 冒烟四连（initialize→tools/list 7 个→tools/call status→未知工具 isError）。

## 附录：原文存档（P68–P76，2026-11-07）

用户原话照录，仅把样本身份按仓库规矩替换为「（书名略）」。

1. 上面那个可以先加入计划我后续才裁决，2. 现在我有个新的想法，之前的一直都是针对扫描版的书，没有考虑过那种非扫描版的教辅资料或者文档或者论文， 他们本身pdf就是矢量的只是我们这边想要还原出原本的那个排版布局。但是这样会导致我们对其中的某些内容的误判，比如本来就是非矢量的图我们硬生生的给转矢量，这样甚至就错失了原本的这个机会（但是这个是一个可以开的选项，如果那个图本身就是矢量图（识别出来）是可以通过矢量绘制的一些 表格图形、数学图像、展示内容、线框之类的数据图什么的，这个可以通过一个开关用来这里面精细调控这次转换是否将这些可以矢量化的图像矢量化），其他的原始的raster或者其他的的图像应该从原版的pdf中原样提取。就是如果原pdf是矢量的而非扫描书本，我们甚至就无需矢量转换这一步是可做可不做的。因为所有的图都可以从原来的里面提取（不需要用mineru的图了），这边就可以直接进入章节转换，有的pdf中可以提取出原来的本来就是latex或者pdf或者其他资源嵌入的矢量图，这边还可以精确的给裁剪回来（在矢量图会话因为可以完美的匹配）。后续来使用，我觉得这样的会更好一些。然后这边对识别是否非扫描版的图书的我认为可以通过的是一种精细化的方法，并非判断整本书是不是扫描版的因为可能半扫描半自己整理的，就是这边在这个图片（mineru提取的出来的）所在的位置上这个图所在的这一页pdf，是否是一整个图在一整个页面中（或者是这张图是否是作为嵌入元素放进来的而非整体的扫描页，当然也要区分一种情况就是这个非扫描版的页面中比如是插入了一个说明的图片比如是一本书的一个截图，他并没有要描述他的意思，只是进行了一个呈现，该图就应该属于是不需要矢量化的说明mineru分的比较细，这边需要从原pdf中抠出来这个图本身来呈现。作为后续章节转换再次嵌入的操作。）。当然上面的操作或多或少已经超出了矢量图的范围，可能是要在矢量图转换之前就进行一个判定，比如在那个dedupe阶段可能还是可以有额外的会话对这些图本身进行一个分类？或者在classify那边增加更多的工具和他们能够查看原图所在的pdf页和前后几页？来判断？或者在dedupe阶段再进行一个整体的判断看看那些是否有问题，再根据配置选项来看某些矢量图（本身是非矢量的图片潜入）是否应该矢量化。3. 第二还有一个比较特殊的情况就是，本身这个矢量图的绘制和样式那一步多多少少有重叠的倾向，因为比如之前这个（书名略）就是，他里面大批量的重复了一种结构行为的图片构造，这种构造具有一种通用的模板，如果机械的全部在矢量图阶段不分你我的重绘，就会出现大量的token消耗并且后续的话还需要再次对这些图进行改造，所以我看这个矢量图阶段可能在后续需要被打散作为一个类似工具调用的手段触发？ 或者说在判定之后再进行绘制，和style这边有时候也需要本身开子会话对这些矢量图的行为进行一个高度的抽象模板。还有就是这边在前面情况下这边还有从非扫描的中抠出来的图的情况也需要调整，还有非扫描的图里面本身已经是矢量的了是否需要重绘的行为（比如原来已经是矢量的了但是书中大量使用同一种模板的行为构造并且和描述的内容上下文有关系这边是否需要自己构造，因为比潜入图片更好精确控制其行为）。这些都是需要考虑的问题，这个在样式阶段可能就要考虑并且控制后续很多图可能都不需要重新绘制了，但是一个样式会话又不太可能完成如此多的任务，需要进行一些调整？或者子会话的功能。并且这边需要在查看某图附近的时候考虑这个图是否应该进行重新绘制的问题。4. 还有就是这个图可能本身就存在外部的图我可以提前的进行放入比如某些raster的图，或者矢量图已经绘制或者提供了，这边有这本书里面的精确的图像，我们可以就只操作对应的排版和布局就行了，资源已经有了，并且还可以自己选择是否对这些资源进行一个矢量化 5. 还有就是可以在事后或者事前可以在语义上调控这其中的阶段中某些 无法精确描述的行为，比如这本书本身某些地方比较复杂，可能我想让里面的模型在处理 xxx 的时候按照某种思路来修改而非按照他的理解，应该为我所指挥。比如在处理样式的时候可以对某些样式特化或者我不想让他按照书上的或者书上有问题，或者需要我的指导更方便。同时还有事后的转换完成之后可以再进行一些调整比如我觉得哪里做的不好他可以精确的返工而非重跑所有环境。而是精确的下发到对应的分区和任务中。 6. 基于前面这些思想，更像是我们的软件本身需要维护一个类似领导会话一样的会话存在，就像是新发布的 deepseek harness super-agent插件。这个顶层可以根据我们的需要对工作流进行宏观的调控起停，对错误进行修复对会话流程的接入和数据资源的流向调控，还有内部的提示词和打断插话之类的。这样可以比如用户有问题可以起某个阶段的流程，但是这也对我们对我们的资源和数据流程管理需要有更严格的把控。特别是处理这些非能够精确用程序语言描述的需求，类似前面说的，样式中对矢量图同一模板配色的行为，子会话或者其他的调控。（同时我补充一个，还可以对样式会话提供一个compare figure类似的可以对比和原文件的样式的，但是这个要求很高对选图是否是原来的或者等等，但是可以提高样式的准确性比如行间距什么的）7. 所以根据前面的所有思考，我们本地的这个手搓的会话管理其可能用起来就没有那么顺手了。可以参考类似dsh的Pi-ai作为底层的会话管理和控制我们可以只做上层流的管理还有webUI。并且webui这边我们还可以参考开源的dsh甚至为我们所用，甚至将我们作为一个插件嵌入在dsh中，用起来就更舒服了。同时也要提供go的cli高性能版本（不过都一样都可以在cli界面中用，总之还是需要一个webui的但是webui可能并非必要的但是也可能必须要，因为这边还需要用户和顶层会话交互。反而这个img2text模块就可以单独弄出去了做md的，但是这边也可以用Pi-ai，后续方便开发和维护，并且他也可以介入到dsh中）8. 以上问题和我的想法可以先记录在docs中完整的把我的原文先记录，然后可以总结和汇总并且加入plan计划，然后你分析一下整体我的想法如何，你看着给我整理一下然后给我汇报。
[X][ ] P78. 逐图阈值：重画图要"够像"才能交，阈值可按图在网页上调，也会量尺寸与位置而不只是墨迹形状。
  改：① `latex.figure_check.min_overlap`（软墨重叠，默认 0.17、0 关闭）与 `max_scale_dev`（宽高比例偏差，默认 0.10）卡住 `submit_figure`，回执直接给数字与差值（"差 3.2 个百分点 / 窄了 9%"）；② `submit_figure` 增 `accept_as_is` + 必填 `note`（尽力还原），放行并记 `fig_accepted_as_is`/`fig_best_overlap`，核对页与报告可查；③ 每图阈值 `fig_min_overlap` 存进 progress item（0 = 用全局），从网页改；④ 网页每图一行：阈值输入 + 「重画这张」（复用现有 `role=vector` 队列，人工打回后从该图作图转录续跑，不新建尝试）+ 「按尽力还原接受」。
  根因：`compareFigureImages` 先用 `fitFigureCanvases` 把候选按原图宽度等比缩放、再按墨迹外接框裁剪、按质心对齐——宽度差异、画布内位置、纵横向比例全被"对齐掉了"，只剩墨迹形状相似度，所以 17% 也能看着"对上了"；而 `submit_figure` 今天只要求"调过一次 compare_figure"，没有任何数值门槛。
  风险：阈值定太严会让模型在达标线上反复重画烧 token（"尽力还原"就是这条的泄压阀）；尺寸偏差要按墨迹外接框算，不能按 standalone 的 trim 边框（那是编译产物，不代表图在页面里的大小）。
[X][ ] P79. 成对图的线条与样式统一：原书里并排、或同一个图号下的 (a)/(b) 两半，转换章节时必须当成一套来排版——线条粗细、配色、字号、箭头两边一致，只保留各自的几何与标签。
  改：`latex_convert.system.md` 的围栏条目加一句"同图号的图彼此也要一致"；`pairedFigureNote`（`internal/latex/pairedfigures.go`）在 convert 首条消息里点名本章是哪几对：两个矢量围栏之间既无小标题、也无`图 8.1`这类图注、且相距 ≤60 行，就判为一对（(a)/(b) 的标注不算图注，正好是同图号的标志）。
  验证：`TestPairedFigureNote` 七个用例（(a)/(b) 判成一对、各自有图注/中间有小标题/相距 70 行都不是）；拿一本八章的成品实测只点出 5 对，没有把"图 8.1 与图 8.2"这种相邻但不同图号的算进去。
[~][ ] P80. 插图绘制的全书统一样式：样式会话产出的 cls 提供一套具名 TikZ 样式族（`dv-wire`/`dv-axis`/`dv-grid`/`dv-label`/`dv-thick`…，配 `\dvfigwire` 等宏），作图会话只写语义引用、围栏开头留守卫兜底，转换只做残留宽度映射；全书绘图参数从此只有一处来源，P79 的成对说明降级为兜底。设计稿 `docs/plan-report/p80-figure-style-unity.md`，分 S1（cls 定义样式族 + `source/figure_ink.json` 实测统计）/S2（作图改语义引用）/S3（convert 映射 + preflight 警告）三步落地。
  改：`latex_style.system.md` 要求 cls 定义整套 `dv-*` 与 `\dvfig*`；`latex_figure.system.md` 改语义引用 + 守卫兜底；新增 `internal/latex/figureink.go` 聚合每图实测线宽；`preflightConvert` 警告残留 `line width=`。
  风险：守卫 `\ifdefined\dvfigloaded` 依赖 cls 在导言区先加载；旧项目围栏写死宽度，只做一次性映射、不强制重画。

[ ][ ] SK-P27. 验收门槛程序强制化 + 用户专用配置层（用户要求 2026-11-18）：矢量图重合度默认 17%（min_overlap 默认 0.17，用户指定）、accept_as_is 必须交够次数（accept_min_submits 默认 3）、阈值三级来源=验收 spec>用户配置(skills_config.json)>内置默认；**config-get/config-set 只进 CLI 不进 MCP/说明书——agent 无权调阈值，用户不提不改**；损坏配置回默认（坏文件不能绕过门槛）。
  改：`skills/kernel` 增 `internal/config/`（Defaults{0.17/0.10/3,100,300}、Load/Save 原子写）；`verify/figure.go` 阈值三级回退；dvagent 增 config-get/config-set（CLI 专属）。
  验证：`go test ./internal/...` 13 包全绿（默认断言/回写回读/损坏回落）。

[ ][ ] SK-P28. 人类可读编号体系 + sha 映射表（设计定稿见 docs/02-task-protocol.md）：imgxxx=原图/vecxxx=矢量成品/pdfpxx=页码；映射表 skills_ids.json（id↔sha/路径/global 页/bbox）；agent 内部仍用 sha。
  改：待实现（ids 包 + 生成器登记 + 看图台/手册引用）。
  验证：待补。

[ ][ ] SK-P29. img2text 任务类型（设计定稿见 docs/02-task-protocol.md）：逐图小轮转会话（上下文 up/down 检索+看图+提交描述三工具）；页码/行定位在 Organize 合并时从 docjson 打标识。
  改：待实现（任务类型+工具+ Organize 定位标识）。
  验证：待补。

[ ][ ] SK-P30. 看图台（HTML）：分组对比/阈值调参导出请求/进度状态/PNG 导出/按 imgxxx 编号指代；复用旧线 watcher 请求文件机制（内核唯一写通道）。
  改：待实现（静态 review 页生成 + 请求处理）。
  验证：待补。

[ ][ ] SK-P32. 终审任务类型固化（T7）：全书 convert/check 落定 + 至少一次核对完成后铺 fin_book 只读终审（章节衔接/目录书签/跨章一致性/装配整体；verdict 复用 check 语义 final schema）；编译修复不固化——验收打回走 rework。
  改：`task.TypeFinal` + `gen.GenFinal`（触发条件/幂等）+ verify final schema + manual-final.md + dvagent gen-final。
  验证：`go test ./internal/...` 15 包全绿（三态触发/可领/幂等；checkDone>0 条件防"未铺核对就终审"）。
