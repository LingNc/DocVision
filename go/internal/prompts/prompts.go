// Package prompts is the single home of every large, static prompt text in
// DocVision: the system prompts of each AI session and the first user
// message of each session. The text lives in templates/*.md (embedded in
// the binary, so there is no runtime file dependency) and this package only
// knows how to look a template up and fill its placeholders.
//
// Why a package instead of string constants next to each session:
//
//   - One place to review the rules every session shares. Two real
//     incidents came from scattered wording: the "do not overlap/crowd"
//     rule lived in two sessions with drifting text, and the figure prompt
//     both asked for a faithful scale and told the model to draw bigger.
//   - Placeholders cannot silently reach the model. {OUTPUT_LANG} and
//     {MAX_ROUNDS} were once sent verbatim because one call site passed a
//     raw constant instead of a rendered prompt; the registry below
//     declares every placeholder and the tests assert that a full render
//     leaves none behind.
//   - Removed tool names cannot linger. Templates declare which tool names
//     they must mention, and a blacklist of retired names (read_md,
//     view_page, list_images, install_font, compile_preview, ...) is
//     checked against every template.
//
// Small, state-coupled fragments deliberately stay where they are used:
// session loop reminders (round budget, image placeholder note, compaction
// instruction, empty-reply nudge), every tool's Definition() description,
// and log wording are all assembled next to the state they describe.
package prompts

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed templates/*.md
var files embed.FS

// Template names. One .md file per prompt: <session>.<role>.md.
const (
	// LaTeX level-1 / level-2 sessions.
	ClassifierSystem  = "latex_classifier.system"
	FigureSystem      = "latex_figure.system"
	StyleSystem       = "latex_style.system"
	ChaptersSystem    = "latex_chapters.system"
	ConvertSystem     = "latex_convert.system"
	StyleFixSystem    = "latex_stylefix.system"
	FinalReviewSystem = "latex_finalreview.system"
	FixSystem         = "latex_fix.system"
	VerifySystem      = "latex_verify.system"
	WatermarkSystem   = "latex_watermark.system"
	CheckerSystem     = "latex_checker.system"
	FigureCheckSystem = "latex_figurecheck.system"

	// T43 风格修复大循环的三个会话。
	StyleFixIntegrateSystem = "latex_stylefix_integrate.system"
	StyleFixAssessSystem    = "latex_stylefix_assess.system"
	StyleFixBlockSystem     = "latex_stylefix_block.system"

	// 各会话的首次用户提示词（静态文案进模板，动态数据用占位符传入）。
	StyleUser       = "latex_style.user"
	ChaptersUser    = "latex_chapters.user"
	ConvertUser     = "latex_convert.user"
	CheckerUser     = "latex_checker.user"
	FigureCheckUser = "latex_figurecheck.user"
	StyleFixUser    = "latex_stylefix.user"
	FinalReviewUser = "latex_finalreview.user"
	FigureUser      = "latex_figure.user"
	Img2TextUser    = "img2text.user"

	// img2text (image understanding) sessions.
	Img2TextSystem = "img2text.system"
	TextOnlySystem = "img2text_textonly.system"
)

// Template describes one prompt file: which placeholders the caller must
// fill, and which text it is expected to contain. The guards in
// prompts_test.go use this to prove that no template ships with an
// unrendered placeholder or a retired tool name.
type Template struct {
	Name string
	File string   // path inside templates/
	Vars []string // placeholder names, without braces
	// MustMention lists tool names that must appear in the text (the model
	// has to be told which tools it may call).
	MustMention []string
	// ForbidMention lists names this template must NOT contain (stale
	// wording that would teach the model a tool that no longer exists, or
	// an output type the caller cannot parse). Case-insensitive.
	ForbidMention []string
}

// retiredNames are tool/flag names that were removed from the code at some
// point but kept appearing in prompts. No template may mention them.
var retiredNames = []string{
	"read_md", "read_lines", "view_page", "view_source_page", "list_images",
	"install_font", "compile_preview", "image_locate", "mermaid_validation",
	"preview.png", "format_fix_attempts",
}

var registry = []Template{
	{
		Name: ClassifierSystem, File: "latex_classifier.system.md",
	},
	{
		Name: FigureSystem, File: "latex_figure.system.md",
		Vars:        []string{"MAX_ROUNDS", "OUTPUT_LANG"},
		MustMention: []string{"compile", "edit_file", "image_context", "read_file", "submit", "view_image", "view_pdf", "write_file"},
	},
	{
		Name: StyleSystem, File: "latex_style.system.md",
		MustMention: []string{"compile", "doc_search", "edit_file", "grep", "list_fonts", "list_source_pages", "read_file", "submit", "submit_style", "view_image", "view_pdf", "write_file"},
	},
	{
		Name: ChaptersSystem, File: "latex_chapters.system.md",
		MustMention: []string{"bash", "edit_file", "grep", "read_file", "submit", "submit_split"},
	},
	{
		Name: ConvertSystem, File: "latex_convert.system.md",
		Vars:        []string{"MAX_ROUNDS"},
		MustMention: []string{"compile", "doc_search", "edit_file", "grep", "list_source_pages", "read_file", "submit", "view_image", "view_pdf", "write_file"},
	},
	{
		Name: StyleFixIntegrateSystem, File: "latex_stylefix_integrate.system.md",
		MustMention: []string{"submit_problems"},
	},
	{
		Name: StyleFixAssessSystem, File: "latex_stylefix_assess.system.md",
		MustMention: []string{"grep", "read_file", "submit_blocks"},
	},
	{
		Name: StyleFixBlockSystem, File: "latex_stylefix_block.system.md",
		MustMention: []string{"compile", "edit_file", "read_file", "submit", "write_file"},
	},
	{
		Name: StyleFixSystem, File: "latex_stylefix.system.md",
		MustMention: []string{"compile", "edit_file", "read_file", "submit", "write_file"},
	},
	{
		Name: FinalReviewSystem, File: "latex_finalreview.system.md",
		MustMention: []string{"bash", "compile", "doc_search", "edit_file", "grep", "list_source_pages", "read_file", "submit", "view_image", "view_pdf", "write_file"},
	},
	{
		Name: FixSystem, File: "latex_fix.system.md",
		MustMention: []string{"bash", "compile", "edit_file", "grep", "list_fonts", "read_file", "submit", "view_image", "view_pdf", "write_file"},
	},
	{
		Name: VerifySystem, File: "latex_verify.system.md",
	},
	{
		Name: WatermarkSystem, File: "latex_watermark.system.md",
	},
	{
		Name: Img2TextSystem, File: "img2text.system.md",
		Vars: []string{"MAX_TOOL_CALLS", "OUTPUT_LANG"},
		// img2text 只出 Mermaid（外加表格/正文），LaTeX 矢量图由 档位1
		// 的作图会话负责。这段提示词曾被加上"用 latex 代码块画矢量图"
		// 的规则，模型于是产出 ```latex/TikZ，而 img2text 侧的解析与
		// 校验都处理不了（真实跑批 39/81 张因此报废）。这里钉住：这条
		// 提示词不得再提 LaTeX 绘图类型。
		ForbidMention: []string{"tikz", "pgfplots", "latex code block", "latex vector"},
	},
	{
		Name: TextOnlySystem, File: "img2text_textonly.system.md",
	},
	{
		Name: StyleUser, File: "latex_style.user.md",
		Vars:        []string{"MAIN_MD"},
		MustMention: []string{"doc_search", "list_source_pages", "read_file", "submit_style", "view_image"},
	},
	{
		Name: ChaptersUser, File: "latex_chapters.user.md",
		Vars:        []string{"TOTAL_LINES", "GRANULARITY"},
		MustMention: []string{"grep", "submit_split"},
	},
	{
		Name: CheckerSystem, File: "latex_checker.system.md",
		Vars:        []string{"MAX_ROUNDS"},
		MustMention: []string{"grep", "read_file", "submit"},
	},
	{
		Name: CheckerUser, File: "latex_checker.user.md",
		Vars:        []string{"CHAPTER_MD", "CHAPTER_TEX", "PARTS_DIR"},
		MustMention: []string{"grep", "read_file", "submit"},
	},
	{
		Name: FigureCheckSystem, File: "latex_figurecheck.system.md",
		Vars:        []string{"MAX_ROUNDS"},
		MustMention: []string{"submit"},
	},
	{
		Name: FigureCheckUser, File: "latex_figurecheck.user.md",
		Vars:        []string{"ORIGINAL", "REDRAWN"},
		MustMention: []string{"submit"},
	},
	{
		Name: ConvertUser, File: "latex_convert.user.md",
		Vars: []string{"CHAPTER_FILE", "TEX_PATH", "CHAPTER_PREVIEW"},
	},
	{
		Name: StyleFixUser, File: "latex_stylefix.user.md",
		Vars:        []string{"CHAPTER_FILE", "ISSUES"},
		MustMention: []string{"compile", "edit_file", "submit"},
	},
	{
		Name: FinalReviewUser, File: "latex_finalreview.user.md",
		Vars:        []string{"PAGES_LINE", "CHAPTERS"},
		MustMention: []string{"bash", "compile", "edit_file", "list_source_pages", "read_file", "submit", "view_pdf"},
	},
	{
		Name: FigureUser, File: "latex_figure.user.md",
		Vars:        []string{"ORIGINAL_SIZE", "VIEW_BUDGET", "CONTEXT"},
		MustMention: []string{"compile", "write_file"},
	},
	{
		Name: Img2TextUser, File: "img2text.user.md",
		Vars: []string{"LINE", "UP_START", "DOWN_END", "UP", "DOWN", "CONTEXT", "MAX_UP", "MAX_DOWN"},
	},
}

// Templates returns the registry (for tests and tooling).
func Templates() []Template {
	out := make([]Template, len(registry))
	copy(out, registry)
	return out
}

// RetiredNames returns the retired tool/flag names no template may mention.
func RetiredNames() []string {
	out := make([]string, len(retiredNames))
	copy(out, retiredNames)
	return out
}

// Files lists the embedded template files (for the guard tests).
func Files() []string {
	entries, err := files.ReadDir("templates")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// Must returns the raw template text for name; it panics on an unknown name
// because a missing prompt is a programming error, not a runtime condition.
func Must(name string) string {
	text, err := Get(name)
	if err != nil {
		panic(err)
	}
	return text
}

// Get returns the raw template text for name.
func Get(name string) (string, error) {
	for _, t := range registry {
		if t.Name == name {
			b, err := files.ReadFile("templates/" + t.File)
			if err != nil {
				return "", fmt.Errorf("prompts: 读取模板 %s 失败: %w", t.File, err)
			}
			// 模板按文本文件惯例结尾带换行；提示词正文按原样使用，
			// 于是尾部换行在这里吃掉——这样编辑器/格式化加不加结尾换行
			// 都不会改变发给模型的正文。
			return strings.TrimRight(string(b), "\n"), nil
		}
	}
	return "", fmt.Errorf("prompts: 未知模板 %q（已知：%s）", name, strings.Join(names(), ", "))
}

// Render fills {KEY} placeholders. Keys are given without braces. A random
// placeholder that nobody supplies stays in the text on purpose: the guard
// test renders with the declared vars and fails if anything is left, which
// is how a forgotten placeholder is caught instead of reaching the model.
func Render(name string, vars map[string]string) string {
	text := Must(name)
	return Fill(text, vars)
}

// Fill substitutes {KEY} placeholders in an already-loaded text.
func Fill(text string, vars map[string]string) string {
	if len(vars) == 0 {
		return text
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		text = strings.ReplaceAll(text, "{"+k+"}", vars[k])
	}
	return text
}

// Placeholders returns every {NAME} token left in text (for guard tests).
func Placeholders(text string) []string {
	var out []string
	seen := map[string]bool{}
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		j := strings.IndexByte(text[i:], '}')
		if j < 0 {
			break
		}
		token := text[i+1 : i+j]
		if token == "" || strings.ContainsAny(token, " \n\t{}\",)") {
			i += j
			continue
		}
		allUpper := true
		for _, r := range token {
			if !(r >= 'A' && r <= 'Z') && r != '_' && !(r >= '0' && r <= '9') {
				allUpper = false
				break
			}
		}
		if allUpper && !seen[token] {
			seen[token] = true
			out = append(out, token)
		}
		i += j
	}
	sort.Strings(out)
	return out
}

func names() []string {
	var out []string
	for _, t := range registry {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}
