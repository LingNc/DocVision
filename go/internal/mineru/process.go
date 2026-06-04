package mineru

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mineru-tools/pkg/util"
)

// SupportedExtensions lists the file extensions that MinerU accepts,
// lower-cased and including the leading dot. It mirrors the Python
// SUPPORTED_EXTENSIONS set exactly.
var SupportedExtensions = map[string]bool{
	".pdf":  true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".jp2":  true,
	".webp": true,
	".gif":  true,
	".bmp":  true,
	".doc":  true,
	".docx": true,
	".ppt":  true,
	".pptx": true,
	".xls":  true,
	".xlsx": true,
	".html": true,
}

// IsSupported reports whether path has a supported extension. The
// comparison is case-insensitive, matching the Python `f.suffix.lower()`.
func IsSupported(path string) bool {
	return SupportedExtensions[strings.ToLower(filepath.Ext(path))]
}

// ProcessSingleFile runs the full lifecycle for one file: resume if a
// previous task exists, otherwise create a new upload task and poll to
// completion. The state file lives in statusDir as "<stem>_task.json".
//
// The returned TaskStatus tells the caller whether the file completed
// now (TaskDone), was already complete on disk (TaskSkip), or failed
// in this run (TaskFail — including poll timeouts).
func ProcessSingleFile(client *MinerUClient, filePath, outputDir, statusDir string) (string, TaskStatus) {
	fileName := filepath.Base(filePath)
	stem := util.BaseNameNoExt(filePath)
	jsonPath := filepath.Join(statusDir, stem+"_task.json")

	// loadInfo returns nil, false if no state file exists. A corrupted
	// file is treated as "no state" so the run can continue.
	loadInfo := func() (*TaskInfo, bool) {
		raw, err := os.ReadFile(jsonPath)
		if err != nil {
			return nil, false
		}
		var info TaskInfo
		if err := json.Unmarshal(raw, &info); err != nil {
			return nil, false
		}
		return &info, true
	}

	// Possibly resume an existing task.
	if info, ok := loadInfo(); ok {
		if info.Status == "done" {
			folder := filepath.Join(outputDir, stem)
			if util.DirExists(folder) && hasAnyFile(folder) {
				fmt.Printf("[%s] [跳过] 已完成\n", fileName)
				return fileName, TaskSkip
			}
		}

		// Resume old "running" or "timeout" tasks. The Python reference
		// uses "timeout" too even though we never actually write it
		// ourselves; matching it keeps us robust if the on-disk file
		// predates this binary.
		if info.BatchID != "" && (info.Status == "running" || info.Status == "timeout") {
			fmt.Printf("[%s] 恢复任务 %s ...\n", fileName, info.BatchID)
			valid, item := client.CheckOldTaskValid(info.BatchID)
			if valid {
				// Server reports the task is already done — short-circuit
				// straight to download. This handles the case where a
				// previous run timed out after the server finished.
				if item != nil && item.State == "done" {
					zipURL := item.FullZipURL
					if zipURL != "" {
						info.Status = "done"
						if err := util.AtomicWriteJSON(jsonPath, info); err != nil {
							fmt.Printf("[%s] 写入任务状态失败: %v\n", fileName, err)
						}
						if err := client.DownloadAndExtract(fileName, zipURL, outputDir, info.OutputFolder); err != nil {
							return fileName, TaskFail
						}
						fmt.Printf("[%s] ✓ 恢复并下载完成\n", fileName)
						return fileName, TaskDone
					}
					// No zip URL — fall through to normal polling.
				}
				success, isTimeout := client.PollBatchWithDisplay(fileName, info.BatchID, info, jsonPath, outputDir)
				if success {
					return fileName, TaskDone
				}
				if isTimeout {
					info.Status = "running"
					if err := util.AtomicWriteJSON(jsonPath, info); err != nil {
						fmt.Printf("[%s] 写入任务状态失败: %v\n", fileName, err)
					}
					return fileName, TaskFail
				}
				fmt.Printf("[%s] 旧任务无法继续，重新提交...\n", fileName)
			} else {
				fmt.Printf("[%s] 旧任务已失效，重新提交...\n", fileName)
			}
		}
	}

	// No resumable task — create a new one.
	batchID, err := client.CreateUploadTask(filePath)
	if err != nil || batchID == "" {
		return fileName, TaskFail
	}

	info := &TaskInfo{
		File:         fileName,
		BatchID:      batchID,
		CreatedAt:    time.Now().Format(time.RFC3339),
		Status:       "running",
		LastError:    "",
		OutputFolder: stem,
	}
	if err := util.AtomicWriteJSON(jsonPath, info); err != nil {
		fmt.Printf("[%s] 写入任务状态失败: %v\n", fileName, err)
	}
	fmt.Printf("[%s] 任务已创建: %s\n", fileName, batchID)

	success, isTimeout := client.PollBatchWithDisplay(fileName, batchID, info, jsonPath, outputDir)
	if isTimeout {
		info.Status = "running"
		if err := util.AtomicWriteJSON(jsonPath, info); err != nil {
			fmt.Printf("[%s] 写入任务状态失败: %v\n", fileName, err)
		}
		return fileName, TaskFail
	}
	if success {
		return fileName, TaskDone
	}
	return fileName, TaskFail
}

// hasAnyFile reports whether dir contains at least one entry. The
// Python reference uses `any(output_folder.iterdir())` which is true
// for any entry, including subdirectories.
func hasAnyFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// ProcessFilesConcurrent runs ProcessSingleFile across files with at
// most maxConcurrent workers, and returns the count of (done, skip,
// fail). The status directory is created if missing. The function is
// safe to call with an empty slice; in that case it prints the same
// error message as the Python reference and returns (0, 0, 0).
func ProcessFilesConcurrent(
	client *MinerUClient,
	files []string,
	outputDir, statusDir string,
	maxConcurrent int,
) (int, int, int) {
	if len(files) == 0 {
		fmt.Println("[错误] 没有找到需要处理的文件")
		return 0, 0, 0
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	fmt.Printf("找到 %d 个文件，并发数: %d\n", len(files), maxConcurrent)

	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		fmt.Printf("[错误] 创建状态目录失败: %v\n", err)
		return 0, 0, 0
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		doneN   int
		skipN   int
		failN   int
		sem     = make(chan struct{}, maxConcurrent)
	)

	for _, f := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(filePath string) {
			defer wg.Done()
			defer func() { <-sem }()

			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[%s] 处理异常: %v\n", filepath.Base(filePath), r)
					mu.Lock()
					failN++
					mu.Unlock()
				}
			}()

			_, status := ProcessSingleFile(client, filePath, outputDir, statusDir)
			mu.Lock()
			switch status {
			case TaskDone:
				doneN++
			case TaskSkip:
				skipN++
			default:
				failN++
			}
			mu.Unlock()
		}(f)
	}

	wg.Wait()

	fmt.Println()
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("处理完成: %d 成功, %d 跳过, %d 失败/超时\n", doneN, skipN, failN)
	fmt.Println(strings.Repeat("=", 50))

	return doneN, skipN, failN
}
