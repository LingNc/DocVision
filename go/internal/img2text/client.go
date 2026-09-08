package img2text

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"mineru-tools/internal/chatstream"
	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// AIClient is a thin HTTP wrapper around an OpenAI-compatible chat
// completions endpoint. It owns the connection pool, base URL, and
// credentials; everything else is per-call.
type AIClient struct {
	http        *http.Client // non-streaming: connect + read total timeout
	streamHTTP  *http.Client // streaming: no total timeout, idle watchdog instead
	baseURL     string
	apiKey      string
	model       string
	requestBody map[string]interface{}
	log         *logger.Logger // optional; stream progress in debug mode

	// Vendor top-level request fields (sent beside model/messages, NOT
	// inside request_body.extra_body).
	thinking        map[string]any
	reasoningEffort string

	// Streaming policy resolved from ModelConfig (default: on).
	stream     bool
	streamIdle time.Duration

	// Fallbacks applied when a call does not set them itself.
	maxTokens   int
	temperature float64

	// Retry controls resolved from ModelConfig / legacy options.
	MaxRetries       int // API retry count for non-rate-limit errors
	RateLimitRetries int // rate-limit retry cap (fallback: in-code cap)
}

// SetLogger attaches a logger for stream-progress debug lines.
func (c *AIClient) SetLogger(l *logger.Logger) { c.log = l }

// NewAIClient builds an AIClient from the loaded configuration. Read and
// connect timeouts come from ModelConfig (matching the Python
// `httpx.Timeout(connect=..., read=..., write=..., pool=10.0)`). Per-call
// retry is handled by the caller (processor.go) so this client stays
// stateless and safe to share across goroutines.
func NewAIClient(mc config.ModelConfig) *AIClient {
	read := 400 * time.Second
	if mc.APITimeout > 0 {
		read = time.Duration(mc.APITimeout) * time.Second
	}
	connect := 60 * time.Second
	if mc.APIConnectTimeout > 0 {
		connect = time.Duration(mc.APIConnectTimeout) * time.Second
	}
	streamIdle := read
	if mc.APIStreamIdleTimeout > 0 {
		streamIdle = time.Duration(mc.APIStreamIdleTimeout) * time.Second
	}
	maxRetries := mc.APIMaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	rateLimit := mc.RateLimitRetries
	if rateLimit <= 0 {
		rateLimit = 100
	}
	// Streaming must not carry a total timeout: a healthy long
	// thinking/output stream would be killed by it.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: connect, KeepAlive: 30 * time.Second}).DialContext
	tr.ResponseHeaderTimeout = connect
	return &AIClient{
		http:             &http.Client{Timeout: connect + read},
		streamHTTP:       &http.Client{Transport: tr},
		baseURL:          strings.TrimRight(mc.BaseURL, "/"),
		apiKey:           mc.APIKey,
		model:            mc.Model,
		requestBody:      mc.RequestBody,
		thinking:         mc.Thinking,
		reasoningEffort:  mc.ReasoningEffort,
		stream:           mc.Streaming(),
		streamIdle:       streamIdle,
		maxTokens:        mc.MaxTokens,
		temperature:      mc.Temperature,
		MaxRetries:       maxRetries,
		RateLimitRetries: rateLimit,
	}
}

// Model returns the configured model name.
func (c *AIClient) Model() string { return c.model }

// Streaming reports whether this client uses SSE streaming.
func (c *AIClient) Streaming() bool { return c.stream }

// RequestSummary renders the effective request parameters for debug logs.
func (c *AIClient) RequestSummary(req *ChatRequest) string {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}
	temp := req.Temperature
	if temp <= 0 {
		temp = c.temperature
	}
	think := "default"
	if c.thinking != nil {
		if v, ok := c.thinking["type"]; ok {
			think = fmt.Sprint(v)
		} else {
			think = fmt.Sprint(c.thinking)
		}
	}
	effort := c.reasoningEffort
	if effort == "" {
		effort = "-"
	}
	return fmt.Sprintf("model=%s stream=%v max_tokens=%d temperature=%.2f thinking=%s reasoning_effort=%s",
		c.model, c.stream, maxTokens, temp, think, effort)
}

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
	StreamOptions  map[string]any   `json:"stream_options,omitempty"`
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
// actually use. Only the first choice is read; Usage and the diagnostic
// fields are filled in by the client for debug logging.
type ChatResponse struct {
	Choices []ChatResponseChoice `json:"choices"`
	Usage   *Usage               `json:"usage,omitempty"`

	Elapsed        time.Duration `json:"-"`
	Streamed       bool          `json:"-"`
	ReasoningChars int           `json:"-"`
	FinishReason   string        `json:"-"`
}

// Usage is the provider token accounting.
type Usage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details,omitempty"`
}

// String renders the usage compactly for debug logs.
func (u *Usage) String() string {
	if u == nil {
		return "usage=-"
	}
	reasoning := 0
	if u.CompletionTokensDetails != nil {
		reasoning = u.CompletionTokensDetails.ReasoningTokens
	}
	return fmt.Sprintf("prompt=%d completion=%d total=%d reasoning=%d",
		u.PromptTokens, u.CompletionTokens, u.TotalTokens, reasoning)
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
//
// The client owns the transport policy: it forces req.Stream to the
// configured mode, fills per-model max_tokens/temperature fallbacks,
// and merges vendor top-level fields (thinking, reasoning_effort).
func (c *AIClient) ChatCompletion(req *ChatRequest) (*ChatResponse, error) {
	payload := *req
	payload.Stream = c.stream
	if payload.MaxTokens <= 0 {
		payload.MaxTokens = c.maxTokens
	}
	if payload.Temperature <= 0 {
		payload.Temperature = c.temperature
	}

	raw, err := c.buildBody(&payload)
	if err != nil {
		return nil, err
	}
	fallback := func(cause error) (*ChatResponse, error) {
		// Gateway without SSE support: retry once non-streaming.
		if c.log != nil {
			c.log.LogWarning(0, "  [stream] 流式请求失败，改用非流式重试一次:", truncate(cause.Error(), 200))
		}
		payload.Stream = false
		raw2, berr := c.buildBody(&payload)
		if berr != nil {
			return nil, cause
		}
		resp2, err2 := c.post(raw2, false)
		if err2 != nil {
			return nil, cause
		}
		return c.decode(resp2, false, time.Now())
	}

	start := time.Now()
	resp, err := c.post(raw, payload.Stream)
	if err != nil {
		if payload.Stream && shouldFallbackToNonStream(err) {
			return fallback(err)
		}
		return nil, err
	}
	if payload.Stream && resp.StatusCode >= 400 && resp.StatusCode < 500 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		httpErr := fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		if shouldFallbackToNonStream(httpErr) {
			return fallback(httpErr)
		}
		return nil, httpErr
	}
	return c.decode(resp, payload.Stream, start)
}

// buildBody marshals the effective payload and merges request_body plus
// the vendor top-level fields (thinking, reasoning_effort); explicit
// config wins over request_body.
func (c *AIClient) buildBody(payload *ChatRequest) ([]byte, error) {
	if payload.Stream {
		payload.StreamOptions = map[string]any{"include_usage": true}
	} else {
		payload.StreamOptions = nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Merge request_body config into the top-level JSON payload. This
	// allows vendor-specific fields (enable_thinking, etc.) to be
	// injected without code changes.
	if len(c.requestBody) > 0 {
		raw, err = injectRequestBody(raw, c.requestBody)
		if err != nil {
			return nil, err
		}
	}
	extra := map[string]interface{}{}
	if c.thinking != nil {
		extra["thinking"] = c.thinking
	}
	if c.reasoningEffort != "" {
		extra["reasoning_effort"] = c.reasoningEffort
	}
	if len(extra) > 0 {
		raw, err = injectRequestBody(raw, extra)
		if err != nil {
			return nil, err
		}
	}
	return raw, nil
}

// post performs the HTTP call with the transport matching the mode.
func (c *AIClient) post(raw []byte, stream bool) (*http.Response, error) {
	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	httpClient := c.http
	if stream {
		httpClient = c.streamHTTP
	}
	return httpClient.Do(httpReq)
}

// decode turns an HTTP response into a ChatResponse (streamed or not).
func (c *AIClient) decode(resp *http.Response, stream bool, start time.Time) (*ChatResponse, error) {
	defer resp.Body.Close()
	if stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return c.readStream(resp, start)
	}

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
	out.Elapsed = time.Since(start)
	return &out, nil
}

// shouldFallbackToNonStream reports whether an error suggests the
// gateway cannot serve the streaming variant of the request.
func shouldFallbackToNonStream(err error) bool {
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "stream") || strings.Contains(s, "event-stream") {
		return true
	}
	if strings.Contains(s, "unsupported") || strings.Contains(s, "not support") {
		return true
	}
	for _, code := range []string{"http 400", "http 404", "http 405", "http 415", "http 422"} {
		if strings.Contains(s, code) {
			return true
		}
	}
	return false
}

// readStream collects an SSE response into a ChatResponse, logging
// throttled progress in debug mode so a long thinking phase is visible.
func (c *AIClient) readStream(resp *http.Response, start time.Time) (*ChatResponse, error) {
	var lastLog time.Time
	var contentChars, reasoningChars int
	progress := func() {
		if c.log == nil || !c.log.TraceEnabled() {
			return
		}
		now := time.Now()
		if !lastLog.IsZero() && now.Sub(lastLog) < 10*time.Second {
			return
		}
		lastLog = now
		c.log.Trace(0, fmt.Sprintf("[stream] %s: elapsed=%s content=%d chars reasoning=%d chars",
			c.model, now.Sub(start).Round(time.Second), contentChars, reasoningChars))
	}
	res, err := chatstream.Collect(resp.Body, chatstream.Options{
		IdleTimeout: c.streamIdle,
		OnContent: func(s string) {
			contentChars += len(s)
			progress()
		},
		OnReasoning: func(s string) {
			reasoningChars += len(s)
			progress()
		},
	})
	if err != nil {
		return nil, err
	}
	msgRaw, err := json.Marshal(res.Message)
	if err != nil {
		return nil, fmt.Errorf("assemble streamed message: %w", err)
	}
	var msg ChatMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return nil, fmt.Errorf("decode streamed message: %w", err)
	}
	out := &ChatResponse{
		Choices:        []ChatResponseChoice{{Message: msg}},
		Elapsed:        time.Since(start),
		Streamed:       true,
		ReasoningChars: res.ReasoningChars,
		FinishReason:   res.FinishReason,
	}
	if res.Usage != nil {
		usageRaw, err := json.Marshal(res.Usage)
		if err == nil {
			var u Usage
			if json.Unmarshal(usageRaw, &u) == nil {
				out.Usage = &u
			}
		}
	}
	return out, nil
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
