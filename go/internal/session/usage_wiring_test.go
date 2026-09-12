package session

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mineru-tools/internal/config"
)

// usageLineFrom reads the last t="usage" line of a transcript.
func usageLineFrom(t *testing.T, path string) map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var found map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue
		}
		if rec["t"] == "usage" {
			found = rec
		}
	}
	if found == nil {
		t.Fatalf("转录里没有 t=usage 行: %s", path)
	}
	return found
}

// TestRunRecordsUsageAndTTFT：会话跑一轮后，转录里必须出现一条用量行，
// 数字来自供应商回包，且首字延迟被真实测量（mock 先睡 60ms 再送第一个增量）。
func TestRunRecordsUsageAndTTFT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		time.Sleep(60 * time.Millisecond) // 首字延迟：第一个增量之前
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\" there\"},\"finish_reason\":\"stop\"}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1200,\"completion_tokens\":40,\"total_tokens\":1240,\"completion_tokens_details\":{\"reasoning_tokens\":15},\"prompt_tokens_details\":{\"cached_tokens\":1000}}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	w, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	on := true
	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "glm-4.6", Stream: &on})
	s := NewSession(client, config.ModelConfig{Model: "glm-4.6"}, config.SessionTuning{},
		"SYS", nil, testLogger(t), 1, "usage-wiring")
	s.SetTranscript(w)

	if _, err := s.Run(RunOptions{UserText: "hello"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	rec := usageLineFrom(t, path)
	if rec["prompt_tokens"] != float64(1200) || rec["completion_tokens"] != float64(40) {
		t.Fatalf("用量数字不对: %v", rec)
	}
	if rec["cached_tokens"] != float64(1000) {
		t.Fatalf("cached_tokens 没记下来: %v", rec)
	}
	if rec["reasoning_tokens"] != float64(15) {
		t.Fatalf("reasoning_tokens 没记下来: %v", rec)
	}
	if rec["model"] != "glm-4.6" || rec["stream"] != true || rec["finish_reason"] != "stop" {
		t.Fatalf("元信息不对: %v", rec)
	}
	ttft := rec["ttft_ms"].(float64)
	dur := rec["duration_ms"].(float64)
	if ttft < 40 || ttft > dur {
		t.Fatalf("首字延迟 = %vms / 总耗时 = %vms（应 >=40 且 <= 总耗时）", ttft, dur)
	}
	if rec["ts"] == nil || rec["ts"] == "" {
		t.Fatalf("用量行没有时间戳: %v", rec)
	}

	// 本地那一半也必须落在用量行上：预览页靠它把厂商 prompt_tokens 的增量
	// 换算成"每张图片实测多少 token"。
	if rec["text_tokens"] == nil || rec["text_tokens"].(float64) <= 0 {
		t.Fatalf("用量行没有记录本地文本估算 (text_tokens): %v", rec)
	}
	if got, ok := rec["image_count"]; ok && got != float64(0) {
		t.Fatalf("这一轮没有图片，image_count 应为 0 或省略: %v", rec)
	}
	// 记的是**请求发出那一刻**的文本估算（请求之后历史又长了一条助手消息，
	// 所以不能拿运行结束后的 promptTextTokens 比）。
	if s.reqTextTokens <= 0 || rec["text_tokens"].(float64) != float64(s.reqTextTokens) {
		t.Fatalf("text_tokens = %v，期望请求发出时的文本估算 %d", rec["text_tokens"], s.reqTextTokens)
	}

	// 消息行也要有时间戳。
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"t":"msg"`) || !strings.Contains(string(data), `"ts":"`) {
		t.Fatalf("消息行缺少时间戳: %s", string(data))
	}
}
