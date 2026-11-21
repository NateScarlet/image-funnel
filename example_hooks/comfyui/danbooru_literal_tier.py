# -*- coding: utf-8 -*-
"""字面命中档次与语义层追加决策：只做判断，不碰数据与排序。

本模块是「是否追加语义层」的唯一决策点：给一档字面命中与调用方的
skip_semantic，回答要不要追加语义层。标签记录、数据集索引、分数常量与排序
逻辑都不属于这里——它们留在打分层与 search，决策规则因此可用一张小表断言。
"""

from enum import IntEnum
from typing import FrozenSet


class LiteralHitTier(IntEnum):
    """字面命中档次，编号由弱到强，方便 search 直接取最高档次汇总结果。

    编号只表达层次强弱，与打分层的分数常量无关（分数常量不在此模块定义）。
    """

    NONE = 0
    FUZZY = 1
    CN_SUBSTRING = 2
    SUBSTRING = 3
    CN_PREFIX = 4
    PREFIX = 5
    CN_EXACT = 6
    EXACT = 7


# 语义层让路的两组档次：search 从 tier 派生强匹配 / 前缀命中判断
STRONG_TIERS: FrozenSet[LiteralHitTier] = frozenset(
    {LiteralHitTier.EXACT, LiteralHitTier.CN_EXACT}
)
PREFIX_TIERS: FrozenSet[LiteralHitTier] = frozenset(
    {LiteralHitTier.PREFIX, LiteralHitTier.CN_PREFIX}
)

_SEMANTIC_SUPPRESSED_TIERS = STRONG_TIERS | PREFIX_TIERS


def should_activate_semantic_layer(tier: LiteralHitTier, skip_semantic: bool) -> bool:
    """强匹配与前缀命中让语义层让路；skip_semantic 为真时一律不追加。"""
    if skip_semantic:
        return False
    return tier not in _SEMANTIC_SUPPRESSED_TIERS
