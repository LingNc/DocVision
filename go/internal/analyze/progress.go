package analyze

import (
	"bufio"
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
func CheckProgressItems(progressRoot string) (completed, invalid int) {
	if _, err := os.Stat(progressRoot); err != nil {
		return 0, 0
	}
	matches, _ := filepath.Glob(filepath.Join(progressRoot, "**", "*.json"))
	if len(matches) == 0 {
		return 0, 0
	}
	// T55：一万多个 json 全量 ReadFile+Unmarshal 在 samba 盘上每次 analyze
	// 都要等很久。并发读 + 只做字节级判定（progress json 的 result 字段是
	// 顶层字符串，"[IMG_TYPE:" 出现即完成、"__INVALID_RESPONSE__" 哨兵单独
	// 剔除），不再反序列化。
	type verdict struct{ done bool }
	verdicts := make([]verdict, len(matches))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i, f := range matches {
		wg.Add(1)
		go func(i int, f string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			data, err := os.ReadFile(f)
			if err != nil {
				return
			}
			if bytes.Contains(data, []byte("[IMG_TYPE:")) && !bytes.Contains(data, []byte("__INVALID_RESPONSE__")) {
				verdicts[i].done = true
			}
		}(i, f)
	}
	wg.Wait()
	for _, v := range verdicts {
		if v.done {
			completed++
		} else {
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
