package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// checkerNoVerdict 报告这次核对失败的原因是"核对会话自己没给出结论"——模型把
// submit 写成了正文（实测：`\submit{ "report": … }` 一整段行文本，finish=stop、
// tool_calls 为空），或者连续 API 错误——而不是"章节有问题"。
//
// 为什么要单独认出来（T209）：这种失败被拿去喂反馈轮时，转换会话收到的
// 是"checker 发现了问题：<其实是核对失败的原因>"，它能做的只有把同样的产物
// 再交一遍，然后撞同一堵墙。实测一条核对连续 6 轮、每轮 2 次会话、每轮约 30 秒
// 全部落在这个循环里，钱花了、结论一个字都没有。判定依据是**没有 submit 回执**
// 这个事实，不是正文长什么样。
func checkerNoVerdict(issues string) bool {
	return strings.HasPrefix(issues, "checker 多次未能提交结论") || strings.HasPrefix(issues, "checker API failure")
}

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
func (r *Runner) checkChapter(proj, base, chapPath, texPath, partsPath string, tid int, convertAttempt int) (bool, string) {
	return r.checkChapterAt(proj, base, chapPath, texPath, partsPath, checkHints{}, tid, "", "", false, convertAttempt)
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

func (r *Runner) checkChapterAt(proj, base, chapPath, texPath, partsPath string, hints checkHints, tid int, transcriptPath, viewPath string, strict bool, convertAttempt int) (bool, string) {
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
	// T204：轮次预算随层级加宽——这一章要是被细分过，父节点的核对要通读整棵
	// 子树（每个孩子的内容都要过一遍），最里层只管自己那一片。层级从章节 ID 数
	// 出来（`chapter_014_child_03_child_02` = 第 2 层），离最深一层还有几层就加
	// 几档；配置见 `sessions.checker.max_tool_rounds_per_level` /
	// `max_tool_rounds_factor`（默认 50 + 20×层数）。
	if levels := maxChapterSplitDepth - chapterDepthOf(base); levels > 0 {
		tuning.MaxToolRounds = tuning.ToolRoundsAtLevels(levels)
	}

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
	userText += checkerClaimNote(view)
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
		var ckSeed liveSeed
		if msgs, stats, err := session.LoadTranscriptStats(trPath); err == nil && len(msgs) > 0 {
			sess.SetMessages(msgs)
			sess.SeedCounters(stats.Rounds, stats.Tools)
			resumedMsgs = msgs
			ckSeed = liveSeed{rounds: stats.Rounds, tools: stats.Tools, active: stats.Active, resumed: true}
			resumeLog(r.log, tid, "checker", base, msgs, stats)
			resumeSkip = replaySubmit(r.log, tid, "checker:"+base, submit, stats)
		}
		if w, err := session.NewTranscript(trPath); err == nil {
			tr = w
			sess.SetTranscript(w)
		}
		// P59：核对会话记录它核对的**转换转录**（文件名里的尝试号与核对自己的
		// 号没有关系：手动重新核对只让核对的号加一，核对的还是同一份产物）。
		// 预览页的树与历史都按这个锚点配对；每次重核（含重开一轮）都写一条，
		// 指回同一份转换。拿不到转换尝试号（0）时宁可不写，也不编一个。
		if convertAttempt < 1 {
			r.log.LogWarning(tid, "[checker]", base, "不知道这次的转换尝试号，核对锚点未记录")
		} else {
			_ = sess.AppendTranscriptNote("checker_target", map[string]any{
				"convert":         checkerAnchorName(base, convertAttempt),
				"convert_attempt": convertAttempt,
			})
		}
		// checker 会话也进实时块：用户要看得到"谁进了 checker、跑到哪了"，
		// 而且 checker 是并发跑的，必须有自己的行而不是挤在转换行里。
		chkHook, chkState, chkClose := r.livePhaseRowState("checker:"+base, "checker:"+base, ckSeed)
		sess.SetProgressHook(chkHook)
		var runErr error
		if !resumeSkip {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "queued"})
			chkState(rowStateQueued)
			release := r.acquireCheckerSlot()
			chkState(rowStateRunning)
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
			_, runErr = sess.Run(session.RunOptions{UserText: resumeUserText(resumedMsgs, userText)})
			release()
		}
		chkClose()
		if runErr == nil {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "checking_result"})
		}
		if runErr == nil && !submit.Submitted {
			// P58：先让复盘会话看一眼——还健康就照常提醒；已经退化成正文工具调用/
			// 重复内容时，截断到最后一个干净轮次再核对（排进重试队列，与页面
			// 的 ⤓ 同一条通路）；救不回来就直接上报，不再拿同一个坏会话烧两轮。
			hint := ""
			if r.cfg.Latex.RescueEnabled() {
				target := rescueTarget{ProjDir: proj, Base: base, Role: "checker", TrPath: trPath}
				if r.rescueBudget(base + "/checker/" + filepath.Base(trPath)) {
					if v, ok := r.rescueReview(target, tid); ok {
						switch v.Verdict {
						case "fail":
							r.log.LogWarning(tid, "[checker]", base, "复盘判定核对会话无法挽救："+v.Reason)
							if tr != nil {
								tr.Close()
								tr = nil
							}
							r.keepCheckerTranscript(trPath, strict)
							return false, "checker 复盘判定会话无法挽救：" + v.Reason
						case "truncate":
							if err := r.rescueQueueTruncate(target, v.From); err == nil {
								r.log.LogWarning(tid, "[checker]", base, fmt.Sprintf("复盘判定核对会话已退化，已排队从第 %d 条记录继续核对：%s", v.From, v.Reason))
								if tr != nil {
									tr.Close()
									tr = nil
								}
								r.keepCheckerTranscript(trPath, strict)
								return false, "checker 会话已退化，已排队从干净轮次继续核对"
							}
						case "retry":
							hint = v.Hint
						}
					}
				}
			}
			// 与 tikz 会话同款：模型漏了 submit 就提醒一次。
			r.log.LogWarning(tid, "[checker]", base, "会话结束但未提交结论，发送提交提醒")
			chkState(rowStateQueued)
			release := r.acquireCheckerSlot()
			chkState(rowStateRunning)
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
			remindText := "You have NOT called submit yet. Use the `submit` TOOL (a real tool call with its JSON arguments — writing `submit` or `\\submit{...}` inside your reply text does nothing): re-read the chapter files if needed, then call submit with your verdict."
			if hint != "" {
				remindText += " Reviewer note: " + hint
			}
			_, runErr = sess.Run(session.RunOptions{
				// T209：同上——写成正文不算，必须真的发一次 submit 工具调用。
				UserText: remindText,
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

// checkerAnchorName 是核对会话的锚点（P59）：产出这份产物的转换转录的**文件名**
// （`convert_<章>` / `convert_<章>.tryN`，不含扩展名）。
//
// 为什么用文件名而不是路径：预览页的章节树按「项目 + 章节 ID」聚合，同一章的
// 顶层转录在 `work/sessions/`、递归子章节的转录在 `<节点>/sessions/`，路径不同
// 但文件名口径一致；锚点只回答"哪一次转换"，位置由扫描目录决定。
func checkerAnchorName(base string, convertAttempt int) string {
	return strings.TrimSuffix(filepath.Base(attemptTranscriptPath("convert_"+base+".jsonl", convertAttempt)), ".jsonl")
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
	// 章节目录要按**它自己的布局**出现，而不是被改名成 parts/。
	//
	// 两种提交约定并存：普通章节把分片直接交在 `<章>/` 下（tex 写
	// `\input{<章>/x.tex}` 或 `\input{parts/x.tex}`），递归/整合过的节点交的是
	// `<章>/parts/…` 与 `<章>/figures/…`（tex 写 `\input{parts/children/x.tex}`
	// 与 `\input{figures/motif_x.tex}`）。旧视图一律把章节目录挂成 `parts/`，
	// 于是后一种约定下 tex 里每一个路径在视图里都指向不存在的位置，核对会话
	// 只能报"`\input` 路径写错了""插图被漏掉了"——真的 `.tex` 却是对的。实测
	// 一个项目留档的 49 条打回理由里，18 条是路径、10 条是插图。
	if st, err := os.Stat(partsPath); err == nil && st.IsDir() {
		// 目录内容铺到视图根：`parts/…`、`figures/…` 与 tex 同级解析。
		if entries, err := os.ReadDir(partsPath); err == nil {
			for _, e := range entries {
				name := e.Name()
				dst := filepath.Join(dir, name)
				if fileExists(dst) {
					continue
				}
				src := filepath.Join(partsPath, name)
				if err := linkAbs(src, dst); err != nil {
					if e.IsDir() {
						_ = copyDir(src, dst)
					} else {
						_ = copyFile(src, dst)
					}
				}
			}
		}
		// 整目录再挂两份：`<章>/`（提交布局里的 `\input{<章>/…}`）与
		// `parts/`（旧的扁平约定与提示词里的 PARTS_DIR）。铺出来的同名条目优先，
		// 这里只补缺。
		for _, alias := range []string{base, "parts"} {
			dst := filepath.Join(dir, alias)
			if fileExists(dst) {
				continue
			}
			if err := linkAbs(partsPath, dst); err != nil {
				_ = copyDir(partsPath, dst)
			}
		}
	} else if err := os.MkdirAll(filepath.Join(dir, "parts"), 0o755); err != nil {
		return ""
	}
	// `<proj>/chapters/<章>.md` → `<proj>`
	linkCheckerAssets(dir, filepath.Dir(filepath.Dir(chapPath)))
	return dir
}

// linkCheckerAssets 把项目级的 figures/ images/ 也挂进核对视图：章节 .tex 里
// `\input{figures/motif_*.tex}`、`\includegraphics{images/…}` 在转换工作区与
// 成书树里都能解析（`chapterWorkTreeAt` 就挂了这两枚软链），核对视图若没有，
// 模型会报告"插图被漏掉了"——同样是视图造成的假问题，代价是一次重转。
func linkCheckerAssets(dir, proj string) {
	for _, asset := range []string{"figures", "images"} {
		src := filepath.Join(proj, "source", asset)
		if !fileExists(src) {
			continue
		}
		dst := filepath.Join(dir, asset)
		if !fileExists(dst) {
			if err := linkAbs(src, dst); err != nil {
				_ = copyDir(src, dst)
			}
			continue
		}
		// 章节自带的 figures/ 已经在视图里了：**逐条并进去**（章节插图与项目
		// 级插图都要能读到，整个跳过会让项目级那批 `\input{figures/…}` 落空）。
		entries, err := os.ReadDir(src)
		if err != nil {
			continue
		}
		for _, e := range entries {
			dstEntry := filepath.Join(dst, e.Name())
			if fileExists(dstEntry) {
				continue
			}
			if err := linkAbs(filepath.Join(src, e.Name()), dstEntry); err != nil {
				if e.IsDir() {
					_ = copyDir(filepath.Join(src, e.Name()), dstEntry)
				} else {
					_ = copyFile(filepath.Join(src, e.Name()), dstEntry)
				}
			}
		}
	}
}

// checkerClaimNote 只点名真实存在的"说法"文件（T183）：以前只要 report.md 在就
// 顺带说 check:notes.md 也在，模型照着去读、白花一次工具调用（读不到还要再想
// 一遍"这个文件为什么没有"）。
func checkerClaimNote(view string) string {
	var claims []string
	if fileExists(filepath.Join(view, "report.md")) {
		claims = append(claims, "check:report.md (the converter's own report)")
	}
	if fileExists(filepath.Join(view, "notes.md")) {
		claims = append(claims, "check:notes.md (its submit notes)")
	}
	if len(claims) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(claims, " and ") + " — CLAIMS to verify against the page and this PDF, never facts."
}
