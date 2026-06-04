package analyze

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			invalid++
			continue
		}
		var doc struct {
			Result  string `json:"result"`
			ImgPath string `json:"img_path"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			invalid++
			continue
		}
		if strings.Contains(doc.Result, "[IMG_TYPE:") && doc.Result != "__INVALID_RESPONSE__" {
			completed++
		} else {
			invalid++
		}
	}
	return completed, invalid
}

// GetProblematicImages returns image paths from logPath that are followed by
// at least one [ERROR] or [WARNING] line before the next image line.
func GetProblematicImages(logPath string) []string {
	problematic := map[string]struct{}{}
	f, err := os.Open(logPath)
	if err != nil {
		return nil
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
		hasError := false
		j := i + 1
		for j < len(lines) && imageLineRef.FindStringSubmatch(lines[j]) == nil {
			if strings.Contains(lines[j], "[ERROR]") || strings.Contains(lines[j], "[WARNING]") {
				hasError = true
			}
			j++
		}
		if hasError {
			problematic[current] = struct{}{}
		}
		i = j - 1
	}

	out := make([]string, 0, len(problematic))
	for k := range problematic {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// PrintProgressReport prints a progress-only summary (no log statistics).
func PrintProgressReport(inputDir, progressRoot, finallyDir string) {
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
	fmt.Println("\n【进度统计】")
	fmt.Printf("  已完成有效: %d\n", completed)
	fmt.Printf("  无效条目:   %d\n", invalid)
	fmt.Printf("  未完成:     %d\n", remaining)
	if totalImages > 0 {
		fmt.Printf("  完成率:     %.2f%%\n", float64(completed)/float64(totalImages)*100)
	}

	// Quality rate from log ERROR/WARNING
	logs := findAllLogs(finallyDir)
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

// findAllLogs returns every img2text_*.log file under logDir sorted by mtime.
func findAllLogs(logDir string) []string {
	matches, _ := filepath.Glob(filepath.Join(logDir, "img2text_*.log"))
	sort.Strings(matches)
	return matches
}
