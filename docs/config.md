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
| `tools.mermaid.fix_attempts` | Mermaid 就地修复轮数：`-1`（默认/未写）= **首次校验失败直接升级修复会话**；`0` = 无限就地修复（受安全上限保护）；`N>0` = N 轮就地修复后再升级（随附模板示例写 2） | -1 |
| `tools.mermaid.timeout` | 单次 Mermaid 校验超时（秒） | 30 |
| `tools.mermaid.session_rounds` | 升级修复会话的提交/检查次数上限（两段合计；≤0 = 关闭升级会话，维持旧的"跳过、下轮重试"） | 32 |
| `tools.mermaid.session_errors` | 升级会话内编译错误累计上限，满了清上下文切备选模型（随附模板示例写 6） | 3 |
| `tools.mermaid.fallback_model` | 备选兜底模型（`models.<条目名>`；条目不存在或留空 = 原模型清上下文重来） | （空） |
**升级修复会话（P12）**：默认（T37，`fix_attempts` 未写或 `-1`）**首次校验失败即升级**
——不做就地修复轮；显式写 `N>0` 时先在同一对话里追加 N 轮修复消息、用尽后再升级。
升级时模型拿到一个**虚拟工作区**：出错的完整响应落在 `submit.md`，配 `write_file`（重写）、
`grep`（查 submit.md / compile_error.log）、`view_image`（看原图）、`submit`（跑 mmdc
检查）四个工具，能过语法检查即可。工作区在 `<finally>/progress_items/mermaid_fix/<图>/`，
submit.md、compile_error.log 与两段 JSONL 转录都保留，出错现场可诊断、下轮可续用。
编译错误累计到 `session_errors` 就**清上下文**、换 `fallback_model`（未配置则原模型）
从头再来；提交/检查次数两段合计受 `session_rounds` 约束。会话走标准会话基础设施
（JSONL 转录、工具轮次软上限、压缩），`analyze` 按文本日志的 `[ToolCall]` 行统计工具
调用，并在进度摘要后报告「启动 N 次 / 触发备选 M 次」。
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
| `tools.python.install_timeout` | 单次 `pip install` 超时（秒）（随附模板示例写 30000，即 8 小时 20 分——只在装大包/慢镜像时才需要；默认 300 秒） | 300 |

## 配置项参考（含默认值）

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `latex.dedupe_group_session_over` | 相似桶成员数超过该值时**该桶独占一个核验会话**；较小的桶可以互相合并进同一会话；`0` = 整池一个会话（旧行为） | 50 |
| `latex.dedupe_batch_images` | 一个核验会话最多处理多少张图（小桶合并时的容量上限） | 150 |
| `latex.dedupe_concurrency` | 同时跑几个桶核验会话。桶之间互不依赖（各自只看自己桶里的图、写自己的 `groups.json`），整理会话仍等它们全部结束；**未写该键时为 2**，`0`/`1` = 串行（旧行为）。每个会话都带着自己的图片历史，调高会成倍放大峰值内存与限流压力 | 2（键缺席时） |
| `latex.dedupe_align_min` | 组内合并的**居中重合度**门槛（0–1）：成员把内容框**居中贴进 S×S 软墨量画布**（S = 池内 P90(内容框长边) 夹在 24–512；**不缩放、不二值化**，格值 = 格内平均软墨量 `1−通道/255`），按 `latex.dedupe_align_shift`·S（默认 0.10）平移搜索，网格 `G` 由 32 逐级翻倍到放得进 S 的最细一级，**判定分取最细一级**的三通道平均 Σmin/Σmax；与组中心达标才共用一张成品，其余保持独立。该书 130 份不同内容实测（T138 定稿后复测）：同标记 100 张两两 4950 对 min 0.462 / 中位 0.610 / ≥0.60 2851 对 / ≥0.75 232 对；30 张不同内容 435 对 max 0.607。**未写该键时为 0.60** | 0.60（键缺席时） |
| `latex.dedupe_align_shift` | 居中重合比较时允许的平移上限，按内容框**边长比例**（0–0.5）。实测 0.10 已饱和（0.10/0.20/0.30/0.40/0.50 结论完全一致，只是比较次数从 49 涨到 1089），0 = 不搜平移（小图上会把同源图判散）。**未写该键时为 0.10** | 0.10（键缺席时） |
| `latex.dedupe_icon_auto` | 小图标**本地自动归组**的重合度门槛（0–1）：与组中心重合度达到它的图标直接合并、不进核验会话；0.60–0.75 仍逐张核验。只对矢量小图标生效（前景框 ≤200×200 像素），**0 = 关闭自动归组**。**未写该键时为 0.75** | 0.75（键缺席时） |
| `latex.dedupe_align_grid` | 逐级搜索**最细一级（判定用的那一级）**的格数：整数 = 固定格数（如 `128`）；`0.95S` = 内容框画布边长 S 的 0.95 倍（S 自适应；**推荐**，固定 32 会把"只差一个符号"的图平均成同一张）。级数从 32 起逐级翻倍到它为止 | `"0.95S"`（键缺席时） |
| `latex.dedupe_align_typical` | "典型成员"门槛（0–1）：与组中心重合度达到它的成员里，取内容框最大者当代表图 | 0.95（键缺席时） |
| `latex.dedupe_attach_min` | 未入桶的图**挂靠**已确认组的门槛（0–1）。挂靠只跟着组走、不改变组本身；**0 = 关闭挂靠** | 0.60（键缺席时） |
| `latex.dedupe_icon_max_area` | 小图标面积上限（像素，宽×高），超过它的图只走逐对相似度、不进重合度归并 | 20000（键缺席时） |
| `latex.dedupe_bucket_max` | 参与重合度归并的小图标最大张数，超过就退回逐对相似度并警告 | 400（键缺席时） |
| `latex.dedupe_model` | 档位1 classify 后整组相似图工具核验会话的模型注册表键；留空回退 `text`，须支持看图和工具调用；随附模板为 `dedupe`，继承 `text` | 空（回退 text） |
| `models.dedupe` | 随附模板的独立去重模型条目，可用 extends 指向 text 或自行指定视觉模型 | 未设置（模板继承 text） |
| `models.rescue` | 复盘会话（未提交收尾的复盘，P58）的模型条目；模板里就写在 `models.checker` 下面、`extends: checker`，想用更便宜的模型复盘就改这一条，或把 `latex.rescue.model` 指到别的条目。**会话限制不写在这里**（上下文/轮次/输出预算在 `latex.sessions.rescue`） | 模板预置（`extends: checker`） |
| `models.text.request_body` | 注入 API 请求体的额外参数（如 enable_thinking），原样合并到请求体顶层 | 见示例 |
| `models.text.api` | 线路协议：`openai`（默认，OpenAI 兼容 `/chat/completions`）、`anthropic`（Anthropic Messages API，`base_url` 指到 `/v1`，自动带 `x-api-key`/`anthropic-version` 头）或 `responses`（OpenAI Responses API，`/v1/responses`，gpt-5/o 系；固定 `store:false` 全量回传，加密思考链请求 `reasoning.encrypted_content` 回放——**加密思考约半小时级有效期**，长间隔续跑可能失效）。`anthropic` **只走非流式**，`responses` 与非流式同款解码但支持 SSE（`response.output_text.delta` 推进实时快照、`response.completed` 收账，网关忽略 `stream:true` 时自动兜底解析整段 JSON）；三者都把应答的缓存读计入命中率、stop_reason 翻译为 finish_reason | `openai` |
| `options.log_level` | 日志等级：info / debug / trace（debug 记请求响应摘要与最终内容，trace 再加流式分片） | info |
| `models.text.stream` | 是否流式接收（SSE）；不写即 true。厂商不支持时自动回退一次非流式 | true |
| `models.text.api_stream_idle_timeout` | 流式模式下两个数据块之间的最大间隔（秒），超时判定卡住；0 取 `api_timeout` | 0 |
| `models.text.thinking` | 顶层 `thinking` 对象：`type` 取 `enabled`/`disabled`/`adaptive`（GLM-4.5+/DeepSeek/Qwen）。**不要放进 `extra_body`**；非法取值启动即报错（disable/enable 等笔误自动纠正并告警） | 未设置 |
| `models.<name>.thinking.type` | 同上；写错会让该模型**每一次请求**都被服务端拒掉（现场一次 `type: disable` 让 8/8 张图只保留原图） | 未设置 |
| `models.text.reasoning_effort` | 顶层 `reasoning_effort`（GLM-5.2+，thinking 开启时生效）：max/xhigh/high/medium/low/minimal/none | 未设置 |
| `models.text.api_timeout` | **非流式**整次请求（连接+读取）总超时（秒）；流式模式改用 idle 超时；所有模型条目可覆盖，留空继承 text | 400 |
| `models.text.api_connect_timeout` | 连接/首字节等待超时（秒） | 60 |
| `models.text.api_max_retries` | 通用 API 错误（5xx 等 transient）重试上限；**4xx 客户端错误不重试**（400 请求体不合法 / 401 密钥 / 403 权限 / 404 模型名 / 422 参数），一次即报错，408 与 429 走各自通道 | 3 |
| `models.text.api_max_retries` | 非限流错误重试次数（指数退避 2s/4s/8s…封顶 30s）；**img2text 的退化空响应重试也用这个数**（T59，原样重发保前缀缓存） | 3 |
| `models.text.rate_limit_retries` | 429 限流重试上限（指数退避封顶 60s）。**只管真限流**：余额不足/配额类错误直接判定并立即返回，不消耗这里的重试次数 | 20 |
| `img2text.concurrency` | AI 图片转文本并发数（原 `options.concurrency`，两处等价） | 10 |
| `img2text.format_fix_attempts` | 格式修复重试次数（0 禁用，1 表示重试一次）；**只能写在 `img2text` 下**，`options.format_fix_attempts` 已不生效 | 1 |
| `img2text.page_marks` | md 产物注入分页页码标记（`[P. N]` 独立行 + 图片锚 `· P.N`；T69） | true |
| `img2text.page_marks_rewrite` | false = 已交付的 finally 文件保持原有风格（原本没页码的不回注成页码版，增量更新照写）；删掉 finally 结果重跑才按 `page_marks` 开关注入 | false |
| `img2text.max_tokens` | img2text 单次请求最大输出（`max_tokens`）（原 `options.max_tokens`，两处等价） | 65536 |
| `img2text.temperature` | img2text 采样温度 | 0.10 |
| `img2text.output_language` | AI 描述输出语言（"Chinese"/"English"） | Chinese |
| `models.text` | **必填**：img2text 等基础流程的默认模型（base_url/api_key/model/request_body/stream/thinking…） | - |
| `models.<name>` | 每个专用 AI 的独立配置，空字段继承 `models.text`；`max_tokens`/`temperature` 作为该模型未指定时的兜底；GLM 可用 `tool_stream: true`（工具参数随流返回）与 `thinking.clear_thinking: false`（保留式思考） | - |
| `models.<条目>.extends` | 命名基座：本条目继承另一个 `models:` 条目的键（加载期在 YAML 节点层合并）；也接受**条目名列表** `extends: [a, b]`——按书写顺序合并、后面的基座覆盖前面的，条目自己的键覆盖所有基座（YAML 重复键本身会报错，多重继承必须用列表）。语义见下方「`extends`：命名基座」一节 | 未设置 |
| `models.<条目>.context_limit` | 模型/平台上下文窗口（tokens）；0=模型侧不设本地上限。会话实际值与 `latex.sessions.<session>.context_limit` 取较小的正值 | 0（不限制） |
| `models.<条目>.max_tokens` | 模型/平台单次请求最大输出；0=模型侧不设上限。会话实际值与 `latex.sessions.<session>.max_tokens` 取较小的正值 | 0（不限制） |
| `models.<条目>.max_images` | 模型/平台单次请求允许携带的图片数；0=不限制。超过时会话在请求前把最旧图片替换成可重新查看的文字提示 | 0（不限制） |
| `models.<条目>.max_concurrency` | 该模型**同时在途的会话数**上限（T179）；0=不限制。键是**实际下发的模型名**：转换、核对、样式修复即便配成不同条目，只要最终落到同一个模型上就共用这一个额度（同模型的多条目取最小值）。阶段并发决定开几个 worker，这一项决定几个 worker 能同时跟同一个模型说话——想限制真正打到厂商那边的请求数就配它，而不是把两个阶段的上限相加 | 0（不限制） |
| `latex.level` | LaTeX 档位（2=图片矢量化，1=全书转换） | 2 |
| `latex.sessions.{drawing,style,chapter,convert,checker,dedupe}.*.max_tool_rounds` | 会话工具轮数上限；**代码无内置默认，省略即 0 = 不限制**（随附模板示例写 128）。checker 是唯一例外：不写就是 **50**（不继承 convert 的轮数，它是**最里层**的基数，往上每层按 `max_tool_rounds_per_level` 加宽，T204），显式写 -1 才是无限 | 0（不限制；checker 50） |
| `latex.sessions.checker.max_tool_rounds_per_level` | **核对会话按层加宽轮次（T204）**：每往上一层（离最深一层更远）加这么多轮——父节点的核对要通读整棵子树，最里层只管自己那一片。负数=不加宽（只想按倍数走时用它） | 20 |
| `latex.sessions.checker.max_tool_rounds_factor` | **核对会话按层翻倍（T204）**：轮次 = `max_tool_rounds × factor^层数 + per_level × 层数`（层数 = 最深一层 − 本层）。1=纯线性；2=每层翻倍（50→100→200→400）。上限 1000 轮兜底 | 1 |
| `latex.sessions.*.context_limit` | 会话上下文窗口（tokens）；0=会话侧不设上限；如果模型也配置正值，实际使用两者中较小值 | 0（不限制） |
| `latex.sessions.*.max_tokens` | 会话单次请求最大输出；0=会话侧不设上限；如果模型也配置正值，实际使用两者中较小值。**这是"思考 + 正文"的总预算**，不是正文长度——开着思考的模型配 8192 会把预算全花在思考上、正文为空（会话因此以空回复失败）；这类模型建议 **≥32768**。被截断且无正文时代码会翻倍重试一次（受窗口约束）并告警（随附模板示例写 drawing/style 131072、dedupe 65536、img2text 65536） | 16384（style 32768） |
| `latex.sessions.*.keep_images` | 本地历史裁剪策略，不是平台图片上限；上下文裁剪时保留最近多少条带图消息。平台上限请配置 `models.<name>.max_images` | 3 |
| `latex.sessions.*.temperature` | 该会话采样温度（同 `models.<name>.temperature`，会话配置优先） | 未设置 |
| `latex.sessions.*.compaction_at` | 触发自动压缩的窗口占用比例（0-1），不写默认 0.85（所有会话一致，不是只继承 drawing） | 0.85 |
| `latex.concurrency` | 档位1/档位2 的通用会话级并发（classify / 档位2 process / style-fix / 提交提醒等共用；style、chapters、assemble、终审是单会话）。也是下面两项的默认值 | 3 |
| `latex.convert_concurrency` | 章节转换的并发 worker 数（T179/T198）：**批次宽度**（同时开这么多章转换）与转换会话的槽位上限是同一个数；0=沿用 `latex.concurrency`。与核对自己一个槽位池，**不放在一起算**：转换在写 `.tex`、核对在读 PDF/原书页，混在一个池里会让核对排在一个慢转换后面 | 0（=concurrency） |
| `latex.checker_concurrency` | 章节核对会话的并发上限（T179）；0=沿用 `latex.concurrency`。核对会话比转换轻、读的是已有 PDF，通常可以开得比转换多；两个池子加起来超过某个模型的 `max_concurrency` 时，多出来的会话排队等模型槽位（启动日志会点名） | 0（=concurrency） |
| `latex.insert_image_description` | 档位2 专用：raster 保留原图时是否嵌入 AI 解释文本（`[Image]( … )`）；档位1 不受它控制（总是生成解释块） | false |
| `latex.style_fix.max_rounds` | 风格修复大循环上限（任何章节汇报样式问题即进入 整合→风格修复→影响评估→分块修复 循环；负数=关闭） | 3 |
| `latex.style_fix.integrate_model` | 问题整合会话（stylefix-integrate）单独用的模型（T65）；空 = 沿用 style 模型 | `integrate` |
| `latex.style_fix.stylefix_style_model` | 风格修复会话（stylefix_style_rN）独立模型（T70）；空 = 沿用 style 模型 | `stylefix-style` |
| `latex.style_fix.stylefix_assess_model` | 影响评估/分组会话（stylefix_assess_rN）独立模型（T70）；空 = 沿用 style 模型 | `stylefix-assess` |
| `latex.style_fix.stylefix_block_model` | 分块修复会话（stylefix-blk-rN-i）独立模型（T70）；空 = 沿用 convert 模型 | `stylefix-block` |
| `latex.style_fix.bookfix_model` | 构建修复会话（book_fix）独立模型（T70）；空 = 沿用 convert 模型 | `bookfix` |
| `latex.style_fix.final_review_model` | 终审会话（final_review）独立模型（T70）；空 = 沿用 convert 模型 | `final-review` |
| `latex.chapter_granularity` | 档位1 章节拆分粒度：large=大章整体 / medium=按本书自动给出最合理拆分（不拆太细也不太大块）/ small=按小节拆分 | medium |
| `latex.keep_temp_dirs` | 保留 `<项目>/work/temp/` 下的临时工作目录（拆章沙箱、逐章转换/修复工作区 `conv_<章>`、矢量图工作区） | false |
| `latex.keep_session_records` | 保留成功会话的 JSONL 转录（与上一项独立）；debug 日志下两项都必定保留 | true（旧版默认 false） |
| `latex.rescue.enabled` | 会话**非正常收尾**时先起一条只读的**复盘会话**看它的转录，得出 `retry`（还健康，照常提醒）/ `truncate`（已退化，截断到最后一个干净轮次继续）/ `fail`（救不回来）三种结论之一。三种触发共用这一条链路：① 跑完但没 submit；② **报错终止**（非致命错误——余额不足/无可用资源包这类致命错误不复盘，直接上报）；③ **声称"工具/工作区不可用"**、会话层纠正过一次之后仍然没 submit。false 时三种触发全部退回"只发提醒"的老路径（请求数与文件与引入开关之前逐字节相同） | true |
| `latex.rescue.max_per_attempt` | 同一次尝试最多复盘几次（防"复盘 → 又没交 → 再复盘"） | 2 |
| `latex.rescue.keep_records` | 复盘会话的转录是否落盘到 `work/sessions/rescue_*.jsonl`（默认只写一行审计记录 `work/rescue/log.jsonl`）；`--debug` 强制落盘 | false |
| `latex.rescue.model` | 复盘会话用的模型条目；空 = 跟随 `latex.checker_model`（模板预置了 `models.rescue`，`extends: checker`） | `"rescue"` |
| `latex.fix.enabled` | 子章节**修复会话**（P60）的总开关：打开后在细分父章节的转换会话里注册 `dispatch_fix`，并在提示词末尾追加"把问题子章节还回去"那一段；子章节转换+核对完、父会话整合之前，runner 先给它一次复查对话，按它写下的 `work/fix/<根章>/plan.json` 逐个子章节跑修复会话（复用该子章节已划好的工作区）并重核，然后把修过的产物刷新回父章节再整合。修复失败只警告、退回原来的整合流程。**默认关闭**：关着时工具面、提示词与请求数都与引入 P60 之前一致 | false |
| `latex.fix.max_rounds` | 一个子章节最多能拿几轮修复（P60）：转录里每条 `fix_target` 锚点算一轮；用完就**不再整章重开**——父章节核对再不过也保留这一版整合产物，未解决的问题照旧经 issues 库/报告向上传递，只记一行警告。0/负数也按 2 算，不是"无限轮" | 2 |
| `latex.fix.model` | 修复会话用的模型条目（P60）；空 = 继承 `latex.convert_model`。会话调优没有单独的块，沿用 `latex.sessions.convert` | `""` |
| `latex.sessions.rescue` | 复盘会话自己的会话调优；没写的字段继承 `sessions.checker`（输出预算与它同配置） | — |
| `latex.sessions.rescue.context_limit` | 复盘会话的上下文窗口（会话与模型两侧取较小值） | 131072 |
| `latex.sessions.rescue.max_tool_rounds` | 复盘会话自己的轮次上限（看几窗转录就该给结论） | 12 |
| `latex.sessions.rescue.max_tokens` | 复盘会话的输出预算；0 = 继承 checker 的配置 | 0 |
| `latex.style_pilot` | 样式试点（T97 重量档）：样式阶段后，选材会话拼一份代表全书特色内容的最小样例（md 行段 + 原书页），试转会话用样式包转换并对照原书核对，修订会话（新上下文）据此定稿 cls，然后才划分章节 | true |
| `latex.pilot_select_model` / `pilot_trial_model` / `pilot_revise_model` | 样式试点三个子会话的独立模型键（T108；`models:` 需注册对应条目，模板默认 `pilot-select` extends chapter、`pilot-trial` extends convert、`pilot-revise` extends style） | 空 = 回退 chapter / convert / style |
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
| `config_version` | 配置模板版本：与当前程序期望值（本版为 **24**）不一致时启动只提示、不报错；`docvision setup` 会把它列为待修项。新增配置块时同步 bump | 24 |
| `preview.enabled` | 跑 `docvision latex` 时自动启动**会话预览服务**（只读；`docvision sessions --serve` 的常驻版），启动日志里给出确切 URL | false |
| `preview.host` | 预览服务监听地址（`0.0.0.0` 会让局域网可访问；转录含全书内容，默认只本机） | "127.0.0.1" |
| `preview.port` | 预览服务端口。配置里写 `0` 与不写都一样取默认 8848（配置分不出「没写」和「写了 0」）；要由内核挑一个空闲端口用命令行 `--port 0`，启动打印的就是实际绑定到的地址。`docvision sessions --serve` 默认也用这里；`--addr` / `--port` 优先于本项 | 8848 |
| `preview.img2text` | T37：预览侧栏额外显示 **img2text 升级修复会话**（`<finally>/progress_items/mermaid_fix/<书>_<图>/` 的转录），按书分组、组内按图名稳定排序编号（`#N ·`）、走侧栏既有搜索；失败的修复工作区**默认保留**可见。img2text 产物在 latex 扫描根之外，本项是看到它们的唯一开关；P18 起 WebUI 有独立 img2text 板块（板块切换 + 每图进度状态）（随附模板示例写 true） | false |
| `preview.img2text_all` | 逐图分析也记录会话转录（`<finally>/progress_items/sessions/<md>/<图>.jsonl`，P18 起在 WebUI img2text 板块展示）；调试完不想堆文件可关。**升级修复会话总是记录、不受此键管** | true |
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
| `latex.checker_model` | 每章核对模型（独立小模型，须支持看图）。核对会话的只读视图是：章节 md、提交的 .tex、分片文件夹、本章编译产物 `<章>.pdf`、转换会话的 report/notes（当"说法"核实）、原书 `source:` part PDF；工具 read_file/grep/view_pdf/list_source_pages/doc_search/submit，默认 50 轮；调优里未配置的字段继承 convert（轮数除外） | "checker" |
| `latex.checker_rounds` | 一章**最多尝试几次**：checker 连续打回同一个转换会话若干轮仍不过，就作废产物、换一个全新会话重来；第 N 层递归（细分）每次 -1，**下限 3**，顶层默认 6。顶层次数与最大递归层数（3）不匹配（`checker_rounds - 3 < 3`）时启动告警——深层只会吃到下限 | 6 |
| `latex.checker_no_verdict_retries` | 核对**没给结论**时自动换新的核对会话再来几次（T233）。只重核、不重转：这一版产物留着、转换会话不重跑，换掉的只是那条已退化的核对会话。用完仍没结论就如实记「核对无结论」并排一条重核请求（产物不动）。**0/未写 = 6** | 6 |
| `latex.max_chapter_lines` | 一个转换会话"好好转完"的章节规模（章节 md 的行数）。超过它、又还没拆分过的节点，`submit` 会被打回并要求先 `request_chapter_split` 按语义拆成 2–16 段（子会话各转各的、各自过 checker，父会话只整合与核对）；已经递归到最深一层不再强制。首条消息会直接把本章行数告诉会话。**0 = 关闭这条规则** | 3000 |
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
| 值是 map（`price`/`request_body`/`thinking` 这种整块） | **逐子键递归深合并**：复写一条只覆盖那一条，没写的子键从基座继承（任意深度；`price: {input: 2}` 不会把基座的 `cached/output/currency` 打回零值） |

判据是「键是否出现」，所以在解码后的值类型上做不了（`int` 分不清「没写」与「写了 0」）。两套规则不要混淆：

- **`extends` 看键是否出现**：`api_timeout: 0` 写在子条目里就是覆盖（该条目这一项真的是 0）。
- **条目没写的字段继承 `models.text` 时看零值**（`ResolveModel`）：`0`/空字符串等于「没写」，继续往 `models.text` 与代码默认走。

报错（加载期即失败，`docvision setup` 会列出）：指向不存在的条目、自引用、成环（`a → b → c → a`，打印完整链）、`extends` 写成空值。未知键则单独出声：`docvision setup` 把它当错误拦下并指出真正写着那一行的条目（`models.base.<键>`），运行期至少打印一行「未知配置键」告警——包括被 `extends` 继承进另一个条目的那份。

## 未知键与同名不同价

- **未知键告警**：`LoadConfig` 对未知键不严格（旧写法仍要能加载），但一个拼错的键不会有任何症状，所以启动时按路径逐条打印 `⚠ <路径>: 未知配置键（第 N 行…）`。严格检查只在 `docvision setup`。
- **同一 wire 模型名不同单价 → 加载期报错**：每处价格查询都按**厂商报的模型名**索引（转录里记的是它），两个条目发往同一个 wire 名却配了不同费率时，费用报告只能随机取一条。同名同价（甚至都不配价）是正常写法。

完整配置见 `config.example.yaml`。
