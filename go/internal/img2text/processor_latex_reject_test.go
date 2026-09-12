// Tests for the "model ignores the prompt and returns a LaTeX/TikZ
// drawing block" path: such a response is invalid (skipped, retried),
// and the log must quote the model's own text plus the expected format.
package img2text

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// writeTestImage writes a real (tiny) PNG plus the markdown lines the
// processor needs. ImageToBase64 re-encodes the decoded image, so the
// bytes must be a decodable image, not a placeholder.
func writeTestImage(t *testing.T, imagesDir, rel string) (lines []string, lineIdx int) {
	t.Helper()
	// resolveImageFile drops the leading "images/" segment and joins the
	// rest with imagesDir, so mirror that layout on disk.
	sub := strings.TrimPrefix(rel, "images/")
	full := filepath.Join(imagesDir, sub)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	lines = []string{"# 标题", "前文", "![图](" + rel + ")", "后文"}
	return lines, 2
}

// tinyPNG encodes a 2x2 image so ImageToBase64 has something real to
// resize and re-encode.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestProcessOneImage_RejectsLatexDrawingBlockWithRawOutput(t *testing.T) {
	reply := "[IMG_TYPE: vector]\n```tikz\n\\begin{tikzpicture}\n\\drawarr ow (0,0) -- (1,1);\n\\end{tikzpicture}\n```"
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		return http.StatusOK, responseText(reply)
	})

	imagesDir := t.TempDir()
	lines, lineIdx := writeTestImage(t, imagesDir, "images/数据结构/a.jpg")

	logPath := filepath.Join(t.TempDir(), "img2text.log")
	errPath := filepath.Join(t.TempDir(), "img2text_error.log")
	l, err := logger.NewLogger(logPath, errPath, 2)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	l.SetQuiet(true)
	defer l.Close()

	// Validation is "off" on purpose: the rejection must not depend on
	// any validator running.
	opts := config.OptionsConfig{
		MaxRetries:        3,
		MermaidValidation: "off",
		MaxWindowUp:       50,
		MaxWindowDown:     50,
	}

	result, status, raw := ProcessOneImage(
		newTestClient(t, ms.server.URL), imagesDir, "images/数据结构/a.jpg", "数据结构",
		lines, lineIdx, l, 1, opts,
	)
	if status != StatusRetry {
		t.Fatalf("status = %q, want %q (result=%q)", status, StatusRetry, result)
	}
	if result != sentinelInvalid {
		t.Fatalf("result = %q, want %q", result, sentinelInvalid)
	}
	if !strings.Contains(raw, "tikz") {
		t.Fatalf("raw output not handed back to the caller: %q", raw)
	}

	// Exactly one request: no TeX compile retry, no repair loop.
	if got := len(ms.calls()); got != 1 {
		t.Fatalf("requests = %d, want 1 (no TeX compile/repair round)", got)
	}

	l.Close()
	loggedBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logged := string(loggedBytes)

	if !strings.Contains(logged, "LaTeX/TikZ drawing block") {
		t.Errorf("log does not name the actual problem:\n%s", logged)
	}
	if !strings.Contains(logged, "数据结构/a.jpg") {
		t.Errorf("log does not name the image:\n%s", logged)
	}
	if !strings.Contains(logged, "\\drawarr ow (0,0) -- (1,1);") {
		t.Errorf("log does not quote the model's own output:\n%s", logged)
	}
	if !strings.Contains(logged, "期望格式：[IMG_TYPE:") {
		t.Errorf("log does not state the expected format:\n%s", logged)
	}
	if strings.Contains(logged, "This is XeTeX") {
		t.Errorf("log mentions the TeX banner: nothing here compiles TeX any more:\n%s", logged)
	}
}

// A Mermaid answer still goes through unchanged (the revert must not
// break the accepted path).
func TestProcessOneImage_AcceptsMermaid(t *testing.T) {
	reply := "[IMG_TYPE: flowchart]\n```mermaid\ngraph TD; A-->B\n```"
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		return http.StatusOK, responseText(reply)
	})

	imagesDir := t.TempDir()
	lines, lineIdx := writeTestImage(t, imagesDir, "images/书/b.jpg")

	l := newTestLogger(t)
	opts := config.OptionsConfig{
		MaxRetries:        3,
		MermaidValidation: "off",
		MaxWindowUp:       50,
		MaxWindowDown:     50,
	}
	result, status, _ := ProcessOneImage(
		newTestClient(t, ms.server.URL), imagesDir, "images/书/b.jpg", "书",
		lines, lineIdx, l, 1, opts,
	)
	if status != StatusOK {
		t.Fatalf("status = %q, want ok (result=%q)", status, result)
	}
	if !strings.HasPrefix(result, "[IMG_TYPE: flowchart]") {
		t.Fatalf("unexpected result: %q", result)
	}
}

// The rendered system prompt (what the model actually receives) must not
// ask for LaTeX/TikZ drawing.
func TestBuildSystemPrompt_HasNoLatexDrawingRequest(t *testing.T) {
	got := strings.ToLower(BuildSystemPrompt(5, "Chinese"))
	for _, banned := range []string{"tikz", "pgfplots", "latex code block", "latex vector graphics"} {
		if strings.Contains(got, banned) {
			t.Errorf("system prompt still asks for %q", banned)
		}
	}
	if !strings.Contains(got, "```mermaid") {
		t.Errorf("system prompt lost the Mermaid instruction")
	}
	if !strings.Contains(got, "max 5 calls") {
		t.Errorf("MAX_TOOL_CALLS placeholder not rendered: %q", got)
	}
}
