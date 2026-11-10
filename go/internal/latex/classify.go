package latex

import (
	"fmt"
	"strings"

	"mineru-tools/internal/config"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// Image classification kinds.
const (
	ClassText   = "text"   // stylised text (artistic question numbers...)
	ClassVector = "vector" // structurally reproducible graphic (TikZ-able)
	ClassRaster = "raster" // photos / screenshots / un-reproducible artwork
)

// Classification is the classifier verdict for one image.
type Classification struct {
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence"`
	Label      string  `json:"label"`
	Reason     string  `json:"reason"`
	Styled     bool    `json:"styled"`     // 带视觉样式（艺术字/颜色/装饰），纯文本无法表达
	StyleNote  string  `json:"style_note"` // 样式描述（字体/颜色/装饰/布局）
}

// classifyMaxTokens 是分类请求的输出预算。512 曾经不够：思考型模型（回复里
// 带 reasoning）会把预算先花在思考上，JSON 还没开始就被截断，而调用方只看到
// "回复中未找到 JSON 对象"（T306）。
const classifyMaxTokens = 2048

// ClassifyImage sends one image to the classifier model and parses the
// strict JSON verdict. 失败返回 error，由调用方记"未分类 + 错误"并让分类
// 阶段停下——**不要**在这里降级成 raster（T306）。
func ClassifyImage(client *session.Client, modelCfg config.ModelConfig, imgBase64, systemExtra string) (Classification, error) {
	req := &session.ChatRequest{
		Model: client.Model(),
		Messages: []session.ChatMessage{
			{Role: "system", Content: prompts.Must(prompts.ClassifierSystem) + systemExtra},
			{Role: "user", Content: []map[string]interface{}{
				{"type": "text", "text": "Classify this document image."},
				{"type": "image_url", "image_url": map[string]string{
					"url": "data:image/jpeg;base64," + imgBase64,
				}},
			}},
		},
		MaxTokens:      classifyMaxTokens,
		Temperature:    0.0,
		ResponseFormat: map[string]any{"type": "json_object"}, // 保证返回一定是 JSON
	}
	resp, sentinel, status := client.CallWithRetry(req)
	if status != "" {
		return Classification{}, fmt.Errorf("分类请求失败: %s", sentinel)
	}
	if len(resp.Choices) == 0 {
		return Classification{}, fmt.Errorf("分类响应为空")
	}
	text := session.ContentString(resp.Choices[0].Message)
	c, err := parseClassification(text)
	if err != nil {
		return Classification{}, classifyReplyError(resp.FinishReason, text, err)
	}
	return c, nil
}

// classifyReplyError 把"回复里没有可用的 JSON"变成能直接定位原因的错误：
// 说清是**被截断**、**空的**、还是**模型说了别的话**（附上它原话的前 200
// 字）。只报一句"回复中未找到 JSON 对象"看不出该调预算、该换端点还是该换
// 模型（T306）。
func classifyReplyError(finishReason, reply string, err error) error {
	if finishReason == "length" {
		return fmt.Errorf("分类回复被截断（max_tokens=%d 用光，多半被思考过程占满；调大预算或换非思考模型）：%w",
			classifyMaxTokens, err)
	}
	snippet := strings.TrimSpace(reply)
	if snippet == "" {
		return fmt.Errorf("分类回复为空（finish_reason=%s）：%w", finishReason, err)
	}
	if r := []rune(snippet); len(r) > 200 {
		snippet = string(r[:200]) + "…"
	}
	return fmt.Errorf("%w；模型实际回复：%q", err, snippet)
}

// parseClassification extracts the JSON verdict from a model reply.
func parseClassification(reply string) (Classification, error) {
	obj, err := parseJSONObject(reply)
	if err != nil {
		return Classification{}, err
	}
	kind, _ := obj["kind"].(string)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "table" {
		// Markdown 可表达的表格走文本管线：img2text 的 table 类型
		// 会产出 Markdown 表格并直接嵌入。
		kind = ClassText
	}
	switch kind {
	case ClassText, ClassVector, ClassRaster:
	default:
		return Classification{}, fmt.Errorf("未知分类 %q", kind)
	}
	c := Classification{Kind: kind, Label: "", Reason: ""}
	if v, ok := obj["confidence"].(float64); ok {
		c.Confidence = v
	}
	if v, ok := obj["label"].(string); ok {
		c.Label = strings.TrimSpace(v)
	}
	if v, ok := obj["reason"].(string); ok {
		c.Reason = strings.TrimSpace(v)
	}
	if v, ok := obj["styled"].(bool); ok {
		c.Styled = v
	}
	if v, ok := obj["style_note"].(string); ok {
		c.StyleNote = strings.TrimSpace(v)
	}
	return c, nil
}
