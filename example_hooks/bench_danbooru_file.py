# -*- coding: utf-8 -*-
"""FileDanbooruTagProvider 延迟与内存基准（编译产物加载路径）。

内存与延迟分阶段测：tracemalloc 仅包住加载/采样阶段，避免拖慢热路径计时。

用法（在 worktree 根目录，先用源数据目录运行 compile_danbooru_data.py 生成产物）：
  .venv\\Scripts\\python.exe example_hooks\\bench_danbooru_file.py ^
    --data-dir <含 tags.bin 与 cooc.bin 的目录> --label after --out .scratch/bench-after.json
"""

from __future__ import annotations

import argparse
import json
import statistics
import sys
import time
import tracemalloc
from pathlib import Path
from typing import Any, Callable, Dict, List

# 作为脚本直接运行时补上 example_hooks 到 import 路径
_HERE = Path(__file__).resolve().parent
if str(_HERE) not in sys.path:
    sys.path.insert(0, str(_HERE))

from comfyui.danbooru import FileDanbooruTagProvider  # noqa: E402


def _rss_mb() -> float:
    """Windows 工作集大小（MB）；失败时返回 0。"""
    try:
        import ctypes
        from ctypes import wintypes

        class PROCESS_MEMORY_COUNTERS(ctypes.Structure):  # type: ignore[misc]
            _fields_ = [
                ("cb", wintypes.DWORD),
                ("PageFaultCount", wintypes.DWORD),
                ("PeakWorkingSetSize", ctypes.c_size_t),
                ("WorkingSetSize", ctypes.c_size_t),
                ("QuotaPeakPagedPoolUsage", ctypes.c_size_t),
                ("QuotaPagedPoolUsage", ctypes.c_size_t),
                ("QuotaPeakNonPagedPoolUsage", ctypes.c_size_t),
                ("QuotaNonPagedPoolUsage", ctypes.c_size_t),
                ("PagefileUsage", ctypes.c_size_t),
                ("PeakPagefileUsage", ctypes.c_size_t),
            ]

        counters = PROCESS_MEMORY_COUNTERS()
        counters.cb = ctypes.sizeof(counters)
        # 显式声明原型，避免 ctypes 默认 int 截断句柄导致调用失败
        psapi = ctypes.WinDLL("psapi", use_last_error=True)
        kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
        get_current_process = kernel32.GetCurrentProcess
        get_current_process.restype = ctypes.c_void_p
        get_process_memory_info = psapi.GetProcessMemoryInfo
        get_process_memory_info.argtypes = [
            ctypes.c_void_p,
            ctypes.POINTER(PROCESS_MEMORY_COUNTERS),
            ctypes.c_uint32,
        ]
        get_process_memory_info.restype = wintypes.BOOL
        ok = get_process_memory_info(
            get_current_process(),
            ctypes.byref(counters),
            ctypes.sizeof(counters),
        )
        if not ok:
            return 0.0
        return counters.WorkingSetSize / 1024 / 1024
    except Exception:
        return 0.0


def _mem(label: str, tracemalloc_on: bool) -> Dict[str, Any]:
    current, peak = tracemalloc.get_traced_memory()
    snap: Dict[str, Any] = {
        "stage": label,
        "rss_mb": round(_rss_mb(), 1),
        "tracemalloc_current_mb": (
            round(current / 1024 / 1024, 1) if tracemalloc_on else None
        ),
        "tracemalloc_peak_mb": round(peak / 1024 / 1024, 1) if tracemalloc_on else None,
    }
    return snap


def _time_one(fn: Callable[[], Any]) -> float:
    t0 = time.perf_counter()
    fn()
    return (time.perf_counter() - t0) * 1000


def _time_median(fn: Callable[[], Any], rounds: int) -> float:
    samples: List[float] = [_time_one(fn) for _ in range(rounds)]
    return statistics.median(samples)


def main() -> None:
    parser = argparse.ArgumentParser(description="FileDanbooruTagProvider 基准")
    parser.add_argument("--data-dir", required=True)
    parser.add_argument("--label", required=True, help="before / after 等标签")
    parser.add_argument("--out", required=True, help="JSON 结果输出路径")
    parser.add_argument("--rounds", type=int, default=7)
    args = parser.parse_args()

    search_queries = [
        ("exact", "1girl"),
        ("prefix", "white"),
        ("substring", "hair"),
        ("typo", "whit hair"),
        ("chinese", "白发"),
        ("single_char", "g"),
        ("miss", "zzzz_no_such_tag_zzzz"),
    ]
    related_cases = [
        ("multi_seed", ["1girl", "white_hair", "masterpiece"]),
        ("single_seed", ["1girl"]),
    ]

    result: Dict[str, Any] = {
        "label": args.label,
        "data_dir": str(Path(args.data_dir).resolve()),
        "python": sys.version.split()[0],
        "rounds": args.rounds,
        "stages": [],
    }

    # ---- 阶段1：构造 + 首次 search/related（tracemalloc 覆盖懒加载全过程）----
    tracemalloc.start()
    result["stages"].append(_mem("baseline_import", True))
    t0 = time.perf_counter()
    provider = FileDanbooruTagProvider(args.data_dir, show_nsfw=True)
    construct_ms = (time.perf_counter() - t0) * 1000
    result["construct_ms"] = round(construct_ms, 1)
    result["stages"].append(_mem("after_construct", True))

    first_search_ms = _time_one(lambda: provider.search("1girl"))
    result["first_search_ms"] = round(first_search_ms, 1)
    result["stages"].append(_mem("after_first_search", True))

    first_related_tags = related_cases[0][1]
    first_related_ms = _time_one(lambda: provider.related(list(first_related_tags)))
    result["first_related_ms"] = round(first_related_ms, 1)
    result["stages"].append(_mem("after_first_related", True))
    tracemalloc.stop()

    # ---- 阶段2：延迟热路径（关闭 tracemalloc）----
    search_lat: Dict[str, float] = {}
    for name, q in search_queries:
        search_lat[name] = round(
            _time_median(lambda query=q: provider.search(query), args.rounds), 1
        )
    result["search_median_ms"] = search_lat
    result["search_max_ms"] = max(search_lat.values())
    result["search_all_under_100ms"] = all(v <= 100 for v in search_lat.values())

    related_lat: Dict[str, float] = {}
    for name, tags in related_cases:
        related_lat[name] = round(
            _time_median(lambda t=tags: provider.related(list(t)), args.rounds), 1
        )
    result["related_median_ms"] = related_lat
    result["related_max_ms"] = max(related_lat.values())

    # 正确性抽查（不参与计时）
    result["sample_search_count"] = len(provider.search("1girl"))
    result["sample_related_count"] = len(provider.related(["1girl"]))
    result["final_rss_mb"] = round(_rss_mb(), 1)

    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(
        json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
