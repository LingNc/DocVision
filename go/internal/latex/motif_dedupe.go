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
	var samples []motifCandidate
	byKey := make(map[string]*motifCandidate)
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
		f, err := os.Open(path)
		if err != nil {
			r.log.LogWarning(0, "[dedupe] 候选图片不可读，保留独立处理:", t.imgPath, err)
			continue
		}
		img, _, decErr := image.Decode(f)
		_ = f.Close()
		if decErr != nil {
			r.log.LogWarning(0, "[dedupe] 图片解码失败，保留独立处理:", t.imgPath, decErr)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			r.log.LogWarning(0, "[dedupe] 图片摘要读取失败，保留独立处理:", t.imgPath, err)
			continue
		}
		sum := sha256.Sum256(data)
		sourceHash := hex.EncodeToString(sum[:])
		if p.DedupeSourceHash != sourceHash || p.DedupeVersion != motifDedupeVersion {
			p.EquivalentTo = ""
			p.DedupeLeaderHash = ""
			p.SharedTex = ""
			p.DedupeChecked = false
			p.DedupeSourceHash = sourceHash
			p.DedupeVersion = motifDedupeVersion
			saveProgress(progDir, p)
		}
		c := &motifCandidate{task: t, progress: p, image: img, path: path, sourceHash: sourceHash}
		samples = append(samples, *c)
		byKey[t.key()] = c
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
	vis := make([]VisualMotifSample, 0, len(samples))
	for _, s := range samples {
		vis = append(vis, VisualMotifSample{Key: s.task.key(), Image: s.image})
	}
	groups := FindVisualMotifCandidates(vis)
	advisory := make(map[string]motifAdvisory)
	for gi, group := range groups {
		for _, key := range group.Members {
			advisory[key] = motifAdvisory{Group: gi + 1, Exact: group.Exact}
		}
	}
	// Similarity is an advisory index only. The AI sees every classified image,
	// including images that the local detector failed to pair with a neighbour.
	pool := make([]*motifCandidate, 0, len(samples))
	for i := range samples {
		pool = append(pool, &samples[i])
	}
	r.log.Log(0, "[dedupe] 初始相似候选", fmt.Sprint(len(groups)), "组，命中", fmt.Sprint(len(advisory)), "/", fmt.Sprint(len(pool)), "张；AI 分组核验")
	if len(pool) < 2 {
		return
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].task.key() < pool[j].task.key() })
	allChecked := true
	for _, c := range pool {
		if !c.progress.DedupeChecked {
			allChecked = false
			break
		}
	}
	if allChecked {
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
	scopes := r.motifScopes(pool, advisory)
	failed := 0
	verified := 0
	for si, scope := range scopes {
		progress.verify(si, len(scopes), len(pool), failed)
		r.log.Log(0, fmt.Sprintf("[dedupe] 核验会话 %d/%d：%d 张图", si+1, len(scopes), len(scope)))
		verdict, err := r.verifyMotifCandidates(scope, outDir, si, 0, advisory)
		if err != nil {
			failed++
			r.log.LogWarning(0, "[dedupe] 核验会话失败，该批图片保持独立；process 将逐图处理:", err)
			continue
		}
		verified += len(scope)
		for _, c := range scope {
			c.progress.DedupeChecked = true
			saveProgress(progDir, c.progress)
		}
		for _, g := range verdict {
			equivalent := make([]*motifCandidate, 0, len(g.Members))
			valid := true
			for _, id := range g.Members {
				if id < 1 || id > len(scope) {
					valid = false
					break
				}
				equivalent = append(equivalent, scope[id-1])
			}
			if !valid {
				continue
			}
			if !motifGroupAllVector(equivalent) {
				r.log.LogWarning(0, fmt.Sprintf("[dedupe] 组 %v 含非矢量图，保持独立（只有全部为矢量图的组才复用同一张重画）", g.Members))
				continue
			}
			var key *motifCandidate
			if g.Key >= 1 && g.Key <= len(scope) {
				key = scope[g.Key-1]
			}
			// Safety gate (user policy: 宁漏不误判). The AI may only narrow a
			// group, never widen it beyond what the similarity detector itself
			// considers a candidate: a member that is not similar to the key is
			// kept independent even if the model merged it, because sharing one
			// drawing would silently delete the real difference.
			safe := make([]*motifCandidate, 0, len(equivalent))
			dropped := 0
			for _, c := range equivalent {
				if key != nil && c != key && !MotifSimilarTo(c.image, key.image) {
					dropped++
					continue
				}
				safe = append(safe, c)
			}
			if dropped > 0 {
				r.log.LogWarning(0, fmt.Sprintf("[dedupe] 组 %v 里有 %d 张与代表图相似度不足，已保持独立（宁可漏掉重复，也不能合并后丢信息）", g.Members, dropped))
			}
			if len(safe) < 2 {
				continue
			}
			applyMotifEquivalenceWithKey(safe, key, progDir)
		}
	}
	progress.verify(1, len(scopes), len(pool), failed)
	_ = verified
}

// motifScopes splits the pool into verification sessions. A similarity bucket
// larger than latex.dedupe_group_session_over gets its own session (a 300-image
// bucket needs many rounds of one-by-one viewing and would drown everything
// else); the remaining small buckets and standalone images are packed into
// consolidation sessions of at most latex.dedupe_batch_images images, where
// cross-bucket merges can still be decided. Setting group_over to 0 restores
// the previous behaviour: one session for the whole pool.
func (r *Runner) motifScopes(pool []*motifCandidate, advisory map[string]motifAdvisory) [][]*motifCandidate {
	over := r.cfg.Latex.DedupeGroupSessionOver
	perBatch := r.cfg.Latex.DedupeBatchImages
	if over <= 0 {
		return [][]*motifCandidate{pool}
	}
	if perBatch <= 0 {
		perBatch = 40
	}
	buckets := map[int][]*motifCandidate{}
	var loose []*motifCandidate
	for _, c := range pool {
		if a, ok := advisory[c.task.key()]; ok && a.Group > 0 {
			buckets[a.Group] = append(buckets[a.Group], c)
		} else {
			loose = append(loose, c)
		}
	}
	nos := make([]int, 0, len(buckets))
	for g := range buckets {
		nos = append(nos, g)
	}
	sort.Ints(nos)
	var scopes [][]*motifCandidate
	var pending []*motifCandidate
	flush := func() {
		if len(pending) > 0 {
			scopes = append(scopes, pending)
			pending = nil
		}
	}
	for _, g := range nos {
		members := buckets[g]
		if len(members) > over {
			flush()
			scopes = append(scopes, members)
			continue
		}
		if len(pending)+len(members) > perBatch {
			flush()
		}
		pending = append(pending, members...)
	}
	for _, c := range loose {
		if len(pending)+1 > perBatch {
			flush()
		}
		pending = append(pending, c)
	}
	flush()
	if len(scopes) == 0 && len(pool) > 0 {
		return [][]*motifCandidate{pool}
	}
	return scopes
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
