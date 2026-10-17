package latex

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/session"
)

func TestParseMotifGroupsRejectsInvalidAndProtectsAnnotations(t *testing.T) {
	for _, bad := range []string{
		`{"groups":[[1,2],[2,3]]}`, `{"groups":[[1,4]]}`,
		`{"groups":[[1,1]]}`, `{"groups":[[]]}`, `{"groups":[[1,2.5]]}`,
		`{"groups":"1,2"}`, `not json`,
	} {
		if _, err := parseMotifGroups(bad, 3); err == nil {
			t.Errorf("accepted unsafe verdict %s", bad)
		}
	}
	got, err := parseMotifGroups(`{"groups":[[1,2]]}`, 3)
	if err != nil || len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("groups=%v err=%v", got, err)
	}
	// The third (numbered) image remains independent when omitted by AI.
	if got[0][0] != 1 || got[0][1] != 2 {
		t.Fatalf("wrong group: %v", got)
	}
}

func TestMotifEquivalenceOneArtifactAndRetainedRefs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "figures"), 0755); err != nil {
		t.Fatal(err)
	}
	tex := `\begin{tikzpicture}\node{z};\end{tikzpicture}`
	a := &task{mdName: "book.md", imgPath: "images/book/a.jpg"}
	b := &task{mdName: "book.md", imgPath: "images/book/b.jpg"}
	c := &task{mdName: "book.md", imgPath: "images/book/numbered.jpg"}
	pa := &imageProgress{Key: a.key(), MDName: a.mdName, ImgPath: a.imgPath, Class: ClassVector, Status: "classified"}
	pb := &imageProgress{Key: b.key(), MDName: b.mdName, ImgPath: b.imgPath, Class: ClassVector, Status: "classified"}
	pc := &imageProgress{Key: c.key(), MDName: c.mdName, ImgPath: c.imgPath, Class: ClassVector, Status: "classified"}
	ca := &motifCandidate{task: a, progress: pa, sourceHash: "hash-a"}
	cb := &motifCandidate{task: b, progress: pb, sourceHash: "hash-b"}
	applyMotifEquivalence([]*motifCandidate{cb, ca}, filepath.Join(dir, "progress_items"))
	if pb.EquivalentTo != a.key() || pc.EquivalentTo != "" || pb.DedupeLeaderHash != "hash-a" {
		t.Fatalf("unsafe relationships: %+v %+v", pb, pc)
	}
	pa.Status = "done"
	pa.TikzCode = tex
	pa.FigPDF = "figures/leader.pdf"
	if err := os.WriteFile(filepath.Join(dir, "figures", "leader.pdf"), []byte("PDF"), 0644); err != nil {
		t.Fatal(err)
	}
	prog := map[string]*imageProgress{a.key(): pa, b.key(): pb, c.key(): pc}
	applyMotifArtifacts([]*task{a, b, c}, prog, filepath.Join(dir, "progress_items"), dir)
	if pb.Status != "done" || pb.TikzCode != tex || pb.Absorbed || pc.Status != "classified" {
		t.Fatalf("artifacts lost refs: %+v %+v", pb, pc)
	}
	materializeSharedMotifTeX([]*task{a, b, c}, prog, filepath.Join(dir, "progress_items"), dir)
	if pa.SharedTex == "" || pa.SharedTex != pb.SharedTex || pc.SharedTex != "" {
		t.Fatalf("shared paths: %+v %+v %+v", pa, pb, pc)
	}
	bytes, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(pa.SharedTex)))
	if err != nil || strings.TrimSpace(string(bytes)) != tex {
		t.Fatalf("shared body=%q err=%v", bytes, err)
	}
	if strings.Contains(string(bytes), `\documentclass`) {
		t.Fatal("standalone wrapper is unsafe inside chapters")
	}
	loaded := map[string]*imageProgress{}
	loadProgress(filepath.Join(dir, "progress_items"), loaded)
	if loaded[b.key()].EquivalentTo != a.key() || loaded[b.key()].SharedTex != pa.SharedTex {
		t.Fatalf("resume relation lost: %+v", loaded[b.key()])
	}
}

func TestSharedMotifEmbedUsesOneInput(t *testing.T) {
	r := testRunner(t, true)
	root := t.TempDir()
	shared := "figures/motif_cafe.tex"
	if err := os.MkdirAll(filepath.Join(root, "figures"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, shared), []byte(`\begin{tikzpicture}\end{tikzpicture}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"images/book/a.png", "images/book/b.png"} {
		p := &imageProgress{Class: ClassVector, TikzCode: `\begin{tikzpicture}\end{tikzpicture}`, SharedTex: shared, ImgPath: path}
		block := r.embedBlock(p, "book.md", root)
		if !strings.Contains(block, `\input{`+shared+"}") || strings.Contains(block, `\begin{tikzpicture}`) {
			t.Fatalf("shared resource not reused for %s: %s", path, block)
		}
	}
}

func TestVerifyMotifCandidatesIndependentVisionSession(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		requests = append(requests, string(body))
		switch len(requests) {
		case 1:
			jsonToolReply(w, "read_file", `{"path":"candidates:manifest.md"}`)
		case 2, 3, 4:
			// Evidence must come from individual originals: no contact sheets exist.
			jsonToolReply(w, "view_image", fmt.Sprintf(`{"path":"originals/%06d.png"}`, len(requests)-1))
		case 5:
			jsonToolReply(w, "motif_groups", `{"op":"add","group":[1,2,3],"key":2}`)
		default:
			jsonToolReply(w, "submit", `{}`)
		}
	}))
	defer srv.Close()
	r := keepTestRunner(t)
	stream := false
	r.cfg.Latex.DedupeModel = ""
	r.cfg.Models = map[string]config.ModelConfig{"text": {BaseURL: srv.URL, APIKey: "key", Model: "text-vision", Stream: &stream}}
	r.clients = map[string]*session.Client{}
	r.models = map[string]config.ModelConfig{}
	root := t.TempDir()
	a := tinyPNG(t, filepath.Join(root, "a.png"))
	var members []*motifCandidate
	for i := 0; i < 12; i++ {
		members = append(members, &motifCandidate{task: &task{mdName: "book.md", imgPath: fmt.Sprintf("original-%d.png", i+1)}, path: a, sourceHash: fmt.Sprint(i)})
	}
	groups, err := r.verifyMotifCandidates(members, root, 0, 0)
	if err != nil || len(groups) != 1 || len(groups[0].Members) != 3 || groups[0].Key != 2 {
		t.Fatalf("verdict=%v err=%v", groups, err)
	}
	if len(requests) != 6 {
		t.Fatalf("one tool session expected: %d requests", len(requests))
	}
	if strings.Contains(requests[0], `"type":"image_url"`) || !strings.Contains(requests[0], "12 images") {
		t.Fatalf("initial message must contain the manifest, not attachments: %.900s", requests[0])
	}
	imageSeen := false
	for _, r := range requests[1:] {
		if strings.Contains(r, `"type":"image_url"`) {
			imageSeen = true
			break
		}
	}
	if !imageSeen {
		t.Fatal("view_image receipt must provide actual visual input")
	}
	if strings.Contains(requests[0], "sheets/") {
		t.Fatal("contact sheets must not be advertised any more")
	}
	transcripts, err := filepath.Glob(filepath.Join(root, "sessions", "motif_dedupe_*.jsonl"))
	if err != nil || len(transcripts) != 1 {
		t.Fatalf("transcripts=%v err=%v", transcripts, err)
	}
	data, _ := os.ReadFile(transcripts[0])
	for _, want := range []string{`"kind":"system"`, `"name":"view_image"`, `"name":"submit"`, `SUBMITTED.`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("transcript missing %s", want)
		}
	}
}

// 相似桶直接预填进 groups.json（key 留空由 AI 选），并且不再生成联系图：
// 证据只能来自逐张原图，避免"看几张拼图就声称看过几百张"。
func TestMotifPoolPrefillsBucketsWithoutContactSheets(t *testing.T) {
	root := t.TempDir()
	path := tinyPNG(t, filepath.Join(root, "original.png"))
	img, err := decodeImage(path)
	if err != nil {
		t.Fatal(err)
	}
	members := make([]*motifCandidate, 336)
	advisory := map[string]motifAdvisory{}
	for i := range members {
		members[i] = &motifCandidate{task: &task{mdName: "book.md", imgPath: fmt.Sprintf("image-%d.png", i+1)}, path: path, image: img, progress: &imageProgress{Class: ClassVector}}
		if i < 12 {
			advisory[members[i].task.key()] = motifAdvisory{Group: 1, Exact: true}
		}
	}
	view := filepath.Join(root, "view")
	if err := prepareMotifScopeView(view, &motifScope{Members: members, Advisory: advisory, Prefill: motifPrefillBuckets(members, advisory)}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(view, "sheets")); !os.IsNotExist(err) {
		t.Fatalf("contact sheets must not be generated: %v", err)
	}
	var stored struct {
		Groups []MotifGroup `json:"groups"`
	}
	raw, err := os.ReadFile(filepath.Join(view, "groups.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Groups) != 1 || len(stored.Groups[0].Members) != 12 || stored.Groups[0].Key != 0 {
		t.Fatalf("buckets must be pre-filled with an unset key: %s", raw)
	}
	seen := map[int]bool{}
	vt := &motifViewTool{base: &ViewImageTool{Root: view}, seen: seen}
	if res, err := vt.Execute(`{"path":"originals/000001.png"}`); err != nil || res.ImageBase64 == "" {
		t.Fatalf("view original: %v", err)
	}
	if len(seen) != 1 || !seen[1] {
		t.Fatalf("viewing one original must mark exactly that id: %v", seen)
	}
}

func TestMotifSubmitRequiresVisualEvidence(t *testing.T) {
	root := t.TempDir()
	groups := NewMotifGroupsTool(filepath.Join(root, "groups.json"), 3)
	tool := &motifSubmitTool{groups: groups, seen: map[int]bool{1: true}}
	res, err := groups.Execute(`{"op":"add","group":[1,2],"key":1}`)
	if err != nil || strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("stage group failed: %+v %v", res, err)
	}
	res, err = tool.Execute(`{}`)
	if err != nil || res.Terminal || tool.done.Submitted || !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("unseen merge accepted: %+v %v", res, err)
	}
	tool.seen[2] = true
	res, err = tool.Execute(`{}`)
	if err != nil || !res.Terminal || !tool.done.Submitted {
		t.Fatalf("valid staged group rejected: %+v %v", res, err)
	}
}

func TestMotifGroupsIncrementalOperations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.json")
	tool := NewMotifGroupsTool(path, 6)
	for _, raw := range []string{`{"op":"add","group":[1,2],"key":1}`, `{"op":"add","group":[3,4],"key":4}`, `{"op":"update","n":2,"group":[3,5]}`, `{"op":"delete","n":1}`} {
		res, err := tool.Execute(raw)
		if err != nil || strings.Contains(res.Text, "REJECTED") {
			t.Fatalf("op %s: %+v %v", raw, res, err)
		}
	}
	got, err := tool.Groups()
	// key 4 was replaced by a member edit, so the representative is unset again
	if err != nil || len(got) != 1 || len(got[0].Members) != 2 || got[0].Members[0] != 3 || got[0].Members[1] != 5 || got[0].Key != 0 {
		t.Fatalf("groups=%v err=%v", got, err)
	}
	res, err := tool.Execute(`{"op":"add","groups":[[5,6],[2,2]]}`)
	if err != nil || !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("invalid batch accepted: %+v %v", res, err)
	}
	got, _ = tool.Groups()
	if len(got) != 1 || got[0].Members[0] != 3 || got[0].Members[1] != 5 {
		t.Fatalf("invalid batch changed state: %v", got)
	}
}

// 相似度只作参考：advisory 文件写出初始组与像素一致标记，manifest 每行带
// similarity_hint；未命中相似度的图片同样出现在全量清单里。
func TestMotifAdvisoryIsReferenceOnly(t *testing.T) {
	root := t.TempDir()
	a := tinyPNG(t, filepath.Join(root, "a.png"))
	img, err := decodeImage(a)
	if err != nil {
		t.Fatal(err)
	}
	b := tinyPNG(t, filepath.Join(root, "b.png"))
	imgB, err := decodeImage(b)
	if err != nil {
		t.Fatal(err)
	}
	members := []*motifCandidate{
		{task: &task{mdName: "book.md", imgPath: "a.png"}, path: a, image: img, sourceHash: "ha", progress: &imageProgress{Class: ClassVector}},
		{task: &task{mdName: "book.md", imgPath: "b.png"}, path: b, image: imgB, sourceHash: "hb", progress: &imageProgress{Class: ClassVector}},
		{task: &task{mdName: "book.md", imgPath: "c.png"}, path: a, image: img, sourceHash: "hc", progress: &imageProgress{Class: ClassVector}},
	}
	advisory := map[string]motifAdvisory{
		members[0].task.key(): {Group: 1, Exact: true},
		members[1].task.key(): {Group: 1},
	}
	if err := prepareMotifScopeView(filepath.Join(root, "view"), &motifScope{Members: members, Advisory: advisory, Prefill: motifPrefillBuckets(members, advisory)}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "view", "similarity_advisory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"initial_group": 1`) || !strings.Contains(string(raw), `"session_images": 3`) {
		t.Fatalf("advisory JSON missing group/pool data: %s", raw)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "view", "manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(manifest)
	if !strings.Contains(text, "### bucket 1 (2 images)") {
		t.Fatalf("manifest missing the initial bucket: %s", text)
	}
	// 第三张没有相似提示，但必须仍在全量清单里（不被排除）。
	if !strings.Contains(text, "## All images in this session") || !strings.Contains(text, "3. originals/000003.png | ") || !strings.Contains(text, "similarity_hint=none") {
		t.Fatalf("unmatched image must stay in the pool: %s", text)
	}
}

// 按相似组分配会话：大桶单独一个会话，小桶与未命中图按 dedupe_batch_images 合并；
// 0 表示退回"整池一个会话"。
func TestMotifBucketScopesFollowBucketSize(t *testing.T) {
	r := keepTestRunner(t)
	mk := func(n int) []*motifCandidate {
		out := make([]*motifCandidate, n)
		for i := range out {
			out[i] = &motifCandidate{task: &task{mdName: "book.md", imgPath: fmt.Sprintf("i%d.png", i)}}
		}
		return out
	}
	pool := mk(120)
	advisory := map[string]motifAdvisory{}
	for i, c := range pool {
		switch {
		case i < 60: // 超过 50：独占一个会话
			advisory[c.task.key()] = motifAdvisory{Group: 1}
		case i < 70: // 小桶，可与其它小桶合并
			advisory[c.task.key()] = motifAdvisory{Group: 2}
		case i < 80:
			advisory[c.task.key()] = motifAdvisory{Group: 3}
		}
		// 其余 40 张没有任何相似桶 -> 未分配
	}
	r.cfg.Latex.DedupeGroupSessionOver = 50
	r.cfg.Latex.DedupeBatchImages = 150
	scopes, unassigned := r.motifBucketScopes(pool, advisory)
	if len(unassigned) != 40 {
		t.Fatalf("unassigned=%d, want 40", len(unassigned))
	}
	if len(scopes) != 2 {
		t.Fatalf("scopes=%d, want 2 (large bucket + packed small buckets)", len(scopes))
	}
	if len(scopes[0].Members) != 60 || len(scopes[0].Prefill) != 1 {
		t.Fatalf("large bucket must own a session: %d members, %d prefill", len(scopes[0].Members), len(scopes[0].Prefill))
	}
	if len(scopes[1].Members) != 20 || len(scopes[1].Prefill) != 2 {
		t.Fatalf("small buckets must be packed together: %d members, %d prefill", len(scopes[1].Members), len(scopes[1].Prefill))
	}
	// 单会话容量上限：小桶加起来超过 dedupe_batch_images 时再拆一个会话。
	r.cfg.Latex.DedupeBatchImages = 10
	scopes, _ = r.motifBucketScopes(pool, advisory)
	for _, sc := range scopes {
		if len(sc.Members) > 60 {
			t.Fatalf("bucket session exceeded its bucket: %d", len(sc.Members))
		}
	}
	total := 0
	for _, sc := range scopes {
		total += len(sc.Members)
	}
	if total != 80 {
		t.Fatalf("bucket members lost: %d/80", total)
	}
	// 0 = 旧行为：整池一个会话，没有"未分配"概念。
	r.cfg.Latex.DedupeGroupSessionOver = 0
	scopes, unassigned = r.motifBucketScopes(pool, advisory)
	if len(scopes) != 1 || len(scopes[0].Members) != 120 || len(unassigned) != 0 {
		t.Fatalf("group_session_over=0 must keep one session, got %d scopes / %d unassigned", len(scopes), len(unassigned))
	}
}

// 整理会话只拿"可能属于已确认组"的未分配图 + 关键图；正常书里大多数图彼此
// 无关，所以通常一个候选都没有，也就不开这个会话。
func TestMotifConsolidationOnlyTakesLocalCandidates(t *testing.T) {
	root := t.TempDir()
	path := tinyPNG(t, filepath.Join(root, "a.png"))
	img, err := decodeImage(path)
	if err != nil {
		t.Fatal(err)
	}
	key := &motifCandidate{task: &task{mdName: "book.md", imgPath: "key.png"}, path: path, image: img}
	like := &motifCandidate{task: &task{mdName: "book.md", imgPath: "like.png"}, path: path, image: img}
	other := &motifCandidate{task: &task{mdName: "book.md", imgPath: "other.png"}, path: path}
	applied := []motifAppliedGroup{{Key: key, Members: []*motifCandidate{key, like}}}
	cands, keys := motifConsolidationCandidates([]*motifCandidate{like, other}, applied)
	if len(keys) != 1 || len(cands) != 1 || cands[0] != like {
		t.Fatalf("candidates=%v keys=%v", cands, keys)
	}
	// 没有已确认组 / 没有未分配图 -> 不开会话。
	if c, k := motifConsolidationCandidates(nil, applied); c != nil || k != nil {
		t.Fatal("no unassigned images must not open a consolidation session")
	}
	if c, k := motifConsolidationCandidates([]*motifCandidate{other}, nil); c != nil || k != nil {
		t.Fatal("no confirmed groups must not open a consolidation session")
	}
}

// 代码侧的保守闸门：与代表图相似度不足的成员即使被 AI 并进组里，也保持独立
// （宁可漏掉重复，也不能合并后丢信息）。
func TestMotifMergeGateRejectsDissimilarMembers(t *testing.T) {
	root := t.TempDir()
	same := tinyPNG(t, filepath.Join(root, "same.png"))
	img, err := decodeImage(same)
	if err != nil {
		t.Fatal(err)
	}
	// 一张完全不同结构的图：纯色大块横条，与 tinyPNG 的点阵毫无相似度。
	otherPath := filepath.Join(root, "other.png")
	other := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			other.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	f, err := os.Create(otherPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, other); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	otherImg, err := decodeImage(otherPath)
	if err != nil {
		t.Fatal(err)
	}
	if MotifSimilarTo(img, otherImg) {
		t.Fatal("dissimilar images must not pass the merge gate")
	}
	if !MotifSimilarTo(img, img) {
		t.Fatal("identical images must pass the merge gate")
	}
}

// 相似度会传递（A~B、B~C 连成一条链），真实事故里 336 张图被并成一个"桶"，
// 逼着模型逐张看了 336 次还注定过不了合并闸门。非逐像素桶超过上限就切成
// seed 锚定的子桶；逐像素桶是可靠的，保持完整。
func TestSplitChainedMotifBuckets(t *testing.T) {
	var fuzzy []string
	for i := 0; i < 336; i++ {
		fuzzy = append(fuzzy, fmt.Sprintf("k%03d", i))
	}
	groups := []VisualMotifGroup{
		{Members: fuzzy, Exact: false},
		{Members: []string{"a", "b", "c"}, Exact: true},
		{Members: []string{"x", "y"}, Exact: false},
	}
	got := splitChainedMotifBuckets(groups, 50)
	var total int
	for _, g := range got {
		if len(g.Members) > 50 {
			t.Fatalf("fuzzy bucket not split: %d", len(g.Members))
		}
		if g.Exact && len(g.Members) != 3 {
			t.Fatalf("exact bucket must stay whole: %v", g.Members)
		}
		total += len(g.Members)
	}
	if total != 336+3+2 {
		t.Fatalf("members lost: %d", total)
	}
	// 逐像素桶即便超过上限也不切（切了只会多画几张）。
	big := make([]string, 120)
	for i := range big {
		big[i] = fmt.Sprintf("e%03d", i)
	}
	got = splitChainedMotifBuckets([]VisualMotifGroup{{Members: big, Exact: true}}, 50)
	if len(got) != 1 || len(got[0].Members) != 120 {
		t.Fatalf("exact bucket must not be split: %d groups", len(got))
	}
}
