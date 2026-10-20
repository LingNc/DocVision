// Package retryq 是「手动重试」请求的落盘队列（P53）。
//
// 预览页是另一个进程、只读观察者，所以它不能真的去跑会话：用户在页面上点
// 「重试这一章」，页面只能往项目的 `work/retry/` 目录**放一个 JSON 文件**。
// 真正干活的是 `docvision latex`，它在转换阶段的安全点（每批章节循环前后）
// 扫这个目录，按「新尝试」处理命中的章节——归档旧产物、分配新的尝试号、用
// 全新会话重转，于是预览页的「历史」里自然多一条，旧的留档不删。
//
// 队列是文件而不是内存：进程没在跑时请求会留在磁盘上，下次启动先处理；
// 处理完的文件改名成 `.done.json`，同一条请求不会被消费两次。
package retryq

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DirName 是队列目录名（相对项目根）：<proj>/work/retry/。
const DirName = "retry"

// Request 是一次手动干预请求。Chapter 是章节基名（`chapter_002`，与转录文件
// 名里的那段一致）；Role 是 convert（重转，开新尝试）/ checker（只重新核对、
// 产物不动）/ continue（接着最近一次尝试干，不建新尝试，除非给了 From）。
type Request struct {
	TS      string `json:"ts"`
	Project string `json:"project,omitempty"`
	Chapter string `json:"chapter"`
	Role    string `json:"role,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Source 记录是谁提的（页面/命令行），排障时对得上。
	Source string `json:"source,omitempty"`
	// From 是"从第几条记录之后继续"的截断点：转录里的行号（1 起、含该行）。
	// 0 = 从末尾继续（不截断）。给了它就开**一个新尝试**，历史只保留到这一行，
	// 后面的轮次丢掉——源转录一字不动（原记录仍在「历史」里）。
	From int `json:"from,omitempty"`
	// Path 是截断的源转录文件名（From > 0 时必填）：本项目 work/sessions/ 下的
	// *.jsonl。显式指定而不是"取最新那条"，因为用户可能正在看第 2 次尝试、
	// 想把这次尝试的某一步回滚出来。
	Path string `json:"path,omitempty"`
}

// Dir 返回项目的队列目录。
func Dir(proj string) string { return filepath.Join(proj, "work", DirName) }

// Queue 把一条请求写进队列，返回落盘路径。文件名带纳秒时间戳与章节名，
// 同一个项目里多条请求互不覆盖（O_EXCL，撞了就换名）。
func Queue(proj string, req Request) (string, error) {
	if strings.TrimSpace(req.Chapter) == "" {
		return "", fmt.Errorf("重试请求缺少章节名")
	}
	dir := Dir(proj)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if req.TS == "" {
		req.TS = time.Now().Format(time.RFC3339)
	}
	if req.Role == "" {
		req.Role = "convert"
	}
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return "", err
	}
	now := time.Now().UnixNano()
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("req-%d-%s.json", now+int64(i), sanitize(req.Chapter))
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			_ = os.Remove(path)
			return "", err
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(path)
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("重试请求文件名冲突，写入失败")
}

// File 是一条待办请求及其来源文件。
type File struct {
	Path string
	Request
}

// Pending 列出还没被消费的请求，按时间（文件名）排序。坏文件跳过而不是
// 让整个队列不可用。
func Pending(proj string) []File {
	entries, err := os.ReadDir(Dir(proj))
	if err != nil {
		return nil
	}
	var out []File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".done.json") {
			// `.done.json` 是处理完的留档，不是待办。
			continue
		}
		path := filepath.Join(Dir(proj), e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var req Request
		if err := json.Unmarshal(data, &req); err != nil {
			continue
		}
		if strings.TrimSpace(req.Chapter) == "" {
			continue
		}
		out = append(out, File{Path: path, Request: req})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Chapters 返回待办请求涉及的章节名（去重、保持顺序），给"要不要强制跑
// convert 阶段"这类快速判断用。
func Chapters(proj string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range Pending(proj) {
		if seen[f.Chapter] {
			continue
		}
		seen[f.Chapter] = true
		out = append(out, f.Chapter)
	}
	return out
}

// Consume 把一条已处理的请求改名成 `.done.json`（就地留档，不删——事后能
// 看出"这一章是被人手动要求重跑的"）。
func Consume(path string) error {
	if path == "" {
		return nil
	}
	return os.Rename(path, strings.TrimSuffix(path, ".json")+".done.json")
}

// sanitize 只保留文件名安全字符，避免章节名被当成路径。
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "chapter"
	}
	return b.String()
}
