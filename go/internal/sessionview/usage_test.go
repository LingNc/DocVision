package sessionview

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// 一个三请求的转录：1000/900 缓存命中 + 800/0 + 600/300 命中，用于核对
// token 合计、缓存命中率、平均首字延迟与输出速度。
const usageTranscript = `{"t":"msg","role":"user","text":"hi","ts":"2026-09-11T20:00:00.000+08:00"}
{"t":"usage","ts":"2026-09-11T20:00:02.000+08:00","model":"glm-4.6","stream":true,"round":1,"prompt_tokens":1000,"cached_tokens":900,"completion_tokens":200,"reasoning_tokens":50,"duration_ms":4000,"ttft_ms":1000,"finish_reason":"tool_calls"}
{"t":"msg","role":"assistant","text":"done","ts":"2026-09-11T20:00:02.100+08:00"}
{"t":"usage","ts":"2026-09-11T20:00:06.000+08:00","model":"glm-4.6","stream":true,"round":2,"prompt_tokens":800,"completion_tokens":100,"duration_ms":2000,"ttft_ms":500,"finish_reason":"stop"}
{"t":"usage","ts":"2026-09-11T20:00:10.000+08:00","model":"glm-4.6","stream":true,"round":3,"prompt_tokens":600,"cached_tokens":300,"completion_tokens":150,"duration_ms":3000,"ttft_ms":500,"finish_reason":"stop"}
{"t":"garbage-line-that-is-not-json
`

func writeUsageTranscript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "proj", "work", "sessions", "convert_chapter_001.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(usageTranscript), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestScanAggregatesUsage(t *testing.T) {
	infos, err := Scan(writeUsageTranscript(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("会话数 = %d", len(infos))
	}
	st := infos[0].Stats
	if st == nil {
		t.Fatal("Stats 为空：usage 行没有被聚合")
	}
	if st.Requests != 3 {
		t.Fatalf("请求数 = %d，期望 3", st.Requests)
	}
	if st.PromptTokens != 2400 || st.CachedTokens != 1200 || st.Completion != 450 || st.ReasoningTokens != 50 {
		t.Fatalf("token 合计不对: %+v", st)
	}
	// 缓存命中率 = 1200/2400 = 50%
	if math.Abs(st.CacheHitPct-50) > 0.01 {
		t.Fatalf("缓存命中率 = %v，期望 50", st.CacheHitPct)
	}
	// 生成时间 = (4000-1000)+(2000-500)+(3000-500) = 7000ms → 450/7 = 64.3 tok/s
	if math.Abs(st.OutputTPS-450.0*1000/7000) > 0.01 {
		t.Fatalf("输出速度 = %v", st.OutputTPS)
	}
	if st.AvgTTFTMS != (1000+500+500)/3 {
		t.Fatalf("平均首字延迟 = %d", st.AvgTTFTMS)
	}
	if st.AvgDurationMS != (4000+2000+3000)/3 {
		t.Fatalf("平均耗时 = %d", st.AvgDurationMS)
	}
	if st.SpanMS != 8000 {
		t.Fatalf("会话跨度 = %d，期望 8000", st.SpanMS)
	}
	// 乱码行不能毁掉整段统计
	if infos[0].Messages != 2 {
		t.Fatalf("消息数 = %d，期望 2", infos[0].Messages)
	}
}

func TestSessionLinesExposeUsage(t *testing.T) {
	root := writeUsageTranscript(t)
	infos, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	lines, _, err := ReadSession(infos[0].Path, 0)
	if err != nil {
		t.Fatal(err)
	}
	var usage int
	for _, l := range lines {
		if !l.IsUsage() {
			continue
		}
		usage++
		if l.Stats == nil || l.Stats.Round == 0 {
			t.Fatalf("usage 行缺少 stats: %+v", l)
		}
		if l.Stats.OutputTPS <= 0 {
			t.Fatalf("单请求输出速度为 0: %+v", l.Stats)
		}
	}
	if usage != 3 {
		t.Fatalf("usage 行数 = %d，期望 3", usage)
	}
}

// 旧转录（没有 ts / usage 行）必须优雅降级：指标为 nil 而不是一堆 0。
func TestScanWithoutUsageHasNoStats(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "old.jsonl")
	if err := os.WriteFile(p, []byte(`{"t":"msg","role":"user","text":"hi"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	infos, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Stats != nil {
		t.Fatalf("旧转录不该有指标: %+v", infos)
	}
}

// T106 回归：price/run_end 也是 t=meta——它们不得覆盖 system 行的提示词快照
// （曾把详情栏抹成"没有正文 / 工具定义 0 个"）。
func TestMetaSnapshotIgnoresNonSystemKinds(t *testing.T) {
	var st transcriptStat
	st.addMeta(`{"t":"meta","kind":"system","model":"m","text":"系统提示词全文","tools":[{},{}]}`)
	st.addMeta(`{"t":"meta","kind":"price","model":"m","input":1}`)
	st.addMeta(`{"t":"meta","kind":"run_end","ok":true}`)
	if st.Meta.PromptChars == 0 || st.Meta.Tools != 2 {
		t.Fatalf("非 system meta 行污染了快照: %+v", st.Meta)
	}
	if st.Meta.Count != 3 {
		t.Fatalf("Count 应计全部 meta 行: %d", st.Meta.Count)
	}
	// 无 kind 的旧行仍按 system 处理（兼容）。
	var old transcriptStat
	old.addMeta(`{"t":"meta","model":"m","text":"旧格式","tools":[{}]}`)
	if old.Meta.PromptChars == 0 || old.Meta.Tools != 1 {
		t.Fatalf("旧格式 meta 行应照常被采纳: %+v", old.Meta)
	}
}
