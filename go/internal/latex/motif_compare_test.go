package latex

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sizedPNG writes a w×h solid-colour PNG and returns its path.
func sizedPNG(t *testing.T, path string, w, h int, c color.RGBA) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// 并排比较是 T126 的第二个修法：一张图一张图看，模型记不住上一张长什么样，
// "同一个标记换了个比例/裁切"看起来就像两个不同的图。比较图只负责把差异摆
// 在一起，回执里同时给出像素尺寸与长宽比——裁切和比例差异用数字说清楚。
func TestMotifCompareImagesRendersPanes(t *testing.T) {
	view := t.TempDir()
	sizedPNG(t, filepath.Join(view, "originals", "000001.png"), 40, 30, color.RGBA{200, 0, 0, 255})
	sizedPNG(t, filepath.Join(view, "originals", "000002.png"), 60, 20, color.RGBA{0, 0, 200, 255})
	sizedPNG(t, filepath.Join(view, "keys", "000001.png"), 25, 50, color.RGBA{0, 150, 0, 255})
	tool := &motifCompareTool{root: view, files: []string{
		"originals/000001.png", "originals/000002.png", "keys/000001.png",
	}}
	res, err := tool.Execute(`{"ids":[1,3,2]}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.ImageBase64 == "" || res.ImageMIME != "image/jpeg" {
		t.Fatalf("comparison must attach one picture: %+v", res)
	}
	raw, err := base64.StdEncoding.DecodeString(res.ImageBase64)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	wantW := motifCompareGap + 2*(motifCompareBoxW+motifCompareGap)
	wantH := motifCompareGap + 2*(motifCompareBoxH+motifCompareLabelH+motifCompareGap)
	if cfg.Width != wantW || cfg.Height != wantH {
		t.Fatalf("3 panes must form a 2x2 grid: got %dx%d want %dx%d", cfg.Width, cfg.Height, wantW, wantH)
	}
	for _, want := range []string{
		"Compared 3 images", "1: 40×30px aspect 1.33", "3: 25×50px aspect 0.50", "2: 60×20px aspect 3.00",
		"not evidence",
	} {
		if !strings.Contains(res.Text, want) {
			t.Fatalf("receipt missing %q: %s", want, res.Text)
		}
	}
}

func TestMotifCompareImagesRejectsBadIDs(t *testing.T) {
	view := t.TempDir()
	sizedPNG(t, filepath.Join(view, "originals", "000001.png"), 8, 8, color.RGBA{0, 0, 0, 255})
	tool := &motifCompareTool{root: view, files: []string{"originals/000001.png", "originals/000001.png"}}
	for _, tc := range []struct{ args, want string }{
		{`{"ids":[1]}`, "2-4"},
		{`{"ids":[1,2,1]}`, "重复"},
		{`{"ids":[1,3]}`, "超出本会话 1..2"},
		{`{"ids":[1,2.5]}`, "必须是整数"},
		{`{"ids":[]}`, "2-4"},
	} {
		res, err := tool.Execute(tc.args)
		if err != nil {
			t.Fatalf("%s: %v", tc.args, err)
		}
		if res.ImageBase64 != "" || !strings.Contains(res.Text, tc.want) {
			t.Fatalf("%s: want rejection %q, got %+v", tc.args, tc.want, res)
		}
	}
	res, err := tool.Execute(`{"ids":[1,2,3,4,5]}`)
	if err != nil || !strings.Contains(res.Text, "2-4") {
		t.Fatalf("five panes must be refused (that is a contact sheet): %+v %v", res, err)
	}
}

// 比较图不是证据：它不写 seen，所以"比过两张"绝不能换来一次合并许可。
func TestMotifCompareImagesIsNotEvidence(t *testing.T) {
	view := t.TempDir()
	sizedPNG(t, filepath.Join(view, "originals", "000001.png"), 12, 10, color.RGBA{10, 10, 10, 255})
	sizedPNG(t, filepath.Join(view, "originals", "000002.png"), 12, 10, color.RGBA{20, 20, 20, 255})
	groups := NewMotifGroupsTool(filepath.Join(view, "groups.json"), 2)
	if res, err := groups.Execute(`{"op":"add","group":[1,2],"key":1}`); err != nil || strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("stage group: %+v %v", res, err)
	}
	seen := map[int]bool{}
	compare := &motifCompareTool{root: view, files: []string{"originals/000001.png", "originals/000002.png"}}
	if res, err := compare.Execute(`{"ids":[1,2]}`); err != nil || res.ImageBase64 == "" {
		t.Fatalf("compare: %+v %v", res, err)
	}
	if len(seen) != 0 {
		t.Fatalf("compare_images must not record evidence: %v", seen)
	}
	submit := &motifSubmitTool{groups: groups, seen: seen}
	res, err := submit.Execute(`{}`)
	if err != nil || !strings.Contains(res.Text, "REJECTED") || submit.done.Submitted {
		t.Fatalf("a comparison must not satisfy the per-image gate: %+v %v", res, err)
	}
}

// submit 交回 0 组是合法决定，但它的后果必须写在回执里：T126 的会话清空了
// 90 张的预填桶、submit 0 组，回执只有一个 "SUBMITTED."，日志也只说"没有
// 通过校验的分组"，于是整本书的图被逐张重画而没人发现。
func TestMotifSubmitZeroGroupsStatesConsequence(t *testing.T) {
	root := t.TempDir()
	groups := NewMotifGroupsTool(filepath.Join(root, "groups.json"), 90)
	if res, err := groups.Execute(`{"op":"add","group":[1,2,3],"key":1}`); err != nil || strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("stage group: %+v %v", res, err)
	}
	cleared, err := groups.Execute(`{"op":"clear"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cleared.Text, "1 组 3 张") || !strings.Contains(cleared.Text, "remove {n,ids:[...]}") {
		t.Fatalf("clear receipt must report what it dropped and the surgical alternative: %s", cleared.Text)
	}
	tool := &motifSubmitTool{groups: groups, seen: map[int]bool{1: true, 2: true, 3: true}, prefill: 1}
	res, err := tool.Execute(`{}`)
	if err != nil || !res.Terminal || !tool.done.Submitted {
		t.Fatalf("zero-group submit must still be accepted: %+v %v", res, err)
	}
	for _, want := range []string{"0 组", "90 张图全部按独立处理", "预填的 1 个相似桶"} {
		if !strings.Contains(res.Text, want) {
			t.Fatalf("zero-group receipt missing %q: %s", want, res.Text)
		}
	}
}

func TestMotifGroupsClearReportsDropped(t *testing.T) {
	root := t.TempDir()
	groups := NewMotifGroupsTool(filepath.Join(root, "groups.json"), 9)
	for _, args := range []string{`{"op":"add","groups":[[1,2,3],[4,5]]}`} {
		if res, err := groups.Execute(args); err != nil || strings.Contains(res.Text, "REJECTED") {
			t.Fatalf("stage groups: %+v %v", res, err)
		}
	}
	res, err := groups.Execute(`{"op":"clear"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "2 组 5 张") {
		t.Fatalf("clear must report the dropped counts: %s", res.Text)
	}
	res, err = groups.Execute(`{"op":"clear"}`)
	if err != nil || !strings.Contains(res.Text, "本来就是空的") {
		t.Fatalf("clearing an empty staging area must say so: %+v %v", res, err)
	}
}
