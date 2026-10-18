package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
)

const motifDedupeVersion = "visual-motif-v2-tools"

// motifCandidate keeps a decoded input and its progress record together for
// one book-level verification pass.
type motifCandidate struct {
	task       *task
	progress   *imageProgress
	image      image.Image
	path       string
	sourceHash string
}

// motifDedupePhase is deliberately after image classification and before
// process. Similarity only proposes candidates; a vision session must confirm
// each non-byte-identical group before any drawing result is shared.
func (r *Runner) motifDedupePhase(tasks []*task, prog map[string]*imageProgress, progDir, outDir string) {
	progress := newMotifProgress(r.log, len(tasks))
	defer progress.finish()
	// 第一遍只读字节、算摘要，不解码（T141）：上一次的结论还完整、输入与规则
	// 也没变时，整段跳过——不再解码 388 张图、不再算几万对重合度、也不再回放
	// 会话。"续跑又要扫一遍候选"的代价因此从分钟级降到读一遍文件。
	type pending struct {
		task *task
		prog *imageProgress
		path string
		hash string
	}
	var pend []pending
	var hashed []motifHashedDedupe
	for ti, t := range tasks {
		progress.scan(ti+1, len(tasks))
		p := prog[t.key()]
		if p == nil || p.Class == "" || p.Absorbed {
			continue
		}
		path, err := resolveImageFile(r.cfg.Paths.ImagesDir, t.imgPath, subjectOf(t.mdName))
		if err != nil {
			r.log.LogWarning(0, "[dedupe] 候选图片不可读，保留独立处理:", t.imgPath, err)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			r.log.LogWarning(0, "[dedupe] 候选图片可读性检查失败，保留独立处理:", t.imgPath, err)
			continue
		}
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		pend = append(pend, pending{task: t, prog: p, path: path, hash: h})
		hashed = append(hashed, motifHashedDedupe{Key: t.key(), Hash: h})
	}
	if art, ok := r.motifReuseDedupe(outDir, prog, hashed); ok {
		r.log.Log(0, fmt.Sprintf("[dedupe] 输入与规则未变（指纹 %s），沿用 source/dedupe/groups.json：%d 份不同内容、%d 组，跳过扫描与核验会话",
			art.Fingerprint, art.Distinct, len(art.Groups)))
		progress.verify(1, 1, len(pend), 0)
		return
	}
	var samples []motifCandidate
	byKey := make(map[string]*motifCandidate)
	for _, it := range pend {
		f, err := os.Open(it.path)
		if err != nil {
			r.log.LogWarning(0, "[dedupe] 候选图片不可读，保留独立处理:", it.task.imgPath, err)
			continue
		}
		img, _, decErr := image.Decode(f)
		_ = f.Close()
		if decErr != nil {
			r.log.LogWarning(0, "[dedupe] 图片解码失败，保留独立处理:", it.task.imgPath, decErr)
			continue
		}
		p := it.prog
		if p.DedupeSourceHash != it.hash || p.DedupeVersion != motifDedupeVersion {
			p.EquivalentTo = ""
			p.DedupeLeaderHash = ""
			p.SharedTex = ""
			p.DedupeChecked = false
			p.DedupeSourceHash = it.hash
			p.DedupeVersion = motifDedupeVersion
			saveProgress(progDir, p)
		}
		c := &motifCandidate{task: it.task, progress: p, image: img, path: it.path, sourceHash: it.hash}
		samples = append(samples, *c)
		byKey[it.task.key()] = c
	}
	if len(samples) < 2 {
		return
	}
	for _, c := range byKey {
		p := c.progress
		if p.EquivalentTo == "" {
			continue
		}
		leader := byKey[p.EquivalentTo]
		if leader == nil || p.DedupeLeaderHash == "" || p.DedupeLeaderHash != leader.sourceHash {
			p.EquivalentTo = ""
			p.SharedTex = ""
			p.DedupeLeaderHash = ""
			p.DedupeChecked = false
			saveProgress(progDir, p)
		}
	}
	// Similarity is an advisory index only. The AI sees every classified image,
	// including images that the local detector failed to pair with a neighbour.
	pool := make([]*motifCandidate, 0, len(samples))
	for i := range samples {
		pool = append(pool, &samples[i])
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].task.key() < pool[j].task.key() })
	byTaskKey := make(map[string]*motifCandidate, len(pool))
	for _, c := range pool {
		byTaskKey[c.task.key()] = c
	}
	// 逐字节一致的图在书里常常出现几十次：同一张图被 PDF 引用 N 遍，只是提取
	// 时各拿了一个名字。字节完全相同就是同一张图片，既不用问模型、也不用看 N
	// 遍——只留一份进核验会话，其余副本跟着代表图的结论走。这里没有放宽相似度
	// （"宁可漏"针对的是要看图的相似判断），字节相同不存在看错的可能。
	pool, copies := motifContentLeaders(pool)
	repeats := 0
	for _, dups := range copies {
		repeats += len(dups)
	}
	vis := make([]VisualMotifSample, 0, len(pool))
	for _, s := range pool {
		vis = append(vis, VisualMotifSample{Key: s.task.key(), Image: s.image})
	}
	// 桶也用组内闸门那套"居中重合"判据（T130）：逐对相似度在小图标上会把
	// 一个连通片切两半（该书 90 张同标记进了桶、另外 10 张落进"未分配"），
	// 桶与闸门因此长期是两套结论。大插图/无前景的图拿不到覆盖率，仍只走逐对
	// 相似度，避免稀疏图形互相"有点像"造成误并。
	applyMotifMetricConfig(r)
	alignShiftPct := r.cfg.Latex.DedupeAlignShift
	if alignShiftPct <= 0 {
		alignShiftPct = motifAlignDefaultShift
	}
	groups, alignMerges := FindVisualMotifCandidatesAligned(vis, r.motifAlignMin(), alignShiftPct)
	if alignMerges > 0 {
		r.log.Log(0, fmt.Sprintf("[dedupe] 居中重合额外并进 %d 对（桶与组内闸门同一判据）", alignMerges))
	} else if alignMerges < 0 {
		r.log.LogWarning(0, fmt.Sprintf("[dedupe] 小图标超过 %d 张，桶退回逐对相似度判据（居中重合太慢）", motifAlignBucketMax))
	}
	advisory := make(map[string]motifAdvisory)
	for gi, group := range groups {
		for _, key := range group.Members {
			advisory[key] = motifAdvisory{Group: gi + 1, Exact: group.Exact}
		}
	}
	r.log.Log(0, fmt.Sprintf("[dedupe] 初始相似候选 %d 组，命中 %d / %d 张不同内容；逐字节重复 %d 张不进会话，自动跟随代表图",
		len(groups), len(advisory), len(pool), repeats))
	if len(pool) < 2 {
		// 全池只有一种内容：不需要核验，其余副本直接跟随这一张。
		if n := motifAdoptCopies(pool, copies, byTaskKey, progDir); n > 0 {
			r.log.Log(0, fmt.Sprintf("[dedupe] 全部候选图逐字节一致，%d 张跟随 %s", n, pool[0].task.key()))
		}
		return
	}
	allChecked := true
	for _, c := range pool {
		if !c.progress.DedupeChecked {
			allChecked = false
			break
		}
	}
	if allChecked {
		motifAdoptCopies(pool, copies, byTaskKey, progDir)
		progress.verify(1, 1, len(pool), 0)
		return
	}
	for _, c := range pool {
		c.progress.EquivalentTo = ""
		c.progress.DedupeLeaderHash = ""
		c.progress.SharedTex = ""
		c.progress.DedupeChecked = false
		saveProgress(progDir, c.progress)
	}
	// 已经"重合到几乎没有疑问"的小图标直接本地定组，不再送核验会话（T131，
	// 用户口径：≥ dedupe_icon_auto 自动、0.60–0.75 交会话）。逐字节副本在
	// 最后一步跟着代表图走，所以这里拿 leaders 而不是缩水后的 pool 去对账。
	leaders := pool
	autoGroups, remaining, autoMerged := r.motifAutoIconGroups(leaders, progDir)
	if len(autoGroups) > 0 {
		r.log.Log(0, fmt.Sprintf("[dedupe] 居中重合 ≥ %.2f 的 %d 张图标自动归为 %d 组，不进核验会话（%d 张代表图照常重画）",
			r.motifIconAutoMin(), autoMerged, len(autoGroups), len(autoGroups)))
		pool = remaining
	}
	// 桶的原始大小（自动定组之前），会话边界按它判（T142 见 motifBucketScopes）。
	bucketFull := map[int]int{}
	for _, c := range leaders {
		if a, ok := advisory[c.task.key()]; ok && a.Group > 0 {
			bucketFull[a.Group]++
		}
	}
	scopes, unassigned := r.motifBucketScopes(pool, advisory, bucketFull)
	// 中间量落盘（T136）：初始相似度分组 + 逐对特征相似/居中重合 + 桶归属。
	var loose []string
	for _, c := range unassigned {
		loose = append(loose, c.task.key())
	}
	r.writeMotifMetrics(outDir, vis, groups, loose, repeats)
	// 中间进度行的分母只数**真的开着的**核验会话：整理会话要不要开，得等
	// 这些会话交出代表图才知道，先算进去就等于预告一个可能不存在的第 M 步。
	steps := len(scopes)
	if steps > 0 {
		progress.verify(0, steps, len(pool), 0)
	}
	// 桶之间互不依赖（每个会话只看自己桶里的图、写自己的 groups.json），所以
	// 可以并发；上限来自 latex.dedupe_concurrency，0/负数 = 串行（旧行为）。
	// 并发不能改变结论：结果按 scope 顺序收回，整理会话仍等它们全部结束。
	limit := r.cfg.Latex.DedupeConcurrency
	if limit < 1 {
		limit = 1
	}
	var started, finished, failCount atomic.Int64
	results := runMotifScopes(scopes, limit, func(sc *motifScope) ([]motifAppliedGroup, bool) {
		n := started.Add(1)
		r.log.Log(0, fmt.Sprintf("[dedupe] 核验会话 %d/%d 开始：%d 张图（%d 个相似桶，并发上限 %d）",
			n, len(scopes), len(sc.Members), len(sc.Prefill), limit))
		got, ok := r.runMotifScope(sc, outDir)
		if !ok {
			failCount.Add(1)
		}
		progress.verify(int(finished.Add(1)), steps, len(pool), int(failCount.Load()))
		return got, ok
	})
	// applied keeps the groups that survived every gate, so the final
	// consolidation session is only offered real representatives.
	applied := append([]motifAppliedGroup{}, autoGroups...)
	for _, got := range results {
		applied = append(applied, got...)
	}
	failed := int(failCount.Load())
	// 绝大多数图片和别的图没有关系（桶是例外），所以未分配的图默认保持独立：
	// 只有本地相似度确实把它和某个已确认组的关键图连起来时，才值得再开一个
	// 整理会话，而且会话里只出现这些候选 + 关键图。
	candidates, keys := r.motifConsolidationCandidates(unassigned, applied)
	if len(candidates) > 0 && len(keys) > 0 {
		progress.verify(len(scopes), len(scopes)+1, len(pool), failed)
		r.log.Log(0, fmt.Sprintf("[dedupe] 整理会话：%d 张未分配图可能属于 %d 个已确认组（只比对关键图）", len(candidates), len(keys)))
		sc := &motifScope{Label: "dedupe-整理", Members: candidates, Keys: keys, KeyBase: len(candidates)}
		got, ok := r.runMotifScope(sc, outDir)
		if ok {
			applied = append(applied, got...)
		} else {
			failed++
		}
	} else if len(unassigned) > 0 {
		r.log.Log(0, fmt.Sprintf("[dedupe] %d 张未分配图与任何已确认组都不相似，保持独立处理", len(unassigned)))
	} else if len(applied) == 0 {
		r.log.Log(0, "[dedupe] 本次没有通过校验的分组，跳过整理会话；process 将逐图处理")
	}
	// T135：会话看过并判为独立的图**不改判**，但那些压根没进会话（本地没有
	// 候选、也没被整理会话问过）的图，若与某个已确认组的代表图重合度仍达标，
	// 就直接挂靠过去——它们是同一枚图标，只是几何差异让不缩放的判据差了一点，
	// 单独重画没有意义。
	if n, extra := r.motifAttachLeftovers(leaders, applied, progDir); n > 0 {
		r.log.Log(0, fmt.Sprintf("[dedupe] %d 张没进会话的图挂靠到已确认组（重合度 ≥ %.2f，记 source=attached）", n, r.motifAttachMin()))
		applied = append(applied, extra...)
	}
	// 会话进度行的 N/M 只说明"第 M 步走到第 N 步"，其中最后一步是整理会话
	// ——它依赖前面会话交出的 key，所以永远是串行、而且可能压根没开。T126
	// 的日志只剩 [dedupe 2/2]，看起来像"两个会话都跑完了"，实际是 0 组合并。
	// 这一行把实际发生的事写死：开了几个会话、合并了几组、几张图独立。
	consolSessions := 0
	if len(candidates) > 0 && len(keys) > 0 {
		consolSessions = 1
	}
	mergedGroups, mergedMembers := 0, 0
	for _, g := range applied {
		mergedGroups++
		mergedMembers += len(g.Members)
	}
	r.log.Log(0, fmt.Sprintf("[dedupe] 小结：核验会话 %d 个 + 整理会话 %d 个；合并 %d 组（%d 张不同内容共用代表图，其余 %d 张独立重画）",
		len(scopes), consolSessions, mergedGroups, mergedMembers, len(leaders)-mergedMembers))
	// 副本没有进会话（字节相同，进会话只是把同一张图重复看一遍），它们的
	// 结论在最后一步补上：跟着各自代表图走——代表图被合并就一起合并，代表
	// 图独立就一起独立，反正本来就是同一张图片。
	if n := motifAdoptCopies(leaders, copies, byTaskKey, progDir); n > 0 {
		r.log.Log(0, fmt.Sprintf("[dedupe] %d 张逐字节重复的图片跟随代表图，不重复作图", n))
	}
	r.writeMotifArtifacts(outDir, leaders, copies, applied, len(autoGroups))
	// 收尾的进度行说的是**实际开了几个会话**：整理会话没开就不该多算一格
	// （T126 的 [dedupe 2/2] 曾经看起来像"两个会话都跑完了"）。
	total := len(scopes) + consolSessions
	if total < 1 {
		total = 1
	}
	progress.verify(total, total, len(pool), failed)
}

// motifContentLeaders splits the pool into one representative per distinct file
// content. Byte equality is proof of identity here: the copies are the very same
// bytes, so inspecting/redrawing one of them covers all of them. Only the
// survivors of the AI verification ever reach a session.
func motifContentLeaders(pool []*motifCandidate) ([]*motifCandidate, map[*motifCandidate][]*motifCandidate) {
	copies := map[*motifCandidate][]*motifCandidate{}
	index := make(map[string]*motifCandidate, len(pool))
	leaders := make([]*motifCandidate, 0, len(pool))
	for _, c := range pool {
		if leader, ok := index[c.sourceHash]; ok {
			copies[leader] = append(copies[leader], c)
			continue
		}
		index[c.sourceHash] = c
		leaders = append(leaders, c)
	}
	return leaders, copies
}

// motifAdoptCopies links every byte-identical copy to whatever its representative
// settled on: the representative's leader when the AI merged it, the
// representative itself otherwise. A copy is never given a different verdict than
// its own bytes, and DedupeChecked is safe to set without a view because the
// representative's picture was inspected (or its independence accepted).
func motifAdoptCopies(leaders []*motifCandidate, copies map[*motifCandidate][]*motifCandidate, byTaskKey map[string]*motifCandidate, progDir string) int {
	linked := 0
	for _, rep := range leaders {
		dups := copies[rep]
		if len(dups) == 0 {
			continue
		}
		leader := rep
		if rep.progress.EquivalentTo != "" {
			if l := byTaskKey[rep.progress.EquivalentTo]; l != nil {
				leader = l
			}
		}
		for _, c := range dups {
			c.progress.EquivalentTo = leader.task.key()
			c.progress.DedupeLeaderHash = leader.sourceHash
			c.progress.DedupeVersion = motifDedupeVersion
			c.progress.DedupeSourceHash = c.sourceHash
			c.progress.DedupeChecked = true
			saveProgress(progDir, c.progress)
			linked++
		}
	}
	return linked
}

// motifAttachLeftovers 把"没进过会话、也没有等价结论"的图和已确认组的代表图
// 比一遍，达标的挂靠过去（T135）。只认会话确认过的 key 作锚点，一次只挂一个
// 组（取最高分，并列时按 applied 顺序先出现者），所以不存在两组合并成环的
// 问题；会话做过判断的图（DedupeChecked）一律跳过——模型看过的东西不推翻。
func (r *Runner) motifAttachLeftovers(leaders []*motifCandidate, applied []motifAppliedGroup, progDir string) (int, []motifAppliedGroup) {
	min := r.motifAttachMin()
	if min <= 0 {
		return 0, nil
	}
	type anchor struct {
		key *motifCandidate
		cov motifAlignImage
		cls string
	}
	var anchors []anchor
	grouped := map[*motifCandidate]bool{}
	for _, g := range applied {
		if g.Key == nil {
			continue
		}
		for _, m := range g.Members {
			grouped[m] = true
		}
		grouped[g.Key] = true
		if cov, ok := motifAlignCoverage(g.Key.image); ok {
			anchors = append(anchors, anchor{key: g.Key, cov: cov, cls: g.Class})
		}
	}
	if len(anchors) == 0 {
		return 0, nil
	}
	shift := r.motifAlignShift()
	var extra []motifAppliedGroup
	attached := 0
	for _, c := range leaders {
		if c == nil || c.image == nil || c.progress == nil {
			continue
		}
		if grouped[c] || c.progress.DedupeChecked || c.progress.EquivalentTo != "" {
			continue
		}
		cov, ok := motifAlignCoverage(c.image)
		if !ok || cov.area > motifAlignIconMaxArea {
			continue
		}
		var best *anchor
		bestScore := min
		for i := range anchors {
			v := cov.overlap(anchors[i].cov, shift)
			if v > bestScore || (best == nil && v >= min) {
				best, bestScore = &anchors[i], v
			}
		}
		if best == nil {
			continue
		}
		c.progress.EquivalentTo = best.key.task.key()
		c.progress.DedupeLeaderHash = best.key.sourceHash
		c.progress.DedupeVersion = motifDedupeVersion
		c.progress.DedupeSourceHash = c.sourceHash
		c.progress.DedupeChecked = true
		saveProgress(progDir, c.progress)
		extra = append(extra, motifAppliedGroup{Key: best.key, Members: []*motifCandidate{c}, Class: best.cls, Source: "attached"})
		attached++
	}
	return attached, extra
}

// motifAutoIconGroups 把"重合到几乎没有疑问"的小图标直接本地合并成组，不再
// 送核验会话（T131）。用户口径：≥ `latex.dedupe_icon_auto`（默认 0.75）自动
// 定组，0.60–0.75 之间仍进会话逐张核验——分数在 0.6 附近是连续铺开的，不设
// 这个更高的自动门槛就等于"一张都不给模型看"。
//
// 不看 class（T134）：vector/raster/text 标签对 dedupe 没有参考价值，图标这类
// 内容在 classify 里本来就有随机性；组的类别只取组内占比最大的 class，供后续
// 阶段参考。代表图与组内闸门同一规则——在"典型成员"（与种子重合度 ≥
// motifAlignTypical）里取内容框最大者。
func (r *Runner) motifAutoIconGroups(cands []*motifCandidate, progDir string) (applied []motifAppliedGroup, remaining []*motifCandidate, merged int) {
	min := r.motifIconAutoMin()
	if min <= 0 {
		return nil, cands, 0
	}
	shift := r.motifAlignShift()
	cov := make([]motifAlignImage, len(cands))
	ok := make([]bool, len(cands))
	for i, c := range cands {
		// 不看 class（T134，用户口径）：vector/raster/text 对"是不是同一枚图标"
		// 没有参考价值，classify 本身带随机性；它只在最后决定"这个组按哪类
		// 处理"，不能当成能不能合并的条件。
		if c == nil || c.image == nil || c.progress == nil {
			continue
		}
		v, good := motifAlignCoverage(c.image)
		// 自动合并只给"真的小"的图标：内容框 ≤ motifAutoIconMaxArea（约 141×141）。
		// 再大就是插图了，两个相似的插图直接合并没经过任何审校，风险不对等
		// ——它们照旧进核验会话。
		if good && v.area <= motifAutoIconMaxArea {
			cov[i], ok[i] = v, true
		}
	}
	// 先按 ≥min 连成连通片（单链，和桶的判据一致），再对每个片选代表图、
	// 逐成员对代表图复核——这样同一枚标记不会因为"种子选得偏"被切成好几个组
	// （实测该书 100 张会切 79/11/5/4 四段，等于多画 3 张）。
	parent := make([]int, len(cands))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for i := 0; i < len(cands); i++ {
		if !ok[i] {
			continue
		}
		for j := i + 1; j < len(cands); j++ {
			if !ok[j] {
				continue
			}
			if cov[i].overlap(cov[j], shift) < min {
				continue
			}
			ri, rj := find(i), find(j)
			if ri != rj {
				parent[rj] = ri
			}
		}
	}
	groups := map[int][]int{}
	for i := range cands {
		if ok[i] {
			root := find(i)
			groups[root] = append(groups[root], i)
		}
	}
	used := make([]bool, len(cands))
	for i := 0; i < len(cands); i++ {
		if !ok[i] {
			continue
		}
		root := find(i)
		members, seen := groups[root], used[root]
		if seen || len(members) < 2 {
			continue
		}
		used[root] = true
		// 代表图：典型成员（对种子片里任意成员都算"典型"用中心口径太绕，这里
		// 直接用"与片内其他成员平均重合度最高"的成员当中心，再在 ≥0.95 的
		// 那一批里取内容框最大者）——与组内闸门同一套规则。
		center, bestMean := -1, -1.0
		for _, m := range members {
			sum, n := 0.0, 0
			for _, o := range members {
				if o == m {
					continue
				}
				sum += cov[m].overlap(cov[o], shift)
				n++
			}
			if n > 0 && sum/float64(n) > bestMean {
				bestMean, center = sum/float64(n), m
			}
		}
		if center < 0 {
			continue
		}
		leader, bestArea, bestScore := -1, -1, -1.0
		for _, m := range members {
			v := cov[m].overlap(cov[center], shift)
			if v < motifAlignTypical {
				continue
			}
			if cov[m].area > bestArea || (cov[m].area == bestArea && v > bestScore) {
				leader, bestArea, bestScore = m, cov[m].area, v
			}
		}
		if leader < 0 {
			leader = center
		}
		group := make([]*motifCandidate, 0, len(members))
		for _, m := range members {
			if m != leader && cov[m].overlap(cov[leader], shift) < min {
				continue // 对代表图不达标的退回池子，交给会话
			}
			group = append(group, cands[m])
			used[m] = true
		}
		if len(group) < 2 {
			for _, m := range members {
				used[m] = false
			}
			continue
		}
		applyMotifEquivalenceWithKey(group, cands[leader], progDir)
		applied = append(applied, motifAppliedGroup{Key: cands[leader], Members: group, Class: ClassVector})
		merged += len(group)
	}
	remaining = make([]*motifCandidate, 0, len(cands))
	for i, c := range cands {
		if !ok[i] || !used[i] {
			remaining = append(remaining, c)
		}
	}
	return applied, remaining, merged
}

// motifAppliedGroup is a confirmed group: only Key is redrawn, every other
// member reuses that drawing.
type motifAppliedGroup struct {
	Key     *motifCandidate
	Members []*motifCandidate
	Class   string
	// Source 是这组的来源，空 = 按位置推断（autoGroups 之前 = auto，其余 =
	// session）；"attached" 是 T135 的挂靠组（本地重合度达标、没进会话）。
	Source string
}

// motifProgressDir is the single place 档位1 stores per-image progress.
// outDir is the project's source directory (<proj>/source) — the same value
// classify and process use — so the progress files sit directly under it.
// It used to be filepath.Dir(outDir), one level too high: dedupe verdicts
// landed in <proj>/progress_items where process never reads them, so a
// resumed run re-verified every image instead of resuming (T120).
func motifProgressDir(outDir string) string {
	return filepath.Join(outDir, "progress_items")
}

// runMotifScopes runs the bucket sessions, at most limit at a time, and returns
// their groups index-aligned with scopes (nil for a session that failed). limit
// <= 1 runs them one after another. Completion order must never leak into the
// result: the downstream consolidation session walks `applied`, and identical
// input has to produce identical sessions.
func runMotifScopes(scopes []*motifScope, limit int, run func(*motifScope) ([]motifAppliedGroup, bool)) [][]motifAppliedGroup {
	out := make([][]motifAppliedGroup, len(scopes))
	if len(scopes) == 0 {
		return out
	}
	if limit <= 1 {
		for i, sc := range scopes {
			if got, ok := run(sc); ok {
				out[i] = got
			}
		}
		return out
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, limit)
	for i, sc := range scopes {
		wg.Add(1)
		go func(i int, sc *motifScope) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if got, ok := run(sc); ok {
				out[i] = got // 每个 goroutine 只写自己的槽位
			}
		}(i, sc)
	}
	wg.Wait()
	return out
}

// runMotifScope runs one session and applies whatever survives the gates.
func (r *Runner) runMotifScope(sc *motifScope, outDir string) ([]motifAppliedGroup, bool) {
	verdict, err := r.verifyMotifScope(sc, outDir)
	if err != nil {
		r.log.LogWarning(0, "[dedupe] 核验会话失败，该批图片保持独立；process 将逐图处理:", err)
		return nil, false
	}
	progDir := motifProgressDir(outDir)
	for _, c := range sc.Members {
		c.progress.DedupeChecked = true
		saveProgress(progDir, c.progress)
	}
	var out []motifAppliedGroup
	for _, g := range verdict {
		cands := make([]*motifCandidate, 0, len(g.Members))
		valid := true
		for _, id := range g.Members {
			c := motifScopeCandidate(sc, id)
			if c == nil {
				valid = false
				break
			}
			cands = append(cands, c)
		}
		if !valid {
			continue
		}
		key := motifScopeCandidate(sc, g.Key)
		// 组的类别按组内占比最大的 classify 结果判定（用户规则）。classify 对
		// 同一张图会给出不同类别，混类不代表"不是同一张图"，整组作废会让真正
		// 的重复图白白各画一遍——真实事故：336 张同图被整组拒掉。
		groupClass := motifGroupClass(cands)
		if groupClass != ClassVector {
			// 多数不是矢量图：这些图不需要重画，但组里的矢量图仍然值得合并成
			// 一张，否则同一张图会被画 N 次。
			vec := motifVectorSubset(cands)
			if len(vec) < 2 {
				r.log.Log(0, fmt.Sprintf("[dedupe] 组 %v 的多数类别是 %s，不需要重画，保持独立", g.Members, groupClass))
				continue
			}
			r.log.Log(0, fmt.Sprintf("[dedupe] 组 %v 的多数类别是 %s，其中 %d 张矢量图合并为一张重画，其余 %d 张保持独立",
				g.Members, groupClass, len(vec), len(cands)-len(vec)))
			cands = vec
			if key == nil || !motifContains(vec, key) {
				key = vec[0]
			}
		} else if !motifGroupAllVector(cands) {
			r.log.Log(0, fmt.Sprintf("[dedupe] 组 %v 的多数类别是矢量图，其中 %d 张非矢量图也按同一张图合并", g.Members, nonVectorCount(cands)))
		}
		// 组内闸门（T129，用户口径：宁漏不误判，但"重合得上"的要合）。
		// 以组中心为基准比较"内容框居中 + 平移搜索"的软重合度：达标的成员
		// 共用一张成品，不达标的保持独立。旧的逐对相似度在 28×38 这种小图上
		// 极其脆弱——中心差半个像素，硬 mask 的 IoU 就从 0.9 掉到 0.3，实测
		// 同一枚标记只认了 21/90；居中重合后同一批图最差 0.647。
		safe, chosen, dropped, worst, aligned := r.motifAlignFilter(cands, key)
		if chosen != nil {
			key = chosen
		}
		if aligned {
			r.log.Log(0, fmt.Sprintf("[dedupe] 组 %v 居中重合校验：合并 %d 张（最差重合度 %.2f），%d 张不足保持独立",
				g.Members, len(safe), worst, dropped))
		} else if dropped > 0 {
			r.log.LogWarning(0, fmt.Sprintf("[dedupe] 组 %v 里有 %d 张与代表图相似度不足，已保持独立（宁可漏掉重复，也不能合并后丢信息）", g.Members, dropped))
		}
		if len(safe) < 2 {
			r.log.LogWarning(0, fmt.Sprintf("[dedupe] 组 %v（%d 张）与代表图的相似度都不够，整组不合并，其中 %d 张保持独立", g.Members, len(cands), len(cands)))
			continue
		}
		applyMotifEquivalenceWithKey(safe, key, progDir)
		cls := ""
		if key != nil && key.progress != nil {
			cls = key.progress.Class
		}
		out = append(out, motifAppliedGroup{Key: key, Members: safe, Class: cls})
	}
	return out, true
}

func motifScopeCandidate(sc *motifScope, id int) *motifCandidate {
	if id <= 0 {
		return nil
	}
	if id <= len(sc.Members) {
		return sc.Members[id-1]
	}
	if k := id - len(sc.Members) - 1; k >= 0 && k < len(sc.Keys) {
		return sc.Keys[k]
	}
	return nil
}

// motifConsolidationCandidates pairs unassigned images with confirmed group
// keys using the same local similarity as the bucket detector. In a normal book
// almost nothing is similar to anything else, so this usually returns nothing
// and no extra session runs at all.
func (r *Runner) motifConsolidationCandidates(unassigned []*motifCandidate, applied []motifAppliedGroup) ([]*motifCandidate, []*motifCandidate) {
	return motifConsolidationCandidatesWith(unassigned, applied, r.motifMergeAllowed)
}

// motifConsolidationCandidates 是严格版（只认逐对相似度）：给测试与"没有配置"
// 的调用方保留的入口；真正跑流程走 (*Runner).motifConsolidationCandidates，
// 它多认一条"居中重合"判据（T129：会话外的 10 张同标记图片就是靠这条才被
// 送进整理会话的）。
func motifConsolidationCandidates(unassigned []*motifCandidate, applied []motifAppliedGroup) ([]*motifCandidate, []*motifCandidate) {
	return motifConsolidationCandidatesWith(unassigned, applied, func(a, b *motifCandidate) bool {
		return MotifSimilarTo(a.image, b.image)
	})
}

func motifConsolidationCandidatesWith(unassigned []*motifCandidate, applied []motifAppliedGroup, allow func(a, b *motifCandidate) bool) ([]*motifCandidate, []*motifCandidate) {
	if len(unassigned) == 0 || len(applied) == 0 {
		return nil, nil
	}
	keys := make([]*motifCandidate, 0, len(applied))
	seen := map[*motifCandidate]bool{}
	for _, g := range applied {
		if g.Key == nil || seen[g.Key] {
			continue
		}
		seen[g.Key] = true
		keys = append(keys, g.Key)
	}
	var cands []*motifCandidate
	for _, c := range unassigned {
		for _, k := range keys {
			if allow(c, k) {
				cands = append(cands, c)
				break
			}
		}
	}
	if len(cands) == 0 {
		return nil, nil
	}
	return cands, keys
}

// motifBucketScopes decides which images each verification session gets.
// A bucket larger than latex.dedupe_group_session_over owns its session; smaller
// buckets are packed together while a session stays within
// latex.dedupe_batch_images. Images in no bucket are NOT put into those sessions:
// in a normal book most figures are unrelated, so they are unassigned and only
// checked once against the confirmed keys (see motifConsolidationCandidates).
func (r *Runner) motifBucketScopes(pool []*motifCandidate, advisory map[string]motifAdvisory, full map[int]int) ([]*motifScope, []*motifCandidate) {
	over := r.cfg.Latex.DedupeGroupSessionOver
	perBatch := r.cfg.Latex.DedupeBatchImages
	if perBatch <= 0 {
		perBatch = 150
	}
	if over <= 0 {
		sc := &motifScope{Members: pool, Advisory: advisory, Prefill: motifPrefillBuckets(pool, advisory)}
		return []*motifScope{sc}, nil
	}
	buckets := map[int][]*motifCandidate{}
	var nos []int
	var unassigned []*motifCandidate
	for _, c := range pool {
		if a, ok := advisory[c.task.key()]; ok && a.Group > 0 {
			if _, seen := buckets[a.Group]; !seen {
				nos = append(nos, a.Group)
			}
			buckets[a.Group] = append(buckets[a.Group], c)
			continue
		}
		unassigned = append(unassigned, c)
	}
	sort.Ints(nos)
	var scopes []*motifScope
	var pending []*motifCandidate
	var pendingGroups []int
	flush := func() {
		if len(pending) == 0 {
			return
		}
		idx := map[*motifCandidate]int{}
		for i, c := range pending {
			idx[c] = i + 1
		}
		sc := &motifScope{Members: pending, Advisory: advisory}
		for _, g := range pendingGroups {
			ids := make([]int, 0, len(buckets[g]))
			for _, c := range buckets[g] {
				if id, ok := idx[c]; ok {
					ids = append(ids, id)
				}
			}
			if len(ids) >= 2 {
				sort.Ints(ids)
				sc.Prefill = append(sc.Prefill, MotifGroup{Members: ids})
			}
		}
		scopes = append(scopes, sc)
		pending, pendingGroups = nil, nil
	}
	// 一个桶该不该独占会话，看的是它在**自动定组之前**有多大（T142）：大桶里
	// 重合度 ≥ dedupe_icon_auto 的成员会被本地定组、从会话池里拿掉，剩下的十
	// 几张按"剩余数量"看就不再是大桶，于是被并进小桶的批次——用户看到的是
	// "最大的那个相似度组和别的图混在同一个会话里"，本该单独出现的第二个会话
	// 也没了。按原始桶大小判断，大桶永远独占自己的会话（剩余为 0 就跳过）。
	width := func(g int) int {
		if n, ok := full[g]; ok {
			return n
		}
		return len(buckets[g])
	}
	for _, g := range nos {
		members := buckets[g]
		if len(members) == 0 {
			continue
		}
		if width(g) > over {
			flush()
			ids := make([]int, len(members))
			for i := range members {
				ids[i] = i + 1
			}
			scopes = append(scopes, &motifScope{Members: members, Advisory: advisory, Prefill: []MotifGroup{{Members: ids}}})
			continue
		}
		if len(pending)+len(members) > perBatch {
			flush()
		}
		pending = append(pending, members...)
		pendingGroups = append(pendingGroups, g)
	}
	flush()
	return scopes, unassigned
}

// motifGroupClass 返回组内成员占比最大的 classify 类别（并列时按
// vector > raster > text 取前者，保证有作图能力时优先合并）。
func motifGroupClass(members []*motifCandidate) string {
	counts := map[string]int{}
	for _, c := range members {
		if c == nil || c.progress == nil {
			continue
		}
		counts[c.progress.Class]++
	}
	best, bestN := "", 0
	for _, cls := range []string{ClassVector, ClassRaster, ClassText} {
		if counts[cls] > bestN {
			best, bestN = cls, counts[cls]
		}
	}
	return best
}

func motifVectorSubset(members []*motifCandidate) []*motifCandidate {
	out := make([]*motifCandidate, 0, len(members))
	for _, c := range members {
		if c != nil && c.progress != nil && c.progress.Class == ClassVector {
			out = append(out, c)
		}
	}
	return out
}

func nonVectorCount(members []*motifCandidate) int {
	n := 0
	for _, c := range members {
		if c == nil || c.progress == nil || c.progress.Class != ClassVector {
			n++
		}
	}
	return n
}

func motifGroupAllVector(members []*motifCandidate) bool {
	if len(members) < 2 {
		return false
	}
	for _, c := range members {
		if c == nil || c.progress == nil || c.progress.Class != ClassVector {
			return false
		}
	}
	return true
}

func applyMotifEquivalence(members []*motifCandidate, progDir string) {
	applyMotifEquivalenceWithKey(members, nil, progDir)
}

// applyMotifEquivalenceWithKey makes exactly one member the shared drawing.
// When the AI named a key, that member is the one process will draw and every
// other member only references its output — "process 只处理最模板的那张，其余
// 位置全部替换成这张". Falling back to the old stability ordering keeps resumed
// runs deterministic when no key is available.
func applyMotifEquivalenceWithKey(members []*motifCandidate, key *motifCandidate, progDir string) {
	if len(members) < 2 {
		return
	}
	leader := key
	if leader == nil || !motifContains(members, leader) {
		sort.Slice(members, func(i, j int) bool {
			a, b := members[i], members[j]
			aVector, bVector := a.progress.Class == ClassVector, b.progress.Class == ClassVector
			if aVector != bVector {
				return aVector
			}
			aDone := a.progress.Status == "done" && a.progress.FigPDF != ""
			bDone := b.progress.Status == "done" && b.progress.FigPDF != ""
			if aDone != bDone {
				return aDone
			}
			return a.task.key() < b.task.key()
		})
		leader = members[0]
	}
	for _, c := range members {
		if c == leader {
			c.progress.EquivalentTo = ""
			c.progress.DedupeLeaderHash = ""
		} else {
			c.progress.EquivalentTo = leader.task.key()
			c.progress.DedupeLeaderHash = leader.sourceHash
		}
		c.progress.DedupeVersion = motifDedupeVersion
		c.progress.DedupeSourceHash = c.sourceHash
		c.progress.DedupeChecked = true
		saveProgress(progDir, c.progress)
	}
}

func motifContains(members []*motifCandidate, want *motifCandidate) bool {
	for _, c := range members {
		if c == want {
			return true
		}
	}
	return false
}

// applyMotifArtifacts materializes each confirmed follower from its leader
// without deleting the follower's task, image reference, or progress record.
func applyMotifArtifacts(tasks []*task, prog map[string]*imageProgress, progDir, outDir string) {
	for _, t := range tasks {
		p := prog[t.key()]
		if p == nil || (p.Status == "done" && p.Class == ClassVector && p.TikzCode != "") || p.EquivalentTo == "" {
			continue
		}
		leader := prog[p.EquivalentTo]
		if leader == nil || leader.EquivalentTo != "" || leader.Class != ClassVector || leader.Status != "done" || leader.TikzCode == "" || leader.FigPDF == "" || !fileExists(filepath.Join(outDir, filepath.FromSlash(leader.FigPDF))) {
			continue
		}
		p.Class = ClassVector
		p.Label = leader.Label
		p.TikzCode = leader.TikzCode
		p.FigPDF = leader.FigPDF
		p.FigPNG = leader.FigPNG
		p.FigSVG = leader.FigSVG
		p.Status = "done"
		p.Kept = false
		p.SVGFail = leader.SVGFail
		p.Error = ""
		saveProgress(progDir, p)
	}
}

// clearUnresolvedMotifLinks drops follower->leader links whose leader never
// produced a drawing (failed, or produced nothing). Followers are otherwise
// skipped by process forever and their figure would silently disappear from the
// book; unlinking means the next run draws them independently. Costing an extra
// drawing is fine, losing a figure is not.
func (r *Runner) clearUnresolvedMotifLinks(tasks []*task, prog map[string]*imageProgress, progDir string) {
	cleared := 0
	for _, t := range tasks {
		p := prog[t.key()]
		if p == nil || p.EquivalentTo == "" {
			continue
		}
		leader := prog[p.EquivalentTo]
		if leader != nil && leader.Status == "done" && leader.TikzCode != "" && leader.EquivalentTo == "" {
			continue
		}
		p.EquivalentTo = ""
		p.DedupeLeaderHash = ""
		p.SharedTex = ""
		saveProgress(progDir, p)
		cleared++
	}
	if cleared > 0 {
		// 不静默：这些图会保持/回退为独立处理，本次不假装已完成。
		r.log.LogWarning(0, fmt.Sprintf("%d 张图跟随的代表图没有产出成品，已解除跟随，下次运行将单独处理", cleared))
	}
}

// materializeSharedMotifTeX places a raw (not standalone) drawing body in
// figures/, which is already staged by chapter workspaces and assembly.
// A failed write leaves the original inline code instead of a broken reference.
func materializeSharedMotifTeX(tasks []*task, prog map[string]*imageProgress, progDir, outDir string) {
	groups := map[string][]*imageProgress{}
	for _, t := range tasks {
		p := prog[t.key()]
		if p == nil || p.EquivalentTo == "" || p.Status != "done" {
			continue
		}
		leader := prog[p.EquivalentTo]
		if leader == nil || leader.Status != "done" || leader.TikzCode == "" || leader.EquivalentTo != "" {
			continue
		}
		groups[leader.Key] = append(groups[leader.Key], p)
	}
	valid := map[string]bool{}
	for key, followers := range groups {
		valid[key] = true
		for _, p := range followers {
			valid[p.Key] = true
		}
	}
	for _, t := range tasks {
		p := prog[t.key()]
		if p != nil && p.SharedTex != "" && !valid[p.Key] {
			p.SharedTex = ""
			saveProgress(progDir, p)
		}
	}
	for key, followers := range groups {
		leader := prog[key]
		sum := sha256.Sum256([]byte(key))
		rel := "figures/motif_" + hex.EncodeToString(sum[:8]) + ".tex"
		if err := os.WriteFile(filepath.Join(outDir, filepath.FromSlash(rel)), []byte(leader.TikzCode+"\n"), 0o644); err != nil {
			leader.SharedTex = ""
			saveProgress(progDir, leader)
			for _, p := range followers {
				p.SharedTex = ""
				saveProgress(progDir, p)
			}
			continue
		}
		leader.SharedTex = rel
		saveProgress(progDir, leader)
		for _, p := range followers {
			p.SharedTex = rel
			saveProgress(progDir, p)
		}
	}
}

func parseMotifGroups(reply string, count int) ([][]int, error) {
	obj, err := parseJSONObject(reply)
	if err != nil {
		return nil, err
	}
	raw, ok := obj["groups"].([]any)
	if !ok {
		return nil, fmt.Errorf("AI response omitted groups")
	}
	seen := map[int]bool{}
	var out [][]int
	for _, item := range raw {
		values, ok := item.([]any)
		if !ok || len(values) == 0 {
			return nil, fmt.Errorf("invalid equivalence group")
		}
		ids := make([]int, 0, len(values))
		for _, value := range values {
			n, ok := value.(float64)
			id := int(n)
			if !ok || float64(id) != n || id < 1 || id > count || seen[id] {
				return nil, fmt.Errorf("invalid or duplicate image index")
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) > 1 {
			out = append(out, ids)
		}
	}
	return out, nil
}
