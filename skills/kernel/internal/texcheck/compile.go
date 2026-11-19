package texcheck

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

// Compile 在 outDir 里对 mainFile 跑默认引擎链（pdflatex→xelatex 回退，两遍交叉引用稳定）。
// 需要特定引擎（如 luadraw 的 lualatex）时用 CompileEngine 显式指定。
func Compile(outDir, mainFile string) (string, error) {
	return CompileEngine(outDir, mainFile, "")
}

// CompileEngine 指定引擎编译；engine="" 走默认回退链。
// lualatex 只按需强制（luadraw 图所在章），不含 luadraw 的章不强制——用户要求按需用引擎。
func CompileEngine(outDir, mainFile, engine string) (string, error) {
	if engine == "" {
		engine = "pdflatex"
		if _, err := exec.LookPath(engine); err != nil {
			if _, err2 := exec.LookPath("xelatex"); err2 == nil {
				engine = "xelatex" // 引擎回退：无 pdflatex 时 xelatex 顶上（参数兼容）
			} else {
				return "", fmt.Errorf("pdflatex/xelatex 均不可用（TeX Live）: %w", err)
			}
		}
	} else {
		if _, err := exec.LookPath(engine); err != nil {
			return "", fmt.Errorf("引擎 %s 不可用: %w（按需引擎——安装 texlive 对应引擎或改用不依赖它的画法）", engine, err)
		}
	}
	runs := 2
	if engine == "tectonic" {
		runs = 1 // Tectonic 自行处理交叉引用重排
	}
	for i := 0; i < runs; i++ {
		var cmd *exec.Cmd
		if engine == "tectonic" {
			cmd = exec.Command(engine, "--keep-intermediates", mainFile)
		} else {
			cmd = exec.Command(engine, "-interaction=nonstopmode", "-halt-on-error", mainFile)
		}
		cmd.Dir = outDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s 失败: %v\n%.800s", engine, err, out)
		}
	}
	pdf := filepath.Join(outDir, mainFile[:len(mainFile)-len(filepath.Ext(mainFile))]+".pdf")
	if _, err := os.Stat(pdf); err != nil {
		return "", fmt.Errorf("编译未产出 PDF: %s", pdf)
	}
	return pdf, nil
}

// luadrawUseRe 匹配 \usepackage[...]{luadraw}（带可选参数，如 [3d]）。
var luadrawUseRe = regexp.MustCompile(`\\usepackage(\[[^\]]*\])?\{luadraw\}`)

// NeedsLuaLaTeX 探测该章是否需要 LuaLaTeX：主文件或同目录任何 .tex 用了 luadraw。
// luadraw 是纯 LuaLaTeX 包；按需引擎——只强制需要的章。
func NeedsLuaLaTeX(dir, mainFile string) bool {
	check := func(path string) bool {
		data, err := os.ReadFile(path)
		return err == nil && luadrawUseRe.Match(data)
	}
	if check(filepath.Join(dir, mainFile)) {
		return true
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tex"))
	for _, m := range matches {
		if filepath.Base(m) == mainFile {
			continue
		}
		if check(m) {
			return true
		}
	}
	return false
}

// Sha256File 算单个文件哈希（验收时记进 verdict，assemble 阶段复核"产物自 acceptance 后未被篡改"）。
func Sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Sha256Dir 递归算目录下全部文件哈希，键为相对路径（验收快照）。
func Sha256Dir(root string) (map[string]string, error) {
	hashes := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		h, err := Sha256File(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		hashes[rel] = h
		return nil
	})
	return hashes, err
}
