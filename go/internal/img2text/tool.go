package img2text

import "strconv"

// BuildTools builds the OpenAI-compatible "tools" payload that exposes
// the get_more_context function to the model. The bounds on more_above
// and more_below come from the per-request cap (config.Options.MaxWindowUp
// and MaxWindowDown) so the model can never ask for more than the
// configured maximum in a single call.
//
// The shape mirrors the Python reference exactly:
//
//	[{
//	    "type": "function",
//	    "function": {
//	        "name": "get_more_context",
//	        "description": "...",
//	        "parameters": {"type": "object", "properties": {...}, "required": [...]}
//	    }
//	}]
func BuildTools(maxPerRequestUp, maxPerRequestDown int) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "get_more_context",
				"description": "Request ADDITIONAL lines above or below the image. " +
					"You will receive ONLY the NEW lines (not previously seen content). " +
					"Use this when the current context is insufficient to understand the image. " +
					"IMPORTANT: Each subsequent call must request MORE lines than the previous call ",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"more_above": map[string]interface{}{
							"type":        "integer",
							"description": rangeDescription("above", maxPerRequestUp),
							"minimum":     1,
							"maximum":     maxPerRequestUp,
						},
						"more_below": map[string]interface{}{
							"type":        "integer",
							"description": rangeDescription("below", maxPerRequestDown),
							"minimum":     1,
							"maximum":     maxPerRequestDown,
						},
					},
					"required": []string{"more_above", "more_below"},
				},
			},
		},
	}
}

// rangeDescription matches the Python f-string:
//   f"How many ADDITIONAL lines to expand {direction} ... (1-{cap}). Must increase each call."
func rangeDescription(direction string, cap int) string {
	return "How many ADDITIONAL lines to expand " + direction +
		" beyond your current view (1-" + strconv.Itoa(cap) +
		"). Must increase each call."
}
