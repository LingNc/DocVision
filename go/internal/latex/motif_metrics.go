package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dedupe 的中间量以前算完就扔（用户：「每一步跑完的中间值和计算的内容……都要
// 留下来，逐对的那种用 csv 比较好方便查看」）。T136：把初始相似度分组、逐对
// 特征相似与居中重合、以及桶的最终归属落进 <proj>/source/dedupe/metrics/：
//
//	pairs.csv    逐对：feature_similar（旧判据，0/1）、align（居中重合，0–1，
//	             拿不到覆盖率时留空）、same_bucket（最终是否同桶）
//	buckets.json 桶 → 成员、未分配，以及逐字节副本数
//
// 派生产物，写失败只警告；体积与份数成平方关系（130 份 ≈ 8385 行）。
func (r *Runner) writeMotifMetrics(outDir string, vis []VisualMotifSample, groups []VisualMotifGroup, unassigned []string, repeats int) {
	dir := filepath.Join(motifArtifactDir(outDir), "metrics")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.logWarning("[dedupe] metrics 目录创建失败:", err)
		return
	}
	n := len(vis)
	keys := make([]string, n)
	cov := make([]motifAlignImage, n)
	ok := make([]bool, n)
	feat := make([]motifFeatureSet, n)
	for i, s := range vis {
		keys[i] = shortMotifKey(s.Key)
		cov[i], ok[i] = motifAlignCoverage(s.Image)
		feat[i] = computeMotifFeatureSet(s.Image)
	}
	bucketOf := map[string]int{}
	for gi, g := range groups {
		for _, k := range g.Members {
			bucketOf[k] = gi + 1
		}
	}
	shift := r.motifAlignShift()
	var b strings.Builder
	b.WriteString("a,b,feature_similar,align,align_level,canvas,skip,same_bucket\n")
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			fs := 0
			if motifFeatureSetSimilar(feat[i], feat[j]) {
				fs = 1
			}
			align, level, canvas, skip := "", 0, 0, ""
			switch {
			case !ok[i] || !ok[j]:
				// 大图/无前景的图拿不到内容框，退回逐对相似度（T138 ⑥）。
				skip = "no_content_box"
			case cov[i].area > motifAlignIconMaxArea || cov[j].area > motifAlignIconMaxArea:
				skip = "too_large"
			default:
				s, g, c := cov[i].overlapDetail(cov[j], shift)
				align, level, canvas = fmt.Sprintf("%.3f", s), g, c
			}
			same := 0
			if bi, oki := bucketOf[vis[i].Key]; oki && bi == bucketOf[vis[j].Key] {
				same = 1
			}
			fmt.Fprintf(&b, "%s,%s,%d,%s,%d,%d,%s,%d\n", keys[i], keys[j], fs, align, level, canvas, skip, same)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "pairs.csv"), []byte(b.String()), 0o644); err != nil {
		r.logWarning("[dedupe] pairs.csv 写盘失败:", err)
		return
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
}

// shortMotifKey 把 "<md>|<img>" 这类 task key 压成文件名，CSV 里才看得清。
func shortMotifKey(k string) string {
	if i := strings.LastIndex(k, "|"); i >= 0 {
		k = k[i+1:]
	}
	return filepath.Base(k)
}
