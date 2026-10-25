package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// backdateForRestore 把一棵树（或一个文件）的 mtime 拨旧，让"这一章还在写"的
// 守卫（2 分钟）不要挡住测试。
func backdateForRestore(t *testing.T, root string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, old, old)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// restoreFixture 搭一个"第 3 次尝试在跑、第 1 次已经整棵留档"的项目。
func restoreFixture(t *testing.T) (proj, base string) {
	t.Helper()
	proj, base = t.TempDir(), "chapter_004"
	sess := filepath.Join(proj, "work", "sessions")
	// 三条转换转录：第 1 次沿用原名，第 2/3 次带 .tryN（就是换尝试的命名规则）。
	writeFile(t, filepath.Join(sess, "convert_"+base+".jsonl"), `{"t":"meta"}`+"\n")
	writeFile(t, filepath.Join(sess, "convert_"+base+".try2.jsonl"), `{"t":"meta"}`+"\n")
	writeFile(t, filepath.Join(sess, "convert_"+base+".try3.jsonl"), `{"t":"meta"}`+"\n")
	// 第 1 次尝试的留档：工作区、result、提交产物、分片目录、报告、问题清单。
	arch := filepath.Join(proj, "work", "attempts", base, "try1")
	writeFile(t, filepath.Join(arch, "work", "state.txt"), "old-work")
	writeFile(t, filepath.Join(arch, "result", "r.txt"), "old-result")
	writeFile(t, filepath.Join(arch, base+".tex"), "old-tex")
	writeFile(t, filepath.Join(arch, base, "p1.tex"), "old-part")
	writeFile(t, filepath.Join(arch, base+".report.md"), "old-report")
	writeFile(t, filepath.Join(arch, "checker-issues.md"), "old-issues")
	// 现役（第 3 次）的工作区与产物。
	node := filepath.Join(proj, "work", "temp", "conv_"+base)
	writeFile(t, filepath.Join(node, "work", "state.txt"), "live-work")
	writeFile(t, filepath.Join(proj, "work", "chapters", base+".tex"), "live-tex")
	writeFile(t, filepath.Join(proj, "work", "reports", base+".md"), "live-report")
	backdateForRestore(t, filepath.Join(proj, "work"))
	return proj, base
}

func readFileAt(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读不到 %s: %v", path, err)
	}
	return string(data)
}

// 切回第 N 次尝试 = 两边互换：现役整棵进留档、留档整棵回工作区，转录跟着走。
func TestRestoreChapterAttemptSwapsLiveAndArchive(t *testing.T) {
	proj, base := restoreFixture(t)
	if why := RestorePrecondition(proj, base, 1); why != "" {
		t.Fatalf("应允许切回第 1 次，却被拒：%s", why)
	}
	msg, err := RestoreChapterAttempt(proj, base, 1)
	if err != nil {
		t.Fatalf("切回失败: %v", err)
	}
	if !strings.Contains(msg, "已切回第 1 次尝试") {
		t.Fatalf("说明里应点名切回到第几次：%s", msg)
	}
	// ① 工作区换成了第 1 次那一份。
	if got := readFileAt(t, filepath.Join(proj, "work", "temp", "conv_"+base, "work", "state.txt")); got != "old-work" {
		t.Fatalf("工作区没换回来: %q", got)
	}
	if got := readFileAt(t, filepath.Join(proj, "work", "chapters", base+".tex")); got != "old-tex" {
		t.Fatalf("产物没换回来: %q", got)
	}
	if got := readFileAt(t, filepath.Join(proj, "work", "chapters", base, "p1.tex")); got != "old-part" {
		t.Fatalf("分片目录没换回来: %q", got)
	}
	if got := readFileAt(t, filepath.Join(proj, "work", "reports", base+".md")); got != "old-report" {
		t.Fatalf("报告没换回来: %q", got)
	}
	// ② 现役这一次整棵进了它自己的留档（第 3 次）。
	parked := filepath.Join(proj, "work", "attempts", base, "try3")
	if got := readFileAt(t, filepath.Join(parked, "work", "state.txt")); got != "live-work" {
		t.Fatalf("现役工作区没留档: %q", got)
	}
	if got := readFileAt(t, filepath.Join(parked, base+".tex")); got != "live-tex" {
		t.Fatalf("现役产物没留档: %q", got)
	}
	if got := readFileAt(t, filepath.Join(parked, base+".report.md")); got != "live-report" {
		t.Fatalf("现役报告没留档: %q", got)
	}
	// ③ 转录跟着号走：第 2/3 次的进了留档，第 1 次的留在 work/sessions。
	sess := filepath.Join(proj, "work", "sessions")
	for _, p := range []string{
		filepath.Join(parked, "convert_"+base+".try3.jsonl"),
		filepath.Join(proj, "work", "attempts", base, "try2", "convert_"+base+".try2.jsonl"),
		filepath.Join(sess, "convert_"+base+".jsonl"),
	} {
		if !fileExists(p) {
			t.Fatalf("转录应该在 %s", p)
		}
	}
	if fileExists(filepath.Join(sess, "convert_"+base+".try3.jsonl")) {
		t.Fatal("第 3 次的转录应该已经跟着留档走，不该留在 work/sessions")
	}
	// ④ 切回来以后"现在这一次"就是第 1 次。
	if live := lastAttemptNumber(filepath.Join(sess, "convert_"+base+".jsonl")); live != 1 {
		t.Fatalf("切回后现役尝试号应为 1，实际 %d", live)
	}
	// ⑤ 留档里只属于留档的东西（问题清单）留在原地，不搬进工作区。
	if !fileExists(filepath.Join(proj, "work", "attempts", base, "try1", "checker-issues.md")) {
		t.Fatal("checker-issues.md 应留在留档目录里")
	}
	if fileExists(filepath.Join(proj, "work", "attempts", base, "try1", base+".tex")) {
		t.Fatal("搬回去的产物不该在留档里留副本")
	}
	// ⑥ 再切一次第 1 次就是"现在这一次"：拒绝，且不会把工作区搬空。
	if why := RestorePrecondition(proj, base, 1); why == "" {
		t.Fatal("第 1 次已经是现役，应该拒绝")
	}
	if got := readFileAt(t, filepath.Join(proj, "work", "chapters", base+".tex")); got != "old-tex" {
		t.Fatalf("被拒的切回不该动文件: %q", got)
	}
}

// 切回去再切回来：现役留着的那份（第 3 次）还能整棵搬回来，且不留两套布局。
func TestRestoreChapterAttemptRoundTrip(t *testing.T) {
	proj, base := restoreFixture(t)
	if _, err := RestoreChapterAttempt(proj, base, 1); err != nil {
		t.Fatalf("第一次切回失败: %v", err)
	}
	if why := RestorePrecondition(proj, base, 3); why != "" {
		t.Fatalf("应允许切回第 3 次，却被拒：%s", why)
	}
	if _, err := RestoreChapterAttempt(proj, base, 3); err != nil {
		t.Fatalf("切回第 3 次失败: %v", err)
	}
	if got := readFileAt(t, filepath.Join(proj, "work", "temp", "conv_"+base, "work", "state.txt")); got != "live-work" {
		t.Fatalf("第 3 次的工作区没搬回来: %q", got)
	}
	if got := readFileAt(t, filepath.Join(proj, "work", "chapters", base+".tex")); got != "live-tex" {
		t.Fatalf("第 3 次的产物没搬回来: %q", got)
	}
	if live := lastAttemptNumber(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")); live != 3 {
		t.Fatalf("往返后现役尝试号应为 3，实际 %d", live)
	}
	// 第 1 次那份回到 try1，问题清单还是它自己的那份。
	if !fileExists(filepath.Join(proj, "work", "attempts", base, "try1", base+".tex")) {
		t.Fatal("第 1 次的产物应回到 try1")
	}
	if !fileExists(filepath.Join(proj, "work", "attempts", base, "try1", "checker-issues.md")) {
		t.Fatal("第 1 次的问题清单不该丢")
	}
}

// 「这一章还在写」的守卫：本章转录、子树里的子节点转录都算，拨旧了才放行。
func TestRestorePreconditionBlocksWhileChapterIsWriting(t *testing.T) {
	proj, base := restoreFixture(t)
	// 把现役那条转录拨到"刚刚还在写"。
	live := filepath.Join(proj, "work", "sessions", "convert_"+base+".try3.jsonl")
	now := time.Now()
	if err := os.Chtimes(live, now, now); err != nil {
		t.Fatal(err)
	}
	if why := RestorePrecondition(proj, base, 1); why == "" || !strings.Contains(why, "还在被写入") {
		t.Fatalf("本章转录刚在写时应拒绝，实际：%q", why)
	}
	backdateForRestore(t, live)

	// 父章节自己那条可能几小时不动，真正在写的是子树里的子节点。
	child := filepath.Join(proj, "work", "temp", "conv_"+base, "work", "_subdivision",
		"node", "sessions", "convert_"+base+"_child_01.jsonl")
	writeFile(t, child, `{"t":"meta"}`+"\n")
	if why := RestorePrecondition(proj, base, 1); why == "" || !strings.Contains(why, "_child_01") {
		t.Fatalf("子节点转录刚在写时应拒绝并点名它，实际：%q", why)
	}
	backdateForRestore(t, child)

	// 流式快照 sidecar（.partial）也算在写。
	partial := filepath.Join(proj, "work", "sessions", "convert_"+base+".try2.jsonl.partial")
	writeFile(t, partial, "{}\n")
	if why := RestorePrecondition(proj, base, 1); why == "" {
		t.Fatal("有新鲜 .partial 时应拒绝")
	}
	backdateForRestore(t, partial)

	if why := RestorePrecondition(proj, base, 1); why != "" {
		t.Fatalf("都拨旧了应该放行，实际：%s", why)
	}
}

func TestRestorePreconditionRejects(t *testing.T) {
	proj, base := restoreFixture(t)
	cases := []struct {
		name    string
		base    string
		attempt int
		want    string
	}{
		{"没写第几次", base, 0, "没写要切回第几次"},
		{"没有这份留档", base, 7, "没有第 7 次尝试的留档"},
		{"子章节归父章节", base + "_child_02", 1, "请对父章节切回"},
		{"章节名带路径", "../" + base, 1, "章节名不合法"},
	}
	for _, c := range cases {
		if why := RestorePrecondition(proj, c.base, c.attempt); !strings.Contains(why, c.want) {
			t.Fatalf("%s：想看到 %q，实际 %q", c.name, c.want, why)
		}
	}
	if _, err := RestoreChapterAttempt(proj, base+"_child_02", 1); err == nil {
		t.Fatal("子章节的切回应该直接失败")
	}
}
