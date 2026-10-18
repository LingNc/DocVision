package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// DefaultAlignGridSpec 是 latex.dedupe_align_grid 未写时的取值：逐级搜索
// 最细一级（判定用那一级）的格数取内容框画布边长 S 的 0.95 倍。
const DefaultAlignGridSpec = "0.95S"

// ParseAlignGrid 解析 latex.dedupe_align_grid 的两种写法：
//
//   - 纯整数（如 "128"）：固定格数；
//   - 数字 + S（如 "0.95S"）：画布边长 S 的比例，取最近的整数格。
//
// 结果夹在 [8, 1024]。比例写法要求 canvas > 0（S 由图片池决定，运行时才知道）。
func ParseAlignGrid(spec string, canvas int) (int, error) {
	s := strings.TrimSpace(spec)
	if s == "" {
		s = DefaultAlignGridSpec
	}
	ratio := false
	if l := strings.ToLower(s); strings.HasSuffix(l, "s") {
		ratio = true
		s = strings.TrimSpace(s[:len(s)-1])
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) {
		return 0, fmt.Errorf("dedupe_align_grid: 无法解析 %q（写整数格数如 128，或比例如 0.95S）", spec)
	}
	var g int
	if ratio {
		if v <= 0 || v > 4 {
			return 0, fmt.Errorf("dedupe_align_grid: 比例 %v 超出 (0,4]，应写 0.95S 这样", v)
		}
		if canvas <= 0 {
			return 0, fmt.Errorf("dedupe_align_grid: 比例写法需要已知画布边长 S")
		}
		g = int(math.Round(v * float64(canvas)))
	} else {
		if v < 1 || v != math.Trunc(v) {
			return 0, fmt.Errorf("dedupe_align_grid: 固定格数必须是正整数，得到 %q（比例请写 0.95S）", spec)
		}
		g = int(v)
	}
	if g < 8 {
		g = 8
	}
	if g > 1024 {
		g = 1024
	}
	return g, nil
}
