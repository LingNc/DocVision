package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mineru-tools/internal/sessionview"
)

// CostReport writes the per-stage AI spending of a finished project: one row
// per pipeline stage with its token split, prefix-cache hit rate, money and
// average per request, plus the book's scale so "每张图/每页要多少钱" is
// answerable. It is a plain log table on purpose — the numbers come from the
// transcripts themselves (t="usage" lines), so nothing has to be instrumented
// during the run and re-running it on an old project gives the same answer.
//
// Prices come from models.<条目>.price in the config. With no price configured
// the whole report is skipped: printing ¥0 would read as "this run was free".
func (r *Runner) CostReport(proj string) {
	if proj == "" {
		return
	}
	prices := r.cfg.ModelPrices()
	if len(prices) == 0 {
		r.log.Log(0, "[cost] 未配置模型价格（models.<条目>.price），跳过费用统计")
		return
	}
	sessions, err := sessionview.Scan(proj)
	if err != nil {
		r.log.LogWarning(0, "[cost] 扫描会话转录失败:", err)
		return
	}
	sessionview.ApplyPrices(sessions, prices)

	rows := sessionview.StageCosts(sessions)
	if len(rows) == 0 {
		return
	}

	images, pages := projectScale(proj)
	r.log.Log(0, "[cost] ===== AI 用量与费用（按阶段） =====")
	r.log.Log(0, fmt.Sprintf("[cost] %-11s %5s %6s %11s %8s %11s %10s",
		"阶段", "会话", "请求", "输入tokens", "缓存命中", "输出tokens", "费用"))
	var reqs, in, cached, out int
	var money float64
	currency := ""
	unpriced := sessionview.UnpricedRequests(sessions)
	for _, row := range rows {
		cache := "-"
		if row.PromptTokens > 0 {
			cache = fmt.Sprintf("%.0f%%", row.CacheHitPct)
		}
		cost := "未配价"
		if row.Currency != "" || row.Cost > 0 {
			cost = fmt.Sprintf("%s%.2f", row.Currency, row.Cost)
			if currency == "" {
				currency = row.Currency
			}
		}
		r.log.Log(0, fmt.Sprintf("[cost] %-11s %5d %6d %11s %8s %11s %s%s",
			row.Stage, row.Sessions, row.Requests,
			sessionview.HumanCount(row.PromptTokens), cache,
			sessionview.HumanCount(row.Completion), cost, unpricedMark(row)))
		reqs += row.Requests
		in += row.PromptTokens
		cached += row.CachedTokens
		out += row.Completion
		money += row.Cost
	}
	// currency 为空 = 没有任何一次请求命中已配置的单价：金额一律写 "-"，
	// 不写 0.00（0.00 会被读成"几乎没花钱"，真实含义是"算不出来"）。
	avg := "-"
	if reqs > 0 && currency != "" {
		avg = fmt.Sprintf("%s%.4f", currency, money/float64(reqs))
	}
	totalMoney := "-"
	if currency != "" {
		totalMoney = fmt.Sprintf("%s%.2f", currency, money)
	}
	r.log.Log(0, fmt.Sprintf("[cost] %-11s %5d %6d %11s %8s %11s %s",
		"合计", len(sessions), reqs, sessionview.HumanCount(in),
		pct(cached, in), sessionview.HumanCount(out), totalMoney))
	if reqs > 0 {
		r.log.Log(0, "[cost] 平均每次请求:", avg, "；缓存命中", pct(cached, in))
	}
	if unpriced > 0 {
		r.log.LogWarning(0, fmt.Sprintf("[cost] 有 %d 次请求的模型没配价格，金额只是**下界**；"+
			"在 models.<条目>.price 里补 input/cached/output（单位：元/百万 tokens）", unpriced))
	}
	// 规模：把总量除到"每张图 / 每页"上，这才是单本书真正要对比的数字。
	scale := []string{}
	if images > 0 {
		scale = append(scale, perUnit("图片 "+fmt.Sprint(images)+" 张 → 每张 ", images, currency, money))
	}
	if pages > 0 {
		scale = append(scale, perUnit("成品 "+fmt.Sprint(pages)+" 页 → 每页 ", pages, currency, money))
	}
	if len(scale) > 0 {
		r.log.Log(0, "[cost] 规模:", strings.Join(scale, "；"))
	} else {
		r.log.Log(0, "[cost] 规模: 未找到图片进度或成品 PDF（每图/每页均价需二者之一）")
	}
}

func unpricedMark(row sessionview.StageCost) string {
	if row.Unpriced > 0 {
		return fmt.Sprintf("（%d 次未配价）", row.Unpriced)
	}
	return ""
}

func pct(part, total int) string {
	if total <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", float64(part)*100/float64(total))
}

// projectScale reads the book's size: how many images the images stage handled
// (one JSON per image under progress_items/) and how many pages the delivered
// PDF has. Both are optional — a level-2 project has no book.pdf, and a run
// that stopped before assemble has no pages — and 0 means "unknown" so the
// averages are left out instead of being divided by a made-up number.
func projectScale(proj string) (images, pages int) {
	// 档位1 的图片进度在 <proj>/source/progress_items，档位2 在 <proj>/progress_items。
	for _, dir := range []string{
		filepath.Join(proj, "source", "progress_items"),
		filepath.Join(proj, "progress_items"),
	} {
		if n := countImageProgress(dir); n > images {
			images = n
		}
	}
	for _, pdf := range []string{
		filepath.Join(proj, "out", "book.pdf"),
		filepath.Join(proj, "out", "main.pdf"),
	} {
		if _, err := os.Stat(pdf); err != nil {
			continue
		}
		if n, err := pdfPageCount(pdf); err == nil && n > 0 {
			pages = n
			break
		}
	}
	return images, pages
}

// countImageProgress counts the per-image progress entries (<dir>/<md>/<key>.json).
func countImageProgress(dir string) int {
	subs, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, sub := range subs {
		if !sub.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, sub.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
				n++
			}
		}
	}
	return n
}

// perUnit renders a per-image / per-page average, or explains why there is no
// number when no price is configured (never "0.0000").
func perUnit(label string, n int, currency string, money float64) string {
	if currency == "" || n == 0 {
		return label + "未配价（models.<条目>.price 里补单价后才有数字）"
	}
	return fmt.Sprintf("%s%s%.4f", label, currency, money/float64(n))
}
