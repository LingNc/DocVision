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
	defer session.SetEstimateConfig(session.EstimateConfig{})
	session.SetEstimateConfig(session.EstimateConfig{ImagePxPerToken: 750, ImageTokensMin: 85, ImageTokensMax: 4096, ImageFallback: 1100})

	cases := []struct {
		w, h, want int
		why        string
	}{
		{1000, 800, 1066, "常规插图按 宽×高/750 折算"},
		{10, 10, 85, "极小图有下限"},
		{4000, 4000, 4096, "超大图有上限"},
		{0, 0, 1100, "取不到尺寸退回兜底常量"},
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

	// 配置能改规则（estimate.image_px_per_token 等），且缺项自动补默认值。
	session.SetEstimateConfig(session.EstimateConfig{ImagePxPerToken: 1000})
	if got := session.ImageTokens(1000, 800); got != 800 {
		t.Errorf("改配置后 ImageTokens(1000,800) = %d，期望 800", got)
	}
	if got := session.ImageTokens(0, 0); got != 1100 {
		t.Errorf("未配置的兜底常量应为默认 1100，实际 %d", got)
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

// TestPageDataCarriesEstimatePolicy 钉住页面数据里带着估算规则本身：详情栏
// 解释"≈ 图片 token 是怎么来的"时必须引用真实配置，不能写死一份会过期的副本。
func TestPageDataCarriesEstimatePolicy(t *testing.T) {
	defer session.SetEstimateConfig(session.EstimateConfig{})
	session.SetEstimateConfig(session.EstimateConfig{ImagePxPerToken: 640, ImageTokensMin: 90, ImageTokensMax: 3000, ImageFallback: 900})

	raw, err := json.Marshal(pageData{Mode: "static", Estimate: CurrentEstimatePolicy()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	html := string(raw)
	for _, want := range []string{`"pxPerToken":640`, `"min":90`, `"max":3000`, `"fallback":900`} {
		if !strings.Contains(html, want) {
			t.Errorf("页面数据里缺少估算规则 %s", want)
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
func TestViewerUnitToggleSourceRules(t *testing.T) {
	js := readAsset(t, "viewer.js")
	html := readAsset(t, "viewer.html")

	if !strings.Contains(html, `id="unit-toggle"`) || !strings.Contains(html, `>token</button>`) {
		t.Error("页签行里没有显示单位开关（默认文案必须是 token）")
	}
	for _, want := range []string{
		"state.unit = storeGet('unit') === 'char' ? 'char' : 'token';",
		"storeSet('unit', state.unit);",
		"refs.unitToggle.textContent = state.unit === 'char' ? '字符' : 'token';",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("开关缺 %q", want)
		}
	}
	// 切换后立刻重画（不刷新页面）：消息流 + 轨迹 + 详情栏 + 侧栏提示。
	idx := strings.Index(js, "refs.unitToggle.addEventListener('click'")
	if idx < 0 {
		t.Fatal("开关没有接线")
	}
	handler := js[idx:]
	if end := strings.Index(handler, "});"); end > 0 {
		handler = handler[:end]
	}
	for _, want := range []string{"renderTimeline();", "renderTrajectory();", "renderDetails();", "refreshList();"} {
		if !strings.Contains(handler, want) {
			t.Errorf("切换单位后没有重画：缺 %s", want)
		}
	}
	// 唯一文案来源：token 形态带 ≈，字符形态是精确数字。
	body := jsFunc(t, js, "countText")
	if !strings.Contains(body, "'≈ ' + fmtTokens(tokens) + ' tokens'") {
		t.Error("token 形态没有带 ≈ 前缀（本地估算必须与厂商数字区分开）")
	}
	if !strings.Contains(body, "(Number(chars) || 0) + ' 字符'") {
		t.Error("字符形态不是精确字符数")
	}
	// 详情栏指标用的是厂商实测值，不能走 countText（不能出现 ≈）。
	stats := jsFunc(t, js, "detailsStatsBlock")
	for _, want := range []string{"tile('输入 tokens', fmtTokens(st.promptTokens)", "tile('输出 tokens', fmtTokens(st.completionTokens)"} {
		if !strings.Contains(stats, want) {
			t.Errorf("详情栏指标瓦片缺 %q", want)
		}
	}
	if strings.Contains(stats, "countText(st.promptTokens") || strings.Contains(stats, "≈ ' + fmtTokens(st.promptTokens)") {
		t.Error("详情栏的厂商实测值被套上了 ≈")
	}
	// 图片瓦片：按配置里的规则说明依据，且始终是 token 口径（带 ≈）。
	if !strings.Contains(stats, "tile('图片 ' + imgs.count + ' 张', '≈ ' + fmtTokens(imgs.tokens)") {
		t.Error("详情栏没有单列图片 token 的本地估算")
	}
	if !strings.Contains(stats, "pol.pxPerToken") || !strings.Contains(stats, "pol.fallback") {
		t.Error("图片瓦片没有引用配置里的折算规则（依据说不清）")
	}
}
