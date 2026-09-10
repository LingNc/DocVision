package sessionview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// pageData is what the viewer finds in its <script type="application/json"
// id="dsh-data"> block. Its presence is what tells the page it is a static
// snapshot: there is no server to poll, so every session is embedded and image
// references are resolved relative to the HTML file.
type pageData struct {
	Mode      string          `json:"mode"`
	Root      string          `json:"root"`
	Generated string          `json:"generated"`
	MediaRoot string          `json:"mediaRoot"`
	Sessions  []staticSession `json:"sessions"`
}

// staticSession is one session with its full transcript inlined.
type staticSession struct {
	SessionInfo
	Lines []Line `json:"lines"`
}

// WriteStaticHTML renders every session into one self-contained HTML file:
// data, stylesheet and script are all inlined, so the file can be opened
// directly over file:// (or copied elsewhere) without the server running.
//
// A nil/empty sessions slice means "scan root first". Image references are
// rewritten relative to outPath, so the media/ directories must keep their
// position under root for the pictures to show up.
func WriteStaticHTML(root, outPath string, sessions []SessionInfo) error {
	if len(sessions) == 0 {
		var err error
		sessions, err = Scan(root)
		if err != nil {
			return err
		}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}

	// The page lives somewhere else than the transcripts (usually directly in
	// root), so the image prefix is the relative way back to root.
	mediaRoot := ""
	if rel, rerr := filepath.Rel(filepath.Dir(outAbs), rootAbs); rerr == nil {
		rel = filepath.ToSlash(rel)
		if rel != "." {
			mediaRoot = strings.TrimSuffix(rel, "/") + "/"
		}
	}

	payload := pageData{
		Mode:      "static",
		Root:      rootAbs,
		Generated: time.Now().Format(time.RFC3339),
		MediaRoot: mediaRoot,
		Sessions:  make([]staticSession, 0, len(sessions)),
	}
	for _, s := range sessions {
		lines, _, err := ReadSession(s.Path, 0)
		if err != nil {
			return fmt.Errorf("会话预览: 读取会话 %s 失败: %w", s.ID, err)
		}
		payload.Sessions = append(payload.Sessions, staticSession{SessionInfo: s, Lines: lines})
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// SetEscapeHTML keeps a transcript from closing the surrounding <script>
	// element: a tool result containing "</script>" would otherwise end the
	// data block and turn the page into injected markup.
	enc.SetEscapeHTML(true)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("会话预览: 序列化会话数据失败: %w", err)
	}

	page, err := renderPage(renderOptions{data: json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outAbs), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outAbs, []byte(page), 0o644); err != nil {
		return fmt.Errorf("会话预览: 写入 %s 失败: %w", outPath, err)
	}
	return nil
}
