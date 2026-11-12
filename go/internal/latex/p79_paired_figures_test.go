package latex

import (
	"strings"
	"testing"
)

// P79：成对的矢量图（并排 / (a)(b)）要在转换时当成一套来排版。
func TestPairedFigureNote(t *testing.T) {
	fence := "LINK: [vector](images/b/x.jpg)"
	// farPair 生成"两张图之间隔着 70 行正文"的章节。
	farPair := func(f string) []string {
		out := []string{f}
		for i := 0; i < 70; i++ {
			out = append(out, "正文")
		}
		return append(out, f)
	}

	cases := []struct {
		name string
		md   []string
		want []string // 期望出现的片段；空 = 期望返回空串
		miss []string
	}{
		{
			name: "两张挨着、(a)/(b) 同图号 → 一对",
			md: []string{
				"## 例9", fence, "```latex", "\\draw (0,0);", "```", "(a)",
				fence, "```latex", "\\draw (1,1);", "```", "(b)", "图 8.9",
			},
			want: []string{"PAIRED FIGURES", "chapter lines 2 + 7"},
		},
		{
			name: "中间夹着各自的图注 → 不是一对",
			md:   []string{fence, "图 8.1", "", fence, "图 8.2"},
			miss: []string{"PAIRED FIGURES"},
		},
		{
			name: "隔得很远 → 不是一对",
			md:   farPair(fence),
			miss: []string{"PAIRED FIGURES"},
		},
		{
			name: "中间有小标题 → 隔断",
			md:   []string{fence, "## 下一节", fence},
			miss: []string{"PAIRED FIGURES"},
		},
		{
			name: "三张连成一串 → 一组三张",
			md:   []string{fence, "", fence, "", fence},
			want: []string{"chapter lines 1 + 3 + 5"},
		},
		{
			name: "只有位图 → 不提",
			md:   []string{"![](images/b/a.jpg)", "![](images/b/b.jpg)"},
			miss: []string{"PAIRED FIGURES"},
		},
		{
			name: "单张矢量图 → 不提",
			md:   []string{fence},
			miss: []string{"PAIRED FIGURES"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pairedFigureNote(c.md)
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Fatalf("缺少 %q：\n%s", w, got)
				}
			}
			for _, m := range c.miss {
				if strings.Contains(got, m) {
					t.Fatalf("不该出现 %q：\n%s", m, got)
				}
			}
			if len(c.want) == 0 && got != "" && len(c.miss) == 0 {
				t.Fatalf("期望空串，实际：%s", got)
			}
		})
	}
}
