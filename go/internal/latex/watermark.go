package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mineru-tools/internal/img2text"
	"mineru-tools/internal/session"
)

// WatermarkMemory is the small cross-phase working memory for watermark
// removal. Watermarks are usually only discoverable on the FULL page
// overview: by the time a fragment is processed the watermark may have
// become plain text, or it may have been cropped into a small image
// that repeats on many pages. So we scan ONCE at the very start of the
// flow (page renders + markdown statistics), persist the findings and
// inject them into every downstream AI session.
type WatermarkMemory struct {
	Detected     bool     `json:"detected"`
	TextPatterns []string `json:"text_patterns"` // exact watermark strings as they appear
	ImageRefs    []string `json:"image_refs"`    // markdown refs of cropped watermark/ad images
	Notes        string   `json:"notes"`         // where/how they appear
}

// Block renders the memory as a prompt injection fragment; empty when
// nothing was detected.
func (m *WatermarkMemory) Block() string {
	if m == nil || !m.Detected {
		return ""
	}
	var b strings.Builder
	b.WriteString("WATERMARK MEMORY (from the full-document pre-scan; it applies to EVERY fragment you see — a single fragment alone may not reveal the pattern):\n")
	if len(m.TextPatterns) > 0 {
		b.WriteString("- Watermark TEXT patterns — do NOT transcribe/typeset/describe them, drop such lines entirely:")
		for _, p := range m.TextPatterns {
			fmt.Fprintf(&b, "\n  - %s", p)
		}
		b.WriteString("\n")
	}
	if len(m.ImageRefs) > 0 {
		b.WriteString("- Watermark/advertisement IMAGE refs (already removed from the document; never extract or describe them):")
		for _, p := range m.ImageRefs {
			fmt.Fprintf(&b, "\n  - %s", p)
		}
		b.WriteString("\n")
	}
	if m.Notes != "" {
		b.WriteString("- Pattern notes: " + m.Notes + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

const watermarkDetectSystemPrompt = `You detect WATERMARK / ADVERTISEMENT artifacts in a parsed document (MinerU output).

You receive: sample page renders (the TRUE visual appearance) and the parsed markdown with statistics of recurring image refs. Watermarks appear as: repeated institution/library/platform names or decorative strings, faint background overlays, page-footer slogans, and small cropped images that recur on many pages (QR codes, \"follow us\" banners, logos) — the same watermark crop often becomes MANY separate image refs, one per page.

Respond with ONLY a JSON object:
{"detected": true|false, "text_patterns": ["exact watermark strings as they appear in the text"], "image_refs": ["images/... refs that are watermark/ad crops"], "notes": "one short sentence: where/how they appear"}

Rules:
- image_refs: only refs whose repetition pattern or visual appearance marks them as watermark/ad crops. NEVER include real content figures, diagrams or photos.
- text_patterns: exact substrings usable for literal matching (keep original wording; include variants if the text differs between pages).
- If nothing watermark-like exists: {"detected": false, "text_patterns": [], "image_refs": [], "notes": ""}`

// watermarkGuidance returns the working-memory block, falling back to
// the given static text when nothing was detected (or the scan failed).
func (r *Runner) watermarkGuidance(fallback string) string {
	if b := r.wm.Block(); b != "" {
		return b
	}
	return fallback
}

// watermarkSample is one markdown document offered to the detector.
type watermarkSample struct {
	name    string
	content string
}

// detectWatermarkPhase runs the one-shot watermark pre-scan (or loads a
// cached result) and stores it on the Runner for the rest of the flow.
func (r *Runner) detectWatermarkPhase(samples []watermarkSample) {
	cachePath := filepath.Join(r.cfg.Paths.LatexProject, "watermark_memory.json")
	if data, err := os.ReadFile(cachePath); err == nil {
		var wm WatermarkMemory
		if json.Unmarshal(data, &wm) == nil {
			r.wm = &wm
			r.log.Log(0, "[watermark] 已载入水印工作记忆:", cachePath)
			return
		}
	}

	r.log.Log(0, "[watermark] 检测水印模式（全览页 + markdown 统计）...")
	wm := r.runWatermarkDetection(samples)
	r.wm = wm
	if data, err := json.MarshalIndent(wm, "", "  "); err == nil {
		if err := os.WriteFile(cachePath, data, 0o644); err != nil {
			r.log.LogWarning(0, "[watermark] 记忆写入失败:", err)
		}
	}
	if wm.Detected {
		r.log.Log(0, fmt.Sprintf("[watermark] 检测到水印: 文本模式 %d 个, 水印图片 %d 张 （记忆已缓存至 %s）",
			len(wm.TextPatterns), len(wm.ImageRefs), cachePath))
	} else {
		r.log.Log(0, "[watermark] 未检测到明显水印")
	}
}

func (r *Runner) runWatermarkDetection(samples []watermarkSample) *WatermarkMemory {
	wm := &WatermarkMemory{}
	client := r.clientFor(r.cfg.Latex.ClassifierModel)
	modelCfg := r.models[r.cfg.Latex.ClassifierModel]

	// 1) Recurring image refs across ALL documents — a watermark crop
	// repeats far more often than any content figure.
	type stat struct {
		count int
		mds   int
	}
	counts := map[string]*stat{}
	for _, s := range samples {
		seen := map[string]bool{}
		for _, m := range imageRefRe.FindAllStringSubmatch(s.content, -1) {
			ref := m[1]
			if counts[ref] == nil {
				counts[ref] = &stat{}
			}
			counts[ref].count++
			if !seen[ref] {
				seen[ref] = true
				counts[ref].mds++
			}
		}
	}
	var tops []string
	for ref, st := range counts {
		if st.count >= 3 && st.mds >= 2 {
			tops = append(tops, ref)
		}
	}
	sort.Slice(tops, func(i, j int) bool { return counts[tops[i]].count > counts[tops[j]].count })
	if len(tops) > 8 {
		tops = tops[:8]
	}

	// 2) Text samples (front of a few documents).
	sort.Slice(samples, func(i, j int) bool { return samples[i].name < samples[j].name })
	var textParts []string
	for i, s := range samples {
		if i >= 3 {
			break
		}
		textParts = append(textParts, fmt.Sprintf("--- %s (first 1200 chars) ---\n%s", s.name, truncateStr(s.content, 1200)))
	}

	// 3) Page renders: first / middle (true visual overview).
	var userMsg strings.Builder
	fmt.Fprintf(&userMsg, "Recurring image refs (count across documents):\n")
	if len(tops) == 0 {
		userMsg.WriteString("(none recurring)")
	} else {
		for _, ref := range tops {
			fmt.Fprintf(&userMsg, "- %s (x%d)\n", ref, counts[ref].count)
		}
	}
	userMsg.WriteString("\nMarkdown samples:\n")
	for _, p := range textParts {
		userMsg.WriteString(p + "\n")
	}

	var imgs []string
	subject := ""
	if len(samples) > 0 {
		subject = subjectOf(samples[0].name)
	}
	if idx, err := buildPageIndex(r.cfg.Paths.MineruOutput, subject); err == nil {
		pagesDir := filepath.Join(r.cfg.Paths.LatexProject, "pages")
		mid := idx.total / 2
		if mid < 1 {
			mid = 1
		}
		for _, pg := range []int{1, mid, idx.total} {
			if pg < 1 || pg > idx.total {
				continue
			}
			path, err := renderSourcePage(r.comp, idx, pagesDir, pg)
			if err != nil {
				continue
			}
			if b64, err := img2text.ImageToBase64(path, 1280); err == nil {
				imgs = append(imgs, b64)
			}
		}
		if len(imgs) > 0 {
			fmt.Fprintf(&userMsg, "\n(%d full page render(s) attached — they show the true visual appearance)", len(imgs))
		}
	}

	userContent := []map[string]interface{}{
		{"type": "text", "text": userMsg.String()},
	}
	for _, b64 := range imgs {
		userContent = append(userContent, map[string]interface{}{
			"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + b64},
		})
	}
	req := &session.ChatRequest{
		Model: modelCfg.Model,
		Messages: []session.ChatMessage{
			{Role: "system", Content: watermarkDetectSystemPrompt},
			{Role: "user", Content: userContent},
		},
		MaxTokens:      1024,
		Temperature:    0.0,
		ResponseFormat: map[string]any{"type": "json_object"},
	}
	resp, sentinel, status := client.CallWithRetry(req)
	if status != "" || resp == nil || len(resp.Choices) == 0 {
		r.log.LogWarning(0, "[watermark] 检测请求失败（本次流程不注入水印记忆）:", sentinel)
		return wm
	}
	obj, err := parseJSONObject(fmt.Sprintf("%v", resp.Choices[0].Message.Content))
	if err != nil {
		r.log.LogWarning(0, "[watermark] 检测结果解析失败:", err)
		return wm
	}
	wm.Detected, _ = obj["detected"].(bool)
	wm.TextPatterns = stringSlice(obj["text_patterns"])
	wm.ImageRefs = stringSlice(obj["image_refs"])
	wm.Notes, _ = obj["notes"].(string)
	if !wm.Detected || (len(wm.TextPatterns) == 0 && len(wm.ImageRefs) == 0) {
		wm.Detected = false
	}
	return wm
}
