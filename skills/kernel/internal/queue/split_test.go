package queue

import (
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func TestValidateSplit(t *testing.T) {
	good := []Segment{{ID: "a", Start: 1, End: 100}, {ID: "b", Start: 101, End: 250}}
	if errs := ValidateSplit(good, 250); len(errs) != 0 {
		t.Fatalf("合法拆分应通过: %v", errs)
	}
	cases := map[string][]Segment{
		"单段":     {{ID: "a", Start: 1, End: 250}},
		"重叠":     {{ID: "a", Start: 1, End: 100}, {ID: "b", Start: 50, End: 250}},
		"不连续":   {{ID: "a", Start: 1, End: 100}, {ID: "b", Start: 102, End: 250}},
		"不完整头": {{ID: "a", Start: 5, End: 100}, {ID: "b", Start: 101, End: 250}},
		"不完整尾": {{ID: "a", Start: 1, End: 100}, {ID: "b", Start: 101, End: 240}},
		"越界":     {{ID: "a", Start: 1, End: 100}, {ID: "b", Start: 101, End: 999}},
	}
	for name, segs := range cases {
		if errs := ValidateSplit(segs, 250); len(errs) == 0 {
			t.Fatalf("%s 应被拒", name)
		}
	}
}

func TestFanOutSplitBlocksParent(t *testing.T) {
	q, _ := Load(filepath.Join(t.TempDir(), "q.json"))
	parent := newTask("conv_ch001_t1", task.TypeConvert)
	_ = q.Enqueue(parent)
	segs := []Segment{{ID: "seg1", Start: 1, End: 100}, {ID: "seg2", Start: 101, End: 250}}
	children, err := q.FanOutSplit("conv_ch001_t1", segs)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 || children[0].ID != "conv_ch001_t1/seg1" {
		t.Fatalf("子任务 id 错: %+v", children)
	}
	got, _ := q.Get("conv_ch001_t1")
	if got.Status != task.StatusBlocked || len(got.BlockedBy) != 2 {
		t.Fatalf("父任务应 blocked 于两个子任务: %s/%v", got.Status, got.BlockedBy)
	}
	// 子任务全过后父任务解锁
	q.RecordVerdict("conv_ch001_t1/seg1", task.Verdict{Pass: true}, 6)
	q.RecordVerdict("conv_ch001_t1/seg2", task.Verdict{Pass: true}, 6)
	if u := q.Unblock("conv_ch001_t1/seg1"); len(u) != 0 {
		t.Fatal("只过一个不应解锁")
	}
	if u := q.Unblock("conv_ch001_t1/seg2"); len(u) != 1 || u[0].ID != "conv_ch001_t1" {
		t.Fatalf("全过应解锁父任务: %+v", u)
	}
}

func TestSpawnRework(t *testing.T) {
	q, _ := Load(filepath.Join(t.TempDir(), "q.json"))
	_ = q.Enqueue(newTask("conv_ch001_t1", task.TypeConvert))
	iss := &Issue{ID: "i000001", TaskID: "conv_ch001_t1", Desc: "第3节公式漏排", Status: "open"}
	rw, err := q.SpawnRework(iss)
	if err != nil {
		t.Fatal(err)
	}
	if rw.Type != task.TypeRework || rw.Sandbox != "tasks/conv_ch001_t1" {
		t.Fatalf("回炉任务应回原沙箱: %+v", rw)
	}
	orig, _ := q.Get("conv_ch001_t1")
	if orig.Status != task.StatusBlocked || orig.BlockedBy[0] != "rework_i000001" {
		t.Fatalf("原任务应 blocked 于回炉: %s/%v", orig.Status, orig.BlockedBy)
	}
	if iss.ReworkID != "rework_i000001" {
		t.Fatal("问题应记录回炉任务 id")
	}
}
