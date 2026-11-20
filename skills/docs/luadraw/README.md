# LuaDraw 使用指南（skills 内置参考）

LuaDraw v3.7（Patrick Fradin，LPPL 1.3c+，来源见 SOURCE.txt）：LuaLaTeX 上的 Lua 数学绘图环境——2D/3D 图用 Lua 代码描述，后端输出 **TikZ**，与我们的矢量验收链（figure_compare / min_overlap）天然兼容。**定位：更好的画笔，不是绕过验收的通道**——所有 luadraw 产物照旧过既有验收门槛。

## 什么时候用

- **立体几何、三维场景、复杂函数图像**：手绘 TikZ 成本高、效果难控的图——luadraw 的主场（3D 透视/实体/旋转见文档 3D 八节）
- **平面图**：手画 TikZ 已经快的（简单示意图/流程），不必上 luadraw——工具按需，不为用而用
- **md 档图转文**：不涉及（那是 img2text 三态的活）

## 简化用法（2026-11-20 用户定调：主路径简单，预编译只是回退脚本）

- **主路径：正常用 lualatex 写**——章内直接 `\usepackage[3d]{luadraw}` 写绘图代码，零额外步骤
- **回退（主机没 lualatex）**：跑独立预编译脚本，把 tex 里的 luadraw 块等价展开为 TikZ 文件，之后 **pdflatex/xelatex/tectonic 正常编译**，编译流程零影响：

```bash
python3 docs/luadraw/tools/luadraw-expand.py <你的章.tex> [files目录] [lua解释器]
# 输出：<name>.tkz × N + 原 tex 块替换为 \input{<name>.tkz}（幂等，可重复跑）
```

内核的自动回退（`NeedsLuaLaTeX` 探测→自动预编译）保留但**非必需**——手动跑脚本是完全等价的独立路径，二选一即可。

## 引擎（按需，不全书强制）

**无 lualatex 的备选（SK-I16，spike 已通过，见 spike/README.md）**：luadraw 本质是
"Lua 代码 → TikZ 文本"，可用 plain lua5.4 预编译成 .tikz 文件后走正常引擎链——回退序：
有 lualatex → 直编（主路径）；无 lualatex 有 lua5.4 → 预编译回退；都没有 → 报错并指引
二选一安装。

luadraw 是**纯 LuaLaTeX 包**。内核已支持按章探测：章内任何 .tex 出现 `\usepackage[...]{luadraw}` → 该章编译自动强制 `lualatex`（`texcheck.NeedsLuaLaTeX` + `CompileEngine`），其余章走原 pdflatex→xelatex 链。**只有用 luadraw 的章需要装 lualatex**（texlive-lualatex）。

## 上手（三个关键机制）

```latex
\usepackage[3d]{luadraw}   % 导言区；[3d] 开关三维（还有 [extensions]）
...
\begin{luadraw}{name=fig1, exec=true}
  g = graphe(-3, 3, -2, 2)      -- Lua 代码画
  ...
  g:Show()                      % 发送到 TeX（g:Save() 存文件）
\end{luadraw}
```

- 环境选项：`name=...`（产物文件名）、`exec=...`（开关执行）、`auto=...`
- **风格钩子：`pictureoptions="..."` 直接注入 tikzpicture 选项**——贴文档风格的官方入口
- 2D 七节（图形方法/几何构造/曲线/变换…）、3D 八节（点类/实体面/场景/中央透视/视锥…）、扩展十四节（多面体/线圈链/阴影面/调色板/povray 导出…）

## 风格对齐规则（必守，确保与全书一致）

1. **默认参数优先**：能用 luadraw 默认线宽/配色就不自定义——它的默认就是为出版图设计的
2. 必须自定义时，线宽/配色/字号**跟随本书 style 配置**（style 阶段产出的绘图模板，见 `docs/08-figure-flow.md`），通过 `pictureoptions` 注入，不逐图散写
3. 字体走 convert 已有的 fontspec 设置，不另起炉灶
4. luadraw 图与手绘 TikZ 图混排时：箭头样式/标注字号/线粗三要素对齐（style 模板里定义好）
5. 产物照旧过 `figure` 验收（min_overlap 0.17 等）——luadraw 画错了一样打回

## 易错点（toolchain 压力测试沉淀，边界基准见其 AGENTS.md）

- **新 OOP API 一律表传参**：`graph:new{window={...},size={...}}` / `graph3d:new{...}` / `Dboxaxes3d{grid=true,...}`——函数式调用（`graph:new(-2,2,...)`）会炸 `attempt to index a number value (local 'args')`；老文档的 `graphe(-3,3,...)` 是旧函数式 API，别看错
- `ld.graph`（非 graph2d）；`cpx.Z`；`Daxes/Dgrid` 的 `#d` 定长表校验静默不画
- luadraw 章 preamble 需 6 组 TikZ 库：patterns, patterns.meta, plotmarks, arrows.meta, bending, decorations.markings, backgrounds

## 检索（给 AI 的精确导航）

```bash
dvagent luadraw-search <关键词...>     # 多关键词 AND；输出标题/源码位置/PDF物理页码
pdftoppm -f <页> -l <页> docs/luadraw/luadraw-doc-en.pdf /tmp/pg   # 看该页预览图
```

- 索引：`luadraw-index.json`（514 条：17 section + 143 subsection + 225 subsubsection + 129 命名示例）
- 页码=物理页（pdftotext/pdftoppm 的 -f/-l 直接可用）；重建索引：`python3 tools/build-index.py doc-src-en luadraw-doc-en.pdf luadraw-index`
- 包内只带本指南 + en PDF + 索引 + 调色板速查；**包源码与宏包本体现场取**（仓库 `skills/docs/luadraw/{doc-src-en,files}` 或 CTAN）

## 许可

LPPL 1.3c or later。全文与源码仅作内部参考， redistribution 遵循 LPPL（CTAN-README.md）。
