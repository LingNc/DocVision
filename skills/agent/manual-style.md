# style 阶段——样式会话工作手册（写样式 / 选材 / 试点 / 试点修复）

## 阶段定位与数据面

- **素材（选材）**：`dvagent style-collect <项目>` 把 classify 判为 styled_text 的图里**印刷文字**汇总进 `style/styled_text/`（一图一条、带来源头）。写样式与转换引用原书文本时从这里取料。
- **试点章**：`dvagent config-set <项目> style.pilot_chapter <章名>` 指定一章先试转——样式问题在这一章上先暴露、先修，不波及其他章。
- **字体**：特殊字体由用户提供进 `fonts/`（doctor 会提示）；缺失按 config `style.fallback_font` 回退并在报告里显式标注，不静默。

## 工作循环（四步）

1. **写样式**：产出 `style/<名>.cls` + 样式手册（排版/字体/图表规则）；用 `style/styled_text/` 里的原书文本校对字体需求（题记花体/特殊符号→字体清单给用户）。
2. **试点**：`config-set style.pilot_chapter` 指定章 → gen-convert 只铺/先跑试点章 → 交卷验收（编译+覆盖率）。
3. **试点样式修复**：验收打回/核对发现样式问题 → 走 rework（原沙箱续接）；样式类系统性问题记 `profile-update`（书级经验），修 cls 后试点章重转。
4. **放量**：试点通过后 gen-convert 铺全书其余章。

## 纪律

- 样式判据（编译过/覆盖率/自包含）与全书同标准——试点章没有降低门槛；
- 字体与阈值同属用户专属资源：agent 只做"缺失→回退→标注→报告"，调整权在用户。

## 绘图样式总览（style-collect 之后、四步试转之前必做）

1. 全书矢量图扫描分类：结构型/示意型/立体几何型/函数图像型——能用 luadraw 的（立体几何/复杂函数图像）标记出来
2. 从模板 `agent/templates/draw-style.tex` 拷贝产出 `book/style/draw-style.tex`，对照原书扫描件**填定四组值**：线粗规格（\dvLineThin/Main/Thick）、标注字号（\dvLabelSize/CaptionSize）、箭头/端点样式（dvarrow/dvpoint/dvgrid）、配色（dvMain/dvAccent，对照 docs/luadraw/luadraw_palettes_list.pdf）
3. 试转章里的矢量图必须 \input 该文件、用其中的宏——**验收时逐图检查：三要素散写（绕过模板）一律打回**
4. 之后 draw 阶段每张图（手绘 TikZ 与 luadraw 一视同仁）都引模板，保证全书风格统一
