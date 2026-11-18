# 05 · 首秀操作手册（A1 真机端到端）

> 目标：从空机器到一本真书交付。每步都可独立中断续跑（幂等是各命令的内建性质）。
> 前置：`go build -o dvagent ./cmd/dvagent`，`./dvagent doctor` 把必需项补齐（pdflatex/MinerU/poppler）。

## 0. 建工作区与登记

```bash
mkdir -p ~/dv-workspace && cd ~/dv-workspace
mkdir -p book && cp /path/to/书.pdf book/book.pdf
D="~/DocVision/skills/kernel/dvagent -workspace ~/dv-workspace"   # 命令简写
$D register demo ~/dv-workspace/book
$D activate demo
```

## 1. PREP（无人值守段，全自动）

```bash
$D doctor demo                                  # 复检项目级（字体目录等）
$D prep-split demo book/book.pdf 100            # 拆 part（提速版，幂等）

# MinerU 二选一：
export MINERU_TOKEN=<你的 token>                # ① API 模式（轻，零安装；断点续跑：重跑不重传）
$D prep-mineru-api demo book/source/parts/*.pdf
$D prep-mineru demo book/source/parts/*.pdf     # ② 本地 CLI（重，需 pip install magic-pdf[full]）

$D prep-organize demo                          # content_list → chapters/*.md（带页码标识）
$D prep-index demo                             # doc_index + 页表（检索与渲染的数据源）
```

## 2. 图片阶段（agent 接管 LLM 工作）

```bash
$D gen-classify demo                           # 图铺 classify 任务（含 imgxxx 登记）
# agent：claim classify → 逐图三态写 verdict.json → submit
$D viewer-build demo                           # 看图台（分组/调阈值/导出 regroup）
# 用户：浏览器打开 work/viewer/index.html 调组 → 导出 regroup.json 存 work/retry/
$D viewer-apply demo work/retry/regroup.json  # 应用分组（组员跟随代表）
$D style-collect demo                          # styled_text 提取内容进素材池
$D gen-draw demo                              # vector 图铺重绘任务（只为代表）
# agent：claim draw → TikZ 作画编译自检 → submit（数值判据自动验收）
$D gen-img2text demo                          # 可选：逐图描述（检索兜底/素材）
# agent：claim img2text → ctx-up/down+page-render → description.md → submit
$D img2text-collect demo                      # 描述回填 doc_index
```

## 3. 转换阶段（agent 逐章）

```bash
$D config-set demo style.pilot_chapter chapter_001   # 指定试点章（可选但推荐）
$D gen-convert demo                           # 章节铺任务（md 拷沙箱作覆盖率对照源）
# agent：claim convert → 写 .tex（自包含）→ 编译+覆盖率自检 → submit
#   上下文不够：dvagent split <项目> <task_id> segs.json（父子扇出，3 层递归）
$D gen-check demo                             # 核对任务自动 blocked 于 convert
# agent：claim check（convert done 自动解锁）→ verdict.json → submit
# 发现问题：issue-add → rework 自动生成、原任务挂起，修完自动解锁
$D gen-final demo                             # 全书落定后铺终审 fin_book
# agent：claim final → 全书级核对 → verdict.json → submit
```

## 4. 交付

```bash
$D assemble-verify demo     # 漂移闸门（验收后产物未被篡改才准装配）
$D deliver demo             # 归集→main.tex→书签→standalone→资源盘点
# 产物在 book/build/：main.tex / standalone.tex / chapters/ / figures/
```

## 故障排查速查

| 现象 | 第一步 |
|---|---|
| 任务 claim 不到 | `$D status demo` 看阶段；blocked=等依赖，leased=租约未过期（或别处在跑） |
| submit 被打回 | 错误清单是机器判据原文：缺产物/自包含/编译/覆盖率/verdict 模式，逐条对修 |
| 想停/想换书 | `pause demo`（任务边界停）/ `abort demo`（封存）；`resume demo` 续 |
| 阈值想调 | `config-set demo figure.min_overlap 0.3`（用户专属；agent 无权） |
| 忘了在干嘛 | 冷启动仪式：新会话跑 `status` → 读 work/ 与 notes.md → claim 继续 |
