// Package troubleshoot 把"问题处理流程"（docs/07-troubleshoot.md）固化成程序：
// 状态机 work/troubleshoot/（start→collect→draft→validate→submit），
// 引导步骤落盘、收集自动化、提交前格式与脱敏校验——绕行的门从"文档劝说"
// 升级为"程序校验不通过就不给出可提交的 issue"。
package troubleshoot

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// State 是处理流程状态机。
type State string

const (
	StateStarted   State = "started"
	StateCollected State = "collected"
	StateDrafted   State = "drafted"
	StateValidated State = "validated"
	StateSubmitted State = "submitted"
)

// Dir 返回问题处理工作区（项目/工作区内）。
func Dir(root string) string { return filepath.Join(root, "work", "troubleshoot") }

// 必需章节（issue 格式规定，校验用）。
var requiredSections = []string{"## 环境", "## 复现步骤", "## 期望与实际", "## 脱敏日志", "## 根因分析", "## 修复建议"}

// secretPatterns 脱敏校验：命中即拒收（token/私钥/AKIA/本地绝对路径）。
var secretPatterns = []struct {
	re   *regexp.Regexp
	name string
}{
	{regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`), "疑似 API token（sk-...）"},
	{regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]{16,}`), "疑似 Bearer token"},
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), "疑似私钥"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "疑似 AWS 密钥"},
	{regexp.MustCompile(`/home/[A-Za-z0-9_.-]+/`), "本地绝对路径（/home/...）"},
}

const reportTemplate = `# 问题报告（按步骤填写；# 开头为注释行，提交前会被剥离）

<!-- 步骤提示：先 troubleshoot collect 自动收集环境；然后填下面各节；validate 通过后才能 submit。 -->

## 环境
<!-- 会由 collect 自动填充，可补充 -->

## 复现步骤
1.
2.

## 期望与实际
<!-- 期望：……；实际：……（贴真实报错全文） -->

## 脱敏日志
<!-- 指向 logs/ 下已收集文件；粘贴内容须先打码（token/私钥/路径） -->

## 根因分析
<!-- 最小复现的结论；若未查明写"未查明+已排查项" -->

## 修复建议
<!-- 最小 diff 思路；能附 patch 最佳（diff -u 输出贴此或放 logs/fix.patch） -->
`

// Start 建立处理工作区（幂等）：目录 + 报告模板 + 状态。
func Start(root string) (State, error) {
	d := Dir(root)
	if err := os.MkdirAll(filepath.Join(d, "logs"), 0o755); err != nil {
		return "", err
	}
	rp := filepath.Join(d, "report.md")
	if _, err := os.Stat(rp); os.IsNotExist(err) {
		if err := os.WriteFile(rp, []byte(reportTemplate), 0o644); err != nil {
			return "", err
		}
	}
	if err := writeState(d, StateStarted); err != nil {
		return "", err
	}
	return StateStarted, nil
}

// Collect 自动收集环境与版本到 logs/，并把 doctor 摘要填进报告"环境"节。
// doctorOut 传 dvagent doctor 的 JSON 文本（由命令层捕获）。
func Collect(root, doctorOut string) (State, error) {
	d := Dir(root)
	if err := os.MkdirAll(filepath.Join(d, "logs"), 0o755); err != nil {
		return "", err
	}
	stamp := time.Now().Format("01-02_1504")
	if err := os.WriteFile(filepath.Join(d, "logs", "doctor-"+stamp+".json"), []byte(doctorOut), 0o644); err != nil {
		return "", err
	}
	// 摘要进报告环境节（幂等：整节替换）
	rp := filepath.Join(d, "report.md")
	body, _ := os.ReadFile(rp)
	section := "## 环境\n\n```json\n" + summarizeDoctor(doctorOut) + "\n```\n"
	body = []byte(replaceSection(string(body), "## 环境", section))
	if err := os.WriteFile(rp, []byte(body), 0o644); err != nil {
		return "", err
	}
	if err := writeState(d, StateCollected); err != nil {
		return "", err
	}
	return StateCollected, nil
}

// summarizeDoctor 压缩 doctor JSON 到关键行（工具 found 摘要+CJK 数）。
func summarizeDoctor(json string) string {
	var lines []string
	lines = append(lines, "doctor 摘要（全文见 logs/）：")
	if i := strings.Index(json, `"cjk_count"`); i > 0 {
		end := strings.Index(json[i:], "\n")
		if end < 0 {
			end = len(json) - i // 单行 JSON：取到结尾
		}
		lines = append(lines, strings.TrimSpace(json[i:i+end]))
	}
	nFound, nAll := 0, 0
	for _, m := range regexp.MustCompile(`"found":\s*true`).FindAllString(json, -1) {
		_ = m
		nFound++
	}
	nAll = len(regexp.MustCompile(`"name":`).FindAllString(json, -1))
	lines = append(lines, fmt.Sprintf("tools: %d/%d found", nFound, nAll))
	return strings.Join(lines, "\n")
}

// Draft 标记进入人工填写阶段（状态推进，报告可编辑）。
func Draft(root string) (State, error) {
	d := Dir(root)
	if err := writeState(d, StateDrafted); err != nil {
		return "", err
	}
	return StateDrafted, nil
}

// Validate 格式与内容校验（提交门禁）：章节齐全 + 无未脱敏秘密 + 有实质内容。
func Validate(root string) ([]string, State, error) {
	d := Dir(root)
	rp := filepath.Join(d, "report.md")
	body, err := os.ReadFile(rp)
	if err != nil {
		return []string{"report.md 不存在（先 troubleshoot start）"}, "", err
	}
	var errs []string
	// 1. 必需章节（剥离注释行后判断）
	stripped := stripComments(string(body))
	for _, sec := range requiredSections {
		if !strings.Contains(stripped, sec) {
			errs = append(errs, "缺少章节: "+sec)
		}
	}
	// 2. 章节非空（每节标题后至少有一行实质内容）
	for _, sec := range requiredSections {
		if chunk := sectionContent(stripped, sec); chunk != "" && len(strings.TrimSpace(chunk)) < 4 {
			errs = append(errs, sec+" 内容过短（至少写一句实质内容）")
		}
	}
	// 3. 脱敏校验（全报告 + logs/ 文本文件）
	targets := map[string]string{"report.md": string(body)}
	entries, _ := os.ReadDir(filepath.Join(d, "logs"))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".txt") {
			if b, err := os.ReadFile(filepath.Join(d, "logs", e.Name())); err == nil {
				targets["logs/"+e.Name()] = string(b)
			}
		}
	}
	for fname, content := range targets {
		for _, sp := range secretPatterns {
			if m := sp.re.FindString(content); m != "" {
				errs = append(errs, fmt.Sprintf("%s 含未脱敏内容（%s）：%s…", fname, sp.name, m[:minInt(len(m), 12)]))
			}
		}
	}
	st := StateDrafted
	if len(errs) == 0 {
		st = StateValidated
	}
	_ = writeState(d, st)
	return errs, st, nil
}

// Submit 生成可提交的 issue 文本（剥离注释），状态记 submitted。
// 返回 issue.md 路径与全文（推送 GitHub 由用户授权后另行执行，见 docs/07）。
func Submit(root string) (string, string, error) {
	d := Dir(root)
	rp := filepath.Join(d, "report.md")
	body, err := os.ReadFile(rp)
	if err != nil {
		return "", "", err
	}
	errs, _, _ := Validate(root)
	if len(errs) > 0 {
		return "", "", fmt.Errorf("校验未通过，不允许提交：\n- %s", strings.Join(errs, "\n- "))
	}
	final := stripComments(string(body))
	out := filepath.Join(d, "issue.md")
	if err := os.WriteFile(out, []byte(final), 0o644); err != nil {
		return "", "", err
	}
	_ = writeState(d, StateSubmitted)
	return out, final, nil
}

// Status 读状态。
func Status(root string) State {
	b, err := os.ReadFile(filepath.Join(Dir(root), "state"))
	if err != nil {
		return ""
	}
	return State(strings.TrimSpace(string(b)))
}

func writeState(d string, st State) error {
	return os.WriteFile(filepath.Join(d, "state"), []byte(st), 0o644)
}

// stripComments 剥掉 <!-- --> 与行首 # 注释。
func stripComments(s string) string {
	re := regexp.MustCompile(`(?s)<!--.*?-->`)
	s = re.ReplaceAllString(s, "")
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "##") {
			continue
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// replaceSection 替换从 heading 到下一个同级/更高级标题前的内容。
func replaceSection(s, heading, newContent string) string {
	idx := strings.Index(s, heading)
	if idx < 0 {
		return s + "\n\n" + newContent
	}
	rest := s[idx+len(heading):]
	end := len(s)
	if n := nextHeading(rest); n >= 0 {
		end = idx + len(heading) + n
	}
	return s[:idx] + newContent + s[end:]
}

// nextHeading 返回下一个 markdown 标题的偏移（相对 rest 起点）。
func nextHeading(rest string) int {
	re := regexp.MustCompile(`(?m)^#{1,6} `)
	loc := re.FindStringIndex(rest)
	if loc == nil {
		return -1
	}
	return loc[0]
}

// sectionContent 取章节正文（去标题行，截到下一标题）。
func sectionContent(s, heading string) string {
	idx := strings.Index(s, heading)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(heading):]
	end := len(rest)
	if n := nextHeading(rest); n >= 0 {
		end = n
	}
	return rest[:end]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
