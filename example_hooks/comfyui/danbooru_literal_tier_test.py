# -*- coding: utf-8 -*-
import unittest

from .danbooru_literal_tier import (
    LiteralHitTier,
    should_activate_semantic_layer,
)


class ShouldActivateSemanticLayerTest(unittest.TestCase):
    """语义层追加决策真值表：8 档 × skip_semantic 直接断言纯函数返回值。

    这张表是 ADR 0005 决策内容的可执行规格：强匹配（精确/中文精确）
    与前缀命中（前缀/中文前缀）让语义层让路，其余档次追加，
    skip_semantic=True 一律不追加。测试不构造数据集、provider 或语义层替身。
    """

    def test_truth_table(self) -> None:
        cases: dict[tuple[LiteralHitTier, bool], bool] = {
            # 强匹配：不追加
            (LiteralHitTier.EXACT, False): False,
            (LiteralHitTier.CN_EXACT, False): False,
            # 前缀命中：不追加
            (LiteralHitTier.PREFIX, False): False,
            (LiteralHitTier.CN_PREFIX, False): False,
            # 子串 / 模糊 / 无命中：追加
            (LiteralHitTier.SUBSTRING, False): True,
            (LiteralHitTier.CN_SUBSTRING, False): True,
            (LiteralHitTier.FUZZY, False): True,
            (LiteralHitTier.NONE, False): True,
            # skip_semantic=True：任意档次都不追加
            (LiteralHitTier.EXACT, True): False,
            (LiteralHitTier.CN_EXACT, True): False,
            (LiteralHitTier.PREFIX, True): False,
            (LiteralHitTier.CN_PREFIX, True): False,
            (LiteralHitTier.SUBSTRING, True): False,
            (LiteralHitTier.CN_SUBSTRING, True): False,
            (LiteralHitTier.FUZZY, True): False,
            (LiteralHitTier.NONE, True): False,
        }
        self.assertEqual(len(LiteralHitTier), len(cases) // 2)
        for (tier, skip_semantic), expected in cases.items():
            with self.subTest(tier=tier, skip_semantic=skip_semantic):
                self.assertEqual(
                    should_activate_semantic_layer(tier, skip_semantic), expected
                )
