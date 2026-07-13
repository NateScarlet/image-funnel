#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
命令处理器模块：将 comfyui.py 中的命令调度逻辑拆分为独立 Handler。

每个 Handler 接收 CommandContext 并在其中执行对应操作。
"""

import logging
from typing import Dict, List, Set, Any, Optional, Protocol, cast

from graphql_utils import update_image_label
from .workflow_prompt_pair import WorkflowPromptPair
from .prompt_fragment import PromptFragment
from .seed_manager import SeedManager
from .filename_manager import FilenameManager
from .weight_manager import WeightManager
from .submission import submit as _submit_fn
from . import variant_engine

_LOGGER = logging.getLogger(__name__)


class CommandContext:
    """命令处理器上下文，封装图片环境及参数"""

    def __init__(
        self,
        img_id: str,
        path: str,
        prompt: Dict[str, Any],
        workflow: Dict[str, Any],
        args: Any,
        comfyui_url: str,
        jobs: int,
        label_to_set: Optional[str],
    ):
        self.img_id = img_id
        self.path = path
        self.prompt = prompt
        self.workflow = workflow
        self.args = args
        self.comfyui_url = comfyui_url
        self.jobs = jobs
        self.label_to_set = label_to_set
        self.skipped: bool = False

    def skip(self) -> None:
        """标记当前图片处理已被跳过"""
        self.skipped = True


class CommandHandler(Protocol):
    """命令处理器协议"""

    def run(self, ctx: CommandContext) -> None:
        """执行指令并提交 ComfyUI 任务。
        如果需要跳过处理，可以调用 ctx.skip() 并直接 return 返回。
        若执行失败则直接抛出异常。
        """
        ...


class QueueHandler:
    """queue 命令：提交工作流到 ComfyUI"""

    def run(self, ctx: CommandContext) -> None:
        pair = WorkflowPromptPair(ctx.workflow, ctx.prompt)
        seed_mgr = SeedManager(pair, pair.seed_nodes)
        filename_mgr = FilenameManager(
            pair, pair.date_filename_nodes, pair.title_to_node
        )

        for q_idx in range(ctx.jobs):
            if ctx.jobs > 1:
                _LOGGER.debug("  -> Queueing run %d/%d", q_idx + 1, ctx.jobs)
            if seed_mgr.update_seeds() == 0:
                raise ValueError(f"Failed to update any seeds for image: {ctx.path}.")
            filename_mgr.update_output_filenames()
            _submit_fn(ctx.prompt, ctx.workflow, ctx.comfyui_url)

        if ctx.label_to_set and ctx.img_id:
            update_image_label(ctx.img_id, ctx.label_to_set)


class AddHandler:
    """add 命令：添加提示词并提交"""

    def run(self, ctx: CommandContext) -> None:
        pair = WorkflowPromptPair(workflow=ctx.workflow, prompt=ctx.prompt)
        fragments = pair.locate_prompts(
            nodes=ctx.args.node, regions=ctx.args.region, is_neg=ctx.args.neg
        )

        prompt_str_arg = " ".join(ctx.args.prompt)
        fragment = next(fragments, None)
        if fragment:
            fragment.add(prompt_str_arg, raw=ctx.args.raw, no_skip=ctx.args.no_skip)

        _submit_simple(ctx.prompt, ctx.workflow, ctx.comfyui_url, ctx.jobs, ctx.path)
        if ctx.label_to_set and ctx.img_id:
            update_image_label(ctx.img_id, ctx.label_to_set)


class RemoveHandler:
    """remove 命令：移除提示词并提交"""

    def run(self, ctx: CommandContext) -> None:
        pair = WorkflowPromptPair(workflow=ctx.workflow, prompt=ctx.prompt)
        fragments = pair.locate_prompts(
            nodes=ctx.args.node, regions=ctx.args.region, is_neg=ctx.args.neg
        )
        hard = getattr(ctx.args, "hard", False)

        if ctx.args.all:
            clip_nodes = [
                nid for nid, node in ctx.prompt.items() if isinstance(node, dict)
            ]
            clip_nodes = [
                nid
                for nid in clip_nodes
                if cast(Dict[str, Any], ctx.prompt[nid]).get("class_type")
                == "CLIPTextEncode"
            ]
            fragments = [PromptFragment(pair, nid) for nid in clip_nodes]

        prompt_list: List[str] = list(ctx.args.prompt)
        remove_prompts: Set[str] = set()
        for p in prompt_list:
            remove_prompts.update((p, p.replace("_", " "), p.replace(" ", "_")))

        any_processed = False
        for fragment in fragments:
            for prompt_str_arg in remove_prompts:
                if fragment.remove(
                    prompt_str_arg,
                    raw=ctx.args.raw,
                    hard=hard,
                    no_skip=ctx.args.no_skip,
                ):
                    any_processed = True

        if not any_processed:
            ctx.skip()
            return

        _submit_simple(ctx.prompt, ctx.workflow, ctx.comfyui_url, ctx.jobs, ctx.path)
        if ctx.label_to_set and ctx.img_id:
            update_image_label(ctx.img_id, ctx.label_to_set)


class AdjustHandler:
    """adjust 命令：调整权重/比例并提交"""

    def run(self, ctx: CommandContext) -> None:
        node_ids: Optional[List[str]] = getattr(ctx.args, "node", None)
        enable_seed_update = ctx.args.update_seed or (ctx.jobs > 1)

        pair = WorkflowPromptPair(workflow=ctx.workflow, prompt=ctx.prompt)
        weight_mgr = WeightManager(pair)
        seed_mgr = SeedManager(pair, pair.seed_nodes)
        filename_mgr = FilenameManager(
            pair, pair.date_filename_nodes, pair.title_to_node
        )

        if ctx.args.adjust_type == "cfg":
            variant_gen = variant_engine.generate_cfg_variants(
                weight_mgr, ctx.prompt, ctx.args.weight, node_ids
            )
        elif ctx.args.adjust_type in ("lora", "l"):
            variant_gen = variant_engine.generate_lora_variants(
                weight_mgr, ctx.args.name, ctx.args.weight
            )
        elif ctx.args.adjust_type == "aspect":
            variant_gen = variant_engine.generate_aspect_variants(
                weight_mgr, ctx.prompt, pair.nodes_cache, ctx.args.ratio, node_ids
            )
        elif ctx.args.adjust_type in ("prompt", "p"):
            fragments = pair.locate_prompts(
                nodes=ctx.args.node, regions=ctx.args.region, is_neg=ctx.args.neg
            )
            variant_gen = variant_engine.generate_prompt_variants(
                weight_mgr, fragments, ctx.args.text, ctx.args.weight, ctx.args.skip_add
            )
        else:
            raise ValueError(f"unexpected adjust type '{ctx.args.adjust_type}'")

        variant_count = 0
        for _ in variant_gen:
            variant_count += 1
            if variant_count > 1:
                enable_seed_update = True
            for _ in range(ctx.jobs):
                if enable_seed_update:
                    if seed_mgr.update_seeds() == 0:
                        raise ValueError(
                            f"Failed to update any seeds for image: {ctx.path}."
                        )
                filename_mgr.update_output_filenames()
                _submit_fn(ctx.prompt, ctx.workflow, ctx.comfyui_url)

        if variant_count == 0 and ctx.args.no_skip:
            for _ in range(ctx.jobs):
                if enable_seed_update:
                    if seed_mgr.update_seeds() == 0:
                        raise ValueError(
                            f"Failed to update any seeds for image: {ctx.path}."
                        )
                filename_mgr.update_output_filenames()
                _submit_fn(ctx.prompt, ctx.workflow, ctx.comfyui_url)

        processed = variant_count > 0 or ctx.args.no_skip
        if not processed:
            ctx.skip()
            return

        if ctx.label_to_set and ctx.img_id:
            update_image_label(ctx.img_id, ctx.label_to_set)


def _submit_simple(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    comfyui_url: str,
    jobs: int,
    path: str,
) -> None:
    """基础提交逻辑：更新种子、文件名并提交"""
    pair = WorkflowPromptPair(workflow, prompt)
    seed_mgr = SeedManager(pair, pair.seed_nodes)
    filename_mgr = FilenameManager(pair, pair.date_filename_nodes, pair.title_to_node)

    for q_idx in range(jobs):
        if jobs > 1:
            _LOGGER.debug("  -> Queueing run %d/%d", q_idx + 1, jobs)
        if seed_mgr.update_seeds() == 0:
            raise ValueError(f"Failed to update any seeds for image: {path}.")
        filename_mgr.update_output_filenames()
        _submit_fn(prompt, workflow, comfyui_url)


# #region 命令注册表

COMMAND_HANDLERS: Dict[str, CommandHandler] = {
    "queue": QueueHandler(),
    "add": AddHandler(),
    "remove": RemoveHandler(),
    "adjust": AdjustHandler(),
}

# #endregion
