# DocVision - 文视

PDF 转 Markdown 自动化工作流。通过 MinerU API 解析 PDF，再用 AI 将文档中的图片转换为文本描述，最终输出结构化的 Markdown 文件。

提供 **Python** 和 **Go** 两种实现，功能完全一致。

## 工作流程

```
PDF 文件 -> 分割 -> MinerU API 解析 -> 整理文件 -> AI 图片转文本 -> 日志分析
```

共 5 个步骤：

1. **split** - 将大 PDF 按页数和文件大小限制分割（MinerU 限制单次最多 200 页）
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

# 初始化配置模板
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

### Python 版本

```bash
pip install -r python/requirements.txt

# 运行完整工作流
python workflow.py

# 仅运行某个步骤
python workflow.py --step split
python workflow.py --step mineru
python workflow.py --step organize
python workflow.py --step img2text
python workflow.py --step analyze
```

将 PDF 文件放入 `files/` 目录，运行工作流即可。

## Go 命令行说明

```
docvision workflow    运行完整流水线（默认），或 --step 指定单步
docvision split       分割 PDF（单文件或 --all 目录模式）
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
```

## Python 脚本独立使用

```bash
# 分割 PDF
python python/split_pdfs.py input.pdf --max-pages 200 --output-dir split_files

# 调用 MinerU API
python python/mineru_api.py
python python/mineru_api.py --file test.pdf

# 整理文件
python python/organize_files.py

# AI 图片转文本
python python/img2text.py
python python/img2text.py --test --number 5 --seed 42

# 分析日志
python python/analyze.py --progress
python python/analyze.py --all -o report.csv

# 按线程拆分日志
python split_log.py
```

## 主要配置说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `mineru.max_pages_per_part` | 单个 PDF 分片最大页数 | 200 |
| `mineru.max_size_mb` | 单个 PDF 分片最大文件大小（MB） | 200 |
| `mineru.max_concurrent` | MinerU API 并发数 | 5 |
| `mineru.upload_timeout` | 上传空闲超时（秒） | 300 |
| `ai.request_body` | 注入 API 请求体的额外参数（如 enable_thinking） | 见示例 |
| `options.concurrency` | AI 图片转文本并发数 | 10 |
| `options.max_retries` | AI 请求更多上下文的最大轮数 | 5 |
| `options.max_context_lines_up` | 图片上方初始上下文行数 | 10 |
| `options.max_context_lines_down` | 图片下方初始上下文行数 | 5 |

完整配置见 `config.example.yaml`。

## 目录结构

```
files/                  源 PDF/Office/图片文件
split_files/            分割后的 PDF
mineru_output/          MinerU API 返回的解析结果
output/                 合并后的 Markdown 和引用的图片
output/images/{主题}/   按主题组织的图片
finally/                AI 处理后的最终 Markdown
finally/progress_items/ AI 处理进度记录（断点续传）
```

## CI/CD

推送 `v*.0` 标签时，GitHub Actions 自动：

1. 运行测试（`go test -race ./...`）
2. 交叉编译 5 个平台二进制
3. 创建 GitHub Release 并上传产物

```bash
git tag v1.0.0
git push origin v1.0.0
```

## 许可证

MIT License - Copyright (c) 2026 LingNc
