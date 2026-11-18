package texcheck

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Compile 在 outDir 里对 mainFile 跑 pdflatex（两遍，交叉引用稳定）。
// 返回产物 PDF 路径。pdflatex 不可用返回错误（调用方跳过判定，不因缺工具卡死）。
func Compile(outDir, mainFile string) (string, error) {
	engine := "pdflatex"
	if _, err := exec.LookPath(engine); err != nil {
		if _, err2 := exec.LookPath("xelatex"); err2 == nil {
			engine = "xelatex" // 引擎回退：无 pdflatex 时 xelatex 顶上（参数兼容）
		} else {
			return "", fmt.Errorf("pdflatex/xelatex 均不可用（TeX Live）: %w", err)
		}
	}
	for i := 0; i < 2; i++ {
		cmd := exec.Command(engine, "-interaction=nonstopmode", "-halt-on-error", mainFile)
		cmd.Dir = outDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("pdflatex 失败: %v\n%.800s", err, out)
		}
	}
	pdf := filepath.Join(outDir, mainFile[:len(mainFile)-len(filepath.Ext(mainFile))]+".pdf")
	if _, err := os.Stat(pdf); err != nil {
		return "", fmt.Errorf("编译未产出 PDF: %s", pdf)
	}
	return pdf, nil
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
