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
	"strings"

	"mineru-tools/internal/img2text"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

const motifDedupeVersion = "visual-motif-v1"
const motifDedupeBatchSize = 10

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
	var samples []motifCandidate
	byKey := make(map[string]*motifCandidate)
	for _, t := range tasks {
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
	r.log.Log(0, "[dedupe] 相似候选组", fmt.Sprint(len(groups)), "个；先由视觉会话核验，未确认的图片仍独立处理")
	for gi, group := range groups {
		members := make([]*motifCandidate, 0, len(group.Members))
		for _, key := range group.Members {
			if c := byKey[key]; c != nil {
				members = append(members, c)
			}
		}
		if len(members) < 2 || !motifGroupHasVector(members) {
			continue
		}
		// Each verification batch shares an anchor. Prefer an existing
		// vector artifact, then the most common pixel appearance, rather
		// than an arbitrary lexicographically first filename.
		frequencies := map[[32]byte]int{}
		for _, c := range members {
			if h, ok := rgbaFingerprint(c.image); ok {
				frequencies[h]++
			}
		}
		sort.Slice(members, func(i, j int) bool {
			a, b := members[i], members[j]
			aDone := a.progress.Class == ClassVector && a.progress.Status == "done" && a.progress.TikzCode != ""
			bDone := b.progress.Class == ClassVector && b.progress.Status == "done" && b.progress.TikzCode != ""
			if aDone != bDone {
				return aDone
			}
			aVec, bVec := a.progress.Class == ClassVector, b.progress.Class == ClassVector
			if aVec != bVec {
				return aVec
			}
			ha, _ := rgbaFingerprint(a.image)
			hb, _ := rgbaFingerprint(b.image)
			if frequencies[ha] != frequencies[hb] {
				return frequencies[ha] > frequencies[hb]
			}
			return a.task.key() < b.task.key()
		})
		if group.Exact {
			applyMotifEquivalence(members, progDir)
			continue
		}
		for bi, batch := range motifCandidateBatches(members, motifDedupeBatchSize) {
			allChecked := true
			for _, c := range batch {
				if !c.progress.DedupeChecked || c.progress.DedupeVersion != motifDedupeVersion || c.progress.DedupeSourceHash != c.sourceHash {
					allChecked = false
					break
				}
			}
			if allChecked {
				continue
			}
			verdictGroups, err := r.verifyMotifCandidates(batch, outDir, gi, bi)
			if err != nil {
				r.log.LogWarning(0, "[dedupe] AI 核验失败，候选保持独立:", err)
				continue
			}
			for _, c := range batch {
				c.progress.DedupeChecked = true
				saveProgress(progDir, c.progress)
			}
			for _, ids := range verdictGroups {
				equivalent := make([]*motifCandidate, 0, len(ids))
				for _, id := range ids {
					equivalent = append(equivalent, batch[id-1])
				}
				if motifGroupHasVector(equivalent) {
					var anchor *motifCandidate
					for _, c := range equivalent {
						if c == batch[0] && c.progress.Class == ClassVector {
							anchor = c
							break
						}
					}
					applyMotifEquivalenceWithAnchor(equivalent, progDir, anchor)
				}
			}
		}
	}
}

func motifGroupHasVector(members []*motifCandidate) bool {
	for _, c := range members {
		if c.progress.Class == ClassVector {
			return true
		}
	}
	return false
}

func motifCandidateBatches(members []*motifCandidate, max int) [][]*motifCandidate {
	if len(members) <= max {
		return [][]*motifCandidate{members}
	}
	if max < 3 {
		max = 3
	}
	var batches [][]*motifCandidate
	for start := 1; start < len(members); start += max - 1 {
		end := start + max - 1
		if end > len(members) {
			end = len(members)
		}
		batch := []*motifCandidate{members[0]}
		batch = append(batch, members[start:end]...)
		batches = append(batches, batch)
	}
	return batches
}

func (r *Runner) verifyMotifCandidates(batch []*motifCandidate, outDir string, groupIndex, batchIndex int) ([][]int, error) {
	if len(batch) < 2 {
		return nil, nil
	}
	images := make([]string, 0, len(batch))
	var imageList strings.Builder
	for i, c := range batch {
		b64, err := img2text.ImageToBase64(c.path, 1280)
		if err != nil {
			return nil, fmt.Errorf("read candidate %s: %w", c.task.imgPath, err)
		}
		images = append(images, b64)
		fmt.Fprintf(&imageList, "%d. %s\n", i+1, c.task.imgPath)
	}

	modelName := r.cfg.Latex.DedupeModel
	if modelName == "" {
		modelName = r.cfg.Latex.ClassifierModel
	} else if _, ok := r.cfg.ResolveModel(modelName); !ok {
		r.log.LogWarning(0, "[dedupe] 未配置独立模型，回退分类模型:", modelName)
		modelName = r.cfg.Latex.ClassifierModel
	}
	client := r.clientFor(modelName)
	modelCfg := r.modelOf(modelName)
	tuning := r.cfg.LatexSession("classifier")
	hashInput := fmt.Sprintf("%d:%d", groupIndex, batchIndex)
	for _, c := range batch {
		hashInput += "\n" + c.task.key() + ":" + c.sourceHash
	}
	idHash := sha256.Sum256([]byte(hashInput))
	label := "motif-dedupe:" + hex.EncodeToString(idHash[:6])
	sess := session.NewSession(client, modelCfg, tuning, prompts.Must(prompts.DedupeSystem), nil, r.log, 1, label)
	// Transcripts are kept even when ordinary drawing records are disabled:
	// group verdicts are needed for audit and for the LaTeX sessions viewer.
	sessionsDir := filepath.Join(outDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dedupe session directory: %w", err)
	}
	tr, err := session.NewTranscript(filepath.Join(sessionsDir, "motif_dedupe_"+hex.EncodeToString(idHash[:8])+".jsonl"))
	if err != nil {
		return nil, fmt.Errorf("create dedupe transcript: %w", err)
	}
	sess.SetTranscript(tr)
	defer tr.Close()
	answer, err := sess.Run(session.RunOptions{UserText: prompts.Render(prompts.DedupeUser, map[string]string{"IMAGES": imageList.String()}), Images: images})
	if err != nil {
		return nil, err
	}
	return parseMotifGroups(answer, len(batch))
}

func applyMotifEquivalence(members []*motifCandidate, progDir string) {
	applyMotifEquivalenceWithAnchor(members, progDir, nil)
}

func applyMotifEquivalenceWithAnchor(members []*motifCandidate, progDir string, anchor *motifCandidate) {
	if len(members) < 2 {
		return
	}
	// Prefer a vector-classified drawing and its completed artifact, then use
	// a stable key so retries cannot select a different leader.
	sort.Slice(members, func(i, j int) bool {
		a, b := members[i], members[j]
		if a == anchor {
			return true
		}
		if b == anchor {
			return false
		}
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
		if !ok || len(values) < 2 {
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
		out = append(out, ids)
	}
	return out, nil
}
