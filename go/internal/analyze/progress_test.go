package analyze

import (
	"os"
	"path/filepath"
	"testing"
)


// T55：ERROR 才算问题图；WARNING（自纠正成功）计良品、单独成集合。
func TestScanLogIssueImagesSplit(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "img2text_20260919_130000.log")
	content := `[10:00:01][T01] [1/4] a.jpg
[10:00:02][T01] [WARNING] auto-corrected prose
[10:00:03][T02] [2/4] b.jpg
[10:00:04][T02] [ERROR] boom
[10:00:05][T03] [3/4] c.jpg
[10:00:06][T03] clean
[10:00:07][T04] [4/4] d.jpg
[10:00:08][T04] [WARNING] w1
[10:00:09][T04] [ERROR] w2
`
	os.WriteFile(log, []byte(content), 0o644)
	errs, warns := ScanLogIssueImages(log)
	if len(errs) != 2 || errs[0] != "b.jpg" || errs[1] != "d.jpg" {
		t.Errorf("errs = %v, want [b.jpg d.jpg]", errs)
	}
	if len(warns) != 1 || warns[0] != "a.jpg" {
		t.Errorf("warns = %v, want [a.jpg]", warns)
	}
	// 旧接口只剩 ERROR 集
	if got := GetProblematicImages(log); len(got) != 2 {
		t.Errorf("GetProblematicImages = %v", got)
	}
}
