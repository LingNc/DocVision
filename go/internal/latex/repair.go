package latex

import (
	"encoding/json"
	"os"
	"strings"
)

// T227：续跑前给转换转录做一次体检，把已经被"看不见原书"带偏的那一段截掉。
//
// 现场：续跑时如果重试 watcher 比原书 PDF 视图先起来（RunBook 里的构建顺序），
// 那一轮派发出去的"继续跑"会话没有 source 挂载点，模型照着历史去
// `view_pdf {path:"source:…"}` 只会拿到 `未知挂载点 "source"`；接着 submit 又被
// "必须和原书对照过"打回，于是它写一段"这本书没有 PDF / 我无法继续"的结论收尾，
// 下一轮续跑再原样重复——转录越滚越长，活儿一步没动。
//
// 代码侧的顺序已经修掉了，但**已经坏在磁盘上的会话不会自己好**：续跑回放的还是
// 那段"我做不到"的历史。这里只认两条硬证据：
//
//	① 工具回执里出现「未知挂载点 "source"」（模型确实够不着原书）；
//	② 同一条「还没和原书对照过…你已经看了 N 页」的打回反复出现（N 不变 = 一步
//	   没动，它只是在原地重试）。
//
// 判据只看**会被回放的那一段**（最后一次压缩检查点之后），并且以"后来真的把原书
// 页看进去了"为分界：那之后的证据才算数——回执里出现原书页图就说明这一轮自己
// 走出去了，之前的报错只是过程，再截就是白扔干活记录。
//
// 命中就把历史截到**这条证据之前最后一条消息**，另写成一条新尝试接着跑。源转录
// 一个字不动（回滚 = 换回原来那条），工作树也不动（P55 那套回退是另一件事）。
const (
	mountErrMarker = `未知挂载点 "source"`
	sourceGateMark = "还没有和原书对照过"
	// sourceViewMark 是"真的看到原书页"的回执前缀（view_pdf 打开 source: 的页）。
	sourceViewMark = "PDF page source:"
	submittedMark  = "SUBMITTED"
	// gateSentenceLen 是"同一条打回"的判据长度：打回文案里的页码/页数/页序号都在
	// 这一段的尾巴上，截这么长足以把「看过 N 页」也含进来。
	gateSentenceLen = 220
)

// repairRow 是体检只关心的几个字段（转录行的其余部分忽略）。
type repairRow struct {
	T          string `json:"t"`
	Role       string `json:"role"`
	Kind       string `json:"kind"`
	Text       string `json:"text"`
	ToolCallID string `json:"tool_call_id"`
	ToolCalls  []struct {
		ID       string `json:"id"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tool_calls"`
}

// repairDerailedTranscript 检查一条**将要续跑**的转录：该截的话把前 N 行抄成
// 一条新尝试，返回新路径、新尝试号与一句原因。不动手时 ok=false。
func repairDerailedTranscript(trPath, workRoot string) (path string, attempt int, why string, ok bool) {
	if !fileExists(trPath) || !fileExists(workRoot) {
		return "", 0, "", false
	}
	rows, err := readRepairRows(trPath)
	if err != nil || len(rows) == 0 {
		return "", 0, "", false
	}
	cut, why := analyzeDerailedTranscript(rows)
	if cut < 0 {
		return "", 0, "", false
	}
	dst, next, err := cutTranscriptAt(trPath, cut+1, lastAttemptNumber(transcriptStem(trPath))+1)
	if err != nil {
		return "", 0, "写新尝试失败：" + err.Error(), false
	}
	return dst, next, why, true
}

// analyzeDerailedTranscript 在解析好的行上做判断，返回**要保留到第几行**（0 基
// 下标，含该行；-1 = 不用修）与原因。纯函数，便于单测。
func analyzeDerailedTranscript(rows []repairRow) (int, string) {
	start := replayStart(rows)
	// 已经交过（真的交进去了，不是被门禁打回）就不用修：续跑会重放提交收账。
	if hasAcceptedSubmit(rows, start) {
		return -1, ""
	}
	lo := lastSourceViewIndex(rows, start) + 1
	mountIdx := -1
	gateFirst := map[string]int{}
	gateCount := map[string]int{}
	for i := lo; i < len(rows); i++ {
		r := rows[i]
		if r.T != "msg" || r.Role != "tool" {
			continue
		}
		text := r.Text
		if strings.Contains(text, mountErrMarker) {
			if mountIdx < 0 {
				mountIdx = i
			}
			continue
		}
		if strings.Contains(text, sourceGateMark) {
			key := gateKey(text)
			if _, seen := gateFirst[key]; !seen {
				gateFirst[key] = i
			}
			gateCount[key]++
		}
	}
	best, why := -1, ""
	consider := func(i int, reason string) {
		if i >= 0 && (best < 0 || i < best) {
			best, why = i, reason
		}
	}
	if mountIdx >= 0 {
		consider(mountIdx, "工具报挂载点不存在（那一轮会话够不着原书目录）")
	}
	for key, first := range gateFirst {
		if gateCount[key] >= 2 {
			consider(first, "同一条「还没和原书对照过」打回反复出现（一步没动）")
		}
	}
	if best < 0 {
		return -1, ""
	}
	// 切点 = 这条证据之前最后一条消息行：留到它，后面那串（工具报错 + 那些
	// "我做不到"的结论）不进新尝试。
	cut := -1
	for i := best - 1; i >= 0; i-- {
		if rows[i].T == "msg" {
			cut = i
			break
		}
	}
	// 压缩检查点之前的行本来就不回放：切在那儿等于没切，真截过去反而会把摘要
	// 之前的老历史整段带回来。宁可不动。
	if cut < start {
		return -1, ""
	}
	return cut, why
}

// replayStart 返回回放的起点：最后一次压缩检查点（它之前的行 session 层不会
// 回放）。没有检查点就是 0。
func replayStart(rows []repairRow) int {
	start := 0
	for i, r := range rows {
		if r.T != "msg" || r.Role != "user" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(r.Text), "=== COMPRESSED SESSION CONTEXT") {
			start = i
		}
	}
	return start
}

// hasAcceptedSubmit 判断回放的历史里有没有**真的交进去**的提交（被门禁打回的
// 不算：那种回执正是这条会话卡住的原因）。
func hasAcceptedSubmit(rows []repairRow, start int) bool {
	submits := map[string]bool{}
	for _, r := range rows[min(start, len(rows)):] {
		if r.T != "msg" || r.Role != "assistant" {
			continue
		}
		for _, c := range r.ToolCalls {
			if strings.HasPrefix(c.Function.Name, "submit") {
				submits[c.ID] = true
			}
		}
	}
	if len(submits) == 0 {
		return false
	}
	for _, r := range rows[min(start, len(rows)):] {
		if r.T != "msg" || r.Role != "tool" || !submits[r.ToolCallID] {
			continue
		}
		if strings.Contains(r.Text, submittedMark) {
			return true
		}
	}
	return false
}

// lastSourceViewIndex 返回回放窗口里**最后一次真的看到原书页**的位置（没有就是
// −1）。它之前的历史不再当作"被带偏"的证据：能看进原书页，说明那一轮已经自行
// 走出去了。
func lastSourceViewIndex(rows []repairRow, start int) int {
	last := -1
	for i := start; i < len(rows); i++ {
		r := rows[i]
		if r.T == "msg" && r.Role == "tool" && strings.Contains(r.Text, sourceViewMark) {
			last = i
		}
	}
	return last
}

// gateKey 取打回文案里"同一条"的判据：从标记词起截 gateSentenceLen 个字符，
// 页码与"已经看了 N 页"都在里面。
func gateKey(text string) string {
	i := strings.Index(text, sourceGateMark)
	if i < 0 {
		return text
	}
	s := text[i:]
	if len(s) > gateSentenceLen {
		s = s[:gateSentenceLen]
	}
	return s
}

// readRepairRows 读转录并按行解析（坏行跳过：体检看的是整体形状）。
func readRepairRows(path string) ([]repairRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseRepairRows(data), nil
}

// parseRepairRows 按行解析转录内容，坏行留一条占位（不让一行坏 JSON 打乱行号
// ——行号就是截断点）。
func parseRepairRows(data []byte) []repairRow {
	lines := strings.Split(string(data), "\n")
	rows := make([]repairRow, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row repairRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			row = repairRow{T: "bad"}
		}
		rows = append(rows, row)
	}
	return rows
}
