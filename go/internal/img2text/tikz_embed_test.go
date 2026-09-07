package img2text

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestEmbedBlockFor(t *testing.T) {
	cases := []struct{ name, result, want string }{
		{"plain text", "[IMG_TYPE: text]\n这是一段说明文字。", "\n\n这是一段说明文字。\n\n"},
		{"latex math", "[IMG_TYPE: latex]\n$$E=mc^2$$", "\n\n$$E=mc^2$$\n\n"},
		{"table", "[IMG_TYPE: table]\n| a | b |\n| - | - |", "\n\n| a | b |\n| - | - |\n\n"},
		{"mermaid passthrough", "[IMG_TYPE: flowchart]\n```mermaid\ngraph TD; A-->B\n```", "\n\n```mermaid\ngraph TD; A-->B\n```\n\n"},
		{"tikz passthrough", "[IMG_TYPE: tikz]\n```tikz\n\\begin{tikzpicture}\\draw(0,0)--(1,1);\n\\end{tikzpicture}\n```", "\n\n```tikz\n\\begin{tikzpicture}\\draw(0,0)--(1,1);\n\\end{tikzpicture}\n```\n\n"},
		{"visual default", "[IMG_TYPE: screenshot]\n一个 IDE 截图。", "\n\n[Image]( 一个 IDE 截图。 )\n\n"},
		{"missing prefix", "普通文本", "\n\n[Image]( 普通文本 )\n\n"},
	}
	for _, tc := range cases {
		if got := embedBlockFor(tc.result); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestValidateTikZ(t *testing.T) {
	if _, err := exec.LookPath("pdflatex"); err != nil {
		t.Skip("pdflatex 不可用")
	}
	good := "[IMG_TYPE: tikz]\n```tikz\n\\begin{tikzpicture}\\draw (0,0) -- (1,1);\n\\end{tikzpicture}\n```"
	res := ValidateTikZ(context.Background(), good, "", 60*time.Second)
	if !res.Valid {
		t.Fatalf("good tikz rejected: %s", res.Error)
	}
	bad := "[IMG_TYPE: tikz]\n```tikz\n\\begin{tikzpicture}\\draw (0,0) -- ;\n\\end{tikzpicture}\n```"
	res2 := ValidateTikZ(context.Background(), bad, "", 60*time.Second)
	if res2.Valid {
		t.Fatal("bad tikz accepted")
	}
}
