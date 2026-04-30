#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
MinerU API 调用脚本 - 调用 MinerU API 解析 PDF 文件
功能：
1. 上传 PDF 文件到 MinerU API
2. 轮询任务状态直到完成
3. 下载并解压结果到指定目录

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
from pathlib import Path
from typing import Optional, Dict, Any, List

import yaml


def load_config(config_path: str = "config.yaml") -> dict:
    """加载配置文件"""
    with open(config_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


class MinerUClient:
    """MinerU API 客户端"""

    def __init__(self, config: dict):
        self.config = config
        mineru_config = config["mineru"]
        self.api_base_url = mineru_config["api_base_url"]
        self.token = mineru_config["token"]
        self.model_version = mineru_config.get("model_version", "vlm")
        self.is_ocr = mineru_config.get("is_ocr", False)
        self.enable_formula = mineru_config.get("enable_formula", True)
        self.enable_table = mineru_config.get("enable_table", True)
        self.language = mineru_config.get("language", "ch")
        self.poll_interval = mineru_config.get("poll_interval", 3)
        self.poll_timeout = mineru_config.get("poll_timeout", 600)

        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.token}"
        }

    def create_task(self, file_url: str, data_id: str = None) -> Optional[str]:
        """
        创建解析任务（URL方式）

        Args:
            file_url: 文件 URL
            data_id: 可选的数据 ID

        Returns:
            task_id 或 None（失败时）
        """
        url = f"{self.api_base_url}/extract/task"
        data = {
            "url": file_url,
            "model_version": self.model_version,
            "is_ocr": self.is_ocr,
            "enable_formula": self.enable_formula,
            "enable_table": self.enable_table,
            "language": self.language
        }
        if data_id:
            data["data_id"] = data_id

        try:
            response = requests.post(url, headers=self.headers, json=data, timeout=30)
            response.raise_for_status()
            result = response.json()

            if result.get("code") == 0:
                task_id = result["data"]["task_id"]
                print(f"  任务已创建: {task_id}")
                return task_id
            else:
                print(f"  创建任务失败: {result.get('msg', '未知错误')}")
                return None
        except Exception as e:
            print(f"  创建任务异常: {e}")
            return None

    def create_batch_upload_tasks(self, file_paths: List[Path]) -> Optional[str]:
        """
        批量上传本地文件并创建解析任务

        Args:
            file_paths: 本地文件路径列表

        Returns:
            batch_id 或 None（失败时）
        """
        url = f"{self.api_base_url}/file-urls/batch"
        files = []
        for fp in file_paths:
            files.append({"name": fp.name, "data_id": fp.stem})

        data = {
            "files": files,
            "model_version": self.model_version,
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
                upload_urls = result["data"]["file_urls"]
                print(f"  批量任务已创建: {batch_id}")

                # 上传文件
                for i, (fp, upload_url) in enumerate(zip(file_paths, upload_urls)):
                    print(f"  上传文件 ({i+1}/{len(file_paths)}): {fp.name}")
                    with open(fp, "rb") as f:
                        upload_resp = requests.put(upload_url, data=f, timeout=120)
                        if upload_resp.status_code == 200:
                            print(f"    上传成功")
                        else:
                            print(f"    上传失败: HTTP {upload_resp.status_code}")

                return batch_id
            else:
                print(f"  创建批量任务失败: {result.get('msg', '未知错误')}")
                return None
        except Exception as e:
            print(f"  创建批量任务异常: {e}")
            return None

    def get_task_result(self, task_id: str) -> Dict[str, Any]:
        """
        获取任务结果

        Args:
            task_id: 任务 ID

        Returns:
            任务状态字典
        """
        url = f"{self.api_base_url}/extract/task/{task_id}"
        try:
            response = requests.get(url, headers=self.headers, timeout=30)
            response.raise_for_status()
            return response.json()
        except Exception as e:
            return {"code": -1, "msg": str(e), "data": {"state": "failed", "err_msg": str(e)}}

    def get_batch_results(self, batch_id: str) -> Dict[str, Any]:
        """
        获取批量任务结果

        Args:
            batch_id: 批量任务 ID

        Returns:
            批量任务状态字典
        """
        url = f"{self.api_base_url}/extract-results/batch/{batch_id}"
        try:
            response = requests.get(url, headers=self.headers, timeout=30)
            response.raise_for_status()
            return response.json()
        except Exception as e:
            return {"code": -1, "msg": str(e), "data": {"extract_result": []}}

    def poll_task(self, task_id: str) -> Dict[str, Any]:
        """
        轮询任务直到完成

        Args:
            task_id: 任务 ID

        Returns:
            最终任务结果
        """
        start_time = time.time()
        state_labels = {
            "pending": "排队中",
            "running": "解析中",
            "converting": "格式转换中"
        }

        while time.time() - start_time < self.poll_timeout:
            result = self.get_task_result(task_id)
            data = result.get("data", {})
            state = data.get("state", "unknown")
            elapsed = int(time.time() - start_time)

            if state == "done":
                print(f"  [{elapsed}s] 解析完成")
                return result
            elif state == "failed":
                err_msg = data.get("err_msg", "未知错误")
                print(f"  [{elapsed}s] 解析失败: {err_msg}")
                return result
            else:
                label = state_labels.get(state, state)
                progress = data.get("extract_progress", {})
                if progress:
                    extracted = progress.get("extracted_pages", 0)
                    total = progress.get("total_pages", 0)
                    print(f"  [{elapsed}s] {label} ({extracted}/{total} 页)")
                else:
                    print(f"  [{elapsed}s] {label}...")
                time.sleep(self.poll_interval)

        print(f"  轮询超时 ({self.poll_timeout}s)")
        return {"code": -1, "msg": "轮询超时", "data": {"state": "timeout"}}

    def poll_batch_tasks(self, batch_id: str) -> list:
        """
        轮询批量任务直到完成

        Args:
            batch_id: 批量任务 ID

        Returns:
            任务结果列表
        """
        start_time = time.time()

        while time.time() - start_time < self.poll_timeout:
            result = self.get_batch_results(batch_id)
            extract_results = result.get("data", {}).get("extract_result", [])

            all_done = True
            for item in extract_results:
                state = item.get("state", "unknown")
                if state not in ("done", "failed"):
                    all_done = False
                    break

            if all_done:
                elapsed = int(time.time() - start_time)
                print(f"  [{elapsed}s] 所有任务完成")
                return extract_results

            elapsed = int(time.time() - start_time)
            done_count = sum(1 for item in extract_results if item.get("state") == "done")
            total_count = len(extract_results)
            print(f"  [{elapsed}s] 进度: {done_count}/{total_count}")
            time.sleep(self.poll_interval)

        print(f"  轮询超时 ({self.poll_timeout}s)")
        return []

    def download_and_extract(self, zip_url: str, output_dir: Path, folder_name: str) -> bool:
        """
        下载并解压结果

        Args:
            zip_url: ZIP 文件 URL
            output_dir: 输出目录
            folder_name: 文件夹名称

        Returns:
            是否成功
        """
        try:
            target_dir = output_dir / folder_name
            target_dir.mkdir(parents=True, exist_ok=True)

            # 下载 ZIP 文件
            print(f"  下载: {zip_url}")
            response = requests.get(zip_url, timeout=120)
            response.raise_for_status()

            # 保存 ZIP 文件
            zip_path = target_dir / "result.zip"
            zip_path.write_bytes(response.content)

            # 解压
            with zipfile.ZipFile(zip_path, 'r') as zip_ref:
                zip_ref.extractall(target_dir)

            # 删除 ZIP 文件
            zip_path.unlink()

            print(f"  已解压到: {target_dir}")
            return True
        except Exception as e:
            print(f"  下载解压失败: {e}")
            return False


def process_single_file(client: MinerUClient, pdf_path: Path, output_dir: Path) -> bool:
    """
    处理单个 PDF 文件（通过批量上传接口）

    Args:
        client: MinerU 客户端
        pdf_path: PDF 文件路径
        output_dir: 输出目录

    Returns:
        是否成功
    """
    print(f"\n处理: {pdf_path.name}")

    # 使用批量上传接口（单个文件也用这个接口）
    batch_id = client.create_batch_upload_tasks([pdf_path])
    if not batch_id:
        return False

    # 轮询等待结果
    results = client.poll_batch_tasks(batch_id)
    if not results:
        return False

    # 处理结果
    for item in results:
        state = item.get("state", "unknown")
        file_name = item.get("file_name", "")
        folder_name = Path(file_name).stem if file_name else pdf_path.stem

        if state == "done":
            zip_url = item.get("full_zip_url")
            if zip_url:
                client.download_and_extract(zip_url, output_dir, folder_name)
            else:
                print(f"  警告: 没有下载链接")
        elif state == "failed":
            err_msg = item.get("err_msg", "未知错误")
            print(f"  解析失败: {err_msg}")
            return False

    return True


def process_split_pdfs(client: MinerUClient, split_dir: Path, output_dir: Path) -> bool:
    """
    处理 split_pdfs 目录下的所有 PDF

    Args:
        client: MinerU 客户端
        split_dir: 分割后的 PDF 目录
        output_dir: 输出目录

    Returns:
        是否成功
    """
    if not split_dir.exists():
        print(f"[错误] 分割目录不存在: {split_dir}")
        return False

    pdf_files = sorted(split_dir.glob("*.pdf"))
    if not pdf_files:
        print(f"[错误] 没有找到 PDF 文件: {split_dir}")
        return False

    print(f"找到 {len(pdf_files)} 个 PDF 文件")

    # 使用批量上传接口
    batch_id = client.create_batch_upload_tasks(pdf_files)
    if not batch_id:
        return False

    # 轮询等待结果
    results = client.poll_batch_tasks(batch_id)
    if not results:
        return False

    # 处理结果
    success_count = 0
    for item in results:
        state = item.get("state", "unknown")
        file_name = item.get("file_name", "")
        folder_name = Path(file_name).stem if file_name else "unknown"

        if state == "done":
            zip_url = item.get("full_zip_url")
            if zip_url:
                if client.download_and_extract(zip_url, output_dir, folder_name):
                    success_count += 1
            else:
                print(f"  警告: {file_name} 没有下载链接")
        elif state == "failed":
            err_msg = item.get("err_msg", "未知错误")
            print(f"  解析失败 {file_name}: {err_msg}")

    print(f"\n处理完成: {success_count}/{len(results)} 成功")
    return success_count > 0


def main():
    parser = argparse.ArgumentParser(description="MinerU API 调用工具")
    parser.add_argument("--config", type=str, default="config.yaml", help="配置文件路径")
    parser.add_argument("--file", type=str, help="单个文件路径")
    parser.add_argument("--batch", action="store_true", help="批量处理模式")
    args = parser.parse_args()

    config = load_config(args.config)
    client = MinerUClient(config)

    output_dir = Path(config["paths"]["mineru_output"])
    output_dir.mkdir(parents=True, exist_ok=True)

    if args.file:
        # 处理单个文件
        pdf_path = Path(args.file)
        if not pdf_path.exists():
            print(f"[错误] 文件不存在: {pdf_path}")
            sys.exit(1)
        success = process_single_file(client, pdf_path, output_dir)
    else:
        # 处理 split_pdfs 目录
        split_dir = Path(config["paths"]["split_dir"])
        success = process_split_pdfs(client, split_dir, output_dir)

    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
