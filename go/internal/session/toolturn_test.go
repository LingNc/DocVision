package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// fakeTool is a scripted session tool.
type fakeTool struct {
	name string
	res  ToolResult
}

func (t *fakeTool) Name() string { return t.name }
func (t *fakeTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":       t.name,
		"parameters": map[string]any{"type": "object"},
	}}
}
func (t *fakeTool) Execute(string) (ToolResult, error) { return t.res, nil }

// TestToolImageTurnDoesNotBreakToolBlock is the regression test for the
// "insufficient tool messages following tool_calls message" 400: a tool
// that returns an image must not inject a user turn BETWEEN tool
// responses when several tools are called in one round.
func TestToolImageTurnDoesNotBreakToolBlock(t *testing.T) {
	var mu sync.Mutex
	round := 0
	var problems []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Messages []struct {
				Role      string `json:"role"`
				ToolCalls []struct {
					ID string `json:"id"`
				} `json:"tool_calls"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(raw, &body)

		// Validate the tool-call block: every assistant tool_calls turn
		// must be followed immediately by matching tool messages.
		for i, m := range body.Messages {
			if m.Role != "assistant" || len(m.ToolCalls) == 0 {
				continue
			}
			for j, tc := range m.ToolCalls {
				idx := i + 1 + j
				if idx >= len(body.Messages) {
					problems = append(problems, fmt.Sprintf("missing tool reply for %s", tc.ID))
					continue
				}
				next := body.Messages[idx]
				if next.Role != "tool" || next.ToolCallID != tc.ID {
					problems = append(problems, fmt.Sprintf("message %d after tool_calls is role=%s id=%s, want tool/%s", idx, next.Role, next.ToolCallID, tc.ID))
				}
			}
		}

		mu.Lock()
		round++
		cur := round
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		if cur == 1 {
			io.WriteString(w, `data: {"choices":[{"delta":{"content":"working"}}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"view_image","arguments":"{}"}}]}}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"image_context","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
			io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		io.WriteString(w, `data: {"choices":[{"delta":{"content":"final answer"},"finish_reason":"stop"}]}`+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	client.SetLogger(log)

	tools := []Tool{
		&fakeTool{name: "view_image", res: ToolResult{Text: "image attached", ImageBase64: "aGk=", ImageMIME: "image/png"}},
		&fakeTool{name: "image_context", res: ToolResult{Text: "context text"}},
	}
	sess := NewSession(client, config.ModelConfig{Model: "m"}, config.SessionTuning{MaxTokens: 100}, "sys", tools, log, 1, "test")
	got, err := sess.Run(RunOptions{UserText: "draw it"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "final answer" {
		t.Errorf("final = %q", got)
	}
	if len(problems) > 0 {
		t.Fatalf("invalid tool block: %s", strings.Join(problems, "; "))
	}
	// Both tool responses must be present, and the vision turn must come
	// after the whole tool block.
	msgs := sess.Messages()
	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.Role)
	}
	joined := strings.Join(roles, ",")
	if !strings.Contains(joined, "assistant,tool,tool,user") {
		t.Fatalf("roles = %s, want the image user turn after both tool replies", joined)
	}

	// 归属句柄：这段 user 文本必须点名**是哪一次调用**投出来的图。旧转录靠
	// 前缀 "Tool image output" 识别，所以前缀不能变；归属只写在文本里，wire 上
	// 仍然是一条普通的 user 消息（不能给 user 消息塞 tool_call_id —— OpenAI 兼容
	// schema 里没有这个位置，DSH 也是靠内部模型而不是 wire 字段）。
	var parts []map[string]interface{}
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		if cand, ok := m.Content.([]map[string]interface{}); ok {
			parts = cand
		}
	}
	if parts == nil {
		t.Fatal("没有找到承载图片的 user 轮")
	}
	var handle string
	var sawImage bool
	for _, part := range parts {
		if part["type"] == "text" {
			handle, _ = part["text"].(string)
		}
		if part["type"] == "image_url" {
			sawImage = true
		}
		if _, bad := part["tool_call_id"]; bad {
			t.Error("user 消息里出现了 tool_call_id：这会违反 OpenAI 兼容 schema")
		}
	}
	if !strings.HasPrefix(handle, toolImageTextPrefix) {
		t.Errorf("句柄 = %q, want 以 %q 开头（旧转录的识别依赖这个前缀）", handle, toolImageTextPrefix)
	}
	if !strings.Contains(handle, "from view_image") || !strings.Contains(handle, "(call call_a)") {
		t.Errorf("句柄 = %q, want 写明工具名与 call id", handle)
	}
	if !sawImage {
		t.Error("user 轮里没有 image_url 段")
	}
}

// TestToolImageHandleText 钉住句柄的三种形态（有归属 / 只有工具名 / 两者都没有）。
func TestToolImageHandleText(t *testing.T) {
	cases := []struct {
		name string
		in   visionTurn
		want string
	}{
		{
			name: "工具名 + call id",
			in:   visionTurn{name: "view_pdf", callID: "call_abc123"},
			want: "Tool image output from view_pdf (call call_abc123) (for your visual review):",
		},
		{
			name: "只有工具名",
			in:   visionTurn{name: "view_image"},
			want: "Tool image output from view_image (for your visual review):",
		},
		{
			name: "都没有（老路径）",
			in:   visionTurn{},
			want: "Tool image output (for your visual review):",
		},
	}
	for _, tc := range cases {
		if got := toolImageHandleText(tc.in); got != tc.want {
			t.Errorf("%s: 句柄 = %q, want %q", tc.name, got, tc.want)
		}
		if !strings.HasPrefix(toolImageHandleText(tc.in), toolImageTextPrefix) {
			t.Errorf("%s: 句柄前缀变了，旧转录会认不出来", tc.name)
		}
	}
	// 内容块只有 text + image_url 两段，不带任何多余字段。
	content := toolImageContent(visionTurn{mime: "image/png", b64: "aGk=", name: "view_image", callID: "call_a"})
	if len(content) != 2 {
		t.Fatalf("content 段数 = %d, want 2", len(content))
	}
	if content[0]["type"] != "text" || content[1]["type"] != "image_url" {
		t.Errorf("content 形状不对: %v", content)
	}
	url, _ := content[1]["image_url"].(map[string]string)
	if url["url"] != "data:image/png;base64,aGk=" {
		t.Errorf("data URI = %q", url["url"])
	}
}
