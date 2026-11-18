package assemble

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// inventory.go：装配前资源盘点（旧线 preflightAssemble 移植）。
// 旧线只写日志；新形态返回结构化报告，由调用方决定怎么呈现（不中止——缺图
// 不该毁掉整本书，T33 同款理由）。

// PreflightReport 是资源盘点结果。
type PreflightReport struct {
	ClsOK       bool     // 样式类就位
	Chapters    int      // 章节 .tex 数
	Figures     int      // figures/ 文件数
	Images      int      // images/ 文件数
	RefsTotal   int      // \includegraphics 引用总数
	RefsMissing []string // 解析不到的引用（章节.tex: 路径）
}

// includeGraphicsRe 抽 \includegraphics[…]{path} 的路径（旧线逐字）。
var includeGraphicsRe = regexp.MustCompile(`\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}`)

// PreflightAssemble 盘点构建树：cls 就位、章节/图目录计数、插图引用逐条解析。
// 缺失逐条列进报告但不中止。
func PreflightAssemble(buildDir, clsName string) PreflightReport {
	countDir := func(rel string) int {
		n := 0
		root := filepath.Join(buildDir, rel)
		_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	rep := PreflightReport{
		Figures: countDir("figures"),
		Images:  countDir("images"),
		ClsOK:   fileExists(filepath.Join(buildDir, clsName+".cls")),
	}
	texs, _ := filepath.Glob(filepath.Join(buildDir, "chapters", "*.tex"))
	rep.Chapters = len(texs)

	// \includegraphics（可带 [选项]）引用解析检查。
	for _, tex := range texs {
		data, err := os.ReadFile(tex)
		if err != nil {
			continue
		}
		for _, m := range includeGraphicsRe.FindAllStringSubmatch(string(data), -1) {
			rep.RefsTotal++
			rel := m[1]
			// 章节 .tex 在 buildDir/chapters/ 下；graphicspath 声明 figures/，
			// 引用也可能是相对 chapters/ 或树内绝对相对路径（images/…）。
			cands := []string{
				filepath.Join(buildDir, rel),
				filepath.Join(buildDir, "figures", rel),
				filepath.Join(filepath.Dir(tex), rel),
			}
			found := false
			for _, c := range cands {
				if fileExists(c) {
					found = true
					break
				}
			}
			if !found {
				rep.RefsMissing = append(rep.RefsMissing, filepath.Base(tex)+": "+rel)
			}
		}
	}
	return rep
}

// inputPathLine 让章节里的 \input{chapter_00X/children/...} 这类"相对 chapters/
// 的路径"在构建树里解析得到（T324）：主文件在构建根，子文件之间按 chapters/ 内的
// 相对路径互相引用；不给 \input@path，全书就只能靠修复会话逐个改路径——每次
// assemble 都要重烧一遍，因为修复只在临时构建树里生效。
const inputPathLine = "\\makeatletter\\def\\input@path{{chapters/}}\\makeatother\n"

// EnsureInputPath 幂等地把 \input@path 注进 main.tex（必须在 \begin{document} 前）。
func EnsureInputPath(mainPath string) error {
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return err
	}
	main := string(data)
	if strings.Contains(main, "\\input@path") || main == "" {
		return nil
	}
	if i := strings.Index(main, "\n"); i >= 0 {
		main = main[:i+1] + inputPathLine + main[i+1:]
	} else {
		main = inputPathLine + main
	}
	return os.WriteFile(mainPath, []byte(main), 0o644)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
