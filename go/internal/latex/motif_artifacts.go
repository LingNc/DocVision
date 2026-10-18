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
// equivalent_to），这里落盘是为了可复查、可交给下游按组操作，并且给续跑一张
// "免重算凭证"——T141 起 `motifReuseDedupe` 拿指纹 + progress 对账，对得上就
// 整段跳过扫描与会话。写失败不影响流程（dedupe 的结论已经在 progress 里了）。
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

// motifHashedDedupe 是"只读字节、不解码"的预扫描结果：复用判据只需要每张图
// 的内容摘要（T141）。
type motifHashedDedupe struct {
	Key  string
	Hash string
}

// motifCurrentRules 与 writeMotifArtifacts 取同一套值，保证写盘与复用判据的
// 指纹口径一致。
func (r *Runner) motifCurrentRules() motifArtifactRules {
	return motifArtifactRules{
		AlignMin:   round3(r.motifAlignMin()),
		AlignShift: round3(r.cfg.Latex.DedupeAlignShift),
		IconAuto:   round3(r.motifIconAutoMin()),
	}
}

// motifReuseDedupe 判断上一次落盘的结论能不能整段沿用（T141）。三个条件缺一
// 不可：① 版本与规则（阈值/平移/自动定组）没变；② 输入没变——指纹只吃"每份
// 不同内容的内容摘要 + 规则"，所以图片被换掉会立刻失配；③ 表里每一条结论都还
// 在 progress 里对得上（ref 存在、摘要一致、核验标记在、跟随关系与代表图一致），
// 且条目覆盖了本次的每一张图（多一张少一张都要重扫）。
//
// 之所以需要：池子里只要还有一张图没被核验（典型是"跟谁都不像、也没进会话"
// 的那批），allChecked 就永远为假，于是每次续跑都要重新解码全池、重算几万对
// 重合度、再把同样的会话放一遍。结论本来就按图存在 progress 里，缺的只是
// "这次输入和规则还是同一套"的证明。
func (r *Runner) motifReuseDedupe(outDir string, prog map[string]*imageProgress, hashed []motifHashedDedupe) (motifArtifact, bool) {
	var zero motifArtifact
	data, err := os.ReadFile(filepath.Join(motifArtifactDir(outDir), "groups.json"))
	if err != nil {
		return zero, false
	}
	var art motifArtifact
	if err := json.Unmarshal(data, &art); err != nil {
		return zero, false
	}
	if art.Version != motifDedupeVersion {
		return zero, false
	}
	rules := r.motifCurrentRules()
	if art.Rules != rules {
		return zero, false
	}
	distinct := make([]string, 0, len(hashed))
	seen := make(map[string]bool, len(hashed))
	want := make(map[string]string, len(hashed))
	for _, h := range hashed {
		want[h.Key] = h.Hash
		if seen[h.Hash] {
			continue
		}
		seen[h.Hash] = true
		distinct = append(distinct, h.Hash)
	}
	if art.Refs != len(hashed) || art.Distinct != len(distinct) {
		return zero, false
	}
	if art.Fingerprint != motifArtifactFingerprint(rules, distinct) {
		return zero, false
	}
	covered := make(map[string]bool, len(hashed))
	// reviewed = 这条结论必须带着核验标记与版本落过盘。独立条目例外：它没有
	// 任何跟随关系可对，证据就是"这次扫到的摘要 == groups.json 记的摘要"，
	// 指纹与覆盖检查已经保证输入没变。要求它们也有标记是个死结——没候选、
	// 没进会话的图永远拿不到标记，于是 100% 的续跑都要整池重算（T148）。
	check := func(item motifArtifactRef, reviewed bool) bool {
		p := prog[item.Ref]
		if p == nil {
			return false
		}
		if item.Hash != "" && want[item.Ref] != item.Hash {
			return false
		}
		if reviewed {
			if !p.DedupeChecked || p.DedupeVersion != motifDedupeVersion {
				return false
			}
			if item.Hash != "" && p.DedupeSourceHash != item.Hash {
				return false
			}
		}
		covered[item.Ref] = true
		return true
	}
	// 副本没有单独的 hash 字段（字节必然与它的代表图相同），只能验跟随关系：
	// 指向的那张图确实存在、摘要与记录的 leader hash 一致。
	checkCopies := func(dups []string) bool {
		for _, d := range dups {
			p := prog[d]
			if p == nil || !p.DedupeChecked || p.EquivalentTo == "" || p.DedupeLeaderHash == "" {
				return false
			}
			leader := prog[p.EquivalentTo]
			if leader == nil || leader.DedupeSourceHash != p.DedupeLeaderHash {
				return false
			}
			covered[d] = true
		}
		return true
	}
	for _, g := range art.Groups {
		for _, m := range g.Members {
			if !check(m, true) || !checkCopies(m.Duplicates) {
				return zero, false
			}
			p := prog[m.Ref]
			if m.Ref == g.Key {
				if p.EquivalentTo != "" || p.DedupeLeaderHash != "" {
					return zero, false
				}
				continue
			}
			if p.EquivalentTo != g.Key || p.DedupeLeaderHash == "" {
				return zero, false
			}
		}
	}
	for _, m := range art.Independent {
		if !check(m, false) || !checkCopies(m.Duplicates) {
			return zero, false
		}
		if p := prog[m.Ref]; p.EquivalentTo != "" || p.DedupeLeaderHash != "" {
			return zero, false
		}
	}
	if len(covered) != len(hashed) {
		return zero, false
	}
	for _, h := range hashed {
		if !covered[h.Key] {
			return zero, false
		}
	}
	return art, true
}

// writeMotifArtifacts 把本次 dedupe 的最终结论写进 <proj>/source/dedupe/。
//
// scores 是逐对明细那一趟已经算好的重合度（T151）：组内这几百分原本又要重算
// 一遍，现在直接查表，查不到（前筛/预算挡掉的）才现算。
func (r *Runner) writeMotifArtifacts(outDir string, leaders []*motifCandidate, copies map[*motifCandidate][]*motifCandidate, applied []motifAppliedGroup, scores map[string]float64) {
	dir := motifArtifactDir(outDir)
	rules := r.motifCurrentRules()
	probe := &motifReviewProbe{scores: scores, shift: r.motifAlignShift(), budget: motifReviewProbeBudget}
	hashes := make([]string, 0, len(leaders))
	ref := func(c *motifCandidate, key *motifCandidate) motifArtifactRef {
		item := motifArtifactRef{Ref: c.task.key(), Image: c.task.imgPath, Hash: c.sourceHash, Class: c.progress.Class}
		if key != nil {
			if v, ok := probe.of(c, key); ok {
				item.AlignScore = round3(v)
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
	for _, g := range applied {
		if g.Key == nil {
			continue
		}
		// 出处由组自己带（auto/session/attached/folded）：组间收口会重排、
		// 合并组，index 位置不再能说明来源（T149）。
		source := g.Source
		if source == "" {
			source = "session"
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
	if err := r.writeMotifReview(dir, leaders, copies, applied, probe.scores); err != nil {
		r.logWarning("[dedupe] review.html 写盘失败（不影响结论，图和分数仍可从 groups.json 看）:", err)
	}
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
	fmt.Fprintf(&b, "- 分组 %d 个、独立 %d 份\n", len(art.Groups), len(art.Independent))
	b.WriteString("- 看图：同目录的 `review.html`（浏览器打开，缩略图点开就是原图；本页的图片链接也是相对路径）\n\n")
	belowMin := false
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
			if minScore < art.Rules.AlignMin {
				belowMin = true
			}
		}
		origin := map[string]string{"auto": "本地自动（≥阈值）", "session": "核验会话", "attached": "挂靠已有组", "folded": "组间收口"}[g.Source]
		if origin == "" {
			origin = g.Source
		}
		fmt.Fprintf(&b, "| %d | %d | %s | %s | %s | %s |\n", i+1, len(g.Members), motifImageLink(g.KeyImage), g.Class, origin, low)
	}
	if belowMin {
		fmt.Fprintf(&b, "\n> 「组内最低分」是**与代表图**的分数，可能低于 %.2f 的入组门槛：同一张图换个分辨率/长宽比，与代表图的分数会明显偏低，判据只要求它当时与组里**某个已确认成员**达标；组间收口时成员随代表图一起带入、也不再重判。逐对分数见 `metrics/pairs.csv`。\n", art.Rules.AlignMin)
	}
	if len(art.Independent) > 0 {
		fmt.Fprintf(&b, "\n## 独立（%d 份）\n\n", len(art.Independent))
		for _, m := range art.Independent {
			fmt.Fprintf(&b, "- %s %s\n", motifImageLink(m.Image), m.Class)
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

// motifImageLink 把 md 里的图片引用写成相对 report.md 的链接：report.md 在
// <proj>/source/dedupe/，图片引用相对 <proj>/source/，所以加一层 ..／。
func motifImageLink(imgPath string) string {
	p := filepath.ToSlash(strings.TrimSpace(imgPath))
	if p == "" {
		return "—"
	}
	return fmt.Sprintf("[`%s`](../%s)", p, strings.TrimPrefix(p, "/"))
}

func round3(v float64) float64 {
	return float64(int64(v*1000+0.5)) / 1000
}
