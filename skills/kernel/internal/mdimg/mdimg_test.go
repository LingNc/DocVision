package mdimg

import (
	"strings"
	"testing"
)

func TestExtractMermaidBlocks(t *testing.T) {
	text := "前文\n\n```mermaid\ngraph TD; A-->B;\n```\n\n后文\n```MERMAID\nsequenceDiagram\n```\n"
	blocks := ExtractMermaidBlocks(text)
	if len(blocks) != 2 || blocks[0] != "graph TD; A-->B;" || !strings.Contains(blocks[1], "sequenceDiagram") {
		t.Fatalf("围栏块抽取: %v", blocks)
	}
	if got := ExtractMermaidBlocks("无块"); len(got) != 0 {
		t.Fatal("无块应为空")
	}
}

func TestAssembleReplaceOrder(t *testing.T) {
	md := "第一段\n\n![图1](images/a.jpg)\n\n中间\n\n![图2](images/b.jpg)\n\n![图3](images/c.jpg)"
	repls := []Replacement{
		{Seq: 1, ImgRef: "images/a.jpg", Text: "提取的题字文字"},
		{Seq: 2, ImgRef: "images/b.jpg", Mermaid: "graph TD; X-->Y;"},
		{Seq: 3, ImgRef: "images/c.jpg", Keep: true},
	}
	out := Assemble(md, repls)
	if !strings.Contains(out, "提取的题字文字") || strings.Contains(out, "images/a.jpg") {
		t.Fatalf("图1应被文字替换:\n%s", out)
	}
	if !strings.Contains(out, "```mermaid\ngraph TD; X-->Y;\n```") || strings.Contains(out, "images/b.jpg") {
		t.Fatalf("图2应被 mermaid 替换:\n%s", out)
	}
	if !strings.Contains(out, "![图3](images/c.jpg)") {
		t.Fatalf("图3应保留:\n%s", out)
	}
}

func TestAssembleMoreImagesThanRepls(t *testing.T) {
	md := "![a](a.jpg)\n![b](b.jpg)"
	out := Assemble(md, []Replacement{{Seq: 1, Text: "文"}})
	if !strings.Contains(out, "![b](b.jpg)") {
		t.Fatalf("无决策的图应原样保留:\n%s", out)
	}
}

// SK-I3 去水印：Drop=true 的图整行删除（无占位），文本行保留。
func TestAssembleWatermarkDrop(t *testing.T) {
	md := "前文\n\n![水印](images/wm.png)\n\n后文\n"
	repls := []Replacement{{ImgRef: "images/wm.png", Drop: true}}
	got, shifts := AssembleWithShifts(md, repls)
	if strings.Contains(got, "wm.png") || strings.Contains(got, "水印") {
		t.Fatalf("水印行应整行删除: %q", got)
	}
	if !strings.Contains(got, "前文") || !strings.Contains(got, "后文") {
		t.Fatalf("文本行应保留: %q", got)
	}
	// 其后的"后文"位置应左移（整行+换行）
	if len(shifts) == 0 || shifts[0].delta >= 0 {
		t.Fatalf("去水印应记录负位移: %+v", shifts)
	}
}

// SK-I21 契约断言：终态 md 零裸图链接（老线 embedBlockForRef 语义）——
// 全部图有 replacement 时，输出不得残留任何图片行。
func TestAssembleNoBareImageLinks(t *testing.T) {
	md := "前文\n\n![图1](images/a1.png)\n\n![表1](images/b2.png)\n\n后文\n"
	repls := []Replacement{
		{ImgRef: "images/a1.png", Text: "[IMG_TYPE: Graph/Chart]\n函数变化率曲线，横轴自变量纵轴函数值。"},
		{ImgRef: "images/b2.png", Mermaid: "graph TD; A-->B"},
	}
	got, _ := AssembleWithShifts(md, repls)
	if strings.Contains(got, "![") {
		t.Fatalf("终态 md 出现裸图链接（SK-I21 契约违反）: %q", got[:min(160, len(got))])
	}
	if !strings.Contains(got, "函数变化率曲线") || !strings.Contains(got, "graph TD") {
		t.Fatalf("替换产物缺失: %q", got)
	}
}

// SK-I22：dev 契约三形态——text 类纯 body / 其余 [Image]( body · P.N ) / mermaid 锚+代码块；
// 页码单一系统：<!-- pN --> 原位转 [P. N]，锚后缀同源。
func TestAssembleDevEmbedFormat(t *testing.T) {
	md := "<!-- p1 -->\n\n前文\n\n![图1](images/a1.png)\n\n![图2](images/b2.png)\n\n![图3](images/c3.png)\n\n<!-- p2 -->\n\n后文\n"
	repls := []Replacement{
		{ImgRef: "images/a1.png", ImgType: "table", Text: "表头A|表头B\n--|--\n1|2"},
		{ImgRef: "images/b2.png", ImgType: "Graph/Chart", Text: "[IMG_TYPE: Graph/Chart]\n函数变化率曲线。"},
		{ImgRef: "images/c3.png", Mermaid: "graph TD; A-->B"},
	}
	got, _ := AssembleWithShifts(md, repls)
	cases := []struct{ want, desc string }{
		{"[P. 1]", "页界注释转我们的 [P. N]"},
		{"表头A|表头B", "table 类纯 body 直嵌"},
		{"[Image]( [IMG_TYPE: Graph/Chart]\n函数变化率曲线。 · P.1 )", "其余类 [Image]( body · P.N )"},
		{"[Image]( 图3 · P.1 )", "mermaid 锚用原图注 alt"},
		{"```mermaid\ngraph TD; A-->B\n```", "mermaid 围栏代码块"},
		{"<!-- p", "不应残留 organize 注释（单一页码系统）"},
	}
	for _, c := range cases {
		if c.desc == "不应残留 organize 注释（单一页码系统）" {
			if strings.Contains(got, c.want) {
				t.Fatalf("%s: 仍含 %q", c.desc, c.want)
			}
			continue
		}
		if !strings.Contains(got, c.want) {
			t.Fatalf("%s: 缺 %q\n实际: %s", c.desc, c.want, got[:min(400, len(got))])
		}
	}
}
