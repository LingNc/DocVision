// Package envcheck 是操作前环境体检（用户要求）：工具链齐备性 + 字体情况，
// 产出可操作的提示（缺什么、怎么装、字体缺失怎么回退/向用户索要）。
// 纯检查零副作用；所有外部命令可注入（Runner）以便测试。
package envcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/LingNc/DocVision/skills/kernel/internal/prep"
)

// ToolCheck 是一个外部工具的体检结果。
type ToolCheck struct {
	Name    string `json:"name"`
	Needed  string `json:"needed"` // 必需/可选
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Hint    string `json:"hint,omitempty"` // 缺失时的可操作建议
}

// FontReport 是字体体检。
type FontReport struct {
	ProjectFontDir string   `json:"project_font_dir"` // 项目字体目录（style 特殊字体放这里）
	ProjectFonts   []string `json:"project_fonts"`    // 项目内已备字体
	FcListFound    bool     `json:"fc_list_found"`      // fontconfig 可用
	CJKCount       int      `json:"cjk_count"`          // 系统 CJK 字体数（0=中文排版有风险）
	Fallback       string   `json:"fallback"`           // 缺失时的回退字体（config 可改）
}

// Report 是完整体检报告。
type Report struct {
	Tools []ToolCheck `json:"tools"`
	Fonts FontReport  `json:"fonts"`
	Hints []string    `json:"hints"` // 汇总行动项（按优先级）
}

// 字体搜索站点（放在提示里给用户）：中文识别用求字体网，西文用 WhatFontIs，
// 开源替代用 Google Fonts（思源系列）。
const fontSearchHint = "字体可让用户在搜索网站识别/获取：求字体网 qiuziti.com（截图识别中文）、WhatFontIs.com（西文）、Google Fonts（思源黑体/宋体开源替代）；用户提供后放入项目 fonts/ 目录，或先回退默认字体。"

// Check 跑体检。projectRoot 用于查项目 fonts/ 目录与 config（可为空）。
func Check(projectRoot string, run prep.Runner) *Report {
	if run == nil {
		run = prep.DefaultRunner
	}
	rep := &Report{}
	look := func(name string) (string, bool) {
		p, err := exec.LookPath(name)
		return p, err == nil
	}
	add := func(name, needed, hint string) {
		path, ok := look(name)
		tc := ToolCheck{Name: name, Needed: needed, Found: ok, Path: path, Hint: hint}
		if ok && name == "pdflatex" {
			if out, err := run(name, "--version"); err == nil {
				tc.Version = firstLine(string(out))
			}
		}
		rep.Tools = append(rep.Tools, tc)
	}
	add("pdfinfo", "必需", "poppler-utils：apt/brew install poppler-utils")
	add("pdftotext", "必需", "poppler-utils（覆盖率判定依赖）")
	add("pdftoppm", "必需", "poppler-utils（页渲染依赖）")
	add("pdfseparate", "可选", "poppler-utils（旧切分兜底用）")
	add("pdfunite", "可选", "poppler-utils（旧切分兜底用）")
	add("mutool", "可选", "mupdf-tools：切分提速后端（无则 pdfcpu）")
	add("pdflatex", "必需", "TeX Live：apt install texlive-full 或 texlive-xetex")
	add("mineru", "必需", "MinerU：pip install -U magic-pdf[full]（API token 配在 MinerU 自己的 config）")
	add("python3", "必需", "MinerU 运行环境")

	// 字体体检
	fr := FontReport{Fallback: "Tex Gyre Termes（泰X默认，config font_family 可改）"}
	if projectRoot != "" {
		fr.ProjectFontDir = "fonts/"
		fd := filepath.Join(projectRoot, "fonts")
		if entries, err := os.ReadDir(fd); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					fr.ProjectFonts = append(fr.ProjectFonts, e.Name())
				}
			}
		}
	}
	if p, ok := look("fc-list"); ok {
		fr.FcListFound = true
		out, err := run(p, ":lang=zh", "family")
		if err == nil {
			seen := map[string]bool{}
			for _, ln := range strings.Split(string(out), "\n") {
				f := strings.TrimSpace(strings.SplitN(ln, ",", 2)[0])
				if f != "" {
					seen[f] = true
				}
			}
			fr.CJKCount = len(seen)
		}
	}
	rep.Fonts = fr

	// 汇总行动项
	for _, tc := range rep.Tools {
		if !tc.Found && tc.Needed == "必需" {
			rep.Hints = append(rep.Hints, "[缺] "+tc.Name+"（必需）："+tc.Hint)
		}
	}
	if rep.Fonts.CJKCount == 0 && rep.Fonts.FcListFound {
		rep.Hints = append(rep.Hints, "[警] 系统无中文字体：中文排版可能出方框——建议装思源/Noto CJK；"+fontSearchHint)
	}
	if rep.Fonts.FcListFound {
		if rep.Fonts.CJKCount < 3 {
			rep.Hints = append(rep.Hints, "[提示] CJK 字体较少（"+itoa(rep.Fonts.CJKCount)+" 款）：样式阶段若需特殊字体（书名/花体），"+fontSearchHint)
		}
	} else {
		rep.Hints = append(rep.Hints, "[警] 无 fontconfig（fc-list 不存在）：无法枚举字体，样式阶段请先装 fontconfig 或直接向用户索要字体文件放入 fonts/")
	}
	if len(rep.Fonts.ProjectFonts) > 0 {
		rep.Hints = append(rep.Hints, "[好] 项目 fonts/ 已备 "+itoa(len(rep.Fonts.ProjectFonts))+" 个字体文件：样式会话优先使用它们")
	}
	// 缺工具但未致阻断时的回退说明
	for _, tc := range rep.Tools {
		if !tc.Found && tc.Needed == "可选" {
			rep.Hints = append(rep.Hints, "[可] "+tc.Name+"（可选）缺失："+tc.Hint+"——缺失时自动走替代路径，不阻断")
		}
	}
	return rep
}

// firstLine 取版本输出首行。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
