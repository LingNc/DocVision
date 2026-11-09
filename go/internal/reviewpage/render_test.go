package reviewpage

import (
	"strings"
	"testing"
)

// P64：一张图要能同时看到原图与成品（并排），状态与链接跟在图注里。
func TestFigurePairsOriginalAndConverted(t *testing.T) {
	it := Item{
		Name:   "a.png",
		Ref:    "book.md::images/book/a.png",
		Image:  "images/book/a.png",
		Figure: "../figures/a.png",
		Status: StatusLabel(StatusDone),
		Note:   "vector",
		Links:  []Link{{Label: "判定记录", Href: "../progress_items/book.md/images_book_a.png.json"}},
	}
	html := Figure(it)
	for _, want := range []string{
		`class="shot orig"`,
		`data-full="../images/book/a.png"`,
		`class="shot conv"`,
		`data-full="../figures/a.png"`,
		"已完成",
		"判定记录",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("卡片缺少 %q：%s", want, html)
		}
	}
}

// 没画出成品的图不伪造"成品"那一栏。
func TestFigureOmitsMissingConverted(t *testing.T) {
	html := Figure(Item{Name: "b.png", Image: "images/book/b.png", Status: StatusLabel(StatusFallback)})
	if strings.Contains(html, "shot conv") {
		t.Fatalf("没有成品却渲染了成品栏：%s", html)
	}
	if !strings.Contains(html, "退回原图") {
		t.Fatalf("应显示退回原图：%s", html)
	}
}

// 状态词表只有一处口径：页面与进度行不能各说各话。
func TestStatusVocabulary(t *testing.T) {
	cases := []struct {
		status  string
		settled bool
		quality string
		label   string
	}{
		{StatusDone, true, "ok", "已完成"},
		{StatusFallback, true, "degraded", "退回原图"},
		{StatusError, false, "pending", "失败"},
		{StatusClassified, false, "pending", "已分类待处理"},
		{"", false, "pending", "未处理"},
	}
	for _, c := range cases {
		if got := StatusSettled(c.status); got != c.settled {
			t.Errorf("%q settled = %v, want %v", c.status, got, c.settled)
		}
		if got := StatusQuality(c.status); got != c.quality {
			t.Errorf("%q quality = %q, want %q", c.status, got, c.quality)
		}
		if got := StatusLabel(c.status); got != c.label {
			t.Errorf("%q label = %q, want %q", c.status, got, c.label)
		}
	}
}

// 命名口径必须与 progress_items 的落盘一致，否则判定记录链接会指空。
func TestSanitizeNameMatchesProgressLayout(t *testing.T) {
	if got, want := SanitizeName("images/book/a.jpg"), "images_book_a.jpg"; got != want {
		t.Fatalf("SanitizeName = %q, want %q", got, want)
	}
	if got, want := ThumbName("0123456789abcdef"), "thumbs/0123456789ab.jpg"; got != want {
		t.Fatalf("ThumbName = %q, want %q", got, want)
	}
	if got := ThumbName("short"); got != "" {
		t.Fatalf("短摘要不该生成缩略图名，got %q", got)
	}
}
