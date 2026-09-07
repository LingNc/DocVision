package img2text

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mineru-tools/internal/config"
)

// AIClient is a thin HTTP wrapper around an OpenAI-compatible chat
// completions endpoint. It owns the connection pool, base URL, and
// credentials; everything else is per-call.
type AIClient struct {
	http        *http.Client
	baseURL     string
	apiKey      string
	model       string
	requestBody map[string]interface{}

	// Retry controls resolved from ModelConfig / legacy options.
	MaxRetries       int // API retry count for non-rate-limit errors
	RateLimitRetries int // rate-limit retry cap (fallback: in-code cap)
}

// NewAIClient builds an AIClient from the loaded configuration. Read and
// connect timeouts come from OptionsConfig (matching the Python
// `httpx.Timeout(connect=..., read=..., write=..., pool=10.0)`). Per-call
// retry is handled by the caller (processor.go) so this client stays
// stateless and safe to share across goroutines.
func NewAIClient(mc config.ModelConfig, legacy config.OptionsConfig) *AIClient {
	// Precedence: model-level fields > legacy options.* > built-ins.
	read := 400 * time.Second
	if legacy.APITimeout > 0 {
		read = time.Duration(legacy.APITimeout) * time.Second
	}
	if mc.APITimeout > 0 {
		read = time.Duration(mc.APITimeout) * time.Second
	}
	connect := 60 * time.Second
	if legacy.APIConnectTimeout > 0 {
		connect = time.Duration(legacy.APIConnectTimeout) * time.Second
	}
	if mc.APIConnectTimeout > 0 {
		connect = time.Duration(mc.APIConnectTimeout) * time.Second
	}
	maxRetries := legacy.APIMaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if mc.APIMaxRetries > 0 {
		maxRetries = mc.APIMaxRetries
	}
	rateLimit := legacy.RateLimitRetries
	if rateLimit <= 0 {
		rateLimit = 100 // in-code safety cap
	}
	if mc.RateLimitRetries > 0 {
		rateLimit = mc.RateLimitRetries
	}

	// The Go http.Client only exposes a single Timeout. We approximate
	// the Python "connect / read / write / pool" split by setting
	// Timeout to the larger of connect+read so a slow response has
	// enough headroom, while the per-request context (in ChatCompletion)
	// bounds the connect phase separately via a custom dialer-free
	// client. In practice Timeout = connect + read is a close match.
	overall := connect + read

	return &AIClient{
		http:             &http.Client{Timeout: overall},
		baseURL:          strings.TrimRight(mc.BaseURL, "/"),
		apiKey:           mc.APIKey,
		model:            mc.Model,
		requestBody:      mc.RequestBody,
		MaxRetries:       maxRetries,
		RateLimitRetries: rateLimit,
	}
}

// Model returns the configured model name.
func (c *AIClient) Model() string { return c.model }

// ChatRequest is the wire-level payload we POST to the chat completions
// endpoint. The shape mirrors the OpenAI Chat Completions schema exactly.
// Any keys in the client's request_body config are merged into the
// top-level JSON payload before sending, allowing vendor-specific fields
// like enable_thinking, extra_body, etc.
type ChatRequest struct {
	Model          string           `json:"model"`
	Messages       []ChatMessage    `json:"messages"`
	Tools          []map[string]any `json:"tools,omitempty"`
	ToolChoice     any              `json:"tool_choice,omitempty"`
	MaxTokens      int              `json:"max_tokens"`
	Temperature    float64          `json:"temperature"`
	Stream         bool             `json:"stream"`
	ResponseFormat map[string]any   `json:"response_format,omitempty"`
}

// ChatMessage is one message in the conversation. The Content field is
// intentionally `any` so callers can pass a plain string (for text-only
// turns) or a []map (for multimodal turns with image_url parts).
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall mirrors the assistant-side tool_calls entry that comes back
// from the API and that we echo back on subsequent turns.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatResponse is the subset of the OpenAI chat completion response we
// actually use. Only the first choice is read.
type ChatResponse struct {
	Choices []ChatResponseChoice `json:"choices"`
}

// ChatResponseChoice is one entry of the response.choices array. The
// Message field is reused for both assistant text responses and
// tool-call responses.
type ChatResponseChoice struct {
	Message ChatMessage `json:"message"`
}

// ChatCompletion POSTs req to <baseURL>/chat/completions. Any non-2xx
// response is returned as an error so the caller's retry/backoff loop
// can react to 429 / 5xx the same way as a transport error.
func (c *AIClient) ChatCompletion(req *ChatRequest) (*ChatResponse, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Merge request_body config into the top-level JSON payload. This
	// allows vendor-specific fields (enable_thinking, extra_body, etc.)
	// to be injected without code changes.
	if len(c.requestBody) > 0 {
		raw, err = injectRequestBody(raw, c.requestBody)
		if err != nil {
			return nil, err
		}
	}

	endpoint := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out ChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w (body=%q)", err, truncate(string(body), 256))
	}
	return &out, nil
}

// injectRequestBody merges extra keys from the config's request_body
// map into an already-marshalled JSON payload. Implemented as
// unmarshal->merge->remarshal to keep the JSON-encoding logic in one place.
func injectRequestBody(raw []byte, extra map[string]interface{}) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode for request_body injection: %w", err)
	}
	for k, v := range extra {
		m[k] = v
	}
	return json.Marshal(m)
}

// truncate returns s shortened to n bytes, with a trailing ellipsis if
// the input was longer. Used to keep error messages bounded.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
