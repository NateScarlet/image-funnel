#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
命令处理器模块：将 comfyui.py 中的命令调度逻辑拆分为独立 Handler。

每个 Handler 实现 CommandHandler 协议，接收图片元数据并执行对应操作。
"""

import logging
from typing import Dict, List, Set, Any, Optional, Tuple, Protocol, cast

from graphql_utils import update_image_label
from workflow_prompt_pair import WorkflowPromptPair
from prompt_fragment import PromptFragment
from seed_manager import SeedManager
from filename_manager import FilenameManager
from weight_manager import WeightManager
from submission import submit as _submit_fn
import variant_engine

_LOGGER = logging.getLogger(__name__)


class CommandHandler(Protocol):
    """命令处理器协议"""

    def run(
        self,
        img_id: str,
        path: str,
        prompt: Dict[str, Any],
        workflow: Dict[str, Any],
        args: Any,
        comfyui_url: str,
        jobs: int,
        label_to_set: Optional[str],
    ) -> Tuple[int, bool]:
        """返回 (success_count (0 或 1), has_errors)"""
        ...


class QueueHandler:
    """queue 命令：提交工作流到 ComfyUI"""

    def run(
        self,
        img_id: str,
        path: str,
        prompt: Dict[str, Any],
        workflow: Dict[str, Any],
        args: Any,
        comfyui_url: str,
        jobs: int,
        label_to_set: Optional[str],
    ) -> Tuple[int, bool]:
        pair = WorkflowPromptPair(workflow, prompt)
        seed_mgr = SeedManager(pair, pair.seed_nodes)
        filename_mgr = FilenameManager(
            pair, pair.date_filename_nodes, pair.title_to_node
        )

        any_success = False
        has_error = False
        for q_idx in range(jobs):
            if jobs > 1:
                _LOGGER.debug("  -> Queueing run %d/%d", q_idx + 1, jobs)
            if seed_mgr.update_seeds() == 0:
                _LOGGER.error(f"Failed to update any seeds for image: {path}.")
                has_error = True
                break
            filename_mgr.update_output_filenames()
            if _submit_fn(prompt, workflow, comfyui_url):
                any_success = True
            else:
                has_error = True

        success = 1 if any_success else 0
        if any_success and label_to_set and img_id:
            update_image_label(img_id, label_to_set)
        return success, has_error


class AddHandler:
    """add 命令：添加提示词并提交"""

    def run(
        self,
        img_id: str,
        path: str,
        prompt: Dict[str, Any],
        workflow: Dict[str, Any],
        args: Any,
        comfyui_url: str,
        jobs: int,
        label_to_set: Optional[str],
    ) -> Tuple[int, bool]:
        pair = WorkflowPromptPair(workflow, prompt)
        fragments = pair.locate_prompts(
            nodes=args.node, regions=args.region, is_neg=args.neg
        )

        prompt_str_arg = " ".join(args.prompt)
        fragment = next(fragments, None)
        if fragment:
            fragment.add(prompt_str_arg, raw=args.raw, no_skip=args.no_skip)

        any_success, submit_error = _submit_simple(
            prompt, workflow, comfyui_url, jobs, path
        )
        success = 1 if any_success else 0
        if any_success and label_to_set and img_id:
            update_image_label(img_id, label_to_set)
        return success, submit_error


class RemoveHandler:
    """remove 命令：移除提示词并提交"""

    def run(
        self,
        img_id: str,
        path: str,
        prompt: Dict[str, Any],
        workflow: Dict[str, Any],
        args: Any,
        comfyui_url: str,
        jobs: int,
        label_to_set: Optional[str],
    ) -> Tuple[int, bool]:
        pair = WorkflowPromptPair(workflow, prompt)
        fragments = pair.locate_prompts(
            nodes=args.node, regions=args.region, is_neg=args.neg
        )
        hard = getattr(args, "hard", False)

        if args.all:
            clip_nodes = [nid for nid, node in prompt.items() if isinstance(node, dict)]
            clip_nodes = [
                nid
                for nid in clip_nodes
                if cast(Dict[str, Any], prompt[nid]).get("class_type")
                == "CLIPTextEncode"
            ]
            fragments = [PromptFragment(pair, nid) for nid in clip_nodes]

        prompt_list: List[str] = list(args.prompt)
        remove_prompts: Set[str] = set()
        for p in prompt_list:
            remove_prompts.update((p, p.replace("_", " "), p.replace(" ", "_")))

        any_processed = False
        for fragment in fragments:
            for prompt_str_arg in remove_prompts:
                if fragment.remove(
                    prompt_str_arg, raw=args.raw, hard=hard, no_skip=args.no_skip
                ):
                    any_processed = True

        if not any_processed:
            return 0, False

        any_success, submit_error = _submit_simple(
            prompt, workflow, comfyui_url, jobs, path
        )
        success = 1 if any_success else 0
        if any_success and label_to_set and img_id:
            update_image_label(img_id, label_to_set)
        return success, submit_error


class AdjustHandler:
    """adjust 命令：调整权重/比例并提交"""

    def run(
        self,
        img_id: str,
        path: str,
        prompt: Dict[str, Any],
        workflow: Dict[str, Any],
        args: Any,
        comfyui_url: str,
        jobs: int,
        label_to_set: Optional[str],
    ) -> Tuple[int, bool]:
        node_ids: Optional[List[str]] = getattr(args, "node", None)
        has_errors = False
        any_image_success = False
        enable_seed_update = args.update_seed or (jobs > 1)

        pair = WorkflowPromptPair(workflow, prompt)
        weight_mgr = WeightManager(pair)
        seed_mgr = SeedManager(pair, pair.seed_nodes)
        filename_mgr = FilenameManager(
            pair, pair.date_filename_nodes, pair.title_to_node
        )

        if args.adjust_type == "cfg":
            variant_gen = variant_engine.generate_cfg_variants(
                weight_mgr, prompt, args.weight, node_ids
            )
        elif args.adjust_type in ("lora", "l"):
            variant_gen = variant_engine.generate_lora_variants(
                weight_mgr, args.name, args.weight
            )
        elif args.adjust_type == "aspect":
            variant_gen = variant_engine.generate_aspect_variants(
                weight_mgr, prompt, pair.nodes_cache, args.ratio, node_ids
            )
        elif args.adjust_type in ("prompt", "p"):
            fragments = pair.locate_prompts(
                nodes=args.node, regions=args.region, is_neg=args.neg
            )
            variant_gen = variant_engine.generate_prompt_variants(
                weight_mgr, fragments, args.text, args.weight, args.skip_add
            )
        else:
            raise ValueError(f"unexpected adjust type '{args.adjust_type}'")

        variant_count = 0
        for _ in variant_gen:
            variant_count += 1
            if variant_count > 1:
                enable_seed_update = True
            for _ in range(jobs):
                if enable_seed_update:
                    if seed_mgr.update_seeds() == 0:
                        has_errors = True
                        break
                filename_mgr.update_output_filenames()
                if _submit_fn(prompt, workflow, comfyui_url):
                    any_image_success = True
                else:
                    has_errors = True

        if variant_count == 0 and args.no_skip:
            for _ in range(jobs):
                if enable_seed_update:
                    if seed_mgr.update_seeds() == 0:
                        has_errors = True
                        break
                filename_mgr.update_output_filenames()
                if _submit_fn(prompt, workflow, comfyui_url):
                    any_image_success = True
                else:
                    has_errors = True

        success = 1 if any_image_success else 0
        if any_image_success and label_to_set and img_id:
            update_image_label(img_id, label_to_set)
        return success, has_errors


def _submit_simple(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    comfyui_url: str,
    jobs: int,
    path: str,
) -> Tuple[bool, bool]:
    """基础提交逻辑：更新种子、文件名并提交"""
    pair = WorkflowPromptPair(workflow, prompt)
    seed_mgr = SeedManager(pair, pair.seed_nodes)
    filename_mgr = FilenameManager(pair, pair.date_filename_nodes, pair.title_to_node)

    any_success = False
    has_error = False
    for q_idx in range(jobs):
        if jobs > 1:
            _LOGGER.debug("  -> Queueing run %d/%d", q_idx + 1, jobs)
        if seed_mgr.update_seeds() == 0:
            _LOGGER.error(f"Failed to update any seeds for image: {path}.")
            has_error = True
            break
        filename_mgr.update_output_filenames()
        if _submit_fn(prompt, workflow, comfyui_url):
            any_success = True
        else:
            has_error = True
    return any_success, has_error


# #region 命令注册表

COMMAND_HANDLERS: Dict[str, CommandHandler] = {
    "queue": QueueHandler(),
    "add": AddHandler(),
    "remove": RemoveHandler(),
    "adjust": AdjustHandler(),
}

# #endregion
