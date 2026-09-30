package latex

// pilot.go — T97 重量档：首章试转 + 样式修订。
//
// 章节划分完成后、全书转换开始前：先用新样式包把第一章真实转换一遍
// （含 checker），再把试转产物铺进样式工作区，复用原样式会话的上下文
// 对照原书修订 cls/manual/example。cls 内容变了 → 作废首章产物，主转换
// 阶段用新样式包重转；没变 → 首章产物直接保留（convertPhase 按产物
// 存在跳过）。pilot 是建议性的：任何失败只警告并写 marker，绝不挡住
// 全书转换；marker 也保证中断续跑不会重复 pilot。

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// pilotMarkerPath 是 pilot 的完成标记（内容为人可读的结果摘要）。
func pilotMarkerPath(proj string) string {
	return filepath.Join(proj, "work", "style", "pilot.done")
}

func (r *Runner) pilotPhase(proj string) {
	marker := pilotMarkerPath(proj)
	if fileExists(marker) {
		return
	}
	writeMarker := func(note string) {
		_ = os.WriteFile(marker, []byte(note+"\n"), 0o644)
	}
	chaps, err := filepath.Glob(filepath.Join(proj, "chapters", "chapter_*.md"))
	if err != nil || len(chaps) == 0 {
		return
	}
	sort.Strings(chaps)
	base := strings.TrimSuffix(filepath.Base(chaps[0]), filepath.Ext(chaps[0]))
	workDir := filepath.Join(proj, "work")
	if fileExists(filepath.Join(workDir, "chapters", base+".tex")) {
		// 首章已转换（含上次 pilot 已成功）：不重复试转。
		writeMarker("skipped: first chapter already converted")
		return
	}
	clsName := classNameOfFile(filepath.Join(proj, "style"))
	if clsName == "" {
		return
	}
	r.phaseNote()("[pilot] 首章试转 %s + 样式修订（latex.style_pilot: false 可关）", base)
	r.log.Log(0, "[pilot] 首章试转", base)

	if err := r.convertOneChapter(proj, clsName, filepath.Join(proj, "style", "manual.md"), chaps[0], workDir, 0, 1, false); err != nil {
		r.log.LogWarning(1, "[pilot] 首章试转失败（跳过样式修订，主转换阶段照常重试）:", err)
		writeMarker("convert failed: " + err.Error())
		return
	}
	if err := r.pilotStyleRevision(proj, clsName, base); err != nil {
		r.log.LogWarning(1, "[pilot] 样式修订失败（沿用现有样式包继续全书转换）:", err)
		writeMarker("revision failed: " + err.Error())
		return
	}
	writeMarker("done")
	r.log.Log(1, "[pilot] 完成")
}

// styleDigest 是当前样式包（cls+manual）的内容指纹：修订后比对决定
// 首章产物是否作废。
func styleDigest(proj string) string {
	h := sha256.New()
	styleDir := filepath.Join(proj, "style")
	for _, pat := range []string{"*.cls", "manual.md"} {
		files, _ := filepath.Glob(filepath.Join(styleDir, pat))
		sort.Strings(files)
		for _, f := range files {
			if data, err := os.ReadFile(f); err == nil {
				h.Write([]byte(filepath.Base(f)))
				h.Write(data)
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// pilotStyleRevision 复用样式会话上下文做 pilot 修订：试转产物铺在
// work/style/pilot/ 里（cls 副本同目录，compile {path:"pilot/<章>.tex"}
// 直接可编译），会话对照原书修订后 submit_style；cls/manual 变了就把
// 首章产物作废（主转换阶段用新样式包重转）。
func (r *Runner) pilotStyleRevision(proj, clsName, base string) error {
	workDir := filepath.Join(proj, "work", "style")
	styleDir := filepath.Join(proj, "style")
	chaptersDir := filepath.Join(proj, "work", "chapters")

	// 1) 铺 pilot 树：章节 tex + parts + 当前 cls 副本（同目录编译可解析）。
	pilotDir := filepath.Join(workDir, "pilot")
	_ = os.RemoveAll(pilotDir)
	if err := os.MkdirAll(pilotDir, 0o755); err != nil {
		return err
	}
	copyFile(filepath.Join(styleDir, clsName+".cls"), filepath.Join(pilotDir, clsName+".cls"))
	if err := copyFile(filepath.Join(chaptersDir, base+".tex"), filepath.Join(pilotDir, base+".tex")); err != nil {
		return fmt.Errorf("试转产物缺失: %w", err)
	}
	parts := filepath.Join(chaptersDir, base)
	if fi, err := os.Stat(parts); err == nil && fi.IsDir() {
		_ = copyDir(parts, filepath.Join(pilotDir, base))
	}

	before := styleDigest(proj)

	// 2) 复用样式会话上下文（同 stylefix 的复用模式）。
	ctxPath := filepath.Join(proj, "work", "style_session.jsonl")
	msgs, err := loadSessionContext(ctxPath)
	if err != nil || len(msgs) == 0 {
		return fmt.Errorf("样式会话上下文不可用: %v", err)
	}
	client := r.clientFor(r.cfg.Latex.StyleModel)
	modelCfg := r.modelOf(r.cfg.Latex.StyleModel)
	tuning := r.cfg.LatexSession("style")
	goal := NewGoalTool(filepath.Join(workDir, "plan.md"))
	submit := &SubmitStyleTool{Workspace: workDir, Gate: func() string { return goalGate(goal) }}
	bashTmp, cleanBash := r.sessionBashTemp(proj, "bash_pilot")
	defer cleanBash()
	tools := r.styleSessionTools(proj, workDir, filepath.Join(proj, "source"), "pilot-style", bashTmp, submit)
	tools = append(tools, goal)

	sess := session.NewSession(client, modelCfg, tuning, r.styleSystemPrompt(), tools, r.log, 1, "pilot-style")
	sess.SetMessages(msgs)
	if _, st, serr := session.LoadTranscriptStats(ctxPath); serr == nil {
		sess.SeedCounters(st.Rounds, st.Tools)
	}
	trPath := filepath.Join(proj, "work", "sessions", "pilot_style.jsonl")
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow("pilot-style", "pilot-style")
	sess.SetProgressHook(liveHook)
	defer liveClose()
	defer func() {
		if err := saveSessionContext(sess, ctxPath); err != nil {
			r.log.LogWarning(1, "[pilot] 样式会话上下文回写失败:", err)
		}
	}()

	userText := prompts.Render(prompts.StylePilotUser, map[string]string{"CHAPTER": base})
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		return fmt.Errorf("样式修订会话失败: %w", err)
	}
	if !r.remindSubmit(sess, 1, "[pilot]", "pilot-style", "submit_style", func() bool { return submit.Set }) {
		return fmt.Errorf("样式修订会话未提交 submit_style（已提醒两次）")
	}

	// 3) 落盘修订后的样式包 + 示例必须仍编译通过（一次打回机会）。
	var compileErr string
	for attempt := 0; attempt < 2; attempt++ {
		cls := classNameOf(submit.Cls)
		if cls == "" {
			compileErr = "cls 缺少 \\ProvidesClass{...}"
		} else {
			if err := os.WriteFile(filepath.Join(styleDir, cls+".cls"), []byte(submit.Cls), 0o644); err != nil {
				return err
			}
			_ = os.WriteFile(filepath.Join(styleDir, "manual.md"), []byte(submit.Manual), 0o644)
			_ = os.WriteFile(filepath.Join(styleDir, "example.tex"), []byte(submit.Example), 0o644)
			r.writeStyleExtras(styleDir, submit)
			scratch, cleanScratch, err := r.tempDir(proj, "pilot_style")
			if err != nil {
				return err
			}
			copyFile(filepath.Join(styleDir, cls+".cls"), filepath.Join(scratch, cls+".cls"))
			copyFile(filepath.Join(styleDir, "example.tex"), filepath.Join(scratch, "example.tex"))
			copyStyleExtras(styleDir, scratch, submit, cls)
			res := r.comp.Compile(scratch, "example.tex")
			cleanScratch()
			if res.OK {
				compileErr = ""
				break
			}
			compileErr = res.Err
		}
		if attempt == 0 {
			submit.Set = false
			if _, err := sess.Run(session.RunOptions{UserText: "The revised example failed to compile:\n" + truncateStr(compileErr, 8000) +
				"\n\nFix the cls/example and call submit_style again."}); err != nil {
				return fmt.Errorf("样式修订打回续跑失败: %w", err)
			}
			if !r.remindSubmit(sess, 1, "[pilot]", "pilot-style", "submit_style", func() bool { return submit.Set }) {
				return fmt.Errorf("样式修订打回后未提交")
			}
		}
	}
	if compileErr != "" {
		return fmt.Errorf("修订后的 example 编译失败: %s", truncateStr(compileErr, 200))
	}

	// 4) cls/manual 变了 → 作废首章产物，主转换阶段用新样式包重转。
	if styleDigest(proj) != before {
		r.log.Log(1, "[pilot] 样式包已修订，作废首章产物", base, "（主转换阶段将用新样式包重转）")
		_ = os.Remove(filepath.Join(chaptersDir, base+".tex"))
		_ = os.RemoveAll(filepath.Join(chaptersDir, base))
		_ = os.Remove(filepath.Join(proj, "work", "reports", base+".md"))
	} else {
		r.log.Log(1, "[pilot] 样式包无需修订，首章产物保留")
	}
	return nil
}
