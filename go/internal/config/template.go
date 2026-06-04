package config

import _ "embed"

// ConfigTemplate is the default config.yaml content written by
// `./docvision init`. It is embedded from default.yaml at build
// time, so the binary has no runtime file dependency. Users must
// replace the placeholder token and API key before running the
// workflow.
//
//go:embed default.yaml
var ConfigTemplate string
