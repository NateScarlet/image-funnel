# -*- coding: utf-8 -*-
"""Danbooru 补全的入口级环境接线。

搜索地址、NSFW 开关、本地数据目录解析、嵌入端点，以及承载在线链路结果缓存的
目录级 SQLite 上下文，都只在最外层入口解析/打开一次。comfyui 补全入口与
`/branch` 补全入口共用本模块，避免两侧各写一份环境接线后逐渐漂移；provider
本身仍由最外层构建并注入给补全逻辑。
"""

import os
from contextlib import contextmanager
from dataclasses import dataclass
from typing import Generator, Optional

from .danbooru import (
    DanbooruTagProvider,
    SQLiteDanbooruCache,
    build_danbooru_tag_provider,
)
from .danbooru_data import resolve_danbooru_data_dir
from .danbooru_embedding import EMBEDDING_PROVIDER_URL_ENV
from .db import SQLiteContext

# 与 SQLiteContext.from_env 使用同一个数据库文件：同一目录下的历史与 Danbooru
# 缓存共享这份 SQLite。
_HOOK_DB_FILENAME = ".io.github.natescarlet.hook.db"


@dataclass(frozen=True)
class DanbooruEnv:
    """入口解析出的 Danbooru 依赖配置。

    data_dir 与 search_url 皆空表示「未配置任何数据来源」，补全退化为纯目录
    补全（comfyui 侧则不注册 Danbooru provider）。
    """

    search_url: str = ""
    show_nsfw: bool = False
    data_dir: str = ""
    embedding_provider_url: str = ""

    @property
    def enabled(self) -> bool:
        return bool(self.data_dir or self.search_url)


def danbooru_env_from_env() -> DanbooruEnv:
    """从 spawn 注入的环境变量解析一次入口级配置。"""
    return DanbooruEnv(
        search_url=os.environ.get("DANBOORU_SEARCH_URL", "").strip(),
        show_nsfw=os.environ.get("DANBOORU_SEARCH_INCLUDE_NSFW", "false").lower()
        in ("true", "1", "yes", "on"),
        data_dir=resolve_danbooru_data_dir(),
        embedding_provider_url=os.environ.get(EMBEDDING_PROVIDER_URL_ENV, "").strip(),
    )


def hook_db_path(root_dir: str, directory_rel_path: str) -> str:
    """目录级 SQLite 数据库文件路径（在线链路缓存与操作历史共用）。"""
    return os.path.join(root_dir, directory_rel_path, _HOOK_DB_FILENAME)


def build_tag_provider(
    env: DanbooruEnv, store: SQLiteContext
) -> Optional[DanbooruTagProvider]:
    """按入口配置构建标签 provider；缓存写入传入的目录级 DB 上下文。

    未配置任何数据来源时返回 None，由调用方决定如何呈现（纯目录补全）。
    """
    if not env.enabled:
        return None
    return build_danbooru_tag_provider(
        data_dir=env.data_dir,
        search_url=env.search_url,
        show_nsfw=env.show_nsfw,
        embedding_provider_url=env.embedding_provider_url,
        cache=SQLiteDanbooruCache(store),
    )


@contextmanager
def build_tag_provider_from_env(
    root_dir: str, directory_rel_path: str
) -> Generator[Optional[DanbooruTagProvider], None, None]:
    """单次执行入口：解析环境并按当前目录打开缓存后构建 provider。

    root_dir 为空时没有可用的缓存落点，按未配置处理；未配置数据来源时不打开
    数据库，也不构造 provider。
    """
    env = danbooru_env_from_env()
    if not env.enabled or not root_dir:
        yield None
        return
    with SQLiteContext(hook_db_path(root_dir, directory_rel_path)) as store:
        yield build_tag_provider(env, store)
