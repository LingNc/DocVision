# Changelog

## [Unreleased]

### Added

#### LaTeX 输出（两档位）
- `docvision latex`：档位2（默认）图片矢量化——分类 AI 逐图标记 text/vector/raster；text 纯内容嵌入文本流（去除 [AI]/[IMG_TYPE] 标记）；vector 由作图 AI 会话写 TikZ，自动编译+栅格化 PNG 视觉核对+确认提交，编译 PDF 以矢量图嵌入；raster 保留原图（可选嵌入 AI 解释文本），矢量失败自动回退并告警
- `docvision latex --level 1`：全书 LaTeX——样式分析 AI（图像裁剪放大/读 md 工具）产出 cls+结构化使用手册+案例并自动试编译；章节划分 AI（grep + 最小 bash 沙箱，虚拟单文件系统）行号划分并全覆盖校验；转换 AI 并发逐章转 .tex（虚拟文件目录，只读他人/只写自己）；汇总多文件 .tex 编译全书 PDF（修复会话兜底）+ 单文件 standalone.tex
- AI 会话基础设施 `internal/session`：可复用多轮会话引擎，可分离工具注册（compile_preview/submit/grep/bash 沙箱/受限读写），可配置上下文窗口（默认 128K，支持 64K/256K 等），达到阈值自动 AI 压缩会话历史（保留关键决策与成果，丢弃草稿/工具噪音）
- 模型注册表 `models:`：每个专用 AI（classifier/drawing/style/chapter/convert/verifier）独立配置 base_url/api_key/model/request_body，空字段继承顶层 ai
- `docvision verify`：AI 核对每张图与嵌入内容（含 TikZ 渲染预览对比），输出问题与修改意见报告（`verify.enabled` 默认关闭，先上日程）
- 断点续传：档位2逐图进度（progress_items），档位1逐阶段进度（progress.json）
- latex 会话日志独立落盘 logs/latex_*.log

### Fixed
- `--config` 传入相对子目录路径时 chdir 后解析错位（改用绝对路径）


## [v1.1.0] - 2026-09-03

### Added
- `install` / `uninstall`：一键安装到 `/usr/local/bin`（无权限回退 `~/.local/bin`）并初始化 `~/.docvision/`；卸载保留配置
- `setup`：自动选择编辑器（`$DOCVISION_EDITOR`/`$EDITOR` 优先，回退 vim → nano → vi）编辑生效配置；保存时严格校验（YAML 语法、未知配置项、类型、必填项、占位符），不通过可回车重编或 q 退出
- 配置查找顺序：`--config` > 当前目录 `config.yaml` > `~/.docvision/config.yaml`（缺失自动从模板创建），任意目录可直接使用
- `workflow <path>` 临时模式：处理任意位置的 PDF/DOCX 文件或目录，中间产物存于 `~/.docvision/jobs/<名称>-<时间戳>/`，仅将最终 `.md` 输出到源文件所在目录

### Changed
- GitHub Actions release 工作流支持 `workflow_dispatch` 手动补发任意标签

## [v1.0.1] - 2026-09-03

### Added
- `analyze -r/--round`：按轮次选择日志（0=最新，1=上一次…，支持 `1-3` 范围）
- `analyze -l/--last`：按时间范围选择日志（`+2d`、`2026Y9M1D-2026Y9M2D`、`2026-09-01_15:30-` 等）
- analyze 默认输出“本轮写入 finally 的文件及成功率统计”

### Fixed
- 日志解析改为按“结束时间 − 耗时 ≈ 开始时间”匹配结果归属，不再依赖线程号顺序：旧日志中线程号复用导致的假“未完成”统计全部纠正（实测 1958 任务：1941 成功 / 17 失败 / 0 未完成）
- img2text 线程号改为独占分配（tid 池），修复任务复用进行中的 tid 导致日志交错
- AI 工具调用消息的 tool_call 类型规范化，修复部分 API 网关报 HTTP 400 (Input should be 'function')
- `finally` 最终文件跳过内容无变化的重复写入


## [v1.0.0] - 2026-08-11

### Added
- 新增"大变"里程碑：split 后的源文件归档到 `done/` 目录，整体工作流闭环
- 支持在同一个工具会话内连续修复 Mermaid，保留上下文并允许后续继续调用工具
- Mermaid 修复次数支持 `0` 表示无限制（受代码内安全上限保护）

### Changed
- Mermaid 默认修复次数从 1 提升到 3
- Mermaid 修复提示词清洗报错栈：剔除 puppeteer 内部堆栈以减少噪音，超长堆栈截断到 16KB

### Fixed
- Mermaid 错误分类与修复提示词加固：常见语法/渲染错误分类更精准，提示词更明确修复方向
- organize 图片路径重写改为严格幂等，修复包含特殊字符的 subject 目录图片缺失问题

### Refactored
- 指纹边车文件后缀从 `.md.fp` 改为 `.fp`，避免与 Markdown 文件混淆

## [v0.3.0] - 2026-08-10

### Added
- DOCX 分割清单（manifest）持久化快速路径：未变化的输入直接复用历史清单，跳过重复计算
- 处理日志与最终输出分离：`img2text` 主日志和错误日志落到独立的 `logs/` 目录，便于归档和分析

### Fixed
- img2text 进度恢复去重：断点续传场景下进度条目不再重复写入
- 主日志选择逻辑统一：analyze / splitlog 在多个候选日志中按规则稳定选择主日志

### Changed
- Python 遗留实现归档到 `legacy/python/`，仅作历史参考，不再维护
- 文档对齐当前 Go 实现：README / 配置示例与 Go 行为同步，移除过时说明

## [v0.2.0] - 2026-08-10

### Added
- Mermaid 输出验证与修复：识别无效 Mermaid 块并触发修复流程
- `docvision init` 时询问是否通过 npm 安装 Mermaid CLI，便于开箱启用 Mermaid 验证
- 任务识别与日志分析优化建议：在分析阶段给出更具操作性的改进提示
- PDF 分割清单（manifest）持久化快速路径：未变化的输入直接复用历史清单，跳过重复计算
- organize 步骤支持增量、幂等的整理运行：重复执行不会产生重复产物

### Fixed
- MinerU 上传超时与路径处理加固：上传空闲超时更稳定，长路径 / 异常路径场景下不再中断
- analyze 工作流使用当前 img2text 日志，而非陈旧日志

## [v0.1.2] - 2026-06-24

### Added
- 工作流步骤开始时显示初始进度提示，提升用户体验

### Fixed
- 进度显示：在工作流步骤开始时显示初始进度，避免用户长时间看不到任何输出

## [v0.1.1] - 2026-06-23

### Added
- DOCX 分割支持：使用与 PDF 相同的页数和大小限制
- DOCX 转 PDF 再分割：通过 LibreOffice 转换为 PDF 获取精确页数后再按页分割
- 无需分割时直接复制 DOCX 文件，避免不必要的 XML 操作

### Fixed
- DOCX 页数检测：从段落启发式改为读取 `docProps/app.xml` 中的实际页数
- DOCX 段落启发式页数估算：从每页 20 段降低为 10 段，更贴近实际排版
- PDF 文件查找：空目录警告改为打印日志并返回 nil，不再中断流程

## [v0.1.0] - 2026-06-04

### Added

#### Go 重写
- 全新 Go 实现，与 Python 版本功能完全一致
- 单二进制文件，无运行时依赖
- 交叉编译支持：Linux/macOS/Windows（amd64 + arm64）
- 8 个子命令：`workflow`、`split`、`mineru`、`organize`、`img2text`、`analyze`、`splitlog`、`init`
- `docvision init` 生成配置模板（`default.yaml` 嵌入二进制）
- Makefile：`make build`、`make release`、`make test`、`make clean`
- DOCX 分割支持：按段落分组，使用与 PDF 相同的页数和大小限制

#### 核心功能
- PDF 分割：按页数 + 文件大小二进制搜索，跳过已分割文件，`--force` 强制重分
- MinerU API：分块上传、空闲超时检测、指数退避轮询、任务断点续传、并发处理
- 文件整理：自动合并分片、按主题组织图片子目录、重写 Markdown 图片路径
- AI 图片转文本：OpenAI 兼容接口、工具调用增量上下文扩展、格式自修复、并发工作池
- 日志分析：状态机解析、8 类错误分类、百分位统计、CSV 导出、进度摘要
- 配置：`request_body` 字段直接注入 API 请求体，支持不同厂商的思考模式配置

#### 工作流模式
- `docvision workflow` 串联全部 5 步，`--step` 单步执行
- img2text 步骤安静模式：控制台仅显示进度百分比，完整日志写入文件

#### CI/CD
- GitHub Actions：推送 `v*.0` 标签自动测试、交叉编译、创建 Release

### Changed
- 配置格式：`ai.enable_thinking` 改为 `ai.request_body`，支持通用请求体注入
- Python `img2text.py` 同步支持新 `request_body` 配置格式（向后兼容 `enable_thinking`）
