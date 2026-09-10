package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 守卫①：模板文件与注册表必须双向一致——磁盘上每个 .md 都有登记
// （防"改了模板忘了接进代码"），每个登记的文件都存在（防打错文件名）。
func TestTemplateRegistryMatchesFiles(t *testing.T) {
	onDisk := map[string]bool{}
	for _, f := range Files() {
		onDisk[f] = true
	}
	registered := map[string]bool{}
	for _, tpl := range Templates() {
		if registered[tpl.File] {
			t.Errorf("注册表里 %s 重复登记", tpl.File)
		}
		registered[tpl.File] = true
		if !onDisk[tpl.File] {
			t.Errorf("注册表登记了不存在的模板 %s", tpl.File)
		}
		if _, err := Get(tpl.Name); err != nil {
			t.Errorf("模板 %s 读取失败: %v", tpl.Name, err)
		}
		if _, err := os.Stat(filepath.Join("templates", tpl.File)); err != nil {
			t.Errorf("模板文件 %s 不存在: %v", tpl.File, err)
		}
	}
	for f := range onDisk {
		if !registered[f] {
			t.Errorf("模板 %s 未被注册表引用（死模板）", f)
		}
	}
}

// 守卫②：声明的占位符必须真的出现在模板里，且"全量渲染后不留残余"——
// 历史事故：{OUTPUT_LANG}/{MAX_ROUNDS} 原样发给了模型。
func TestTemplatesRenderCleanly(t *testing.T) {
	for _, tpl := range Templates() {
		raw := Must(tpl.Name)
		vars := map[string]string{}
		for _, v := range tpl.Vars {
			vars[v] = "<<" + v + ">>"
			if !strings.Contains(raw, "{"+v+"}") {
				t.Errorf("%s 声明了占位符 {%s}，但模板里没有出现", tpl.Name, v)
			}
		}
		got := Render(tpl.Name, vars)
		if left := Placeholders(got); len(left) > 0 {
			t.Errorf("%s 渲染后仍有未填充占位符 %v", tpl.Name, left)
		}
		for _, v := range tpl.Vars {
			if !strings.Contains(got, "<<"+v+">>") {
				t.Errorf("%s 渲染后未见 {%s} 的替换结果", tpl.Name, v)
			}
		}
		// 未声明的占位符不应存在（例如把 {OUTPUT} 写成 {OUTPUTLANG}）
		if left := Placeholders(raw); len(left) > len(tpl.Vars) {
			t.Errorf("%s 含未声明的占位符 %v（声明：%v）", tpl.Name, left, tpl.Vars)
		}
	}
}

// 守卫③：提示词里提到的工具名必须是真实存在的工具，且退役名字不得复活
// （read_md/view_page/list_images/install_font/compile_preview 都曾长期
// 留在提示词里，模型因此调用不存在的工具）。
func TestTemplatesMentionOnlyLiveTools(t *testing.T) {
	for _, tpl := range Templates() {
		text := Must(tpl.Name)
		for _, want := range tpl.MustMention {
			if !strings.Contains(text, want) {
				t.Errorf("%s 未提到工具 %q（该会话需要它）", tpl.Name, want)
			}
		}
		for _, dead := range RetiredNames() {
			if strings.Contains(text, dead) {
				t.Errorf("%s 提到了已删除的 %q", tpl.Name, dead)
			}
		}
	}
}

// 守卫④：退役名字表本身要覆盖历史事故清单（防止顺手删条目）。
func TestRetiredNamesCoverHistory(t *testing.T) {
	want := []string{"read_md", "view_page", "list_images", "install_font", "compile_preview", "preview.png"}
	have := map[string]bool{}
	for _, n := range RetiredNames() {
		have[n] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("退役名字表缺少 %q", w)
		}
	}
}

// Fill 只替换给定键，未知占位符保持原样（守卫②依赖这个语义）。
func TestFillLeavesUnknownPlaceholders(t *testing.T) {
	got := Fill("a {KNOWN} b {UNKNOWN}", map[string]string{"KNOWN": "x"})
	if got != "a x b {UNKNOWN}" {
		t.Fatalf("Fill = %q", got)
	}
	if ph := Placeholders(got); len(ph) != 1 || ph[0] != "UNKNOWN" {
		t.Fatalf("Placeholders = %v", ph)
	}
}
