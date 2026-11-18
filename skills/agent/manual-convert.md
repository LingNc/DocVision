# T3 convert——章节转换

## 你要产出什么

把 `inputs/chapter.md`（MinerU 拼装的章节 md，含 `![caption](images/<sha>)` 图链接）转成 LaTeX：主产物 `out/<章名>.tex`（spec 里 `main_file` 指定的名字），编译通过 + 内容覆盖率 ≥85%。

## 工作方法

1. **读章节 md**，规划章节结构（`\section` 层级跟着 md 标题走）。
2. **图的处理**：md 里的图链接对应的矢量图已由 draw 任务重绘（`figures/` 白名单下有成品 TikZ）。按图的 caption/编号引用——用 `\input{figures/<名>.tex}` 或转 `\includegraphics`（以任务包白名单为准）。
3. **OCR 存疑处以原书为准**：`doc-search "<可疑词>"` 拿全书页号+bbox → `page-render <页> [bbox]` 渲染原书局部 → 看图定夺原文。md 是 OCR 参考，**原书是权威**。
4. **公式**：md 里 `$$…$$` 转 LaTeX 数学；MinerU 公式块可能是图片，同样回原书核对。
5. **编译循环**：`pdflatex <章名>.tex` 修到零错误；**然后用 pdftotext 自检覆盖率**——验收器按 CJK 三字组比对 md 与 PDF 文本，低于 85% 打回并列出缺口段落清单。
6. **写报告**：`out/report.md`（可选但建议）——本章做了什么、哪些图引用了、遗留疑点。上游整合与核对靠它。

## 验收会卡什么

- 缺主产物/编译失败 → 直接打回。
- 指向 `_subdivision/`、`work/temp/` 等临时路径 → **自包含判据打回**（所有 `\input`/`\includegraphics` 必须解析到成品树内）。
- 覆盖率：第一次不达标打回（附缺口段落清单）；补全后仍不达标会放行但记 `coverage_note`（缺口随报告上行，不静默）。

## 章节太大怎么办

别硬转——提交拆分提案：

```bash
# segments.json: [{"id":"seg1","start":1,"end":150}, {"id":"seg2","start":151,"end":290}]
dvagent split <项目> conv_<章名> segments.json
```

kernel 验证（2–16 段、完整、连续、无重叠）后自动扇出子任务，子任务全过父任务解锁。

## 交还仪式

notes.md 两行交接语（停在 X/下一步 Y）——rework 回来时先读它。
