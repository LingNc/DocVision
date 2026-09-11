// Package sessionview turns DocVision AI session transcripts into something a
// human can browse: the append-only JSONL files written by
// internal/session/transcript.go are scanned, parsed and rendered either as a
// self-contained static HTML snapshot or by a local read-only HTTP server that
// the page polls while a session is still running.
//
// The page renders what the transcript actually contains and nothing more.
// Transcripts hold user/assistant/tool messages, optional reasoning traces,
// tool calls and image references. What the model was told at the start of a
// run — the system prompt and the tool definitions — is recorded in separate
// t="meta" lines that are never replayed; the viewer shows the newest one as a
// "what was the model instructed" card instead of pretending the information
// is missing.
package sessionview

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"mineru-tools/internal/session"
)

// RootProject is the sidebar group used for transcripts that sit directly in
// the scan root, i.e. whose relative path has no project directory above them.
const RootProject = "（根目录）"

// ProjectFor returns the sidebar group of a transcript: the first segment of
// its path relative to the scan root. A scan root can hold several projects
// (latex_project/, latex_project_0909/, and later latex_project/<书名>/), so
// that first segment is exactly the "which project is this session from"
// question the grouped sidebar answers.
func ProjectFor(rel string) string {
	id := filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(rel), "./"))
	i := strings.Index(id, "/")
	if i <= 0 {
		return RootProject
	}
	return id[:i]
}

// projectMarkers are entries that only a project WORKSPACE root has: the
// multi-project marker docvision writes, the phase state files, and the
// per-stage directories of either level.
var projectMarkers = []string{
	".docvision_project.json", "progress.json", "progress_items",
	"work", "source", "style", "chapters", "doc_index",
}

// structuralNames are workspace internals. A directory with one of these
// names is never itself a project: without this, the legacy single-project
// layout (latex_project/work/sessions/*.jsonl) would be read as a project
// called "work" instead of a session of the project latex_project.
var structuralNames = map[string]bool{
	"work": true, "source": true, "style": true, "chapters": true,
	"sessions": true, "temp": true, "views": true, "pages": true,
	"build": true, "out": true, "doc_index": true, "reports": true,
	"media": true, "figures": true, "images": true, "pdfview": true,
}

// ProjectGroup is the sidebar group of one transcript: which project it
// belongs to, and whether that project sits in the legacy single-project
// layout (the output root IS the workspace) rather than the multi-project
// one (the output root holds one directory per book).
type ProjectGroup struct {
	Name string
	// Legacy is true when the group directory is itself a workspace root
	// (style/, chapters/, progress.json … live directly inside it), which is
	// what an output root looked like before projects were introduced.
	Legacy bool
}

// projectGroupFor decides the sidebar group of rel (a transcript path
// relative to the scan root) by looking at the tree, not just the path:
//
//	latex_project/测试-概率论/work/sessions/convert_01.jsonl -> latex_project/测试-概率论
//	latex_project/work/style_session.jsonl                    -> latex_project   (legacy root)
//	projA/x.jsonl                                             -> projA
//
// The middle case is why a plain "first segment" rule is not enough any
// more: with one directory per book, every book would collapse into the
// single group "latex_project" and the grouping would stop answering
// "which book is this session from". The second segment is only accepted
// when that directory really is a workspace (marker file or stage
// directories) and is not a structural name such as "work".
func (s *scanner) projectGroupFor(root, rel string) ProjectGroup {
	seg := strings.Split(rel, "/")
	if len(seg) < 2 || structuralNames[seg[0]] {
		// Either a transcript directly under the scan root, or one whose
		// first segment is a workspace internal — which happens when the
		// scan root already IS a project (docvision sessions --dir <proj>).
		// In both cases the root is the project; "work" is not a book name.
		return ProjectGroup{Name: RootProject, Legacy: isProjectWorkspace(root)}
	}
	name := seg[0]
	if len(seg) >= 3 && !structuralNames[seg[1]] {
		cand := filepath.Join(root, seg[0], seg[1])
		if isProjectWorkspace(cand) {
			return ProjectGroup{Name: seg[0] + "/" + seg[1], Legacy: false}
		}
	}
	return ProjectGroup{Name: name, Legacy: isProjectWorkspace(filepath.Join(root, seg[0]))}
}

// isProjectWorkspace reports whether dir looks like a docvision project
// workspace. The answer is cached: the live viewer rescans every 2 seconds
// and must not stat the same directories again on every poll.
func (s *scanner) isProjectWorkspace(dir string) bool {
	return s.workspace(dir)
}

// isProjectWorkspace is the uncached check (also used by tests).
func isProjectWorkspace(dir string) bool {
	for _, name := range projectMarkers {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// MetaInfo summarises the t="meta" lines of one transcript: what the model was
// told when a run started (system prompt + tool definitions). Meta lines are
// never replayed and are not messages, so they are counted separately and the
// sidebar uses their presence to say "this session has a prompt snapshot".
type MetaInfo struct {
	// Count is how many meta lines the transcript holds. A resumed session
	// writes a new one whenever the rendered system prompt changed, so a
	// transcript can legitimately carry several.
	Count int `json:"count"`
	// PromptChars is the character count of the newest system prompt.
	PromptChars int `json:"promptChars"`
	// Model is the model the newest run was sent to.
	Model string `json:"model,omitempty"`
	// SessionLabel is the newest run's session label (style, convert:…).
	SessionLabel string `json:"sessionLabel,omitempty"`
	// SystemSHA is the short hash the writer stored for the newest prompt.
	SystemSHA string `json:"systemSha,omitempty"`
	// Tools is how many tool definitions the newest meta line recorded.
	Tools int `json:"tools"`
}

// ToolSchema is one tool definition as recorded by a t="meta" line. Parameters
// keeps the JSON schema as raw text: the viewer only ever pretty-prints it, and
// keeping it a string means a malformed schema can never break the payload
// encoding of the whole page.
type ToolSchema struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  string `json:"parameters,omitempty"`
}

// UsageStats is the token/latency accounting of one API request (a t="usage"
// line) or, with Requests > 1, the aggregate over a whole session. All the
// derived numbers a reader wants — prefix-cache hit rate, average time to
// first token, output speed — are computed here so the viewer only formats
// them (and any JSONL consumer gets the same definitions).
type UsageStats struct {
	// Requests is how many API requests the numbers cover.
	Requests int `json:"requests"`
	// PromptTokens / Completion are the provider counters summed over the
	// covered requests; CachedTokens is the part of PromptTokens that hit the
	// provider prefix cache (0 when the provider does not report it).
	PromptTokens int `json:"promptTokens"`
	CachedTokens int `json:"cachedTokens"`
	Completion   int `json:"completionTokens"`
	// ReasoningTokens is the thinking share of Completion (when reported).
	ReasoningTokens int `json:"reasoningTokens,omitempty"`
	// DurationMS is the wall time (one request), or the sum over requests.
	DurationMS int64 `json:"durationMs"`
	// TTFTMS is the time to first streamed delta (or the average, in an
	// aggregate). AvgTTFTMS is the aggregate average.
	TTFTMS    int64 `json:"ttftMs,omitempty"`
	AvgTTFTMS int64 `json:"avgTtftMs,omitempty"`
	// AvgDurationMS is the aggregate average request duration.
	AvgDurationMS int64 `json:"avgDurationMs,omitempty"`
	// CacheHitPct is 100*CachedTokens/PromptTokens (0 when unknown).
	CacheHitPct float64 `json:"cacheHitPct,omitempty"`
	// OutputTPS is completion tokens per second of *generation* time
	// (duration minus TTFT, so queueing/thinking latency is not counted as
	// generation), summed over the covered requests.
	OutputTPS float64 `json:"outputTps,omitempty"`
	// FirstTS / LastTS bound the covered requests (transcripts that predate
	// timestamps leave these empty and the span unavailable).
	FirstTS string `json:"firstTs,omitempty"`
	LastTS  string `json:"lastTs,omitempty"`
	// SpanMS is LastTS - FirstTS when both are known (>0).
	SpanMS int64 `json:"spanMs,omitempty"`
	// Streamed is true when the covered requests used SSE streaming.
	Streamed bool `json:"streamed,omitempty"`
	// Round / Model / Finish describe a single request (empty in aggregates).
	Round  int    `json:"round,omitempty"`
	Model  string `json:"model,omitempty"`
	Finish string `json:"finish,omitempty"`
	// Kind names the request inside its session: "" ordinary turn,
	// "nudge" (forced text answer) or "compact" (context summarisation).
	Kind string `json:"kind,omitempty"`

	// genMS accumulates generation time for OutputTPS; not serialised.
	genMS int64
}

// add folds another set of counters (one request or another aggregate) in.
func (u *UsageStats) add(o UsageStats) {
	u.Requests += o.Requests
	u.PromptTokens += o.PromptTokens
	u.CachedTokens += o.CachedTokens
	u.Completion += o.Completion
	u.ReasoningTokens += o.ReasoningTokens
	u.DurationMS += o.DurationMS
	u.TTFTMS += o.TTFTMS
	u.genMS += o.genMS
	if o.Streamed {
		u.Streamed = true
	}
	if o.FirstTS != "" && (u.FirstTS == "" || o.FirstTS < u.FirstTS) {
		u.FirstTS = o.FirstTS
	}
	if o.LastTS > u.LastTS {
		u.LastTS = o.LastTS
	}
	if o.Model != "" {
		u.Model = o.Model
	}
}

// finish recomputes the derived numbers. It is called after any add so an
// aggregate always carries consistent ratios.
func (u *UsageStats) finish() {
	if u.Requests > 1 {
		u.AvgTTFTMS = u.TTFTMS / int64(u.Requests)
		u.AvgDurationMS = u.DurationMS / int64(u.Requests)
		u.Round = 0
		u.Finish = ""
	}
	if u.PromptTokens > 0 {
		u.CacheHitPct = float64(u.CachedTokens) * 100 / float64(u.PromptTokens)
	}
	if u.genMS > 0 {
		u.OutputTPS = float64(u.Completion) * 1000 / float64(u.genMS)
	}
	if u.FirstTS != "" && u.LastTS != "" && u.LastTS > u.FirstTS {
		if t0, err := time.Parse(time.RFC3339, u.FirstTS); err == nil {
			if t1, err := time.Parse(time.RFC3339, u.LastTS); err == nil {
				u.SpanMS = t1.Sub(t0).Milliseconds()
			}
		}
	}
}

// usageGenMS is the generation window of one request: total duration minus the
// wait for the first token, floored at 1ms so a same-millisecond reply still
// yields a finite speed instead of a division by zero.
func usageGenMS(durationMS, ttftMS int64) int64 {
	if durationMS <= ttftMS {
		return 1
	}
	return durationMS - ttftMS
}

// LiveWindow is how recent a transcript's mtime must be for the viewer to
// show it as "being written right now". The writer Syncs after every message,
// so a running session keeps its mtime fresh; 60s is long enough to survive a
// slow model turn and short enough to mean "this session is probably alive".
const LiveWindow = 60 * time.Second

// SessionInfo describes one transcript file found under the scan root.
type SessionInfo struct {
	// ID is the path relative to the scan root, slash separated. It is the
	// stable identifier used by the API and by the viewer's session list.
	ID string `json:"id"`
	// Label is the stage tag inferred from the file name, e.g. "style",
	// "chapters", "convert:chapter_01" or "vector:mind map".
	Label string `json:"label"`
	// Title is the same stage as a short Chinese display name ("样式",
	// "转换 · chapter_01", "矢量图 · mind map").
	Title string `json:"title"`
	// Name is the transcript file name shown in the sidebar.
	Name string `json:"name"`
	// Path is the absolute path of the JSONL transcript.
	Path string `json:"path"`
	// Messages counts the message lines (t == "msg"). That is what a reader
	// of the viewer thinks of as "the conversation"; meta lines and
	// unparseable lines are not messages.
	Messages int `json:"messages"`
	// Project is the sidebar group this session belongs to. It is the first
	// path segment under the scan root, extended to "<root>/<book>" when the
	// session sits inside a per-book project workspace of the multi-project
	// layout (see projectGroupFor).
	Project string `json:"project"`
	// ProjectLegacy marks a group whose directory is itself a workspace root,
	// i.e. the pre-multi-project layout. The sidebar labels it so an old
	// project is not mistaken for a book of the new layout.
	ProjectLegacy bool `json:"projectLegacy,omitempty"`
	// Meta summarises the t="meta" lines (what the model was told when this
	// run started). It is nil for a transcript written before meta lines
	// existed, which is why the viewer shows nothing for those.
	Meta *MetaInfo `json:"meta,omitempty"`
	// Bytes is the transcript size, which the live viewer compares between
	// polls to decide whether an incremental fetch is worth doing.
	Bytes int64 `json:"size"`
	// ModTime is the transcript mtime, shown as relative time.
	ModTime time.Time `json:"mtime"`
	// Live reports whether the transcript looks like it is being appended to
	// right now (mtime within LiveWindow).
	Live bool `json:"live"`
	// Stage / StageTitle group sessions by pipeline stage inside a project
	// ("vector", "convert", "checker", …) — the sidebar's second level.
	Stage      string `json:"stage,omitempty"`
	StageTitle string `json:"stageTitle,omitempty"`
	// ImageName / Page / ImageOrder / ImageType / ImageCaption describe WHICH
	// image a level-2 per-image session belongs to (doc_index): the MinerU
	// image base name, its page in the book, its position among the book's
	// images, the block type and the caption. They make per-image sessions
	// findable by image name / page / order / type instead of by hash.
	//
	// ImageLabel / ImageFile / ImageShort / ImagePath are the display side of
	// that identity. MinerU names extracted images by content hash and a
	// 64-character hash is not a name a reader can use, so the sidebar title and
	// --list show the caption, else the readable label the transcript file name
	// carries, else **ImageShort** ("bfeafce8.jpg": 8 characters of the hash, 12
	// when two images of the same book would collide, plus the original
	// extension). The full file name, the doc_index path
	// ("images/<book>/<file>") and the hash itself stay in the row tooltip and
	// in the search haystack, so no information is lost.
	ImageName    string `json:"imageName,omitempty"`
	ImageLabel   string `json:"imageLabel,omitempty"`
	ImageFile    string `json:"imageFile,omitempty"`
	ImageShort   string `json:"imageShort,omitempty"`
	ImagePath    string `json:"imagePath,omitempty"`
	ImageOrder   int    `json:"imageOrder,omitempty"`
	Page         int    `json:"page,omitempty"`
	ImageType    string `json:"imageType,omitempty"`
	ImageCaption string `json:"imageCaption,omitempty"`
	// ProjectStages is the project's progress.json (stage → status) so the
	// sidebar can show how far the pipeline got, not just which sessions exist.
	ProjectStages map[string]string `json:"projectStages,omitempty"`
	// Cost is the money this session's usage added up to, filled by
	// ApplyPrices when the config has rates for the model; nil when unpriced
	// (the viewer shows no cost rather than ¥0).
	Cost *CostStats `json:"cost,omitempty"`
	// Stats aggregates the t="usage" lines: tokens in/out, prefix-cache hit
	// rate, latency, TTFT and output speed. Nil for transcripts written before
	// usage recording existed (the viewer then shows no metrics rather than
	// zeros that look like a measurement).
	Stats *UsageStats `json:"stats,omitempty"`
}

// Line is one transcript line, verbatim plus parsed. Bad is set when the line
// is not valid JSON — a transcript can contain a torn line or an unrelated
// format, and one bad line must never hide the rest of the session.
type Line struct {
	// N is the 1-based line number inside the transcript file.
	N int `json:"n"`
	// Raw is the original line, so the viewer can still show something for a
	// line the parser could not understand. On the wire it is a JSON string
	// (see MarshalJSON): a json.RawMessage must be valid JSON, and an
	// unparseable line is precisely the case this field exists for.
	Raw json.RawMessage `json:"raw,omitempty"`
	// Type is the transcript's "t" field ("msg" for every message the writer
	// produces; other values are kept and ignored by the viewer).
	Type string `json:"t,omitempty"`
	// Role is user/assistant/tool.
	Role string `json:"role,omitempty"`
	// Text is the message body (for multipart messages, the concatenated text
	// parts — image parts become Images entries instead).
	Text string `json:"text,omitempty"`
	// Images holds the "file://media/<name>" references; the referenced files
	// live in the media/ directory next to the transcript.
	Images []string `json:"images,omitempty"`
	// Calls are the assistant's tool calls.
	Calls []session.ToolCall `json:"tool_calls,omitempty"`
	// CallID links a tool result back to the assistant's tool call.
	CallID string `json:"tool_call_id,omitempty"`
	// Reasoning is the provider's reasoning_content (思维链).
	Reasoning string `json:"reasoning_content,omitempty"`
	// Bad marks a line that could not be parsed as JSON.
	Bad bool `json:"bad,omitempty"`

	// ---- t == "meta" lines only (never replayed messages) ----
	// A meta line records what the model was told when a run started. It is
	// not part of the conversation: it has no role, it is not counted as a
	// message, and the viewer renders it as one card above the timeline
	// rather than as a message.
	Kind string `json:"kind,omitempty"`
	// SessionLabel is the meta line's "session_label" (style, convert:…).
	SessionLabel string `json:"session_label,omitempty"`
	// Model is the model the run was sent to.
	Model string `json:"model,omitempty"`
	// SystemSHA is the writer's short hash of the system prompt.
	SystemSHA string `json:"system_sha,omitempty"`
	// Tools are the tool definitions that were sent with this run.
	Tools []ToolSchema `json:"tools,omitempty"`

	// ---- timestamps and t == "usage" lines ----
	// TS is when the line was written (RFC3339 ms), empty in transcripts
	// produced before timestamps existed.
	TS string `json:"ts,omitempty"`
	// Stats carries one request's accounting for t == "usage" lines: the
	// viewer sums them into the session metrics (tokens in/out, prefix-cache
	// hit rate, latency, TTFT, output speed).
	Stats *UsageStats `json:"stats,omitempty"`
}

// IsMeta reports whether this line is a t="meta" record rather than a message.
// The viewer keys every "meta versus message" decision off this so a transcript
// that gains new non-"msg" types later still renders as messages only.
func (l Line) IsMeta() bool { return l.Type == "meta" }

// IsUsage reports whether this line is a t="usage" record (one API request's
// token accounting and latency). Like meta lines it is never a message.
func (l Line) IsUsage() bool { return l.Type == "usage" }

// MarshalJSON is what keeps a broken line deliverable. json.RawMessage
// validates its bytes while encoding and refuses anything that is not valid
// JSON, so a torn line would make the whole page payload unencodable (the
// static export and both API endpoints all marshal Lines). The shadow type
// drops the promoted Raw field so the string declared here wins, and every
// other field is still produced by the struct tags.
func (l Line) MarshalJSON() ([]byte, error) {
	type shadow Line
	return json.Marshal(struct {
		shadow
		Raw string `json:"raw,omitempty"`
	}{shadow: shadow(l), Raw: string(l.Raw)})
}

// LabelFor infers the pipeline stage tag from a transcript path. The names are
// produced by internal/latex:
//
//	work/style_session.jsonl                          -> style
//	work/sessions/chapters.jsonl                      -> chapters
//	work/sessions/convert_<章>.jsonl                  -> convert:<章>
//	<proj>/source/sessions/vector_<书名>__<sha>__<图>.jsonl -> vector:<图>
//
// Anything else keeps its base name so an unknown file stays identifiable.
func LabelFor(rel string) string {
	base := transcriptStem(rel)
	switch base {
	case "":
		return "会话"
	case "style_session", "style":
		return "style"
	case "chapters":
		return "chapters"
	}
	for _, p := range []struct{ prefix, label string }{
		{"convert_", "convert"},
		{"checker_", "checker"},
		{"style_fix_", "style-fix"},
		{"vector_", "vector"},
		{"figure_check_", "figure-check"},
	} {
		if strings.HasPrefix(base, p.prefix) && len(base) > len(p.prefix) {
			return p.label + ":" + stageSubject(p.label, strings.TrimPrefix(base, p.prefix))
		}
	}
	return "会话:" + base
}

// TitleFor renders the label above as a short Chinese display name for the
// sidebar and the --list table.
func TitleFor(rel string) string {
	label := LabelFor(rel)
	stage, subject, ok := strings.Cut(label, ":")
	if !ok {
		switch label {
		case "style":
			return "样式"
		case "chapters":
			return "章节划分"
		case "会话":
			return "会话"
		}
		return label
	}
	switch stage {
	case "convert":
		return "转换 · " + subject
	case "vector":
		return "矢量图 · " + subject
	case "checker":
		return "核对 · " + subject
	case "style-fix":
		return "样式修复 · " + subject
	case "figure-check":
		return "逐图校验 · " + subject
	case "会话":
		return subject
	}
	return label
}

// transcriptStem returns the transcript's file name without its extension.
func transcriptStem(rel string) string {
	return strings.TrimSuffix(path.Base(filepath.ToSlash(rel)), ".jsonl")
}

// stageSubject turns the file-name remainder into a readable subject. Per-image
// transcripts are named <prefix>_<书名>__<图片名>__<图label> (vector_ for the
// drawing session, figure_check_ for its verification): the book name and the
// image name are noisy for a sidebar row, and the label itself uses underscores
// where the caption had spaces.
func stageSubject(stage, rest string) string {
	if stage != "vector" && stage != "figure-check" {
		return rest
	}
	var keep []string
	for _, part := range strings.Split(rest, "__") {
		if isHashSegment(part) {
			continue
		}
		if strings.TrimSpace(part) == "" {
			continue
		}
		keep = append(keep, part)
	}
	if len(keep) == 0 {
		return rest
	}
	return strings.Join(strings.Fields(strings.ReplaceAll(keep[len(keep)-1], "_", " ")), " ")
}

// isHashSegment reports whether a name segment is a hex digest (a content hash,
// not something a human asked to see).
func isHashSegment(s string) bool {
	if len(s) < 16 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// Scan walks root recursively and describes every *.jsonl transcript found,
// newest first (live sessions therefore float to the top), tie-broken by ID so
// the order is stable across calls.
//
// Two kinds of entries are skipped on purpose: directories named media/ (they
// hold the image payloads referenced by the transcripts, never transcripts
// themselves) and .git (never a session store). A single unreadable
// subdirectory does not abort the scan.
func Scan(root string) ([]SessionInfo, error) {
	return (&scanner{root: root}).scan()
}

// scanner caches per-file message counts between scans so the live server's
// 2-second polling does not re-read transcripts that did not change.
type scanner struct {
	root   string
	mu     sync.Mutex
	counts map[string]countEntry
	// ws caches "is this directory a project workspace" between polls.
	ws map[string]bool
}

// workspace caches the workspace check (see isProjectWorkspace).
func (s *scanner) workspace(dir string) bool {
	s.mu.Lock()
	if v, ok := s.ws[dir]; ok {
		s.mu.Unlock()
		return v
	}
	s.mu.Unlock()
	v := isProjectWorkspace(dir)
	s.mu.Lock()
	if s.ws == nil {
		s.ws = map[string]bool{}
	}
	s.ws[dir] = v
	s.mu.Unlock()
	return v
}

type countEntry struct {
	size    int64
	modTime time.Time
	stat    transcriptStat
}

// transcriptStat is what one streaming pass over a transcript yields: the
// message count the sidebar shows, plus a summary of its meta lines.
type transcriptStat struct {
	Messages int
	Meta     *MetaInfo
	// Usage aggregates the t="usage" lines seen in the same single pass.
	Usage *UsageStats
}

func (s *scanner) scan() ([]SessionInfo, error) {
	root, err := filepath.Abs(s.root)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("会话预览: 无法读取目录 %s: %w", s.root, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("会话预览: %s 不是目录", s.root)
	}

	var out []SessionInfo
	now := time.Now()
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip what we cannot read and keep going: one broken branch of
			// the tree must not hide every other session.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && (d.Name() == "media" || d.Name() == ".git") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		stat := s.fileStat(p, info.Size(), info.ModTime())
		grp := s.projectGroupFor(root, rel)
		out = append(out, SessionInfo{
			ID:            rel,
			Label:         LabelFor(rel),
			Title:         TitleFor(rel),
			Name:          d.Name(),
			Path:          p,
			Project:       grp.Name,
			ProjectLegacy: grp.Legacy,
			Messages:      stat.Messages,
			Meta:          stat.Meta,
			Stats:         stat.Usage,
			Bytes:         info.Size(),
			ModTime:       info.ModTime(),
			Live:          now.Sub(info.ModTime()) < LiveWindow,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	s.enrichSessions(root, out)
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ModTime.Equal(out[j].ModTime) {
			return out[i].ModTime.After(out[j].ModTime)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// fileStat returns the transcript's message count and meta summary, reusing the
// cached value while (size, mtime) is unchanged. Meta lines change the summary
// but never the message count.
func (s *scanner) fileStat(p string, size int64, modTime time.Time) transcriptStat {
	s.mu.Lock()
	if e, ok := s.counts[p]; ok && e.size == size && e.modTime.Equal(modTime) {
		s.mu.Unlock()
		return e.stat
	}
	s.mu.Unlock()

	stat, err := readStat(p)
	if err != nil {
		return transcriptStat{}
	}
	s.mu.Lock()
	if s.counts == nil {
		s.counts = map[string]countEntry{}
	}
	s.counts[p] = countEntry{size: size, modTime: modTime, stat: stat}
	s.mu.Unlock()
	return stat
}

// readStat streams the file once and counts "t":"msg" lines without building
// the parsed messages (transcripts are append-only text, so a single pass is
// enough and nothing is materialised). Meta lines are summarised on the way,
// with the newest one winning: a resumed run re-writes the prompt, and the run
// that is actually ongoing is the one a reader cares about.
func readStat(p string) (transcriptStat, error) {
	f, err := os.Open(p)
	if err != nil {
		return transcriptStat{}, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 64*1024)
	var stat transcriptStat
	for {
		text, rerr := br.ReadString('\n')
		trimmed := strings.TrimSpace(text)
		if trimmed != "" {
			// Only the type is decoded for every line: messages are the vast
			// majority and must stay cheap to skip.
			var head struct {
				T string `json:"t"`
			}
			if json.Unmarshal([]byte(trimmed), &head) == nil {
				switch head.T {
				case "msg":
					stat.Messages++
				case "meta":
					stat.addMeta(trimmed)
				case "usage":
					stat.addUsage(trimmed)
				}
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return stat, nil
			}
			return stat, rerr
		}
	}
}

// addUsage folds one t="usage" line into the session aggregate. A line that
// cannot be parsed is skipped: a torn tail must never hide the metrics of the
// requests that did complete.
func (stat *transcriptStat) addUsage(line string) {
	var rec transcriptLine
	if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.T != "usage" {
		return
	}
	one := UsageStats{
		Requests: 1,
		// Model 必须带上：费用报告按 t="usage" 行里的**厂商模型名**查价格表，
		// 少了它每次会话都"查无此价"，金额永远是 0（真实缺陷：聚合里漏字段，
		// 于是配好的价格一个也用不上）。
		Model:           rec.Model,
		PromptTokens:    rec.PromptTokens,
		CachedTokens:    rec.CachedTokens,
		Completion:      rec.Completion,
		ReasoningTokens: rec.ReasoningTokens,
		DurationMS:      rec.DurationMS,
		TTFTMS:          rec.TTFTMS,
		Streamed:        rec.Stream != nil && *rec.Stream,
		FirstTS:         rec.TS,
		LastTS:          rec.TS,
	}
	one.genMS = usageGenMS(rec.DurationMS, rec.TTFTMS)
	if stat.Usage == nil {
		stat.Usage = &UsageStats{}
	}
	stat.Usage.add(one)
	stat.Usage.finish()
}

// addMeta folds one meta line into the summary. A meta line that cannot be
// decoded is ignored: the count shown to the reader must never be inflated by
// data the viewer cannot explain.
func (stat *transcriptStat) addMeta(line string) {
	var rec struct {
		Model        string            `json:"model"`
		SessionLabel string            `json:"session_label"`
		SysHash      string            `json:"system_sha"`
		Text         string            `json:"text"`
		Tools        []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal([]byte(line), &rec) != nil {
		return
	}
	if stat.Meta == nil {
		stat.Meta = &MetaInfo{}
	}
	stat.Meta.Count++
	stat.Meta.PromptChars = utf8.RuneCountInString(rec.Text)
	stat.Meta.Model = rec.Model
	stat.Meta.SessionLabel = rec.SessionLabel
	stat.Meta.SystemSHA = rec.SysHash
	stat.Meta.Tools = len(rec.Tools)
}

// HumanSize renders a byte count for terminal output and the sidebar.
func HumanSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.2f MB", float64(bytes)/1024/1024)
	}
}

// HumanCount renders a token count compactly (12.3k / 1.20M), used by the
// --list table and anywhere a raw count would be unreadable.
func HumanCount(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.2fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ReadSession returns the transcript lines starting at fromLine, which is the
// number of lines the caller already has: 0 reads from the beginning, and
// passing the previously returned nextFrom fetches only what was appended
// since. nextFrom is the line count after this read, so the pair
// (lines, nextFrom) is enough for a poller to stay in sync.
//
// Unparseable lines are returned with Bad set. A trailing fragment without a
// newline that does not parse is treated as a half-written line and left
// uncounted, so the next poll reads it again instead of losing it forever.
func ReadSession(filePath string, fromLine int) ([]Line, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fromLine, err
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, 64*1024)
	n := 0
	var out []Line
	for {
		text, rerr := br.ReadString('\n')
		terminated := rerr == nil
		if text == "" && rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return out, n, rerr
		}
		raw := strings.TrimRight(text, "\r\n")
		line, ok := parseLine(n+1, raw)
		if !ok && !terminated {
			// Torn tail: the writer had not finished this line yet.
			break
		}
		n++
		if n > fromLine {
			out = append(out, line)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return out, n, rerr
		}
	}
	return out, n, nil
}

// parseLine decodes one transcript line. The second result is false when the
// line is empty or is not a JSON object — callers distinguish "bad data to
// report" from "the file ends in the middle of a write".
func parseLine(n int, raw string) (Line, bool) {
	line := Line{N: n}
	if strings.TrimSpace(raw) == "" {
		return line, true
	}
	line.Raw = json.RawMessage(raw)
	var rec transcriptLine
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		line.Bad = true
		return line, false
	}
	line.Type = rec.T
	line.Role = rec.Role
	line.Text = rec.Text
	line.Images = rec.Images
	line.Calls = rec.Calls
	line.CallID = rec.CallID
	line.Reasoning = rec.Reasoning
	line.TS = rec.TS
	if rec.T == "usage" {
		streamed := rec.Stream != nil && *rec.Stream
		line.Stats = &UsageStats{
			Requests:        1,
			PromptTokens:    rec.PromptTokens,
			CachedTokens:    rec.CachedTokens,
			Completion:      rec.Completion,
			ReasoningTokens: rec.ReasoningTokens,
			DurationMS:      rec.DurationMS,
			TTFTMS:          rec.TTFTMS,
			Round:           rec.Round,
			Model:           rec.Model,
			Finish:          rec.Finish,
			Kind:            rec.Kind,
			Streamed:        streamed,
		}
		line.Stats.genMS = usageGenMS(rec.DurationMS, rec.TTFTMS)
		line.Stats.FirstTS = rec.TS
		line.Stats.LastTS = rec.TS
		line.Stats.finish()
	}
	if rec.T == "meta" {
		// A meta line reuses "text" for the system prompt, but it is not a
		// message: the viewer keeps the meta view of it separately so a
		// "message" is never a 40k-character prompt.
		line.Kind = rec.Kind
		line.SessionLabel = rec.SessionLabel
		line.Model = rec.Model
		line.SystemSHA = rec.SysHash
		for _, t := range rec.Tools {
			line.Tools = append(line.Tools, ToolSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  strings.TrimSpace(string(t.Parameters)),
			})
		}
	}
	return line, true
}

// transcriptLine mirrors internal/session's on-disk record. It is duplicated
// on purpose: the viewer must tolerate unknown fields and drift, and it must
// not silently start depending on writer internals it does not own. The meta
// fields below are the same deliberate copy of the writer's t="meta" shape.
type transcriptLine struct {
	T         string             `json:"t"`
	Role      string             `json:"role,omitempty"`
	Text      string             `json:"text,omitempty"`
	Images    []string           `json:"images,omitempty"`
	Calls     []session.ToolCall `json:"tool_calls,omitempty"`
	CallID    string             `json:"tool_call_id,omitempty"`
	Reasoning string             `json:"reasoning_content,omitempty"`

	// ---- t == "meta" lines only ----
	Kind         string           `json:"kind,omitempty"`
	SessionLabel string           `json:"session_label,omitempty"`
	Model        string           `json:"model,omitempty"`
	SysHash      string           `json:"system_sha,omitempty"`
	Tools        []transcriptTool `json:"tools,omitempty"`

	// TS is the line write time (every line since usage recording exists).
	TS string `json:"ts,omitempty"`

	// ---- t == "usage" lines only ----
	Stream          *bool  `json:"stream,omitempty"`
	Round           int    `json:"round,omitempty"`
	PromptTokens    int    `json:"prompt_tokens,omitempty"`
	CachedTokens    int    `json:"cached_tokens,omitempty"`
	Completion      int    `json:"completion_tokens,omitempty"`
	ReasoningTokens int    `json:"reasoning_tokens,omitempty"`
	DurationMS      int64  `json:"duration_ms,omitempty"`
	TTFTMS          int64  `json:"ttft_ms,omitempty"`
	Finish          string `json:"finish_reason,omitempty"`
}

// transcriptTool is one tool definition inside a meta line. Parameters stays a
// json.RawMessage here so the raw bytes survive decoding; it is handed to the
// viewer as a string afterwards.
type transcriptTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}
