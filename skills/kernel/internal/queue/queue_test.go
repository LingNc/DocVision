package queue

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func newTask(id string, ty task.Type) *task.Task {
	return &task.Task{ID: id, Type: ty, Project: "test", Sandbox: "tasks/" + id, Status: task.StatusPending}
}

func TestEnqueueIdempotent(t *testing.T) {
	q, err := Load(filepath.Join(t.TempDir(), "q.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = q.Enqueue(newTask("a", task.TypeClassify))
	_ = q.Enqueue(newTask("a", task.TypeClassify))
	if len(q.Tasks) != 1 {
		t.Fatalf("同 id 应幂等，实际 %d 条", len(q.Tasks))
	}
}

func TestClaimLeaseAndReclaim(t *testing.T) {
	q, err := Load(filepath.Join(t.TempDir(), "q.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = q.Enqueue(newTask("a", task.TypeClassify))
	_ = q.Enqueue(newTask("b", task.TypeConvert))

	got := q.Claim("agent-1", 5, 1, task.TypeClassify)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("应只领到 classify 任务，实际 %+v", got)
	}
	if got[0].Status != task.StatusLeased {
		t.Fatal("领后应为 leased")
	}
	// 租约未过期：不可再领
	if again := q.Claim("agent-2", 5, 1); len(again) != 1 || again[0].ID != "b" {
		t.Fatalf("租约未过期时不应重复派发，实际 %+v", again)
	}
	// 租约过期：自动回队列
	q.Tasks["a"].Lease.ExpiresAt = time.Now().Add(-time.Second)
	if again := q.Claim("agent-2", 5, 1); len(again) != 1 || again[0].ID != "a" {
		t.Fatalf("租约过期后应可重领，实际 %+v", again)
	}
}

func TestDeterministicDispatchOrder(t *testing.T) {
	q, _ := Load(filepath.Join(t.TempDir(), "q.json"))
	first := newTask("first", task.TypeConvert)
	first.Created = time.Now().Add(-time.Hour)
	second := newTask("second", task.TypeConvert)
	_ = q.Enqueue(second)
	_ = q.Enqueue(first)
	got := q.Claim("x", 1, 60)
	if len(got) != 1 || got[0].ID != "first" {
		t.Fatalf("派发应按创建时间升序，实际 %v", got)
	}
}

func TestVerdictFailReturnsToPending(t *testing.T) {
	q, _ := Load(filepath.Join(t.TempDir(), "q.json"))
	_ = q.Enqueue(newTask("a", task.TypeConvert))
	_ = q.Claim("x", 1, 60)
	nt, err := q.RecordVerdict("a", task.Verdict{At: time.Now(), Pass: false, Errors: []string{"缺产物"}}, 6)
	if err != nil {
		t.Fatal(err)
	}
	if nt.Status != task.StatusPending || nt.Attempts != 1 {
		t.Fatalf("打回应回 pending 且 attempts+1，实际 %s/%d", nt.Status, nt.Attempts)
	}
	nt, _ = q.RecordVerdict("a", task.Verdict{At: time.Now(), Pass: true}, 6)
	if nt.Status != task.StatusDone || len(nt.Verdicts) != 2 {
		t.Fatalf("通过应为 done 且留两条 verdict，实际 %s/%d", nt.Status, len(nt.Verdicts))
	}
}

func TestUnblock(t *testing.T) {
	q, _ := Load(filepath.Join(t.TempDir(), "q.json"))
	parent := newTask("parent", task.TypeConvert)
	parent.Status = task.StatusBlocked
	parent.BlockedBy = []string{"child1", "child2"}
	_ = q.Enqueue(parent)
	_ = q.Enqueue(newTask("child1", task.TypeConvert))
	_ = q.Enqueue(newTask("child2", task.TypeConvert))

	q.RecordVerdict("child1", task.Verdict{At: time.Now(), Pass: true}, 6)
	if u := q.Unblock("child1"); len(u) != 0 {
		t.Fatal("还有一个依赖，不应解锁")
	}
	q.RecordVerdict("child2", task.Verdict{At: time.Now(), Pass: true}, 6)
	if u := q.Unblock("child2"); len(u) != 1 || u[0].ID != "parent" {
		t.Fatalf("全过应解锁父任务，实际 %+v", u)
	}
}
