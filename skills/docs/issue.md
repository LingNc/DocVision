# skills 线 issue 台账（独立编号 SK-I<N>）

> 与参考实现 `docs/issue.md` 的 T 编号完全隔离——本文件只记 skills 线自己的问题/需求。原参考实现 T328（skills 线开发）已迁移为 SK-I1。

**旗标语义**（同仓库规矩）：第一个 `[ ]`=Agent 完成态（`~` 在做 / `X` 完成），第二个 `[ ]`=开发者审核（`T` 通过 / `F` 打回）。

[X][ ] SK-I1. skills 线开发与判据迁移：LLM 会话外迁到宿主 agent，内核只做确定性执行+验收。
  根因：目标形态=用户只装 skill + MinerU，零 LLM API 配置，放下 PDF 跑到底/中断无损续/换 agent 可续。
  改：分支 `feat/skills`，新增 `skills/`（docs 设计文档+验收标准 A1-A10、kernel/ 十四包、agent/ 手册）；判据逐字移植参考实现（覆盖率 T168/dedupe T130-T151/compareFigure T307-T312/checker T222-T233/书签 T49/盘点 T33/input@path T324）；五类任务闭环+rework；详见本目录 plan.md SK-P1–SK-P26 与参考实现 docs/agent-batch-history.md 第三十七至第六十二批。
  验证：`go test ./internal/...` 12 包全绿（含 poppler/pdftotext 真集成冒烟、A1 全链演练 SK-P24）。

[X][ ] SK-I2. 验收门槛全部程序强制化（用户要求，2026-11-18）：矢量图重合度默认 17%、accept_as_is 必须交够次数才放行、**阈值只允许用户显式调整**（agent 无权自调，用户不提不改）；章节划分语义修正（初划独立单层阶段可有可无；递归细化在 convert 阶段内部，默认 3 层，无初划=4 层）；img2text 是独立任务类型（逐图小轮转会话：上下文上下检索/看图/提交描述，kernel 整理）；人类可读编号 imgxxx/vecxxx/pdfxxx + sha 映射表。
  根因：防模型偷懒与防阈值被 agent 自行放宽；img2text 与 latex 转换是两类任务不能混。
  改：skills/docs/plan.md SK-P27+（config 层）、SK-P28（编号映射）、SK-P29（img2text 类型）、SK-P30（看图台）——四项全部落地。
  验证：config 层单测 + e2e 复跑；2026-11-18 23:4x 复核实物：ids/img2text+mdimg/viewer/final 四包俱在且测试绿，SK-P28–P30 台账"待实现"字样已更正。

## 待拍板（2026-11-19 原仓库全量克隆后对照发现，均非 bug 为增强）

- [ ] SK-I3. **watermark 检测缺失**：原程序有 `latex_watermark` 阶段（检测书内水印/广告图，建议删除）；我们两档都没有——md 档靠 agent 三态可能判 keep 保留水印，LaTeX 档原样嵌入。是否需要加专门检测（或并入 classify/rework 提示词要求）？
- [ ] SK-I4. **verify 单图三对照弱一层**：原程序 `latex_verify` 是模型 QA（同一张图三视角：原图/TikZ 渲染/正文嵌入内容语义对照）；我们 figure 验收是程序几何判据+accept 次数，语义对照只靠 rework 轮模型自查，无强制 QA 任务。是否补一个 verify 任务类型？
- [ ] SK-I5. **classify 可补 confidence**：原 `latex_classifier` 有置信度+低置信强制复查；我们三态分类无 confidence 字段。小增强，可做可不做。
- [x] SK-I6. **[IMG_TYPE] 已保留**（11-19，用户拍板）：img2text 产物首行强制 [IMG_TYPE: 八类词表] + verdict.json img_type 字段一致校验（verify/schema.go 三道闸），成品 md 随文保留类型头。

其余对照结论（无缺失）：原 chapters 拆章≈我们 convert 内递归细化+行数上限；fix/stylefix 三段式≈style 四步合并+rework 循环；~~styled 标记职能由 styled_text 类等价覆盖~~（此结论被 SK-I18 推翻：当时确属丢失，已恢复四类+styled 独立标记）。

## 已修复（2026-11-19 下午，daoshu 实操前修）
- [x] SK-I7. MinerU 上游拒收 model_version="v2"（传 400：model_version not supported）——上传仅带后端自配字段；显式 model_version 需等后端支持。
  根因：上游 API 契约收紧。改：mineruapi/client.go requestUploadURL 移除 model_version。
  验证：e2e TestMineruAPIViaCLI 真 API 绿。
- [x] SK-I8. NormalizeLayout/auto 布局错位（嵌套搬运把产物搬成 <part>/auto/<data_id>/，images 丢失；上游直出 part 根时被二次搬空）——重写为三形态自识别：标准 auto 原样返回 / part 根原样返回（最浅命中优先，防嵌套残留干扰）/ 中毒嵌套摊平到 <part>/auto/；prep-organize/gen-img2text 兼容 part 根产物布局。
  根因：上游 MinerU 产物布局变更（直出 part 根）+ 旧归位逻辑"整目录搬移"+ 顶部"auto 存在即返回"误判。
  验证：normalize 4 新用例（摊平/part 根不动/最浅命中/缺失报错）+ e2e 全链绿 + 全量 20 包 0 FAIL。
  交接勘误：HANDOVER 所列 mcp.go 改动实际不需要（mcp.go 未引用 NormalizeLayout）；mcp.go 与 main.go 同经 DownloadAndExtract→NormalizeLayout 链，函数内自愈后调用方零改动。

## 记录修正与补充（2026-11-19 下午二轮，按原 DocVision 老线对齐）
- [x] SK-I7 修正：拒收真相是**值非法**而非字段非法——11-19 实测探针：model_version="vlm"→200，"v2"→-10002，省略→200。合法值仅 pipeline/vlm/MinerU-HTML。最终方案：默认 model_version="vlm"（老线同款值），不再置空省略。
- [x] SK-I8 修正：**auto 层是本线自引入的中间态，老代码结构约定=part 根无 auto**（老 docindex 直读 <part>/*_content_list.json）。NormalizeLayout 终版=收敛 part 根：任意上游形态（顶层文件夹/auto 层/直出/混合残留）mergeInto 合并进 <part>/ 删壳，幂等；prep-mineru-api 断点①判据改为 part 根 content_list 存在。
- [x] SK-I9. MinerU 配置默认值对齐老线实测可用值：model_version=vlm / is_ocr=true / language=ch（我们原默认 false/""，质量退化和老线不一致）；顺带修复 main.go 零值不安全直赋（config.yaml mineru 段少写几行会被静默重置的 bug）。
  验证：normalize 4 新用例（嵌套收敛/幂等/混合残留/缺失报错）+ 全量 20 包 0 FAIL + e2e 真 API 带 vlm 线上复验。

## luadraw 引入批次（2026-11-19）
- [x] SK-I10. 引擎按需：章内探测 \usepackage{luadraw} → 该章强制 lualatex（texcheck.CompileEngine/NeedsLuaLaTeX，verify 编译验收接线；不全书强制）。
- [x] SK-I11. style 总览动作已进 manual-style（模板产出+打回散写规则）；gen-draw 按章按需铺已落地——CLI `gen-draw <项目> <章md名>` 从章 md 插图 sha 集合过滤（nil=全书默认不变，纯增量不影响在跑流程）。
- [x] SK-I12. ~~claim --wait~~ **claim-wait 命令已落地**：`dvagent claim-wait <项目> <owner> [--interval N] [--timeout N] [--types t1,t2]`——阻塞轮询等内核分发，timeout=0 不限；会话阻塞不占 CPU 可随停。
- [x] SK-I13. draw-style.tex 模板已入 `agent/templates/`；manual-style 增"绘图样式总览"节；checkspec 条件式强制已落地——style/draw-style.tex 存在才要求主文件引用模板（散写打回），无模板老项目自动跳过。
- [x] SK-I14. luadraw 文档入 skills：包体+文档源码（仓库）、en PDF+514 条索引（PDF 物理页码映射，tools/build-index.py 可重建）+ luadraw-search 检索命令；包内只带 PDF+索引+指南，源码/包体现场取。
- [x] SK-I15. 项目共享记忆区 `memory/`（feat/skills-memory 分支）：CLI+MCP 各四命令（put/get/list/rm），原子写无锁，约定 render/notes/digest 三分区；register 自动建区。行为验证：put→get→list→rm 全链 + 路径逃逸拒绝。
- [x] SK-I3. 水印检测（用户拍板默认去水印）：classify 识别→verdict 标 "watermark":true；img2text 合成阶段整行删除（Replacement.Drop，含 pagemark 负位移）。验证：TestAssembleWatermarkDrop。
- [x] SK-I4. ~~verify 单图三对照~~——**用户裁定不实施**（原程序该阶段自身也未启用过，不做无效功）。
- [x] SK-I5. 分类置信度（用户拍板）：classify verdict 必填 confidence（0~1），<0.6 低置信强制重判；越界/缺填打回。验证：TestClassifyConfidenceGates 五用例。
- [x] SK-I16 **内核已落地**（22:35，0cae7c4）：texcheck.PrecompileLuaDraw/DecideEngine/LuaAvailable——luadraw 环境块→lua5.4 预编译→TikZ→正常链；双路探测（有 lualatex 直编/无则预编译）；垫片与 lingnc-memory/toolchain/luadraw 对齐（complex 3502 行产物过 tectonic 为边界基准，table.append 合并语义坑由其发现）；测试改编自 toolchain user-2d/user-complex。spike **已通过**（22:09，docs/luadraw/spike/）：plain lua5.4 独立跑通官方 3D 示例，产出完整 tikzpicture（1016B 含透视/着色/裁剪）。四个垫片方案定型（searcher 基名归一/luadraw_base 先载/lualibs 垫片/tex 空桩）。**实现待排期**：texcheck 加预编译步骤 + NeedsLuaLaTeX 改双路探测（有 lualatex 直编/无则预编译）。go-lua 内嵌经评估不采纳（luadraw 用 Lua5.3+ 语法 // 整除，gopher-lua 5.1 不兼容）。
- [x] SK-I17. organize.go 命名推导不兼容 part 根输入（运行会话 e23e4e2 实跑暴露）：part 根输入时 `Dir(auto)` 取到 mineru/ 致章节命名错位（mineru.md 双写覆盖）。修：partStem() 统一推导（part 根取自身名/auto 布局取父目录名），循环命名与 bookStem 两处同换；docindex 报错带试过的 md 名与现存目录名。验证：TestPartStemPartRoot 双布局用例 + daoshu 实跑（prep-organize 产出 part md+book md 正确命名、prep-index 建 68 条目/10 页）。
- [x] SK-I20. 账本防伪戳（用户 11-20 "手造队列严令禁止"）：Queue.KernelStamp=dvqueue/v1，Save 盖戳/Load 验戳（有任务无戳=手造，拒载并指回禁令）；queue-stamp <项目> 迁移旧版无戳队列；Save/Stamp 加 FUSE root 属主文件 rm+重建回退。验证：3 队列测试 + daoshu 全链路（拒载→stamp→claim 领任务）。
- [x] SK-I21. keep 态收窄为仅水印（用户 11-20，老线 dev 分支 embedBlockForRef 原文核实）：终态 md 无裸图链接——非水印图必须 text（[IMG_TYPE:] 描述）或 mermaid；原 keep 场景改 text+描述（对应老线 `[Image]( 描述 · P.N )`）。schema 硬闸 + 手册/提示词同步 + assemble 输出 bare_links 指标。
- [x] SK-I22. md 档输出对齐老线 dev 契约 + 页码单一系统（用户 11-20：dev 的 [Image]( body · P.N ) 锚格式 + 两套页码共存冲突）——organize 的 <!-- pN -->（content_list 全页枚举）为地面真值，AssembleWithShifts 原位转 [P. N]（格式我们的，编号修复缺页漂移）；pagemark 模糊对齐那套退出生成路径（弃）；嵌入三形态照 dev embedBlockForRef：text 类纯 body/其余 [Image]( body · P.N )/mermaid=[Image]( alt · P.N )+围栏；测试 21 包 0 FAIL（新增 dev 三形态用例）。
- [x] SK-I23. style 模板目录未进沙箱编译搜索路径（对方会话 11-20 在装有 TeX 的机器实跑暴露：TestStyleTemplateEnforcement 失败，draw 章编译报 File 'draw-style' not found——无 TeX 机器因跳过编译而假绿）。修：texcheck.CompileEngineEnv 附加 env，defaultCompile 传 TEXINPUTS=<project>/style（前置合并且保留默认路径）。

