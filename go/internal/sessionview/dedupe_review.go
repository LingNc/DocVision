package sessionview

import (
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// T154：dedupe 的复查页（<项目>/source/dedupe/review.html）同时挂到会话预览页上。
//
// 页面只写一份：disk 上的那份图片走相对路径（../<图片>、thumbs/<摘要>.jpg），这里
// 直接把它发出去、只补一个 <base href="/media/<项目>/source/dedupe/">，相对地址
// 于是全落到本服务的 /media 上。好处是"磁盘上能看"和"网页里能看"永远是同一份
// 内容，不会有两套渲染各说各话；坏处是页面必须自带 <head>，所以找不到时退回
// 在最前面插 base。
func (v *viewerServer) serveDedupeReview(w http.ResponseWriter, r *http.Request) {
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	dir, ok := v.dedupeDir(project)
	if !ok {
		http.Error(w, "拒绝越界路径", http.StatusBadRequest)
		return
	}
	page, err := os.ReadFile(filepath.Join(dir, "review.html"))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "这个项目还没有 dedupe 复查页（整组核验跑完才有）", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(injectBaseHref(page, v.dedupeMediaBase(project)))
}

// dedupeDir 把侧栏的项目名映射到 <root>/<项目>/source/dedupe；根目录组（旧单项目
// 布局）就是 <root>/source/dedupe。项目名来自查询串，所以一律过 resolveUnderRoot。
func (v *viewerServer) dedupeDir(project string) (string, bool) {
	base := strings.TrimSpace(v.root)
	if project != "" && project != RootProject {
		abs, ok := resolveUnderRoot(v.root, project)
		if !ok {
			return "", false
		}
		base = abs
	}
	if base == "" {
		return "", false
	}
	return filepath.Join(base, "source", "dedupe"), true
}

// dedupeMediaBase 是复查页里相对地址的落点：页面在 source/dedupe/ 下，所以 base
// 指到同一个目录，../images/… 与 thumbs/… 才解析得对。
func (v *viewerServer) dedupeMediaBase(project string) string {
	if project == "" || project == RootProject {
		return "/media/source/dedupe/"
	}
	return "/media/" + escapeMediaSegments(project) + "/source/dedupe/"
}

// escapeMediaSegments 按路径段转义：项目名里有中文、空格、# 时，不转义会被浏览器
// 当成另一条路径（图片全 404）。
func escapeMediaSegments(p string) string {
	parts := strings.Split(strings.TrimPrefix(filepath.ToSlash(p), "/"), "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// injectBaseHref 在 <head> 之后插一行 <base>；没有 <head> 就放到最前面。
func injectBaseHref(page []byte, base string) []byte {
	tag := []byte(`<base href="` + html.EscapeString(base) + `">`)
	lower := strings.ToLower(string(page))
	if i := strings.Index(lower, "<head>"); i >= 0 {
		at := i + len("<head>")
		out := make([]byte, 0, len(page)+len(tag)+1)
		out = append(out, page[:at]...)
		out = append(out, '\n')
		out = append(out, tag...)
		return append(out, page[at:]...)
	}
	out := make([]byte, 0, len(page)+len(tag)+1)
	out = append(out, tag...)
	out = append(out, '\n')
	return append(out, page...)
}

// dedupeReviewProjects 列出"有复查页可看"的项目：侧栏在整组核验阶段行上挂一个
// 按钮，点开就是 review.html。只看 latex 板块的会话（img2text 的 Project 不在
// 扫描根下）。
func (v *viewerServer) dedupeReviewProjects(sessions []SessionInfo) []string {
	if strings.TrimSpace(v.root) == "" {
		return nil
	}
	seen := make(map[string]bool, 8)
	out := make([]string, 0, 8)
	for _, s := range sessions {
		if s.Board != "" || s.Project == "" || seen[s.Project] {
			continue
		}
		seen[s.Project] = true
		dir, ok := v.dedupeDir(s.Project)
		if !ok {
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, "review.html")); err == nil && st.Mode().IsRegular() {
			out = append(out, s.Project)
		}
	}
	sort.Strings(out)
	return out
}
