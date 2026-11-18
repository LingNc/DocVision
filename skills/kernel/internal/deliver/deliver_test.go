package deliver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/verify"
)

// seedDoneTask 造一个 done 任务（带真实哈希快照：跑一遍 Checker.Run）。
func seedDoneTask(t *testing.T, q *queue.Queue, root string, tk *task.Task, outFiles map[string]string) {
	t.Helper()
	outDir := filepath.Join(root, tk.Sandbox, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range outFiles {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ck := verify.NewChecker(root)
	_, notes := ck.Run(tk)
	tk.Status = task.StatusDone
	tk.Verdicts = []task.Verdict{{At: time.Now(), Pass: true, Notes: notes}}
	if err := q.Enqueue(tk); err != nil {
		t.Fatal(err)
	}
}

func TestDeliverHappyPath(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	// 两章 done + 一张图 done
	seedDoneTask(t, q, root, &task.Task{
		ID: "conv_chapter_001", Type: task.TypeConvert, Stage: "convert", Sandbox: "tasks/conv_chapter_001",
		Accept: map[string]any{"main_file": "chapter_001.tex"},
	}, map[string]string{"chapter_001.tex": "\\chapter{一}\n正文\n"})
	seedDoneTask(t, q, root, &task.Task{
		ID: "conv_chapter_002", Type: task.TypeConvert, Stage: "convert", Sandbox: "tasks/conv_chapter_002",
		Accept: map[string]any{"main_file": "chapter_002.tex"},
	}, map[string]string{"chapter_002.tex": "\\chapter{二}\n正文\n"})
	seedDoneTask(t, q, root, &task.Task{
		ID: "drw_figa", Type: task.TypeDraw, Stage: "draw", Sandbox: "tasks/drw_figa",
		Accept: map[string]any{},
	}, map[string]string{"figure.tex": "% tikz\n"})

	rep, err := Deliver(q, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean || len(rep.Chapters) != 2 || len(rep.Figures) != 1 {
		t.Fatalf("归集错: %+v", rep)
	}
	// 主文件按章序 + 带书签
	main, _ := os.ReadFile(rep.MainTex)
	s := string(main)
	if !contains(s, "chapter_001") || !contains(s, "chapter_002") || rep.Bookmarks != 2 {
		t.Fatalf("主文件/书签错: %.300s", s)
	}
	// standalone 内联了两章
	st, _ := os.ReadFile(rep.Standalone)
	if !contains(string(st), "% ==== chapters/chapter_001.tex ====") || !contains(string(st), "% ==== chapters/chapter_002.tex ====") {
		t.Fatalf("standalone 内联错: %.200s", st)
	}
	// 盘点：两章就位、无缺失引用
	if rep.Preflight.Chapters != 2 || len(rep.Preflight.RefsMissing) != 0 {
		t.Fatalf("盘点错: %+v", rep.Preflight)
	}
}

func TestDeliverDriftGate(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	seedDoneTask(t, q, root, &task.Task{
		ID: "conv_chapter_001", Type: task.TypeConvert, Stage: "convert", Sandbox: "tasks/conv_chapter_001",
		Accept: map[string]any{"main_file": "chapter_001.tex"},
	}, map[string]string{"chapter_001.tex": "v1\n"})
	// 验收后篡改产物
	if err := os.WriteFile(filepath.Join(root, "tasks/conv_chapter_001/out/chapter_001.tex"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Deliver(q, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean || len(rep.Drift) != 1 {
		t.Fatalf("漂移闸门应拒装配: %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(root, "build", "main.tex")); !os.IsNotExist(err) {
		t.Fatal("漂移时不动 build 树")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
