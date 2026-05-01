#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
MinerU API 调用脚本 - 调用 MinerU API 解析 PDF 文件
功能：
1. 为每个 PDF 创建独立任务
2. 支持断点续传（任务状态记录）
3. 智能轮询显示（指数退避 + 进度阈值）
4. 支持无限等待（poll_timeout = 0）
5. 并发处理多个任务
6. 超时后恢复旧任务
7. 下载并解压结果到指定目录

用法:
    python mineru_api.py                     # 处理 split_pdfs 目录下的所有 PDF
    python mineru_api.py --file test.pdf     # 处理单个文件
    python mineru_api.py --config my.yaml    # 指定配置文件
"""

import os
import sys
import time
import json
import zipfile
import argparse
import requests
import threading
from pathlib import Path
from enum import Enum
from typing import Optional, Dict, Any, List, Tuple
from datetime import datetime
from concurrent.futures import ThreadPoolExecutor, as_completed

import yaml


class TaskStatus(Enum):
    DONE = "done"    # 本次成功处理
    SKIP = "skip"    # 之前已完成，直接跳过
    FAIL = "fail"    # 最终失败（包括超时退出）


def load_config(config_path: str = "config.yaml") -> dict:
    """加载配置文件"""
    with open(config_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def atomic_write_json(json_path: Path, data: dict):
    """原子写入 JSON 文件（先写临时文件再替换）"""
    tmp_path = json_path.with_suffix(".tmp")
    with open(tmp_path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
    os.replace(tmp_path, json_path)


class MinerUClient:
    """MinerU API 客户端"""

    def __init__(self, config: dict):
        self.config = config
        mineru_config = config["mineru"]
        self.api_base_url = mineru_config["api_base_url"]
        self.token = mineru_config["token"]
        self.model_version = mineru_config.get("model_version", "vlm")
        self.is_ocr = mineru_config.get("is_ocr", True)
        self.enable_formula = mineru_config.get("enable_formula", True)
        self.enable_table = mineru_config.get("enable_table", True)
        self.language = mineru_config.get("language", "ch")
        self.poll_interval = mineru_config.get("poll_interval", 3)
        self.log_poll_interval = mineru_config.get("log_poll_interval", 3)
        self.poll_timeout = mineru_config.get("poll_timeout", 0)
        self.progress_threshold = mineru_config.get("progress_threshold", 80)

        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.token}"
        }

    def create_single_upload_task(self, file_path: Path) -> Optional[str]:
        """为单个文件创建上传任务"""
        url = f"{self.api_base_url}/file-urls/batch"
        data = {
            "files": [{"name": file_path.name, "data_id": file_path.stem}],
            "model_version": self.model_version,
            "is_ocr": self.is_ocr,
            "enable_formula": self.enable_formula,
            "enable_table": self.enable_table,
            "language": self.language
        }

        try:
            response = requests.post(url, headers=self.headers, json=data, timeout=30)
            response.raise_for_status()
            result = response.json()

            if result.get("code") == 0:
                batch_id = result["data"]["batch_id"]
                upload_url = result["data"]["file_urls"][0]

                with open(file_path, "rb") as f:
                    upload_resp = requests.put(upload_url, data=f, timeout=120)
                    if upload_resp.status_code == 200:
                        return batch_id
                    else:
                        print(f"[{file_path.name}] 上传失败: HTTP {upload_resp.status_code}")
                        return None
            else:
                print(f"[{file_path.name}] 创建任务失败: {result.get('msg', '未知错误')}")
                return None
        except Exception as e:
            print(f"[{file_path.name}] 创建任务异常: {e}")
            return None

    def get_batch_results(self, batch_id: str) -> Dict[str, Any]:
        """获取批量任务结果"""
        url = f"{self.api_base_url}/extract-results/batch/{batch_id}"
        try:
            response = requests.get(url, headers=self.headers, timeout=30)
            response.raise_for_status()
            return response.json()
        except Exception as e:
            return {"code": -1, "msg": str(e), "data": {"extract_result": []}}

    def check_old_task_valid(self, batch_id: str) -> Tuple[bool, Optional[dict]]:
        """
        检查旧任务是否有效
        Returns: (是否有效, 结果项)
        """
        result = self.get_batch_results(batch_id)
        extract_results = result.get("data", {}).get("extract_result", [])
        if not extract_results:
            return False, None
        item = extract_results[0]
        state = item.get("state", "unknown")
        # 如果任务已失败或不存在，视为无效
        if result.get("code") != 0 or state == "failed":
            return False, item
        return True, item

    def poll_batch_with_display(
        self,
        file_name: str,
        batch_id: str,
        info: dict,
        json_path: Path,
        output_dir: Path
    ) -> Tuple[bool, bool]:
        """
        智能轮询批量任务状态（指数退避 + 进度阈值）

        Returns:
            (是否成功, 是否超时) - 超时表示任务仍在运行，应保留 running 状态
        """
        start_time = time.time()
        last_log_time = start_time
        log_interval = self.log_poll_interval
        last_progress = None
        last_valid_progress = None  # 记录最后一次有效的页数进度

        state_labels = {
            "pending": "排队中",
            "running": "解析中",
            "converting": "格式转换中",
            "waiting-file": "等待文件上传"
        }

        while True:
            result = self.get_batch_results(batch_id)
            extract_results = result.get("data", {}).get("extract_result", [])

            if not extract_results:
                time.sleep(self.poll_interval)
                continue

            item = extract_results[0]
            state = item.get("state", "unknown")
            elapsed = int(time.time() - start_time)

            progress = item.get("extract_progress", {})
            current_progress = (
                progress.get("extracted_pages", 0),
                progress.get("total_pages", 0)
            )

            # 计算页数变化量
            pages_delta = 0
            if last_progress is not None:
                pages_delta = current_progress[0] - last_progress[0]

            # 记录最后一次有效的页数进度（total_pages > 0）
            if current_progress[1] > 0:
                last_valid_progress = current_progress

            # 检测是否需要立即输出：
            # 1. 首次运行
            # 2. 页数变化 >= 阈值
            # 3. 状态变为终态 (done/failed)
            has_significant_change = (
                last_progress is None or
                pages_delta >= self.progress_threshold or
                state in ("done", "failed")
            )

            now = time.time()

            if has_significant_change:
                # 立即输出并重置间隔
                if state == "done":
                    total_pages = current_progress[1]
                    if last_valid_progress and last_valid_progress[1] > 0:
                        total_pages = last_valid_progress[1]
                    if total_pages > 0:
                        print(f"[{file_name}] [{elapsed}s] 解析完成，共 {total_pages} 页")
                    else:
                        print(f"[{file_name}] [{elapsed}s] 解析完成")
                elif state == "failed":
                    err_msg = item.get("err_msg", "未知错误")
                    print(f"[{file_name}] [{elapsed}s] 解析失败: {err_msg}")
                else:
                    label = state_labels.get(state, state)
                    pages_info = f"({current_progress[0]}/{current_progress[1]} 页)" if current_progress[1] > 0 else ""
                    print(f"[{file_name}] [{elapsed}s] {label} {pages_info}")

                log_interval = self.log_poll_interval
                last_log_time = now
                last_progress = current_progress

                # 处理完成
                if state == "done":
                    info["status"] = "done"
                    atomic_write_json(json_path, info)

                    zip_url = item.get("full_zip_url")
                    if zip_url:
                        folder_name = info["output_folder"]
                        success = self.download_and_extract(file_name, zip_url, output_dir, folder_name)
                        if success:
                            print(f"[{file_name}] ✓ 已完成并下载")
                            return True, False
                        else:
                            info["status"] = "failed"
                            info["last_error"] = "下载解压失败"
                            atomic_write_json(json_path, info)
                            return False, False
                    else:
                        info["status"] = "failed"
                        info["last_error"] = "没有下载链接"
                        atomic_write_json(json_path, info)
                        return False, False

                elif state == "failed":
                    err_msg = item.get("err_msg", "未知错误")
                    info["status"] = "failed"
                    info["last_error"] = err_msg
                    atomic_write_json(json_path, info)
                    return False, False

            elif now - last_log_time >= log_interval:
                # 按退避间隔输出
                label = state_labels.get(state, state)
                pages_info = f"({current_progress[0]}/{current_progress[1]} 页)" if current_progress[1] > 0 else ""
                print(f"[{file_name}] [{elapsed}s] {label} {pages_info}")

                log_interval = min(log_interval * 2, 64)
                last_log_time = now
                last_progress = current_progress

            # 检查超时
            if self.poll_timeout > 0 and elapsed >= self.poll_timeout:
                print(f"\n[{file_name}] [错误] 轮询超时 ({self.poll_timeout}s)")
                print(f"[{file_name}] 任务仍在运行，建议稍后重新运行脚本继续等待")
                # 保留 running 状态，返回超时标志
                return False, True

            time.sleep(self.poll_interval)

    def download_and_extract(self, file_name: str, zip_url: str, output_dir: Path, folder_name: str) -> bool:
        """下载并解压结果"""
        try:
            target_dir = output_dir / folder_name
            target_dir.mkdir(parents=True, exist_ok=True)

            print(f"[{file_name}] 下载结果...")
            response = requests.get(zip_url, timeout=120)
            response.raise_for_status()

            zip_path = target_dir / "result.zip"
            zip_path.write_bytes(response.content)

            with zipfile.ZipFile(zip_path, 'r') as zip_ref:
                zip_ref.extractall(target_dir)

            zip_path.unlink()
            return True
        except Exception as e:
            print(f"[{file_name}] 下载解压失败: {e}")
            return False


def process_single_file(
    client: MinerUClient,
    pdf_file: Path,
    output_dir: Path,
    status_dir: Path
) -> Tuple[str, TaskStatus]:
    """
    处理单个 PDF 文件（支持断点续传和恢复）

    Returns:
        (文件名, TaskStatus)
    """
    json_path = status_dir / f"{pdf_file.stem}_task.json"

    # 检查是否已完成
    if json_path.exists():
        with open(json_path, "r", encoding="utf-8") as f:
            info = json.load(f)

        if info.get("status") == "done":
            output_folder = output_dir / pdf_file.stem
            if output_folder.exists() and any(output_folder.iterdir()):
                print(f"[{pdf_file.name}] [跳过] 已完成")
                return pdf_file.name, TaskStatus.SKIP

        # 尝试恢复旧任务（包括 running 状态，覆盖超时退出的情况）
        old_batch_id = info.get("batch_id")
        if old_batch_id and info.get("status") in ("running", "timeout"):
            print(f"[{pdf_file.name}] 恢复任务 {old_batch_id} ...")
            is_valid, item = client.check_old_task_valid(old_batch_id)
            if is_valid:
                # 检查是否已经完成（服务端早已完成但本地未下载）
                state = item.get("state", "unknown")
                if state == "done":
                    zip_url = item.get("full_zip_url")
                    if zip_url:
                        info["status"] = "done"
                        atomic_write_json(json_path, info)
                        folder_name = info["output_folder"]
                        if client.download_and_extract(pdf_file.name, zip_url, output_dir, folder_name):
                            print(f"[{pdf_file.name}] ✓ 恢复并下载完成")
                            return pdf_file.name, TaskStatus.DONE
                    # 没有下载链接，走正常轮询流程
                # 旧任务有效，继续轮询
                success, is_timeout = client.poll_batch_with_display(
                    pdf_file.name, old_batch_id, info, json_path, output_dir
                )
                if success:
                    return pdf_file.name, TaskStatus.DONE
                elif is_timeout:
                    # 超时退出，保留 running 状态，下次继续恢复
                    info["status"] = "running"
                    atomic_write_json(json_path, info)
                    return pdf_file.name, TaskStatus.FAIL
                # 轮询返回失败但非超时，尝试重建任务
                print(f"[{pdf_file.name}] 旧任务无法继续，重新提交...")
            else:
                print(f"[{pdf_file.name}] 旧任务已失效，重新提交...")

    # 创建新任务
    batch_id = client.create_single_upload_task(pdf_file)
    if not batch_id:
        return pdf_file.name, TaskStatus.FAIL

    info = {
        "file": pdf_file.name,
        "batch_id": batch_id,
        "created_at": datetime.now().isoformat(),
        "status": "running",
        "last_error": "",
        "output_folder": pdf_file.stem
    }
    atomic_write_json(json_path, info)
    print(f"[{pdf_file.name}] 任务已创建: {batch_id}")

    success, is_timeout = client.poll_batch_with_display(
        pdf_file.name, batch_id, info, json_path, output_dir
    )

    if is_timeout:
        # 超时，保留 running 状态，下次继续恢复
        info["status"] = "running"
        atomic_write_json(json_path, info)
        return pdf_file.name, TaskStatus.FAIL

    if success:
        return pdf_file.name, TaskStatus.DONE
    return pdf_file.name, TaskStatus.FAIL


def process_all_files_concurrent(client: MinerUClient, split_dir: Path, output_dir: Path, max_concurrent: int) -> bool:
    """
    并发处理所有 PDF 文件
    """
    if not split_dir.exists():
        print(f"[错误] 分割目录不存在: {split_dir}")
        return False

    pdf_files = sorted(split_dir.glob("*.pdf"))
    if not pdf_files:
        print(f"[错误] 没有找到 PDF 文件: {split_dir}")
        return False

    print(f"找到 {len(pdf_files)} 个 PDF 文件，并发数: {max_concurrent}")

    status_dir = output_dir / "tasks"
    status_dir.mkdir(parents=True, exist_ok=True)

    results = {"success": 0, "skip": 0, "fail": 0}
    lock = threading.Lock()

    def worker(pdf_file: Path) -> Tuple[str, TaskStatus]:
        return process_single_file(client, pdf_file, output_dir, status_dir)

    with ThreadPoolExecutor(max_workers=max_concurrent) as executor:
        futures = {executor.submit(worker, pdf): pdf for pdf in pdf_files}

        for future in as_completed(futures):
            pdf = futures[future]
            try:
                file_name, status = future.result()
                with lock:
                    if status == TaskStatus.DONE:
                        results["success"] += 1
                    elif status == TaskStatus.SKIP:
                        results["skip"] += 1
                    else:
                        results["fail"] += 1
            except Exception as e:
                print(f"[{pdf.name}] 处理异常: {e}")
                with lock:
                    results["fail"] += 1

    print(f"\n{'='*50}")
    print(f"处理完成: {results['success']} 成功, {results['skip']} 跳过, {results['fail']} 失败/超时")
    print(f"{'='*50}")

    return results["fail"] == 0


def main():
    parser = argparse.ArgumentParser(description="MinerU API 调用工具")
    parser.add_argument("--config", type=str, default="config.yaml", help="配置文件路径")
    parser.add_argument("--file", type=str, help="单个文件路径")
    args = parser.parse_args()

    config = load_config(args.config)
    client = MinerUClient(config)
    max_concurrent = config["mineru"].get("max_concurrent", 5)

    output_dir = Path(config["paths"]["mineru_output"])
    output_dir.mkdir(parents=True, exist_ok=True)

    if args.file:
        # 处理单个文件
        pdf_path = Path(args.file)
        if not pdf_path.exists():
            print(f"[错误] 文件不存在: {pdf_path}")
            sys.exit(1)

        status_dir = output_dir / "tasks"
        status_dir.mkdir(parents=True, exist_ok=True)

        file_name, status = process_single_file(client, pdf_path, output_dir, status_dir)
        sys.exit(0 if status == TaskStatus.DONE else 1)
    else:
        # 并发处理 split_pdfs 目录
        split_dir = Path(config["paths"]["split_dir"])
        success = process_all_files_concurrent(client, split_dir, output_dir, max_concurrent)
        sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
