package latex

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCtx(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// T280：被打断的一轮风格修復会话必须真的能"续上"——第 1 轮重跑时接自己那条
// 转录（append-only，打断也已经在盘上），而不是每次都退回样式阶段的上下文。
func TestStyleRoundSeedResumesSameRound(t *testing.T) {
	proj := t.TempDir()
	writeCtx(t, filepath.Join(proj, "work", "style_session.jsonl"), `{"t":"msg","role":"user","text":"样式阶段"}`+"\n")
	writeCtx(t, filepath.Join(proj, "work", "sessions", "stylefix_style_r1.jsonl"),
		`{"t":"msg","role":"user","text":"第1轮修复"}`+"\n")

	msgs, seed := styleRoundSeed(proj, 1)
	if filepath.Base(seed) != "stylefix_style_r1.jsonl" {
		t.Fatalf("第 1 轮应当接自己那条转录，got %s", seed)
	}
	if len(msgs) != 1 || msgs[0].Content != "第1轮修复" {
		t.Fatalf("接错了上下文: %+v", msgs)
	}
}

// 新一轮接着上一轮的记忆：第 2 轮从 r1 的转录起（r2 还没有文件）。
func TestStyleRoundSeedChainsPreviousRound(t *testing.T) {
	proj := t.TempDir()
	writeCtx(t, filepath.Join(proj, "work", "style_session.jsonl"), `{"t":"msg","role":"user","text":"样式阶段"}`+"\n")
	writeCtx(t, filepath.Join(proj, "work", "sessions", "stylefix_style_r1.jsonl"),
		`{"t":"msg","role":"user","text":"第1轮修复"}`+"\n")

	_, seed := styleRoundSeed(proj, 2)
	if filepath.Base(seed) != "stylefix_style_r1.jsonl" {
		t.Fatalf("第 2 轮应当接着第 1 轮，got %s", seed)
	}
	writeCtx(t, filepath.Join(proj, "work", "sessions", "stylefix_style_r2.jsonl"),
		`{"t":"msg","role":"user","text":"第2轮修复"}`+"\n")
	msgs, seed := styleRoundSeed(proj, 2)
	if filepath.Base(seed) != "stylefix_style_r2.jsonl" || msgs[0].Content != "第2轮修复" {
		t.Fatalf("第 2 轮应当优先接自己那条，got %s / %+v", seed, msgs)
	}
}

// 第一次进循环：退回样式阶段的上下文；什么都没有时明确返回空（调用方报错，
// 而不是拿一份空上下文去跑一个必然跑歪的会话）。
func TestStyleRoundSeedFallsBackToStyleSession(t *testing.T) {
	proj := t.TempDir()
	if msgs, seed := styleRoundSeed(proj, 1); len(msgs) != 0 || seed != "" {
		t.Fatalf("没有任何上下文时应当返回空，got %q %d", seed, len(msgs))
	}
	writeCtx(t, filepath.Join(proj, "work", "style_session.jsonl"), `{"t":"msg","role":"user","text":"样式阶段"}`+"\n")
	msgs, seed := styleRoundSeed(proj, 1)
	if filepath.Base(seed) != "style_session.jsonl" || len(msgs) != 1 {
		t.Fatalf("应当退回样式阶段的上下文，got %s %d", seed, len(msgs))
	}
}
