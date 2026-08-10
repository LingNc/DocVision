package split

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// testPDFPages is the page count of the multi-page fixture used by
// every cache test. Eight pages is enough to exercise split-by-page
// (3 parts at maxPages=3) while staying small.
const testPDFPages = 8

var (
	seedOnce sync.Once
	seedPath string
	seedErr  error
)

// onePageSeed builds (once per process) a minimal one-page PDF
// using pdfcpu's internal demo xref helper. The page content is a
// standard "Hello, world" sample page, which is enough for
// api.TrimFile to round-trip cleanly.
func onePageSeed(t *testing.T) string {
	t.Helper()
	seedOnce.Do(func() {
		dir, err := os.MkdirTemp("", "split-seed-*")
		if err != nil {
			seedErr = err
			return
		}
		conf := model.NewDefaultConfiguration()
		mediaBox := types.RectForFormat("A4")
		p := model.Page{MediaBox: mediaBox, Fm: model.FontMap{}, Buf: new(bytes.Buffer)}
		pdfcpu.CreateTestPageContent(p)
		xt, err := pdfcpu.CreateDemoXRef()
		if err != nil {
			seedErr = err
			return
		}
		rootDict, err := xt.Catalog()
		if err != nil {
			seedErr = err
			return
		}
		if err := pdfcpu.AddPageTreeWithSamplePage(xt, rootDict, p); err != nil {
			seedErr = err
			return
		}
		out := filepath.Join(dir, "seed.pdf")
		if err := api.CreatePDFFile(xt, out, conf); err != nil {
			seedErr = err
			return
		}
		seedPath = out
	})
	if seedErr != nil {
		t.Skipf("seed generation failed: %v", seedErr)
	}
	return seedPath
}

// makeTestPDF writes a deterministic multi-page PDF at
// filepath.Join(t.TempDir(), name) and returns its path. The
// fixture is built by merging testPDFPages copies of a one-page
// seed PDF, so tests stay hermetic.
func makeTestPDF(t *testing.T, name string) string {
	t.Helper()
	conf := model.NewDefaultConfiguration()
	seed := onePageSeed(t)
	merged := filepath.Join(t.TempDir(), name)
	in := make([]string, testPDFPages)
	for i := range in {
		in[i] = seed
	}
	if err := api.MergeCreateFile(in, merged, false, conf); err != nil {
		t.Fatalf("merge pdf: %v", err)
	}
	if n, err := api.PageCountFile(merged); err != nil || n != testPDFPages {
		t.Fatalf("fixture page count: got %d err %v, want %d", n, err, testPDFPages)
	}
	return merged
}

// touchPDF bumps the mtime of path to one nanosecond later so
// manifest matching can observe a real source change.
func touchPDF(t *testing.T, path string) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	newTime := st.ModTime().Add(1)
	if err := os.Chtimes(path, newTime, newTime); err != nil {
		t.Fatal(err)
	}
}
