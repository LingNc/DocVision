package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fcFixture = `/usr/share/fonts/opentype/noto/NotoSerifCJK-Regular.ttc: Noto Serif CJK SC
/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc: Noto Sans CJK SC,Noto Sans CJK SC Bold
/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc: Noto Sans CJK TC
/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf: Droid Sans Fallback
`

// T291：README.md 不是字体，不能混进"可用字体"里。
func TestProjectFontsSkipNonFontFiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"README.md", "SourceHan.otf", "sub/ignored.ttf"} {
		p := filepath.Join(dir, f)
		if strings.Contains(f, "/") {
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			continue
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, others := projectFontFiles(dir)
	if len(files) != 1 || files[0] != "SourceHan.otf" {
		t.Fatalf("项目字体目录只该认出 SourceHan.otf: %v", files)
	}
	if len(others) != 1 || others[0] != "README.md" {
		t.Fatalf("README.md 应当被归到非字体文件: %v", others)
	}
	// 只有说明文件时要说清"没有字体文件"，但也要提到那个说明文件。
	reply := formatFontReply(nil, []string{"README.md"}, "", nil)
	if !strings.Contains(reply, "NO font files") || !strings.Contains(reply, "README.md") {
		t.Fatalf("没有字体文件时的回执不对: %q", reply)
	}
}

// T291：系统字体以前 `Run()` 之后又 `Output()`，第二次调用必然失败，回执里
// 只剩一个空的 "System fonts (fc-list):" 标题——用户看到的就是这个。
func TestFormatFontReplyListsSystemFamilies(t *testing.T) {
	reply := formatFontReply(nil, nil, fcFixture, nil)
	for _, want := range []string{"System CJK font families (4", "Noto Serif CJK SC", "Noto Sans CJK SC", "Noto Sans CJK TC", "Droid Sans Fallback", "do not run fc-list there"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("回执里缺 %q:\n%s", want, reply)
		}
	}
	// 字重变体不占一行：`Noto Sans CJK SC Bold` 归并到 `Noto Sans CJK SC`。
	if strings.Contains(reply, "Noto Sans CJK SC Bold") {
		t.Fatalf("字重名不该单列:\n%s", reply)
	}
	// 简体排在最前面（模型最常用）。
	sc := strings.Index(reply, "Noto Sans CJK SC")
	if sc < 0 || sc > strings.Index(reply, "Droid Sans Fallback") {
		t.Fatalf("SC 家族应当排在其它之前:\n%s", reply)
	}
}

// fc-list 不可用（容器里没装 fontconfig）时要如实说，而不是留个空标题。
func TestFormatFontReplyWithoutFcList(t *testing.T) {
	reply := formatFontReply(nil, nil, "", os.ErrNotExist)
	if !strings.Contains(reply, "No CJK-capable system fonts found") {
		t.Fatalf("fc-list 缺失时的回执不对: %q", reply)
	}
}
