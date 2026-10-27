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

// noRescue 关掉 P58 的复盘会话。本组测试要数"核对会话自己"的次数，而复盘会话
// 会拿同一台假服务器再打一次模型（预算还是跨测试共享的全局计数），次数会飘。
func noRescue(r *Runner) {
	off := false
	r.cfg.Latex.Rescue.Enabled = &off
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

// TestCheckerViewHoldsTwoFilesOneFolder: 核对会话拿到的只读视图里必须有它要对照
// 的两个文件（md / tex）与分片目录。**其余条目按章节真实布局铺开**（P55 之后再
// 补：`<章>/` 别名、`figures/`、`images/`）——旧断言要求"正好三项"，正是那条
// 过窄的视图让 tex 里的 `\input{parts/…}` 在视图里指向不存在的位置。
func TestCheckerViewHoldsTwoFilesOneFolder(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	view := ensureCheckerView(proj, base, chapPath, texPath, partsPath)
	if view == "" {
		t.Fatal("no view")
	}
	want := []string{base + ".md", base + ".tex", "parts", base, "t1.tex"}
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
			// T165/T217：没 submit 就是没结论，不再兜底当通过（旧行为让一个
			// 从未提交结论的 checker 把章节放进了书）。这种失败与"章节有问题"
			// 分开：文案点明"不是内容问题"，轮次预算也放到 4 次。
			name: "no verdict is not a pass",
			reply: func(w http.ResponseWriter, round int32) {
				jsonReply(w, "looks fine to me")
			},
			wantOK:     false,
			wantIssues: "核对始终没有给出结论（不是内容问题，共尝试 4 次）",
		},
		{
			// T242：交了 issues 却一条问题都没写，先在同一条会话里要一次清单
			// ——模型补上了就照常按清单打回。
			name: "issues without text asks for the list",
			reply: func(w http.ResponseWriter, round int32) {
				switch round {
				case 1:
					jsonToolReply(w, "submit", `{"report":{"status":"issues"}}`)
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
			// T242：要过还是空的就按"这次没结论"收尾——一句空清单不能作废
			// 整章重转（旧行为正是如此）。
			name: "issues without text is not a verdict",
			reply: func(w http.ResponseWriter, round int32) {
				if round == 1 {
					jsonToolReply(w, "submit", `{"report":{"status":"issues"}}`)
					return
				}
				jsonReply(w, "reported")
			},
			wantOK:     false,
			wantIssues: "核对始终没有给出结论（不是内容问题",
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
			ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 1, 1)
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
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0, 1)
	if !ok {
		t.Fatalf("want pass after one degenerate response, got issues: %q", issues)
	}
	if calls.Load() < 2 {
		t.Fatalf("requests = %d, want >= 2 (degenerate + recovery)", calls.Load())
	}
}

// 每一轮都退化（T217 起是 4 轮）：不通过，且原因是"厂商/网络错误"而不是
// "章节有问题"——上层据此不做"换会话重转换"（那只会让同一个坏模型再失败 N 次）。
func TestCheckerDegenerateInBothAttemptsFails(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		degenerateJSON(w)
	})
	noRescue(r)
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0, 1)
	if ok {
		t.Fatal("几轮都没有结论，不能判通过")
	}
	if !strings.Contains(issues, "核对始终没有给出结论") {
		t.Fatalf("issues = %q, want 核对始终没有给出结论（上层据此不把整章作废重来）", issues)
	}
}

func TestCheckerStrictModeAcceptsExplicitPass(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
	})
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, checkHints{}, 0,
		filepath.Join(proj, "nested", "sessions", "checker.jsonl"), filepath.Join(proj, "nested", "checker-view"), true, 1)
	if !ok || issues != "" {
		t.Fatalf("explicit pass rejected: ok=%v issues=%q", ok, issues)
	}
}

// P59：核对会话在转录里写锚点——它核对的是**哪一条转换转录**。手动重新核对
// 只让核对的号加一，核对的仍是同一份产物，所以锚点必须指回做出这份产物的那次
// 转换（尝试号也是它的，不是核对自己的）。
func TestCheckerWritesConvertAnchor(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
	})
	transcript := filepath.Join(proj, "work", "sessions", "checker_"+base+".try3.jsonl")
	view := filepath.Join(proj, "work", "views", "checker_"+base+"_try3")
	if ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, checkHints{}, 0, transcript, view, true, 2); !ok {
		t.Fatalf("explicit pass rejected: %q", issues)
	}
	data, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		`"kind":"checker_target"`,
		`"convert":"convert_` + base + `.try2"`,
		`"convert_attempt":2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("核对转录缺锚点 %s: %s", want, body)
		}
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
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, checkHints{}, 0, transcript, view, true, 1)
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
	ok, errText := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, checkHints{}, 0,
		filepath.Join(proj, "nested", "sessions", "checker.jsonl"), filepath.Join(proj, "nested", "checker-view"), true, 1)
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
	if ok, _ := first.checkChapterAt(proj, base, chapPath, texPath, partsPath, checkHints{}, 0, transcript, view, true, 1); ok {
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
	ok, errText := first.checkChapterAt(proj, base, chapPath, texPath, partsPath, checkHints{}, 0, transcript, view, true, 1)
	if !ok || errText != "" || calls.Load() != 1 {
		t.Fatalf("updated checker resume failed: ok=%v err=%q calls=%d", ok, errText, calls.Load())
	}
}

// T165/T217：核对一次结论都没拿到**不再**降级成通过——实测一条跑完的章节，它的
// checker 8 轮 25 次工具调用 38 分钟一次 submit 都没发，却被旧兜底判成"核对通过"
// 写进了书。没结论就是没结论；这种失败也不是内容问题，先把 4 次预算用满再上报。
func TestCheckerWithoutVerdictFailsAfterFourAttempts(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		degenerateJSON(w)
	})
	noRescue(r)
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0, 1)
	if ok {
		t.Fatal("a checker that never submits a verdict must NOT be treated as a pass")
	}
	if !strings.Contains(issues, "核对始终没有给出结论（不是内容问题，共尝试 4 次）") {
		t.Fatalf("issues = %q, want 核对始终没有给出结论（共尝试 4 次）", issues)
	}
	// 每轮最多 3 次请求（原始 + 重发 + nudge），4 轮 = 12。
	if n := calls.Load(); n > 12 {
		t.Fatalf("requests = %d, want <= 12", n)
	}
	if n := calls.Load(); n < 8 {
		t.Fatalf("requests = %d, want >= 8 (four full attempts)", n)
	}
}

// T217（a）：核对会话一次 submit 都没发（只回正文）时，旧实现 2 次就上报，调用方
// 把它当成"核对不过"作废整章从头重转。现在这种"没结论"的失败放宽到 4 次**全新
// 会话**（同一转录路径、同一只读视图）；4 次都没有结论才上报，并在文案里点明
// "不是内容问题、共尝试几次"。
func TestCheckerNoVerdictRetriesFourAttempts(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		jsonReply(w, "looks fine to me")
	})
	noRescue(r)
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0, 1)
	if ok {
		t.Fatal("没给结论不能判通过")
	}
	if want := "核对始终没有给出结论（不是内容问题，共尝试 4 次）"; !strings.HasPrefix(issues, want) {
		t.Fatalf("issues = %q, want 前缀 %q", issues, want)
	}
	// 每次重开核对会话都在同一份转录里写一条锚点：4 条 = 4 次会话。
	transcript := filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl")
	data, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), `"kind":"checker_target"`); n != 4 {
		t.Fatalf("核对会话 = %d 次, want 4（转录 %s）", n, transcript)
	}
	// 每次会话两条请求：首请求 + 提交提醒（复盘已关，所以只有这两条）。
	if n := calls.Load(); n != 8 {
		t.Fatalf("requests = %d, want 8（4 次会话 × 首请求 + 提醒）", n)
	}
}

// T217（b）：多出来的那些轮次是**可用**的——前两次只回正文，第 3 次会话提交 pass
// 就按通过收账，不能因为前两次没结论把一章本来正确的产物作废。
func TestCheckerNoVerdictExtraAttemptCanPass(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	var calls atomic.Int32
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		// 每次会话 = 首请求 + 提交提醒（复盘已关）：第 5 条请求就是第 3 次会话。
		if calls.Add(1) == 5 {
			jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
			return
		}
		jsonReply(w, "looks fine to me")
	})
	noRescue(r)
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, partsPath, 0, 1)
	if !ok || issues != "" {
		t.Fatalf("第 3 次会话的 pass 没被采纳: ok=%v issues=%q", ok, issues)
	}
	if n := calls.Load(); n != 5 {
		t.Fatalf("requests = %d, want 5（前两次各 2 条 + 第 3 次首请求）", n)
	}
	transcript := filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl")
	data, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), `"kind":"checker_target"`); n != 3 {
		t.Fatalf("核对会话 = %d 次, want 3", n)
	}
}

// TestCheckerSeesProductAndHints：T172 —— checker 以前只拿到 md 与 tex 两个文本，
// 看不见产物、也看不见原书页，只能把"md 这样写、tex 那样写"的差异当成问题猜。
// 现在只读视图里还挂着本次编译产物（<base>.pdf）与转换会话的 report / notes
// （后者是"说法"，要它自己核实），并给它 view_pdf 与原文检索工具。
func TestCheckerSeesProductAndHints(t *testing.T) {
	proj, base, chapPath, texPath, partsPath := checkerFixture(t)
	pdf := filepath.Join(proj, "work", "temp", "conv_"+base, base+".pdf")
	if err := os.MkdirAll(filepath.Dir(pdf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pdf, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(proj, "work", "reports", base+".md")
	if err := os.MkdirAll(filepath.Dir(report), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(report, []byte("# 报告\n覆盖率 0.93\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	view := filepath.Join(proj, "nested", "checker-view")

	var firstBody string
	r, _ := checkerRunner(t, func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if firstBody == "" {
			firstBody = string(body)
		}
		jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
	})
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath,
		checkHints{PDF: pdf, Report: report, Notes: "表 3.1 在原页 p12/p13 分两页，已按页排"}, 0, "", view, true, 1)
	if !ok {
		t.Fatalf("explicit pass rejected: %q", issues)
	}
	for _, name := range []string{base + ".pdf", "report.md", "notes.md"} {
		if !fileExists(filepath.Join(view, name)) {
			t.Errorf("checker 只读视图缺 %s", name)
		}
	}
	if b, err := os.ReadFile(filepath.Join(view, "notes.md")); err != nil || !strings.Contains(string(b), "p12") {
		t.Errorf("notes.md 内容没写进去: %q %v", string(b), err)
	}
	for _, want := range []string{"check:" + base + ".pdf", "report.md", "notes.md", `"name":"view_pdf"`} {
		if !strings.Contains(firstBody, want) {
			t.Errorf("首条消息 / 工具表里没有 %q", want)
		}
	}
}

// T183：只点名真实存在的"说法"文件——不存在的不提，免得模型去读一个没有的文件。
func TestCheckerClaimNoteNamesExistingOnly(t *testing.T) {
	view := t.TempDir()
	if got := checkerClaimNote(view); got != "" {
		t.Fatalf("两个都没有时不该有这句: %q", got)
	}
	if err := os.WriteFile(filepath.Join(view, "report.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := checkerClaimNote(view)
	if !strings.Contains(got, "report.md") || strings.Contains(got, "notes.md") {
		t.Fatalf("只有 report.md 时不该提 notes.md: %q", got)
	}
	if err := os.WriteFile(filepath.Join(view, "notes.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	both := checkerClaimNote(view)
	if !strings.Contains(both, "report.md") || !strings.Contains(both, "notes.md") {
		t.Fatalf("两个都在时都要点名: %q", both)
	}
}

// 核对视图必须按章节**自己的布局**铺开（两种提交约定并存）：否则一个完全正确
// 的 `\input{parts/children/x.tex}` / `\input{figures/motif_x.tex}` 在视图里都指向
// 不存在的位置，核对只能报"路径写错了""插图被漏掉了"——实测 49 条留档打回理由
// 里 18 条路径、10 条插图。
func TestCheckerViewMirrorsChapterLayout(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_009_child_03"
	chapDir := filepath.Join(proj, "chapters")
	submitDir := filepath.Join(proj, "work", "chapters", base)
	for _, d := range []string{chapDir, filepath.Join(submitDir, "parts", "children"), filepath.Join(submitDir, "figures"), filepath.Join(proj, "source", "figures")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tex := filepath.Join(proj, "work", "chapters", base+".tex")
	if err := os.WriteFile(tex, []byte("\\input{parts/children/c1.tex}\n\\input{figures/motif_aa.tex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(chapDir, base+".md")
	if err := os.WriteFile(md, []byte("# m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submitDir, "parts", "children", "c1.tex"), []byte("child\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submitDir, "figures", "motif_aa.tex"), []byte("fig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 项目级兜底：tex 引用项目 figures 里的图（非递归章节的常态）。
	if err := os.WriteFile(filepath.Join(proj, "source", "figures", "motif_bb.tex"), []byte("fig2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	view := ensureCheckerView(proj, base, md, tex, submitDir)
	if view == "" {
		t.Fatal("视图没建出来")
	}
	for _, rel := range []string{
		base + ".tex", base + ".md",
		"parts/children/c1.tex",         // 递归节点的真实布局
		"figures/motif_aa.tex",          // 章节目录里的插图
		"figures/motif_bb.tex",          // 项目级插图兜底
		base + "/parts/children/c1.tex", // 提交布局里的 `\input{<章>/…}`
	} {
		if _, err := os.Stat(filepath.Join(view, rel)); err != nil {
			t.Fatalf("视图里缺 %s: %v", rel, err)
		}
	}

	// 扁平约定：分片直接交在章节目录下，`parts/x.tex` 与 `<章>/x.tex` 都要能读。
	flat := filepath.Join(proj, "work", "chapters", "chapter_011")
	if err := os.MkdirAll(flat, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flat, "table1.tex"), []byte("t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tex2 := filepath.Join(proj, "work", "chapters", "chapter_011.tex")
	if err := os.WriteFile(tex2, []byte("\\input{parts/table1.tex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md2 := filepath.Join(chapDir, "chapter_011.md")
	if err := os.WriteFile(md2, []byte("# m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	view2 := ensureCheckerView(proj, "chapter_011", md2, tex2, flat)
	for _, rel := range []string{"parts/table1.tex", "chapter_011/table1.tex", "table1.tex"} {
		if _, err := os.Stat(filepath.Join(view2, rel)); err != nil {
			t.Fatalf("扁平约定视图里缺 %s: %v", rel, err)
		}
	}
}

// P59/P60 后续：递归子章节的核对只读视图。用户看到的现象是"子章节的核对读不到
// parts/"。这里按**真实目录**铺一遍：子章节自己的临时工作区（
// `work/temp/conv_<根>/work/_subdivision/<child>/node/work`，里面有它编译时读的
// `parts/`）、提交契约要求的分片文件夹（`work:<子章>/`），再走一遍真正的提交
// （placeChapterFiles）与真视图构建（节点内的 `checker-view`），逐条断言两种
// 提交约定下 `parts/` 都能解析。
func TestCheckerViewRecursiveChildHasPartsAliases(t *testing.T) {
	proj := t.TempDir()
	root, child := "chapter_03", "chapter_03_child_02"
	node := filepath.Join(proj, "work", "temp", "conv_"+root, "work", "_subdivision", "child_02", "node")
	work := filepath.Join(node, "work")
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(work, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 转换会话的工作区：它编译时 cwd = work，所以 `\input{parts/…}` 读的是
	// 工作区根下的 parts/。
	write(filepath.Join("parts", "x.tex"), "part x\n")
	write(filepath.Join("parts", "sub", "y.tex"), "part y\n")
	// 整合过的主 .tex：递归/整合节点交出来的布局就是 `\input{parts/…}`。
	write(child+".tex", "\\section{Child}\n\\input{parts/x.tex}\n\\input{parts/sub/y.tex}\n")
	// 提交契约要的分片文件夹是 `work:<子章>/`——模型把它要交的分片放这里。
	write(filepath.Join(child, "parts", "x.tex"), "part x\n")
	write(filepath.Join(child, "parts", "sub", "y.tex"), "part y\n")

	result := filepath.Join(node, "result")
	if _, err := placeChapterFiles(work, result, child); err != nil {
		t.Fatalf("提交两路径失败: %v", err)
	}
	// 子章节的 md 在 `_subdivision/<child>/input/chapter.md`（不在
	// `<proj>/chapters/`）——这正是"按 chapPath 往上两级猜项目根"会猜错的地方。
	inputDir := filepath.Join(node, "..", "input")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(inputDir, "chapter.md")
	if err := os.WriteFile(md, []byte("# child\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 项目级插图：子章节的 .tex 会 `\input{figures/motif_…}`（工作区与成书树都
	// 能解析），核对视图同样要挂上，否则又被报成"插图被漏掉"。
	if err := os.MkdirAll(filepath.Join(proj, "source", "figures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "source", "figures", "motif_bb.tex"), []byte("fig\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	view := ensureCheckerViewAt(proj, filepath.Join(node, "checker-view"), child, md,
		filepath.Join(result, child+".tex"), filepath.Join(result, child))
	if view == "" {
		t.Fatal("视图没建出来")
	}
	for _, rel := range []string{
		child + ".md", child + ".tex",
		"parts/x.tex", "parts/sub/y.tex", // `\input{parts/…}`（T186 的 PARTS_DIR）
		child + "/parts/x.tex", // 提交布局里的 `\input{<章>/…}`
		"figures/motif_bb.tex", // 项目级插图（以前子章节挂不上）
	} {
		if _, err := os.Stat(filepath.Join(view, rel)); err != nil {
			t.Errorf("子章节核对视图里缺 %s: %v", rel, err)
		}
	}
}

// 反面对照（这条也是"核对读不到 parts/"的真实来源之一）：分片只留在**工作区**
// 根下的 `parts/`、没有进提交的分片文件夹时，只读视图照实映出提交内容——`parts/`
// 在、但里面是空的。核对判的是提交产物，工作区里的 `parts/` 不会被偷渡进视图
// （模型本地编译能过，是因为编译的 cwd 就是工作区根）。这才是需要修转换侧的地方，
// 不是视图把文件藏起来了。
func TestCheckerViewDoesNotLeakWorkTreeParts(t *testing.T) {
	proj := t.TempDir()
	root, child := "chapter_03", "chapter_03_child_03"
	node := filepath.Join(proj, "work", "temp", "conv_"+root, "work", "_subdivision", "child_03", "node")
	work := filepath.Join(node, "work")
	if err := os.MkdirAll(filepath.Join(work, "parts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "parts", "z.tex"), []byte("z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, child+".tex"), []byte("\\input{parts/z.tex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 提交的分片文件夹存在但是空的（"create it even when empty"）。
	if err := os.MkdirAll(filepath.Join(work, child), 0o755); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(node, "result")
	if _, err := placeChapterFiles(work, result, child); err != nil {
		t.Fatalf("提交两路径失败: %v", err)
	}
	view := ensureCheckerViewAt(proj, filepath.Join(node, "checker-view"), child, "",
		filepath.Join(result, child+".tex"), filepath.Join(result, child))
	if view == "" {
		t.Fatal("视图没建出来")
	}
	if _, err := os.Stat(filepath.Join(view, "parts")); err != nil {
		t.Fatalf("视图必须保留 parts/ 别名（即使提交的是空文件夹）: %v", err)
	}
	if _, err := os.Stat(filepath.Join(view, "parts", "z.tex")); err == nil {
		t.Fatal("工作区里的 parts/ 不该被偷渡进只读视图：核对只能看到提交产物")
	}
	if _, err := os.Stat(filepath.Join(work, "parts", "z.tex")); err != nil {
		t.Fatalf("工作区文件不该被动过: %v", err)
	}
}
