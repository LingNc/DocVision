package img2text

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// TestProgressLineCountsThisRunOnly pins the format contract: every
// number on the compact line is about THIS RUN (the resumed baseline is
// reported on the "Already done" line instead).
func TestProgressLineCountsThisRunOnly(t *testing.T) {
	if got := progressLine(0, 396, 0, 0, 0, 10); got != "[0/396] 0.00% (done: 0, errors: 0, warns: 0, running: 10)" {
		t.Errorf("initial line = %q", got)
	}
	if got := progressLine(208, 396, 206, 2, 0, 10); got != "[208/396] 52.53% (done: 206, errors: 2, warns: 0, running: 10)" {
		t.Errorf("mid line = %q", got)
	}
	if got := progressLine(396, 396, 396, 0, 0, 0); got != "[396/396] 100.00% (done: 396, errors: 0, warns: 0, running: 0)" {
		t.Errorf("final line = %q", got)
	}
	if got := progressLine(0, 0, 0, 0, 0, 0); !strings.Contains(got, "[0/0] 0.00%") {
		t.Errorf("empty run must not divide by zero: %q", got)
	}
}

// TestResumedRunProgressIgnoresAlreadyDone is the regression test for
// the confusing resume output the user hit:
//
//	Already done: 8666 | To process: 396
//	[8874/9062] 97.93% (done: 8874, errors: 0, warns: 0, running: 10)
//
// One image is already recorded on disk, one is pending: the run must
// report [1/1] — not [2/2], and certainly not a baseline-inflated number.
func TestResumedRunProgressIgnoresAlreadyDone(t *testing.T) {
	root := t.TempDir()
	imagesDir := filepath.Join(root, "images")
	outputDir := filepath.Join(root, "output")
	finallyDir := filepath.Join(root, "finally")
	progressRoot := filepath.Join(finallyDir, "progress_items")
	for _, d := range []string{imagesDir, outputDir, progressRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a.png", "b.png"} {
		writeTestPNG(t, filepath.Join(imagesDir, name))
	}
	md := "前 ![a](images/a.png) 中 ![b](images/b.png) 后\n"
	if err := os.WriteFile(filepath.Join(outputDir, "doc.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	// a.png 已经处理过（断点续传的历史记录）
	done := ProgressItem{Key: "doc.md::images/a.png", Result: "[IMG_TYPE: chart]\nold answer", ImgPath: "images/a.png"}
	off := strings.Index(md, "![a]")
	done.SetOffsets([]OffsetPair{{Start: off, End: off + len("![a](images/a.png)")}})
	if err := SaveProgressItem(progressRoot, "doc.md", "images/a.png", done); err != nil {
		t.Fatal(err)
	}

	ms := newMockChatServer(t, func(_ int, _ recordedRequest) (int, string) {
		return 200, `{"choices":[{"message":{"role":"assistant","content":"[IMG_TYPE: chart]\nanswer"}}]}`
	})
	cfg := &config.Config{}
	cfg.Paths.ImagesDir = imagesDir
	cfg.Paths.OutputDir = outputDir
	cfg.Paths.FinallyDir = finallyDir
	cfg.Options.Concurrency = 1
	cfg.Models = map[string]config.ModelConfig{
		"text": {BaseURL: ms.server.URL, APIKey: "k", Model: "m"},
	}

	out := captureStdout(t, func() {
		if err := Run(cfg, newTestLogger(t), RunOptions{Quiet: true}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "Already done: 1 | To process: 1") {
		t.Errorf("header must state the baseline and this run's work, got:\n%s", out)
	}
	if !strings.Contains(out, "[1/1] 100.00%") {
		t.Errorf("progress must count THIS run only ([1/1]), got:\n%s", out)
	}
	for _, bad := range []string{"[2/2]", "97.", "[9096/"} {
		if strings.Contains(out, bad) {
			t.Errorf("progress must not fold in the already-done baseline (found %q):\n%s", bad, out)
		}
	}
}

// writeTestPNG writes a tiny PNG.
func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// captureStdout runs fn with os.Stdout redirected into a pipe.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}
