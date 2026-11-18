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
