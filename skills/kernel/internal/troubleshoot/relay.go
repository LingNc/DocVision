package troubleshoot

// relay.go：匿名兜底客户端——把已过校验的 issue.md 经 relay 服务器代发
// （用户部署的公网小服务，服务器端持有 GitHub 凭据；正文保留 agent 署名块）。
// docs/07-troubleshoot.md 上报流程兜底段。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// jsonMarshal/jsonUnmarshal/httpPostJSON：最小封装（测试可替换）。
var (
	jsonMarshal   = json.Marshal
	jsonUnmarshal = json.Unmarshal
	httpPostJSON  = func(url string, payload []byte) ([]byte, error) {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
)

// SubmitViaRelay 匿名兜底：经 relay 服务器代发 issue.md（须先通过 submit 校验）。
// author = agent 署名（谁发现/提交的问题），由 relay 附在正文末尾署名块。
func SubmitViaRelay(root, relayURL, author string) (string, error) {
	issuePath := filepath.Join(Dir(root), "issue.md")
	if _, err := os.Stat(issuePath); err != nil {
		return "", fmt.Errorf("issue.md 不存在（先 troubleshoot submit 过校验）")
	}
	body, err := os.ReadFile(issuePath)
	if err != nil {
		return "", err
	}
	title := "skills 问题反馈"
	for _, ln := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(ln, "# ") || strings.HasPrefix(ln, "## ") {
			title = strings.TrimSpace(strings.TrimLeft(ln, "#"))
			break
		}
	}
	payload, _ := jsonMarshal(map[string]string{"title": title, "body": string(body), "author": author})
	resp, err := httpPostJSON(strings.TrimRight(relayURL, "/")+"/api/submit", payload)
	if err != nil {
		return "", err
	}
	var out struct {
		OK       bool   `json:"ok"`
		IssueURL string `json:"issue_url"`
		Error    string `json:"error"`
	}
	if err := jsonUnmarshal(resp, &out); err != nil {
		return "", fmt.Errorf("relay 响应解析失败: %w", err)
	}
	if !out.OK {
		return "", fmt.Errorf("relay 拒绝: %s", out.Error)
	}
	return out.IssueURL, nil
}
