// DocVision Relay —— 匿名 issue 兜底服务器（用户部署在公网的小服务，Go）。
//
// 用途：使用者拒绝/无法 GitHub 授权时，把 troubleshoot 产出的 issue 经本服务
// 以服务器持有者的 GitHub 账号代为提交（服务器端 token，使用者零凭证）。
// 署名规则：保留提交者（agent 署名）在 issue 正文末尾的"提交者"块——代发不抹来源。
//
// 运行（服务器侧，所有秘密走环境变量）：
//   RELAY_GITHUB_TOKEN=ghp_xxx RELAY_REPO=LingNc/DocVision RELAY_LISTEN=:8787 go run ./skills/relay
// 接口：
//   POST /api/submit  {"title": "...", "body": "...", "author": "agent-署名"}
//   GET  /health
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// rateLimiter 简单内存限流：每 IP 每小时 limit 次。
type rateLimiter struct {
	mu    sync.Mutex
	hits  map[string][]time.Time
	limit int
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}, limit: limit}
}

func (r *rateLimiter) allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	var keep []time.Time
	for _, t := range r.hits[ip] {
		if now.Sub(t) < time.Hour {
			keep = append(keep, t)
		}
	}
	r.hits[ip] = keep
	if len(keep) >= r.limit {
		return false
	}
	r.hits[ip] = append(r.hits[ip], now)
	return true
}

// submitRequest 提交载荷。
type submitRequest struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Author string `json:"author"` // agent 署名（谁发现/提交的问题）
}

// issueResponse GitHub 创建 issue 的响应（只要 URL）。
type issueResponse struct {
	HTMLURL string `json:"html_url"`
	Number  int    `json:"number"`
}

func main() {
	token := os.Getenv("RELAY_GITHUB_TOKEN")
	repo := os.Getenv("RELAY_REPO")
	if repo == "" {
		repo = "LingNc/DocVision"
	}
	listen := os.Getenv("RELAY_LISTEN")
	if listen == "" {
		listen = ":8787"
	}
	rate := 5
	if v := os.Getenv("RELAY_RATE"); v != "" {
		fmt.Sscanf(v, "%d", &rate)
	}
	if token == "" {
		log.Fatal("RELAY_GITHUB_TOKEN 未设置（服务器端 GitHub 凭据）")
	}
	rl := newRateLimiter(rate)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/submit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
			return
		}
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !rl.allow(ip) {
			http.Error(w, `{"error":"rate limited（每小时限 RELAY_RATE 次）"}`, http.StatusTooManyRequests)
			return
		}
		var req submitRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, `{"error":"payload 解析失败"}`, http.StatusBadRequest)
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		req.Body = strings.TrimSpace(req.Body)
		req.Author = strings.TrimSpace(req.Author)
		if req.Title == "" || len([]rune(req.Title)) > 120 {
			http.Error(w, `{"error":"title 必填且 ≤120 字"}`, http.StatusBadRequest)
			return
		}
		if req.Body == "" || len(req.Body) > 60000 {
			http.Error(w, `{"error":"body 必填且 ≤60000 字节"}`, http.StatusBadRequest)
			return
		}
		if req.Author == "" {
			req.Author = "anonymous-agent"
		}
		// 署名块：代发不抹来源
		body := req.Body + "\n\n---\n**提交者**（agent 署名，经 DocVision relay 匿名兜底通道）: " + req.Author + "\n"

		url, err := createIssue(token, repo, "[relay] "+req.Title, body)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"GitHub 提交失败: %s"}`, err.Error()), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "issue_url": url})
	})
	log.Printf("DocVision relay listening on %s (repo=%s, rate=%d/h)", listen, repo, rate)
	log.Fatal(http.ListenAndServe(listen, mux))
}

// createIssue 用服务器端 token 代建 issue（正文带署名块）。
func createIssue(token, repo, title, body string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body})
	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("https://api.github.com/repos/%s/issues", repo), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "docvision-relay")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	var ir issueResponse
	if err := json.NewDecoder(resp.Body).Decode(&ir); err != nil {
		return "", err
	}
	return ir.HTMLURL, nil
}
