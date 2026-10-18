package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// dedupe 的结果以前只活在两个地方：每个会话自己的 `.work/groups.json`（模型
// 编辑的中间态）和每张图的 `progress_items/<md>/<img>.json` 里的
// `equivalent_to`。于是人想复查"到底并了哪几组、代表图是谁、为什么"只能去翻
// 转录；桶扫描与重合度分数更是只在内存里算一遍就丢了。T133 把最终结论落成
// 一棵固定的目录：
//
//	<proj>/source/dedupe/
//	  groups.json   最终分组（组、代表图、成员、与代表图的重合度、类别、是否本地自动定组）
//	  report.md     给人看的一页汇总
//
// 它只是**派生产物**：进程重启后的真源仍是 progress_items（每图的
// equivalent_to），这里落盘是为了可复查、可交给下游按组操作，也方便以后拿
// 指纹直接跳过重算。写失败不影响流程（dedupe 的结论已经在 progress 里了）。
type motifArtifact struct {
	Version     string               `json:"version"`
	BuiltAt     string               `json:"built_at"`
	Rules       motifArtifactRules   `json:"rules"`
	Refs        int                  `json:"refs"`
	Distinct    int                  `json:"distinct"`
	Fingerprint string               `json:"fingerprint"`
	Groups      []motifArtifactGroup `json:"groups"`
	Independent []motifArtifactRef   `json:"independent"`
}

type motifArtifactRules struct {
	AlignMin   float64 `json:"align_min"`
	AlignShift float64 `json:"align_shift"`
	IconAuto   float64 `json:"icon_auto"`
}

type motifArtifactGroup struct {
	Key      string `json:"key"`
	KeyImage string `json:"key_image"`
	Class    string `json:"class"`
	// Source 标出处：auto＝本地 ≥ dedupe_icon_auto 直接定组、session＝核验
	// 会话里定的、attached＝后期挂靠到已有组（T134/P55）。复查时一眼看出。
	Source  string             `json:"source"`
	Members []motifArtifactRef `json:"members"`
}

type motifArtifactRef struct {
	Ref        string   `json:"ref"`
	Image      string   `json:"image"`
	Hash       string   `json:"hash"`
	Class      string   `json:"class"`
	AlignScore float64  `json:"align_score,omitempty"`
	Duplicates []string `json:"duplicates,omitempty"`
}

// motifArtifactDir 是 dedupe 结果的固定目录（与 progress_items 同级，都在
// <proj>/source/ 下）。
func motifArtifactDir(outDir string) string {
	return filepath.Join(filepath.Dir(motifProgressDir(outDir)), "dedupe")
}

// motifArtifactFingerprint 是"输入 + 规则"的指纹：只要它不变，同一批图重跑
// 必然得到同一张 groups.json（会话是随机的，本地判据不是）。
func motifArtifactFingerprint(rules motifArtifactRules, hashes []string) string {
	sort.Strings(hashes)
	h := sha256.New()
	fmt.Fprintf(h, "v=%s;min=%.3f;shift=%.3f;auto=%.3f\n", motifDedupeVersion, rules.AlignMin, rules.AlignShift, rules.IconAuto)
	for _, s := range hashes {
		fmt.Fprintf(h, "%s\n", s)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// writeMotifArtifacts 把本次 dedupe 的最终结论写进 <proj>/source/dedupe/。
func (r *Runner) writeMotifArtifacts(outDir string, leaders []*motifCandidate, copies map[*motifCandidate][]*motifCandidate, applied []motifAppliedGroup, autoCount int) {
	dir := motifArtifactDir(outDir)
	rules := motifArtifactRules{
		AlignMin:   round3(r.motifAlignMin()),
		AlignShift: round3(r.cfg.Latex.DedupeAlignShift),
		IconAuto:   round3(r.motifIconAutoMin()),
	}
	hashes := make([]string, 0, len(leaders))
	ref := func(c *motifCandidate, key *motifCandidate) motifArtifactRef {
		item := motifArtifactRef{Ref: c.task.key(), Image: c.task.imgPath, Hash: c.sourceHash, Class: c.progress.Class}
		if key != nil && key.image != nil && c.image != nil {
			if a, ok := motifAlignCoverage(c.image); ok {
				if b, ok2 := motifAlignCoverage(key.image); ok2 {
					item.AlignScore = round3(a.overlap(b, r.motifAlignShift()))
				}
			}
		}
		for _, d := range copies[c] {
			item.Duplicates = append(item.Duplicates, d.task.key())
		}
		return item
	}
	for _, c := range leaders {
		hashes = append(hashes, c.sourceHash)
	}
	art := motifArtifact{
		Version:     motifDedupeVersion,
		BuiltAt:     time.Now().Format(time.RFC3339),
		Rules:       rules,
		Distinct:    len(leaders),
		Fingerprint: motifArtifactFingerprint(rules, hashes),
	}
	for _, c := range leaders {
		art.Refs += 1 + len(copies[c])
	}
	inGroup := map[*motifCandidate]bool{}
	for i, g := range applied {
		if g.Key == nil {
			continue
		}
		source := "session"
		if i < autoCount {
			source = "auto"
		}
		group := motifArtifactGroup{Key: g.Key.task.key(), KeyImage: g.Key.task.imgPath, Class: g.Key.progress.Class, Source: source}
		for _, m := range g.Members {
			inGroup[m] = true
			group.Members = append(group.Members, ref(m, g.Key))
		}
		art.Groups = append(art.Groups, group)
	}
	for _, c := range leaders {
		if !inGroup[c] {
			art.Independent = append(art.Independent, ref(c, nil))
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.logWarning("[dedupe] 结果目录创建失败（结论已在 progress 里，不影响流程）:", err)
		return
	}
	if data, err := json.MarshalIndent(art, "", "  "); err == nil {
		if err := os.WriteFile(filepath.Join(dir, "groups.json"), append(data, '\n'), 0o644); err != nil {
			r.logWarning("[dedupe] 结果写盘失败（结论已在 progress 里，不影响流程）:", err)
			return
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "report.md"), []byte(motifArtifactReport(art)), 0o644)
	r.logInfo(fmt.Sprintf("[dedupe] 结果已落盘：%s（%d 份不同内容、%d 组、%d 份独立）",
		filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(dir)), "dedupe")), art.Distinct, len(art.Groups), len(art.Independent)))
}

// motifArtifactReport 是给人看的一页汇总：组有多大、代表图是谁、组内最差的
// 重合度是多少（判断这一组是否"看起来都像同一张"）。
func motifArtifactReport(art motifArtifact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# dedupe 结果（%s）\n\n", art.BuiltAt)
	fmt.Fprintf(&b, "- 引用 %d 条 → 不同内容 %d 份\n", art.Refs, art.Distinct)
	fmt.Fprintf(&b, "- 判据：与组中心重合 ≥ %.2f、平移 %.0f%%、本地自动定组 %.2f（0 = 关闭）\n", art.Rules.AlignMin, art.Rules.AlignShift*100, art.Rules.IconAuto)
	fmt.Fprintf(&b, "- 指纹：`%s`（输入与规则没变时，重跑得到的就是这张表）\n", art.Fingerprint)
	fmt.Fprintf(&b, "- 分组 %d 个、独立 %d 份\n\n", len(art.Groups), len(art.Independent))
	if len(art.Groups) > 0 {
		b.WriteString("## 分组\n\n| # | 张数 | 代表图 | 类别 | 来源 | 组内最低分 |\n|---|------|--------|------|------|------------|\n")
	}
	for i, g := range art.Groups {
		minScore, have := 1.0, false
		for _, m := range g.Members {
			if m.Ref == g.Key || m.AlignScore <= 0 {
				continue
			}
			have = true
			if m.AlignScore < minScore {
				minScore = m.AlignScore
			}
		}
		low := "—"
		if have {
			low = fmt.Sprintf("%.3f", minScore)
		}
		origin := map[string]string{"auto": "本地自动（≥阈值）", "session": "核验会话", "attached": "挂靠已有组"}[g.Source]
		if origin == "" {
			origin = g.Source
		}
		fmt.Fprintf(&b, "| %d | %d | `%s` | %s | %s | %s |\n", i+1, len(g.Members), g.KeyImage, g.Class, origin, low)
	}
	if len(art.Independent) > 0 {
		fmt.Fprintf(&b, "\n## 独立（%d 份）\n\n", len(art.Independent))
		for _, m := range art.Independent {
			fmt.Fprintf(&b, "- `%s` %s\n", m.Image, m.Class)
		}
	}
	return b.String()
}

// 日志在测试里可能是 nil（别为了写一行日志让纯函数变得不可测）。
func (r *Runner) logInfo(msg string) {
	if r.log != nil {
		r.log.Log(0, msg)
	}
}

func (r *Runner) logWarning(msg string, args ...any) {
	if r.log == nil {
		return
	}
	r.log.LogWarning(0, append([]any{msg}, args...)...)
}

func round3(v float64) float64 {
	return float64(int64(v*1000+0.5)) / 1000
}
