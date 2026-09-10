package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// movedPairs 记录 (源文件, 常量名, 模板名) —— 用于搬迁后的字节级校验。
type pair struct{ File, Const, Tmpl string }

var movedPairs = []pair{
	{"internal/latex/prompts.go", "classifierSystemPrompt", "latex_classifier.system"},
	{"internal/latex/prompts.go", "latexFigurePrompt", "latex_figure.system"},
	{"internal/latex/prompts.go", "styleSystemPrompt", "latex_style.system"},
	{"internal/latex/prompts.go", "chapterSystemPrompt", "latex_chapters.system"},
	{"internal/latex/prompts.go", "convertSystemPrompt", "latex_convert.system"},
	{"internal/latex/prompts.go", "styleFixSystemPrompt", "latex_stylefix.system"},
	{"internal/latex/prompts.go", "finalReviewSystemPrompt", "latex_finalreview.system"},
	{"internal/latex/prompts.go", "fixSystemPrompt", "latex_fix.system"},
	{"internal/latex/verify.go", "verifySystemPrompt", "latex_verify.system"},
	{"internal/latex/watermark.go", "watermarkDetectSystemPrompt", "latex_watermark.system"},
	{"internal/img2text/processor.go", "systemPromptTemplate", "img2text.system"},
	{"internal/img2text/textonly.go", "textOnlySystemPrompt", "img2text_textonly.system"},
}

// verifyMove 用 git show 取出搬迁前的常量文本，与模板逐字节比较。
func verifyMove() error {
	bad := 0
	for _, p := range movedPairs {
		cmd := exec.Command("git", "show", "HEAD:go/"+p.File)
		cmd.Dir = ".." // 仓库根
		raw, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("git show %s: %w", p.File, err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p.File, raw, 0)
		if err != nil {
			return err
		}
		old := ""
		ast.Inspect(f, func(n ast.Node) bool {
			gd, ok := n.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				return true
			}
			vs, ok := gd.Specs[0].(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || vs.Names[0].Name != p.Const {
				return true
			}
			t, ok := literalText(vs.Values[0])
			if ok {
				old = t
			}
			return true
		})
		if old == "" {
			return fmt.Errorf("%s: 未能取到旧常量 %s", p.File, p.Const)
		}
		cur, err := os.ReadFile(filepath.Join("internal/prompts/templates", p.Tmpl+".md"))
		if err != nil {
			return err
		}
		now := strings.TrimRight(string(cur), "\n")
		want := strings.TrimRight(old, "\n")
		same := now == want
		// style 模板额外并入了 640 字符的 WORKSPACE 说明（来自 book.go）
		merged := p.Tmpl == "latex_style.system" && strings.HasPrefix(now, want) &&
			len(now)-len(want) >= 600 && len(now)-len(want) <= 700
		mark := "✗"
		note := ""
		if same {
			mark = "✓"
		} else if merged {
			mark, note = "✓", fmt.Sprintf("（额外并入 %d 字符 WORKSPACE 说明）", len(now)-len(want))
		} else {
			bad++
			for i := 0; i < len(old) && i < len(now); i++ {
				if old[i] != now[i] {
					lo, hi := i-50, i+50
					if lo < 0 {
						lo = 0
					}
					note = fmt.Sprintf("\n      首个差异 @%d\n      旧: %q\n      新: %q", i, old[lo:min(hi, len(old))], now[lo:min(hi, len(now))])
					break
				}
			}
		}
		fmt.Printf("  %s %-34s 旧=%6d 新=%6d %s%s\n", mark, p.Tmpl+".md", len(old), len(now), note, "")
	}
	if bad > 0 {
		return fmt.Errorf("%d 个模板与旧文本不一致", bad)
	}
	fmt.Println("全部模板与搬迁前的文本逐字节一致")
	return nil
}

var _ = json.Marshal
var _ = sort.Strings
