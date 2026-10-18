package latex

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

// T146：逐对 CSV 要留下"这一对是怎么算出来的"——判定层、画布、搜了几级、
// 判定层上搜到的平移格数。只有分数时无法判断 0.6 是尺寸差异还是平移没搜对。
func TestMotifMetricsPairsTraceColumns(t *testing.T) {
	r := testRunner(t, true)
	outDir := t.TempDir()
	a := canonTestImage(120, 160, 0)
	b := canonTestImage(120, 160, 1)
	pool := []*motifCandidate{
		{task: &task{mdName: "book.md", imgPath: "images/book/a.png"}, image: a},
		{task: &task{mdName: "book.md", imgPath: "images/book/b.png"}, image: b},
	}
	r.writeMotifMetrics(outDir, pool, nil, nil, 0, nil)
	f, err := os.Open(filepath.Join(motifArtifactDir(outDir), "metrics", "pairs.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("两两比较应只有 1 行数据，得到 %d 行（含表头共 %d）", len(rows)-1, len(rows))
	}
	want := []string{"a", "b", "feature_similar", "align", "align_level", "canvas", "levels", "shift_y", "shift_x", "skip", "same_bucket"}
	if len(rows[0]) != len(want) {
		t.Fatalf("表头列数 %d，期望 %d：%v", len(rows[0]), len(want), rows[0])
	}
	for i, name := range want {
		if rows[0][i] != name {
			t.Fatalf("第 %d 列是 %q，期望 %q", i+1, rows[0][i], name)
		}
	}
	row := rows[1]
	if row[3] == "" || row[4] == "0" || row[5] == "0" || row[6] == "0" {
		t.Fatalf("算得出分数的对必须带 align/align_level/canvas/levels：%v", row)
	}
	// 两张图内容框一致、只是符号不同 → 平移量应当就是 0（没有错位可搜）。
	if row[7] != "0" || row[8] != "0" {
		t.Fatalf("内容框对齐的两张图 shift 应为 0，得到 (%s,%s)", row[7], row[8])
	}
	if row[9] != "" {
		t.Fatalf("能算出重合度的对不该带 skip：%q", row[9])
	}
}
