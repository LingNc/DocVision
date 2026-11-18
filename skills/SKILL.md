# SKILL · DocVision Skills —— 书籍转换流水线

> **这是什么**：把扫描版书籍 PDF 转成结构化产物的 agent 驱动流水线，两种产出模式——
> **md 档（img2text）**：干净 Markdown（图片逐张三态文字化：提取文字 / mermaid 结构图 / 原图保留，自动页码标记）；
> **LaTeX 档**：全书 LaTeX（矢量图 TikZ 重绘、统一样式、编译+覆盖率验收、全书终审、装配交付）。
> 作者：绫袅（LingNc）· 版本见包名 · 仓库：https://github.com/LingNc/DocVision

## 环境要求

| 依赖 | 用途 | 缺了怎么办 |
|---|---|---|
| 本包 `kernel/bin/dvagent` | 全部操作入口（预编译 linux/amd64；自行编译：`cd kernel && go build -o bin/dvagent ./cmd/dvagent`，需 Go≥1.25） | 直接可用 |
| poppler-utils（pdfinfo/pdftotext/pdftoppm） | 拆书/覆盖率/渲染 | apt install poppler-utils |
| TeX Live（pdflatex） | LaTeX 档编译判据（md 档 不需要） | apt install texlive-xetex |
| MinerU | 扫描页 OCR：`export MINerU_TOKEN=<token>` 后走 API 模式（零安装）；或本地 pip install magic-pdf[full] | 见 doctor 提示 |
| mmdc（可选） | mermaid 校验（没有则跳过记 note） | npm i -g @mermaid-js/mermaid-cli |

## 快速开始（30 秒）

```bash
D=./kernel/bin/dvagent                      # 全部操作都经它（CLI）
$D doctor                                   # ① 体检：缺什么怎么装一页说清
mkdir -p ws/book && cp 你的书.pdf ws/book/  # ② 建工作区放书
$D -workspace ws register demo ws/book && $D -workspace ws activate demo
# ③ PREP 无人值守段（幂等，可中断续跑）
$D -workspace ws prep-split demo ws/book/书.pdf 100
$D -workspace ws prep-mineru-api demo ws/book/source/parts/*.pdf   # 需 MINERU_TOKEN
$D -workspace ws prep-organize demo && $D -workspace ws prep-index demo
```

之后进入 **agent 工作段**：agent 读本包 `agent/README.md`（冷启动仪式+铁律）与 `agent/manual-*.md`（七类任务手册），用 `claim` 领任务、`submit` 交卷，判据内核强制。收尾二选一：

```bash
$D -workspace ws img2text-assemble demo chapters/book.md out/book.md   # md 档：合成干净 md
$D -workspace ws deliver demo                                            # LaTeX 档：装配交付
```

完整流程（含每个 agent 该干什么）见 **`docs/05-first-run.md`**；设计判据见 `docs/01-04`。

## 目录

- `SKILL.md`（本文件：入口与启动）· `PACKAGE-INFO.md`（署名与依赖页）
- `agent/` 宿主 agent 手册：README=冷启动+铁律；manual-*.md=七类任务做法
- `docs/` 设计文档：01 架构 · 02 任务协议 · 03 接口 · 04 迁移台账 · 05 首秀手册
- `kernel/` dvagent CLI 源码（`bin/` 预编译二进制；`go build ./cmd/dvagent` 可重编）

## 遇到问题（必读）

**禁止绕行**：任何环节失败不许用模型自身能力顶替/伪造产物（见 `docs/07-troubleshoot.md`）——
停止该环节 → 查明根因 → 向用户汇报 → 最小修复。日志脱敏、issue 格式、
密钥申请流程都在 07 文档。用户档案（token 等）存 `data/profile.env`（600 权限，见 `docs/06-profile-config.md`）。

## 纪律（对 agent 与使用者同效）

- 验收门槛程序强制（矢量图重合度默认 17%、accept 必交够次数），不由模型判断；
- 阈值只允许用户显式调整：`dvagent config-set <项目> figure.min_overlap 0.3`（agent 无权）；
- 状态全部落盘：中断/换会话/换机器，跑一遍 `status` 即续上，零记忆负担。

