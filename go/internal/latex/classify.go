package latex

import (
	"fmt"
	"strings"

	"mineru-tools/internal/config"
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
}

// ClassifyImage sends one image to the classifier model and parses the
// strict JSON verdict. Any failure degrades to ClassRaster (the safe
// choice: keep the original image).
func ClassifyImage(client *session.Client, modelCfg config.ModelConfig, imgBase64, systemExtra string) (Classification, error) {
	req := &session.ChatRequest{
		Model: client.Model(),
		Messages: []session.ChatMessage{
			{Role: "system", Content: classifierSystemPrompt + systemExtra},
			{Role: "user", Content: []map[string]interface{}{
				{"type": "text", "text": "Classify this document image."},
				{"type": "image_url", "image_url": map[string]string{
					"url": "data:image/jpeg;base64," + imgBase64,
				}},
			}},
		},
		MaxTokens:      512,
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
	return parseClassification(text)
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
	return c, nil
}
