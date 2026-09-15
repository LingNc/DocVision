package sessionview

import (
	"encoding/json"
	"strings"
	"testing"

	"mineru-tools/internal/session"
)

// TestLineEstimateIsLocalAndComplete 钉住"行内 token 是本地估算"这条数据契约：
// 每行的文本 / 思考 / 工具参数 / 图片都有估算，用的是 internal/session 的
// 估算器（不是页面上另算一套），并且原样下发到页面数据里。
func TestLineEstimateIsLocalAndComplete(t *testing.T) {
	root := t.TempDir()
	// media 引用按**转录所在目录**解析（与 viewer.js 的 mediaURL 同规则）。
	writePNG(t, jsonl(root, "proj", "work", "sessions", "media", "fig.png"), 1000, 800)
	writeFile(t, jsonl(root, "proj", "work", "sessions", "est_01.jsonl"), transcript(
		`{"t":"meta","kind":"system","session_label":"convert:01","model":"wire-A","sys_hash":"abc123","text":"系统提示词","tools":[{"name":"read_file","description":"读文件","parameters":"{\"type\":\"object\"}"}]}`,
		`{"t":"msg","role":"user","text":"先看这一章的结构。","ts":"2025-01-02T03:00:01Z"}`,
		`{"t":"msg","role":"assistant","text":"读一下。","reasoning_content":"想一下这一步要读哪个文件。","ts":"2025-01-02T03:00:02Z","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"work/a.tex\"}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_1","text":"ok (1.2 KB)","ts":"2025-01-02T03:00:03Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/fig.png"],"ts":"2025-01-02T03:00:04Z"}`,
		`{"t":"usage","ts":"2025-01-02T03:00:05Z","model":"wire-A","round":1,"prompt_tokens":12345,"cached_tokens":10000,"completion_tokens":678}`,
	))

	lines, _, err := ReadSession(jsonl(root, "proj", "work", "sessions", "est_01.jsonl"), 0)
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if len(lines) != 6 {
		t.Fatalf("行数 = %d，期望 6", len(lines))
	}

	// ① 元信息：系统提示词的本地估算 + 工具定义的估算都下发。
	meta := lines[0]
	if meta.Est == nil || meta.Est.Text != session.TextTokens("系统提示词") {
		t.Fatalf("meta 行没有本地 token 估算: %+v", meta.Est)
	}
	if len(meta.Tools) != 1 || meta.Tools[0].DescTokens == 0 || meta.Tools[0].ParamTokens == 0 {
		t.Fatalf("工具定义的估算没有下发: %+v", meta.Tools)
	}

	// ② 助手行：正文、思考、工具参数三者都有。
	asst := lines[2]
	if asst.Est == nil {
		t.Fatal("助手行没有估算")
	}
	if asst.Est.Text == 0 || asst.Est.Reasoning == 0 || len(asst.Est.Calls) != 1 || asst.Est.Calls[0] == 0 {
		t.Fatalf("助手行估算不完整: %+v", asst.Est)
	}
	if want := session.TextTokens(asst.Calls[0].Function.Name) + session.TextTokens(asst.Calls[0].Function.Arguments); asst.Est.Calls[0] != want {
		t.Fatalf("工具参数估算 = %d，期望 %d（与压缩阈值同一套公式）", asst.Est.Calls[0], want)
	}

	// ③ 结果行与图片行。
	if lines[3].Est == nil || lines[3].Est.Text != session.TextTokens("ok (1.2 KB)") {
		t.Fatalf("结果行没有文本估算: %+v", lines[3].Est)
	}
	img := lines[4].Est
	if img == nil || img.ImageCount != 1 {
		t.Fatalf("图片行没有数出图片: %+v", img)
	}
	// 1000×800 → 800000/750 = 1066。
	if img.Images != session.ImageTokens(1000, 800) || img.Images != 1066 {
		t.Fatalf("图片 token 估算 = %d，期望 1066（按尺寸折算）", img.Images)
	}

	// ④ 用量行只有厂商数字，没有本地估算（避免两套口径混在一行里）。
	if lines[5].Est != nil {
		t.Errorf("用量行不该有本地估算: %+v", lines[5].Est)
	}
}

// TestImageTokenEstimateRules 钉住单张图片的估算规则：有尺寸按像素折算并夹在
// 上下限内，取不到尺寸退回配置的兜底常量，空文本算 0。
func TestImageTokenEstimateRules(t *testing.T) {
	defer session.ResetEstimates()
	session.SetEstimateConfig(session.ImageEstimate{PxPerToken: 750, MinTokens: 85, MaxTokens: 4096})

	cases := []struct {
		w, h, want int
		why        string
	}{
		{1000, 800, 1066, "常规插图按 宽×高/750 折算"},
		{10, 10, 85, "极小图有下限"},
		{4000, 4000, 4096, "超大图有上限"},
		{0, 0, 1100, "取不到尺寸退回常量"},
	}
	for _, c := range cases {
		if got := session.ImageTokens(c.w, c.h); got != c.want {
			t.Errorf("ImageTokens(%d,%d) = %d，期望 %d（%s）", c.w, c.h, got, c.want, c.why)
		}
	}
	if got := session.TextTokens(""); got != 0 {
		t.Errorf("空文本估算 = %d，期望 0", got)
	}
	if session.TextTokens("中文四个字") != session.TextTokens("中文四个字") {
		t.Error("文本估算不稳定")
	}

	// 配置能改规则（estimate.px_per_token 等），且缺项自动补默认值。
	session.SetEstimateConfig(session.ImageEstimate{PxPerToken: 1000})
	if got := session.ImageTokens(1000, 800); got != 800 {
		t.Errorf("改配置后 ImageTokens(1000,800) = %d，期望 800", got)
	}
	if got := session.ImageTokens(0, 0); got != 1100 {
		t.Errorf("未配置的常量应为默认 1100，实际 %d", got)
	}
}

// TestImageEstimateTwoMethods 钉住"两种计量方法各自可选、参数可改"：
// fixed 每张一个常量（不看尺寸），pixels 按尺寸折算并夹在上下限内，none 记 0。
func TestImageEstimateTwoMethods(t *testing.T) {
	defer session.ResetEstimates()
	session.SetEstimateConfig(session.ImageEstimate{Method: session.ImageMethodFixed, Tokens: 1100})

	if got := session.ImageTokens(1000, 800); got != 1100 {
		t.Errorf("fixed 模式 ImageTokens(1000,800) = %d，期望每张 1100", got)
	}
	if got := session.ImageTokens(0, 0); got != 1100 {
		t.Errorf("fixed 模式未知尺寸 = %d，期望 1100", got)
	}
	if got := session.EstimateSettings().Describe(); got != "fixed 1100/张" {
		t.Errorf("fixed 规则描述 = %q", got)
	}

	session.SetEstimateConfig(session.ImageEstimate{Method: session.ImageMethodPixels, PxPerToken: 750, MinTokens: 85, MaxTokens: 4096})
	if got := session.ImageTokens(1000, 800); got != 1066 {
		t.Errorf("pixels 模式 ImageTokens(1000,800) = %d，期望 1066", got)
	}
	if got := session.EstimateSettings().Describe(); got != "pixels 750px per token（85–4096）" {
		t.Errorf("pixels 规则描述 = %q", got)
	}

	session.SetEstimateConfig(session.ImageEstimate{Method: session.ImageMethodNone})
	if got := session.ImageTokens(1000, 800); got != 0 {
		t.Errorf("none 模式 = %d，期望 0", got)
	}
}

// TestPerModelEstimateOverridesGlobal 钉住"同一个模型可以单独一套规则，且不影响
// 其他模型"：models.<名>.image_tokens 走的就是这条通道（条目名或 wire 模型 id）。
func TestPerModelEstimateOverridesGlobal(t *testing.T) {
	defer session.ResetEstimates()
	session.SetEstimateConfig(session.ImageEstimate{Method: session.ImageMethodPixels, PxPerToken: 750, MinTokens: 85, MaxTokens: 4096})
	session.SetModelEstimate("text", session.ImageEstimate{Method: session.ImageMethodFixed, Tokens: 1100})
	session.SetModelEstimate("glm-5.3-flash-official", session.ImageEstimate{Method: session.ImageMethodPixels, PxPerToken: 780, MinTokens: 85, MaxTokens: 8192})

	// 配了单独规则的模型用自己那套（含按 wire id 大小写不敏感匹配）。
	if got := session.ImageTokensForModel("text", 1000, 800); got != 1100 {
		t.Errorf("text = %d，期望 fixed 1100", got)
	}
	if got := session.ImageTokensForModel("GLM-5.3-Flash-Official", 2880000, 1); got != 3692 {
		t.Errorf("glm = %d，期望 2880000/780 = 3692", got)
	}
	// 没配的模型跟着全局默认走。
	if got := session.ImageTokensForModel("wire-B", 1000, 800); got != 1066 {
		t.Errorf("未配置模型 = %d，期望继承全局 1066", got)
	}
	// 全局改了，单独配置的模型不受影响。
	session.SetEstimateConfig(session.ImageEstimate{Method: session.ImageMethodNone})
	if got := session.ImageTokensForModel("text", 1000, 800); got != 1100 {
		t.Errorf("改全局后 text = %d，期望仍是自己那套 1100", got)
	}
	if got := session.ImageTokensForModel("wire-B", 1000, 800); got != 0 {
		t.Errorf("改全局后未配置模型 = %d，期望 0", got)
	}
}

// TestMetaInfoCarriesPromptTokenEstimate 钉住侧栏/详情栏那条"系统提示词快照"
// 提示有 token 口径可用（字符数是精确值，token 是本地估算）。
func TestMetaInfoCarriesPromptTokenEstimate(t *testing.T) {
	root := t.TempDir()
	prompt := strings.Repeat("系统提示词很长的一段。\n", 40)
	writeFile(t, jsonl(root, "proj", "work", "sessions", "meta_01.jsonl"), transcript(
		`{"t":"meta","kind":"system","model":"wire-A","text":`+jsonString(prompt)+`}`,
		`{"t":"msg","role":"user","text":"hi"}`,
	))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Meta == nil {
		t.Fatalf("没有读到元信息: %+v", sessions)
	}
	m := sessions[0].Meta
	if m.PromptChars == 0 {
		t.Fatal("字符数没读到")
	}
	if m.PromptTokenEst != session.TextTokens(prompt) {
		t.Fatalf("提示词 token 估算 = %d，期望 %d", m.PromptTokenEst, session.TextTokens(prompt))
	}
}

// TestSessionCarriesItsOwnEstimateRule 钉住页面数据里带着**本会话那条模型**的估算
// 规则：详情栏解释"≈ 图片 token 是怎么来的"时必须引用真实配置（多模型可以各选一套），
// 不能写死一份会过期的副本，也不能再假设全页只有一条规则。
func TestSessionCarriesItsOwnEstimateRule(t *testing.T) {
	defer session.ResetEstimates()
	session.SetEstimateConfig(session.ImageEstimate{Method: session.ImageMethodPixels, PxPerToken: 640, MinTokens: 90, MaxTokens: 3000})
	session.SetModelEstimate("wire-B", session.ImageEstimate{Method: session.ImageMethodFixed, Tokens: 950})

	root := t.TempDir()
	for _, tc := range []struct{ file, model string }{{"est_a.jsonl", "wire-A"}, {"est_b.jsonl", "wire-B"}} {
		writeFile(t, jsonl(root, "proj", "work", "sessions", tc.file), transcript(
			`{"t":"meta","kind":"system","model":"`+tc.model+`","text":"提示词"}`,
			`{"t":"msg","role":"user","text":"hi"}`,
		))
	}
	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byModel := map[string]*SessionEstimate{}
	for _, s := range sessions {
		if s.Estimate == nil {
			t.Fatalf("会话 %s 没有带上估算规则", s.ID)
		}
		byModel[s.Estimate.Model] = s.Estimate
	}
	global := byModel["wire-A"]
	if global == nil || global.Method != session.ImageMethodPixels || global.PxPerToken != 640 || global.Min != 90 || global.Max != 3000 {
		t.Fatalf("继承全局默认的会话规则不对: %+v", global)
	}
	own := byModel["wire-B"]
	if own == nil || own.Method != session.ImageMethodFixed || own.Tokens != 950 || own.Rule != "fixed 950/张" {
		t.Fatalf("有单独规则的会话规则不对: %+v", own)
	}

	raw, err := json.Marshal(sessions[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	html := string(raw)
	for _, want := range []string{`"method":`, `"pxPerToken":640`, `"min":90`, `"max":3000`, `"rule":"pixels 640px per token（90–3000）"`} {
		if !strings.Contains(html, want) {
			t.Errorf("页面数据里缺少估算规则 %s（%s）", want, html)
		}
	}
}

// jsonString 把一个 Go 字符串写成 JSON 字符串字面量（测试夹具用）。
func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// TestViewerUnitToggleSourceRules 钉住显示单位开关的源码契约（真实渲染的行为
// 断言在 TestViewerUnitToggleInChromium）：
//
//	· 默认 token，选择落 localStorage，且切换后立刻重画三块内容；
//	· 计数文案只有一个来源 countText：token 形态一律带 ≈，字符形态不带；
//	· 详情栏的厂商数字（输入 tokens、输出 tokens…）不经过 countText。
