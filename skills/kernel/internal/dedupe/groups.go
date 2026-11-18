package dedupe

import (
	"fmt"
	"sort"
	"strings"
)

// groups.go：成组结构校验（旧线 motif_groups.go 的验证逻辑部分）。
// 交互式 Tool 壳不进新架构——新形态下 agent 直接编辑 groups.json，
// kernel 在 submit 时跑这里的机械校验（validate 通过才算数）。

// GroupIDs 是一个最终结构组：Key 是 AI 挑的代表（被重画的那张），其余成员复用。
type GroupIDs struct {
	Key     int   `json:"key"`
	Members []int `json:"members"`
}

// NormalizeGroup 校验并规范化一组 ID（旧线 normalizeMotifGroup）。
func NormalizeGroup(raw []int, count int) ([]int, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("group 至少需要 2 个 ID")
	}
	seen := map[int]bool{}
	out := append([]int(nil), raw...)
	for _, id := range out {
		if id < 1 || id > count {
			return nil, fmt.Errorf("ID %d 超出 1..%d", id, count)
		}
		if seen[id] {
			return nil, fmt.Errorf("ID %d 在同组重复", id)
		}
		seen[id] = true
	}
	sort.Ints(out)
	return out, nil
}

// ValidateGroups 全量校验（旧线 validateMotifGroups）：每组合法、key 必须是
// 该组成员、一个 ID 至多属于一个组。
func ValidateGroups(groups []GroupIDs, count int) error {
	used := map[int]int{}
	for i, g := range groups {
		ng, err := NormalizeGroup(g.Members, count)
		if err != nil {
			return fmt.Errorf("第 %d 组：%w", i+1, err)
		}
		if g.Key != 0 {
			found := false
			for _, id := range ng {
				if id == g.Key {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("第 %d 组：key %d 不在该组里（key 必须是组内最有代表性的一张）", i+1, g.Key)
			}
		}
		for _, id := range ng {
			if prev, ok := used[id]; ok {
				return fmt.Errorf("ID %d 同时出现在第 %d、%d 组", id, prev+1, i+1)
			}
			used[id] = i
		}
	}
	return nil
}

// ValidateGroupShape 强制"每组有代表图"这一件事（旧线 validateMotifGroupShape）：
// 整池一组也允许（用户明确要可行），只要指定了 key——process 只画 key、其余复用。
func ValidateGroupShape(groups []GroupIDs) error {
	for i, g := range groups {
		if g.Key == 0 {
			return fmt.Errorf("第 %d 组还没有 key：请挑出该组最有代表性的一张作为 key（其余成员之后直接复用它的图，只有 key 会被重画）", i+1)
		}
	}
	return nil
}

// RenderGroups 输出人类可读的组列表（旧线 renderMotifGroups）。
func RenderGroups(groups []GroupIDs) string {
	if len(groups) == 0 {
		return "groups.json 为空，当前没有暂存分组。"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d 个分组：\n", len(groups))
	for i, g := range groups {
		key := "key=? (必须指定)"
		if g.Key != 0 {
			key = fmt.Sprintf("key=%d", g.Key)
		}
		fmt.Fprintf(&b, "%d. %s %v\n", i+1, key, g.Members)
	}
	return b.String()
}
