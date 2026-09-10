package img2text

import (
	"strings"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/prompts"
)

// NoTextMarker is returned by ExtractTextOnly when the image carries no
// readable text (the caller should keep the original image instead).
const NoTextMarker = "[NO_TEXT]"

// ExtractTextOnly runs one vision call and returns only the visible text
// of the image. contextText is optional surrounding document text used
// for reading order/terminology; the model is told not to copy it.
// Returns (text, StatusOK) or (sentinel, StatusError).
func ExtractTextOnly(
	client *AIClient,
	imgBase64, contextText string,
	opts config.OptionsConfig,
	log *logger.Logger,
	tid int,
) (string, string) {
	user := "Extract the visible text of this image."
	if strings.TrimSpace(contextText) != "" {
		user += "\n\nSurrounding document text (for reading order and terminology only — do NOT copy it into your answer):\n```\n" +
			truncate(contextText, 4000) + "\n```"
	}
	req := &ChatRequest{
		Model: client.Model(),
		Messages: []ChatMessage{
			{Role: "system", Content: prompts.Must(prompts.TextOnlySystem)},
			{Role: "user", Content: []map[string]interface{}{
				{"type": "text", "text": user},
				{"type": "image_url", "image_url": map[string]string{
					"url": "data:image/jpeg;base64," + imgBase64,
				}},
			}},
		},
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
	}
	resp, sentinel, status := doCallWithRetryFull(client, req, client.MaxRetries, client.RateLimitRetries, log, tid)
	if status != "" {
		return sentinel, status
	}
	if len(resp.Choices) == 0 {
		return sentinelEmpty, StatusError
	}
	text := strings.TrimSpace(contentString(resp.Choices[0].Message))
	// Defensive: some models still prepend the extraction header.
	if strings.HasPrefix(text, "[IMG_TYPE:") {
		if i := strings.Index(text, "]"); i >= 0 {
			text = strings.TrimSpace(text[i+1:])
		}
	}
	return text, StatusOK
}
