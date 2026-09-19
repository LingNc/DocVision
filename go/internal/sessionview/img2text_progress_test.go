package sessionview

import (
	"os"
	"path/filepath"
	"testing"
)

// P18：进度板块从 progress_items 树推导 每书 总数/完成/升级/待处理。
func TestScanImg2TextProgress(t *testing.T) {
	root := t.TempDir()
	md := "# 书\n\n![图一](images/书/a1.jpg)\n\n![图二](images/书/b2.jpg)\n\n![图三](images/书/c3.jpg)\n\n![图四](images/书/d4.jpg)\n"
	// 真实布局：<md>.md/original.md（书副本）+ <md>/（逐图 json）
	os.MkdirAll(filepath.Join(root, "书.md"), 0o755)
	os.WriteFile(filepath.Join(root, "书.md", "original.md"), []byte(md), 0o644)
	itemDir := filepath.Join(root, "书")
	os.MkdirAll(itemDir, 0o755)
	// a1 有结果 → done
	os.WriteFile(filepath.Join(itemDir, "images_书_a1.jpg.json"),
		[]byte("{\"key\":\"书.md::images/书/a1.jpg\",\"result\":\"[IMG_TYPE: Flowchart] graph TD\"}"), 0o644)
	// 无 IMG_TYPE 的 json 不算 done
	os.WriteFile(filepath.Join(itemDir, "images_书_x9.jpg.json"), []byte(`{"key":"","result":""}`), 0o644)
	// b2 有修复工作区 → escalated；d4 有结果+工作区 → fixed
	os.MkdirAll(filepath.Join(root, "mermaid_fix", "书_b2.jpg"), 0o755)
	os.MkdirAll(filepath.Join(root, "mermaid_fix", "书_d4.jpg"), 0o755)
	os.WriteFile(filepath.Join(itemDir, "images_书_d4.jpg.json"),
		[]byte("{\"key\":\"书.md::images/书/d4.jpg\",\"result\":\"[IMG_TYPE: Flowchart] graph\"}"), 0o644)
	// c3 什么都没有 → pending

	books := ScanImg2TextProgress(root)
	if len(books) != 1 {
		t.Fatalf("books=%d want 1", len(books))
	}
	b := books[0]
	if b.Total != 4 || b.Done != 1 || b.Fixed != 1 || b.Escalated != 1 || b.Pending != 1 {
		t.Fatalf("counts total=%d done=%d fixed=%d esc=%d pending=%d, want 4/1/1/1/1", b.Total, b.Done, b.Fixed, b.Escalated, b.Pending)
	}
	if b.Items[0].Name != "a1.jpg" || b.Items[0].Status != "done" {
		t.Errorf("item0 = %+v", b.Items[0])
	}
	if b.Items[1].Status != "escalated" || b.Items[2].Status != "pending" || b.Items[3].Status != "fixed" {
		t.Errorf("items: %+v", b.Items)
	}
}

// 空目录/不存在 → nil，不报错。
func TestScanImg2TextProgressEmpty(t *testing.T) {
	if got := ScanImg2TextProgress(filepath.Join(t.TempDir(), "nope")); got != nil {
		t.Errorf("want nil, got %v", got)
	}
	if got := ScanImg2TextProgress(t.TempDir()); got != nil {
		t.Errorf("want nil, got %v", got)
	}
}
