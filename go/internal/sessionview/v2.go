package sessionview

import (
	"fmt"
	"net/http"
	"path"
)

// v2Page is the shell the rebuilt viewer mounts into. It reuses the classic
// page's theme pre-script (applied before first paint so there is no flash)
// and loads the embedded Vue bundle from /v2/viewer.{css,js}. The classic
// viewer keeps / untouched — this is a side route for the migration.
const v2Page = `<!DOCTYPE html>
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
<link rel="stylesheet" href="/v2/viewer.css">
</head>
<body>
<div id="app"></div>
<script src="/v2/viewer.js"></script>
</body>
</html>
`

// serveV2 routes the /v2 subtree: the rebuilt viewer's page and its two
// embedded build files. Everything else under /v2 is a miss.
func (v *viewerServer) serveV2(w http.ResponseWriter, r *http.Request, p string) {
	switch p {
	case "/v2", "/v2/", "/v2/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, v2Page)
	case "/v2/viewer.js", "/v2/viewer.css":
		v.serveV2Asset(w, path.Base(p))
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
		http.Error(w, "v2 构建产物缺失：先在 web/ 里执行 npm run build 并提交 dist", http.StatusNotFound)
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
