package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// motifViewTool delegates to the normal view_image implementation. Only a
// successful, uncropped image receipt is evidence for a merge; exact pixels
// share evidence. Contact sheets keep a 300-image group inspectable in one
// context without forcing 300 image attachments into the initial user turn.
type motifViewTool struct {
	base    *ViewImageTool
	covered map[string][]int
	seen    map[int]bool
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
	// Sheets are 768px wide; the default 1280 view preserves every tile.
	zoom := intArg(args, "zoom", 1280)
	if uncropped && (zoom <= 0 || zoom >= 768) && result.ImageBase64 != "" {
		for _, id := range t.covered[full] {
			t.seen[id] = true
		}
	}
	return result, nil
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
	for _, group := range groups {
		for _, id := range group {
			if !t.seen[id] {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: image %d has not been inspected. Use view_image on its original or uncropped numbered contact sheet, or omit its group.", id)}, nil
			}
		}
	}
	return t.done.Execute(`{}`) // reuse the normal terminal submit receipt
}

type motifAdvisory struct {
	Group int  `json:"initial_group,omitempty"`
	Exact bool `json:"exact_content,omitempty"`
}

func (r *Runner) verifyMotifCandidates(members []*motifCandidate, outDir string, groupIndex, batchIndex int, hints ...map[string]motifAdvisory) ([][]int, error) {
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
	covered, err := prepareMotifViewWithAdvisory(view, members, advisory)
	if err != nil {
		return nil, err
	}
	mounts := []Mount{{Name: "candidates", Dir: view}}
	bashMounts := []Mount{{Name: "work", Dir: view}}
	imageTool := &motifViewTool{base: &ViewImageTool{Root: view, BareSearch: true, SoftMax: r.cfg.Tools.View.ImageMax, WarnRatio: r.cfg.Tools.View.WarnRatio}, covered: covered, seen: seen}
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
	// A whole book motif group can contain hundreds of distinct crops. Contact
	// sheets reduce tool use, but never truncate the candidate inventory/context.
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

func prepareMotifView(root string, members []*motifCandidate) (map[string][]int, error) {
	return prepareMotifViewWithAdvisory(root, members, nil)
}

func prepareMotifViewWithAdvisory(root string, members []*motifCandidate, advisory map[string]motifAdvisory) (map[string][]int, error) {
	originals := filepath.Join(root, "originals")
	sheets := filepath.Join(root, "sheets")
	if err := os.MkdirAll(originals, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(sheets, 0755); err != nil {
		return nil, err
	}
	covered := map[string][]int{}
	pixelGroups := map[[32]byte][]int{}
	for i, c := range members {
		if c.image != nil {
			h, ok := rgbaFingerprint(c.image)
			if ok {
				pixelGroups[h] = append(pixelGroups[h], i+1)
			}
		}
	}
	var manifest strings.Builder
	manifest.WriteString("# Complete image pool\n\nAll classified images are available to the AI. Local similarity is advisory only: it may suggest an initial group, but it never excludes an image or proves equivalence. Inspect details before grouping; different numbers/labels/colors remain separate. Read originals for uncertain sheet tiles.\n\n## Originals\n\n")
	for i, c := range members {
		ext := filepath.Ext(c.path)
		rel := fmt.Sprintf("originals/%06d%s", i+1, ext)
		dest := filepath.Join(root, filepath.FromSlash(rel))
		if !fileExists(dest) {
			if err := linkAbs(c.path, dest); err != nil {
				if err := copyFile(c.path, dest); err != nil {
					return nil, err
				}
			}
		}
		ids := []int{i + 1}
		if c.image != nil {
			if h, ok := rgbaFingerprint(c.image); ok {
				ids = pixelGroups[h]
			}
		}
		covered[dest] = ids
		advice := "none"
		if a, ok := advisory[c.task.key()]; ok && a.Group > 0 {
			advice = fmt.Sprintf("group-%d", a.Group)
			if a.Exact {
				advice += ":exact-pixels"
			}
		}
		fmt.Fprintf(&manifest, "%d. %s | %s | %s | similarity_hint=%s\n", i+1, rel, c.task.mdName, c.task.imgPath, advice)
	}
	manifest.WriteString("\n## Numbered contact sheets\n\nEach tile is labeled with its original ID. Sheets are views of the complete image pool, not separate sessions or fixed-size equivalence groups.\n\n")
	// 64 views per sheet is a display layout only; all sheets stay in ONE session.
	for start := 0; start < len(members); start += 64 {
		end := start + 64
		if end > len(members) {
			end = len(members)
		}
		rows := (end - start + 7) / 8
		sheet := image.NewRGBA(image.Rect(0, 0, 768, rows*112))
		draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		ids := []int{}
		for i := start; i < end; i++ {
			img := members[i].image
			if img == nil {
				var err error
				img, err = decodeImage(members[i].path)
				if err != nil {
					return nil, err
				}
			}
			b := img.Bounds()
			w, h := b.Dx(), b.Dy()
			if w == 0 || h == 0 {
				return nil, fmt.Errorf("empty candidate image %d", i+1)
			}
			tile := (i - start)
			x0 := (tile % 8) * 96
			y0 := (tile / 8) * 112
			tw, th := 88, 88
			if w > h {
				th = h * 88 / w
			} else {
				tw = w * 88 / h
			}
			if tw < 1 {
				tw = 1
			}
			if th < 1 {
				th = 1
			}
			for y := 0; y < th; y++ {
				for x := 0; x < tw; x++ {
					sheet.Set(x0+4+x, y0+20+y, img.At(b.Min.X+x*w/tw, b.Min.Y+y*h/th))
				}
			}
			drawMotifID(sheet, x0+4, y0+3, i+1)
			ids = append(ids, i+1)
		}
		rel := fmt.Sprintf("sheets/%03d.png", start/64+1)
		full := filepath.Join(root, filepath.FromSlash(rel))
		f, err := os.Create(full)
		if err != nil {
			return nil, err
		}
		encErr := png.Encode(f, sheet)
		closeErr := f.Close()
		if encErr != nil {
			return nil, encErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		covered[full] = ids
		fmt.Fprintf(&manifest, "- %s: IDs %d–%d\n", rel, start+1, end)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.md"), []byte(manifest.String()), 0644); err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(members))
	for i, c := range members {
		a := advisory[c.task.key()]
		// A candidate may legitimately carry no progress record (e.g. a caller
		// building the view directly); never panic while writing the advisory.
		class := ""
		if c.progress != nil {
			class = c.progress.Class
		}
		items = append(items, map[string]any{
			"id":            i + 1,
			"key":           c.task.key(),
			"initial_group": a.Group,
			"exact_content": a.Exact,
			"class":         class,
			"source_hash":   c.sourceHash,
		})
	}
	advisoryJSON := map[string]any{
		"description": "Local similarity is advisory only; the complete image pool remains available to the AI.",
		"pool_size":   len(members),
		"items":       items,
	}
	data, err := json.MarshalIndent(advisoryJSON, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "similarity_advisory.json"), append(data, '\n'), 0644); err != nil {
		return nil, err
	}
	return covered, nil
}

// A tiny numeric label avoids introducing a font or external rendering tool.
func drawMotifID(dst *image.RGBA, x, y, id int) {
	digits := []string{"111101101101111", "010110010010111", "111001111100111", "111001111001111", "101101111001001", "111100111001111", "111100111101111", "111001001001001", "111101111101111", "111101111001111"}
	for n, ch := range strconv.Itoa(id) {
		pattern := digits[int(ch-'0')]
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if pattern[row*3+col] == '1' {
					draw.Draw(dst, image.Rect(x+n*8+col*2, y+row*2, x+n*8+col*2+2, y+row*2+2), image.NewUniform(color.Black), image.Point{}, draw.Src)
				}
			}
		}
	}
}
