package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// 运行目录 `/home/share/samba-share/PDF2MD/config.yaml` 的改写稿校验。
//
// temp/ 已被 .gitignore，所以这两份文件在别人的检出里不存在（测试自动跳过）；
// 留成测试是为了让"其余内容逐字保留、只有下面这几处不同"这条承诺**可复核**，
// 而不是靠人眼看 diff。源文件里的密钥在 temp/config_src_redacted.yaml 里已被
// 占位符替换（本文件、测试输出都不含任何真实密钥）。
const (
	rewriteSrc = "../../../temp/config_src_redacted.yaml"
	rewriteNew = "../../../temp/config_new.yaml"
)

// flatten 把一份配置拍成 `路径 → 标量值` 的表，`路径 → 键存在` 靠键集合单独判定。
func flatten(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("没有 %s（temp/ 已 gitignore）: %v", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out := map[string]string{}
	var walk func(n *yaml.Node, p string)
	walk = func(n *yaml.Node, p string) {
		if n == nil {
			return
		}
		switch n.Kind {
		case yaml.DocumentNode:
			if len(n.Content) > 0 {
				walk(n.Content[0], p)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				k := n.Content[i].Value
				q := k
				if p != "" {
					q = p + "." + k
				}
				walk(n.Content[i+1], q)
			}
		case yaml.SequenceNode:
			out[p] = "[]"
		default:
			out[p] = n.Tag + ":" + n.Value
		}
	}
	walk(documentRoot(&doc), "")
	return out
}

// TestRunConfigRewriteKeepsEverythingElse 钉住条款：改写稿只改动下面列出的键，
// 其余键逐字保留（值也逐字相同）。
func TestRunConfigRewriteKeepsEverythingElse(t *testing.T) {
	src, dst := flatten(t, rewriteSrc), flatten(t, rewriteNew)

	// 允许发生变化（或消失/新增）的路径前缀。
	allowed := []string{
		"config_version",
		"estimate.",                                                              // 四个旧键 → method + tokens/px_per_token/min_tokens/max_tokens
		"models.heavy.",                                                          // 新增的命名基座
		"models.drawing.", "models.style.", "models.chapter.", "models.convert.", // 四个角色条目并成一条基座
		"img2text.figure_check.", "latex.figure_check.", // 缩进修正（原稿写在 img2text 下，那里没有这个键）
	}
	allowedPath := func(p string) bool {
		for _, a := range allowed {
			if p == a || strings.HasPrefix(p, a) {
				return true
			}
		}
		return false
	}

	for p, want := range src {
		if allowedPath(p) {
			continue
		}
		got, ok := dst[p]
		if !ok {
			t.Errorf("键 %s 在改写稿里丢了（源值 %s）", p, want)
			continue
		}
		if got != want {
			t.Errorf("键 %s 的值被改了：%s → %s", p, want, got)
		}
	}
	for p := range dst {
		if allowedPath(p) {
			continue
		}
		if _, ok := src[p]; !ok {
			t.Errorf("改写稿多出了键 %s", p)
		}
	}
}

// TestRunConfigRewriteResolvesIdentically 钉住"估算数字不变、角色行为不变"：两份
// 配置里的每个角色解析出的有效模型与图片计量规则完全一致。
func TestRunConfigRewriteResolvesIdentically(t *testing.T) {
	if _, err := os.Stat(rewriteSrc); err != nil {
		t.Skipf("没有 %s: %v", rewriteSrc, err)
	}
	src, err := LoadConfig(rewriteSrc)
	if err != nil {
		t.Fatalf("源配置加载失败: %v", err)
	}
	dst, err := LoadConfig(rewriteNew)
	if err != nil {
		t.Fatalf("改写稿加载失败: %v", err)
	}

	roles := []string{"text", "classifier", "drawing", "style", "chapter", "convert", "checker", "verifier"}
	for _, role := range roles {
		a, okA := src.ResolveModel(role)
		b, okB := dst.ResolveModel(role)
		if okA != okB {
			t.Errorf("%s: 能否解析变了 %v → %v", role, okA, okB)
			continue
		}
		// 逐字段比较（reflect.DeepEqual 会把"哪一条条目提供这个值"也算进去，
		// 而声明式改写的意义正是"值不变、来源变了"）。
		if a.BaseURL != b.BaseURL || a.APIKey != b.APIKey || a.Model != b.Model ||
			a.MaxTokens != b.MaxTokens || a.Temperature != b.Temperature ||
			a.APITimeout != b.APITimeout || a.APIConnectTimeout != b.APIConnectTimeout ||
			a.APIMaxRetries != b.APIMaxRetries || a.RateLimitRetries != b.RateLimitRetries ||
			a.APIStreamIdleTimeout != b.APIStreamIdleTimeout || a.ReasoningEffort != b.ReasoningEffort ||
			a.Streaming() != b.Streaming() || !reflect.DeepEqual(a.RequestBody, b.RequestBody) ||
			!reflect.DeepEqual(a.Thinking, b.Thinking) || a.Price != b.Price {
			t.Errorf("%s 的有效模型配置变了:\n 源=%+v\n 新=%+v", role, a, b)
		}
		if ga, gb := src.ResolveImageEstimate(role), dst.ResolveImageEstimate(role); ga != gb {
			t.Errorf("%s 的图片计量规则变了: %+v → %+v", role, ga, gb)
		}
	}
	// 四个角色原本逐字相同，改写后仍必须完全相同。
	for _, role := range []string{"drawing", "style", "chapter", "convert"} {
		a, _ := dst.ResolveModel(role)
		b, _ := dst.ResolveModel("drawing")
		if a.Model != b.Model || a.Thinking["type"] != b.Thinking["type"] || a.ReasoningEffort != b.ReasoningEffort {
			t.Errorf("%s 与 drawing 不再等价: %+v vs %+v", role, a, b)
		}
	}
	// 价格表也一致（全 0 = 未配置时两边都为空）。
	if len(src.ModelPrices()) != len(dst.ModelPrices()) {
		t.Errorf("价格表条目数变了: %d → %d", len(src.ModelPrices()), len(dst.ModelPrices()))
	}
}

// TestRunConfigRewriteStrictValidation 钉住改写稿能过严格校验：除了"凭据仍是
// 占位符"（红线：报告与测试都不含密钥），不该有任何问题——没有未知键、没有
// 语法错误、没有 extends 报错。
func TestRunConfigRewriteStrictValidation(t *testing.T) {
	if _, err := os.Stat(rewriteNew); err != nil {
		t.Skipf("没有 %s: %v", rewriteNew, err)
	}
	problems := ValidateFile(rewriteNew)
	var left []string
	for _, p := range problems {
		if strings.Contains(p, "占位符") {
			continue
		}
		left = append(left, p)
	}
	if len(left) != 0 {
		sort.Strings(left)
		t.Fatalf("改写稿除了占位符之外还有问题: %v", left)
	}
	// 未知键告警同样必须为空（这条不经过 setup）。
	if ks, err := unknownKeys(mustRead(t, rewriteNew)); err != nil || len(ks) != 0 {
		t.Fatalf("改写稿有未知键: %+v (%v)", ks, err)
	}
}

// TestRunConfigRewriteKeepsFourRolesToOneEntry 钉住"那四块并成一条命名条目"：
// 改写稿里恰好一条 heavy，四个角色各一行 extends 指向它。
func TestRunConfigRewriteKeepsFourRolesToOneEntry(t *testing.T) {
	if _, err := os.Stat(rewriteNew); err != nil {
		t.Skipf("没有 %s: %v", rewriteNew, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(mustRead(t, rewriteNew), &doc); err != nil {
		t.Fatal(err)
	}
	models := mapNodeValue(documentRoot(&doc), modelsKey)
	if models == nil {
		t.Fatal("没有 models:")
	}
	entries := map[string]*yaml.Node{}
	for i := 0; i+1 < len(models.Content); i += 2 {
		entries[models.Content[i].Value] = models.Content[i+1]
	}
	base, ok := entries["heavy"]
	if !ok {
		t.Fatal("没有命名基座 heavy")
	}
	if n := len(base.Content) / 2; n < 4 {
		t.Errorf("heavy 太单薄（%d 个键），应当装着四个角色共用的模型/单价/思考设置", n)
	}
	for _, role := range []string{"drawing", "style", "chapter", "convert"} {
		e, ok := entries[role]
		if !ok {
			t.Errorf("缺少角色条目 models.%s", role)
			continue
		}
		ext := mapNodeValue(e, extendsKey)
		if ext == nil || ext.Value != "heavy" {
			t.Errorf("models.%s 应当只有一行 extends: heavy，实际 %d 个键", role, len(e.Content)/2)
		}
	}
	if len(entries) != 9 {
		t.Errorf("条目数 = %d，期望 9（text/classifier/heavy/四个角色/checker/verifier）", len(entries))
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
