package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 每条消息带时间戳、每次请求一条 t="usage"：预览页靠它们算时长/token/缓存命中。
func TestTranscriptWritesTimestampsAndUsage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	w, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(ChatMessage{Role: "user", Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	err = w.AppendUsage(UsageRecord{
		Model: "glm-4.6", Stream: true, Round: 1,
		PromptTokens: 1000, CachedTokens: 900, Completion: 200, Reasoning: 50,
		Duration: 4 * time.Second, TTFT: 1500 * time.Millisecond, Finish: "tool_calls",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("期望 2 行，得到 %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"ts":"`) {
		t.Fatalf("消息行没有时间戳: %s", lines[0])
	}
	for _, want := range []string{`"t":"usage"`, `"prompt_tokens":1000`, `"cached_tokens":900`,
		`"completion_tokens":200`, `"reasoning_tokens":50`, `"duration_ms":4000`,
		`"ttft_ms":1500`, `"stream":true`, `"finish_reason":"tool_calls"`} {
		if !strings.Contains(lines[1], want) {
			t.Fatalf("usage 行缺少 %s: %s", want, lines[1])
		}
	}

	// 回放必须忽略 usage 行（只认 t=msg）。
	replay, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range replay {
		if m.Role == "" {
			t.Fatalf("回放里出现了非消息行: %+v", m)
		}
	}
	if len(replay) != 1 || replay[0].Role != "user" {
		t.Fatalf("回放 = %+v", replay)
	}
}
