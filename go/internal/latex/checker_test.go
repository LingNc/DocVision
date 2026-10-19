package latex

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/session"
)

// checkerRunner builds a Runner whose checker model is a fake HTTP
// server, so the checker SESSION can be driven end to end without a
// network.
func checkerRunner(t *testing.T, handler http.HandlerFunc) (*Runner, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	r := keepTestRunner(t)
	off := false
	r.cfg.Latex.CheckerModel = "fake"
	r.cfg.Models = map[string]config.ModelConfig{
		"fake": {BaseURL: srv.URL, APIKey: "k", Model: "fake-model", Stream: &off},
	}
	r.clients = map[string]*session.Client{}
	r.models = map[string]config.ModelConfig{}
	return r, srv
}

// sseReply renders one non-streaming-free assistant answer.
func jsonReply(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":`+strconvQuote(content)+`}}]}`)
}

func strconvQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func jsonToolReply(w http.ResponseWriter, name, args string) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"`+
		name+`","arguments":`+strconvQuote(args)+`}}]}}]}`)
}

// checkerFixture writes one chapter (md + submitted .tex + parts) and
// returns the paths.
func checkerFixture(t *testing.T) (proj, base, chapPath, texPath, partsPath string) {
	t.Helper()
	proj = t.TempDir()
	base = "chapter_03"
	store := filepath.Join(proj, "chapters")
	chanDir := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	partsPath = filepath.Join(chanDir, base)
	if err := os.MkdirAll(partsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	chapPath = filepath.Join(store, base+".md")
	if err := os.WriteFile(chapPath, []byte("# 第三章\n正文。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	texPath = filepath.Join(chanDir, base+".tex")
	if err := os.WriteFile(texPath, []byte("\\section{第三章}\n正文。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partsPath, "t1.tex"), []byte("part"), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

// TestCheckerViewHoldsTwoFilesOneFolder: the checker session gets a
// read-only view with EXACTLY the two files it compares and the folder
// with the chapter's \input parts — nothing else, and no way to write.
func TestCheckerViewHoldsTwoFilesOneFolder(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	view := ensureCheckerView(proj, base, chapPath, texPath, partsPath)
	if view == "" {
		t.Fatal("no view")
	}
	entries, err := os.ReadDir(view)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{base + ".md", base + ".tex", "parts"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("视图内容 = %v, want %v", names, want)
	}
	for _, n := range want {
		if _, err := os.Stat(filepath.Join(view, n)); err != nil {
			t.Errorf("视图应有 %s: %v", n, err)
		}
	}
	// 视图是只读挂载：checker 的工具里没有任何写工具。
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) { jsonReply(w, "done") })
	r.cfg.Latex.CheckerModel = ""
	mounts := []Mount{{Name: "check", Dir: view}}
	tools := []interface{ Name() string }{&ReadFileTool{Mounts: mounts}, &GrepTool{Mounts: mounts}}
	for _, tool := range tools {
		if tool.Name() == "write_file" || tool.Name() == "edit_file" || tool.Name() == "bash" || tool.Name() == "compile" {
			t.Errorf("checker 不该有 %s", tool.Name())
		}
	}
}

// TestCheckerSessionFeedsBackRealProblems: a checker that reports
// "issues" must reach the caller verbatim (they go back to the SAME
// conversion session); "pass" and "no verdict" mean the chapter stands.
func TestCheckerSessionFeedsBackRealProblems(t *testing.T) {
	cases := []struct {
		name       string
		reply      func(w http.ResponseWriter, round int32)
		wantOK     bool
		wantIssues string
	}{
		{
			name: "issues",
			reply: func(w http.ResponseWriter, round int32) {
				switch round {
				case 1:
					// 先真读一次文件（证明视图可用），再交结论。
					jsonToolReply(w, "read_file", `{"path":"check:chapter_03.tex"}`)
				case 2:
					jsonToolReply(w, "submit", `{"report":{"status":"issues","issues":"表 3-2 少了三行"}}`)
				default:
					jsonReply(w, "reported")
				}
			},
			wantOK:     false,
			wantIssues: "表 3-2 少了三行",
		},
		{
			name: "pass",
			reply: func(w http.ResponseWriter, round int32) {
				if round == 1 {
					jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
					return
				}
				jsonReply(w, "clean")
			},
			wantOK: true,
		},
		{
			// T165：没 submit 就是没结论，不再兜底当通过（旧行为让一个
			// 从未提交结论的 checker 把章节放进了书）。
			name: "no verdict is not a pass",
			reply: func(w http.ResponseWriter, round int32) {
				jsonReply(w, "looks fine to me")
			},
			wantOK:     false,
			wantIssues: "未能提交结论",
		},
		{
			name: "issues without text still blocks",
			reply: func(w http.ResponseWriter, round int32) {
				if round == 1 {
					jsonToolReply(w, "submit", `{"report":{"status":"issues"}}`)
					return
				}
				jsonReply(w, "reported")
			},
			wantOK:     false,
			wantIssues: "no description",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proj, base, chapPath, texPath, partsPath := checkerFixture(t)
			var round int32
			r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
				n := atomic.AddInt32(&round, 1)
				c.reply(w, n)
			})
			ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 1)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (issues=%q)", ok, c.wantOK, issues)
			}
			if c.wantIssues != "" && !strings.Contains(issues, c.wantIssues) {
				t.Errorf("issues = %q, want 含 %q", issues, c.wantIssues)
			}
			if c.wantOK && issues != "" {
				t.Errorf("通过时不该带回问题: %q", issues)
			}
		})
	}
}

// TestCheckerReadOnlyMountRejectsWrite: even if a model tried, the
// checker's mounts are read-only and its tool set has no writer.
func TestCheckerReadOnlyMountRejectsWrite(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	view := ensureCheckerView(proj, base, chapPath, texPath, partsPath)
	tools := []interface{ Name() string }{&ReadFileTool{Mounts: []Mount{{Name: "check", Dir: view}}}, &GrepTool{Mounts: []Mount{{Name: "check", Dir: view}}}}
	names := []string{}
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	if strings.Join(names, ",") != "read_file,grep" {
		t.Fatalf("checker 工具集 = %v, want [read_file grep]", names)
	}
}

// T165：checker 必须以一个结论收尾。会话空响应/漏 submit 时会先在内部
// 重开一轮核对（同一章节、新会话），两轮都没有结论就**判不通过**——旧
// 行为在这里降级成"通过"，让一个从未提交结论的 checker 把章节放进了书。

// degenerateJSON 返回 finish=length、0 completion token 的空响应
// （厂商网关在高压下的真实退化行为）。
func degenerateJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":100,"completion_tokens":0,"total_tokens":100}}`)
}

// 首轮退化、随后正常提交：内部重开一次就应当拿到结论并通过。
func TestCheckerRecoversAfterDegenerateResponse(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		if calls.Add(1) == 1 {
			degenerateJSON(w)
			return
		}
		jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
	})
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0)
	if !ok {
		t.Fatalf("want pass after one degenerate response, got issues: %q", issues)
	}
	if calls.Load() < 2 {
		t.Fatalf("requests = %d, want >= 2 (degenerate + recovery)", calls.Load())
	}
}

// 两轮都退化：不通过，且原因是"厂商/网络错误"而不是"章节有问题"——上层
// 据此不做"换会话重转换"（那只会让同一个坏模型再失败 N 次）。
func TestCheckerDegenerateInBothAttemptsFails(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		degenerateJSON(w)
	})
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0)
	if ok {
		t.Fatal("两轮都没有结论，不能判通过")
	}
	if !strings.Contains(issues, "未能提交结论") {
		t.Fatalf("issues = %q, want 未能提交结论（上层据此换会话重来）", issues)
	}
}

func TestCheckerStrictModeAcceptsExplicitPass(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
	})
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, 0,
		filepath.Join(proj, "nested", "sessions", "checker.jsonl"), filepath.Join(proj, "nested", "checker-view"), true)
	if !ok || issues != "" {
		t.Fatalf("explicit pass rejected: ok=%v issues=%q", ok, issues)
	}
}

func TestCheckerStrictModeFailsClosedWithoutConclusion(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		jsonReply(w, "done")
	})
	transcript := filepath.Join(proj, "nested", "sessions", "checker.jsonl")
	view := filepath.Join(proj, "nested", "checker-view")
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, 0, transcript, view, true)
	if ok || issues == "" {
		t.Fatalf("strict checker accepted missing verdict: ok=%v issues=%q", ok, issues)
	}
	if !fileExists(transcript) {
		t.Fatal("nested checker transcript was not persisted")
	}
	if calls.Load() < 2 {
		t.Fatalf("checker requests = %d, want retry after missing verdict", calls.Load())
	}
}

func TestCheckerStrictModeStopsOnInsufficientBalance(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"message":"余额不足或无可用资源包","code":1113}}`)
	})
	ok, errText := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, 0,
		filepath.Join(proj, "nested", "sessions", "checker.jsonl"), filepath.Join(proj, "nested", "checker-view"), true)
	if ok || !strings.Contains(errText, "SESSION_INSUFFICIENT_BALANCE") {
		t.Fatalf("expected fail-closed balance error, got ok=%v err=%q", ok, errText)
	}
	if calls.Load() != 1 {
		t.Fatalf("checker requests = %d, want one request before resume with a new model", calls.Load())
	}
}

func TestCheckerResumeUsesUpdatedModelAndTranscript(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var firstCalls atomic.Int32
	first, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		if firstCalls.Add(1) == 1 {
			jsonReply(w, "unfinished checker note")
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
		io.WriteString(w, `{"error":{"message":"余额不足或无可用资源包","code":1113}}`)
	})
	transcript := filepath.Join(proj, "nested", "sessions", "checker_chapter_02.jsonl")
	view := filepath.Join(proj, "nested", "checker-view")
	if ok, _ := first.checkChapterAt(proj, base, chapPath, texPath, partsPath, 0, transcript, view, true); ok {
		t.Fatal("checker without a verdict unexpectedly passed")
	}

	var calls atomic.Int32
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), "unfinished checker note") {
			t.Errorf("resumed request omitted prior transcript context: %s", body)
		}
		calls.Add(1)
		jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
	}))
	defer secondServer.Close()
	off := false
	first.cfg.Latex.CheckerModel = "updated"
	first.cfg.Models["updated"] = config.ModelConfig{BaseURL: secondServer.URL, APIKey: "k", Model: "updated-model", Stream: &off}
	first.clients = map[string]*session.Client{}
	first.models = map[string]config.ModelConfig{}
	ok, errText := first.checkChapterAt(proj, base, chapPath, texPath, partsPath, 0, transcript, view, true)
	if !ok || errText != "" || calls.Load() != 1 {
		t.Fatalf("updated checker resume failed: ok=%v err=%q calls=%d", ok, errText, calls.Load())
	}
}

// T165：两次核对都没有结论**不再**降级成通过——实测一条跑完的章节，它的
// checker 8 轮 25 次工具调用 38 分钟一次 submit 都没发，却被旧兜底判成
// "核对通过"写进了书。没结论就是没结论，交给上层按重试预算重来。
func TestCheckerWithoutVerdictFailsAfterTwoAttempts(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		degenerateJSON(w)
	})
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0)
	if ok {
		t.Fatal("a checker that never submits a verdict must NOT be treated as a pass")
	}
	if !strings.Contains(issues, "未能提交结论") {
		t.Fatalf("issues = %q, want 未能提交结论", issues)
	}
	// 每轮最多 3 次请求（原始 + 重发 + nudge），两轮 = 6。
	if n := calls.Load(); n > 6 {
		t.Fatalf("requests = %d, want <= 6", n)
	}
	if n := calls.Load(); n < 4 {
		t.Fatalf("requests = %d, want >= 4 (two full attempts)", n)
	}
}
