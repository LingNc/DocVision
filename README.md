# DocVision - 文视

PDF 转 Markdown 自动化工作流。通过 MinerU API 解析 PDF，再用 AI 将文档中的图片转换为文本描述，最终输出结构化的 Markdown 文件。

提供 **Go** 实现（推荐）；历史 **Python** 实现已归档至 `legacy/python/`，仅供参考且不再维护。

## 工作流程

```
PDF 文件 -> 分割 -> MinerU API 解析 -> 整理文件 -> AI 图片转文本 -> 日志分析
```

共 5 个步骤：

1. **split** - 将大文件按页数和文件大小限制分割（PDF 和 DOCX，MinerU 限制单次最多 200 页）
2. **mineru** - 调用 MinerU API 解析文件，支持并发、断点续传、上传进度显示
3. **organize** - 整理解析结果，合并分片，按引用收集图片到按主题子目录
4. **img2text** - 用 AI 模型识别图片内容并转为文本，支持并发和上下文增量扩展（工具调用）
5. **analyze** - 分析处理日志，统计耗时、成功率、进度

## 快速开始

### 配置

```bash
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，填入：

- MinerU API token（从 [mineru.net](https://mineru.net) 获取）
- AI 模型 API 地址和密钥（兼容 OpenAI 接口的模型）

### Go 版本（推荐）

```bash
cd go
make build

# 初始化配置模板；会询问是否安装 Mermaid CLI
./build/docvision init

# 运行完整工作流
./build/docvision workflow

# 仅运行某个步骤
./build/docvision split --all
./build/docvision mineru
./build/docvision organize
./build/docvision img2text
./build/docvision analyze

# 指定配置文件
./build/docvision workflow -c my.yaml
```

交叉编译：

```bash
make release   # 输出到 go/release/（Linux/macOS/Windows，amd64+arm64）
```

### Mermaid 验证工具（推荐安装）

如果 AI 输出 Mermaid 图，推荐安装 Mermaid CLI：

```bash
# 需要先安装 Node.js/npm
npm install -g @mermaid-js/mermaid-cli
```

`docvision init` 生成配置文件后会询问是否通过 npm 安装 Mermaid CLI；只有确认后才执行 `npm install -g @mermaid-js/mermaid-cli`，不会自动安装 Node.js/npm。

- `mermaid_validation: auto`：找不到 `mmdc` 时给出安装提示，并跳过本次验证；
- `mermaid_validation: strict`：找不到 `mmdc` 时明确报错，需安装后再继续；
- `mermaid_validation: off`：禁用 Mermaid 验证。

### Python 版本

历史 Python 实现已归档至 `legacy/python/`，不再维护，建议使用 Go 版本。

将 PDF 或 DOCX 文件放入 `files/` 目录，运行工作流即可。

## Go 命令行说明

```
docvision workflow    运行完整流水线（默认），或 --step 指定单步
docvision split       分割 PDF/DOCX（单文件或 --all 目录模式）
docvision mineru      调用 MinerU API 解析文件
docvision organize    整理解析结果
docvision img2text    AI 图片转文本（--test 测试模式）
docvision analyze     分析日志（--progress 仅进度，--all 汇总历史）
docvision splitlog    按线程 ID 拆分日志
docvision init        生成配置模板
```

### img2text 测试模式

```bash
./build/docvision img2text --test                  # 随机 10 张
./build/docvision img2text --test --number 5       # 随机 5 张
./build/docvision img2text --test --seed 42        # 固定随机种子
```

### analyze 选项

```bash
./build/docvision analyze                          # 分析最新日志
./build/docvision analyze --progress               # 仅显示进度摘要
./build/docvision analyze --all                    # 汇总所有历史日志
./build/docvision analyze --threads                # 显示线程详细统计
./build/docvision analyze --percentiles 90,95,99   # 自定义百分位
./build/docvision analyze -o report.csv            # 导出 CSV
# analyze 默认即输出“本轮写入 finally 的文件及成功率统计”
```

## Python 脚本独立使用

历史 Python 脚本已归档至 `legacy/python/`，独立使用方式请参阅该目录下的 `README.md`。

## 主要配置说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `mineru.max_pages_per_part` | 单个文件分片最大页数（PDF 和 DOCX） | 200 |
| `mineru.max_size_mb` | 单个文件分片最大大小（MB） | 200 |
| `mineru.max_concurrent` | MinerU API 并发数 | 5 |
| `mineru.upload_timeout` | 上传空闲超时（秒） | 300 |
| `mineru.log_poll_interval` | 控制台日志输出间隔（秒） | 3 |
| `mineru.progress_threshold` | 页数变化阈值，达到此值立即刷新输出 | 80 |
| `ai.request_body` | 注入 API 请求体的额外参数（如 enable_thinking） | 见示例 |
| `options.concurrency` | AI 图片转文本并发数 | 10 |
| `options.max_retries` | AI 请求更多上下文的最大轮数 | 5 |
| `options.max_context_lines_up` | 图片上方初始上下文行数 | 10 |
| `options.max_context_lines_down` | 图片下方初始上下文行数 | 5 |
| `options.api_connect_timeout` | API 连接超时（秒） | 60 |
| `options.api_max_retries` | 非限流错误的 API 重试次数 | 3 |
| `options.rate_limit_retries` | 限流错误重试次数（0 表示无限次，由代码设置上限） | 0 |
| `options.format_fix_attempts` | 格式修复重试次数（0 禁用，1 表示重试一次） | 1 |
| `options.mermaid_validation` | Mermaid 验证模式（off/auto/strict） | auto |
| `options.mermaid_command` | Mermaid CLI 命令 | mmdc |
| `options.mermaid_fix_attempts` | Mermaid 独立修正次数（0 表示无限次，受代码内安全上限保护） | 3 |
| `options.mermaid_timeout` | 单个 Mermaid 验证超时（秒） | 30 |
| `options.max_tokens` | API 调用最大 token 数 | 65536 |
| `paths.logs_dir` | img2text 处理日志目录（`img2text_*.log` + `img2text_error_*.log`） | `./logs` |
| `paths.done_dir` | 分割完成后源文件被归档到的目录；空字符串或与 `input_dir` 相同会报错 | `<input_dir>/done` |

完整配置见 `config.example.yaml`。

## 目录结构

```
files/                  源 PDF/Office/图片文件
files/done/             分割完成后归档的源文件（成功 split 后从 files/ 移入）
split_files/            分割后的 PDF/DOCX
mineru_output/          MinerU API 返回的解析结果
output/                 合并后的 Markdown 和引用的图片
output/images/{主题}/   按主题组织的图片
finally/                AI 处理后的最终 Markdown
finally/progress_items/ AI 处理进度记录（断点续传）
logs/                   img2text 处理日志（img2text_*.log + img2text_error_*.log）
```

> `files/done/` 在 SplitAll 模式下自动维护：每次 split 成功的源文件会被 `os.Rename` 到这里；DOCX 直通文件（页数低于阈值）保留在 `files/`，等下次评估。
> `--force` 会在 split 前把 `done/` 中同名的源文件移回 `files/` 再处理。
> 旧的 `*.pdf.done` / `*.docx.done` 标记会在 SplitAll 开始时被迁移到 `done/` 并去掉后缀。
> `os.Rename` 跨文件系统会失败（EXDEV），此时仅打印 warning，不会中断 split；如需跨盘归档请在 `paths.done_dir` 选择同盘路径。
> `logs/` 是 T7 新增目录，专门放 `img2text` 处理期间生成的主日志和错误日志。
> `finally/` 仍保存最终 Markdown 和 `progress_items/` 断点续传记录；
> 分析 / 拆分工具默认从 `logs/` 读取，并回退到 `finally/` 以兼容旧日志。

## CI/CD

推送 `v*.*.*` 标签时，GitHub Actions 自动：

1. 运行测试（`go test -race ./...`）
2. 交叉编译 5 个平台二进制
3. 创建 GitHub Release 并上传产物

```bash
git tag v0.1.2
git push origin v0.1.2
```

## 许可证

MIT License - Copyright (c) 2026 LingNc
