package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T189：空源路径曾经一路走到 os.Open，报出 `open : no such file or directory`——
// 与图无关的错误被记成 fallback，续跑重放时又照样失败，同一张图永远画不出来。
// 现在必须在 copyFile 这层就说出"源路径为空、目标是谁"。
func TestCopyFileRejectsEmptySource(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "fig.pdf")
	err := copyFile("", dst)
	if err == nil {
		t.Fatal("空源路径必须报错")
	}
	if !strings.Contains(err.Error(), "源路径为空") || !strings.Contains(err.Error(), "fig.pdf") {
		t.Fatalf("错误信息要说清空路径与目标，实际: %v", err)
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		t.Fatal("失败时不该留下目标文件")
	}
}
