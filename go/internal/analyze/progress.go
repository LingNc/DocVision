package analyze

import (
	"bufio"
	"encoding/json"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"mineru-tools/internal/logfind"
)

// imageMDRef matches markdown image references to image files.
var imageMDRef = regexp.MustCompile(`!\[.*?\]\((images/.+?\.(?:jpg|jpeg|png|gif|webp))\)`)

// imageLineRef matches "[N/M] path/to/img.jpg" in log lines.
var imageLineRef = regexp.MustCompile(`\[\d+/\d+\]\s+(\S+\.(?:jpg|jpeg|png|gif|webp))`)

// CountImagesInMDFiles counts `![...](images/...)` references across all .md
// files in inputDir (non-recursive).
func CountImagesInMDFiles(inputDir string) int {
	matches, _ := filepath.Glob(filepath.Join(inputDir, "*.md"))
	total := 0
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		total += len(imageMDRef.FindAllString(string(data), -1))
	}
	return total
}

// CountImagesPerMD returns a map of md filename to image count.
func CountImagesPerMD(inputDir string) map[string]int {
	result := map[string]int{}
	matches, _ := filepath.Glob(filepath.Join(inputDir, "*.md"))
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		result[filepath.Base(f)] = len(imageMDRef.FindAllString(string(data), -1))
	}
	return result
}

// CheckProgressItems scans every JSON file under progressRoot. A file counts
// as "completed" when its "result" field contains "[IMG_TYPE:" and is not the
// sentinel "__INVALID_RESPONSE__". Returns completed, invalid counts.
// progressStatsCacheFile is the incremental verdict cache kept inside
// progressRoot. Keyed by relative path with (mtime,size) as the invalidator:
// unchanged files reuse the cached verdict, only new/changed files are read
// (T55 追问：samba 盘上每次全量读一万多个 json 太慢——现在稳定状态下
// analyze 只做 stat 级索引）。
const progressStatsCacheFile = ".progress_stats.json"

type progressStatsCache struct {
	Files map[string]progressStatsEntry `json:"files"`
}

type progressStatsEntry struct {
	Mtime int64 `json:"m"`
	Size  int64 `json:"s"`
	Done  bool  `json:"d"`
}

func CheckProgressItems(progressRoot string) (completed, invalid int) {
	if _, err := os.Stat(progressRoot); err != nil {
		return 0, 0
	}
	matches, _ := filepath.Glob(filepath.Join(progressRoot, "**", "*.json"))
	if len(matches) == 0 {
		return 0, 0
	}

	// 载入旧缓存（没有/损坏都当作空）。
	cache := progressStatsCache{Files: map[string]progressStatsEntry{}}
	if data, err := os.ReadFile(filepath.Join(progressRoot, progressStatsCacheFile)); err == nil {
		_ = json.Unmarshal(data, &cache)
		if cache.Files == nil {
			cache.Files = map[string]progressStatsEntry{}
		}
	}

	type job struct {
		idx  int
		path string
		rel  string
	}
	jobs := make([]job, 0, 256)
	results := make([]bool, len(matches))
	seen := make([]bool, len(matches)) // 有定论（缓存或新读）
	for i, f := range matches {
		if filepath.Base(f) == progressStatsCacheFile {
			continue
		}
		st, err := os.Stat(f)
		if err != nil {
			continue // invalid，不算完成
		}
		rel, rerr := filepath.Rel(progressRoot, f)
		if rerr != nil {
			rel = f
		}
		if e, ok := cache.Files[rel]; ok && e.Mtime == st.ModTime().Unix() && e.Size == st.Size() {
			results[i] = e.Done
			seen[i] = true
			continue
		}
		jobs = append(jobs, job{i, f, rel})
	}

	// 只读变化的文件（首次/缓存缺失时=全部，之后≈0）。并发 64：samba
	// 延迟高，小文件多读并发收益大。
	var wg sync.WaitGroup
	sem := make(chan struct{}, 64)
	newVerdicts := make(map[string]progressStatsEntry, len(jobs))
	var mu sync.Mutex
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			data, err := os.ReadFile(j.path)
			done := err == nil && bytes.Contains(data, []byte("[IMG_TYPE:")) && !bytes.Contains(data, []byte("__INVALID_RESPONSE__"))
			results[j.idx] = done
			seen[j.idx] = true
			st, _ := os.Stat(j.path)
			if st != nil {
				mu.Lock()
				newVerdicts[j.rel] = progressStatsEntry{Mtime: st.ModTime().Unix(), Size: st.Size(), Done: done}
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()

	// 合并回缓存并落盘（失败静默——只影响下次速度）。
	for rel, e := range newVerdicts {
		cache.Files[rel] = e
	}
	// 清掉已消失文件的旧条目，防缓存无限胀大。
	live := make(map[string]bool, len(matches))
	for _, f := range matches {
		if rel, rerr := filepath.Rel(progressRoot, f); rerr == nil {
			live[rel] = true
		}
	}
	for rel := range cache.Files {
		if !live[rel] {
			delete(cache.Files, rel)
		}
	}
	if data, err := json.Marshal(cache); err == nil {
		tmp := filepath.Join(progressRoot, progressStatsCacheFile+".tmp")
		if os.WriteFile(tmp, data, 0o644) == nil {
			_ = os.Rename(tmp, filepath.Join(progressRoot, progressStatsCacheFile))
		}
	}

	for i := range matches {
		switch {
		case seen[i] && results[i]:
			completed++
		default:
			invalid++
		}
	}
	return completed, invalid
}

// GetProblematicImages returns image paths from logPath that are followed by
// at least one [ERROR] line before the next image line. [WARNING] does NOT
// count (T55): warnings are corrected successes (T36 — the output deviated
// but the program fixed it and the result was written), so warning images
// are 良品, not problems.
func GetProblematicImages(logPath string) []string {
	errImgs, _ := ScanLogIssueImages(logPath)
	return errImgs
}

// ScanLogIssueImages splits the per-image issue scan into two sets: images
// followed by ≥1 [ERROR] line (真问题) and images followed by [WARNING] only
// (自纠正成功，单独统计供"警告数"展示). Images in both classes land in the
// error set only.
func ScanLogIssueImages(logPath string) (errImgs, warnImgs []string) {
	errSet := map[string]struct{}{}
	warnSet := map[string]struct{}{}
	f, err := os.Open(logPath)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i := 0; i < len(lines); i++ {
		m := imageLineRef.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		current := m[1]
		hasErr, hasWarn := false, false
		j := i + 1
		for j < len(lines) && imageLineRef.FindStringSubmatch(lines[j]) == nil {
			if strings.Contains(lines[j], "[ERROR]") {
				hasErr = true
			} else if strings.Contains(lines[j], "[WARNING]") {
				hasWarn = true
			}
			j++
		}
		if hasErr {
			errSet[current] = struct{}{}
			delete(warnSet, current)
		} else if hasWarn {
			if _, bad := errSet[current]; !bad {
				warnSet[current] = struct{}{}
			}
		}
		i = j - 1
	}

	errImgs = make([]string, 0, len(errSet))
	for k := range errSet {
		errImgs = append(errImgs, k)
	}
	sort.Strings(errImgs)
	warnImgs = make([]string, 0, len(warnSet))
	for k := range warnSet {
		warnImgs = append(warnImgs, k)
	}
	sort.Strings(warnImgs)
	return errImgs, warnImgs
}

// PrintProgressReport prints a progress-only summary (no log statistics).
// logsDir is consulted first and finallyDir is used as a fallback so old
// logs written before the T7 logs_dir split still show up in the report.
func PrintProgressReport(inputDir, progressRoot, logsDir, finallyDir string) {
	sep := strings.Repeat("=", 70)
	fmt.Println(sep)
	fmt.Println("进度检查报告")
	fmt.Println(sep)

	totalImages := CountImagesInMDFiles(inputDir)
	perMD := CountImagesPerMD(inputDir)

	fmt.Println("\n【源文件统计】")
	fmt.Printf("  总图片数: %d\n", totalImages)
	keys := make([]string, 0, len(perMD))
	for k := range perMD {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("    %s: %d\n", k, perMD[k])
	}

	completed, invalid := CheckProgressItems(progressRoot)
	remaining := totalImages - completed - invalid
	if remaining < 0 {
		remaining = 0
		fmt.Println("  (注: 进度条目多于当前 md 引用，剩余按 0 计)")
	}
	fmt.Println("\n【进度统计】")
	fmt.Printf("  已完成有效: %d\n", completed)
	fmt.Printf("  无效条目:   %d\n", invalid)
	fmt.Printf("  未完成:     %d\n", remaining)
	if invalid > 0 {
		fmt.Println("  注:         “无效条目”= 校验未通过或格式不符而被跳过的图片，结果没有写盘；")
		fmt.Println("              下轮 img2text 会把它们重新处理（进度文件保留原状），所以它们不是失败。")
	}
	if totalImages > 0 {
		fmt.Printf("  完成率:     %.2f%%\n", float64(completed)/float64(totalImages)*100)
	}
	if totalImages > 0 && invalid > 0 {
		fmt.Printf("  填补后可达: %.2f%%（若下轮把 %d 个无效条目全部补上）\n",
			float64(completed+invalid)/float64(totalImages)*100, invalid)
	}

	// Quality rate from log ERROR/WARNING
	logs, err := logfind.FindAllWithFallback(logsDir, finallyDir)
	if err != nil {
		logs = nil
	}
	if len(logs) > 0 && completed > 0 {
		problemSet := map[string]struct{}{}
		for _, lf := range logs {
			for _, p := range GetProblematicImages(lf) {
				problemSet[p] = struct{}{}
			}
		}
		good := completed
		// We don't have the completed_img_paths here; the caller may already
		// have computed it. Provide a coarse metric: completed minus any
		// problematic image we know about.
		if len(problemSet) > 0 {
			// Without the actual completed paths, the conservative quality
			// rate is "completed - problematic" capped at 0.
			bad := len(problemSet)
			if bad > completed {
				bad = completed
			}
			good = completed - bad
		}
		rate := 0.0
		if completed > 0 {
			rate = float64(good) / float64(completed) * 100
		}
		fmt.Println("\n【良品率】")
		fmt.Printf("  无错误/警告: %d/%d (%.2f%%)\n", good, completed, rate)
	}
}
