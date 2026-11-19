package gen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/verify"
)

// TestE2EClassifyLoop 端到端：图目录 → classify 任务 → agent 干活 → kernel 验收。
func TestE2EClassifyLoop(t *testing.T) {
	root := t.TempDir()
	imgDir := filepath.Join(root, "source", "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"fig1.png", "fig2.png"} {
		if err := os.WriteFile(filepath.Join(imgDir, n), []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	if _, err := GenClassify(q, root, "source/images"); err != nil {
		t.Fatal(err)
	}

	// agent 领走全部任务
	claimed := q.Claim("agent-e2e", 5, 60, task.TypeClassify)
	if len(claimed) != 2 {
		t.Fatalf("应领到 2 个任务: %d", len(claimed))
	}
	ck := verify.NewChecker(root)

	// fig1：agent 写了合法 verdict → 通过记 done
	good := claimed[0]
	outDir := filepath.Join(root, good.Sandbox, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "verdict.json"),
		[]byte(`{"class":"text","styled":true,"style_note":"测试样式线索","confidence":0.9,"extracted":"图 1-1 决策树"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	errs, notes := ck.Run(good)
	if len(errs) != 0 {
		t.Fatalf("合法 classify 应通过: %v", errs)
	}
	if _, ok := notes["artifact_sha256"]; !ok {
		t.Fatal("应带哈希快照")
	}
	v := task.Verdict{At: time.Now(), Pass: true, Notes: notes}
	if _, err := q.RecordVerdict(good.ID, v, 6); err != nil {
		t.Fatal(err)
	}
	if st, _ := q.Get(good.ID); st.Status != task.StatusDone {
		t.Fatalf("应记 done: %s", st.Status)
	}

	// fig2：agent 写了空 class（想蒙混）→ 打回 pending
	bad := claimed[1]
	outDir2 := filepath.Join(root, bad.Sandbox, "out")
	if err := os.MkdirAll(outDir2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir2, "verdict.json"), []byte(`{"class":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	errs, _ = ck.Run(bad)
	if len(errs) == 0 {
		t.Fatal("空 class 应打回（不降级判据）")
	}
	v2 := task.Verdict{At: time.Now(), Pass: false, Errors: errs}
	nb, _ := q.RecordVerdict(bad.ID, v2, 6)
	if nb.Status != task.StatusPending || nb.Attempts != 1 {
		t.Fatalf("打回应回 pending 且 attempts+1: %s/%d", nb.Status, nb.Attempts)
	}

	// 冷启动续接：重新加载队列（模拟新会话），状态原样
	q2, _ := queue.Load(filepath.Join(root, "q.json"))
	st, _ := q2.Get(good.ID)
	if st.Status != task.StatusDone {
		t.Fatalf("重载后 done 应保持: %s", st.Status)
	}
	// 打回即释放租约、任务回 pending → 可立即重领（agent 补完再交）
	// （"租约过期回收"的中断恢复场景由 queue_test 覆盖，此处不重复）
	if again := q2.Claim("agent-2", 5, 60, task.TypeClassify); len(again) != 1 || again[0].ID != bad.ID {
		t.Fatalf("打回后应立即可重领: %+v", again)
	}
}

// TestE2EConvertLoop 端到端：章节 md → convert 任务 → agent 交 .tex → 验收
// （本机无 pdflatex：编译跳过记 note；PDF 缺失覆盖率跳过——环境缺失不卡死判据）。
func TestE2EConvertLoop(t *testing.T) {
	root := t.TempDir()
	chDir := filepath.Join(root, "chapters")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "# 第一章\n"
	for i := 0; i < 30; i++ {
		md += "监督学习与无监督学习的模型评估方法在特征工程中的应用实践案例。\n"
	}
	if err := os.WriteFile(filepath.Join(chDir, "chapter_001.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	if _, err := GenConvert(q, root, "chapters"); err != nil {
		t.Fatal(err)
	}
	claimed := q.Claim("agent-e2e", 5, 60, task.TypeConvert)
	if len(claimed) != 1 {
		t.Fatalf("应领到 1 个 convert 任务: %d", len(claimed))
	}
	tk := claimed[0]
	outDir := filepath.Join(root, tk.Sandbox, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 坏产物：引用临时路径（自包含判据）→ 打回
	badTex := "\\chapter{一}\n\\input{_subdivision/sec1}\n"
	if err := os.WriteFile(filepath.Join(outDir, "chapter_001.tex"), []byte(badTex), 0o644); err != nil {
		t.Fatal(err)
	}
	ck := verify.NewChecker(root)
	errs, _ := ck.Run(tk)
	if len(errs) == 0 || !containsStr(errs, "临时路径") {
		t.Fatalf("临时路径引用应被打回: %v", errs)
	}

	// 好产物：自包含 → 通过（编译/覆盖率因缺工具跳过记 note）
	goodTex := "\\chapter{一}\n正文内容\n"
	if err := os.WriteFile(filepath.Join(outDir, "chapter_001.tex"), []byte(goodTex), 0o644); err != nil {
		t.Fatal(err)
	}
	errs, notes := ck.Run(tk)
	if len(errs) != 0 {
		t.Fatalf("自包含产物应通过: %v", errs)
	}
	if _, ok := notes["artifact_sha256"]; !ok {
		t.Fatal("应带哈希快照")
	}
	v := task.Verdict{At: time.Now(), Pass: true, Notes: notes}
	nt, _ := q.RecordVerdict(tk.ID, v, 6)
	if nt.Status != task.StatusDone {
		t.Fatalf("应记 done: %s", nt.Status)
	}
}

func containsStr(errs []string, sub string) bool {
	for _, e := range errs {
		for i := 0; i+len(sub) <= len(e); i++ {
			if e[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
