# SKILL.md —— DocVision Skills 技能自述

> 你（宿主 agent）正在读的是一个**技能包的自述文件**。读完这份文件你就知道该怎么办。

## 这是什么

把扫描版书籍 PDF 转成结构化产物的转换技能：档位2 输出干净 Markdown（图片三态处理：文字化/mermaid/保留 + 页码标记），档位1 输出全书 LaTeX（矢量图重绘/样式/编译/终审）。**你干活，内核验收**——所有"过/不过"由 `dvagent` 内核机械判定，你不背判据，只照手册把活干好。

作者：绫袅（LingNc）。版本见包名后缀。

## 前置依赖（内核不提供二进制）

内核是 Go 源码，**在你本机编译**（go ≥1.25，自动下工具链）：

```bash
cd kernel && go build -o dvagent ./cmd/dvagent
```

为什么源码交付不给二进制：跨平台/跨架构无法预制，编译是一次性成本，且 `go build` 即完整性校验。

## 启动序列（五步）

```bash
D=./dvagent                                   # 内核 CLI
$D doctor              # ① 体检：缺什么装什么（poppler/MinerU 或 MINERU_TOKEN/pdflatex 仅档位1）
$D -workspace <ws> register <项目> <项目根>     # ② 建工作区登记项目
$D -workspace <ws> activate <项目>              # ③ 激活
# ④ 照 docs/05-first-run.md 放书跑全流程（PREP→图片阶段→转换→交付）
# ⑤ 你作为 agent 的工作方法：读 agent/README.md（冷启动仪式+铁律）与 agent/manual-*.md（七类任务手册）
```

## 三条纪律（违反会被打回，且烧你的 token）

1. **状态零保存**：进度全在磁盘（队列/制品/notes.md），你的上下文只是投影；看图→决策→写笔记→忘掉。
2. **产物只在任务沙箱 `out/`**：引用临时路径=打回；交还前 notes.md 追加两行交接语。
3. **阈值不许你动**：验收门槛（重合度/行数/覆盖率）程序强制，只有用户能调（`config-set`，你没这个工具）。

## 结构索引

- `docs/01-architecture.md` 架构与状态纪律 · `02-task-protocol.md` 七类任务契约 · `03-agent-interface.md` 接口哲学 · `04-migration-inventory.md` 判据来源台账 · `05-first-run.md` 首秀操作手册
- `agent/README.md` 冷启动 + 铁律 · `agent/manual-*.md` 七类任务工作手册 · `agent/prompts-img2text.md` 档位2 提示词
- `kernel/` 内核源码（dvagent CLI：32 命令 + MCP 模式 `dvagent mcp`）

