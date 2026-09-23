package sessionview

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// T74：已提交/已定型的会话即使 mtime 还在 LiveWindow 内也不算运行中。
func TestLiveNowEndSignal(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-5 * time.Second)

	// 以 SUBMIT 回执收尾 → 不 live
	st := transcriptStat{SawTool: true, SawSubmit: true, EndsWithSubmit: true}
	if liveNow(st, fresh, now) {
		t.Fatal("提交收尾 + mtime 新鲜也不应算 live")
	}
	// 以 [IMG_TYPE: 定型答案收尾 → 不 live
	st2 := transcriptStat{SawTyped: true, EndsWithTyped: true}
	if liveNow(st2, fresh, now) {
		t.Fatal("定型收尾 + mtime 新鲜也不应算 live")
	}
	// 干过活但没交 + mtime 新鲜 → live（正在跑/等模型）
	st3 := transcriptStat{SawTool: true}
	if !liveNow(st3, fresh, now) {
		t.Fatal("未提交 + 新鲜 mtime 应算 live")
	}
	// 收尾信号之后又有新写入（重跑）→ 恢复 live
	st4 := transcriptStat{SawSubmit: true, EndsWithSubmit: false}
	if !liveNow(st4, fresh, now) {
		t.Fatal("提交后又有新内容（重跑中）应算 live")
	}
	// mtime 过期 → 不 live
	if liveNow(st3, now.Add(-2*LiveWindow), now) {
		t.Fatal("mtime 超窗不应算 live")
	}
}

// readStat 的终态追踪：SUBMITTED 之后没有别的 msg 行 → EndsWithSubmit。
func TestReadStatEndSignal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	content := `{"t":"msg","role":"assistant"}
{"t":"msg","role":"tool","content":"SUBMITTED. ok"}
`
	write := content + `{"t":"msg","role":"assistant","content":"[IMG_TYPE: table]"}
`
	os.WriteFile(p, []byte(content), 0o644)
	if st, err := readStat(p); err != nil || !st.EndsWithSubmit {
		t.Fatalf("submit 收尾未识别: %+v %v", st, err)
	}
	os.WriteFile(p, []byte(write), 0o644)
	if st, err := readStat(p); err != nil || !st.EndsWithTyped || st.EndsWithSubmit {
		t.Fatalf("定型收尾未识别: %+v %v", st, err)
	}
}
