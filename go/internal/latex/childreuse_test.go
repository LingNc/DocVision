package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T236：子章节上一次已经转换并通过核对时，父节点重新派发不该把它再跑一遍。
func TestChildPassedEarlier(t *testing.T) {
	nodeRoot := t.TempDir()
	base := "chapter_002_child_01"
	sess := filepath.Join(nodeRoot, "sessions")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	stem := filepath.Join(sess, "convert_"+base+".jsonl")
	writeRec := func(trPath string, lines ...string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(trPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range lines {
			if _, err := f.WriteString(l + "\n"); err != nil {
				t.Fatal(err)
			}
		}
		_ = f.Close()
	}
	activity := func(state string) string {
		return `{"t":"meta","kind":"chapter_activity","state":"` + state + `","ts":"2026-10-24T00:00:00+08:00"}`
	}

	// 产物还没有：一律重跑（哪怕转录里已经写着 pass）。
	writeRec(stem, activity("issues"), activity("pass"))
	if childPassedEarlier(nodeRoot, base) {
		t.Fatal("产物不在时不该认为可以复用")
	}
	result := filepath.Join(nodeRoot, "result")
	if err := os.MkdirAll(result, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result, base+".tex"), []byte("%% child\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !childPassedEarlier(nodeRoot, base) {
		t.Fatal("产物在、最后一条状态是 pass 时应可复用")
	}

	// 后面又被判过问题 / 报过错：不能复用（那一次还得接着改）。
	writeRec(stem, activity("issues"))
	if childPassedEarlier(nodeRoot, base) {
		t.Fatal("最后一条是 issues 时不该复用")
	}
	writeRec(stem, activity("pass"))
	if !childPassedEarlier(nodeRoot, base) {
		t.Fatal("又通过之后应可复用")
	}

	// 新的一次尝试（try2）才是最近的状态：它以 error 收尾，不能拿第 1 次的 pass 顶。
	try2 := filepath.Join(sess, "convert_"+base+".try2.jsonl")
	writeRec(try2, activity("error"))
	if childPassedEarlier(nodeRoot, base) {
		t.Fatal("最新一次尝试是 error 时不该复用")
	}
}

func TestLastActivityStateIgnoresUsageAndMessages(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "convert_chapter_001.jsonl")
	body := `{"t":"meta","kind":"chapter_activity","state":"running"}` + "\n" +
		`{"t":"msg","role":"assistant","text":"跑完了"}` + "\n" +
		`{"t":"usage","input":1}` + "\n" +
		`{"t":"meta","kind":"run_end","ok":true}` + "\n"
	if err := os.WriteFile(tr, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lastActivityState(tr); got != "running" {
		t.Fatalf("lastActivityState = %q", got)
	}
	if got := lastActivityState(filepath.Join(dir, "nope.jsonl")); got != "" {
		t.Fatalf("不存在的转录应返回空串，得到 %q", got)
	}
}
