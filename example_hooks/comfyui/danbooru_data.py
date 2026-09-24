# -*- coding: utf-8 -*-
"""Danbooru 本地数据编译：源 CSV/parquet → 运行时专用二进制产物。

编译由独立脚本手动触发（option C）：
  uv run example_hooks/compile_danbooru_data.py <源数据目录>

源数据目录是外部项目的数据（如 DanbooruSearchOnline 的 origin_database）；
产物固定写入补全读取的 ${IMAGE_FUNNEL_DATA_DIR}/danbooru
（IMAGE_FUNNEL_DATA_DIR 缺失时按 image-funnel 默认 UserConfigDir 回落），
用户无需知道补全脚本从哪里读。

运行时 FileDanbooruTagProvider 只加载编译产物，不再读取源文件；
源文件识别（含变体文件名/重命名）与下载提示均在编译路径。
"""

from __future__ import annotations

import csv
import io
import os
import struct
import sys
from array import array
from dataclasses import dataclass
from pathlib import Path
from typing import (
    Any,
    Dict,
    FrozenSet,
    Iterable,
    List,
    Optional,
    Sequence,
    Set,
    Tuple,
    cast,
)

# #region 编译产物文件名与格式

# 脚本定义的规范产物名（与外部项目源文件名解耦）
COMPILED_TAGS_FILENAME = "tags.bin"
COMPILED_COOC_FILENAME = "cooc.bin"

_COOC_MAGIC = b"DNCC"
_COOC_VERSION = 1
# header: magic(4) + version(u32le) + n_nodes(u32le) + n_edges(u32le)
_COOC_HEADER = struct.Struct("<4sIII")

# 编译失败时给用户的下载指引（仅编译脚本路径使用）
SOURCE_DOWNLOAD_HINT = (
    "请从以下来源下载源数据并放入该目录：\n"
    "  GitHub:     https://github.com/SuzumiyaAkizuki/DanbooruSearchOnline "
    "(origin_database/tags_enhanced.csv, origin_database/cooccurrence_clean.parquet)\n"
    "  HuggingFace: https://huggingface.co/spaces/SAkizuki/DanbooruSearch\n"
    "  ModelScope:  https://www.modelscope.cn/studios/SAkizuki/DanbooruSearchOnline"
)

# 运行时缺编译产物的提示：指向编译脚本（下载指引已移到脚本）
COMPILE_SCRIPT_HINT = (
    "请先运行编译脚本生成本地数据：\n"
    "  uv run example_hooks/compile_danbooru_data.py <源数据目录>\n"
    "源数据下载地址见编译脚本缺文件时的输出；"
    "产物写入 ${IMAGE_FUNNEL_DATA_DIR}/danbooru。"
)

# #endregion

# #region 源数据识别（含变体文件名）

# 精确文件名优先，其次按 schema 扫描重命名/变体输入
_TAGS_SOURCE_PREFERRED = (
    "tags_enhanced.csv",
    "tags.csv",
    "danbooru_tags.csv",
)
_COOC_SOURCE_PREFERRED = (
    "cooccurrence_clean.parquet",
    "cooccurrence.parquet",
    "cooc.parquet",
    "tag_cooc.parquet",
)

# schema 探测所需列（变体输入按内容识别）
_TAGS_REQUIRED_COLUMNS: FrozenSet[str] = frozenset({"name"})
_COOC_REQUIRED_COLUMNS: FrozenSet[str] = frozenset({"tag_a", "tag_b", "count"})


def find_tags_source(data_dir: Path) -> Optional[Path]:
    """在数据目录中识别 tags 源 CSV：精确名优先，再按 name 列扫描。"""
    for name in _TAGS_SOURCE_PREFERRED:
        p = data_dir / name
        if p.is_file():
            return p
    if not data_dir.is_dir():
        return None
    for p in sorted(data_dir.glob("*.csv")):
        if _csv_has_columns(p, _TAGS_REQUIRED_COLUMNS):
            return p
    return None


def find_cooc_source(data_dir: Path) -> Optional[Path]:
    """在数据目录中识别共现源 parquet：精确名优先，再按必需列扫描。"""
    for name in _COOC_SOURCE_PREFERRED:
        p = data_dir / name
        if p.is_file():
            return p
    if not data_dir.is_dir():
        return None
    for p in sorted(data_dir.glob("*.parquet")):
        if _parquet_has_columns(p, _COOC_REQUIRED_COLUMNS):
            return p
    return None


def _read_csv_text(path: Path) -> str:
    """按候选编码依次解码 CSV（发布数据可能为非 UTF-8）。"""
    raw = path.read_bytes()
    for encoding in ("utf-8-sig", "utf-8", "gbk", "gb18030"):
        try:
            return raw.decode(encoding)
        except UnicodeDecodeError:
            continue
    raise UnicodeDecodeError("utf-8", raw, 0, len(raw), f"无法识别 CSV 编码: {path}")


def _csv_has_columns(path: Path, required: FrozenSet[str]) -> bool:
    try:
        text = _read_csv_text(path)
    except UnicodeDecodeError:
        return False
    reader = csv.reader(io.StringIO(text))
    try:
        header = next(reader)
    except StopIteration:
        return False
    return required.issubset({(c or "").strip() for c in header})


def _parquet_has_columns(path: Path, required: FrozenSet[str]) -> bool:
    import pyarrow.parquet as pq  # pyright: ignore[reportMissingTypeStubs]

    pq_mod = cast(Any, pq)
    try:
        schema = pq_mod.read_schema(path)
        names = cast(List[str], schema.names)
    except Exception:
        return False
    return required.issubset(set(names))


# #endregion

# #region 编译写出

# 源 tags 的 category 数字编码 → 与在线服务一致的字符串类目
_SOURCE_CATEGORY_MAP = {
    "0": "General",
    "1": "Artist",
    "3": "Copyright",
    "4": "Character",
    "5": "Meta",
}


@dataclass(frozen=True)
class SourceTagRow:
    """编译期单条标签（字段已归一化）。"""

    tag: str
    cn_name: str
    wiki: str
    post_count: int
    category: str
    nsfw: bool
    name_norm: str
    cn_aliases_norm: Tuple[str, ...]


def normalize_literal(text: str) -> str:
    """字面匹配用归一化：小写、下划线转空格、折叠空白。"""
    return " ".join(text.replace("_", " ").lower().split())


def split_cn_aliases(cn_name: str) -> List[str]:
    """中文名按常见分隔符拆成多个别名。"""
    parts: List[str] = []
    for chunk in cn_name.replace("，", ",").replace("、", ",").split(","):
        chunk = chunk.strip()
        if chunk:
            parts.append(chunk)
    return parts


def read_source_tags(path: Path) -> List[SourceTagRow]:
    """读入 tags 源 CSV 并完成归一化（编码回退 + category 映射）。"""
    records: List[SourceTagRow] = []
    seen: Set[str] = set()
    text = _read_csv_text(path)
    reader = csv.DictReader(io.StringIO(text))
    for row in reader:
        name = (row.get("name") or "").strip()
        if not name or name in seen:
            continue
        seen.add(name)
        category_raw = (row.get("category") or "0").strip()
        category = _SOURCE_CATEGORY_MAP.get(category_raw, "General")
        cn_name = (row.get("cn_name") or "").strip()
        nsfw = (row.get("nsfw") or "0").strip() == "1"
        try:
            post_count = int((row.get("post_count") or "0").strip() or "0")
        except ValueError:
            post_count = 0
        records.append(
            SourceTagRow(
                tag=name,
                cn_name=cn_name,
                wiki=(row.get("wiki") or "").strip(),
                post_count=post_count,
                category=category,
                nsfw=nsfw,
                name_norm=normalize_literal(name),
                cn_aliases_norm=tuple(
                    normalize_literal(a) for a in split_cn_aliases(cn_name)
                ),
            )
        )
    return records


def read_source_cooc(path: Path) -> List[Tuple[str, str, int]]:
    """读入共现源 parquet 为 (tag_a, tag_b, count) 列表（校验必需列）。"""
    import pyarrow.parquet as pq  # pyright: ignore[reportMissingTypeStubs]

    pq_mod = cast(Any, pq)
    table = pq_mod.read_table(path)
    cols = set(cast(List[str], table.column_names))
    missing = _COOC_REQUIRED_COLUMNS - cols
    if missing:
        raise ValueError(f"共现数据缺少必需列 {missing}: {path}")
    a_vals = cast(List[Any], table.column("tag_a").to_pylist())
    b_vals = cast(List[Any], table.column("tag_b").to_pylist())
    c_vals = cast(List[Any], table.column("count").to_pylist())
    pairs: List[Tuple[str, str, int]] = []
    for a_raw, b_raw, c_raw in zip(a_vals, b_vals, c_vals):
        a = str(a_raw) if a_raw is not None else ""
        b = str(b_raw) if b_raw is not None else ""
        if not a or not b:
            continue
        try:
            count = int(c_raw)
        except (TypeError, ValueError):
            continue
        pairs.append((a, b, count))
    return pairs


def write_compiled_tags(path: Path, rows: Sequence[SourceTagRow]) -> None:
    """将归一化标签行写为 Arrow IPC（含预计算 norm 字段）。"""
    import pyarrow as pa  # pyright: ignore[reportMissingTypeStubs]
    import pyarrow.ipc as ipc  # pyright: ignore[reportMissingTypeStubs]

    pa_mod = cast(Any, pa)
    ipc_mod = cast(Any, ipc)
    table = pa_mod.table(
        {
            "tag": pa_mod.array([r.tag for r in rows], type=pa_mod.string()),
            "cn_name": pa_mod.array([r.cn_name for r in rows], type=pa_mod.string()),
            "wiki": pa_mod.array([r.wiki for r in rows], type=pa_mod.string()),
            "post_count": pa_mod.array(
                [r.post_count for r in rows], type=pa_mod.int64()
            ),
            "category": pa_mod.array([r.category for r in rows], type=pa_mod.string()),
            "nsfw": pa_mod.array([r.nsfw for r in rows], type=pa_mod.bool_()),
            "name_norm": pa_mod.array(
                [r.name_norm for r in rows], type=pa_mod.string()
            ),
            "cn_aliases_norm": pa_mod.array(
                [list(r.cn_aliases_norm) for r in rows],
                type=pa_mod.list_(pa_mod.string()),
            ),
        }
    )
    sink = pa_mod.OSFile(str(path), "wb")
    try:
        writer = ipc_mod.new_file(sink, table.schema)
        try:
            writer.write_table(table)
        finally:
            writer.close()
    finally:
        sink.close()


def build_cooc_arrays(
    tag_names: Sequence[str],
    pairs: Iterable[Tuple[str, str, int]],
) -> Tuple["array[int]", "array[int]", "array[int]"]:
    """由共现对构建以 tag 下标为节点的双向 CSR（合并同向重复边）。

    - 任一端不在 tags 表中的边丢弃（运行时无法解析为标签详情）
    - count<=0 的边丢弃
    - 同向多行 count 累加；邻居按 id 升序保证产物确定性
    """
    id_by_name = {name: i for i, name in enumerate(tag_names)}
    n_nodes = len(tag_names)
    # 邻接先用 dict 聚合，再压实为 CSR
    adj: List[Dict[int, int]] = [dict() for _ in range(n_nodes)]
    for a, b, count in pairs:
        if count <= 0:
            continue
        ia = id_by_name.get(a)
        ib = id_by_name.get(b)
        if ia is None or ib is None or ia == ib:
            continue
        # 源表每对一行（可能仅存一个方向）；related 双向扫语义 → 两端都挂边
        adj[ia][ib] = adj[ia].get(ib, 0) + count
        adj[ib][ia] = adj[ib].get(ia, 0) + count

    n_edges = sum(len(d) for d in adj)
    offsets: array[int] = array("I", bytes(4 * (n_nodes + 1)))
    neighbors: array[int] = array("I", bytes(4 * n_edges))
    counts: array[int] = array("I", bytes(4 * n_edges))
    pos = 0
    for node, d in enumerate(adj):
        offsets[node] = pos
        for nb in sorted(d):
            neighbors[pos] = nb
            # count 语义为非负出现次数；异常大值钳制到 uint32 上限
            c = d[nb]
            counts[pos] = c if c <= 0xFFFFFFFF else 0xFFFFFFFF
            pos += 1
    offsets[n_nodes] = pos
    return offsets, neighbors, counts


def write_compiled_cooc(
    path: Path,
    n_nodes: int,
    offsets: "array[int]",
    neighbors: "array[int]",
    counts: "array[int]",
) -> None:
    """写出 CSR 二进制：header + offsets + neighbors + counts（uint32 LE）。"""
    n_edges = len(neighbors)
    if len(offsets) != n_nodes + 1:
        raise ValueError(f"offsets 长度应为 n_nodes+1: {len(offsets)}")
    if len(counts) != n_edges:
        raise ValueError("counts 与 neighbors 长度不一致")
    if offsets[-1] != n_edges:
        raise ValueError("offsets[-1] 应等于边数")
    header = _COOC_HEADER.pack(_COOC_MAGIC, _COOC_VERSION, n_nodes, n_edges)
    with path.open("wb") as f:
        f.write(header)
        f.write(offsets.tobytes())
        f.write(neighbors.tobytes())
        f.write(counts.tobytes())


def compile_dataset(
    source_dir: str | Path, output_dir: str | Path
) -> Tuple[Path, Path]:
    """将源数据目录编译为运行时产物写入 output_dir；缺源时抛出含下载指引的错误。

    source_dir：外部项目源数据（tags CSV + 共现 parquet）所在目录。
    output_dir：补全读取的编译产物目录（通常为 default_compile_data_dir()）。
    返回 (tags 产物路径, cooc 产物路径)。
    """
    root = Path(source_dir)
    if not root.is_dir():
        raise FileNotFoundError(f"源数据目录不存在: {root}\n{SOURCE_DOWNLOAD_HINT}")

    tags_src = find_tags_source(root)
    cooc_src = find_cooc_source(root)
    missing: List[str] = []
    if tags_src is None:
        missing.append("tags 源 CSV（需含 name 列）")
    if cooc_src is None:
        missing.append("共现源 parquet（需含 tag_a/tag_b/count 列）")
    if missing or tags_src is None or cooc_src is None:
        raise FileNotFoundError(
            f"源数据缺失: {'、'.join(missing)}（目录: {root}）\n"
            f"{SOURCE_DOWNLOAD_HINT}"
        )

    rows = read_source_tags(tags_src)
    if not rows:
        raise ValueError(f"tags 源无有效数据行: {tags_src}")
    pairs = read_source_cooc(cooc_src)
    offsets, neighbors, counts = build_cooc_arrays([r.tag for r in rows], pairs)

    out_root = Path(output_dir)
    out_root.mkdir(parents=True, exist_ok=True)
    tags_out = out_root / COMPILED_TAGS_FILENAME
    cooc_out = out_root / COMPILED_COOC_FILENAME
    write_compiled_tags(tags_out, rows)
    write_compiled_cooc(cooc_out, len(rows), offsets, neighbors, counts)
    return tags_out, cooc_out


# #endregion

# #region 运行时加载


@dataclass(frozen=True)
class CompiledTagRecord:
    """运行时单条标签（含预计算归一化字段；rec_id 即 CSR 节点下标）。"""

    rec_id: int
    tag: str
    cn_name: str
    wiki: str
    post_count: int
    category: str
    nsfw: bool
    name_norm: str
    cn_aliases_norm: Tuple[str, ...]


class CompiledCoocIndex:
    """双向 CSR 共现索引：related 按种子度数遍历，不做全表扫。"""

    def __init__(
        self,
        offsets: "array[int]",
        neighbors: "array[int]",
        counts: "array[int]",
    ) -> None:
        self.offsets = offsets
        self.neighbors = neighbors
        self.counts = counts

    @property
    def n_nodes(self) -> int:
        return len(self.offsets) - 1

    @property
    def n_edges(self) -> int:
        return len(self.neighbors)

    def aggregate(self, seed_ids: Set[int]) -> Dict[int, int]:
        """聚合种子邻居计数（排除仍在种子集合内的节点）；多 seed 跨 seed 求和。"""
        if not seed_ids:
            return {}
        offsets = self.offsets
        neighbors = self.neighbors
        counts = self.counts
        scores: Dict[int, int] = {}
        for sid in seed_ids:
            if sid < 0 or sid >= self.n_nodes:
                continue
            start = offsets[sid]
            end = offsets[sid + 1]
            for i in range(start, end):
                nb = neighbors[i]
                if nb in seed_ids:
                    continue
                scores[nb] = scores.get(nb, 0) + counts[i]
        return scores


def load_compiled_tags(path: Path) -> List[CompiledTagRecord]:
    """读入 tags 编译产物（Arrow IPC）。"""
    import pyarrow.ipc as ipc  # pyright: ignore[reportMissingTypeStubs]

    ipc_mod = cast(Any, ipc)
    with ipc_mod.open_file(str(path)) as reader:
        table = reader.read_all()
    tags = cast(List[Any], table.column("tag").to_pylist())
    cn_names = cast(List[Any], table.column("cn_name").to_pylist())
    wiki = cast(List[Any], table.column("wiki").to_pylist())
    post_counts = cast(List[Any], table.column("post_count").to_pylist())
    categories = cast(List[Any], table.column("category").to_pylist())
    nsfws = cast(List[Any], table.column("nsfw").to_pylist())
    name_norms = cast(List[Any], table.column("name_norm").to_pylist())
    aliases_list = cast(List[Any], table.column("cn_aliases_norm").to_pylist())
    records: List[CompiledTagRecord] = []
    for i in range(len(tags)):
        aliases_raw = aliases_list[i]
        aliases: Tuple[str, ...]
        if aliases_raw is None:
            aliases = ()
        else:
            aliases = tuple(str(a) for a in cast(Iterable[Any], aliases_raw))
        records.append(
            CompiledTagRecord(
                rec_id=i,
                tag=str(tags[i]),
                cn_name=str(cn_names[i] or ""),
                wiki=str(wiki[i] or ""),
                post_count=int(post_counts[i] or 0),
                category=str(categories[i] or "General"),
                nsfw=bool(nsfws[i]),
                name_norm=str(name_norms[i] or ""),
                cn_aliases_norm=aliases,
            )
        )
    return records


def load_compiled_cooc(path: Path) -> CompiledCoocIndex:
    """读入共现 CSR 产物；魔数/版本/长度不符时快速失败。"""
    data = path.read_bytes()
    if len(data) < _COOC_HEADER.size:
        raise ValueError(f"共现产物过短: {path}")
    magic, version, n_nodes, n_edges = _COOC_HEADER.unpack_from(data, 0)
    if magic != _COOC_MAGIC:
        raise ValueError(f"共现产物格式不符: {path}")
    if version != _COOC_VERSION:
        raise ValueError(
            f"共现产物版本不支持: {version}（期望 {_COOC_VERSION}）: {path}"
        )
    header_size = _COOC_HEADER.size
    offsets_bytes = (n_nodes + 1) * 4
    edges_bytes = n_edges * 8
    expected = header_size + offsets_bytes + edges_bytes
    if len(data) != expected:
        raise ValueError(
            f"共现产物长度不符: 期望 {expected} 字节，实际 {len(data)}: {path}"
        )
    offsets: array[int] = array("I")
    offsets.frombytes(data[header_size : header_size + offsets_bytes])
    neighbors: array[int] = array("I")
    nb_start = header_size + offsets_bytes
    neighbors.frombytes(data[nb_start : nb_start + n_edges * 4])
    counts: array[int] = array("I")
    counts.frombytes(data[nb_start + n_edges * 4 :])
    if offsets[-1] != n_edges:
        raise ValueError(f"共现产物 offsets 与边数不一致: {path}")
    return CompiledCoocIndex(offsets, neighbors, counts)


def has_compiled_dataset(data_dir: str | Path) -> bool:
    """目录内是否同时存在 tags 与 cooc 编译产物。"""
    root = Path(data_dir)
    return (root / COMPILED_TAGS_FILENAME).is_file() and (
        root / COMPILED_COOC_FILENAME
    ).is_file()


# #endregion


def resolve_image_funnel_data_dir() -> str:
    """解析主应用数据根目录：IMAGE_FUNNEL_DATA_DIR，缺省回落 image-funnel 默认。

    与 cmd/server/config.go 一致：env 为空时用 UserConfigDir/io.github.natescarlet.image-funnel。
    供编译脚本使用——编译不一定在钩子注入环境下运行，可能没有 IMAGE_FUNNEL_DATA_DIR。
    补全入口不走此回退（钩子保证 IMAGE_FUNNEL_DATA_DIR 存在），见 resolve_danbooru_data_dir。
    """
    env = os.environ.get("IMAGE_FUNNEL_DATA_DIR", "").strip()
    if env:
        return env
    return os.path.join(_user_config_dir(), "io.github.natescarlet.image-funnel")


def _user_config_dir() -> str:
    """跨平台复刻 Go os.UserConfigDir；失败时与主应用一致回落当前目录。"""
    if sys.platform == "win32":
        appdata = os.environ.get("APPDATA", "").strip()
        return appdata if appdata else "."
    if sys.platform == "darwin":
        return str(Path.home() / "Library" / "Application Support")
    xdg = os.environ.get("XDG_CONFIG_HOME", "").strip()
    if xdg:
        return xdg
    home = os.environ.get("HOME", "").strip()
    if home:
        return str(Path(home) / ".config")
    return "."


def resolve_danbooru_data_dir() -> str:
    """补全入口解析本地 Danbooru 数据目录（入口调用，非 provider 内部）。

    - DANBOORU_DATA_DIR 显式非空 → 直接使用（缺产物时 provider 快速失败）
    - 否则默认 ${IMAGE_FUNNEL_DATA_DIR}/danbooru，且仅当编译产物存在时启用
    - 均不可用 → 返回空串（回落在线 URL 链路或跳过 Danbooru）

    补全一定在 IMAGE_FUNNEL_DATA_DIR 存在的环境调用，故不在此做 UserConfigDir 回退。
    """
    explicit = os.environ.get("DANBOORU_DATA_DIR", "").strip()
    if explicit:
        return explicit
    base = os.environ.get("IMAGE_FUNNEL_DATA_DIR", "").strip()
    if not base:
        return ""
    default_dir = os.path.join(base, "danbooru")
    return default_dir if has_compiled_dataset(default_dir) else ""


def default_compile_data_dir() -> str:
    """编译产物输出目录（与补全默认读取路径对接）：${IMAGE_FUNNEL_DATA_DIR}/danbooru。

    IMAGE_FUNNEL_DATA_DIR 缺失时按 image-funnel 默认（UserConfigDir）回落，
    因为编译脚本可能没有钩子注入的环境变量。
    """
    return os.path.join(resolve_image_funnel_data_dir(), "danbooru")
