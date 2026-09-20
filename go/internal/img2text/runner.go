package img2text

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/pkg/util"
)

// imageRefRe is the regex used to find markdown image references. It
// matches `![alt](images/...jpg|jpeg|png|gif|webp)` and is the same
// pattern used by the analyze package, kept here for self-containment.
var imageRefRe = regexp.MustCompile(`(?i)(?:!\[.*?\]\(|<img[^>]*?src=["'])(images/.+?\.(?:jpg|jpeg|png|gif|webp))(?:\)|["'][^>]*>)`)

// RunOptions collects the CLI flags that only affect how the runner
// samples / filters work. Image processing itself is driven by the
// config.Options block.
type RunOptions struct {
	TestMode bool
	Number   int
	Seed     string
	Quiet    bool // when true, suppress verbose output; show only progress percentages
}

// imageTask is one image's work item. A task may cover multiple byte
// ranges inside the same markdown file when the source document
// references the same image more than once; only one AI call is made per
// task and every range gets the same final result.
type imageTask struct {
	key     string // "<mdName>::<imgPath>"
	mdName  string
	imgPath string
	lineIdx int          // line index for the first offset (used by ProcessOneImage context)
	offsets []OffsetPair // all occurrences of this image in mdName, sorted ascending
}

// mdEntry caches the per-file line table for one markdown file.
type mdEntry struct {
	name    string
	content string
	lines   []string
	starts  []int // cumulative line starts, len = len(lines)+1
}

// runResult is what workers push to the writer goroutine. The result
// carries every offset for the task (typically one, but possibly many
// when the same image was referenced more than once in the source
// markdown). Worker writes always replace the first offset's Start/End
// with the legacy fields so older readers still see a sensible value.
type runResult struct {
	key     string
	result  string
	imgPath string
	offsets []OffsetPair
	isError bool // true if the result is an error sentinel
	// rawSnippet is the model's last raw response (truncated) for items
	// discarded as invalid; the writer quotes it so "Skipped invalid
	// response" says WHAT was wrong, not just that something was.
	rawSnippet string
}

// Run is the top-level entry point. It scans the output directory for
// markdown files, extracts every image reference, dispatches the work
// across N worker goroutines, persists per-item progress, and finally
// rewrites the markdown files with type-based embeds (text/table/code
// embedded directly, mermaid as its code block, everything else as a
// readable [Image]( description )).
//
// The flow matches the Python reference:
//
//  1. Build the AIClient from config and load on-disk progress.
//  2. Scan *.md files, locate every image reference.
//  3. Filter out already-done items using the progress dir.
//  4. In test mode, sample N items with the given seed.
//  5. Run a worker pool that puts results on a channel.
//  6. A single writer goroutine writes per-item JSON progress files.
//  7. Finally rewrite each *.md with the embed blocks inserted.
func Run(cfg *config.Config, logger *logger.Logger, opts RunOptions) error {
	if logger == nil {
		return fmt.Errorf("img2text.Run: logger is nil")
	}
	if opts.TestMode && opts.Number <= 0 {
		opts.Number = 10
	}

	imagesDir := cfg.Paths.ImagesDir
	outputDir := cfg.Paths.OutputDir
	finallyDir := cfg.Paths.FinallyDir
	if err := os.MkdirAll(finallyDir, 0o755); err != nil {
		return fmt.Errorf("create finally dir: %w", err)
	}

	progressRoot := filepath.Join(finallyDir, "progress_items")
	if err := os.MkdirAll(progressRoot, 0o755); err != nil {
		return fmt.Errorf("create progress root: %w", err)
	}

	progress := LoadProgress(progressRoot)
	// Pipeline-specific overrides: img2text.model may reference a
	// models: registry name (empty resolves to the top-level ai block
	// with options.* as request-control defaults).
	aiOpts := cfg.Options
	mc, _ := cfg.ResolveModel(cfg.Img2Text.Model)
	if cfg.Img2Text.MaxTokens > 0 {
		aiOpts.MaxTokens = cfg.Img2Text.MaxTokens
	}
	if cfg.Img2Text.Temperature > 0 {
		aiOpts.Temperature = cfg.Img2Text.Temperature
	}
	if cfg.Img2Text.RequestBody != nil {
		mc.RequestBody = cfg.Img2Text.RequestBody
	}
	client := NewAIClient(mc)
	client.SetLogger(logger)

	// P12：升级修复会话配置（就地修复轮用完后启用）。备选模型在加载期解析，
	// 解析失败视同未配置（保持原模型清上下文重来的行为）。
	fixCfg := resolveMermaidFixConfig(cfg, mc, imagesDir)

	// Discover markdown files (sorted, like Python's sorted(...)).
	mdFiles, err := filepath.Glob(filepath.Join(outputDir, "*.md"))
	if err != nil {
		return fmt.Errorf("glob markdown: %w", err)
	}
	sort.Strings(mdFiles)

	modeStr := "FULL BATCH MODE"
	if opts.TestMode {
		modeStr = fmt.Sprintf("TEST MODE (random sample, n=%d)", opts.Number)
	}

	// Always log header to file. In quiet mode, also print clean
	// header to console (without timestamps).
	logger.Log(0, "Found", strconv.Itoa(len(mdFiles)), "file(s) | Model:", client.Model(),
		"| Default ctx: up=", cfg.Options.MaxContextLinesUp,
		"down=", cfg.Options.MaxContextLinesDown)
	logger.Log(0, "Max tool rounds:", cfg.Options.MaxRetries,
		"| Max window: up=", cfg.Options.MaxWindowUp,
		"down=", cfg.Options.MaxWindowDown,
		"| Concurrency:", cfg.Options.Concurrency, "|", modeStr)
	logger.Log(0, strings.Repeat("=", 60))
	if opts.Quiet {
		fmt.Fprintf(os.Stdout, "Found %d file(s) | Model: %s | Default ctx: up= %d down= %d\n",
			len(mdFiles), client.Model(), cfg.Options.MaxContextLinesUp, cfg.Options.MaxContextLinesDown)
		fmt.Fprintf(os.Stdout, "Max tool rounds: %d | Max window: up= %d down= %d | Concurrency: %d | %s\n",
			cfg.Options.MaxRetries, cfg.Options.MaxWindowUp, cfg.Options.MaxWindowDown,
			cfg.Options.Concurrency, modeStr)
	}

	mdCache := map[string]mdEntry{}
	// tasksByKey deduplicates references: when the same image appears
	// more than once inside the same markdown we want a single AI call,
	// but every reference must still be replaced in the final output.
	tasksByKey := map[string]*imageTask{}
	// refCount reports the total number of image references across all
	// md files; used purely for logging "Total images available:".
	refCount := 0
	for _, mdf := range mdFiles {
		data, err := os.ReadFile(mdf)
		if err != nil {
			logger.LogWarning(0, "  [", filepath.Base(mdf), "] read error:", err)
			continue
		}
		content := string(data)
		lines := strings.Split(content, "\n")
		starts := make([]int, len(lines)+1)
		starts[0] = 0
		for i, line := range lines {
			starts[i+1] = starts[i] + len(line) + 1
		}
		name := filepath.Base(mdf)
		mdCache[name] = mdEntry{
			name:    name,
			content: content,
			lines:   lines,
			starts:  starts,
		}
		matches := imageRefRe.FindAllStringSubmatchIndex(content, -1)
		refCount += len(matches)
		logger.Log(0, "  ", name+":", strconv.Itoa(len(matches)), "images")
		for _, m := range matches {
			imgPath := content[m[2]:m[3]]
			off := OffsetPair{Start: m[0], End: m[1]}
			key := name + "::" + imgPath
			if t, ok := tasksByKey[key]; ok {
				t.offsets = append(t.offsets, off)
				continue
			}
			il := findLineIndex(starts, m[0])
			tasksByKey[key] = &imageTask{
				key:     key,
				mdName:  name,
				imgPath: imgPath,
				lineIdx: il,
				offsets: []OffsetPair{off},
			}
		}
	}
	allTasks := make([]imageTask, 0, len(tasksByKey))
	for _, t := range tasksByKey {
		sort.Slice(t.offsets, func(i, j int) bool { return t.offsets[i].Start < t.offsets[j].Start })
		allTasks = append(allTasks, *t)
	}
	logger.Log(0, "Total images available:", strconv.Itoa(refCount),
		"| Unique tasks:", strconv.Itoa(len(allTasks)))

	remaining := make([]imageTask, 0, len(allTasks))
	for _, t := range allTasks {
		if progress[t.key] {
			continue
		}
		remaining = append(remaining, t)
	}

	var pending []imageTask
	if opts.TestMode {
		seed, logSeed := resolveSeed(opts.Seed)
		rng := rand.New(rand.NewSource(seed))
		logger.Log(0, "Test seed:", strconv.FormatInt(logSeed, 10),
			"(use --seed", strconv.FormatInt(logSeed, 10), "to reproduce this run)")
		n := opts.Number
		if n > len(remaining) {
			n = len(remaining)
		}
		if n > 0 {
			idx := rng.Perm(len(remaining))[:n]
			for _, i := range idx {
				pending = append(pending, remaining[i])
			}
		}
		logger.Log(0, "Randomly selected:", strconv.Itoa(n),
			"images (seed=", strconv.FormatInt(logSeed, 10)+")")
	} else {
		pending = remaining
	}
	logger.Log(0, "Already done:", strconv.Itoa(len(allTasks)-len(pending)),
		"| To process:", strconv.Itoa(len(pending)))
	logger.Log(0, strings.Repeat("=", 60))
	if opts.Quiet {
		fmt.Fprintf(os.Stdout, "Already done: %d | To process: %d\n",
			len(allTasks)-len(pending), len(pending))
	}

	progressMu := sync.Mutex{}
	progressData := map[string]runResult{}

	if len(pending) == 0 {
		logger.Log(0, "All done! Clear progress_items/ directory to re-run.")
	} else {
		if opts.Quiet {
			logger.SetQuiet(true)
		}
		// P18：逐图分析转录默认记录（preview.img2text_all，默认 true；
		// 调试完不想堆文件可显式 false）。升级修复会话总是记录、不受此键管。
		recordAll := cfg.Preview.Img2TextAll == nil || *cfg.Preview.Img2TextAll
		if recordAll {
			logger.Log(0, "逐图会话转录写入（preview.img2text_all 可关）",
				filepath.Join(progressRoot, "sessions"))
		}
		runWorkers(client, pending, imagesDir, mdCache, progressRoot,
			logger, &progressData, &progressMu, cfg.Options, opts.Quiet, fixCfg, recordAll)
		if opts.Quiet {
			logger.SetQuiet(false)
		}
	}

	// Final pass: rewrite each *.md with the embed blocks. Merge the
	// history recorded on disk with the work we just produced this run so
	// the rebuild survives "all done, nothing to process" cases (e.g. the
	// finally file was deleted but progress_items is intact).
	historyItems := LoadProgressItems(progressRoot)
	historyByKey := make(map[string]ProgressItem, len(historyItems))
	for _, it := range historyItems {
		historyByKey[it.Key] = it
	}
	progressMu.Lock()
	for k, v := range progressData {
		if !v.isError && strings.Contains(v.result, "[IMG_TYPE:") {
			item := ProgressItem{
				Key:     k,
				Result:  v.result,
				ImgPath: v.imgPath,
			}
			item.SetOffsets(v.offsets)
			historyByKey[k] = item
		}
	}
	progressMu.Unlock()

	logger.Log(0, "\nWriting final files...")
	for _, mdf := range mdFiles {
		name := filepath.Base(mdf)
		entry, ok := mdCache[name]
		if !ok {
			continue
		}

		// Collect every successful item belonging to this markdown. We
		// restrict to the current markdown content so stale offsets from
		// a previous version of the file do not silently corrupt it. We
		// also re-check that the image path at each recorded offset still
		// matches the one this item was generated for; otherwise the
		// cached AI answer is for the wrong image and must be discarded
		// so the current run can reprocess it.
		type rep struct {
			off  OffsetPair
			item ProgressItem
		}
		var reps []rep
		for k, it := range historyByKey {
			parts := strings.SplitN(k, "::", 2)
			if len(parts) != 2 || parts[0] != name {
				continue
			}
			expected := expectedImgPath(it, parts[1])
			if expected == "" {
				// Cannot establish which image this item is for; keep
				// the legacy "offset still matches an image ref" check
				// but skip the path-equality guard. This mirrors the
				// conservative contract requested by the audit.
				expected = ""
			}
			valid := validateOffsetsWithPath(entry.content, it.EffectiveOffsets(), expected)
			for _, o := range valid {
				reps = append(reps, rep{off: o, item: it})
			}
		}

		outPath := filepath.Join(finallyDir, name)
		if len(reps) == 0 {
			if !util.FileExists(outPath) {
				if err := os.WriteFile(outPath, []byte(entry.content), 0o644); err != nil {
					logger.LogError(0, "  [", name, "] write failed:", err)
				}
			}
			continue
		}
		// Retention: keep the PRE-EMBED original of this markdown (with
		// all image refs intact) under progress_items so the replacement
		// is always reversible. Written once per md (first embed round);
		// later rounds never overwrite the archived original.
		origPath := filepath.Join(progressRoot, name, "original.md")
		if !util.FileExists(origPath) {
			if err := os.MkdirAll(filepath.Dir(origPath), 0o755); err != nil {
				logger.LogError(0, "  [", name, "] archive original failed:", err)
			} else if err := os.WriteFile(origPath, []byte(entry.content), 0o644); err != nil {
				logger.LogError(0, "  [", name, "] archive original failed:", err)
			} else {
				logger.Log(0, "  已留存原版（嵌入前）:", origPath)
			}
		}

		// Process replacements right-to-left so byte offsets stay valid.
		sort.Slice(reps, func(i, j int) bool { return reps[i].off.Start > reps[j].off.Start })

		nc := entry.content
		for _, r := range reps {
			nc = nc[:r.off.Start] + embedBlockForRef(r.item.Result, entry.content[r.off.Start:r.off.End]) + nc[r.off.End:]
		}
		// Only write when the rebuilt content differs from what is already
		// on disk. Files whose images were fully processed in earlier rounds
		// rebuild to the identical content, so this round should not touch
		// them (their mtime stays unchanged). Only files with newly
		// processed images actually change and get written.
		written, werr := writeFinalMD(outPath, []byte(nc))
		if werr != nil {
			logger.LogError(0, "  [", name, "] write failed:", werr)
			continue
		}
		if !written {
			logger.Log(0, "  跳过（内容无变化）:", name)
			continue
		}
		logger.Log(0, "  Saved:", name, "("+strconv.Itoa(len(reps))+" replacements)")
	}

	logger.Log(0, strings.Repeat("=", 60))
	logger.Log(0, "Done!")
	return nil
}

// writeFinalMD writes content to outPath unless the file already exists with
// byte-identical content, in which case it is left untouched (mtime preserved)
// and written=false is returned.
func writeFinalMD(outPath string, content []byte) (written bool, err error) {
	if existing, err := os.ReadFile(outPath); err == nil {
		if bytes.Equal(existing, content) {
			return false, nil
		}
	}
	return true, os.WriteFile(outPath, content, 0o644)
}

// runWorkers is the producer/consumer loop. N workers call
// ProcessOneImage and push results into a channel. A single writer
// goroutine drains that channel, writes per-item JSON via
// SaveProgressItem, and updates the in-memory progress map. This matches
// the Python reference's writer_thread + ThreadPoolExecutor split.
func runWorkers(
	client *AIClient,
	pending []imageTask,
	imagesDir string,
	mdCache map[string]mdEntry,
	progressRoot string,
	logger *logger.Logger,
	progressData *map[string]runResult,
	progressMu *sync.Mutex,
	opts config.OptionsConfig,
	quiet bool,
	fixCfg *MermaidFixConfig,
	// recordAll (P18, preview.img2text_all 默认 true) writes one transcript
	// per image task under progressRoot/sessions/<md>/<图>.jsonl.
	recordAll bool,
) {
	results := make(chan runResult, len(pending))
	var wg sync.WaitGroup
	var writerWG sync.WaitGroup
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	// tidPool doubles as the concurrency limiter and the worker-id source:
	// holding a token means owning tid N exclusively for the lifetime of one
	// task, so a thread's START/DONE pairs never interleave in the log.
	tidPool := make(chan int, concurrency)
	for i := 1; i <= concurrency; i++ {
		tidPool <- i
	}
	// 进度只统计**本次运行**：断点续传时"此前已完成"的数已经在
	// "Already done: N | To process: M" 那行里说过一次，再混进进度线会
	// 让续跑一上来就显示 97.93%（用户："按照这次的算，不要按照总的加上
	// done，只看这次的 to process"）。所以 total = 本次待处理数，
	// doneCount 从 0 起。
	total := len(pending)
	var running atomic.Int64

	// Show initial progress immediately before starting any workers.
	if quiet && total > 0 {
		fmt.Fprint(os.Stdout, progressLine(0, total, 0, 0, 0, 0))
		os.Stdout.Sync()
	}

	// Writer goroutine: drains results and persists them.
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		count := 0
		doneCount := 0 // 只数本次处理的
		errorCount := 0
		warnCount := 0

		progressOut := bufio.NewWriter(os.Stdout)

		// T53：running 计数原先只在有结果到达时刷新——2 张图各跑几分钟时
		// 进度行一直停在初始的 running: 0。抽循环体为 handle，主循环改成
		// select：results 到达照常处理，2s 心跳重印当前计数（含 running）。
		reprint := func() {
			if quiet && total > 0 {
				fmt.Fprint(progressOut, "\r"+progressLine(doneCount, total, doneCount-errorCount,
					errorCount, warnCount, int(running.Load())))
				progressOut.Flush()
			}
		}
		heartbeat := time.NewTicker(2 * time.Second)
		defer heartbeat.Stop()

		handle := func(r runResult) {
			count++
			if r.result == "__INVALID_RESPONSE__" {
				// T15：校验未通过（mermaid/格式不符）是**错误**不是警告——警告
				// 只留给可自动纠正的事；这些项仍跳过不落进度、下轮重试。
				errorCount++
				logger.LogError(0, fmt.Sprintf(
					"Skipped invalid response for %s, will retry next run. %s. Raw output: %s",
					r.imgPath, expectedFormatHint, snippet(r.rawSnippet),
				))
				return
			}
			parts := strings.SplitN(r.key, "::", 2)
			if len(parts) != 2 {
				logger.LogError(0, "Invalid key format:", r.key)
				return
			}
			mdName := parts[0]
			imgRel := parts[1]
			item := ProgressItem{
				Key:     r.key,
				Result:  r.result,
				ImgPath: r.imgPath,
			}
			item.SetOffsets(r.offsets)
			if err := SaveProgressItem(progressRoot, mdName, imgRel, item); err != nil {
				logger.LogError(0, "save progress failed:", err)
			}
			progressMu.Lock()
			(*progressData)[r.key] = r
			progressMu.Unlock()

			// Track counts for progress reporting.
			// Only count as error if result contains error sentinels like
			// [IMG_API_ERROR], [IMG_MISSING], etc. — NOT [IMG_TYPE:] which
			// indicates a successful result.
			if r.isError {
				errorCount++
			}
			doneCount++

			// Always log result details to file (logger handles quiet
			// mode: file gets everything, console is suppressed).
			logger.Log(0, strings.Repeat("-", 50))
			logger.Log(0, "["+strconv.Itoa(count)+"/"+strconv.Itoa(total)+"]",
				r.imgPath, "(from", mdName+")")
			preview := r.result
			if len(preview) > 500 {
				preview = preview[:500]
			}
			logger.Log(0, "RESULT:\n"+preview)
			logger.Log(0, strings.Repeat("-", 50))

			reprint()
		}
		for {
			select {
			case <-heartbeat.C:
				reprint()
			case r, open := <-results:
				if !open {
					goto drained
				}
				handle(r)
			}
		}
	drained:
		if quiet && total > 0 {
			// Final progress line (ensure 100% is printed).
			fmt.Fprint(progressOut, "\r"+progressLine(total, total, doneCount-errorCount,
				errorCount, warnCount, 0)+"\n")
			progressOut.Flush()
		}
	}()

	for _, t := range pending {
		wg.Add(1)
		tid := <-tidPool
		running.Add(1)
		go func(tt imageTask) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			startTime := time.Now()
			logger.Log(tid, "▶ START", tt.key)
			defer func() {
				if r := recover(); r != nil {
					logger.LogError(tid, "  [panic]", r)
				}
			}()
			entry, ok := mdCache[tt.mdName]
			if !ok {
				results <- runResult{key: tt.key,
					result:  "[IMG_WORKER_FATAL: md missing]",
					imgPath: tt.imgPath,
					offsets: tt.offsets,
					isError: true}
				return
			}
			subject := strings.TrimSuffix(tt.mdName, filepath.Ext(tt.mdName))
			// T37 debug：preview.img2text_all 时给本任务挂一张逐图转录。
			// client 是共享指针，这里拷一份结构体、只换 record——http
			// 客户端与配置都是只读共享，逐任务互不影响。
			taskClient := client
			var rec *ExchangeRecorder
			if recordAll {
				tpath := transcriptPathFor(progressRoot, tt.mdName, tt.imgPath)
				// 与 mermaid_fix 同款轮转：上次运行的转录改名 prevN——
				// NewTranscript 是 append 模式，不轮转会让多次运行混在
				// 一个文件里，"一张图的历次调用历史"也无从看起（T56 追问）。
				rotateTranscript(tpath)
				if r, err := NewExchangeRecorder(tpath, tt.key, client.Model(), ""); err != nil {
					logger.LogWarning(tid, "  [recorder] 转录创建失败:", err)
				} else {
					rec = r
					c := *client
					c.SetRecorder(rec.Record)
					taskClient = &c
				}
			}
			r, status, raw := ProcessOneImage(
				taskClient, imagesDir, tt.imgPath, subject,
				entry.lines, tt.lineIdx,
				logger, tid, opts, fixCfg,
			)
			if rec != nil {
				_ = rec.Close()
			}
			elapsed := time.Since(startTime).Seconds()
			elapsedStr := strconv.FormatFloat(elapsed, 'f', 2, 64)
			isErr := false
			if status == StatusOK {
				logger.Log(tid, "✓", "["+elapsedStr+"s]", "DONE")
			} else {
				preview := r
				if len(preview) > 200 {
					preview = preview[:200]
				}
				logger.LogError(tid, "✗", "["+elapsedStr+"s]", "FAILED", preview)
				isErr = true
				if status == StatusRetry {
					r = "__INVALID_RESPONSE__"
				}
			}
			running.Add(-1) // 计数先落，writer 渲染时 running 已准确
			results <- runResult{key: tt.key,
				result:     r,
				imgPath:    tt.imgPath,
				offsets:    tt.offsets,
				isError:    isErr,
				rawSnippet: snippet(raw)}
		}(t)
	}

	wg.Wait()
	close(results)
	// close(results) signals the writer goroutine to exit on its next
	// range iteration; wg.Wait above guarantees no new sends are pending.
	writerWG.Wait()
}

// progressLine renders the ONE compact progress line of the img2text
// run. Every number is about THIS RUN only (processed, total = to process,
// successes, errors, warns, running): the resumed baseline is reported
// once on the "Already done" line instead — mixing it in made a resumed
// run open at e.g. [8874/9062] 97.93% while only 396 images were left.
func progressLine(processed, total, ok, errors, warns, running int) string {
	pct := 0.0
	if total > 0 {
		pct = float64(processed) * 100.0 / float64(total)
	}
	if ok < 0 {
		ok = 0
	}
	return fmt.Sprintf("[%d/%d] %.2f%% (done: %d, errors: %d, warns: %d, running: %d)",
		processed, total, pct, ok, errors, warns, running)
}

// expectedImgPath returns the image path that a historical record was
// generated for. ImgPath is preferred because it is the authoritative
// field written by SaveProgressItem; the key suffix is used as a fallback
// for legacy JSON that omits img_path. When neither source yields a
// usable value, expectedImgPath returns "" so the caller can take the
// conservative "skip path-equality guard" branch.
func expectedImgPath(it ProgressItem, keySuffix string) string {
	if it.ImgPath != "" {
		return it.ImgPath
	}
	return keySuffix
}

// imagePathAtOffset extracts the image path captured by imageRefRe at the
// given [start, end) range. It returns "" if the range is out of bounds
// or the substring there is not a complete image reference (we want to
// keep the same single-pass behaviour as validateOffsets, so the path
// check piggybacks on the regex match).
func imagePathAtOffset(content string, off OffsetPair) string {
	if off.Start < 0 || off.End > len(content) || off.Start >= off.End {
		return ""
	}
	m := imageRefRe.FindStringSubmatchIndex(content[off.Start:off.End])
	if m == nil {
		return ""
	}
	// m[2], m[3] are relative to the substring; translate back to content.
	return content[off.Start+m[2] : off.Start+m[3]]
}

// validateOffsets returns the subset of offsets that still correspond
// to image references inside content. An offset is considered valid when
// the byte range is in bounds AND the substring there matches the
// shared image reference pattern (i.e. it is still an
// ![...](images/...) markdown image). Offsets that fail this check are
// silently dropped so a markdown edit that shifted or removed an image
// reference cannot corrupt the rebuilt file.
func validateOffsets(content string, offsets []OffsetPair) []OffsetPair {
	if len(offsets) == 0 {
		return nil
	}
	out := make([]OffsetPair, 0, len(offsets))
	for _, o := range offsets {
		if o.Start < 0 || o.End > len(content) || o.Start >= o.End {
			continue
		}
		sub := content[o.Start:o.End]
		// Accept both markdown ![...](...) image links and self-contained
		// HTML <img ...> references. They close differently (")" vs ">").
		isMarkdown := strings.HasPrefix(sub, "![") && strings.HasSuffix(sub, ")")
		isHTML := strings.HasPrefix(sub, "<img") && strings.HasSuffix(sub, ">")
		if !isMarkdown && !isHTML {
			continue
		}
		// Verify the substring really is an image reference; this catches
		// the case where the markdown has been edited at that byte range
		// (token mismatch) so we cannot safely replace it.
		if imageRefRe.FindStringIndex(sub) == nil {
			continue
		}
		out = append(out, o)
	}
	return out
}

// validateOffsetsWithPath is validateOffsets plus an extra guard: each
// surviving offset must reference the expectedPath image. When
// expectedPath is "" the path check is skipped (conservative behaviour
// for legacy records we cannot attribute to any specific image).
//
// This catches the case where a user renamed images/a.jpg to
// images/b.jpg at the same byte range; the offset would otherwise still
// pass the regex check and silently rewrite unrelated content with the
// cached AI answer for the original image.
func validateOffsetsWithPath(content string, offsets []OffsetPair, expectedPath string) []OffsetPair {
	if len(offsets) == 0 {
		return nil
	}
	base := validateOffsets(content, offsets)
	if expectedPath == "" {
		return base
	}
	out := make([]OffsetPair, 0, len(base))
	for _, o := range base {
		if imagePathAtOffset(content, o) != expectedPath {
			continue
		}
		out = append(out, o)
	}
	return out
}

// findLineIndex returns the line index (0-based) that contains the byte
// offset pos, using the cumulative-starts table from mdCache.
func findLineIndex(starts []int, pos int) int {
	for i := 0; i < len(starts)-1; i++ {
		if starts[i] <= pos && pos < starts[i+1] {
			return i
		}
	}
	// Fallback: very last line if pos is at end-of-file.
	return len(starts) - 1
}

// resolveSeed returns (seed, logSeed). When opts.Seed is empty we use
// the default fixed seed 42; when it is "random" we pick a random
// non-negative int64; otherwise we parse it as an integer.
func resolveSeed(seedStr string) (int64, int64) {
	if seedStr == "" {
		return 42, 42
	}
	if strings.EqualFold(seedStr, "random") {
		s := time.Now().UnixNano()
		if s < 0 {
			s = -s
		}
		return s, s
	}
	n, err := strconv.ParseInt(seedStr, 10, 64)
	if err != nil {
		return 42, 42
	}
	return n, n
}

// embedBlockFor converts an AI result (with its [IMG_TYPE:] prefix)
// into the final markdown embed, chosen by content type:
//   - non-image content (pure text, LaTeX math, tables, code) embeds
//     DIRECTLY so downstream AI readers get searchable text;
//   - mermaid embeds as its (already validated) code block;
//   - remaining visual types keep a readable [Image]( description ).
func embedBlockFor(result string) string {
	return embedBlockForRef(result, "")
}

// embedBlockForRef 与 embedBlockFor 相同，mermaid 分支额外用原始图片引用里
// 的 alt 文本当 [Image] 前缀的描述（T16）：mermaid 也是视觉内容的一种，最终
// 文件里先有一个统一的 `[Image]( 描述 )` 锚，后面才跟 ```mermaid 代码块；
// 模型偶尔吞掉结尾围栏，嵌入前补齐，避免破坏后续 markdown。
func embedBlockForRef(result, ref string) string {
	typ, body := splitImgTypePrefix(result)
	body = strings.TrimSpace(body)
	switch {
	case typ == "text" || typ == "latex" || typ == "math" || typ == "formula" ||
		typ == "table" || typ == "code":
		return "\n\n" + body + "\n\n"
	case typ == "mermaid", strings.Contains(body, "```mermaid"):
		label := altOfRef(ref)
		if label == "" {
			label = "mermaid"
		}
		if !strings.HasPrefix(body, "```") {
			// A bare diagram body (legacy progress data) gets the fence.
			return "\n\n[Image]( " + label + " )\n\n```mermaid\n" + body + "\n```\n\n"
		}
		return "\n\n[Image]( " + label + " )\n\n" + ensureClosedFence(body) + "\n\n"
	default:
		return "\n\n[Image]( " + body + " )\n\n"
	}
}

// htmlAltRe extracts the alt attribute of an <img> reference.
var htmlAltRe = regexp.MustCompile(`(?i)\balt=["']([^"']*)["']`)

// fenceLineRe counts markdown fence lines (``` or ```lang at line start).
var fenceLineRe = regexp.MustCompile("(?m)^\\s*" + "```" + "(?:[a-zA-Z0-9_-]+)?\\s*$")

// altOfRef pulls a human-readable alt from the original image reference:
// markdown `![alt](…)` → alt; `<img … alt="…">` → the attribute; else "".
func altOfRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "![") {
		if i := strings.Index(ref, "]("); i > 2 {
			return strings.TrimSpace(ref[2:i])
		}
		return ""
	}
	if m := htmlAltRe.FindStringSubmatch(ref); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// ensureClosedFence appends a closing fence when the body has an odd number
// of fence lines (T16: the model sometimes swallows the trailing ```).
func ensureClosedFence(body string) string {
	if len(fenceLineRe.FindAllString(body, -1))%2 == 1 {
		body += "\n```"
	}
	return body
}

// Missing or malformed prefixes return ("", whole input) and the
// caller falls back to the [Image] embed.
func splitImgTypePrefix(result string) (string, string) {
	idx := strings.Index(result, "[IMG_TYPE:")
	if idx < 0 {
		return "", result
	}
	rest := result[idx+len("[IMG_TYPE:"):]
	end := strings.Index(rest, "]")
	if end < 0 {
		return "", result
	}
	typ := strings.ToLower(strings.TrimSpace(rest[:end]))
	body := rest[end+1:]
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	return typ, body
}

// resolveMermaidFixConfig 从 tools.mermaid.session_* 组装升级修复会话配置。
// 返回 nil 表示功能关闭（session_rounds ≤ 0）。工作区根 = <finally>/progress_items/
// mermaid_fix/，具体到图的工作区由调用方按图 Key 懒创建（并发下每图独立、无共享；
// submit.md / compile_error.log / 转录都留在里面，出错现场可诊断、下轮可续用）。
func resolveMermaidFixConfig(cfg *config.Config, mc config.ModelConfig, imagesDir string) *MermaidFixConfig {
	rounds := resolveMermaidSessionRounds(cfg.Tools.Mermaid.SessionRounds)
	if rounds <= 0 {
		return nil
	}
	fix := &MermaidFixConfig{
		Rounds:        rounds,
		ErrorLimit:    resolveMermaidSessionErrors(cfg.Tools.Mermaid.SessionErrors),
		FallbackModel: strings.TrimSpace(cfg.Tools.Mermaid.FallbackModel),
		Command:       cfg.Tools.Mermaid.Command,
		Timeout:       time.Duration(cfg.Tools.Mermaid.Timeout) * time.Second,
		Primary:       mc,
		ImagesDir:     imagesDir,
		WorkspaceRoot: filepath.Join(cfg.Paths.FinallyDir, "progress_items", "mermaid_fix"),
	}
	if fix.Timeout <= 0 {
		fix.Timeout = 30 * time.Second
	}
	if fix.FallbackModel != "" {
		fb, ok := cfg.ResolveModel(fix.FallbackModel)
		if ok {
			fix.Fallback = fb
		} else {
			// 备选条目不存在：退回原模型清上下文重来（FallbackModel 清空）。
			fix.FallbackModel = ""
		}
	}
	return fix
}
