# DocVision - 文视

PDF 转 Markdown 自动化工作流。通过 MinerU API 解析 PDF，再用 AI 将文档中的图片转换为文本描述，最终输出结构化的 Markdown 文件。

提供 **Go** 实现（推荐）；历史 **Python** 实现已归档至 `legacy/python/`，仅供参考且不再维护。

## 工作流程

```
PDF 文件 -> 分割 -> MinerU API 解析 -> 整理文件 -> AI 图片转文本 -> 日志分析
```

`docvision workflow` 共 5 个步骤：

1. **split** - 将大文件按页数和文件大小限制分割（PDF 和 DOCX，MinerU 限制单次最多 200 页）
2. **mineru** - 调用 MinerU API 解析文件，支持并发、断点续传、上传进度显示
3. **organize** - 整理解析结果，合并分片，按引用收集图片到按主题子目录
4. **img2text** - 用 AI 模型识别图片内容并转为文本，支持并发和上下文增量扩展（工具调用）
5. **analyze** - 分析处理日志（img2text 与 latex 日志均支持），统计耗时、成功率、进度

另有两条**独立运行**的命令，不属于 workflow 步骤（`workflow --step` 不接受这两个名字）：

- **latex** - LaTeX 输出（图片矢量化 / 全书转换，见 [LaTeX 输出](docs/latex.md)；自带前置流程与日志分析）
- **verify** - AI 核对图片与嵌入内容（默认关闭，`verify.enabled`；只能显式运行 `docvision verify`）

## 环境要求

- **Go 1.25+**（见 `go/go.mod`；模块在 `go/`）；历史 Python 版本已归档，不再维护
- **MinerU API token**（[mineru.net](https://mineru.net)），在配置里填 `mineru.token`
- **AI 模型**：任何 OpenAI 兼容的 Chat Completions 端点、Anthropic Messages API 端点或 OpenAI Responses API 端点（`models.*.api: openai|anthropic|responses`），在 `models:` 注册表里配置
- **可选**：[Mermaid CLI](https://github.com/mermaid-js/mermaid-cli)（校验 AI 输出的 Mermaid 图，需 Node.js/npm）、TeX Live（`docvision latex` 编译与图片矢量化需要 xelatex 等引擎）

## 安装与构建

```bash
cd go
make build

# 一键安装到 /usr/local/bin（无权限时回退 ~/.local/bin），
# 并初始化 ~/.docvision/config.yaml；之后任意目录可用 docvision
./build/docvision install

# 编辑生效的配置（当前目录 config.yaml 优先，否则 ~/.docvision/config.yaml），
# 自动选择 vim/nano；保存时校验语法与配置项，有问题可回车重编或 q 退出
docvision setup

# 初始化配置模板（会询问是否安装 Mermaid CLI）
./build/docvision init
```

交叉编译：`make release`（输出到 `go/release/`，共 5 个产物：Linux/macOS 各 amd64+arm64，Windows 仅 amd64）。

Mermaid 校验默认 `tools.mermaid.validation: auto`（找不到 `mmdc` 时提示安装并跳过本次校验）；`strict` 明确报错，`off` 禁用。装法：`npm install -g @mermaid-js/mermaid-cli`（`docvision init` 会询问，确认后才执行，不会自动装 Node.js/npm）。

## 快速开始

### 1. 配置

```bash
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，至少填两处：

- `mineru.token`：MinerU API token（从 [mineru.net](https://mineru.net) 获取）
- `models.text`：基础流程的默认模型（**必填**，`base_url` / `api_key` / `model`）；其余专用 AI（classifier/drawing/style/chapter/convert/checker/verifier）可单独配置，空字段自动继承 `models.text`

```yaml
mineru:
  token: "你的 MinerU token"
paths:
  output_dir: "./output"          # 解析结果（合并后的 .md 与图片）
  images_dir: "./output/images"
models:
  text:
    base_url: "https://api.example.com/v1"
    api_key: "sk-..."
    model: "your-model"
  drawing:                        # 其余角色只写差异，空字段继承 models.text
    extends: "text"               # 也可指向另一个条目：命名基座（详见 docs/config.md）
    model: "your-vision-model"
```

配置写在 `config_version: 11`（版本不符会提示更新配置文件）；**每一项配置的含义、默认值与相关行为见 [配置参考](docs/config.md)**，完整模板见 `config.example.yaml`。

### 2. 跑起来

```bash
# 处理任意位置的 PDF/DOCX（单文件或整个目录）——临时模式：
# 中间文件保存在 ~/.docvision/jobs/，最终 .md 输出到源文件所在目录
docvision workflow /path/to/paper.pdf
docvision workflow /path/to/pdf-dir/

# 项目模式（老用法不变）：在项目目录里放 config.yaml + files/，然后
cd 项目目录 && docvision workflow

# 从原始文档直接到 LaTeX（自动补跑 分割→MinerU→整理 前置流程）
docvision latex --project 我的书 /path/to/book.pdf

# 只看 AI 会话做了什么（转录渲染成可浏览页面：左栏按项目→流程阶段分组）
docvision sessions

# 只看这次跑了多少 token、命中多少缓存、花了多少钱（按阶段统计 + 每图/每页均摊）
docvision sessions --cost

# 指定配置文件（默认当前目录 config.yaml，其次 ~/.docvision/config.yaml）
docvision workflow -c my.yaml
```

## 命令一览

| 命令 | 作用 |
| --- | --- |
| `docvision workflow` | 运行完整流水线，或 `--step` 指定单步（split\|mineru\|organize\|img2text\|analyze） |
| `docvision split` | 分割 PDF/DOCX（单文件或 `--all` 目录模式） |
| `docvision mineru` | 调用 MinerU API 解析文件 |
| `docvision organize` | 整理解析结果 |
| `docvision img2text` | AI 图片转文本（`--test` 测试模式） |
| `docvision latex` | LaTeX 输出（可直接传 PDF/DOCX 自动补前置流程，或传 md 名只处理指定文件；跑完打印按阶段 AI 用量与费用，可配 `latex.figure_check` 开逐图校验、`preview.enabled` 边跑边看会话） |
| `docvision verify` | AI 核对输出与原图（默认关闭；只能显式运行，不参与自动流程） |
| `docvision sessions` | 会话预览：把 AI 会话转录渲染成可浏览页面（静态导出 / 本地实时服务；目录与端口默认取配置，行内计数可切 `token`（本地估算，带 ≈）/`字符`，`--list` 的提示词列同口径（`--unit char` 切字符），详情栏为厂商实测指标；`--cost` 只出费用报告） |
| `docvision analyze` | 分析日志（`--progress` 仅进度，`--all` 汇总历史，`--logfile` 指定日志） |
| `docvision splitlog` | 按线程 ID 拆分日志（`--logfile` / `--output-dir`） |
| `docvision init` | 生成配置模板 |
| `docvision install` | 安装到系统路径（`/usr/local/bin`，无权限时回退 `~/.local/bin`）并初始化 `~/.docvision` |
| `docvision setup` | 编辑配置文件，保存时校验语法与配置项 |
| `docvision uninstall` | 从系统路径移除已安装的 docvision |

每个命令的完整选项、嵌入格式（img2text / 档位2）、`analyze` 与 `sessions` 的细节见 **[命令与输出细节](docs/commands.md)**；每个命令的 `--help` 也是同一份说明。

## 输出与目录

- 解析产物：`paths.output_dir`（合并后的 `.md` + 按主题分目录的 `images/`）
- LaTeX：档位1 每本书一个工作区 `latex_project/<项目名>/`，档位2 为 `finally_latex/<项目名>/`；目录里有什么、每一步产出什么见 [LaTeX 输出](docs/latex.md)

## 文档

| 文档 | 内容 |
| --- | --- |
| [docs/commands.md](docs/commands.md) | 全部命令与参数、img2text/档位2 嵌入格式、`analyze` 选项、`sessions` 预览页与指标 |
| [docs/config.md](docs/config.md) | 配置项全表（`mineru` / `paths` / `models` / `latex` / `tools` / `sessions` / `preview` / `estimate` / `verify`）与默认值 |
| [docs/latex.md](docs/latex.md) | LaTeX 档位1/档位2 全流程、`latex` 用法与阶段控制、项目目录布局、原书检索工具、AI 核对 |
| [docs/sessions.md](docs/sessions.md) | 会话基础设施（上下文窗口/压缩/转录）、提示词管理、会话沙箱与挂载表、调试日志 |
| [docs/dev.md](docs/dev.md) | 代码目录结构、CI/CD、历史 Python 实现 |
| [docs/agent-batch-history.md](docs/agent-batch-history.md) | 每一批改动的原始记录（含真实缺陷、证据数字与设计理由） |

## CI/CD

推送 `v*.*.*` 标签时 GitHub Actions 自动运行测试、交叉编译 5 个平台产物并创建 GitHub Release（只有推标签会触发，没有手动入口）：不带 `-` 的标签发正式版，带 `-` 的标签（如 `v1.5.0-beta.4`）发**预发布**（同样带产物，只是不会被标成 Latest）。只推 master 不打标签不会触发构建；细节见 [开发与代码结构](docs/dev.md)。

## 许可证

MIT License - Copyright (c) 2026 LingNc
