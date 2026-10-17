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
	if len(groups) == 0 {
		return
	}
	pool := make([]*motifCandidate, 0, len(byKey))
	seen := make(map[string]bool, len(byKey))
	for _, group := range groups {
		for _, key := range group.Members {
			if seen[key] {
				continue
			}
			if c := byKey[key]; c != nil {
				pool = append(pool, c)
				seen[key] = true
			}
		}
	}
	r.log.Log(0, "[dedupe] 相似候选组", fmt.Sprint(len(groups)), "个，候选池", fmt.Sprint(len(pool)), "张；交由一个视觉会话做最终结构分组")
	if len(pool) < 2 || !motifGroupHasVector(pool) {
		return
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].task.key() < pool[j].task.key() })
	failed := 0
	progress.verify(0, 1, len(pool), failed)
	allChecked := true
	for _, c := range pool {
		if !c.progress.DedupeChecked {
			allChecked = false
			break
		}
	}
	if !allChecked {
		for _, c := range pool {
			c.progress.EquivalentTo = ""
			c.progress.DedupeLeaderHash = ""
			c.progress.SharedTex = ""
			c.progress.DedupeChecked = false
			saveProgress(progDir, c.progress)
		}
		verdictGroups, err := r.verifyMotifCandidates(pool, outDir, 0, 0)
		if err != nil {
			failed++
			r.log.LogWarning(0, "[dedupe] AI 核验失败，候选保持独立；process 将逐图处理候选图片:", err)
		} else {
			for _, c := range pool {
				c.progress.DedupeChecked = true
				saveProgress(progDir, c.progress)
			}
			for _, ids := range verdictGroups {
				equivalent := make([]*motifCandidate, 0, len(ids))
				valid := true
				for _, id := range ids {
					if id < 1 || id > len(pool) {
						valid = false
						break
					}
					equivalent = append(equivalent, pool[id-1])
				}
				if valid && motifGroupHasVector(equivalent) {
					applyMotifEquivalence(equivalent, progDir)
				}
			}
		}
	}
	progress.verify(1, 1, len(pool), failed)
}

func motifGroupHasVector(members []*motifCandidate) bool {
	for _, c := range members {
		if c.progress.Class == ClassVector {
			return true
		}
	}
	return false
}

func applyMotifEquivalence(members []*motifCandidate, progDir string) {
	if len(members) < 2 {
		return
	}
	// Prefer a vector-classified drawing and its completed artifact, then use
	// a stable key so retries cannot select a different leader.
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
	leader := members[0]
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
