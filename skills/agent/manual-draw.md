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
