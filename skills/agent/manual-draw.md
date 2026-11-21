# T2 draw——矢量图重绘

## 你要产出什么

把 `inputs/` 里的原图重绘成 `out/figure.tex`（独立可编译的 TikZ/pgfplots 单图），kernel 验收时编译成 `figure.pdf` 后与**原图**跑软墨迹对齐。

## 工作循环

1. 看原图（你的看图工具），在 `scratch/` 里起草：结构草图、坐标估计、标签清单。
2. 写 `figure.tex`（`\documentclass[tikz]{standalone}` 或 `article+tikz`；自包含，不引用沙箱外路径）。
3. 编译自检：`pdflatex figure.tex` → `figure.pdf`，**用你自己的看图工具看渲染结果**，与原图并排比对。
4. 迭代：重合度不够/尺寸不对就改坐标重编，直到对齐。
5. 把成品放进 `out/`（figure.tex + 编译产物放一起，kernel 只按 spec 收 figure.tex 并自己编译 figure.pdf 比对）。

## 验收会卡什么（全部机器判定）

- 重合度 < `min_overlap`（按图 spec，常见 0.5）→ 打回，错误里附**对齐成绩**（各起点得分）和**尺寸句**（物理 mm 对比）——照证据改坐标，别猜。
- 尺寸偏差 > `max_scale_dev`（常见 0.10）：**"看着对上了其实窄了一截"会被 mm 口径抓住**——改坐标/画布，不是改线宽。
- 交够 `accept_min_submits` 次仍不达标 → 放行但记 note（人工会复核，别摆烂）。

## 技巧

- 标签/文字用节点放，坐标轴刻度先定范围再画数据。
- 复杂图先画骨架（主结构线）再补细节——对齐成绩先看主结构得分。
- notes.md 记你试过的坐标系与换算关系，rework 回来直接续。

## luadraw：立体几何与复杂图像的首选（2026-11-19 引入）

- 手绘 TikZ 成本高或效果难控的图（立体几何/三维场景/复杂函数图像）→ 用 **luadraw**（LuaLaTeX 绘图，后端 TikZ，与验收链天然兼容）；简单示意图照旧手绘 TikZ
- 用法/检索/风格规则全文见 `docs/luadraw/README.md`；流程与并发见 `docs/08-figure-flow.md`
- 引擎：章内用了 luadraw → 内核自动强制 lualatex（按需，不全书）；没装 lualatex 时报错指引装 texlive-lualatex
- **风格统一是硬要求**：先读 `book/style/draw-style.tex`（style 阶段产出的绘图模板），按它的三要素（线粗/标注字号/箭头样式）与 `pictureoptions` 默认串画；模板没有的参照全书已有 TikZ 图的视觉参数
- 快速定位示例：`dvagent luadraw-search <关键词>`（标题→源码位置→PDF页码），`pdftoppm -f <页> -l <页> docs/luadraw/luadraw-doc-en.pdf /tmp/pg` 看预览图
- luadraw 产物照过 figure 验收（min_overlap 等）——工具不降低门槛

## 章节即片段（SK-I24/25 契约）

你的产物是**片段**（被 stub 包壳编译）：**禁止 `\usepackage`/`\documentclass` 等导言命令**——luadraw 由内核按需在 stub 导言区自动注入，其余宏包依赖走 style 模板（`<book>.cls`）。
