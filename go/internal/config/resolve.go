package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultDir returns ~/.docvision, the per-user home for the global
// config and ad-hoc job workspaces.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".docvision"), nil
}

// DefaultConfigPath returns ~/.docvision/config.yaml.
func DefaultConfigPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// EnsureDefaultConfig creates ~/.docvision/ and a template config.yaml
// when the global config does not exist. Returns the config path and
// whether the file was just created.
func EnsureDefaultConfig() (string, bool, error) {
	string2, err := DefaultConfigPath()
	if err != nil {
		return "", false, err
	}
	string3, err := DefaultDir()
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(string3, 0o755); err != nil {
		return "", false, fmt.Errorf("create %s: %w", string3, err)
	}
	if _, err := os.Stat(string2); err == nil {
		return string2, false, nil
	}
	if err := os.WriteFile(string2, []byte(ConfigTemplate), 0o644); err != nil {
		return "", false, fmt.Errorf("write %s: %w", string2, err)
	}
	return string2, true, nil
}

// ResolveConfigPath applies the configuration lookup order:
//  1. explicit path (from --config)
//  2. ./config.yaml in the current directory (kept for existing projects)
//  3. ~/.docvision/config.yaml (auto-created from the template)
//
// source is "explicit" / "local" / "global"; created reports that the
// global config was just written.
func ResolveConfigPath(explicit string) (path, source string, created bool, err error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, "explicit", false, nil
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		abs, aerr := filepath.Abs("config.yaml")
		if aerr != nil {
			return "", "", false, aerr
		}
		return abs, "local", false, nil
	}
	path, created, err = EnsureDefaultConfig()
	if err != nil {
		return "", "", false, err
	}
	return path, "global", created, nil
}

// ValidateData parses config content strictly (unknown keys are errors),
// applies defaults, and runs semantic checks. It returns every problem
// found so an editor loop can show them all at once; empty slice = valid.
// `models.<名>.extends` is merged at the node level first, exactly like
// LoadConfig does, so the strict path and the run path agree.
func ValidateData(data []byte) []string {
	var problems []string
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []string{"YAML 语法错误: " + err.Error()}
	}
	if err := mergeModelExtends(&doc); err != nil {
		problems = append(problems, err.Error())
	}
	cfg := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(mergedData(data, &doc)))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		var terr *yaml.TypeError
		if errors.As(err, &terr) {
			// yaml.v3 reports "line N: field X not found in type config.ModelConfig"
			// — it cannot name the registry entry, and after a merge the line may
			// have moved into another entry. Replace that line with the config
			// paths that really carry the key, so the message points at a line the
			// user can edit.
			for _, e := range terr.Errors {
				problems = append(problems, nameUnknownField(e, data))
			}
		} else {
			return []string{"YAML 语法错误: " + err.Error()}
		}
	}
	setDefaults(cfg)
	if err := validatePaths(cfg); err != nil {
		problems = append(problems, err.Error())
	}
	if err := checkDuplicateWireNames(cfg); err != nil {
		problems = append(problems, err.Error())
	}
	if cfg.ConfigVersion != CurrentConfigVersion {
		problems = append(problems, fmt.Sprintf("config_version 应为 %d（当前为 %d）—— 请参考 config.example.yaml 更新配置文件", CurrentConfigVersion, cfg.ConfigVersion))
	}
	problems = append(problems, semanticChecks(cfg)...,
	)
	return problems
}

// nameUnknownField turns yaml.v3's "line N: field X not found in type
// config.ModelConfig" into a message that names the config path the key is read
// at, appending the entries that inherited it through `extends`. Anything it
// cannot recognise is returned unchanged.
func nameUnknownField(msg string, data []byte) string {
	field := ""
	if i := strings.Index(msg, "field "); i >= 0 {
		rest := msg[i+len("field "):]
		if j := strings.Index(rest, " not found"); j > 0 {
			field = rest[:j]
		}
	}
	if field == "" {
		return msg
	}
	keys, err := unknownKeys(data)
	if err != nil {
		return msg
	}
	var paths []string
	for _, k := range keys {
		if k.Path == field || strings.HasSuffix(k.Path, "."+field) {
			paths = append(paths, k.Path)
		}
	}
	if len(paths) == 0 {
		return msg
	}
	sort.Strings(paths)
	return fmt.Sprintf("%s（未知配置键 %s）", msg, strings.Join(paths, ", "))
}

// mergedData re-encodes the merged node tree; when encoding is impossible (it
// only fails for a document we could not have built) the original bytes are
// used, so the strict decode below still reports its own problems.
func mergedData(original []byte, doc *yaml.Node) []byte {
	out, err := yaml.Marshal(doc)
	if err != nil {
		return original
	}
	return out
}

func semanticChecks(cfg *Config) []string {
	var p []string
	req := func(cond bool, msg string) {
		if !cond {
			p = append(p, msg)
		}
	}
	req(strings.TrimSpace(cfg.Mineru.APIBaseURL) != "", "mineru.api_base_url 不能为空")
	req(strings.TrimSpace(cfg.Mineru.Token) != "", "mineru.token 不能为空")
	req(!strings.Contains(cfg.Mineru.Token, "YOUR-"), "mineru.token 仍是占位符，请填入真实 token")
	req(cfg.Mineru.PollInterval >= 1, "mineru.poll_interval 必须 >= 1")
	req(cfg.Mineru.MaxConcurrent >= 1, "mineru.max_concurrent 必须 >= 1")
	text := cfg.Models[defaultModelKey]
	req(strings.TrimSpace(text.BaseURL) != "", "models.text.base_url 不能为空")
	req(strings.TrimSpace(text.APIKey) != "", "models.text.api_key 不能为空")
	req(!strings.Contains(text.APIKey, "YOUR-"), "models.text.api_key 仍是占位符，请填入真实 key")
	req(strings.TrimSpace(text.Model) != "", "models.text.model 不能为空")
	req(cfg.Options.Concurrency >= 1, "options.concurrency 必须 >= 1")

	req(cfg.Options.MaxTokens >= 1, "options.max_tokens 必须 >= 1")
	switch strings.ToLower(cfg.Options.LogLevel) {
	case "info", "debug", "trace":
	default:
		p = append(p, fmt.Sprintf("options.log_level 必须为 info/debug/trace（当前 %q）", cfg.Options.LogLevel))
	}
	// Per-model vendor request fields: catch typos before a long run.
	for name, m := range cfg.Models {
		label := "models." + name
		if m.ReasoningEffort != "" && !validReasoningEfforts[m.ReasoningEffort] {
			p = append(p, fmt.Sprintf("%s.reasoning_effort 必须为 max/xhigh/high/medium/low/minimal/none（当前 %q）", label, m.ReasoningEffort))
		}
		if m.Thinking != nil {
			if v, ok := m.Thinking["type"]; ok {
				if s, _ := v.(string); s != "enabled" && s != "disabled" {
					p = append(p, fmt.Sprintf("%s.thinking.type 必须为 enabled 或 disabled（当前 %v）", label, v))
				}
			} else {
				p = append(p, fmt.Sprintf("%s.thinking 缺少 type（enabled|disabled）", label))
			}
		}
		if m.APIStreamIdleTimeout < 0 {
			p = append(p, fmt.Sprintf("%s.api_stream_idle_timeout 不能为负", label))
		}
		if m.ImageTokens != nil {
			p = append(p, checkEstimateMethod(label+".image_tokens", *m.ImageTokens)...)
		}
	}
	p = append(p, checkEstimateMethod("estimate", cfg.Estimate)...)
	req(strings.TrimSpace(cfg.Paths.InputDir) != "", "paths.input_dir 不能为空")
	req(strings.TrimSpace(cfg.Paths.OutputDir) != "", "paths.output_dir 不能为空")
	req(strings.TrimSpace(cfg.Paths.FinallyDir) != "", "paths.finally_dir 不能为空")
	req(strings.TrimSpace(cfg.Paths.LogsDir) != "", "paths.logs_dir 不能为空")
	return p
}

// checkEstimateMethod validates one estimate block (the top-level `estimate:`
// or a model's `image_tokens:`): the method must be one of the three known
// names, and the parameters it uses must be ordered.
func checkEstimateMethod(label string, e EstimateConfig) []string {
	var p []string
	switch strings.ToLower(strings.TrimSpace(e.Method)) {
	case "", EstimateMethodFixed, EstimateMethodPixels, EstimateMethodNone:
	default:
		p = append(p, fmt.Sprintf("%s.method 必须为 fixed/pixels/none（当前 %q）", label, e.Method))
	}
	if e.PxPerToken < 0 || e.Tokens < 0 || e.MinTokens < 0 || e.MaxTokens < 0 {
		p = append(p, fmt.Sprintf("%s 的 tokens/px_per_token/min_tokens/max_tokens 不能为负", label))
	}
	if e.MaxTokens != 0 && e.MinTokens != 0 && e.MaxTokens < e.MinTokens {
		p = append(p, fmt.Sprintf("%s.max_tokens 不能小于 min_tokens（%d < %d）", label, e.MaxTokens, e.MinTokens))
	}
	return p
}

// ValidateFile runs ValidateData on the file at path.
func ValidateFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("读取配置失败: %v", err)}
	}
	return ValidateData(data)
}
