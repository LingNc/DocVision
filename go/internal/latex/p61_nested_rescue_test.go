package latex

/*
 * P61/D：嵌套子章节的转换也要有复盘。
 *
 * 用户的现场是一条嵌套子章节的转换转录：尾巴上反复出现"You have NOT submitted
 * yet"、越堆越乱，却从来没有被复盘过。排查结果：**普通**嵌套子章节（只转换、
 * 不再细分）走的是与顶层同一条 `remindOrRescue`，复盘本来是接上的；真正漏的是
 * **自己又细分过**的节点——它的整合轮没提交时那条分支直接判失败，既没有提醒也
 * 没有复盘，外层只能看到"子章节转换或 checker 失败"。
 *
 * 这个用例跑真实的三层细分树：chapter_01 → child_01 → child_01_child_0N，
 * 孙章节正常转换+核对通过，child_01 的整合轮不给 submit。期望：恰好一次复盘、
 * 结论 truncate 之后正好排一条"从这一条记录继续"的请求（角色 continue、章节
 * 就是那个嵌套子章节），并且这条路里不再往同一个坏会话里发提醒。
 */

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/session"
)

// submit 工具的描述里点名"这一章该交哪个 .tex 名"（`must be <章>.tex`），
// 假模型靠它认出当前会话是哪一层：第三层（孙章节）直接交产物，第二层请求细分。
var grandchildTarget = regexp.MustCompile(`(chapter_01_child_\d+_child_\d+)\.tex`)
var childTarget = regexp.MustCompile(`(chapter_01_child_\d+)\.tex`)

type nestedRescueStats struct {
	mu        sync.Mutex
	rescues   int
	reminders int
}

func (s *nestedRescueStats) add(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

func TestNestedChildIntegrationNoSubmitIsRescued(t *testing.T) {
	resetRescueBudget()
	proj := t.TempDir()
	workDir := filepath.Join(proj, "work")
	chapters := filepath.Join(proj, "chapters")
	style := filepath.Join(proj, "style")
	for _, dir := range []string{workDir, chapters, style} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	chapterPath := filepath.Join(chapters, "chapter_01.md")
	// 8 行：两个子章节各 4 行，孙章节各 2 行——submit 拒收"不足两行"的章节，
	// 叶子必须至少有内容可交。
	if err := os.WriteFile(chapterPath, []byte(
		"# First\nfirst a\nfirst b\nfirst c\n# Second\nsecond a\nsecond b\nsecond c"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"book.cls":    "\\ProvidesClass{book}\n",
		"manual.md":   "# Manual\n",
		"example.tex": "\\section{Example}\n",
	} {
		if err := os.WriteFile(filepath.Join(style, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	stats := &nestedRescueStats{}
	requests := 0
	// 跑飞了（例如某一轮永远被拒）就直接把请求打死：用例会以明确原因失败，
	// 不挂到 go test 的超时。
	guard := func(w http.ResponseWriter, lastUser string) bool {
		requests++
		if requests > 120 {
			t.Errorf("请求数超过 120，用例跑飞了（最后一条 user：%.120s）", lastUser)
			http.Error(w, `{"error":{"message":"runaway test"}}`, http.StatusBadRequest)
			return false
		}
		return true
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var apiReq subdivisionAPIRequest
		if err := json.Unmarshal(body, &apiReq); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		lastUser := ""
		for _, m := range apiReq.Messages {
			if m.Role == "user" {
				lastUser = m.Content
			}
		}
		if !guard(w, lastUser) {
			return
		}
		toolset := map[string]bool{}
		submitDesc := ""
		for _, tool := range apiReq.Tools {
			toolset[tool.Function.Name] = true
			if tool.Function.Name == "submit" {
				submitDesc = tool.Function.Description
			}
		}
		switch {
		// 复盘会话：只读转录 + 交结论。
		case toolset["read_session"]:
			stats.add(func() { stats.rescues++ })
			subdivisionReply(w, subdivisionCall("rescue-submit", "submit", map[string]any{
				"verdict": "truncate", "from": 1, "reason": "整合轮退化", "hint": "",
			}))
		// 核对会话：只读（read_file/grep，没有 write_file）。
		case toolset["read_file"] && toolset["grep"] && !toolset["write_file"]:
			subdivisionReply(w, subdivisionCall("check-submit", "submit", map[string]any{
				"report": map[string]any{"status": "pass"},
			}))
		// 整合轮：child_01 **不给 submit**（这正是漏掉提醒/复盘的那一轮）；
		// child_02 正常交，证明这条路对其余子章节没有副作用。
		case strings.Contains(lastUser, "All child chapters have passed"):
			stats.add(func() {
				if strings.Contains(lastUser, "You have NOT submitted yet") {
					stats.reminders++
				}
			})
			if strings.Contains(lastUser, "work:chapter_01_child_01.tex") {
				p60Text(w, "The children look fine, I think we are done.")
				return
			}
			subdivisionReply(w,
				subdivisionWrite("work:chapter_01_child_02.tex", "\\section{Child two integrated}\n"),
				subdivisionWrite("work:chapter_01_child_02/part.tex", "integrated part\n"),
				subdivisionCall("child-two-submit", "submit", map[string]any{
					"path": "work:chapter_01_child_02.tex", "dir": "work:chapter_01_child_02/",
					"report": map[string]any{"status": "pass"},
				}),
			)
		// 嵌套子章节/孙章节的转换轮：靠 submit 工具描述里的章节名区分。
		case strings.Contains(lastUser, "input:chapter.md"):
			// 第三层（孙章节）就是叶子：写产物 + 真调 submit。
			if m := grandchildTarget.FindStringSubmatch(submitDesc); m != nil {
				base := m[1]
				subdivisionReply(w,
					subdivisionWrite("work:"+base+".tex", "\\section{Grandchild}\n"),
					subdivisionWrite("work:"+base+"/part.tex", "grandchild part\n"),
					subdivisionCall("gc-submit-"+base, "submit", map[string]any{
						"path": "work:" + base + ".tex", "dir": "work:" + base + "/",
						"report": map[string]any{"status": "pass"},
					}),
				)
				return
			}
			// 第二层（child_0N）：自己也细分成两个孙章节。
			base := "chapter_01_child_01"
			if m := childTarget.FindStringSubmatch(submitDesc); m != nil {
				base = m[1]
			}
			subdivisionReply(w,
				subdivisionWrite("work:"+base+".tex", "\\section{Child draft}\n"),
				subdivisionCall("child-split", "request_chapter_split", map[string]any{"children": []any{
					map[string]any{"title": "Grand first", "start_line": 1, "end_line": 2},
					map[string]any{"title": "Grand second", "start_line": 3, "end_line": 4},
				}}),
			)
		default:
			// 顶层章节：细分成两个子章节。
			subdivisionReply(w,
				subdivisionWrite("work:chapter_01.tex", "\\section{Parent draft}\n"),
				subdivisionCall("parent-split", "request_chapter_split", map[string]any{"children": []any{
					map[string]any{"title": "First", "start_line": 1, "end_line": 4},
					map[string]any{"title": "Second", "start_line": 5, "end_line": 8},
				}}),
			)
		}
	}))
	defer server.Close()

	r := keepTestRunner(t)
	r.cfg.Latex.Concurrency = 2
	r.cfg.Latex.ConvertModel = "fake"
	r.cfg.Latex.CheckerModel = "fake"
	// 这条用例只钉复盘接线：修复会话（P60）关掉，假模型的分派表才只需要认
	// 转换/核对/复盘三种会话。
	off := false
	r.cfg.Latex.Fix.Enabled = &off
	r.cfg.Models = map[string]config.ModelConfig{"fake": {BaseURL: server.URL, APIKey: "test", Model: "fake", Stream: &off}}
	r.clients = map[string]*session.Client{}
	r.models = map[string]config.ModelConfig{}
	r.projDir = proj

	err := r.convertOneChapter(proj, "book", filepath.Join(style, "manual.md"), chapterPath, workDir, 1, 1, false)
	if err == nil {
		t.Fatal("嵌套子章节的整合轮没提交，整章应当按失败收尾")
	}
	if !strings.Contains(err.Error(), "child_01") {
		t.Errorf("失败原因应点名那个嵌套子章节：%v", err)
	}
	if stats.rescues != 1 {
		t.Fatalf("嵌套子章节集成轮没提交应当恰好触发一次复盘，实际 %d 次", stats.rescues)
	}
	if stats.reminders != 0 {
		t.Errorf("复盘判 truncate 后不该再往同一个坏会话发提醒，实际 %d 次", stats.reminders)
	}
	queued := rescueQueuedRequests(t, proj)
	if len(queued) != 1 {
		t.Fatalf("应当恰好排一条重试请求，实际 %d 条: %v", len(queued), queued)
	}
	if got := queued[0]["chapter"]; got != "chapter_01_child_01" {
		t.Errorf("重试请求的章节 = %v，期望 chapter_01_child_01（嵌套子章节自己）", got)
	}
	if got := queued[0]["role"]; got != "continue" {
		t.Errorf("重试请求的角色 = %v，期望 continue（从这条记录继续）", got)
	}
	path := StringOf(queued[0]["path"])
	if !strings.HasSuffix(path, "convert_chapter_01_child_01.jsonl") {
		t.Errorf("重试请求应当指向嵌套子章节自己的转录，实际 %q", path)
	}
}

// StringOf 是测试里读 JSON 字段的小工具（字段可能是 nil）。
func StringOf(v any) string {
	s, _ := v.(string)
	return s
}
