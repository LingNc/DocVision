package sessionview

import (
	"io"
	"encoding/hex"
	"crypto/sha256"
	"context"
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
	// Extras are additional scan directories (T37 img2text sessions),
	// each rendered with its own grouping rules.
	Extras []ScanExtra
	// Render configures the /api/mermaid preview endpoint (T56).
	Render RenderConfig
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
	bound, stop, err := StartWithRender(opt.Root, opt.Addr, opt.Extras, opt.Render)
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
	return StartWith(root, addr, nil)
}

// StartWith is Start plus extra scan directories (T37 img2text sessions).
func StartWith(root, addr string, extras []ScanExtra) (bound Bound, stopped <-chan struct{}, err error) {
	return StartWithRender(root, addr, extras, RenderConfig{})
}

// RenderConfig carries what the /api/mermaid endpoint needs to render
// diagram previews (T56): the mermaid-cli command ("" = mmdc on PATH;
// empty-after-LookPath = endpoint reports unavailable and the page falls
// back to showing the code block) and the per-render timeout.
type RenderConfig struct {
	Command string
	Timeout time.Duration
}

// StartWithRender is StartWith plus the mermaid render configuration.
func StartWithRender(root, addr string, extras []ScanExtra, rc RenderConfig) (bound Bound, stopped <-chan struct{}, err error) {
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
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return Bound{}, nil, fmt.Errorf("会话预览: 监听 %s 失败（端口可能已被占用，请换一个端口）: %w", addr, err)
	}
	srv := &http.Server{
		Handler:           newViewerServer(rootAbs, extras, rc),
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
	root  string
	extra []string
	scan  *scanner
	// img2textRoot is the progress_items directory behind the img2text
	// extra root ("" when preview.img2text is off); /api/img2text-progress
	// scans it for the P18 progress board.
	img2textRoot  string
	mermaidCmd    string
	mermaidTimeout time.Duration
}

// newViewerServer builds the read-only viewer. extras are the absolute
// directories of T37 img2text sessions; they are scanned for sessions
// and allowed as media roots (their transcripts reference sibling media/
// dirs that live outside the main root).
func newViewerServer(root string, extras []ScanExtra, rc ...RenderConfig) *viewerServer {
	ex := make([]ScanExtra, len(extras))
	copy(ex, extras)
	extraDirs := make([]string, 0, len(extras))
	for _, e := range extras {
		if e.Dir == "" {
			continue
		}
		if abs, err := filepath.Abs(e.Dir); err == nil {
			extraDirs = append(extraDirs, abs)
		}
	}
	// P18：img2text 板块的进度数据源 = progress_items 根（img2text extra
	// 的 Dir 就是它）。
	img2textRoot := ""
	for _, e := range extras {
		if e.Kind == "img2text" && e.Dir != "" {
			if abs, err := filepath.Abs(e.Dir); err == nil {
				img2textRoot = abs
			}
			break
		}
	}
	v := &viewerServer{root: root, extra: extraDirs, scan: &scanner{root: root, extras: ex}, img2textRoot: img2textRoot}
	if len(rc) > 0 {
		v.mermaidCmd = rc[0].Command
		v.mermaidTimeout = rc[0].Timeout
	}
	return v
}

// ServeHTTP routes by hand instead of using http.ServeMux: ServeMux rewrites
// "." and ".." segments with a 301 before the handler sees them, which would
// turn a traversal attempt into a redirect instead of the explicit rejection
// this local tool wants.
func (v *viewerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead &&
		!(r.Method == http.MethodPost && r.URL.Path == "/api/mermaid") {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "只读服务：仅支持 GET/HEAD（/api/mermaid 接受 POST）", http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	switch {
	// v2 是唯一界面（"/"）；"/viewer.{css,js}" 是
	// Vue 构建产物。旧页三件套已随退役删除。
	case p == "/" || p == "/index.html":
		v.serveV2(w, r, p)
	case p == "/viewer.css" || p == "/viewer.js":
		v.serveV2Asset(w, path.Base(p))
	case p == "/api/index":
		v.serveIndex(w)
	case p == "/api/session":
		v.serveSession(w, r)
	case p == "/api/img2text-progress":
		v.serveImg2TextProgress(w)
	case p == "/api/mermaid":
		v.serveMermaid(w, r)
	case strings.HasPrefix(p, "/media/"), strings.HasPrefix(p, "/file/"):
		v.serveFile(w, r)
	default:
		http.NotFound(w, r)
	}
}

// serveImg2TextProgress reports the P18 img2text progress board: per-book
// per-image status built from the progress_items tree (empty when no
// img2text extra root is configured).
func (v *viewerServer) serveImg2TextProgress(w http.ResponseWriter) {
	books := ScanImg2TextProgress(v.img2textRoot)
	if books == nil {
		books = []Img2TextBook{}
	}
	writeJSON(w, map[string]any{"books": books})
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
	// Partial (P7) is the live streaming snapshot of the message currently
	// being generated: <transcript>.partial, written by the running session
	// and removed the moment the full message lands. nil when absent.
	Partial *PartialInfo `json:"partial,omitempty"`
}

// PartialInfo mirrors session.PartialRecord (kept local so the preview
// package does not import the session package).
type PartialInfo struct {
	Phase string `json:"phase"`
	Text  string `json:"text"`
	Ts    int64  `json:"ts"`
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
	var partial *PartialInfo
	if data, err := os.ReadFile(found.Path + ".partial"); err == nil {
		var p PartialInfo
		if json.Unmarshal(data, &p) == nil && p.Text != "" {
			partial = &p
		}
	}
	writeJSON(w, sessionResponse{ID: id, Lines: lines, NextFrom: next, Bytes: found.Bytes, Partial: partial})
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
	// T37：主根之外，img2text 附加根的 media/ 也要能取（转录里的
	// file://media/ 引用以转录所在目录为基）。resolveUnderRoot 只查越界、
	// 不查存在——主根对任何相对路径都"命中"——所以逐根尝试时要确认文件
	// 真的存在，全部根都没有才 404；全部越界才拒。
	var f *os.File
	for _, root := range append([]string{v.root}, v.extra...) {
		abs, ok := resolveUnderRoot(root, rel)
		if !ok {
			continue
		}
		candidate, err := os.Open(abs)
		if err != nil {
			continue
		}
		f = candidate
		break
	}
	if f == nil {
		// 区分"存在但越界"（400）与"哪儿都没有"（404）：前者说明请求
		// 本身不合法，后者只是文件缺失。
		allowed := false
		for _, root := range append([]string{v.root}, v.extra...) {
			if _, ok := resolveUnderRoot(root, rel); ok {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(w, "拒绝越界路径", http.StatusBadRequest)
			return
		}
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

// ---------- /api/mermaid（T56 图表预览） ----------

// serveMermaid renders one mermaid diagram source to SVG via mermaid-cli and
// caches it by content hash under the user cache dir. The page POSTs the
// fenced block's source; on any failure it falls back to showing the code
// block, so errors here are reported in-band instead of as HTTP failures.
func (v *viewerServer) serveMermaid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fail := func(msg string) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
	}
	command := v.mermaidCmd
	if command == "" {
		command = "mmdc"
	}
	if _, err := exec.LookPath(command); err != nil {
		fail("mermaid 渲染器不可用: " + err.Error())
		return
	}
	var req struct {
		Source string `json:"source"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil || json.Unmarshal(body, &req) != nil || strings.TrimSpace(req.Source) == "" {
		fail("请求格式不对")
		return
	}
	sum := sha256.Sum256([]byte(req.Source))
	hash := hex.EncodeToString(sum[:])[:24]

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	cacheDir = filepath.Join(cacheDir, "docvision-mermaid")
	cached := filepath.Join(cacheDir, hash+".svg")
	if data, err := os.ReadFile(cached); err == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "svg": string(data)})
		return
	}

	work, err := os.MkdirTemp("", "docvision-mermaid-web-")
	if err != nil {
		fail(err.Error())
		return
	}
	defer os.RemoveAll(work)
	in := filepath.Join(work, "in.mmd")
	out := filepath.Join(work, "out.svg")
	if err := os.WriteFile(in, []byte(req.Source+"\n"), 0o600); err != nil {
		fail(err.Error())
		return
	}
	timeout := v.mermaidTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, "-i", in, "-o", out, "-b", "transparent")
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = runErr.Error()
		}
		fail(msg)
		return
	}
	svg, err := os.ReadFile(out)
	if err != nil {
		fail("渲染产物读取失败: " + err.Error())
		return
	}
	if os.MkdirAll(cacheDir, 0o755) == nil {
		_ = os.WriteFile(cached, svg, 0o644) // 缓存失败只影响下次速度
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "svg": string(svg)})
}
