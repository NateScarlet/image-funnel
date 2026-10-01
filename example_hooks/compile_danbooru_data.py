#!/usr/bin/env -S uv run
# -*- coding: utf-8 -*-
# /// script
# requires-python = ">=3.11"
# dependencies = [
#   "numpy",
#   "pyarrow",
#   "requests",
# ]
# ///
"""将外部项目源数据（tags CSV + 共现 parquet）编译为补全运行时产物。

用法：
  uv run example_hooks/compile_danbooru_data.py <源数据目录>

- 源数据目录：外部项目数据所在目录（如 DanbooruSearchOnline 的 origin_database）。
- 产物固定写入补全读取路径 ${IMAGE_FUNNEL_DATA_DIR}/danbooru
  （IMAGE_FUNNEL_DATA_DIR 缺失时按 image-funnel 默认 UserConfigDir 回落），
  用户无需知道补全脚本会到哪里读取。
- 识别源输入：优先精确文件名（tags_enhanced.csv / cooccurrence_clean.parquet），
  否则按 schema 扫描目录内变体/重命名文件（tags 需 name 列；共现需 tag_a/tag_b/count）。
- 缺源数据时打印下载地址并以非零退出（下载提示只在本脚本出现）。
- 产物为 tags.bin（Arrow IPC）、cooc.bin（双向 CSR）与 embeddings.bin（标签向量），
  运行时 FileDanbooruTagProvider 只加载这三个文件。
- 标签向量需调用用户提供的 OpenAI 兼容 /v1/embeddings 接口生成：
  端点由 --embedding-provider-url 或 HOOK_AUTOCOMPLETE_EMBEDDING_PROVIDER_URL 指定
  （缺省用本地 LM Studio 默认端点）；接口不可达时快速失败中止编译，不产出半成品。

编译完成后，补全默认数据目录即可启用本地 Danbooru 标签补全
（见 example_hooks/comfyui_add.toml）。
"""

from __future__ import annotations

import argparse
import logging
import os
import sys
from dataclasses import replace
from pathlib import Path

# 作为脚本直接运行时补上 example_hooks 到 import 路径
_HERE = Path(__file__).resolve().parent
if str(_HERE) not in sys.path:
    sys.path.insert(0, str(_HERE))

from comfyui.danbooru_data import (  # noqa: E402
    COMPILED_COOC_FILENAME,
    COMPILED_EMBEDDINGS_FILENAME,
    COMPILED_TAGS_FILENAME,
    SOURCE_DOWNLOAD_HINT,
    compile_dataset,
    default_compile_data_dir,
    find_cooc_source,
    find_tags_source,
)
from comfyui.danbooru_embedding import (  # noqa: E402
    DEFAULT_COMPILE_TIMEOUT_MS,
    DEFAULT_EMBEDDING_BATCH_SIZE,
    DEFAULT_EMBEDDING_MODEL,
    EMBEDDING_PROVIDER_URL_ENV,
    OpenAITagEmbeddingSource,
    OpenAIEmbeddingClient,
    parse_embedding_endpoint,
)

# 编译期批量生成向量耗时远长于交互式单条查询（一次请求 = batch_size 条），
# 故脚本自带的默认端点带更长的 timeoutMs；URL 里显式指定的值一律优先
DEFAULT_COMPILE_PROVIDER_URL = (
    "http://localhost:1234#apiKey="
    f"&model={DEFAULT_EMBEDDING_MODEL}&timeoutMs={DEFAULT_COMPILE_TIMEOUT_MS}"
)


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )
    parser = argparse.ArgumentParser(
        description="编译 Danbooru 标签、共现与标签向量数据为补全运行时专用产物"
    )
    parser.add_argument(
        "source_dir",
        help="源数据目录（外部项目 tags CSV + 共现 parquet 所在处）；"
        "产物写入 ${IMAGE_FUNNEL_DATA_DIR}/danbooru",
    )
    parser.add_argument(
        "--embedding-provider-url",
        default=os.environ.get(EMBEDDING_PROVIDER_URL_ENV, "").strip()
        or DEFAULT_COMPILE_PROVIDER_URL,
        help="OpenAI 兼容 /v1/embeddings 端点，"
        "语法 <base>#apiKey=<k>&model=<m>&timeoutMs=<ms>；"
        f"缺省取 ${EMBEDDING_PROVIDER_URL_ENV} 或 {DEFAULT_COMPILE_PROVIDER_URL}",
    )
    parser.add_argument(
        "--embedding-batch-size",
        type=int,
        default=DEFAULT_EMBEDDING_BATCH_SIZE,
        help=f"标签向量生成的串行批量大小，默认 {DEFAULT_EMBEDDING_BATCH_SIZE}",
    )
    parser.add_argument(
        "--embedding-timeout-ms",
        type=int,
        default=None,
        help="单批嵌入请求的超时毫秒数；不指定时用端点 URL 里的 timeoutMs"
        f"（脚本自带默认端点为 {DEFAULT_COMPILE_TIMEOUT_MS}，"
        "因为批量生成远慢于交互式单条查询）",
    )
    args = parser.parse_args()
    source_dir = Path(args.source_dir)
    output_dir = Path(default_compile_data_dir())

    if not source_dir.is_dir():
        print(f"源数据目录不存在: {source_dir}", file=sys.stderr)
        print(SOURCE_DOWNLOAD_HINT, file=sys.stderr)
        sys.exit(1)

    tags_src = find_tags_source(source_dir)
    cooc_src = find_cooc_source(source_dir)
    if tags_src is None or cooc_src is None:
        missing: list[str] = []
        if tags_src is None:
            missing.append("tags 源 CSV（需含 name 列）")
        if cooc_src is None:
            missing.append("共现源 parquet（需含 tag_a/tag_b/count 列）")
        print(
            f"源数据缺失: {'、'.join(missing)}（目录: {source_dir}）",
            file=sys.stderr,
        )
        print(SOURCE_DOWNLOAD_HINT, file=sys.stderr)
        sys.exit(1)

    parsed = parse_embedding_endpoint(args.embedding_provider_url)
    # 仅在显式指定时覆盖端点 URL 里的 timeoutMs
    endpoint = (
        replace(parsed, timeout=args.embedding_timeout_ms / 1000)
        if args.embedding_timeout_ms is not None
        else parsed
    )
    print(f"tags 源: {tags_src}")
    print(f"共现源: {cooc_src}")
    print(f"产物输出: {output_dir}")
    print(f"嵌入端点: {endpoint.url}（model={endpoint.model}）")
    embedding_source = OpenAITagEmbeddingSource(
        OpenAIEmbeddingClient(endpoint), batch_size=args.embedding_batch_size
    )
    compiled = compile_dataset(source_dir, output_dir, embedding_source)
    print(f"编译完成: {compiled.tags}")
    print(f"编译完成: {compiled.cooc}")
    print(f"编译完成: {compiled.embeddings}")
    print(
        f"产物文件名: {COMPILED_TAGS_FILENAME}, {COMPILED_COOC_FILENAME}, "
        f"{COMPILED_EMBEDDINGS_FILENAME}；"
        "补全将从该输出目录读取以启用本地 Danbooru 标签补全。"
    )


if __name__ == "__main__":
    main()
