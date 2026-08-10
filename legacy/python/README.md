# Legacy Python Implementation

This directory contains the historical Python implementation of DocVision.
It is preserved for reference only and is **no longer maintained**.

The current implementation is in the Go directory at the repository root
(`go/`). The Go implementation is the reference implementation; the Python
sources here are kept solely for historical context and may diverge from
current behaviour.

## Layout

- `workflow.py` – orchestrator entry point that invokes the sub-scripts in
  `python/`.
- `python/` – the original `python/` sub-scripts (split, MinerU API, organize,
  img2text, analyze, migrate_progress).
- `requirements.txt` – Python dependency manifest for the archived scripts.

## Usage (not recommended)

If you still want to run the archived scripts:

```bash
cd legacy/python
pip install -r requirements.txt
python workflow.py
```

Sub-scripts must be executed from `legacy/python/` so the relative
`python/<script>.py` paths used by `workflow.py` resolve correctly.

## Compatibility

The archived Python implementation is not advertised as feature-equivalent to
the Go implementation. Behaviour, defaults, and supported file types may
differ. For new work, please use the Go implementation.