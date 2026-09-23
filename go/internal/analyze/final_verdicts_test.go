package analyze

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// T75：最终一轮口径——旧日志报错、新日志修好的图不算错误；
// 新日志仍报错的图保留；警告同理。
func TestFinalImageVerdicts(t *testing.T) {
	dir := t.TempDir()
	oldLog := filepath.Join(dir, "img2text_20260101_000001.log")
	newLog := filepath.Join(dir, "img2text_20260102_000000.log")
	os.WriteFile(oldLog, []byte("[1/3] images/a.jpg\n[ERROR] invalid\n[2/3] images/b.jpg\n[WARNING] self-corrected\n[3/3] images/c.jpg\n"), 0o644)
	os.WriteFile(newLog, []byte("[1/1] images/a.jpg\nok\n"), 0o644)
	os.Chtimes(oldLog, time.Unix(1000, 0), time.Unix(1000, 0))
	os.Chtimes(newLog, time.Unix(2000, 0), time.Unix(2000, 0))

	v := FinalImageVerdicts([]string{oldLog, newLog})
	if v["images/a.jpg"].Err {
		t.Fatal("a.jpg 在新日志已修好，不应再算错误")
	}
	if !v["images/b.jpg"].Warn {
		t.Fatal("b.jpg 只在旧日志出现过，警告应保留")
	}
	if v["images/c.jpg"].Err || v["images/c.jpg"].Warn {
		t.Fatal("c.jpg 干净")
	}
	// 同文件内多次出现：取最后一次
	again := filepath.Join(dir, "img2text_20260103_000000.log")
	os.WriteFile(again, []byte("[1/2] images/d.jpg\n[ERROR] bad\n[2/2] images/d.jpg\nok\n"), 0o644)
	os.Chtimes(again, time.Unix(3000, 0), time.Unix(3000, 0))
	v2 := FinalImageVerdicts([]string{oldLog, newLog, again})
	if v2["images/d.jpg"].Err || v2["images/d.jpg"].Warn {
		t.Fatal("d.jpg 最后一次出现干净，终态应干净")
	}
}
