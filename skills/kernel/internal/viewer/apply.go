package viewer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/LingNc/DocVision/skills/kernel/internal/dedupe"
	"github.com/LingNc/DocVision/skills/kernel/internal/figcheck"
	"github.com/LingNc/DocVision/skills/kernel/internal/ids"
)

// apply.go：viewer-apply——消费看图台导出的重分组请求（regroup.json），
// 按用户确认的阈值重算分组，写覆盖文件 groups_override.json；
// GenDraw 读到覆盖文件后只为**代表图**铺重绘任务（组员跟随代表，不再各画一张）。

// RegroupRequest 是看图台导出的请求格式（页面"导出重分组请求"的产物）。
// type=regroup：按阈值重算；type=group_edit：用户显式增删成员/并拆组（v3），
// 请求里的 groups 直接成为覆盖分组（信任用户人工判断，kernel 只校验编号存在）。
type RegroupRequest struct {
	Type     string      `json:"type"` // "regroup" | "group_edit"
	AlignMin float64     `json:"align_min"`
	ShiftPct float64     `json:"shift_pct"`
	Groups   []GroupCard `json:"groups,omitempty"`
}

// OverrideGroups 是覆盖文件格式（GenDraw 消费）。
type OverrideGroups struct {
	Threshold float64     `json:"threshold"`
	Groups    []GroupCard `json:"groups"`
}

// OverridePath 覆盖文件路径。
func OverridePath(projectRoot string) string {
	return filepath.Join(projectRoot, "work", "viewer", "groups_override.json")
}

// ApplyRegroup 按请求阈值重算分组并写覆盖文件。返回新分组（含阈值）。
func ApplyRegroup(projectRoot string, req RegroupRequest) (*OverrideGroups, error) {
	tbl, err := ids.Load(projectRoot)
	if err != nil {
		return nil, err
	}
	// group_edit：用户显式分组（v3）——校验编号存在、rep 必须在组内，直接落覆盖。
	if req.Type == "group_edit" {
		if len(req.Groups) == 0 {
			return nil, fmt.Errorf("group_edit 请求没有 groups")
		}
		ov := &OverrideGroups{Threshold: req.AlignMin}
		for _, g := range req.Groups {
			if len(g.Members) < 2 {
				return nil, fmt.Errorf("组 %s 成员不足 2 个", g.Rep)
			}
			repIn := false
			for _, m := range g.Members {
				if _, ok := tbl.Get(m); !ok {
					return nil, fmt.Errorf("编号 %s 不存在于映射表", m)
				}
				if m == g.Rep {
					repIn = true
				}
			}
			if !repIn {
				return nil, fmt.Errorf("代表 %s 必须在该组成员里", g.Rep)
			}
			members := append([]string(nil), g.Members...)
			sort.Strings(members)
			ov.Groups = append(ov.Groups, GroupCard{Rep: g.Rep, Members: members, Exact: g.Exact})
		}
		sort.Slice(ov.Groups, func(i, j int) bool { return ov.Groups[i].Rep < ov.Groups[j].Rep })
		return saveOverride(projectRoot, ov)
	}
	if req.AlignMin <= 0 {
		return nil, fmt.Errorf("align_min 必须为正（收到的请求：%+v）", req)
	}
	if req.ShiftPct <= 0 {
		req.ShiftPct = 0.10
	}
	var samples []dedupe.Sample
	pathOf := map[string]string{}
	for _, r := range tbl.List(ids.KindImg) {
		imgPath := filepath.Join(projectRoot, filepath.FromSlash(r.Path))
		img, err := figcheck.DecodeImage(imgPath)
		if err != nil {
			continue
		}
		samples = append(samples, dedupe.Sample{Key: r.ID, Image: img})
		pathOf[r.ID] = r.Path
	}
	groups, _ := dedupe.FindCandidatesAligned(samples, req.AlignMin, req.ShiftPct)
	ov := &OverrideGroups{Threshold: req.AlignMin}
	byPath := map[string]string{}
	for _, r := range tbl.Records {
		byPath[r.Path] = r.ID
	}
	for _, g := range groups {
		gc := GroupCard{Exact: g.Exact}
		for _, m := range g.Members {
			if id, ok := byPath[pathOf[m]]; ok {
				gc.Members = append(gc.Members, id)
			}
		}
		sort.Strings(gc.Members)
		if len(gc.Members) >= 2 {
			if id, ok := byPath[pathOf[g.Members[0]]]; ok {
				gc.Rep = id
			}
			ov.Groups = append(ov.Groups, gc)
		}
	}
	sort.Slice(ov.Groups, func(i, j int) bool { return ov.Groups[i].Rep < ov.Groups[j].Rep })
	return saveOverride(projectRoot, ov)
}

// saveOverride 写覆盖文件。
func saveOverride(projectRoot string, ov *OverrideGroups) (*OverrideGroups, error) {
	if err := os.MkdirAll(filepath.Dir(OverridePath(projectRoot)), 0o755); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(ov, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(OverridePath(projectRoot), b, 0o644); err != nil {
		return nil, err
	}
	return ov, nil
}

// LoadOverride 读覆盖文件（不存在返回 nil, nil——调用方走默认"每图各画"）。
func LoadOverride(projectRoot string) (*OverrideGroups, error) {
	b, err := os.ReadFile(OverridePath(projectRoot))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ov OverrideGroups
	if err := json.Unmarshal(b, &ov); err != nil {
		return nil, fmt.Errorf("覆盖文件损坏: %w", err)
	}
	return &ov, nil
}
