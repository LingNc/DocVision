package sessionview

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// T255：同一个 scanner 会被轮询并发复用（预览页每 2 秒重建索引，两个请求撞在
// 一起时就是两个 goroutine 同时走 scan）。以前 walk 回调直接往 s.keep 里写，
// 真机上崩过 `fatal error: concurrent map writes`——这里用 -race 钉住。
func TestConcurrentScanOnSharedScanner(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "proj", "work", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		p := filepath.Join(dir, fmt.Sprintf("convert_chapter_%03d.jsonl", i))
		body := `{"t":"msg","role":"user","content":"hi"}` + "\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sc := &scanner{root: root}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := sc.scan(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	sc.saveCountCache(root)
}
