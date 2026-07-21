#!/usr/bin/env python
# -*- coding: utf-8 -*-

from dataclasses import dataclass, field
from typing import List, Optional, cast
import os
import json


@dataclass
class ComfyUIConfig:
    """Centralized configuration for ComfyUI hook scripts.

    All environment variable reads are collected here.
    Callers receive a fully-typed Config object instead of scattering os.getenv calls.
    """

    image_paths: List[str] = field(default_factory=lambda: cast(List[str], []))
    image_ids: List[str] = field(default_factory=lambda: cast(List[str], []))
    comfyui_url: str = "http://127.0.0.1:8188"
    max_match: int = 4
    jobs: int = 1
    label_to_set: Optional[str] = None
    required_rating: Optional[int] = None
    hook_output_dir: str = ""
    comfyui_output_dir: str = ""
    hook_name: str = ""
    trigger: str = ""
    action_path: str = ""
    logging_level: str = "WARNING"

    @staticmethod
    def from_env() -> "ComfyUIConfig":
        """Read all config from environment variables."""
        image_paths_str = os.getenv("IMAGE_FUNNEL_IMAGE_PATHS", "")
        image_ids_str = os.getenv("IMAGE_FUNNEL_IMAGE_IDS", "")

        def parse_json_list(s: str) -> List[str]:
            if s:
                try:
                    return json.loads(s)
                except json.JSONDecodeError:
                    return []
            return []

        max_match_str = os.getenv("HOOK_MAX_MATCH", "4")
        max_match = int(max_match_str) if max_match_str else 4

        jobs_str = os.getenv("HOOK_JOBS", "1")
        jobs = int(jobs_str) if jobs_str else 1

        required_rating_str = os.getenv("HOOK_IMAGE_RATING")
        required_rating = int(required_rating_str) if required_rating_str else None

        return ComfyUIConfig(
            image_paths=parse_json_list(image_paths_str),
            image_ids=parse_json_list(image_ids_str),
            comfyui_url=os.getenv("COMFYUI_URL", "http://127.0.0.1:8188"),
            max_match=max_match,
            jobs=jobs,
            label_to_set=os.getenv("HOOK_IMAGE_SET_LABEL"),
            required_rating=required_rating,
            hook_output_dir=os.getenv("HOOK_OUTPUT_DIR", ""),
            comfyui_output_dir=os.getenv("COMFYUI_OUTPUT_DIR", ""),
            hook_name=os.getenv("IMAGE_FUNNEL_HOOK_NAME", ""),
            trigger=os.getenv("IMAGE_FUNNEL_TRIGGER", ""),
            action_path=os.getenv("IMAGE_FUNNEL_ACTION", ""),
            logging_level=os.getenv("HOOK_LOGGING_LEVEL", "WARNING").upper(),
        )
