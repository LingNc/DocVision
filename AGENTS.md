# Agent工作目录

你是Agent。该文件是一个说明，说明当前文件夹是一个对于Agent的工作区，这里是Agent所管理的范围，对于编辑修改这里的文件和文件夹无需通过任何审核，但是必须遵守以下的规定。

## 该文件夹目的
方便管理所有的用户无指定目录时，Agent产出和中间的临时文件等，并且整理和分类用户的不同任务在不同的文件夹保证不会混乱。该目录下每个文件夹都是一个项目（也可以包含嵌套项目）内部包含项目自己的说明，无非必要请勿在根目录（当前文件所在目录）下防止无用文件。

## 维护内容

- **DocVision（本目录主体）**：PDF→Markdown 自动化工作流（Go，模块 `mineru-tools`，代码在 `go/`；运行目录 `/home/share/samba-share/PDF2MD`，config.yaml 与 docvision 二进制随代码更新）。现状要点（**这里是唯一需要日常维护的部分**；每一批的详细改动、真实缺陷与证据数字在 `docs/agent-batch-history.md`）：
  - **命令面**：`docvision latex`（档位1 = 图片矢量化 + 全书 LaTeX；档位2 = 干净 Markdown）、`docvision workflow`、`docvision img2text`、`docvision verify`、`docvision sessions`（转录预览页）、`docvision setup`。
  - **档位1 全流程**：files → classify → process(images) → style（样式虚拟工作区，产出 cls + manual.md + example.tex）→ chapters（划分）→ convert（逐章并发；每章一个私有工作视图与编译 scratch；checker 小模型核对，问题打回**同一个**会话最多 3 轮）→ feedback（定向样式修复）→ assemble（修复会话 + 终审会话）→ 整棵 build 树交付 `out/`。档位2 产物是**零注释**的干净 Markdown（矢量图嵌 SVG）。
  - **多项目布局**：输出根下每本书一个工作区 `<root>/<项目名>/`（`--project` 显式指定；旧单项目布局自动沿用不迁移）。
  - **AI 会话基础设施** `go/internal/session`：可分离工具、上下文窗口、JSONL 转录（append-only；`t=meta` 行只记录**永不回放**；续跑只回放最后一次压缩检查点之后）、**两级压缩**（先零成本本地裁剪、再 AI 摘要；阈值检查在**每次请求之前**）、工具轮次软限制、前缀缓存亲和（稳定 `user` + `[cache-probe]`）。
  - **提示词集中管理** `go/internal/prompts/templates/*.md`：`prompts.Must/Render` + 注册表（`Vars`/`MustMention`/`RetiredNames`）+ 4 条守护测试；**禁止**再往会话源码里内联大段提示词，**禁止**出现退役工具名。
  - **虚拟工作区**：每会话一张挂载表（`Runner.sessionMounts` 是唯一来源），`work:`（可写，逐章会话是私有视图 `work/views/chapter_<章>`）、`project:`（最小只读视图）、`source:`（原书 PDF 视图）、`build:`（编译 scratch）；会话 bash 在 **bubblewrap** 里看到同一棵窄树（无网络），`/tmp` 是**会话私有持久目录**；`tools.python` 给沙箱内提供 Python 环境（`mode: system|venv|conda`，conda 默认 base，宿主侧自动 pip 安装）。
  - **看图/尺度**：`view_pdf`（自己的产物与原书同一工具，回执给 mm）、`view_image`（原图 + 实测印刷尺寸 mm）、`list_source_pages`/`doc_search`（原书页索引）；看图有软预算（`tools.view.*`，只提醒不拦截）。
  - **编译**：通用 `compile {path, engine, passes, bib, shell_escape, args, timeout}`（引擎 latexmk/xelatex/pdflatex/lualatex 可现场选），章节 `compile` 编译 wrapper 并同样接受 `engine/passes/args`；成功只报产物名 + 页数 + 一句 "See it with view_pdf"。
  - **详细批次历史（第一批～第二十七批，原文）** → **`docs/agent-batch-history.md`**。查设计理由、历史事故根因时读它；AGENTS.md 只写现状，不再堆批次流水。

**发布线与 CHANGELOG**：v1.5.0 测试线，当前标签 **`v1.5.0-beta.3`**（第十八～二十七批尚未打标签，以 `v1.5.0-beta.3-NN-g<sha>` 形式随二进制发布）。各标签**实际**覆盖范围（按提交可达性判定，非按文档批次号）：`v1.2.0`＝LaTeX 首版（两档位/会话基础设施/模型注册表/verify，未单独打标签），`v1.3.0-beta`＝自助化 + 样式虚拟工作区 + 字体 + 每章 checker + view_page 按需渲染，`v1.4.0-beta`＝img2text 按类型嵌入 + TikZ 编译校验（仅一个提交），`v1.5.0-beta.1`＝嵌入类型细分/styled 标记/风格统一/preflight/chapter_granularity/水印工作记忆/跨页图表拼接/doc_search/tools 块/配置 v2 等，`v1.5.0-beta.2`＝第一～三批（流式接收+thinking、日志三级 trace、SVG 多后端、编译警告反馈、多工具图片轮 400、文本图只提取文本、预览图、classify 起始行），`v1.5.0-beta.3`＝**第四～十七批**（JSONL 转录起，至 part 级页码定位/临时目录与保留开关/逐章私有工作视图）。CHANGELOG 已按标签分节（`v1.2.0`（未打标签，附注说明）/`v1.3.0-beta`/`v1.4.0-beta`/`v1.5.0-beta.1`/`.2`/`.3` + 历史各节）：条目按**引入该条的提交**归属到对应标签，节日期取标签创建日期（脚本用 `git log -S` 逐条回溯 + 提交区间映射生成，可复核）；`v1.5.0-beta.2` 及更早标签保持不动。运行目录：`/home/share/samba-share/PDF2MD`（config.yaml 与 docvision 二进制随代码更新）。


## 规则

- **文档同步（强制）**：功能／工具／配置项／CLI 的增删改，必须在同一批提交里同步三处——① `README.md`（用户手册）② 相关命令的 `--help` 文本（`go/cmd/docvision/*.go` 的 Short/Long/Flags）③ `CHANGELOG.md`。**删除或改名**工具/配置项时，必须全局 grep 旧名并清理（曾漏掉：`view_page`、`list_images`、`install_font`、`compile_preview`、`preview.png`、`read_md`、`mermaid_validation`、`workflow --step latex`、`options.format_fix_attempts`——README/帮助文本里长期残留旧行为）。
- **CHANGELOG 规则**：按标签分节，节日期取**标签创建日期**；每条条目归到「引入它的提交」所在的标签（用 `git log -S <条目片段> -- CHANGELOG.md` 回溯，别凭印象归批）；无标签的版本另立小节并注明。条目里的 Added/Changed/Fixed 分类要与内容相符（新功能不要塞进 Fixed）。
- **配置项一致性**：新增配置项同步 `config.go` 默认值 + `internal/config/default.yaml` + `config.example.yaml`；模板注释不得描述已移除的行为；README 配置表的"默认值"列写**代码默认值**，与随附模板示例不同处要注明（如 `raster_dpi`：代码 110 / 示例 220）。
- **发布线纪律**：`v1.5.0-beta.N` 为测试线。发布后**不要移动既有标签**；仅在"标签创建当天 + 未推送 + 内容确实是笔误"时才允许 `git tag -f`，并在提交信息与回复里说明旧→新指向。其余情况一律打新标签。
- **提示词维护**：内置提示词一律写在 `go/internal/prompts/templates/*.md`，通过 `prompts.Must/Render` 取用；**不要**再往会话源码里内联大段提示词。新增/改名工具时，同步该会话模板并更新注册表的 `MustMention`；模板里禁止出现退役工具名（黑名单见 `RetiredNames`）。搬迁或修改提示词必须**抽取或逐字校对**，禁止凭记忆重写。
- **文档审计**：大改动后（或用户要求时）做一次只读审计——`README.md` + 全部 `--help` 文本 + 配置模板 vs 代码事实，产出"过期／缺失／仍准确"三段清单，再定点修补；禁止整体重写文档。


## Agent 自举说明

**本文档作为项目全局状态机与综述，能够防止代码库膨胀后上下文丢失，确保后续 Agent 辅助开发时拥有完整的记忆和设计初衷。**

### 维护责任
- 每次会话开始前，应优先读取 AGENTS.md 了解项目状态。
- 当用户需要变更该文件夹部分功能时应修改当前文件(ADGENTS.md)加入用户的新规则或者修改现有规定。
- 保持 AGENTS.md 与其余部分的描述一致性。
- 有新的项目加入时应及时修改"维护内容"板块，对当前文件夹中的项目分条列项说明大概功能。
- **批次记录写档案**：每批改动追加到 `docs/agent-batch-history.md`（原文、含真实缺陷与证据），AGENTS.md 只更新"现状要点"里受影响的条目——AGENTS.md 有指令预算上限，不许再堆批次流水。
- 当用户有新的规则或者自发的发现需要进行新的规则约束时应及时编辑该文件的"规则板块"。

### 必须做的事
- 当当前文件被你变更时，必须回复：“根目录AGENTS.md已修改。”。