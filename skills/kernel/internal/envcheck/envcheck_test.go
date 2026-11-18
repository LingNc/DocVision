package envcheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckStructure(t *testing.T) {
	rep := Check("", nil)
	if len(rep.Tools) < 7 {
		t.Fatalf("工具项不足: %d", len(rep.Tools))
	}
	names := map[string]bool{}
	for _, tc := range rep.Tools {
		names[tc.Name] = true
	}
	for _, want := range []string{"pdflatex", "mineru", "pdfinfo", "pdftotext", "python3", "mutool", "fc-list"} {
		if want == "fc-list" {
			continue // fc-list 在字体段不在工具段
		}
		if !names[want] {
			t.Fatalf("缺工具项 %s", want)
		}
	}
	// 本机有 poppler：pdfinfo 应 Found
	for _, tc := range rep.Tools {
		if tc.Name == "pdfinfo" && !tc.Found {
			t.Fatal("本机应有 pdfinfo")
		}
	}
}

func TestCheckProjectFonts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fonts", "FZKai.TTF"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := Check(root, nil)
	if len(rep.Fonts.ProjectFonts) != 1 || rep.Fonts.ProjectFonts[0] != "FZKai.TTF" {
		t.Fatalf("项目字体应被枚举: %+v", rep.Fonts.ProjectFonts)
	}
	found := false
	for _, h := range rep.Hints {
		if contains(h, "fonts/ 已备") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应有已备字体提示: %v", rep.Hints)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
