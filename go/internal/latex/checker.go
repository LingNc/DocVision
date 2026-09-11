package latex

import (
	"os"
	"path/filepath"
	"strings"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// checkChapter reviews ONE converted chapter with its own minimal,
// read-only session: the small text model gets exactly two files (the
// chapter markdown and the submitted .tex) and one folder (that
// chapter's \input parts), plus read_file/grep to look at them, and
// hands its verdict in with submit. It cannot write anything, and it
// does not touch the book.
//
// Hard problems are returned to the caller, which feeds them back to the
// SAME conversion session (its context is still live, so it is the
// cheapest and most accurate fixer). Transport/validation problems
// degrade to (true, "") — compile and the final assembly review remain
// the hard gates.
func (r *Runner) checkChapter(proj, base, chapPath, texPath, partsPath string, tid int) (bool, string) {
	view := ensureCheckerView(proj, base, chapPath, texPath, partsPath)
	if view == "" {
		r.log.LogWarning(tid, "[checker]", base, "无法准备只读视图，跳过核对")
		return true, ""
	}
	checkerName := r.cfg.Latex.CheckerModel
	if checkerName == "" {
		checkerName = r.cfg.Latex.ConvertModel
	}
	client := r.clientFor(checkerName)
	modelCfg := r.models[checkerName]
	tuning := r.cfg.LatexSession("checker")

	mounts := []Mount{{Name: "check", Dir: view}}
	submit := &SubmitDoneTool{
		Label:         "the review of chapter " + base,
		RequireReport: true, // 结论就用这个交（不落盘：写不写 .checker 由 runner 决定）
	}
	tools := []session.Tool{
		&ReadFileTool{Mounts: mounts},
		&GrepTool{Mounts: mounts},
		submit,
	}
	sess := session.NewSession(client, modelCfg, tuning,
		renderPrompt(prompts.Must(prompts.CheckerSystem), tuning, r.outputLang()),
		tools, r.log, tid, "checker:"+base)

	trPath := filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl")
	if tr, err := session.NewTranscript(trPath); err == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	// checker 会话也进实时块：用户要看得到"谁进了 checker、跑到哪了"，
	// 而且 checker 是并发跑的，必须有自己的行而不是挤在转换行里。
	chkHook, chkClose := r.livePhaseRow("checker:"+base, "checker:"+base)
	sess.SetProgressHook(chkHook)
	defer chkClose()
	userText := prompts.Render(prompts.CheckerUser, map[string]string{
		"CHAPTER_MD":  "check:" + base + ".md",
		"CHAPTER_TEX": "check:" + base + ".tex",
		"PARTS_DIR":   "check:parts/",
	})
	if r.cfg.Latex.RemoveWatermark {
		userText += "\n\nWatermark note: if watermark-like content (institution marks, faint background text) is absent from the .tex, that is CORRECT — watermark removal is enabled. Do not report it as missing content."
	}
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		r.log.LogWarning(tid, "[checker]", base, "会话失败，视为通过:", err)
		r.keepSessionFile(trPath)
		return true, ""
	}
	r.keepSessionFile(trPath)
	if !submit.Submitted {
		r.log.LogWarning(tid, "[checker]", base, "未提交结论，视为通过")
		return true, ""
	}
	if submit.Status == "issues" {
		issues := strings.TrimSpace(submit.Issues)
		if issues == "" {
			issues = "the checker reported problems but gave no description"
		}
		return false, issues
	}
	return true, ""
}

// ensureCheckerView materialises the checker's read-only view: exactly
// the two files it must compare and the folder with the chapter's
// \input parts. It is rebuilt on every call so it can never point at a
// stale chapter.
func ensureCheckerView(proj, base, chapPath, texPath, partsPath string) string {
	if proj == "" || base == "" {
		return ""
	}
	dir := filepath.Join(proj, "work", "views", "checker_"+base)
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	link := func(src, name string) {
		if !fileExists(src) {
			return
		}
		if err := linkAbs(src, filepath.Join(dir, name)); err != nil {
			_ = copyFile(src, filepath.Join(dir, name))
		}
	}
	link(chapPath, base+".md")
	link(texPath, base+".tex")
	if st, err := os.Stat(partsPath); err == nil && st.IsDir() {
		if err := linkAbs(partsPath, filepath.Join(dir, "parts")); err != nil {
			_ = copyDir(partsPath, filepath.Join(dir, "parts"))
		}
	} else if err := os.MkdirAll(filepath.Join(dir, "parts"), 0o755); err != nil {
		return ""
	}
	return dir
}
