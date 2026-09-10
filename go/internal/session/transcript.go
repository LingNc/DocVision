package session

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Transcript JSONL format (one JSON object per line, append-only):
//
//	{"t":"meta","label":"style","savedAt":...}
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
}

// TranscriptWriter appends messages of one session to a JSONL file.
type TranscriptWriter struct {
	path     string // absolute path of the .jsonl
	mediaDir string // absolute dir for image payloads
	file     *os.File
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
	return &TranscriptWriter{
		path:     abs,
		mediaDir: filepath.Join(filepath.Dir(abs), "media"),
		file:     f,
	}, nil
}

// Close releases the underlying file.
func (w *TranscriptWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Append persists one message. Base64 image payloads (data URLs in
// multipart content) are extracted into media files.
func (w *TranscriptWriter) Append(msg ChatMessage) error {
	line := transcriptLine{T: "msg", Role: msg.Role, CallID: msg.ToolCallID}
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
