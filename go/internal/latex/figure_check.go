package latex

import (
	"fmt"
	"path/filepath"
	"strings"

	"mineru-tools/internal/img2text"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// figureChecker verifies ONE redrawn figure against the original bitmap with a
// vision model (latex.figure_check.enabled, default off).
//
// Why it exists: the drawing session only ever sees its own render, so a figure
// that compiles cleanly can still be visibly wrong — a missing axis label, a
// rearranged layout, content running off the canvas. Only a comparison against
// the original catches that, and the cheapest fixer is the session that
// submitted it (its context is still live). So a failed check is fed back to
// that same session and the next render is verified again, up to a round cap.
//
// It is created lazily (on the first check) and lives for the whole image, so
// round 2 of the check sees what round 1 complained about. Transport or
// validation problems degrade to PASS, like the chapter checker: the check is
// an extra net, never a new way for a book to fail.
type figureChecker struct {
	closeRow func()
	log      *logger.Logger
	submit   *SubmitDoneTool
	sess     *session.Session
	orig     string // base64 of the original bitmap
	name     string // display name for logs (the figures/<name> base)
	tid      int
	max      int // round cap from latex.figure_check.max_rounds
	checks   int // verification rounds actually run
	tr       *session.TranscriptWriter
	closed   bool
}

// newFigureChecker builds the checker for one figure: original bitmap + the
// submitted render, judged by a vision model that may only call submit.
func (r *Runner) newFigureChecker(tid int, imgFile, name string, maxRounds int) func(png string) (bool, string) {
	modelName := r.cfg.Latex.FigureCheck.Model
	if modelName == "" {
		modelName = "verifier"
	}
	if !r.hasModel(modelName) {
		// 没配这个模型：退回作图模型（它至少是能看图的），并在日志里说清楚，
		// 免得"校验开着却从没跑过"变成谜。
		r.log.LogWarning(tid, "[figure-check] models 里没有", modelName, "条目，改用作图模型:", r.cfg.Latex.DrawingModel)
		modelName = r.cfg.Latex.DrawingModel
	}
	orig, err := img2text.ImageToBase64(imgFile, 1280)
	if err != nil {
		r.log.LogWarning(tid, "[figure-check] 读取原图失败，跳过逐图校验:", err)
		return nil
	}
	if maxRounds < 1 {
		maxRounds = 1
	}
	submit := &SubmitDoneTool{
		Label:         "the comparison of the redrawn figure with the original",
		RequireReport: true,
		ReportPrompt:  "REQUIRED verdict. status=pass when the redrawn figure faithfully reproduces the original's content and layout; status=issues when it does not, with report.issues listing each visible discrepancy (what is wrong and where).",
	}
	tuning := r.cfg.LatexSession("checker")
	hook, closeRow := r.livePhaseRow("figure-check:"+name, "figure-check:"+name)
	sess := session.NewSession(r.clientFor(modelName), r.modelOf(modelName), tuning,
		renderPrompt(prompts.Must(prompts.FigureCheckSystem), tuning, r.outputLang()),
		[]session.Tool{submit}, r.log, tid, "figure-check:"+name)
	sess.SetProgressHook(hook)
	trPath := filepath.Join(r.projDir, "work", "sessions", "figure_check_"+sanitizeName(name)+".jsonl")
	var tr *session.TranscriptWriter
	if w, err := session.NewTranscript(trPath); err == nil {
		sess.SetTranscript(w)
		tr = w // 校验可能跨多轮，转录要活到 close() 为止
	}
	fc := &figureChecker{closeRow: closeRow, log: r.log, submit: submit, sess: sess,
		orig: orig, name: name, tid: tid, max: maxRounds, tr: tr}
	return fc.check
}

// check runs one verification round on the freshly rasterised render and
// returns the verdict plus the problem list to hand back to the drawing
// session. The checker conversation stays alive across rounds (round 2 sees
// what round 1 complained about) and is closed as soon as the figure passes or
// the round cap is reached. A missing verdict (model stopped, no submit) counts
// as PASS with a warning: the check is an extra net, never a new way for a book
// to fail.
func (fc *figureChecker) check(png string) (bool, string) {
	fig, err := img2text.ImageToBase64(png, 1280)
	if err != nil {
		fc.log.LogWarning(fc.tid, "[figure-check] 读取渲染图失败，视为通过:", err)
		fc.close()
		return true, ""
	}
	fc.checks++
	userText := prompts.Render(prompts.FigureCheckUser, map[string]string{
		"ORIGINAL": "the original bitmap " + filepath.Base(fc.orig),
		"REDRAWN":  filepath.Base(png),
	})
	if fc.checks > 1 {
		userText = "This is round " + fmt.Sprint(fc.checks) +
			" of the same comparison, after the drawing session tried to fix the problems you listed. " +
			"Compare the two images again and submit your verdict."
	}
	if _, err := fc.sess.Run(session.RunOptions{UserText: userText, Images: []string{fc.orig, fig}}); err != nil {
		fc.log.LogWarning(fc.tid, "[figure-check] 校验会话失败，视为通过:", err)
		fc.close()
		return true, ""
	}
	if !fc.submit.Submitted {
		fc.log.LogWarning(fc.tid, "[figure-check] 未提交结论，视为通过")
		fc.close()
		return true, ""
	}
	if fc.submit.Status == "issues" {
		issues := strings.TrimSpace(fc.submit.Issues)
		if issues == "" {
			issues = "the checker reported visible differences but gave no description"
		}
		if fc.checks >= fc.max {
			fc.close()
		}
		return false, issues
	}
	fc.close()
	return true, ""
}

// feedback is what the drawing session receives when its figure did not pass:
// the checker's own words, plus the two rules it must obey when fixing.
func figureCheckFeedback(problems string) string {
	return "A vision check compared your submitted figure with the ORIGINAL image and found visible differences:\n" +
		problems + "\n\n" +
		"Fix exactly these points in figure.tex (do not change anything else), call compile {path: \"figure.tex\"}, " +
		"compare your render with the original once more, and call submit again. " +
		"Sizes, positions and labels in the original are the reference."
}

// close finalises the live row and the transcript (this figure is done: it
// passed, or the round cap is reached).
func (fc *figureChecker) close() {
	if fc.closed {
		return
	}
	fc.closed = true
	if fc.closeRow != nil {
		fc.closeRow()
	}
	if fc.tr != nil {
		_ = fc.tr.Close()
	}
}
