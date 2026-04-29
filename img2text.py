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

import os, re, json, base64, time, traceback, random, argparse, signal, sys
from pathlib import Path
from io import BytesIO
from concurrent.futures import ThreadPoolExecutor, as_completed
import threading
from threading import Lock, Thread
from queue import Queue

import httpx
import yaml
from openai import OpenAI
from PIL import Image

print_lock = Lock()
_log_file_path = None          # 全局日志文件路径
_error_log_path = None         # 错误日志文件路径
# Width (digits) used to format thread ids in logs; set in main() based on concurrency
_thread_id_width = 2
_g_up, _g_down, _g_max_up, _g_max_down = 3, 10, 50, 50

def ts(): return time.strftime("%H:%M:%S")

def log(*args):
    # 获取当前线程短编号用于日志（工作线程从1开始，主线程/其他使用0）
    tname = threading.current_thread().name
    tid = None
    if "ThreadPoolExecutor" in tname and "_" in tname:
        try:
            num_str = tname.split("_")[-1]
            tid = int(num_str) + 1
        except Exception:
            tid = 0
    else:
        tid = 0
    tid_str = str(tid).zfill(_thread_id_width)
    msg = f"[{ts()}][T{tid_str}] " + " ".join(str(a) for a in args)

    with print_lock:
        print(msg, flush=True)
        # 如果日志文件路径已设置，则写入文件
        if _log_file_path:
            try:
                with open(_log_file_path, "a", encoding="utf-8") as f:
                    f.write(msg + "\n")
            except Exception:
                pass   # 避免因日志写入失败影响主流程

def log_error(*args):
    """记录错误到控制台和错误日志文件"""
    msg = " ".join(str(a) for a in args)
    log(f"  [ERROR] {msg}")
    if _error_log_path:
        with print_lock:
            try:
                with open(_error_log_path, "a", encoding="utf-8") as f:
                    f.write(f"{time.strftime('%Y-%m-%d %H:%M:%S')} [ERROR] {msg}\n")
            except Exception:
                pass

def log_warning(*args):
    """记录警告到控制台和错误日志文件"""
    msg = " ".join(str(a) for a in args)
    log(f"  [WARNING] {msg}")
    if _error_log_path:
        with print_lock:
            try:
                with open(_error_log_path, "a", encoding="utf-8") as f:
                    f.write(f"{time.strftime('%Y-%m-%d %H:%M:%S')} [WARNING] {msg}\n")
            except Exception:
                pass

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
Start your response DIRECTLY with "[IMG_TYPE:" (no extra text before). Do NOT include any introductory phrases, conversational text, meta-commentary, or analysis before "[IMG_TYPE:". Wrong: "Based on the image...".

## Tool: get_more_context
Start with a small context window. To get more, call get_more_context(more_above=N, more_below=M) — N and M are additional lines.
You receive only the delta. Max 3 calls.

## LANGUAGE
Respond in {OUTPUT_LANG}.
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
    # half_up = max_per_up // 2
    # half_down = max_per_down // 2
    # dynamic_prompt = SYSTEM_PROMPT.format(half_up=half_up, half_down=half_down)

    # Build tools dynamically from current config
    tools = build_tools(max_per_up, max_per_down)

    # Initial context
    parts, start, end = get_context_lines(lines, img_line_idx, cur_up, cur_down)
    ctx_text = "\n".join(parts)

    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
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
            content = choice.message.content or ""
            # 检测是否包含 XML 格式的工具调用
            if "<tool_call>" in content or "<function=" in content:
                # 尝试解析 XML 格式的工具调用
                func_match = re.search(r"<function=([^>]+)>", content)
                if func_match and func_match.group(1) == "get_more_context":
                    # 提取参数
                    above_match = re.search(r"<parameter=more_above>([^<]+)</parameter>", content)
                    below_match = re.search(r"<parameter=more_below>([^<]+)</parameter>", content)
                    if above_match and below_match:
                        try:
                            more_up = int(above_match.group(1))
                            more_down = int(below_match.group(1))
                            # 模拟一个标准的 tool_calls 消息
                            tool_call_obj = {
                                "id": f"call_{int(time.time())}",
                                "type": "function",
                                "function": {
                                    "name": "get_more_context",
                                    "arguments": json.dumps({"more_above": more_up, "more_below": more_down})
                                }
                            }
                            # 将 assistant 消息带上 tool_calls 加入 messages
                            messages.append({
                                "role": "assistant",
                                "content": None,
                                "tool_calls": [tool_call_obj]
                            })
                            # 手动执行工具调用（复用现有逻辑）
                            actual_up = min(more_up, max_per_up)
                            actual_down = min(more_down, max_per_down)
                            new_up = cur_up + actual_up
                            new_down = cur_down + actual_down
                            log(f"    [ToolCall] (XML) AI wants +{more_up}up/-{more_down}down -> window {cur_up}/{cur_down}>{new_up}/{new_down}")
                            delta = get_delta_lines(lines, img_line_idx, cur_up, cur_down, new_up, new_down)
                            if delta:
                                result_text = (
                                    f"Added {actual_up} lines above and {actual_down} lines below. "
                                    f"Window is now [{img_line_idx - new_up} to {img_line_idx + new_down}].\n\n"
                                    f"NEW content (delta only):\n\n" + "\n".join(delta)
                                )
                            else:
                                result_text = f"No new lines could be added. Window remains [{img_line_idx - cur_up} to {img_line_idx + cur_down}]. Please proceed."
                            cur_up, cur_down = new_up, new_down
                            messages.append({"role": "tool", "tool_call_id": tool_call_obj["id"], "content": result_text})
                            continue  # 继续下一轮对话，模型会基于新上下文继续
                        except ValueError:
                            pass  # 参数解析失败，忽略，走下面的普通返回
            # 没有 XML 工具调用或解析失败，正常返回内容
            return content

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
        result = (call_ai_with_tools(
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

        # 强制清理：移除 [IMG_TYPE: 之前的所有前缀内容，并在有前缀时警告
        idx = result.find("[IMG_TYPE:")
        if idx != -1:
            if idx > 0:
                # 前面有内容，输出警告
                prefix = result[:idx].strip()
                log_warning(f"Unexpected prefix before '[IMG_TYPE:' in {img_path_str}: {prefix[:80]}")
            result = result[idx:]
        else:
            log_error(f"No '[IMG_TYPE:' found in result from {img_path_str}: {result[:100]}")
            return "__INVALID_RESPONSE__"

        return result.strip()
    except Exception as e:
        return f"[IMG_PROCESS_ERROR: {e}]"

# ─── Progress ──────────────────────────────────────────────────────
def load_progress(progress_root):
    """从子目录加载进度，每个 MD 文件一个子目录"""
    progress = {}
    if not progress_root.exists():
        return progress
    for md_dir in progress_root.iterdir():
        if not md_dir.is_dir():
            continue
        for item_file in md_dir.glob("*.json"):
            try:
                with open(item_file, "r", encoding="utf-8") as f:
                    data = json.load(f)
                key = data["key"]
                progress[key] = {k: v for k, v in data.items() if k != "key"}
            except Exception:
                continue
    return progress

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
    # 读取语言配置并注入系统提示词
    global SYSTEM_PROMPT
    lang = config["options"].get("output_language", "Chinese")
    lang_str = str(lang).strip()
    if lang_str.lower() in ("en", "english", "英文"):
        output_lang = "English"
    else:
        output_lang = "Chinese"
    SYSTEM_PROMPT = SYSTEM_PROMPT.replace("{OUTPUT_LANG}", output_lang)
    api_timeout = config["options"].get("api_timeout", 400)
    api_connect_timeout = config["options"].get("api_connect_timeout", 60)
    api_max_retries = config["options"].get("api_max_retries", 3)
    rate_limit_retries = config["options"].get("rate_limit_retries", 0)
    concurrency = config["options"].get("concurrency", 3)
    # 设置日志线程ID宽度（根据并发数自动对齐）
    global _thread_id_width
    _thread_id_width = len(str(concurrency))
    indir   = Path(config["paths"]["input_dir"])
    imgdir  = Path(config["paths"]["images_dir"])
    outdir  = Path(config["paths"]["output_dir"])

    outdir.mkdir(parents=True, exist_ok=True)

    # 设置全局日志文件路径（每次运行生成带时间戳的独立日志）
    from datetime import datetime
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    global _log_file_path, _error_log_path
    _log_file_path = outdir / f"img2text_{timestamp}.log"
    _error_log_path = outdir / f"img2text_errors_{timestamp}.log"

    # 创建进度存储根目录（按 MD 文件分子目录）
    progress_root = outdir / "progress_items"
    progress_root.mkdir(exist_ok=True)
    progress = load_progress(progress_root)
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
        log("All done! Clear progress_items/ directory to re-run.")
    else:
        def worker(t):
            key, mn, ip, il, ms, me = t
            start_time = time.time()
            log(f"▶ START {key}")
            try:
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
                elapsed = time.time() - start_time
                log(f"✓ [{elapsed:.2f}s] DONE")
                return key, r, ms, me, ip
            except Exception as e:
                elapsed = time.time() - start_time
                log_error(f"✗ [{elapsed:.2f}s] FAILED {e}")
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
                    if result == "__INVALID_RESPONSE__":
                        log_warning(f"Skipped invalid response for {ip}, will retry next run.")
                    else:
                        # 解析 key: "2027操作系统.md::images/xxx.jpg"
                        parts = key.split("::")
                        if len(parts) != 2:
                            log_error(f"Invalid key format: {key}")
                            done_count += 1
                            continue
                        md_filename = parts[0]          # "2027操作系统.md"
                        img_rel_path = parts[1]         # "images/xxx.jpg"
                        # 去掉 .md 后缀作为子目录名
                        subdir_name = md_filename.replace(".md", "")
                        subdir = progress_root / subdir_name
                        subdir.mkdir(exist_ok=True)
                        # 生成安全的文件名：将图片路径中的 '/' 替换为 '_'，并保留扩展名
                        safe_img_name = img_rel_path.replace("/", "_").replace("\\", "_")
                        item_file = subdir / f"{safe_img_name}.json"

                        item_data = {
                            "key": key,
                            "result": result,
                            "start": ms,
                            "end": me,
                            "img_path": ip
                        }
                        # 原子写入
                        tmp_file = item_file.with_suffix(".tmp")
                        with open(tmp_file, "w", encoding="utf-8") as f:
                            json.dump(item_data, f, ensure_ascii=False, indent=2)
                        os.replace(tmp_file, item_file)

                        # 更新内存 progress
                        progress[key] = {"result": result, "start": ms, "end": me, "img_path": ip}

                        log("-" * 50)
                        log(f"[{done_count+1}/{total}] {ip} (from {md_filename})")
                        log(f"RESULT:\n{result[:500]}")
                        log("-" * 50)
                    done_count += 1
                except Exception as e:
                    log_error(f"Writer error: {e}"); traceback.print_exc()
                finally:
                    result_queue.task_done()

        writer = Thread(target=writer_thread, daemon=False)
        writer.start()

        ex = ThreadPoolExecutor(max_workers=concurrency)
        try:
            fs = {ex.submit(worker, t): t for t in pending}
            for f in as_completed(fs):
                t = fs[f]
                try:
                    res = f.result()
                    result_queue.put(res)
                except Exception as e:
                    log_error(f"Worker fatal: {e}"); traceback.print_exc()
                    # ensure writer can finish
                    result_queue.put((t[0], f"[IMG_SUBMIT_ERROR: {e}]", t[4], t[5], t[2]))
        except KeyboardInterrupt:
            log("\nCtrl+C received, exiting...")
            sys.exit(1)   # 立即退出进程

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
