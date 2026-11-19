# SK-I16 spike 实证（2026-11-19 22:09）：luadraw 两段式预编译，通过

**结论**：luadraw 可在 plain lua5.4 下独立运行、直接产出完整 TikZ——无 lualatex 机器的
备选路径（两段式预编译）**技术可行**。

## 证据
- `runner.lua`：standalone 运行器（四个关键垫片见下）
- `spike-output.tkz`：官方 doc Introduction 3D 示例（五边形+红色折线+蓝色半透明金字塔）
  的预编译产物——完整 \begin{tikzpicture}…\end{tikzpicture}（1016 字节，含透视投影、
  面片着色、裁剪域），可被任何引擎（pdflatex/xelatex/tectonic）直接 \input 编译

## runner 的四个关键垫片（内核实现时照此办理）
1. **自定义 searcher 按基名归一**：包内 require 风格不统一（带/不带 .lua 后缀并存），
   LuaTeX 靠 kpse 无感，plain lua 会双名加载导致类表/元表分裂。searcher 命中带后缀名时
   先查 package.loaded[基名]，加载后双名登记。
2. **先 require 'luadraw_base'**：基座工具（模块假设其已就位）。
3. **lualibs 垫片**：LuaLaTeX 预载 lualibs 提供 table.copy 等全局工具，standalone 自备
   （目前仅 table.copy 一处，按需增补）。
4. **tex 空桩**：全 22 模块仅 graph.lua:269 的 Sendtotex 用 tex.sprint（Show 专用），
   Show() 重定向 Savetofile 即绕开；空桩纯防御。

## 产物特性（实现时的红利）
- 纯 TikZ 文本：可 diff、可按内容哈希缓存（memory/render/）、figure_compare 照常三对照
- 预编译毫秒级（lua 计算几何远快于 TeX 宏展开）

## 验证复跑
```
lua5.4 runner.lua   # 产出 spike-output.tkz 同款（路径见 runner 内 FILES 常量）
```
