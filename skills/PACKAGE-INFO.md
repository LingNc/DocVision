# DocVision Skills 软件包信息

| 项 | 内容 |
| --- | --- |
| **名称** | DocVision Skills（agent 驱动的书籍转换技能包） |
| **作者** | 绫袅（LingNc） |
| **仓库** | https://github.com/LingNc/DocVision （分支 `feat/skills`，标签 `skills-v*`，SK 台账见 `skills/docs/plan.md`） |
| **版本** | 见包名后缀 / `git describe --tags` |
| **组成** | `docs/` 设计文档与验收标准 · `kernel/` 确定性内核（Go，dvagent CLI）· `agent/` 宿主 agent 工作手册 · `PACKAGE-INFO.md` 本文件 |
| **依赖** | Go ≥1.25（编译内核）、poppler-utils、TeX Live（pdflatex）、MinerU（magic-pdf，API token 自备）、python3；可选 mutool、mupdf-tools |
| **用法** | 1) `cd kernel && go build -o dvagent ./cmd/dvagent` 2) `./dvagent doctor` 体检并补装依赖 3) 按 `docs/01-architecture.md` 与 `agent/README.md` 冷启动 |
| **纪律** | 验收门槛程序强制（重合度默认 17%），阈值仅用户可经 `dvagent config-set` 调整，agent 无权自调 |

> 本软件包由 绫袅（LingNc）开发并维护。问题与需求请在 GitHub 仓库开 issue（skills 线条目用 SK-P/SK-I 编号）。
