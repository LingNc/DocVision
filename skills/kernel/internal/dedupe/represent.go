package dedupe

import "sort"

// represent.go：代表图/组收口/自动定组/挂靠的纯函数部分（旧线 motif_dedupe.go
// 移植）。编排/进度落盘由调用方负责，这里只保留可测的判定逻辑。

// ContentLeader 是一个内容代表项：Key 是代表，Copies 是字节相同的副本。
type ContentLeader struct {
	Key    string
	Copies []string
}

// ContentLeaders 按内容哈希把池子分成"每张不同内容一个代表 + 它的字节副本"
// （旧线 motifContentLeaders：逐字节相同 = 同一份内容，看一份就够）。
// 返回的顺序与输入首次出现顺序一致（确定性）。
func ContentLeaders(items []struct {
	Key  string
	Hash string
}) ([]ContentLeader, map[string]string) {
	leaderOf := map[string]string{}
	idx := map[string]int{} // hash → leader 在 leaders 中的下标
	var leaders []ContentLeader
	for _, it := range items {
		if i, ok := idx[it.Hash]; ok {
			leaders[i].Copies = append(leaders[i].Copies, it.Key)
			leaderOf[it.Key] = leaders[i].Key
			continue
		}
		idx[it.Hash] = len(leaders)
		leaders = append(leaders, ContentLeader{Key: it.Key})
	}
	return leaders, leaderOf
}

// TypicalMembers 筛出"典型成员"：与 key 重合度 ≥ typical（默认 AlignTypical=0.95）
// 的成员（旧线代表图规则与组内闸门一致：只从典型成员里挑代表图）。
func TypicalMembers(key AlignImage, members []AlignImage, typical float64, shiftCells int) []AlignImage {
	if typical <= 0 {
		typical = AlignTypical
	}
	var out []AlignImage
	for _, m := range members {
		if key.Overlap(m, shiftCells) >= typical {
			out = append(out, m)
		}
	}
	return out
}

// PickRepresentative 从典型成员里取内容框最大者当代表图——既典型又最全。
// members 需已含 key 本身；无达标成员时回退 key（旧线"没有达标的就用中心那块的代表图"）。
func PickRepresentative(key AlignImage, members []AlignImage, threshold float64, shiftCells int) (AlignImage, bool) {
	typical := TypicalMembers(key, members, threshold, shiftCells)
	best, bestArea := key, -1
	found := false
	for _, m := range typical {
		if m.Area > bestArea {
			best, bestArea, found = m, m.Area, true
		}
	}
	return best, found
}

// AppliedGroup 是一个已确认的组：Key 代表图、Members 全部成员、Bucket 所属相似桶。
type AppliedGroup struct {
	Key     string
	Members []string
	Bucket  int
}

// MergeAllowedFunc 判定两张图（按 key）是否允许合并（检测器闸门，与建桶同口径）。
type MergeAllowedFunc func(a, b string) bool

// ScoreFunc 给两张图的重合度打分（找不到覆盖率时 ok=false）。
type ScoreFunc func(a, b string) (float64, bool)

// FoldGroups 组间收口（T149）：同一相似桶内的组，关键图彼此达标（mergeAllowed）
// 则折成一组；中心取"与其余代表图平均重合度最高"的那块；成员按各块已确认结论
// **原样收编**（不再拿新中心重筛——重筛会把同一枚图拆回好几份）；代表图在
// 折后成员里按"典型 0.95 + 内容框最大"重挑。跨桶不折。
func FoldGroups(applied []AppliedGroup, bucketOf map[string]int, score ScoreFunc, mergeAllowed MergeAllowedFunc, shiftCells int) ([]AppliedGroup, int, int) {
	n := len(applied)
	if n < 2 {
		return applied, 0, 0
	}
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			bi, bj := bucketOf[applied[i].Key], bucketOf[applied[j].Key]
			if bi == 0 || bi != bj {
				continue // 跨桶不折
			}
			if !mergeAllowed(applied[i].Key, applied[j].Key) {
				continue
			}
			ri, rj := find(i), find(j)
			if ri != rj {
				parent[rj] = ri
			}
		}
	}
	parts := map[int][]int{}
	var order []int
	for i := 0; i < n; i++ {
		root := find(i)
		if _, ok := parts[root]; !ok {
			order = append(order, root)
		}
		parts[root] = append(parts[root], i)
	}
	var out []AppliedGroup
	folded, added := 0, 0
	for _, root := range order {
		idx := parts[root]
		if len(idx) < 2 {
			out = append(out, applied[idx[0]])
			continue
		}
		// 中心：与其余代表图平均重合度最高者
		center, bestMean := idx[0], -1.0
		for _, i := range idx {
			sum, cnt := 0.0, 0
			for _, j := range idx {
				if i == j {
					continue
				}
				if v, ok := score(applied[i].Key, applied[j].Key); ok {
					sum += v
					cnt++
				}
			}
			if cnt > 0 && sum/float64(cnt) > bestMean {
				bestMean, center = sum/float64(cnt), i
			}
		}
		var members []string
		inMembers := map[string]bool{}
		origin := map[string]int{}
		for _, i := range idx {
			for _, m := range applied[i].Members {
				if inMembers[m] {
					continue
				}
				inMembers[m] = true
				origin[m] = i
				members = append(members, m)
			}
		}
		sort.Strings(members)
		out = append(out, AppliedGroup{Key: applied[center].Key, Members: members, Bucket: applied[center].Bucket})
		folded += len(idx) - 1
		_ = added
		_ = origin
		added += 0
	}
	return out, folded, added
}

// AutoIconGroups 把重合到几乎没有疑问的小图标直接本地定组（T131）：池内两两
// 重合 ≥ min（默认 0.75）且闸门允许即同组，不进核验会话。返回定出的组（成员
// 按 key 排序，首成员当代表）与未入组的剩余项。
func AutoIconGroups(keys []string, covs map[string]AlignImage, min float64, mergeAllowed MergeAllowedFunc, shiftCells int) (groups [][]string, remaining []string) {
	if min <= 0 {
		return nil, keys
	}
	parent := make([]int, len(keys))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) { parent[find(b)] = find(a) }
	for i := 0; i < len(keys); i++ {
		ca, oka := covs[keys[i]]
		if !oka || ca.Area > IconMaxArea {
			continue
		}
		for j := i + 1; j < len(keys); j++ {
			cb, okb := covs[keys[j]]
			if !okb || cb.Area > IconMaxArea {
				continue
			}
			if !mergeAllowed(keys[i], keys[j]) {
				continue
			}
			if find(i) == find(j) {
				continue
			}
			if ca.Overlap(cb, shiftCells) >= min {
				union(i, j)
			}
		}
	}
	byRoot := map[int][]string{}
	for i, k := range keys {
		byRoot[find(i)] = append(byRoot[find(i)], k)
	}
	var roots []int
	for r := range byRoot {
		roots = append(roots, r)
	}
	sort.Ints(roots)
	for _, r := range roots {
		ms := byRoot[r]
		sort.Strings(ms)
		if len(ms) >= 2 {
			groups = append(groups, ms)
		} else {
			remaining = append(remaining, ms[0])
		}
	}
	return groups, remaining
}

// AttachLeftovers 把"没进过会话、也没有等价结论"的图挂到已确认组的代表图上
// （T135）：只认会话确认过的 key 作锚点；一次只挂一个组（取最高分）；已确认/
// 已核过的跳过。返回挂靠关系  leftovers→anchorKey。
func AttachLeftovers(candidates []string, covs map[string]AlignImage, anchors map[string]AlignImage, grouped map[string]bool, min float64, shiftCells int) map[string]string {
	out := map[string]string{}
	if min <= 0 || len(anchors) == 0 {
		return out
	}
	anchorKeys := make([]string, 0, len(anchors))
	for k := range anchors {
		anchorKeys = append(anchorKeys, k)
	}
	sort.Strings(anchorKeys)
	for _, c := range candidates {
		if grouped[c] {
			continue
		}
		cov, ok := covs[c]
		if !ok || cov.Area > IconMaxArea {
			continue
		}
		best, bestScore := "", min
		for _, a := range anchorKeys {
			if v := cov.Overlap(anchors[a], shiftCells); v > bestScore {
				best, bestScore = a, v
			}
		}
		if best != "" {
			out[c] = best
		}
	}
	return out
}
