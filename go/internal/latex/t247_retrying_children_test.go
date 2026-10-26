package latex

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/session"
)

// T247：还在重试队列里的子节点不是失败——父章节这次不提交，但也不记失败。
func TestRetryingChildrenErrorIsNotFailure(t *testing.T) {
	var err error = &retryingChildrenError{Base: "chapter_900", Children: []string{"child_03", "child_01"}}
	kids, ok := isRetryingChildren(err)
	if !ok {
		t.Fatalf("isRetryingChildren = false, want true")
	}
	if strings.Join(kids, ",") != "child_03,child_01" {
		t.Fatalf("children = %v", kids)
	}
	// 报错文案按章名排序，日志里读起来稳定。
	if got := err.Error(); !strings.Contains(got, "child_01, child_03") {
		t.Fatalf("Error() = %q, 期望按章名排序", got)
	}
	// 别的错误不能被认成这一类。
	if _, ok := isRetryingChildren(errors.New("子章节转换或 checker 失败；父章节禁止提交: child_01: 没提交")); ok {
		t.Fatalf("普通失败被认成了「还在重试」")
	}
	if _, ok := isRetryingChildren(nil); ok {
		t.Fatalf("nil 被认成了「还在重试」")
	}
}

// writeConvertTranscript 造一条"最新一次转换"的转录，最后一条 chapter_activity 是 state。
func writeConvertTranscript(t *testing.T, proj, base, state string) string {
	t.Helper()
	dir := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "convert_"+base+".jsonl")
	w, err := session.NewTranscript(path)
	if err != nil {
		t.Fatalf("NewTranscript: %v", err)
	}
	if err := w.AppendNote("chapter_activity", map[string]any{"state": state}); err != nil {
		t.Fatalf("AppendNote: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path
}

func writeProduct(t *testing.T, proj, base string, withVerdict bool) {
	t.Helper()
	dir := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	tex := filepath.Join(dir, base+".tex")
	if err := os.WriteFile(tex, []byte("% product\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if withVerdict {
		if err := os.WriteFile(tex+".checker", []byte("missing section"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// T247：失败清单是按 goroutine 返回值追加的，没有生命周期。同一章后来被重试
// 队列推通了（核对 pass、.checker 已删），阶段汇总就不该再报它失败。
func TestDropHealedFailures(t *testing.T) {
	proj := t.TempDir()

	// chapter_901：先失败、后来通过。
	writeProduct(t, proj, "chapter_901", false)
	writeConvertTranscript(t, proj, "chapter_901", "pass")

	// chapter_902：仍是真失败（产物在，但核对给了 issues）。
	writeProduct(t, proj, "chapter_902", true)
	writeConvertTranscript(t, proj, "chapter_902", "issues")

	// chapter_903：没有产物。
	writeConvertTranscript(t, proj, "chapter_903", "error")

	fails := []string{
		"chapter_901.md: 子章节转换或 checker 失败；父章节禁止提交: child_02: 细分父章节整合会话未提交（已提醒两次）",
		"chapter_902.md: 子章节转换或 checker 失败；父章节禁止提交: child_01: 没提交",
		"chapter_903.md: 会话失败: context canceled",
	}
	kept, healed := dropHealedFailures(proj, fails)
	if len(healed) != 1 || healed[0] != "chapter_901" {
		t.Fatalf("healed = %v, want [chapter_901]", healed)
	}
	if len(kept) != 2 {
		t.Fatalf("kept = %v, want 两条", kept)
	}
	for _, k := range kept {
		if strings.HasPrefix(k, "chapter_901") {
			t.Fatalf("已通过的章节仍留在失败清单里: %v", kept)
		}
	}
}

// 只有"产物在 + 没有 .checker + 最新转录最后状态是 pass"三条同时成立才算通过。
func TestChapterPassedOnDiskNeedsAllThree(t *testing.T) {
	proj := t.TempDir()

	// 产物在、状态 pass、但没有 .checker → 通过。
	writeProduct(t, proj, "chapter_910", false)
	writeConvertTranscript(t, proj, "chapter_910", "pass")
	if !chapterPassedOnDisk(proj, "chapter_910") {
		t.Fatalf("chapter_910 应判为已通过")
	}

	// 状态是 pass 但产物不在 → 不算。
	writeConvertTranscript(t, proj, "chapter_911", "pass")
	if chapterPassedOnDisk(proj, "chapter_911") {
		t.Fatalf("chapter_911 没有产物，不该判为已通过")
	}

	// 产物在、状态 pass，但留着 .checker（核对没过）→ 不算。
	writeProduct(t, proj, "chapter_912", true)
	writeConvertTranscript(t, proj, "chapter_912", "pass")
	if chapterPassedOnDisk(proj, "chapter_912") {
		t.Fatalf("chapter_912 有 .checker，不该判为已通过")
	}

	// 产物在、没有 .checker，但最后状态不是 pass → 不算。
	writeProduct(t, proj, "chapter_913", false)
	writeConvertTranscript(t, proj, "chapter_913", "checking_result")
	if chapterPassedOnDisk(proj, "chapter_913") {
		t.Fatalf("chapter_913 状态不是 pass，不该判为已通过")
	}
}
