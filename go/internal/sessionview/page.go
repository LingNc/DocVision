package sessionview

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

// The viewer is three plain files — no CDN, no build step, no framework. The
// static export inlines all three so the snapshot works over file://, while
// the live server serves them as separate requests.
//
//go:embed assets/*
var assets embed.FS

// renderOptions carries what differs between the two modes.
type renderOptions struct {
	// data is the JSON snapshot of every session; nil for the live page, whose
	// missing #dsh-data element is what makes it poll /api/*.
	data json.RawMessage
}

const (
	stylePlaceholder  = "{{DSH_STYLE}}"
	scriptPlaceholder = "{{DSH_SCRIPT}}"
	dataPlaceholder   = "{{DSH_DATA}}"
)

// renderPage fills the shared template. Static output inlines the stylesheet,
// the script and the session data; the live page keeps the two asset links and
// carries a comment instead of the data element.
func renderPage(opt renderOptions) (string, error) {
	html, err := assets.ReadFile("assets/viewer.html")
	if err != nil {
		return "", fmt.Errorf("会话预览: 读取页面模板失败: %w", err)
	}
	styleTag := `<link rel="stylesheet" href="viewer.css">`
	scriptTag := `<script src="viewer.js"></script>`
	dataTag := "<!-- 实时模式：没有 #dsh-data 块，页面改从 /api/* 拉取会话数据 -->"
	if opt.data != nil {
		css, err := assets.ReadFile("assets/viewer.css")
		if err != nil {
			return "", fmt.Errorf("会话预览: 读取样式表失败: %w", err)
		}
		js, err := assets.ReadFile("assets/viewer.js")
		if err != nil {
			return "", fmt.Errorf("会话预览: 读取脚本失败: %w", err)
		}
		styleTag = "<style>\n" + string(css) + "</style>"
		scriptTag = "<script>\n" + string(js) + "</script>"
		dataTag = `<script type="application/json" id="dsh-data">` + string(opt.data) + `</script>`
	}
	return strings.NewReplacer(
		stylePlaceholder, styleTag,
		scriptPlaceholder, scriptTag,
		dataPlaceholder, dataTag,
	).Replace(string(html)), nil
}
