package config

// ConfigTemplate is the default config.yaml content written by
// `./docvision init`. It mirrors config.example.yaml with helpful
// comments. Users must replace the placeholder token and API key
// before running the workflow.
const ConfigTemplate = `# DocVision Configuration
# Copy this file to config.yaml and fill in your credentials.

# MinerU API Configuration
mineru:
  api_base_url: "https://mineru.net/api/v4"
  token: "YOUR-MINERU-TOKEN-HERE"
  model_version: "vlm"         # pipeline, vlm, or MinerU-HTML
  is_ocr: true                 # enable OCR for best results
  enable_formula: true
  enable_table: true
  language: "ch"               # ch, en, japan, korean, etc.
  poll_interval: 3             # seconds between API polling attempts
  log_poll_interval: 3         # seconds between console log output (exponential backoff starts here)
  poll_timeout: 0              # max seconds to wait for task completion (0 = unlimited)
  progress_threshold: 80       # page count delta that triggers an immediate progress log
  max_pages_per_part: 200      # max pages per PDF part (MinerU limit)
  max_size_mb: 200             # max file size in MB per PDF part (MinerU limit)
  max_concurrent: 5            # number of concurrent MinerU tasks
  upload_timeout: 300          # upload idle timeout in seconds (no timeout while data is flowing)

# AI Image-to-Text config (OpenAI-compatible)
ai:
  base_url: "https://your-api-proxy.com/v1"
  api_key: "sk-YOUR-API-KEY-HERE"
  model: "Qwen/Qwen3.6-27B"
  enable_thinking: false       # pass as top-level param to disable thinking mode (Qwen3.6+)

# Processing options
options:
  max_context_lines_up: 10     # default: initial lines above image
  max_context_lines_down: 5    # default: initial lines below image
  max_window_up: 50            # maximum total lines above (cap)
  max_window_down: 50          # maximum total lines below (cap)
  max_retries: 5               # max retries for AI to request more context
  api_timeout: 400             # request timeout in seconds for the OpenAI call
  api_connect_timeout: 60      # connect timeout in seconds
  api_max_retries: 3           # API retry count for non-rate-limit errors
  rate_limit_retries: 0        # 0 means unlimited with a safety cap in code
  concurrency: 10              # number of concurrent worker threads
  temperature: 0.10
  output_language: "Chinese"   # "Chinese" or "English"
  format_fix_attempts: 1       # 0=disable, 1=one retry for missing [IMG_TYPE:]
  max_tokens: 65536

# Paths
paths:
  input_dir: "./files"              # dir with source files
  split_dir: "./split_files"        # dir for split PDF parts
  mineru_output: "./mineru_output"  # dir for MinerU API results
  output_dir: "./output"            # dir with merged .md files
  images_dir: "./output/images"
  finally_dir: "./finally"          # final output directory
`
