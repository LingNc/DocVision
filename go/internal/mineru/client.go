package mineru

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/pkg/util"
)

// chunkSize is the upload chunk size in bytes (1 MiB). It matches the
// Python reference and gives reasonable progress granularity.
const chunkSize = 1 * 1024 * 1024

// maxUploadRetries is the number of attempts for the whole
// create-task + upload sequence. The Python reference retries three
// times; we match that exactly.
const maxUploadRetries = 3

// retrySleep is the delay between retry attempts, matching the Python
// `time.sleep(3)` between retries.
const retrySleep = 3 * time.Second

// logIntervalCap is the upper bound on the exponential back-off for both
// upload-progress and poll-status logging. The Python code uses 64s.
const logIntervalCap = 64

// stateLabels mirrors the Chinese labels used by the Python reference
// for the four non-terminal poll states.
var stateLabels = map[string]string{
	"pending":      "排队中",
	"running":      "解析中",
	"converting":   "格式转换中",
	"waiting-file": "等待文件上传",
}

// MinerUClient wraps the HTTP client and configuration needed to talk
// to the MinerU API. It is safe for concurrent use; the underlying
// *http.Client and the chunked uploader are stateless w.r.t. each call.
type MinerUClient struct {
	cfg     config.MinerUConfig
	http    *http.Client
	baseURL string
	headers http.Header
}

// NewClient builds a MinerUClient from a config.MinerUConfig. The
// returned client reuses one *http.Client for the lifetime of the
// process so that connection pooling works as expected.
func NewClient(cfg config.MinerUConfig) *MinerUClient {
	return &MinerUClient{
		cfg: cfg,
		http: &http.Client{
			Timeout: 0, // per-request timeouts are set explicitly
		},
		baseURL: strings.TrimRight(cfg.APIBaseURL, "/"),
		headers: http.Header{
			"Content-Type":  {"application/json"},
			"Authorization": {"Bearer " + cfg.Token},
		},
	}
}

// CreateUploadTask requests an upload URL for filePath, then PUTs the
// file contents to it in 1 MiB chunks. It returns the new batch ID on
// success. On failure it retries up to maxUploadRetries times with a
// fixed retrySleep between attempts. An empty string + nil error means
// the API accepted the task but the upload returned non-200; a non-nil
// error means we could not get a usable upload URL at all.
func (c *MinerUClient) CreateUploadTask(filePath string) (string, error) {
	fileName := filepath.Base(filePath)
	stem := util.BaseNameNoExt(filePath)

	info, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", filePath, err)
	}
	fileSize := info.Size()

	for attempt := 1; attempt <= maxUploadRetries; attempt++ {
		uploadURL, batchID, err := c.requestUploadURL(fileName, stem)
		if err != nil {
			fmt.Printf("[%s] 创建任务异常 (%d/%d): %v\n", fileName, attempt, maxUploadRetries, err)
			if attempt < maxUploadRetries {
				time.Sleep(retrySleep)
				continue
			}
			return "", err
		}

		if err := c.chunkedUpload(fileName, uploadURL, filePath, fileSize); err != nil {
			fmt.Printf("[%s] 上传异常 (%d/%d): %v\n", fileName, attempt, maxUploadRetries, err)
			if attempt < maxUploadRetries {
				time.Sleep(retrySleep)
				continue
			}
			return "", err
		}

		return batchID, nil
	}

	// Unreachable: the loop either returns or continues. The signature
	// needs a return here to keep the compiler happy.
	return "", fmt.Errorf("[%s] 创建任务失败: 超过最大重试次数", fileName)
}

// requestUploadURL POSTs to /file-urls/batch to obtain a presigned
// upload URL and the batch_id we should poll on.
func (c *MinerUClient) requestUploadURL(fileName, stem string) (string, string, error) {
	body := map[string]interface{}{
		"files": []map[string]string{
			{"name": fileName, "data_id": stem},
		},
		"model_version":  c.cfg.ModelVersion,
		"is_ocr":         c.cfg.IsOCR,
		"enable_formula": c.cfg.EnableFormula,
		"enable_table":   c.cfg.EnableTable,
		"language":       c.cfg.Language,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}

	endpoint := c.baseURL + "/file-urls/batch"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	req.Header = c.headers.Clone()

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var parsed UploadTaskResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return "", "", fmt.Errorf("decode response: %w (body=%q)", err, string(rawBody))
	}

	if parsed.Code != 0 {
		msg := parsed.Msg
		if msg == "" {
			msg = "未知错误"
		}
		return "", "", fmt.Errorf("创建任务失败: %s", msg)
	}

	if parsed.Data.BatchID == "" || len(parsed.Data.FileURLs) == 0 {
		return "", "", fmt.Errorf("response missing batch_id/file_urls: %q", string(rawBody))
	}

	return parsed.Data.FileURLs[0], parsed.Data.BatchID, nil
}

// chunkedUpload PUTs filePath to uploadURL in chunkSize pieces, logging
// progress with exponential backoff (3s -> 6s -> ... -> 64s cap). It
// enforces an idle timeout: if no chunk is sent for uploadTimeout
// seconds, the upload is aborted with an error.
func (c *MinerUClient) chunkedUpload(fileName, uploadURL, filePath string, fileSize int64) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Use a *url.URL to make sure the caller can pass either a relative
	// (unlikely) or absolute URL.
	if _, err := url.Parse(uploadURL); err != nil {
		return fmt.Errorf("invalid upload url %q: %w", uploadURL, err)
	}

	idleTimeout := time.Duration(c.cfg.UploadTimeout) * time.Second
	if idleTimeout <= 0 {
		idleTimeout = 300 * time.Second
	}

	// Build a per-request client that gives the connection enough time
	// to settle (connect 30s, read 2x idle) but is bounded so a stuck
	// server eventually surfaces an error.
	reqClient := &http.Client{
		Timeout: idleTimeout * 2,
	}

	// We cannot use http.NewRequest with a streaming body and a known
	// Content-Length without buffering. Build a custom *io.Pipe and let
	// a goroutine pump chunks while the main goroutine runs the request.
	pr, pw := io.Pipe()

	req, err := http.NewRequest(http.MethodPut, uploadURL, pr)
	if err != nil {
		return err
	}
	req.ContentLength = fileSize
	req.Header.Set("Content-Length", fmt.Sprintf("%d", fileSize))

	uploaded := int64(0)
	lastActivity := time.Now()
	lastLog := time.Now()
	logInterval := 3 * time.Second

	type pumpResult struct {
		err error
	}
	done := make(chan pumpResult, 1)

	go func() {
		buf := make([]byte, chunkSize)
		for {
			// Idle timeout check: if the network has been silent for too
			// long, abort. We measure from the last chunk we sent (or the
			// start of the upload) so a slow connection still works.
			if time.Since(lastActivity) > idleTimeout {
				_ = pw.CloseWithError(fmt.Errorf("上传空闲超过 %s", idleTimeout))
				done <- pumpResult{err: fmt.Errorf("上传空闲超过 %s", idleTimeout)}
				return
			}

			n, rerr := f.Read(buf)
			if n > 0 {
				if _, werr := pw.Write(buf[:n]); werr != nil {
					done <- pumpResult{err: werr}
					return
				}
				uploaded += int64(n)
				lastActivity = time.Now()

				now := time.Now()
				if now.Sub(lastLog) >= logInterval {
					pct := 0
					if fileSize > 0 {
						pct = int(uploaded * 100 / fileSize)
					}
					fmt.Printf("[%s] 上传中 %.1f/%.1f MB (%d%%)\n",
						fileName,
						float64(uploaded)/(1024*1024),
						float64(fileSize)/(1024*1024),
						pct,
					)
					if logInterval < logIntervalCap*time.Second {
						logInterval *= 2
						if logInterval > logIntervalCap*time.Second {
							logInterval = logIntervalCap * time.Second
						}
					}
					lastLog = now
				}
			}
			if rerr == io.EOF {
				_ = pw.Close()
				done <- pumpResult{err: nil}
				return
			}
			if rerr != nil {
				_ = pw.CloseWithError(rerr)
				done <- pumpResult{err: rerr}
				return
			}
		}
	}()

	fmt.Printf("[%s] 开始上传 (%.1f MB)...\n", fileName, float64(fileSize)/(1024*1024))

	resp, err := reqClient.Do(req)
	if err != nil {
		// Make sure the pump goroutine has exited.
		<-done
		return err
	}
	defer resp.Body.Close()
	// Drain pump so we don't leak the goroutine if the server is slow.
	<-done

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[%s] 上传失败: HTTP %d\n", fileName, resp.StatusCode)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	fmt.Printf("[%s] 上传完成\n", fileName)
	return nil
}

// GetBatchResults queries the poll endpoint and returns the decoded
// body. Network/parse errors are folded into a synthetic response with
// code = -1 so callers can treat "no result" and "transport failure"
// uniformly. This matches the Python implementation's behaviour.
func (c *MinerUClient) GetBatchResults(batchID string) (*BatchResult, error) {
	endpoint := fmt.Sprintf("%s/extract-results/batch/%s", c.baseURL, batchID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.headers.Clone()

	resp, err := c.http.Do(req)
	if err != nil {
		return &BatchResult{Code: -1, Msg: err.Error()}, nil
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return &BatchResult{Code: -1, Msg: err.Error()}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &BatchResult{Code: -1, Msg: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))}, nil
	}

	var out BatchResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return &BatchResult{Code: -1, Msg: fmt.Sprintf("decode: %v", err)}, nil
	}
	return &out, nil
}

// CheckOldTaskValid returns true if the server still tracks batchID and
// the task is not in a terminal "failed" state. The second return value
// is the (possibly nil) result item, useful for callers that want to
// inspect the current state without a second round-trip.
func (c *MinerUClient) CheckOldTaskValid(batchID string) (bool, *ExtractResultItem) {
	result, err := c.GetBatchResults(batchID)
	if err != nil || result == nil {
		return false, nil
	}
	items := result.Data.ExtractResult
	if len(items) == 0 {
		return false, nil
	}
	item := items[0]
	if result.Code != 0 || item.State == "failed" {
		return false, &item
	}
	return true, &item
}

// PollBatchWithDisplay polls the task until it reaches a terminal state
// or pollTimeout is exceeded. info is mutated in place and persisted to
// jsonPath on every state transition. It returns (success, isTimeout):
//
//   - success=true  -> task reached "done" and result was downloaded.
//   - success=false, isTimeout=true -> pollTimeout elapsed; status kept
//     as "running" so the next run resumes.
//   - success=false, isTimeout=false -> task reached "failed" or the
//     download step failed; the caller may decide to retry.
func (c *MinerUClient) PollBatchWithDisplay(
	fileName, batchID string,
	info *TaskInfo,
	jsonPath, outputDir string,
) (bool, bool) {
	start := time.Now()
	lastLog := start
	logInterval := time.Duration(c.cfg.LogPollInterval) * time.Second
	if logInterval <= 0 {
		logInterval = 3 * time.Second
	}

	// lastProgress tracks the (extracted, total) tuple from the last
	// log line. nil means we have not logged anything yet.
	var lastProgress *ExtractProgress
	var lastValidProgress *ExtractProgress

	saveState := func() {
		if err := util.AtomicWriteJSON(jsonPath, info); err != nil {
			fmt.Printf("[%s] 写入任务状态失败: %v\n", fileName, err)
		}
	}

	for {
		result, _ := c.GetBatchResults(batchID)
		items := result.Data.ExtractResult
		if len(items) == 0 {
			time.Sleep(time.Duration(c.cfg.PollInterval) * time.Second)
			continue
		}
		item := items[0]
		state := item.State
		elapsed := int(time.Since(start).Seconds())

		progress := item.ExtractProgress
		current := progress

		var pagesDelta int
		if lastProgress != nil {
			pagesDelta = current.ExtractedPages - lastProgress.ExtractedPages
		}
		if current.TotalPages > 0 {
			cp := current
			lastValidProgress = &cp
		}

		// "Significant change" triggers immediate output. The Python
		// reference fires on first run, on big page-count jumps, and on
		// terminal states.
		significant := lastProgress == nil ||
			pagesDelta >= c.cfg.ProgressThreshold ||
			state == "done" || state == "failed"

		now := time.Now()
		if significant {
			switch state {
			case "done":
				totalPages := current.TotalPages
				if lastValidProgress != nil && lastValidProgress.TotalPages > 0 {
					totalPages = lastValidProgress.TotalPages
				}
				if totalPages > 0 {
					fmt.Printf("[%s] [%ds] 解析完成，共 %d 页\n", fileName, elapsed, totalPages)
				} else {
					fmt.Printf("[%s] [%ds] 解析完成\n", fileName, elapsed)
				}
			case "failed":
				errMsg := item.ErrMsg
				if errMsg == "" {
					errMsg = "未知错误"
				}
				fmt.Printf("[%s] [%ds] 解析失败: %s\n", fileName, elapsed, errMsg)
			default:
				label := stateLabels[state]
				if label == "" {
					label = state
				}
				pagesInfo := ""
				if current.TotalPages > 0 {
					pagesInfo = fmt.Sprintf("(%d/%d 页)", current.ExtractedPages, current.TotalPages)
				}
				fmt.Printf("[%s] [%ds] %s %s\n", fileName, elapsed, label, pagesInfo)
			}

			logInterval = time.Duration(c.cfg.LogPollInterval) * time.Second
			if logInterval <= 0 {
				logInterval = 3 * time.Second
			}
			lastLog = now
			cp := current
			lastProgress = &cp

			if state == "done" {
				info.Status = "done"
				saveState()

				zipURL := item.FullZipURL
				if zipURL == "" {
					info.Status = "failed"
					info.LastError = "没有下载链接"
					saveState()
					return false, false
				}
				folderName := info.OutputFolder
				if err := c.DownloadAndExtract(fileName, zipURL, outputDir, folderName); err != nil {
					info.Status = "failed"
					info.LastError = "下载解压失败"
					saveState()
					return false, false
				}
				fmt.Printf("[%s] ✓ 已完成并下载\n", fileName)
				return true, false
			}
			if state == "failed" {
				errMsg := item.ErrMsg
				if errMsg == "" {
					errMsg = "未知错误"
				}
				info.Status = "failed"
				info.LastError = errMsg
				saveState()
				return false, false
			}
		} else if now.Sub(lastLog) >= logInterval {
			label := stateLabels[state]
			if label == "" {
				label = state
			}
			pagesInfo := ""
			if current.TotalPages > 0 {
				pagesInfo = fmt.Sprintf("(%d/%d 页)", current.ExtractedPages, current.TotalPages)
			}
			fmt.Printf("[%s] [%ds] %s %s\n", fileName, elapsed, label, pagesInfo)

			if logInterval < time.Duration(logIntervalCap)*time.Second {
				logInterval *= 2
				if logInterval > time.Duration(logIntervalCap)*time.Second {
					logInterval = time.Duration(logIntervalCap) * time.Second
				}
			}
			lastLog = now
			cp := current
			lastProgress = &cp
		}

		// Timeout check (0 = no limit, matches the Python reference).
		if c.cfg.PollTimeout > 0 && elapsed >= c.cfg.PollTimeout {
			fmt.Printf("\n[%s] [错误] 轮询超时 (%ds)\n", fileName, c.cfg.PollTimeout)
			fmt.Printf("[%s] 任务仍在运行，建议稍后重新运行脚本继续等待\n", fileName)
			return false, true
		}

		time.Sleep(time.Duration(c.cfg.PollInterval) * time.Second)
	}
}

// DownloadAndExtract fetches zipURL into <outputDir>/<folderName>/result.zip
// and then unzips it in place, deleting the zip on success. The
// download itself has a fixed 120s timeout, matching the Python code.
func (c *MinerUClient) DownloadAndExtract(fileName, zipURL, outputDir, folderName string) error {
	targetDir := filepath.Join(outputDir, folderName)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	fmt.Printf("[%s] 下载结果...\n", fileName)

	dlClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := dlClient.Get(zipURL)
	if err != nil {
		fmt.Printf("[%s] 下载解压失败: %v\n", fileName, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err := fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		fmt.Printf("[%s] 下载解压失败: %v\n", fileName, err)
		return err
	}

	zipPath := filepath.Join(targetDir, "result.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	if err := util.Unzip(zipPath, targetDir); err != nil {
		fmt.Printf("[%s] 下载解压失败: %v\n", fileName, err)
		return err
	}
	if err := os.Remove(zipPath); err != nil {
		// Non-fatal: log but keep going.
		fmt.Printf("[%s] 清理 zip 失败: %v\n", fileName, err)
	}
	return nil
}
