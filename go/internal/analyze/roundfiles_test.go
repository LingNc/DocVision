package analyze

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestGroupSessionsByFile(t *testing.T) {
	sessions := []Session{
		{Key: "a.md::images/x.jpg", Status: StatusSuccess},
		{Key: "a.md::images/y.jpg", Status: StatusFailed},
		{Key: "a.md::images/z.jpg", Status: StatusIncomplete},
		{Key: "b.md::images/w.jpg", Status: StatusSuccess},
		{Key: "no-key", Status: StatusFailed},
	}
	stats := GroupSessionsByFile(sessions)
	if got := stats["a.md"].Total; got != 3 {
		t.Fatalf("a.md total = %d, want 3", got)
	}
	if got := stats["a.md"].Success; got != 1 {
		t.Fatalf("a.md success = %d, want 1", got)
	}
	if got := stats["b.md"].Success; got != 1 {
		t.Fatalf("b.md success = %d, want 1", got)
	}
	if got := stats["no-key"].Total; got != 1 {
		t.Fatalf("no-key total = %d, want 1", got)
	}
}

func TestPrintRoundFileSummary(t *testing.T) {
	sessions := []Session{
		{Key: "a.md::images/x.jpg", Status: StatusSuccess},
		{Key: "a.md::images/y.jpg", Status: StatusSuccess},
		{Key: "a.md::images/z.jpg", Status: StatusFailed},
		{Key: "b.md::images/w.jpg", Status: StatusFailed},
	}
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	PrintRoundFileSummary("img2text_test.log", sessions)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()
	for _, want := range []string{"本轮输出文件统计", "a.md", "b.md", "已写出/更新的最终文件", "成功率"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}
