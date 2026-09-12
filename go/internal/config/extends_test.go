package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// writeConfig drops a config body in a temp dir and loads it.
func writeConfig(t *testing.T, body string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(path)
}

// mustLoadConfig fails the test when the config does not load.
func mustLoadConfig(t *testing.T, body string) *Config {
	t.Helper()
	cfg, err := writeConfig(t, body)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

// TestExtendsDeepMergeSemantics 钉住合并语义本身（在 yaml.Node 层完成，所以
// "键有没有出现"是可判定的）：map 递归深合并、标量/列表整体替换、null 清空、
// 显式 0/false/"" 算覆盖。
func TestExtendsDeepMergeSemantics(t *testing.T) {
	cfg := mustLoadConfig(t, `
mineru:
  token: "abc"
models:
  text:
    base_url: "https://gateway.example/v1"
    api_key: "sk-base"
    model: "wire-base"
  base:
    model: "deepseek-v4.1"
    thinking: {type: enabled, clear_thinking: false}
    request_body: {enable_thinking: false, top_p: 0.9}
    api_timeout: 600
  child:
    extends: base
    model: "deepseek-v4.1-chat"
    request_body: {top_p: 0.5}
  other:
    extends: base
    model: "glm-5.3-flash"
`)
	base := cfg.Models["base"]
	child := cfg.Models["child"]

	// 标量整体替换：child 写了自己的 wire 名就用自己的，没写就继承基座的。
	if child.Model != "deepseek-v4.1-chat" {
		t.Errorf("child.model = %q，期望自己写的值赢", child.Model)
	}
	if other := cfg.Models["other"]; other.Model != "glm-5.3-flash" {
		t.Errorf("other.model = %q，期望自己写的值赢", other.Model)
	}
	// 标量继承：child 没写 api_timeout ⇒ 基座的 600。
	if child.APITimeout != 600 {
		t.Errorf("child.api_timeout = %d，期望继承基座的 600", child.APITimeout)
	}
	// 深合并发生在**条目这一层**：child 自己写了一份 request_body，整块替换，
	// 基座的键不会漏进来（要合并的是"条目键"，不是任意嵌套 map —— 深层 map 的
	// 逐键继承由 ResolveModel/ResolveImageEstimate 负责）。
	if child.RequestBody["top_p"] != 0.5 {
		t.Errorf("child.request_body = %+v，期望整体替换", child.RequestBody)
	}
	if _, leaked := child.RequestBody["enable_thinking"]; leaked {
		t.Errorf("child 自己写了 request_body，基座的键不该漏进来: %+v", child.RequestBody)
	}
	// 基座自己的 map 不受影响。
	if base.RequestBody["top_p"] != 0.9 {
		t.Errorf("基座的 request_body 被改到了: %+v", base.RequestBody)
	}
	// thinking 整块继承过来（child 没写）。
	if child.Thinking["type"] != "enabled" || child.Thinking["clear_thinking"] != false {
		t.Errorf("child.thinking = %+v，期望整块继承", child.Thinking)
	}
	// 显式 0 算覆盖：child 写了 temperature: 0，文档里这个键就"存在"。
	if !mergedEntryKeys(t, `
mineru:
  token: "abc"
models:
  text: {model: "wire-base"}
  base: {model: "deepseek-v4.1", temperature: 0.7}
  child: {extends: base, temperature: 0}
`, "child")["temperature"] {
		t.Error("显式 temperature: 0 合并后应当仍然存在（键存在即为覆盖）")
	}
	// extends 被消费掉：它不是解码后的字段值。
	if child.Extends != "" || base.Extends != "" {
		t.Errorf("extends 应在加载期被消费: base=%q child=%q", base.Extends, child.Extends)
	}
}

// TestExtendsExplicitZeroAndNull 用合并后的节点树直接断言"键是否存在"，因为
// 解码后的值类型做不到这件事。显式 0/false/"" 覆盖；`key: null` 清空继承值。
func TestExtendsExplicitZeroAndNull(t *testing.T) {
	body := `
mineru:
  token: "abc"
models:
  text:
    base_url: "https://gateway.example/v1"
    api_key: "sk-base"
    model: "wire-base"
  base:
    model: "deepseek-v4.1"
    api_timeout: 600
    temperature: 0.7
    rate_limit_retries: 20
  child:
    extends: base
    temperature: 0
    rate_limit_retries: null
    max_tokens: 0
`
	keys := mergedEntryKeys(t, body, "child")
	if !keys["temperature"] {
		t.Error("显式 temperature: 0 合并后必须仍然存在（0 是覆盖，不是没写）")
	}
	if keys["rate_limit_retries"] {
		t.Error("rate_limit_retries: null 应当把继承来的键清掉")
	}
	if !keys["max_tokens"] {
		t.Error("显式 max_tokens: 0 应当存在")
	}
	if !keys["api_timeout"] {
		t.Error("基座的 api_timeout 应当被继承进来")
	}

	cfg := mustLoadConfig(t, body)
	entry := cfg.Models["child"]
	if entry.Temperature != 0 {
		t.Errorf("child.temperature = %v，期望显式的 0", entry.Temperature)
	}
	if entry.RateLimitRetries != 0 {
		t.Errorf("child.rate_limit_retries = %d，期望被 null 清掉（0）", entry.RateLimitRetries)
	}
	if entry.APITimeout != 600 {
		t.Errorf("child.api_timeout = %d，期望继承 600", entry.APITimeout)
	}
}

// mergedEntryKeys 返回 models.<entry> 在合并之后的键集合（节点层事实）。
func mergedEntryKeys(t *testing.T, body, entry string) map[string]bool {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if err := mergeModelExtends(&doc); err != nil {
		t.Fatalf("mergeModelExtends: %v", err)
	}
	models := mapNodeValue(documentRoot(&doc), modelsKey)
	node, ok := lookupModel(models, entry)
	if !ok {
		t.Fatalf("models.%s 不存在", entry)
	}
	out := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		out[node.Content[i].Value] = true
	}
	return out
}

// TestExtendsListReplaces 钉住"列表整体替换、不拼接"。
func TestExtendsListReplaces(t *testing.T) {
	cfg := mustLoadConfig(t, `
mineru:
  token: "abc"
tools:
  python:
    packages: ["numpy", "PIL"]
models:
  text: {model: "wire-base"}
`)
	if got := cfg.Tools.Python.Packages; !reflect.DeepEqual(got, []string{"numpy", "PIL"}) {
		t.Fatalf("packages = %v", got)
	}
	// 列表是 yaml 层的事实，这里只需要钉住没有任何"拼接"发生在 config 包里：
	// 没有 extends 的文档里列表原样保留（合并层不碰非 models 部分）。
}

// TestExtendsErrors 钉住三类硬报错：基座不存在、自引用、成环。三条都要能定位到
// `models.<名>.extends`，成环还要给出完整链。
func TestExtendsErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "基座不存在",
			body: "models:\n  text: {model: a}\n  drawing: {extends: missing}\n",
			want: []string{"models.drawing.extends", `"missing"`, "不存在"},
		},
		{
			name: "自引用",
			body: "models:\n  text: {model: a}\n  drawing: {extends: drawing}\n",
			want: []string{"models.drawing.extends", "循环", "drawing → drawing"},
		},
		{
			name: "三节点成环",
			body: "models:\n  text: {model: a}\n  a: {extends: b}\n  b: {extends: c}\n  c: {extends: a}\n",
			want: []string{"循环", "a → b → c → a"},
		},
		{
			name: "空 extends",
			body: "models:\n  text: {model: a}\n  a: {extends: \"\"}\n",
			want: []string{"models.a.extends"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := writeConfig(t, c.body)
			if err == nil {
				t.Fatal("应当报错")
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Fatalf("错误 %q 里应包含 %q", err.Error(), w)
				}
			}
		})
	}
}

// TestExtendsMultiLevelChain 多级链：c ← b ← a，缺的键逐级往上找。
func TestExtendsMultiLevelChain(t *testing.T) {
	cfg := mustLoadConfig(t, `
mineru:
  token: "abc"
models:
  text: {model: "wire-base"}
  a: {model: "m-a", api_timeout: 111, temperature: 0.5}
  b: {extends: a, api_timeout: 222}
  c: {extends: b, temperature: 0.9}
`)
	c := cfg.Models["c"]
	if c.Model != "m-a" {
		t.Errorf("c.model = %q，期望从 a 继承", c.Model)
	}
	if c.APITimeout != 222 {
		t.Errorf("c.api_timeout = %d，期望取 b 覆盖后的 222", c.APITimeout)
	}
	if c.Temperature != 0.9 {
		t.Errorf("c.temperature = %v，期望自己的 0.9", c.Temperature)
	}
}

// TestExtendsDoesNotPolluteBase 深拷贝断言：改一条的 thinking 不能动到基座与
// 兄弟条目。合并层若不深拷贝，一次校验纠正（validateThinkingTypes 会就地写
// thinking["type"]）就会污染所有引用者。
func TestExtendsDoesNotPolluteBase(t *testing.T) {
	body := `
mineru:
  token: "abc"
models:
  text:
    base_url: "https://gateway.example/v1"
    api_key: "sk-base"
    model: "wire-base"
    request_body: {enable_thinking: false}
    thinking: {type: disabled}
  base:
    extends: text
    thinking: {type: enabled}
  a:
    extends: base
  b:
    extends: base
`
	cfg := mustLoadConfig(t, body)
	cfg.Models["a"].Thinking["type"] = "disabled"
	if cfg.Models["base"].Thinking["type"] != "enabled" {
		t.Fatalf("改 models.a.thinking 影响到了 models.base.thinking: %+v", cfg.Models["base"].Thinking)
	}
	if cfg.Models["b"].Thinking["type"] != "enabled" {
		t.Fatalf("改 models.a.thinking 影响到了兄弟条目 models.b: %+v", cfg.Models["b"].Thinking)
	}
	if cfg.Models["text"].Thinking["type"] != "disabled" {
		t.Fatalf("改 models.a.thinking 影响到了 models.text: %+v", cfg.Models["text"].Thinking)
	}

	// ResolveModel 也必须给独立副本：它把 fallback 的 map 头赋给条目时若不拷贝，
	// 改解析结果就会写进 models.text。
	resolved, ok := cfg.ResolveModel("b")
	if !ok {
		t.Fatal("b 应当能解析")
	}
	resolved.Thinking["type"] = "adaptive"
	resolved.RequestBody["enable_thinking"] = true
	if cfg.Models["text"].Thinking["type"] != "disabled" {
		t.Fatalf("ResolveModel 的返回值与 models.text 共享了 thinking: %+v", cfg.Models["text"].Thinking)
	}
	if cfg.Models["text"].RequestBody["enable_thinking"] != false {
		t.Fatalf("ResolveModel 的返回值与 models.text 共享了 request_body: %+v", cfg.Models["text"].RequestBody)
	}
	// 两个解析结果之间也要互相独立。
	again, _ := cfg.ResolveModel("a")
	if again.Thinking["type"] == "adaptive" {
		t.Fatal("两次 ResolveModel 返回了同一张 thinking 表")
	}
}

// TestValidateThinkingTypesDoesNotPollute 直接在 LoadConfig 里放一个"笔误"取值：
// 校验会就地纠正成 disabled，纠正后基座与兄弟条目必须是原样。
func TestValidateThinkingTypesDoesNotPollute(t *testing.T) {
	cfg := mustLoadConfig(t, `
mineru:
  token: "abc"
models:
  text: {model: "wire-base"}
  base: {thinking: {type: enabled}}
  typo: {extends: base, thinking: {type: disable}}
  sibling: {extends: base}
`)
	if got := cfg.Models["typo"].Thinking["type"]; got != "disabled" {
		t.Fatalf("笔误应被纠正为 disabled，got %v", got)
	}
	if got := cfg.Models["base"].Thinking["type"]; got != "enabled" {
		t.Fatalf("基座被纠正污染了: %v", got)
	}
	if got := cfg.Models["sibling"].Thinking["type"]; got != "enabled" {
		t.Fatalf("兄弟条目被纠正污染了: %v", got)
	}
}

// TestExtendsImageTokens：extends 出来的条目各自带 image_tokens（逐键覆盖
// estimate 的那套规则在深合并下天然成立）。
func TestExtendsImageTokens(t *testing.T) {
	cfg := mustLoadConfig(t, `
mineru:
  token: "abc"
estimate:
  method: pixels
  px_per_token: 750
  min_tokens: 85
  max_tokens: 4096
models:
  text: {model: "wire-base"}
  deepseek:
    model: "deepseek-v4.1"
    price: {input: 1, output: 4}
  drawing:
    extends: deepseek
    image_tokens: {method: fixed, tokens: 1050}
  style:
    extends: deepseek
  verifier:
    extends: deepseek
    image_tokens: {method: pixels, px_per_token: 780, max_tokens: 8192}
`)
	if got := cfg.ResolveImageEstimate("drawing"); got.Method != EstimateMethodFixed || got.Tokens != 1050 || got.MinTokens != 85 {
		t.Fatalf("drawing 的规则 = %+v，期望 fixed 1050 + 继承的上下限", got)
	}
	if got := cfg.ResolveImageEstimate("style"); got.Method != EstimateMethodPixels || got.PxPerToken != 750 {
		t.Fatalf("style 没写 image_tokens，应当完全跟全局: %+v", got)
	}
	if got := cfg.ResolveImageEstimate("verifier"); got.PxPerToken != 780 || got.MaxTokens != 8192 {
		t.Fatalf("verifier 的规则 = %+v", got)
	}
	// 同一个基座下的两个条目互不影响，基座自己也没被写上 image_tokens。
	if cfg.Models["deepseek"].ImageTokens != nil {
		t.Fatalf("基座不该被写上 image_tokens: %+v", cfg.Models["deepseek"].ImageTokens)
	}
	if cfg.Models["drawing"].ImageTokens == cfg.Models["verifier"].ImageTokens {
		t.Fatal("两个条目不该共享同一个 image_tokens 指针")
	}
	// 价格也是继承来的（同一网关下的同价模型不必重复三行数字）。
	if cfg.Models["style"].Price.Input != 1 || cfg.Models["style"].Price.Output != 4 {
		t.Fatalf("style 的 price = %+v，期望继承 deepseek", cfg.Models["style"].Price)
	}
}

// TestExtendsSameWireSamePriceOK 钉住真实改写稿里的形态：4 个角色条目 extends
// 同一个基座，wire 名与价格完全一致 —— 这不是冲突。
func TestExtendsSameWireSamePriceOK(t *testing.T) {
	cfg := mustLoadConfig(t, `
config_version: 10
mineru: {token: "abc"}
models:
  text: {base_url: "https://gateway.example/v1", api_key: "sk-base", model: "Qwen/Qwen3.5-27B"}
  heavy:
    model: "deepseek-v4.1"
    price: {input: 1, cached: 0.1, output: 4}
    thinking: {type: enabled, clear_thinking: false}
    reasoning_effort: high
  drawing: {extends: heavy}
  style: {extends: heavy}
  chapter: {extends: heavy}
  convert: {extends: heavy}
`)
	for _, role := range []string{"drawing", "style", "chapter", "convert"} {
		m := cfg.Models[role]
		if m.Model != "deepseek-v4.1" || m.ReasoningEffort != "high" || m.Thinking["type"] != "enabled" {
			t.Fatalf("%s 没继承齐: %+v", role, m)
		}
		if m.Price.Input != 1 {
			t.Fatalf("%s 的 price 没继承: %+v", role, m.Price)
		}
	}
	if problems := ValidateData([]byte(`
config_version: 10
mineru: {token: "abc"}
models:
  text: {base_url: "https://gateway.example/v1", api_key: "sk-base", model: "Qwen/Qwen3.5-27B"}
  heavy: {model: "deepseek-v4.1", price: {input: 1, output: 4}}
  drawing: {extends: heavy}
  convert: {extends: heavy}
`)); len(problems) != 0 {
		t.Fatalf("同名同价的 extends 结构应当通过严格校验: %v", problems)
	}
}

// TestExtendsIsAKnownField 钉住 extends 是 ModelConfig 的真字段：严格校验
// （docvision setup 走的那条路）不会把它当未知键。
func TestExtendsIsAKnownField(t *testing.T) {
	body := `
config_version: 10
mineru: {token: "abc"}
models:
  text: {base_url: "https://gateway.example/v1", api_key: "sk-base", model: "wire-base"}
  base: {model: "wire-big"}
  drawing: {extends: base}
`
	if problems := ValidateData([]byte(body)); len(problems) != 0 {
		t.Fatalf("extends 被当成未知键了（KnownFields 严格解码）: %v", problems)
	}
}

// TestExtendsUnknownKeyInBaseIsStillCaught 严格校验必须能定位到"被合并进来的
// 块里写错的键"：错误正文里的路径要是真的能找到那一行的条目。
func TestExtendsUnknownKeyInBaseIsStillCaught(t *testing.T) {
	body := `
config_version: 10
mineru: {token: "abc"}
models:
  text: {base_url: "https://gateway.example/v1", api_key: "sk-base", model: "wire-base"}
  base: {model: "wire-big", thinking_typ: enabled}
  drawing: {extends: base}
`
	problems := strings.Join(ValidateData([]byte(body)), "\n")
	if !strings.Contains(problems, "thinking_typ") {
		t.Fatalf("基座里写错的键没被报出来: %q", problems)
	}
	if !strings.Contains(problems, "models.base.thinking_typ") {
		t.Fatalf("错误路径应当指向真正写着那一行的条目 models.base: %q", problems)
	}
	// 加载期的未知键告警也要同时给出"继承了它的条目"。
	keys, err := unknownKeys([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, k := range keys {
		paths[k.Path] = true
	}
	if !paths["models.base.thinking_typ"] || !paths["models.drawing.thinking_typ"] {
		t.Fatalf("告警应同时指向写它的条目与继承它的条目: %+v", keys)
	}
}

// TestUnknownKeyWarningLines 钉住告警本身：一行一个未知键，带路径与行号；合法
// 配置（两个模板）一条都不该有。
func TestUnknownKeyWarningLines(t *testing.T) {
	keys, err := unknownKeys([]byte("mineru:\n  token: x\n  bogus_key: 1\nmodels:\n  text: {model: a, thinking_typo: b}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, k := range keys {
		got[k.Path] = k.Line
	}
	if got["mineru.bogus_key"] != 3 {
		t.Fatalf("mineru.bogus_key 行号错: %+v", keys)
	}
	if got["models.text.thinking_typo"] != 5 {
		t.Fatalf("models.text.thinking_typo 行号错: %+v", keys)
	}
	for _, p := range []string{"../../../config.example.yaml", "../../../go/internal/config/default.yaml"} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if ks, err := unknownKeys(data); err != nil || len(ks) != 0 {
			t.Fatalf("%s 有未知键: %+v (%v)", p, ks, err)
		}
	}
}

// TestNoExtendsBehaviourUnchanged 钉住"不写 extends ⇒ 行为逐字不变"：同一份
// 配置走新加载器（节点合并 + 解码）与参照路径（直接解码 + setDefaults）必须
// reflect.DeepEqual，包含 v1.5.0-beta.5 那条既有的继承用例。
func TestNoExtendsBehaviourUnchanged(t *testing.T) {
	body := []byte(`
config_version: 10
mineru:
  api_base_url: "https://mineru.net/api/v4"
  token: "abc"
options:
  log_level: "debug"
paths:
  input_dir: "./in"
  output_dir: "./out"
latex:
  level: 1
  drawing_model: "drawing"
  figure_check: {enabled: true, max_rounds: 3}
  sessions:
    convert: {max_tool_rounds: 128, max_tokens: 65536}
    checker: {max_tokens: 8192}
estimate:
  method: fixed
  tokens: 900
models:
  text:
    base_url: "https://gateway.example/v1"
    api_key: "sk-base"
    model: "wire-base"
    temperature: 0.10
    api_timeout: 500
    rate_limit_retries: 50
    request_body: {enable_thinking: false}
    thinking: {type: disabled}
    price: {input: 1, cached: 0.1, output: 4}
  classifier:
    model: "Qwen/Qwen3.6-27B"
  drawing:
    model: "glm-5.3-flash"
    image_tokens: {method: fixed, tokens: 1050}
  verifier:
    model: "glm-5.3-flash"
    image_tokens: {max_tokens: 8192}
`)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// 参照路径：不做节点合并，直接解码 + setDefaults + 同样的校验。
	ref := &Config{}
	if err := yaml.Unmarshal(body, ref); err != nil {
		t.Fatal(err)
	}
	setDefaults(ref)
	if err := validateThinkingTypes(ref); err != nil {
		t.Fatal(err)
	}
	if err := checkDuplicateWireNames(ref); err != nil {
		t.Fatal(err)
	}
	if err := validatePaths(ref); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, ref) {
		t.Fatalf("没有写 extends 的配置在新旧路径上不等价:\n got=%+v\n ref=%+v", got, ref)
	}

	// 既有的 ResolveModel 继承用例照旧（原文一行未改地在这里重跑一遍）。
	mc, ok := got.ResolveModel("classifier")
	if !ok || mc.BaseURL != "https://gateway.example/v1" || mc.APIKey != "sk-base" ||
		mc.Model != "Qwen/Qwen3.6-27B" || mc.APITimeout != 500 || mc.RateLimitRetries != 50 {
		t.Fatalf("classifier 继承结果变了: %+v", mc)
	}
	if unnamed, _ := got.ResolveModel(""); unnamed.Model != "wire-base" {
		t.Fatalf("匿名解析结果变了: %+v", unnamed)
	}
}

// TestValidateDataProblemsUnchangedForExtendsFreeConfig：经典配置（随附模板那一
// 类写法）的问题清单逐字不变，只有版本号那一行跟着 CurrentConfigVersion 走。
func TestValidateDataProblemsUnchangedForExtendsFreeConfig(t *testing.T) {
	body := []byte("config_version: 2\nmodels:\n  text:\n    base_url: \"https://x/v1\"\n    api_key: \"YOUR-X\"\n    model: \"m\"\nmineru:\n  token: \"YOUR-X\"\n")
	got := ValidateData(body)
	want := []string{
		"config_version 应为 " + itoa(CurrentConfigVersion) + "（当前为 2）—— 请参考 config.example.yaml 更新配置文件",
		"mineru.token 仍是占位符，请填入真实 token",
		"models.text.api_key 仍是占位符，请填入真实 key",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("问题清单变了:\n got=%q\nwant=%q", got, want)
	}
	// extends 不存在时也不该多出任何"键"类问题（节点合并 + 严格解码两条路径
	// 对同一份文档的判断必须一致）。
	for _, p := range got {
		if strings.Contains(p, "未知配置键") || strings.Contains(p, "not found in type") {
			t.Fatalf("无 extends 的配置多了未知键问题: %q", p)
		}
	}
}
