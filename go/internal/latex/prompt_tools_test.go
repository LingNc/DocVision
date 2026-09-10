package latex

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"mineru-tools/internal/prompts"
)

// 提示词里锁定的工具名必须在代码里真实存在：模板声明的 MustMention
// 逐个到 Name() 方法里核对（工具改名/删除而提示词没跟上时立刻失败）。
func TestPromptToolNamesExistInCode(t *testing.T) {
	live := liveToolNames(t)
	for _, tpl := range prompts.Templates() {
		for _, name := range tpl.MustMention {
			if _, ok := live[name]; !ok {
				t.Errorf("模板 %s 引用了不存在的工具 %q（代码里的工具：%v）",
					tpl.Name, name, sortedKeys(live))
			}
		}
	}
}

// 反向检查：代码里的每个工具名都必须在**某个**模板里出现过，否则说明
// 新工具加进来了却没有告诉模型怎么用（只在本包范围内核对 latex 工具）。
func TestLiveLatexToolsAreMentionedSomewhere(t *testing.T) {
	all := map[string]bool{}
	for _, tpl := range prompts.Templates() {
		for _, n := range tpl.MustMention {
			all[n] = true
		}
	}
	for name, where := range liveToolNames(t) {
		if filepath.Dir(where) != "." { // 只核对本包（internal/latex）的工具
			continue
		}
		if !all[name] {
			t.Errorf("工具 %q（%s）在所有提示词模板里都没有被提到", name, where)
		}
	}
}

// liveToolNames 扫描包内源码，收集 `func (x *XxxTool) Name() string { return "..." }`
// 里的工具名；同时扫描 img2text 包（img2text 的会话提示词也要核对）。
func liveToolNames(t *testing.T) map[string]string {
	t.Helper()
	re := regexp.MustCompile(`func \(\w+ \*(\w+Tool)\) Name\(\) string \{ return ("[^"]*"|` + "`[^`]*`" + `)`)
	out := map[string]string{}
	dirs := []string{".", filepath.Join("..", "img2text")}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("读取 %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".go" || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
				name := strings.Trim(m[2], "\"`")
				if name != "" {
					out[name] = filepath.Join(dir, e.Name())
				}
			}
		}
	}
	if len(out) < 10 {
		t.Fatalf("只找到 %d 个工具名，扫描逻辑可能失效", len(out))
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
