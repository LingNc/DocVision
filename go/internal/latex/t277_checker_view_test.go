package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T277：核对视图必须**自包含**。产物在细分父章节里落在 `integrated-stage/`
// （下一轮开始就 RemoveAll），而视图活得比它久：软链一断，会话再读
// `check:<章>.tex` / `check:<章>/…` 全是"文件不存在"，错误回执里还列着那些名字
// （readdir 看得见断链），模型只能反复重试到退化。
func TestCheckerViewIsSelfContained(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_010"
	stage := filepath.Join(proj, "work", "temp", "conv_"+base, "integrated-stage")
	if err := os.MkdirAll(filepath.Join(stage, base, "children"), 0o755); err != nil {
		t.Fatal(err)
	}
	texPath := filepath.Join(stage, base+".tex")
	if err := os.WriteFile(texPath, []byte("\\input{"+base+"/children/c1.tex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, base, "children", "c1.tex"), []byte("child body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chapPath := filepath.Join(proj, "chapters", base+".md")
	if err := os.MkdirAll(filepath.Dir(chapPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chapPath, []byte("# chapter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	view := ensureCheckerViewAt(proj, filepath.Join(proj, "work", "views", "checker_"+base), base,
		chapPath, texPath, filepath.Join(stage, base))
	if view == "" {
		t.Fatal("视图没建出来")
	}

	// 下一轮开始：暂存目录整个清掉。
	if err := os.RemoveAll(stage); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		base + ".tex",
		filepath.Join(base, "children", "c1.tex"),
		filepath.Join("parts", "children", "c1.tex"),
	} {
		if data, err := os.ReadFile(filepath.Join(view, rel)); err != nil || len(data) == 0 {
			t.Fatalf("视图条目 %s 读不到（%v）——它还在指着一个已经被清掉的暂存目录", rel, err)
		}
	}
}

// T277：没挂产物页的那几轮，提示词不许再提 "this PDF"——模型会照着去找
// `check:<章>.pdf`，拿一串"文件不存在"把自己拖到退化。
func TestCheckerClaimNoteMentionsPDFOnlyWhenMounted(t *testing.T) {
	view := t.TempDir()
	if err := os.WriteFile(filepath.Join(view, "notes.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	note := checkerClaimNote(view, "chapter_010")
	if note == "" || !strings.Contains(note, "notes.md") {
		t.Fatalf("应当点名 notes.md，got %q", note)
	}
	if strings.Contains(note, "this PDF") {
		t.Fatalf("没挂 PDF 时不该提 this PDF：%q", note)
	}
	if err := os.WriteFile(filepath.Join(view, "chapter_010.pdf"), []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if note := checkerClaimNote(view, "chapter_010"); !strings.Contains(note, "this PDF") {
		t.Fatalf("挂了 PDF 时应当提 this PDF：%q", note)
	}
}

// T277：按当前产物重编产物页时，没有产物要如实报错（调用方退回"没有产物页"的
// 核对），而不是给出一个别的章节/上一轮的 PDF 去误导核对会话。
func TestCompileChapterForCheckNeedsProduct(t *testing.T) {
	r := newRoundTestRunner(t)
	if _, err := r.compileChapterForCheck(t.TempDir(), "chapter_010"); err == nil {
		t.Fatal("没有产物时应当报错")
	}
}
