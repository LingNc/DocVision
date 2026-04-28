#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
[v5] Image-to-Text Converter - Incremental context + CLI test mode
Usage:
  python img2text.py                          # full batch run
  python img2text.py --test                   # test: random 10 images (default)
  python img2text.py --test --number 5        # test: random 5 images
  python img2text.py --test --seed 42         # test: random with fixed seed 42
  python img2text.py --test --seed random     # test: random with random seed (logged)
"""

import os, re, json, base64, time, traceback, random, argparse
from pathlib import Path
from io import BytesIO
from concurrent.futures import ThreadPoolExecutor, as_completed
from threading import Lock, Thread
from queue import Queue

import httpx
import yaml
from openai import OpenAI
from PIL import Image

print_lock = Lock()
_g_up, _g_down, _g_max_up, _g_max_down = 3, 10, 50, 50

def ts(): return time.strftime("%H:%M:%S")
def log(*args):
    with print_lock: print(f"[{ts()}]", *args, flush=True)

def load_config(path="config.yaml"):
    with open(path, "r", encoding="utf-8") as f: return yaml.safe_load(f)

def image_to_base64(image_path: Path, max_size=1280) -> str:
    img = Image.open(image_path)
    if img.mode == "RGBA": img = img.convert("RGB")
    w, h = img.size
    if w > max_size or h > max_size:
        r = min(max_size / w, max_size / h)
        img = img.resize((int(w * r), int(h * r)), Image.LANCZOS)
    buf = BytesIO(); img.save(buf, format="JPEG", quality=85)
    return base64.b64encode(buf.getvalue()).decode("utf-8")

IMAGE_RE = re.compile(r'!\[.*?\]\((images/[^)]+\.(?:jpg|jpeg|png|gif|webp))\)')

# ─── Context: return specific line range ───────────────────────────
def get_context_lines(lines, img_idx, up, down):
    """Return lines in [img_idx-up, img_idx+down] with line number prefixes."""
    start = max(0, img_idx - up)
    end = min(len(lines), img_idx + down + 1)
    parts = []
    for i in range(start, end):
        prefix = ">>>[IMG] " if i == img_idx else f"[L{i}] "
        parts.append(prefix + lines[i])
    return parts, start, end

def get_delta_lines(lines, img_idx, old_up, old_down, new_up, new_down):
    """Return ONLY the new lines that were added when expanding from (old_up,old_down) to (new_up,new_down)."""
    old_start = max(0, img_idx - old_up)
    old_end = min(len(lines), img_idx + old_down + 1)
    new_start = max(0, img_idx - new_up)
    new_end = min(len(lines), img_idx + new_down + 1)

    parts = []
    # Lines added above
    if new_start < old_start:
        for i in range(new_start, old_start):
            parts.append(f"[L{i}] {lines[i]}")
    # Lines added below
    if new_end > old_end:
        for i in range(old_end, new_end):
            parts.append(f"[L{i}] {lines[i]}")
    return parts

# ─── Tool builder: max per-request from config ──────────────────────
def build_tools(max_per_request_up, max_per_request_down):
    return [{
        "type": "function",
        "function": {
            "name": "get_more_context",
            "description": (
                "Request ADDITIONAL lines above or below the image. "
                "You will receive ONLY the NEW lines (not previously seen content). "
                "Use this when the current context is insufficient to understand the image."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "more_above": {
                        "type": "integer",
                        "description": f"How many ADDITIONAL lines to expand UPWARD beyond your current view (0-{max_per_request_up}).",
                        "minimum": 1, "maximum": max_per_request_up
                    },
                    "more_below": {
                        "type": "integer",
                        "description": f"How many ADDITIONAL lines to expand DOWNWARD beyond your current view (0-{max_per_request_down}).",
                        "minimum": 1, "maximum": max_per_request_down
                    }
                },
                "required": ["more_above", "more_below"]
            }
        }
    }]

# ─── System Prompt ─────────────────────────────────────────────────
SYSTEM_PROMPT = """You are a document image analyst. Describe images from  technical Chinese textbook as structured, machine-readable content.

## Priority: Correctness > Completeness > Conciseness
First ensure you understand the image content. Then ensure correctness by verifying image content (labels, arrows, values) against surrounding text. If window insufficient or content is unclear, call get_more_context. Then be exhaustive (every visible element). Finally trim redundancy.

## Core rule: describe WHAT is visible, never WHY/HOW.
If ambiguous or overly complex, call get_more_context to resolve; if still unclear, mark [?] and describe only what is certain. No guessing.

## Rules:
1. **Identify image type**: Table, Flowchart, Gantt Chart, Architecture/Network Diagram, Graph/Chart, Formula, Code screenshot, or Simple illustration.
2. **Mermaid** for flowcharts, Gantt charts, sequence diagrams, class diagrams, state diagrams, ER diagrams, mind maps, timeline, Sankey, pie charts, quadrant charts, requirement diagrams. Use ```mermaid code block.
3. **Markdown table** for tabular data: ALL rows and columns exactly as shown.
4. **LaTeX** for formulas: $$...$$ block or $...$ inline.
5. **Structured text** for diagrams not suitable for Mermaid: preserve ALL labels, arrows, relationships shown.
6. **Code block** for code screenshots.
7. **Graph description**: key data points, max/min, trends for charts.
8. **Be EXHAUSTIVE**: every visible text, number, label. No summary.

## Output format:
[IMG_TYPE: <type>]
<description / mermaid / table / latex>

## CRITICAL
Start your response DIRECTLY with "[IMG_TYPE:" (no extra text before).

## Tool: get_more_context
Start with a small context window. To get more, call get_more_context(more_above=N, more_below=M) — N and M are additional lines.
- more_above: additional lines to fetch above current view.
- more_below: additional lines to fetch below current view.
You receive only the delta. Max 3 calls.If uncertain how many lines you need, request half of the maximum ({half_up} for upward, {half_down} for downward). Never request 0.
"""

# ─── Core: AI call with INCREMENTAL tool_call ──────────────────────
def call_ai_with_tools(client, model, img_b64, lines, img_line_idx, max_tokens,
                       max_tool_rounds=3, max_api_retries=3, rate_limit_retries=0,
                       enable_thinking=None, temperature=0.3):
    """
    Call AI API with incremental context expansion.
    Uses per-request cap from global config (_g_max_up, _g_max_down).
    No cumulative cap across rounds — AI can keep expanding 3 times.
    """
    cur_up, cur_down = _g_up, _g_down
    max_per_up, max_per_down = _g_max_up, _g_max_down

    # 动态计算一半的最大值
    half_up = max_per_up // 2
    half_down = max_per_down // 2
    dynamic_prompt = SYSTEM_PROMPT.format(half_up=half_up, half_down=half_down)

    # Build tools dynamically from current config
    tools = build_tools(max_per_up, max_per_down)

    # Initial context
    parts, start, end = get_context_lines(lines, img_line_idx, cur_up, cur_down)
    ctx_text = "\n".join(parts)

    messages = [
        {"role": "system", "content": dynamic_prompt},
        {"role": "user", "content": [
            {"type": "text", "text": (
                f"The image to describe is at line {img_line_idx}. "
                f"Context: [{img_line_idx - cur_up} to {img_line_idx + cur_down}] ({cur_up}↑, {cur_down}↓).\n"
                f"```\n{ctx_text}\n```\n"
                f"Understand image before output. If confused, call get_more_context(↑N, ↓M) — max per call: ↑{max_per_up}, ↓{max_per_down}."
            )},
            {"type": "image_url", "image_url": {"url": f"data:image/jpeg;base64,{img_b64}"}}
        ]}
    ]

    rate_limit_limit = 100 if rate_limit_retries == 0 else rate_limit_retries

    for round_num in range(max_tool_rounds + 1):
        retry = 0
        rate_limit_retry = 0
        while True:
            try:
                extra_body = {}
                if enable_thinking is not None:
                    extra_body["enable_thinking"] = enable_thinking
                response = client.chat.completions.create(
                    model=model,
                    messages=messages,
                    tools=tools if round_num < max_tool_rounds else None,
                    tool_choice="auto" if round_num < max_tool_rounds else "none",
                    temperature=temperature,
                    max_tokens=max_tokens,
                    extra_body=extra_body
                )
                break
            except Exception as e:
                err = str(e)
                err_lower = err.lower()

                if "429" in err or "rate" in err_lower:
                    if rate_limit_retry < rate_limit_limit:
                        wait = min(2 ** rate_limit_retry * 2, 60)
                        time.sleep(wait)
                        rate_limit_retry += 1
                        continue
                    return "[IMG_RATE_LIMIT_EXCEEDED]"

                if any(kw in err_lower for kw in ("connect", "timeout", "handshake", "timed out")):
                    if retry < max_api_retries:
                        wait = min(2 ** retry * 5, 60)
                        time.sleep(wait)
                        retry += 1
                        continue
                    return "[IMG_CONNECTION_TIMEOUT]"

                if retry < max_api_retries:
                    time.sleep(2)
                    retry += 1
                    continue

                return f"[IMG_API_ERROR: {err[:200]}]"

        choice = response.choices[0]

        if choice.message.tool_calls:
            tc_list = [{"id": tc.id, "type": "function",
                         "function": {"name": tc.function.name, "arguments": tc.function.arguments}}
                        for tc in choice.message.tool_calls]
            messages.append({"role": "assistant", "content": choice.message.content or "", "tool_calls": tc_list})

            for tc in choice.message.tool_calls:
                if tc.function.name == "get_more_context":
                    args = json.loads(tc.function.arguments)
                    more_up = args.get("more_above", 0)
                    more_down = args.get("more_below", 0)

                    # if more_up == 0 and more_down == 0:
                    #     log(f"    [ToolCall] AI requested 0 expansion, ignoring")
                    #     result_text = (
                    #         "Invalid: must request >=1 line. Please proceed directly."
                    #     )
                    #     messages.append({"role": "tool", "tool_call_id": tc.id, "content": result_text})
                    #     continue

                    # Clamp each REQUEST to per-request max; no cumulative cap
                    actual_up = min(more_up, max_per_up)
                    actual_down = min(more_down, max_per_down)
                    new_up = cur_up + actual_up
                    new_down = cur_down + actual_down

                    log(f"    [ToolCall] AI wants +{more_up}up/-{more_down}down -> "
                        f"window {cur_up}/{cur_down}>{new_up}/{new_down} "
                        f"(per-request max={max_per_up}/{max_per_down})")

                    # Get ONLY delta (new) lines
                    delta = get_delta_lines(lines, img_line_idx, cur_up, cur_down, new_up, new_down)

                    if delta:
                        result_text = (
                            f"Added {actual_up} lines above and {actual_down} lines below. "
                            f"Window is now [{img_line_idx - new_up} to {img_line_idx + new_down}].\n\n"
                            f"NEW content (delta only):\n\n" + "\n".join(delta)
                        )
                    else:
                        result_text = (
                            f"No new lines could be added. Window remains [{img_line_idx - cur_up} to {img_line_idx + cur_down}]. "
                            f"Please proceed with your best analysis."
                        )

                    cur_up, cur_down = new_up, new_down
                    messages.append({"role": "tool", "tool_call_id": tc.id, "content": result_text})

            continue  # next round
        else:
            return choice.message.content or "[IMG_EMPTY_RESPONSE]"

    # Forced final
    messages.append({"role": "user", "content": "Provide your best analysis now."})
    try:
        extra_body = {}
        if enable_thinking is not None:
            extra_body["enable_thinking"] = enable_thinking
        r = client.chat.completions.create(
            model=model,
            messages=messages,
            temperature=temperature,
            max_tokens=max_tokens,
            extra_body=extra_body
        )
        return r.choices[0].message.content or "[IMG_EMPTY_RESPONSE]"
    except:
        return "[IMG_ERROR]"

# ─── Process one image ─────────────────────────────────────────────
def process_one_image(client, model, images_dir, img_path_str, lines, img_line_idx,
                       max_tool_rounds=3, max_tokens=65536, max_api_retries=3,
                       rate_limit_retries=0, enable_thinking=None, temperature=0.3):
    img_file = images_dir / Path(img_path_str).name
    if not img_file.exists(): return f"[IMG_MISSING: {img_path_str}]"
    try: img_b64 = image_to_base64(img_file)
    except Exception as e: return f"[IMG_ERROR: {img_path_str} - {e}]"
    try:
        return (call_ai_with_tools(
            client,
            model,
            img_b64,
            lines,
            img_line_idx,
            max_tokens,
            max_tool_rounds,
            max_api_retries,
            rate_limit_retries,
            enable_thinking,
            temperature,
        ) or "").strip()
    except Exception as e:
        return f"[IMG_PROCESS_ERROR: {e}]"

# ─── Progress ──────────────────────────────────────────────────────
def load_progress(pf):
    if Path(pf).exists():
        with open(pf, "r", encoding="utf-8") as f: return json.load(f)
    return {}

def save_progress(pf, data):
    with open(pf, "w", encoding="utf-8") as f: json.dump(data, f, ensure_ascii=False, indent=2)

# ─── Main ────────────────────────────────────────────────────────
def parse_args():
    p = argparse.ArgumentParser(description="Image-to-Text Converter")
    p.add_argument("--test", action="store_true", help="Enable test mode (random sample)")
    p.add_argument("--number", type=int, default=10, help="Number of test images (default: 10)")
    p.add_argument("--seed", type=str, default=None, help="Random seed: 'random' or a number (for reproducibility)")
    return p.parse_args()

def main():
    global _g_up, _g_down, _g_max_up, _g_max_down
    args = parse_args()

    config = load_config()
    base_url = config["api"]["base_url"]
    api_key  = config["api"]["api_key"]
    model    = config["api"]["model"]
    enable_thinking = config["api"].get("enable_thinking", False)
    max_ret = config["options"]["max_retries"]
    _g_up    = config["options"]["max_context_lines_up"]
    _g_down  = config["options"]["max_context_lines_down"]
    _g_max_up  = config["options"].get("max_window_up", 50)
    _g_max_down = config["options"].get("max_window_down", 50)
    max_tok  = config["options"]["max_tokens"]
    temperature = config["options"].get("temperature", 0.3)
    api_timeout = config["options"].get("api_timeout", 400)
    api_connect_timeout = config["options"].get("api_connect_timeout", 60)
    api_max_retries = config["options"].get("api_max_retries", 3)
    rate_limit_retries = config["options"].get("rate_limit_retries", 0)
    concurrency = config["options"].get("concurrency", 3)
    indir   = Path(config["paths"]["input_dir"])
    imgdir  = Path(config["paths"]["images_dir"])
    outdir  = Path(config["paths"]["output_dir"])

    outdir.mkdir(parents=True, exist_ok=True)

    pf = str(outdir / "_progress.json")
    progress = load_progress(pf)
    client = OpenAI(
        base_url=base_url,
        api_key=api_key,
        timeout=httpx.Timeout(
            connect=api_connect_timeout,
            read=api_timeout,
            write=api_connect_timeout,
            pool=10.0,
        ),
        max_retries=0,
    )

    md_files = sorted(indir.glob("*.md"))

    mode_str = f"TEST MODE (random sample, n={args.number})" if args.test else "FULL BATCH MODE"
    log(f"Found {len(md_files)} file(s) | Model: {model} | Default ctx: up={_g_up} down={_g_down}")
    log(f"Max tool rounds: {max_ret} | Max window: up={_g_max_up} down={_g_max_down} | Concurrency: {concurrency} | {mode_str}")
    log("=" * 60)

    # Collect ALL tasks
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

    log(f"Total images available: {len(all_tasks)}")

    if args.test:
        # ── TEST MODE: random sample ──
        if args.seed:
            if args.seed.lower() == "random":
                seed = random.randint(0, 2**31 - 1)
            else:
                seed = int(args.seed)
        else:
            seed = 42  # default fixed seed for reproducibility
        random.seed(seed)
        log(f"Test seed: {seed}  (use --seed {seed} to reproduce this run)")

        # Remove already done and sample
        remaining = [t for t in all_tasks if t[0] not in progress]
        n = min(args.number, len(remaining))
        pending = random.sample(remaining, n) if n > 0 else []
        log(f"Randomly selected: {n} images (seed={seed})")
    else:
        # ── FULL MODE ──
        pending = [t for t in all_tasks if t[0] not in progress]

    log(f"Already done: {len(all_tasks) - len(pending)} | To process: {len(pending)}")
    log("=" * 60)

    if not pending:
        log("All done! Clear _progress.json to re-run.")
    else:
        def worker(t):
            try:
                key, mn, ip, il, ms, me = t
                lines = md_cache[mn][1]
                r = process_one_image(
                    client,
                    model,
                    imgdir,
                    ip,
                    lines,
                    il,
                    max_ret,
                    max_tok,
                    api_max_retries,
                    rate_limit_retries,
                    enable_thinking,
                    temperature,
                )
                return key, r, ms, me, ip
            except Exception as e:
                return t[0], f"[IMG_WORKER_FATAL: {e}]", t[4], t[5], t[2]

        # Use a producer-consumer pattern: workers put results into a queue,
        # a single writer thread takes results and writes progress/logs.
        result_queue = Queue()
        total = len(pending)

        def writer_thread():
            done_count = 0
            while done_count < total:
                key, result, ms, me, ip = result_queue.get()
                try:
                    progress[key] = {"result": result, "start": ms, "end": me, "img_path": ip}
                    done_count += 1
                    save_progress(pf, progress)
                    log("-" * 50)
                    log(f"[{done_count}/{total}] {ip} (from {key.split('::')[0]})")
                    log(f"RESULT:\n{result[:500]}")
                    log("-" * 50)
                except Exception as e:
                    log(f"  Writer error: {e}"); traceback.print_exc()
                finally:
                    result_queue.task_done()

        writer = Thread(target=writer_thread)
        writer.start()

        with ThreadPoolExecutor(max_workers=concurrency) as ex:
            fs = {ex.submit(worker, t): t for t in pending}
            for f in as_completed(fs):
                t = fs[f]
                try:
                    res = f.result()
                    result_queue.put(res)
                except Exception as e:
                    log(f"  Worker fatal: {e}"); traceback.print_exc()
                    # ensure writer can finish
                    result_queue.put((t[0], f"[IMG_SUBMIT_ERROR: {e}]", t[4], t[5], t[2]))

        # wait until writer processed all results
        result_queue.join()
        writer.join()

    # ─── Write output ──────────────────────────────────────────
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
            nc = nc[:s] + f"\n\n<!-- IMG: {ip} -->\n[AI] {result}\n\n<!-- /IMG -->\n\n" + nc[e:]
        (outdir / mdf.name).write_text(nc, encoding="utf-8")
        log(f"  Saved: {mdf.name} ({len(reps)} replacements)")

    log("=" * 60)
    log("Done!")

if __name__ == "__main__":
    main()
