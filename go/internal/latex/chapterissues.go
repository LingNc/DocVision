package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ChapterIssue 是一条"这一章向原书还原时没做到 / 绕过了手册"的问题。
//
// 为什么要有这个结构：问题原来只存在于 `submit` 的 report 里，而 report 是
// **每章一份**——父章节整合完子章节后，子章节的问题没有任何机制带到上面，
// 顶层报告只写着父章节自己想起来的几条；到了风格修复大循环，整合会话只能
// 看到顶层那份残缺清单。于是"子章节说过的问题"在层级之间蒸发。
//
// 现在每条问题都是一个带稳定 ID 的条目，跨层原样传递：子节点提交时写出自己
// 的 store，父节点整合时由**代码**把子节点仍未解决的条目继承过来（不需要
// 父会话逐个读、也不需要它抄一遍），父会话只决定两件事——要不要顺手修
// （修了就把 ID 放进 `report.resolved`），以及自己有没有**新**问题
// （`report.issues`）。没有修的一律原样向上，到顶层的报告里就是整棵子树的
// 未解问题全集。
type ChapterIssue struct {
	// ID 形如 "<来源章节>#<序号>"，跨层稳定；`report.resolved` 用它删条。
	ID     string `json:"id"`
	Source string `json:"source"` // 最初提出这条问题的章节（顶层章节名）
	Depth  int    `json:"depth"`  // 提出它的节点在递归树里的深度
	Kind   string `json:"kind,omitempty"`
	Text   string `json:"text"`
	// Status 为 open 或 resolved；resolved 的条目留在 store 里（便于事后
	// 看到"这条是父章节修掉的"），但不再向上传递。
	Status string `json:"status"`
}

const (
	issueOpen     = "open"
	issueResolved = "resolved"
)

// readIssueStore 读一个节点的 issue store；文件不存在时返回空（不是错误：
// 第一次尝试、或没写 report 的节点就是没有）。
func readIssueStore(path string) []ChapterIssue {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var list []ChapterIssue
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return list
}

func writeIssueStore(path string, list []ChapterIssue) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if list == nil {
		list = []ChapterIssue{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// openIssues 只留未解决的条目，并按 ID 去重（同一个 ID 只出现一次）。
func openIssues(list []ChapterIssue) []ChapterIssue {
	seen := make(map[string]bool, len(list))
	out := make([]ChapterIssue, 0, len(list))
	for _, it := range list {
		it.Text = strings.TrimSpace(it.Text)
		if it.Text == "" {
			continue
		}
		if it.Status == issueResolved {
			continue
		}
		it.Status = issueOpen
		if it.ID == "" {
			it.ID = it.Source + "#?"
		}
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		out = append(out, it)
	}
	return out
}

// mergeChapterIssues 生成一个节点自己的 store：继承来的（未解决）+ 自己新报的，
// 再按 `resolved` 把父章节明确修掉的条目划掉。
//
// ID 规则：继承条目沿用原 ID（跨层稳定），自己新报的用 "<base>#<n>"——同一个
// 子节点在不同尝试里重报同一条问题不会重复，因为新条目先按文本去重。
func mergeChapterIssues(base string, depth int, own []string, inherited []ChapterIssue, resolved []string) []ChapterIssue {
	killed := make(map[string]bool, len(resolved))
	for _, id := range resolved {
		if id = strings.TrimSpace(id); id != "" {
			killed[id] = true
		}
	}
	out := make([]ChapterIssue, 0, len(inherited)+len(own))
	seenText := make(map[string]bool, len(inherited)+len(own))
	for _, it := range openIssues(inherited) {
		if seenText[it.Text] {
			continue
		}
		seenText[it.Text] = true
		if killed[it.ID] {
			it.Status = issueResolved
		}
		out = append(out, it)
	}
	n := 0
	for _, text := range own {
		text = strings.TrimSpace(text)
		if text == "" || seenText[text] {
			continue
		}
		seenText[text] = true
		n++
		out = append(out, ChapterIssue{
			ID:     fmt.Sprintf("%s#%d", base, n),
			Source: base,
			Depth:  depth,
			Text:   text,
			Status: issueOpen,
		})
	}
	return out
}

// reportTextList 读 report.issues / report.resolved，两种写法都收：一个字符串
// （旧的、一段话）或字符串数组（新推荐，一个问题一条，便于跨层合并）；数组里
// 的元素也可以带 id 的对象 {"id":…,"text":…}，方便父章节顺手引用子条目。
func reportTextList(v any) []string {
	switch val := v.(type) {
	case string:
		var out []string
		for _, line := range strings.Split(val, "\n") {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*• "))
			line = strings.TrimSpace(strings.TrimPrefix(line, "\u2022"))
			if line != "" {
				out = append(out, line)
			}
		}
		return out
	case []any:
		var out []string
		for _, item := range val {
			switch it := item.(type) {
			case string:
				if t := strings.TrimSpace(it); t != "" {
					out = append(out, t)
				}
			case map[string]any:
				text, _ := it["text"].(string)
				if t := strings.TrimSpace(text); t != "" {
					out = append(out, t)
				}
			}
		}
		return out
	case []string:
		var out []string
		for _, it := range val {
			if t := strings.TrimSpace(it); t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	return nil
}

// renderIssuesSection 把未解决问题渲染成报告里的一节。**没修的问题必须原样
// 出现**——这一节由代码写，不依赖父会话记得抄。
func renderIssuesSection(open []ChapterIssue) string {
	if len(open) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## 未解决的问题（机制合并，共 ")
	b.WriteString(fmt.Sprintf("%d 条：来自子章节的会原样向上传递，修掉的请在 submit 的 report.resolved 里写 ID）\n\n", len(open)))
	for _, it := range open {
		src := it.Source
		if src == "" {
			src = "本节点"
		}
		fmt.Fprintf(&b, "- `%s`（来源 %s，深度 %d）：%s\n", it.ID, src, it.Depth, strings.ReplaceAll(it.Text, "\n", " "))
	}
	return b.String()
}
