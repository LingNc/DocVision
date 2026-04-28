#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
[旧版] Image-to-Text Converter - 简单文本匹配方式
"""

import os, sys, re, json, base64, time, traceback
from pathlib import Path
from io import BytesIO
from concurrent.futures import ThreadPoolExecutor, as_completed
from threading import Lock

import yaml
from openai import OpenAI
from PIL import Image

print_lock = Lock()

def ts():
    return time.strftime("%H:%M:%S")

def log(*args):
    with print_lock:
        print(f"[{ts()}]", *args, flush=True)

def load_config(path="config.yaml"):
    with open(path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)

def image_to_base64(image_path: Path, max_size=1280) -> str:
    img = Image.open(image_path)
    if img.mode == "RGBA":
        img = img.convert("RGB")
    w, h = img.size
    if w > max_size or h > max_size:
        ratio = min(max_size / w, max_size / h)
        img = img.resize((int(w * ratio), int(h * ratio)), Image.LANCZOS)
    buf = BytesIO()
    img.save(buf, format="JPEG", quality=85)
    return base64.b64encode(buf.getvalue()).decode("utf-8")

IMAGE_RE = re.compile(r'!\[.*?\]\((images/[^)]+\.(?:jpg|jpeg|png|gif|webp))\)')

def get_context(lines, img_idx, up=20, down=20):
    start = max(0, img_idx - up)
    end = min(len(lines), img_idx + down + 1)
    ctx = []
    for i in range(start, end):
        if i == img_idx:
            ctx.append(f">>> [IMAGE at line {i}] <<<")
        else:
            ctx.append(f"[L{i}] {lines[i]}")
    return "\n".join(ctx)

SYSTEM_PROMPT = """You are a professional document analyst. Convert images in a technical markdown document into lossless text descriptions.

## Rules:
1. **Identify image type first**: Table, Flowchart, Gantt Chart, Architecture/Network Diagram, Graph/Chart (bar, line, pie), Formula, Code screenshot, or Simple text/illustration.
2. **Mermaid** for: flowcharts, gantt charts, sequence diagrams, class diagrams, state diagrams, ER diagrams, mind maps, timeline, Sankey, pie charts. Output as ```mermaid code block.
3. **Markdown table** for tabular data: include ALL rows and columns.
4. **LaTeX** for mathematical formulas: $$...$$ for block, $...$ for inline.
5. **Structured text** for architecture diagrams that cannot fit Mermaid: use bullet points / tree structure, preserve ALL labels, arrows, relationships.
6. **Code block** for code screenshots.
7. **Graph description**: list key data points, max/min values, trends for bar/line charts.
8. **Be EXHAUSTIVE**: include every visible text, number, label, value.

## Output format:
[IMG_TYPE: <type>]
<your description / mermaid / table / latex here>

If you CANNOT determine content due to insufficient context, respond EXACTLY:
[NEED_CONTEXT: up=N down=N]
where N (1-50) is additional context lines needed.
"""

def call_ai(client, model, img_b64, ctx_text, max_tokens, max_api_retries=5):
    last_error = None
    for retry in range(max_api_retries):
        try:
            response = client.chat.completions.create(
                model=model,
                messages=[
                    {"role": "system", "content": SYSTEM_PROMPT},
                    {"role": "user", "content": [
                        {"type": "text", "text": f"Analyze the image. Surrounding context:\n\n```\n{ctx_text}\n```"},
                        {"type": "image_url", "image_url": {"url": f"data:image/jpeg;base64,{img_b64}"}}
                    ]}
                ],
                temperature=0.3,
                max_tokens=max_tokens
            )
            return response.choices[0].message.content
        except Exception as e:
            last_error = e
            err_msg = str(e)
            if "429" in err_msg or "rate" in err_msg.lower():
                wait = min(2 ** retry * 2, 30)
                time.sleep(wait)
            elif retry < max_api_retries - 1:
                time.sleep(2)
    raise last_error

def process_one_image(client, model, images_dir, img_path_str, lines, img_line_idx,
                       max_retries=3, up=20, down=20, max_tokens=65536):
    img_file = images_dir / Path(img_path_str).name
    if not img_file.exists():
        return f"[IMG_MISSING: {img_path_str}]"
    try:
        img_b64 = image_to_base64(img_file)
    except Exception as e:
        return f"[IMG_ERROR: {img_path_str} - {e}]"

    cur_up, cur_down = up, down
    for attempt in range(max_retries + 1):
        ctx = get_context(lines, img_line_idx, cur_up, cur_down)
        result = call_ai(client, model, img_b64, ctx, max_tokens)
        need_match = re.match(r'\[NEED_CONTEXT:\s*up=(\d+)\s*down=(\d+)\]', result.strip() if result else "")
        if need_match and attempt < max_retries:
            ask_up = min(max(int(need_match.group(1)), 1), 80)
            ask_down = min(max(int(need_match.group(2)), 1), 80)
            cur_up = ask_up
            cur_down = ask_down
            continue
        return result.strip() if result else "[IMG_EMPTY_RESPONSE]"
    return (result or "").strip()

def load_progress(progress_file):
    if Path(progress_file).exists():
        with open(progress_file, "r", encoding="utf-8") as f:
            return json.load(f)
    return {}

def save_progress(progress_file, data):
    with open(progress_file, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

def main():
    config = load_config()
    base_url   = config["api"]["base_url"]
    api_key    = config["api"]["api_key"]
    model      = config["api"]["model"]
    max_retries = config["options"]["max_retries"]
    def_up     = config["options"]["max_context_lines_up"]
    def_down   = config["options"]["max_context_lines_down"]
    max_tokens = config["options"]["max_tokens"]
    input_dir  = Path(config["paths"]["input_dir"])
    images_dir = Path(config["paths"]["images_dir"])
    output_dir = Path(config["paths"]["output_dir"])

    output_dir.mkdir(parents=True, exist_ok=True)
    (output_dir / "images").mkdir(parents=True, exist_ok=True)

    progress_file = str(output_dir / "_progress.json")
    progress = load_progress(progress_file)

    client = OpenAI(base_url=base_url, api_key=api_key)

    md_files = sorted(input_dir.glob("*.md"))
    log(f"Found {len(md_files)} file(s) | Model: {model} | MaxOutput: {max_tokens}")
    log(f"Threads: 5 | ContextRetries: {max_retries}")
    log("=" * 60)

    all_tasks = []
    md_lines_cache = {}

    for md_file in md_files:
        content = md_file.read_text(encoding="utf-8")
        lines = content.split("\n")
        md_lines_cache[md_file.name] = (content, lines)
        matches = list(IMAGE_RE.finditer(content))
        log(f"  {md_file.name}: {len(matches)} images")
        if not matches:
            dest = output_dir / md_file.name
            if not dest.exists() or md_file.stat().st_mtime > dest.stat().st_mtime:
                dest.write_text(content, encoding="utf-8")
            continue
        line_starts = [0]
        for line in lines:
            line_starts.append(line_starts[-1] + len(line) + 1)
        for m in matches:
            pos = m.start()
            img_line = 0
            for i, s in enumerate(line_starts):
                if i < len(lines) and s <= pos < line_starts[i + 1]:
                    img_line = i
                    break
            key = f"{md_file.name}::{m.group(1)}"
            all_tasks.append((key, md_file.name, m.group(1), img_line, m.start(), m.end()))

    log(f"Total images: {len(all_tasks)}")

    # LIMIT: only process first 10 images for testing
    pending = [t for t in all_tasks if t[0] not in progress][:10]
    log(f"Done: {len(all_tasks) - len(pending)} | Test limit: {len(pending)} (max 10)")

    if not pending:
        log("All images already processed!")
    else:
        def worker(task):
            key, md_name, img_path, img_line, mstart, mend = task
            lines = md_lines_cache[md_name][1]
            result = process_one_image(
                client, model, images_dir, img_path, lines, img_line,
                max_retries, def_up, def_down, max_tokens
            )
            return key, result, mstart, mend, img_path

        done_count = 0
        with ThreadPoolExecutor(max_workers=3) as executor:
            futures = {executor.submit(worker, t): t for t in pending}
            for future in as_completed(futures):
                try:
                    key, result, mstart, mend, img_path = future.result()
                    progress[key] = {"result": result, "start": mstart, "end": mend, "img_path": img_path}
                    done_count += 1
                    save_progress(progress_file, progress)
                    log(f"  [{done_count}/{len(pending)}] {img_path}")
                    log(f"    => {result[:120]}...")
                except Exception as e:
                    log(f"  ERROR: {e}")
                    traceback.print_exc()

    # Apply replacements
    log("Writing final markdown files...")
    for md_file in md_files:
        content, lines = md_lines_cache[md_file.name]
        replacements = []
        for task_key, info in progress.items():
            if task_key.startswith(md_file.name + "::"):
                replacements.append((info["start"], info["end"], info["result"], info["img_path"]))
        if not replacements:
            if not (output_dir / md_file.name).exists():
                (output_dir / md_file.name).write_text(content, encoding="utf-8")
            continue
        new_content = content
        for start, end, result, img_path in sorted(replacements, key=lambda x: -x[0]):
            block = f"\n\n<!-- IMG: {img_path} -->\n**[Image Analysis]**\n\n{result}\n\n<!-- /IMG -->\n\n"
            new_content = new_content[:start] + block + new_content[end:]
        dest = output_dir / md_file.name
        dest.write_text(new_content, encoding="utf-8")
        log(f"  Saved: {md_file.name} ({len(replacements)} replacements)")

    log("=" * 60)
    log("Done!")

if __name__ == "__main__":
    main()
