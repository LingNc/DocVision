package sessionview

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultAddr is the loopback address the viewer listens on. The tool is a
// local debugging aid, so it binds to 127.0.0.1 unless the caller says
// otherwise.
const DefaultAddr = "127.0.0.1:8848"

// Serve runs the read-only session viewer until the process is interrupted.
//
// The URL is printed once the listener is actually up, so "--addr 127.0.0.1:0"
// reports the port the kernel picked. openBrowser is honoured for callers that
// want it; the CLI deliberately passes false and lets the user click the
// printed link.
func Serve(root, addr string, openBrowser bool) error {
	if addr == "" {
		addr = DefaultAddr
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	st, err := os.Stat(rootAbs)
	if err != nil {
		return fmt.Errorf("会话预览: 无法读取目录 %s: %w", root, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("会话预览: %s 不是目录", root)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("会话预览: 监听 %s 失败（端口可能已被占用，请用 --addr 换一个端口）: %w", addr, err)
	}

	url := "http://" + displayAddr(ln.Addr()) + "/"
	fmt.Printf("会话预览: %s\n", url)
	fmt.Printf("目录: %s（只读服务，Ctrl+C 停止）\n", rootAbs)
	if openBrowser {
		openInBrowser(url)
	}

	srv := &http.Server{
		Handler:           newViewerServer(rootAbs),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.Serve(ln)
}

// displayAddr turns a listener address into something a human can paste into a
// browser: a wildcard bind is shown as loopback because that is where the page
// is reachable from.
func displayAddr(a net.Addr) string {
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		return a.String()
	}
	host := tcp.IP.String()
	if tcp.IP == nil || tcp.IP.IsUnspecified() {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, fmt.Sprint(tcp.Port))
}

func openInBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// viewerServer is the whole HTTP surface: the page, two asset files, two JSON
// endpoints and a read-only file route. Everything is derived from the scan
// root and nothing is ever written.
type viewerServer struct {
	root string
	scan *scanner
}

func newViewerServer(root string) *viewerServer {
	return &viewerServer{root: root, scan: &scanner{root: root}}
}

// ServeHTTP routes by hand instead of using http.ServeMux: ServeMux rewrites
// "." and ".." segments with a 301 before the handler sees them, which would
// turn a traversal attempt into a redirect instead of the explicit rejection
// this local tool wants.
func (v *viewerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "只读服务：仅支持 GET/HEAD", http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	switch {
	case p == "/" || p == "/index.html":
		v.servePage(w, r)
	case p == "/viewer.css" || p == "/viewer.js":
		v.serveAsset(w, r, path.Base(p))
	case p == "/api/index":
		v.serveIndex(w)
	case p == "/api/session":
		v.serveSession(w, r)
	case strings.HasPrefix(p, "/media/"), strings.HasPrefix(p, "/file/"):
		v.serveFile(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (v *viewerServer) servePage(w http.ResponseWriter, r *http.Request) {
	page, err := renderPage(renderOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, page)
}

func (v *viewerServer) serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	data, err := assets.ReadFile("assets/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch path.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	fmt.Fprint(w, string(data))
}

// indexResponse is what the page polls every couple of seconds: enough to
// render the sidebar and to decide whether the open session grew.
type indexResponse struct {
	Root      string        `json:"root"`
	Generated string        `json:"generated"`
	Sessions  []SessionInfo `json:"sessions"`
}

func (v *viewerServer) serveIndex(w http.ResponseWriter) {
	sessions, err := v.scan.scan()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []SessionInfo{}
	}
	writeJSON(w, indexResponse{
		Root:      v.root,
		Generated: time.Now().Format(time.RFC3339),
		Sessions:  sessions,
	})
}

// sessionResponse is one incremental read: the lines from the requested line
// number on, plus the line count the client should ask from next time.
type sessionResponse struct {
	ID       string `json:"id"`
	Lines    []Line `json:"lines"`
	NextFrom int    `json:"nextFrom"`
	Bytes    int64  `json:"size"`
}

func (v *viewerServer) serveSession(w http.ResponseWriter, r *http.Request) {
	id := path.Clean(strings.TrimPrefix(r.URL.Query().Get("id"), "/"))
	from, err := parseFrom(r.URL.Query().Get("from"))
	if err != nil {
		http.Error(w, "from 参数无效", http.StatusBadRequest)
		return
	}
	// The id must name a session the scan actually found: that both rejects
	// traversal and keeps the endpoint from turning into a general file reader.
	sessions, err := v.scan.scan()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var found *SessionInfo
	for i := range sessions {
		if sessions[i].ID == id {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		http.NotFound(w, r)
		return
	}
	lines, next, err := ReadSession(found.Path, from)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if lines == nil {
		lines = []Line{}
	}
	writeJSON(w, sessionResponse{ID: id, Lines: lines, NextFrom: next, Bytes: found.Bytes})
}

// serveFile serves /media/... and /file/... straight from the scan root. The
// viewer only ever asks for media files, but the root is what the paths are
// relative to, so both prefixes mean the same thing.
func (v *viewerServer) serveFile(w http.ResponseWriter, r *http.Request) {
	var rel string
	switch {
	case strings.HasPrefix(r.URL.Path, "/media/"):
		rel = strings.TrimPrefix(r.URL.Path, "/media/")
	default:
		rel = strings.TrimPrefix(r.URL.Path, "/file/")
	}
	abs, ok := resolveUnderRoot(v.root, rel)
	if !ok {
		http.Error(w, "拒绝越界路径", http.StatusBadRequest)
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		// A directory listing would expose the tree; the viewer never needs it.
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

// resolveUnderRoot maps a slash path from the viewer onto the filesystem and
// reports whether it stays inside root. ".." is refused outright rather than
// cleaned away: a request containing it is never legitimate here, and refusing
// it is what the traversal test asserts.
func resolveUnderRoot(root, rel string) (string, bool) {
	if strings.ContainsRune(rel, 0) {
		return "", false
	}
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", false
	}
	for _, el := range strings.Split(rel, "/") {
		if el == ".." {
			return "", false
		}
	}
	clean := path.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	abs := filepath.Join(root, filepath.FromSlash(clean))
	within, err := filepath.Rel(root, abs)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}

func parseFrom(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid from")
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}
