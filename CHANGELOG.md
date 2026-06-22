# Changelog

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