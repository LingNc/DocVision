# T1 classify——图片四类分类 + styled 标记

## 你要决定什么

看任务包 `inputs/` 里的那张图，判定三态之一，写 `out/verdict.json`：

```json
{"class": "styled_text", "extracted": "<图里印刷文字的原文>"}
{"class": "vector", "reason": "<为什么可矢量重绘的一句话证据>"}
{"class": "raster"}
```

## 四类定义 + styled 标记（SK-I18 恢复旧参考实现，原模板 prompts-archive/latex_classifier.system.md）

verdict.json（必填 confidence 与 styled）：

```json
{
  "class": "text | table | vector | raster",
  "styled": true,
  "style_note": "styled=true 必填：字体/颜色/边框/装饰的具体线索",
  "extracted": "text/table 必填：提取的文本（table 重打为 md 表）",
  "reason": "vector 必填：为什么可矢量重绘的一句话证据",
  "watermark": false,
  "confidence": 0.95
}
```

**四类判据**：
- **text**——本质是文字：正文段落、艺术字标题/题号、装饰标签、水印文字、纯渲染的简单公式。必须附 extracted。
- **table**——可用 Markdown 表表达的表格（简单表/对齐表）。必须附 extracted（md 表形式）。复杂到 md 表达不了（合并单元格/嵌套）判 vector。
- **vector**——可矢量重绘的结构：几何图/流程图/架构图/复杂表格/有精确数学要求的曲线。必须附 reason。
- **raster**——照片、扫描截图、纹理。允许无附加字段。

**styled 独立标记**（不是类别）：
- `styled: true`——带视觉样式（艺术字/颜色/边框/装饰）且对忠实还原重要 → **进 style 素材池**（style-collect 选材），style_note 必填
- `styled: false`——正文/普通公式/普通表格 → **直通 md，不进样式流程**
- 拿不准就如实填并压低 confidence（<0.6 内核强制重判）

**下游分流**：text/table+styled=false → 文字化直通 md；text/table+styled=true → 文字化 + style 素材池；vector → draw；raster → 保留。

## 置信度（SK-I5，必填）

- verdict.json 必须带 `"confidence": <0~1>`——对自己这次分类的把握打分
- **犹豫就压低**：边界情况（像矢量又像高清位图、styled_text 与 raster 难辨）不许硬判——`confidence < 0.6` 内核强制打回重判
- 填 0.6~1.0 的分类与铁稳的分类同等入库；低分被打回不扣分，硬判被判错才浪费轮次

## 水印识别

- 看到水印/广告条（扫描书常见的页脚水印、网站广告图）：class 照旧判（通常 raster），**verdict 加 `"watermark": true`**——img2text 阶段会据此默认去水印（合成整行删除）
- 没水印的书（干净 PDF）不用管这条
