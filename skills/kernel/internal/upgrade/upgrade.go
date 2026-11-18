// Package upgrade 上游升级自检：开跑前/排障时先查"上游是否已有更新或已修"，
// 避免在旧版本上白排查（SKILL.md 第 0 步与 docs/07 的"查"步都指向这里）。
// 数据源：GitHub REST（公开读免鉴权）；本地基准：git 检出的 HEAD 或安装包 .skill-version。
package upgrade

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Upstream 上游仓库配置（config.yaml upstream 段 / 内置默认）。
type Upstream struct {
	Repo       string `json:"repo"`
	Branch     string `json:"branch"`
	SkillsPath string `json:"skills_path"`
	APITemplate string `json:"api_template"` // https://api.github.com 或镜像；测试注入
}

// DefaultUpstream 出厂默认。
func DefaultUpstream() Upstream {
	return Upstream{Repo: "LingNc/DocVision", Branch: "feat/skills", SkillsPath: "skills",
		APITemplate: "https://api.github.com"}
}

// Result 自检结果。
type Result struct {
	LocalRef        string `json:"local_ref"`         // 本地基准（HEAD 或 .skill-version）
	RemoteRef       string `json:"remote_ref"`        // 远端分支 HEAD
	Behind          int    `json:"behind"`            // 落后提交数（-1=未知）
	SkillsChanged   bool   `json:"skills_changed"`    // skills/ 上游有本地没有的提交
	LatestSkillsSHA string `json:"latest_skills_sha"` // skills/ 最新提交
	Source          string `json:"source"`            // git | version-file
	UpgradeNeeded   bool   `json:"upgrade_needed"`    // 结论：需要升级
	Note            string `json:"note"`
}

// Check 跑自检。localRef 显式给（如 git HEAD）；空则尝试探测：
// 优先 git rev-parse（在 repoRoot 内），退而读 versionFile（安装包的 .skill-version）。
func Check(u Upstream, repoRoot, versionFile, localRef string) (*Result, error) {
	d := DefaultUpstream()
	if u.Repo == "" {
		u.Repo = d.Repo
	}
	if u.Branch == "" {
		u.Branch = d.Branch
	}
	if u.SkillsPath == "" {
		u.SkillsPath = d.SkillsPath
	}
	if u.APITemplate == "" {
		u.APITemplate = d.APITemplate
	}
	res := &Result{Source: "version-file"}
	// 本地基准
	if localRef == "" && repoRoot != "" {
		if out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--short", "HEAD").Output(); err == nil {
			localRef = strings.TrimSpace(string(out))
			res.Source = "git"
		}
	}
	if localRef == "" && versionFile != "" {
		if b, err := os.ReadFile(versionFile); err == nil {
			localRef = strings.TrimSpace(string(b))
		}
	}
	res.LocalRef = localRef

	api := u.APITemplate
	if api == "" {
		api = "https://api.github.com"
	}
	client := &http.Client{Timeout: 15 * time.Second}

	// 远端分支 HEAD
	branchURL := fmt.Sprintf("%s/repos/%s/commits/%s", api, u.Repo, u.Branch)
	var br struct {
		SHA string `json:"sha"`
	}
	if err := getJSON(client, branchURL, &br); err != nil {
		res.Note = "远端查询失败（网络/限流？）：" + err.Error()
		return res, nil
	}
	res.RemoteRef = br.SHA[:12]

	// skills/ 最新提交
	skURL := fmt.Sprintf("%s/repos/%s/commits?path=%s&sha=%s&per_page=1", api, u.Repo, u.SkillsPath, u.Branch)
	var sks []struct {
		SHA string `json:"sha"`
	}
	if err := getJSON(client, skURL, &sks); err == nil && len(sks) > 0 {
		res.LatestSkillsSHA = sks[0].SHA[:12]
	}
	// 本地是否已含 skills/ 最新提交：前缀启发式判不了祖先关系（本地领先 skills 提交时
	// 两者无前缀关系但本地已含），用 compare(latest...local) 判 ancestry——
	// identical/ahead=已含（不变更），behind/diverged=本地缺（有变更）。
	res.SkillsChanged = false
	if res.LatestSkillsSHA != "" && res.LocalRef != "" && !strings.HasPrefix(res.LatestSkillsSHA, localRef) && !strings.HasPrefix(localRef, res.LatestSkillsSHA) {
		cmp2URL := fmt.Sprintf("%s/repos/%s/compare/%s...%s", api, u.Repo, res.LatestSkillsSHA, res.LocalRef)
		var cmp2 struct {
			Status string `json:"status"`
		}
		if err := getJSON(client, cmp2URL, &cmp2); err == nil {
			res.SkillsChanged = cmp2.Status == "behind" || cmp2.Status == "diverged"
		} else {
			res.SkillsChanged = true // 查不动时按"有变更"保守处理（只引导查上游，无副作用）
		}
	}

	// behind 数（compare API）
	if res.LocalRef != "" && len(res.LocalRef) >= 7 {
		cmpURL := fmt.Sprintf("%s/repos/%s/compare/%s...%s", api, u.Repo, res.LocalRef, u.Branch)
		var cmp struct {
			Status  string `json:"status"`
			BehindBy int   `json:"behind_by"`
		}
		if err := getJSON(client, cmpURL, &cmp); err == nil && cmp.Status == "behind" {
			res.Behind = cmp.BehindBy
		} else if err == nil {
			res.Behind = 0
		}
	}
	res.UpgradeNeeded = (res.Behind > 0) || res.SkillsChanged
	if res.UpgradeNeeded {
		res.Note = fmt.Sprintf("上游有更新（落后 %d 提交，skills/ %s）：先 git pull / 重出安装包再排障或开跑", res.Behind, map[bool]string{true: "有变更", false: "无变更"}[res.SkillsChanged])
	} else {
		res.Note = "已是最新（或上游 skills/ 无新提交）"
	}
	return res, nil
}

// VersionFilePath 安装包内版本文件路径约定（技能根 .skill-version）。
func VersionFilePath(skillRoot string) string { return filepath.Join(skillRoot, ".skill-version") }

func getJSON(c *http.Client, url string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "docvision-skills-upgrade-check")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
