# 原参考实现提示词全档（逐字归档）

来源：上游 DocVision 仓库 `go/internal/prompts/templates/`（LingNc/DocVision，master 分支）。
逐字拷贝，未改动一字——用于与 `../prompts-img2text.md`（策略摘编）对照核对。
机制已随 skills 新架构变更（三态分流/内核验收），提示词内容按用户要求原样保留，其中含 `[IMG_TYPE: <type>]` 类型标识规则（用户拍板保留）。

| 文件 | 用途 |
|---|---|
| `img2text.system.md` | 图转文 system 提示词：八类图判定 + `[IMG_TYPE:]` 输出头 + mermaid/LaTeX/表格规则 |
| `img2text.user.md` | 图片任务会话开场模板：行号定位 + 上下文行 + get_more_context 增量上下文 |
| `img2text_textonly.system.md` | 纯文字化提示词（OCR 单轮） |
