#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
日志分析脚本：统计图片处理期间的工具调用次数、耗时、成功率等指标。
支持并发日志（按线程ID分组状态机解析）。

用法:
    python analyze.py                    # 分析最新日志
    python analyze.py --all                # 汇总所有历史日志
    python analyze.py --logfile <path>     # 指定日志文件
    python analyze.py --threads            # 显示线程详细统计
    python analyze.py --percentiles 90,95,99  # 自定义百分位数
    python analyze.py --output report.csv  # 导出CSV
"""

import re
import sys
import csv
import argparse
from collections import defaultdict
from pathlib import Path
from datetime import datetime, timedelta
from statistics import mean, median, stdev
from typing import List, Dict, Any, Optional, Tuple

import yaml

# =============================================================================
# 正则模式定义
# =============================================================================
LOG_PATTERNS = {
    'timestamp': re.compile(r'^\[(\d{2}:\d{2}:\d{2})\]'),
    'thread_id': re.compile(r'\[T(\d+)\]'),
    # 匹配完整格式: [HH:MM:SS][Txx] ▶ START key
    'start': re.compile(r'^\[\d{2}:\d{2}:\d{2}\]\[T\d+\]\s*▶\s*START\s+(.+)$'),
    # 匹配: [HH:MM:SS][Txx] ✓ [3.12s] DONE [IMG_TYPE: ...]
    'done': re.compile(r'^\[\d{2}:\d{2}:\d{2}\]\[T\d+\]\s*✓\s*\[(\d+\.?\d*)s\]\s*DONE(?:\s+\[IMG_TYPE:\s*([^\]]+)\])?'),
    # 匹配: [HH:MM:SS][Txx] ✗ [0.89s] FAILED error_msg
    'failed': re.compile(r'^\[\d{2}:\d{2}:\d{2}\]\[T\d+\]\s*✗\s*\[(\d+\.?\d*)s\]\s*FAILED\s*(.+)$'),
    'tool_call': re.compile(r'\[ToolCall\]'),
    'warning': re.compile(r'\[WARNING\]'),
    'error': re.compile(r'\[ERROR\]'),
}

# 错误分类正则
ERROR_PATTERNS = {
    'connection_timeout': re.compile(r'connection|timeout|timed out', re.I),
    'rate_limit': re.compile(r'429|rate.*limit', re.I),
    'img_missing': re.compile(r'IMG_MISSING', re.I),
    'api_error': re.compile(r'IMG_API_ERROR|IMG_PROCESS_ERROR', re.I),
    'worker_fatal': re.compile(r'IMG_WORKER_FATAL', re.I),
    'invalid_response': re.compile(r'__INVALID_RESPONSE__', re.I),
}


# =============================================================================
# 配置加载
# =============================================================================
def load_config(config_path: str = "config.yaml") -> Dict[str, Any]:
    with open(config_path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def find_latest_log(log_dir: Path) -> Optional[Path]:
    """查找 output 目录下最新的 img2text_*.log 文件"""
    log_files = list(log_dir.glob("img2text_*.log"))
    if not log_files:
        return None
    return max(log_files, key=lambda p: p.stat().st_mtime)


def find_all_logs(log_dir: Path) -> List[Path]:
    """查找所有 img2text_*.log 文件，按时间排序"""
    log_files = list(log_dir.glob("img2text_*.log"))
    return sorted(log_files, key=lambda p: p.stat().st_mtime)


# =============================================================================
# 日志解析（状态机方式，按线程ID分组）
# =============================================================================
class Session:
    """单张图片的处理会话"""
    def __init__(self, key: str, tid: str, start_ts: str):
        self.key = key                    # 如 "ch1.md::images/fig1.jpg"
        self.tid = tid                    # 线程ID
        self.start_ts = start_ts          # 开始时间戳
        self.tool_calls = 0               # 工具调用次数
        self.status = 'pending'           # pending/success/failed
        self.elapsed: Optional[float] = None
        self.error_type: Optional[str] = None
        self.error_msg: Optional[str] = None
        self.img_type: Optional[str] = None  # 从 DONE 行提取的图片类型

    def to_dict(self) -> Dict[str, Any]:
        return {
            'key': self.key,
            'tid': self.tid,
            'start_ts': self.start_ts,
            'tool_calls': self.tool_calls,
            'status': self.status,
            'elapsed': self.elapsed,
            'error_type': self.error_type,
            'error_msg': self.error_msg,
            'img_type': self.img_type,
        }


def parse_timestamp(ts_str: str) -> datetime:
    """解析 HH:MM:SS 时间戳（假设同一天）"""
    today = datetime.now().date()
    hour, minute, second = map(int, ts_str.split(':'))
    return datetime.combine(today, datetime.min.time().replace(hour=hour, minute=minute, second=second))


def classify_error(error_msg: str) -> str:
    """根据错误消息分类错误类型"""
    for error_name, pattern in ERROR_PATTERNS.items():
        if pattern.search(error_msg):
            return error_name
    return 'unknown'


def parse_log_line(line: str) -> Tuple[Optional[str], Optional[str], str]:
    """解析单行日志，返回 (timestamp, thread_id, content)"""
    ts_match = LOG_PATTERNS['timestamp'].match(line)
    if not ts_match:
        return None, None, line
    ts = ts_match.group(1)

    tid_match = LOG_PATTERNS['thread_id'].search(line)
    tid = tid_match.group(1) if tid_match else "0"

    content = line[ts_match.end():].strip()
    return ts, tid, content


def analyze_log(log_path: Path) -> List[Session]:
    """
    分析日志文件，按线程ID分组，使用状态机跟踪每张图片的处理会话。
    返回所有完成的 Session 列表。
    """
    sessions: List[Session] = []
    current_sessions: Dict[str, Session] = {}  # tid -> Session

    with open(log_path, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.rstrip('\n\r')
            ts, tid, content = parse_log_line(line)
            if not tid:
                continue

            # 检查是否是 START 行
            start_match = LOG_PATTERNS['start'].search(line)
            if start_match:
                key = start_match.group(1)
                # 如果该线程有未完成的会话，先关闭它（标记为异常）
                if tid in current_sessions:
                    old_session = current_sessions[tid]
                    old_session.status = 'incomplete'
                    sessions.append(old_session)
                # 创建新会话
                current_sessions[tid] = Session(key, tid, ts or "00:00:00")
                continue

            # 如果没有活动会话，跳过
            if tid not in current_sessions:
                continue

            session = current_sessions[tid]

            # 检查工具调用
            if LOG_PATTERNS['tool_call'].search(line):
                session.tool_calls += 1
                continue

            # 检查 DONE 行
            done_match = LOG_PATTERNS['done'].search(line)
            if done_match:
                session.elapsed = float(done_match.group(1))
                session.status = 'success'
                session.img_type = done_match.group(2)  # 可能为 None
                sessions.append(session)
                del current_sessions[tid]
                continue

            # 检查 FAILED 行
            failed_match = LOG_PATTERNS['failed'].search(line)
            if failed_match:
                session.elapsed = float(failed_match.group(1))
                session.status = 'failed'
                session.error_msg = failed_match.group(2).strip()
                session.error_type = classify_error(session.error_msg)
                sessions.append(session)
                del current_sessions[tid]
                continue

    # 处理未关闭的会话（日志不完整）
    for session in current_sessions.values():
        session.status = 'incomplete'
        sessions.append(session)

    return sessions


# =============================================================================
# 统计分析
# =============================================================================
def compute_percentile(values: List[float], p: float) -> float:
    """计算百分位数"""
    if not values:
        return 0.0
    sorted_vals = sorted(values)
    k = (len(sorted_vals) - 1) * p / 100.0
    f = int(k)
    c = f + 1 if f + 1 < len(sorted_vals) else f
    if f == c:
        return sorted_vals[f]
    return sorted_vals[f] * (c - k) + sorted_vals[c] * (k - f)


def compute_statistics(sessions: List[Session], percentiles: List[int] = None) -> Dict[str, Any]:
    """计算所有统计指标"""
    if percentiles is None:
        percentiles = [90, 95, 99]

    total = len(sessions)
    success_sessions = [s for s in sessions if s.status == 'success']
    failed_sessions = [s for s in sessions if s.status == 'failed']
    incomplete_sessions = [s for s in sessions if s.status == 'incomplete']

    # 基础计数
    stats = {
        'total': total,
        'success': len(success_sessions),
        'failed': len(failed_sessions),
        'incomplete': len(incomplete_sessions),
        'success_rate': len(success_sessions) / total * 100 if total > 0 else 0,
    }

    # 工具调用统计
    tool_calls_list = [s.tool_calls for s in success_sessions]
    if tool_calls_list:
        stats['tool_calls'] = {
            'total': sum(tool_calls_list),
            'avg': mean(tool_calls_list),
            'median': median(tool_calls_list),
            'min': min(tool_calls_list),
            'max': max(tool_calls_list),
            'distribution': defaultdict(int),
        }
        for tc in tool_calls_list:
            stats['tool_calls']['distribution'][tc] += 1
        # 调用最多的图片
        tc_by_key = {s.key: s.tool_calls for s in success_sessions}
        stats['tool_calls']['top'] = sorted(tc_by_key.items(), key=lambda x: x[1], reverse=True)[:20]
    else:
        stats['tool_calls'] = None

    # 耗时统计（仅成功）
    elapsed_list = [s.elapsed for s in success_sessions if s.elapsed is not None]
    if elapsed_list:
        stats['elapsed'] = {
            'total': sum(elapsed_list),
            'avg': mean(elapsed_list),
            'median': median(elapsed_list),
            'min': min(elapsed_list),
            'max': max(elapsed_list),
            'stdev': stdev(elapsed_list) if len(elapsed_list) > 1 else 0,
        }
        for p in percentiles:
            stats['elapsed'][f'p{p}'] = compute_percentile(elapsed_list, p)
    else:
        stats['elapsed'] = None

    # 按工具调用次数分组的耗时统计
    elapsed_by_tool_calls: Dict[int, List[float]] = defaultdict(list)
    for s in success_sessions:
        if s.elapsed is not None:
            elapsed_by_tool_calls[s.tool_calls].append(s.elapsed)

    stats['elapsed_by_tool_calls'] = {}
    for tc, elist in sorted(elapsed_by_tool_calls.items()):
        stats['elapsed_by_tool_calls'][tc] = {
            'count': len(elist),
            'avg': mean(elist),
            'median': median(elist),
        }

    # 错误分类统计
    error_dist: Dict[str, int] = defaultdict(int)
    for s in failed_sessions:
        error_dist[s.error_type or 'unknown'] += 1
    stats['error_distribution'] = dict(error_dist)

    # 线程统计
    thread_stats: Dict[str, Dict[str, Any]] = defaultdict(lambda: {
        'count': 0, 'success': 0, 'failed': 0,
        'elapsed_total': 0.0, 'elapsed_max': 0.0
    })
    for s in sessions:
        ts = thread_stats[s.tid]
        ts['count'] += 1
        if s.status == 'success':
            ts['success'] += 1
            if s.elapsed:
                ts['elapsed_total'] += s.elapsed
                ts['elapsed_max'] = max(ts['elapsed_max'], s.elapsed)
        elif s.status == 'failed':
            ts['failed'] += 1

    for tid, ts in thread_stats.items():
        ts['success_rate'] = ts['success'] / ts['count'] * 100 if ts['count'] > 0 else 0
        ts['avg_elapsed'] = ts['elapsed_total'] / ts['success'] if ts['success'] > 0 else 0

    stats['thread_stats'] = dict(thread_stats)
    stats['thread_count'] = len(thread_stats)

    # 吞吐量估算（总成功图片 / 总耗时）
    if elapsed_list:
        total_wall_time = sum(elapsed_list)
        stats['throughput'] = len(success_sessions) / total_wall_time if total_wall_time > 0 else 0
    else:
        stats['throughput'] = 0

    return stats


# =============================================================================
# 报告输出
# =============================================================================
def print_report(stats: Dict[str, Any], log_path: Path, args: argparse.Namespace):
    """打印统计报告"""
    print("=" * 70)
    print(f"日志分析报告: {log_path.name}")
    print("=" * 70)

    # 基础统计
    print(f"\n【基础统计】")
    print(f"  总图片数:      {stats['total']}")
    print(f"  成功:          {stats['success']} ({stats['success_rate']:.1f}%)")
    print(f"  失败:          {stats['failed']}")
    print(f"  未完成:        {stats['incomplete']}")
    print(f"  线程数:        {stats['thread_count']}")

    # 工具调用统计
    if stats['tool_calls']:
        tc = stats['tool_calls']
        print(f"\n【工具调用统计】")
        print(f"  总调用次数:    {tc['total']}")
        print(f"  平均调用:      {tc['avg']:.2f} 次/图片")
        print(f"  中位数:        {tc['median']:.0f}")
        print(f"  范围:          {tc['min']} - {tc['max']}")
        print(f"\n  调用次数分布:")
        for count, num in sorted(tc['distribution'].items()):
            pct = num / stats['success'] * 100 if stats['success'] > 0 else 0
            print(f"    {count} 次: {num:5d} 张 ({pct:5.1f}%)")
        print(f"\n  调用最多的前10张图片:")
        for key, count in tc['top'][:10]:
            short_key = key.split('::')[-1] if '::' in key else key
            print(f"    {count:2d} 次 - {short_key[:50]}")

    # 耗时统计
    if stats['elapsed']:
        el = stats['elapsed']
        print(f"\n【耗时统计】（仅成功）")
        print(f"  总耗时:        {el['total']:.1f}s")
        print(f"  平均耗时:      {el['avg']:.2f}s")
        print(f"  中位数:        {el['median']:.2f}s")
        print(f"  最小/最大:     {el['min']:.2f}s / {el['max']:.2f}s")
        print(f"  标准差:        {el['stdev']:.2f}s")
        for p in args.percentiles:
            print(f"  P{p:02d}:           {el.get(f'p{p}', 0):.2f}s")
        print(f"\n  吞吐量:        {stats['throughput']:.2f} 张/秒")

        # 按工具调用分组
        if stats['elapsed_by_tool_calls']:
            print(f"\n  按工具调用次数分组的平均耗时:")
            for tc, data in sorted(stats['elapsed_by_tool_calls'].items()):
                print(f"    {tc} 次调用: {data['avg']:.2f}s (n={data['count']})")

    # 错误分布
    if stats['error_distribution']:
        print(f"\n【错误分类统计】")
        for error_type, count in sorted(stats['error_distribution'].items(), key=lambda x: -x[1]):
            print(f"  {error_type:20s}: {count:4d}")

    # 线程统计
    if args.threads and stats['thread_stats']:
        print(f"\n【线程详细统计】")
        for tid in sorted(stats['thread_stats'].keys(), key=lambda x: int(x)):
            ts = stats['thread_stats'][tid]
            print(f"  T{tid:>2s}: {ts['count']:4d}张 "
                  f"成功{ts['success']:4d} ({ts['success_rate']:5.1f}%) "
                  f"平均{ts['avg_elapsed']:6.2f}s "
                  f"最大{ts['elapsed_max']:6.2f}s")


def export_csv(sessions: List[Session], output_path: Path):
    """导出详细数据到CSV"""
    with open(output_path, 'w', newline='', encoding='utf-8') as f:
        writer = csv.writer(f)
        writer.writerow(['key', 'thread_id', 'tool_calls', 'status', 'elapsed_sec',
                        'error_type', 'error_msg', 'img_type'])
        for s in sessions:
            writer.writerow([
                s.key, s.tid, s.tool_calls, s.status, s.elapsed or '',
                s.error_type or '', s.error_msg or '', s.img_type or ''
            ])
    print(f"\nCSV已导出: {output_path}")


# =============================================================================
# 主函数
# =============================================================================
def main():
    parser = argparse.ArgumentParser(
        description="分析 img2text 日志，统计工具调用、耗时、成功率等指标"
    )
    parser.add_argument("--all", action="store_true",
                       help="汇总所有历史日志进行分析")
    parser.add_argument("--logfile", type=str,
                       help="指定单个日志文件路径")
    parser.add_argument("--config", default="config.yaml",
                       help="配置文件路径 (默认: config.yaml)")
    parser.add_argument("--threads", action="store_true",
                       help="显示线程详细统计")
    parser.add_argument("--percentiles", type=str, default="90,95,99",
                       help="自定义百分位数，逗号分隔 (默认: 90,95,99)")
    parser.add_argument("--output", type=str,
                       help="导出CSV文件路径")
    args = parser.parse_args()

    # 解析百分位数
    try:
        percentiles = [int(x.strip()) for x in args.percentiles.split(',')]
    except ValueError:
        print(f"错误: 无效的百分位数格式: {args.percentiles}")
        sys.exit(1)
    args.percentiles = percentiles

    # 加载配置
    try:
        config = load_config(args.config)
        outdir = Path(config["paths"]["output_dir"])
    except Exception as e:
        print(f"加载配置失败: {e}")
        sys.exit(1)

    # 确定要分析的日志文件
    log_paths: List[Path] = []
    if args.logfile:
        log_paths = [Path(args.logfile)]
    elif args.all:
        log_paths = find_all_logs(outdir)
        if not log_paths:
            print("未找到任何日志文件")
            sys.exit(1)
        print(f"找到 {len(log_paths)} 个日志文件")
    else:
        latest = find_latest_log(outdir)
        if not latest:
            print("未找到日志文件")
            sys.exit(1)
        log_paths = [latest]

    # 分析所有日志
    all_sessions: List[Session] = []
    for log_path in log_paths:
        sessions = analyze_log(log_path)
        all_sessions.extend(sessions)
        if len(log_paths) > 1:
            print(f"  {log_path.name}: {len(sessions)} 条记录")

    if not all_sessions:
        print("未解析到任何会话数据")
        sys.exit(1)

    # 计算统计
    stats = compute_statistics(all_sessions, percentiles)

    # 打印报告
    print_report(stats, log_paths[-1] if len(log_paths) == 1 else log_paths[0], args)

    # 导出CSV
    if args.output:
        export_csv(all_sessions, Path(args.output))


if __name__ == "__main__":
    main()