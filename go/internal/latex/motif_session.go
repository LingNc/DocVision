package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// motifViewTool delegates to the normal view_image implementation. Only a
// successful, uncropped view of an individual original counts as evidence for a
// merge. There are deliberately NO contact sheets: a stitched overview makes
// small differences (题号、下标、箭头) invisible, and it let a model "inspect"
// hundreds of images with a handful of calls.
type motifViewTool struct {
	base *ViewImageTool
	seen map[int]bool
}

func (t *motifViewTool) Name() string               { return t.base.Name() }
func (t *motifViewTool) Definition() map[string]any { return t.base.Definition() }
func (t *motifViewTool) Execute(raw string) (session.ToolResult, error) {
	args, err := parseJSONObject(raw)
	if err != nil {
		return session.ToolResult{}, err
	}
	path, _ := args["path"].(string)
	path = strings.TrimPrefix(path, "candidates:")
	args["path"] = path
	full, err := t.base.resolve(path)
	if err != nil {
		return session.ToolResult{}, err
	}
	encoded, _ := json.Marshal(args)
	result, err := t.base.Execute(string(encoded))
	if err != nil {
		return result, err
	}
	uncropped := pctArg(args, "left", 0) == 0 && pctArg(args, "top", 0) == 0 && pctArg(args, "right", 100) == 100 && pctArg(args, "bottom", 100) == 100
	if uncropped && result.ImageBase64 != "" {
		if id := motifOriginalID(full); id > 0 {
			t.seen[id] = true
		}
	}
	return result, nil
}

// motifOriginalID reads the 1-based id out of originals/000123.jpg.
func motifOriginalID(path string) int {
	base := filepath.Base(path)
	dot := strings.IndexByte(base, '.')
	if dot <= 0 {
		return 0
	}
	if filepath.Base(filepath.Dir(path)) != "originals" {
		return 0
	}
	id, err := strconv.Atoi(base[:dot])
	if err != nil || id < 1 {
		return 0
	}
	return id
}

type motifSubmitTool struct {
	done   SubmitDoneTool
	groups *MotifGroupsTool
	seen   map[int]bool
}

func (t *motifSubmitTool) Name() string { return "submit" }
func (t *motifSubmitTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "submit", "description": "Submit the persisted motif_groups result. Call motif_groups list first and inspect every member of each stored group with view_image; omitted images remain independent.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}
func (t *motifSubmitTool) Execute(raw string) (session.ToolResult, error) {
	if _, err := parseJSONObject(raw); err != nil {
		return session.ToolResult{}, err
	}
	groups, err := t.groups.Groups()
	if err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	if err := validateMotifGroupShape(groups, t.groups.count); err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	for _, group := range groups {
		for _, id := range group.Members {
			if !t.seen[id] {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: image %d has not been inspected. Open originals/%06d.* yourself with view_image (uncropped) before merging it; contact sheets no longer exist, and a group you never looked at is not evidence.", id, id)}, nil
			}
		}
	}
	return t.done.Execute(`{}`) // reuse the normal terminal submit receipt
}

type motifAdvisory struct {
	Group int  `json:"initial_group,omitempty"`
	Exact bool `json:"exact_content,omitempty"`
}

func (r *Runner) verifyMotifCandidates(members []*motifCandidate, outDir string, groupIndex, batchIndex int, hints ...map[string]motifAdvisory) ([]MotifGroup, error) {
	if len(members) < 2 {
		return nil, nil
	}
	var identity strings.Builder
	identity.WriteString(motifDedupeVersion)
	for _, c := range members {
		fmt.Fprintf(&identity, "\n%s:%s", c.task.key(), c.sourceHash)
	}
	sum := sha256.Sum256([]byte(identity.String()))
	id := hex.EncodeToString(sum[:8])
	sessionsDir := filepath.Join(outDir, "sessions")
	view := filepath.Join(sessionsDir, "motif_dedupe_"+id+".work")
	seen := map[int]bool{}
	var advisory map[string]motifAdvisory
	if len(hints) > 0 {
		advisory = hints[0]
	}
	if err := prepareMotifViewWithAdvisory(view, members, advisory); err != nil {
		return nil, err
	}
	mounts := []Mount{{Name: "candidates", Dir: view}}
	bashMounts := []Mount{{Name: "work", Dir: view}}
	imageTool := &motifViewTool{base: &ViewImageTool{Root: view, BareSearch: true, SoftMax: r.cfg.Tools.View.ImageMax, WarnRatio: r.cfg.Tools.View.WarnRatio}, seen: seen}
	groupTool := NewMotifGroupsTool(filepath.Join(view, "groups.json"), len(members))
	submit := &motifSubmitTool{groups: groupTool, seen: seen}
	bashTmp, cleanBash := r.sessionBashTemp(filepath.Dir(outDir), "bash_dedupe")
	defer cleanBash()
	bash := &WorkBashTool{Root: view, Mounts: bashMounts, TmpDir: bashTmp, MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(filepath.Dir(outDir)), Log: r.log, Tid: 1}
	tools := []session.Tool{&ReadFileTool{Mounts: mounts}, &GrepTool{Mounts: mounts}, bash, imageTool, groupTool, submit}
	modelName := r.cfg.Latex.DedupeModel
	if modelName == "" {
		modelName = "text"
	} else if _, ok := r.cfg.ResolveModel(modelName); !ok {
		r.log.LogWarning(0, "[dedupe] 未配置独立模型，回退 text:", modelName)
		modelName = "text"
	}
	tuning := r.cfg.LatexSession("dedupe")
	// Every merge needs its own originals opened one by one (no contact sheets),
	// so the round budget has to cover the pool instead of a handful of sheets.
	if tuning.MaxToolRounds > 0 && tuning.MaxToolRounds < len(members)+16 {
		tuning.MaxToolRounds = len(members) + 16
	}
	label := "dedupe:" + id
	sess := session.NewSession(r.clientFor(modelName), r.modelOf(modelName), tuning,
		renderPrompt(prompts.Must(prompts.DedupeSystem), tuning, r.outputLang()), tools, r.log, 1, label)
	tr, err := session.NewTranscript(filepath.Join(sessionsDir, "motif_dedupe_"+id+".jsonl"))
	if err != nil {
		return nil, err
	}
	defer tr.Close()
	sess.SetTranscript(tr)
	hook, closeRow := r.livePhaseRow(label, label)
	defer closeRow()
	sess.SetProgressHook(hook)
	user := prompts.Render(prompts.DedupeUser, map[string]string{"IMAGES": fmt.Sprintf("candidates:manifest.md (%d images; complete image pool; candidates:similarity_advisory.json is advisory only; edit candidates:groups.json with motif_groups)", len(members))})
	_, runErr := sess.Run(session.RunOptions{UserText: user})
	for attempts := 0; runErr == nil && !submit.done.Submitted && attempts < 2; attempts++ {
		_, runErr = sess.Run(session.RunOptions{UserText: "You have NOT called submit yet. Review the persisted groups.json if needed, then call submit with no arguments."})
	}
	if runErr != nil {
		return nil, runErr
	}
	if !submit.done.Submitted {
		return nil, fmt.Errorf("dedupe session ended without submit")
	}
	return submit.groups.Groups()
}

func prepareMotifViewWithAdvisory(root string, members []*motifCandidate, advisory map[string]motifAdvisory) error {
	originals := filepath.Join(root, "originals")
	if err := os.MkdirAll(originals, 0755); err != nil {
		return err
	}
	// Initial similarity buckets are the STARTING grouping, not a verdict: they
	// are pre-filled into groups.json so the AI verifies/splits them instead of
	// inventing a grouping from scratch (and instead of dumping every image into
	// one bucket). Images without a similarity hint stay independent.
	initial := map[int][]int{}
	var standalone []int
	for i, c := range members {
		id := i + 1
		ext := filepath.Ext(c.path)
		rel := fmt.Sprintf("originals/%06d%s", id, ext)
		dest := filepath.Join(root, filepath.FromSlash(rel))
		if !fileExists(dest) {
			if err := linkAbs(c.path, dest); err != nil {
				if err := copyFile(c.path, dest); err != nil {
					return err
				}
			}
		}
		if a, ok := advisory[c.task.key()]; ok && a.Group > 0 {
			initial[a.Group] = append(initial[a.Group], id)
		} else {
			standalone = append(standalone, id)
		}
	}
	groupNos := make([]int, 0, len(initial))
	for g := range initial {
		groupNos = append(groupNos, g)
	}
	sort.Ints(groupNos)

	var manifest strings.Builder
	manifest.WriteString("# Complete image pool\n\n")
	manifest.WriteString("Every classified image is listed below. The local similarity buckets are a STARTING grouping: they are already written into `groups.json`, and they are only a hint — they may contain different images (split them) or miss a match (merge across buckets). Never treat a bucket as proof.\n\n")
	manifest.WriteString("There are NO contact sheets. To judge anything you must open the individual original with `view_image` (`originals/000123.jpg`); a merge is only accepted for images you opened yourself, uncropped.\n\n")

	if len(groupNos) > 0 {
		manifest.WriteString("## Initial similarity buckets (pre-filled in groups.json)\n\n")
	}
	for _, g := range groupNos {
		ids := initial[g]
		exact := false
		for _, id := range ids {
			if a := advisory[members[id-1].task.key()]; a.Exact {
				exact = true
				break
			}
		}
		kind := "visually similar"
		if exact {
			kind = "byte-identical pixels somewhere in the bucket"
		}
		fmt.Fprintf(&manifest, "### bucket %d — %d image(s), %s\n\n", g, len(ids), kind)
		for _, id := range ids {
			c := members[id-1]
			fmt.Fprintf(&manifest, "- %d. originals/%06d%s | %s | %s | class=%s\n", id, id, filepath.Ext(c.path), c.task.mdName, c.task.imgPath, motifClassOf(c))
		}
		manifest.WriteString("\n")
	}
	if len(standalone) > 0 {
		fmt.Fprintf(&manifest, "## Images with no similarity hint (%d)\n\nThey are independent by default, but they may still belong to a bucket above — merge them if the drawing really is the same structure.\n\n", len(standalone))
		for _, id := range standalone {
			c := members[id-1]
			fmt.Fprintf(&manifest, "- %d. originals/%06d%s | %s | %s | class=%s\n", id, id, filepath.Ext(c.path), c.task.mdName, c.task.imgPath, motifClassOf(c))
		}
		manifest.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.md"), []byte(manifest.String()), 0644); err != nil {
		return err
	}

	// Persist the starting grouping so `motif_groups list` shows real work to
	// verify, and so a resumed session keeps the structure.
	start := make([]MotifGroup, 0, len(groupNos))
	for _, g := range groupNos {
		if len(initial[g]) < 2 {
			continue
		}
		// Key stays 0 on purpose: the AI must pick the representative, and
		// submit refuses while any group still has key=0.
		start = append(start, MotifGroup{Members: initial[g]})
	}
	payload, err := json.MarshalIndent(map[string]any{"groups": start}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "groups.json"), append(payload, '\n'), 0644); err != nil {
		return err
	}

	items := make([]map[string]any, 0, len(members))
	for i, c := range members {
		a := advisory[c.task.key()]
		items = append(items, map[string]any{
			"id":            i + 1,
			"key":           c.task.key(),
			"initial_group": a.Group,
			"exact_content": a.Exact,
			"class":         motifClassOf(c),
			"source_hash":   c.sourceHash,
		})
	}
	advisoryJSON := map[string]any{
		"description": "Local similarity is advisory only. The AI decides the final structural groups; images with no hint are still eligible.",
		"pool_size":   len(members),
		"buckets":     len(groupNos),
		"items":       items,
	}
	data, err := json.MarshalIndent(advisoryJSON, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "similarity_advisory.json"), append(data, '\n'), 0644)
}

func motifClassOf(c *motifCandidate) string {
	if c == nil || c.progress == nil {
		return ""
	}
	return c.progress.Class
}

func prepareMotifView(root string, members []*motifCandidate) error {
	return prepareMotifViewWithAdvisory(root, members, nil)
}
