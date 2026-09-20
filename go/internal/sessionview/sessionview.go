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
	"crypto/sha256"
	"encoding/hex"

	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	// 旧版单项目 = 组目录本身是工作区（work/、progress.json 直接挂在它
	// 下面）。但现代多项目布局的书目录**同样**是工作区——区别在转录的
	// 落点：现代布局收在工作区自己的 work/sessions/ 或 source/sessions/
	// 里，旧版是 work/<file>.jsonl、sessions/<file>.jsonl 直接躺着。
	// 不区分这一点，每本新书都会被误挂「旧版单项目」徽标（T13）。
	legacy := isProjectWorkspace(filepath.Join(root, seg[0]))
	if len(seg) >= 3 && (seg[1] == "work" || seg[1] == "source") && seg[2] == "sessions" {
		legacy = false
	}
	return ProjectGroup{Name: name, Legacy: legacy}
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
	// PromptTokenEst is the LOCAL token estimate of that same prompt (never a
	// provider number; the viewer marks it with ≈).
	PromptTokenEst int `json:"promptTokenEst"`
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
	// DescTokens / ParamTokens are the LOCAL token estimates of the description
	// and of the parameter schema (the viewer shows them with a ≈ prefix).
	DescTokens  int `json:"descTokens,omitempty"`
	ParamTokens int `json:"paramTokens,omitempty"`
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
	// 不带 omitempty：前端 sidebar/details 直接 .toFixed()，0 命中率被省略
	// 时会 undefined 崩掉整栏渲染（T27 实测：28 个零缓存会话空白侧栏）。
	CacheHitPct float64 `json:"cacheHitPct"`
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
// slow model turn (thinking + rate-limit backoff routinely exceed a minute
// without any append) and still short enough to mean "probably alive".
const LiveWindow = 3 * time.Minute

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
	// Board (P18) is "img2text" for sessions from the img2text extra root
	// (empty = the latex/main board).
	Board string `json:"board,omitempty"`
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
	// SHA is the transcript's content fingerprint (P10, 12 hex) — shown in
	// the details panel so a reported problem pins the exact session state.
	SHA string `json:"sha,omitempty"`
	// Live reports whether the transcript looks like it is being appended to
	// right now (mtime within LiveWindow).
	Live bool `json:"live"`
	// EndState is the per-session completion signal for the sidebar's block
	// view (T22): "done" = a SUBMIT receipt exists, "error" = work happened
	// (tool receipts) but nothing was ever submitted, "" = not started yet.
	// Live overrides both visually (green pulse beats everything).
	EndState string `json:"endState,omitempty"`
	// ImgShortKeep (T37) is the hash-keep length the img2text grouping
	// picked for this session's short image name (8, escalated to 12 on
	// collision). Zero = default 8. Server-side only.
	ImgShortKeep int `json:"-"`
	// ChapterOrder is the chapter number carried by the transcript file name
	// ("convert_chapter_003.jsonl" → 3), the block-view badge for chapter
	// sessions the way ImageOrder numbers per-image ones.
	ChapterOrder int `json:"chapterOrder,omitempty"`
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
	// Estimate is the per-image token rule that applies to THIS session's
	// model, plus the measured per-image value when the usage lines allow
	// deriving one. The details pane states the rule it actually used instead
	// of keeping a copy that could go stale.
	Estimate *SessionEstimate `json:"estimate,omitempty"`
}

// SessionEstimate describes, for one session, how the local per-image token
// estimate is made and — when the vendor's usage lines allow it — what an image
// actually cost.
type SessionEstimate struct {
	// Model is the wire model id the rule was resolved for ("" = the global
	// estimate block, used by transcripts that record no model).
	Model string `json:"model,omitempty"`
	// Method / Tokens / PxPerToken / Min / Max are the resolved parameters of
	// that rule.
	Method     string `json:"method"`
	Tokens     int    `json:"tokens"`
	PxPerToken int    `json:"pxPerToken"`
	Min        int    `json:"min"`
	Max        int    `json:"max"`
	// Rule is the same thing in one line, e.g. "fixed 1100/张" or
	// "pixels 750px per token（85–4096）".
	Rule string `json:"rule"`
	// MeasuredPerImage is a per-image cost DERIVED FROM THE VENDOR's
	// prompt_tokens (see measuredImageTokens), 0 when no usable sample exists.
	// MeasuredSamples counts the request-to-request steps it came from and
	// MeasuredImages how many images those steps added.
	MeasuredPerImage int `json:"measuredPerImage,omitempty"`
	MeasuredSamples  int `json:"measuredSamples,omitempty"`
	MeasuredImages   int `json:"measuredImages,omitempty"`
}

// estimateInfoFor resolves the rule for one session and adds whatever the
// usage lines can say about the real per-image cost.
func estimateInfoFor(stat transcriptStat) *SessionEstimate {
	model := stat.modelID()
	rule := session.EstimateForModel(model)
	out := &SessionEstimate{
		Model:      model,
		Method:     rule.Method,
		Tokens:     rule.Tokens,
		PxPerToken: rule.PxPerToken,
		Min:        rule.MinTokens,
		Max:        rule.MaxTokens,
		Rule:       rule.Describe(),
	}
	out.MeasuredPerImage, out.MeasuredSamples, out.MeasuredImages = measuredImageTokens(stat.imageSamples)
	return out
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
	// Est carries the LOCAL token estimate of this line's text (and of its
	// images), computed with the same estimator the context threshold uses.
	// Everything under Est is an estimate and the viewer marks it with ≈;
	// Stats above is the provider's own, exact number.
	Est *LineEstimate `json:"est,omitempty"`
}

// LineEstimate is the local estimate of one transcript line: what the line's
// text (plus tool arguments, reasoning and images) costs in prompt tokens. It is
// what the viewer shows when the display unit is "token" — always with a ≈
// prefix, because no provider ever reported these numbers.
type LineEstimate struct {
	// Text estimates the line's Text; Reasoning its reasoning_content.
	Text      int `json:"text,omitempty"`
	Reasoning int `json:"reasoning,omitempty"`
	// Calls estimates the arguments of each tool call, parallel to Calls.
	Calls []int `json:"calls,omitempty"`
	// Images / ImageCount estimate the attached images (by pixel size when the
	// media file's dimensions are readable) and how many there are.
	Images     int `json:"images,omitempty"`
	ImageCount int `json:"imageCount,omitempty"`
}

// Tokens returns the line's total local estimate (text + reasoning + tool
// arguments + images); the trajectory table's token column shows it.
func (e LineEstimate) Tokens() int {
	total := e.Text + e.Reasoning + e.Images
	for _, c := range e.Calls {
		total += c
	}
	return total
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

// ScanWith scans root plus every extra directory (T37 img2text sessions).
func ScanWith(root string, extras []ScanExtra) ([]SessionInfo, error) {
	return (&scanner{root: root, extras: extras}).scan()
}

// scanner caches per-file message counts between scans so the live server's
// 2-second polling does not re-read transcripts that did not change.
type scanner struct {
	root   string
	extras []ScanExtra
	// img2Text accumulates the sessions found in img2text extra roots
	// during one scan, so the numbering pass can index them per group.
	img2Text []img2TextEntry
	mu       sync.Mutex
	counts   map[string]countEntry
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
	// SHA is the content hash of the whole transcript file (P10, 12 hex of
	// SHA-256 over the raw bytes) — the session's fingerprint at this size/
	// mtime; a user can quote it to pin the exact state they saw.
	SHA string
	// Usage aggregates the t="usage" lines seen in the same single pass.
	Usage *UsageStats
	// SawTool / SawSubmit record whether any tool receipt (role=tool) appeared
	// and whether one of them was a SUBMIT receipt (the canonical completion
	// signal every submit tool writes: "SUBMITTED. …"). Both are two raw-line
	// substring checks in the same single pass — no extra decoding. Together
	// they give the sidebar's block view a per-session end state:
	//   SawSubmit        → done（正常结束）
	//   SawTool && !SawSubmit → error（干过活但没交——错误终止/半途而废）
	//   neither          → pending（领了任务还没开工）
	// T56：img2text 逐图分析是一次性会话，完成信号不是 SUBMIT 而是最终
	// 答案里的 "[IMG_TYPE:" 标记——模型偶尔调 getmorecontext 工具时会被
	// "SawTool && !SawSubmit" 误判成 error（方块视图整片爆红）。
	// SawTyped = assistant 行含 [IMG_TYPE:，与 SawSubmit 同为 done 信号。
	SawTool   bool
	SawSubmit bool
	SawTyped  bool
	// model is the wire model id recorded by a meta or usage line; it decides
	// which per-image token rule the page quotes (and applies).
	model string
	// imageSamples are the per-request numbers the measured per-image cost is
	// derived from (see measuredImageTokens).
	imageSamples []imageUsageSample
}

// modelID is the model this transcript was sent to, "" when no line recorded
// one (transcripts written before meta/usage lines existed).
func (stat transcriptStat) modelID() string {
	if stat.Usage != nil && stat.Usage.Model != "" {
		return stat.Usage.Model
	}
	if stat.Meta != nil {
		return stat.Meta.Model
	}
	return stat.model
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
	// T38：WalkDir 只收集候选文件，真正的读文件（readStat：消息数 + 内容
	// SHA + 用量聚合）放到 walk 之后**并行**做。网络盘（samba）上每次
	// open/read 的往返 latency 是秒级扫描的主因，串行读 138 个转录会把
	// 首页 /api/index 拖住很久；并发后 wall time 近似 latency 而不是
	// latency × 文件数。会话顺序按下标保留。
	type candidate struct {
		full string
		name string
		size int64
		mt   time.Time
	}
	var candidates []candidate
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
		candidates = append(candidates, candidate{full: p, name: d.Name(), size: info.Size(), mt: info.ModTime()})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	stats := make([]transcriptStat, len(candidates))
	var wg sync.WaitGroup
	workers := 16
	if len(candidates) < workers {
		workers = len(candidates)
	}
	jobs := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				stats[i] = s.fileStat(candidates[i].full, candidates[i].size, candidates[i].mt)
			}
		}()
	}
	for i := range candidates {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	for i, c := range candidates {
		rel, rerr := filepath.Rel(root, c.full)
		if rerr != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		stat := stats[i]
		grp := s.projectGroupFor(root, rel)
		out = append(out, SessionInfo{
			ID:            rel,
			Label:         LabelFor(rel),
			Title:         TitleFor(rel),
			Name:          c.name,
			Path:          c.full,
			Project:       grp.Name,
			ProjectLegacy: grp.Legacy,
			Messages:      stat.Messages,
			Meta:          stat.Meta,
			Stats:         stat.Usage,
			Estimate:      estimateInfoFor(stat),
			Bytes:         c.size,
			ModTime:       c.mt,
			SHA:           stat.SHA,
			Live:          now.Sub(c.mt) < LiveWindow,
			EndState:      liveEndState(endStateOf(stat), now.Sub(c.mt) < LiveWindow),
			ChapterOrder:  chapterOrderOf(c.name),
		})
	}

	s.img2Text = s.img2Text[:0]
	out = s.scanExtras(out, now)
	s.numberImg2Text(out)

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

// liveEndState 压掉运行中会话的 error 终态：SawTool && !SawSubmit 本意是
// "干过活但没交——错误终止/半途而废"，但正在等模型回复（思考/限流退避）的
// 会话同样是"干过活还没交"。live 的绿脉冲原本盖住视觉，60s 窗口一掉红色就
// 露出来、下一笔写入又变绿——"运行中/失败来回跳"即是它。live（含窗口内
// 余晖）期间 error 不算数；done 是真提交，保留。
func liveEndState(state string, live bool) string {
	if live && state == "error" {
		return ""
	}
	return state
}

// endStateOf folds the receipt signals into the block-view completion state.
func endStateOf(stat transcriptStat) string {
	switch {
	case stat.SawSubmit || stat.SawTyped:
		return "done"
	case stat.SawTool:
		return "error"
	}
	return ""
}

// chapterOrderOf extracts the chapter number from a transcript file name
// ("convert_chapter_003.jsonl" → 3, "checker-chapter-12" → 12); 0 when the
// name carries none (style/chapters/vector sessions).
func chapterOrderOf(name string) int {
	base := strings.TrimSuffix(name, path.Ext(name))
	if m := chapterOrderRe.FindStringSubmatch(base); m != nil {
		n, err := strconv.Atoi(m[1])
		if err == nil {
			return n
		}
	}
	return 0
}

// chapterOrderRe matches the chapter number in convert/checker/style-fix/
// figure-check transcript names. The "chapter" word keeps it from misreading
// unrelated digits (hashes, dates) in other session kinds.
var chapterOrderRe = regexp.MustCompile(`chapter[_-]?0*(\d+)$`)

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
	h := sha256.New()
	var stat transcriptStat
	for {
		text, rerr := br.ReadString('\n')
		h.Write([]byte(text))
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
					// T22：原始行子串检查（零额外解码）——tool 回执行必含
					// `"role":"tool"`（写入方是紧凑 JSON），submit 回执以
					// SUBMITTED 开头；assistant 的正常文本只会写 "Submitted"。
					if strings.Contains(trimmed, `"role":"tool"`) {
						stat.SawTool = true
						if strings.Contains(trimmed, "SUBMITTED") {
							stat.SawSubmit = true
						}
					}
					if strings.Contains(trimmed, `"role":"assistant"`) && strings.Contains(trimmed, "[IMG_TYPE:") {
						stat.SawTyped = true
					}
				case "meta":
					stat.addMeta(trimmed)
				case "usage":
					stat.addUsage(trimmed)
				}
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				stat.SHA = hex.EncodeToString(h.Sum(nil))[:12]
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
	if rec.Model != "" {
		stat.model = rec.Model
	}
	stat.imageSamples = append(stat.imageSamples, imageUsageSample{
		prompt: rec.PromptTokens,
		text:   rec.TextTokens,
		images: rec.ImageCount,
		// A usage line without the local half (written before those fields
		// existed) cannot be part of a measurement.
		usable: rec.Round > 0 && rec.TextTokens > 0,
	})
	if stat.Usage == nil {
		stat.Usage = &UsageStats{}
	}
	stat.Usage.add(one)
	stat.Usage.finish()
}

// imageUsageSample is one request's numbers, as far as the measured per-image
// cost needs them: the vendor's prompt_tokens, the local text-only estimate and
// how many images the request carried.
type imageUsageSample struct {
	prompt int
	text   int
	images int
	usable bool
}

// measuredImageTokens derives the MEASURED per-image token cost from the usage
// lines of one session. Between two consecutive requests the vendor's
// prompt_tokens grows by the text that was added plus the images that were
// added, so
//
//	per-image = (Δprompt_tokens − Δtext_estimate) / Δimages
//
// The text part comes from the same local estimator that fills the ≈ numbers
// (recorded on the usage line as text_tokens), never from a second formula.
//
// What it cannot see, and therefore skips: a step that added no image, a step
// whose request SHRANK (local pruning or an AI compaction removed content —
// the delta then measures the removal, not an image), and usage lines written
// before the local half was recorded. It also stays a MEASUREMENT, not a rule:
// gateway-reported prompt_tokens move between steps of the same session (real
// logs: the same 900x1272 page render measured anywhere between ~630 and ~1800
// tokens on one glm endpoint), so the result is the MEDIAN of the usable steps
// and the caller reports how many steps and images it came from.
func measuredImageTokens(samples []imageUsageSample) (perImage, steps, images int) {
	var per []int
	for i := 0; i+1 < len(samples); i++ {
		a, b := samples[i], samples[i+1]
		if !a.usable || !b.usable || a.prompt <= 0 || b.prompt <= 0 {
			continue
		}
		dPrompt := b.prompt - a.prompt
		dText := b.text - a.text
		dImages := b.images - a.images
		if dImages <= 0 || dPrompt <= 0 || dText < 0 {
			continue
		}
		v := (dPrompt - dText) / dImages
		if v <= 0 {
			continue
		}
		per = append(per, v)
		images += dImages
	}
	if len(per) == 0 {
		return 0, 0, 0
	}
	sort.Ints(per)
	mid := len(per) / 2
	if len(per)%2 == 1 {
		return per[mid], len(per), images
	}
	return (per[mid-1] + per[mid]) / 2, len(per), images
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
	stat.Meta.PromptTokenEst = session.TextTokens(rec.Text)
	stat.Meta.Model = rec.Model
	if rec.Model != "" {
		stat.model = rec.Model
	}
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

// HumanTokenEst renders a LOCAL token estimate for terminal output (≈ 1.5k).
// Same form as the viewer's fmtTokens (assets/viewer.js): k below a million,
// two decimals of M above it, and the ≈ prefix is part of the value because a
// local estimate must never be mistaken for a provider number. Provider
// counts go through HumanCount instead — they are exact.
func HumanTokenEst(n int) string {
	switch {
	case n >= 1000000:
		return "≈ " + fmt.Sprintf("%.2fM", float64(n)/1000000)
	case n >= 10000:
		return "≈ " + fmt.Sprintf("%.0fk", float64(n)/1000)
	case n >= 1000:
		return "≈ " + fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return "≈ " + fmt.Sprintf("%d", n)
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

	// The per-image rule depends on the model the session talks to, and the
	// transcript records it on its meta (or first usage) line.
	model := sessionModelOf(filePath)

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
			estimateLine(&line, filePath, model)
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
				DescTokens:  session.TextTokens(t.Description),
				ParamTokens: session.TextTokens(string(t.Parameters)),
			})
		}
	}
	return line, true
}

// estimateLine attaches the LOCAL token estimate of one parsed line: its text,
// its reasoning, its tool-call arguments and its images. The numbers come from
// internal/session's estimator (the one the context threshold uses) — the viewer
// must never grow a second formula, or the two would drift apart.
//
// Images are estimated from the media file the line references; only the file
// header is read, and a file that cannot be read falls back to the configured
// constant instead of failing the read.
func estimateLine(line *Line, transcriptPath, model string) {
	est := &LineEstimate{
		Text:      session.TextTokens(line.Text),
		Reasoning: session.TextTokens(line.Reasoning),
	}
	for _, c := range line.Calls {
		name := session.TextTokens(c.Function.Name)
		args := session.TextTokens(c.Function.Arguments)
		est.Calls = append(est.Calls, name+args)
	}
	if len(line.Images) > 0 {
		base := filepath.Dir(transcriptPath)
		for _, ref := range line.Images {
			est.ImageCount++
			est.Images += mediaImageTokens(base, ref, model)
		}
	}
	if est.Text+est.Reasoning+est.Images > 0 || len(est.Calls) > 0 {
		line.Est = est
	}
}

// mediaImageTokens estimates one "file://media/<name>" image reference. Results
// are cached by path: a media file is named after its content hash, so the same
// path always means the same picture, and the live server re-reads transcripts
// on every poll.
func mediaImageTokens(baseDir, ref, model string) int {
	rel := strings.TrimPrefix(strings.TrimSpace(ref), "file://")
	if rel == "" {
		return session.ImageTokensForModel(model, 0, 0)
	}
	p := filepath.Join(baseDir, filepath.FromSlash(rel))
	key := model + "\x00" + p
	if v, ok := imageTokensCache.Load(key); ok {
		return v.(int)
	}
	v := session.ImageTokensOfFileForModel(model, p)
	imageTokensCache.Store(key, v)
	return v
}

// imageTokensCache memoises per-image estimates (see mediaImageTokens). The key
// is the model plus the path: the same file costs different amounts under
// different models' rules.
var imageTokensCache sync.Map

// sessionModelOf returns the wire model id a transcript records on its meta or
// usage line, "" when it records none. The live server re-reads transcripts
// every couple of seconds and a transcript's model never changes, so the answer
// is cached by (path, size): only a file that grew is looked at again, and the
// lookup stops at the first line that carries a model.
func sessionModelOf(path string) string {
	size := int64(-1)
	if st, err := os.Stat(path); err == nil {
		size = st.Size()
	}
	key := path + "\x00" + strconv.FormatInt(size, 10)
	if v, ok := sessionModelCache.Load(key); ok {
		return v.(string)
	}
	model := readSessionModel(path)
	sessionModelCache.Store(key, model)
	return model
}

func readSessionModel(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		raw := sc.Bytes()
		if !bytes.Contains(raw, []byte(`"model"`)) {
			continue
		}
		var rec struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(raw, &rec) == nil && rec.Model != "" {
			return rec.Model
		}
	}
	return ""
}

// sessionModelCache memoises sessionModelOf by (path, size).
var sessionModelCache sync.Map

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
	H         string             `json:"h,omitempty"`

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
	// ImageCount / TextTokens are the local half of that request: how many
	// images it carried and the text-only local estimate. Two consecutive lines
	// give the measured per-image cost (see measuredImageTokens); absent in
	// transcripts written before these fields existed.
	ImageCount int `json:"image_count,omitempty"`
	TextTokens int `json:"text_tokens,omitempty"`
}

// transcriptTool is one tool definition inside a meta line. Parameters stays a
// json.RawMessage here so the raw bytes survive decoding; it is handed to the
// viewer as a string afterwards.
type transcriptTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}
