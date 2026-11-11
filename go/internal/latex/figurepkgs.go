package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// preambleOnlyPrefixes 是不能出现在章节正文里的命令（T320）：作图会话在代码
// 顶端带了 `\usepackage{tikz-3dplot}\standaloneconfig{...}` 这类前言行，转换
// 会话把它原样粘进 .tex 会直接编译失败——"Can be used only in preamble"。
var preambleOnlyPrefixes = []string{
	`\documentclass`, `\usepackage`, `\RequirePackage`,
	`\standaloneconfig`, `\begin{document}`, `\end{document}`,
}

// stripFigurePreamble 去掉图代码里的前言行，只留可以进正文的部分。
// `\usetikzlibrary{...}` 留在原处：它在正文里也能用。
func stripFigurePreamble(code string) string {
	if code == "" {
		return ""
	}
	lines := strings.Split(code, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		drop := false
		for _, p := range preambleOnlyPrefixes {
			if strings.HasPrefix(t, p) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, ln)
		}
	}
	// 去掉前后空行，免得围栏里多出空白。
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

var (
	figUsePackageRe = regexp.MustCompile(`\\usepackage(?:\[[^\]]*\])?\{([^}]+)\}`)
	figUseLibraryRe = regexp.MustCompile(`\\usetikzlibrary\{([^}]+)\}`)
	figRequireRe    = regexp.MustCompile(`\\RequirePackage(?:\[[^\]]*\])?\{([^}]+)\}`)
)

// figureNeeds 是"本书已产出的矢量图需要什么"的统计结果。
type figureNeeds struct {
	Packages []string       // 形如 "tikz-3dplot (18)"
	Libs     []string       // 形如 "arrows.meta (71)"
	PkgCount map[string]int // 原始计数，给"样式类漏加载了哪些"用
	LibCount map[string]int
}

func (f figureNeeds) empty() bool { return len(f.Packages) == 0 && len(f.Libs) == 0 }

// figurePackageNeeds 扫本书所有矢量图的代码，数出它们需要的宏包与 TikZ 库
// （T320）。样式阶段把这份清单写进首条消息，要求样式类自己加载——转换会话
// 只能原样粘贴图代码、加不了宏包，缺一个就会被逼着重画整张图。
func figurePackageNeeds(progDir string) figureNeeds {
	files, _ := filepath.Glob(filepath.Join(progDir, "*", "*.json"))
	pkgs, libs := map[string]int{}, map[string]int{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var p imageProgress
		if json.Unmarshal(data, &p) != nil || p.TikzCode == "" {
			continue
		}
		collect := func(re *regexp.Regexp, into map[string]int) {
			for _, m := range re.FindAllStringSubmatch(p.TikzCode, -1) {
				for _, name := range strings.Split(m[1], ",") {
					if name = strings.TrimSpace(name); name != "" {
						into[name]++
					}
				}
			}
		}
		collect(figUsePackageRe, pkgs)
		collect(figRequireRe, pkgs)
		collect(figUseLibraryRe, libs)
	}
	return figureNeeds{Packages: rankNeed(pkgs), Libs: rankNeed(libs), PkgCount: pkgs, LibCount: libs}
}

// rankNeed 按出现次数降序、同数按名字排，转成 "名字 (次数)" 的形式，最多 12 条。
func rankNeed(counts map[string]int) []string {
	type kv struct {
		name string
		n    int
	}
	list := make([]kv, 0, len(counts))
	for k, v := range counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].name < list[j].name
	})
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, e.name+" ("+itoa(e.n)+")")
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

var clsLoadRe = regexp.MustCompile(`\\(?:RequirePackage|usepackage)(?:\[[^\]]*\])?\{([^}]*)\}`)

// missingFigurePackages 比对"图代码要什么"与"样式类加载了什么"，返回缺的那些
// （T321）。缺一个宏包的后果不是编译报错就是把图重画——两种都不该静默发生。
func missingFigurePackages(clsText string, needs figureNeeds) (pkgs, libs []string) {
	loaded := map[string]bool{}
	for _, m := range clsLoadRe.FindAllStringSubmatch(clsText, -1) {
		for _, name := range strings.Split(m[1], ",") {
			if name = strings.TrimSpace(name); name != "" {
				loaded[name] = true
			}
		}
	}
	for name, n := range needs.PkgCount {
		if !loaded[name] {
			pkgs = append(pkgs, fmt.Sprintf("%s（%d 张）", name, n))
		}
	}
	for name, n := range needs.LibCount {
		if !strings.Contains(clsText, name) {
			libs = append(libs, fmt.Sprintf("%s（%d 张）", name, n))
		}
	}
	sort.Strings(pkgs)
	sort.Strings(libs)
	return pkgs, libs
}

// warnMissingFigurePackages 在转换前置检查里喊一嗓子：样式类没加载图代码需要的
// 宏包/库时，转换会话只能编译失败或者背着人重画（T321）。
func (r *Runner) warnMissingFigurePackages(proj string) {
	needs := figurePackageNeeds(filepath.Join(proj, "source", "progress_items"))
	if needs.empty() {
		return
	}
	var clsText strings.Builder
	files, _ := filepath.Glob(filepath.Join(proj, "style", "*.cls"))
	for _, f := range files {
		if data, err := os.ReadFile(f); err == nil {
			clsText.Write(data)
		}
	}
	if clsText.Len() == 0 {
		return
	}
	pkgs, libs := missingFigurePackages(clsText.String(), needs)
	if len(pkgs) == 0 && len(libs) == 0 {
		return
	}
	msg := "[preflight] 样式类没有加载图代码需要的："
	if len(pkgs) > 0 {
		msg += "宏包 " + strings.Join(pkgs, "、") + "；"
	}
	if len(libs) > 0 {
		msg += "TikZ 库 " + strings.Join(libs, "、") + "；"
	}
	msg += "转换会话粘不进 \\usepackage，缺了会被逼着重画整张图（或编译失败）。在 cls 里补上 \\RequirePackage / \\usetikzlibrary 之后重转这些章节。"
	r.log.LogWarning(0, msg)
}
