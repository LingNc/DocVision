package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// VerifyOptions drives the AI verification pass (默认关闭，verify.enabled).
type VerifyOptions struct {
	// ProgressDir holds the level-2 progress items to verify.
	// Default: <latex.output_dir>/progress_items.
	ProgressDir string
	// ReportPath is the output markdown report.
	ReportPath string
}

// verifyVerdict is the verification model's structured answer.
type verifyVerdict struct {
	OK          bool     `json:"ok"`
	Issues      []string `json:"issues"`
	Suggestions []string `json:"suggestions"`
}

// RunVerify checks each processed image against its embedded content
// and writes a human-readable report. It never modifies the output.
func (r *Runner) RunVerify(opts VerifyOptions) error {
	progDir := opts.ProgressDir
	if progDir == "" {
		progDir = filepath.Join(r.cfg.Paths.LatexOutput, "progress_items")
	}
	items := map[string]*imageProgress{}
	loadProgress(progDir, items)
	if len(items) == 0 {
		return fmt.Errorf("在 %s 未找到可核对的进度记录", progDir)
	}

	client := r.clientFor(r.cfg.Verify.VerifierModel)
	conc := r.cfg.Verify.Concurrency
	if conc <= 0 {
		conc = 2
	}
	tidPool := make(chan int, conc)
	for i := 1; i <= conc; i++ {
		tidPool <- i
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := map[string]*verifyVerdict{}

	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	total := 0
	for _, k := range keys {
		p := items[k]
		if p.Status != "done" {
			continue
		}
		if p.Class == ClassVector && p.FigPDF == "" && p.TikzCode == "" && p.Content == "" {
			continue
		}
		if p.Class == ClassRaster && p.Content == "" {
			continue // bare link, nothing to verify
		}
		total++
		wg.Add(1)
		tid := <-tidPool
		go func(key string, p *imageProgress) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			v := r.verifyOne(client, p, tid)
			mu.Lock()
			results[key] = v
			mu.Unlock()
		}(k, p)
	}
	wg.Wait()

	// Report.
	reportPath := opts.ReportPath
	if reportPath == "" {
		reportPath = filepath.Join(r.cfg.Paths.LatexOutput, r.cfg.Verify.ReportFile)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# AI 核对报告\n\n- 生成时间: %s\n- 核对项: %d\n\n", time.Now().Format("2006-01-02 15:04:05"), total)
	problems := 0
	byMD := map[string][]string{}
	for _, k := range keys {
		v, ok := results[k]
		if !ok {
			continue
		}
		p := items[k]
		line := fmt.Sprintf("- %s — **%s**", p.ImgPath, map[bool]string{true: "通过", false: "发现问题"}[v.OK])
		if v.OK && len(v.Suggestions) == 0 {
			byMD[p.MDName] = append(byMD[p.MDName], line)
			continue
		}
		problems++
		if !v.OK {
			line += "\n  - 问题:\n" + bullets(v.Issues)
		}
		if len(v.Suggestions) > 0 {
			line += "\n  - 修改意见:\n" + bullets(v.Suggestions)
		}
		byMD[p.MDName] = append(byMD[p.MDName], line)
	}
	mds := make([]string, 0, len(byMD))
	for m := range byMD {
		mds = append(mds, m)
	}
	sort.Strings(mds)
	for _, m := range mds {
		fmt.Fprintf(&b, "## %s\n\n", m)
		b.WriteString(strings.Join(byMD[m], "\n"))
		b.WriteString("\n\n")
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(reportPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	r.log.Log(0, "[verify] 核对完成:", strconv.Itoa(total), "项，", strconv.Itoa(problems), "项有问题 ->", reportPath)
	return nil
}

func bullets(list []string) string {
	var out []string
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, "    - "+s)
		}
	}
	return strings.Join(out, "\n")
}

// verifyOne checks a single progress item.
func (r *Runner) verifyOne(client *session.Client, p *imageProgress, tid int) *verifyVerdict {
	dummy := &verifyVerdict{OK: true}
	imgFile, err := resolveImageFile(r.cfg.Paths.ImagesDir, p.ImgPath, subjectOf(p.MDName))
	if err != nil {
		r.log.LogWarning(tid, "[verify] 原图缺失，跳过:", p.ImgPath)
		return dummy
	}
	img64, err := img2textImageToBase64(imgFile)
	if err != nil {
		return dummy
	}

	content := p.Content
	if p.Class == ClassVector && p.TikzCode != "" {
		content = "TikZ code:\n" + p.TikzCode
	}
	if content == "" {
		return dummy
	}

	parts := []map[string]interface{}{
		{"type": "text", "text": "Verify the content below against the original image." +
			"\n\nEmbedded content:\n" + truncateStr(content, 6000)},
		{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + img64}},
	}
	// Vector figures: attach the rendered preview for visual diff.
	if p.Class == ClassVector && p.FigPNG != "" {
		pngPath := filepath.Join(r.cfg.Paths.LatexOutput, p.FigPNG)
		if png64, err := ReadImageFile(pngPath); err == nil {
			parts = append(parts, map[string]interface{}{
				"type": "text", "text": "Rendered preview of the TikZ re-drawing:"},
				map[string]interface{}{
					"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + png64}})
		}
	}
	req := &session.ChatRequest{
		Model: client.Model(),
		Messages: []session.ChatMessage{
			{Role: "system", Content: prompts.Must(prompts.VerifySystem)},
			{Role: "user", Content: parts},
		},
		MaxTokens: 2048, Temperature: 0.0,
		ResponseFormat: map[string]any{"type": "json_object"},
	}
	resp, sentinel, status := client.CallWithRetry(req)
	if status != "" {
		r.log.LogWarning(tid, "[verify] 请求失败:", sentinel)
		return dummy
	}
	obj, err := parseJSONObject(session.ContentString(resp.Choices[0].Message))
	if err != nil {
		r.log.LogWarning(tid, "[verify] 解析失败:", err)
		return dummy
	}
	v := &verifyVerdict{}
	if ok, ok2 := obj["ok"].(bool); ok2 {
		v.OK = ok
	}
	v.Issues = stringSlice(obj["issues"])
	v.Suggestions = stringSlice(obj["suggestions"])
	return v
}

func stringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, it := range arr {
		if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
