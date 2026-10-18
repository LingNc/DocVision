package latex

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/session"
)

type subdivisionAPIRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Function struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"function"`
	} `json:"tools"`
}

func subdivisionReply(w http.ResponseWriter, calls ...map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{
		"role": "assistant", "content": "", "tool_calls": calls,
	}}}})
}

func subdivisionCall(id, name string, args any) map[string]any {
	encoded, _ := json.Marshal(args)
	return map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}
}

func subdivisionWrite(path, content string) map[string]any {
	return subdivisionCall("write-"+path, "write_file", map[string]any{"path": path, "content": content})
}

func TestRecursiveChapterConversionChecksChildrenBeforePromotion(t *testing.T) {
	proj := t.TempDir()
	workDir := filepath.Join(proj, "work")
	chapters := filepath.Join(proj, "chapters")
	style := filepath.Join(proj, "style")
	for _, dir := range []string{workDir, chapters, style} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	chapterPath := filepath.Join(chapters, "chapter_01.md")
	if err := os.WriteFile(chapterPath, []byte("# First\nFirst text\n# Second\nSecond text"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"book.cls":    "\\ProvidesClass{book}\n",
		"manual.md":   "# Manual\n",
		"example.tex": "\\section{Example}\n",
	} {
		if err := os.WriteFile(filepath.Join(style, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var requests atomic.Int32
	var formalWasPremature atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var apiReq subdivisionAPIRequest
		if err := json.Unmarshal(body, &apiReq); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		lastUser := ""
		for _, message := range apiReq.Messages {
			if message.Role == "user" {
				lastUser = message.Content
			}
		}
		toolset := map[string]bool{}
		submitDescription := ""
		for _, tool := range apiReq.Tools {
			toolset[tool.Function.Name] = true
			if tool.Function.Name == "submit" {
				submitDescription = tool.Function.Description
			}
		}
		n := requests.Add(1)
		switch {
		case toolset["read_file"] && toolset["grep"] && !toolset["write_file"]:
			subdivisionReply(w, subdivisionCall("checker-"+string(rune('0'+n)), "submit", map[string]any{"report": map[string]any{"status": "pass"}}))
		case strings.Contains(lastUser, "All child chapters have passed"):
			if fileExists(filepath.Join(workDir, "chapters", "chapter_01.tex")) {
				formalWasPremature.Store(true)
			}
			subdivisionReply(w,
				subdivisionWrite("work:chapter_01.tex", "\\section{Integrated}\n"),
				subdivisionWrite("work:chapter_01/part.tex", "integrated part\n"),
				subdivisionCall("parent-submit", "submit", map[string]any{"path": "work:chapter_01.tex", "dir": "work:chapter_01/", "report": map[string]any{"status": "pass"}}),
			)
		case strings.Contains(lastUser, "input:chapter.md"):
			base := "chapter_01_child_01"
			if strings.Contains(submitDescription, "chapter_01_child_02.tex") {
				base = "chapter_01_child_02"
			}
			subdivisionReply(w,
				subdivisionWrite("work:"+base+".tex", "\\section{Child}\n"),
				subdivisionWrite("work:"+base+"/part.tex", "child part\n"),
				subdivisionCall("child-submit-"+base, "submit", map[string]any{"path": "work:" + base + ".tex", "dir": "work:" + base + "/", "report": map[string]any{"status": "pass"}}),
			)
		default:
			subdivisionReply(w,
				subdivisionWrite("work:chapter_01.tex", "\\section{Parent draft}\n"),
				subdivisionWrite("work:chapter_01/part.tex", "draft part\n"),
				subdivisionCall("split", "request_chapter_split", map[string]any{"children": []any{
					map[string]any{"title": "First", "start_line": 1, "end_line": 2},
					map[string]any{"title": "Second", "start_line": 3, "end_line": 4},
				}}),
			)
		}
	}))
	defer server.Close()

	r := keepTestRunner(t)
	r.cfg.Latex.Concurrency = 2
	r.cfg.Latex.ConvertModel = "fake"
	r.cfg.Latex.CheckerModel = "fake"
	off := false
	r.cfg.Models = map[string]config.ModelConfig{"fake": {BaseURL: server.URL, APIKey: "test", Model: "fake", Stream: &off}}
	r.clients = map[string]*session.Client{}
	r.models = map[string]config.ModelConfig{}
	r.projDir = proj
	if err := r.convertOneChapter(proj, "book", filepath.Join(style, "manual.md"), chapterPath, workDir, 1, 1, false); err != nil {
		t.Fatalf("convertOneChapter: %v", err)
	}

	if formalWasPremature.Load() {
		t.Fatal("parent output appeared in the formal chapter tree before its checker passed")
	}
	formalTex := filepath.Join(workDir, "chapters", "chapter_01.tex")
	if !fileExists(formalTex) {
		t.Fatal("checker-approved parent was not promoted")
	}
	for _, child := range []string{"chapter_01_child_01", "chapter_01_child_02"} {
		if !fileExists(filepath.Join(workDir, "chapters", "chapter_01", "children", child+".tex")) {
			t.Errorf("child artifact %s missing from parent resources", child)
		}
	}
	nodeRoot := filepath.Join(proj, "work", "temp", "conv_chapter_01")
	for _, path := range []string{
		filepath.Join(proj, "work", "sessions", "convert_chapter_01.jsonl"),
		filepath.Join(nodeRoot, "sessions", "checker.jsonl"),
		filepath.Join(workDir, "reports", "chapter_01.md"),
		filepath.Join(nodeRoot, "work", "_subdivision", "children-manifest.md"),
		filepath.Join(nodeRoot, "work", "_subdivision", "child_01", "node", "sessions", "convert.jsonl"),
		filepath.Join(nodeRoot, "work", "_subdivision", "child_01", "node", "sessions", "checker.jsonl"),
	} {
		if !fileExists(path) {
			t.Errorf("recursive record missing: %s", path)
		}
	}
	if requests.Load() < 5 {
		t.Fatalf("API requests = %d, expected parent, children, and checkers", requests.Load())
	}
}
