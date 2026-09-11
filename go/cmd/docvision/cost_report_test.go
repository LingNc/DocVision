package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"mineru-tools/internal/sessionview"
)

// captureStdout runs fn with os.Stdout redirected to a pipe.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

// 没配价格（或单价全是 0）时，费用列与合计一律写 "-"，绝不写 0.00 ——
// 0.00 会被读成"几乎没花钱"，真实含义是"没配价、算不出来"。
func TestCostReportShowsNoMoneyWhenUnpriced(t *testing.T) {
	sessions := []sessionview.SessionInfo{
		{ID: "work/sessions/convert_chapter_001.jsonl", Label: "convert:chapter_001",
			Stats: &sessionview.UsageStats{Requests: 3, PromptTokens: 1000, CachedTokens: 500, Completion: 200}},
	}
	out := captureStdout(t, func() { printCostReport(sessions, false) })
	if !strings.Contains(out, "未配价") {
		t.Fatalf("未配价必须显式标出: %s", out)
	}
	if strings.Contains(out, "0.00") {
		t.Fatalf("未配价时不该出现 0.00（会被读成没花钱）: %s", out)
	}
	if !strings.Contains(out, "单价全是 0") {
		t.Fatalf("提示应说明「没写或单价全 0」都算未配价: %s", out)
	}
	// 配了价格时金额照常显示（回归保护）。
	priced := []sessionview.SessionInfo{
		{ID: "work/sessions/convert_chapter_001.jsonl", Label: "convert:chapter_001",
			Stats: &sessionview.UsageStats{Requests: 3, PromptTokens: 1000, CachedTokens: 500, Completion: 200},
			Cost:  &sessionview.CostStats{Currency: "¥", Total: 0.12, Input: 0.1, Output: 0.02}},
	}
	out2 := captureStdout(t, func() { printCostReport(priced, true) })
	if !strings.Contains(out2, "¥0.12") {
		t.Fatalf("配价后应显示金额: %s", out2)
	}
}
