package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/LingNc/DocVision/skills/kernel/internal/mdimg"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// verdict_schema 验收：T1 classify 与 T4 check 的 verdict.json 模式校验。
// 对应 docs/02-task-protocol.md：验收只看磁盘上的 verdict.json，不看 agent 自述。

// ClassifyVerdict 是 T1 三态分类的落盘结构（agent 写 out/verdict.json）。
type ClassifyVerdict struct {
	Class     string `json:"class"`     // styled_text | vector | raster
	Extracted string `json:"extracted"`   // styled_text 必填：提取的文本内容
	Reason    string `json:"reason"`      // vector 必填：为什么可矢量重绘的一句话证据
}

// CheckVerdict 是 T4 核对的落盘结构（agent 写 out/verdict.json）。
type CheckVerdict struct {
	Result  string   `json:"result"`  // pass | issues | no_verdict
	Issues  []string `json:"issues"`  // result=issues 时必填，每条必须带位置指针
}

// checkVerdictSchema 按 acceptance["verdict_schema"] 分派校验。
// 返回错误清单（空 = 通过）。
func (c *Checker) checkVerdictSchema(t *task.Task, sandboxAbs string) []string {
	schema, _ := t.Accept["verdict_schema"].(string)
	if schema == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(sandboxAbs, "out", "verdict.json"))
	if err != nil {
		return []string{"缺少 verdict.json: " + err.Error()}
	}
	switch schema {
	case "classify":
		return checkClassifyVerdict(b)
	case "check", "final":
		// final（终审）复用 check 语义：issues 必带位置指针、no_verdict 由重试上限兜底。
		return checkCheckVerdict(b)
	case "img2text":
		return c.checkImg2TextVerdict(b, sandboxAbs)
	}
	return []string{"未知 verdict_schema: " + schema}
}

// checkClassifyVerdict 三态校验（沿用旧线"失败不降级成 raster"判据）：
// class 必须合法；styled_text 必须附提取内容；vector 必须附可重绘证据。
func checkClassifyVerdict(b []byte) []string {
	var v ClassifyVerdict
	if err := json.Unmarshal(b, &v); err != nil {
		return []string{"verdict.json 解析失败: " + err.Error()}
	}
	var errs []string
	switch v.Class {
	case "styled_text":
		if strings.TrimSpace(v.Extracted) == "" {
			errs = append(errs, "styled_text 分类必须附 extracted（提取的文本内容），不允许空提取占位")
		}
	case "vector":
		if strings.TrimSpace(v.Reason) == "" {
			errs = append(errs, "vector 分类必须附 reason（为什么可矢量重绘的一句话证据）")
		}
	case "raster":
		// raster 允许无附加字段
	case "":
		errs = append(errs, "verdict.json 缺少 class 字段（不允许以空分类蒙混过关，失败就重试，不降级）")
	default:
		errs = append(errs, fmt.Sprintf("class %q 不合法（只允许 styled_text/vector/raster）——分类不确定就重试，不得乱填", v.Class))
	}
	return errs
}

// checkCheckVerdict 核对校验：issues 每条必须带位置指针；
// no_verdict 视为"无法核对"，由 RecordVerdict 的 attempts 上限兜底（旧线判据：
// 绝不因无结论无限重转整章——达到 maxAttempts 即 failed，不再派）。
func checkCheckVerdict(b []byte) []string {
	var v CheckVerdict
	if err := json.Unmarshal(b, &v); err != nil {
		return []string{"verdict.json 解析失败: " + err.Error()}
	}
	switch v.Result {
	case "pass":
		return nil
	case "issues":
		if len(v.Issues) == 0 {
			return []string{"result=issues 但 issues 为空清单（要么 pass，要么列具体问题）"}
		}
		var errs []string
		for i, iss := range v.Issues {
			if !hasPositionPointer(iss) {
				errs = append(errs, fmt.Sprintf("第 %d 条问题缺位置指针（必须能指到 md 段落或 .tex 位置或原书页）: %.60s", i+1, iss))
			}
		}
		return errs
	case "no_verdict":
		return []string{"no_verdict：无法给出核对结论，允许重试；达到重试上限后任务标记 failed（绝不因无结论无限重转）"}
	case "":
		return []string{"verdict.json 缺少 result 字段（pass/issues/no_verdict 三选一）"}
	}
	return []string{fmt.Sprintf("result %q 不合法（只允许 pass/issues/no_verdict）", v.Result)}
}

// hasPositionPointer 判断一条问题是否带位置指针：含 md/.tex 扩展名、章节号、页码或行号任一即算。
func hasPositionPointer(s string) bool {
	for _, marker := range []string{".md", ".tex", "第", "节", "页", "行", "chapter", "page", "L."} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}


// checkImg2TextVerdict 档位2 三态验收：result 声明 + 对应产物存在性 + mermaid 校验。
// mermaid 校验失败即打回（旧线"兜底错误"的落点：语法错不让它进合成）。
// img2TextTypeHeaderRe 产物首行的 [IMG_TYPE: <type>] 标识（沿用原参考实现输出格式，
// 用户拍板保留：成品 md 嵌入后类型标识随文保留，便于核对与后续处理）。
var img2TextTypeHeaderRe = regexp.MustCompile(`^\[IMG_TYPE:\s*(.+?)\s*\]`)

// img2TextAllowedTypes 八类图词表（与原提示词逐一对应，沿用"分类不确定就重试，不得乱填"判据）。
var img2TextAllowedTypes = map[string]bool{
	"Table": true, "Flowchart": true, "Gantt Chart": true,
	"Architecture/Network Diagram": true, "Graph/Chart": true,
	"Formula": true, "Code screenshot": true, "Simple illustration": true,
}

func (c *Checker) checkImg2TextVerdict(b []byte, sandboxAbs string) []string {
	var v struct {
		Result  string `json:"result"`
		ImgType string `json:"img_type"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return []string{"verdict.json 解析失败: " + err.Error()}
	}
	outDir := filepath.Join(sandboxAbs, "out")
	switch v.Result {
	case "text":
		data, err := os.ReadFile(filepath.Join(outDir, "replacement.md"))
		if err != nil || strings.TrimSpace(string(data)) == "" {
			return []string{"result=text 必须附非空 out/replacement.md"}
		}
		return checkImg2TextTypeHeader(v.ImgType, data)
	case "mermaid":
		data, err := os.ReadFile(filepath.Join(outDir, "figure.mmd"))
		if err != nil || strings.TrimSpace(string(data)) == "" {
			return []string{"result=mermaid 必须附非空 out/figure.mmd"}
		}
		if vr := mdimg.ValidateMermaid(context.Background(), "```mermaid\n"+string(data)+"\n```", "", 0); vr.Available && !vr.Valid {
			return []string{"mermaid 校验失败（兜底打回，按报错修语法）: " + vr.Error}
		}
		return checkImg2TextTypeHeader(v.ImgType, data)
	case "keep":
		return nil
	case "":
		return []string{"verdict.json 缺少 result 字段（text/mermaid/keep 三选一）"}
	}
	return []string{fmt.Sprintf("result %q 不合法（只允许 text/mermaid/keep）", v.Result)}
}

// checkImg2TextTypeHeader 校验 [IMG_TYPE:] 三道闸（仅 text/mermaid；keep 态豁免）：
// 产物首行必须是 [IMG_TYPE: <type>]；类型在八类词表内；verdict 的 img_type 与首行一致。
func checkImg2TextTypeHeader(imgType string, artifact []byte) []string {
	first := strings.TrimSpace(strings.SplitN(string(artifact), "\n", 2)[0])
	m := img2TextTypeHeaderRe.FindStringSubmatch(first)
	if m == nil {
		return []string{"产物首行必须是 [IMG_TYPE: <type>]（八类词表见手册，沿用原输出格式）"}
	}
	headerType := m[1]
	if !img2TextAllowedTypes[headerType] {
		return []string{fmt.Sprintf("IMG_TYPE %q 不在八类词表内（Table/Flowchart/Gantt Chart/Architecture\\/Network Diagram/Graph\\/Chart/Formula/Code screenshot/Simple illustration）——分类不确定就重试，不得乱填", headerType)}
	}
	if imgType == "" {
		return []string{"verdict.json 缺少 img_type 字段（须与产物 [IMG_TYPE:] 首行一致）"}
	}
	if imgType != headerType {
		return []string{fmt.Sprintf("img_type %q 与产物首行 [IMG_TYPE: %s] 不一致", imgType, headerType)}
	}
	return nil
}
