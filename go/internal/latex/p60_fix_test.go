package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// p60Fixture 建一棵最小的细分树：<proj>/work/temp/conv_<根章>/work/_subdivision/child_NN。
// 计划文件与子章节名校验都对着它做（子章节名的唯一真源就是这些真实目录）。
func p60Fixture(t *testing.T, rootChapter string, children ...string) (proj, subdiv string) {
	t.Helper()
	proj = t.TempDir()
	subdiv = filepath.Join(proj, "work", "temp", "conv_"+rootChapter, "work", "_subdivision")
	for _, child := range children {
		if err := os.MkdirAll(filepath.Join(subdiv, child), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return proj, subdiv
}

// P60：计划文件的往返——写出的路径、schema 版本、分组、时间戳都要能原样读回。
func TestFixPlanRoundTrip(t *testing.T) {
	proj, _ := p60Fixture(t, "chapter_014", "child_01", "child_03")
	plan := fixPlan{
		Root: "chapter_014",
		Children: []fixPlanChild{
			{Child: "child_01", Issues: []fixPlanIssue{
				{ID: "ISSUE-1", What: "表格第二行缺一列", Rule: "与原书表格逐格一致", Where: "chapter_014_child_01.tex:120"},
				{ID: "ISSUE-2", What: "公式被截断", Rule: "公式完整可编译", Where: "parts/part2.tex:8"},
			}},
			{Child: "child_03", Issues: []fixPlanIssue{
				{ID: "figure-scale", What: "插图过大", Rule: "不超出正文宽度", Where: "chapter_014_child_03.tex:44"},
			}},
		},
	}
	path, err := WriteFixPlan(proj, "chapter_014", plan)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(proj, "work", "fix", "chapter_014", "plan.json"); path != want {
		t.Fatalf("plan path = %q, want %q", path, want)
	}
	if path != fixPlanPath(proj, "chapter_014") {
		t.Fatalf("path helper 与返回值不一致")
	}
	got, err := ReadFixPlan(proj, "chapter_014")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || fixPlanVersion != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if got.Root != "chapter_014" {
		t.Errorf("root = %q", got.Root)
	}
	if got.Created == "" {
		t.Error("created 应当被填上")
	}
	if len(got.Children) != 2 || got.Children[0].Child != "child_01" || got.Children[1].Child != "child_03" {
		t.Fatalf("children = %+v", got.Children)
	}
	if len(got.Children[0].Issues) != 2 || got.Children[0].Issues[1].ID != "ISSUE-2" {
		t.Fatalf("child_01 issues = %+v", got.Children[0].Issues)
	}
	if got.Children[0].Issues[0].Where != "chapter_014_child_01.tex:120" {
		t.Errorf("where 没有原样保留: %q", got.Children[0].Issues[0].Where)
	}
	// 落盘的就是 v1 形状（给人看/手改的文件，键名不能漂）。
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatalf("计划文件不是合法 JSON: %v", err)
	}
	for _, key := range []string{"version", "created", "root", "children"} {
		if _, ok := shape[key]; !ok {
			t.Errorf("计划文件缺键 %q", key)
		}
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Error("计划文件应当以换行结尾")
	}
}

// P60：子章节名必须是细分树里的真实目录名（child_NN），自由文本/路径一律拒。
func TestWriteFixPlanRejectsInvalidChild(t *testing.T) {
	proj, _ := p60Fixture(t, "chapter_007")
	for _, child := range []string{"", "child_1", "child_001", "child_02/../../etc", "child_02x", "../child_02", "child"} {
		plan := fixPlan{Root: "chapter_007", Children: []fixPlanChild{{
			Child:  child,
			Issues: []fixPlanIssue{{ID: "a", What: "w", Rule: "r", Where: "x.tex:1"}},
		}}}
		if _, err := WriteFixPlan(proj, "chapter_007", plan); err == nil {
			t.Errorf("child %q 应当被拒", child)
		}
	}
	if _, err := os.Stat(fixPlanPath(proj, "chapter_007")); !os.IsNotExist(err) {
		t.Errorf("被拒的写入不该落盘: %v", err)
	}
}

// P60：不认识的 schema 版本直接报错——宁可停下，也不按猜测执行。
func TestReadFixPlanRejectsUnknownVersion(t *testing.T) {
	proj, _ := p60Fixture(t, "chapter_007")
	dir := fixPlanDir(proj, "chapter_007")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"version":2,"created":"2026-10-23T00:00:00Z","root":"chapter_007","children":[{"child":"child_01","issues":[{"id":"a","what":"w","rule":"r","where":"x:1"}]}]}`
	if err := os.WriteFile(fixPlanPath(proj, "chapter_007"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFixPlan(proj, "chapter_007"); err == nil || !strings.Contains(err.Error(), "版本") {
		t.Fatalf("未知版本应当被拒: %v", err)
	}
}

// P60：字段非空、id 不重复——计划执行时"哪条改了"要能回答。
func TestWriteFixPlanRejectsIncompleteIssue(t *testing.T) {
	proj, _ := p60Fixture(t, "chapter_007")
	good := fixPlanIssue{ID: "a", What: "w", Rule: "r", Where: "x:1"}
	cases := map[string][]fixPlanIssue{
		"缺 id":    {{ID: " ", What: "w", Rule: "r", Where: "x:1"}},
		"缺 what":  {{ID: "a", What: "", Rule: "r", Where: "x:1"}},
		"缺 rule":  {{ID: "a", What: "w", Rule: " ", Where: "x:1"}},
		"缺 where": {{ID: "a", What: "w", Rule: "r"}},
		"重复 id":   {good, good},
	}
	for name, issues := range cases {
		plan := fixPlan{Root: "chapter_007", Children: []fixPlanChild{{Child: "child_01", Issues: issues}}}
		if _, err := WriteFixPlan(proj, "chapter_007", plan); err == nil {
			t.Errorf("%s: 应当被拒", name)
		}
	}
}

// P60：dispatch_fix 的校验——参数、子章节名、字段、重复 id、没细分过。
// 校验失败一律 REJECTED 且**不写任何文件**。
func TestDispatchFixValidation(t *testing.T) {
	proj, _ := p60Fixture(t, "chapter_014", "child_01")
	okIssue := `{"id":"a","child":"child_01","where":"x.tex:1","what":"w","rule":"r"}`
	cases := []struct {
		name string
		args string
		want string
	}{
		{"空 issues", `{"issues":[]}`, "issues is empty"},
		{"缺参数", `{}`, "issues is empty"},
		{"坏 JSON", `not-json`, ""},
		{"非 child_NN", `{"issues":[{"id":"a","child":"part_1","where":"x","what":"w","rule":"r"}]}`, "子章节名不合法"},
		{"路径式 child", `{"issues":[{"id":"a","child":"child_01/../../x","where":"x","what":"w","rule":"r"}]}`, "子章节名不合法"},
		{"不存在的 child", `{"issues":[{"id":"a","child":"child_09","where":"x","what":"w","rule":"r"}]}`, "does not exist"},
		{"缺 id", `{"issues":[{"child":"child_01","where":"x","what":"w","rule":"r"}]}`, "id is empty"},
		{"缺 where", `{"issues":[{"id":"a","child":"child_01","what":"w","rule":"r"}]}`, "where is empty"},
		{"缺 what", `{"issues":[{"id":"a","child":"child_01","where":"x","rule":"r"}]}`, "what is empty"},
		{"缺 rule", `{"issues":[{"id":"a","child":"child_01","where":"x","what":"w"}]}`, "rule is empty"},
		{"重复 id", `{"issues":[` + okIssue + `,` + okIssue + `]}`, "id"},
	}
	for _, c := range cases {
		tool := &DispatchFixTool{Proj: proj, Root: "chapter_014"}
		res, err := tool.Execute(c.args)
		if c.want == "" {
			if err == nil {
				t.Errorf("%s: 应当返回解析错误", c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: 不该以 error 返回: %v", c.name, err)
			continue
		}
		if !strings.HasPrefix(res.Text, "REJECTED") || !strings.Contains(res.Text, c.want) {
			t.Errorf("%s: 回执 = %q，期望包含 %q", c.name, res.Text, c.want)
		}
	}
	if _, err := os.Stat(fixPlanPath(proj, "chapter_014")); !os.IsNotExist(err) {
		t.Errorf("校验失败不该写出计划文件: %v", err)
	}
}

// P60：没细分过的章节不能下派（没有 child_NN 就没有修复对象）。
func TestDispatchFixWithoutSubdivision(t *testing.T) {
	proj := t.TempDir()
	tool := &DispatchFixTool{Proj: proj, Root: "chapter_014"}
	res, err := tool.Execute(`{"issues":[{"id":"a","child":"child_01","where":"x","what":"w","rule":"r"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "not been subdivided") {
		t.Errorf("回执 = %q", res.Text)
	}
}

// P60：正常下派——按子章节分组、写计划、回执里有计划路径与子章节数。
func TestDispatchFixWritesPlan(t *testing.T) {
	proj, _ := p60Fixture(t, "chapter_014", "child_01", "child_03")
	tool := &DispatchFixTool{Proj: proj, Root: "chapter_014",
		Work: filepath.Join(proj, "work", "temp", "conv_chapter_014", "work")}
	args := `{"issues":[
		{"id":"ISSUE-1","child":"child_03","where":"a.tex:3","what":"缺一行","rule":"与原书一致"},
		{"id":"ISSUE-2","child":"child_01","where":"b.tex:9","what":"公式截断","rule":"完整可编译"},
		{"id":"ISSUE-3","child":"child_01","where":"b.tex:20","what":"表格错列","rule":"逐格一致"}
	],"note":"先确保 cls 里的 note 环境可用"}`
	res, err := tool.Execute(args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "FIX PLAN RECORDED") {
		t.Fatalf("回执 = %q", res.Text)
	}
	path := fixPlanPath(proj, "chapter_014")
	for _, want := range []string{path, "3 issue(s)", "2 child chapter(s)", "child_01 (2)", "child_03 (1)", "note 环境"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("回执缺 %q: %s", want, res.Text)
		}
	}
	plan, err := ReadFixPlan(proj, "chapter_014")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Root != "chapter_014" || len(plan.Children) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	// 子章节按名字排序，issue 保持父会话给的顺序。
	if plan.Children[0].Child != "child_01" || plan.Children[1].Child != "child_03" {
		t.Fatalf("children 顺序 = %+v", plan.Children)
	}
	if got := plan.Children[0].Issues; len(got) != 2 || got[0].ID != "ISSUE-2" || got[1].ID != "ISSUE-3" {
		t.Fatalf("child_01 issues 顺序 = %+v", got)
	}
	if plan.Children[1].Issues[0].Rule != "与原书一致" {
		t.Errorf("rule 没有原样保留: %+v", plan.Children[1].Issues[0])
	}
}

// P60：显式 enabled: false ——修复会话的第一道守卫就是开关，没开就一个字都不动。
func TestRunChildFixDisabledByDefault(t *testing.T) {
	r := keepTestRunner(t)
	off := false
	r.cfg.Latex.Fix = config.FixConfig{Enabled: &off}
	if r.cfg.Latex.FixEnabled() {
		t.Fatal("latex.fix.enabled 显式关闭后不该生效")
	}
	err := r.runChildFix("/nonexistent", "cls", fixTarget{
		Child: "child_02", Base: "chapter_014_child_02", NodeRoot: "/nonexistent/node", Depth: 1,
	}, []fixPlanIssue{{ID: "a", What: "w", Rule: "r", Where: "x:1"}}, 0)
	if err == nil || !strings.Contains(err.Error(), "latex.fix.enabled") {
		t.Fatalf("关闭时应当直接返回未启用错误: %v", err)
	}
}

// P60：轮数预算——转录里的每条 fix_target 就是一轮；用完退回父章节重试。
func TestRunChildFixBudgetExhausted(t *testing.T) {
	on := true
	r := keepTestRunner(t)
	r.cfg.Latex.Fix = config.FixConfig{Enabled: &on, MaxRounds: 2}
	nodeRoot := t.TempDir()
	work := filepath.Join(nodeRoot, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(nodeRoot, "input", "chapter.md")
	if err := os.MkdirAll(filepath.Dir(input), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, []byte("# 子章节\n正文。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	trPath := filepath.Join(nodeRoot, "sessions", "fix_chapter_014_child_02.jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	body.WriteString(`{"t":"msg","role":"user","text":"修复"}` + "\n")
	for i := 0; i < 2; i++ {
		body.WriteString(`{"t":"meta","kind":"fix_target","convert":"convert_chapter_014_child_02","issue_ids":["a"]}` + "\n")
	}
	if err := os.WriteFile(trPath, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := fixRoundsUsed(trPath); got != 2 {
		t.Fatalf("fixRoundsUsed = %d, want 2", got)
	}
	err := r.runChildFix(t.TempDir(), "cls", fixTarget{
		Child: "child_02", Base: "chapter_014_child_02", NodeRoot: nodeRoot, InputMD: input, Depth: 1,
	}, []fixPlanIssue{{ID: "a", What: "w", Rule: "r", Where: "x:1"}}, 0)
	if err == nil || !strings.Contains(err.Error(), "轮数已用完") {
		t.Fatalf("预算用完应当拒绝: %v", err)
	}
	// 没有工作区/输入时也不能凭空重建（修复会话只复用已划好的工作区）。
	err = r.runChildFix(t.TempDir(), "cls", fixTarget{
		Child: "child_02", Base: "chapter_014_child_02", NodeRoot: t.TempDir(), Depth: 1,
	}, []fixPlanIssue{{ID: "a", What: "w", Rule: "r", Where: "x:1"}}, 0)
	if err == nil || !strings.Contains(err.Error(), "工作区不存在") {
		t.Fatalf("缺工作区应当拒绝: %v", err)
	}
}

// P60：fix 只作用于递归子章节（顶层章节没有"父会话下派"这回事）。
func TestRunChildFixRejectsTopLevel(t *testing.T) {
	on := true
	r := keepTestRunner(t)
	r.cfg.Latex.Fix = config.FixConfig{Enabled: &on}
	err := r.runChildFix("/nonexistent", "cls", fixTarget{
		Child: "child_02", Base: "chapter_014", Depth: 0,
	}, []fixPlanIssue{{ID: "a", What: "w", Rule: "r", Where: "x:1"}}, 0)
	if err == nil || !strings.Contains(err.Error(), "递归子章节") {
		t.Fatalf("顶层调用应当拒绝: %v", err)
	}
}

// P60：修复会话的模型默认继承 convert。
func TestFixSessionModelInheritsConvert(t *testing.T) {
	r := keepTestRunner(t)
	r.cfg.Latex.ConvertModel = "convert-entry"
	if got := r.fixSessionModel(); got != "convert-entry" {
		t.Errorf("空的 fix.model 应当继承 convert: %q", got)
	}
	r.cfg.Latex.Fix.Model = "fix-entry"
	if got := r.fixSessionModel(); got != "fix-entry" {
		t.Errorf("fix.model 应当优先: %q", got)
	}
}
