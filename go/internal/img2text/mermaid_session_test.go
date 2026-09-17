package img2text

// P12 升级修复会话的单元测试：submit 检查 / 错误累计 → 升级 / 轮次上限 /
// resolve 归一化。mmdc 用假脚本替代（同 mermaid_test.go 的做法）：内容含
// "BROKEN" 时退出 1，否则退出 0。

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"mineru-tools/internal/config"
)

// fakeMMDC installs a shell script named mmdc that exits 1 when any
// argument or stdin marker "BROKEN" is involved, 0 otherwise.
func fakeMMDC(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a shell command")
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "mmdc")
	script := "#!/bin/sh\nfor a in \"$@\"; do\n  if [ -f \"$a\" ] && grep -q BROKEN \"$a\"; then exit 1; fi\ndone\nexit 0\n"
	// mmdc 收到 -i <文件>：文件内容含 BROKEN 即失败（模拟语法错误）。
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
}

// newFixState 组装一个挂到临时工作区的会话状态。
func newFixState(t *testing.T, rounds, errLimit int) (*mermaidFixSessionState, string) {
	t.Helper()
	fakeMMDC(t)
	ws := t.TempDir()
	cfg := &MermaidFixConfig{
		Rounds:     rounds,
		ErrorLimit: errLimit,
		Command:    "mmdc",
		Timeout:    5 * time.Second,
		Primary:    config.ModelConfig{Model: "m1"},
		Fallback:   config.ModelConfig{Model: "m2"},
	}
	st := &mermaidFixSessionState{cfg: cfg, ws: ws, log: newTestLogger(t), tid: 0}
	return st, ws
}

func TestMermaidFixSubmitOK(t *testing.T) {
	st, ws := newFixState(t, 6, 3)
	good := "[IMG_TYPE: mermaid]\n```mermaid\ngraph TD\nA-->B\n```"
	if err := os.WriteFile(filepath.Join(ws, mermaidFixSubmitFile), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	submit := st.tools()[3].(*mermaidSubmitTool)
	res, err := submit.Execute("{}")
	if err != nil {
		t.Fatal(err)
	}
	if !st.submitted || st.final != good {
		t.Fatalf("submitted=%v final=%q want submitted with %q", st.submitted, st.final, good)
	}
	if res.Text == "" || st.checks != 1 {
		t.Fatalf("checks=%d res=%q", st.checks, res.Text)
	}
}

func TestMermaidFixSubmitNoMermaidCounts(t *testing.T) {
	st, ws := newFixState(t, 6, 3)
	if err := os.WriteFile(filepath.Join(ws, mermaidFixSubmitFile), []byte("no block here"), 0o644); err != nil {
		t.Fatal(err)
	}
	submit := st.tools()[3].(*mermaidSubmitTool)
	if _, err := submit.Execute("{}"); err != nil {
		t.Fatal(err)
	}
	if st.submitted || st.errors != 1 {
		t.Fatalf("submitted=%v errors=%d want 1 error", st.submitted, st.errors)
	}
	// 错误落盘（compile_error.log 保留出错现场）。
	if _, err := os.Stat(filepath.Join(ws, mermaidFixErrorFile)); err != nil {
		t.Fatalf("compile_error.log not written: %v", err)
	}
}

func TestMermaidFixErrorBudgetTriggersEscalation(t *testing.T) {
	st, ws := newFixState(t, 6, 3)
	if err := os.WriteFile(filepath.Join(ws, mermaidFixSubmitFile), []byte("no block"), 0o644); err != nil {
		t.Fatal(err)
	}
	submit := st.tools()[3].(*mermaidSubmitTool)
	for i := 1; i <= 3; i++ {
		if _, err := submit.Execute("{}"); err != nil {
			t.Fatal(err)
		}
	}
	if !st.escalated {
		t.Fatal("escalated=false, want true after 3 compile errors")
	}
	// 已在备选段：继续失败不再二次升级。
	if _, err := submit.Execute("{}"); err != nil {
		t.Fatal(err)
	}
	if st.errors != 4 {
		t.Fatalf("errors=%d want 4", st.errors)
	}
}

func TestMermaidFixRoundsCap(t *testing.T) {
	st, ws := newFixState(t, 2, 99)
	if err := os.WriteFile(filepath.Join(ws, mermaidFixSubmitFile), []byte("no block"), 0o644); err != nil {
		t.Fatal(err)
	}
	submit := st.tools()[3].(*mermaidSubmitTool)
	for i := 0; i < 5; i++ {
		if _, err := submit.Execute("{}"); err != nil {
			t.Fatal(err)
		}
	}
	if st.checks != 2 {
		t.Fatalf("checks=%d want capped at rounds=2", st.checks)
	}
}

func TestMermaidFixWriteAndGrep(t *testing.T) {
	st, ws := newFixState(t, 6, 3)
	tools := st.tools()
	write := tools[0].(*mermaidWriteTool)
	grep := tools[1].(*mermaidGrepTool)
	if _, err := write.Execute(`{"content":"[IMG_TYPE: mermaid]\nBROKEN"}`); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(ws, mermaidFixSubmitFile))
	if err != nil || string(data) != "[IMG_TYPE: mermaid]\nBROKEN" {
		t.Fatalf("submit.md = %q err=%v", data, err)
	}
	res, err := grep.Execute(`{"pattern":"BROKEN"}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text == "" || !contains(res.Text, "BROKEN") {
		t.Fatalf("grep result=%q", res.Text)
	}
}

func TestResolveMermaidSessionParams(t *testing.T) {
	if got := resolveMermaidSessionRounds(nil); got != mermaidSessionRoundsDefault {
		t.Fatalf("nil rounds = %d", got)
	}
	zero := 0
	if got := resolveMermaidSessionRounds(&zero); got != 0 {
		t.Fatalf("0 rounds = %d (0 = 关闭)", got)
	}
	if got := resolveMermaidSessionErrors(nil); got != mermaidSessionErrorsDefault {
		t.Fatalf("nil errors = %d", got)
	}
	neg := -1
	if got := resolveMermaidSessionErrors(&neg); got != mermaidSessionErrorsDefault {
		t.Fatalf("negative errors = %d", got)
	}
}

func TestMermaidFixSessionDisabled(t *testing.T) {
	// Rounds=0 → 会话直接不跑（runner 侧也不建配置）。
	got, ok := MermaidFixSession(&MermaidFixConfig{Rounds: 0}, "x", "y", newTestLogger(t), 0)
	if ok || got != "" {
		t.Fatalf("disabled session returned (%q, %v)", got, ok)
	}
}

func TestMermaidFixSessionInDirFakeModel(t *testing.T) {
	// 不发真实对话：httptest 返回 401（鉴权错误不可重试，见 session/client.go
	// 的 4xx 分类）→ Run 报错 → 未提交 → 失败路径，工作区 submit.md 保留原文。
	fakeMMDC(t)
	ws := t.TempDir()
	authFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"bad key"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(authFail.Close)
	cfg := &MermaidFixConfig{
		Rounds:     1,
		ErrorLimit: 3,
		Command:    "mmdc",
		Timeout:    time.Second,
		Primary: config.ModelConfig{
			Model:   "m1",
			BaseURL: authFail.URL,
			APIKey:  "k",
		},
	}
	prev := "[IMG_TYPE: mermaid]\n```mermaid\nBROKEN\n```"
	got, ok := mermaidFixSessionInDir(cfg, ws, prev, "parse error", newTestLogger(t), 0)
	if ok || got != "" {
		t.Fatalf("broken endpoint should fail, got (%q, %v)", got, ok)
	}
	// submit.md 必须保留出错原文（工作区现场可诊断）。
	data, err := os.ReadFile(filepath.Join(ws, mermaidFixSubmitFile))
	if err != nil || string(data) != prev {
		t.Fatalf("submit.md = %q err=%v", data, err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestResolveMermaidFixConfig(t *testing.T) {
	// rounds=0 → 关闭（返回 nil）。
	cfg := &config.Config{}
	zero := 0
	cfg.Tools.Mermaid.SessionRounds = &zero
	if got := resolveMermaidFixConfig(cfg, config.ModelConfig{}, "imgs"); got != nil {
		t.Fatalf("rounds=0 should disable, got %+v", got)
	}

	// 默认开启；备选条目不存在 → FallbackModel 清空（退回原模型）。
	cfg2 := &config.Config{}
	six := 6
	cfg2.Tools.Mermaid.SessionRounds = &six
	cfg2.Tools.Mermaid.SessionErrors = intPtr3(2)
	cfg2.Tools.Mermaid.FallbackModel = "no-such-entry"
	cfg2.Tools.Mermaid.Command = "mmdc"
	cfg2.Tools.Mermaid.Timeout = 15
	cfg2.Paths.FinallyDir = t.TempDir()
	primary := config.ModelConfig{Model: "primary-model"}
	got := resolveMermaidFixConfig(cfg2, primary, "imgs")
	if got == nil {
		t.Fatal("want non-nil fix config")
	}
	if got.Rounds != 6 || got.ErrorLimit != 2 || got.Timeout != 15*time.Second {
		t.Fatalf("fields = %+v", got)
	}
	if got.FallbackModel != "" {
		t.Fatalf("unresolvable fallback should be cleared, got %q", got.FallbackModel)
	}
	if got.Primary.Model != "primary-model" {
		t.Fatalf("primary not carried: %+v", got)
	}
	if !contains(got.WorkspaceRoot, "mermaid_fix") {
		t.Fatalf("workspace root = %q", got.WorkspaceRoot)
	}

	// 备选条目存在 → 完整 ModelConfig 带回。
	cfg2.Models = map[string]config.ModelConfig{
		"text":     {Model: "base"},
		"bigmodel": {Model: "big-model", BaseURL: "http://x", APIKey: "kk"},
	}
	cfg2.Tools.Mermaid.FallbackModel = "bigmodel"
	got2 := resolveMermaidFixConfig(cfg2, primary, "imgs")
	if got2.Fallback.Model != "big-model" || got2.Fallback.BaseURL != "http://x" {
		t.Fatalf("fallback not resolved: %+v", got2.Fallback)
	}
}

// intPtr3 与本文件其它 intPtr 助手同职责（避免与 repair 测试文件的 intPtr 冲突
// 就不复用名字了——同包内函数名唯一）。
func intPtr3(v int) *int { return &v }

// T50：会话本身失败（模型退化空响应、API 错误）或提醒后仍未提交时，同样升级
// 备选模型段——原先只有编译错误累计才升级，会话失败会让备选模型形同虚设。

// fixSSEToolCall / fixSSEText 组装升级修复测试用的流式应答。
func fixSSEToolCall(w http.ResponseWriter, id, name, args string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":%q,\"type\":\"function\",\"function\":{\"name\":%q,\"arguments\":%q}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":10,\"total_tokens\":110}}\n\n", id, name, args)
	io.WriteString(w, "data: [DONE]\n\n")
}

func fixSSEText(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":3,\"total_tokens\":103}}\n\n", text)
	io.WriteString(w, "data: [DONE]\n\n")
}

// degenerateServer 总是返回 finish=length、0 completion token 的空响应
// （现场 Qwen 网关在高压下对 mermaid-fix 会话的真实行为）。
func degenerateServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":\"length\"}],\"usage\":{\"prompt_tokens\":1447,\"completion_tokens\":0,\"total_tokens\":1447}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
}

func TestMermaidFixSessionFailureEscalates(t *testing.T) {
	st, ws := newFixState(t, 6, 3)
	srv := degenerateServer(t)
	defer srv.Close()
	st.cfg.Primary = config.ModelConfig{Model: "m1", BaseURL: srv.URL, APIKey: "k"}
	st.cfg.Fallback = config.ModelConfig{Model: "m2", BaseURL: srv.URL, APIKey: "k"}
	if err := os.WriteFile(filepath.Join(ws, mermaidFixSubmitFile), []byte("BROKEN doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st.runOneStage(st.cfg.Primary, ws, 0, "parse error") {
		t.Fatal("stage 0 should fail against the degenerate server")
	}
	if !st.escalated {
		t.Fatal("escalated=false, want true after a failed session (T50)")
	}
}

func TestMermaidFixSessionFailureNoFallbackStays(t *testing.T) {
	st, ws := newFixState(t, 6, 3)
	srv := degenerateServer(t)
	defer srv.Close()
	st.cfg.Primary = config.ModelConfig{Model: "m1", BaseURL: srv.URL, APIKey: "k"}
	st.cfg.Fallback = config.ModelConfig{} // 未配置 fallback_model
	if err := os.WriteFile(filepath.Join(ws, mermaidFixSubmitFile), []byte("BROKEN doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st.runOneStage(st.cfg.Primary, ws, 0, "parse error") {
		t.Fatal("stage 0 should fail against the degenerate server")
	}
	if st.escalated {
		t.Fatal("escalated=true without a fallback model — stage 1 would run a zero ModelConfig")
	}
	if !hasFallbackModel(st.cfg) {
		t.Log("hasFallbackModel correctly reports no fallback")
	} else {
		t.Fatal("hasFallbackModel=true for a zero ModelConfig")
	}
}

// 端到端：原模型段退化失败 → 备选模型段 write_file + submit 成功。
func TestMermaidFixFallbackStageSucceeds(t *testing.T) {
	fakeMMDC(t)
	bad := degenerateServer(t)
	defer bad.Close()
	var step int
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step++
		switch step {
		case 1:
			fixSSEToolCall(w, "c1", "write_file", "{\"content\":\"[IMG_TYPE: mermaid]\\n```mermaid\\ngraph TD\\nA-->B\\n```\"}")
		case 2:
			fixSSEToolCall(w, "c2", "submit", "{}")
		default:
			fixSSEText(w, "done")
		}
	}))
	defer good.Close()

	ws := t.TempDir()
	cfg := &MermaidFixConfig{
		Rounds:        6,
		ErrorLimit:    3,
		FallbackModel: "bigmodel",
		Command:       "mmdc",
		Timeout:       5 * time.Second,
		Primary:       config.ModelConfig{Model: "m1", BaseURL: bad.URL, APIKey: "k"},
		Fallback:      config.ModelConfig{Model: "m2", BaseURL: good.URL, APIKey: "k"},
	}
	final, ok := mermaidFixSessionInDir(cfg, ws, "BROKEN prev", "parse error", newTestLogger(t), 0)
	if !ok {
		t.Fatal("fallback stage should have fixed the document")
	}
	if !contains(final, "graph TD") {
		t.Fatalf("final = %q", final)
	}
	if _, err := os.Stat(filepath.Join(ws, "session-stage1.jsonl")); err != nil {
		t.Fatalf("stage-1 transcript missing: %v", err)
	}
}
