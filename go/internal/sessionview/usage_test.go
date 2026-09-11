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
