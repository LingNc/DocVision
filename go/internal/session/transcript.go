package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Transcript JSONL format (one JSON object per line, append-only):
//
//	{"t":"meta","kind":"system","label":"style","model":"glm-4.6", ...}
//	{"t":"meta","kind":"system","system":"<完整系统提示词>","tools":[{"name":...}]}
//	{"t":"msg","role":"user","text":"...","images":["file://media/ab12.jpg"]}
//	{"t":"msg","role":"assistant","text":"...","tool_calls":[...]}
//	{"t":"msg","role":"tool","tool_call_id":"...","text":"..."}
//
// Base64 image payloads are NOT written inline: they are stored as
// separate media files under <transcript dir>/media/ and referenced as
// "file://media/<name>" (relative to the transcript file), so the JSONL
// stays small. On load the files are read back into base64 data URLs.

type transcriptLine struct {
	T      string     `json:"t"`
	Role   string     `json:"role,omitempty"`
	Text   string     `json:"text,omitempty"`
	Images []string   `json:"images,omitempty"`
	Calls  []ToolCall `json:"tool_calls,omitempty"`
	CallID string     `json:"tool_call_id,omitempty"`
	// Reasoning keeps the provider's reasoning_content (思维链) so a
	// resumed session replays the history byte-identically: vendors with
	// retained thinking (GLM thinking.clear_thinking:false) expect the
	// full chain back and it is also a precondition for prefix caching.
	Reasoning string `json:"reasoning_content,omitempty"`

	// ---- t == "meta" lines only (never replayed) ----
	// A meta line records WHAT the model was told at the start of this run
	// (system prompt + the tool definitions that were sent). It is written
	// for inspection only: LoadTranscript ignores every non-"msg" line, so
	// resume semantics are unchanged and the live system prompt stays the
	// single source of truth (it is re-rendered per run — watermark flag,
	// mounts, available source pages).
	Kind    string         `json:"kind,omitempty"`
	Label   string         `json:"session_label,omitempty"`
	Model   string         `json:"model,omitempty"`
	SysHash string         `json:"system_sha,omitempty"`
	Tools   []toolSnapshot `json:"tools,omitempty"`

	// TS is the write time (RFC3339, milliseconds). Every line carries it so a
	// reader can reconstruct the timeline (duration, idle gaps) without a log.
	// Transcripts written before this field existed simply have no ts.
	TS string `json:"ts,omitempty"`

	// ---- t == "usage" lines only (never replayed) ----
	// One usage line per completed API request, so the preview page (and any
	// script reading the JSONL) can compute input/output tokens, prefix-cache
	// hit rate, latency, time-to-first-token and output speed per session.
	Stream          *bool  `json:"stream,omitempty"`
	Round           int    `json:"round,omitempty"`
	PromptTokens    int    `json:"prompt_tokens,omitempty"`
	CachedTokens    int    `json:"cached_tokens,omitempty"`
	Completion      int    `json:"completion_tokens,omitempty"`
	ReasoningTokens int    `json:"reasoning_tokens,omitempty"`
	DurationMS      int64  `json:"duration_ms,omitempty"`
	TTFTMS          int64  `json:"ttft_ms,omitempty"`
	Finish          string `json:"finish_reason,omitempty"`
	// ImageCount / TextTokens are the LOCAL half of the request: how many
	// images it carried and the text-only local estimate (see
	// Session.snapshotRequest). Two consecutive lines then give the measured
	// per-image cost: the vendor's prompt_tokens delta minus the text growth,
	// divided by the images added. The key is image_count, not images — a
	// "msg" line already uses images for its media references.
	// Absent in transcripts written before these fields existed.
	ImageCount int `json:"image_count,omitempty"`
	TextTokens int `json:"text_tokens,omitempty"`
}

// toolSnapshot is the tool definition recorded in a meta line: exactly what
// the API received (name, description, parameter schema).
type toolSnapshot struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// TranscriptWriter appends messages of one session to a JSONL file.
type TranscriptWriter struct {
	path     string // absolute path of the .jsonl
	mediaDir string // absolute dir for image payloads
	file     *os.File
	// lastMetaHash guards against duplicate meta lines when a resumed
	// session re-attaches to the same transcript file.
	lastMetaHash string
}

// NewTranscript opens (creating if needed) the JSONL transcript at path.
// Media files land in <dir of path>/media/.
func NewTranscript(path string) (*TranscriptWriter, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(abs), "media"), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	w := &TranscriptWriter{
		path:     abs,
		mediaDir: filepath.Join(filepath.Dir(abs), "media"),
		file:     f,
	}
	w.lastMetaHash = w.readLastMetaHash()
	return w, nil
}

// Close releases the underlying file. A leftover partial snapshot would
// otherwise make the preview page show a "streaming" tail for a session
// that is already gone.
func (w *TranscriptWriter) Close() error {
	w.ClearPartial()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// P7 流式快照（sidecar）：<transcript>.partial，唯一内容是「当前正在流式
// 生成的那条消息」的最新进度。转录本身仍是整行落盘的 append-only JSONL——
// partial 是临时覆写文件，消息完整落盘即删；预览进程与跑会话的进程不必是
// 同一个，所以走磁盘而不是内存广播。

// PartialRecord is the JSON payload of the sidecar file.
type PartialRecord struct {
	Phase string `json:"phase"` // "content" | "reasoning"
	Text  string `json:"text"`  // accumulated text so far (tail-capped upstream)
	Ts    int64  `json:"ts"`    // unix millis, for the viewer to show freshness
}

// WritePartial atomically replaces the sidecar (tmp file + rename: a
// concurrent reader must never see a half-written JSON).
func (w *TranscriptWriter) WritePartial(p PartialRecord) error {
	if w == nil {
		return nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tmp := w.partialPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, w.partialPath())
}

// ClearPartial removes the sidecar; a missing file is not an error.
func (w *TranscriptWriter) ClearPartial() {
	if w == nil {
		return
	}
	os.Remove(w.partialPath())
	os.Remove(w.partialPath() + ".tmp")
}

func (w *TranscriptWriter) partialPath() string { return w.path + ".partial" }

// ToolSnapshot is one tool definition as sent to the API.
type ToolSnapshot struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// AppendMeta records one t="meta" line: what the model was told when this
// session started. It is never replayed (LoadTranscript skips non-"msg"
// lines), it exists so a transcript explains itself — the session preview
// page renders it, and "what was the model actually instructed" no longer
// needs a --debug log.
//
// To keep resumed sessions from piling up identical copies, the system
// prompt is hashed and an identical consecutive meta line is skipped.
func (w *TranscriptWriter) AppendMeta(kind, label, model, system string, tools []ToolSnapshot) error {
	if w == nil || w.file == nil {
		return nil
	}
	sum := sha256.Sum256([]byte(system))
	hash := hex.EncodeToString(sum[:8])
	if w.lastMetaHash == kind+":"+hash {
		return nil
	}
	line := transcriptLine{
		T:       "meta",
		Kind:    kind,
		Label:   label,
		Model:   model,
		SysHash: hash,
		Text:    system,
	}
	for _, t := range tools {
		line.Tools = append(line.Tools, toolSnapshot{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
	}
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	if _, err := w.file.Write(append(data, '\n')); err != nil {
		return err
	}
	w.lastMetaHash = kind + ":" + hash
	return nil
}

// lastMetaHash remembers the (kind, system hash) of the meta line written by
// THIS writer, so re-attaching a transcript for a resumed run does not
// duplicate an unchanged prompt.
func (w *TranscriptWriter) readLastMetaHash() string {
	data, err := os.ReadFile(w.path)
	if err != nil {
		return ""
	}
	idx := bytes.LastIndex(data, []byte(`{"t":"meta"`))
	if idx < 0 {
		return ""
	}
	end := bytes.IndexByte(data[idx:], '\n')
	if end < 0 {
		end = len(data) - idx
	}
	var line transcriptLine
	if err := json.Unmarshal(data[idx:idx+end], &line); err != nil {
		return ""
	}
	return line.Kind + ":" + line.SysHash
}

// UsageRecord is the per-request accounting appended as a t="usage" line.
type UsageRecord struct {
	Model string
	// Kind names the request inside the session: "" for an ordinary turn,
	// "nudge" (forced text answer after an empty reply) or "compact"
	// (context summarisation). Token totals must include all of them: a
	// session that compacted 5 times really did pay for those prompts.
	Kind         string
	Stream       bool
	Round        int
	PromptTokens int
	CachedTokens int
	Completion   int
	Reasoning    int
	Duration     time.Duration
	TTFT         time.Duration
	Finish       string
	// TextTokens is the local text-only estimate of that request and Images is
	// how many images it carried (see Session.snapshotRequest). They are what
	// the preview needs to turn a prompt_tokens delta into a measured
	// per-image cost; 0 means the record predates the fields.
	TextTokens int
	Images     int
}

// AppendUsage records one completed API request. Like meta lines it is never
// replayed (LoadTranscript only reads t="msg"), so it cannot change resume
// semantics; it exists so the preview page and any JSONL reader can derive
// cache hit rate, token counts, latency, TTFT and output speed.
func (w *TranscriptWriter) AppendUsage(u UsageRecord) error {
	if w == nil || w.file == nil {
		return nil
	}
	stream := u.Stream
	line := transcriptLine{
		T:               "usage",
		TS:              nowStamp(),
		Kind:            u.Kind,
		Model:           u.Model,
		Stream:          &stream,
		Round:           u.Round,
		PromptTokens:    u.PromptTokens,
		CachedTokens:    u.CachedTokens,
		Completion:      u.Completion,
		ReasoningTokens: u.Reasoning,
		DurationMS:      u.Duration.Milliseconds(),
		TTFTMS:          u.TTFT.Milliseconds(),
		Finish:          u.Finish,
		ImageCount:      u.Images,
		TextTokens:      u.TextTokens,
	}
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	_, err = w.file.Write(append(data, '\n'))
	return err
}

// nowStamp is the timestamp format every transcript line carries.
func nowStamp() string { return time.Now().Format("2006-01-02T15:04:05.000Z07:00") }

// Append persists one message. Base64 image payloads (data URLs in
// multipart content) are extracted into media files.
func (w *TranscriptWriter) Append(msg ChatMessage) error {
	line := transcriptLine{T: "msg", Role: msg.Role, CallID: msg.ToolCallID, TS: nowStamp()}
	switch c := msg.Content.(type) {
	case string:
		line.Text = c
	case []map[string]interface{}:
		var b strings.Builder
		for _, part := range c {
			if t, ok := part["text"].(string); ok {
				b.WriteString(t)
				continue
			}
			if iu, ok := part["image_url"].(map[string]string); ok {
				ref, err := w.storeImage(iu["url"])
				if err != nil {
					return err
				}
				line.Images = append(line.Images, ref)
			}
		}
		line.Text = b.String()
	case nil:
		// assistant tool_calls-only turn
	default:
		data, err := json.Marshal(c)
		if err != nil {
			return err
		}
		line.Text = string(data)
	}
	line.Calls = msg.ToolCalls
	line.Reasoning = msg.ReasoningContent
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	if _, err := w.file.Write(append(data, '\n')); err != nil {
		return err
	}
	return w.file.Sync()
}

var dataURLRe = regexp.MustCompile(`^data:([^;]+);base64,(.*)$`)

// storeImage decodes a data URL, writes the payload to the media dir
// and returns a "file://media/<name>" reference.
func (w *TranscriptWriter) storeImage(dataURL string) (string, error) {
	m := dataURLRe.FindStringSubmatch(dataURL)
	if m == nil {
		return "", fmt.Errorf("transcript: 非法图片 URL（期望 data:...;base64,）")
	}
	ext := ".png"
	switch {
	case strings.Contains(m[1], "jpeg"):
		ext = ".jpg"
	case strings.Contains(m[1], "gif"):
		ext = ".gif"
	case strings.Contains(m[1], "webp"):
		ext = ".webp"
	}
	raw, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	name := hex.EncodeToString(sum[:8]) + ext
	dst := filepath.Join(w.mediaDir, name)
	if !fileExistsT(dst) {
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return "", err
		}
	}
	return "file://media/" + name, nil
}

func fileExistsT(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// LoadTranscript reconstructs the conversation stored at path. Media
// references are read back as base64 data URLs so the messages are
// wire-ready. Returns nil, nil when the file does not exist.
func LoadTranscript(path string) ([]ChatMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	baseDir := filepath.Dir(path)
	var msgs []ChatMessage
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var line transcriptLine
		if err := json.Unmarshal([]byte(ln), &line); err != nil {
			return nil, fmt.Errorf("transcript %s: %w", path, err)
		}
		if line.T != "msg" {
			continue
		}
		msg := ChatMessage{Role: line.Role, ToolCallID: line.CallID, ToolCalls: line.Calls, ReasoningContent: line.Reasoning}
		switch {
		case len(line.Images) > 0:
			parts := []map[string]interface{}{}
			if line.Text != "" {
				parts = append(parts, map[string]interface{}{"type": "text", "text": line.Text})
			}
			for _, ref := range line.Images {
				payload, mime, err := readMediaRef(baseDir, ref)
				if err != nil {
					return nil, err
				}
				parts = append(parts, map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]string{"url": "data:" + mime + ";base64," + payload},
				})
				// 重放时同样按尺寸估算：续跑会话的上下文估算不能因为"图是从
				// 转录里读回来的"就退回固定常量。
				msg.ImageTokens = append(msg.ImageTokens, ImageTokensOfBase64(payload))
			}
			msg.Content = parts
		case line.Role == "assistant" && line.Calls != nil:
			// content may legitimately be empty for tool_calls turns
			if line.Text != "" {
				msg.Content = line.Text
			}
		default:
			msg.Content = line.Text
		}
		msgs = append(msgs, msg)
	}
	// Compaction is append-only on disk: everything before the newest
	// COMPRESSED marker was already summarised, so replaying it would
	// resurrect a history the session deliberately dropped.
	last := -1
	for i, m := range msgs {
		if m.Role == "user" && strings.HasPrefix(ContentString(m), compactedMarker) {
			last = i
		}
	}
	if last > 0 {
		msgs = msgs[last:]
	}
	return msgs, nil
}

func readMediaRef(baseDir, ref string) (string, string, error) {
	if !strings.HasPrefix(ref, "file://") {
		return "", "", fmt.Errorf("transcript: 非法媒体引用 %q", ref)
	}
	p := filepath.Join(baseDir, strings.TrimPrefix(ref, "file://"))
	raw, err := os.ReadFile(p)
	if err != nil {
		return "", "", fmt.Errorf("transcript: 读取媒体 %s: %w", ref, err)
	}
	mime := "image/png"
	switch {
	case strings.HasSuffix(p, ".jpg"), strings.HasSuffix(p, ".jpeg"):
		mime = "image/jpeg"
	case strings.HasSuffix(p, ".gif"):
		mime = "image/gif"
	case strings.HasSuffix(p, ".webp"):
		mime = "image/webp"
	}
	return base64.StdEncoding.EncodeToString(raw), mime, nil
}
