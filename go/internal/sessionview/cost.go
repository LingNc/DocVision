package sessionview

import (
	"math"
	"sort"
	"strings"

	"mineru-tools/internal/config"
)

// CostStats is the money side of one session's (or one stage's) usage. It is
// nil whenever no model the session used has a configured price: a bill of
// "¥0" for an unpriced model is a lie, and the viewer prints nothing instead.
type CostStats struct {
	// Currency is the price table's currency symbol (¥ by default).
	Currency string `json:"currency"`
	// Total is input + output, in Currency.
	Total float64 `json:"total"`
	// Input is the full prompt cost (fresh + cached parts).
	Input float64 `json:"input"`
	// Output is the completion cost (thinking included, as providers bill it).
	Output float64 `json:"output"`
}

func (c *CostStats) add(o CostStats) {
	if c.Currency == "" {
		c.Currency = o.Currency
	}
	c.Total += o.Total
	c.Input += o.Input
	c.Output += o.Output
}

// SessionCost prices one session's usage against the price table. ok is false
// when the session's model has no rate — the caller then shows no money at all
// (and counts the requests as unpriced) instead of inventing ¥0.
func SessionCost(st *UsageStats, prices map[string]config.PriceConfig) (*CostStats, bool) {
	if st == nil || len(prices) == 0 {
		return nil, false
	}
	// The aggregate carries one Model field (the last one seen) and no
	// per-request breakdown, so a session that switched models mid-flight is
	// priced with the model its last request used — documented, and rare
	// (each pipeline session is bound to one registry entry).
	p, ok := prices[st.Model]
	if !ok || !p.Configured() {
		return nil, false
	}
	c := &CostStats{Currency: p.Currency}
	fresh := st.PromptTokens - st.CachedTokens
	if fresh < 0 {
		fresh = 0
	}
	c.Input = round6((float64(fresh)*p.Input + float64(st.CachedTokens)*p.CachedRate()) / 1e6)
	c.Output = round6(float64(st.Completion) * p.Output / 1e6)
	c.Total = round6(c.Input + c.Output)
	return c, true
}

// currencyOf picks a display currency for the "unpriced" case: the table's
// first non-empty one, else empty.
func currencyOf(prices map[string]config.PriceConfig) string {
	names := make([]string, 0, len(prices))
	for name := range prices {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if cur := prices[name].Currency; cur != "" {
			return cur
		}
	}
	return ""
}

// ApplyPrices fills SessionInfo.Cost for every session and returns the total.
// Sessions whose model has no rate keep Cost == nil (the viewer shows no cost
// for them) but still count in the total's Priced/Unpriced tallies when the
// total was told about them.
func ApplyPrices(sessions []SessionInfo, prices map[string]config.PriceConfig) {
	for i := range sessions {
		cost, ok := SessionCost(sessions[i].Stats, prices)
		if !ok {
			sessions[i].Cost = nil
			continue
		}
		sessions[i].Cost = cost
	}
}

// StageOf returns the pipeline stage of a session (the label before ":", e.g.
// "convert" for "convert:chapter_002"). It is the grouping key of the cost
// report: one row per stage is what "按阶段统计" means.
func StageOf(s SessionInfo) string {
	label := s.Label
	if label == "" {
		label = LabelFor(s.ID)
	}
	stage, _, _ := strings.Cut(label, ":")
	if stage == "" {
		return "其他"
	}
	return stage
}

// StageCost is one row of the per-stage cost report.
type StageCost struct {
	// Stage is the pipeline stage (convert / checker / style / …).
	Stage string `json:"stage"`
	// Sessions is how many transcripts fed this row, Requests how many API
	// requests they made.
	Sessions int `json:"sessions"`
	Requests int `json:"requests"`
	// Tokens are the summed provider counters of the stage.
	PromptTokens int `json:"promptTokens"`
	CachedTokens int `json:"cachedTokens"`
	Completion   int `json:"completionTokens"`
	// CacheHitPct is Σcached/Σprompt (percent).
	CacheHitPct float64 `json:"cacheHitPct"`
	// Cost is the summed money; Unpriced counts the requests whose model has
	// no configured rate (so Cost is a lower bound when it is > 0).
	Cost     float64 `json:"cost"`
	Currency string  `json:"currency"`
	Unpriced int     `json:"unpriced,omitempty"`
}

// AvgCostPerRequest is the stage's average spend per API request.
func (s StageCost) AvgCostPerRequest() float64 {
	if s.Requests == 0 {
		return 0
	}
	return s.Cost / float64(s.Requests)
}

// StageCosts aggregates sessions per stage, most expensive first (the report
// exists to answer "where did the money go"). Stages whose models have no rate
// are still listed, with Unpriced > 0 and Cost 0.
func StageCosts(sessions []SessionInfo) []StageCost {
	byStage := map[string]*StageCost{}
	order := []string{}
	for _, s := range sessions {
		stage := StageOf(s)
		row, ok := byStage[stage]
		if !ok {
			row = &StageCost{Stage: stage}
			byStage[stage] = row
			order = append(order, stage)
		}
		row.Sessions++
		if s.Stats != nil {
			row.Requests += s.Stats.Requests
			row.PromptTokens += s.Stats.PromptTokens
			row.CachedTokens += s.Stats.CachedTokens
			row.Completion += s.Stats.Completion
		}
		if s.Cost != nil {
			row.Cost += s.Cost.Total
			row.Currency = s.Cost.Currency
		} else if s.Stats != nil && s.Stats.Requests > 0 {
			// 有调用但没配价格：金额只是下界，报告里必须说出来。
			row.Unpriced += s.Stats.Requests
		}
	}
	out := make([]StageCost, 0, len(order))
	for _, stage := range order {
		row := byStage[stage]
		if row.PromptTokens > 0 {
			row.CacheHitPct = float64(row.CachedTokens) * 100 / float64(row.PromptTokens)
		}
		out = append(out, *row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Requests > out[j].Requests
	})
	return out
}

// UnpricedRequests counts requests whose model has no rate — the money totals
// are lower bounds as long as this is > 0.
func UnpricedRequests(sessions []SessionInfo) int {
	n := 0
	for _, s := range sessions {
		if s.Cost == nil && s.Stats != nil {
			n += s.Stats.Requests
		}
	}
	return n
}

// TotalCost sums every session's cost (nil-safe); nil when nothing is priced.
func TotalCost(sessions []SessionInfo) *CostStats {
	var total CostStats
	found := false
	for _, s := range sessions {
		if s.Cost == nil {
			continue
		}
		total.add(*s.Cost)
		found = true
	}
	if !found {
		return nil
	}
	return &total
}

// round6 keeps the serialised numbers readable (0.59 instead of 0.5900000000000001);
// a micro-unit is far below any real per-request cost.
func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
