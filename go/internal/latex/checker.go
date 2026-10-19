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
// SAME conversion session. Ordinary chapters retain the historical transport
// fallback; recursive nodes use strict mode and fail closed on missing verdicts.
func (r *Runner) checkChapter(proj, base, chapPath, texPath, partsPath string, tid int) (bool, string) {
	return r.checkChapterAt(proj, base, chapPath, texPath, partsPath, tid, "", "", false)
}

func (r *Runner) checkChapterAt(proj, base, chapPath, texPath, partsPath string, tid int, transcriptPath, viewPath string, strict bool) (bool, string) {
	view := viewPath
	if view == "" {
		view = ensureCheckerView(proj, base, chapPath, texPath, partsPath)
	} else {
		view = ensureCheckerViewAt(viewPath, base, chapPath, texPath, partsPath)
	}
	if view == "" {
		r.log.LogWarning(tid, "[checker]", base, "无法准备只读视图，跳过核对")
		if strict {
			return false, "checker 只读视图无法准备"
		}
		return true, ""
	}
	checkerName := r.cfg.Latex.CheckerModel
	if checkerName == "" {
		checkerName = r.cfg.Latex.ConvertModel
	}
	client := r.clientFor(checkerName)
	modelCfg := r.modelOf(checkerName)
	tuning := r.cfg.LatexSession("checker")

	mounts := []Mount{{Name: "check", Dir: view}}
	trPath := transcriptPath
	if trPath == "" {
		trPath = filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl")
	}
	userText := prompts.Render(prompts.CheckerUser, map[string]string{
		"CHAPTER_MD":  "check:" + base + ".md",
		"CHAPTER_TEX": "check:" + base + ".tex",
		"PARTS_DIR":   "check:parts/",
	})
	if r.cfg.Latex.RemoveWatermark {
		userText += "\n\nWatermark note: if watermark-like content (institution marks, faint background text) is absent from the .tex, that is CORRECT — watermark removal is enabled. Do not report it as missing content."
	}

	// T46：核对必须以一个结论收尾，不能"出错就结束"。两轮尝试：
	// 每轮先正常跑；模型漏了 submit 就按 tikz/mermaid-fix 同款提醒一次；
	// 本轮仍拿不到结论（会话错误 / 提醒后仍未提交）再重开一轮全新会话。
	// 两轮都失败才按既有降级语义视为通过（compile 与终审仍是硬关卡）。
	const checkerAttempts = 2
	for attempt := 1; attempt <= checkerAttempts; attempt++ {
		if attempt > 1 {
			r.log.LogWarning(tid, "[checker]", base, "第 1 次核对未拿到结论，重开核对会话（第 2/2 次）")
		}
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
		var tr *session.TranscriptWriter
		var resumedMsgs []session.ChatMessage
		var resumeSkip bool
		if msgs, stats, err := session.LoadTranscriptStats(trPath); err == nil && len(msgs) > 0 {
			sess.SetMessages(msgs)
			sess.SeedCounters(stats.Rounds, stats.Tools)
			resumedMsgs = msgs
			resumeLog(r.log, tid, "checker", base, msgs, stats)
			resumeSkip = replaySubmit(r.log, tid, "checker:"+base, submit, stats)
		}
		if w, err := session.NewTranscript(trPath); err == nil {
			tr = w
			sess.SetTranscript(w)
		}
		// checker 会话也进实时块：用户要看得到"谁进了 checker、跑到哪了"，
		// 而且 checker 是并发跑的，必须有自己的行而不是挤在转换行里。
		chkHook, chkClose := r.livePhaseRow("checker:"+base, "checker:"+base)
		sess.SetProgressHook(chkHook)
		var runErr error
		if !resumeSkip {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "queued"})
			release := r.acquireChapterSlot()
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
			_, runErr = sess.Run(session.RunOptions{UserText: resumeUserText(resumedMsgs, userText)})
			release()
		}
		chkClose()
		if runErr == nil {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "checking_result"})
		}
		if runErr == nil && !submit.Submitted {
			// 与 tikz 会话同款：模型漏了 submit 就提醒一次。
			r.log.LogWarning(tid, "[checker]", base, "会话结束但未提交结论，发送提交提醒")
			release := r.acquireChapterSlot()
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
			_, runErr = sess.Run(session.RunOptions{
				UserText: "You have NOT called submit yet. Re-read the chapter files if needed, then call submit with your verdict.",
			})
			release()
		}
		if runErr != nil {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "error"})
			if tr != nil {
				tr.Close()
				tr = nil
			}
			r.log.LogWarning(tid, "[checker]", base, "会话错误:", runErr)
			if strict && isSessionAPIError(runErr.Error()) {
				if tr != nil {
					tr.Close()
					tr = nil
				}
				r.keepCheckerTranscript(trPath, strict)
				return false, "checker API failure: " + runErr.Error()
			}
			continue
		}
		if !submit.Submitted {
			r.log.LogWarning(tid, "[checker]", base, "提醒后仍未提交结论")
			if tr != nil {
				tr.Close()
				tr = nil
			}
			continue
		}
		if submit.Status == "issues" {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "issues"})
			if tr != nil {
				tr.Close()
				tr = nil
			}
			issues := strings.TrimSpace(submit.Issues)
			if issues == "" {
				issues = "the checker reported problems but gave no description"
			}
			r.keepCheckerTranscript(trPath, strict)
			return false, issues
		}
		if submit.Status != "pass" {
			r.log.LogWarning(tid, "[checker]", base, "提交了未知结论状态:", submit.Status)
			if tr != nil {
				tr.Close()
				tr = nil
			}
			continue
		}
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "pass"})
		if tr != nil {
			tr.Close()
			tr = nil
		}
		r.keepCheckerTranscript(trPath, strict)
		return true, ""
	}
	r.log.LogWarning(tid, "[checker]", base, checkerAttempts, "次核对均未拿到结论")
	r.keepCheckerTranscript(trPath, strict)
	if strict {
		return false, "checker 多次未能提交结论"
	}
	return true, ""
}

func (r *Runner) keepCheckerTranscript(path string, strict bool) {
	if strict {
		r.log.Log(0, "[checker] 保留递归节点核对转录:", path)
		return
	}
	r.keepSessionFile(path)
}

// ensureCheckerView materialises the checker's read-only view: exactly
// the two files it must compare and the folder with the chapter's
// \input parts. It is rebuilt on every call so it can never point at a
// stale chapter.
func ensureCheckerView(proj, base, chapPath, texPath, partsPath string) string {
	if proj == "" || base == "" {
		return ""
	}
	return ensureCheckerViewAt(filepath.Join(proj, "work", "views", "checker_"+base), base, chapPath, texPath, partsPath)
}

func ensureCheckerViewAt(dir, base, chapPath, texPath, partsPath string) string {
	if dir == "" || base == "" {
		return ""
	}
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
