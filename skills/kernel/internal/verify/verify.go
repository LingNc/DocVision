// Package verify 实现验收器：机械执行验收 spec，只看磁盘事实，不看 agent 自述。
// 内置检查可组合；编译/覆盖率等重型检查以 Hook 形式注入（由具体阶段装配）。
package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// Hook 是重型检查注入点：返回 nil 表示通过，否则返回错误清单。
type Hook func(t *task.Task, projectRoot string) []string

// Checker 是验收器。
type Checker struct {
	ProjectRoot string
	// CompileHook 编译检查（未装配时跳过编译类验收）。
	CompileHook Hook
	// CoverageHook 覆盖率检查。
	CoverageHook Hook
}

// forbiddenRef 匹配指向临时/沙箱内部路径的引用（自包含判据）。
var forbiddenRef = regexp.MustCompile(`(_subdivision/|node/result|work/temp/|\.gate/)`)

// Run 对任务跑一次完整验收，返回错误清单（空 = 通过）。
// 规则：任一检查不过即整体不过；spec 里没有的要求不检查。
func (c *Checker) Run(t *task.Task) []string {
	var errs []string
	root := c.ProjectRoot

	// 1. 沙箱越界：out/ 下所有文件必须在任务沙箱内。
	sandboxAbs := filepath.Join(root, t.Sandbox)
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
	if main, ok := t.Accept["main_file"].(string); ok && main != "" {
		b, err := os.ReadFile(filepath.Join(sandboxAbs, "out", main))
		if err == nil && forbiddenRef.Match(b) {
			errs = append(errs, "主产物引用临时路径（必须自包含）: "+main)
		}
	}

	// 4. 重型检查（装配了才跑）。
	if t.Accept["require_compile"] == true && c.CompileHook != nil {
		errs = append(errs, c.CompileHook(t, root)...)
	}
	if min, ok := t.Accept["min_coverage"].(float64); ok && min > 0 && c.CoverageHook != nil {
		errs = append(errs, c.CoverageHook(t, root)...)
	}
	return errs
}
