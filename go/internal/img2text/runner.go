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
}

// Run is the top-level entry point. It scans the output directory for
// markdown files, extracts every image reference, dispatches the work
// across N worker goroutines, persists per-item progress, and finally
// rewrites the markdown files with type-based embeds (direct text for
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
	client := NewAIClient(mc, aiOpts)

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
		runWorkers(client, pending, imagesDir, mdCache, progressRoot,
			logger, &progressData, &progressMu, cfg.Options, opts.Quiet)
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
			nc = nc[:r.off.Start] + embedBlockFor(r.item.Result) + nc[r.off.End:]
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
	total := len(pending)

	// Show initial progress immediately before starting any workers.
	if quiet && total > 0 {
		fmt.Fprintf(os.Stdout, "[0/%d] 0.00%% (done: 0, errors: 0, warns: 0)", total)
		os.Stdout.Sync()
	}

	// Writer goroutine: drains results and persists them.
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		count := 0
		doneCount := 0
		errorCount := 0
		warnCount := 0

		progressOut := bufio.NewWriter(os.Stdout)

		for r := range results {
			count++
			if r.result == "__INVALID_RESPONSE__" {
				warnCount++
				logger.LogWarning(0, "Skipped invalid response for",
					r.imgPath+", will retry next run.")
				continue
			}
			parts := strings.SplitN(r.key, "::", 2)
			if len(parts) != 2 {
				logger.LogError(0, "Invalid key format:", r.key)
				continue
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

			if quiet {
				// Print progress with 2-decimal precision on every update.
				if total > 0 {
					pct := float64(doneCount) * 100.0 / float64(total)
					fmt.Fprintf(progressOut, "\r[%d/%d] %.2f%% (done: %d, errors: %d, warns: %d)",
						doneCount, total, pct, doneCount-errorCount, errorCount, warnCount)
					progressOut.Flush()
				}
			}
		}
		if quiet && total > 0 {
			// Final progress line (ensure 100% is printed).
			fmt.Fprintf(progressOut, "\r[%d/%d] 100.00%% (done: %d, errors: %d, warns: %d)\n",
				total, total, doneCount-errorCount, errorCount, warnCount)
			progressOut.Flush()
		}
	}()

	for _, t := range pending {
		wg.Add(1)
		tid := <-tidPool
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
			r, status := ProcessOneImage(
				client, imagesDir, tt.imgPath, subject,
				entry.lines, tt.lineIdx,
				logger, tid, opts,
			)
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
			results <- runResult{key: tt.key,
				result:  r,
				imgPath: tt.imgPath,
				offsets: tt.offsets,
				isError: isErr}
		}(t)
	}

	wg.Wait()
	close(results)
	// close(results) signals the writer goroutine to exit on its next
	// range iteration; wg.Wait above guarantees no new sends are pending.
	writerWG.Wait()
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
//   - mermaid / tikz embed as their code blocks (already validated);
//   - remaining visual types keep a readable [Image]( description ).
func embedBlockFor(result string) string {
	typ, body := splitImgTypePrefix(result)
	body = strings.TrimSpace(body)
	switch {
	case typ == "text" || typ == "latex" || typ == "math" || typ == "formula" ||
		typ == "table" || typ == "code":
		return "\n\n" + body + "\n\n"
	case typ == "mermaid" || typ == "tikz",
		strings.Contains(body, "```mermaid"), strings.Contains(body, "```tikz"):
		// Diagram types whose body IS an already-validated code block.
		return "\n\n" + body + "\n\n"
	default:
		return "\n\n[Image]( " + body + " )\n\n"
	}
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
