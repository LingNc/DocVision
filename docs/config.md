# 主要配置说明

> 返回 [README](../README.md)（文档索引见 README「文档」一节）。


| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `mineru.max_pages_per_part` | 单个文件分片最大页数（PDF 和 DOCX） | 200 |
| `mineru.max_size_mb` | 单个文件分片最大大小（MB） | 200 |
| `mineru.max_concurrent` | MinerU API 并发数 | 5 |
| `mineru.upload_timeout` | 上传空闲超时（秒） | 300 |
| `mineru.log_poll_interval` | 控制台日志输出间隔（秒） | 3 |
| `mineru.progress_threshold` | 页数变化阈值，达到此值立即刷新输出 | 80 |

## 工具配置（tools:）

各流程共用的工具参数（get_more_context / image_context / 校验工具）：

| 键 | 说明 | 默认 |
| --- | --- | --- |
| `tools.context.initial_up` / `initial_down` | 图片初始上下文行数（上/下） | 10 / 5 |
| `tools.context.max_up` / `max_down` | 上下文可扩展上限（行） | 50 / 50 |
| `tools.context.max_calls` | 单图最大上下文扩展请求次数 | 5 |
| `tools.mermaid.validation` | Mermaid 校验模式（off/auto/strict） | auto |
| `tools.mermaid.command` | Mermaid CLI 命令 | mmdc |
| `tools.mermaid.fix_attempts` | Mermaid 独立修正次数（0=无限，受安全上限保护） | 3 |
| `tools.mermaid.timeout` | 单次 Mermaid 校验超时（秒） | 30 |
| `tools.bash.sandbox` | 会话 bash 是否用 bubblewrap 内核沙箱（沙箱里只有本会话的挂载表；缺 bwrap 自动回退并记警告） | true |
| `tools.bash.max_output` | 会话 bash 工具返回给模型的字符上限 | 5000 |
| `tools.python.enabled` | 是否给会话 bash 提供 Python 环境 | true |
| `tools.python.mode` | 环境形态：`system` / `venv` / `conda`（venv 会按 `env_dir` 自动创建） | system |
| `tools.python.interpreter` | 解释器路径（留空 = `env_dir/bin/python3` 或 PATH 里的 `python3`） | 空 |
| `tools.python.env_dir` | venv 目录（不存在则宿主侧自动创建）或 conda 环境前缀 | 空 |
| `tools.python.conda_env` | conda 环境名（`env_dir` 为空时用它；留空或 `base`/`root` = conda 的 **base** 环境） | 空 |
| `tools.python.pip_index_url` | 宿主侧自动安装用的 PyPI 镜像（如 `https://pypi.tuna.tsinghua.edu.cn/simple`）；留空 = 用宿主 `pip.conf` | 空 |
| `tools.python.packages` | 启动前确保可导入的模块（缺则宿主侧安装；`PIL`→`Pillow`、`fitz`→`PyMuPDF` 等自动换算成 PyPI 包名） | 空 |
| `tools.python.auto_install` | 会话 bash 报 `ModuleNotFoundError` 时由宿主侧自动 `pip install` 并提示重试 | true |
| `tools.python.install_timeout` | 单次 `pip install` 超时（秒） | 300 |

## 配置项参考（含默认值）

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `models.text.request_body` | 注入 API 请求体的额外参数（如 enable_thinking），原样合并到请求体顶层 | 见示例 |
| `options.log_level` | 日志等级：info / debug / trace（debug 记请求响应摘要与最终内容，trace 再加流式分片） | info |
| `models.text.stream` | 是否流式接收（SSE）；不写即 true。厂商不支持时自动回退一次非流式 | true |
| `models.text.api_stream_idle_timeout` | 流式模式下两个数据块之间的最大间隔（秒），超时判定卡住；0 取 `api_timeout` | 0 |
| `models.text.thinking` | 顶层 `thinking` 对象：`type` 取 `enabled`/`disabled`/`adaptive`（GLM-4.5+/DeepSeek/Qwen）。**不要放进 `extra_body`**；非法取值启动即报错（disable/enable 等笔误自动纠正并告警） | 未设置 |
| `models.<name>.thinking.type` | 同上；写错会让该模型**每一次请求**都被服务端拒掉（现场一次 `type: disable` 让 8/8 张图只保留原图） | 未设置 |
| `models.text.reasoning_effort` | 顶层 `reasoning_effort`（GLM-5.2+，thinking 开启时生效）：max/xhigh/high/medium/low/minimal/none | 未设置 |
| `models.text.api_timeout` | **非流式**整次请求（连接+读取）总超时（秒）；流式模式改用 idle 超时；所有模型条目可覆盖，留空继承 text | 400 |
| `models.text.api_connect_timeout` | 连接/首字节等待超时（秒） | 60 |
| `models.text.api_max_retries` | 通用 API 错误（5xx 等 transient）重试上限；**4xx 客户端错误不重试**（400 请求体不合法 / 401 密钥 / 403 权限 / 404 模型名 / 422 参数），一次即报错，408 与 429 走各自通道 | 3 |
| `models.text.api_max_retries` | 非限流错误重试次数（指数退避 2s/4s/8s…封顶 30s） | 3 |
| `models.text.rate_limit_retries` | 429 限流重试上限（指数退避封顶 60s）。**只管真限流**：余额不足/配额类错误直接判定并立即返回，不消耗这里的重试次数 | 20 |
| `img2text.concurrency` | AI 图片转文本并发数（原 `options.concurrency`，两处等价） | 10 |
| `img2text.format_fix_attempts` | 格式修复重试次数（0 禁用，1 表示重试一次）；**只能写在 `img2text` 下**，`options.format_fix_attempts` 已不生效 | 1 |
| `img2text.max_tokens` | img2text 单次请求最大输出（`max_tokens`）（原 `options.max_tokens`，两处等价） | 65536 |
| `img2text.temperature` | img2text 采样温度 | 0.10 |
| `img2text.output_language` | AI 描述输出语言（"Chinese"/"English"） | Chinese |
| `models.text` | **必填**：img2text 等基础流程的默认模型（base_url/api_key/model/request_body/stream/thinking…） | - |
| `models.<name>` | 每个专用 AI 的独立配置，空字段继承 `models.text`；`max_tokens`/`temperature` 作为该模型未指定时的兜底；GLM 可用 `tool_stream: true`（工具参数随流返回）与 `thinking.clear_thinking: false`（保留式思考） | - |
| `models.<条目>.extends` | 命名基座：本条目继承另一个 `models:` 条目的键（加载期在 YAML 节点层合并）；也接受**条目名列表** `extends: [a, b]`——按书写顺序合并、后面的基座覆盖前面的，条目自己的键覆盖所有基座（YAML 重复键本身会报错，多重继承必须用列表）。语义见下方「`extends`：命名基座」一节 | 未设置 |
| `models.<条目>.price.*` | 单价四键。条目**一项费率都没写**时继承 `models.text` 的费率；写了任意一项就用自己这一份。两个条目发往**同一个 wire 模型名**却配了不同单价 → 加载期报错 | 继承 `models.text`（或全 0 = 未配置） |
| `latex.level` | LaTeX 档位（2=图片矢量化，1=全书转换） | 2 |
| `latex.sessions.{drawing,style,chapter,convert,checker}.*.max_tool_rounds` | 会话工具轮数上限；**代码无内置默认，省略即 0 = 不限制**（随附模板示例写 128）。checker 是唯一例外：不写就是 **50**（不继承 convert 的轮数），显式写 -1 才是无限 | 0（不限制；checker 50） |
| `latex.sessions.*.context_limit` | 会话上下文窗口（tokens），达到阈值自动 AI 压缩 | 131072 |
| `latex.sessions.*.max_tokens` | 该会话**单次请求最大输出**（不含厂商单独计费的思维链预算）：drawing 16384、style 内置 32768、其余 16384（随附模板示例把 drawing/style 写成 131072） | 16384 / style 32768 |
| `latex.sessions.*.temperature` | 该会话采样温度（同 `models.<name>.temperature`，会话配置优先） | 未设置 |
| `latex.sessions.*.compaction_at` | 触发自动压缩的窗口占用比例（0-1），不写默认 0.85（所有会话一致，不是只继承 drawing） | 0.85 |
| `latex.concurrency` | 档位1/档位2 的会话级并发（classify / 档位2 process / 档位1 逐章 convert / style-fix 共用；style、chapters、assemble、终审是单会话） | 3 |
| `latex.insert_image_description` | 档位2 专用：raster 保留原图时是否嵌入 AI 解释文本（`[Image]( … )`）；档位1 不受它控制（总是生成解释块） | false |
| `latex.chapter_granularity` | 档位1 章节拆分粒度：small=按小节拆分 / large=按大章整体拆分 | small |
| `latex.keep_temp_dirs` | 保留 `<项目>/work/temp/` 下的临时工作目录（拆章沙箱、逐章转换/修复工作区 `conv_<章>`、矢量图工作区） | false |
| `latex.keep_session_records` | 保留成功会话的 JSONL 转录（与上一项独立）；debug 日志下两项都必定保留 | false |
| `latex.compile.engine` | 编译引擎（pdflatex / xelatex / lualatex） | xelatex |
| `latex.compile.timeout` | 单次编译超时（秒） | 120 |
| `latex.compile.raster_command` | PDF 栅格化工具 | pdftoppm |
| `latex.compile.raster_dpi` | 矢量图 PDF 栅格化为 PNG 的分辨率（SVG 不可用时的回退产物）（随附模板示例写 220） | 110 |
| `latex.compile.max_fix_rounds` | 编译→核对→修复整体循环上限（修复会话与终审会话共用）（随附模板示例写 40） | 8 |
| `latex.compile.final_review` | 全书编译成功后是否再跑终审会话（汇总/整理/重编译）；未通过只告警、照常交付 | true |
| `verify.enabled` | AI 核对开关标记（默认关闭；核对只能显式运行 `docvision verify`） | false |
| `verify.verifier_model` | `models:` 中支持图像的校验模型代号 | "verifier" |
| `tools.view.image_max` | 同一张图片文件的 `view_image` 软预算（只提醒不拦截；负值不限） | 30 |
| `tools.view.pdf_max` | 同一个 PDF **每页**的 `view_pdf` 软预算（同上） | 25 |
| `tools.view.warn_ratio` | 用满该比例起提醒"仅剩 N 次" | 0.7 |
| `latex.sessions.*.tool_rounds_warn_ratio` | 工具轮次提醒起点比例 | 0.7 |
| `latex.sessions.*.tool_rounds_grace` | 达到 `max_tool_rounds` 后仍可用的额外轮次（负值=无宽限） | 20 |
| `latex.sessions.*.prune_tool_chars` | 本地裁剪阈值：超过该长度的工具结果裁成"前半 + 后 1/4"（负值=不裁剪） | 4096 |
| `latex.sessions.*.keep_images` | 本地裁剪时保留的最近图片数（负值=全保留） | 3 |
| `verify.concurrency` | 核对并发数 | 2 |
| `verify.report_file` | 核对报告文件名 | verify_report.md |
| `latex.figure_check.enabled` | 逐图校验开关（**默认关闭**）：矢量图提交后再用能看图的模型把"重画图 ↔ 原图"比一遍 | false |
| `latex.figure_check.model` | 校验用的 `models:` 条目（必须能看图；缺该条目时退回作图模型并在日志说明） | "verifier" |
| `latex.figure_check.max_rounds` | 校验轮次上限：第 1 轮校验 + 最多这么多轮"打回同一作图会话修正" | 2 |
| `models.<条目>.price.input` | 该模型**未命中前缀缓存**的输入单价（元/百万 tokens；`0` = 未配置，费用报告整块不显示） | 0 |
| `models.<条目>.price.cached` | 命中前缀缓存的输入单价（不填/`0` 按 `input` 计——宁可高估也不凭空打折） | 0 |
| `models.<条目>.price.output` | 输出单价（含思考 tokens，与厂商口径一致） | 0 |
| `models.<条目>.price.currency` | 金额前缀符号 | "¥"（仅在配了价格时补默认） |
| `config_version` | 配置模板版本：与当前程序期望值（本版为 **10**）不一致时启动只提示、不报错；`docvision setup` 会把它列为待修项。新增配置块时同步 bump | 10 |
| `preview.enabled` | 跑 `docvision latex` 时自动启动**会话预览服务**（只读；`docvision sessions --serve` 的常驻版），启动日志里给出确切 URL | false |
| `preview.host` | 预览服务监听地址（`0.0.0.0` 会让局域网可访问；转录含全书内容，默认只本机） | "127.0.0.1" |
| `preview.port` | 预览服务端口。配置里写 `0` 与不写都一样取默认 8848（配置分不出「没写」和「写了 0」）；要由内核挑一个空闲端口用命令行 `--port 0`，启动打印的就是实际绑定到的地址。`docvision sessions --serve` 默认也用这里；`--addr` / `--port` 优先于本项 | 8848 |
| `estimate.method` | **本地估算**的图片计量方法：`pixels`（按尺寸折算）/ `fixed`（每张固定值）/ `none`（本地按 0 计） | "pixels" |
| `estimate.tokens` | `method: fixed` 时每张的固定值；`method: pixels` 下**取不到尺寸**（未知格式/文件读不到）时也用它 | 1100 |
| `estimate.px_per_token` | `method: pixels` 的折算比例：单张图片 token ≈ `宽×高 / 本项` | 750 |
| `estimate.min_tokens` | `method: pixels` 的单张下限 | 85 |
| `estimate.max_tokens` | `method: pixels` 的单张上限 | 4096 |
| `models.<条目>.image_tokens.method` | **该模型单独一套**计量方法（覆盖 `estimate.method`），其余键同理逐键覆盖 `estimate.*`；没写的键继承全局、没配 `image_tokens` 的条目完全跟全局走 | 继承 `estimate` |
| `models.<条目>.image_tokens.tokens` | 该模型的每张固定值 / pixels 模式下取不到尺寸时的值 | 继承 `estimate.tokens` |
| `models.<条目>.image_tokens.px_per_token` | 该模型的折算比例 | 继承 `estimate.px_per_token` |
| `models.<条目>.image_tokens.min_tokens` | 该模型的单张下限 | 继承 `estimate.min_tokens` |
| `models.<条目>.image_tokens.max_tokens` | 该模型的单张上限 | 继承 `estimate.max_tokens` |
| `paths.latex_output` | 档位2 输出根：每本书的工作区是 `<latex_output>/<项目名>/`（旧版单项目布局则沿用根目录本身） | ./finally_latex |
| `paths.latex_project` | 档位1 输出根：每本书的工作区是 `<latex_project>/<项目名>/`（旧版单项目布局则沿用根目录本身） | ./latex_project |
| `paths.fonts` | AI 字体目录：缺字体时按样式会话报告的清单手动放入，编译环境注入 `TEXINPUTS`/`OSFONTDIR`（无下载工具） | ./fonts |
| `img2text.model` | 基础流程模型（models: 注册表代号，默认 text） | text |
| `latex.checker_model` | 每章核对模型（独立小模型）。核对本身是一个极简会话：只读两个文件（章节 md + 提交的 .tex）与一个文件夹（分片），工具只有 read_file/grep/submit，默认 50 轮；调优里未配置的字段继承 convert（轮数除外） | "checker" |
| `latex.remove_watermark` | 水印处理：true 时样式/转换/核对 AI 会检测并排除水印 | false |
| `paths.logs_dir` | img2text 处理日志目录（`img2text_*.log` + `img2text_error_*.log`） | `./logs` |
| `paths.done_dir` | 分割完成后源文件被归档到的目录；空字符串或与 `input_dir` 相同会报错 | `<input_dir>/done` |

> **图片 token 的本地估算**：只影响本地估算（上下文压缩阈值、预览页里带 `≈` 的数字），厂商回执的 `prompt_tokens`/`completion_tokens` 一律照抄。每张图能算多少 token 取决于厂商，所以有三种计量方法，并且**可以按模型各选一套**：优先取 `models.<条目>.image_tokens.<键>`，没写的键取顶层 `estimate.<键>`，都没有才用代码默认值（条目名与 wire 模型 id 都能命中覆盖；`latex.drawing_model` 这类角色键只是指向条目，不参与计量）。默认值是折中而非普适：实测 `glm-5.3-flash-official` 上 2.88M 像素的页面渲染约 3697 token（≈780 px/token），而 `deepseek-v4.1-flash` 上大图饱和在 ≈1050 token/张——所以默认用 `pixels`（750/85/4096）先算准尺寸大图，按张固定计费的端点给该条目写 `method: fixed`，不关心本地预扣的写 `method: none`。

> `latex.remove_watermark` 开启后流程开始时先做一次水印检测：全览页渲染 + markdown 重复图片统计，结果缓存在本项目工作区的 `watermark_memory.json`（档位1 `<latex_project>/<项目名>/`，档位2 `<latex_output>/<项目名>/`）并作为工作记忆注入后续所有会话；水印图片引用直接剔除不再处理。

`estimate` 的四项参数只影响**本地估算**：上下文压缩阈值、以及会话预览页里带 `≈` 的数字（行内计数、轨迹表的计数列、详情栏的「图片 N 张 ≈ …」）。厂商回执的 `prompt_tokens` / `completion_tokens` 一律照抄，不受这里影响。三种方法的含义与各自用到的参数：

| `method` | 含义 | 用到的参数 |
| --- | --- | --- |
| `pixels`（默认） | 每张 ≈ `宽×高 / px_per_token`，夹在 `[min_tokens, max_tokens]` | `px_per_token`、`min_tokens`、`max_tokens`；取不到尺寸时用 `tokens` |
| `fixed` | 每张固定 `tokens` 个，不看尺寸（按张计费的端点） | `tokens` |
| `none` | 本地按 0 计（厂商数字里仍含图片 token，只是本地不预扣） | 无 |

## `extends`：命名基座

`models.<条目>.extends: <另一条目名>` 让这个条目只写**差异键**，其余键取被指向的条目：

```yaml
models:
  gateway-a:
    model: "glm-5.3-flash"
    temperature: 1
    thinking: {type: enabled, clear_thinking: false}
    price: {input: 1, cached: 0.1, output: 4}
  drawing:
    extends: "gateway-a"     # 只写差异；其余（key/单价/thinking/请求控制）全部继承
    tool_stream: true
  style:
    extends: "drawing"       # 基座自己也可以 extends 别人（链式），逐级往上找
```

**多重基座**：`extends` 也接受**条目名列表**——按书写顺序合并、后面的基座覆盖
前面的，条目自己的键覆盖所有基座（YAML 不允许同一键写两次，多重继承必须用列表）；
列表里的基座同样可以自身 extends（链式 + 多重混用）：

```yaml
models:
  vision-heavy:
    extends: ["gateway-a", "heavy"]   # heavy 的键覆盖 gateway-a 的，自己再补差异
    max_tokens: 32768
```

**合并语义**（在 YAML 节点层、解码之前按条目键合并）：

| 情况 | 结果 |
| --- | --- |
| 子条目没写这个键 | 取基座的值 |
| 子条目写了（任意值） | 用子条目的值整体替换；**显式 `0` / `false` / `""` 也算覆盖** |
| 子条目写了 `null` | 显式清空，基座那个键不生效 |
| 值是列表 | 整体替换，**不拼接** |
| 值是 map（`request_body`/`thinking` 这种整块） | 整块替换，**不做半合并**（要对某个嵌套键覆盖就整块写出来） |

判据是「键是否出现」，所以在解码后的值类型上做不了（`int` 分不清「没写」与「写了 0」）。两套规则不要混淆：

- **`extends` 看键是否出现**：`api_timeout: 0` 写在子条目里就是覆盖（该条目这一项真的是 0）。
- **条目没写的字段继承 `models.text` 时看零值**（`ResolveModel`）：`0`/空字符串等于「没写」，继续往 `models.text` 与代码默认走。

报错（加载期即失败，`docvision setup` 会列出）：指向不存在的条目、自引用、成环（`a → b → c → a`，打印完整链）、`extends` 写成空值。未知键则单独出声：`docvision setup` 把它当错误拦下并指出真正写着那一行的条目（`models.base.<键>`），运行期至少打印一行「未知配置键」告警——包括被 `extends` 继承进另一个条目的那份。

## 未知键与同名不同价

- **未知键告警**：`LoadConfig` 对未知键不严格（旧写法仍要能加载），但一个拼错的键不会有任何症状，所以启动时按路径逐条打印 `⚠ <路径>: 未知配置键（第 N 行…）`。严格检查只在 `docvision setup`。
- **同一 wire 模型名不同单价 → 加载期报错**：每处价格查询都按**厂商报的模型名**索引（转录里记的是它），两个条目发往同一个 wire 名却配了不同费率时，费用报告只能随机取一条。同名同价（甚至都不配价）是正常写法。

完整配置见 `config.example.yaml`。
