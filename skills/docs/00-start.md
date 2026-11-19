# START · 从这里开始（所有读者的第一站）

> 本文件回答三个问题：**你是谁该读什么**、**这本书走哪条流水线**、**遇到不懂翻哪里**。

## 0. 铁律（所有读者，尤其 agent）：文档优先，源码最后

**遇到任何"不知道怎么办/行为不符合预期"——按此顺序查，禁止跳步去读内核源码：**

1. **任务手册** `agent/manual-<任务类型>.md` —— 你手里任务的完整操作说明（怎么领、干什么、交什么、怎么算过）
2. **规约文档** `docs/01-architecture.md` ~ `docs/08-figure-flow.md` —— 架构、任务协议、判据、流程设计
3. **内核自述** `dvagent <命令> --help` / `dvagent doctor` —— 命令参数与运行环境
4. **源码是最后手段**，且看完必须回头核对文档是否过时——文档随版本走，源码实现细节随时会变；只信文档，不信你从源码里推测的行为

> 理由：流水线由内核程序强制判据，文档描述的是**契约**；源码是契约的一种实现。从源码推测行为 = 把契约和实现混淆，版本一升就全错。历史上所有"看了源码才搞懂"的时刻，都是文档欠账——那是文档该修，不是你该读码。

## 1. 读者分诊

| 你是谁 | 先读 | 再读 |
|---|---|---|
| **执行 agent**（被派来干活的） | `agent/README.md`（冷启动仪式+铁律） | 领到任务后读对应 `agent/manual-*.md` |
| **人类操作者**（跑 PREP 段的） | 本文 §2 流程图 + `docs/05-first-run.md` | 卡住查 `docs/07-troubleshoot.md` 状态机 |
| **skills 宿主系统**（加载本技能的） | `SKILL.md` | `docs/01-architecture.md` |

## 2. 模式分诊：这本书走哪条流水线

```
拿到扫描版书籍 PDF
        │
        ▼
要干净 Markdown（图片逐张三态文字化/结构图/保留）？
        │
   ┌────┴────┐
   是                否（要排版精良的矢量 PDF 书）
   ▼                ▼
【md 档】          【LaTeX 档】
```

### md 档最短路径

```bash
D=./kernel/bin/dvagent
$D -workspace ws doctor                                   # ① 体检
mkdir -p ws && cp 书.pdf ws/ && cd ws
$D register demo book && $D activate demo
$D prep-split demo book/书.pdf 100                        # ② 拆 part（幂等）
$D prep-mineru-api demo book/source/parts/*.pdf           # ③ OCR（需 MINERU_TOKEN，断点续传）
$D prep-organize demo && $D prep-index demo               # ④ 整理+索引
# ⑤ agent 工作段：gen-classify 铺任务 → 逐图三态（手册：agent/manual-classify.md、manual-img2text.md）
$D img2text-assemble demo chapters/书.md out/book.md      # ⑥ 程序合成成品
```
每步产物与判据见 `docs/05-first-run.md`；第 ⑤ 步的 agent 行为规范见 `agent/README.md`。

### LaTeX 档最短路径

```bash
D=./kernel/bin/dvagent
$D -workspace ws doctor && mkdir -p ws && cp 书.pdf ws/ && cd ws
$D register demo book && $D activate demo
$D prep-split/prep-mineru-api/prep-organize/prep-index demo   # 同 md 档 ②③④
# agent 工作段：classify → draw → convert → check → rework 循环 → final（手册五本，见 agent/README.md 表）
$D deliver demo                                                # 装配交付
```

两档的 PREP 段（②③④）完全相同；分水岭在 agent 工作段的任务类型。

## 3. 名词对照（旧实现 → 现行命令）

| 你印象里的旧名 | 现行等价 | 说明 |
|---|---|---|
| `get_more_context(↑N, ↓M)`（单工具双方向，md 行号定位 `{LINE}/[{UP_START},{DOWN_END}]`） | `dvagent ctx <项目> <任务id> [--up N] [--down M]`——**同形态对齐**：单次双方向、md 行内容+行号（图像 sha 定位），返回 `{image_line, range, lines}` |
| 进度/账本由模型输出 JSON 自报 | `dvagent claim/submit` + 内核 verdict.json | 账本权从模型手里没收给了内核状态机（防伪造进度） |
| `verify` 三对照 QA 阶段 | 无（SK-I4，用户裁定不实施） | 几何判据（min_overlap 等）+ rework 自查覆盖 |

文档与实现不一致时，以 `upgrade-check` 后的最新文档为准，并向维护者报 issue（docs/07 状态机）。
