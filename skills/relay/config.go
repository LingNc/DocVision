// config.go：relay 服务器配置加载。
//
// 分层优先级（与技能侧 docs/06 规定一致）：
//   环境变量（RELAY_LISTEN / RELAY_REPO / RELAY_RATE）> config 文件 > 内置默认。
//
// 密钥纪律：RELAY_GITHUB_TOKEN 是服务器侧秘密，只走环境变量，永不进 config 文件
//（docs/06-profile-config.md：relay 服务器的凭据属服务器侧秘密，只在部署机环境变量）。
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// relayConfig 服务器配置（全部非秘密项；秘密=环境变量）。
type relayConfig struct {
	Listen string `yaml:"listen"` // 监听地址，如 ":8787"
	Repo   string `yaml:"repo"`   // 代建 issue 的目标仓库 owner/name
	Rate   int    `yaml:"rate"`   // 每 IP 每小时提交上限
}

// defaults 内置默认（与 93 批 env-only 版的行为保持一致）。
var defaults = relayConfig{Listen: ":8787", Repo: "LingNc/DocVision", Rate: 5}

// loadConfig 从 path 读 yaml（缺文件=纯默认；解析失败=报错不静默），
// 再让环境变量覆盖。token 由调用方从环境变量读，不经此层。
func loadConfig(path string) (relayConfig, error) {
	cfg := defaults
	if raw, err := os.ReadFile(path); err == nil {
		var yc relayConfig
		if err := yaml.Unmarshal(raw, &yc); err != nil {
			return cfg, fmt.Errorf("config 文件解析失败: %w", err)
		}
		// 零值=未写，保留默认
		if yc.Listen != "" {
			cfg.Listen = yc.Listen
		}
		if yc.Repo != "" {
			cfg.Repo = yc.Repo
		}
		if yc.Rate > 0 {
			cfg.Rate = yc.Rate
		}
	} else if !os.IsNotExist(err) {
		return cfg, fmt.Errorf("config 文件读取失败: %w", err)
	}
	if v := os.Getenv("RELAY_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("RELAY_REPO"); v != "" {
		cfg.Repo = v
	}
	if v := os.Getenv("RELAY_RATE"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &cfg.Rate); n != 1 || err != nil {
			return cfg, fmt.Errorf("RELAY_RATE 解析失败: %q", v)
		}
	}
	return cfg, nil
}
