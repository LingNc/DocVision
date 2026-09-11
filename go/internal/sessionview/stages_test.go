package sessionview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 侧栏第二层的来源：项目 → 流程阶段 → 会话，外加书的进展与"这个逐图会话画的
// 是哪张图"。这里用一份真实的目录结构（progress.json + doc_index + 转录文件名）
// 把 enrichSessions 钉住。
func TestEnrichSessionsAddsStageProgressAndImageIdentity(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "latex_project", "book")
	mustWrite(t, filepath.Join(proj, "work", "sessions", "convert_chapter_001.jsonl"), `{"t":"msg","message":{"role":"user","content":"x"}}`+"\n")
	mustWrite(t, filepath.Join(proj, "work", "sessions", "checker_chapter_001.jsonl"), `{"t":"msg","message":{"role":"user","content":"x"}}`+"\n")
	mustWrite(t, filepath.Join(proj, "work", "style_session.jsonl"), `{"t":"msg","message":{"role":"user","content":"x"}}`+"\n")
	mustWrite(t, filepath.Join(proj, "source", "sessions", "vector_book__abc123__Venn_diagram.jsonl"), `{"t":"msg","message":{"role":"user","content":"x"}}`+"\n")
	mustWrite(t, filepath.Join(proj, "work", "sessions", "figure_check_book__abc123__Venn_diagram.jsonl"), `{"t":"msg","message":{"role":"user","content":"x"}}`+"\n")
	// progress.json：档位级状态
	mustWrite(t, filepath.Join(proj, "progress.json"), `{"style":"done","images":"done","chapters":"running","convert":"","assemble":""}`+"\n")
	// doc_index：图片块带 img/page/type/text
	idx := map[string]any{"entries": []map[string]any{
		{"seq": 1, "page": 3, "type": "text", "text": "正文块"},
		{"seq": 2, "page": 7, "type": "image", "img": "images/book/abc123.jpg", "text": "Venn diagram of two sets"},
		{"seq": 3, "page": 7, "type": "image", "img": "images/book/def456.jpg", "text": ""},
	}}
	data, _ := json.Marshal(idx)
	mustWrite(t, filepath.Join(proj, "doc_index", "doc_index.json"), string(data)+"\n")

	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]SessionInfo{}
	for _, s := range sessions {
		byLabel[s.Label] = s
	}
	vec, ok := byLabel["vector:Venn diagram"]
	if !ok {
		t.Fatalf("矢量图会话没被识别: %+v", byLabel)
	}
	if vec.Stage != "vector" || vec.StageTitle != "矢量图" {
		t.Fatalf("阶段分组字段缺失: stage=%q title=%q", vec.Stage, vec.StageTitle)
	}
	if vec.ImageName != "abc123" || vec.Page != 7 || vec.ImageOrder != 1 || vec.ImageType != "image" {
		t.Fatalf("逐图会话没有按图片名/页/顺序/类型定位: %+v", vec)
	}
	if vec.ImageCaption != "Venn diagram of two sets" {
		t.Fatalf("图注没带出来: %q", vec.ImageCaption)
	}
	if got := vec.ProjectStages["style"]; got != "done" {
		t.Fatalf("progress.json 没进侧栏: %+v", vec.ProjectStages)
	}
	// 逐图校验会话同样带图片身份（转录名与矢量图同形），阶段是"逐图校验"。
	fc, ok := byLabel["figure-check:book__abc123__Venn_diagram"]
	if !ok {
		t.Fatalf("逐图校验会话没被识别: %+v", byLabel)
	}
	if fc.Stage != "figure-check" || fc.StageTitle != "逐图校验" {
		t.Fatalf("逐图校验阶段字段不对: stage=%q title=%q", fc.Stage, fc.StageTitle)
	}
	if fc.ImageName != "abc123" || fc.Page != 7 || fc.ImageOrder != 1 {
		t.Fatalf("逐图校验会话也该定位到图片: %+v", fc)
	}
	// 非逐图会话也有阶段与进展，但没有图片身份。
	conv := byLabel["convert:chapter_001"]
	if conv.Stage != "convert" || conv.ImageName != "" {
		t.Fatalf("转换会话的阶段/图片字段不对: %+v", conv)
	}
	if conv.ProjectStages["chapters"] != "running" {
		t.Fatalf("转换会话也带项目进展（书级信息对每个会话都一样）: %+v", conv.ProjectStages)
	}
	// 阶段分组按流程顺序、只统计本项目。
	sum := SummarizeStages(sessions, conv.Project)
	order := []string{}
	for _, r := range sum {
		order = append(order, r.Stage)
	}
	want := []string{"vector", "style", "convert", "checker", "figure-check"}
	if len(order) != len(want) {
		t.Fatalf("阶段分组 = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("阶段必须按流程顺序: got %v want %v", order, want)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// 转录文件名是 vector_<书>__<图片名>__<标签>，图片名取倒数第二段（标签里可能
// 有下划线，不能按 "_" 切）。
func TestVectorImageBaseParsing(t *testing.T) {
	cases := map[string]string{
		"p/source/sessions/vector_book__abc123__Venn_diagram.jsonl":     "abc123",
		"p/source/sessions/vector_测试-概率论__deadbeef__concept_map.jsonl":  "deadbeef",
		"p/source/sessions/vector_book__abc123.jsonl":                   "",
		"p/work/sessions/figure_check_book__abc123__Venn_diagram.jsonl": "abc123",
		"p/work/style_session.jsonl":                                    "",
	}
	for in, want := range cases {
		if got := vectorImageBase(in); got != want {
			t.Errorf("vectorImageBase(%q) = %q, want %q", in, got, want)
		}
	}
}
