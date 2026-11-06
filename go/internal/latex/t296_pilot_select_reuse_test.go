package latex

import (
	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
	"os"
	"path/filepath"
	"testing"
)

// writeSubmitTranscript 造一条"模型调了 submit 且已收到回执"的转录。
func writeSubmitTranscript(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	tr, err := session.NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	var call session.ToolCall
	call.ID = "call_1"
	call.Type = "function"
	call.Function.Name = "submit"
	call.Function.Arguments = "{}"
	if err := tr.Append(session.ChatMessage{Role: "user", Content: "select"}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Append(session.ChatMessage{Role: "assistant", ToolCalls: []session.ToolCall{call}}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Append(session.ChatMessage{Role: "tool", ToolCallID: "call_1", Content: "submitted"}); err != nil {
		t.Fatal(err)
	}
}

// T296：选材已经做过（转录里 submit 收账 + spec.json 就绪）时不再重跑选材会话。
func TestPilotSpecReusedFromPrevRun(t *testing.T) {
	proj := t.TempDir()
	src := filepath.Join(proj, "source")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "book.md"), "l1\nl2\nl3\nl4\nl5\nl6\n")
	pilotDir := pilotDirOf(proj)
	if err := os.MkdirAll(pilotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(pilotDir, "spec.json"),
		`{"items":[{"kind":"md","file":"book.md","start":1,"end":5},{"kind":"pdf","file":"p1.pdf","pages":[2,3]}]}`+"\n")
	trPath := filepath.Join(proj, "work", "sessions", "style_pilot_select.jsonl")
	writeSubmitTranscript(t, trPath)

	lg, _ := logger.NewLogger("", "", 2)
	r := &Runner{log: lg}
	spec, ok := r.pilotSpecFromPrevRun(proj, trPath)
	if !ok || spec == nil {
		t.Fatal("应当沿用上次的选材结果")
	}
	if len(spec.MDRanges) != 1 || spec.MDRanges[0].File != "book.md" ||
		spec.MDRanges[0].Start != 1 || spec.MDRanges[0].End != 5 {
		t.Fatalf("md 段没读对: %+v", spec.MDRanges)
	}
	if len(spec.PDFPages) != 1 || len(spec.PDFPages[0].Pages) != 2 {
		t.Fatalf("pdf 页没读对: %+v", spec.PDFPages)
	}
}

// 没交过题（或 spec.json 不在/为空）时必须老实重选，不能凭空沿用。
func TestPilotSpecNotReusedWithoutSubmit(t *testing.T) {
	proj := t.TempDir()
	pilotDir := pilotDirOf(proj)
	if err := os.MkdirAll(pilotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(pilotDir, "spec.json"), `{"items":[{"kind":"md","file":"book.md","start":1,"end":5}]}`+"\n")
	trPath := filepath.Join(proj, "work", "sessions", "style_pilot_select.jsonl")
	lg, _ := logger.NewLogger("", "", 2)
	r := &Runner{log: lg}
	if _, ok := r.pilotSpecFromPrevRun(proj, trPath); ok {
		t.Fatal("没有转录时不该沿用")
	}
	// 有转录但没有 submit 回执。
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	tr, err := session.NewTranscript(trPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = tr.Append(session.ChatMessage{Role: "assistant", Content: "still working"})
	tr.Close()
	if _, ok := r.pilotSpecFromPrevRun(proj, trPath); ok {
		t.Fatal("没有 submit 回执时不该沿用")
	}
	// pdf 段缺失（半截清单）也不沿用。
	writeSubmitTranscript(t, trPath)
	if _, ok := r.pilotSpecFromPrevRun(proj, trPath); ok {
		t.Fatal("只有 md 段时不该沿用")
	}
}
