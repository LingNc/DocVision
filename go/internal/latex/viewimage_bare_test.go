package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 「给文件名就行」这条老路子必须在**项目级会话**（style/convert/fix，Subject =
// images 共享根）里也成立：图片其实在 images/<书>/<file>，只给文件名本来会拼成
// images/<file> 而报错——而提示词恰恰让样式会话"用 list_source_pages 打印的文件
// 名"（那是裸文件名）。唯一匹配就接受，同名多份则报错列出候选，绝不猜。
func TestViewImageBareNameResolvesUnderImagesRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "images", "测试-概率论")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "b6f6f41eff8b4ee7f04e713f8e9db7dd4010f68b9f58beba4923d0f340ed5550.jpg"
	img := filepath.Join(sub, name)
	if err := os.WriteFile(img, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: root, Subject: "images", BareSearch: true}
	// 裸文件名、md 引用路径、subject/file 三种写法都必须指向同一个文件。
	for _, rel := range []string{name, "images/测试-概率论/" + name, "测试-概率论/" + name} {
		got, err := tool.resolve(rel)
		if err != nil {
			t.Fatalf("resolve(%q) 应成功: %v", rel, err)
		}
		if got != img {
			t.Errorf("resolve(%q) = %q, want %q", rel, got, img)
		}
	}
	// 不存在的名字仍然报错，且提示里带真实文件名。
	if _, err := tool.resolve("nope.jpg"); err == nil {
		t.Fatal("不存在的文件名必须报错")
	} else if !strings.Contains(err.Error(), "实际有") {
		t.Fatalf("提示信息 = %q", err.Error())
	}
	// 没有开启 BareSearch 的会话（按书为 Subject）行为不变：只给文件名
	// 解析不到别本书里的同名图。
	strict := &ViewImageTool{Root: root, Subject: "images"}
	if _, err := strict.resolve(name); err == nil {
		t.Fatal("未开启 BareSearch 时不该做跨目录搜索")
	}
}

// 同名文件出现在两本书的目录里：拒绝并列出候选（曾经担心的"悄悄拿错图"）。
func TestViewImageBareNameAmbiguousIsRejected(t *testing.T) {
	root := t.TempDir()
	for _, book := range []string{"书甲", "书乙"} {
		dir := filepath.Join(root, "images", book)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "fig.jpg"), []byte("jpg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tool := &ViewImageTool{Root: root, Subject: "images", BareSearch: true}
	_, err := tool.resolve("fig.jpg")
	if err == nil {
		t.Fatal("同名文件必须报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "不唯一") {
		t.Fatalf("错误信息 = %q", msg)
	}
	for _, want := range []string{"images/书甲/fig.jpg", "images/书乙/fig.jpg"} {
		if !strings.Contains(msg, want) {
			t.Errorf("候选里缺少 %s: %q", want, msg)
		}
	}
	// 给完整路径仍然可以精确取到。
	if _, err := tool.resolve("images/书乙/fig.jpg"); err != nil {
		t.Fatalf("完整路径必须可用: %v", err)
	}
}

// assemble/final-review 会话：Root 是 build 树（没有 Subject），图片在
// images/<书>/ 下，裸文件名同样要能唯一匹配到。
func TestViewImageBareNameWorksInBuildTree(t *testing.T) {
	build := t.TempDir()
	dir := filepath.Join(build, "images", "测试-概率论")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(dir, "fig.png")
	if err := os.WriteFile(img, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: build, BareSearch: true}
	got, err := tool.resolve("fig.png")
	if err != nil {
		t.Fatalf("build 树里裸文件名应可解析: %v", err)
	}
	if got != img {
		t.Fatalf("resolve = %q, want %q", got, img)
	}
	// 排在 build 树根上的文件（figures/ 之类）优先按原路径解析。
	flat := filepath.Join(build, "flat.jpg")
	if err := os.WriteFile(flat, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := tool.resolve("flat.jpg"); err != nil || got != flat {
		t.Fatalf("根目录文件应直接命中: %q %v", got, err)
	}
}
