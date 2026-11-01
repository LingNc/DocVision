package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T278：重核必须开一条**磁盘上还不存在**的核对尝试。以前用内存里的尝试号
// （进程重启后是 0）→ 一律算成 try2，而 try2 往往就是上一次退化到底的转录：
// 恢复它等于把"重新核对"当场判死（现场：chapter_011 的 try2 已是第五次退化的
// 现场，89 条历史、提醒打满，第一轮就再次退化）。
func TestFreshCheckerAttemptSkipsExistingTranscripts(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_011"
	dir := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	tr := filepath.Join(dir, "checker_"+base+".jsonl")
	// 1（无后缀）、try2、try5 都已经在磁盘上（都是旧现场）。
	for _, p := range []string{tr, attemptTranscriptPath(tr, 2), attemptTranscriptPath(tr, 5)} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := newRoundTestRunner(t)
	n := r.freshCheckerAttemptNo(proj, base)
	if n != 6 {
		t.Fatalf("应当接着磁盘上的最大号往后开（6），got %d", n)
	}
	if fileExists(attemptTranscriptPath(tr, n)) {
		t.Fatalf("try%d 已经存在，重核会去恢复那条旧会话", n)
	}
	if next := r.freshCheckerAttemptNo(proj, base); next != 7 {
		t.Fatalf("第二次应当继续往后（7），got %d", next)
	}
}

// 没有任何旧转录时，第一轮重核从 try2 开始（try1 是无后缀那条，属于第一次转换）。
func TestFreshCheckerAttemptStartsAtTwo(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, "work", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := newRoundTestRunner(t)
	if n := r.freshCheckerAttemptNo(proj, "chapter_010"); n != 2 {
		t.Fatalf("全新一章应当从 try2 开始，got %d", n)
	}
}
