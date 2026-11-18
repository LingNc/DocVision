package split

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// TestSplitPDFBackendsBothWork 两种后端各跑一遍，产物名/页数/总页数都要一致。
// 后端靠 DOCVISION_PDF_SPLIT_TOOL 强制指定，缺哪个可执行文件就跳过哪一路。
func TestSplitPDFBackendsBothWork(t *testing.T) {
	for _, tool := range []string{"mutool", "pdfcpu"} {
		t.Run(tool, func(t *testing.T) {
			if tool == "mutool" {
				if _, err := execLookPath("mutool"); err != nil {
					t.Skip("本机没有 mutool")
				}
			}
			t.Setenv("DOCVISION_PDF_SPLIT_TOOL", tool)
			src := makeTestPDF(t, "doc.pdf")
			outDir := filepath.Join(t.TempDir(), "out")
			if err := SplitPDF(src, 2, 0, outDir, true); err != nil {
				t.Fatalf("split via %s: %v", tool, err)
			}
			parts := listParts(t, outDir, "doc")
			if len(parts) != 4 {
				t.Fatalf("%s: 期望 4 份（%d 页，每份 2 页），实际 %d", tool, testPDFPages, len(parts))
			}
			total := 0
			for _, p := range parts {
				total += pageCountOf(t, p)
			}
			if total != testPDFPages {
				t.Fatalf("%s: 分片页数合计 %d，源文件 %d 页", tool, total, testPDFPages)
			}
		})
	}
}

// TestSplitPDFRefinesOversizePartsOnTheirOwn 体积上限压到"整本一片装不下"时，
// 分片要就地再切（在分片自己身上切，不再回头碰源文件），且页段必须连续覆盖
// 整本、顺序与文件名一致。
func TestSplitPDFRefinesOversizePartsOnTheirOwn(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")

	// 先不限体积跑一遍，拿到"整本一片"的大小。
	base := filepath.Join(t.TempDir(), "base")
	if err := SplitPDF(src, 0, 0, base, true); err != nil {
		t.Fatalf("split: %v", err)
	}
	whole := listParts(t, base, "doc")
	if len(whole) != 1 {
		t.Fatalf("不限体积时应只有 1 份，实际 %d", len(whole))
	}
	st, err := os.Stat(whole[0])
	if err != nil {
		t.Fatal(err)
	}
	limitMB := float64(st.Size()) / (1 << 20) / 2 // 只装得下一半

	outDir := filepath.Join(t.TempDir(), "out")
	if err := SplitPDF(src, 0, limitMB, outDir, true); err != nil {
		t.Fatalf("split with size limit: %v", err)
	}
	parts := listParts(t, outDir, "doc")
	if len(parts) < 2 {
		t.Fatalf("上限压到一半后应至少 2 份，实际 %d", len(parts))
	}
	m, err := LoadManifest(ManifestPath(outDir, "doc"))
	if err != nil || m == nil {
		t.Fatalf("manifest 读不到: %v", err)
	}
	want := 1
	for _, p := range m.Parts {
		if p.PageStart != want {
			t.Fatalf("分片 %s 起始页 %d，期望 %d（页序断了）", p.Filename, p.PageStart, want)
		}
		want = p.PageEnd + 1
	}
	if want != testPDFPages+1 {
		t.Fatalf("页段只覆盖到 %d，源文件 %d 页", want-1, testPDFPages)
	}
}

func execLookPath(name string) (string, error) { return exec.LookPath(name) }

func pageCountOf(t *testing.T, path string) int {
	t.Helper()
	n, err := api.PageCountFile(path)
	if err != nil {
		t.Fatalf("读 %s 页数失败: %v", filepath.Base(path), err)
	}
	return n
}
