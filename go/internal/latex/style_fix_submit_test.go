package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T64（重构，原 T66）：分块提交的勾选核对走共享存储——本块问题
// 必须全部 Checked，少一个就 REJECTED，不靠模型报参数。
func TestSubmitBlockFixRequiresAllChecked(t *testing.T) {
	mk := func(t *testing.T, checked ...string) *ProblemStore {
		t.Helper()
		st := loadProblemStore(filepath.Join(t.TempDir(), "problems.json"))
		done := map[string]bool{}
		for _, id := range checked {
			done[id] = true
		}
		for _, id := range []string{"P1", "P2", "P3"} {
			st.Problems = append(st.Problems, StyleProblem{ID: id, Title: id, Detail: id, Checked: done[id]})
		}
		return st
	}
	t.Run("缺勾打回", func(t *testing.T) {
		submit := &SubmitBlockFixTool{Assigned: []string{"P1", "P2", "P3"}, Store: mk(t, "P1", "P2")}
		res, err := submit.Execute(`{"chapters":[{"chapter":"chapter_001","resolved":true}]}`)
		if err != nil {
			t.Fatal(err)
		}
		if submit.Submitted {
			t.Fatal("缺勾却提交成功了")
		}
		if !strings.Contains(res.Text, "P3") {
			t.Fatalf("回执没点名缺失的 P3: %q", res.Text)
		}
	})
	t.Run("全勾通过", func(t *testing.T) {
		submit := &SubmitBlockFixTool{Assigned: []string{"P1", "P2"}, Store: mk(t, "P1", "P2")}
		res, err := submit.Execute(`{"chapters":[{"chapter":"chapter_001","resolved":true,"note":"ok"}]}`)
		if err != nil {
			t.Fatal(err)
		}
		if !submit.Submitted || !strings.Contains(res.Text, "SUBMITTED") {
			t.Fatalf("全勾却被拒: %q", res.Text)
		}
	})
	t.Run("计划未完成打回", func(t *testing.T) {
		g := NewGoalTool(filepath.Join(t.TempDir(), "plan.md"))
		_, _ = g.Execute(`{"op":"add","text":"待办"}`)
		submit := &SubmitBlockFixTool{Assigned: []string{"P1"}, Store: mk(t, "P1"), Goal: g}
		res, _ := submit.Execute(`{"chapters":[{"chapter":"chapter_001","resolved":true}]}`)
		if submit.Submitted || !strings.Contains(res.Text, "待办") {
			t.Fatalf("计划未完成应打回: %q", res.Text)
		}
	})
	t.Run("无存储时不受限", func(t *testing.T) {
		submit := &SubmitBlockFixTool{}
		if _, err := submit.Execute(`{"chapters":[{"chapter":"chapter_001","resolved":true}]}`); err != nil {
			t.Fatal(err)
		}
		if !submit.Submitted {
			t.Fatal("无清单场景不应被打回")
		}
	})
}

// T64：problems 工具的权限分离与增删改查勾。
func TestProblemsTool(t *testing.T) {
	dir := t.TempDir()
	st := loadProblemStore(filepath.Join(dir, "problems.json"))
	w := &ProblemsTool{Store: st, Write: true}
	r := &ProblemsTool{Store: st}

	must := func(tool *ProblemsTool, args, want string) string {
		res, err := tool.Execute(args)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Text, want) {
			t.Fatalf("%s → %q（要含 %q）", args, res.Text, want)
		}
		return res.Text
	}
	// 写侧：逐条 add（自动编号）→ update → delete。
	must(w, `{"op":"add","title":"题号样式","detail":"粗体","chapters":["chapter_001"]}`, "P1")
	must(w, `{"op":"add","title":"表格跨页","detail":"longtable"}`, "P2")
	must(w, `{"op":"update","id":"P2","detail":"改后"}`, "已更新")
	must(w, `{"op":"get","id":"P2"}`, "改后")
	must(w, `{"op":"add","id":"P1","title":"撞","detail":"车"}`, "已存在")
	// 读侧：list/search/check。
	must(r, `{"op":"list"}`, "P1")
	must(r, `{"op":"search","query":"粗体"}`, "P1")
	must(r, `{"op":"check","ids":["P1","P2"]}`, "打勾 2 个")
	must(r, `{"op":"get","id":"P1"}`, "[x]")
	must(r, `{"op":"check","id":"P1","checked":false}`, "取消打勾")
	// 权限：读侧不能写，写侧不能勾。
	must(r, `{"op":"add","title":"x","detail":"y"}`, "REJECTED")
	must(w, `{"op":"check","id":"P1"}`, "REJECTED")
	// 落盘持久化：新句柄读回同一状态。
	st2 := loadProblemStore(filepath.Join(dir, "problems.json"))
	if len(st2.Problems) != 2 || st2.Problems[1].Detail != "改后" {
		t.Fatalf("持久化状态 = %+v", st2.Problems)
	}
	must(w, `{"op":"delete","id":"P1"}`, "已删除")
	// 删除中间项后新 id 继续递增（P3，不重用 P1 防混淆）。
	must(w, `{"op":"add","title":"新问题","detail":"d"}`, "P3")
}

// T64：goal 工具——有序计划、plan.md 落盘、提交门 Pending。
func TestGoalTool(t *testing.T) {
	planPath := filepath.Join(t.TempDir(), "plan.md")
	g := NewGoalTool(planPath)
	exec := func(args string) string {
		res, err := g.Execute(args)
		if err != nil {
			t.Fatal(err)
		}
		return res.Text
	}
	exec(`{"op":"add","text":"读汇报"}`)
	exec(`{"op":"add","text":"整合清单"}`)
	if out := exec(`{"op":"list"}`); !strings.Contains(out, "1. [ ] 读汇报") || !strings.Contains(out, "2. [ ] 整合清单") {
		t.Fatalf("list = %q", out)
	}
	if pend := g.Pending(); len(pend) != 2 {
		t.Fatalf("pending = %v", pend)
	}
	exec(`{"op":"check","n":1}`)
	if pend := g.Pending(); len(pend) != 1 || !strings.Contains(pend[0], "整合清单") {
		t.Fatalf("pending = %v", pend)
	}
	// plan.md 落盘且新句柄能读回（含勾选态）。
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "- [x] 读汇报") {
		t.Fatalf("plan.md = %q", data)
	}
	g2 := NewGoalTool(planPath)
	if len(g2.Pending()) != 1 {
		t.Fatalf("读回 pending = %v", g2.Pending())
	}
	// 批量 add + 批量 check（一次调用多条，模型不必逐条刷）。
	// 此刻清单：1 读汇报[x] 2 整合清单[ ] 3 任务A 4 任务B 5 任务C
	exec(`{"op":"add","items":["任务A","任务B","任务C"]}`)
	if pend := g.Pending(); len(pend) != 4 {
		t.Fatalf("批量 add 后 pending = %v", pend)
	}
	exec(`{"op":"check","ns":[3,4]}`)
	if pend := g.Pending(); len(pend) != 2 || !strings.Contains(pend[1], "任务C") {
		t.Fatalf("批量 check 后 pending = %v", pend)
	}
	if out := exec(`{"op":"check","ns":[99]}`); !strings.Contains(out, "REJECTED") {
		t.Fatalf("越界批量 check 必须拒: %q", out)
	}
	exec(`{"op":"delete","n":2}`) // 整合清单
	exec(`{"op":"delete","n":4}`) // 任务C
	if len(g.Pending()) != 0 {
		t.Fatalf("pending = %v", g.Pending())
	}
}

// problems 批量形：get {ids}/update {items}/delete {ids}。
func TestProblemsToolBatchOps(t *testing.T) {
	dir := t.TempDir()
	store := loadProblemStore(filepath.Join(dir, "problems.json"))
	tool := &ProblemsTool{Store: store, Write: true}
	exec := func(args string) string {
		res, err := tool.Execute(args)
		if err != nil {
			t.Fatal(err)
		}
		return res.Text
	}
	exec(`{"op":"add","items":[{"title":"a","detail":"da"},{"title":"b","detail":"db"},{"title":"c","detail":"dc"}]}`)
	// get 批量
	if out := exec(`{"op":"get","ids":["P1","P3"]}`); !strings.Contains(out, "da") || !strings.Contains(out, "dc") || strings.Contains(out, "db") {
		t.Fatalf("get 批量 = %q", out)
	}
	// update 批量
	exec(`{"op":"update","items":[{"id":"P1","title":"A2"},{"id":"P2","detail":"db2"}]}`)
	if out := exec(`{"op":"get","id":"P1"}`); !strings.Contains(out, "A2") {
		t.Fatalf("update 批量没生效: %q", out)
	}
	// update 批量里坏 id 整批不动
	exec(`{"op":"update","items":[{"id":"P3","title":"C2"},{"id":"P9","title":"x"}]}`)
	if out := exec(`{"op":"get","id":"P3"}`); strings.Contains(out, "C2") {
		t.Fatalf("坏 id 应整批拒绝: %q", out)
	}
	// delete 批量
	exec(`{"op":"delete","ids":["P1","P2"]}`)
	if out := exec(`{"op":"list"}`); strings.Contains(out, "P1") || strings.Contains(out, "P2") || !strings.Contains(out, "P3") {
		t.Fatalf("delete 批量 = %q", out)
	}
}
