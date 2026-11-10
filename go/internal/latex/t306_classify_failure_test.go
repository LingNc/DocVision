package latex

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// T306：分类失败绝不能写成 raster。以前两次请求都失败时会把这张图记成
// class=raster / label=unclassified 并继续往下走，等于把"没看懂 / 没连上"
// 永久固化成"这张图本来就是位图"——之后既不会重试也不会重画，跟真的位图
// 分不开；夹在中间的一张失败会让整本书悄悄少一张矢量图。
func TestClassifyFailureIsNotRaster(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(filepath.Join(dir, "run.log"), filepath.Join(dir, "error.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	// ImagesDir 指向空目录：图片解析必然失败，走"拿不到结论"这条路径。
	r := NewRunner(&config.Config{}, log)
	r.cfg.Paths.ImagesDir = filepath.Join(dir, "images")
	tasks := []*task{{mdName: "book.md", imgPath: "a.jpg"}}
	prog := map[string]*imageProgress{}

	var got error
	captureStdout(t, func() { got = r.classifyPhase(tasks, nil, prog, dir, true) })
	if got == nil {
		t.Fatal("分类没做完必须报错让阶段停下，实际返回 nil")
	}
	if !strings.Contains(got.Error(), "分类没有全部完成") {
		t.Fatalf("错误信息没说明分类没做完：%v", got)
	}
	p := prog[tasks[0].key()]
	if p == nil {
		t.Fatal("失败也要留一条记录（下次据此重试）")
	}
	if p.Class != "" {
		t.Fatalf("失败不能给出类别（尤其不能是 raster），实际 class=%q", p.Class)
	}
	if p.Status != ImageStatusError || p.Error == "" {
		t.Fatalf("失败要记成 error 并写明原因，实际 status=%q error=%q", p.Status, p.Error)
	}

	// 下次重跑：这条"没类别"的记录必须重新排队，不能当成已分类跳过。
	if !classifyNeeded(p) {
		t.Fatal("上一轮失败的图必须重新分类")
	}
}

// T306：已有类别的条目重跑分类只会白花请求（还可能把图归到别的类去）。
func TestClassifyNeeded(t *testing.T) {
	cases := []struct {
		name string
		p    *imageProgress
		want bool
	}{
		{"没有记录", nil, true},
		{"只有 error 没有类别", &imageProgress{Status: ImageStatusError, Error: "x"}, true},
		{"已分类", &imageProgress{Class: ClassVector, Status: "classified"}, false},
		{"画完了", &imageProgress{Class: ClassRaster, Status: "done"}, false},
		{"process 失败但类别在", &imageProgress{Class: ClassVector, Status: ImageStatusError, Error: "会话错误"}, false},
		{"类别是空白", &imageProgress{Class: "  ", Status: "classified"}, true},
	}
	for _, c := range cases {
		if got := classifyNeeded(c.p); got != c.want {
			t.Fatalf("%s: classifyNeeded=%v，want %v", c.name, got, c.want)
		}
	}
}

// T306：收尾错误要点名图、说清"不是这些图是位图"、并给出下一步。
func TestClassifyAbortError(t *testing.T) {
	fails := make([]*imageProgress, 0, 7)
	for i := 0; i < 7; i++ {
		fails = append(fails, classifyFailure(&task{mdName: "book.md", imgPath: "img" + string(rune('a'+i)) + ".jpg"}, "分类请求失败", errors.New("HTTP 400")))
	}
	msg := classifyAbortError(fails).Error()
	for _, want := range []string{"还有 7 张", "imga.jpg", " 等", "不是\"这些图是位图\""} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误信息缺 %q：%s", want, msg)
		}
	}
}

// T306：失败原因要说清楚——是被截断、是空的、还是模型说了别的话（并附上原话）。
func TestClassifyReplyError(t *testing.T) {
	base := errors.New("回复中未找到 JSON 对象")

	got := classifyReplyError("length", `{"confidence":0.9,"lab`, base).Error()
	if !strings.Contains(got, "被截断") || !strings.Contains(got, "max_tokens=2048") {
		t.Fatalf("截断要说清预算：%s", got)
	}

	got = classifyReplyError("stop", "   ", base).Error()
	if !strings.Contains(got, "回复为空") {
		t.Fatalf("空回复要单独说：%s", got)
	}

	long := strings.Repeat("这是一段普通回复。", 40)
	got = classifyReplyError("stop", long, base).Error()
	if !strings.Contains(got, "模型实际回复") || !strings.Contains(got, "…") {
		t.Fatalf("非空回复要附原话并截断：%s", got)
	}
	if n := len([]rune(got)); n > 320 {
		t.Fatalf("错误信息过长（%d 字）：%s", n, got)
	}
}
