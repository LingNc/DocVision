package troubleshoot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartCollectValidateSubmit(t *testing.T) {
	root := t.TempDir()
	st, err := Start(root)
	if err != nil || st != StateStarted {
		t.Fatalf("start: %v %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(Dir(root), "report.md")); err != nil {
		t.Fatal("模板应生成")
	}
	// collect 自动填环境节
	st, err = Collect(root, `{"tools":[{"name":"mmdc","found":true}],"fonts":{"cjk_count":20}}`)
	if err != nil || st != StateCollected {
		t.Fatalf("collect: %v %v", st, err)
	}
	body, _ := os.ReadFile(filepath.Join(Dir(root), "report.md"))
	if !strings.Contains(string(body), "cjk_count") {
		t.Fatal("环境节应被 collect 填充")
	}
	// 未填写 → validate 拒
	errs, _, _ := Validate(root)
	if len(errs) == 0 {
		t.Fatal("空报告应校验失败")
	}
	// 填好各节（含真实内容）
	fill := `## 复现步骤
1. dvagent prep-mineru-api demo x.pdf

## 期望与实际
期望：上传成功；实际：DNS 失败 dial tcp: lookup api... （真实报错）

## 脱敏日志
见 logs/doctor-*.json

## 根因分析
沙箱 DNS 不解析 mineru.net；换网络后重试。

## 修复建议
文档补充网络要求；代码无改动。`
	b, _ := os.ReadFile(filepath.Join(Dir(root), "report.md"))
	s := string(b)
	idx := strings.Index(s, "## 复现步骤")
	s = s[:idx] + fill + "\n"
	os.WriteFile(filepath.Join(Dir(root), "report.md"), []byte(s), 0o644)

	errs, stt, _ := Validate(root)
	if len(errs) != 0 {
		t.Fatalf("填好后应通过: %v", errs)
	}
	if stt != StateValidated {
		t.Fatalf("状态应为 validated: %s", stt)
	}
	// submit 出 issue.md（无注释）
	p, final, err := Submit(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(final, "步骤提示") || strings.Contains(final, "<!--") {
		t.Fatal("提交文本不应含注释/提示")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("issue.md 应生成")
	}
}

func TestValidateRedaction(t *testing.T) {
	root := t.TempDir()
	Start(root)
	leak := "\n\n## 复现步骤\n1. x\n\n## 期望与实际\nBearer abcdefghijklmnopqrstuvwxyz1234567890\n\n## 脱敏日志\nx\n\n## 根因分析\nx\n\n## 修复建议\nx"
	b, _ := os.ReadFile(filepath.Join(Dir(root), "report.md"))
	s := string(b)
	idx := strings.Index(s, "## 复现步骤")
	s = s[:idx] + leak + "\n"
	os.WriteFile(filepath.Join(Dir(root), "report.md"), []byte(s), 0o644)
	errs, _, _ := Validate(root)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "Bearer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应检出 Bearer 未脱敏: %v", errs)
	}
}

func TestStripComments(t *testing.T) {
	out := stripComments("a\n<!-- x -->\n# 注释\n## 环境\n正文")
	if strings.Contains(out, "注释") || strings.Contains(out, "x -->") {
		t.Fatalf("注释应被剥离: %q", out)
	}
	if !strings.Contains(out, "## 环境") {
		t.Fatal("标题保留")
	}
}
