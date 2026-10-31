package sessionview

import (
	"os"
	"path/filepath"
	"testing"
)

// T272：`sessions` 命令与 `sessions --serve` 走的都是 ScanWith，而这条路以前
// 完全不碰磁盘索引缓存（只有 Scan 会读写）——一本几千条转录的书每次启动都要把
// 每个文件从头读一遍，缓存文件存在也等于没有。这里用"同长度同 mtime 的垃圾
// 内容"钉住：第二遍必须信缓存，不能重读。
func TestScanWithUsesIndexCache(t *testing.T) {
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

	first, err := ScanWith(root, nil)
	if err != nil || len(first) != 1 || first[0].Messages != 2 {
		t.Fatalf("首次扫描 = %+v, %v", first, err)
	}
	if _, err := os.Stat(filepath.Join(root, countCacheName)); err != nil {
		t.Fatalf("ScanWith 之后应写出索引缓存: %v", err)
	}

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
	second, err := ScanWith(root, nil)
	if err != nil || len(second) != 1 {
		t.Fatalf("第二遍扫描 = %+v, %v", second, err)
	}
	if second[0].Messages != 2 {
		t.Fatalf("第二遍消息数 = %d，缓存没生效（重读了文件）", second[0].Messages)
	}
}

// T272：预览服务的长驻 scanner 也必须从同一份缓存起步，否则第一个
// /api/index 仍要把所有转录读一遍——启动慢原样复现。
func TestServeScannerLoadsIndexCache(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "proj", "work", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "convert_chapter_902.jsonl")
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
	if _, err := ScanWith(root, nil); err != nil { // 先把缓存种好
		t.Fatal(err)
	}
	cachePath := filepath.Join(root, countCacheName)
	cs0, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}

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

	v := newViewerServer(root, nil)
	sessions, err := v.scanAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Messages != 2 {
		t.Fatalf("预览服务首轮 = %+v，没走缓存", sessions)
	}
	// 全部命中缓存时不该重写索引（预览页每 2 秒轮询一次，白写很贵）。
	cs1, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !cs1.ModTime().Equal(cs0.ModTime()) {
		t.Fatalf("没有重算却重写了索引缓存（mtime %v → %v）", cs0.ModTime(), cs1.ModTime())
	}
}
