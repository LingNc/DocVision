package queue

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// 手造队列（有任务无戳）必须被拒并指向禁令。
func TestLoadRejectsHandcraftedQueue(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "skills_queue.json")
	os.WriteFile(p, []byte(`{"tasks":{"img2txt_x":{"task_id":"img2txt_x","type":"img2text","status":"pending"}}}`), 0o644)
	_, err := Load(p)
	if err == nil || !contains(err.Error(), "手造") {
		t.Fatalf("无戳有任务的队列应被拒且指回禁令: %v", err)
	}
}

// 内核 Save 的队列有戳，Load 放行。
func TestStampedQueueRoundtrip(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "skills_queue.json")
	q, _ := Load(p)
	q.Enqueue(&task.Task{ID: "t1", Type: task.TypeImg2Text, Status: task.StatusPending})
	if err := q.Save(); err != nil {
		t.Fatal(err)
	}
	q2, err := Load(p)
	if err != nil {
		t.Fatalf("内核落盘的队列应通过验戳: %v", err)
	}
	if q2.KernelStamp != QueueStamp || len(q2.Tasks) != 1 {
		t.Fatalf("戳或任务丢失: %s %d", q2.KernelStamp, len(q2.Tasks))
	}
}

// 空旧队列放行（下次 Save 盖戳）；Stamp 迁移幂等。
func TestLegacyEmptyQueueAndStamp(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "skills_queue.json")
	os.WriteFile(p, []byte(`{"tasks":{}}`), 0o644)
	if _, err := Load(p); err != nil {
		t.Fatalf("空旧队列应放行: %v", err)
	}
	lq, _ := LoadRaw(p)
	if err := lq.Stamp(); err != nil {
		t.Fatal(err)
	}
	if err := lq.Stamp(); err != nil { // 幂等
		t.Fatal(err)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("盖戳后应通过: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
