// Package deliver 是装配交付管道：漂移闸门 → 归集制品 → 生成主文件 →
// 书签注入 → standalone 内联 → 资源盘点。全部确定性操作；LLM 不参与装配本体
// （终审是另一个话题，本管道只管"把过验收的制品组装成能编译的书"）。
package deliver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/LingNc/DocVision/skills/kernel/internal/assemble"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/texcheck"
	"github.com/LingNc/DocVision/skills/kernel/internal/verify"
)

// Report 是交付管道的结构化回执。
type Report struct {
	BuildDir    string                 `json:"build_dir"`
	Chapters    []string               `json:"chapters"`     // 归集的章节 .tex（排序）
	Figures     []string               `json:"figures"`      // 归集的矢量图 .tex
	Drift       []verify.ArtifactDrift `json:"drift"`        // 漂移明细（clean=false 时非空）
	Clean       bool                   `json:"clean"`        // 漂移闸门：false = 拒装配
	Preflight   assemble.PreflightReport `json:"preflight"`  // 装配后资源盘点
	MainTex     string                 `json:"main_tex"`     // 主文件路径
	Standalone  string                 `json:"standalone"`   // standalone.tex 路径
	Bookmarks   int                    `json:"bookmarks_injected"` // 注入的书签数（估算）
}

// Deliver 跑完整交付管道。漂移闸门不过时返回 Clean=false 且不动 build 树。
// clsName 是样式类名（可选；给了才拷 style/ 进构建树）。
func Deliver(q *queue.Queue, projectRoot, clsName string) (*Report, error) {
	rep := &Report{BuildDir: filepath.Join(projectRoot, "build")}

	// ① 漂移闸门：验收后被改/删的产物一律拒装配（手滑不该进书）。
	drifts := verify.CheckDrift(q.Tasks, projectRoot)
	for _, d := range drifts {
		if d.NoSnapshot {
			continue // 无快照无法复核，不拦（记进报告由人看）
		}
		if len(d.Mismatched) > 0 || len(d.Missing) > 0 {
			rep.Drift = append(rep.Drift, d)
		}
	}
	rep.Clean = len(rep.Drift) == 0
	if !rep.Clean {
		return rep, nil
	}

	// ② 归集：done 的 conv_* 任务 out/*.tex → build/chapters/（含分片子目录）；
	//         done 的 drw_* 任务 out/figure.tex → build/figures/<stem>.tex。
	chDir := filepath.Join(rep.BuildDir, "chapters")
	figDir := filepath.Join(rep.BuildDir, "figures")
	for _, d := range []string{chDir, figDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return rep, err
		}
	}
	var convIDs []string
	for id, t := range q.Tasks {
		if t.Status == task.StatusDone && t.Type == task.TypeConvert {
			convIDs = append(convIDs, id)
		}
	}
	sort.Strings(convIDs)
	for _, id := range convIDs {
		t := q.Tasks[id]
		stem := strings.TrimPrefix(id, "conv_")
		outDir := filepath.Join(projectRoot, t.Sandbox, "out")
		if err := filepath.Walk(outDir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".tex") {
				return err
			}
			rel, _ := filepath.Rel(outDir, p)
			dst := filepath.Join(chDir, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := copyFile(p, dst); err != nil {
				return err
			}
			if filepath.Base(p) == t.Accept["main_file"] {
				rep.Chapters = append(rep.Chapters, filepath.ToSlash(rel))
			}
			return nil
		}); err != nil {
			return rep, err
		}
		_ = stem
	}
	var drawIDs []string
	for id, t := range q.Tasks {
		if t.Status == task.StatusDone && t.Type == task.TypeDraw {
			drawIDs = append(drawIDs, id)
		}
	}
	sort.Strings(drawIDs)
	for _, id := range drawIDs {
		t := q.Tasks[id]
		stem := strings.TrimPrefix(id, "drw_")
		src := filepath.Join(projectRoot, t.Sandbox, "out", "figure.tex")
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(figDir, stem+".tex")
		if err := copyFile(src, dst); err != nil {
			return rep, err
		}
		rep.Figures = append(rep.Figures, stem+".tex")
	}

	// ③ 主文件：documentclass + 按序 \input 章节。
	// SK-I24：书含 CJK 时 book+pdflatex 必挂——扫描章节内容，CJK 用 ctexbook
	// （引擎链 pdflatex→xelatex 自适配；luadraw 章仍由 NeedsLuaLaTeX 按需强制 lualatex）。
	mainPath := filepath.Join(rep.BuildDir, "main.tex")
	var cjkScan []byte
	for _, ch := range rep.Chapters {
		if cb, rerr := os.ReadFile(filepath.Join(projectRoot, ch)); rerr == nil {
			cjkScan = append(cjkScan, cb...)
		}
	}
	docclass, _ := texcheck.FragmentDocclass(cjkScan)
	var b strings.Builder
	b.WriteString("\\documentclass{" + docclass + "}\n")
	if clsName != "" {
		// 样式类进构建树
		clsSrc := filepath.Join(projectRoot, "style", clsName+".cls")
		if _, err := os.Stat(clsSrc); err == nil {
			_ = copyFile(clsSrc, filepath.Join(rep.BuildDir, clsName+".cls"))
			fmt.Fprintf(&b, "\\usepackage{%s}\n", clsName)
		}
	}
	b.WriteString("\\begin{document}\n")
	for _, ch := range rep.Chapters {
		name := strings.TrimSuffix(filepath.Base(ch), ".tex")
		fmt.Fprintf(&b, "\\input{chapters/%s}\n", name)
	}
	b.WriteString("\\end{document}\n")
	if err := os.WriteFile(mainPath, []byte(b.String()), 0o644); err != nil {
		return rep, err
	}
	rep.MainTex = mainPath

	// ④ input@path + 书签 + standalone
	if err := assemble.EnsureInputPath(mainPath); err != nil {
		return rep, err
	}
	if err := assemble.EnsurePDFBookmarks(projectRoot, rep.BuildDir); err != nil {
		return rep, err
	}
	rep.Bookmarks = countLinesContaining(mainPath, "\\pdfbookmark")
	standalone, err := assemble.BuildStandaloneTex(mainPath)
	if err != nil {
		return rep, err
	}
	rep.Standalone = filepath.Join(rep.BuildDir, "standalone.tex")
	if err := os.WriteFile(rep.Standalone, []byte(standalone), 0o644); err != nil {
		return rep, err
	}

	// ⑤ 装配后资源盘点
	rep.Preflight = assemble.PreflightAssemble(rep.BuildDir, clsName)
	return rep, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func countLinesContaining(path, sub string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, ln := range strings.Split(string(data), "\n") {
		if strings.Contains(ln, sub) {
			n++
		}
	}
	return n
}
