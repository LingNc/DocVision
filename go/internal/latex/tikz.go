package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
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

	sess := session.NewSession(client, modelCfg, tuning, renderPrompt(latexFigurePrompt, tuning, env.OutputLang), []session.Tool{
		&WriteWorkFileTool{Root: scratch},
		&EditWorkFileTool{Root: scratch},
		&ReadFileTool{Root: scratch},
		&GrepTool{Root: scratch},
		&CompileFigureTool{Comp: comp, State: state, EngineIsXe: engineIsXe, Log: log, Tid: tid},
		// 看图预算（软，提醒不拦截）：tools.view.pdf_max / warn_ratio。
		&ViewPDFTool{Root: scratch, Comp: comp, SoftMax: cfgPdfMax, WarnRatio: cfgWarnRatio},
		&SubmitFigureTool{State: state},
		&ImageContextTool{Content: env.MDContent, CurrentImg: env.CurrentImg, MaxUp: env.MaxUp, MaxDown: env.MaxDown},
		// 原图:每次看图都附带"印刷尺寸/像素/有效 dpi"测量 + 软预算提醒。
		&ViewImageTool{Root: env.ImagesDir, Subject: imageSubject(env.CurrentImg),
			SoftMax: cfgImageMax, WarnRatio: cfgWarnRatio,
			Measure: func() string { return measureHint(env.CurrentImgAbs) }}, // 与自动测量等价，这里是避免重复回溯
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
	initial := strings.Join([]string{
		"Redraw the attached image as TikZ.",
		"",
		ratioLine,
		"",
		"Its surrounding document context (for correct labels/terminology):",
		"```",
		truncateStr(contextText, 4000),
		"```",
		"",
		"Begin: write the TikZ code to figure.tex with write_file, then call compile {path: \"figure.tex\"}.",
	}, "\n")

	// 会话转录（JSONL）：每条消息实时追加，图片以 file:// 媒体引用存储。
	// 若此前运行在同一张图上中断（进程被杀 / 网络断连），恢复历史上下文
	// 继续会话，避免从零重烧 token。
	sessDir := filepath.Join(outDir, "sessions")
	trPath := filepath.Join(sessDir, "vector_"+texBase+".jsonl")
	if msgs, err := session.LoadTranscript(trPath); err != nil {
		log.LogWarning(tid, "[tikz] 转录读取失败（忽略，按全新会话继续）:", err)
	} else if len(msgs) > 0 {
		sess.SetMessages(msgs)
		if tr, err := session.NewTranscript(trPath); err == nil {
			sess.SetTranscript(tr)
			defer tr.Close()
			log.Log(tid, "[tikz] 恢复中断的会话:", filepath.Base(trPath), "(", strconv.Itoa(len(msgs)), "条历史消息 )")
			initial = "The session was interrupted earlier. Continue from where you left off: check your last compile result, fix figure.tex if needed, and call submit once the preview faithfully matches the original image."
		}
	} else if tr, err := session.NewTranscript(trPath); err == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}

	_, runErr := sess.Run(session.RunOptions{UserText: initial, Images: []string{imgBase64}})
	result := TikZResult{Rounds: sess.ToolInvoked}
	if runErr != nil {
		result.Reason = "会话错误: " + runErr.Error()
	}
	// 未提交时提醒一次提交（模型可能漏掉 submit 就停了）。
	if !state.submitted && runErr == nil {
		log.LogWarning(tid, "[tikz] 会话结束但模型未提交，发送提交提醒:")
		if _, err := sess.Run(session.RunOptions{
			UserText: "You have NOT called submit yet. Call submit now with {path: \"figure.tex\"} (the file must match the last successful compile). If no compile succeeded yet, fix figure.tex, call compile {path: \"figure.tex\"}, then submit.",
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
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
