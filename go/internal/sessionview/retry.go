package sessionview

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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
//
// 请求带角色（P53 阶段二）：convert = 重转这一章（新尝试）、checker = 只重新核对
// 已提交产物、continue = 接着这一章最近一次尝试继续跑（不新建尝试）。
// 运行期的消费者是 latex 里的 watcher，任何阶段都收，忙的章节留队。

var chapterNameRe = regexp.MustCompile(`^chapter_[0-9]+(_child_[0-9]+)*$`)

type retryRequest struct {
	Project string `json:"project"`
	Chapter string `json:"chapter"`
	Reason  string `json:"reason"`
	Role    string `json:"role"`
	// From/Path 是"从第几条记录之后继续"（P53 阶段二）：From 是转录行号（1 起、含该行），
	// Path 是源转录文件名（本项目 work/sessions/ 下的 *.jsonl）。0 = 从末尾继续。
	From int    `json:"from,omitempty"`
	Path string `json:"path,omitempty"`
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
	// 矢量图重画（P53）：不带章节，点名一条 vector_*.jsonl 作图转录。
	role := strings.TrimSpace(req.Role)
	if role == "vector" {
		v.serveVectorRetry(w, req)
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
	if role == "" {
		role = "convert"
	}
	if role != "convert" && role != "checker" && role != "continue" {
		http.Error(w, "角色只能是 convert、checker、continue 或 vector", http.StatusBadRequest)
		return
	}
	// 继续跑的对象是**已有的那条会话**：没有转录就没什么可接着的。
	if role == "continue" {
		matches, _ := filepath.Glob(filepath.Join(projDir, "work", "sessions", "convert_"+base+"*.jsonl"))
		if len(matches) == 0 {
			http.Error(w, "这一章还没有转换记录，没法接着跑（先跑一次或重试转换）", http.StatusBadRequest)
			return
		}
	}
	// 重新核对的对象是**已提交的产物**：还没交过就没什么可核的，先重试转换。
	if role == "checker" && !fileExistsIn(projDir, "work", "chapters", base+".tex") {
		http.Error(w, "这一章还没有提交产物，无法重新核对（先重试转换）", http.StatusBadRequest)
		return
	}
	// 从某条记录之后继续：必须先确认源转录在这个项目里、行号落在范围里，
	// 否则排进队列也是白排（worker 那边还会再查一次）。
	srcName := ""
	if req.From > 0 {
		if role != "continue" {
			http.Error(w, "只有「继续跑」可以指定从第几条记录开始", http.StatusBadRequest)
			return
		}
		srcName = filepath.Base(strings.TrimSpace(req.Path))
		if !transcriptNameRe.MatchString(srcName) {
			http.Error(w, "源转录不合法（要 work/sessions/ 下的 *.jsonl）", http.StatusBadRequest)
			return
		}
		lines, err := countTranscriptLines(filepath.Join(projDir, "work", "sessions", srcName))
		if err != nil {
			http.Error(w, "源转录不存在: "+srcName, http.StatusNotFound)
			return
		}
		if req.From > lines {
			http.Error(w, fmt.Sprintf("第 %d 条记录超出范围（这条转录只有 %d 行）", req.From, lines), http.StatusBadRequest)
			return
		}
	}
	path, err := retryq.Queue(projDir, retryq.Request{
		Project: req.Project, Chapter: base, Role: role,
		Reason: strings.TrimSpace(req.Reason), Source: "web",
		From: req.From, Path: srcName,
	})
	if err != nil {
		http.Error(w, "写入重试队列失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	note := "已加入重试队列：正在跑的 docvision latex 会把它排进并发槽位（任何阶段都收）；没在跑就留到下次启动"
	if role == "checker" {
		note = "已加入重新核对队列：只重跑核对会话，产物不动；正在跑的 latex 会把它排进并发槽位"
	}
	if role == "continue" && req.From > 0 {
		note = fmt.Sprintf("已排队：从第 %d 条记录之后继续——会开一个新尝试，历史只保留到这一行，源转录保留", req.From)
	} else if role == "continue" {
		note = "已加入继续跑队列：接着这一章最近一次尝试干（不新建尝试、不归档），正在跑的 latex 会把它排进并发槽位"
	}
	writeJSON(w, map[string]any{
		"ok": true, "chapter": base, "project": req.Project, "role": role,
		"queued": filepath.Base(path), "note": note,
	})
}

// fileExistsIn 在项目目录下按段拼路径并确认是普通文件。
func fileExistsIn(root string, parts ...string) bool {
	st, err := os.Stat(filepath.Join(append([]string{root}, parts...)...))
	return err == nil && st.Mode().IsRegular()
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
		if files := retryq.Pending(dir); len(files) > 0 {
			seen := map[string]bool{}
			var chapters []string
			for _, f := range files {
				if f.Chapter == "" { // 矢量图重画没有章节，不挂章节角标
					continue
				}
				if seen[f.Chapter] {
					continue
				}
				seen[f.Chapter] = true
				chapters = append(chapters, f.Chapter)
			}
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

// retryRoles 是"每个项目里每一章排的是哪个角色的重试"（P53）：章节行有两个
// 角色（转换 / 核对），「待重试」必须挂在对应的那一行，而不是两行都挂。
func (v *viewerServer) retryRoles(sessions []SessionInfo) map[string]map[string]string {
	out := map[string]map[string]string{}
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
		files := retryq.Pending(dir)
		if len(files) == 0 {
			out[s.Project] = nil
			continue
		}
		roles := map[string]string{}
		for _, f := range files {
			if f.Chapter == "" { // 矢量图重画：没有章节行可挂
				continue
			}
			role := f.Role
			if role == "" {
				role = "convert"
			}
			// 同一章两种角色都排着时保留最后一条（消费者两条都会跑）。
			roles[f.Chapter] = role
		}
		out[s.Project] = roles
	}
	for k, m := range out {
		if len(m) == 0 {
			delete(out, k)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// /api/drop —— 删掉一条"转错的记录"（P53 阶段二）。
//
// 历史里一条作废的尝试会一直挂在「历史 N」里；用户看到某条明显跑歪（模型跑
// 飞了、上下文坏了）的记录时，希望能直接丢掉它，而不是永远留着。这里只做
// **转录文件**的删除，且必须满足：
//
//	① 项目在扫描根下（复用 projectWorkspace）；
//	② 文件就在这个项目的 work/sessions/ 里、扩展名 .jsonl（拒绝任何路径穿越）；
//	③ 它现在**没有被写入**（mtime 超出现在 90 秒且没有 .partial 快照）——
//	   正在跑的会话删转录会把它自己写坏。
//
// 带 .tryN 的转录连它的作废归档（work/attempts/<章>/try<N>/）一起删，因为那
// 正是"这一轮的错误记录"。
type dropRequest struct {
	Project string `json:"project"`
	Path    string `json:"path"`
}

var transcriptNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.jsonl$`)

func (v *viewerServer) serveDrop(w http.ResponseWriter, r *http.Request) {
	if !v.allowRetry {
		http.Error(w, "写入口已关闭（docvision sessions 用 --no-retry 启动的）", http.StatusForbidden)
		return
	}
	var req dropRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	projDir, ok := v.projectWorkspace(req.Project)
	if !ok {
		http.Error(w, "项目不存在", http.StatusNotFound)
		return
	}
	name := filepath.Base(strings.TrimSpace(req.Path))
	if !transcriptNameRe.MatchString(name) {
		http.Error(w, "只能删转录文件（*.jsonl）", http.StatusBadRequest)
		return
	}
	full := filepath.Join(projDir, "work", "sessions", name)
	st, err := os.Stat(full)
	if err != nil || !st.Mode().IsRegular() {
		http.Error(w, "转录不存在: "+name, http.StatusNotFound)
		return
	}
	if time.Since(st.ModTime()) < 90*time.Second || fileExistsIn(projDir, "work", "sessions", name+".partial") {
		http.Error(w, "这条会话可能还在写（90 秒内有更新），等它停下来再删", http.StatusConflict)
		return
	}
	if err := os.Remove(full); err != nil {
		http.Error(w, "删除失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// 作废尝试的归档一并清掉：它和这条转录是同一次尝试的两半。
	removedArchive := ""
	if base, attempt, ok := attemptFromTranscriptName(name); ok {
		dir := filepath.Join(projDir, "work", "attempts", base, "try"+strconv.Itoa(attempt))
		if _, err := os.Stat(dir); err == nil {
			if err := os.RemoveAll(dir); err == nil {
				removedArchive = filepath.ToSlash(filepath.Join("work", "attempts", base, "try"+strconv.Itoa(attempt)))
			}
		}
	}
	writeJSON(w, map[string]any{
		"ok": true, "deleted": name, "archive": removedArchive,
		"note": "记录已删除（证据目录 work/attempts 里的同一次尝试也一起删了）",
	})
}

// attemptFromTranscriptName 从 convert_chapter_002.try3.jsonl 反推章节与尝试号。
func attemptFromTranscriptName(name string) (string, int, bool) {
	stem := strings.TrimSuffix(name, ".jsonl")
	idx := strings.LastIndex(stem, ".try")
	if idx < 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(stem[idx+len(".try"):])
	if err != nil || n < 1 {
		return "", 0, false
	}
	base := stem[:idx]
	for _, prefix := range []string{"convert_", "checker_"} {
		if strings.HasPrefix(base, prefix) {
			return strings.TrimPrefix(base, prefix), n, true
		}
	}
	return "", 0, false
}

// countTranscriptLines 数一条转录有多少行（只用来校验"从第几条继续"的范围）。
func countTranscriptLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return 0, nil
	}
	return strings.Count(text, "\n") + 1, nil
}

// serveVectorRetry 处理"重画这张图"（P53）：只收 source/sessions 下的
// vector_*.jsonl，排进队列由运行中的 latex 派发。
func (v *viewerServer) serveVectorRetry(w http.ResponseWriter, req retryRequest) {
	projDir, ok := v.projectWorkspace(req.Project)
	if !ok {
		http.Error(w, "项目不存在", http.StatusNotFound)
		return
	}
	name := filepath.Base(strings.TrimSpace(req.Path))
	if !strings.HasPrefix(name, "vector_") || !transcriptNameRe.MatchString(name) {
		http.Error(w, "只能重画某条作图会话（source/sessions/vector_*.jsonl）", http.StatusBadRequest)
		return
	}
	if !fileExistsIn(projDir, "source", "sessions", name) {
		http.Error(w, "这条作图转录不存在: "+name, http.StatusNotFound)
		return
	}
	if !dirExists(filepath.Join(projDir, "source", "progress_items")) {
		http.Error(w, "这个项目还没有逐图进度（images 阶段没跑过），没法重画", http.StatusBadRequest)
		return
	}
	// 从某条记录之后继续：先确认行号真的落在这条转录里（worker 还会再查一次）。
	if req.From > 0 {
		lines, err := countTranscriptLines(filepath.Join(projDir, "source", "sessions", name))
		if err != nil {
			http.Error(w, "读不到这条作图转录: "+name, http.StatusNotFound)
			return
		}
		if req.From > lines {
			http.Error(w, fmt.Sprintf("第 %d 条记录超出范围（这条转录只有 %d 行）", req.From, lines), http.StatusBadRequest)
			return
		}
	}
	path, err := retryq.Queue(projDir, retryq.Request{
		Project: req.Project, Role: "vector",
		Reason: strings.TrimSpace(req.Reason), Source: "web", Path: name, From: req.From,
	})
	if err != nil {
		http.Error(w, "写入重试队列失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	note := "已排队重画这张图：会开一条新的作图尝试（旧转录留档成 .tryN），" +
		"类别与分组不变；画完要看到新图，样式/划分/转换/组装需要重跑"
	if req.From > 0 {
		note = fmt.Sprintf("已排队：从第 %d 条记录之后继续——历史截到这一条，完整旧转录留档成 .tryN；"+
			"类别与分组不变，画完要看到新图，下游阶段需要重跑", req.From)
	}
	writeJSON(w, map[string]any{
		"ok": true, "project": req.Project, "role": "vector", "image": name,
		"queued": filepath.Base(path), "from": req.From, "note": note,
	})
}

// dirExists 判断一个目录在不在。
func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
