package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// T153：桶扫描与逐对明细是整条 dedupe 扫描里最贵的两段——130 份不同内容实测
// 7.0 秒中有 5.5 秒花在这里（桶扫描 2.3s、逐对明细 3.4s）。而这两段的结果只
// 取决于「输入的内容摘要集合 + 判据参数」：与转录、会话、progress 里的结论都
// 无关。所以把它们按下标落进 <proj>/source/dedupe/scan.json，续跑时只要键对得
// 上就直接读回来——
//
//	· 桶不再重扫（跳过 FindVisualMotifCandidatesAligned）；
//	· 逐对明细不再重算（writeMotifMetrics 只补算缓存里没有的对）；
//	· 续跑从 7.0s 降到 ~1.3s（剩下的只有读字节、解码、覆盖率、特征与复查页）。
//
// 缓存必须**结论中立**：读回来走的是同一条记账路径（同桶的仍不记账、跨桶的仍
// 先过前筛再按 motifMetricsCostBudget 扣），所以 pairs.csv 与一次全新扫描逐行
// 相同；预算挡掉的对在缓存里本来就没有分数，续跑依旧是 skip=pair_cap。
//
// 键（motifScanKey）把影响扫描结果的判据全算进去：版本、判据门槛、平移、
// 自动定组、典型成员门槛、最细层格数规格、小图标面积上限、桶容量上限，加上
// 排序去重后的内容摘要。任何一条变了就当作没有缓存——宁可重扫，不给旧结论。
// 文件里的 format 只负责形状（字段增删），与判据无关。
const motifScanCacheFormat = 1

// motifScanCacheName 是缓存文件名（与 groups.json/report.md/review.html 同级）。
const motifScanCacheName = "scan.json"

// motifScanCache 是扫描中间量的落盘形状。Buckets 与 Pairs 都是"按下标算过
// 一次"的原始结果，不含任何会话结论。
type motifScanCache struct {
	Format  int    `json:"format"`
	Version string `json:"version"`
	Key     string `json:"key"`
	BuiltAt string `json:"built_at"`
	// Merges 是桶扫描自己报告的额外合并对数（负值 = 小图标太多退回逐对
	// 相似度），只为了让续跑的日志与首次扫描一字不差。
	Merges  int                      `json:"align_merges"`
	Buckets []motifScanBucket        `json:"buckets"`
	Pairs   map[string]motifScanPair `json:"pairs"`
}

type motifScanBucket struct {
	// Members 是桶成员（完整 task key，md::图片路径），顺序与扫描输出一致。
	Members []string `json:"members"`
	Exact   bool     `json:"exact"`
}

// motifScanPair 是一对的判定明细（字段名压短：整本书几千对，键面只为了
// 文件别太大，可读性看 pairs.csv）。
type motifScanPair struct {
	Score  float64 `json:"s"`
	Level  int     `json:"l"`
	Canvas int     `json:"c"`
	Levels int     `json:"n"`
	ShiftY int     `json:"y"`
	ShiftX int     `json:"x"`
}

func motifScanPairOf(d motifAlignDetail) motifScanPair {
	return motifScanPair{Score: d.Score, Level: d.Level, Canvas: d.Canvas, Levels: d.Levels, ShiftY: d.ShiftY, ShiftX: d.ShiftX}
}

func (p motifScanPair) detail() motifAlignDetail {
	return motifAlignDetail{Score: p.Score, Level: p.Level, Canvas: p.Canvas, Levels: p.Levels, ShiftY: p.ShiftY, ShiftX: p.ShiftX}
}

// motifScanKey 是扫描缓存的键：判据 + 内容摘要集合。摘要先排序去重——同一份
// 内容被引用多少次不影响扫描结果（桶只按不同内容跑）。
func (r *Runner) motifScanKey(hashes []string) string {
	seen := make(map[string]bool, len(hashes))
	list := make([]string, 0, len(hashes))
	for _, h := range hashes {
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		list = append(list, h)
	}
	sort.Strings(list)
	shift := r.cfg.Latex.DedupeAlignShift
	if shift <= 0 {
		shift = motifAlignDefaultShift
	}
	h := sha256.New()
	fmt.Fprintf(h, "v=%s;fmt=%d;min=%.4f;shift=%.4f;auto=%.4f;typical=%.4f;grid=%s;maxarea=%d;bucket=%d\n",
		motifDedupeVersion, motifScanCacheFormat, r.motifAlignMin(), shift, r.motifIconAutoMin(),
		r.motifAlignTypical(), motifAlignGridSpec, motifAlignIconMaxArea, motifAlignBucketMax)
	for _, s := range list {
		fmt.Fprintf(h, "%s\n", s)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// motifScanCachePath 是缓存的固定位置。
func motifScanCachePath(dir string) string { return filepath.Join(dir, motifScanCacheName) }

// loadMotifScanCache 读回上一次的扫描中间量；键不符、形状不符、文件损坏一律
// 返回 nil（当作没有缓存重扫）。损坏只警告：这是派生产物，不是结论来源。
func (r *Runner) loadMotifScanCache(dir, key string) *motifScanCache {
	path := motifScanCachePath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c motifScanCache
	if err := json.Unmarshal(data, &c); err != nil {
		r.logWarning("[dedupe] scan.json 读不回来（按没有缓存处理，重扫）:", err)
		return nil
	}
	if c.Format != motifScanCacheFormat || c.Version != motifDedupeVersion || c.Key != key {
		return nil
	}
	return &c
}

// groupsFor 把缓存的桶还原成扫描输出。任何一个成员在当前池子里找不到（说明
// 输入的内容集合其实变了，只是摘要撞上了不可能的情形），就整份作废。
func (c *motifScanCache) groupsFor(pool []*motifCandidate) []VisualMotifGroup {
	if c == nil {
		return nil
	}
	known := make(map[string]bool, len(pool))
	for _, p := range pool {
		known[p.task.key()] = true
	}
	out := make([]VisualMotifGroup, 0, len(c.Buckets))
	for _, b := range c.Buckets {
		if len(b.Members) == 0 {
			return nil
		}
		g := VisualMotifGroup{Exact: b.Exact, Members: make([]string, len(b.Members))}
		for i, m := range b.Members {
			if !known[m] {
				return nil
			}
			g.Members[i] = m
		}
		out = append(out, g)
	}
	return out
}

// saveMotifScanCache 在同一次扫描里把桶与逐对明细写成一份缓存。写失败只警告：
// 下次续跑多花 5 秒，结论不受影响。
func (r *Runner) saveMotifScanCache(dir, key string, merges int, groups []VisualMotifGroup, pairs map[string]motifScanPair) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.logWarning("[dedupe] scan.json 目录创建失败:", err)
		return
	}
	c := motifScanCache{
		Format:  motifScanCacheFormat,
		Version: motifDedupeVersion,
		Key:     key,
		BuiltAt: time.Now().Format(time.RFC3339),
		Merges:  merges,
		Pairs:   pairs,
	}
	for _, g := range groups {
		c.Buckets = append(c.Buckets, motifScanBucket{Members: append([]string(nil), g.Members...), Exact: g.Exact})
	}
	data, err := json.Marshal(&c)
	if err != nil {
		r.logWarning("[dedupe] scan.json 序列化失败:", err)
		return
	}
	tmp := motifScanCachePath(dir) + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		r.logWarning("[dedupe] scan.json 写盘失败:", err)
		return
	}
	if err := os.Rename(tmp, motifScanCachePath(dir)); err != nil {
		r.logWarning("[dedupe] scan.json 落位失败:", err)
	}
}
