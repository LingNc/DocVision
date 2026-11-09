package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// T273：tool_aliases 是模型级开关（把网关换给模型的另一套工具名映射回本程序
// 的名字）。条目没写时从 models.text 继承，且继承的是拷贝——改一个条目不能
// 顺带改掉基座与其它引用者。
func TestToolAliasesInheritance(t *testing.T) {
	data := []byte(`
models:
  text:
    base_url: "https://registry.example/v1"
    api_key: "sk-registry"
    model: "glm-5.3-flash"
    tool_aliases:
      Read: read_file
      Bash: bash
  haiku:
    model: "claude-haiku-test"
    api: anthropic
`)
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		t.Fatal(err)
	}
	setDefaults(cfg)

	// 条目没写 tool_aliases → 整张表从 models.text 继承。
	got, ok := cfg.ResolveModel("haiku")
	if !ok {
		t.Fatal("haiku should resolve")
	}
	if got.ToolAliases["Read"] != "read_file" || got.ToolAliases["Bash"] != "bash" {
		t.Fatalf("haiku.tool_aliases = %v", got.ToolAliases)
	}
	// 明确写了就用自己的那份（不再混入基座）。
	cfg.Models["own"] = ModelConfig{Model: "z", ToolAliases: map[string]string{"Read": "read_file"}}
	mine, _ := cfg.ResolveModel("own")
	if len(mine.ToolAliases) != 1 || mine.ToolAliases["Bash"] != "" {
		t.Fatalf("own.tool_aliases = %v，写了就该用自己的整张表", mine.ToolAliases)
	}
	// 从基座继承来的那份必须是拷贝：改条目不能改到 models.text。
	got.ToolAliases["Read"] = "改写"
	if cfg.Models["text"].ToolAliases["Read"] != "read_file" {
		t.Fatalf("models.text 被改到了：%v", cfg.Models["text"].ToolAliases)
	}
}
