package sessionview

import (
	"embed"
	"fmt"
	"net/http"
	"path"
	"strings"
)

// The web/ build (single-file IIFE + single-file CSS) lands in assets/dist
// and is embedded on purpose: //go:embed is a compile-time directive, so a
// missing file fails the build rather than the page.
//
//go:embed assets/*
var assets embed.FS

/*
 * v2Page 是 Vue 版查看器的页面壳（旧页三件套退役后它就是唯一界面）。
 * 主题预置脚本在首帧渲染前应用，避免"先白后黑"的闪烁；资源走根路径
 * /viewer.{css,js}（构建产物从 assets/dist 内嵌伺服）。/v2 子树保留为
 * 别名：旧书签与 muscle memory 不断，伺服的内容与根路径完全一致。
 * 静态导出（WriteStaticHTML）复用同一壳，把 CSS/JS/数据全部内联。
 */
const v2PageHead = `<!DOCTYPE html>
<html lang="zh-CN" data-theme="light">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>DocVision 会话预览</title>
<script>
/* 主题在首帧渲染前应用，避免"先白后黑"的闪烁；取不到存储一律回退白天模式。 */
(function () {
  var theme = 'light';
  try {
    var saved = window.localStorage.getItem('dsh.sessionview.theme');
    if (saved === 'dark' || saved === 'light') { theme = saved; }
  } catch (err) { /* file:// 下可能禁用 localStorage：静默用默认值 */ }
  document.documentElement.setAttribute('data-theme', theme);
})();
</script>
`

// v2Page 是实时模式的完整 HTML：资源外链，数据走 /api/*。
const v2Page = v2PageHead + `<link rel="stylesheet" href="/viewer.css">
</head>
<body>
<div id="app"></div>
<script src="/viewer.js"></script>
</body>
</html>
`

// writeV2PageHTML 输出页面（live 或 static 共用响应头逻辑）。
func writeV2PageHTML(w http.ResponseWriter, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, page)
}

// serveV2 routes the v2 page on both mounts: "/" (the primary, old viewer
// retired). Asset requests are answered from
// assets/dist by basename so both prefixes work.
func (v *viewerServer) serveV2(w http.ResponseWriter, r *http.Request, p string) {
	base := path.Base(p)
	switch {
	case p == "/" || p == "/index.html":
		writeV2PageHTML(w, v2Page)
	case base == "viewer.js" || base == "viewer.css":
		v.serveV2Asset(w, base)
	default:
		http.NotFound(w, r)
	}
}

// serveV2Asset serves a file out of assets/dist, where the web/ build lands.
// The build output is committed on purpose: //go:embed is a compile-time
// directive, so a missing file fails the build rather than the page.
func (v *viewerServer) serveV2Asset(w http.ResponseWriter, name string) {
	data, err := assets.ReadFile("assets/dist/" + name)
	if err != nil {
		http.Error(w, "构建产物缺失：先在 web/ 里执行 npm run build 并提交 dist", http.StatusNotFound)
		return
	}
	switch path.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, string(data))
}

// staticPage renders the self-contained snapshot: same shell, inlined CSS/JS
// and the #dsh-data block the Vue data layer boots from instead of /api.
func staticPage(data string) (string, error) {
	css, err := assets.ReadFile("assets/dist/viewer.css")
	if err != nil {
		return "", fmt.Errorf("会话预览: 读取构建样式失败: %w", err)
	}
	js, err := assets.ReadFile("assets/dist/viewer.js")
	if err != nil {
		return "", fmt.Errorf("会话预览: 读取构建脚本失败: %w", err)
	}
	var b strings.Builder
	b.WriteString(v2PageHead)
	b.WriteString("<style>\n")
	b.Write(css)
	b.WriteString("\n</style>\n</head>\n<body>\n<div id=\"app\"></div>\n")
	b.WriteString(`<script type="application/json" id="dsh-data">`)
	b.WriteString(data)
	b.WriteString("</script>\n<script>\n")
	b.Write(js)
	b.WriteString("\n</script>\n</body>\n</html>\n")
	return b.String(), nil
}
