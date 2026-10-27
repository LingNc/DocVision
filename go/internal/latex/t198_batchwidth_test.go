package latex

import (
	"testing"

	"mineru-tools/internal/config"
)

// T198：转换批次宽度必须跟 latex.convert_concurrency 走。以前它写死
// latex.concurrency——用户把 convert_concurrency 调到 15，启动账目与进度行都说
// "并发 15"，实际只开 5 个 worker（会话槽位池虽是 15，但根本到不了上限；实测
// 峰值转换 6 个会话）。两处必须同源，否则"调了配置没反应"。
func TestConvertBatchWidth(t *testing.T) {
	cases := []struct {
		name          string
		conc, convert int
		want          int
	}{
		{"只写 convert_concurrency", 5, 15, 15},
		{"只写 concurrency", 7, 0, 7},
		{"两者都写，转换优先", 5, 4, 4},
		{"两者都没写按老默认 3", 0, 0, 3},
	}
	for _, c := range cases {
		r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{Concurrency: c.conc, ConvertConcurrency: c.convert}}}
		if got := r.convertBatchWidth(); got != c.want {
			t.Fatalf("%s: convertBatchWidth() = %d, 期望 %d", c.name, got, c.want)
		}
		// 启动账目里的"转换 N"必须与批宽一致，否则又是两处走岔。
		if c.convert > 0 || c.conc > 0 {
			if w := r.cfg.Latex.ConvertWorkers(); w != c.want {
				t.Fatalf("%s: ConvertWorkers() = %d, 与批宽 %d 不一致", c.name, w, c.want)
			}
		}
	}
}
