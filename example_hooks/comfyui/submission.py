#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
Submission：封装 ComfyUI 工作流提交逻辑。
"""

import json
import uuid
import urllib.request
from typing import Dict, Any


def submit(
    prompt: Dict[str, Any],
    workflow: Dict[str, Any],
    comfyui_url: str,
) -> bool:
    """提交工作流到 ComfyUI 的 /prompt 接口"""
    client_id: str = str(uuid.uuid4())
    payload: Dict[str, Any] = {
        "prompt": prompt,
        "client_id": client_id,
        "extra_data": {"extra_pnginfo": {"workflow": workflow}},
    }
    data: bytes = json.dumps(payload).encode("utf-8")
    req: urllib.request.Request = urllib.request.Request(
        f"{comfyui_url}/prompt",
        data=data,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req) as f:
            json.loads(f.read().decode("utf-8"))
            return True
    except Exception:
        return False
