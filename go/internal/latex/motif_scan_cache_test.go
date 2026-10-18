package latex

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// T153：读回扫描缓存必须与重新扫描**结论一致**——桶一样、逐对分数一样、
// pairs.csv 逐字节一样。缓存只是省时间，不允许改变任何结果。
func TestMotifScanCacheReuseIsOutputNeutral(t *testing.T) {
	r := testRunner(t, true)
	// 两张内容一致、两张内容不同：桶扫描会真的分组，逐对明细也会真的算分。
	pool := []*motifCandidate{
		{task: &task{mdName: "book.md", imgPath: "images/book/a.png"}, image: canonTestImage(120, 160, 0)},
		{task: &task{mdName: "book.md", imgPath: "images/book/b.png"}, image: canonTestImage(120, 160, 0)},
		{task: &task{mdName: "book.md", imgPath: "images/book/c.png"}, image: canonTestImage(120, 160, 1)},
		{task: &task{mdName: "book.md", imgPath: "images/book/d.png"}, image: canonTestImage(140, 160, 2)},
	}
	vis := make([]VisualMotifSample, 0, len(pool))
	hashes := make([]string, 0, len(pool))
	for i, c := range pool {
		vis = append(vis, VisualMotifSample{Key: c.task.key(), Image: c.image})
		hashes = append(hashes, string(rune('a'+i)))
	}
	freshDir := t.TempDir()
	groups, merges := FindVisualMotifCandidatesAligned(vis, r.motifAlignMin(), 10)
	if len(groups) == 0 {
		t.Fatal("这组样本应当扫出至少一个桶，测试前提不成立")
	}
	freshScoreMap, freshPairs := r.writeMotifMetrics(freshDir, pool, groups, nil, 0, nil)
	if len(freshPairs) == 0 {
		t.Fatal("这组样本应当至少算出一对分数，测试前提不成立")
	}

	key := r.motifScanKey(hashes)
	r.saveMotifScanCache(motifArtifactDir(freshDir), key, merges, groups, freshPairs)
	cache := r.loadMotifScanCache(motifArtifactDir(freshDir), key)
	if cache == nil {
		t.Fatal("刚写下的扫描缓存读不回来")
	}
	if cache.Merges != merges || len(cache.Buckets) != len(groups) {
		t.Fatalf("缓存里的桶不对：merges=%d/%d buckets=%d/%d", cache.Merges, merges, len(cache.Buckets), len(groups))
	}
	cached := cache.groupsFor(pool)
	if cached == nil {
		t.Fatal("桶的成员都在池子里，缓存不该作废")
	}
	for i := range groups {
		if len(cached[i].Members) != len(groups[i].Members) || cached[i].Exact != groups[i].Exact {
			t.Fatalf("第 %d 个桶还原不一致：%v vs %v", i+1, cached[i], groups[i])
		}
		for j := range groups[i].Members {
			if cached[i].Members[j] != groups[i].Members[j] {
				t.Fatalf("第 %d 个桶的成员顺序变了：%v vs %v", i+1, cached[i].Members, groups[i].Members)
			}
		}
	}

	// 第二遍：桶与逐对明细全部来自缓存。
	reuseDir := t.TempDir()
	reuseScores, reusePairs := r.writeMotifMetrics(reuseDir, pool, cached, nil, 0, cache.Pairs)
	freshCSV := readMotifMetricsFile(t, freshDir, "pairs.csv")
	reuseCSV := readMotifMetricsFile(t, reuseDir, "pairs.csv")
	if !bytes.Equal(freshCSV, reuseCSV) {
		t.Fatalf("沿用缓存后 pairs.csv 变了：\n全新：\n%s\n沿用：\n%s", freshCSV, reuseCSV)
	}
	if len(reusePairs) != len(freshPairs) {
		t.Fatalf("沿用缓存后逐对明细份数变了：%d vs %d", len(reusePairs), len(freshPairs))
	}
	for k, v := range freshPairs {
		if reusePairs[k] != v {
			t.Fatalf("沿用缓存后 %q 的明细变了：%+v vs %+v", k, reusePairs[k], v)
		}
	}
	if len(reuseScores) != len(freshScoreMap) {
		t.Fatalf("沿用缓存后逐对分数份数变了：%d vs %d", len(reuseScores), len(freshScoreMap))
	}
	for k, v := range freshScoreMap {
		if reuseScores[k] != v {
			t.Fatalf("沿用缓存后 %q 的分数变了：%v vs %v", k, reuseScores[k], v)
		}
	}
}

func readMotifMetricsFile(t *testing.T, outDir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(motifArtifactDir(outDir), "metrics", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// 判据参数变了，缓存必须失效：宁可重扫，也不能拿旧门槛的桶当结论。
func TestMotifScanCacheKeyFollowsCriteria(t *testing.T) {
	r := testRunner(t, true)
	outDir := t.TempDir()
	hashes := []string{"a", "b"}
	key := r.motifScanKey(hashes)
	r.saveMotifScanCache(motifArtifactDir(outDir), key, 0, nil, map[string]motifScanPair{})
	if r.loadMotifScanCache(motifArtifactDir(outDir), key) == nil {
		t.Fatal("同键应当命中")
	}
	if r.loadMotifScanCache(motifArtifactDir(outDir), key+"x") != nil {
		t.Fatal("换键必须不命中")
	}
	// 同一批内容的顺序不影响键（缓存只按不同内容跑）。
	if r.motifScanKey([]string{"b", "a", "a"}) != key {
		t.Fatal("键应当与摘要顺序、重复无关")
	}
	// 判据不同 → 键不同。
	prev := r.cfg.Latex.DedupeAlignMin
	r.cfg.Latex.DedupeAlignMin = 0.72
	defer func() { r.cfg.Latex.DedupeAlignMin = prev }()
	if r.motifScanKey(hashes) == key {
		t.Fatal("判据变了，键必须跟着变")
	}
}

// 缓存是派生产物：读不懂就当没有，绝不因为一个坏文件打断整本书。
func TestMotifScanCacheCorruptIsIgnored(t *testing.T) {
	r := testRunner(t, true)
	outDir := t.TempDir()
	dir := motifArtifactDir(outDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(motifScanCachePath(dir), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := r.loadMotifScanCache(dir, "whatever"); c != nil {
		t.Fatal("坏文件应当当作没有缓存")
	}
}
