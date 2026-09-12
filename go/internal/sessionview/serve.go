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

// ServeOptions is everything the CLI's banner has to report: what is scanned,
// where it listens, and where each of those values came from. The provenance
// fields are plain labels (config key / flag / 内置默认) — no prose.
type ServeOptions struct {
	// Root is the directory scanned for transcripts.
	Root string
	// Addr is host:port to listen on ("" = DefaultAddr, port 0 = kernel picks).
	Addr string
	// DirSource says where Root came from, e.g. "config paths.latex_project".
	DirSource string
	// AddrSource says where Addr came from, e.g. "--port".
	AddrSource string
	// ConfigNote is the resolved config file, or "未找到" when none applies.
	ConfigNote string
	// OpenBrowser is honoured for callers that want it; the CLI deliberately
	// leaves it false and lets the user click the printed URL.
	OpenBrowser bool
}

// Bound is where the viewer actually listens, as net.Listen reported it — not
// as the config or the flags asked for it. A wildcard bind is a real address to
// print (0.0.0.0:8849) but not one to open in a browser, so it also carries the
// loopback URL to click.
type Bound struct {
	// Addr is host:port exactly as bound: "127.0.0.1:8848", "0.0.0.0:8849",
	// "[::]:8849", or the port the kernel picked when 0 was requested.
	Addr string
	// URL is what a human can paste into a browser: the bound address, except
	// that a wildcard bind is rendered as loopback (that is where the page is
	// reachable from).
	URL string
	// Browse is the loopback URL to click, set only when Addr is a wildcard
	// bind (otherwise it is URL and saying it twice helps nobody).
	Browse string
}

// Serve runs the read-only session viewer until the process is interrupted.
//
// It prints the banner once the listener is actually up.
func Serve(opt ServeOptions) error {
	bound, stop, err := Start(opt.Root, opt.Addr)
	if err != nil {
		return err
	}
	fmt.Print(serveBanner(bound, opt))
	if opt.OpenBrowser {
		openInBrowser(bound.URL)
	}
	<-stop
	return nil
}

// serveBanner renders the startup lines: where it listens, what it scans and
// which config is behind both choices. Each value carries where it came from;
// there is no prose.
//
// The listen address is the one net.Listen returned, so a wildcard bind prints
// "0.0.0.0:8849" and a port the kernel picked prints the port it picked. A
// wildcard bind gets one extra line with the loopback URL, because that is the
// address a browser can actually open.
func serveBanner(bound Bound, opt ServeOptions) string {
	rootAbs, _ := filepath.Abs(opt.Root)
	var b strings.Builder
	if bound.Browse != "" {
		fmt.Fprintf(&b, "会话预览: %s（只读服务，Ctrl+C 停止）\n", bound.Addr)
		fmt.Fprintf(&b, "浏览 %s\n", bound.Browse)
	} else {
		fmt.Fprintf(&b, "会话预览: %s（只读服务，Ctrl+C 停止）\n", bound.URL)
	}
	fmt.Fprintf(&b, "目录: %s%s\n", rootAbs, sourceSuffix(opt.DirSource))
	fmt.Fprintf(&b, "配置: %s%s\n", configNote(opt.ConfigNote), addrSourceSuffix(opt.AddrSource))
	return b.String()
}

// addrSourceSuffix renders " · 监听地址来源 X" (or "" when the caller gave none).
func addrSourceSuffix(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	return " · 监听地址来源 " + src
}

// sourceSuffix renders " · 来源 X" (or "" when the caller gave no label).
func sourceSuffix(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	return " · 来源 " + src
}

// configNote keeps the config line factual when no config was found.
func configNote(note string) string {
	if strings.TrimSpace(note) == "" {
		return "未找到"
	}
	return note
}

// Start launches the read-only viewer in the background and returns where it
// actually bound plus a channel that is closed when the listener stops. It
// prints NOTHING: the caller owns the terminal (a latex run paints a live
// progress block there, and a stray Printf from a goroutine would land in the
// middle of it), and it may want the address for its own log line.
//
// addr "" = DefaultAddr; port 0 = the kernel picks one, so the returned Bound
// is the authoritative address.
func Start(root, addr string) (bound Bound, stopped <-chan struct{}, err error) {
	if addr == "" {
		addr = DefaultAddr
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return Bound{}, nil, err
	}
	st, err := os.Stat(rootAbs)
	if err != nil {
		return Bound{}, nil, fmt.Errorf("会话预览: 无法读取目录 %s: %w", root, err)
	}
	if !st.IsDir() {
		return Bound{}, nil, fmt.Errorf("会话预览: %s 不是目录", root)
	}
	ln, err := net.Listen(listenNetwork(addr), addr)
	if err != nil {
		return Bound{}, nil, fmt.Errorf("会话预览: 监听 %s 失败（端口可能已被占用，请换一个端口）: %w", addr, err)
	}
	srv := &http.Server{
		Handler:           newViewerServer(rootAbs),
		ReadHeaderTimeout: 10 * time.Second,
	}
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(ln)
		close(done)
	}()
	out := Bound{Addr: ln.Addr().String(), URL: "http://" + displayAddr(ln.Addr()) + "/"}
	if isWildcardAddr(ln.Addr()) {
		out.Browse = out.URL
	}
	return out, done, nil
}

// listenNetwork 按**写死的字面 host** 挑网络族：Go 的 net.Listen("tcp", "0.0.0.0:8849")
// 遇到通配 host 会优先选 IPv6 双栈套接字，内核于是回报 "[::]:8849"——用户按配置写了
// 0.0.0.0、横幅却显示 [::]，看着就像配置没生效。所以：0.0.0.0 → tcp4、:: → tcp6，
// 只有 host 留空（":8849"）才交给双栈，loopback 与主机名照旧用 tcp。
func listenNetwork(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "tcp"
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "0.0.0.0":
		return "tcp4"
	case "::":
		return "tcp6"
	}
	return "tcp"
}

// isWildcardAddr reports whether a listener bound every interface (0.0.0.0 /
// ::), which is exactly when the printed address is not a URL to open.
func isWildcardAddr(a net.Addr) bool {
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		return false
	}
	return tcp.IP == nil || tcp.IP.IsUnspecified()
}

// PreviewAddr renders host:port for the config-driven viewer ("" host =
// loopback, port 0 = let the kernel choose).
func PreviewAddr(host string, port int) string {
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, fmt.Sprint(port))
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
	Root      string `json:"root"`
	Generated string `json:"generated"`
	// Every session carries its own per-image estimate rule (SessionInfo.
	// Estimate): with per-model rules there is no single page-wide one.
	Sessions []SessionInfo `json:"sessions"`
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
