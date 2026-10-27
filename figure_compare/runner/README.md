# Figure Compare session runner

This is the single-session Go harness recovered from the experiment history. It invokes the configured drawing model through the repository's session layer, offers workspace editing/compile/compare tools, and writes an append-only JSONL transcript. All file locations are runtime parameters; no private sample or machine path is embedded.

Run from this directory:

```bash
go run . -config <config.yaml> -original <reference-image> \
  -compare-script ../algorithms/compare.py -out <run-directory>
```

The runner exposes `view_pdf` before `compare_figure`: the model can inspect the actual compiled PDF, then receive similarity, channel evidence, and a `worst_region` percentage box. It can call `compare_figure` again with that box and a larger `zoom` to focus on the weakest area before submitting.

The default transcript is `<run-directory>/session.jsonl`. Override with `-transcript`. To continue a transcript after trimming it in the viewer, restart with the same `-out` / `-transcript` and add `-resume`; this replays the retained messages without posting a new opening user turn. Compile and compare again before submitting because in-memory gates reset on process restart. Use `-model` to select a configured alias; otherwise `latex.drawing_model` is used. `-context` optionally supplies surrounding document context for the figure session and is also returned by `image_context`; keep real document text outside the repository. `-python` selects the interpreter for the comparator. The supplied config must define the model and session settings; the runtime must also provide XeLaTeX, pdfinfo, and pdftocairo.

Open `../viewer/viewer.html`, click “选择工作目录”, and choose the parent directory containing the run subdirectories. The viewer recursively lists `session.jsonl` files; select one session to inspect its transcript, `media/` thumbnails, and `compare/round_*` evidence. Do not commit actual config, image, JSONL, or run outputs.
