package latex

import (
	"sync"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// T14：convert 阶段多 goroutine 同时首次解析模型时，clientFor 曾无锁并发写
// clients/models，直接 "fatal error: concurrent map writes" 崩掉整个进程。
// 这里用 -race 钉死：并发 clientFor / modelOf / hasModel 不得有数据竞争。
func TestClientForConcurrentAccess(t *testing.T) {
	cfg := &config.Config{}
	r := NewRunner(cfg, newTestLogger(t))
	names := []string{"classifier", "style", "chapter", "convert_01", "convert_02", "convert_03", "checker", "drawing"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, n := range names {
				_ = r.clientFor(n)
				_ = r.modelOf(n)
				_ = r.hasModel(n)
			}
		}()
	}
	wg.Wait()
	if len(r.clients) != len(names) {
		t.Fatalf("应解析 %d 个模型，实际 %d", len(names), len(r.clients))
	}
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	l, err := logger.NewLogger(t.TempDir()+"/run.log", t.TempDir()+"/err.log", 2)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
