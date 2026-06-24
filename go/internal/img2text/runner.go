package img2text

import (
	"bufio"
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
var imageRefRe = regexp.MustCompile(`!\[.*?\]\((images/.+?\.(?:jpg|jpeg|png|gif|webp))\)`)

// RunOptions collects the CLI flags that only affect how the runner
// samples / filters work. Image processing itself is driven by the
// config.Options block.
type RunOptions struct {
	TestMode bool
	Number   int
	Seed     string
	Quiet    bool // when true, suppress verbose output; show only progress percentages
}

// imageTask is the per-image work item collected by Run.
type imageTask struct {
	key     string // "<mdName>::<imgPath>"
	mdName  string
	imgPath string
	lineIdx int
	start   int
	end     int
}

// mdEntry caches the per-file line table for one markdown file.
type mdEntry struct {
	name    string
	content string
	lines   []string
	starts  []int // cumulative line starts, len = len(lines)+1
}

// runResult is what workers push to the writer goroutine.
type runResult struct {
	key     string
	result  string
	start   int
	end     int
	imgPath string
	isError bool // true if the result is an error sentinel
}

// Run is the top-level entry point. It scans the output directory for
// markdown files, extracts every image reference, dispatches the work
// across N worker goroutines, persists per-item progress, and finally
// rewrites the markdown files with `<!-- IMG: ... --> [AI] result` blocks.
//
// The flow matches the Python reference:
//
//  1. Build the AIClient from config and load on-disk progress.
//  2. Scan *.md files, locate every image reference.
//  3. Filter out already-done items using the progress dir.
//  4. In test mode, sample N items with the given seed.
//  5. Run a worker pool that puts results on a channel.
//  6. A single writer goroutine writes per-item JSON progress files.
//  7. Finally rewrite each *.md with the [AI] blocks inserted.
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
	client := NewAIClient(cfg.AI, cfg.Options)

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
	var allTasks []imageTask
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
		logger.Log(0, "  ", name+":", strconv.Itoa(len(matches)), "images")
		for _, m := range matches {
			imgPath := content[m[2]:m[3]]
			il := findLineIndex(starts, m[0])
			allTasks = append(allTasks, imageTask{
				key:     name + "::" + imgPath,
				mdName:  name,
				imgPath: imgPath,
				lineIdx: il,
				start:   m[0],
				end:     m[1],
			})
		}
	}
	logger.Log(0, "Total images available:", strconv.Itoa(len(allTasks)))

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

	// Final pass: rewrite each *.md with the [AI] blocks.
	logger.Log(0, "\nWriting final files...")
	for _, mdf := range mdFiles {
		name := filepath.Base(mdf)
		entry, ok := mdCache[name]
		if !ok {
			continue
		}
		progressMu.Lock()
		var reps []runResult
		for k, v := range progressData {
			if strings.HasPrefix(k, name+"::") {
				reps = append(reps, v)
			}
		}
		progressMu.Unlock()
		// Process replacements right-to-left so byte offsets stay valid.
		sort.Slice(reps, func(i, j int) bool { return reps[i].start > reps[j].start })

		outPath := filepath.Join(finallyDir, name)
		if len(reps) == 0 {
			if !util.FileExists(outPath) {
				if err := os.WriteFile(outPath, []byte(entry.content), 0o644); err != nil {
					logger.LogError(0, "  [", name, "] write failed:", err)
				}
			}
			continue
		}
		nc := entry.content
		for _, r := range reps {
			block := "\n\n<!-- IMG: " + r.imgPath + " -->\n[AI] " +
				r.result + "\n\n<!-- /IMG -->\n\n"
			nc = nc[:r.start] + block + nc[r.end:]
		}
		if err := os.WriteFile(outPath, []byte(nc), 0o644); err != nil {
			logger.LogError(0, "  [", name, "] write failed:", err)
			continue
		}
		logger.Log(0, "  Saved:", name, "("+strconv.Itoa(len(reps))+" replacements)")
	}

	logger.Log(0, strings.Repeat("=", 60))
	logger.Log(0, "Done!")
	return nil
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
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)

	for _, t := range pending {
		wg.Add(1)
		sem <- struct{}{}
		go func(tt imageTask) {
			defer wg.Done()
			defer func() { <-sem }()
			tid := nextWorkerTID(concurrency)
			startTime := time.Now()
			logger.Log(tid, "▶ START", tt.key)
			defer func() {
				if r := recover(); r != nil {
					logger.LogError(tid, "  [panic]", r)
				}
			}()
			entry, ok := mdCache[tt.mdName]
			if !ok {
				results <- runResult{tt.key, "[IMG_WORKER_FATAL: md missing]",
					tt.start, tt.end, tt.imgPath, true}
				return
			}
			r, status := ProcessOneImage(
				client, imagesDir, tt.imgPath,
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
			results <- runResult{tt.key, r, tt.start, tt.end, tt.imgPath, isErr}
		}(t)
	}

	// Writer goroutine: drains results and persists them.
	go func() {
		count := 0
		total := len(pending)
		doneCount := 0
		errorCount := 0
		warnCount := 0

		progressOut := bufio.NewWriter(os.Stdout)

		// Show initial progress immediately so the user sees activity.
		if quiet && total > 0 {
			fmt.Fprintf(progressOut, "[0/%d] 0.00%% (done: 0, errors: 0, warns: 0)", total)
			progressOut.Flush()
		}
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
				Start:   r.start,
				End:     r.end,
				ImgPath: r.imgPath,
			}
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

	wg.Wait()
	close(results)
	// close(results) signals the writer goroutine to exit on its next
	// range iteration; wg.Wait above guarantees no new sends are pending.
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

// tidSeqMu protects tidSeq. Worker thread IDs are surfaced in log
// lines as [T<id>]; we approximate the Python ThreadPoolExecutor-style
// stable IDs with a process-wide atomic that wraps modulo concurrency.
var (
	tidSeqMu sync.Mutex
	tidSeq   int
)

func nextWorkerTID(concurrency int) int {
	tidSeqMu.Lock()
	tidSeq++
	if concurrency > 0 && tidSeq > concurrency {
		tidSeq = 1
	}
	id := tidSeq
	tidSeqMu.Unlock()
	return id
}
