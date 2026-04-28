#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
[v3 final] Image-to-Text Converter - Proper OpenAI tool_call
- 默认上下文: 上3行, 下10行 (configurable)
- AI 调用 get_more_context 工具时返回真实的文档行内容
- 测试模式: 最多 10 张
"""

import os, re, json, base64, time, traceback
from pathlib import Path
from io import BytesIO
from concurrent.futures import ThreadPoolExecutor, as_completed
from threading import Lock

import yaml
from openai import OpenAI
from PIL import Image

print_lock = Lock()
_g_up, _g_down = 3, 10  # global defaults, set by main()

def ts(): return time.strftime("%H:%M:%S")
def log(*args):
    with print_lock: print(f"[{ts()}]", *args, flush=True)

# ─── Config ────────────────────────────────────────────────────────
def load_config(path="config.yaml"):
    with open(path, "r", encoding="utf-8") as f: return yaml.safe_load(f)

# ─── Image ─────────────────────────────────────────────────────────
def image_to_base64(image_path: Path, max_size=1280) -> str:
    img = Image.open(image_path)
    if img.mode == "RGBA": img = img.convert("RGB")
    w, h = img.size
    if w > max_size or h > max_size:
        r = min(max_size / w, max_size / h)
        img = img.resize((int(w * r), int(h * r)), Image.LANCZOS)
    buf = BytesIO()
    img.save(buf, format="JPEG", quality=85)
    return base64.b64encode(buf.getvalue()).decode("utf-8")

IMAGE_RE = re.compile(r'!\[.*?\]\((images/[^)]+\.(?:jpg|jpeg|png|gif|webp))\)')

# ─── Context ────────────────────────────────────────────────────────
def get_context_lines(lines, img_idx, up, down):
    start = max(0, img_idx - up)
    end = min(len(lines), img_idx + down + 1)
    parts = []
    for i in range(start, end):
        prefix = ">>>[IMG] " if i == img_idx else f"[L{i}] "
        parts.append(prefix + lines[i])
    return parts, start, end

# ─── Tool definition ───────────────────────────────────────────────
TOOLS = [{
    "type": "function",
    "function": {
        "name": "get_more_context",
        "description": "Request additional surrounding lines from the markdown document to better understand the image. Call when the current context is insufficient.",
        "parameters": {
            "type": "object",
            "properties": {
                "lines_above": {
                    "type": "integer",
                    "description": "How many lines ABOVE the image you need (0-80). Specify the TOTAL lines you want, including the default 3.",
                    "minimum": 0, "maximum": 80
                },
                "lines_below": {
                    "type": "integer",
                    "description": "How many lines BELOW the image you need (0-80). Specify the TOTAL lines you want, including the default 10.",
                    "minimum": 0, "maximum": 80
                }
            },
            "required": ["lines_above", "lines_below"]
        }
    }
}]

# ─── System Prompt ─────────────────────────────────────────────────
SYSTEM_PROMPT = """You are a professional document analyst. Convert images in a technical Chinese markdown document into lossless text descriptions.

## Rules:
1. **Identify image type first**: Table, Flowchart, Gantt Chart, Architecture/Network Diagram, Graph/Chart (bar, line, pie), Formula, Code screenshot, or Simple text/illustration.
2. **Mermaid** for: flowcharts, gantt charts, sequence diagrams, class diagrams, state diagrams, ER diagrams, mind maps, timeline, Sankey. Output as ```mermaid code block.
3. **Markdown table** for tabular data: include ALL rows/columns.
4. **LaTeX** for formulas: $$...$$ for block, $...$ for inline.
5. **Structured text** for diagrams that cannot be Mermaid: preserve ALL labels, arrows, relationships.
6. **Code block** for code screenshots.
7. **Graph description**: key data points, max/min, trends for charts.
8. **Be EXHAUSTIVE**: every visible text, number, label. No summary.

## Output:
[IMG_TYPE: <type>]
<description / mermaid / table / latex>

## Tool: get_more_context
Currently you are given 3 lines above and 10 lines below the image. If you need more context to understand the image, call get_more_context(lines_above=N, lines_below=M) where N/M are the TOTAL lines you want (not additional). You may call this up to 3 times.
"""

# ─── Core: AI call with real tool_call ─────────────────────────────
def call_ai_with_tools(client, model, img_b64, lines, img_line_idx, max_tokens, max_tool_rounds=3, max_api_retries=5):
    cur_up, cur_down = _g_up, _g_down
    parts, start, end = get_context_lines(lines, img_line_idx, cur_up, cur_down)
    ctx_text = "\n".join(parts)

    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": [
            {"type": "text", "text": f"Analyze this image from a technical textbook. You have {cur_up} lines above and {cur_down} lines below (total {len(parts)} lines).\n\nContext:\n```\n{ctx_text}\n```\n\nIf the context is not enough, call get_more_context with the TOTAL lines you want."},
            {"type": "image_url", "image_url": {"url": f"data:image/jpeg;base64,{img_b64}"}}
        ]}
    ]

    for round_num in range(max_tool_rounds + 1):
        for retry in range(max_api_retries):
            try:
                response = client.chat.completions.create(
                    model=model,
                    messages=messages,
                    tools=TOOLS if round_num < max_tool_rounds else None,
                    tool_choice="auto" if round_num < max_tool_rounds else "none",
                    temperature=0.3, max_tokens=max_tokens
                )
                break
            except Exception as e:
                err = str(e)
                if "429" in err or "rate" in err.lower():
                    time.sleep(min(2**retry * 2, 30))
                elif retry < max_api_retries - 1:
                    time.sleep(2)
                else: raise

        choice = response.choices[0]

        if choice.message.tool_calls:
            # Record assistant's tool call
            tc_list = [{"id": tc.id, "type": "function",
                         "function": {"name": tc.function.name, "arguments": tc.function.arguments}}
                        for tc in choice.message.tool_calls]
            messages.append({"role": "assistant", "content": choice.message.content or "", "tool_calls": tc_list})

            for tc in choice.message.tool_calls:
                if tc.function.name == "get_more_context":
                    args = json.loads(tc.function.arguments)
                    ask_up = args.get("lines_above", cur_up)
                    ask_down = args.get("lines_below", cur_down)

                    # Use the MAX of current and requested
                    new_up = max(cur_up, ask_up)
                    new_down = max(cur_down, ask_down)

                    log(f"    [ToolCall] AI wants total up={ask_up}/down={ask_down} → window={new_up}/{new_down}")

                    parts2, s2, e2 = get_context_lines(lines, img_line_idx, new_up, new_down)
                    result_text = f"Context expanded. Now {new_up} lines above, {new_down} below:\n\n" + "\n".join(parts2)

                    cur_up, cur_down = new_up, new_down
                    messages.append({"role": "tool", "tool_call_id": tc.id, "content": result_text})

            continue  # next round
        else:
            return choice.message.content or "[IMG_EMPTY_RESPONSE]"

    # Forced final
    messages.append({"role": "user", "content": "Provide your best analysis now."})
    try:
        r = client.chat.completions.create(model=model, messages=messages, temperature=0.3, max_tokens=max_tokens)
        return r.choices[0].message.content or "[IMG_EMPTY_RESPONSE]"
    except:
        return "[IMG_ERROR]"

# ─── Process one image ─────────────────────────────────────────────
def process_one_image(client, model, images_dir, img_path_str, lines, img_line_idx,
                       max_tool_rounds=3, max_tokens=65536):
    img_file = images_dir / Path(img_path_str).name
    if not img_file.exists(): return f"[IMG_MISSING: {img_path_str}]"
    try: img_b64 = image_to_base64(img_file)
    except Exception as e: return f"[IMG_ERROR: {img_path_str} - {e}]"
    return (call_ai_with_tools(client, model, img_b64, lines, img_line_idx, max_tokens, max_tool_rounds) or "").strip()

# ─── Progress ──────────────────────────────────────────────────────
def load_progress(pf):
    if Path(pf).exists():
        with open(pf, "r", encoding="utf-8") as f: return json.load(f)
    return {}

def save_progress(pf, data):
    with open(pf, "w", encoding="utf-8") as f: json.dump(data, f, ensure_ascii=False, indent=2)

# ─── Main (test mode: max 10) ─────────────────────────────────────
def main():
    global _g_up, _g_down
    config = load_config()
    base_url = config["api"]["base_url"]
    api_key  = config["api"]["api_key"]
    model    = config["api"]["model"]
    max_ret = config["options"]["max_retries"]
    _g_up   = config["options"]["max_context_lines_up"]
    _g_down = config["options"]["max_context_lines_down"]
    max_tok = config["options"]["max_tokens"]
    indir   = Path(config["paths"]["input_dir"])
    imgdir  = Path(config["paths"]["images_dir"])
    outdir  = Path(config["paths"]["output_dir"])

    outdir.mkdir(parents=True, exist_ok=True)
    (outdir / "images").mkdir(parents=True, exist_ok=True)

    pf = str(outdir / "_progress.json")
    progress = load_progress(pf)
    client = OpenAI(base_url=base_url, api_key=api_key)

    md_files = sorted(indir.glob("*.md"))
    log(f"Found {len(md_files)} file(s) | Model: {model} | Default ctx: up={_g_up} down={_g_down}")
    log(f"Max tool rounds: {max_ret} | TEST MODE: max 10 images")
    log("=" * 60)

    all_tasks = []
    md_cache = {}
    for mdf in md_files:
        content = mdf.read_text(encoding="utf-8")
        lines = content.split("\n")
        md_cache[mdf.name] = (content, lines)
        matches = list(IMAGE_RE.finditer(content))
        log(f"  {mdf.name}: {len(matches)} images")
        if not matches: continue
        lstarts = [0]
        for line in lines: lstarts.append(lstarts[-1] + len(line) + 1)
        for m in matches:
            pos = m.start(); il = 0
            for i, s in enumerate(lstarts):
                if i < len(lines) and s <= pos < lstarts[i + 1]: il = i; break
            all_tasks.append((f"{mdf.name}::{m.group(1)}", mdf.name, m.group(1), il, m.start(), m.end()))

    log(f"Total available: {len(all_tasks)}")
    pending = [t for t in all_tasks if t[0] not in progress][:10]
    log(f"Done: {len(all_tasks)-len(pending)} | Test: {len(pending)}")
    log("=" * 60)

    if not pending:
        log("All done! Clear _progress.json to re-run.")
    else:
        def worker(t):
            key, mn, ip, il, ms, me = t
            lines = md_cache[mn][1]
            r = process_one_image(client, model, imgdir, ip, lines, il, max_ret, max_tok)
            return key, r, ms, me, ip

        done = 0
        with ThreadPoolExecutor(max_workers=3) as ex:
            fs = {ex.submit(worker, t): t for t in pending}
            for f in as_completed(fs):
                try:
                    key, result, ms, me, ip = f.result()
                    progress[key] = {"result": result, "start": ms, "end": me, "img_path": ip}
                    done += 1; save_progress(pf, progress)
                    log("-" * 50)
                    log(f"[{done}/{len(pending)}] {ip}")
                    log(f"RESULT:\n{result[:500]}")
                    log("-" * 50)
                except Exception as e:
                    log(f"  ERROR: {e}"); traceback.print_exc()

    log("\nWriting final files...")
    for mdf in md_files:
        content, lines = md_cache[mdf.name]
        reps = []
        for tk, info in progress.items():
            if tk.startswith(mdf.name + "::"): reps.append((info["start"], info["end"], info["result"], info["img_path"]))
        if not reps:
            if not (outdir / mdf.name).exists(): (outdir / mdf.name).write_text(content, encoding="utf-8")
            continue
        nc = content
        for s, e, result, ip in sorted(reps, key=lambda x: -x[0]):
            nc = nc[:s] + f"\n\n<!-- IMG: {ip} -->\n**[AI Analysis]**\n\n{result}\n\n<!-- /IMG -->\n\n" + nc[e:]
        (outdir / mdf.name).write_text(nc, encoding="utf-8")
        log(f"  Saved: {mdf.name} ({len(reps)} replacements)")

    log("=" * 60)
    log("Done!")

if __name__ == "__main__":
    main()
