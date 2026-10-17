package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	// dbPath mirrors `seen` into the session workspace. The submit gate lives in
	// memory, but a resumed session replays its transcript instead of re-showing
	// images, so without the file every resumed run would be rejected with
	// "image N has not been inspected" (T121).
	dbPath string
	// keyOffset is len(Members): keys/000001.ext is id keyOffset+1. Used by the
	// final consolidation session, where existing groups are referenced by their
	// key image instead of by re-reading every member.
	keyOffset int
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
		if id := motifScopeID(full, t.keyOffset); id > 0 && !t.seen[id] {
			t.seen[id] = true
			t.saveSeen()
		}
	}
	return result, nil
}

// saveSeen persists the evidence set so an interrupted verification session can
// keep the images it already looked at (T121). Best effort: a failed write only
// costs a re-inspection.
func (t *motifViewTool) saveSeen() {
	if t.dbPath == "" {
		return
	}
	ids := make([]int, 0, len(t.seen))
	for id := range t.seen {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	payload, err := json.MarshalIndent(map[string]any{"inspected": ids}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(t.dbPath, append(payload, '\n'), 0644)
}

// loadMotifSeen restores the evidence set written by saveSeen.
func loadMotifSeen(path string) map[int]bool {
	seen := map[int]bool{}
	data, err := os.ReadFile(path)
	if err != nil {
		return seen
	}
	var doc struct {
		Inspected []int `json:"inspected"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return seen
	}
	for _, id := range doc.Inspected {
		if id > 0 {
			seen[id] = true
		}
	}
	return seen
}

// motifScopeID maps a work-dir path back to its 1-based session id:
// originals/000123.jpg -> 123, keys/000004.jpg -> keyOffset+4.
func motifScopeID(path string, keyOffset int) int {
	base := filepath.Base(path)
	dot := strings.IndexByte(base, '.')
	if dot <= 0 {
		return 0
	}
	id, err := strconv.Atoi(base[:dot])
	if err != nil || id < 1 {
		return 0
	}
	switch filepath.Base(filepath.Dir(path)) {
	case "originals":
		return id
	case "keys":
		return keyOffset + id
	}
	return 0
}

type motifSubmitTool struct {
	done   SubmitDoneTool
	groups *MotifGroupsTool
	seen   map[int]bool
	// prefill is the number of similarity buckets that were staged in
	// groups.json before the model touched it. Only used to tell the truth in
	// the receipt of a 0-group submit: "you cleared the starting buckets too".
	prefill int
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
	if len(groups) == 0 {
		// "SUBMITTED." alone reads like success, and a 0-group submit is a
		// success — of a decision whose consequence the model never sees. State
		// it: this was the T126 failure (a prefilled 90-image bucket cleared,
		// 0 groups submitted, the log said nothing, 130 images redrawn alone).
		res, err := t.done.Execute(`{}`) // reuse the normal terminal submit receipt
		if err != nil {
			return res, err
		}
		res.Text = strings.TrimSpace(res.Text) + " " + t.zeroGroupNote()
		return res, nil
	}
	return t.done.Execute(`{}`) // reuse the normal terminal submit receipt
}

// zeroGroupNote spells out what a 0-group submit means downstream.
func (t *motifSubmitTool) zeroGroupNote() string {
	note := fmt.Sprintf("0 组：本次一张都不合并，本会话 %d 张图全部按独立处理（谁也不会共用别人的成品）。", t.groups.count)
	if t.prefill > 0 {
		note += fmt.Sprintf(" 起始预填的 %d 个相似桶同样被清空了，代码不会替你恢复——只有你 add 回去的组才算数。", t.prefill)
	}
	return note
}

type motifAdvisory struct {
	Group int  `json:"initial_group,omitempty"`
	Exact bool `json:"exact_content,omitempty"`
}

// motifScope is one verification session: its own image set, the ids the AI may
// use, the starting groups (similarity buckets) and, for the final consolidation
// session, the key images of the groups established by earlier sessions.
type motifScope struct {
	Label    string
	Members  []*motifCandidate // ids 1..len(Members)
	Keys     []*motifCandidate // ids keyOffset+1.. (existing groups, by key)
	Prefill  []MotifGroup
	Advisory map[string]motifAdvisory
	// KeyBase is len(Members); keys occupy ids KeyBase+1.. and stand for groups
	// confirmed by earlier sessions.
	KeyBase int
	// Groups lets the manifest describe what each key stands for.
	Groups []motifScopeGroup
}

// motifScopeGroup is the description of an already-confirmed group, shown to the
// consolidation session so it can place an unassigned image by looking at the
// key image only.
type motifScopeGroup struct {
	KeyID   int
	Members []int
	Label   string
	Class   string
}

func (r *Runner) verifyMotifCandidates(members []*motifCandidate, outDir string, groupIndex, batchIndex int, hints ...map[string]motifAdvisory) ([]MotifGroup, error) {
	var advisory map[string]motifAdvisory
	if len(hints) > 0 {
		advisory = hints[0]
	}
	scope := &motifScope{Members: members, Advisory: advisory, Prefill: motifPrefillBuckets(members, advisory)}
	return r.verifyMotifScope(scope, outDir)
}

func (r *Runner) verifyMotifScope(sc *motifScope, outDir string) ([]MotifGroup, error) {
	total := len(sc.Members) + len(sc.Keys)
	if total < 2 {
		return nil, nil
	}
	var identity strings.Builder
	identity.WriteString(motifDedupeVersion)
	for _, c := range sc.Members {
		fmt.Fprintf(&identity, "\nm:%s:%s", c.task.key(), c.sourceHash)
	}
	for _, c := range sc.Keys {
		fmt.Fprintf(&identity, "\nk:%s:%s", c.task.key(), c.sourceHash)
	}
	sum := sha256.Sum256([]byte(identity.String()))
	id := hex.EncodeToString(sum[:8])
	sessionsDir := filepath.Join(outDir, "sessions")
	trPath := filepath.Join(sessionsDir, "motif_dedupe_"+id+".jsonl")
	view := filepath.Join(sessionsDir, "motif_dedupe_"+id+".work")
	if err := prepareMotifScopeView(view, sc); err != nil {
		return nil, err
	}
	// Evidence already collected in a previous process counts: the same id means
	// the same image set (the id is a hash of every member's source hash).
	seenFile := filepath.Join(view, "seen.json")
	seen := loadMotifSeen(seenFile)
	mounts := []Mount{{Name: "candidates", Dir: view}}
	bashMounts := []Mount{{Name: "work", Dir: view}}
	imageTool := &motifViewTool{
		base:      &ViewImageTool{Root: view, BareSearch: true, SoftMax: r.cfg.Tools.View.ImageMax, WarnRatio: r.cfg.Tools.View.WarnRatio},
		seen:      seen,
		dbPath:    seenFile,
		keyOffset: len(sc.Members),
	}
	groupTool := NewMotifGroupsTool(filepath.Join(view, "groups.json"), total)
	compareFiles := make([]string, 0, total)
	for i, c := range sc.Members {
		compareFiles = append(compareFiles, fmt.Sprintf("originals/%06d%s", i+1, filepath.Ext(c.path)))
	}
	for i, c := range sc.Keys {
		compareFiles = append(compareFiles, fmt.Sprintf("keys/%06d%s", i+1, filepath.Ext(c.path)))
	}
	compare := &motifCompareTool{root: view, files: compareFiles}
	submit := &motifSubmitTool{groups: groupTool, seen: seen, prefill: len(sc.Prefill)}
	bashTmp, cleanBash := r.sessionBashTemp(filepath.Dir(outDir), "bash_dedupe")
	defer cleanBash()
	bash := &WorkBashTool{Root: view, Mounts: bashMounts, TmpDir: bashTmp, MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(filepath.Dir(outDir)), Log: r.log, Tid: 1}
	tools := []session.Tool{&ReadFileTool{Mounts: mounts}, &GrepTool{Mounts: mounts}, bash, imageTool, compare, groupTool, submit}
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
	if tuning.MaxToolRounds > 0 && tuning.MaxToolRounds < total+16 {
		tuning.MaxToolRounds = total + 16
	}
	label := "dedupe:" + id
	if sc.Label != "" {
		label = sc.Label + ":" + id
	}
	sess := session.NewSession(r.clientFor(modelName), r.modelOf(modelName), tuning,
		renderPrompt(prompts.Must(prompts.DedupeSystem), tuning, r.outputLang()), tools, r.log, 1, label)
	tr, err := session.NewTranscript(trPath)
	if err != nil {
		return nil, err
	}
	defer tr.Close()
	sess.SetTranscript(tr)
	hook, closeRow := r.livePhaseRow(label, label)
	defer closeRow()
	sess.SetProgressHook(hook)
	user := prompts.Render(prompts.DedupeUser, map[string]string{"IMAGES": fmt.Sprintf("candidates:manifest.md (%d images in this session; similarity buckets are a starting point; edit candidates:groups.json with motif_groups; every group needs key)", total)})
	// 中断续跑（T121）：同一张图集必然落到同一个转录名，所以"有没有历史"就是
	// "上次是不是被中断了"。走与其他会话完全相同的续跑路径——回放历史、计数
	// 接着数、已 submit 的直接收账（不再请求模型），未提交的原样续行。
	resumed := false
	if msgs, st, lerr := session.LoadTranscriptStats(trPath); lerr != nil {
		r.log.LogWarning(0, "[dedupe] 转录读取失败（按全新会话继续）:", lerr)
	} else if len(msgs) > 0 {
		// 证据也要跟着历史回来：seen.json 是最快的路径，但旧版本跑出来的转
		// 录没有这个文件，压缩点之后的回放也可能漏掉早期的看图调用——转录
		// 里的看图表征同样是证据，必须并入，否则重放的 submit 会被判"没看过"。
		for id := range motifEvidenceFromMessages(msgs, view, len(sc.Members)) {
			seen[id] = true
		}
		imageTool.saveSeen()
		sess.SetMessages(msgs)
		sess.SeedCounters(st.Rounds, st.Tools)
		resumeLog(r.log, 0, label, filepath.Base(trPath), msgs, st)
		if replaySubmit(r.log, 0, label, submit, st) {
			resumed = true
		} else {
			user = resumeUserText(msgs, user)
		}
	}
	if !resumed {
		_, runErr := sess.Run(session.RunOptions{UserText: user})
		for attempts := 0; runErr == nil && !submit.done.Submitted && attempts < 2; attempts++ {
			_, runErr = sess.Run(session.RunOptions{UserText: "You have NOT called submit yet. Review the persisted groups.json if needed, then call submit with no arguments."})
		}
		if runErr != nil {
			return nil, runErr
		}
	}
	if !submit.done.Submitted {
		return nil, fmt.Errorf("dedupe session ended without submit")
	}
	return submit.groups.Groups()
}

// motifPrefillBuckets turns the local similarity buckets that lie entirely
// inside this session into its starting groups. Keys stay 0: the AI must pick
// the representative.
func motifPrefillBuckets(members []*motifCandidate, advisory map[string]motifAdvisory) []MotifGroup {
	buckets := map[int][]int{}
	var nos []int
	for i, c := range members {
		a := advisory[c.task.key()]
		if a.Group <= 0 {
			continue
		}
		if _, ok := buckets[a.Group]; !ok {
			nos = append(nos, a.Group)
		}
		buckets[a.Group] = append(buckets[a.Group], i+1)
	}
	sort.Ints(nos)
	out := make([]MotifGroup, 0, len(nos))
	for _, g := range nos {
		if len(buckets[g]) < 2 {
			continue
		}
		out = append(out, MotifGroup{Members: buckets[g]})
	}
	return out
}

// prepareMotifScopeView materialises one session's workspace: originals for its
// members, keys/ for the group representatives it may attach images to, the
// manifest, and the pre-filled starting groups. No contact sheets.
func prepareMotifScopeView(root string, sc *motifScope) error {
	originals := filepath.Join(root, "originals")
	if err := os.MkdirAll(originals, 0755); err != nil {
		return err
	}
	link := func(src, dst string) error {
		if fileExists(dst) {
			return nil
		}
		if err := linkAbs(src, dst); err != nil {
			return copyFile(src, dst)
		}
		return nil
	}
	for i, c := range sc.Members {
		if err := link(c.path, filepath.Join(root, "originals", fmt.Sprintf("%06d%s", i+1, filepath.Ext(c.path)))); err != nil {
			return err
		}
	}
	keysDir := filepath.Join(root, "keys")
	if len(sc.Keys) > 0 {
		if err := os.MkdirAll(keysDir, 0755); err != nil {
			return err
		}
	}
	for i, c := range sc.Keys {
		if err := link(c.path, filepath.Join(keysDir, fmt.Sprintf("%06d%s", i+1, filepath.Ext(c.path)))); err != nil {
			return err
		}
	}

	var b strings.Builder
	b.WriteString("# Images in this verification session\n\n")
	b.WriteString("Similarity buckets below are a STARTING grouping (already written into `groups.json`); they can be split, and cross-bucket merges are allowed.\n\n")
	b.WriteString("There are NO contact sheets: open each original with `view_image` (`originals/000123.jpg`). A merge is only accepted for images you opened yourself, uncropped. Every group needs `key` = its single most representative member. A missed duplicate is acceptable; merging two different drawings loses content.\n\n")
	if len(sc.Prefill) > 0 {
		b.WriteString("## Starting buckets\n\n")
		for i, g := range sc.Prefill {
			fmt.Fprintf(&b, "### bucket %d (%d images)\n\n", i+1, len(g.Members))
			for _, id := range g.Members {
				c := sc.Members[id-1]
				fmt.Fprintf(&b, "- %d. originals/%06d%s | %s | %s | class=%s\n", id, id, filepath.Ext(c.path), c.task.mdName, c.task.imgPath, motifClassOf(c))
			}
			b.WriteString("\n")
		}
	}
	if len(sc.Members) > 0 {
		b.WriteString("## All images in this session\n\n")
		for i, c := range sc.Members {
			hint := "none"
			if a := sc.Advisory[c.task.key()]; a.Group > 0 {
				hint = fmt.Sprintf("bucket-%d", a.Group)
				if a.Exact {
					hint += ":exact-pixels"
				}
			}
			fmt.Fprintf(&b, "- %d. originals/%06d%s | %s | %s | class=%s | similarity_hint=%s\n", i+1, i+1, filepath.Ext(c.path), c.task.mdName, c.task.imgPath, motifClassOf(c), hint)
		}
		b.WriteString("\n")
	}
	if len(sc.Keys) > 0 {
		off := len(sc.Members)
		b.WriteString("## Existing groups (by key image)\n\n")
		b.WriteString("Each `keys/` file is the representative of a group confirmed earlier. To place an image into one of these groups, add it to that key's group: `{\"op\":\"add\",\"group\":[<image id>, <key id>],\"key\":<key id>}`. Use the key id for groups you keep as they are; do not add members you never looked at.\n\n")
		for i, c := range sc.Keys {
			desc := motifScopeGroup{KeyID: off + i + 1}
			for _, g := range sc.Groups {
				if g.KeyID == desc.KeyID {
					desc = g
					break
				}
			}
			fmt.Fprintf(&b, "- key id %d. keys/%06d%s | %s | %s | class=%s", desc.KeyID, i+1, filepath.Ext(c.path), c.task.mdName, c.task.imgPath, motifClassOf(c))
			if desc.Label != "" {
				fmt.Fprintf(&b, " | group=%s", desc.Label)
			}
			if len(desc.Members) > 0 {
				fmt.Fprintf(&b, " | members=%d", len(desc.Members))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.md"), []byte(b.String()), 0644); err != nil {
		return err
	}

	// Pre-fill only when nothing is persisted yet: a re-entered (resumed) session
	// must find the grouping it had already built, otherwise the restart silently
	// throws away the model's motif_groups edits (T121).
	groupsPath := filepath.Join(root, "groups.json")
	if !fileExists(groupsPath) {
		start := sc.Prefill
		if start == nil {
			start = []MotifGroup{}
		}
		payload, err := json.MarshalIndent(map[string]any{"groups": start}, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(groupsPath, append(payload, '\n'), 0644); err != nil {
			return err
		}
	}

	items := make([]map[string]any, 0, len(sc.Members))
	for i, c := range sc.Members {
		a := sc.Advisory[c.task.key()]
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
		"description":      "Local similarity is advisory only. The AI decides the final structural groups; images with no hint are still eligible.",
		"session_images":   len(sc.Members),
		"existing_key_ids": len(sc.Keys),
		"items":            items,
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

// motifReceiptRe matches the receipt view_image prints for a successful view:
// "Image originals/000123.png (crop 0%,0%-100%,100%, width 1280px) attached."
var motifReceiptRe = regexp.MustCompile(`^Image (\S+) \(crop (\d+)%,(\d+)%-(\d+)%,(\d+)%, width \d+px\) attached\.`)

// motifEvidenceFromMessages rebuilds the "already inspected" set from a replayed
// transcript. seen.json is the fast path, but a transcript recorded before that
// file existed (older binary), or one whose early turns were dropped by
// compaction, still carries the proof in its receipts — resume must not lose
// evidence it can see. Only an uncropped view of an individual original counts,
// exactly like the live tool, so the gate itself is not weakened.
func motifEvidenceFromMessages(msgs []session.ChatMessage, view string, keyOffset int) map[int]bool {
	seen := map[int]bool{}
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		for _, line := range strings.Split(session.ContentString(m), "\n") {
			path, ok := motifUncroppedReceipt(strings.TrimSpace(line))
			if !ok {
				continue
			}
			if id := motifReceiptID(view, path, keyOffset); id > 0 {
				seen[id] = true
			}
		}
	}
	return seen
}

// motifUncroppedReceipt returns the viewed path when the line is a full-frame
// receipt. %.0f prints whole percentages, so a full frame is always 0,0-100,100.
func motifUncroppedReceipt(line string) (string, bool) {
	m := motifReceiptRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	if m[2] != "0" || m[3] != "0" || m[4] != "100" || m[5] != "100" {
		return "", false
	}
	return strings.TrimPrefix(m[1], "candidates:"), true
}

// motifReceiptID maps a receipt path to its session id. Models may name the file
// as originals/000123.png or as a bare 000123.png (view_image resolves bare names
// inside the workspace), so a bare name is looked up on disk.
func motifReceiptID(view, path string, keyOffset int) int {
	if id := motifScopeID(path, keyOffset); id > 0 {
		return id
	}
	base := filepath.Base(path)
	if strings.ContainsAny(base, "/\\") {
		return 0
	}
	for _, dir := range []string{"originals", "keys"} {
		if _, err := os.Stat(filepath.Join(view, dir, base)); err != nil {
			continue
		}
		return motifDirID(dir, base, keyOffset)
	}
	return 0
}

// motifDirID applies the numbering of motifScopeID inside a known subdirectory.
func motifDirID(dir, base string, keyOffset int) int {
	dot := strings.IndexByte(base, '.')
	if dot <= 0 {
		return 0
	}
	id, err := strconv.Atoi(base[:dot])
	if err != nil || id < 1 {
		return 0
	}
	if dir == "keys" {
		return keyOffset + id
	}
	return id
}
