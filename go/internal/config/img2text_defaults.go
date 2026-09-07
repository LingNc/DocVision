package config

// applyImg2TextOverrides migrates img2text-block pipeline tuning (model-
// level bits such as concurrency/output_language) onto the Options
// block so the rest of the code keeps reading cfg.Options. Tool
// tunables now live in tools: and are mapped in setDefaults.
func applyImg2TextOverrides(cfg *Config) {
	o := cfg.Img2Text
	if o.Concurrency > 0 {
		cfg.Options.Concurrency = o.Concurrency
	}
	if o.OutputLanguage != "" {
		cfg.Options.OutputLanguage = o.OutputLanguage
	}
	if o.FormatFixAttempts > 0 {
		cfg.Options.FormatFixAttempts = o.FormatFixAttempts
	}
	if o.MaxTokens > 0 {
		cfg.Options.MaxTokens = o.MaxTokens
	}
	if o.Temperature > 0 {
		cfg.Options.Temperature = o.Temperature
	}
}
