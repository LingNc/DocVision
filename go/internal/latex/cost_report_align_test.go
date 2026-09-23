package latex

import (
	"strings"
	"testing"
)

// T71：表头与数据行的显示列对齐（中文表头按 2 列计、数值列右对齐）。
func TestCostHeaderAlignment(t *testing.T) {
	header := "会话"
	padded := padLeftWide(header, 5)
	// 「会话」显示宽 4，右对齐到 5 → 左 1 空格
	if padded != " 会话" {
		t.Fatalf("表头右对齐错: %q", padded)
	}
	// 数据行 %5d 同宽：9 → "    9"（5 列）；表头显示宽也应为 5 列
	w := 0
	for _, r := range padded {
		if r >= 0x2E80 {
			w += 2
		} else {
			w += 1
		}
	}
	if w != 5 {
		t.Fatalf("表头显示宽 %d != 5", w)
	}
	if padLeftWide("输入tokens", 11) != " 输入tokens" {
		t.Fatalf("输入tokens 对齐错: %q", padLeftWide("输入tokens", 11))
	}
	if !strings.HasSuffix(padLeftWide("缓存命中", 8), "缓存命中") || len(padLeftWide("缓存命中", 8)) != len("缓存命中") {
		t.Fatalf("缓存命中 宽度应恰好 8 列无补位: %q", padLeftWide("缓存命中", 8))
	}
}
