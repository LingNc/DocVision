package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
//
// T217：一次结论都没拿到的路径单独有了一句更明确的文案（"核对始终没有给出结论
// （不是内容问题，共尝试 N 次）…"），它同样是"没结论"、同样必须跳过反馈轮。
//
// 现场补（用户报的截图）：核对会话退化、已经被排进队列"从干净轮次继续核对"时，
// 返回的是"checker 会话已退化，已排队从干净轮次继续核对"——这不是章节的问题，
// 而且那一章的核对已经重新排队了；可它不在上面三条前缀里，于是照样被当成
// "checker 发现了问题"喂回转换会话，转换只能把同一份产物再交一遍，而界面上一
// 边写着"已排队继续核对"、一边又要求转换会话修内容，两件事自相矛盾。同一类的
// 还有"复盘判定会话无法挽救"（放弃这次核对）与"只读视图无法准备"（基础设施
// 失败）——三种都**没有对章节下过任何结论**。
func checkerNoVerdict(issues string) bool {
	return strings.HasPrefix(issues, "核对始终没有给出结论") ||
		strings.HasPrefix(issues, "checker 多次未能提交结论") ||
		strings.HasPrefix(issues, "checker API failure") ||
		strings.HasPrefix(issues, "checker 会话已退化") ||
		strings.HasPrefix(issues, "checker 复盘判定会话无法挽救") ||
		strings.HasPrefix(issues, "checker 只读视图无法准备")
}

// checkerNoVerdictRetryCycles 是"核对没给结论"之后**只重开核对会话**的次数。
//
// T222：这类收尾以前会走进"作废产物、换全新会话整章重转"那条路（最多 6 次），
// 而界面上那句话写的是「已排队从干净轮次继续核对」——两件事对不上，而且重转
// 出来的是同一份东西：产物根本还没被判定过，重做一遍既不会更有结论，也没理由
// 假设新的转换会话能救回一个坏掉的核对模型。所以这里只再开一次核对会话（同一份
// 产物、同一条只读视图），仍没有结论就以"checker 不可用"收尾并保留产物。
const checkerNoVerdictRetryCycles = 1

// chapterRecheckPending 报告这一章的产物在、但上一次核对**没有下过判定**
// （`<章>.tex.checker` 里是 checkerNoVerdict 认得的那类文案）。
//
// T222：这种产物不能被"已有产物，跳过"静默采纳——跳过意味着整章连核对都不跑，
// 一份从没被判定过的东西就这样进了书。带这个标记的章节重跑时先重核这一份。
func chapterRecheckPending(texPath string) bool {
	data, err := os.ReadFile(texPath + ".checker")
	if err != nil {
		return false
	}
	return checkerNoVerdict(string(data))
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

// checkChapterAt 是核对入口。它把这一轮的结果落到**核对转录**上（T231）：没给结论
// 的几种收尾（退化、复盘判定无法挽救、只读视图没准备好、始终不 submit）统一写
// no_verdict；会话真报错（API/余额）照旧留 error——那是真的出错，不能一起糊成
// "没结论"。写在入口而不是每个 return 上，是因为返回点有六处，漏一处就会重新
// 出现"页面上说会话错误、实际只是核对没交结论"。
func (r *Runner) checkChapterAt(proj, base, chapPath, texPath, partsPath string, hints checkHints, tid int, transcriptPath, viewPath string, strict bool, convertAttempt int) (bool, string) {
	ok, issues := r.runChapterCheck(proj, base, chapPath, texPath, partsPath, hints, tid, transcriptPath, viewPath, strict, convertAttempt)
	if !ok && checkerNoVerdict(issues) && !strings.HasPrefix(issues, "checker API failure") {
		trPath := transcriptPath
		if trPath == "" {
			trPath = filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl")
		}
		markCheckerNoVerdict(trPath)
	}
	return ok, issues
}

func (r *Runner) runChapterCheck(proj, base, chapPath, texPath, partsPath string, hints checkHints, tid int, transcriptPath, viewPath string, strict bool, convertAttempt int) (bool, string) {
	view := viewPath
	if view == "" {
		view = ensureCheckerView(proj, base, chapPath, texPath, partsPath)
	} else {
		view = ensureCheckerViewAt(proj, viewPath, base, chapPath, texPath, partsPath)
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
	// T230：**这条转录已经被一整轮的"没提交"预算打满过**，就别再拿它开新会话了。
	// 旧行为是只要文件还在就回放，于是父节点的再驱动、页面上的重新核对都在同一条
	// 文件后面继续追加：现场一条核对转录 319 行、几十万字节，34 次 run_end、74 条
	// 章节活动，全是"把 submit 写进正文 → 被提醒 → 再写一遍"，钱花了、结论一个字
	// 没有。到线就如实报"没结论"（上游按 T222 保留产物），要换干净会话走 ⤓ 截断
	// 或「重新核对」——那两条会另开一次尝试，提醒数从零算起。
	//
	// 判据用"这条转录里真实留下的提醒条数"，而不是"开了几次会话"：一次调用内部
	// 重开 4 个会话（T217）是设计好的预算，跨调用的重复才是退化。
	if n := countCheckerSubmitReminders(loadTranscriptMessages(trPath)); n >= checkerNoVerdictAttempts {
		r.keepCheckerTranscript(trPath, strict)
		r.log.LogWarning(tid, "[checker]", base, "这条核对转录里已有",
			strconv.Itoa(n), "次「没提交」的提醒，不再原地续跑:", filepath.Base(trPath))
		return false, fmt.Sprintf("checker 多次未能提交结论：这条核对转录里已经攒了 %d 次提交提醒"+
			"（会话反复把结论写进正文，没有真的调用 submit），不再原地续跑它——"+
			"产物保留，要换干净的核对会话用页面上的 ⤓ 截断或「重新核对」", n)
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

	// T46：核对必须以一个结论收尾，不能"出错就结束"。每轮先正常跑；模型漏了
	// submit 就按 tikz/mermaid-fix 同款提醒一次；本轮仍拿不到结论（会话错误 /
	// 提醒后仍未提交）再重开一轮全新会话。
	//
	// T217：**一次结论都没拿到**时把预算放宽到 checkerNoVerdictAttempts 次。这种
	// 失败不是内容问题（会话跑挂、或模型始终不调 submit），旧实现 2 次就上报，
	// 调用方把它当成"核对不过"而作废整章、从头重转——实测一整章就因为核对始终
	// 没提交被白白重跑一遍。重开一次核对会话比整章重来便宜得多，所以这里多重试
	// 几次（每次都是全新会话、同一转录路径、同一只读视图）。
	// 反过来，**只要有任何一次拿到过结论**，预算就回到 checkerAttempts（2）：
	// 那时要处理的是内容，交给调用方按 checker_rounds 把问题喂回转换会话。
	apiFailure := ""
	sawVerdict := false
	attemptsMade := 0
	for attempt := 1; ; attempt++ {
		limit := checkerAttempts
		if !sawVerdict {
			limit = checkerNoVerdictAttempts
		}
		if attempt > limit {
			break
		}
		attemptsMade = attempt
		if attempt > 1 {
			r.log.LogWarning(tid, "[checker]", base, fmt.Sprintf("第 %d 次核对未拿到结论，重开核对会话（第 %d/%d 次）", attempt-1, attempt, limit))
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
			// 与 convert 走同一个入口（maybeRescue），结论处理不再有第二份。
			dec := r.maybeRescue(tid, "[checker]", base, rescueTarget{
				ProjDir: proj, Base: base, Role: "checker", TrPath: trPath,
				NoToolNudge: sess.ToolAccessNudged(), Sess: sess,
			})
			if dec.Stop {
				if tr != nil {
					tr.Close()
					tr = nil
				}
				r.keepCheckerTranscript(trPath, strict)
				if dec.Verdict == "fail" {
					return false, "checker 复盘判定会话无法挽救：" + dec.Reason
				}
				return false, "checker 会话已退化，已排队从干净轮次继续核对"
			}
			hint := dec.Hint
			// 与 tikz 会话同款：模型漏了 submit 就提醒一次。
			r.log.LogWarning(tid, "[checker]", base, "会话结束但未提交结论，发送提交提醒")
			chkState(rowStateQueued)
			release := r.acquireCheckerSlot()
			chkState(rowStateRunning)
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
			remindText := checkerSubmitReminderMark + " Use the `submit` TOOL (a real tool call with its JSON arguments — writing `submit` or `\\submit{...}` inside your reply text does nothing): re-read the chapter files if needed, then call submit with your verdict."
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
		// T217：拿到过结论了（issues 或无法识别的状态都算）——轮次预算就此回到
		// checkerAttempts，不再为"没结论"多开会话。
		sawVerdict = true
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
	r.log.LogWarning(tid, "[checker]", base, attemptsMade, "次核对均未拿到结论")
	r.keepCheckerTranscript(trPath, strict)
	if apiFailure != "" {
		return false, "checker API failure: " + apiFailure
	}
	// T217：一次结论都没有，与"核对说了这一章有问题"是两回事。旧文案只说
	// "未能提交结论"，调用方与用户都容易读成"内容没通过"；这里点明原因、次数，
	// 让上层（checkerNoVerdict）与人都看得出该做的是重开核对，而不是重转整章。
	if !sawVerdict {
		return false, fmt.Sprintf("核对始终没有给出结论（不是内容问题，共尝试 %d 次）：会话一次都没有调用 submit，可能把结论写成了正文或直接结束", attemptsMade)
	}
	// 旧行为在这里对非 strict 节点**直接判通过**——实测代价极大：一条跑完的
	// 章节它的 checker 从头到尾没提交过结论（8 轮、25 次工具调用、38 分钟），
	// 却因为这条兜底被当成"核对通过"写进了书。没结论就是没结论。
	return false, "checker 多次未能提交结论（会话未调用 submit）"
}

// checkerAttempts 是一次核对里"已经拿到过结论"时的会话预算；
// checkerNoVerdictAttempts 是**一次结论都没拿到**时的预算（T217：重开一个核对
// 会话比整章重转便宜得多）。后者同时是"同一条转录还能不能被再次回放"的线——
// 攒满这一整轮还没结论，说明这条历史本身已经跑不动了（见 checkChapterAt 入口）。
//
// T230：这条线以前只管**一次调用内部**重开几次。转录文件是跨调用共享的，于是
// 父节点的再驱动、页面上的重新核对把同一条历史反复回放，坏尾巴越滚越长：实测
// 一条递归节点的核对转录 319 行、几十万字节，34 次 run_end、74 条章节活动、18 条
// 核对锚点，全是"把 submit 写进正文 → 被提醒 → 再写一遍"的死循环，钱花了、结论
// 一个字没有。现在同一个数字也当"这条转录还能不能再次回放"的线：攒够一整轮还
// 没结论就不再回放它（如实报"没结论"，上游按 T222 保留产物）。要换干净会话，
// 走 ⤓ 截断或页面「重新核对」——那两条会另开一次尝试，提醒数从零算起。
const (
	checkerAttempts          = 2
	checkerNoVerdictAttempts = 4
)

// checkerSubmitReminderMark 是"这条会话没有交结论"的提醒开头（T230）。转录里
// 出现一次这样的 user 消息，就等于那一次续跑没能拿到结论——判断一条核对转录
// 是否已经退化到不该再回放，只看这个计数，不看正文写了什么。
const checkerSubmitReminderMark = "You have NOT called submit yet."

// loadTranscriptMessages 读一条转录的历史消息（读不到就当空：新会话）。
func loadTranscriptMessages(path string) []session.ChatMessage {
	if path == "" {
		return nil
	}
	msgs, _, err := session.LoadTranscriptStats(path)
	if err != nil {
		return nil
	}
	return msgs
}

// countCheckerSubmitReminders 数一条（将要回放的）会话历史里有几次提交提醒。
func countCheckerSubmitReminders(msgs []session.ChatMessage) int {
	n := 0
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		s, ok := m.Content.(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(s), checkerSubmitReminderMark) {
			n++
		}
	}
	return n
}

func (r *Runner) keepCheckerTranscript(path string, strict bool) {
	if strict {
		r.log.Log(0, "[checker] 保留递归节点核对转录:", path)
		return
	}
	r.keepSessionFile(path)
}

// markCheckerNoVerdict 把"这一轮核对没给结论"写在这条核对转录自己身上（T231）。
//
// 这里不写 error：核对会话确实跑完了，只是没交结论，页面上原来那条"会话错误"
// 既冤枉了转换、也说不清到底发生了什么。
func markCheckerNoVerdict(trPath string) {
	_ = appendTranscriptActivity(trPath, "no_verdict")
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
	return ensureCheckerViewAt(proj, filepath.Join(proj, "work", "views", "checker_"+base), base, chapPath, texPath, partsPath)
}

// ensureCheckerViewAt 铺开一份只读视图。`proj` 必须显式传进来：递归子章节的 md
// 不在 `<proj>/chapters/` 下，而在 `<节点>/../input/chapter.md`，按 chapPath 往上
// 两级猜项目根会把子章节猜成 `_subdivision/child_02`，于是项目级 `figures|images`
// 一个都挂不上（T186 的"插图被漏掉"就这样在子章节一侧继续发生）。
func ensureCheckerViewAt(proj, dir, base, chapPath, texPath, partsPath string) string {
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
	// `<proj>/source/figures|images` 一并挂上（章节 .tex 里 `\input{figures/…}`、
	// `\includegraphics{images/…}` 在转换工作区与成书树里都能解析）。
	linkCheckerAssets(dir, proj)
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
