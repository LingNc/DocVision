package sessionview

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mineru-tools/internal/retryq"
)

// /api/retry —— 页面上那个「重试这一章」的落点（P53）。
//
// 预览页是**只读观察者**：真正干活的是另一个进程里的 `docvision latex`。
// 所以这里不执行任何命令，只做一件事——把请求写进项目的 `work/retry/`
// 队列目录。正在跑的 latex 会在转换阶段的安全点消费它；没在跑就留到下次
// 启动。写入口可以整体关掉（--no-retry）。
//
// 三道校验，任何一道不过都拒绝：① 项目名必须是扫描根下的真实目录（复用
// resolveUnderRoot，杜绝 `..`）；② 章节名必须长得像 `chapter_002`
// （`chapter_004_child_01` 也认）；③ 那个章节的 md 必须真的在磁盘上。

var chapterNameRe = regexp.MustCompile(`^chapter_[0-9]+(_child_[0-9]+)*$`)

type retryRequest struct {
	Project string `json:"project"`
	Chapter string `json:"chapter"`
	Reason  string `json:"reason"`
	Role    string `json:"role"`
}

// projectWorkspace 把页面上的项目名换成磁盘目录：RootProject（旧版单项目
// 布局）就是扫描根本身，其它名字必须在根下且是目录。
func (v *viewerServer) projectWorkspace(project string) (string, bool) {
	if strings.TrimSpace(v.root) == "" {
		return "", false
	}
	if project == "" || project == RootProject {
		return v.root, true
	}
	abs, ok := resolveUnderRoot(v.root, project)
	if !ok {
		return "", false
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", false
	}
	return abs, true
}

func (v *viewerServer) serveRetry(w http.ResponseWriter, r *http.Request) {
	if !v.allowRetry {
		http.Error(w, "重试入口已关闭（docvision sessions 用 --no-retry 启动的）", http.StatusForbidden)
		return
	}
	var req retryRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	base := strings.TrimSuffix(strings.TrimSpace(req.Chapter), ".md")
	if !chapterNameRe.MatchString(base) {
		http.Error(w, "章节名不合法（期望 chapter_002 这种）", http.StatusBadRequest)
		return
	}
	projDir, ok := v.projectWorkspace(req.Project)
	if !ok {
		http.Error(w, "项目不存在", http.StatusNotFound)
		return
	}
	if st, err := os.Stat(filepath.Join(projDir, "chapters", base+".md")); err != nil || !st.Mode().IsRegular() {
		// 子章节由父章节的细分树产出（不在 chapters/ 下），单独重跑它得连同父
		// 节点的整合一起走——那不是"重试这一章"，明说而不是含糊地报不存在。
		if strings.Contains(base, "_child_") {
			http.Error(w, "子章节不能单独重试，请重试它所属的顶层章节: "+base, http.StatusBadRequest)
			return
		}
		http.Error(w, "章节不存在: "+base, http.StatusNotFound)
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "convert"
	}
	path, err := retryq.Queue(projDir, retryq.Request{
		Project: req.Project, Chapter: base, Role: role,
		Reason: strings.TrimSpace(req.Reason), Source: "web",
	})
	if err != nil {
		http.Error(w, "写入重试队列失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "chapter": base, "project": req.Project,
		"queued": filepath.Base(path),
		"note":   "已加入重试队列：正在跑的 docvision latex 会在转换阶段处理它；没在跑就留到下次启动",
	})
}

// pendingRetries 是每个项目当前排队的重试点（页面据此给章节行挂「待重试」）。
// 只列 latex 板块的项目（img2text 的 Project 不在扫描根下）。
func (v *viewerServer) pendingRetries(sessions []SessionInfo) map[string][]string {
	out := map[string][]string{}
	for _, s := range sessions {
		if s.Board != "" || s.Project == "" {
			continue
		}
		if _, done := out[s.Project]; done {
			continue
		}
		dir, ok := v.projectWorkspace(s.Project)
		if !ok {
			continue
		}
		if chapters := retryq.Chapters(dir); len(chapters) > 0 {
			sort.Strings(chapters)
			out[s.Project] = chapters
		} else {
			out[s.Project] = nil // 标记"看过这个项目了"
		}
	}
	for k, v := range out {
		if len(v) == 0 {
			delete(out, k)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
