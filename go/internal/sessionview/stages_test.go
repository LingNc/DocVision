package sessionview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	fc, ok := byLabel["figure-check:Venn diagram"]
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

// 逐图会话的**标题**：有图注就用图注，没有就用短写文件名（前 8 位 + 原扩展名），
// 64 位内容哈希绝不当标题；完整文件名 / 哈希 / doc_index 路径留在字段与悬浮说明里。
func TestImageSessionTitlesUseShortFileNames(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "latex_project", "book")
	// 两张图：一张有图注，一张没有；名字都是 MinerU 的 64 位内容哈希。
	const hashA = "bfeafce8653de44b45033ad467609c389911684a0fbcb95915a0b13d3ca94024"
	const hashB = "220c0aa621bd333ec6fbd0a2047f4baa7420d912697481deb97319f5d19c44db"
	mustWrite(t, filepath.Join(proj, "source", "sessions", "vector_book__"+hashA+"__$A\\subset B.jsonl"),
		`{"t":"msg","message":{"role":"user","content":"x"}}`+"\n")
	mustWrite(t, filepath.Join(proj, "source", "sessions", "vector_book__"+hashB+"__"+hashB+".jsonl"),
		`{"t":"msg","message":{"role":"user","content":"x"}}`+"\n")
	idx := map[string]any{"entries": []map[string]any{
		{"seq": 1, "page": 4, "type": "image", "img": "images/book/" + hashB + ".jpg", "text": ""},
		{"seq": 2, "page": 9, "type": "image", "img": "images/book/" + hashA + ".jpg", "text": "$A\\subset B"},
	}}
	data, _ := json.Marshal(idx)
	mustWrite(t, filepath.Join(proj, "doc_index", "doc_index.json"), string(data)+"\n")

	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]SessionInfo{}
	for _, s := range sessions {
		byName[s.ImageName] = s
	}

	captioned, ok := byName[hashA]
	if !ok {
		t.Fatalf("有图注的逐图会话没被扫到: %+v", sessions)
	}
	if captioned.Title != "矢量图 · $A\\subset B" {
		t.Errorf("有图注时标题应当用图注，得到 %q", captioned.Title)
	}
	if captioned.ImageShort != "bfeafce8.jpg" || captioned.ImageFile != hashA+".jpg" || captioned.ImagePath != "images/book/"+hashA+".jpg" {
		t.Errorf("短名/全名/路径不对: short=%q file=%q path=%q", captioned.ImageShort, captioned.ImageFile, captioned.ImagePath)
	}

	// 没有图注（doc_index 里 text 为空）→ 标题是短写文件名，绝不是 64 位哈希。
	plain, ok := byName[hashB]
	if !ok {
		t.Fatalf("无图注的逐图会话没被扫到: %+v", sessions)
	}
	if plain.ImageCaption != "" {
		t.Fatalf("这张图不该有图注: %q", plain.ImageCaption)
	}
	if plain.Title != "矢量图 · 220c0aa6.jpg" {
		t.Errorf("无图注时标题应当是短写文件名，得到 %q", plain.Title)
	}
	if strings.Contains(plain.Title, hashB) {
		t.Errorf("标题里又出现了 64 位哈希: %q", plain.Title)
	}
	if plain.ImageShort != "220c0aa6.jpg" || plain.ImageFile != hashB+".jpg" {
		t.Errorf("短名/全名不对: short=%q file=%q", plain.ImageShort, plain.ImageFile)
	}
	// 完整名字没丢：哈希与路径都还在（页面把它们放进行的悬浮说明）。
	if plain.ImageName != hashB || plain.ImagePath != "images/book/"+hashB+".jpg" {
		t.Errorf("完整哈希或路径丢了: name=%q path=%q", plain.ImageName, plain.ImagePath)
	}
}

// 同一本书里短名撞车时加长到 12 位；真不行就保留全名——两行绝不显示同一个短名。
func TestImageShortNamesAreUniqueInsideABook(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "latex_project", "book")
	// 前 8 位相同、第 9 位起不同（真实哈希撞 8 位前缀的形态）。
	const a = "abcdef12345600000000000000000000000000000000000000000000000000a1"
	const b = "abcdef12999900000000000000000000000000000000000000000000000000b2"
	const c = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc3"
	for _, h := range []string{a, b, c} {
		mustWrite(t, filepath.Join(proj, "source", "sessions", "vector_book__"+h+"__"+h+".jsonl"),
			`{"t":"msg","message":{"role":"user","content":"x"}}`+"\n")
	}
	mustWrite(t, filepath.Join(proj, "doc_index", "doc_index.json"), `{"entries":[
		{"seq":1,"page":1,"type":"image","img":"images/book/`+a+`.jpg","text":""},
		{"seq":2,"page":2,"type":"image","img":"images/book/`+b+`.jpg","text":""},
		{"seq":3,"page":3,"type":"image","img":"images/book/`+c+`.jpg","text":""}]}`+"\n")

	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range sessions {
		got[s.ImageName] = s.ImageShort
	}
	if got[a] != "abcdef123456.jpg" || got[b] != "abcdef129999.jpg" {
		t.Errorf("前 8 位撞车时应当退化到 12 位: a=%q b=%q", got[a], got[b])
	}
	if got[a] == got[b] {
		t.Errorf("撞车的两行短名仍然相同: %q", got[a])
	}
	if got[c] != "ffffffff.jpg" {
		t.Errorf("没撞车的图仍该用 8 位: c=%q", got[c])
	}
	if got[a] == got[b] {
		t.Errorf("同一本书里出现了两个相同的短名: %q", got[a])
	}
}

// 短名只动内容哈希：可读的名字（MinerU 偶尔保留原词）原样保留。
func TestShortImageNameKeepsReadableNames(t *testing.T) {
	if got := shortImageName("venn_diagram", ".png", 8); got != "venn_diagram.png" {
		t.Errorf("可读名字被截断了: %q", got)
	}
	if got := shortImageName(strings.Repeat("a", 64), ".jpg", 8); got != "aaaaaaaa.jpg" {
		t.Errorf("哈希没短写: %q", got)
	}
	if got := shortImageName("abc123", ".jpg", 8); got != "abc123.jpg" {
		t.Errorf("短名字不该被动: %q", got)
	}
}

// 连 12 位都撞（前 12 位完全相同）时保留全名——短名只是显示层的东西，
// 唯一性优先：宁可长一点，也不能让两行显示成同一张图。
func TestShortImageNameFallsBackToFullName(t *testing.T) {
	long := strings.Repeat("a", 60)
	sessions := []SessionInfo{
		{ImageName: long + "1", ImageFile: long + "1.jpg"},
		{ImageName: long + "2", ImageFile: long + "2.jpg"},
	}
	assignShortImageNames(sessions, []int{0, 1})
	if sessions[0].ImageShort != long+"1.jpg" || sessions[1].ImageShort != long+"2.jpg" {
		t.Errorf("撞到 12 位还没区分开时应当保留全名: %q / %q", sessions[0].ImageShort, sessions[1].ImageShort)
	}
	if sessions[0].ImageShort == sessions[1].ImageShort {
		t.Error("兜底后两行的短名仍然相同")
	}
}
