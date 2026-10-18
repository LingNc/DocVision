package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// dedupe 的中间量以前算完就扔（用户：「每一步跑完的中间值和计算的内容……都要
// 留下来，逐对的那种用 csv 比较好方便查看」）。T136：把初始相似度分组、逐对
// 特征相似与居中重合、以及桶的最终归属落进 <proj>/source/dedupe/metrics/：
//
//	pairs.csv    逐对：feature_similar（旧判据，0/1）、align（居中重合，0–1，
//	             拿不到覆盖率时留空）、align_level/canvas/levels/shift_y/
//	             shift_x（判定层格数、画布边长、搜了几级、判定层上的平移格数）、
//	             same_bucket（最终是否同桶）、skip（没算分的原因，见下）
//	buckets.json 桶 → 成员、未分配，以及逐字节副本数
//
// skip 的取值：no_content_box（拿不到内容框，退回逐对相似度）、too_large
// （不是小图标，居中重合判据对它不适用）、prefilter（两者墨量占比或长宽比差
// 太远，桶扫描阶段就不算这一对）、pair_cap（预算用完，见 motifMetricsCostBudget）。
//
// 派生产物，写失败只警告；体积与份数成平方关系（130 份 ≈ 8385 行）。
//
// T151：这里以前对所有对**串行**算逐对明细，一次 130 份内容就要 26s（占整条
// 扫描的关键路径），而其中九成时间是那几百对"两边都是插图、画布 500 见方"的
// 组合——它们既不在同一个桶里，也过不了桶扫描的前筛。现在换成：同桶的对永远
// 算（复查时真正要看的就是这批，且桶里都是小图标，几乎不花钱）、跨桶的对先
// 过桶扫描同一套廉价前筛、剩下的按"画布²×级数"记账、逐行并行算。
// 返回的逐对分数供复查页直接用，不必再算第二遍。
//
// T153：cached 是上一次扫描落盘的逐对明细（scan.json 读回来的），命中就不重算
// 那一对，但仍按同一条记账走——同桶的照旧免费、跨桶的照旧先扣预算再算，所以
// pairs.csv 的内容与"全部重算"逐行相同；缓存里没有的对（预算挡掉的）保持
// skip=pair_cap，不会因为续跑凭空多出分数。返回本次真正算过/用过的明细，
// 供调用方写回缓存。
func (r *Runner) writeMotifMetrics(outDir string, pool []*motifCandidate, groups []VisualMotifGroup, unassigned []string, repeats int, cached map[string]motifScanPair) (map[string]float64, map[string]motifScanPair) {
	dir := filepath.Join(motifArtifactDir(outDir), "metrics")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.logWarning("[dedupe] metrics 目录创建失败:", err)
		return nil, nil
	}
	n := len(pool)
	keys := make([]string, n)
	full := make([]string, n)
	cov := make([]motifAlignImage, n)
	ok := make([]bool, n)
	feat := make([]motifFeatureSet, n)
	for i, c := range pool {
		full[i] = c.task.key()
		keys[i] = shortMotifKey(full[i])
		cov[i], ok[i] = c.coverage()
		feat[i] = c.features()
	}
	bucketOf := map[string]int{}
	for gi, g := range groups {
		for _, k := range g.Members {
			bucketOf[k] = gi + 1
		}
	}
	shift := r.motifAlignShift()
	// 复查页要按"独立图 vs 代表图"查分，所以两个方向都存。
	scores := make(map[string]float64, 2*n)
	scored := make(map[string]motifScanPair, 2*n)
	det := make([]motifAlignDetail, n)
	done := make([]bool, n)
	budget := motifMetricsCostBudget
	var b strings.Builder
	b.WriteString("a,b,feature_similar,align,align_level,canvas,levels,shift_y,shift_x,skip,same_bucket\n")
	for i := 0; i < n; i++ {
		// 一行分两批：同桶的必算，跨桶的先过前筛、再按预算。
		batch := make([]int, 0, 64)
		budgeted := make([]int, 0, 64)
		for j := i + 1; j < n; j++ {
			if !ok[i] || !ok[j] || cov[i].area > motifAlignIconMaxArea || cov[j].area > motifAlignIconMaxArea {
				continue
			}
			if bi, oki := bucketOf[full[i]]; oki && bi == bucketOf[full[j]] {
				batch = append(batch, j)
				continue
			}
			if !motifAlignWorthScore(cov[i], cov[j]) {
				continue
			}
			budgeted = append(budgeted, j)
		}
		for _, j := range budgeted {
			cost := motifAlignCost(cov[i], cov[j])
			if cost > budget {
				continue
			}
			budget -= cost
			batch = append(batch, j)
		}
		motifScorePairs(cov, i, motifScoreFresh(i, full, batch, cached, det, done), shift, det, done)
		for j := i + 1; j < n; j++ {
			fs := 0
			if motifFeatureSetSimilar(feat[i], feat[j]) {
				fs = 1
			}
			align, level, canvas, levels, shiftY, shiftX := "", 0, 0, 0, 0, 0
			skip := ""
			switch {
			case !ok[i] || !ok[j]:
				// 大图/无前景的图拿不到内容框，退回逐对相似度（T138 ⑥）。
				skip = "no_content_box"
			case cov[i].area > motifAlignIconMaxArea || cov[j].area > motifAlignIconMaxArea:
				skip = "too_large"
			case !done[j] && !motifAlignWorthScore(cov[i], cov[j]):
				skip = "prefilter"
			case !done[j]:
				skip = "pair_cap"
			}
			if done[j] {
				d := det[j]
				align = fmt.Sprintf("%.3f", d.Score)
				level, canvas, levels, shiftY, shiftX = d.Level, d.Canvas, d.Levels, d.ShiftY, d.ShiftX
				scores[motifPairRef(full[i], full[j])] = d.Score
				scores[motifPairRef(full[j], full[i])] = d.Score
				scored[motifPairRef(full[i], full[j])] = motifScanPairOf(d)
			}
			same := 0
			if bi, oki := bucketOf[full[i]]; oki && bi == bucketOf[full[j]] {
				same = 1
			}
			fmt.Fprintf(&b, "%s,%s,%d,%s,%d,%d,%d,%d,%d,%s,%d\n",
				keys[i], keys[j], fs, align, level, canvas, levels, shiftY, shiftX, skip, same)
		}
		for _, j := range batch {
			done[j] = false
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "pairs.csv"), []byte(b.String()), 0o644); err != nil {
		r.logWarning("[dedupe] pairs.csv 写盘失败:", err)
	}
	type bucket struct {
		ID      int      `json:"id"`
		Members []string `json:"members"`
	}
	doc := struct {
		Distinct int      `json:"distinct"`
		Repeats  int      `json:"byte_identical_copies"`
		Buckets  []bucket `json:"buckets"`
		Loose    []string `json:"unassigned"`
	}{Distinct: n, Repeats: repeats}
	for gi, g := range groups {
		bk := bucket{ID: gi + 1}
		for _, k := range g.Members {
			bk.Members = append(bk.Members, shortMotifKey(k))
		}
		sort.Strings(bk.Members)
		doc.Buckets = append(doc.Buckets, bk)
	}
	for _, k := range unassigned {
		doc.Loose = append(doc.Loose, shortMotifKey(k))
	}
	sort.Strings(doc.Loose)
	if data, err := json.MarshalIndent(doc, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "buckets.json"), append(data, '\n'), 0o644)
	}
	return scores, scored
}

// motifScoreFresh 把这一行挑出来的对分成"缓存里已有"与"要现算"两批：已有的
// 直接填回 det/done（预算已经在调用方按同一条规则扣过，所以分数集合与全量重算
// 完全一致），只有剩下的交给 motifScorePairs。
func motifScoreFresh(i int, full []string, batch []int, cached map[string]motifScanPair, det []motifAlignDetail, done []bool) []int {
	if len(cached) == 0 {
		return batch
	}
	fresh := make([]int, 0, len(batch))
	for _, j := range batch {
		if p, ok := cached[motifPairRef(full[i], full[j])]; ok {
			det[j] = p.detail()
			done[j] = true
			continue
		}
		fresh = append(fresh, j)
	}
	return fresh
}

// motifMetricsCostBudget 是 pairs.csv 逐对明细的记账上限（T151）。一对图的代价
// 实测约等于"画布边长² × 搜索级数"：38² 一级 ≈ 0.5ms，512² 四级 ≈ 19ms。
// 按这个权重记账，130 份内容全算约 0.94G（并行后 ≈ 3s），1000 张以上的书就
// 停在 1.2G 上不再涨——超出预算的对仍然写 feature_similar，align 留空并标
// skip=pair_cap，所以 CSV 永远读得懂，不会让扫描变成几分钟的黑盒等待。
const motifMetricsCostBudget = 1_200_000_000

// motifPairRef 是逐对分数的查表键（两个方向分别存）。
func motifPairRef(a, b string) string { return a + "\x00" + b }

// motifScorePairs 并行算一行里的那批逐对明细。比较是纯函数，结果与串行一致。
func motifScorePairs(cov []motifAlignImage, i int, js []int, shift int, det []motifAlignDetail, done []bool) {
	if len(js) == 0 {
		return
	}
	workers := runtime.NumCPU()
	if workers > motifAlignWorkers {
		workers = motifAlignWorkers
	}
	if workers > len(js) {
		workers = len(js)
	}
	var next int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				k := int(atomic.AddInt64(&next, 1)) - 1
				if k >= len(js) {
					return
				}
				j := js[k]
				det[j] = cov[i].overlapDetail(cov[j], shift)
				done[j] = true
			}
		}()
	}
	wg.Wait()
}

// shortMotifKey 把 "<md>::<img>" 这类 task key 压成文件名，CSV 里才看得清。
func shortMotifKey(k string) string {
	if i := strings.LastIndex(k, "|"); i >= 0 {
		k = k[i+1:]
	}
	return filepath.Base(k)
}
