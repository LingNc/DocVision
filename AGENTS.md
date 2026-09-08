# Agent工作目录

你是Agent。该文件是一个说明，说明当前文件夹是一个对于Agent的工作区，这里是Agent所管理的范围，对于编辑修改这里的文件和文件夹无需通过任何审核，但是必须遵守以下的规定。

## 该文件夹目的
方便管理所有的用户无指定目录时，Agent产出和中间的临时文件等，并且整理和分类用户的不同任务在不同的文件夹保证不会混乱。该目录下每个文件夹都是一个项目（也可以包含嵌套项目）内部包含项目自己的说明，无非必要请勿在根目录（当前文件所在目录）下防止无用文件。

## 维护内容

- **DocVision（本目录主体）**：PDF→Markdown 自动化工作流（Go）。v1.2 起新增 LaTeX 输出两档位（`docvision latex`，图片矢量化 / 全书 LaTeX）、AI 会话基础设施（`go/internal/session`：可分离工具、上下文窗口与自动压缩）、模型注册表（`config.yaml models:`）、AI 核对（`verify.enabled` 默认关闭）。v1.3+ latex 自带完整工作流（files/ 输入 → 自动跳过前置 → 按档位处理 → 日志分析；原始扫描页 view_page 按需渲染；样式虚拟工作区；fonts 字体管理；每章 checker 小模型核对；转换会话只读检索原始文档（doc_search 块索引 + view_page 原页渲染，缓存共享）），workflow 已不再包含 latex/verify 步骤。v1.4 起 img2text 嵌入按类型分流（文本/公式/表格直接嵌入，mermaid/tikz 代码块，视觉类型 [Image]( 描述 )），不再使用 <!-- IMG -->/[AI] 包装。v1.4+ latex 日志带 img2text 同格式 ▶ START/✓ DONE/✗ FAILED 会话标记（analyze/splitlog 通用）；矢量回退记 ERROR 并在 markdown 就地 `<!-- DOCVISION-ERROR -->` 标注、状态 fallback 下次自动重试；档位1 转换会话 submit 必带工作汇报（实时写 work/reports/<章>.md，固定格式），多数章节报 cls/手册问题时打回原样式会话（上下文持久化于 work/style_session.json）修正后全新上下文重新并发转换（上限 2 轮）。开发约定：分步开发并适时提交 git；新增配置项必须同步 `config.go` 默认值、`default.yaml`/`config.example.yaml` 模板与 `setup` 严格校验。


## 规则


## Agent 自举说明

**本文档作为项目全局状态机与综述，能够防止代码库膨胀后上下文丢失，确保后续 Agent 辅助开发时拥有完整的记忆和设计初衷。**

### 维护责任
- 每次会话开始前，应优先读取 AGENTS.md 了解项目状态。
- 当用户需要变更该文件夹部分功能时应修改当前文件(ADGENTS.md)加入用户的新规则或者修改现有规定。
- 保持 AGENTS.md 与其余部分的描述一致性。
- 有新的项目加入时应及时修改"维护内容"板块，对当前文件夹中的项目分条列项说明大概功能。
- 当用户有新的规则或者自发的发现需要进行新的规则约束时应及时编辑该文件的"规则板块"。

### 必须做的事
- 当当前文件被你变更时，必须回复：“根目录AGENTS.md已修改。”。