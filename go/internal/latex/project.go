package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ------------------------------------------------------------------
// 工作区布局：<输出根>/<项目名>/（同一输出根下多项目并存）
// ------------------------------------------------------------------
//
// v1.5.0-beta.3 及更早：输出根**本身**就是一本书的工作区
// （<root>/work、<root>/source、<root>/progress.json、<root>/out…），
// 于是一个目录里放不下两本书——书 A 跑完再跑书 B，两者互相覆盖、
// 会话与进度串在一起（progress.json / doc_index / source 都只有一份）。
//
// 现在：<root>/<项目名>/ 才是一本书的工作区，例如
//   latex_project/测试-概率论/、latex_project/线性代数/
//   finally_latex/测试-概率论/          （档位2 的输出根是 paths.latex_output）
// 项目名默认取这本书的"主题名"——就是 images/<主题>/ 用的那个名字
// （主 markdown 的主文件名，见 projectSourceName），可用
// `docvision latex --project <名字>` 覆盖。
//
// 兼容旧版：<root> 本身若已经是一个工作区（含 work/ source/
// progress.json doc_index/ progress_items/ 中的任一项），就继续把它当
// 项目根（旧行为），绝不搬迁、也不改动用户既有工程。

const (
	// projectMarkerFile 记录"这个目录是哪本书的工作区"，只用于同名项目
	// 的去重判定（同名目录已属于另一本书时，新项目退到 <名字>-2）。
	projectMarkerFile = ".docvision_project.json"

	// projectNameMaxRunes 限制项目名长度：多数文件系统的单个文件名上限
	// 是 255 **字节**，中文一个字 3 字节，60 个字留足余量。
	projectNameMaxRunes = 60

	// defaultProjectName 是既没有 --project 也推导不出主题名时的兜底。
	defaultProjectName = "book"
)

// legacyProjectEntries 是"输出根本身就是项目工作区"的判据：命中任意
// 一项即视为旧版单项目根（继续沿用）。用户既有工程里必然包含其中之一
// ——实测 `latex_project/` 顶层就是 build/ chapters/ doc_index/ out/
// progress.json source/ style/ work/（chapters、style、build、out 直接
// 挂在项目根下，不都在 work/ 里，所以它们也要算上，否则"只跑到一半"
// 的旧工程会被误判成空目录）：
//   - 档位1（全书）：work/ source/ style/ chapters/ build/ out/
//     doc_index/ progress.json
//   - 档位2（图片矢量化）：progress_items/（它没有 work/ 与
//     progress.json，若不认这一项，用户已有的 finally_latex/ 会被判成
//     "空目录"而改走新布局，历史逐图进度全部作废＝全量重跑）
var legacyProjectEntries = []string{
	"work", "source", "style", "chapters", "build", "out",
	"doc_index", "progress.json", "progress_items",
}

// ProjectLayout 是一次"项目根"解析的结果。
type ProjectLayout struct {
	// Root 是配置里的输出根（档位1: paths.latex_project，档位2:
	// paths.latex_output），只作为新项目的父目录使用。
	Root string
	// Dir 是本次运行真正的工作区（内容全部落在它下面）。
	Dir string
	// Name 是项目名（Dir 的目录名）。
	Name string
	// Legacy 为真表示 Root 本身就是既有工作区，Dir == Root（旧行为）。
	Legacy bool
	// Note 是人话解释（info 级别写进日志，也便于测试断言分支）。
	Note string
}

// projectMarker 是工作区里的自述文件（不是配置，删掉只会让同名去重
// 退化为"复用同名目录"）。
type projectMarker struct {
	Name    string `json:"name"`
	Source  string `json:"source,omitempty"` // 主 markdown 名（主题名来源）
	Level   int    `json:"level,omitempty"`  // 1=全书 2=图片矢量化
	Created string `json:"created,omitempty"`
}

// SanitizeProjectName 把用户/自动推导的名字变成安全的**单层**目录名：
// 去掉路径分隔符（正反斜杠换成 '-'）、控制字符、首尾空白与点，保留中文；
// "." / ".." / 空串一律返回 ""（由调用方回退）。
func SanitizeProjectName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '/' || r == '\\':
			// 路径分隔符：换掉而不是删掉，避免 "a/b" → "ab" 这种意外同名
			b.WriteRune('-')
		case r < 0x20 || r == 0x7f:
			// 控制字符（含 NUL）直接丢弃
		case strings.ContainsRune(`<>:"|?*`, r):
			// Windows 保留字符：目录名要能跨平台拷贝
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	// 连续的 "." 折叠成 '-'：任何位置出现 ".." 都是可疑片段（"../../etc"、
	// "a/../b" 这类穿越串），而单个点要保留（"v1.0"、"第1.2节"都正常）。
	out := b.String()
	for strings.Contains(out, "..") {
		out = strings.ReplaceAll(out, "..", "-")
	}
	// 首尾空白、点与连字符：顺手把 "." / ".." / "--" 这类清成空串。
	out = strings.Trim(out, " \t.-")
	if out == "" || out == "." || out == ".." {
		return ""
	}
	if r := []rune(out); len(r) > projectNameMaxRunes {
		out = strings.Trim(string(r[:projectNameMaxRunes]), " \t.-")
		if out == "" {
			return ""
		}
	}
	return out
}

// reservedProjectNames 是布局判据自己用的名字：项目若叫 work/source/…，
// 它就会落在 <root>/work，下一次运行 looksLikeProject(<root>) 立刻把
// 输出根误判成"旧版单项目工作区"，两本书又混回一个目录。
func reservedProjectName(name string) bool {
	lower := strings.ToLower(name)
	for _, e := range legacyProjectEntries {
		if lower == strings.ToLower(e) {
			return true
		}
	}
	return false
}

// looksLikeProject reports whether dir itself already IS a project
// workspace (as opposed to a container of project directories).
func looksLikeProject(dir string) bool {
	if dir == "" {
		return false
	}
	for _, e := range legacyProjectEntries {
		if _, err := os.Stat(filepath.Join(dir, e)); err == nil {
			return true
		}
	}
	return false
}

// projectOwnerOf returns the book that owns dir (the marker's source
// markdown, falling back to its recorded name): "" means "no marker,
// nothing known about it" — such a directory is reused, never renamed.
func projectOwnerOf(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, projectMarkerFile))
	if err != nil {
		return ""
	}
	var m projectMarker
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	if m.Source != "" {
		return m.Source
	}
	return m.Name
}

// writeProjectMarker records which book owns dir. Only written for the
// new multi-project layout: an untouched legacy workspace must stay
// byte-for-byte as the user left it.
func writeProjectMarker(lay ProjectLayout, source string, level int) error {
	m := projectMarker{Name: lay.Name, Source: source, Level: level, Created: time.Now().Format(time.RFC3339)}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(lay.Dir, projectMarkerFile), data, 0o644)
}

// resolveProjectDir is the SINGLE place that decides which directory the
// current book works in. Everything downstream (work/ source/ style/
// chapters/ doc_index/ out/ build/ progress.json、临时目录、pdfview、
// 会话挂载视图…) is derived from its result, so no other code path may
// build a project path itself.
//
//	root   — 输出根（paths.latex_project / paths.latex_output）
//	want   — 显式项目名（--project），空则用 source 推导
//	source — 主题名（主 markdown 名，images/<主题>/ 用的那个），空则用兜底
//
// 优先级（用户场景：同一台机器上既有旧工程 latex_project/*，又想再开
// 一本新书）：
//  1. 显式 --project → **一律** <root>/<名字>/，即使 root 里有旧工程
//     （这就是"在这台机器上开新项目"的显式出口）；
//  2. 没给 --project 且 root 命中旧布局标记 → 沿用 root（既有工程原地
//     可跑，一个字节都不动）；但若本书在 root 下已经有自己的项目目录，
//     继续用它（否则同一本书会随参数在 root 与子目录之间跳）；
//  3. 其余情况 → <root>/<默认项目名>/（默认名 = 主题名安全化）。
func resolveProjectDir(root, want, source string) (ProjectLayout, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return ProjectLayout{}, fmt.Errorf("未配置 LaTeX 输出根目录")
	}
	root = filepath.Clean(root)

	wantName := SanitizeProjectName(want)
	explicit := wantName != "" && !reservedProjectName(wantName)
	name := wantName
	if !explicit {
		name = SanitizeProjectName(source)
		if name == "" || reservedProjectName(name) {
			name = defaultProjectName
		}
	}

	// 1) 显式项目名：无条件走新布局。
	if explicit {
		lay, err := pickProjectDir(root, name, source)
		if err != nil {
			return ProjectLayout{}, err
		}
		lay.Note = fmt.Sprintf("[project] 已指定 --project，工作区: %s（输出根 %s）", lay.Dir, root)
		if sanitized := wantName; sanitized != strings.TrimSpace(want) {
			lay.Note += fmt.Sprintf("；项目名已安全化: %q -> %q", strings.TrimSpace(want), sanitized)
		}
		return lay, nil
	}
	// --project 给了名字但不可用（"."/".."/空/保留字）：退回自动命名并说明。
	badWant := strings.TrimSpace(want) != ""

	// 2) 没给 --project：root 本身是既有工作区 → 沿用（旧行为）。
	if looksLikeProject(root) {
		cand, err := pickProjectDir(root, name, source)
		if err != nil {
			return ProjectLayout{}, err
		}
		if dirExists(cand.Dir) {
			cand.Note = fmt.Sprintf("[project] 本书在此输出根下已有项目目录 %s，继续使用（未指定 --project；旧版单项目目录 %s 保持原样）",
				cand.Dir, root)
			return cand, nil
		}
		abs := root
		if a, err := filepath.Abs(root); err == nil {
			abs = a
		}
		return ProjectLayout{
			Root: root, Dir: root, Name: filepath.Base(abs), Legacy: true,
			Note: fmt.Sprintf("[project] 检测到旧版单项目布局 %s，未指定 --project，继续沿用（既有内容不改动、原地可跑）；要在同一输出根下新建独立项目请用 --project <名字>，新项目会放在 %s/<项目名>/",
				root, root),
		}, nil
	}

	// 3) 新布局：<root>/<项目名>/。
	lay, err := pickProjectDir(root, name, source)
	if err != nil {
		return ProjectLayout{}, err
	}
	lay.Note = fmt.Sprintf("[project] 项目工作区: %s（项目名 %q，输出根 %s）", lay.Dir, lay.Name, root)
	if badWant {
		lay.Note += fmt.Sprintf("；--project %q 不是可用的目录名，已改用 %q", strings.TrimSpace(want), lay.Name)
	}
	return lay, nil
}

// pickProjectDir joins root and name (both already sanitised) and applies
// the same-name rule: a directory already owned by ANOTHER book (marker
// records a different source markdown) is stepped aside to <名字>-2.
// A directory without a marker — empty, or one the user made by hand, or
// a刚才从旧布局搬过来的工程 — is reused as-is, so repeated runs stay in
// the same workspace (断点续传/幂等).
func pickProjectDir(root, name, source string) (ProjectLayout, error) {
	dir := filepath.Join(root, name)
	if !withinDir(root, dir) {
		return ProjectLayout{}, fmt.Errorf("项目名 %q 越出输出根 %s", name, root)
	}
	if owner := projectOwnerOf(dir); owner != "" && source != "" && owner != source {
		for i := 2; i <= 99; i++ {
			cand := filepath.Join(root, fmt.Sprintf("%s-%d", name, i))
			if o := projectOwnerOf(cand); o == "" || o == source {
				dir = cand
				break
			}
		}
	}
	return ProjectLayout{Root: root, Dir: dir, Name: filepath.Base(dir)}, nil
}

// dirExists reports whether path exists as a directory.
func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// withinDir reports whether path sits inside base (defense in depth: the
// name is already sanitised to a single path element).
func withinDir(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// projectSourceName derives the book's 主题名 — the name of
// images/<主题>/ — from its main markdown: the base name without
// extension. With several markdown files (batch run over output/) the
// same "largest md" heuristic as the style phase / PDF view is used.
func projectSourceName(sourceDir string, files []string) string {
	if len(files) == 1 {
		return subjectOf(filepath.Base(filepath.Clean(files[0])))
	}
	main := mainMDFile(sourceDir, files)
	if main == "" {
		return ""
	}
	return subjectOf(filepath.Base(main))
}

// useProjectDir resolves the workspace of the current book, remembers it
// on the Runner (ProjectDir) and logs which layout was chosen. It is the
// only caller of resolveProjectDir inside a run.
func (r *Runner) useProjectDir(root, want, source string, level int) (string, error) {
	lay, err := resolveProjectDir(root, want, source)
	if err != nil {
		return "", err
	}
	r.log.Log(0, lay.Note)
	if lay.Legacy {
		// 旧工程一个字节都不动：连项目标记也不写。
		r.projDir = lay.Dir
		return lay.Dir, nil
	}
	if err := os.MkdirAll(lay.Dir, 0o755); err != nil {
		return "", fmt.Errorf("创建项目目录 %s: %w", lay.Dir, err)
	}
	if err := writeProjectMarker(lay, source, level); err != nil {
		r.log.LogWarning(0, "[project] 项目标记写入失败（同名去重将退化为直接复用）:", err)
	}
	r.projDir = lay.Dir
	return lay.Dir, nil
}

// ProjectDir is the workspace resolved by the current run
// (<root>/<项目名>/, or the output root itself for legacy single-project
// layouts). Empty before RunBook/RunImages resolved it.
func (r *Runner) ProjectDir() string { return r.projDir }

// workRoot is the project workspace with a fallback for callers that run
// outside RunBook/RunImages (e.g. a bare Runner in tests): the configured
// level-1 project dir, preserving the pre-multi-project behaviour.
func (r *Runner) workRoot() string {
	if r.projDir != "" {
		return r.projDir
	}
	return r.cfg.Paths.LatexProject
}

// findProjectDirs lists the project workspaces directly under root (used
// by `docvision verify`, which has no book name to derive a name from).
// It only reports directories that actually look like a workspace.
func findProjectDirs(root string) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Clean(root))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if looksLikeProject(dir) {
			out = append(out, dir)
		}
	}
	return out
}

// resolveExistingProjectDir finds the workspace to operate on when the
// caller has no book in hand (docvision verify): the root itself for a
// legacy layout, a single sub-project, or an explicit --project name.
// Several candidates without --project is an error — silently picking
// one would verify the wrong book.
func resolveExistingProjectDir(root, want string) (ProjectLayout, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return ProjectLayout{}, fmt.Errorf("未配置 LaTeX 输出根目录")
	}
	root = filepath.Clean(root)

	dirs := findProjectDirs(root)
	if want != "" {
		name := SanitizeProjectName(want)
		if name != "" {
			dir := filepath.Join(root, name)
			if dirExists(dir) {
				return ProjectLayout{Root: root, Dir: dir, Name: name}, nil
			}
			return ProjectLayout{}, fmt.Errorf("在 %s 下未找到项目 %q（现有项目: %s）",
				root, name, strings.Join(projectBaseNames(dirs), ", "))
		}
	}
	if looksLikeProject(root) {
		abs := root
		if a, err := filepath.Abs(root); err == nil {
			abs = a
		}
		return ProjectLayout{Root: root, Dir: root, Name: filepath.Base(abs), Legacy: true}, nil
	}
	switch len(dirs) {
	case 0:
		return ProjectLayout{}, fmt.Errorf("在 %s 下未找到任何项目工作区（也不像旧版单项目布局）", root)
	case 1:
		return ProjectLayout{Root: root, Dir: dirs[0], Name: filepath.Base(dirs[0])}, nil
	default:
		return ProjectLayout{}, fmt.Errorf("%s 下有 %d 个项目（%s）：请用 --project <名字> 指定",
			root, len(dirs), strings.Join(projectBaseNames(dirs), ", "))
	}
}

func projectBaseNames(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, filepath.Base(d))
	}
	return out
}
