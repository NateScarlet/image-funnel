---
name: "develop-hook"
description: "指导 AI 助手如何在 ImageFunnel 项目中定义、编写、优化和调试各类 Hook 配置文件与外部执行脚本。"
---

# Hook 开发指南

本指南用于指导 AI 助手如何快速且规范地为 ImageFunnel 添加、重构或调试各类自定义 Hook 逻辑（如图片搬运、归档、发送至 ComfyUI 等）。

---

## 1. 结构与目录规范

所有 Hook 的配置文件应放在环境变量 `IMAGE_FUNNEL_HOOK_DIR` 指定的 Hook 目录中。若未指定该环境变量，服务端默认不加载任何 Hook（在开发环境和调试模式下通常配置为项目根目录下的 `example_hooks/`）。每个 Hook 由以下两部分组成：
1. **`.toml` 配置文件**：声明 Hook 的基本信息、触发时机、笔记指令定义及自定义环境变量。
2. **执行脚本/外部程序**：由 `.toml` 的 `command` 字段调用的任意脚本（如 Python 脚本、Powershell 脚本等）。

---

## 2. Hook 配置文件 (.toml) 规范

TOML 配置文件应该提供清晰的描述与声明：

```toml
# Hook 的基础声明
id = "your-hook-id"                # 全局唯一标识符
name = "Hook 展示名称"
description = "用于描述 Hook 逻辑的简短文本"
command = "python your_script.py"  # 触发时在外部调用的 Shell 命令行

# 笔记指令配置（可选，允许在笔记中使用斜杠指令触发）
[directive]
name = "your_directive"            # 斜杠指令名字，例如 "/your_directive"
usage = """
/your_directive <argument>
指令的详细用法和参数说明。
"""
# 动作指令执行成功/失败后的处理行为（可选，默认为 "COMMENT_OUT"）。支持以下三种显式选项：
# - "COMMENT_OUT": 将指令行注释（%% ... %%），并在其后追加脚本 stdout/stderr 的 alert 语法块（`>[!stdout]` / `>[!stderr]`）。
# - "REMOVE": 从笔记中彻底删除这一行指令。
# - "KEEP": 保留本行指令内容不变，并在其后追加脚本 stdout/stderr 的 alert 语法块。
# 脚本成功执行后，可通过向 IMAGE_FUNNEL_ACTION 环境变量指向的文件写入操作名称来覆盖此行为。
on_success_action = "REMOVE"        
on_fail_action = "KEEP"            

# 触发条件订阅（可多选）
[on.post_update_image_metadata]    # 当单张图片元数据更新时触发
rating = [4, 5]                    # 匹配的评分
label = ["Red", "Yellow"]          # 匹配的颜色标签

[on.post_update_note]              # 当笔记更新时触发
ignore_directive = false           # 为 true 时忽略 [directive] 约束（即使定义了指令，笔记中未含该指令也依然触发）

[on.image_dispatch]                # 允许在前端 UI 的图片上方直接显示手动派发按钮触发
[on.note_dispatch]                 # 允许在前端 UI 的笔记上方直接显示手动派发按钮触发

# 自定义环境变量（可选）
[env]
HOOK_CUSTOM_ENV_VAR = "some-value" # 注入给脚本的静态自定义配置环境变量
```

> [!IMPORTANT]
> **笔记指令调用语法与限制**：
> 1. **独占一行**：指令行必须独立占有一行，不能与同一行中的其他普通文本混用。
> 2. **严格的行首触发**：斜杠 `/` 必须位于该行的绝对行首，前面仅允许包含可选的缩进空白字符（空格 ` ` 或制表符 `\t`），**不能有任何其他文本字符**。
> 3. **匹配示例**：
>    - `  /your_directive hello` （正确，允许前导缩进空格）
>    - `/your_directive` （正确）
>    - `这是指令：/your_directive` （错误，非行首斜杠不会被识别，也不会触发自动完成或 Hook 执行）
> 4. **参数自动完成语法规范**：
>    为了启用智能的指令子命令、参数和选项补全，`usage` 的定义应遵循 Docopt 风格语法：
>    - **用法定义行**：必须以 `/[指令名]` 开头。如果一个指令有多种不同格式，可以分多行定义，均以 `/` 开头（自动完成会提供它们作为候选分支）。
>    - **子命令 (Subcommand)**：纯单词（例如 `lora`），用于分支匹配。
>    - **位置参数 (Positional Argument)**：使用 `<>` 包裹（例如 `<prompt>`），补全时会自动插入并在编辑器中选中该占位符。
>    - **可选选项 (Option)**：使用 `[]` 包裹。支持无参标志（例如 `[--all]`）或带参选项（例如 `[--region <region>]`）。
>    - **说明文本**：用法定义行下方的非指令行会被合并为整体说明显示在补全列表中。在行内通过多个空格分隔选项和描述（例如 `--all  在所有节点中删除`）可让自动完成提取为该选项的专属说明。

---

## 3. 注入的环境变量列表

当 Hook 被触发执行外部脚本时，Go 服务端的 Hook Runner 会自动向外部命令进程注入以下环境变量：

### 3.1 基础与触发上下文
- `IMAGE_FUNNEL_HOOK_ID`: 当前 Hook 的 ID。
- `IMAGE_FUNNEL_HOOK_NAME`: 当前 Hook 的名称。
- `IMAGE_FUNNEL_TRIGGER`: 具体的触发器类型名称（如 `post_update_image_metadata`、`post_update_note`、`image_dispatch` 等）。
- `IMAGE_FUNNEL_ROOT_DIR`: 服务端配置的图片管理根目录绝对路径（用于本地其他需要根目录地址的操作）。
- `IMAGE_FUNNEL_DIRECTORY_ID`: 当前触发目录的 GraphQL 唯一 ID（即 Node ID）。
- `IMAGE_FUNNEL_DIRECTORY_REL_PATH`: 当前触发目录相对于图片管理根目录的**规范化相对路径**（如果正好在根目录下触发，该变量为 `""` 或未注入）。
- `IMAGE_FUNNEL_NOTE_PATHS`: 笔记文件的绝对路径列表，格式为 JSON 字符串数组（如 `["C:\\rootDir\\subDir\\1.md"]`）（仅在笔记触发事件中有效）。

### 3.2 被处理的图片信息
- `IMAGE_FUNNEL_IMAGE_IDS`: 被处理的图片 ID 列表，格式为 JSON 字符串数组（如 `["img:123", "img:456"]`）。
- `IMAGE_FUNNEL_IMAGE_PATHS`: 被处理的图片绝对路径列表，格式为 JSON 字符串数组（如 `["C:\\rootDir\\subDir\\1.png"]`）。

### 3.3 单图更新状态（仅在单图更新触发如 `post_update_image_metadata` 时有效）
- `IMAGE_FUNNEL_IMAGE_RATING`: 触发图片的新评分 (int)。
- `IMAGE_FUNNEL_IMAGE_LABEL`: 触发图片的新颜色标签 (string)。
- `IMAGE_FUNNEL_IMAGE_ACTION`: 触发图片的新动作 (string)。
- `IMAGE_FUNNEL_IMAGE_OLD_RATING`: 触发图片的旧评分 (int)。
- `IMAGE_FUNNEL_IMAGE_OLD_LABEL`: 触发图片的旧颜色标签 (string)。
- `IMAGE_FUNNEL_IMAGE_OLD_ACTION`: 触发图片的旧动作 (string)。

### 3.4 API 鉴权信息
- `IMAGE_FUNNEL_GRAPHQL_URL`: 服务端的 GraphQL 终点 API 路径（如 `http://localhost:8000/graphql`）。
- `IMAGE_FUNNEL_TOKEN`: 服务端专门为此 Hook 执行签发的临时临时 JWT 鉴权令牌。访问 GraphQL 服务时应以 `Authorization: Bearer <TOKEN>` 请求头发送。

### 3.5 操作覆盖
- `IMAGE_FUNNEL_ACTION`: Runner 提供的一个临时唯一文件路径（不提前创建），脚本可以通过向该文件写入操作名称来覆盖指令执行后的行为。支持的操作：`COMMENT_OUT`、`REMOVE`、`KEEP`。只有脚本成功结束（退出码 0）时才会读取此文件；出错时总是按 `on_fail_action` 执行。Runner 执行完成后会自动清理此临时文件。写入不支持的操作会导致 Runner 报错。

---

## 4. 脚本开发黄金法则与核心规范

编写 Hook 逻辑的外部脚本时，必须严格遵守以下法则，以保证代码的安全、高性能和符合六边形设计架构：

### 4.1 相对路径优先原则 (`relativeToRoot`)
* **核心规范**：严禁在外部脚本中自行处理或规范化操作系统绝对物理路径（如盘符大小写比对等），以防止在 Windows/UNC 环境下产生目录越界。
* **做法**：凡是通过 GraphQL API 对外发出变更请求（如移动图片、查询或新建子目录等）时，凡是路径入参，必须使用 GraphQL 输入参数中的 `relativeToRoot` 属性。
* **路径推算**：
  直接根据 `IMAGE_FUNNEL_DIRECTORY_REL_PATH` 环境变量推算相对目标目录。
  - 在根目录下触发：`IMAGE_FUNNEL_DIRECTORY_REL_PATH` 为空，目标相对路径即为子目录名。
  - 在子目录下触发：基于 `os.path.dirname(dir_rel)` 与 `os.path.basename(dir_rel)` 构造同级或下级目录相对路径。

### 4.2 快速失败原则 (Fail-fast)
* **核心规范**：外部脚本如果无法正常进行，应直接崩溃报错并返回非零退出码（如 `sys.exit(1)`），禁止做多余的兜底或忽略错误。
* **做法**：
  - 在脚本入口处，强制通过 `os.environ` 校验所需环境变量（如 `IMAGE_FUNNEL_IMAGE_IDS` 等）的存在性，如果缺失立即抛出 `ValueError`。
  - GraphQL 请求返回的 JSON 如果包含 `"errors"`，直接抛出异常终止进程。

### 4.3 避免过度防御性编程 (Zero-I/O Validation)
* **核心规范**：不在本地脚本中对传入的参数做重复校验。
* **做法**：
  - 不必在本地通过 `os.path.exists(path)` 对每一个传入的图片路径做物理存在性检测。Go 服务端本身的 GraphQL API 入口在执行操作时会进行极其严密的安全性及物理存在性验证。
  - 如果无需进行本地文件解析，脚本应该只解析 `IMAGE_FUNNEL_IMAGE_IDS` and `IMAGE_FUNNEL_DIRECTORY_REL_PATH`，完全忽略 `IMAGE_FUNNEL_IMAGE_PATHS` 环境变量，以消除本地磁盘扫描 I/O 开销。

### 4.4 批量合批调用原则
* **核心规范**：禁止在循环中对每个图片分别发出 API 变更或查询调用。
* **做法**：
  - 同一个笔记指令或事件处理上下文中的图片均位于同一目录下，它们的目标相对路径是一致的。
  - 应当收集所有的图片 ID 数组，仅调用一次 GraphQL API 批量修改，极大地节省 network 握手与服务端数据库开销。

### 4.5 图片匹配数量保护 (`--max-match`)
* **核心规范**：通过 GraphQL 查询匹配图片的脚本（如 `/add`、`/remove`、`/adjust` 指令）应支持 `--max-match` 选项，防止误操作批量处理过多图片。
* **做法**：
  - 脚本应添加 `--max-match` 参数，默认值从 `HOOK_MAX_MATCH` 环境变量读取，未设置时默认为 `4`。
  - `0` 代表不限制匹配数量。
  - 负数或非法值应立即报错退出。
  - 当匹配的图片数量超过 `max-match` 时，脚本应跳过执行并在 stderr 中输出跳过原因。
  - 在 TOML 配置文件的 `[env]` 节中设置 `HOOK_MAX_MATCH` 可覆盖默认值。

### 4.6 日志输出策略 (stdout / stderr 分离)
* **核心规范**：脚本的 stdout 与 stderr 有明确分工，因为脚本执行结果是在完成后才显示给用户的，过程状态是噪音。
* **stdout**：只输出可解析的结果数据（Unix 风格），如 `processed 3/5`。用户可能关心的结构化数据通过 stdout 输出。
* **stderr**：用于调试和错误信息。所有 `logging` 输出默认发送到 stderr，通过 `HOOK_LOGGING_LEVEL` 环境变量控制可见级别。
* **日志级别**：脚本应通过 `HOOK_LOGGING_LEVEL` 环境变量控制日志级别，默认 `WARNING`。过程进度信息使用 `_LOGGER.debug()`，仅在用户显式设置 `HOOK_LOGGING_LEVEL=DEBUG` 时可见。
* **做法**：
  ```python
  log_level_str = os.getenv("HOOK_LOGGING_LEVEL", "WARNING").upper()
  log_level = getattr(logging, log_level_str, logging.WARNING)
  logging.basicConfig(level=log_level, format="%(asctime)s [%(levelname)s] %(message)s")
  ```
  - 过程进度消息（如 `[1/5] Processing image`）使用 `_LOGGER.debug()`
  - 跳过/警告信息使用 `_LOGGER.warning()`
  - 错误信息使用 `_LOGGER.error()`
  - 最终结果使用 `print()` 输出到 stdout

---

## 5. Hook 脚本开发模板 (Python 示例)

以下是符合上述规范的极简 Python Hook 开发模板：

```python
#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import sys
import json
import logging
import argparse
from typing import List, Optional
from graphql_utils import move_images  # 本地封装好的 GraphQL execute 方法

_LOGGER = logging.getLogger(__name__)

def parse_args():
    parser = argparse.ArgumentParser(description="Your Hook Description")
    parser.add_argument("suffix", help="Suffix for the target directory")
    return parser.parse_args()

def main() -> None:
    # 从 HOOK_LOGGING_LEVEL 环境变量读取日志级别，默认 WARNING
    log_level_str = os.getenv("HOOK_LOGGING_LEVEL", "WARNING").upper()
    log_level = getattr(logging, log_level_str, logging.WARNING)
    logging.basicConfig(
        level=log_level,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )

    args = parse_args()
    suffix = args.suffix.strip()
    if not suffix:
        _LOGGER.error("Suffix cannot be empty.")
        sys.exit(1)

    # 1. 快速失败：校验必需的环境变量并解析 ID 列表
    image_ids_str = os.getenv("IMAGE_FUNNEL_IMAGE_IDS", "")
    if not image_ids_str:
        raise ValueError("Environment variable IMAGE_FUNNEL_IMAGE_IDS is missing.")

    try:
        image_ids: List[str] = json.loads(image_ids_str)
    except Exception as e:
        _LOGGER.error("Failed to parse IMAGE_FUNNEL_IMAGE_IDS: %s", e)
        sys.exit(1)

    if not image_ids:
        _LOGGER.error("No image IDs to process.")
        sys.exit(1)

    _LOGGER.debug("Received %d image(s) to fork with suffix: %s", len(image_ids), suffix)

    # 2. 相对路径推算：利用 IMAGE_FUNNEL_DIRECTORY_REL_PATH 规避绝对路径处理
    dir_rel_path = os.getenv("IMAGE_FUNNEL_DIRECTORY_REL_PATH", "")
    if not dir_rel_path.strip():
        # 在根目录下触发：目标相对路径即为 suffix 子目录
        dest_dir = suffix
    else:
        # 在子目录下触发：目标为同级目录 "{当前目录名},{suffix}"
        dir_name = os.path.basename(dir_rel_path)
        parent_dir_rel_path = os.path.dirname(dir_rel_path)
        dest_dir = os.path.join(parent_dir_rel_path, f"{dir_name},{suffix}")
    
    dest_dir = os.path.normpath(dest_dir)

    # 3. 批量高效调用 GraphQL：单次请求移动所有图片
    _LOGGER.debug("Moving %d image(s) to relative path '%s'...", len(image_ids), dest_dir)
    move_images(image_ids, dest_dir)  # GraphQL 执行 relativeToRoot 变更

    print(f"processed {len(image_ids)} image(s) successfully.")
    sys.exit(0)

if __name__ == "__main__":
    main()
```

---

## 6. 程序化自动完成

当用户在笔记编辑器中输入指令参数时，自动完成系统不仅支持基于 `usage` 语法的静态补全，还可以通过外部脚本提供动态建议，例如从后端 API 获取可用区域名、Lora 名称、已缓存的工作流节点 ID 等。

### 6.1 启用自动完成脚本

在 TOML 配置文件的 `[directive]` 下添加 `[directive.autocomplete]` 节：

```toml
[directive]
name = "adjust"
usage = "/adjust lora <name> <weight> [--region <region>]..."

[directive.autocomplete]
command = "uv run your_script.py autocomplete"
```

`command` 中的 `autocomplete` 子命令用于告诉脚本入口当前处于自动完成模式（而非执行模式）。如果脚本不支持自动完成，可以不配置此节，系统将仅使用 `usage` 的静态补全。

### 6.2 自动完成注入的环境变量

自动完成脚本运行时，除了常规 Hook 环境变量外，还会额外注入以下上下文变量：

| 环境变量 | 格式 | 说明 |
|---|---|---|
| `IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS` | JSON 字符串数组 | 当前行已完成的单词列表（类似 bash `COMP_WORDS`），例如 `["--region", "<region>", "--node"]` |
| `IMAGE_FUNNEL_AUTOCOMPLETE_CWORD_IDX` | 整数 | 当前正在输入的单词在 `CWORDS` 中的索引（类似 bash `COMP_CWORD`） |
| `IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD` | 字符串 | 当前光标前一个完整单词，常用于判断上下文（如识别到 `--region` 则应补全区域名） |
| `IMAGE_FUNNEL_AUTOCOMPLETE_LINE_PREFIX` | 字符串 | 当前行光标前的完整文本（包括指令名），例如 `/adjust lora --region ` |
| `IMAGE_FUNNEL_AUTOCOMPLETE_QUERY` | 字符串 | 当前正在输入的单词片段（用户已键入但未完成的文本） |

此外还会注入 `IMAGE_FUNNEL_ROOT_DIR`、`IMAGE_FUNNEL_GRAPHQL_URL`（用于脚本自行查询服务端数据）、`IMAGE_FUNNEL_NOTE_PATHS`、`IMAGE_FUNNEL_IMAGE_PATHS`（若当前笔记有配套图片）以及 TOML `[env]` 中定义的自定义变量。

### 6.3 输出格式 (JSONL)

脚本的 **stdout** 必须输出 **JSONL**（每行一个 JSON 对象），每行代表一个建议项。空行会被跳过。

```jsonl
{"text": "positive", "displayText": "positive", "description": "正向提示词区域"}
{"text": "negative", "displayText": "negative", "description": "负向提示词区域"}
```

每个 JSON 对象的字段：

| 字段 | 必需 | 类型 | 说明 |
|---|---|---|---|
| `text` | 是 | 字符串 | 插入到文本框的完整文本（选项、值等）。如果当前用户在输入选项值（如 `--region` 之后），给出具体的值 |
| `displayText` | 否 | 字符串 | 浮层中显示的友好文本。未提供时默认使用 `text` 的值 |
| `description` | 否 | 字符串 | 建议项的描述，显示在浮层中 `displayText` 下方 |
| `type` | 否 | 字符串 | 类型标签，用于显示分类标识。不提供时不显示标签。常见值如 `"region"`、`"lora"`、`"node"` 等 |
| `style` | 否 | 字符串 | 额外的样式类名称。例如，`"muted"` 表示此提示词已存在于工作流对应目标区域中，前端会以此应用特定置灰样式 |

脚本的 **stderr** 不会影响自动完成结果，但会被记录到服务端日志中，适用于调试信息。

### 6.4 与静态补全的集成

动态 API 建议与 `usage` 静态补全共同显示在同一菜单中。API 返回的建议会排在位置参数建议前面，取代 `<>` 占位符建议，而选项（`[--option]`）和子命令建议始终显示。

在典型工作流中，用户：
1. 在 `usage` 静态定义的提示下选择 `--region <region>` 选项
2. `--region <region>` 被插入文本框，其中 `<region>` 占位符自动选中
3. 自动完成脚本被立即调用
4. 返回的建议（如区域名 `"positive"`、`"negative"`）出现在菜单中
5. 用户选择一个建议项，`<region>` 占位符被替换为所选值

### 6.5 Python 示例

以下是一个典型的 `autocomplete` 子命令实现，它读取解析上下文，根据当前光标前的单词决定返回何种建议：

```python
#!/usr/bin/env python
# -*- coding: utf-8 -*-

import os
import json
import sys
from typing import Iterator


# JSONL 输出的推荐数据结构
class AutocompleteSuggestion:
    def __init__(self, text: str, display_text: str, description: str = "", type: str = "", style: str = ""):
        self.text = text
        self.displayText = display_text
        self.description = description
        self.type = type
        self.style = style

    def to_jsonl(self) -> str:
        return json.dumps({
            "text": self.text,
            "displayText": self.displayText,
            "description": self.description,
            "type": self.type,
            "style": self.style,
        }, ensure_ascii=False)


def autocomplete() -> Iterator[AutocompleteSuggestion]:
    query = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_QUERY", "")
    cwords_str = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_CWORDS", "[]")
    cword_idx = int(os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_CWORD_IDX", "0"))
    prev_word = os.getenv("IMAGE_FUNNEL_AUTOCOMPLETE_PREV_WORD", "")

    cwords: list[str] = json.loads(cwords_str)
    q = query.lower()

    # 示例：识别到 --region 选项时补全区域名称
    if prev_word == "--region":
        regions = ["positive", "negative", "inpaint"]
        for name in regions:
            if q and not name.startswith(q):
                continue
            yield AutocompleteSuggestion(
                text=name,
                display_text=name,
                description=f"{name} 区域",
                type="region",
            )

    # 示例：识别到 lora 子命令时补全 Lora 名称
    if prev_word == "lora" or (len(cwords) >= 1 and cwords[0] == "lora"):
        loras = get_available_loras()
        for lora_name in loras:
            if q and not lora_name.lower().startswith(q):
                continue
            yield AutocompleteSuggestion(
                text=lora_name,
                display_text=lora_name,
                description="可用 Lora",
                type="lora",
            )


def main() -> None:
    if len(sys.argv) > 1 and sys.argv[1] == "autocomplete":
        for s in autocomplete():
            print(s.to_jsonl())
        sys.exit(0)

    # 正常执行模式
    # ...


if __name__ == "__main__":
    main()
```

### 6.6 错误处理

- 脚本退出码非 0 时，Runner 会记录错误日志并返回空结果（不会阻塞用户输入）。
- JSONL 中某一行解析失败会导致整个自动完成请求出错，并返回错误给前端。
- 脚本应使用 `stderr` 输出调试或错误日志，不要污染 `stdout`，因 `stdout` 的全部内容必须为有效的 JSONL。
