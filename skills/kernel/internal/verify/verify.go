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
	engine := ""
	if texcheck.NeedsLuaLaTeX(outDir, main) {
		engine = "lualatex" // 按需引擎：只强制 luadraw 章
	}
	if _, err := texcheck.CompileEngine(outDir, main, engine); err != nil {
		if strings.Contains(err.Error(), "不可用") {
			return nil // 缺工具跳过（与覆盖率同款哲学）
		}
		return []string{"编译失败: " + err.Error()}
	}
	return nil
}
