# P81 设计稿：现代 3D 与平面绘图支持（luadraw 优先）

> 状态：**设计稿，未实现**。条目在 `docs/plan.md` 的 P81。
> 用户原文要点：给作图会话补上"现代的 3D 与平面绘图支持"，首选 LuaLaTeX 下的 luadraw（Lua 代码直接算、直接渲染 3D 关系与平面图），`tikz-3dplot`、`pgfplots` 3D 之类作为**可选**；为此要改提示词/契约，"能后续支持这样使用最好这样使用"。

---

## 1. 环境事实（2026-11-19 实测，TeX Live 2026 / LuaHBTeX 1.24.0）

"只要装了 LaTeX 就能用"这个假设**不成立**，先摆实测：

| 探测 | 结果 |
|---|---|
| `which lualatex` | `/usr/local/texlive/2026/bin/x86_64-linux/lualatex` ✓ |
| `lualatex --version` | LuaHBTeX 1.24.0（TeX Live 2026） ✓ |
| `kpsewhich tikz.sty` | 命中 ✓ |
| `kpsewhich pgfplots.sty` | 命中 ✓（3D 曲面/散点可用） |
| `kpsewhich tikz-3dplot.sty` | 命中 ✓ |
| `kpsewhich luacode.sty` | 命中 ✓ |
| **`kpsewhich luadraw.sty`** | **缺失** ✗ |
| **`kpsewhich luadraw.lua`** | **缺失** ✗ |
| **`kpsewhich 3dplot.sty`** | **缺失** ✗ |

CTAN 侧（`https://ctan.org/pkg/luadraw`，读到 2026-11-19）：

- luadraw 是 **LuaLaTeX 专用**环境（"Lua-based mathematical graphs for **LuaLaTeX**"），二维三维都做，**画出来仍是 TikZ**，可自动存图；
- 收录于 TeX Live / MiKTeX，包名 `luadraw`，当前版本 3.7（2026-11-05）；
- 结论：**要 luadraw 就必须用 LuaLaTeX 编译，并且要单独装这个包**（本机没装：`tlmgr install luadraw`，约 8 MB，含英文/法文文档）。

用户提到的另外两个名字要**先核对再写**：CTAN 上没有 `3dplot`、也没有 `3dtools` 这两个包——直接 `\usepackage{3dplot}` 会当场编译失败。这一族的真实候选是 `tikz-3dplot`（本机 ✓）与 `pgfplots` 的 3D（本机 ✓）。S0 的第一件事就是用 `kpsewhich` 把候选名单逐个探一遍，按实际命中的名字写进契约，**不按记忆猜包名**。

## 2. 管道现状（决定了要改哪儿）

1. **引擎是全局单例**：`latex.compile.engine`（默认 `xelatex`），三处消费同一份值——作图会话的 `CompileFigureTool`、章节 `compile`、整书 build。luadraw 只在 LuaLaTeX 下能跑，所以"一章用 luadraw、别处还用 xelatex"是做不到的：**要 Lua 就整本换引擎**。
2. **图代码的 standalone 包装是硬写的**：`buildStandalone(body, engineIsXe)`（`internal/latex/tools.go`）固定 `tikz` + `pgfplots` + `amsmath`，只在 `engineIsXe` 时补 `ctex`；而 `engineIsXe` 的判定是"引擎名含 xe"（`internal/latex/tikz.go`）。换成 lualatex 后中文图会**丢 CJK 支持**——判定应当是"需要 CJK（xe 或 lua）"，不是"是不是 xe"。
3. **围栏已经能声明宏包**：`buildStandalone` 会把围栏里的 `\usepackage` / `\usetikzlibrary` 提到导言区；`internal/latex/figurepkgs.go` 从围栏里抽"这张图需要哪些宏包/库"，`preflightConvert` 点名样式类漏加载的那些（T321）。luadraw 走的就是这条既有通路，不需要新机制。
4. **样式类负责加载宏包**（T321/T324）：所以 `\usepackage[3d]{luadraw}` 落在 cls，围栏里只写图形本体，别每张图重复声明。

## 3. 目标契约

- **一个开关管"能不能用 Lua 作图"**：`latex.compile.engine: lualatex` 只是引擎；还得有 `latex.compile.allow_lua: false`（默认关）。关着的时候提示词里根本不出现 Lua/luadraw 这些词（提示词只减不增，不给没装的用户灌无关信息）；打开才在作图会话的首条消息里点明"本项目的绘图后端是 luadraw，围栏里可以写 Lua"。
- **能力探测放在前置检查，不放在失败之后**：`preflightConvert` 里探 `lualatex` 与 `kpsewhich luadraw.sty`；缺就**停**，回执直接给一行 `tlmgr install luadraw`（不静默降级成 TikZ——降级会让 3D 图被画成 2D 而没人知道，T321 那次的教训）。
- **样式包要说清后端**：cls 里声明绘图后端，`manual.md` 固定加一段 `## Drawing toolkit`（用了哪个包、版本、一条最小可用示例），让转换会话照着写、而不是每章自己发明。
- **产物不变**：图最终仍是 PDF + 已有的数值门槛（重叠度/尺寸，T307/P78）与 `compare_figure` 证据叠加——判据层零改动，这正是这次设计最省的地方。

## 4. 三步落地

### S1 引擎与能力（不动提示词，可独立验收）

- `buildStandalone` 的第二参数从 `engineIsXe` 改成 `needCJK`（`xe` 或 `lua` 都加 `ctex`）；`tikz.go` 那处判定同步。
- `latex.compile.allow_lua`（默认 `false`）进 `config.go` 默认值 + 两份模板 + `docs/config.md`；`CurrentConfigVersion` 跟着 bump。
- `preflightConvert` 增一段：`engine=lualatex` 时探 `lualatex` 可执行与 `luadraw.sty`（`allow_lua` 打开时）；缺就返回错误，消息里带安装命令。
- `figurepkgs.go` 的宏包表加 `luadraw`/`luacode`（跟 `pgfplots`/`tikz-3dplot` 同一张表）。
- 验收：把引擎切到 lualatex、`allow_lua: false`，现有书能照旧编过（说明引擎迁移本身无副作用）。

### S2 提示词与 cls 契约

- `latex_style.system.md`：cls 必须声明绘图后端，`manual.md` 必须有 `## Drawing toolkit`；若项目开了 `allow_lua`，要求 cls 加载 luadraw 并给一条 3D 最小示例。
- `latex_figure.system.md`：只在 `allow_lua` 打开时追加一行"可以用 luadraw 写图形本体（Lua），输出仍是 TikZ；不要自己 `\usepackage`"。
- `latex_convert.system.md`：只透传围栏，不加关于引擎的话（转换不该关心引擎）。
- 验收：同一张 3D 图分别用 `tikz-3dplot` 与 luadraw 各画一遍，产物都能过既有数值门槛。

### S3 校验与安全

- **`\directlua` 静态 lint**：出图前扫图代码，命中 `io.`、`os.`、`package.`、`require`、`dofile`、`loadfile`、`load(` 一律拒绝并说明原因（编译期 Lua 完全够得着文件系统，`-no-shell-escape` 拦不住它）。
- 图代码编译改用 lualatex（`CompileFigureTool` 已吃 `engine`，只差 `buildStandalone` 的 CJK 判定）。
- 整书 build 跟随引擎；`latex.compile.timeout` 对 LuaLaTeX 给更宽的默认（慢是常态）。
- 验收：lint 用例（正常 luadraw 图放行、含 `io.open` 的图拒绝）；一本书全链跑通，图仍是矢量、门槛仍生效。

## 5. 风险与边界

| 风险 | 处置 |
|---|---|
| 引擎是整本的，混用不可能 | 前置检查在**开工前**判死，而不是跑到第 N 章才发现 |
| LuaLaTeX 比 XeLaTeX 慢（pgf/luadraw 计算都在编译期） | `latex.compile.timeout` 给 Lua 更宽的默认；日志里报每章编译耗时便于事后调 |
| `\directlua` 能跑任意 Lua（含 `os.execute`） | 静态 lint + 默认关的 `allow_lua` 双闸；只在用户显式打开时才放行 |
| 本机没装 luadraw | 探测到缺失就在前置检查里停，给 `tlmgr install luadraw`；**不自动装**（装什么由用户决定） |
| 用户口中的 `3dplot`/`tikztools3d` 在 CTAN 不存在 | S0 用 `kpsewhich` 逐个核对真实包名，写进契约的是**实际命中**的名字 |

## 6. 明确不做

- 不重写已有的 TikZ 图；不开"两个引擎并行编译同一本书"这种复杂度。
- 不把 `tlmgr install` 写进程序自动执行（装包属于用户环境决策）。
- 不在提示词里堆 luadraw 的 API 教学——模型自己查文档，提示词只说明"本项目允许用它、围栏怎么写、别自己 \usepackage"。

## 7. 验证清单

1. `allow_lua: false` + `engine: lualatex`：现有项目全链跑通，产物与 xelatex 版逐页页数一致（CJK 不缺字）。
2. `allow_lua: true` + 装好 luadraw：一张 3D 图用 luadraw 画成，过数值门槛、`compare_figure` 证据正常。
3. lint：含 `io.`/`os.` 的图代码被拒，回执说清原因与改法。
4. 缺 luadraw 时前置检查停，回执含安装命令，**不产出**任何降级图。
5. 整书编译耗时对比记录（xelatex vs lualatex，同一本书同一章）。
