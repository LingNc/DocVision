package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
func ValidateData(data []byte) []string {
	var problems []string
	cfg := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		var terr *yaml.TypeError
		if errors.As(err, &terr) {
			for _, e := range terr.Errors {
				problems = append(problems, e)
			}
		} else {
			return []string{"YAML 语法错误: " + err.Error()}
		}
	}
	setDefaults(cfg)
	if err := validatePaths(cfg); err != nil {
		problems = append(problems, err.Error())
	}
	if cfg.ConfigVersion != CurrentConfigVersion {
		problems = append(problems, fmt.Sprintf("config_version 应为 %d（当前为 %d）—— 请参考 config.example.yaml 更新配置文件", CurrentConfigVersion, cfg.ConfigVersion))
	}
	problems = append(problems, semanticChecks(cfg)...,
	)
	return problems
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
	req(strings.TrimSpace(cfg.Paths.InputDir) != "", "paths.input_dir 不能为空")
	req(strings.TrimSpace(cfg.Paths.OutputDir) != "", "paths.output_dir 不能为空")
	req(strings.TrimSpace(cfg.Paths.FinallyDir) != "", "paths.finally_dir 不能为空")
	req(strings.TrimSpace(cfg.Paths.LogsDir) != "", "paths.logs_dir 不能为空")
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
