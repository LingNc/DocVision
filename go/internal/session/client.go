// Package session provides the reusable AI conversation engine used by
// the LaTeX pipelines: an OpenAI-compatible client, a multi-turn
// session with a pluggable tool registry, a configurable context
// window and AI-driven auto-compaction when the window fills up.
package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// Client is a thin HTTP wrapper around an OpenAI-compatible chat
// completions endpoint. It is stateless and safe to share across
// goroutines; retry/backoff lives in CallWithRetry.
type Client struct {
	http        *http.Client
	baseURL     string
	apiKey      string
	model       string
	requestBody map[string]interface{}
	log         *logger.Logger // optional; logs retry/backoff waits when set

	// Request/retry controls resolved from ModelConfig (with defaults).
	maxAPIRetries int
	rateLimitCap  int
}

// SetLogger attaches a logger so retry/backoff waits become visible in
// the log file (the console stays untouched).
func (c *Client) SetLogger(l *logger.Logger) {
	c.log = l
}

// NewClient builds a Client from a resolved ModelConfig.
func NewClient(cfg config.ModelConfig) *Client {
	read := 400 * time.Second
	if cfg.APITimeout > 0 {
		read = time.Duration(cfg.APITimeout) * time.Second
	}
	connect := 60 * time.Second
	if cfg.APIConnectTimeout > 0 {
		connect = time.Duration(cfg.APIConnectTimeout) * time.Second
	}
	maxRetries := cfg.APIMaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	rateLimit := cfg.RateLimitRetries
	if rateLimit <= 0 {
		rateLimit = 100 // in-code safety cap, matches img2text
	}
	return &Client{
		http:          &http.Client{Timeout: connect + read},
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:        cfg.APIKey,
		model:         cfg.Model,
		requestBody:   cfg.RequestBody,
		maxAPIRetries: maxRetries,
		rateLimitCap:  rateLimit,
	}
}

// Model returns the configured model name.
func (c *Client) Model() string { return c.model }

// ChatRequest mirrors the OpenAI chat completions schema.
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

// ChatMessage is one message in a conversation. Content is either a
// string or a []map multimodal part list.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall mirrors the assistant-side tool_calls entry.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ChatResponse is the subset of the response we use.
type ChatResponse struct {
	Choices []ChatResponseChoice `json:"choices"`
}

// ChatResponseChoice is one entry of choices.
type ChatResponseChoice struct {
	Message ChatMessage `json:"message"`
}

// ChatCompletion POSTs the request. Non-2xx responses are returned as
// errors so the retry loop can react to 429/5xx uniformly.
func (c *Client) ChatCompletion(req *ChatRequest) (*ChatResponse, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
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

// CallWithRetry performs the bounded exponential-backoff loop shared by
// all pipelines: rate limits (429) back off 2*2^n capped at 60s,
// connection issues 5*2^n capped at 60s, other errors (e.g. transient
// 5xx with a non-JSON body) 2*2^n capped at 30s. The same sentinel
// strings as img2text are returned so log analysers can classify
// failures uniformly.
func (c *Client) CallWithRetry(req *ChatRequest) (*ChatResponse, string, string) {
	maxAPIRetries := c.maxAPIRetries
	rateLimitLimit := c.rateLimitCap
	retry := 0
	rateRetry := 0
	waitLog := func(tag string, wait time.Duration) {
		if c.log != nil {
			c.log.LogWarning(0, "  ["+tag+"] 等待重试:", wait)
		}
	}
	for {
		resp, err := c.ChatCompletion(req)
		if err == nil {
			return resp, "", ""
		}
		errStr := err.Error()
		lower := strings.ToLower(errStr)
		if strings.Contains(errStr, "429") || strings.Contains(lower, "rate") {
			if rateRetry < rateLimitLimit {
				wait := time.Duration(1<<rateRetry) * 2 * time.Second
				if wait > 60*time.Second {
					wait = 60 * time.Second
				}
				waitLog("RateLimit", wait)
				time.Sleep(wait)
				rateRetry++
				continue
			}
			return nil, "[SESSION_RATE_LIMIT_EXCEEDED]", "error"
		}
		if containsAny(lower, "connect", "timeout", "handshake", "timed out") {
			if retry < maxAPIRetries {
				wait := time.Duration(1<<retry) * 5 * time.Second
				if wait > 60*time.Second {
					wait = 60 * time.Second
				}
				waitLog("ConnRetry", wait)
				time.Sleep(wait)
				retry++
				continue
			}
			return nil, "[SESSION_CONNECTION_TIMEOUT]", "error"
		}
		if retry < maxAPIRetries {
			// Generic API error (e.g. transient 5xx with a non-JSON
			// body from the proxy): exponential backoff 2s, 4s, 8s...
			wait := time.Duration(1<<retry) * 2 * time.Second
			if wait > 30*time.Second {
				wait = 30 * time.Second
			}
			waitLog("APIRetry", wait)
			time.Sleep(wait)
			retry++
			continue
		}
		return nil, fmt.Sprintf("[SESSION_API_ERROR: %s]", truncate(errStr, 200)), "error"
	}
}

// injectRequestBody merges extra config keys into a marshalled payload.
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

// ContentString extracts assistant text from a message, supporting
// plain strings and structured multimodal content.
func ContentString(m ChatMessage) string {
	switch c := m.Content.(type) {
	case string:
		return c
	case nil:
		return ""
	default:
		raw, err := json.Marshal(c)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

// NormalizeToolCallTypes fills empty tool-call types with "function" so
// strict gateways accept echoed assistant messages.
func NormalizeToolCallTypes(msg *ChatMessage) {
	for i := range msg.ToolCalls {
		if msg.ToolCalls[i].Type == "" {
			msg.ToolCalls[i].Type = "function"
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
