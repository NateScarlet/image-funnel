# -*- coding: utf-8 -*-
import os
import re
import tomllib
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Tuple, cast

from .node_accessor import NodeAccessor

_SCORE_TAG_PATTERN = re.compile(r"\bscore_[0-9]+(?:_[a-z0-9]+)*\b", re.IGNORECASE)


class MissingDataDirError(RuntimeError):
    """当 IMAGE_FUNNEL_DATA_DIR 环境变量缺失时抛出的快速失败错误。"""

    def __init__(self) -> None:
        super().__init__("Missing required environment variable: IMAGE_FUNNEL_DATA_DIR")


def get_data_dir() -> str:
    """获取全局数据目录，若未设置则触发快速失败报错。"""
    data_dir = os.getenv("IMAGE_FUNNEL_DATA_DIR", "").strip()
    if not data_dir:
        raise MissingDataDirError()
    return data_dir


def get_config_path() -> str:
    """获取 comfyui_model_formats.toml 的绝对路径。"""
    return os.path.join(get_data_dir(), "comfyui_model_formats.toml")


@dataclass
class ModelFormatConfig:
    default_format: str = "sdxl"
    models: Dict[str, str] = field(default_factory=lambda: cast(Dict[str, str], {}))

    @classmethod
    def load(cls) -> "ModelFormatConfig":
        """从配置文件加载配置。若文件不存在，使用默认配置。"""
        config_path = get_config_path()
        if not os.path.isfile(config_path):
            return cls(default_format="sdxl", models={})

        with open(config_path, "rb") as f:
            raw_data = tomllib.load(f)

        default_format = str(raw_data.get("default_format", "sdxl"))

        models: Dict[str, str] = {}
        raw_models = raw_data.get("models")
        if isinstance(raw_models, dict):
            raw_models_dict = cast(Dict[str, Any], raw_models)
            for k, v in raw_models_dict.items():
                models[str(k)] = str(v)

        return cls(default_format=default_format, models=models)

    def save(self) -> str:
        """将当前配置保存写回 comfyui_model_formats.toml 文件，并返回实际保存路径。"""
        config_path = get_config_path()
        os.makedirs(os.path.dirname(config_path), exist_ok=True)

        lines: List[str] = [
            "# ComfyUI 模型提示词格式全局配置文件",
            f'default_format = "{self.default_format}"',
            "",
            "[models]",
        ]
        for model_name, fmt in sorted(self.models.items()):
            escaped_name = model_name.replace('"', '\\"')
            lines.append(f'"{escaped_name}" = "{fmt}"')
        lines.append("")

        content = "\n".join(lines)
        with open(config_path, "w", encoding="utf-8") as f:
            f.write(content)

        return config_path

    def resolve_format(self, ckpt_name: str, prompt_text: str) -> str:
        """解析指定模型文件名的提示词格式。优先级：显式映射 > 提示词推理 > 默认格式。"""
        if not ckpt_name:
            return self.default_format

        # 1. 显式映射（含 disabled）
        if ckpt_name in self.models:
            return self.models[ckpt_name]

        # 2. 提示词推理
        inferred = infer_format_from_prompt(prompt_text)
        if inferred is None:
            return self.default_format

        # 自动记录推理结果供后续复用
        self.models[ckpt_name] = inferred
        self.save()
        return inferred


def _strip_comment_lines(text: str) -> str:
    """剔除以 // 开头的注释行，注释不参与格式推理。"""
    return "\n".join(
        line for line in text.splitlines() if not line.strip().startswith("//")
    )


def infer_format_from_prompt(prompt_text: str) -> Optional[str]:
    """基于提示词文本推理模型期望的标签格式。

    先剔除注释行与 score_* 评分标签（评分标签是唯一在 anima/sdxl 下都保留下划线的
    标签，不能作为判断依据），再比较空格与下划线数量：空格多于下划线 → anima；
    否则 → sdxl。若空格与下划线均为 0（空文本或仅含评分标签），无法判断，返回 None。
    """
    if not prompt_text:
        return None

    masked = _SCORE_TAG_PATTERN.sub("", _strip_comment_lines(prompt_text))
    # 若剔除评分标签后已无任何标签内容（如仅剩 ", " 等分隔符），无法判断
    if not re.search(r"[A-Za-z0-9\u4e00-\u9fff]", masked):
        return None

    space_count = masked.count(" ")
    underscore_count = masked.count("_")

    if space_count == 0 and underscore_count == 0:
        return None

    return "anima" if space_count > underscore_count else "sdxl"


def trace_model_name_for_node(
    prompt_meta: Dict[str, Any], start_node_id: str
) -> Optional[str]:
    """从指定 prompt 节点逆向追溯 clip 连线，获取源头 CheckpointLoader/UNETLoader 的模型文件名。"""
    visited: set[str] = set()
    current_id: Optional[str] = start_node_id

    while current_id and current_id not in visited:
        visited.add(current_id)
        node_raw = prompt_meta.get(current_id)
        if not isinstance(node_raw, dict):
            break

        node = cast(Dict[str, Any], node_raw)
        inputs_raw = node.get("inputs")
        if not isinstance(inputs_raw, dict):
            break

        inputs = cast(Dict[str, Any], inputs_raw)

        # 校验是否本身就是模型加载节点（含 DualCLIPLoader 的 clip_name1/clip_name2）
        for key in (
            "ckpt_name",
            "model_name",
            "unet_name",
            "clip_name",
            "clip_name1",
            "clip_name2",
        ):
            val = inputs.get(key)
            if isinstance(val, str) and val.strip():
                return val.strip()

        # 继续沿着 clip 输入向上追溯
        clip_link_raw = inputs.get("clip")
        if isinstance(clip_link_raw, list):
            clip_list = cast(List[Any], clip_link_raw)
            if len(clip_list) > 0:
                current_id = str(clip_list[0])
                continue

        # 若无 clip，尝试 model 输入
        model_link_raw = inputs.get("model")
        if isinstance(model_link_raw, list):
            model_list = cast(List[Any], model_link_raw)
            if len(model_list) > 0:
                current_id = str(model_list[0])
                continue

        break

    return None


def clip_text_encode_node_ids(prompt_meta: Dict[str, Any]) -> List[str]:
    """返回 prompt 中所有 CLIPTextEncode 类型节点的 ID 列表。"""
    return [
        str(nid)
        for nid, node in prompt_meta.items()
        if isinstance(node, dict)
        and cast(Dict[str, Any], node).get("class_type") == "CLIPTextEncode"
    ]


def format_text_for_node(
    prompt_meta: Dict[str, Any],
    node_id: str,
    text: str,
    inference_text: Optional[str] = None,
) -> str:
    """按指定节点相连模型的格式重排 text 文本。

    inference_text 用于格式推理（默认取 text 本身）；解析格式时若模型存在且
    IMAGE_FUNNEL_DATA_DIR 缺失，会触发 MissingDataDirError 快速失败（不静默降级）。
    无法追溯模型、或模型为 disabled 时原样返回 text。
    """
    ckpt_name = trace_model_name_for_node(prompt_meta, node_id)
    if not ckpt_name:
        return text
    source = inference_text if inference_text is not None else text
    fmt = ModelFormatConfig.load().resolve_format(ckpt_name, source)
    return format_prompt_text(text, fmt)


def format_workflow_prompt_pair(accessor: NodeAccessor) -> None:
    """集中式重排：将工作流内所有 CLIPTextEncode 节点的提示词全文按各自模型格式统一重排。

    与 adjust_output_dir 同一定位（提交前预处理），应用在所有会输出图片的路径
    （__main__.py 中各命令 + 复制增强），保证任一输出路径都得到一致的标签格式。
    workflow 与 prompt 双轨道同步重排；模型不可追溯或 disabled 时保持原样。
    """
    prompt_meta = accessor.prompt
    for node_id in clip_text_encode_node_ids(prompt_meta):
        workflow_text = accessor.get_workflow_node_text(node_id)
        prompt_text = accessor.get_prompt_input(node_id, "text")
        if not isinstance(prompt_text, str):
            prompt_text = ""

        # 推理来源：优先 prompt 轨道文本，空时回落到 workflow 轨道文本
        if prompt_text:
            inference_source = prompt_text
        elif isinstance(workflow_text, str):
            inference_source = workflow_text
        else:
            inference_source = ""

        if isinstance(workflow_text, str):
            accessor.update_workflow_node_text(
                node_id,
                format_text_for_node(
                    prompt_meta, node_id, workflow_text, inference_source
                ),
            )
        accessor.set_prompt_input(
            node_id,
            "text",
            format_text_for_node(prompt_meta, node_id, prompt_text, inference_source),
        )


def format_prompt_text(text: str, format_type: str) -> str:
    """根据 format_type ("anima" 或 "sdxl") 格式化提示词文本。

    - anima: 转换为小写；普通 Tag 下划线转空格，但严格保留 score_* 评分词的下划线。
    - sdxl: 空格转下划线。
    - 注释行（// 开头）保持原样不处理。
    """
    if not text:
        return text

    fmt = format_type.lower()
    if fmt not in ("anima", "sdxl"):
        return text

    lines = text.splitlines(keepends=True)
    new_lines: List[str] = []

    for line in lines:
        if line.strip().startswith("//"):
            new_lines.append(line)
            continue

        if fmt == "anima":
            formatted_line = _format_anima_line(line)
        else:
            formatted_line = _format_sdxl_line(line)

        new_lines.append(formatted_line)

    return "".join(new_lines)


def _format_anima_line(line: str) -> str:
    """处理 Anima 格式整行文本。"""
    score_matches: List[Tuple[str, str]] = []

    def replace_score(match: re.Match[str]) -> str:
        idx = len(score_matches)
        token = f"XSCORETOKEN{idx}X"
        matched_str = match.group(0).lower()
        score_matches.append((token, matched_str))
        return token

    # 替换 score_* 标签为占位符（不带下划线，防止被 replace('_', ' ') 破坏）
    masked = _SCORE_TAG_PATTERN.sub(replace_score, line)

    # 普通文字：转小写，下划线转空格
    masked = masked.lower()
    masked = masked.replace("_", " ")

    # 还原 score_* 占位符
    for token, original in score_matches:
        masked = masked.replace(token.lower(), original)

    return masked


def _format_sdxl_line(line: str) -> str:
    """处理 SDXL 格式整行文本。"""
    parts = line.split(",")
    new_parts: List[str] = []
    for part in parts:
        stripped = part.strip()
        if not stripped:
            new_parts.append(part)
            continue

        leading_space = part[: len(part) - len(part.lstrip())]
        trailing_space = part[len(part.rstrip()) :]

        content = part.strip()
        m = re.match(r"^\((.+):([0-9.-]+)\)$", content)
        if m:
            inner_tag = m.group(1).strip().replace(" ", "_")
            weight = m.group(2)
            replaced_part = f"({inner_tag}:{weight})"
        else:
            replaced_part = content.replace(" ", "_")

        new_parts.append(f"{leading_space}{replaced_part}{trailing_space}")

    return ",".join(new_parts)
