package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"mineru-tools/internal/config"
	"mineru-tools/internal/session"
	"mineru-tools/internal/sessionview"
)

// sessionsCmdFor 造一个只用来解析 flag 的 sessions 命令（不执行 RunE）。
func sessionsCmdFor(t *testing.T) (*cobra.Command, func(args ...string)) {
	t.Helper()
	cmd := newSessionsCmd()
	parse := func(args ...string) {
		if err := cmd.ParseFlags(args); err != nil {
			t.Fatalf("ParseFlags(%v): %v", args, err)
		}
	}
	return cmd, parse
}

// TestSessionsRootPrefersFlag 钉住 --dir 仍然最优先（相对路径按启动目录解析）。
func TestSessionsRootPrefersFlag(t *testing.T) {
	start := t.TempDir()
	cmd, parse := sessionsCmdFor(t)
	parse("--dir", "sub/dir")
	cfg := &config.Config{}
	cfg.Paths.LatexProject = filepath.Join(start, "from-config")

	root, src := sessionsRoot(cmd, cfg, start)
	if root != filepath.Join(start, "sub", "dir") {
		t.Errorf("root = %q，期望 --dir 解析后的路径", root)
	}
	if src != "--dir" {
		t.Errorf("来源 = %q，期望 --dir", src)
	}
}

// TestSessionsRootComesFromConfig 钉住默认目录确实来自配置（用户报的就是这里：
// 单独跑 sessions --serve 时没用配置），档位2 取 latex_output。
func TestSessionsRootComesFromConfig(t *testing.T) {
	start := t.TempDir()
	project := filepath.Join(start, "latex_project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	level2 := filepath.Join(start, "latex_output")
	if err := os.MkdirAll(level2, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Paths.LatexProject = project
	cfg.Paths.LatexOutput = level2

	cmd, _ := sessionsCmdFor(t)
	root, src := sessionsRoot(cmd, cfg, start)
	if root != project || src != "config paths.latex_project" {
		t.Errorf("档位1 root = %q / %q，期望 %q / config paths.latex_project", root, src, project)
	}

	cfg.Latex.Level = 2
	root, src = sessionsRoot(cmd, cfg, start)
	if root != level2 || src != "config paths.latex_output" {
		t.Errorf("档位2 root = %q / %q，期望 %q / config paths.latex_output", root, src, level2)
	}
}

// TestSessionsRootFallsBackAndSaysWhy 钉住退回当前目录时横幅能说出原因（目录不存在
// 与没有配置是两回事）。
func TestSessionsRootFallsBackAndSaysWhy(t *testing.T) {
	start := t.TempDir()
	cmd, _ := sessionsCmdFor(t)

	cfg := &config.Config{}
	cfg.Paths.LatexProject = filepath.Join(start, "does-not-exist")
	root, src := sessionsRoot(cmd, cfg, start)
	if root != start {
		t.Errorf("配置目录不存在时应退回启动目录，得到 %q", root)
	}
	if !strings.Contains(src, "当前目录") || !strings.Contains(src, "paths.latex_project") {
		t.Errorf("来源 = %q，应说明是当前目录且指出配置项", src)
	}

	root, src = sessionsRoot(cmd, nil, start)
	if root != start || !strings.Contains(src, "未找到 config") {
		t.Errorf("无配置时 root = %q / %q，期望启动目录 + 未找到 config", root, src)
	}
}

// TestSessionsAddrPrecedence 钉住监听地址的优先级：
// --addr > --port > 配置 preview.host/port > 内置默认。
func TestSessionsAddrPrecedence(t *testing.T) {
	cfg := &config.Config{}
	cfg.Preview.Host = "0.0.0.0"
	cfg.Preview.Port = 8848

	// 没有配置：内置默认。
	cmd, _ := sessionsCmdFor(t)
	if addr, src := sessionsAddr(cmd, nil); addr != sessionview.DefaultAddr || src != "内置默认" {
		t.Errorf("无配置 addr = %q / %q，期望内置默认", addr, src)
	}
	// 有配置：preview.host/port。
	if addr, src := sessionsAddr(cmd, cfg); addr != "0.0.0.0:8848" || src != "config preview.host/port" {
		t.Errorf("配置 addr = %q / %q，期望 0.0.0.0:8848 / config preview.host/port", addr, src)
	}
	// --port 覆盖端口，host 仍取配置。
	cmd, parse := sessionsCmdFor(t)
	parse("--port", "9000")
	if addr, src := sessionsAddr(cmd, cfg); addr != "0.0.0.0:9000" || src != "--port" {
		t.Errorf("--port addr = %q / %q，期望 0.0.0.0:9000 / --port", addr, src)
	}
	// --addr 最优先。
	cmd, parse = sessionsCmdFor(t)
	parse("--port", "9000", "--addr", "127.0.0.1:7777")
	if addr, src := sessionsAddr(cmd, cfg); addr != "127.0.0.1:7777" || src != "--addr" {
		t.Errorf("--addr addr = %q / %q，期望 127.0.0.1:7777 / --addr", addr, src)
	}
}

// TestSessionsConfigNote 钉住配置那一行的三种事实：读到哪个文件 / 没找到 /
// 文件在但读不了（三种不能长得一样）。
func TestSessionsConfigNote(t *testing.T) {
	saved := resolvedConfigPath
	defer func() { resolvedConfigPath = saved }()

	resolvedConfigPath = "/tmp/x/config.yaml"
	if got := sessionsConfigNote(&config.Config{}); got != "/tmp/x/config.yaml" {
		t.Errorf("读到配置时 = %q", got)
	}
	resolvedConfigPath = ""
	if got := sessionsConfigNote(nil); got != "未找到" {
		t.Errorf("没有配置时 = %q", got)
	}
}

// TestSessionsBrokenConfigAborts 钉住 T26：配置文件存在但损坏（这里用
// extends 引用不存在的条目，就是用户现场那类错误）时，sessions 必须**返回错误
// 终止**，而不是打一行提示后带着默认目录/端口继续跑。
func TestSessionsBrokenConfigAborts(t *testing.T) {
	saved := resolvedConfigPath
	savedWd, _ := os.Getwd()
	defer func() {
		resolvedConfigPath = saved
		_ = os.Chdir(savedWd)
	}()

	dir := t.TempDir()
	broken := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(broken, []byte("models:\n  drawing:\n    extends: nothinking\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadConfig(broken); err == nil {
		t.Fatal("预置条件失败：这份配置应当加载报错")
	}
	resolvedConfigPath = broken
	cmd, _ := sessionsCmdFor(t)
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("sessions 对损坏的配置应报错终止（T26）")
	}
}

// TestSessionsFlagsRegistered 钉住命令行面：--port 存在、--addr 默认留空
// （默认值由配置决定，flag 默认值不能再写死 8848，否则配置永远排不上）。
func TestSessionsFlagsRegistered(t *testing.T) {
	cmd, _ := sessionsCmdFor(t)
	for _, name := range []string{"dir", "serve", "addr", "port", "list", "unit", "out", "cost"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("缺少 --%s", name)
		}
	}
	if got := cmd.Flags().Lookup("addr").DefValue; got != "" {
		t.Errorf("--addr 默认值 = %q，必须留空让配置生效", got)
	}
}

// TestApplyEstimateConfigReachesEstimator 钉住配置真的走到估算器：全局 estimate
// 是默认，models.<名>.image_tokens 只覆盖那一条，且 wire 模型 id 也认（会话与
// 预览页只知道 wire id，不知道条目名）。
func TestApplyEstimateConfigReachesEstimator(t *testing.T) {
	defer session.ResetEstimates()
	cfg := &config.Config{
		Estimate: config.EstimateConfig{Method: config.EstimateMethodPixels, PxPerToken: 750, MinTokens: 85, MaxTokens: 4096},
		Models: map[string]config.ModelConfig{
			"text":     {Model: "wire-default"},
			"drawing":  {Model: "glm-5.3-flash", ImageTokens: &config.EstimateConfig{Method: config.EstimateMethodFixed, Tokens: 1050}},
			"verifier": {Model: "glm-5.3-flash-official", ImageTokens: &config.EstimateConfig{MaxTokens: 8192}},
		},
	}
	applyEstimateConfig(cfg)

	if got := session.ImageTokensForModel("wire-unknown", 1000, 800); got != 1066 {
		t.Errorf("未配置模型 = %d，期望继承全局 1066", got)
	}
	// 条目名与 wire 模型 id 都能命中同一条覆盖。
	for _, key := range []string{"drawing", "GLM-5.3-Flash"} {
		if got := session.ImageTokensForModel(key, 1000, 800); got != 1050 {
			t.Errorf("%s = %d，期望 fixed 1050", key, got)
		}
	}
	// 只覆盖 max_tokens 的条目：其余继承全局。
	if got := session.EstimateForModel("glm-5.3-flash-official"); got.Method != config.EstimateMethodPixels ||
		got.MaxTokens != 8192 || got.MinTokens != 85 {
		t.Errorf("部分覆盖的规则 = %+v", got)
	}
}

// ---------------------------------------------------------------------------
// --list 的「提示词」列：默认 token 估算（带 ≈），--unit char 切回精确字符数
// ---------------------------------------------------------------------------

// 假的提示词快照：字符数与本地估算刻意不成比例（6247 字符 → 1512 tokens），因此
// 断言 "≈ 1.5k" 就证明这一列取的是 Meta.PromptTokenEst（估算器算好的那个数），
// 而不是拿字符数除个系数凑的（6247/4 ≈ 1.6k）。
const (
	listCharsSmall = 900
	listCharsMid   = 6247
	listCharsBig   = 120000
	listTokSmall   = 980
	listTokMid     = 1512
	listTokBig     = 62384
)

func listMeta(chars, tokens int) *sessionview.MetaInfo {
	return &sessionview.MetaInfo{Count: 1, PromptChars: chars, PromptTokenEst: tokens, Model: "glm-4.6", Tools: 12}
}

func listRow(id, project, title string, messages int, bytes int64, meta *sessionview.MetaInfo) sessionview.SessionInfo {
	return sessionview.SessionInfo{
		ID: id, Project: project, Title: title, Messages: messages,
		Bytes: bytes, ModTime: time.Date(2026, 9, 12, 10, 30, 0, 0, time.UTC), Meta: meta,
	}
}

// listFixtureASCII 全 ASCII 假数据（项目名、会话名、路径都不含中文）。
func listFixtureASCII() []sessionview.SessionInfo {
	return []sessionview.SessionInfo{
		listRow("work/style_session.jsonl", "book", "style", 12, 4096, listMeta(listCharsMid, listTokMid)),
		listRow("work/sessions/chapters.jsonl", "book", "chapters", 3, 12*1024, listMeta(listCharsSmall, listTokSmall)),
		listRow("source/sessions/vector_x.jsonl", "other", "convert_chapter_01", 1234, 3*1024*1024, listMeta(listCharsBig, listTokBig)),
	}
}

// listFixtureCJK 含中文的假数据：同一列里的中文宽度一致（同一个项目组的真实行就是
// 这样），于是"每行列的显示起点相同"才是个有意义的断言。
func listFixtureCJK() []sessionview.SessionInfo {
	return []sessionview.SessionInfo{
		listRow("work/style_session.jsonl", "（根目录）", "样式会话", 12, 4096, listMeta(listCharsMid, listTokMid)),
		listRow("work/sessions/chapters.jsonl", "（根目录）", "章节划分", 3, 12*1024, listMeta(listCharsSmall, listTokSmall)),
		listRow("source/sessions/vector_x.jsonl", "（根目录）", "逐图校验", 1234, 3*1024*1024, listMeta(listCharsBig, listTokBig)),
	}
}

// listFixtureNoSnapshot 混一行"转录里没有 t=meta 行"的老会话（提示词列是 "-"）。
func listFixtureNoSnapshot() []sessionview.SessionInfo {
	return []sessionview.SessionInfo{
		listRow("work/style_session.jsonl", "book", "style", 12, 4096, listMeta(listCharsMid, listTokMid)),
		listRow("work/sessions/chapters.jsonl", "book", "chapters", 3, 12*1024, nil),
	}
}

// captureSessionsTable 跑一遍 printSessions 把表格文本取回来。printSessions 直接写
// os.Stdout，测试里换掉这个变量即可——不必为测试给生产代码加一层 io.Writer。
func captureSessionsTable(t *testing.T, sessions []sessionview.SessionInfo, unit string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "list-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdout
	os.Stdout = f
	printSessions(sessions, "/root", "--dir", unit)
	os.Stdout = old
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tableLines 把表格与后面的汇总行分开：汇总前面有一个空行。
func tableLines(t *testing.T, table string) (header string, rows []string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("表格是空的")
	}
	for i, l := range lines {
		if l == "" {
			return lines[0], lines[1:i]
		}
	}
	return lines[0], lines[1:]
}

// displayWidth 是终端显示宽度：CJK / 全角按 2 格，其余按 1 格。
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0xA4CF,
			r >= 0xAC00 && r <= 0xD7A3, r >= 0xF900 && r <= 0xFAFF,
			r >= 0xFE30 && r <= 0xFE4F, r >= 0xFF00 && r <= 0xFF60,
			r >= 0xFFE0 && r <= 0xFFE6, r >= 0x20000:
			n += 2
		default:
			n++
		}
	}
	return n
}

// cellStarts 找出这一行每个字段的起始位置，同时给出 rune 偏移与显示宽度偏移：
// tabwriter 按 rune 数补空格（`b.cell.width += utf8.RuneCount(...)`），而"对齐"
// 是这张表在终端里的样子，两个口径都要看。
//
// 字段边界靠"≥2 个空格"（tabwriter 的填充至少 2 个空格，而表里的字段内容——包括
// 「4.0 KB」「2026-09-12 10:30:00」——不含连续空格）。
func cellStarts(line string) (runeOffsets, displayOffsets []int) {
	rs := []rune(line)
	i := 0
	for i < len(rs) {
		runeOffsets = append(runeOffsets, i)
		displayOffsets = append(displayOffsets, displayWidth(string(rs[:i])))
		for i < len(rs) {
			if rs[i] == ' ' && i+1 < len(rs) && rs[i+1] == ' ' {
				for i < len(rs) && rs[i] == ' ' {
					i++
				}
				break
			}
			i++
		}
	}
	return runeOffsets, displayOffsets
}

// cells 按同一套规则切出字段内容（去掉填充空格）。
func cells(line string) []string {
	rs := []rune(line)
	starts, _ := cellStarts(line)
	out := make([]string, 0, len(starts))
	for i, st := range starts {
		end := len(rs)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		out = append(out, strings.TrimRight(string(rs[st:end]), " "))
	}
	return out
}

// listColumns 是表头顺序，也是"别的列一个都没动"的参照。
var listColumns = []string{"项目", "阶段", "消息数", "提示词", "用量", "成本", "大小", "修改时间", "路径"}

// TestSessionsListColumnsUnchanged 钉住表头顺序与列数：只有第 4 列随 --unit 换词。
func TestSessionsListColumnsUnchanged(t *testing.T) {
	header, rows := tableLines(t, captureSessionsTable(t, listFixtureASCII(), sessionsUnitToken))
	want := []string{"项目", "阶段", "消息数", "提示词 tokens", "用量", "成本", "大小", "修改时间", "路径"}
	if got := cells(header); !reflect.DeepEqual(got, want) {
		t.Errorf("token 表头 = %q，期望 %q", got, want)
	}
	if len(rows) != 3 {
		t.Fatalf("数据行 = %d，期望 3", len(rows))
	}
	for _, line := range append([]string{header}, rows...) {
		if got := len(cells(line)); got != len(listColumns) {
			t.Errorf("列数 = %d，期望 %d：%q", got, len(listColumns), line)
		}
	}
}

// TestSessionsListPromptDefaultsToTokens 钉住默认口径：表头写明单位、值是本地估算的
// ≈ 形态，且取的是估算器算好的那个数。
func TestSessionsListPromptDefaultsToTokens(t *testing.T) {
	table := captureSessionsTable(t, listFixtureASCII(), sessionsUnitToken)
	header, rows := tableLines(t, table)
	if !strings.Contains(header, "提示词 tokens") {
		t.Errorf("默认表头 = %q，期望含「提示词 tokens」", header)
	}
	if strings.Contains(table, "字符") {
		t.Error("默认 token 口径下不该出现「字符」")
	}
	for i, want := range []string{"≈ 1.5k", "≈ 980", "≈ 62k"} {
		if got := cells(rows[i])[3]; got != want {
			t.Errorf("第 %d 行提示词列 = %q，期望 %q（整行 %q）", i+1, got, want, rows[i])
		}
	}
}

// TestSessionsListUnitCharIsExactCharacters 钉住 --unit char：表头换词、值是精确
// 字符数、整张表不再出现 ≈（与今天的样子一字不差）。
func TestSessionsListUnitCharIsExactCharacters(t *testing.T) {
	table := captureSessionsTable(t, listFixtureASCII(), sessionsUnitChar)
	header, rows := tableLines(t, table)
	if !strings.Contains(header, "提示词 字符") {
		t.Errorf("char 表头 = %q，期望含「提示词 字符」", header)
	}
	if strings.Contains(table, "≈") {
		t.Error("char 口径下不该出现 ≈（字符数是精确值）")
	}
	for i, want := range []string{"6247 字符", "900 字符", "120000 字符"} {
		if got := cells(rows[i])[3]; got != want {
			t.Errorf("第 %d 行提示词列 = %q，期望 %q（整行 %q）", i+1, got, want, rows[i])
		}
	}
}

// TestSessionsListPromptColumnWithoutSnapshot 没有 t=meta 行的老转录在两种口径下都是
// "-"（不能显示成 0：0 会被读成"提示词是空的"）。
func TestSessionsListPromptColumnWithoutSnapshot(t *testing.T) {
	for _, unit := range []string{sessionsUnitToken, sessionsUnitChar} {
		_, rows := tableLines(t, captureSessionsTable(t, listFixtureNoSnapshot(), unit))
		if got := cells(rows[1])[3]; got != "-" {
			t.Errorf("%s：无提示词快照的行 = %q，期望 -", unit, got)
		}
	}
}

// TestSessionsListOtherColumnsIdenticalAcrossUnits 钉住"只有提示词列跟着单位走"：
// 其余 8 列在两种单位下逐字段完全相同，汇总行也一模一样。
func TestSessionsListOtherColumnsIdenticalAcrossUnits(t *testing.T) {
	tok := captureSessionsTable(t, listFixtureCJK(), sessionsUnitToken)
	chr := captureSessionsTable(t, listFixtureCJK(), sessionsUnitChar)
	tokHeader, tokRows := tableLines(t, tok)
	chrHeader, chrRows := tableLines(t, chr)

	sameExceptPrompt := func(a, b string) {
		t.Helper()
		ca, cb := cells(a), cells(b)
		if len(ca) != len(cb) {
			t.Fatalf("列数不同：%q / %q", a, b)
		}
		for i := range ca {
			if i == 3 {
				continue // 第 4 列就是被测的那一列
			}
			if ca[i] != cb[i] {
				t.Errorf("第 %d 列 = %q（token）/ %q（char），期望相同", i+1, ca[i], cb[i])
			}
		}
	}
	sameExceptPrompt(tokHeader, chrHeader)
	for i := range tokRows {
		sameExceptPrompt(tokRows[i], chrRows[i])
	}
	if a, b := tok[strings.Index(tok, "\n\n"):], chr[strings.Index(chr, "\n\n"):]; a != b {
		t.Errorf("汇总行不同：\n%q\n%q", a, b)
	}
}

// TestSessionsListColumnsAlign 钉住定宽对齐：两种单位 × 纯 ASCII / 含中文假数据，
// 所有行（含表头）的列起点在 rune 口径上一致（tabwriter 的排布不变量），数据行的列
// 起点在**显示宽度**口径（中文算 2 格）上也一致——即中文列宽与变宽的数字/估算值都
// 没有把后面的列挤歪。
func TestSessionsListColumnsAlign(t *testing.T) {
	cases := []struct {
		name     string
		sessions []sessionview.SessionInfo
	}{
		{"ascii", listFixtureASCII()},
		{"cjk", listFixtureCJK()},
	}
	for _, c := range cases {
		for _, unit := range []string{sessionsUnitToken, sessionsUnitChar} {
			header, rows := tableLines(t, captureSessionsTable(t, c.sessions, unit))
			all := append([]string{header}, rows...)

			wantRunes, _ := cellStarts(all[0])
			for _, line := range all[1:] {
				gotRunes, _ := cellStarts(line)
				if !reflect.DeepEqual(gotRunes, wantRunes) {
					t.Errorf("%s/%s：列起点(rune) = %v，期望 %v：%q", c.name, unit, gotRunes, wantRunes, line)
				}
			}
			wantDisp := []int(nil)
			for i, line := range rows {
				_, gotDisp := cellStarts(line)
				if i == 0 {
					wantDisp = gotDisp
					continue
				}
				if !reflect.DeepEqual(gotDisp, wantDisp) {
					t.Errorf("%s/%s：列起点(显示宽度) = %v，期望 %v：%q", c.name, unit, gotDisp, wantDisp, line)
				}
			}
		}
	}

}

// TestSessionsListTokenColumnAlignsWithoutSnapshot 默认（token）口径下，混进一行"没有
// 提示词快照"的老会话（该列是 "-"）也不会歪：token 值全是 ASCII，rune 补位与显示补位
// 一致（char 口径的「N 字符」与 "-" 混排按 rune 补位，与改动前一样）。
func TestSessionsListTokenColumnAlignsWithoutSnapshot(t *testing.T) {
	header, rows := tableLines(t, captureSessionsTable(t, listFixtureNoSnapshot(), sessionsUnitToken))
	if len(header) == 0 || len(rows) != 2 {
		t.Fatalf("表格形状不对：header=%q rows=%q", header, rows)
	}
	_, wantDisp := cellStarts(rows[0])
	for _, line := range rows[1:] {
		if _, gotDisp := cellStarts(line); !reflect.DeepEqual(gotDisp, wantDisp) {
			t.Errorf("列起点(显示宽度) = %v，期望 %v：%q", gotDisp, wantDisp, line)
		}
	}
	if got := cells(rows[1])[3]; got != "-" {
		t.Errorf("无提示词快照的行 = %q，期望 -", got)
	}
	_, headerDisp := cellStarts(header)
	if len(headerDisp) != len(listColumns) {
		t.Errorf("表头列起点 = %v，期望 %d 列", headerDisp, len(listColumns))
	}
}

// TestSessionsUnitFlag 钉住开关本身：默认 token、char 可切、忽略大小写与首尾空白、
// 其它取值报错并列出可选值。
func TestSessionsUnitFlag(t *testing.T) {
	cmd, parse := sessionsCmdFor(t)
	if got := cmd.Flags().Lookup("unit").DefValue; got != sessionsUnitToken {
		t.Errorf("--unit 默认值 = %q，期望 %q", got, sessionsUnitToken)
	}
	if got, err := sessionsUnit(cmd); err != nil || got != sessionsUnitToken {
		t.Errorf("默认 = %q, %v；期望 token, nil", got, err)
	}
	for _, in := range []string{"char", " CHAR ", "Token"} {
		parse("--unit", in)
		got, err := sessionsUnit(cmd)
		if err != nil || got != strings.ToLower(strings.TrimSpace(in)) {
			t.Errorf("--unit %q = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"chars", "tokens", "", "字"} {
		parse("--unit", in)
		_, err := sessionsUnit(cmd)
		if err == nil {
			t.Fatalf("--unit %q 应当报错", in)
		}
		for _, want := range []string{"--unit 取值无效", "token|char"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("--unit %q 的报错 = %q，期望含 %q", in, err, want)
			}
		}
	}
}

// TestSessionsListInvalidUnitFailsTheCommand 端到端：非法 --unit 让命令失败，不会
// 悄悄按默认口径打一张表出来。
func TestSessionsListInvalidUnitFailsTheCommand(t *testing.T) {
	oldErr := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	os.Stderr = devnull
	defer func() { os.Stderr = oldErr }()

	// T26 起配置读失败直接终止，本命令要走到 --unit 校验就必须先有份能加载的
	// 配置（T26 测试同款的最小配置），并给裸命令补上 --config 持久旗标。
	validCfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(validCfg, []byte("config_version: 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	saved := resolvedConfigPath
	defer func() { resolvedConfigPath = saved }()
	resolvedConfigPath = validCfg

	cmd := newSessionsCmd()
	cmd.PersistentFlags().StringP("config", "c", "", "配置文件路径（测试桩）")
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--list", "--unit", "bogus", "--dir", t.TempDir()})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("非法 --unit 应当让命令失败")
	}
	for _, want := range []string{"--unit 取值无效", "token|char"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("报错 = %q，期望含 %q", err, want)
		}
	}
}
