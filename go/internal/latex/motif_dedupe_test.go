package latex

import (
	"encoding/json"
	"fmt"
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
		case 2:
			jsonToolReply(w, "view_image", `{"path":"sheets/001.png"}`)
		case 3:
			jsonToolReply(w, "motif_groups", `{"op":"add","group":[1,2,3,4,5,6,7,8,9,10,11,12]}`)
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
	if err != nil || len(groups) != 1 || len(groups[0]) != 12 {
		t.Fatalf("verdict=%v err=%v", groups, err)
	}
	if len(requests) != 4 {
		t.Fatalf("whole group must use one tool session: %d requests", len(requests))
	}
	if strings.Contains(requests[0], `"type":"image_url"`) || !strings.Contains(requests[0], "text-vision") || !strings.Contains(requests[0], "12 images") {
		t.Fatalf("initial message must contain full manifest, not attachments: %.900s", requests[0])
	}
	if !strings.Contains(requests[2], `"type":"image_url"`) {
		t.Fatal("view_image receipt must provide actual visual input")
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

func TestMotifWholeGroup336Images(t *testing.T) {
	root := t.TempDir()
	path := tinyPNG(t, filepath.Join(root, "original.png"))
	img, err := decodeImage(path)
	if err != nil {
		t.Fatal(err)
	}
	members := make([]*motifCandidate, 336)
	for i := range members {
		members[i] = &motifCandidate{task: &task{mdName: "book.md", imgPath: fmt.Sprintf("image-%d.png", i+1)}, path: path, image: img}
	}
	covered, err := prepareMotifView(filepath.Join(root, "view"), members)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	view := &motifViewTool{base: &ViewImageTool{Root: filepath.Join(root, "view")}, covered: covered, seen: seen}
	for i := 1; i <= 6; i++ {
		res, err := view.Execute(fmt.Sprintf(`{"path":"sheets/%03d.png"}`, i))
		if err != nil || res.ImageBase64 == "" {
			t.Fatalf("sheet %d: %v", i, err)
		}
	}
	if len(seen) != 336 {
		t.Fatalf("viewed %d/336", len(seen))
	}
	ids := make([]int, 336)
	for i := range ids {
		ids[i] = i + 1
	}
	raw, _ := json.Marshal(map[string]any{"group": ids, "op": "add"})
	groupTool := NewMotifGroupsTool(filepath.Join(root, "groups.json"), 336)
	if res, err := groupTool.Execute(string(raw)); err != nil || strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("stage whole group: %+v %v", res, err)
	}
	submit := &motifSubmitTool{groups: groupTool, seen: seen}
	res, err := submit.Execute(`{}`)
	if err != nil || !res.Terminal || !submit.done.Submitted {
		t.Fatalf("whole group lost: %v %v", res.Text, err)
	}
}

func TestMotifSubmitRequiresVisualEvidence(t *testing.T) {
	root := t.TempDir()
	groups := NewMotifGroupsTool(filepath.Join(root, "groups.json"), 3)
	tool := &motifSubmitTool{groups: groups, seen: map[int]bool{1: true}}
	res, err := groups.Execute(`{"op":"add","group":[1,2]}`)
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
	for _, raw := range []string{`{"op":"add","group":[1,2]}`, `{"op":"add","group":[3,4]}`, `{"op":"update","n":2,"group":[3,5]}`, `{"op":"delete","n":1}`} {
		res, err := tool.Execute(raw)
		if err != nil || strings.Contains(res.Text, "REJECTED") {
			t.Fatalf("op %s: %+v %v", raw, res, err)
		}
	}
	got, err := tool.Groups()
	if err != nil || len(got) != 1 || len(got[0]) != 2 || got[0][0] != 3 || got[0][1] != 5 {
		t.Fatalf("groups=%v err=%v", got, err)
	}
	res, err := tool.Execute(`{"op":"add","groups":[[5,6],[2,2]]}`)
	if err != nil || !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("invalid batch accepted: %+v %v", res, err)
	}
	got, _ = tool.Groups()
	if len(got) != 1 || got[0][0] != 3 || got[0][1] != 5 {
		t.Fatalf("invalid batch changed state: %v", got)
	}
}
