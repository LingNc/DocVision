package prep

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MinerUConfig 是 MinerU 调用的参数。
type MinerUConfig struct {
	// Command 是 MinerU 可执行名（默认 "mineru"）。
	Command string
	// ExtraArgs 追加参数（如 -l 语言、设备开关）。
	ExtraArgs []string
}

// MinerUResult 是一次 MinerU 调用的产物定位。
type MinerUResult struct {
	AutoDir       string // MinerU 的 auto 输出目录（<mineruOutput>/<name>/auto）
	ContentList   string // *_content_list.json（优先非 v2）
	LayoutJSON    string // layout.json
	OriginPDFName string // part 文件名（调用方需自行拷为 *_origin.pdf）
}

// RunMinerU 对单个 part PDF 跑 MinerU（vLLM 模式：mineru -p <pdf> [-d <device>]）。
// MinerU 会自己挑输出目录（<mineruOutput>/<part名>/auto/）；本函数只负责调用与
// 产物定位，不猜内部布局——content_list/layout 用 glob 找。
func RunMinerU(partPDF, mineruOutput string, cfg MinerUConfig, run Runner) (*MinerUResult, error) {
	if run == nil {
		run = DefaultRunner
	}
	cmd := cfg.Command
	if cmd == "" {
		cmd = "mineru"
	}
	if _, err := os.Stat(partPDF); err != nil {
		return nil, fmt.Errorf("part PDF 不存在: %s", partPDF)
	}
	args := append([]string{"-p", partPDF}, cfg.ExtraArgs...)
	if out, err := run(cmd, args...); err != nil {
		return nil, fmt.Errorf("%s 失败: %v %.300s", cmd, err, out)
	}
	base := filepath.Base(partPDF)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	autoDir := filepath.Join(mineruOutput, name, "auto")
	res := &MinerUResult{AutoDir: autoDir, OriginPDFName: base}
	// 产物定位（MinerU 版本间布局有差异，glob 兜底）
	matches, _ := filepath.Glob(filepath.Join(autoDir, "*_content_list.json"))
	for _, m := range matches {
		if !strings.Contains(filepath.Base(m), "_v2") {
			res.ContentList = m
			break
		}
	}
	if res.ContentList == "" && len(matches) > 0 {
		res.ContentList = matches[0]
	}
	if l, err := filepath.Glob(filepath.Join(autoDir, "layout.json")); err == nil && len(l) > 0 {
		res.LayoutJSON = l[0]
	}
	if res.ContentList == "" {
		return res, fmt.Errorf("MinerU 未产出 content_list（auto 目录：%s）", autoDir)
	}
	return res, nil
}
