package latex

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/logger"
)

// newRoundTestRunner 给测试一个安静的真 logger（快照/回退的告警走它）。
func newRoundTestRunner(t *testing.T) *Runner {
	t.Helper()
	lg, err := logger.NewLogger(filepath.Join(t.TempDir(), "rounds.log"), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lg.Close() })
	lg.SetQuiet(true)
	return &Runner{log: lg}
}

// P55：每落一条消息拍一张工作区快照；从第 N 条记录继续时工作树退回那一刻
// ——改过的文件恢复原样，后来新建的文件被清掉。
func TestRoundLogRollback(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_002"
	work := filepath.Join(proj, "work", "temp", "conv_"+base, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := newRoundTestRunner(t)
	write("chapter_002.tex", "%% v1\n")
	l := r.openRoundLog(proj, base, work, 1)
	if l == nil {
		t.Fatal("快照仓库没开起来（宿主有 git 吗？）")
	}
	l.commitLine(2)
	write("chapter_002.tex", "%% v2 跑飞了\n")
	write("junk.tex", "scratch\n")
	l.commitLine(4)

	// 退回第 4 行：v2 与 junk 都在。
	if _, ok := r.rollbackChapterWork(proj, base, "convert_"+base+".jsonl", 4); !ok {
		t.Fatal("第 4 行应能回退")
	}
	if got, _ := os.ReadFile(filepath.Join(work, "chapter_002.tex")); string(got) != "%% v2 跑飞了\n" {
		t.Fatalf("第 4 行内容 = %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(work, "junk.tex")); err != nil {
		t.Fatalf("junk.tex 该在: %v", err)
	}

	// 退回第 2 行：v1 回来，junk 被清掉。
	if _, ok := r.rollbackChapterWork(proj, base, "convert_"+base+".jsonl", 2); !ok {
		t.Fatal("第 2 行应能回退")
	}
	if got, _ := os.ReadFile(filepath.Join(work, "chapter_002.tex")); string(got) != "%% v1\n" {
		t.Fatalf("第 2 行内容 = %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(work, "junk.tex")); err == nil {
		t.Fatal("junk.tex 应被清掉")
	}

	// 行号落在两张快照之间：退到更早的那张（宁可多退一轮，不留半新半旧）。
	if n, ok := r.rollbackChapterWork(proj, base, "convert_"+base+".jsonl", 3); !ok || n != 2 {
		t.Fatalf("第 3 行应退到 2，得到 %d / %v", n, ok)
	}
}

// 没有快照（老会话、git 不在、深度 > 0）时降级成"只回退对话"：不报错、不拦。
func TestRoundLogMissingJournalDegrades(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_003"
	r := newRoundTestRunner(t)
	if _, ok := r.rollbackChapterWork(proj, base, "convert_"+base+".jsonl", 5); ok {
		t.Fatal("没有快照时不该声称回退成功")
	}
	// 快照仓库在、但那一行之前没有条目。
	work := filepath.Join(proj, "work", "temp", "conv_"+base, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	l := r.openRoundLog(proj, base, work, 1)
	if l == nil {
		t.Skip("宿主没有 git")
	}
	l.commitLine(8)
	if _, ok := r.rollbackChapterWork(proj, base, "convert_"+base+".jsonl", 3); ok {
		t.Fatal("第 3 行之前没有快照，不该声称回退成功")
	}
	if _, ok := r.rollbackChapterWork(proj, base, "convert_"+base+".jsonl", 8); !ok {
		t.Fatal("第 8 行应能回退")
	}
}

// T236：回退工作区不能连 `_subdivision`（子章节自己的转录与产物）一起退回那一刻。
// 父会话续跑之后会照常重新派发子章节，把它们退掉就等于把已经做完的子节点从头
// 再跑一遍——第四章实测就是这么重烧了四个子章节。
func TestRoundLogRollbackKeepsSubdivision(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_002"
	work := filepath.Join(proj, "work", "temp", "conv_"+base, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := newRoundTestRunner(t)
	write(filepath.Join(work, base+".tex"), "%% v1\n")
	l := r.openRoundLog(proj, base, work, 1)
	if l == nil {
		t.Skip("宿主没有 git")
	}
	l.commitLine(2)
	childTex := filepath.Join(work, "_subdivision", "child_01", "node", "result", base+"_child_01.tex")
	write(childTex, "%% 子章节产物\n")
	write(filepath.Join(work, "junk.tex"), "scratch\n")
	l.commitLine(4)

	if _, ok := r.rollbackChapterWork(proj, base, "convert_"+base+".jsonl", 2); !ok {
		t.Fatal("第 2 行应能回退")
	}
	if got, _ := os.ReadFile(filepath.Join(work, base+".tex")); string(got) != "%% v1\n" {
		t.Fatalf("父章节该退回第 2 行，得到 %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(work, "junk.tex")); err == nil {
		t.Fatal("后来新建的临时文件应被清掉")
	}
	if _, err := os.Stat(childTex); err != nil {
		t.Fatalf("子章节产物应保留: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(work), ".subdivision-keep")); err == nil {
		t.Fatal("暂存目录应已放回、不留残渣")
	}
}

func TestTranscriptAttemptParse(t *testing.T) {
	cases := []struct {
		name     string
		wantBase string
		wantN    int
		wantOK   bool
	}{
		{"convert_chapter_002.jsonl", "convert_chapter_002", 1, true},
		{"convert_chapter_002.try3.jsonl", "convert_chapter_002", 3, true},
		{"checker_chapter_002.try2.jsonl", "", 0, false},
		{"chapter_002.md", "", 0, false},
	}
	for _, c := range cases {
		b, n, ok := transcriptAttempt(c.name)
		if b != c.wantBase || n != c.wantN || ok != c.wantOK {
			t.Fatalf("%s → %q/%d/%v", c.name, b, n, ok)
		}
	}
}

// 没有快照的老会话（P55 之前产生的）：只退对话，工作区与**已提交的产物**都
// 不动，也不该为此凭空建一个空仓库。
func TestApplyWorkspaceCutWithoutSnapshotKeepsProducts(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_004"
	chapters := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(chapters, 0o755); err != nil {
		t.Fatal(err)
	}
	tex := filepath.Join(chapters, base+".tex")
	if err := os.WriteFile(tex, []byte("%% kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(proj, "work", "temp", "conv_"+base, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "scratch.tex"), []byte("later edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := newRoundTestRunner(t)
	rolled, ok := r.applyWorkspaceCut(proj, base, "convert_"+base+".jsonl", 7)
	if ok || rolled != 0 {
		t.Fatalf("没有快照时不该声称回退: %d / %v", rolled, ok)
	}
	if _, err := os.Stat(tex); err != nil {
		t.Fatalf("已提交的产物不该被挪走: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(work, "scratch.tex")); string(got) != "later edit\n" {
		t.Fatalf("工作区不该被动: %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(proj, "work", "rounds", base)); err == nil {
		t.Fatal("老会话不该被建出一个空快照仓库")
	}
}
