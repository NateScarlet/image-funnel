# -*- coding: utf-8 -*-
import os
import tempfile
import unittest
from unittest.mock import patch

from .model_format import (
    ModelFormatConfig,
    MissingDataDirError,
    format_prompt_text,
    get_config_path,
    infer_format_from_prompt,
    trace_model_name_for_node,
)


class TestModelFormat(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp_dir = tempfile.mkdtemp()
        self._env_patch = patch.dict(
            os.environ, {"IMAGE_FUNNEL_DATA_DIR": self.tmp_dir}
        )
        self._env_patch.start()

    def tearDown(self) -> None:
        self._env_patch.stop()

    def test_missing_data_dir_raises_error(self) -> None:
        with patch.dict(os.environ, {"IMAGE_FUNNEL_DATA_DIR": ""}):
            with self.assertRaises(MissingDataDirError):
                get_config_path()

    def test_format_prompt_text_anima(self) -> None:
        raw_text = "Blue_Hair, score_7, Safe, Cat_Ears\n// comment line: score_1"
        formatted = format_prompt_text(raw_text, "anima")
        expected = "blue hair, score_7, safe, cat ears\n// comment line: score_1"
        self.assertEqual(formatted, expected)

    def test_format_prompt_text_sdxl(self) -> None:
        raw_text = "blue hair, (cat ears:1.2), masterpiece"
        formatted = format_prompt_text(raw_text, "sdxl")
        expected = "blue_hair, (cat_ears:1.2), masterpiece"
        self.assertEqual(formatted, expected)

    def test_format_prompt_text_disabled_returns_original(self) -> None:
        raw_text = "Blue_Hair, score_7\n// maintain as-is"
        self.assertEqual(format_prompt_text(raw_text, "disabled"), raw_text)

    def test_infer_format_from_prompt(self) -> None:
        # 空格多于下划线 -> anima
        self.assertEqual(
            infer_format_from_prompt("blue hair, Safe, detailed eyes"), "anima"
        )
        # 剔除 score_7 后全为空格 -> anima
        self.assertEqual(
            infer_format_from_prompt("masterpiece, best quality, score_7, safe"),
            "anima",
        )
        self.assertEqual(infer_format_from_prompt("blue hair, cat ears"), "anima")
        # 下划线多于空格 -> sdxl
        self.assertEqual(
            infer_format_from_prompt("blue_hair, cat_ears, detailed_eyes"), "sdxl"
        )
        self.assertEqual(infer_format_from_prompt("blue_hair"), "sdxl")
        # 注释行被剔除，不参与判断
        self.assertEqual(
            infer_format_from_prompt("blue hair, safe\n// caption note"), "anima"
        )
        # 无法判断 -> None
        self.assertIsNone(infer_format_from_prompt(""))
        self.assertIsNone(infer_format_from_prompt("score_7"))
        self.assertIsNone(infer_format_from_prompt("score_7, score_8"))
        self.assertIsNone(infer_format_from_prompt("// only comments"))

    def test_resolve_format_explicit_mapping_overrides_inference(self) -> None:
        config = ModelFormatConfig.load()
        config.models["animaModel.safetensors"] = "sdxl"
        # 提示词推理本会判为 anima，但显式映射覆盖为 sdxl
        fmt = config.resolve_format("animaModel.safetensors", "blue hair, safe")
        self.assertEqual(fmt, "sdxl")

    def test_resolve_format_disabled(self) -> None:
        config = ModelFormatConfig.load()
        config.models["someModel.safetensors"] = "disabled"
        fmt = config.resolve_format("someModel.safetensors", "blue hair")
        self.assertEqual(fmt, "disabled")

    def test_resolve_format_infers_and_persists(self) -> None:
        config = ModelFormatConfig.load()
        fmt = config.resolve_format("someModel.safetensors", "blue hair, safe")
        self.assertEqual(fmt, "anima")
        # 推理结果被自动记录到配置文件，供后续复用
        reloaded = ModelFormatConfig.load()
        self.assertEqual(reloaded.models.get("someModel.safetensors"), "anima")

    def test_resolve_format_falls_back_to_default_when_cannot_infer(self) -> None:
        config = ModelFormatConfig.load()
        # 仅评分标签无法判断，回落到默认格式 sdxl，且不写入 models
        self.assertEqual(
            config.resolve_format("someModel.safetensors", "score_7"), "sdxl"
        )
        self.assertEqual(config.resolve_format("someModel.safetensors", ""), "sdxl")
        self.assertEqual(config.models, {})

    def test_config_save_load_roundtrip(self) -> None:
        config = ModelFormatConfig.load()
        config.models["a.safetensors"] = "anima"
        config.models["b.safetensors"] = "disabled"
        saved_path = config.save()
        self.assertTrue(os.path.isfile(saved_path))

        reloaded = ModelFormatConfig.load()
        self.assertEqual(reloaded.models["a.safetensors"], "anima")
        self.assertEqual(reloaded.models["b.safetensors"], "disabled")

    def test_trace_model_name_for_node(self) -> None:
        prompt_meta = {
            "4": {
                "class_type": "CheckpointLoaderSimple",
                "inputs": {"ckpt_name": "animaPencilXL_v10.safetensors"},
            },
            "8": {
                "class_type": "LoraLoader",
                "inputs": {"clip": ["4", 1]},
            },
            "6": {
                "class_type": "CLIPTextEncode",
                "inputs": {"clip": ["8", 1], "text": "masterpiece, 1girl"},
            },
        }

        ckpt_name = trace_model_name_for_node(prompt_meta, "6")
        self.assertEqual(ckpt_name, "animaPencilXL_v10.safetensors")

    def test_trace_model_name_for_dual_clip_loader(self) -> None:
        """追溯结尾的 DualCLIPLoader 节点应读取其 clip_name1 作为模型名。"""
        prompt_meta = {
            "4": {
                "class_type": "DualCLIPLoader",
                "inputs": {
                    "clip_name1": "animaPencilXL_v10.safetensors",
                    "clip_name2": "clip_sd_xl.safetensors",
                    "type": "sdxl",
                },
            },
            "6": {
                "class_type": "CLIPTextEncode",
                "inputs": {"clip": ["4", 0], "text": "masterpiece"},
            },
        }

        ckpt_name = trace_model_name_for_node(prompt_meta, "6")
        self.assertEqual(ckpt_name, "animaPencilXL_v10.safetensors")


if __name__ == "__main__":
    unittest.main()
