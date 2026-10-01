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
  （缺省用本地 LM Studio 默认端点）；接口不可达时快速失败中止编译，不产出半成品，
  并提示「配置嵌入服务」或「--no-embedding 跳过向量」两条出路。
- 向量产物先写 embeddings.bin.new 再原子重命名接管：正式产物被常驻补全进程
  mmap 占用而无法覆盖时，已生成的向量完整保留在 .new 文件里，退出 image-funnel
  后手动重命名即可生效，不必重新生成。
- 源数据更新后自动增量重编译：旧产物即缓存，探测确认当前端点能复现旧向量后，
  文本未变的直接搬旧向量，只把新增/改动的文本发给接口（实测全量 48 分钟 → 22 秒）。
- 没有嵌入向量服务也能编译：用 --no-embedding 只产出字面匹配所需的
  tags.bin 与 cooc.bin，并移除已有的向量产物（避免旧向量与新标签表行序错位）。
- 编译成功不代表语义层已启用：还需要在钩子配置里设置
  HOOK_AUTOCOMPLETE_EMBEDDING_PROVIDER_URL，脚本成功后会提示这一点。

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
from typing import Optional

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
    open_vector_cache,
)
from comfyui.danbooru_embedding import (  # noqa: E402
    DEFAULT_COMPILE_PROVIDER_URL,
    DEFAULT_COMPILE_TIMEOUT_MS,
    DEFAULT_EMBEDDING_BATCH_SIZE,
    EMBEDDING_PROVIDER_URL_ENV,
    EmbeddingEndpoint,
    EmbeddingError,
    NullTagEmbeddingSource,
    OpenAITagEmbeddingSource,
    OpenAIEmbeddingClient,
    ReusingTagEmbeddingSource,
    TagEmbeddingSource,
    embedding_failure_hint,
    parse_embedding_endpoint,
    semantic_layer_enable_hint,
    staged_embeddings_path,
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
    parser.add_argument(
        "--no-embedding",
        action="store_true",
        help="不生成标签向量：只编译字面匹配所需的标签与共现产物，"
        "并移除已有的向量产物（没有嵌入向量服务时用这个）",
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

    print(f"tags 源: {tags_src}")
    print(f"共现源: {cooc_src}")
    print(f"产物输出: {output_dir}")

    embedding_source: TagEmbeddingSource
    endpoint: Optional[EmbeddingEndpoint] = None
    if args.no_embedding:
        print("标签向量: 跳过（--no-embedding）")
        # --no-embedding 只清正式产物；上次编译遗留的暂存向量属于用户成果，不动它
        pending = staged_embeddings_path(output_dir)
        if pending.is_file():
            print(f"提示: 存在未接管的向量产物 {pending}（本次未改动它）")
        embedding_source = NullTagEmbeddingSource()
    else:
        try:
            parsed = parse_embedding_endpoint(args.embedding_provider_url)
        except ValueError as e:
            print(f"嵌入服务 URL 配置错误: {e}", file=sys.stderr)
            sys.exit(1)
        # 仅在显式指定时覆盖端点 URL 里的 timeoutMs
        endpoint = (
            replace(parsed, timeout=args.embedding_timeout_ms / 1000)
            if args.embedding_timeout_ms is not None
            else parsed
        )
        print(f"嵌入端点: {endpoint.url}（model={endpoint.model}）")
        # 已有产物即缓存：探测确认当前端点能复现旧向量后，未变文本直接搬旧向量，
        # 只把新增/改动的文本发给接口
        cache = open_vector_cache(output_dir)
        if cache is None:
            embedding_source: TagEmbeddingSource = OpenAITagEmbeddingSource(
                OpenAIEmbeddingClient(endpoint), batch_size=args.embedding_batch_size
            )
        else:
            embedding_source = ReusingTagEmbeddingSource(
                OpenAIEmbeddingClient(endpoint),
                cache,
                batch_size=args.embedding_batch_size,
            )

    try:
        compiled = compile_dataset(source_dir, output_dir, embedding_source)
    except EmbeddingError as e:
        # 快速失败中止编译（不产出半成品），但要告诉用户下一步怎么走
        print(f"标签向量生成失败: {e}", file=sys.stderr)
        assert endpoint is not None
        print(embedding_failure_hint(endpoint), file=sys.stderr)
        sys.exit(1)
    except (PermissionError, FileExistsError) as e:
        # 产物被占用 / 遗留未接管产物：消息里已带恢复步骤（暂存文件在哪、怎么重命名），
        # 不要再叠一层 traceback 把提示埋掉
        print(str(e), file=sys.stderr)
        sys.exit(1)

    print(f"编译完成: {compiled.tags}")
    print(f"编译完成: {compiled.cooc}")
    if compiled.embeddings is not None:
        print(f"编译完成: {compiled.embeddings}")
        print(
            f"产物文件名: {COMPILED_TAGS_FILENAME}, {COMPILED_COOC_FILENAME}, "
            f"{COMPILED_EMBEDDINGS_FILENAME}；"
            "补全将从该输出目录读取以启用本地 Danbooru 标签补全。"
        )
        # 编译产物齐备 ≠ 语义层已启用：还差补全侧的环境变量
        assert endpoint is not None
        print(semantic_layer_enable_hint(endpoint))
    else:
        print(
            f"产物文件名: {COMPILED_TAGS_FILENAME}, {COMPILED_COOC_FILENAME}；"
            "补全将从该输出目录读取以启用本地 Danbooru 标签补全"
            "（仅字面匹配：按标签名/中文名/前缀/子串/笔误匹配，不做语义检索）。"
        )


if __name__ == "__main__":
    main()
