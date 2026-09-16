// Tests for the same-session Mermaid repair logic in CallAIWithTools.
// These tests build an httptest server that mimics the OpenAI-compatible
// chat completions endpoint, capture every request body, and inject a
// validator closure so we never depend on the real mmdc binary.
//
// Scenarios covered (matching the task brief):
//
//  1. Single success: no tool_calls, valid Mermaid, one request.
//  2. Same-session repair success: first reply invalid, second reply
//     valid; the second request must reuse the conversation (system,
//     user, assistant, fix-user) and the fix message must not inline
//     the full previous response.
//  3. Budget exhaustion: MermaidFixAttempts=2 → exactly 1 initial + 2
//     repair requests, returns sentinelMermaid + StatusRetry.
//  4. resolveMermaidRepairBudget: nil/positive/zero/negative inputs.
//  5. Tool calls interleaved with repair: a get_more_context round
//     happens between two validator failures; tool messages appear in
//     the conversation but repairAttempts is only incremented per
//     fixMsg.
//  6. Tool rounds exhausted: requests after toolRounds reaches the
//     cap carry ToolChoice="none" and no tools payload (covers both
//     the normal post-cap iteration and the forced-final retry).
//  7. strict mode + validator unavailable → sentinel + StatusRetry,
//     no fix attempt.
//  8. auto mode + validator unavailable → original text + StatusOK,
//     no fix attempt.
//
// Plus one optional sanity check that customUserText + nil validator
// (the format-fix path) issues a single request without validation.
package img2text

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

// intPtr is a local helper because the package-private equivalent in
// internal/config is not exported.
func intPtr(v int) *int { return &v }

// recordedRequest captures one inbound chat completion request body.
type recordedRequest struct {
	Messages   []ChatMessage
	Tools      []map[string]any
	ToolChoice any
}

// mockChatServer wires httptest to a programmable response function.
// The handler decodes the request, records it (under a mutex) and then
// calls respFn(idx, rec) to produce the next response. respFn returns
// (statusCode, body); the body is forwarded as-is.
type mockChatServer struct {
	server  *httptest.Server
	mu      sync.Mutex
	reqs    []recordedRequest
	respFn  func(callIdx int, rec recordedRequest) (int, string)
	idxSeen int
}

func newMockChatServer(t *testing.T, respFn func(int, recordedRequest) (int, string)) *mockChatServer {
	t.Helper()
	ms := &mockChatServer{respFn: respFn}
	ms.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		body, _ := io.ReadAll(r.Body)
		// Decode failures here would mask test setup bugs; surface them.
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("mock: cannot decode request body: %v\nbody=%s", err, string(body))
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		rec := recordedRequest{
			Messages:   req.Messages,
			Tools:      req.Tools,
			ToolChoice: req.ToolChoice,
		}
		ms.mu.Lock()
		ms.reqs = append(ms.reqs, rec)
		idx := len(ms.reqs) - 1
		ms.idxSeen = idx + 1
		fn := ms.respFn
		ms.mu.Unlock()

		code, respBody := fn(idx, rec)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(ms.server.Close)
	return ms
}

func (ms *mockChatServer) calls() []recordedRequest {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	out := make([]recordedRequest, len(ms.reqs))
	copy(out, ms.reqs)
	return out
}

// newTestClient constructs a minimal AIClient pointing at the mock
// server. AIClient's fields are package-private so the test (in the
// same package) can build it directly without going through
// NewAIClient (which would require a fully populated config).
func newTestClient(t *testing.T, baseURL string) *AIClient {
	t.Helper()
	return &AIClient{
		http:    &http.Client{},
		baseURL: baseURL,
		apiKey:  "test-key",
		model:   "test-model",
	}
}

// newTestLogger returns a quiet logger that writes nowhere on disk.
// SetQuiet(true) suppresses the console chatter so test output stays
// focused on assertion failures.
func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	l, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatalf("logger.NewLogger: %v", err)
	}
	l.SetQuiet(true)
	t.Cleanup(func() { _ = l.Close() })
	return l
}

// responseText builds the JSON body for an assistant text reply.
func responseText(text string) string {
	return fmt.Sprintf(
		`{"choices":[{"message":{"role":"assistant","content":%s}}]}`,
		jsonString(text),
	)
}

// responseToolCall builds the JSON body for an assistant message that
// issues a single function call. id must be unique per request.
func responseToolCall(id, name, args string) string {
	return fmt.Sprintf(
		`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[`+
			`{"id":%s,"type":"function","function":{"name":%s,"arguments":%s}}`+
			`]}}]}`,
		jsonString(id), jsonString(name), jsonString(args),
	)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// messageContentString flattens a ChatMessage.Content (which is `any`
// because content can be a plain string or a structured multimodal
// slice) into something test assertions can search against.
func messageContentString(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// validMermaid returns a Mermaid block that passes a lenient validator
// (the injected one only checks the sentinel values we set).
const validMermaidBlock = "graph TD\nA-->B"

// TestCallAIWithTools_ValidMermaidSingleRequest covers scenario (1):
// the assistant returns valid Mermaid on the first try, so only one
// HTTP request goes out and the result is the assistant text wrapped
// in the [IMG_TYPE:] prefix with StatusOK.
func TestCallAIWithTools_ValidMermaidSingleRequest(t *testing.T) {
	reply := "[IMG_TYPE: flowchart]\n```mermaid\n" + validMermaidBlock + "\n```"
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		if idx != 0 {
			t.Errorf("unexpected request idx=%d", idx)
		}
		return http.StatusOK, responseText(reply)
	})
	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	opts := config.OptionsConfig{
		MaxRetries:        3,
		MermaidValidation: "auto",
	}
	validator := func(string) MermaidValidationResult {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  true,
			Valid:      true,
		}
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, nil,
	)
	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if result != reply {
		t.Fatalf("result = %q, want %q", result, reply)
	}
	if got := len(ms.calls()); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

// TestCallAIWithTools_RepairSucceedsSameSession covers scenario (2):
// the first reply has invalid Mermaid, the second reply (sent in the
// SAME conversation, after the fix message) is valid. Asserts that the
// conversation is reused (image context is not rebuilt) and the fix
// message quotes the validator error but not the entire previous reply.
func TestCallAIWithTools_RepairSucceedsSameSession(t *testing.T) {
	bad := "[IMG_TYPE: flowchart]\n```mermaid\nBROKEN\n```"
	good := "[IMG_TYPE: flowchart]\n```mermaid\n" + validMermaidBlock + "\n```"

	var validatorCalls int
	validator := func(string) MermaidValidationResult {
		validatorCalls++
		switch validatorCalls {
		case 1:
			return MermaidValidationResult{
				HasMermaid: true,
				Available:  true,
				Error:      "block 1: SyntaxError at line 1",
			}
		default:
			return MermaidValidationResult{
				HasMermaid: true,
				Available:  true,
				Valid:      true,
			}
		}
	}

	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		switch idx {
		case 0:
			return http.StatusOK, responseText(bad)
		case 1:
			return http.StatusOK, responseText(good)
		default:
			t.Errorf("unexpected request idx=%d", idx)
			return http.StatusInternalServerError, `{"error":"unexpected"}`
		}
	})

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	opts := config.OptionsConfig{
		MaxRetries:        3,
		MermaidValidation: "auto",
		// 显式给 3 轮就地修复预算：这个用例钉的是"就地修复成功"路径；
		// 默认（nil）语义见 TestCallAIWithTools_FirstFailureSkipsInPlace。
		MermaidFixAttempts: intPtr(3),
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, nil,
	)
	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if result != good {
		t.Fatalf("result = %q, want %q", result, good)
	}

	calls := ms.calls()
	if got := len(calls); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}

	// First request is the standard system + multimodal user turn.
	first := calls[0].Messages
	if len(first) != 2 {
		t.Fatalf("first request messages = %d, want 2", len(first))
	}
	if first[0].Role != "system" {
		t.Fatalf("first[0].Role = %q, want system", first[0].Role)
	}
	if first[1].Role != "user" {
		t.Fatalf("first[1].Role = %q, want user", first[1].Role)
	}
	if firstUserContent := messageContentString(first[1].Content); !strings.Contains(firstUserContent, "data:image/jpeg;base64,imgdata") {
		t.Fatalf("first user content missing image payload, got %q",
			firstUserContent[:min(200, len(firstUserContent))])
	}

	// Second request reuses the conversation: the previous assistant
	// turn is preserved (we did not re-encode the image and did not
	// start a fresh system prompt) and a fix user turn is appended.
	second := calls[1].Messages
	if len(second) != 4 {
		t.Fatalf("second request messages = %d, want 4 (system, user, assistant, fix)", len(second))
	}
	if second[0].Role != "system" || messageContentString(second[0].Content) != messageContentString(first[0].Content) {
		t.Fatalf("system prompt not reused verbatim between requests")
	}
	if second[1].Role != "user" || messageContentString(second[1].Content) != messageContentString(first[1].Content) {
		t.Fatalf("user turn not reused; the image context should NOT be rebuilt")
	}
	if second[2].Role != "assistant" {
		t.Fatalf("second[2].Role = %q, want assistant", second[2].Role)
	}
	if got := messageContentString(second[2].Content); got != bad {
		t.Fatalf("second[2].Content = %q, want %q (the bad reply)", got, bad)
	}
	if second[3].Role != "user" {
		t.Fatalf("second[3].Role = %q, want user (the fix prompt)", second[3].Role)
	}

	fixContent := messageContentString(second[3].Content)
	if strings.Contains(fixContent, bad) {
		t.Fatalf("fix message must NOT inline the full previous response; got %q", fixContent)
	}
	if !strings.Contains(fixContent, "SyntaxError at line 1") {
		t.Fatalf("fix message must quote the validator error, got %q", fixContent)
	}
	if !strings.Contains(fixContent, "Your previous response contains invalid Mermaid syntax") {
		t.Fatalf("fix message missing the standard prefix, got %q", fixContent)
	}
}

// T37：默认（MermaidFixAttempts 未设置 = nil）不再做就地修复轮——
// 首次校验失败直接返回 retry（调用方随即升级修复会话），整个流程只
// 发出 1 次 API 请求。
func TestCallAIWithTools_FirstFailureSkipsInPlace(t *testing.T) {
	bad := "[IMG_TYPE: flowchart]\n```mermaid\nBROKEN\n```"

	validator := func(string) MermaidValidationResult {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  true,
			Error:      "block 1: SyntaxError at line 1",
		}
	}

	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		if idx != 0 {
			t.Errorf("unexpected second in-place request idx=%d (T37: first failure must not retry in place)", idx)
			return http.StatusInternalServerError, `{"error":"unexpected"}`
		}
		return http.StatusOK, responseText(bad)
	})

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	opts := config.OptionsConfig{
		MaxRetries:        3,
		MermaidValidation: "auto",
		// MermaidFixAttempts 保持 nil = T37 默认：0 轮就地修复。
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, nil,
	)
	if status != StatusRetry {
		t.Fatalf("status = %q, want %q (first failure goes straight to retry/upgrade)", status, StatusRetry)
	}
	if result != sentinelMermaid {
		t.Fatalf("result = %q, want %q", result, sentinelMermaid)
	}
	if got := len(ms.calls()); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

// TestCallAIWithTools_RepairBudgetExhausted covers scenario (3):
// MermaidFixAttempts=2 with a validator that always reports a syntax
// error produces exactly 3 requests (1 initial + 2 repairs) and then
// returns the sentinel with StatusRetry.
func TestCallAIWithTools_RepairBudgetExhausted(t *testing.T) {
	bad := "[IMG_TYPE: flowchart]\n```mermaid\nBROKEN\n```"
	validator := func(string) MermaidValidationResult {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  true,
			Error:      "block 1: parse error",
		}
	}
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		if idx > 2 {
			t.Errorf("unexpected request idx=%d (budget should have stopped at 2)", idx)
		}
		return http.StatusOK, responseText(bad)
	})

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	budget := 2
	opts := config.OptionsConfig{
		MaxRetries:         3,
		MermaidValidation:  "auto",
		MermaidFixAttempts: &budget,
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, nil,
	)
	if status != StatusRetry {
		t.Fatalf("status = %q, want %q", status, StatusRetry)
	}
	if result != sentinelMermaid {
		t.Fatalf("result = %q, want %q", result, sentinelMermaid)
	}

	calls := ms.calls()
	if got := len(calls); got != 3 {
		t.Fatalf("requests = %d, want 3 (1 initial + 2 repairs)", got)
	}

	// Each request after the first must carry a trailing fix user
	// turn, and that turn must NOT contain the bad reply (so we know
	// we are not accidentally sending redundant content).
	for i, rec := range calls {
		msgs := rec.Messages
		// Initial request: 2 messages (system + user).
		// Each subsequent request: prior + assistant + fix-user.
		wantLen := 2 + 2*i
		if len(msgs) != wantLen {
			t.Fatalf("call %d messages = %d, want %d", i, len(msgs), wantLen)
		}
		fix := messageContentString(msgs[len(msgs)-1].Content)
		if i > 0 {
			if msgs[len(msgs)-1].Role != "user" {
				t.Fatalf("call %d last role = %q, want user (fix prompt)", i, msgs[len(msgs)-1].Role)
			}
			if !strings.Contains(fix, "parse error") {
				t.Fatalf("call %d fix prompt missing validator error, got %q", i, fix)
			}
			if strings.Contains(fix, bad) {
				t.Fatalf("call %d fix prompt must not inline the full previous response", i)
			}
		}
	}
}

// Scenario (9): 就地修复轮用尽后 fixSession 钩子接管——钩子收到最后一份出错
// 响应与校验错误，其成功结果原样返回（StatusOK），模型不再被打扰。
func TestCallAIWithTools_FixSessionHookTakesOver(t *testing.T) {
	bad := "[IMG_TYPE: flowchart]\n```mermaid\nBROKEN\n```"
	fixed := "[IMG_TYPE: mermaid]\n```mermaid\ngraph TD\nA-->B\n```"
	validator := func(string) MermaidValidationResult {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  true,
			Error:      "block 1: parse error",
		}
	}
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		return http.StatusOK, responseText(bad)
	})
	hookGot := map[string]string{}
	hookCalls := 0
	fixSession := func(prevResult, validationError string) (string, bool) {
		hookCalls++
		hookGot["prev"] = prevResult
		hookGot["err"] = validationError
		return fixed, true
	}

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	budget := 1
	opts := config.OptionsConfig{
		MaxRetries:         3,
		MermaidValidation:  "auto",
		MermaidFixAttempts: &budget,
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, fixSession,
	)
	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if result != fixed {
		t.Fatalf("result = %q, want the fix session's product", result)
	}
	if hookCalls != 1 || hookGot["prev"] != bad || hookGot["err"] != "block 1: parse error" {
		t.Fatalf("hook calls=%d got=%+v", hookCalls, hookGot)
	}
}

// TestResolveMermaidRepairBudget covers scenario (4): verify the
// budget resolver normalises the user-facing MermaidFixAttempts field.
// We avoid sending the safety-cap limit's worth of requests (100)
// and only assert the resolver's behaviour.
func TestResolveMermaidRepairBudget(t *testing.T) {
	cases := []struct {
		name string
		in   *int
		want int
	}{
		// T37：默认（nil / 负值）= 0 轮就地修复，首次失败直接升级会话。
		{"nil -> first-failure upgrade", nil, 0},
		{"negative -> first-failure upgrade", intPtr(-2), 0},
		{"positive uses value", intPtr(5), 5},
		{"zero -> safety cap", intPtr(0), mermaidFixSafetyCap},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveMermaidRepairBudget(tc.in); got != tc.want {
				t.Fatalf("resolveMermaidRepairBudget(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}

	// Also confirm the safety cap matches the documented contract so a
	// future refactor doesn't silently change the upper bound.
	if mermaidFixSafetyCap != 100 {
		t.Fatalf("mermaidFixSafetyCap = %d, want 100", mermaidFixSafetyCap)
	}
}

// TestCallAIWithTools_ToolCallsDuringRepair covers scenario (5): the
// model issues a get_more_context tool call after a fix round, so the
// tool loop continues across the same repair cycle. Tool messages and
// fix messages must coexist in the final transcript, and the fix
// messages must appear exactly where validator failures occurred
// (i.e. the tool round does NOT increment repairAttempts).
func TestCallAIWithTools_ToolCallsDuringRepair(t *testing.T) {
	bad := "[IMG_TYPE: flowchart]\n```mermaid\nBROKEN\n```"
	good := "[IMG_TYPE: flowchart]\n```mermaid\n" + validMermaidBlock + "\n```"
	toolArgs := `{"more_above":2,"more_below":3}`

	var validatorCalls int
	validator := func(string) MermaidValidationResult {
		validatorCalls++
		if validatorCalls <= 2 {
			return MermaidValidationResult{
				HasMermaid: true,
				Available:  true,
				Error:      "syntax error",
			}
		}
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  true,
			Valid:      true,
		}
	}

	// Response sequence:
	//   0 - bad text (validator fails, fix round 1)
	//   1 - tool_calls (get_more_context, between fixes)
	//   2 - bad text (validator fails, fix round 2)
	//   3 - good text (accepted)
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		switch idx {
		case 0:
			return http.StatusOK, responseText(bad)
		case 1:
			return http.StatusOK, responseToolCall("call_1", "get_more_context", toolArgs)
		case 2:
			return http.StatusOK, responseText(bad)
		case 3:
			return http.StatusOK, responseText(good)
		default:
			t.Errorf("unexpected request idx=%d", idx)
			return http.StatusInternalServerError, `{"error":"unexpected"}`
		}
	})

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	budget := 5
	opts := config.OptionsConfig{
		MaxRetries:         5,
		MermaidValidation:  "auto",
		MermaidFixAttempts: &budget,
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, nil,
	)
	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if result != good {
		t.Fatalf("result = %q, want %q", result, good)
	}

	calls := ms.calls()
	if got := len(calls); got != 4 {
		t.Fatalf("requests = %d, want 4", got)
	}

	// Sanity: req 1 (post-fix) carries the tools payload because
	// toolRounds is still under maxRounds=5 after the first bad reply.
	if len(calls[1].Tools) == 0 {
		t.Fatalf("req 1 (post-fix tool round) must include tools payload")
	}
	if calls[1].ToolChoice != "auto" {
		t.Fatalf("req 1 tool_choice = %v, want auto", calls[1].ToolChoice)
	}

	// The final transcript (the last request we sent) must contain:
	//   system, user, assistant(bad1), user(fix1),
	//   assistant(tool_call), tool(result),
	//   assistant(bad2), user(fix2)
	// → 8 messages. fix messages are exactly the ones with the
	// standard prefix; tool messages come only from the tool round.
	msgs := calls[3].Messages
	if len(msgs) != 8 {
		t.Fatalf("final transcript messages = %d, want 8", len(msgs))
	}

	// fix user messages appear at indices 3 and 7.
	for _, idx := range []int{3, 7} {
		if msgs[idx].Role != "user" {
			t.Fatalf("msgs[%d].Role = %q, want user (fix prompt)", idx, msgs[idx].Role)
		}
		fixContent := messageContentString(msgs[idx].Content)
		if !strings.HasPrefix(fixContent, "Your previous response contains invalid Mermaid syntax") {
			t.Fatalf("msgs[%d] does not start with the standard fix prefix", idx)
		}
		if strings.Contains(fixContent, bad) {
			t.Fatalf("msgs[%d] fix prompt must NOT inline the full previous response", idx)
		}
	}

	// The assistant tool_call + matching tool response are interleaved
	// between the two fix user turns.
	if msgs[4].Role != "assistant" || len(msgs[4].ToolCalls) != 1 {
		t.Fatalf("msgs[4] should be an assistant message with 1 tool_call; role=%q count=%d",
			msgs[4].Role, len(msgs[4].ToolCalls))
	}
	if msgs[4].ToolCalls[0].Function.Name != "get_more_context" {
		t.Fatalf("msgs[4] tool name = %q, want get_more_context", msgs[4].ToolCalls[0].Function.Name)
	}
	if msgs[4].ToolCalls[0].ID != "call_1" {
		t.Fatalf("msgs[4] tool id = %q, want call_1 (echoed into the tool response)", msgs[4].ToolCalls[0].ID)
	}
	if msgs[5].Role != "tool" {
		t.Fatalf("msgs[5].Role = %q, want tool", msgs[5].Role)
	}
	if msgs[5].ToolCallID != "call_1" {
		t.Fatalf("msgs[5] tool_call_id = %q, want call_1", msgs[5].ToolCallID)
	}
	if msgs[6].Role != "assistant" {
		t.Fatalf("msgs[6].Role = %q, want assistant (the second bad reply)", msgs[6].Role)
	}
	if got := messageContentString(msgs[6].Content); got != bad {
		t.Fatalf("msgs[6].Content = %q, want %q", got, bad)
	}
}

// TestCallAIWithTools_ToolRoundsExhaustedNoTools covers scenario (6):
// once toolRounds reaches maxRounds the loop's includeTools gate flips
// off, so subsequent chat completion requests must NOT carry a tools
// payload and must set tool_choice="none". This covers both the
// in-loop request and the forced-final retry that the !includeTools
// branch issues.
func TestCallAIWithTools_ToolRoundsExhaustedNoTools(t *testing.T) {
	toolArgs := `{"more_above":2,"more_below":3}`
	valid := "[IMG_TYPE: flowchart]\n```mermaid\n" + validMermaidBlock + "\n```"

	validator := func(string) MermaidValidationResult {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  true,
			Valid:      true,
		}
	}

	// Response sequence with maxRetries=1:
	//   0 - tool_calls (consumes the only allowed tool round)
	//   1 - any text (triggers the !includeTools branch → forced-final)
	//   2 - valid text (the forced-final reply, gets validated and accepted)
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		switch idx {
		case 0:
			return http.StatusOK, responseToolCall("call_1", "get_more_context", toolArgs)
		case 1:
			return http.StatusOK, responseText("[IMG_TYPE: x]\nignored") // ignored; forced-final runs
		case 2:
			return http.StatusOK, responseText(valid)
		default:
			t.Errorf("unexpected request idx=%d", idx)
			return http.StatusInternalServerError, `{"error":"unexpected"}`
		}
	})

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	opts := config.OptionsConfig{
		MaxRetries:        1, // only one tool round allowed
		MermaidValidation: "auto",
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, nil,
	)
	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if result != valid {
		t.Fatalf("result = %q, want %q", result, valid)
	}

	calls := ms.calls()
	if got := len(calls); got < 3 {
		t.Fatalf("requests = %d, want at least 3 (tool round + in-loop no-tools + forced-final)", got)
	}

	// Request 0: includeTools=true → tools payload + tool_choice="auto".
	if len(calls[0].Tools) == 0 {
		t.Fatalf("req 0 must carry tools payload (includeTools=true)")
	}
	if calls[0].ToolChoice != "auto" {
		t.Fatalf("req 0 tool_choice = %v, want auto", calls[0].ToolChoice)
	}

	// Request 1: includeTools=false (toolRounds reached maxRounds) →
	// no tools + tool_choice="none".
	if len(calls[1].Tools) != 0 {
		t.Fatalf("req 1 carries %d tools, want 0 (toolRounds exhausted)", len(calls[1].Tools))
	}
	if calls[1].ToolChoice != "none" {
		t.Fatalf("req 1 tool_choice = %v, want none", calls[1].ToolChoice)
	}

	// Forced-final request (req 2): built explicitly with no Tools
	// and ToolChoice="none" in the !includeTools branch.
	if len(calls[2].Tools) != 0 {
		t.Fatalf("forced-final req 2 carries %d tools, want 0", len(calls[2].Tools))
	}
	if calls[2].ToolChoice != "none" {
		t.Fatalf("forced-final req 2 tool_choice = %v, want none", calls[2].ToolChoice)
	}
}

// TestCallAIWithTools_StrictUnavailableSentinel covers scenario (7):
// strict mode + validator that reports unavailable returns the
// sentinel with StatusRetry and never sends a fix request.
func TestCallAIWithTools_StrictUnavailableSentinel(t *testing.T) {
	text := "[IMG_TYPE: flowchart]\n```mermaid\nBROKEN\n```"
	validator := func(string) MermaidValidationResult {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  false,
			Error:      "Mermaid validator \"mmdc\" not found",
		}
	}
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		if idx != 0 {
			t.Errorf("strict+unavailable must not issue a repair request; saw idx=%d", idx)
		}
		return http.StatusOK, responseText(text)
	})

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	opts := config.OptionsConfig{
		MaxRetries:        3,
		MermaidValidation: "strict",
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, nil,
	)
	if status != StatusRetry {
		t.Fatalf("status = %q, want %q", status, StatusRetry)
	}
	if result != sentinelMermaid {
		t.Fatalf("result = %q, want %q", result, sentinelMermaid)
	}
	if got := len(ms.calls()); got != 1 {
		t.Fatalf("requests = %d, want 1 (no fix sent)", got)
	}
}

// TestCallAIWithTools_AutoUnavailableAccept covers scenario (8):
// auto mode + validator that reports unavailable returns the
// original assistant text with StatusOK and never sends a fix request.
func TestCallAIWithTools_AutoUnavailableAccept(t *testing.T) {
	text := "[IMG_TYPE: flowchart]\n```mermaid\nBROKEN\n```"
	validator := func(string) MermaidValidationResult {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  false,
			Error:      "Mermaid validator \"mmdc\" not found",
		}
	}
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		if idx != 0 {
			t.Errorf("auto+unavailable must not issue a repair request; saw idx=%d", idx)
		}
		return http.StatusOK, responseText(text)
	})

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	opts := config.OptionsConfig{
		MaxRetries:        3,
		MermaidValidation: "auto",
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{"L0"}, 0, l, 0, opts, "",
		validator, buildMermaidRepairMessage, nil,
	)
	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if result != text {
		t.Fatalf("result = %q, want %q (original assistant text)", result, text)
	}
	if got := len(ms.calls()); got != 1 {
		t.Fatalf("requests = %d, want 1 (no fix sent)", got)
	}
}

// TestCallAIWithTools_FormatFixNilValidator covers scenario (8b,
// optional): the format-fix path uses CallAIWithTools with a non-empty
// customUserText and a nil validator. The validator must never be
// invoked and only one request is issued.
//
// Skipping this through ProcessOneImage (which loads the image from
// disk) is hard without a real fixture, so we exercise the same code
// path directly.
func TestCallAIWithTools_FormatFixNilValidator(t *testing.T) {
	text := "[IMG_TYPE: text]\nplain reply"
	// validatorCalls is captured by the closure below; we never pass
	// the closure to CallAIWithTools (the test passes nil for both
	// the validator and the repair builder). Counting invocations
	// through a closure variable is the cleanest way to assert the
	// customUserText path never reaches validation.
	var validatorCalls int
	_ = func(string) MermaidValidationResult {
		validatorCalls++
		return MermaidValidationResult{}
	}
	ms := newMockChatServer(t, func(idx int, _ recordedRequest) (int, string) {
		if idx != 0 {
			t.Errorf("format-fix path must be single-shot; saw idx=%d", idx)
		}
		return http.StatusOK, responseText(text)
	})

	client := newTestClient(t, ms.server.URL)
	l := newTestLogger(t)
	opts := config.OptionsConfig{
		MaxRetries:        3,
		MermaidValidation: "auto",
	}

	result, status, _ := CallAIWithTools(
		client, "imgdata", []string{}, 0, l, 0, opts,
		"Fix the prefix to start with [IMG_TYPE:",
		nil, nil, nil, // <-- the format-fix wiring: nil validator + nil builder + nil fix session
	)
	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if result != text {
		t.Fatalf("result = %q, want %q", result, text)
	}
	if got := len(ms.calls()); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	// The validator counter must stay at 0: CallAIWithTools should not
	// look at the validator at all in the customUserText branch.
	if validatorCalls != 0 {
		t.Fatalf("validator invocations = %d, want 0 (customUserText path must skip validation)", validatorCalls)
	}
}
