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
	// Project is the sidebar group this session belongs to: the first path
	// segment under the scan root (see ProjectFor).
	Project string `json:"project"`
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
}

// IsMeta reports whether this line is a t="meta" record rather than a message.
// The viewer keys every "meta versus message" decision off this so a transcript
// that gains new non-"msg" types later still renders as messages only.
func (l Line) IsMeta() bool { return l.Type == "meta" }

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
		{"vector_", "vector"},
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
	case "会话":
		return subject
	}
	return label
}

// transcriptStem returns the transcript's file name without its extension.
func transcriptStem(rel string) string {
	return strings.TrimSuffix(path.Base(filepath.ToSlash(rel)), ".jsonl")
}

// stageSubject turns the file-name remainder into a readable subject. Vector
// transcripts are named vector_<书名>__<sha256>__<图label>: the hash is noise
// and the label itself uses underscores where the caption had spaces.
func stageSubject(stage, rest string) string {
	if stage != "vector" {
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
		out = append(out, SessionInfo{
			ID:       rel,
			Label:    LabelFor(rel),
			Title:    TitleFor(rel),
			Name:     d.Name(),
			Path:     p,
			Project:  ProjectFor(rel),
			Messages: stat.Messages,
			Meta:     stat.Meta,
			Bytes:    info.Size(),
			ModTime:  info.ModTime(),
			Live:     now.Sub(info.ModTime()) < LiveWindow,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

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
}

// transcriptTool is one tool definition inside a meta line. Parameters stays a
// json.RawMessage here so the raw bytes survive decoding; it is handed to the
// viewer as a string afterwards.
type transcriptTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}
