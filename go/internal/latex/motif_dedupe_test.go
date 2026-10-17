package latex

import (
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
		`{"groups":[[1,1]]}`, `{"groups":[[1]]}`, `{"groups":[[1,2.5]]}`,
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
	var requestBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		requestBody = string(body)
		jsonReply(w, `{"groups":[[1,2]]}`)
	}))
	defer srv.Close()
	r := keepTestRunner(t)
	stream := false
	r.cfg.Latex.DedupeModel = "dedupe"
	r.cfg.Latex.ClassifierModel = "classifier"
	r.cfg.Models = map[string]config.ModelConfig{
		"dedupe":     {BaseURL: srv.URL, APIKey: "key", Model: "dedupe-vision", Stream: &stream},
		"classifier": {BaseURL: srv.URL, APIKey: "key", Model: "classifier-vision", Stream: &stream},
	}
	r.clients = map[string]*session.Client{}
	r.models = map[string]config.ModelConfig{}
	root := t.TempDir()
	a := tinyPNG(t, filepath.Join(root, "a.png"))
	b := tinyPNG(t, filepath.Join(root, "b.png"))
	members := []*motifCandidate{
		{task: &task{mdName: "book.md", imgPath: "a.png"}, path: a, sourceHash: "ha"},
		{task: &task{mdName: "book.md", imgPath: "b.png"}, path: b, sourceHash: "hb"},
	}
	groups, err := r.verifyMotifCandidates(members, root, 0, 0)
	if err != nil || len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatalf("verdict=%v err=%v", groups, err)
	}
	if strings.Count(requestBody, `"type":"image_url"`) != 2 || !strings.Contains(requestBody, "dedupe-vision") || !strings.Contains(requestBody, "b.png") {
		t.Fatalf("dedupe must see both originals and use its own model: %.650s", requestBody)
	}
	transcripts, err := filepath.Glob(filepath.Join(root, "sessions", "motif_dedupe_*.jsonl"))
	if err != nil || len(transcripts) != 1 {
		t.Fatalf("viewer transcript=%v err=%v", transcripts, err)
	}
}

func TestMotifCandidatesBatchesIncludeAnchor(t *testing.T) {
	members := make([]*motifCandidate, 23)
	for i := range members {
		members[i] = &motifCandidate{task: &task{imgPath: string(rune('a' + i))}}
	}
	batches := motifCandidateBatches(members, 10)
	if len(batches) != 3 || len(batches[0]) != 10 || len(batches[1]) != 10 || len(batches[2]) != 5 {
		t.Fatalf("batches=%v", batches)
	}
	for _, b := range batches {
		if b[0] != members[0] {
			t.Fatal("cross-batch canonical anchor lost")
		}
	}
}
