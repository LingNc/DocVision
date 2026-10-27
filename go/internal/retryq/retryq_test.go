package retryq

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQueuePendingConsume(t *testing.T) {
	proj := t.TempDir()
	path, err := Queue(proj, Request{Chapter: "chapter_002", Role: "convert", Reason: "测试"})
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	if filepath.Dir(path) != Dir(proj) {
		t.Fatalf("队列目录 = %s, want %s", filepath.Dir(path), Dir(proj))
	}
	pending := Pending(proj)
	if len(pending) != 1 || pending[0].Chapter != "chapter_002" || pending[0].Reason != "测试" {
		t.Fatalf("Pending = %+v", pending)
	}
	if got := Chapters(proj); len(got) != 1 || got[0] != "chapter_002" {
		t.Fatalf("Chapters = %v", got)
	}
	if err := Consume(path); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got := Pending(proj); len(got) != 0 {
		t.Fatalf("消费后仍有待办: %+v", got)
	}
	// 消费 = 改名留档，不是删除：事后能看出"这一章被人要求重跑过"。
	if _, err := os.Stat(filepath.Join(Dir(proj), filepath.Base(path[:len(path)-len(".json")])+".done.json")); err != nil {
		t.Fatalf("留档文件不存在: %v", err)
	}
}

func TestChaptersDedupesAndSkipsBadFiles(t *testing.T) {
	proj := t.TempDir()
	for i := 0; i < 3; i++ {
		if _, err := Queue(proj, Request{Chapter: "chapter_001"}); err != nil {
			t.Fatalf("Queue: %v", err)
		}
	}
	if _, err := Queue(proj, Request{Chapter: "chapter_003"}); err != nil {
		t.Fatalf("Queue: %v", err)
	}
	if err := os.WriteFile(filepath.Join(Dir(proj), "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Chapters(proj)
	if len(got) != 2 {
		t.Fatalf("Chapters = %v, want 2 个去重后的章节", got)
	}
}

func TestQueueRejectsEmptyChapter(t *testing.T) {
	if _, err := Queue(t.TempDir(), Request{Chapter: "  "}); err == nil {
		t.Fatal("空章节名应当被拒绝")
	}
}

func TestSanitizeKeepsItInsideTheDir(t *testing.T) {
	proj := t.TempDir()
	path, err := Queue(proj, Request{Chapter: "../../etc/passwd"})
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	if filepath.Dir(path) != Dir(proj) {
		t.Fatalf("章节名被当成路径: %s", path)
	}
}
