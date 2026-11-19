// Package mineruapi 是 MinerU API 客户端（旧线 go/internal/mineru 逐字移植骨架）：
// 上传（1MiB 分块+空闲超时）→ 排队轮询（状态标签/进度日志/超时续等）→
// 下载结果 zip 并解压。token 走环境变量 MINERU_TOKEN（不落盘）；模型参数走
// skills_config.json 的 mineru 段（用户专属 config-set）。
package mineruapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	chunkSize        = 1 << 20
	maxUploadRetries = 3
	retrySleep       = 3 * time.Second
	logIntervalCap   = 64
	dataIDMaxLen     = 128
	dataIDHashLen    = 16
)

var stateLabels = map[string]string{
	"pending": "排队中", "running": "解析中", "converting": "格式转换中", "waiting-file": "等待文件上传",
}

// Config 是 API 参数（Token 从环境变量 MINERU_TOKEN 读）。
type Config struct {
	APIBaseURL    string
	ModelVersion  string
	IsOCR         bool
	EnableFormula bool
	EnableTable   bool
	Language      string
	PollInterval  int // 秒
	PollTimeout   int // 秒（0=不限）
	UploadTimeout int // 秒（空闲超时）
}

// DefaultConfig 出厂默认——对齐原 DocVision 实测可用值（老线同款配置长期在跑）。
// model_version 合法值仅 pipeline / vlm / MinerU-HTML（11-19 实测探针：vlm→200，v2→-10002，省略→200）。
func DefaultConfig() Config {
	return Config{
		APIBaseURL:    "https://mineru.net/api/v4", // api.mineru.net 旧域已不可解析
		ModelVersion:  "vlm",
		IsOCR:         true,
		EnableFormula: true,
		EnableTable:   true,
		Language:      "ch",
		PollInterval:  3,
		PollTimeout:   0,
		UploadTimeout: 300,
	}
}

// Client 是 MinerU API 客户端。
type Client struct {
	cfg     Config
	http    *http.Client
	baseURL string
	token   string
	Printf  func(format string, args ...any) // 进度日志注入（测试静默）
}

// NewClient 建客户端；token 缺省读环境变量 MINERU_TOKEN。
func NewClient(cfg Config, token string) (*Client, error) {
	if token == "" {
		token = os.Getenv("MINERU_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("MinerU token 未提供：设环境变量 MINERU_TOKEN（或调用方注入）")
	}
	if cfg.APIBaseURL == "" {
		cfg = DefaultConfig()
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 3
	}
	if cfg.UploadTimeout <= 0 {
		cfg.UploadTimeout = 300
	}
	return &Client{cfg: cfg, http: &http.Client{}, baseURL: strings.TrimRight(cfg.APIBaseURL, "/"), token: token,
		Printf: func(string, ...any) {}}, nil
}

// UploadTaskResponse/ExtractResult* 是 API 响应结构（旧线 types.go 逐字）。
type UploadTaskResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		BatchID  string   `json:"batch_id"`
		FileURLs []string `json:"file_urls"`
	} `json:"data"`
}

type ExtractProgress struct {
	ExtractedPages int `json:"extracted_pages"`
	TotalPages     int `json:"total_pages"`
}

type ExtractResultItem struct {
	State           string          `json:"state"`
	ErrMsg          string          `json:"err_msg"`
	FullZipURL      string          `json:"full_zip_url"`
	ExtractProgress ExtractProgress `json:"extract_progress"`
}

type BatchResult struct {
	Code int `json:"code"`
	Msg  string
	Data struct {
		ExtractResult []ExtractResultItem `json:"extract_result"`
	} `json:"data"`
}

func (c *Client) headers() http.Header {
	return http.Header{"Content-Type": {"application/json"}, "Authorization": {"Bearer " + c.token}}
}

// dataIDForUpload：API 限 128 字符，超长中文名截断+哈希后缀（旧线判据）。
func dataIDForUpload(stem string) string {
	if len(stem) <= dataIDMaxLen {
		return stem
	}
	sum := sha256.Sum256([]byte(stem))
	return stem[:dataIDMaxLen-dataIDHashLen-1] + "_" + hex.EncodeToString(sum[:])[:dataIDHashLen]
}

// CreateUploadTask 建批任务+分块上传，返回 batch_id（失败重试 3 次）。
func (c *Client) CreateUploadTask(filePath string) (string, error) {
	fileName := filepath.Base(filePath)
	stem := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}
	var lastErr error // 记录真实错误：循环内 := 曾遮蔽外层 err 导致 %!w(<nil>) 吞错（首秀 agent 抓到）
	for attempt := 1; attempt <= maxUploadRetries; attempt++ {
		uploadURL, batchID, rerr := c.requestUploadURL(fileName, stem)
		if rerr == nil {
			rerr = c.chunkedUpload(uploadURL, filePath, info.Size())
		}
		if rerr == nil {
			return batchID, nil
		}
		lastErr = rerr
		if attempt < maxUploadRetries {
			time.Sleep(retrySleep)
		}
	}
	return "", fmt.Errorf("创建/上传任务失败（重试 %d 次）: %w", maxUploadRetries, lastErr)
}

func (c *Client) requestUploadURL(fileName, stem string) (string, string, error) {
	// 上游 2026-11 变更：version 字段传任何值（含空串）都报 -10002，非空才发送该键
	bodyMap := map[string]any{
		"files":          []map[string]string{{"name": fileName, "data_id": dataIDForUpload(stem)}},
		"is_ocr":         c.cfg.IsOCR,
		"enable_formula": c.cfg.EnableFormula,
		"enable_table":   c.cfg.EnableTable,
		"language":       c.cfg.Language,
	}
	if c.cfg.ModelVersion != "" {
		bodyMap["model_version"] = c.cfg.ModelVersion
	}
	body, _ := json.Marshal(bodyMap)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/file-urls/batch", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header = c.headers()
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed UploadTaskResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", "", err
	}
	if parsed.Code != 0 {
		return "", "", fmt.Errorf("创建任务失败: %s", parsed.Msg)
	}
	if parsed.Data.BatchID == "" || len(parsed.Data.FileURLs) == 0 {
		return "", "", fmt.Errorf("响应缺 batch_id/file_urls")
	}
	return parsed.Data.FileURLs[0], parsed.Data.BatchID, nil
}

// chunkedUpload 分块 PUT + 空闲超时（旧线结构简化：泵协程+定时器）。
func (c *Client) chunkedUpload(uploadURL, filePath string, fileSize int64) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := url.Parse(uploadURL); err != nil {
		return fmt.Errorf("非法上传 URL: %w", err)
	}
	idle := time.Duration(c.cfg.UploadTimeout) * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, pr)
	if err != nil {
		return err
	}
	req.ContentLength = fileSize

	idleErr := fmt.Errorf("上传空闲超过 %s", idle)
	idleTimer := time.AfterFunc(idle, func() { _ = pw.CloseWithError(idleErr) })
	type res struct{ err error }
	done := make(chan res, 1)
	go func() {
		defer idleTimer.Stop()
		buf := make([]byte, chunkSize)
		for {
			n, rerr := f.Read(buf)
			if n > 0 {
				if _, werr := pw.Write(buf[:n]); werr != nil {
					done <- res{werr}
					return
				}
				idleTimer.Reset(idle)
			}
			if rerr == io.EOF {
				_ = pw.Close()
				done <- res{nil}
				return
			}
			if rerr != nil {
				_ = pw.CloseWithError(rerr)
				done <- res{rerr}
				return
			}
		}
	}()
	resp, err := c.http.Do(req)
	if err != nil {
		_ = pw.CloseWithError(err)
		<-done
		return err
	}
	defer resp.Body.Close()
	if perr := (<-done).err; perr != nil {
		return perr
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("上传 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// GetBatchResults 轮询任务状态（网络错折叠为 code=-1，旧线语义）。
func (c *Client) GetBatchResults(batchID string) *BatchResult {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/extract-results/batch/%s", c.baseURL, batchID), nil)
	if err != nil {
		return &BatchResult{Code: -1, Msg: err.Error()}
	}
	req.Header = c.headers()
	resp, err := c.http.Do(req)
	if err != nil {
		return &BatchResult{Code: -1, Msg: err.Error()}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return &BatchResult{Code: -1, Msg: err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &BatchResult{Code: -1, Msg: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	var out BatchResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return &BatchResult{Code: -1, Msg: "decode: " + err.Error()}
	}
	return &out
}

// CheckOldTaskValid 旧线语义移植：按 batch_id 查老任务（断点续跑入口）。
// (valid, doneItem)：valid=服务端仍认得且非 failed；doneItem 在已 done 时非 nil
// （调用方直接续下载，不再轮询）。invalid → 调用方重传。
func (c *Client) CheckOldTaskValid(batchID string) (bool, *ExtractResultItem) {
	result := c.GetBatchResults(batchID)
	if len(result.Data.ExtractResult) == 0 {
		return false, nil
	}
	item := result.Data.ExtractResult[0]
	if item.State == "failed" {
		return false, nil
	}
	if item.State == "done" && item.FullZipURL != "" {
		return true, &item
	}
	return true, nil
}

// PollBatch 轮询到终态：done→成功；failed→错误；超时→errTimeout（续等语义）。
var errTimeout = fmt.Errorf("poll timeout")

func (c *Client) PollBatch(batchID string) (*ExtractResultItem, error) {
	start := time.Now()
	for {
		result := c.GetBatchResults(batchID)
		if len(result.Data.ExtractResult) == 0 {
			time.Sleep(time.Duration(c.cfg.PollInterval) * time.Second)
			continue
		}
		item := result.Data.ExtractResult[0]
		state := item.State
		label := stateLabels[state]
		if label == "" {
			label = state
		}
		elapsed := int(time.Since(start).Seconds())
		if item.ExtractProgress.TotalPages > 0 {
			c.Printf("[%ds] %s (%d/%d 页)\n", elapsed, label, item.ExtractProgress.ExtractedPages, item.ExtractProgress.TotalPages)
		} else {
			c.Printf("[%ds] %s\n", elapsed, label)
		}
		switch state {
		case "done":
			if item.FullZipURL == "" {
				return nil, fmt.Errorf("解析完成但没有下载链接")
			}
			return &item, nil
		case "failed":
			if item.ErrMsg == "" {
				item.ErrMsg = "未知错误"
			}
			return nil, fmt.Errorf("解析失败: %s", item.ErrMsg)
		}
		if c.cfg.PollTimeout > 0 && elapsed >= c.cfg.PollTimeout {
			return nil, errTimeout
		}
		time.Sleep(time.Duration(c.cfg.PollInterval) * time.Second)
	}
}

// DownloadAndExtract 下载结果 zip 并解压到 targetDir，删除 zip。
func (c *Client) DownloadAndExtract(zipURL, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Get(zipURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载 HTTP %d", resp.StatusCode)
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
	out.Close()
	if err := unzip(zipPath, targetDir); err != nil {
		return err
	}
	return os.Remove(zipPath)
}

// NormalizeLayout 归位解压产物：MinerU zip 内层是 <data_id>/auto/...，
// 解压到 mineru/<part>/ 后可能多套一层（<part>/<part>/auto）。
// 本函数找到 content_list 所在目录，把其 <...>/auto 整层归位到 <dir>/auto。
// 幂等：已是标准布局则不动。返回归位后的 auto 目录路径（找不到则报错）。
// copyEntry 递归拷贝单个文件或目录（NormalizeLayout 的 rename 失败回退用；FUSE 上 rename 不可靠）。
func copyEntry(from, to string) error {
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		return os.WriteFile(to, data, 0o644)
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyEntry(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// NormalizeLayout 归位解压产物——统一收敛到 part 根（对齐原 DocVision 结构约定：
// <part>/{<data_id>_content_list.json, images/}，无 auto 层）。上游 zip 形态任意
// （带顶层文件夹/带 auto 层/直出 part 根/混合残留），一律摊平合并进 part 根。
// 返回 part 根本身。
func NormalizeLayout(dir string) (string, error) {
	// 深度搜 content_list（取最浅命中：walk 字典序下目录名排在文件名前，嵌套的会先被访问，
	// 深度比较才稳）——证明产物存在且定位产物根。
	var found string
	depth := func(p string) int { return strings.Count(p, string(os.PathSeparator)) }
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, "_content_list.json") && !strings.Contains(name, "_v2") {
			dp := filepath.Dir(p)
			if found == "" || depth(dp) < depth(found) {
				found = dp
			}
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("%s 下未找到 content_list（MinerU 产物结构不符）", dir)
	}
	// 1) 产物根不是 part 根：把产物根的内容合并上移进 part 根，清掉嵌套壳。
	if found != dir {
		if err := mergeInto(found, dir); err != nil {
			return "", err
		}
		// 清壳：从 found 往 dir 方向逐层删空目录
		for d := found; d != dir && strings.HasPrefix(d, dir); {
			if err := os.Remove(d); err != nil {
				break // 非空（有独有残留）则保留，不硬删
			}
			d = filepath.Dir(d)
		}
	}
	// 2) part 根下的 auto/ 壳（历史布局残留）：内容合并上移，删掉壳。
	if auto := filepath.Join(dir, "auto"); dirExists(auto) {
		if err := mergeInto(auto, dir); err != nil {
			return "", err
		}
		_ = os.RemoveAll(auto)
	}
	return dir, nil
}

// dirExists 目录存在性。
func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// mergeInto 把 src 目录的内容合并进 dst（逐条：目录递归合并，文件 dst 已有则跳过，
// 否则 rename、失败（FUSE/跨卷）则 copyEntry 递归拷贝）。幂等。
func mergeInto(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if dirExists(to) {
				if err := mergeInto(from, to); err != nil {
					return err
				}
				continue
			}
			if err := os.Rename(from, to); err != nil {
				if err := copyEntry(from, to); err != nil {
					return err
				}
				_ = os.RemoveAll(from)
			}
			continue
		}
		// 文件：dst 已有同名则保留现有（part 根产物优先）
		if _, err := os.Stat(to); err == nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			if err := copyEntry(from, to); err != nil {
				return err
			}
			_ = os.Remove(from)
		}
	}
	return nil
}

// unzip 解压（防 zip-slip：拒绝绝对路径与 ..）。
func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		clean := filepath.Clean(f.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("zip 含危险路径: %s", f.Name)
		}
		target := filepath.Join(dst, clean)
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, rc)
		w.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}


// ProfilePath 由调用方传入技能目录下 data/profile.env 路径；空=不读文件。
// profile 优先级：显式参数 > 环境变量 > data/profile.env > 默认值。
func (c *Client) loadProfile(profilePath string) {
	if profilePath == "" {
		return
	}
	b, err := os.ReadFile(profilePath)
	if err != nil {
		return // 无 profile 文件静默走环境变量
	}
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		k, v, ok := strings.Cut(ln, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "MINERU_TOKEN":
			if c.token == "" && os.Getenv("MINERU_TOKEN") == "" {
				c.token = strings.TrimSpace(v)
			}
		case "MINERU_API_BASE_URL":
			if c.cfg.APIBaseURL == DefaultConfig().APIBaseURL && os.Getenv("MINERU_API_BASE_URL") == "" {
			c.cfg.APIBaseURL = strings.Trim(strings.TrimSpace(v), `"'`)
				c.baseURL = strings.TrimRight(c.cfg.APIBaseURL, "/")
			}
		}
	}
}

// LoadProfile 供命令层显式注入 profile 路径（技能目录 data/profile.env）。
func (c *Client) LoadProfile(profilePath string) { c.loadProfile(profilePath) }
