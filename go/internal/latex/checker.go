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
	return r.checkChapterAt(proj, base, chapPath, texPath, partsPath, checkHints{}, tid, "", "", false)
}

// checkHints 是转换会话留给 checker 的线索：本次编译产物、它自己的工作汇报与
// submit.notes（T172）。它们都只是**说法**——checker 必须自己对着原书页或产物
// 页核实；给它们的目的是让"看起来不对但其实正确"的差异（md 是 OCR 文本、公式
// 与表格来自原页、插图排成浮动体）不必被误报成缺失。
type checkHints struct {
	PDF    string // 编译出的章节 PDF（挂成 check:<base>.pdf）
	Report string // 转换会话的 report（挂成 check:report.md）
	Notes  string // submit.notes（写成 check:notes.md）
}

func (r *Runner) checkChapterAt(proj, base, chapPath, texPath, partsPath string, hints checkHints, tid int, transcriptPath, viewPath string, strict bool) (bool, string) {
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
	// T172：把产物与转换会话自己的说法挂进只读视图。旧行为 checker 只拿到 md 与
	// tex 两个文本，看不见产物、也看不见原书页——凡是"md 这样写、tex 那样写"的
	// 差异都只能猜，于是把 OCR 噪声、原页公式、排成浮动体的表格报成缺失，转换
	// 会话被逼着改坏本来正确的内容。
	if hints.PDF != "" && fileExists(hints.PDF) {
		if err := linkAbs(hints.PDF, filepath.Join(view, base+".pdf")); err != nil {
			_ = copyFile(hints.PDF, filepath.Join(view, base+".pdf"))
		}
	}
	if hints.Report != "" && fileExists(hints.Report) {
		_ = linkAbs(hints.Report, filepath.Join(view, "report.md"))
	}
	if notes := strings.TrimSpace(hints.Notes); notes != "" {
		_ = os.WriteFile(filepath.Join(view, "notes.md"), []byte(notes+"\n"), 0o644)
	}

	checkerName := r.cfg.Latex.CheckerModel
	if checkerName == "" {
		checkerName = r.cfg.Latex.ConvertModel
	}
	client := r.clientFor(checkerName)
	modelCfg := r.modelOf(checkerName)
	tuning := r.cfg.LatexSession("checker")

	checkMounts := []Mount{{Name: "check", Dir: view}}
	// view_pdf 还要能打开原书 part（source:）与刚才挂进来的产物页。
	viewMounts := append([]Mount{}, checkMounts...)
	viewMounts = append(viewMounts, r.pdfView.Mounts()...)
	trPath := transcriptPath
	if trPath == "" {
		trPath = filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl")
	}
	userText := prompts.Render(prompts.CheckerUser, map[string]string{
		"CHAPTER_MD":  "check:" + base + ".md",
		"CHAPTER_TEX": "check:" + base + ".tex",
		"PARTS_DIR":   "check:parts/",
	})
	if fileExists(filepath.Join(view, base+".pdf")) {
		userText += "\n\nThe compiled PDF of this LaTeX is check:" + base + ".pdf — open its pages with view_pdf to see what the page actually looks like (floats, table splits, figure placement)."
	}
	if r.pdfView != nil && r.pdfView.total > 0 {
		userText += "\n\nThe book's original pages are readable as source:<part>.pdf (view_pdf), and list_source_pages / doc_search locate them. When the markdown is unreadable, or the .tex follows the page rather than the markdown text, judge against the page."
	}
	if fileExists(filepath.Join(view, "report.md")) || fileExists(filepath.Join(view, "notes.md")) {
		userText += "\n\ncheck:report.md (the converter's own report) and check:notes.md (its submit notes) are CLAIMS to verify against the page and this PDF — never facts."
	}
	if r.cfg.Latex.RemoveWatermark {
		userText += "\n\nWatermark note: if watermark-like content (institution marks, faint background text) is absent from the .tex, that is CORRECT — watermark removal is enabled. Do not report it as missing content."
	}

	// T46：核对必须以一个结论收尾，不能"出错就结束"。两轮尝试：
	// 每轮先正常跑；模型漏了 submit 就按 tikz/mermaid-fix 同款提醒一次；
	// 本轮仍拿不到结论（会话错误 / 提醒后仍未提交）再重开一轮全新会话。
	// 两轮都失败才按既有降级语义视为通过（compile 与终审仍是硬关卡）。
	const checkerAttempts = 2
	apiFailure := ""
	for attempt := 1; attempt <= checkerAttempts; attempt++ {
		if attempt > 1 {
			r.log.LogWarning(tid, "[checker]", base, "第 1 次核对未拿到结论，重开核对会话（第 2/2 次）")
		}
		submit := &SubmitDoneTool{
			Label:         "the review of chapter " + base,
			RequireReport: true, // 结论就用这个交（不落盘：写不写 .checker 由 runner 决定）
		}
		tools := []session.Tool{
			&ReadFileTool{Mounts: checkMounts},
			&GrepTool{Mounts: checkMounts},
			&ViewPDFTool{Mounts: viewMounts, Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio()},
			submit,
		}
		tools = append(tools, r.sourcePageTools()...)
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
			release := r.acquireCheckerSlot()
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
			release := r.acquireCheckerSlot()
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
			// 厂商/网络错误（含空响应重发后仍空）不是章节本身的问题：本轮
			// 记下来、重开一次核对会话；两次都这样就把这个原因上报，**不要**
			// 拿它去触发"换会话重转换"——那只会让同一个坏模型再失败 N 次。
			if isSessionFatalError(runErr.Error()) {
				// 余额不足/无可用资源包：换会话、重开都只会得到同一个错误，
				// 立刻上报（用户换模型后续跑）。
				if tr != nil {
					tr.Close()
					tr = nil
				}
				r.keepCheckerTranscript(trPath, strict)
				return false, "checker API failure: " + runErr.Error()
			}
			if isSessionAPIError(runErr.Error()) {
				apiFailure = runErr.Error()
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
	if apiFailure != "" {
		return false, "checker API failure: " + apiFailure
	}
	// 旧行为在这里对非 strict 节点**直接判通过**——实测代价极大：一条跑完的
	// 章节它的 checker 从头到尾没提交过结论（8 轮、25 次工具调用、38 分钟），
	// 却因为这条兜底被当成"核对通过"写进了书。没结论就是没结论。
	return false, "checker 多次未能提交结论（会话未调用 submit）"
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
