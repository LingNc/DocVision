package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"mineru-tools/internal/session"
)

// checkChapter asks the configured small text model (checker_model,
// defaulting to convert_model) whether the converted .tex faithfully
// represents the chapter markdown: content completeness, correct
// chapter, no dropped or invented sections. Returns (ok, issues).
// Transport/validation problems degrade to ok=true (compile + final
// assembly review remain the hard gates).
func (r *Runner) checkChapter(base, chapPath, texPath string) (bool, string) {
	checkerName := r.cfg.Latex.CheckerModel
	if checkerName == "" {
		checkerName = r.cfg.Latex.ConvertModel
	}
	client := r.clientFor(checkerName)
	modelCfg := r.models[checkerName]
	_ = r.cfg.LatexSession("checker") // tuning reserved for future use

	chapData, err1 := os.ReadFile(chapPath)
	texData, err2 := os.ReadFile(texPath)
	if err1 != nil || err2 != nil {
		return true, ""
	}
	prompt := strings.Join([]string{
		"You are a strict but pragmatic reviewer. Compare the chapter markdown with its converted LaTeX and answer in JSON only.",
		"Check: (1) content completeness (sections/paragraphs preserved, nothing major dropped or invented); (2) correct chapter (the tex corresponds to this markdown); (3) LaTeX structure sane (cls/manual usage is reviewed elsewhere - focus on content).",
		"Format: {\"ok\": true|false, \"issues\": \"...\"}. Keep issues short and actionable; set ok=true when only trivial cosmetic nitpicks remain.",
		"",
		"## Chapter markdown (chapters/" + base + ".md):",
		"```",
		truncateStr(string(chapData), 20000),
		"```",
		"",
		"## Converted LaTeX (" + base + ".tex):",
		"```",
		truncateStr(string(texData), 20000),
		"```",
	}, "\n")

	req := &session.ChatRequest{
		Model: modelCfg.Model,
		Messages: []session.ChatMessage{
			{Role: "user", Content: prompt},
		},
	}
	resp, sentinel, status := client.CallWithRetry(req, 2, 3)
	if resp == nil {
		r.log.LogWarning(0, "[checker]", base, "调用失败，视为通过:", sentinel, status)
		return true, ""
	}
	text := strings.TrimSpace(session.ContentString(resp.Choices[0].Message))
	if i := strings.Index(text, "{"); i >= 0 {
		text = text[i:]
	}
	if j := strings.LastIndex(text, "}"); j >= 0 {
		text = text[:j+1]
	}
	var verdict struct {
		OK     bool   `json:"ok"`
		Issues string `json:"issues"`
	}
	if err := json.Unmarshal([]byte(text), &verdict); err != nil {
		return true, ""
	}
	if verdict.OK {
		return true, ""
	}
	return false, fmt.Sprintf("%v", verdict.Issues)
}
