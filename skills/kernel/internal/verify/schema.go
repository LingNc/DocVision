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

// ClassifyVerdict 是 T1 分类的落盘结构（SK-I18 恢复旧参考实现四类 + styled 独立标记，
// 原模板 prompts-archive/latex_classifier.system.md）。下游分流：
//   text/table + styled=false → 文字化直通 md，不进样式流程
//   text/table + styled=true  → 文字化 + 进 style 素材池（CollectStyleMaterials）
//   vector → draw 重绘；raster → 保留原图
type ClassifyVerdict struct {
	Class     string `json:"class"`     // text | table | vector | raster
	Extracted string `json:"extracted"`   // text/table 必填：提取的文本内容（table 重打为 md 表）
	Reason    string `json:"reason"`      // vector 必填：为什么可矢量重绘的一句话证据
	// Styled 独立布尔标记（必填）：内容带视觉样式（艺术字/颜色/边框/装饰）且对忠实还原重要才 true。
	Styled *bool `json:"styled"`
	// StyleNote styled=true 必填：样式线索（字体/颜色/边框/装饰的具体描述）。
	StyleNote string `json:"style_note"`
	// Watermark 水印标记（SK-I3，可选）：true 时合成默认去水印。
	Watermark bool `json:"watermark"`
	// Confidence 分类置信度（SK-I5，必填 0~1）：拿不准就压低，<0.6 内核强制重判。
	Confidence *float64 `json:"confidence"`
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
		case "stylefix-assess":
		return c.checkStylefixAssessVerdict(b, sandboxAbs)
	case "stylefix":
		return checkStylefixVerdict(b)
	case "img2text":
		return c.checkImg2TextVerdict(b, sandboxAbs)
	}
	return []string{"未知 verdict_schema: " + schema}
}

// checkClassifyVerdict 四类校验（SK-I18 恢复旧参考实现；styled 独立标记必填）。
// text/table 必须附提取内容；vector 必须附可重绘证据；raster 允许无附加。
func checkClassifyVerdict(b []byte) []string {
	var v ClassifyVerdict
	if err := json.Unmarshal(b, &v); err != nil {
		return []string{"verdict.json 解析失败: " + err.Error()}
	}
	var errs []string
	switch v.Class {
	case "text", "table":
		if strings.TrimSpace(v.Extracted) == "" {
			errs = append(errs, v.Class+" 分类必须附 extracted（提取的文本内容；table 重打为 md 表），不允许空提取占位")
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
		errs = append(errs, fmt.Sprintf("class %q 不合法（只允许 text/table/vector/raster）——分类不确定就重试，不得乱填", v.Class))
	}
	// styled 独立标记（必填）：视觉样式且对忠实还原重要才 true；true 必须给 style_note
	if v.Styled == nil {
		errs = append(errs, "verdict.json 必须填 styled（布尔：内容带艺术字/颜色/边框/装饰等视觉样式且对还原重要→true；正文/普通公式/普通表→false）")
	} else if *v.Styled && strings.TrimSpace(v.StyleNote) == "" {
		errs = append(errs, "styled=true 必须附 style_note（字体/颜色/边框/装饰的具体样式线索）")
	}
	// SK-I5 置信度闸（必填；低置信强制重判，不许硬判）
	if v.Confidence == nil {
		errs = append(errs, "verdict.json 必须填 confidence（0~1，对自己分类把握打分；犹豫就压低，<0.6 强制重判）")
	} else if *v.Confidence < 0 || *v.Confidence > 1 {
		errs = append(errs, fmt.Sprintf("confidence %.2f 越界（须在 0~1）", *v.Confidence))
	} else if *v.Confidence < 0.6 {
		errs = append(errs, fmt.Sprintf("confidence %.2f < 0.6：低置信强制重判——拿不准就重看原图，不许硬判", *v.Confidence))
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
// checkStylefixAssessVerdict：assess 结论（done/blocked）+ blocks.json 已产出（一章一块由 assess 自验）。
func (c *Checker) checkStylefixAssessVerdict(b []byte, sandboxAbs string) []string {
	var v struct {
		Result string `json:"result"`
		Blocks int    `json:"blocks"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return []string{"verdict.json 解析失败: " + err.Error()}
	}
	switch v.Result {
	case "done":
		if v.Blocks < 1 {
			return []string{"result=done 必须 blocks>=1"}
		}
		data, err := os.ReadFile(filepath.Join(sandboxAbs, "out", "blocks.json"))
		if err != nil || len(data) == 0 {
			return []string{"done 必须附 out/blocks.json（非空）"}
		}
		return nil
	case "blocked":
		if strings.TrimSpace(v.Reason) == "" {
			return []string{"blocked 必须附 reason"}
		}
		return nil
	case "":
		return []string{"verdict.json 缺少 result（done/blocked）"}
	}
	return []string{fmt.Sprintf("result %q 不合法（只允许 done/blocked）", v.Result)}
}

// checkStylefixVerdict：逐章适配结论（adapted/blocked）。
func checkStylefixVerdict(b []byte) []string {
	var v struct {
		Result    string `json:"result"`
		Chapter   string `json:"chapter"`
		DiffLines int    `json:"diff_lines"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return []string{"verdict.json 解析失败: " + err.Error()}
	}
	if v.Chapter == "" {
		return []string{"verdict.json 缺少 chapter"}
	}
	switch v.Result {
	case "adapted":
		return nil
	case "blocked":
		if strings.TrimSpace(v.Reason) == "" {
			return []string{"blocked 必须附 reason（不许硬改内容）"}
		}
		return nil
	case "":
		return []string{"verdict.json 缺少 result（adapted/blocked）"}
	}
	return []string{fmt.Sprintf("result %q 不合法（只允许 adapted/blocked）", v.Result)}
}

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
