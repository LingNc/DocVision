package latex

import (
	"testing"

	"mineru-tools/internal/logger"
	"mineru-tools/internal/retryq"
)

// T211：子章节重试要落到父章节的「继续跑」，不能再消费掉就完事。
func TestChildChapterParent(t *testing.T) {
	cases := []struct {
		base   string
		parent string
		child  bool
	}{
		{"chapter_004", "", false},
		{"chapter_004_child_02", "chapter_004", true},
		{"chapter_004_child_02_child_01", "chapter_004", true},
	}
	for _, c := range cases {
		p, ok := childChapterParent(c.base)
		if ok != c.child || (ok && p != c.parent) {
			t.Errorf("childChapterParent(%q) = (%q,%v), want (%q,%v)", c.base, p, ok, c.parent, c.child)
		}
	}
}

// 父章节已有「继续跑」时不重复排，没有时排一条。
func TestQueueParentContinueFor(t *testing.T) {
	proj := t.TempDir()
	log, err := logger.NewLogger(t.TempDir()+"/run.log", t.TempDir()+"/err.log", 2)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{log: log}
	r.queueParentContinueFor(proj, "书A", "chapter_024", "chapter_024_child_06")
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Chapter != "chapter_024" || retryRoleOf(pending[0].Request) != "continue" {
		t.Fatalf("队列 = %+v", pending)
	}
	r.queueParentContinueFor(proj, "书A", "chapter_024", "chapter_024_child_06")
	if got := len(retryq.Pending(proj)); got != 1 {
		t.Fatalf("重复排队了：%d 条", got)
	}
}
