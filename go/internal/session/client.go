// Package session provides the reusable AI conversation engine used by
// the LaTeX pipelines: an OpenAI-compatible client, a multi-turn
// session with a pluggable tool registry, a configurable context
// window and AI-driven auto-compaction when the window fills up.
package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/chatstream"
	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// Client is a thin HTTP wrapper around an OpenAI-compatible chat
// completions endpoint. It is stateless and safe to share across
// goroutines; retry/backoff lives in CallWithRetry.
type Client struct {
	http        *http.Client // non-streaming: connect + read total timeout
	streamHTTP  *http.Client // streaming: no total timeout, idle watchdog instead
	baseURL     string
	apiKey      string
	model       string
	requestBody map[string]interface{}
	log         *logger.Logger // optional; logs retry/backoff waits and stream progress when set

	// Vendor top-level request fields (sent beside model/messages, NOT
	// inside request_body.extra_body).
	thinking        map[string]any
	reasoningEffort string
	toolStream      bool // GLM: stream tool-call arguments alongside content

	// Streaming policy resolved from ModelConfig (default: on).
	stream     bool
	streamIdle time.Duration

	// Fallbacks applied when a call does not set them itself.
	maxTokens   int
	temperature float64

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
	streamIdle := read
	if cfg.APIStreamIdleTimeout > 0 {
		streamIdle = time.Duration(cfg.APIStreamIdleTimeout) * time.Second
	}
	maxRetries := cfg.APIMaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	rateLimit := cfg.RateLimitRetries
	if rateLimit <= 0 {
		// 429 限流重试上限（退避封顶 60s）。账户级错误（余额/配额）不走
		// 这条路——它们已经直接判定为 [SESSION_INSUFFICIENT_BALANCE] 立即
		// 返回，所以这里只是"真限流"的自恢复次数，20 次足够（用户指定）。
		rateLimit = 20
	}
	// The streaming client must NOT carry a total timeout: a healthy
	// long thinking/output stream would be killed by it. The connect /
	// first-byte wait stays bounded, and chatstream aborts on silence.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: connect, KeepAlive: 30 * time.Second}).DialContext
	tr.ResponseHeaderTimeout = connect
	return &Client{
		http:            &http.Client{Timeout: connect + read},
		streamHTTP:      &http.Client{Transport: tr},
		baseURL:         strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:          cfg.APIKey,
		model:           cfg.Model,
		requestBody:     cfg.RequestBody,
		thinking:        cfg.Thinking,
		reasoningEffort: cfg.ReasoningEffort,
		toolStream:      cfg.ToolStream != nil && *cfg.ToolStream,
		stream:          cfg.Streaming(),
		streamIdle:      streamIdle,
		maxTokens:       cfg.MaxTokens,
		temperature:     cfg.Temperature,
		maxAPIRetries:   maxRetries,
		rateLimitCap:    rateLimit,
	}
}

// Model returns the configured model name.
func (c *Client) Model() string { return c.model }

// Streaming reports whether this client uses SSE streaming.
func (c *Client) Streaming() bool { return c.stream }

// StreamIdleTimeout returns the silence budget used while streaming.
func (c *Client) StreamIdleTimeout() time.Duration { return c.streamIdle }

// RequestSummary renders the effective request parameters for debug
// logs: model, streaming, budget and the vendor thinking controls.
func (c *Client) RequestSummary(req *ChatRequest) string {
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
	clearThinking := "-"
	if c.thinking != nil {
		if v, ok := c.thinking["clear_thinking"]; ok {
			clearThinking = fmt.Sprint(v)
		}
	}
	return fmt.Sprintf("model=%s stream=%v tool_stream=%v max_tokens=%d temperature=%.2f thinking=%s clear_thinking=%s reasoning_effort=%s",
		c.model, c.stream, c.toolStream, maxTokens, temp, think, clearThinking, effort)
}

// ChatRequest mirrors the OpenAI chat completions schema.
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
	// User is a stable per-session identifier (the OpenAI `user` field).
	// Gateways use it for routing/affinity: new-api can hash on it so one
	// conversation always reaches the SAME upstream channel, which is what
	// keeps the vendor's prefix cache (per upstream key) warm across turns.
	User string `json:"user,omitempty"`
	// StreamHook (P7) receives throttled progressive snapshots of the
	// streaming message ("content" / "reasoning" + accumulated text, tail
	// capped). Per-request field: concurrent sessions sharing one client
	// must not overwrite each other's hook.
	StreamHook func(phase, text string) `json:"-"`
}

// ChatMessage is one message in a conversation. Content is either a
// string or a []map multimodal part list.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// ReasoningContent 思维链（assistant 消息）：逐字节存回历史并随
	// 请求重发——GLM 保留式思考（clear_thinking:false）要求完整回传，
	// 同时保证请求前缀逐字节一致以提高 prompt cache 命中。
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// ImageTokens is the LOCAL token estimate of every image part of this
	// message, in the order the images appear (see ImageTokens). It exists so
	// the context estimate counts pictures by their dimensions instead of one
	// flat constant; json:"-" keeps it off the wire, where the vendor counts
	// the pixels itself and reports the total in prompt_tokens.
	ImageTokens []int `json:"-"`
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

// ChatResponse is the subset of the response we use, plus diagnostics
// filled in by the client (never decoded from the wire).
type ChatResponse struct {
	Choices []ChatResponseChoice `json:"choices"`
	Usage   *Usage               `json:"usage,omitempty"`

	// Elapsed is the wall-clock duration of the call.
	Elapsed time.Duration `json:"-"`
	// TTFT is the time from sending the request to the FIRST streamed delta
	// (thinking counts: the first visible output of a reasoning model). For a
	// non-streamed reply the whole body arrives at once, so TTFT equals Elapsed
	// — callers averaging latencies must look at Streamed before mixing them.
	TTFT time.Duration `json:"-"`
	// Streamed reports whether the reply arrived as an SSE stream.
	Streamed bool `json:"-"`
	// ReasoningChars counts reasoning/thinking deltas (streaming only).
	ReasoningChars int `json:"-"`
	// FinishReason is the provider finish_reason (streaming only).
	FinishReason string `json:"-"`
}

// Usage is the provider token accounting.
type Usage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details,omitempty"`
	// PromptTokensDetails carries the provider's prefix-cache accounting
	// (Zhipu/GLM report cached_tokens here); it makes cache hits visible
	// in the debug log, which is the only way to see why a request missed.
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
}

// CachedTokens reports the provider-side prefix cache hit size (0 when the
// provider does not report it).
func (u *Usage) CachedTokens() int {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CachedTokens
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
	line := fmt.Sprintf("prompt=%d completion=%d total=%d reasoning=%d",
		u.PromptTokens, u.CompletionTokens, u.TotalTokens, reasoning)
	if c := u.CachedTokens(); c > 0 {
		line += fmt.Sprintf(" cached=%d(%.0f%%)", c, float64(c)*100/float64(max(u.PromptTokens, 1)))
	}
	return line
}

// ChatResponseChoice is one entry of choices.
type ChatResponseChoice struct {
	Message ChatMessage `json:"message"`
}

// ChatCompletion POSTs the request. Non-2xx responses are returned as
// errors so the retry loop can react to 429/5xx uniformly.
//
// The client owns the transport policy: it forces req.Stream to the
// configured mode, fills in the per-model max_tokens/temperature
// fallbacks, and merges the vendor top-level fields (thinking,
// reasoning_effort) after request_body so explicit config wins.
func (c *Client) ChatCompletion(req *ChatRequest) (*ChatResponse, error) {
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
	c.logCacheProbe(&payload, raw)
	fallback := func(cause error) (*ChatResponse, error) {
		// Gateway without SSE (or without streamed tool calls): retry
		// once as a single JSON response instead of burning the whole
		// retry budget on the same unsupported request.
		c.logf("  [stream] 流式请求失败，改用非流式重试一次:", truncate(cause.Error(), 200))
		payload.Stream = false
		raw2, berr := c.buildBody(&payload)
		if berr != nil {
			return nil, cause
		}
		resp2, err2 := c.post(raw2, false)
		if err2 != nil {
			return nil, cause
		}
		return c.decode(resp2, false, time.Now(), nil)
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
	return c.decode(resp, payload.Stream, start, req.StreamHook)
}

// buildBody marshals the effective payload and merges the configured
// request_body plus the vendor top-level fields (thinking,
// reasoning_effort); explicit config wins over request_body.
func (c *Client) buildBody(payload *ChatRequest) ([]byte, error) {
	if payload.Stream {
		payload.StreamOptions = map[string]any{"include_usage": true}
	} else {
		payload.StreamOptions = nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
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
	if c.toolStream {
		// GLM 工具流式输出：与 stream:true 搭配，工具参数随流增量返回。
		// 我们的 SSE 组装器按 delta.tool_calls 增量累积，线格式兼容。
		extra["tool_stream"] = true
	}
	if len(extra) > 0 {
		raw, err = injectRequestBody(raw, extra)
		if err != nil {
			return nil, err
		}
	}
	return raw, nil
}

// logCacheProbe writes a debug fingerprint of the outgoing body. Prefix
// caching (GLM `prompt_tokens_details.cached_tokens`) only pays off while
// the head of the request stays byte-identical: a changed head_sha means WE
// changed the prefix (system prompt, tool block, first messages), while an
// unchanged sha together with cached=0 points at the gateway routing the
// request to a different upstream (a provider-side cache is per upstream
// key/node, never global).
func (c *Client) logCacheProbe(payload *ChatRequest, raw []byte) {
	if c.log == nil || !c.log.DebugEnabled() {
		return
	}
	head := raw
	if len(head) > 512 {
		head = head[:512]
	}
	sum := sha256.Sum256(head)
	tools := "none"
	if payload.Tools != nil {
		tools = fmt.Sprintf("%d", len(payload.Tools))
	}
	// ToolChoice is `any`: %q on a nil (classify/other pipelines leave it
	// unset) printed the Go format artifact `%!q(<nil>)`.
	choice := "auto"
	if v, ok := payload.ToolChoice.(string); ok && v != "" {
		choice = v
	} else if payload.ToolChoice != nil {
		choice = fmt.Sprintf("%v", payload.ToolChoice)
	}
	c.log.Debug(0, fmt.Sprintf("[cache-probe] body=%dB head_sha=%x tools=%s tool_choice=%s messages=%d user=%q",
		len(raw), sum[:8], tools, choice, len(payload.Messages), payload.User))
}

// post performs the HTTP call with the transport matching the mode.
func (c *Client) post(raw []byte, stream bool) (*http.Response, error) {
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
func (c *Client) decode(resp *http.Response, stream bool, start time.Time, hook func(phase, text string)) (*ChatResponse, error) {
	defer resp.Body.Close()
	if stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return c.readStream(resp, start, hook)
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
	out.TTFT = out.Elapsed // one JSON body: nothing to measure separately
	return &out, nil
}

// logf writes a warning line when a logger is attached.
func (c *Client) logf(args ...interface{}) {
	if c.log != nil {
		c.log.LogWarning(0, args...)
	}
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
	// Generic HTTP 400 is NOT a streaming hint (a malformed transcript
	// returns 400 too): only method/media/route errors trigger a fallback.
	for _, code := range []string{"http 404", "http 405", "http 415", "http 422"} {
		if strings.Contains(s, code) {
			return true
		}
	}
	return false
}

// readStream collects an SSE response into a ChatResponse, logging
// throttled progress in debug mode so a long thinking phase is visible.
func (c *Client) readStream(resp *http.Response, start time.Time, hook func(phase, text string)) (*ChatResponse, error) {
	var lastLog time.Time
	var contentChars, reasoningChars int
	var ttft time.Duration
	markFirst := func() {
		if ttft == 0 {
			ttft = time.Since(start)
		}
	}
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
	// P7: progressive partial snapshots. Throttled to ~150ms and tail-capped
	// so the sidecar write stays cheap even on fast, long streams.
	var contentBuf, reasoningBuf strings.Builder
	var lastHook time.Time
	emit := func(phase string, buf *strings.Builder) {
		if hook == nil || buf.Len() == 0 {
			return
		}
		now := time.Now()
		if now.Sub(lastHook) < 150*time.Millisecond {
			return
		}
		lastHook = now
		text := buf.String()
		if len(text) > 16*1024 {
			text = text[len(text)-16*1024:]
		}
		hook(phase, text)
	}
	res, err := chatstream.Collect(resp.Body, chatstream.Options{
		IdleTimeout: c.streamIdle,
		OnContent: func(s string) {
			markFirst()
			contentChars += len(s)
			progress()
			contentBuf.WriteString(s)
			emit("content", &contentBuf)
		},
		OnReasoning: func(s string) {
			markFirst()
			reasoningChars += len(s)
			progress()
			reasoningBuf.WriteString(s)
			emit("reasoning", &reasoningBuf)
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
		TTFT:           ttft,
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
		if c.log == nil {
			return
		}
		if tag == "RateLimit" {
			// 限速等待是常规自恢复事件，不进 error 日志。
			c.log.LogInfo(0, "  ["+tag+"] 等待重试:", wait)
		} else {
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
		// 余额/配额类错误与 HTTP 状态无关（网关可能用 400/402/429 任意一种
		// 报），等下去永远不会好：直接收尾，不做任何重试。
		if insufficientBalance(lower) {
			if c.log != nil {
				c.log.LogError(0, "  [RateLimit] 上游账户不可用（余额/配额），停止重试:", truncate(errStr, 160))
			}
			return nil, "[SESSION_INSUFFICIENT_BALANCE]", "error"
		}
		// 4xx 里的"客户端错误"（400 请求体不合法、401/403 密钥或权限、
		// 404 模型名、422 参数）重试多少次都是同一个结果。现场一次
		// `thinking.type: disable` 让服务端 400 拒掉每个请求，却被当成
		// "transient" 重试 5 次（2/4/8/16/30s）——8 张图光退避就烧掉十几分钟。
		// 408（超时）与 429（限流）走各自的通道，所以排除掉。
		if code, ok := httpStatusCode(errStr); ok && code >= 400 && code < 500 && code != 408 && code != 429 {
			return nil, fmt.Sprintf("[SESSION_API_ERROR: HTTP %d (不可重试): %s]", code, truncate(errStr, 200)), "error"
		}
		if strings.Contains(errStr, "429") || strings.Contains(lower, "rate") {
			// 账户级错误已在上面统一拦下（2026-09-11 实测 `余额不足或无可用
			// 资源包` 让三个转换会话在冻结的请求体上空转了 3h31m/239 次）。
			if rateRetry < rateLimitLimit {
				wait := backoffWait(rateRetry, 2*time.Second, 60*time.Second)
				waitLog("RateLimit", wait)
				time.Sleep(wait)
				rateRetry++
				continue
			}
			if c.log != nil {
				c.log.LogError(0, "  [RateLimit] 重试上限用尽（rate_limit_retries=", rateLimitLimit, "），放弃本次请求")
			}
			return nil, "[SESSION_RATE_LIMIT_EXCEEDED]", "error"
		}
		if containsAny(lower, "connect", "timeout", "handshake", "timed out") {
			if retry < maxAPIRetries {
				wait := backoffWait(retry, 5*time.Second, 60*time.Second)
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
			wait := backoffWait(retry, 2*time.Second, 30*time.Second)
			waitLog("APIRetry", wait)
			time.Sleep(wait)
			retry++
			continue
		}
		return nil, fmt.Sprintf("[SESSION_API_ERROR: %s]", truncate(errStr, 200)), "error"
	}
}

// httpStatusCode extracts the status code from the "HTTP 400: {...}"
// error string produced by ChatCompletion (both the streaming and the
// non-streaming path use that form).
func httpStatusCode(errStr string) (int, bool) {
	i := strings.Index(errStr, "HTTP ")
	if i < 0 {
		return 0, false
	}
	rest := errStr[i+len("HTTP "):]
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j == 0 {
		return 0, false
	}
	code, err := strconv.Atoi(rest[:j])
	if err != nil {
		return 0, false
	}
	return code, true
}

// backoffWait returns base*2^attempt capped at max. The shift is clamped
// BEFORE the multiplication: 1<<n * 2 * time.Second overflows int64 for
// n >= 55 and produced negative waits (-2562047h47m16.854775808s) in real
// runs, which time.Sleep treats as "no wait at all".
func backoffWait(attempt int, base, max time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 20 {
		attempt = 20
	}
	wait := base * time.Duration(int64(1)<<uint(attempt))
	if wait <= 0 || wait > max {
		wait = max
	}
	return wait
}

// insufficientBalance reports an upstream account/quota error: retrying
// is pointless, the operator must top up. Matched on the message body a
// gateway sends with HTTP 429 (e.g. "余额不足或无可用资源包,请充值",
// code 1113) plus the usual English equivalents.
func insufficientBalance(lower string) bool {
	for _, p := range []string{
		"余额不足", "无可用资源包", "请充值", "欠费", "配额",
		"insufficient", "no available resource", "quota", "out of credit", "billing", "balance",
	} {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
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
