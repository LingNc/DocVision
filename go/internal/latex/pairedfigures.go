package latex

import (
	"regexp"
	"strconv"
	"strings"
)

// pairMaxGap 是"两张图算不算一对"的行距上限（P79）：一个图的围栏本身就有
// 二三十行，并排的 (a)/(b) 之间只隔一个围栏与图注，正常在 60 行以内；再远
// 就是正文里的两张不同图了。
const pairMaxGap = 60

// figureCaptionRe 认"图注"行：中文 `图 8.1`／`表 3.2`、英文 `Figure 8.1`／
// `Fig. 3`。中间夹着图注就说明这是**两张各自有编号**的图，不是一对；(a)/(b)
// 这类子图标注不算图注——它们本来就说明"同一张图的两半"。
var figureCaptionRe = regexp.MustCompile(`^\s*(图|表|Figure|Fig\.?)\s*[\d一二三四五六七八九十]`)

// pairedFigureNote 找出本章里"成对"的矢量图（P79）：两个围栏之间既没有小标题、
// 也没有各自的图注，说明原书里它们是同一个图号下的 (a)/(b)、或并排摆着的两半。
// 转换时要让这一对共享同一套线条粗细/配色/字号/箭头，而不是各自沿用自己围栏里
// 的那套（那会让左右两边粗细不一、颜色不同）。
//
// 只认矢量图（`LINK: [vector](`）：纯位图没有"线条样式"这回事。返回值是给会话
// 首条消息追加的一段英文说明，没有成对图就返回空串。
func pairedFigureNote(mdLines []string) string {
	const maxPairs = 5
	starts := make([]int, 0, 8) // 0-based 行号
	headings := make([]int, 0, 8)
	for i, line := range mdLines {
		t := strings.TrimSpace(line)
		if n := len(t) - len(strings.TrimLeft(t, "#")); n >= 1 && n <= 6 && (n == len(t) || t[n] == ' ') {
			headings = append(headings, i)
			continue
		}
		if strings.HasPrefix(t, "LINK: [vector](") {
			starts = append(starts, i)
		}
	}
	if len(starts) < 2 {
		return ""
	}
	separates := func(a, b int) bool {
		for _, h := range headings {
			if h > a && h < b {
				return true
			}
		}
		for i := a + 1; i < b && i < len(mdLines); i++ {
			if figureCaptionRe.MatchString(mdLines[i]) {
				return true
			}
		}
		return false
	}
	groups := make([][]int, 0, 4)
	cur := []int{starts[0]}
	for _, st := range starts[1:] {
		prev := cur[len(cur)-1]
		if st-prev <= pairMaxGap && !separates(prev, st) {
			cur = append(cur, st)
			continue
		}
		if len(cur) > 1 {
			groups = append(groups, cur)
		}
		cur = []int{st}
	}
	if len(cur) > 1 {
		groups = append(groups, cur)
	}
	if len(groups) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("PAIRED FIGURES (P79): these vector figures belong to ONE figure number in the book (side-by-side halves or (a)/(b)), so they must look like ONE set — the same line weights, colour palette, font size and arrow heads for every figure in the group (pick one choice from the manual's figure style and apply it to all of them). Each figure keeps its own geometry and labels; when you place them side by side, match their heights too. If a group below is really two unrelated figures, ignore it.")
	shown := 0
	for _, g := range groups {
		if shown >= maxPairs {
			break
		}
		nums := make([]string, 0, len(g))
		for _, l := range g {
			nums = append(nums, strconv.Itoa(l+1))
		}
		b.WriteString("\n- chapter lines " + strings.Join(nums, " + ") + " form one group.")
		shown++
	}
	if len(groups) > shown {
		b.WriteString("\n- (and " + strconv.Itoa(len(groups)-shown) + " more group(s) further down.)")
	}
	return b.String()
}
