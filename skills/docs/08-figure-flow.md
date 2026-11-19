# 矢量图流程与子会话并发（设计文档，2026-11-19）

> 背景：引入 luadraw 后，矢量图绘制能力增强、风格统一需求上升，且宿主 agent 可能开多个子会话。
> 本文规定 style 与 draw 的分工、按需绘制触发方式、并发控制与阻塞唤醒。已落地项标注 ✅，待办见 issue.md。

## 1. 分工：style 总览先行，draw 按需响应

**不再设"矢量图统一处理阶段"**。改为：

1. **style 阶段新增"绘图样式总览"动作**（style-collect 之后、四步试转之前）：
   - 全书矢量图扫描分类（结构型/示意型/立体几何型/函数图像型，可用 luadraw 的标记出来）
   - 产出**绘图风格模板** `book/style/draw-style.tex`：定义三要素（线粗规格、标注字号、箭头样式）+ luadraw `pictureoptions` 默认串 + 配色（对照 palettes 速查）
   - 之后每张矢量图的绘制都引用该模板——保证风格统一（用户硬要求）
2. **convert 阶段按需请求绘制**：convert 遇到本章待绘制矢量图 → 内核 `gen-draw` 铺该图任务（按章按需，不全书预铺）
   - convert 会话可以自己领任务画（单会话模式，现状兼容）
   - **或多子会话模式**：另开 draw 子会话专领 draw 任务，convert 不阻塞在画上——图任务与 convert 任务在内核队列并存，按 claim 分发
3. **阻塞语义**：convert 会话若必须等某图（如该图是本章唯一矢量图且文字引用它）→ 阻塞等待（见 §3），其余章内容继续

## 2. 引擎按需（已落地 ✅）

章内任何 .tex 出现 `\usepackage{luadraw}` → `texcheck.NeedsLuaLaTeX` 探测到 → 该章编译强制 `lualatex`（`CompileEngine`）；其余章原链。**全书不必统一引擎**。

## 3. 并发控制（内核队列 = 天然的锁与分发）

多子会话共用工作区时，**互斥与分发由内核任务队列承担**，不另造锁机制：

- **任务粒度互斥**：每张图一个任务，claim 租约（lease）= 锁。两会话不会领到同一任务；租约过期自动回收（防会话暴毙死锁）
- **工作区分配**：所有会话共享同一 project root；各任务的产物写自己的 `tasks/<id>/out/`，由 submit 验收后入正式区——不存在跨会话写冲突区
- **排队**：任务先到先得（queue 序），多会话并行时内核自然分发
- **阻塞等待（待办 SK-I12）**：`dvagent claim --wait`——无任务可领时阻塞轮询直到有任务/超时。阻塞会话不占 CPU；"内核唤醒"= 轮询到新任务（无推送机制，轮询间隔默认 5s 可配）。会话也可以直接被用户停掉，队列不受影响

## 4. 状态机与交接

- draw 任务沿用现有任务协议（accept/verify/verdict.json），验收门槛不变（min_overlap/比例/次数）
- luadraw 产物是 TikZ → figure_compare 照常三对照；**验收不因工具而降低**（docs/07 铁律）

## 5. 待办（issue.md 对应）

- SK-I10 ✅ 引擎按需（CompileEngine/NeedsLuaLaTeX）
- SK-I11 待办：gen-draw 按章按需铺任务（现状按全书铺）+ style 总览动作进 manual-style
- SK-I12 待办：`claim --wait` 阻塞命令
- SK-I13 待办：draw-style.tex 模板产出物规范（style 阶段验收加一项"模板已产出"）
