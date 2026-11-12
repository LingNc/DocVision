# P80 插图绘制的全书统一样式：由 cls 提供 `dv-*` 样式族

> 设计稿。对应 `docs/plan.md` 的 P80；P79（成对图按一套样式）是它的第一步，样式族落地后 P79 的那条说明降级为兜底。

## 1. 现状

- 每张矢量图的线条粗细、颜色、字号、箭头都**写死在围栏里**（`line width=0.46mm`、`font=\fontsize{11}{12}`），数值来自作图会话对**那张图**原书墨迹的实测。同一本书里不同图的数值自然不同，把它们排到同一页上就一粗一细。
- 转换阶段（`latex_convert.system.md`）才做"重排"，靠模型逐图判断"要不要把这个粗细改掉"。它看到的是样式手册 `## Vector figure style` 的一段散文，一致性靠模型自觉，无法事后核查。
- P79 只解决"同一个图号的并排两半"这一种最刺眼的不一致。
- cls 目前只管版式与宏包（T321 让它按章节实际用到的库声明、T324 要求加载基线 TikZ 库），**不涉及绘图参数**——全书没有一处"绘图参数"的单一来源。

## 2. 目标

1. 全书插图共享**一套具名绘图样式**：线宽、颜色、字号、箭头长度、虚线节距都从一处来；
2. 作图会话只写**语义引用**（`draw[dv-wire]`），不写毫米数；
3. 转换阶段不再逐图猜线条，只对"还写死宽度"的围栏做一次映射；
4. 单图例外用**相对倍数**表达（`dv-thick` = 1.8× 基准），不破坏统一性。

## 3. 契约：样式族与宏

cls 里定义（数值由样式会话从实测统计里挑）：

```latex
\providecommand\dvfigloaded{}          % 让作图会话的兜底定义失效
\newcommand\dvfigwire{0.45mm}          % 基准线宽（全书一个数）
\newcommand\dvfigaxis{0.30mm}
\newcommand\dvfiggrid{0.18mm}
\newcommand\dvfigarrowlen{1.6mm}
\newcommand\dvfigdash{1.2mm}
\newcommand\dvfigfont{10}
\definecolor{dvcolor}{HTML}{1A1A1A}
\definecolor{dvfill}{HTML}{D9D9D9}
\tikzset{
  dv-wire/.style  = {line width=\dvfigwire, line cap=round, line join=round},
  dv-thin/.style  = {line width=\dvfigwire*0.6, line cap=round},
  dv-thick/.style = {line width=\dvfigwire*1.8, line cap=round},
  dv-axis/.style  = {line width=\dvfigaxis, -{Stealth[length=\dvfigarrowlen]}},
  dv-grid/.style  = {line width=\dvfiggrid, dash pattern=on \dvfigdash off \dvfigdash},
  dv-dash/.style  = {line width=\dvfigwire, dash pattern=on \dvfigdash off \dvfigdash},
  dv-label/.style = {font=\fontsize{\dvfigfont}{\dvfigfont*1.2}\selectfont, inner sep=0pt},
  dv-node/.style  = {draw=dvcolor, fill=white, line width=\dvfigwire, inner sep=1pt},
  dv-fill/.style  = {fill=dvfill, opacity=0.25, draw=none},
}
```

作图会话在围栏开头写**带守卫的兜底**（作图时 cls 还不存在，必须能独立编译）：

```latex
\ifdefined\dvfigloaded\else
  \tikzset{dv-wire/.style={line width=0.46mm,...}, ...}  % 该图自己的实测值
\fi
```

cls 在 `\begin{document}` 之前就把 `\dvfigloaded` 定义好，于是转换/整书编译时**类里的值赢**，单图编译时走兜底。

## 4. 三处分工

| 位置 | 现在 | 之后 |
|---|---|---|
| style 会话（`latex_style.system.md`） | cls 只管版式；manual 里一段散文描述图风格 | cls 必须定义上面整套 `dv-*` + `\dvfig*`；取值来自 `source/figure_ink.json`；manual 的 `## Vector figure style` 逐条列出名字与取值 |
| images 阶段的作图会话（`latex_figure.system.md`） | 写死 `line width=…` | 写 `draw[dv-wire]`，围栏开头放守卫兜底块（保留实测值） |
| convert（`latex_convert.system.md`） | 逐图判断怎么改线宽 | 只把残留的 `line width=…` 映射到最近的 `dv-*`；P79 的成对说明降级为兜底 |

## 5. 新增的机器产物

- `<proj>/source/figure_ink.json`（images 阶段写，纯本地解析，不花 token）：每图 `{img, class, width_median_mm, dash_used, font_pt}` + 全书汇总（按 `class` 分组的中位数）。style 会话通过 `project:` 视图读它。
- `preflightConvert` 增加一条**警告**：围栏里出现 `line width=` 而不属于 `dv-*` 定义 → "这张图还写死线宽"（不硬卡，也不许它变成失败理由）。

## 6. 分三步落地

1. **S1**：`figure_ink.json` 聚合 + style 会话在 cls 里定义样式族 + manual 记账。作图与转换都不动——先把"单一来源"建起来，可单独验证类能编译、`dv-*` 可被手写图引用。
2. **S2**：作图会话改语义引用 + 守卫兜底（提示词改动最大的一步）。旧图不受影响（仍写死宽度），新画的图从此走样式族。
3. **S3**：convert 做残留映射 + preflight 警告；P79 说明降级。

## 7. 风险与取舍

- **守卫依赖加载顺序**：cls 在导言区、图在正文，成立；`standalone` 单图编译时没有 cls，走兜底，行为与现在一致。
- **字号统一后大小图的观感**：`dv-label` 固定 pt 是**有意的**（书上的标签本来就是固定字号），例外给 `dv-label-sm`/`dv-label-lg` 两级，不再按图各写一个数。
- **旧项目不强制重画**：围栏已写死宽度的，只由 convert 映射一次，可逐条核查；想彻底统一就重跑 images（有阈值门槛与退回机制管着）。
- **`dv-*` 是提示词契约**：改名要同步提示词 + 注册表 `MustMention`，否则会话会写出类里没有的名字（preflight 会点名，但那是事后）。
- **样式会话的负担**：它要从统计里挑数（而不是自己编），cls 里多二十行；换来的是全书一致与可核查。

## 8. 验证计划

- 单测：`figure_ink.json` 聚合（中位数、按类别分组、空输入）；`preflightConvert` 检出写死宽度；cls 渲染后 `\providecommand\dvfigloaded` 在导言区、`dv-*` 齐全（`TestPreflightConvert` 系列邻居）。
- 端到端 A（S3）：拿一本围栏已写死宽度的书跑 `--only <章>`，检查产物 `dv-*` 覆盖率、编译通过、页数不变。
- 端到端 B（S2）：用同一章 md 的副本跑一次作图，确认单图能独立编译（走兜底），且画出的图并入整书后用的是类里的线宽。
