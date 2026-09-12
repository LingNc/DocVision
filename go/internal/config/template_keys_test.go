package config

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// templateKeyPaths 返回一份 YAML 文档里出现的全部键路径（`a.b.c`；列表记成
// `a.b[]`），另外单独给出 models: 下**所有条目键的并集**（`model`、`thinking.type`
// …）。值不参与比较 —— 两份模板允许取不同的值（raster_dpi 那种先例），但同一节
// 的键集合必须一致：否则"照抄 config.example.yaml 的键"与"docvision init 生成的
// 模板"会走出两条不同的配置面，用户改了一份不知道另一份有新东西。
//
// models: 的内容按"条目键的并集"比较，因为示例可以多写几条名字不同的条目
// （gateway-a / gateway-b / heavy 这种命名基座）来演示 extends，条目名本身不是
// 一份"键"。每个条目内部的键集合则由 models.<任一条目>.<键> 逐一覆盖到。
func templateKeyPaths(t *testing.T, path string) (map[string]bool, map[string]bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out := map[string]bool{}
	collectKeys(documentRoot(&doc), "", out)

	entries := map[string]bool{}
	models := mapNodeValue(documentRoot(&doc), modelsKey)
	if models == nil || models.Kind != yaml.MappingNode {
		return out, entries
	}
	for i := 0; i+1 < len(models.Content); i += 2 {
		entry := models.Content[i+1]
		if entry.Kind != yaml.MappingNode {
			continue
		}
		sub := map[string]bool{}
		collectKeys(entry, "", sub)
		for k := range sub {
			entries[k] = true
		}
	}
	return out, entries
}

func collectKeys(n *yaml.Node, prefix string, out map[string]bool) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) > 0 {
			collectKeys(n.Content[0], prefix, out)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			p := key
			if prefix != "" {
				p = prefix + "." + key
			}
			out[p] = true
			collectKeys(n.Content[i+1], p, out)
		}
	case yaml.SequenceNode:
		out[prefix+"[]"] = true
	}
}

// TestConfigTemplateKeyParity 钉住两份模板的键面一致（值可以不同）。
func TestConfigTemplateKeyParity(t *testing.T) {
	example, exampleEntries := templateKeyPaths(t, "../../../config.example.yaml")
	builtin, builtinEntries := templateKeyPaths(t, "../../../go/internal/config/default.yaml")

	// models: 之外的一切按路径逐字比较，但忽略 models.<条目> 那一层（条目名不同）。
	strip := func(m map[string]bool) map[string]bool {
		out := map[string]bool{}
		for k := range m {
			if strings.HasPrefix(k, modelsKey+".") {
				continue
			}
			out[k] = true
		}
		return out
	}
	if diff := keyDiff(strip(example), strip(builtin)); len(diff) != 0 {
		t.Fatalf("两份模板在 models: 之外的键面不一致：%v", diff)
	}
	if diff := keyDiff(exampleEntries, builtinEntries); len(diff) != 0 {
		t.Fatalf("两份模板的 models 条目键面不一致：%v", diff)
	}
	// 示例必须把 extends 演示出来，否则这个新能力在模板里无处可学。
	for _, want := range []string{extendsKey, "image_tokens.method", "image_tokens.tokens", "image_tokens.px_per_token", "image_tokens.min_tokens", "image_tokens.max_tokens"} {
		if !exampleEntries[want] {
			t.Errorf("config.example.yaml 里没有演示 %s", want)
		}
	}
}

// keyDiff 返回两侧键集合的对称差（排序后）。
func keyDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, "-"+k)
		}
	}
	for k := range b {
		if !a[k] {
			out = append(out, "+"+k)
		}
	}
	sort.Strings(out)
	return out
}

// TestConfigTemplateVersionIsCurrent 钉住两份模板的 config_version 与代码期望值
// 一致：版本号是用户发现"模板过期"的唯一信号。
func TestConfigTemplateVersionIsCurrent(t *testing.T) {
	for _, p := range []string{"../../../config.example.yaml", "../../../go/internal/config/default.yaml"} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			ConfigVersion int `yaml:"config_version"`
		}
		if err := yaml.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if got.ConfigVersion != CurrentConfigVersion {
			t.Errorf("%s: config_version = %d，期望 %d", p, got.ConfigVersion, CurrentConfigVersion)
		}
	}
}

// TestTemplatesLoadAndResolve 钉住两份模板**都能加载**，且关键继承链真的成立：
// models.<角色> 写 extends 后，基座的字段要能原样解析出来。
func TestTemplatesLoadAndResolve(t *testing.T) {
	for _, p := range []string{"../../../config.example.yaml", "../../../go/internal/config/default.yaml"} {
		cfg, err := LoadConfig(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		for _, role := range []string{"classifier", "drawing", "style", "chapter", "convert", "checker", "verifier"} {
			mc, ok := cfg.ResolveModel(role)
			if !ok {
				t.Fatalf("%s: models.%s 应当能解析", p, role)
			}
			if mc.BaseURL == "" || mc.APIKey == "" || mc.Model == "" {
				t.Errorf("%s: models.%s 没解析出完整的 base_url/api_key/model: %+v", p, role, mc)
			}
		}
		// estimate 的三种方法都能被读进来（示例里把 3 个取值都提到了）。
		if got := cfg.Estimate.Method; got != EstimateMethodPixels {
			t.Errorf("%s: estimate.method = %q", p, got)
		}
	}
}

// TestTemplatesAreStrictlyValid 钉住两份模板本身能过严格校验（只有占位符凭据
// 会被告知，那是模板的本意）。
func TestTemplatesAreStrictlyValid(t *testing.T) {
	for _, p := range []string{"../../../config.example.yaml", "../../../go/internal/config/default.yaml"} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, prob := range ValidateData(data) {
			if contains(prob, "占位符") || contains(prob, "不能为空") {
				continue
			}
			t.Errorf("%s: %s", p, prob)
		}
	}
}

// TestTemplatesResolveImageEstimatePerRole 钉住示例里那两条 image_tokens 覆盖真
// 的按"逐键"生效（其余键仍继承全局）。
func TestTemplatesResolveImageEstimatePerRole(t *testing.T) {
	example := "../../../config.example.yaml"
	cfg, err := LoadConfig(example)
	if err != nil {
		t.Fatal(err)
	}
	drawing := cfg.ResolveImageEstimate("drawing")
	if drawing.Method != EstimateMethodFixed || drawing.Tokens != 1050 {
		t.Errorf("drawing 应当按 fixed 1050 计: %+v", drawing)
	}
	if drawing.PxPerToken != cfg.Estimate.PxPerToken || drawing.MinTokens != cfg.Estimate.MinTokens {
		t.Errorf("drawing 没写的键应当继承全局: %+v vs %+v", drawing, cfg.Estimate)
	}
	style := cfg.ResolveImageEstimate("style")
	if style.MaxTokens != 8192 || style.Method != cfg.Estimate.Method {
		t.Errorf("style 应当只覆盖 max_tokens: %+v", style)
	}
	verifier := cfg.ResolveImageEstimate("verifier")
	if verifier.PxPerToken != 780 {
		t.Errorf("verifier 应当覆盖 px_per_token: %+v", verifier)
	}
	// 没有 image_tokens 的条目完全跟全局走。
	if got := cfg.ResolveImageEstimate("chapter"); !reflect.DeepEqual(got, cfg.Estimate) {
		t.Errorf("chapter 应当完全跟全局: %+v vs %+v", got, cfg.Estimate)
	}
}
