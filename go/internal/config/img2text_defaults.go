package config

// applyImg2TextOverrides migrates img2text-block tuning onto the legacy
// Options block so the rest of the code keeps reading cfg.Options.
// Old configs that only set options: keys keep working; a non-zero
// (or non-empty) img2text value wins.
func applyImg2TextOverrides(cfg *Config) {
	o := cfg.Img2Text
	if o.Concurrency > 0 {
		cfg.Options.Concurrency = o.Concurrency
	}
	if o.OutputLanguage != "" {
		cfg.Options.OutputLanguage = o.OutputLanguage
	}
	if o.MaxContextLinesUp > 0 {
		cfg.Options.MaxContextLinesUp = o.MaxContextLinesUp
	}
	if o.MaxContextLinesDown > 0 {
		cfg.Options.MaxContextLinesDown = o.MaxContextLinesDown
	}
	if o.MaxWindowUp > 0 {
		cfg.Options.MaxWindowUp = o.MaxWindowUp
	}
	if o.MaxWindowDown > 0 {
		cfg.Options.MaxWindowDown = o.MaxWindowDown
	}
	if o.MaxRetries > 0 {
		cfg.Options.MaxRetries = o.MaxRetries
	}
	if o.APITimeout > 0 {
		cfg.Options.APITimeout = o.APITimeout
	}
	if o.APIConnectTimeout > 0 {
		cfg.Options.APIConnectTimeout = o.APIConnectTimeout
	}
	if o.APIMaxRetries > 0 {
		cfg.Options.APIMaxRetries = o.APIMaxRetries
	}
	if o.RateLimitRetries > 0 {
		cfg.Options.RateLimitRetries = o.RateLimitRetries
	}
	if o.FormatFixAttempts > 0 {
		cfg.Options.FormatFixAttempts = o.FormatFixAttempts
	}
	if o.MermaidValidation != "" {
		cfg.Options.MermaidValidation = o.MermaidValidation
	}
	if o.MermaidCommand != "" {
		cfg.Options.MermaidCommand = o.MermaidCommand
	}
	if o.MermaidFixAttempts != nil {
		cfg.Options.MermaidFixAttempts = o.MermaidFixAttempts
	}
	if o.MermaidTimeout > 0 {
		cfg.Options.MermaidTimeout = o.MermaidTimeout
	}
	if o.TikzValidation != "" {
		cfg.Options.TikzValidation = o.TikzValidation
	}
	if o.TikzEngine != "" {
		cfg.Options.TikzEngine = o.TikzEngine
	}
	if o.MaxTokens > 0 {
		cfg.Options.MaxTokens = o.MaxTokens
	}
	if o.Temperature > 0 {
		cfg.Options.Temperature = o.Temperature
	}
}
