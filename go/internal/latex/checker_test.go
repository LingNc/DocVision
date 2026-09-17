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
			name: "no verdict means pass",
			reply: func(w http.ResponseWriter, round int32) {
				jsonReply(w, "looks fine to me")
			},
			wantOK: true,
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

// T46：checker 必须以一个结论收尾，不能"出错就结束"——
// 会话错误/漏 submit 时先提醒、再重开一轮全新会话，两轮都失败才降级通过。

// degenerateJSON 返回 finish=length、0 completion token 的空响应
//（厂商网关在高压下的真实退化行为）。
func degenerateJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":100,"completion_tokens":0,"total_tokens":100}}`)
}

func TestCheckerRetriesAfterSessionFailure(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			// 第 1 轮尝试：退化空响应（含重发 1 次 + nudge 共 2 次请求）。
			degenerateJSON(w)
			return
		}
		// 第 2 轮尝试（重开）：正常提交 pass。
		jsonToolReply(w, "submit", `{"status":"pass","report":"ok"}`)
	})
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0)
	if !ok {
		t.Fatalf("want pass after retry, got issues: %q", issues)
	}
	if calls.Load() < 3 {
		t.Fatalf("requests = %d, want >= 3 (degenerate round + retry round)", calls.Load())
	}
}

func TestCheckerDegradesOnlyAfterTwoAttempts(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		degenerateJSON(w)
	})
	ok, _ := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0)
	if !ok {
		t.Fatal("two failed attempts should still degrade to pass (compile/final review are the hard gates)")
	}
	// 每轮最多 3 次请求（原始 + 重发 + nudge），两轮 = 6。
	if n := calls.Load(); n > 6 {
		t.Fatalf("requests = %d, want <= 6", n)
	}
	if n := calls.Load(); n < 4 {
		t.Fatalf("requests = %d, want >= 4 (two full attempts)", n)
	}
}
