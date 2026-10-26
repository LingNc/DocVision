package latex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/retryq"
)

// TestAPIErrorRetryLimitDefaultsTo30：接口错误的重试上限默认 30（T243，以前 6 次
// 太少——现场一次上游抖动就能耗掉 25 分钟，然后整章被记成失败）。
func TestAPIErrorRetryLimitDefaultsTo30(t *testing.T) {
	cases := []struct{ in, want int }{{0, 30}, {-5, 30}, {1, 1}, {4, 4}}
	for _, c := range cases {
		cfg := config.LatexConfig{APIErrorRetries: c.in}
		if got := cfg.APIErrorRetryLimit(); got != c.want {
			t.Fatalf("api_error_retries=%d 应当算 %d 次，实际 %d", c.in, c.want, got)
		}
	}
	if config.DefaultAPIErrorRetries != 30 {
		t.Fatalf("默认值应当是 30，实际 %d", config.DefaultAPIErrorRetries)
	}
}

// TestPausedErrorSaysHowToResume：暂停的措辞必须说清"这不是失败"以及"怎么接着
// 跑"——人看到这条消息时唯一的正确动作是重启程序或点「继续跑」（T243）。
func TestPausedErrorSaysHowToResume(t *testing.T) {
	err := error(&pausedError{Base: "chapter_009", Retries: 30, Cause: "api error: [SESSION_API_ERROR]"})
	if !isPaused(err) {
		t.Fatal("pausedError 应当被认成暂停")
	}
	msg := err.Error()
	for _, want := range []string{"暂停", "不是", "重启", "继续跑", "30"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("暂停说明里应当有 %q，实际：%s", want, msg)
		}
	}
	// 上游错误被包一层时仍然认得出来（调用方会 fmt.Errorf 的地方不少）。
	if !isPaused(fmt.Errorf("会话失败: %w", err)) {
		t.Fatal("包一层之后应当仍然认得出来")
	}
	// 普通的接口错误、普通失败都不是暂停。
	if isPaused(errors.New("api error: [SESSION_API_ERROR]")) {
		t.Fatal("没经过重试预算的接口错误不是暂停")
	}
	if isPaused(pauseError(nil)) {
		t.Fatal("空清单不是暂停")
	}
	// 整轮收尾：说"暂停"、说怎么续跑，章节按名字排序（可复现的输出）。
	batch := pauseError([]string{"chapter_011: x", "chapter_004: y"})
	if !isPaused(batch) {
		t.Fatal("整轮收尾的暂停也该被认出来")
	}
	if got := batch.Error(); !strings.HasPrefix(got, "chapter_004: y; chapter_011: x") {
		t.Fatalf("章节应当按名字排序，实际：%s", got)
	}
}

// TestRescueAfterErrorPausesWhenBudgetExhausted：次数用满后 rescueAfterError 返回
// 暂停错误（不是原样的接口错误），并且**不再排队**——再排一次还是同一个错误，
// 正确的下一步是人来推（T243）。
func TestRescueAfterErrorPausesWhenBudgetExhausted(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(filepath.Join(dir, "run.log"), filepath.Join(dir, "error.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	tr := filepath.Join(dir, "work", "sessions", "convert_chapter_777.jsonl")
	if err := os.MkdirAll(filepath.Dir(tr), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, []byte("{\"t\":\"msg\",\"role\":\"user\",\"text\":\"hi\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{APIErrorRetries: 1}}, log: log}
	target := rescueTarget{ProjDir: dir, Base: "chapter_777", Role: "convert", TrPath: tr}
	cause := errors.New("api error: [SESSION_API_ERROR] bad gateway")

	first := r.rescueAfterError(nil, 0, "[convert]", "chapter_777", "submit",
		func() bool { return false }, target, cause)
	if isPaused(first) {
		t.Fatal("第一次接口错误应当先排队重试，不是直接暂停")
	}
	if files := retryq.Pending(dir); len(files) != 1 {
		t.Fatalf("第一次应当排一条续跑请求，实际 %d 条", len(files))
	}

	second := r.rescueAfterError(nil, 0, "[convert]", "chapter_777", "submit",
		func() bool { return false }, target, cause)
	if !isPaused(second) {
		t.Fatalf("次数用满后应当返回暂停错误，实际：%v", second)
	}
	if strings.Contains(second.Error(), "会话失败") {
		t.Fatalf("暂停不该被写成「会话失败」：%s", second.Error())
	}
	if files := retryq.Pending(dir); len(files) != 1 {
		t.Fatalf("暂停后不该再多排请求（还是同一个接口错误），实际 %d 条", len(files))
	}
	// 致命错误（余额不足）仍然照旧立刻失败，不走暂停这条路。
	fatal := r.rescueAfterError(nil, 0, "[convert]", "chapter_777", "submit",
		func() bool { return false }, target, errors.New("api error: [SESSION_INSUFFICIENT_BALANCE]"))
	if isPaused(fatal) {
		t.Fatal("余额不足是致命错误，不该被认成暂停")
	}
}

// TestUpstreamExhaustionPausesChapter：真的跑一章、连撞两次接口错误（上限 1），
// 这一章的状态要落到 paused——不是 error，不换尝试、不归档产物、也不留重试请求
// （T243）。这是"这次暂停，重启或网页继续跑"这条需求在磁盘上的样子。
func TestUpstreamExhaustionPausesChapter(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		chapterError:     502,
		chapterErrorBody: `{"error":{"message":"[SESSION_API_ERROR] bad gateway"}}`,
		apiErrorRetries:  1,
	})
	if run.err == nil {
		t.Fatal("第一次接口错误应当先把这一章排进重试队列（不是直接成功）")
	}
	tr := filepath.Join(run.proj, "work", "sessions", "convert_chapter_01.jsonl")
	if got := lastChapterActivity(t, tr); got != "queued" {
		t.Fatalf("排上重试时状态应当是 queued，实际 %q", got)
	}
	files := retryq.Pending(run.proj)
	if len(files) != 1 {
		t.Fatalf("应当有一条续跑请求，实际 %d 条", len(files))
	}
	// 把退避时间拨到过去：模拟"退避到点，派发端接着跑同一条会话"。
	duePending(t, files[0].Path)
	if n := run.runner.dispatchRetries(run.proj); n != 1 {
		t.Fatalf("到点的续跑请求应当被派发一次，实际 %d", n)
	}
	run.runner.retryWatcherWG.Wait()

	// 又撞同一个接口错误，次数用满 → 暂停。
	if got := lastChapterActivity(t, tr); got != "paused" {
		t.Fatalf("次数用满后状态应当是 paused，实际 %q", got)
	}
	if files := retryq.Pending(run.proj); len(files) != 0 {
		t.Fatalf("暂停后不该再留重试请求，实际 %d 条", len(files))
	}
	if _, err := os.Stat(filepath.Join(run.proj, "work", "attempts", "chapter_01")); err == nil {
		t.Fatal("暂停不该归档产物、不该开新尝试（work/attempts 下不该有这一章）")
	}
	if _, err := os.Stat(filepath.Join(run.proj, "work", "chapters", "chapter_01.tex")); err == nil {
		t.Fatal("接口错误没有提交过任何产物，不该凭空出现 .tex")
	}
}

// duePending 把队列里一条请求的 NotBefore 拨到过去（只给用例用）。
func duePending(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var req retryq.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	req.NotBefore = time.Now().Add(-time.Minute).Format(time.RFC3339)
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
