package sessionview

import (
	"os"
	"path/filepath"
	"testing"
)

// T253：索引缓存按 (size, mtime) 命中就不再重读文件。这里把文件内容换成同样
// 长度、同样 mtime 的垃圾——没缓存的话消息数会归零，命中缓存则沿用上次结果。
func TestIndexCacheSkipsUnchangedFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "proj", "work", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "convert_chapter_901.jsonl")
	body := `{"t":"msg","role":"user","content":"hi"}
{"t":"msg","role":"assistant","content":"ok"}
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	mt := st.ModTime()

	first, err := Scan(root)
	if err != nil || len(first) != 1 {
		t.Fatalf("首次扫描 = %v, %v", first, err)
	}
	if first[0].Messages != 2 {
		t.Fatalf("消息数 = %d, want 2", first[0].Messages)
	}
	if _, err := os.Stat(filepath.Join(root, countCacheName)); err != nil {
		t.Fatalf("扫描后应写出索引缓存: %v", err)
	}

	// 同样长度、同样 mtime，内容换成无法解析的行。
	junk := make([]byte, len(body))
	for i := range junk {
		junk[i] = 'x'
	}
	if err := os.WriteFile(p, junk, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}

	second, err := Scan(root)
	if err != nil || len(second) != 1 {
		t.Fatalf("二次扫描 = %v, %v", second, err)
	}
	if second[0].Messages != 2 {
		t.Fatalf("命中缓存时消息数应沿用 2，实际 %d（说明又读了一遍文件）", second[0].Messages)
	}
}
