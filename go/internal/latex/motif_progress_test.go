package latex

import (
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/logger"
)

func TestMotifProgressVisibleInQuietMode(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(filepath.Join(dir, "run.log"), filepath.Join(dir, "error.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.SetQuiet(true)
	out := captureStdout(t, func() {
		p := newMotifProgress(log, 388)
		p.scan(388, 388)
		p.verify(0, 1, 336, 0)
		p.verify(1, 1, 336, 1)
		p.finish()
	})
	for _, want := range []string{"[dedupe] 扫描候选 0/388", "[dedupe 0/1] 核验整组 336 张", "[dedupe 1/1] done (failed: 1)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in quiet output %q", want, out)
		}
	}
}
