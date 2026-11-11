package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// TikZResult is the outcome of one figure-drawing session.
type TikZResult struct {
	Submitted bool
	Code      string
	PDFPath   string // scratch PDF of the confirmed compile
	PNGPath   string // rasterised preview of the confirmed compile
	Rounds    int
	Fallback  bool     // true when the pipeline gave up and keeps the original
	Merges    []string // image paths absorbed into this figure
	Uncertain bool     // submitted code contains % [?] uncertainty marks
	Reason    string   // why it fell back
	// BestOverlap / AcceptedAsIs / AcceptNote：提交时的门槛成绩与"尽力还原"
	// 标记（T307）。AcceptedAsIs 为真表示这一版没达到阈值、是人为/模型选择
	// 接受的，之后不再自动重画。
	BestOverlap  float64
	ScaleDev     float64
	AcceptedAsIs bool
	AcceptNote   string
	// Resumed 为真表示这次没有真的跑会话：转录里最后一次提交本来就**已被收下**、
	// 成品也已在磁盘上，直接收账（T316）。调用方据此保留进度里已有的代码与成绩，
	// 不要用空值把它们覆盖掉。
	Resumed bool
}

// RunTikZSession drives one complete draw→compile→review→submit cycle
// for a single classified-vector image.
func RunTikZSession(
	client *session.Client,
	modelCfg config.ModelConfig,
	tuning config.SessionTuning,
	comp *Compiler,
	imgBase64 string,
	contextText string,
	outDir string,
	dstTex, dstPDF, dstPNG string, // final destinations (absolute)
	env FigureEnv, // document context for cross-page merging
	log *logger.Logger,
	tid int,
	keepTemps, keepRecords bool,
) (TikZResult, error) {
	cfgImageMax := env.ViewImageMax
	cfgPdfMax := env.ViewPDFMax
	cfgWarnRatio := env.ViewWarnRatio
	if cfgWarnRatio <= 0 {
		cfgWarnRatio = 0.7
	}
	// 持久工作区：按镜像命名（不处理完不删除，中断后下次续上）。
	// 成功提交后清理；失败保留供 resume。
	texBase := strings.TrimSuffix(filepath.Base(dstTex), ".tex")
	scratch := filepath.Join(outDir, "sessions", "vector_"+texBase+".work")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		return TikZResult{}, err
	}
	state := &tikzState{workDir: scratch}
	engineIsXe := strings.Contains(strings.ToLower(comp.engine), "xe") ||
		strings.Contains(strings.ToLower(comp.engine), "lua")

	submitFig := &SubmitFigureTool{State: state, MinOverlap: env.MinOverlap, MaxScaleDev: env.MaxScaleDev,
		AcceptMinSubmits: env.AcceptMinSubmits, LiveThreshold: env.LiveThreshold}
	compareFig := &compareFigureTool{State: state, Comp: comp, OriginalPath: env.CurrentImgAbs}
	sess := session.NewSession(client, modelCfg, tuning, renderPrompt(prompts.Must(prompts.FigureSystem), tuning, env.OutputLang), []session.Tool{
		&WriteWorkFileTool{Root: scratch},
		&EditWorkFileTool{Root: scratch},
		&ReadFileTool{Root: scratch},
		&GrepTool{Root: scratch},
		&CompileFigureTool{Comp: comp, State: state, EngineIsXe: engineIsXe, Log: log, Tid: tid},
		// 看图预算（软，提醒不拦截）：tools.view.pdf_max / warn_ratio。
		&ViewPDFTool{Root: scratch, Comp: comp, SoftMax: cfgPdfMax, WarnRatio: cfgWarnRatio},
		compareFig,
		submitFig,
		&ImageContextTool{Content: env.MDContent, CurrentImg: env.CurrentImg, MaxUp: env.MaxUp, MaxDown: env.MaxDown},
		// 原图:每次看图都附带"印刷尺寸/像素/有效 dpi"测量 + 软预算提醒。
		&ViewImageTool{Root: env.ImagesDir, Subject: imageSubject(env.CurrentImg),
			SoftMax: cfgImageMax, WarnRatio: cfgWarnRatio,
			Measure: func() string {
				// 只回测量事实（mm/px/dpi）；"按原尺寸重画"的指令由
				// figure.system.md（要求 21-23）与首轮 ORIGINAL_SIZE 说一次就够，
				// 每次看图都复读一遍是纯冗余（30 次看图 ≈ 450 tokens 噪声）。
				return measureHint(env.CurrentImgAbs)
			}},
	}, log, tid, "tikz")

	// 原图位图尺寸 → 宽高比：模型只看渲染图，无法判断物理大小，
	// 不给参考就会出现"画满画布、比例失真"。
	// 印刷尺寸测量（mm + 有效 dpi）：模型无法从像素判断物理大小，
	// 不给绝对尺度就会把图放大到整页（实测 21% 页宽 → 62%）。
	ratioLine := measureHint(env.CurrentImgAbs)
	if ratioLine == "" {
		if w, h := imageSize(env.CurrentImg); w > 0 && h > 0 {
			ratioLine = fmt.Sprintf("The original bitmap is %dx%d px: aspect ratio %.2f:1 (width:height). Your drawing must keep that aspect ratio and must NOT be blown up to page size.", w, h, float64(w)/float64(h))
		}
	}
	// 看图预算只报数字（一句话）：额度、口径说清即可，工具回执在超限时
	// 会给出用量，不需要在提示词里反复强调"计划着看"。
	viewBudget := fmt.Sprintf("View budget: %d looks at the original image, %d at any one page of your compiled PDF.", cfgImageMax, cfgPdfMax)
	if cfgImageMax <= 0 {
		viewBudget = fmt.Sprintf("View budget: unlimited looks at the original image, %d at any one page of your compiled PDF.", cfgPdfMax)
	}
	initial := prompts.Render(prompts.FigureUser, map[string]string{
		"CONTEXT":       truncateStr(contextText, 4000),
		"ORIGINAL_SIZE": ratioLine,
		"VIEW_BUDGET":   viewBudget,
	})

	// 会话转录（JSONL）：每条消息实时追加，图片以 file:// 媒体引用存储。
	// 若此前运行在同一张图上中断（进程被杀 / 网络断连），恢复历史上下文
	// 继续会话，避免从零重烧 token。T28/T29：轮次/工具数/已用从历史继续；
	// 已提交的重放收账直接完成；历史能自己续（tool 回执收尾）就不再插入
	// 任何 user 内容，原图也不重附。
	sessDir := filepath.Join(outDir, "sessions")
	trPath := filepath.Join(sessDir, "vector_"+texBase+".jsonl")
	resumeSkip := false
	var imgs []string
	if msgs, st, err := session.LoadTranscriptStats(trPath); err != nil {
		log.LogWarning(tid, "[tikz] 转录读取失败（忽略，按全新会话继续）:", err)
	} else if len(msgs) > 0 {
		sess.SetMessages(msgs)
		sess.SeedCounters(st.Rounds, st.Tools)
		if tr, err := session.NewTranscript(trPath); err == nil {
			sess.SetTranscript(tr)
			defer tr.Close()
		}
		resumeLog(log, tid, "tikz", filepath.Base(trPath), msgs, st)
		if replaySubmit(log, tid, "tikz", submitFig, st) {
			state.submitted = true
			resumeSkip = true
		} else if st.SubmitAccepted && env.CreditResume && fileExists(dstTex) && fileExists(dstPDF) {
			// T316：转录里最后一次提交**已经被收下**、成品也还在磁盘上（成品名由
			// md+图片+label 决定，前一次运行已经交付过）。新进程没有 compile/compare
			// 现场，重放提交必然被门槛打回，接着又被"你还没提交"的提醒催一次，模型
			// 只能空转（现场：一轮白烧 6 万 token，最后还把画好的图记成"未提交"）。
			// 这种情况不下场，直接按已交付收账。
			log.Log(tid, "[tikz] 转录里的提交已被收下、成品在位且成绩仍达门槛，直接按已完成收账（不再跑会话）:",
				filepath.Base(trPath))
			return TikZResult{Submitted: true, Resumed: true, PDFPath: dstPDF, PNGPath: dstPNG}, nil
		} else {
			initial = resumeUserText(msgs, "The session was interrupted earlier. Continue from where you left off: check your last compile result, fix figure.tex if needed, run compile, call compare_figure, and submit once the comparison faithfully matches the original image.")
		}
	} else if tr, err := session.NewTranscript(trPath); err == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
		imgs = []string{imgBase64}
	}

	var runErr error
	if !resumeSkip {
		_, runErr = sess.Run(session.RunOptions{UserText: initial, Images: imgs})
	}
	result := TikZResult{Rounds: sess.ToolInvoked}
	if runErr != nil {
		result.Reason = "会话错误: " + runErr.Error()
	}
	// 未提交时提醒一次提交（模型可能漏掉 submit 就停了）。
	if !state.submitted && runErr == nil {
		log.LogWarning(tid, "[tikz] 会话结束但模型未提交，发送提交提醒:")
		if _, err := sess.Run(session.RunOptions{
			// T316：这段以前只提 compile，模型于是直接 submit，撞上"必须先
			// compare_figure"的门槛被打回，有的就此停在被打回那一版（整张图的
			// 成品随之作废）。三步写全、顺序也写明。
			UserText: "You have NOT called submit yet. Do these in order: (1) compile {path: \"figure.tex\"} until it succeeds, (2) call compare_figure and look at its comparison, (3) call submit with {path: \"figure.tex\"} (the file must match the last successful compile). Submitting before compare_figure succeeds is rejected.",
		}); err == nil && state.submitted {
			result.Rounds = sess.ToolInvoked
		}
	}
	if !state.submitted {
		if result.Reason == "" {
			result.Reason = "模型未提交最终图形"
			if state.compileErr != "" {
				result.Reason += "（最后编译错误: " + truncateStr(state.compileErr, 300) + "）"
			}
		}
		result.Fallback = true
		// 失败时保留工作区（中断/重试可续上；状态 done 的图下次进不来）。
		return result, nil
	}

	// 逐图校验（latex.figure_check.enabled，默认关闭）：把刚提交的重画图栅格
	// 化后与原图一起交给视觉模型比对；不一致就把问题打回**同一个作图会话**
	// （上下文还在，它最便宜也最准），最多 CheckRounds 轮，仍不合格则记警告
	// 照常交付——校验是额外的一张网，不是让整本书失败的新理由。
	if env.CheckFigure != nil && state.lastPDF != "" {
		rounds := env.CheckRounds
		if rounds < 1 {
			rounds = 1
		}
		for i := 1; i <= rounds; i++ {
			png := filepath.Join(scratch, fmt.Sprintf("check_%d.png", i))
			if err := comp.Rasterize(state.lastPDF, strings.TrimSuffix(png, ".png")); err != nil {
				log.LogWarning(tid, "[figure-check] 栅格化失败，跳过逐图校验:", err)
				break
			}
			ok, problems := env.CheckFigure(png)
			if ok {
				log.Log(tid, "[figure-check] 逐图校验通过:", filepath.Base(dstPDF))
				break
			}
			codeBefore := state.finalCode
			log.LogWarning(tid, fmt.Sprintf("[figure-check] 第 %d 轮未通过，打回作图会话:", i), truncateStr(problems, 300))
			if i >= rounds {
				log.LogWarning(tid, "[figure-check] 已达校验轮次上限，交付当前版本（问题已记日志）:", filepath.Base(dstPDF))
				break
			}
			if _, err := sess.Run(session.RunOptions{UserText: figureCheckFeedback(problems)}); err != nil {
				log.LogWarning(tid, "[figure-check] 打回会话失败，交付当前版本:", err)
				break
			}
			if state.finalCode == codeBefore {
				log.LogWarning(tid, "[figure-check] 会话未修改图形，停止校验并交付当前版本")
				break
			}
		}
	}

	// T189：续跑重放（resumeSubmit 收账）时本进程一次也没编译过，state 里没有
	// 产物路径。旧代码直接拿空路径去 copy，报出 `open : no such file or directory`
	// ——这条错误与图本身无关，却被记成 fallback，下次运行照样重放、照样失败，
	// 于是同一张图永远画不出来（实测同一 key 跨 5 次运行重复出现）。这里先补一次
	// 编译把 PDF 补出来；真补不出来就报清楚原因，而不是让一个空路径去撞系统错误。
	if state.lastPDF == "" || !fileExists(state.lastPDF) {
		if strings.TrimSpace(state.finalCode) == "" {
			return TikZResult{}, fmt.Errorf("作图会话没有可交付的图形代码（提交记录为空），无法交付 %s", filepath.Base(dstTex))
		}
		texFile := filepath.Join(state.workDir, "standalone.tex")
		if err := os.WriteFile(texFile, []byte(buildStandalone(state.finalCode, engineIsXe)), 0o644); err != nil {
			return TikZResult{}, err
		}
		start := time.Now()
		res := comp.Compile(state.workDir, "standalone.tex")
		LogCompileResult(log, tid, "preview", res, time.Since(start))
		if !res.OK {
			return TikZResult{}, fmt.Errorf("交付前重新编译失败（%s）：%s", filepath.Base(dstTex), truncateStr(res.Err, 300))
		}
		state.lastPDF = res.PDF
	}

	// Persist the confirmed artifacts.
	if err := os.MkdirAll(filepath.Dir(dstTex), 0o755); err != nil {
		return TikZResult{}, err
	}
	if err := os.WriteFile(dstTex, []byte(buildStandalone(state.finalCode, engineIsXe)), 0o644); err != nil {
		return TikZResult{}, err
	}
	if err := copyFile(state.lastPDF, dstPDF); err != nil {
		return TikZResult{}, err
	}
	if err := comp.Rasterize(dstPDF, strings.TrimSuffix(dstPNG, ".png")); err != nil {
		log.LogWarning(tid, "TikZ 最终栅格化失败（PDF 已保留）:", err)
	}
	result.Submitted = true
	result.Code = state.finalCode
	result.Merges = state.merges
	result.Uncertain = state.uncertain
	result.PDFPath = dstPDF
	result.PNGPath = dstPNG
	// 门槛成绩与"尽力还原"标记（T307）：成绩给人看，标记让这张图不再自动重画。
	result.BestOverlap = state.bestOverlap
	result.ScaleDev = state.scaleDevAtBest
	result.AcceptedAsIs = state.acceptedAsIs
	result.AcceptNote = state.acceptNote
	if keepTemps {
		log.Log(0, "[temp] 保留矢量图工作区:", scratch)
	} else {
		os.RemoveAll(scratch)
	}
	tr := filepath.Join(outDir, "sessions", "vector_"+texBase+".jsonl") // 已完成
	if keepRecords {
		log.Log(0, "[session] 保留会话记录:", tr)
	} else {
		os.Remove(tr)
	}
	return result, nil
}

func copyFile(src, dst string) error {
	if strings.TrimSpace(src) == "" {
		// 空路径去 os.Open 只会得到 `open : no such file or directory`，
		// 看不出是哪一步、也看不出目标是谁（T189 的现场就是这个形状）。
		return fmt.Errorf("copyFile: 源路径为空，无法写出 %s", filepath.Base(dst))
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
