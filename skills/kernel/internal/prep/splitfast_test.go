package prep

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitFastBasic(t *testing.T) {
	if _, err := os.Stat("/usr/bin/pdfseparate"); err != nil {
		if _, err2 := findInPath("pdfseparate"); err2 != nil {
			t.Skip("无 poppler")
		}
	}
	dir := t.TempDir()
	pdf := filepath.Join(dir, "book.pdf")
	makeTestPDF(t, pdf, 5)
	os.Setenv("DOCVISION_PDF_SPLIT_TOOL", "pdfcpu")
	defer os.Unsetenv("DOCVISION_PDF_SPLIT_TOOL")

	names, err := SplitFast(pdf, dir, SplitFastConfig{PagesPerPart: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0] != "book_part1.pdf" || names[2] != "book_part3.pdf" {
		t.Fatalf("5 页按 2 页/份应得 3 片: %v", names)
	}
	// 总页数守恒：2+2+1
	total := 0
	for _, n := range names {
		p, err := pageCount(filepath.Join(dir, n), DefaultRunner)
		if err != nil {
			t.Fatal(err)
		}
		total += p
	}
	if total != 5 {
		t.Fatalf("页数守恒 5: %d", total)
	}
}

func TestSplitFastRefineOversize(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "book.pdf")
	makeTestPDF(t, pdf, 4)
	os.Setenv("DOCVISION_PDF_SPLIT_TOOL", "pdfcpu")
	defer os.Unsetenv("DOCVISION_PDF_SPLIT_TOOL")

	// 上限给 1 字节：必超限 → 4 页 part 二分到底，细化出 4 个单页分片
	names, err := SplitFast(pdf, dir, SplitFastConfig{PagesPerPart: 4, MaxSizeBytes: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 4 {
		t.Fatalf("超限二分应细化出 4 个单页片: %v", names)
	}
	total := 0
	for _, n := range names {
		p, _ := pageCount(filepath.Join(dir, n), DefaultRunner)
		total += p
	}
	if total != 4 {
		t.Fatalf("细化后页数守恒 4: %d（%v）", total, names)
	}
}
