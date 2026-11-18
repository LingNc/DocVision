# Agent 工作手册（冷启动说明）

> 这份文档是给**宿主 agent** 的"如何在这个流水线上干活"。kernel 只管验收（见 `../docs/02-task-protocol.md` 的验收 spec），这里讲的是把活干好的方法。每次会话开始或被压缩后，按下面的仪式恢复上下文——**你的上下文里没有任何进度，进度全在磁盘上**。

## 冷启动仪式（每次会话/压缩后，成本几百 token）

```bash
dvagent -workspace <工作区> status          # ① 看全局：各项目阶段、队列统计
```

向用户播报："项目 X 在 convert 阶段，Y 任务待处理、Z 失败。是否继续？"——用户确认后：

```bash
dvagent -workspace <工作区> claim <项目> <你的会话名> 3 convert
```

拿到 **3 个自包含任务包**，按顺序干完，逐个 `submit`。不要在上下文里记任何进度——下一个任务包自带全部所需信息。

## 铁律（违反会被验收器打回，且浪费的是你的 token）

1. **状态零保存**：进度、结论、坑，全部写进任务沙箱的 `notes.md`（结论）和 `scratch/`（过程）。你的上下文只是磁盘状态的瞬时投影。
2. **看图纪律**：打开一张图 → 决策 → 立刻把结论写进 notes.md → 这张图从"脑子里"清掉。百图连看时，串味只影响逐图隔离的笔记文件，不影响正确性。
3. **产物只在 `out/`**：`submit` 只收沙箱 `out/` 里的文件；引用 `_subdivision/`、`work/temp/` 等临时路径 = 打回（自包含判据）。
4. **交还仪式**：每个任务交还前，在 notes.md 末尾追加两行：`停在 X` / `下一步 Y`。重入时先读这两行（记忆泡泡重入 O(1)）。
5. **别信自己记得**：回炉指令、核对发现的问题，kernel 都落盘（issues），你只需 claim rework 任务，不用记住任何事。

## 五类任务手册

| 任务类型 | 手册 |
| --- | --- |
| classify（图片三态分类） | [manual-classify.md](manual-classify.md) |
| draw（矢量图重绘） | [manual-draw.md](manual-draw.md) |
| convert（章节转换） | [manual-convert.md](manual-convert.md) |
| check（核对，只读） | [manual-check.md](manual-check.md) |
| rework（复盘回炉） | [manual-rework.md](manual-rework.md) |

## 常用命令速查

```bash
dvagent status                          # 全局蒸馏（冷启动唯一入口）
dvagent claim <项目> <owner> [n] [类型]  # 领 n 个任务（+ 租约）
dvagent submit <项目> <id,id,...>        # 批量提交（逐条验收）
dvagent note <项目> <task_id> <文本>     # 追加任务笔记
dvagent split <项目> <parent> <segs.json> # 章节拆分提案（2–16 段）
dvagent doc-search <项目> "<query>"      # 全书检索（回页号+bbox）
dvagent page-render <项目> <global页> [bbox] # 原书页渲染/裁剪
dvagent profile-update <项目> <文本>     # 书级经验档案（append-only）
dvagent pause/resume/abort <项目>        # 生命周期控制
```
