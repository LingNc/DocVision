// Command promptextract moves prompt text out of Go source files into
// template files, using go/ast so the extracted text and the replacement
// offsets are exact. It is a one-shot development tool (deleted after use).
//
// Usage:
//
//	go run ./tmp_prompt_extract -mode systems            # move const prompts
//	go run ./tmp_prompt_extract -mode list-users         # list user-prompt sites
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type move struct {
	File  string // source file
	Const string // const name to move
	Tmpl  string // template base name (without .md)
}

var _ = sort.Strings

var systemMoves = []move{
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

// literalText joins the string literals of a (possibly concatenated)
// expression, returning the text and whether every operand was a literal.
func literalText(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := unquote(v.Value)
		if err != nil {
			return "", false
		}
		return s, true
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		l, ok1 := literalText(v.X)
		r, ok2 := literalText(v.Y)
		if !ok1 || !ok2 {
			return "", false
		}
		return l + r, true
	case *ast.ParenExpr:
		return literalText(v.X)
	}
	return "", false
}

// unquote handles both raw (backtick) and interpreted literals.
func unquote(lit string) (string, error) {
	if strings.HasPrefix(lit, "`") {
		return strings.TrimSuffix(strings.TrimPrefix(lit, "`"), "`"), nil
	}
	var out strings.Builder
	body := lit[1 : len(lit)-1]
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		i++
		if i >= len(body) {
			break
		}
		switch body[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case '\\':
			out.WriteByte('\\')
		case '"':
			out.WriteByte('"')
		case '\'':
			out.WriteByte('\'')
		default:
			out.WriteByte('\\')
			out.WriteByte(body[i])
		}
	}
	return out.String(), nil
}

func main() {
	mode := flag.String("mode", "systems", "systems | list-users")
	flag.Parse()
	switch *mode {
	case "list-users":
		listUsers()
		return
	case "verify":
		if err := verifyMove(); err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			os.Exit(1)
		}
		return
	}
	if err := doSystems(); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func doSystems() error {
	if err := os.MkdirAll("internal/prompts/templates", 0o755); err != nil {
		return err
	}
	byFile := map[string][]move{}
	for _, m := range systemMoves {
		byFile[m.File] = append(byFile[m.File], m)
	}
	for file, moves := range byFile {
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
		if err != nil {
			return err
		}
		type repl struct {
			start, end int
			text       string
		}
		var repls []repl
		moved := map[string]string{} // const -> accessor
		for _, m := range moves {
			var found bool
			ast.Inspect(f, func(n ast.Node) bool {
				gd, ok := n.(*ast.GenDecl)
				if !ok || gd.Tok != token.CONST || len(gd.Specs) != 1 {
					return true
				}
				vs, ok := gd.Specs[0].(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 || vs.Names[0].Name != m.Const || len(vs.Values) != 1 {
					return true
				}
				text, ok := literalText(vs.Values[0])
				if !ok {
					return true
				}
				found = true
				// write template
				path := filepath.Join("internal/prompts/templates", m.Tmpl+".md")
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					panic(err)
				}
				fmt.Printf("写入 %s（%d 字符，来自 %s:%d）\n", path, len(text),
					file, fset.Position(gd.Pos()).Line)
				// delete the whole const declaration (keyword + doc comment)
				start := fset.Position(gd.Pos()).Offset
				end := fset.Position(gd.End()).Offset
				if gd.Doc != nil {
					start = fset.Position(gd.Doc.Pos()).Offset
				}
				repls = append(repls, repl{start, end, ""})
				moved[m.Const] = accessor(m.Const)
				return true
			})
			if !found {
				return fmt.Errorf("%s: 未找到常量 %s", file, m.Const)
			}
		}
		// Identifier usages elsewhere in the same file — but never inside a
		// range that is being deleted (the const's own name lives there, and
		// overlapping edits produced garbage the first time).
		inDeleted := func(off int) bool {
			for _, r := range repls {
				if off >= r.start && off < r.end {
					return true
				}
			}
			return false
		}
		for name, acc := range moved {
			ast.Inspect(f, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok || id.Name != name {
					return true
				}
				off := fset.Position(id.Pos()).Offset
				if inDeleted(off) {
					return true
				}
				if off > 0 && src[off-1] == '.' { // already qualified (prompts.X)
					return true
				}
				repls = append(repls, repl{off, fset.Position(id.End()).Offset, acc})
				return true
			})
		}
		// apply from the end so offsets stay valid
		sort.Slice(repls, func(i, j int) bool { return repls[i].start > repls[j].start })
		out := string(src)
		for _, r := range repls {
			if r.start < 0 || r.end > len(out) || r.start > r.end {
				return fmt.Errorf("%s: 非法替换区间 %d-%d", file, r.start, r.end)
			}
			out = out[:r.start] + r.text + out[r.end:]
		}
		if err := os.WriteFile(file, []byte(out), 0o644); err != nil {
			return err
		}
		fmt.Printf("改写 %s：%d 处替换\n", file, len(repls))
	}
	return nil
}

// accessor maps a Go const name to prompts.<Const> accessor name.
func accessor(constName string) string {
	name := strings.TrimSuffix(constName, "SystemPrompt")
	name = strings.TrimSuffix(name, "Prompt")
	name = strings.TrimSuffix(name, "Template")
	switch name {
	case "system":
		name = "System"
	case "classifier":
		name = "ClassifierSystem"
	case "latexFigure":
		name = "FigureSystem"
	case "style":
		name = "StyleSystem"
	case "chapter":
		name = "ChaptersSystem"
	case "convert":
		name = "ConvertSystem"
	case "styleFix":
		name = "StyleFixSystem"
	case "finalReview":
		name = "FinalReviewSystem"
	case "fix":
		name = "FixSystem"
	case "verify":
		name = "VerifySystem"
	case "watermarkDetect":
		name = "WatermarkSystem"
	case "textOnly":
		name = "TextOnlySystem"
	}
	return "prompts.Must(prompts." + name + ")"
}

// listUsers prints the user-prompt construction sites so their static text
// can be moved template by template.
func listUsers() {
	files := []string{
		"internal/latex/book.go", "internal/latex/assemble.go", "internal/latex/tikz.go",
		"internal/latex/checker.go", "internal/latex/classify.go", "internal/latex/runner.go",
		"internal/latex/verify.go", "internal/latex/watermark.go",
		"internal/img2text/processor.go", "internal/img2text/textonly.go", "internal/img2text/tikz.go",
	}
	type item struct {
		File  string `json:"file"`
		Line  int    `json:"line"`
		Kind  string `json:"kind"`
		Elems int    `json:"elems"`
		Chars int    `json:"chars"`
		First string `json:"first"`
	}
	var items []item
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			// []string{...} or []map[string]interface{}{{...}} with a text field
			arr, ok := cl.Type.(*ast.ArrayType)
			if !ok {
				return true
			}
			if id, ok := arr.Elt.(*ast.Ident); !ok || id.Name != "string" {
				return true
			}
			total := 0
			first := ""
			for i, el := range cl.Elts {
				if t, ok := literalText(el); ok {
					total += len(t)
					if i == 0 {
						first = strings.SplitN(t, "\n", 2)[0]
					}
				}
			}
			if total < 400 {
				return true
			}
			items = append(items, item{File: file, Line: fset.Position(cl.Pos()).Line, Kind: "[]string",
				Elems: len(cl.Elts), Chars: total, First: first})
			return true
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(items)
}
