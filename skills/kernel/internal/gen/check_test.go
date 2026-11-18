package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func TestGenCheck(t *testing.T) {
	root := t.TempDir()
	chDir := filepath.Join(root, "chapters")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chDir, "chapter_001.md"), []byte("# 一\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chDir, "chapter_002.md"), []byte("# 二\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	if _, err := GenConvert(q, root, "chapters"); err != nil {
		t.Fatal(err)
	}
	rep, err := GenCheck(q)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 2 || rep.BlockedNow != 2 {
		t.Fatalf("应为 2 个 blocked 核对任务: %+v", rep)
	}
	// 核对任务 blocked 于对应 convert，claim 拿不到
	if got := q.Claim("x", 5, 60, task.TypeCheck); len(got) != 0 {
		t.Fatalf("blocked 核对不应可领: %+v", got)
	}
	// convert 全过 → 核对自动解锁（T4 延迟核对的结构表达）
	q.RecordVerdict("conv_chapter_001", task.Verdict{Pass: true}, 6)
	q.RecordVerdict("conv_chapter_002", task.Verdict{Pass: true}, 6)
	if u := q.Unblock("conv_chapter_001"); len(u) != 1 || u[0].ID != "chk_chapter_001" {
		t.Fatalf("conv1 过应解锁 chk1: %+v", u)
	}
	if u := q.Unblock("conv_chapter_002"); len(u) != 1 {
		t.Fatalf("conv2 过应解锁 chk2: %+v", u)
	}
	// 解锁后可领
	if got := q.Claim("x", 5, 60, task.TypeCheck); len(got) != 2 {
		t.Fatalf("解锁后应可领 2 个核对: %+v", got)
	}
	// 幂等
	rep2, err := GenCheck(q)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Added != 0 {
		t.Fatalf("重复生成应幂等: %+v", rep2)
	}
}

func TestGenCheckSkipsFailedConvert(t *testing.T) {
	root := t.TempDir()
	chDir := filepath.Join(root, "chapters")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chDir, "chapter_009.md"), []byte("# 九"), 0o644); err != nil {
		t.Fatal(err)
	}
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	if _, err := GenConvert(q, root, "chapters"); err != nil {
		t.Fatal(err)
	}
	// 把 convert 打到 failed（6 次打回）
	for i := 0; i < 6; i++ {
		q.RecordVerdict("conv_chapter_009", task.Verdict{Pass: false, Errors: []string{"缺产物"}}, 6)
	}
	if st, _ := q.Get("conv_chapter_009"); st.Status != task.StatusFailed {
		t.Fatalf("前置：convert 应已 failed: %s", st.Status)
	}
	rep, err := GenCheck(q)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 0 {
		t.Fatalf("failed convert 不应派核对（没东西可核）: %+v", rep)
	}
}
