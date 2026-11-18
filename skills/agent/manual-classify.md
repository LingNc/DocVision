# T1 classify——图片三态分类

## 你要决定什么

看任务包 `inputs/` 里的那张图，判定三态之一，写 `out/verdict.json`：

```json
{"class": "styled_text", "extracted": "<图里印刷文字的原文>"}
{"class": "vector", "reason": "<为什么可矢量重绘的一句话证据>"}
{"class": "raster"}
```

## 三态定义

- **styled_text**：本质是排版文本（表格化文字、公式以外的印刷内容）。必须提取文字到 `extracted`——**空提取 = 打回**（不会降级成 raster，不确定就重看原图再交）。
- **vector**：线条/几何构成的示意图（流程图、坐标图、电路图、简单图表）。必须给 `reason` 证据（"由直线+箭头构成，无照片纹理"之类）。这类会被 draw 任务重绘成 TikZ。
- **raster**：照片、复杂渐变、任何矢量重绘不划算的图。不需要附加字段。

## 判据

- 像素完全一致/近一致的重复图 kernel 已在 dedupe 层并桶，你拿到的就是"key 图"——分类结论会被跟随图共享，**拿不准时宁可打回重看，不要乱填**（乱填值=打回，且不会记 done）。
- 判断证据看几何：文字基线整齐+行结构=styled_text；可拆成线段/圆/箭头的路径=vector；连续调制的灰度/彩色像素=raster。

## 交还

verdict.json 写好后 `submit`。notes.md 记一行分类理由（给下游 draw 任务当线索）。
