# T6 img2text——档位2 逐图文字化（小轮转）

> 档位2 = 书级输出**干净 md** 的流水线：你逐图处理，kernel 最后按你的决策
> 重组装（`img2text-assemble`）。这是参考实现 img2text 模式的任务级落点。

## 三态决策（每张图必须选其一）

1. **能文字化**（表格化文字/印刷内容/可纯文本表达的图）→ 写 `out/replacement.md`：
   提取的文字内容（原文摘录，格式用 md）。kernel 用它**替换** md 里的图片行。
2. **结构图**（流程图/时序图/状态图/拓扑图等关系结构）→ 写 `out/figure.mmd`：
   mermaid 源码。kernel 包成 ```mermaid 围栏块替换图片行，并用 mmdc 校验
   （校验失败你的任务会被打回，按报错修语法）。
3. **无法文字化**（照片/复杂图/文字化反而损信息）→ 不写产物或写
   `out/verdict.json {"result":"keep"}` → md 里**原样保留**这张图。

## 工具三件套（小轮转）

1. **上下文检索**：`dvagent ctx-up <项目> <task_id>` / `ctx-down`——该图在 doc_index 里的前后文（默认 3 块）。
2. **看图**：任务包 acceptance 带 `global`（全书页）+ `figure_hint_bbox`，`page-render <项目> <global> <bbox>` 渲染后用你的看图工具打开。
3. **提交**：按三态写产物后 `submit`。

## 纪律

- 决策基于你**看见的**——结构图才给 mermaid，印刷文字就给原文，别给照片硬套 mermaid。
- notes.md 一行交代决策理由（给后续重组装/回炉当线索）。
- 重组装顺序按 md 中图片出现序，你的 entry_seq 必须对应——别交错提交。
