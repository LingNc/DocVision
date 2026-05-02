# DocVision - 文视

PDF 转 Markdown 自动化工作流。通过 MinerU API 解析 PDF，再用 AI 将文档中的图片转换为文本描述，最终输出结构化的 Markdown 文件。

## 工作流程

```
PDF 文件 -> 分割 -> MinerU API 解析 -> 整理文件 -> AI 图片转文本 -> 日志分析
```

共 5 个步骤，由 `workflow.py` 统一调度：

1. **split_pdfs** - 将大 PDF 按页数限制分割（MinerU 限制单次最多 200 页）
2. **mineru_api** - 调用 MinerU API 解析 PDF，支持并发、断点续传、上传进度显示
3. **organize_files** - 整理解析结果，合并分片，按引用收集图片
4. **img2text** - 用 AI 模型识别图片内容并转为文本，支持并发和上下文增量扩展
5. **analyze** - 分析处理日志，统计耗时、成功率、进度

## 快速开始

### 安装依赖

```bash
pip install -r requirements.txt
```

### 配置

```bash
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，填入：

- MinerU API token（从 [mineru.net](https://mineru.net) 获取）
- AI 模型 API 地址和密钥（兼容 OpenAI 接口的模型）

### 运行

```bash
# 运行完整工作流
python workflow.py

# 仅运行某个步骤
python workflow.py --step split
python workflow.py --step mineru
python workflow.py --step organize
python workflow.py --step img2text
python workflow.py --step analyze

# 指定配置文件
python workflow.py --config my.yaml
```

将 PDF 文件放入 `files/` 目录，运行 `python workflow.py` 即可。

## 主要配置说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `mineru.max_pages_per_part` | 单个 PDF 分片最大页数 | 200 |
| `mineru.max_concurrent` | MinerU API 并发数 | 5 |
| `mineru.upload_timeout` | 上传空闲超时（秒） | 300 |
| `options.concurrency` | AI 图片转文本并发数 | 10 |
| `options.max_retries` | AI 请求更多上下文的最大轮数 | 5 |
| `options.max_context_lines_up` | 图片上方初始上下文行数 | 10 |
| `options.max_context_lines_down` | 图片下方初始上下文行数 | 5 |

完整配置见 `config.example.yaml`。

## 各脚本独立使用

每个脚本都可以独立运行：

```bash
# 分割 PDF
python python/split_pdfs.py input.pdf --max-pages 200 --output-dir split_pdfs

# 调用 MinerU API
python python/mineru_api.py
python python/mineru_api.py --file test.pdf

# 整理文件
python python/organize_files.py

# AI 图片转文本
python python/img2text.py
python python/img2text.py --test                  # 测试模式，随机 10 张
python python/img2text.py --test --number 5       # 随机 5 张
python python/img2text.py --test --seed 42        # 固定随机种子

# 分析日志
python python/analyze.py
python python/analyze.py --progress               # 仅显示进度摘要
python python/analyze.py --all                    # 汇总所有历史日志
```

## 目录结构

```
files/              源 PDF 文件
split_pdfs/         分割后的 PDF
mineru_output/      MinerU API 返回的解析结果
output/             合并后的 Markdown 和引用的图片
output/temp/        各分片原始 Markdown
finally/            AI 处理后的最终 Markdown
finally/progress_items/  AI 处理进度记录（断点续传）
```

## 许可证

MIT License - Copyright (c) 2026 LingNc
