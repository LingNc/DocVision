package latex

import (
	"os"
	"path/filepath"
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
) (TikZResult, error) {
	scratch, err := os.MkdirTemp(outDir, "tikzwork-")
	if err != nil {
		return TikZResult{}, err
	}
	state := &tikzState{workDir: scratch}
	engineIsXe := strings.Contains(strings.ToLower(comp.engine), "xe") ||
		strings.Contains(strings.ToLower(comp.engine), "lua")

	sess := session.NewSession(client, modelCfg, tuning, latexFigurePrompt, []session.Tool{
		&CompilePreviewTool{Comp: comp, State: state, EngineIsXe: engineIsXe},
		&SubmitFigureTool{State: state},
		&ImageContextTool{Content: env.MDContent, CurrentImg: env.CurrentImg, MaxUp: env.MaxUp, MaxDown: env.MaxDown},
		&ImageLocateTool{Content: env.MDContent, CurrentImg: env.CurrentImg},
		&ViewImageTool{Root: env.ImagesDir},
	}, log, tid, "tikz")

	initial := strings.Join([]string{
		"Redraw the attached image as TikZ.",
		"",
		"Its surrounding document context (for correct labels/terminology):",
		"```",
		truncateStr(contextText, 4000),
		"```",
		"",
		"Begin: write the TikZ code and call compile_preview.",
	}, "\n")

	_, runErr := sess.Run(session.RunOptions{UserText: initial, Images: []string{imgBase64}})
	result := TikZResult{Rounds: sess.ToolInvoked}
	if runErr != nil {
		result.Reason = "会话错误: " + runErr.Error()
	}
	if !state.submitted {
		if result.Reason == "" {
			result.Reason = "模型未提交最终图形"
			if state.compileErr != "" {
				result.Reason += "（最后编译错误: " + truncateStr(state.compileErr, 300) + "）"
			}
		}
		result.Fallback = true
		// Cleanup scratch on failure.
		os.RemoveAll(scratch)
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
	result.PDFPath = dstPDF
	result.PNGPath = dstPNG
	os.RemoveAll(scratch)
	return result, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
