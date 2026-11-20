// Package verify 实现验收器：机械执行验收 spec，只看磁盘事实，不看 agent 自述。
// 重型判据（编译/覆盖率）默认装配 texcheck 实现，工具缺失时跳过并记 note（不卡死整本书）。
package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/texcheck"
)

// Hook 是重型检查注入点：返回 nil 表示通过，否则返回错误清单。
type Hook func(t *task.Task, projectRoot string) []string

// Checker 是验收器。
type Checker struct {
	ProjectRoot string
	// CoverageSource 覆盖率对照源 md 路径（相对沙箱；未配置时覆盖率检查跳过）。
	CoverageSource string
	// CompileHook 编译检查覆盖（测试注入用）；默认真实跑 pdflatex。
	CompileHook Hook
	// SkillRoot 技能根（luadraw 模块目录的上级；SK-I16 预编译回退用）。空=回退路径不生效。
	SkillRoot string
}

// WithSkillRoot 链式设置技能根（main/mcp 装配时调用）。
func (c *Checker) WithSkillRoot(root string) *Checker {
	c.SkillRoot = root
	return c
}

// NewChecker 装配默认重型判据的验收器。
func NewChecker(projectRoot string) *Checker {
	return &Checker{ProjectRoot: projectRoot, CoverageSource: "inputs/chapter.md"}
}

// forbiddenRef 匹配指向临时/沙箱内部路径的引用（自包含判据）。
var forbiddenRef = regexp.MustCompile(`(_subdivision/|node/result|work/temp/|\.gate/)`)

// Run 对任务跑一次完整验收。
// 返回错误清单（空 = 通过）与备注（覆盖率放行说明、哈希快照等，进 verdict.Notes）。
// 规则：spec 里没有的要求不检查；任一硬检查不过即整体不过。
func (c *Checker) Run(t *task.Task) ([]string, map[string]string) {
	var errs []string
	notes := map[string]string{}
	root := c.ProjectRoot
	sandboxAbs := filepath.Join(root, t.Sandbox)

	// 1. 沙箱越界：out/ 下所有文件必须在任务沙箱内。
	filepath.Walk(filepath.Join(sandboxAbs, "out"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(sandboxAbs, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			errs = append(errs, "产物越出任务沙箱: "+p)
		}
		return nil
	})

	// 2. spec 要求的产物文件必须存在。
	if want, ok := t.Accept["required_files"].([]any); ok {
		for _, f := range want {
			p := filepath.Join(sandboxAbs, "out", fmt.Sprint(f))
			if _, err := os.Stat(p); os.IsNotExist(err) {
				errs = append(errs, "缺少产物: out/"+fmt.Sprint(f))
			}
		}
	}

	// 3. 自包含判据：主产物不得引用临时路径。
	main := ""
	if m, ok := t.Accept["main_file"].(string); ok {
		main = m
	}
	var mainAbs string
	if main != "" {
		mainAbs = filepath.Join(sandboxAbs, "out", main)
		b, err := os.ReadFile(mainAbs)
		if err != nil {
			errs = append(errs, "主产物不可读: "+main)
		} else if forbiddenRef.Match(b) {
			errs = append(errs, "主产物引用临时路径（必须自包含）: "+main)
		}
	}

	// 4. 编译检查（工具缺失跳过记 note，不当作失败）。
	if t.Accept["require_compile"] == true && main != "" {
		hook := c.CompileHook
		if hook == nil {
			hook = c.defaultCompile
		}
		errs = append(errs, hook(t, root)...)
	}

	// 5. 覆盖率检查：第一次不达标打回，第二次放行并记缺口（T168 判据）。
	if t.Accept["min_coverage"] != false {
		if cerrs, cnotes := c.runCoverage(t, root, mainAbs); len(cerrs) > 0 {
			errs = append(errs, cerrs...)
		} else {
			for k, v := range cnotes {
				notes[k] = v
			}
		}
	}

	// 6. verdict.json 模式校验（T1 classify / T4 check）。
	errs = append(errs, c.checkVerdictSchema(t, sandboxAbs)...)

	// 7. T2 作图数值判据（figure_ref 触发：软墨迹对齐 + mm 尺寸 + accept_min_submits）。
	ferrs, fnotes := c.checkFigure(t, sandboxAbs)
	errs = append(errs, ferrs...)
	for k, v := range fnotes {
		notes[k] = v
	}

	// 7. out/ 哈希快照进 notes（assemble 复核"acceptance 后未被篡改"）。
	if hashes, err := texcheck.Sha256Dir(filepath.Join(sandboxAbs, "out")); err == nil {
		if b, err := json.Marshal(hashes); err == nil {
			notes["artifact_sha256"] = string(b)
		}
	}
	return errs, notes
}

// runCoverage 覆盖率判定的完整流程，返回（打回错误清单, 备注）。
// t.Attempts==0 时第一次不达标打回；≥1 时放行并记缺口说明。
func (c *Checker) runCoverage(t *task.Task, projectRoot, mainAbs string) ([]string, map[string]string) {
	if c.CoverageSource == "" {
		return nil, nil
	}
	mdPath := filepath.Join(projectRoot, t.Sandbox, c.CoverageSource)
	if _, err := os.Stat(mdPath); err != nil {
		return nil, nil // 无对照源（如图元任务）跳过
	}
	pdfPath := mainAbs
	if pdfPath == "" {
		pdfPath = filepath.Join(projectRoot, t.Sandbox, "out", "main.pdf")
	} else {
		pdfPath = pdfPath[:len(pdfPath)-len(filepath.Ext(pdfPath))] + ".pdf"
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return nil, map[string]string{"coverage_skip": "无 PDF 产物，跳过覆盖率"}
	}
	cov, missing, err := texcheck.ChapterCoverage(mdPath, pdfPath)
	if err != nil {
		// pdftotext 不可用：跳过并记 note（旧线判据：不能因缺工具卡死整本书）。
		return nil, map[string]string{"coverage_skip": err.Error()}
	}
	if cov >= texcheck.MinChapterCoverage {
		return nil, map[string]string{"coverage": fmt.Sprintf("%.2f", cov)}
	}
	if texcheck.CoverageDecision(cov, t.Attempts) {
		return []string{fmt.Sprintf("内容覆盖率 %.0f%% 低于 %.0f%% 阈值，缺得最多的是：%v（第一次打回，补全后再交）",
			cov*100, texcheck.MinChapterCoverage*100, missing)}, nil
	}
	return nil, map[string]string{"coverage_note": texcheck.CoverageGapNote(cov, missing, "")}
}

// defaultCompile 默认编译检查：真实跑引擎链；章内含 luadraw 时按需强制 lualatex。
func (c *Checker) defaultCompile(t *task.Task, projectRoot string) []string {
	main := fmt.Sprint(t.Accept["main_file"])
	outDir := filepath.Join(projectRoot, t.Sandbox, "out")
	// 绘图风格模板（SK-I13）：style 阶段产出 book/style/draw-style.tex 后自动生效——
	// 主文件必须引用模板（\input draw-style 或用其 \dv 宏），散写打回；老项目无模板则跳过。
	if data, err := os.ReadFile(filepath.Join(projectRoot, "style", "draw-style.tex")); err == nil && len(data) > 0 {
		mainData, rerr := os.ReadFile(filepath.Join(outDir, main))
		if rerr == nil {
			body := string(mainData)
			if !strings.Contains(body, "draw-style") && !strings.Contains(body, `\dvLine`) {
				return []string{"绘图风格模板 style/draw-style.tex 已产出但主文件未引用（须 \\input 该模板或用其 \\dvLine 宏）——逐图散写一律打回"}
			}
		}
	}
	// SK-I24：章节是装配片段（无 \documentclass），不能直接编译——包一层 stub 书主文件再编；
	// 否则有 TeX 的机器上 convert 章（\chapter 正文）按判据必挂（无 TeX 机器跳过所以假绿）。
	compileDir, compileMain := outDir, main
	stubEngine := ""
	if b, rerr := os.ReadFile(filepath.Join(outDir, main)); rerr == nil && !strings.Contains(string(b), `\documentclass`) {
		stubDir := filepath.Join(projectRoot, t.Sandbox, "_compile")
		if err := os.MkdirAll(stubDir, 0o755); err != nil {
			return []string{"编译 stub 目录创建失败: " + err.Error()}
		}
		rel, rerr := filepath.Rel(stubDir, filepath.Join(outDir, main))
		if rerr != nil {
			rel = filepath.Join(outDir, main)
		}
		// CJK → ctexbook + 强制 xelatex（公共助手；luadraw 的 lualatex 优先级更高，见下）
		docclass, forcedEngine := texcheck.FragmentDocclass(b)
		stub := "\\documentclass{" + docclass + "}\n" +
			"\\usepackage[T1]{fontenc}\n" +
			"\\usepackage{tikz}\n" +
			"\\usetikzlibrary{arrows.meta}\n" +
			"\\usepackage{graphicx}\n" +
			"\\begin{document}\n" +
			"\\input{" + filepath.ToSlash(rel) + "}\n" +
			"\\end{document}\n"
		if err := os.WriteFile(filepath.Join(stubDir, "main.tex"), []byte(stub), 0o644); err != nil {
			return []string{"编译 stub 写入失败: " + err.Error()}
		}
		compileDir, compileMain = stubDir, "main.tex"
		if forcedEngine != "" {
			stubEngine = forcedEngine
		}
	}
	// 引擎双路探测（SK-I16）：luadraw 章——有 lualatex 直编；无则 lua5.4 预编译成 TikZ 走正常链。
	filesDir := filepath.Join(c.SkillRoot, "docs", "luadraw", "files")
	engine, _, derr := texcheck.DecideEngine(outDir, main, filesDir)
	if derr != nil {
		if strings.Contains(derr.Error(), "二选一安装") {
			return nil // 缺工具按既有哲学跳过（报错指引交给 doctor/日志）
		}
		return []string{"luadraw 引擎决策失败: " + derr.Error()}
	}
	// SK-I23：style/ 模板目录进 TEXINPUTS——否则沙箱内 \input{draw-style} 编译找不到文件	（对方会话装 TeX 机器实跑暴露：File 'draw-style' not found）。
	// 优先级：luadraw 强制的 lualatex > CJK 强制的 xelatex（lualatex 天然处理 CJK，
	// 被强转 xelatex 反而坏 luadraw 图——SK-I24 审查修正）；仅自动回退链（engine 空）时应用 CJK 强制。
	if stubEngine != "" && engine == "" {
		engine = stubEngine
	}
	sep := string(os.PathListSeparator)
	env := map[string]string{"TEXINPUTS": filepath.Join(c.ProjectRoot, "style") + sep + outDir + sep}
	if _, err := texcheck.CompileEngineEnv(compileDir, compileMain, engine, env); err != nil {
		if strings.Contains(err.Error(), "不可用") {
			return nil // 缺工具跳过（与覆盖率同款哲学）
		}
		return []string{"编译失败: " + err.Error()}
	}
	return nil
}
