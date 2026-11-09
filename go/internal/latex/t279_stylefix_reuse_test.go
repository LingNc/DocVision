package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T279：工作汇报内容没变时重入风格修复循环，不该再把整合会话从第 1 轮烧一遍。
func TestStyleFixReportsSHAStable(t *testing.T) {
	a := map[string]string{"chapter_001": "报告甲\n- 结论: 存在问题\n", "chapter_002": "报告乙\n"}
	b := map[string]string{"chapter_002": "报告乙\n", "chapter_001": "报告甲\n- 结论: 存在问题\n"}
	if styleFixReportsSHA(a) != styleFixReportsSHA(b) {
		t.Fatal("同一批报告（顺序不同）应当得到同一个指纹")
	}
	b["chapter_002"] = "报告乙（改过）\n"
	if styleFixReportsSHA(a) == styleFixReportsSHA(b) {
		t.Fatal("报告内容变了，指纹必须变——否则会沿用已经过期的清单")
	}
	if styleFixReportsSHA(nil) == styleFixReportsSHA(a) {
		t.Fatal("多一份/少一份报告也必须让指纹变化")
	}
}

// 指纹对得上才沿用；沿用只清打勾，不动问题定义。
func TestReuseStyleProblemsOnlyWhenReportsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "style_fix", "problems.json")
	store := loadProblemStore(path)
	store.Problems = []StyleProblem{
		{ID: "p1", Title: "标题栏行距", Checked: true},
		{ID: "p2", Title: "定理环境间距"},
	}
	store.ReportsSHA = "sha-old"
	if err := store.saveLocked(); err != nil {
		t.Fatal(err)
	}

	// 落盘再读回：指纹与勾选状态都得还在（不然永远判定"过期"，每次都重跑整合）。
	back := loadProblemStore(path)
	if back.ReportsSHA != "sha-old" || len(back.Problems) != 2 || !back.Problems[0].Checked {
		t.Fatalf("problems.json 往返丢了东西: %+v", back)
	}

	if _, ok := reuseStyleProblems(back, "sha-new"); ok {
		t.Fatal("指纹不同时不能沿用（报告已经变了）")
	}
	got, ok := reuseStyleProblems(back, "sha-old")
	if !ok || len(got) != 2 {
		t.Fatalf("指纹一致时应当沿用: ok=%v n=%d", ok, len(got))
	}
	for _, p := range got {
		if p.Checked {
			t.Fatalf("沿用时要清掉上一轮的打勾（%s 还是 checked），否则本轮会被判成已解决", p.ID)
		}
	}
	// 清打勾是**故意**写回存储的：下一轮要按"全部未解决"重新派活。
	if back.Problems[0].Checked {
		t.Fatal("沿用后应当把清空后的勾选状态写回存储")
	}
	// 返回的是副本：调用方改它不该动到存储。
	got[0].Title = "改过的标题"
	if back.Problems[0].Title != "标题栏行距" {
		t.Fatal("返回的清单必须是副本")
	}
	if _, ok := reuseStyleProblems(loadProblemStore(filepath.Join(t.TempDir(), "none.json")), "sha-old"); ok {
		t.Fatal("没有清单时不能沿用")
	}
}

// 整合失败不能留下"可用指纹"：否则下一轮会拿着一份空清单跳过整合。
func TestProblemStoreFingerprintWrittenOnlyOnSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "problems.json")
	store := loadProblemStore(path)
	store.ReportsSHA = ""
	if err := store.saveLocked(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || len(data) == 0 {
		t.Fatalf("落盘失败: %v", err)
	}
	if back := loadProblemStore(path); back.ReportsSHA != "" || len(back.Problems) != 0 {
		t.Fatal("空清单不该带指纹")
	}
}
