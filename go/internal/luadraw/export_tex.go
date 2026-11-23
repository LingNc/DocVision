package luadraw

import (
	"regexp"
	"strings"
)

var documentClassPattern = regexp.MustCompile(`(?m)^\\documentclass(?:\[[^]]*\])?\{[^}]+\}\s*`)
var documentBodyPattern = regexp.MustCompile(`(?s)\\begin\s*\{document\}(.*?)\\end\s*\{document\}`)
var documentStartPattern = regexp.MustCompile(`(?s)\\begin\s*\{document\}`)
var luadrawPackageLine = regexp.MustCompile(`(?m)^\s*\\usepackage(?:\[[^]]*\])?\{luadraw\}\s*`)

func OriginalTeXPreamble(code string, sourceIsTeX bool) string {
	if !sourceIsTeX {
		return ""
	}
	var preamble string
	if class := documentClassPattern.FindStringIndex(code); class != nil {
		if doc := documentStartPattern.FindStringIndex(code); doc != nil && doc[0] >= class[1] {
			preamble = code[class[1]:doc[0]]
		}
	} else if env := envPattern.FindStringIndex(code); env != nil {
		preamble = code[:env[0]]
	}
	return luadrawPackageLine.ReplaceAllString(preamble, "")
}

// SelfContainedTeX embeds Lua code in the official LuaDraw environment.
// It deliberately contains no references to intermediate Lua/TikZ files.
func SelfContainedTeX(code string, options map[string]any, sourceIsTeX bool) string {
	body := code
	customPreamble := ""
	if sourceIsTeX {
		customPreamble = OriginalTeXPreamble(code, true)
		if m := documentBodyPattern.FindStringSubmatch(code); len(m) == 2 {
			body = m[1]
		}
		if matches := envPattern.FindStringSubmatch(body); len(matches) == 4 && matches[1] == matches[3] {
			body = matches[2]
		}
	}

	preamble := strings.Builder{}
	seen := map[string]bool{}
	if hasChinese(body) {
		preamble.WriteString("\\usepackage{ctex}\n")
		seen["ctex"] = true
	}
	packages := stringsOpt(options["tex_packages"])
	if packages == nil {
		packages = stringsOpt(options["packages"])
	}
	for _, name := range packages {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(name) || seen[name] || name == "luadraw" {
			continue
		}
		preamble.WriteString("\\usepackage{" + name + "}\n")
		seen[name] = true
	}
	if p := strOpt(options, "preamble", ""); p != "" {
		preamble.WriteString(p)
		if !strings.HasSuffix(p, "\n") {
			preamble.WriteByte('\n')
		}
	}
	if customPreamble != "" {
		preamble.WriteString(customPreamble)
		if !strings.HasSuffix(customPreamble, "\n") {
			preamble.WriteByte('\n')
		}
	}

	threeD := strings.Contains(body, ".graph3d") || strings.Contains(body, "graph3d:") || strings.Contains(body, "pt3d")
	ldOptions := ""
	if threeD {
		ldOptions = "3d"
	}
	var out strings.Builder
	out.WriteString("\\documentclass[tikz,border=2pt]{standalone}\n")
	out.WriteString(preamble.String())
	out.WriteString("\\usepackage[" + ldOptions + "]{luadraw}\n")
	out.WriteString("\\begin{document}\n\\begin{luadraw}{name=submitted}\n")
	out.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		out.WriteByte('\n')
	}
	out.WriteString("\\end{luadraw}\n\\end{document}\n")
	return out.String()
}

func hasChinese(s string) bool {
	for _, r := range s {
		if r >= 0x2e80 && r <= 0x9fff {
			return true
		}
	}
	return false
}
