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
	"strings"
)

// Compile 在 outDir 里对 mainFile 跑默认引擎链（pdflatex→xelatex 回退，两遍交叉引用稳定）。
// 需要特定引擎（如 luadraw 的 lualatex）时用 CompileEngine 显式指定。
func Compile(outDir, mainFile string) (string, error) {
	return CompileEngine(outDir, mainFile, "")
}

// CompileEngine 指定引擎编译；engine="" 走默认回退链。
// lualatex 只按需强制（luadraw 图所在章），不含 luadraw 的章不强制——用户要求按需用引擎。
func CompileEngine(outDir, mainFile, engine string) (string, error) {
	return CompileEngineEnv(outDir, mainFile, engine, nil)
}

// CompileEngineEnv 同 CompileEngine，附加环境变量（如 TEXINPUTS 让沙箱编译找到
// 项目级 style/ 模板——SK-I13 强制引用 draw-style 后，不把模板目录加进搜索路径会
// 编译报 File 'draw-style' not found；11-20 对方会话在装有 TeX 的机器实跑暴露）。
// env 的 TEXINPUTS 值会与现存 TEXINPUTS 前置合并（保留默认搜索路径）。
func CompileEngineEnv(outDir, mainFile, engine string, env map[string]string) (string, error) {
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
		if len(env) > 0 {
			cmd.Env = os.Environ()
			for k, v := range env {
				if k == "TEXINPUTS" {
					// 前置合并：新路径优先，尾冒号保留默认搜索路径
					if old := os.Getenv("TEXINPUTS"); old != "" {
						v = v + ":" + old
					} else {
						v = v + ":"
					}
				}
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}
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
var luadrawUseRe = regexp.MustCompile(`\\usepackage(\[[^\]]*\])?\{luadraw\}|\\begin\{luadraw\}`)

// NeedsLuaLaTeX 探测该章是否需要 LuaLaTeX：主文件或同目录任何 .tex 用了 luadraw。
// luadraw 是纯 LuaLaTeX 包；按需引擎——只强制需要的章。
func NeedsLuaLaTeX(dir, mainFile string) bool {
	check := func(path string) bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		// 注释行不算使用：预编译回退保留原块为 % 注释，若把注释当使用会永远"需要 lualatex"，
		// 幂等标记失效（DecideEngine 的"预编译后仍存在"误判）。
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "%") {
				continue
			}
			if luadrawUseRe.MatchString(line) {
				return true
			}
		}
		return false
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

// FragmentDocclass 判定片段/全书主文件的文档类与强制引擎（SK-I24 抽公共）：
// 正文含 CJK（rune ≥ 0x2E80）时 pdflatex 直编中文必挂——ctexbook + 强制 xelatex；
// 纯 ASCII/西欧 → book + 空引擎（走默认回退链）。luadraw 章的 lualatex 优先级
// 高于本强制（lualatex 天然处理 CJK），调用方只在 engine 为空时应用本强制。
func FragmentDocclass(content []byte) (docclass, engine string) {
	for _, r := range string(content) {
		if r >= 0x2E80 {
			return "ctexbook", "xelatex"
		}
	}
	return "book", ""
}
