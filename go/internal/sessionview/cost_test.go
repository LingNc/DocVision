package sessionview

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

func priceTable() map[string]config.PriceConfig {
	return map[string]config.PriceConfig{
		"m-cheap": {Input: 1, Cached: 0.1, Output: 4, Currency: "¥"},
		"m-big":   {Input: 2, Output: 8, Currency: "¥"},
	}
}

// 价格必须按"未命中缓存输入 / 命中缓存输入 / 输出"三段分开算：把缓存部分
// 按原价算会虚高十倍，把未命中部分按缓存价算会凭空打折。
func TestSessionCostSplitsCacheAndOutput(t *testing.T) {
	st := &UsageStats{Requests: 2, PromptTokens: 1_000_000, CachedTokens: 900_000, Completion: 100_000, Model: "m-cheap"}
	cost, ok := SessionCost(st, priceTable())
	if !ok || cost == nil {
		t.Fatal("配了价格就必须算出成本")
	}
	// 100k 未命中 × 1元/百万 + 900k 命中 × 0.1元/百万 + 100k 输出 × 4元/百万
	want := 0.1 + 0.09 + 0.4
	if diff := cost.Total - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("总额 = %v, want %v", cost.Total, want)
	}
	// 没配 cached 时按 input 计（宁可高估）：m-big 的 900k 缓存部分按 2 元算。
	st2 := &UsageStats{Requests: 1, PromptTokens: 1_000_000, CachedTokens: 900_000, Completion: 0, Model: "m-big"}
	cost2, _ := SessionCost(st2, priceTable())
	want2 := (0.1 + 0.9) * 2
	if diff := cost2.Total - want2; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("未配 cached 时应按 input 价：got %v want %v", cost2.Total, want2)
	}
}

// 没配价格的模型：Cost 必须是 nil（页面不显示金额），绝不能显示 ¥0。
func TestUnpricedModelHasNoCost(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "a/work/sessions/convert_01.jsonl", Stats: &UsageStats{Requests: 3, PromptTokens: 100, Model: "unknown-model"}},
	}
	ApplyPrices(sessions, priceTable())
	if sessions[0].Cost != nil {
		t.Fatal("没配价格的模型不该有金额")
	}
	if n := UnpricedRequests(sessions); n != 3 {
		t.Fatalf("没配价的请求数应被统计（金额是下界）：got %d", n)
	}
	// 一张价格表都没有时同样什么都不算。
	sessions2 := []SessionInfo{{ID: "a/work/sessions/convert_01.jsonl", Stats: &UsageStats{Requests: 1, Model: "m-cheap"}}}
	ApplyPrices(sessions2, map[string]config.PriceConfig{})
	if sessions2[0].Cost != nil {
		t.Fatal("空价格表不该产生金额")
	}
}

// 按阶段汇总：convert / checker / style-fix 各自一行，贵的排前面，合计一致。
func TestStageCostsGroupByStage(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "book/work/sessions/convert_01.jsonl", Stats: &UsageStats{Requests: 2, PromptTokens: 100_000, CachedTokens: 50_000, Completion: 10_000, Model: "m-cheap"}},
		{ID: "book/work/sessions/checker_01.jsonl", Stats: &UsageStats{Requests: 1, PromptTokens: 10_000, Completion: 1_000, Model: "m-cheap"}},
		{ID: "book/work/sessions/style_fix_01.jsonl", Stats: &UsageStats{Requests: 1, PromptTokens: 20_000, Completion: 2_000, Model: "m-big"}},
		{ID: "book/work/style_session.jsonl", Stats: &UsageStats{Requests: 5, PromptTokens: 500_000, CachedTokens: 400_000, Completion: 50_000, Model: "m-big"}},
	}
	ApplyPrices(sessions, priceTable())
	rows := StageCosts(sessions)
	if len(rows) != 4 {
		t.Fatalf("应有 4 个阶段行，got %d: %+v", len(rows), rows)
	}
	byName := map[string]StageCost{}
	for _, r := range rows {
		byName[r.Stage] = r
	}
	for _, want := range []string{"convert", "checker", "style-fix", "style"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("缺阶段 %q（LabelFor 必须认得 checker_/style_fix_ 前缀）: %+v", want, rows)
		}
	}
	// 贵的排前面：style（500k 输入、50k 输出）必然第一。
	if rows[0].Stage != "style" {
		t.Fatalf("应按费用降序，got %+v", rows)
	}
	// 合计 = 各行之和。
	total := TotalCost(sessions)
	var sum float64
	for _, r := range rows {
		sum += r.Cost
	}
	if diff := total.Total - sum; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("合计 %v ≠ 各行之和 %v", total.Total, sum)
	}
	// 缓存命中率是加权口径（Σcached/Σprompt）。
	if got := byName["convert"].CacheHitPct; got < 49 || got > 51 {
		t.Fatalf("convert 缓存命中率 = %v, want ~50", got)
	}
}

// 真实缺陷回归：UsageStats 聚合曾经**丢掉 model 字段**，于是费用报告查无此价、
// 金额永远是 0，而界面上只看到"未配价"——价格配了却一个也用不上。
func TestUsageAggregateKeepsModelName(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/convert_chapter_001.jsonl"
	lines := `{"ts":"2026-09-11T20:20:00Z","t":"usage","round":1,"model":"wire-A","prompt_tokens":1000,"cached_tokens":900,"completion_tokens":10}
{"ts":"2026-09-11T20:21:00Z","t":"usage","round":2,"model":"wire-A","prompt_tokens":2000,"cached_tokens":1900,"completion_tokens":20}
`
	if err := os.WriteFile(p, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := readStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Usage == nil || st.Usage.Model != "wire-A" {
		t.Fatalf("聚合必须带上 model（费用报告靠它查价），got %+v", st.Usage)
	}
	prices := map[string]config.PriceConfig{"wire-A": {Input: 1, Cached: 0.1, Output: 4, Currency: "¥"}}
	cost, ok := SessionCost(st.Usage, prices)
	if !ok || cost == nil || cost.Total <= 0 {
		t.Fatalf("配了价格就该算出金额，got %+v ok=%v", cost, ok)
	}
}

// 页面要显示费用，靠的就是索引 JSON 里的这个字段：扫描 + 定价之后必须能
// 序列化出 cost（viewer.js 读 session.cost）。
func TestScanThenPriceExposesCostInJSON(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/work/sessions/convert_chapter_001.jsonl"
	if err := os.MkdirAll(dir+"/work/sessions", 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"ts":"2026-09-11T20:20:00Z","t":"usage","round":1,"model":"wire-A","prompt_tokens":1000000,"cached_tokens":900000,"completion_tokens":100000}
`
	if err := os.WriteFile(p, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	sessions, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	ApplyPrices(sessions, map[string]config.PriceConfig{
		"wire-A": {Input: 1, Cached: 0.1, Output: 4, Currency: "¥"},
	})
	data, err := json.Marshal(sessions)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"cost":{"currency":"¥","total":0.59,`) {
		t.Fatalf("索引 JSON 里没有费用（页面就显示不出来）: %s", data)
	}
	if !strings.Contains(string(data), `"label":"convert:chapter_001"`) {
		t.Fatalf("阶段标签变了，费用报告的分组会跟着变: %s", data)
	}
}
