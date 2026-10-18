# dedupe 全流程（从扫描到落盘）

本文是 dedupe（档位1 的图片整组核验）**从头到尾的完整过程档案**：每一步做什么、判据怎么算、阈值在哪、实测数字多少、哪些地方明知不完美。术语定义见 [dedupe.md](dedupe.md)，动机与历史见 [issue.md](issue.md)，档位1 其余阶段见 [latex.md](latex.md)。

## 0. 输入与产物

| 项 | 内容 |
|----|------|
| 输入 | 本次已 classify 完的**全部图片**（`source/progress_items/<md>/<img>.json` 有 `class`） |
| 代码入口 | `latex.Runner.motifDedupePhase`（`go/internal/latex/motif_dedupe.go`） |
| 产物 1 | 每图 progress 的 `equivalent_to` / `dedupe_leader_hash` / `shared_tex` / `dedupe_checked`（**真源**） |
| 产物 2 | `<proj>/source/dedupe/groups.json`（组/代表图/成员/分数/类别/来源/副本 + 规则与指纹）、`report.md`（一页汇总） |
| 产物 3 | `<proj>/source/dedupe/metrics/{pairs.csv,buckets.json}`（中间量，T136） |
| 下游 | process 只重画每组代表图（`key`），组内其余位置复用它的 `source/figures/motif_*.tex`，原图 LINK 保留 |

## 1. 全景（12 步）

```
已分类图片（388 条引用）
 ①  可读性检查
 ②  逐字节收缩：sha256 相同 → 只留代表（388 → 130 份，258 副本）
 ③  指纹复用：输入与规则没变就整段跳过（连解码都不做，T141）
 ④  全池逐对特征相似度 → 并查集（O(N²) 的 0/1 判据）
 ⑤  全池居中重合扫描（小图标，≥ dedupe_align_min 的边再并一次）→ 桶 = ④ ∪ ⑤
 ⑥  自动定组：≥ dedupe_icon_auto（0.75）的小图标直接成组、不进会话
 ⑦  分桶成会话作用域：桶原始大小 > 50 独占，其余小桶合并 ≤ 150
 ⑧  核验会话（并发）：模型看图 → motif_groups → submit → runner 逐成员复核
 ⑨  整理会话（串行、可选）：未分配图 × 已确认组代表图
 ⑩  挂靠收口：没进过会话、但对某组代表图仍 ≥ dedupe_attach_min 的直接挂进去
 ⑪  副本跟随代表图；写 groups.json / report.md / metrics
 ⑫  process 按组共享 tex；代表图无成品则解除跟随
```

标了 ①②③ 的都不看像素内容（或只在 ③ 之后看一次），④⑤ 是**分桶之前的一次全池扫描**——桶是并查集的结果，不是扫描的边界（T130）。

## 2. 逐步细节

### 2.1 候选池与逐字节收缩

`motifContentLeaders(pool)`：按文件内容 `sha256` 分组，每组留一张 **leader** 进后面的扫描与会话，其余进 `copies`。

- 为什么可以这样砍：字节完全相同就是同一张图片，看一份等于看全部，不存在看错的可能。
- 副本**不进会话**，结论在最后一步整体跟随（见 2.11）。
- 实测该书：388 条引用 → 130 份不同内容（258 副本）。

### 2.2 指纹复用（T141）

先做一次**只算哈希、不解码**的预扫，得到排序后的哈希集，再校验 `source/dedupe/groups.json`：

- `v` = `motifDedupeVersion`；`rules` = `{align_min, align_shift, icon_auto}`；`fingerprint = sha256("v=…;min=%.3f;shift=%.3f;auto=%.3f" + 排序去重哈希)` 前 16 位十六进制；
- 还要求 `refs` 条数、每张图的 progress 状态、覆盖关系逐项对得上。

任一不符 → 正常全量重扫（**宁可重算，不能不认账**）；全对 → 打印 `[dedupe] 输入与规则未变（指纹 …），沿用 …` 后直接返回，不解码、不算特征、不开会话。

### 2.3 逐对特征相似度（初始分桶的一半）

`findVisualMotifCandidates`：每张图**只算一次**特征集 `computeMotifFeatureSet`（像素指纹/形状/主色），然后 O(N²) 调 `motifFeatureSetSimilar`（0/1 判据）并做并查集。

- 这一步就是"最初那个相似度分组"，一直存在，不是遗留代码。
- 它的问题：在小图标上会把同一枚标记的一个连通片切两半——该书实测纯相似度只连出 **1 个 90 张的大桶**，另有 10 张同标记的图落进"未分配"。

### 2.4 居中重合扫描（初始分桶的另一半，T130）

`FindVisualMotifCandidatesAligned(vis, alignMin, shiftPct)` 在 2.3 之后**再扫一遍全池**：对每一对能算出覆盖率的图算居中重合度 `align`，`align ≥ alignMin`（默认 0.60）就往并查集里加一条边。

- 结果：桶 = 2.3 ∪ 2.4。**桶只能是召回的超集，不能当重合度的唯一来源**——否则被 2.3 切开的半边永远补不回来。
- 只有内容框 ≤ `dedupe_icon_max_area`（默认 200000 px²，约 447×447）且覆盖率合格（`w,h ≥ 3`、`w*h ≥ 16`、墨量 ≥ 0.01）的图参与；更大的图、纯留白的图 `pairs.csv` 里 `skip=too_large`，仍只靠 2.3。
- 预筛（不改变判据，只省时间）：`|Δ墨量占比| ≤ 0.12`、`|Δ长宽比| ≤ 0.35`、已在同桶的对直接跳过、逐字节副本只算一份。
- 并行：按 `runtime.NumCPU()`（≤ `motifAlignWorkers`）并行算「达标与否」，再**按原顺序串行 union**——合并顺序不变，分组逐字一致。
- 规模闸门：要扫的图超过 `dedupe_bucket_max`（默认 400）时，整段退回纯 2.3 并警告（O(N²) 太慢）。
- 实测该书：C(130,2)=8385 对里 882 对 `too_large`，7503 对算出分数；`align ≥ 0.60` 的 2983 对中有 **817 对是 2.3 没有连的**（265 对在同一相似桶内、552 对跨桶），正是它们把桶从 `[90, …]` 补成 `[102, 2]`。

**居中重合度（align）怎么算**——这是全流程唯一的几何判据，桶和组内闸门共用：

1. **内容框**：把三通道取最大后 > `0.2`（`motifCanonInkLevel`）的像素当墨，求外接矩形；只用来定位贴图，所以原图的留白/位置不影响比较。
2. **软墨量**：不二值化，`w = 1 − 通道/255`（0–1 连续值）。
3. **画布 `S`**：`clamp(P90(内容框长边), 24, 512)`。注意当前生产代码是按**每一对**取 `max(a.maxSide, b.maxSide)` 再夹到 24–512（池级 P90 的 `motifCanonCanvas` 有实现和单测，但没有生产调用点——见第 4 节）。
4. **贴图**：内容框**居中**贴进 S×S，**不缩放**（尺寸差一倍就是尺寸差一倍，不归一化）。
5. **分箱**：按 G×G 网格，格值 = 该格内软墨和 ÷ 格面积（内容框外的部分算 0），三通道各自独立。
6. **逐级搜索**：G 从 32 起**翻倍**，一直到"放得进 S 的最细一级"（`finest = round(0.95·S)`，`latex.dedupe_align_grid` 可写固定整数或 `0.95S` 这种比例，上限 1024）。
   - 最粗一级：搜索窗口 = `±round(shiftPct·G)` 格（默认 0.10·G），中心在 (0,0)；
   - 之后每一级：以上一级选中的格**换算到本层（×2）**为中心，在 **±2 格**内搜（`motifCanonRefine`）；
   - **不扩窗**：单次扫描，不会再因为"最优解落在窗口边缘"翻倍重搜（T143，此前那套扩窗最多重搜 4 轮、平均 314 次重叠度计算，是整段扫描的主要成本）。
7. **打分**：每一级对每个候选平移算三通道平均 Jaccard `Σmin/Σmax`，取最大者；**判定分 = 最终（最细）那一级的分数**，粗层只负责把平移搜到位。
   - 为什么必须看最细层：只差一个符号的交换图（内容框 171×390）在 G=32 时是 0.765、G=256 时掉到 0.373——粗网格会把不同的图并成一张。
   - 小图（内容框 34–45 px，占本书绝大多数）`finest ≈ 0.95S < 64`，**只有 32 这一级**，逐级搜索退化为单层；真正的多级只在大图（S ≥ 68）上发生，`pairs.csv` 的 `align_level` 会写实际用到的层（本书出现过 32/128/256）。

### 2.5 自动定组（T131/T134）

`motifAutoIconGroups(leaders)`，`dedupe_icon_auto`（默认 0.75，0=关闭）：

1. 只取 `内容框面积 ≤ 20000 px²`（约 141×141）的图——再大就是插图了，两枚相似插图不经审校直接合并风险不对等，它们照旧进会话；
2. 按 `align ≥ 0.75` 连成连通片（**单链**，和桶同一个判据，不按 class 过滤——class 对"是不是同一枚图标"没有参考价值，T134）；
3. 每片选**中心**（片内与其余成员平均重合度最高者）→ 代表图 = 与中心 `align ≥ 0.95` 的成员里**内容框最大**者（既典型又最全）；
4. 逐成员对**代表图**再算一次 `align ≥ 0.75`，不达标的退回池子交给会话（避免"对中心 0.76、对代表图 0.71"的图悄悄进组）；
5. 成组（≥2 张）就写 progress、`applied`，不再进会话。

实测该书：**78 张 → 18 组**，代表图照常各重画一次。

### 2.6 分桶 → 会话作用域（T142）

`motifBucketScopes(pool, advisory, bucketFull)`：

- `pool` 是自动定组之后**剩下的**图；`bucketFull` 是**自动定组之前**每个桶的大小；
- 一个桶该不该独占会话，看**原始桶大小** `> dedupe_group_session_over`（默认 50）——大桶里 0.75 以上的成员被本地定组拿掉后，"剩余十几张"不该因此被并进小桶批次（T142：用户看到的"最大的相似度组和别的图混在一个会话"就是按剩余数判断造成的）；
- 大桶独占一个会话（prefill = 整桶为一组）；小桶按 `dedupe_batch_images`（默认 150）合进同一会话；`dedupe_group_session_over ≤ 0` 则整池一个会话（旧行为）；
- 不属于任何桶的图 → `unassigned`，**不塞进**这些会话。
- 会话间互不依赖，按 `dedupe_concurrency`（缺席时 2，0/1 = 串行）并发；结果按 scope 顺序收回。

### 2.7 核验会话

每个会话是一个独立的模型会话，工作区挂载 `candidates`（只读：`manifest.md`、`similarity_advisory.json`、`originals/NNNNNN.<ext>`；写 `candidates:manifest.md` 这种 `名称:路径` 形式）。工具：`read_file` / `grep` / `bash`（沙箱 `/work`，可跑 Python）/ `view_image` / `compare_images` / `motif_groups` / `submit`。

- **必须逐张看原图**：`seen.json` 记录看过哪些 id；`compare_images` 一次并排 2–4 张（整数 id），合成图上每一格都算"看过"（T132）。没看过的成员不允许合并。
- `motif_groups` 增量维护 `groups.json`（`add/remove/update/clear/list`，`add` 必须带 `key`），桶只作为 `prefill` 起点，模型可拆可并（跨桶合并也行）。
- `submit` 终审后 runner 复核（`runMotifScope`）：
  1. **组类别** = 组内占比最大的 `class`（vector > raster > text 并列时取前者）；
  2. 组内成员逐个对**代表图**算 `align ≥ dedupe_align_min`（0.60），不达标的**保持独立**（宁可漏，不可误判）；
  3. 模型点名的 `key` 若自己不合格或内容框明显偏小，退回到"合格成员里内容框最大"；
  4. 代表图选了但最终无成品 → 跟随它的图会被解除跟随（`clearUnresolvedMotifLinks`），绝不让图从书里消失；
  5. 整组都不达标 → 整组不合并，只警告。
- 会话失败（API 错误、超时）只让这批图保持独立，不挡后续阶段。

### 2.8 整理会话

`motifConsolidationCandidates(unassigned, applied)`：先用本地相似度把 `unassigned` 与各已确认组的代表图配对，**只有真配上的**候选 + 关键图进会话（互不相似就完全不开）；会话里只出现这几张，不重看整组。跑完后同样走 2.7 的复核。

### 2.9 挂靠收口（T135）

`motifAttachLeftovers(leaders, applied)`：会话**看过并判为独立**的图不改判；但那些**压根没进会话**（本地没有候选、整理会话也没问过）的图，只要对某个已确认组的代表图 `align ≥ dedupe_attach_min`（默认 0.60）就直接挂进去，`source=attached`，代表图不变。

这条和 2.8 一起保证"0.6 以上连通 → 一个组"，同时不推翻模型已经做过的判断。

### 2.10 副本跟随

`motifAdoptCopies`：每个副本跟着自己的 leader——leader 被合并就跟随同一个 leader，leader 独立就跟随 leader 自己；同时写入 `dedupe_leader_hash` 做一致性校验（哈希对不上就作废重来）。

### 2.11 落盘与续跑

- `writeMotifArtifacts` 写 `groups.json`（含 `version`/`built_at`/`rules`/`refs`/`distinct`/`fingerprint`/`groups`/`independent`）与 `report.md`；
- `writeMotifMetrics` 写 `metrics/pairs.csv`（逐对 `feature_similar, align, align_level, canvas, skip, same_bucket`）与 `metrics/buckets.json`（桶、未分配、副本数）；
- 续跑三层：① `motifReuseDedupe` 指纹命中（2.2）→ 不解码直接沿用 `groups.json`；② 否则 progress 里 `dedupe_checked` 全为真 → 只补副本跟随；③ 中途中断的会话按 T121 回放历史继续（`seen.json` 落盘，看过的证据不丢）。

## 3. 阈值一览

| 配置 | 默认 | 作用 |
|------|------|------|
| `latex.dedupe_align_min` | 0.60 | 能不能进同一组（对组代表图）；也是 2.4 往桶里加边的门槛 |
| `latex.dedupe_icon_auto` | 0.75（0=关） | 小图标直接本地定组、不进会话 |
| `latex.dedupe_attach_min` | 0.60 | 没进会话的图挂靠到已确认组 |
| `latex.dedupe_align_typical` | 0.95 | 代表图候选：与中心"典型"的门槛 |
| `latex.dedupe_align_shift` | 0.10 | 允许平移 `0.10·S` |
| `latex.dedupe_align_grid` | `0.95S` | 最细一级的格数（固定整数或画布比例） |
| `latex.dedupe_icon_max_area` | 200000 | 参与居中重合的图上限（px²） |
| 内部常量 `motifAutoIconMaxArea` | 20000 | 自动定组的图上限（px²） |
| `latex.dedupe_bucket_max` | 400 | 超过就退回纯相似度分桶 |
| `latex.dedupe_group_session_over` | 50 | 桶原始大小超过它 → 独占会话 |
| `latex.dedupe_batch_images` | 150 | 单会话最多几张 |
| `latex.dedupe_concurrency` | 空缺=2 | 桶会话并发数 |

## 4. 实测（某测试书）

| 量 | 数 |
|----|----|
| 引用 / 不同内容 / 副本 | 388 / 130 / 258 |
| 逐对总数 | C(130,2) = 8385 |
| 没走居中重合（`too_large`） | 882 |
| 有 `align` 的对 | 7503（中位数 0.578；≥0.60 的 2983；≥0.75 的 232） |
| 纯相似度连出的桶 | 1 个 90 张 |
| 相似度 ∪ 重合度的桶 | `[102, 2]` + 未分配 26 |
| 重合度新补的边 | 817（桶内 265、跨桶 552） |
| 自动定组 | 78 张 → 18 组 |
| 核验会话 | 大桶剩 24 + 小桶 2 = 26 张；会话里的组最终 7 张（最低 0.611） |
| 整理会话 | 26 张未分配 × 19 个代表图，未归位任何一张 |
| 最终 | 19 组（85 份）+ 45 份独立；组内最低分 0.611–1.000 |
| 成本 | 覆盖率 191ms、逐对相似度 0.30s、重合度扫描 104s → 3.5s（T140）→ 2.3s（T143 去掉扩窗） |

**已知不完美（都是口径问题，不是 bug）**：

1. 画布 `S` 文档写的是池级 P90，生产代码实际按每对 `max(maxSide)`——两者在本书上都是 41 左右（图标尺寸集中），差异没暴露；要么把 `motifCanonCanvas` 接进生产，要么把文档改成实际口径。
2. 不缩放 ⇒ 一枚图标在两种字号下（差 ≥1.6×）判据会掉到 0.6 以下，靠会话/挂靠补，而不是靠判据。
3. 同一枚图标在本书被切成了 18 个自动组：它们是不同字号/不同笔画的近似变体，`align` 落在 0.75 以上但彼此不是同一张——`0.75` 只对"同一枚、同尺寸"可靠。
4. 小图标只有单层 G=32，逐级搜索在本书基本没被触发（大图才有 128/256）。
5. O(N²) 是硬上限：130 份 2.3s（T143 后），400 份就是 10 倍（所以有 `dedupe_bucket_max`）。
