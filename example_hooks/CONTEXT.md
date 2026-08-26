# ImageFunnel 外部钩子（Example Hooks）领域语言

本上下文定义了 `example_hooks` 模块特有的、主应用无需感知的语义和概念，主要涉及通过外部 Python 脚本和 ComfyUI 交互来扩展主应用的功能。

## 核心概念

**钩子配置 / Hook Config**
定义在 `.toml` 文件中的外部钩子声明，包括元数据、可执行脚本命令行、环境配置以及触发条件（例如在提交会话后触发）。

**指令 / Directive**
在笔记（Note）中以 `/` 开头的斜杠指令（例如 `/fork`, `/add`）。钩子服务在触发时扫描笔记并匹配相应的指令以提取参数。

**工作流提示词操作 / ComfyUI Workflow Prompt Action**
与 ComfyUI 交互的特定动作类型，包含：

- **添加提示词 / Add Prompt**: 给符合特定评分（例如 4 星以上）的图片所关联的 ComfyUI 工作流添加新的提示词标签并发送到生成队列。
- **调整权重 / Adjust Weight**: 调整工作流中特定 Lora 的权重或提示词的比重。
- **移除提示词 / Remove Prompt**: 自动剔除工作流中的某些标签或节点。

**Danbooru 标签自动补全 / Danbooru Autocomplete**
ComfyUI 提示词编辑时基于 [DanbooruSearchOnline](https://github.com/SuzumiyaAkizuki/DanbooruSearchOnline) 的输入联想。支持根据关键字进行语义搜索（Suggestions）或根据已有标签推荐关联标签（Related Tags）。`/add` 指令在无输入且工作流含区域标记、且尚未指定 `--region`/`--node` 目标时，优先以 `--region <name>` 选项形式直接建议全部可用区域；选定目标或工作流无区域后才进入关联标签推荐。

[接口文档](https://sakizuki-danboorusearch.hf.space/api/openapi.json)

**目录分流 / Fork**
根据指令参数将筛选保留的图片及配套的 XMP 文件，移动到同级按规则命名的子目录（例如 `原目录名,suffix`，未指定 suffix 时默认为 `TODO`）中，以实现图片的物理归类。

**输出目录调整 / Output Directory Adjustment**
ComfyUI 钩子在提交前将工作流输出节点的 `filename_prefix` 自动调整为图片当前所在目录（相对 ComfyUI 输出目录的 rel_dir）的过程。期望行为是输出文件**总是直接落在图片当前目录下，不创建任何子目录**：rel_dir 之外的所有目录层级一律拍平为 `__` 连接的文件名前缀，包括纯字符串前缀中的字面目录（`C/D/image_` → `C__D__image_`）、日期模板前的字面目录（`C/D/%date:...%` → `C__D__%date:...%`）以及无法映射 rel_dir 时模板变量之间的分隔符（`%Project.value%/%Title.value%/...` → `%Project.value%__%Title.value%__...`）。唯一的例外是模板非日期变量与 rel_dir 分段匹配成功时：变量值本身充当 rel_dir 路径（如 `%Project.value%/%Title.value%` 对应 `NewProject/NewTitle`），此时分隔符保留。拍平时先按标准路径清理合并连续分隔符（ComfyUI 对连续分隔符本就是合并处理的，如字面 `TODO//x` → `TODO__x`），再逐分隔符替换；段名中字面的 `__` 不被改动（`a/__b` → `a____b`）。prompt 严格由 workflow 模板简单求值（变量替换 + 日期替换）得到，不做任何额外清理，因此模板变量求值为空时会残留连续 `__`（如 `%Title.value%` 为空时 `%Project.value%__%Title.value%__%date:...%` 求值为 `TODO____<date>`）。约束：**不得静默丢弃原有路径数据**（直接取 basename 是错误做法）；workflow 模板与 prompt 求值结果必须保持一致（prompt 不能持有 workflow 无法复现的值）。

**运行器 / Runner**
外部 Python 脚本的统一调度入口 `runner.py`，负责解析命令行参数并分发给具体子命令模块。

**常驻补全服务 / Autocomplete Serve**
`comfyui.autocomplete serve` 子命令将补全脚本作为 **JSON-RPC 常驻服务**运行：stdin 读请求、stdout 写响应、stderr 记录日志，stdin 关闭即退出。请求参数沿用自动补全上下文（`cwords`/`cwordIdx`/`prevWord`/`linePrefix`/`query` + `imagePaths`/`imageIDs`/`notePath` + `rootDir`/`directoryRelPath`），响应复用现有 JSONL 建议结构，目标指令名从 `cwords[0]` 推导。**依赖由 serve 入口构建并注入**：解析器构建一次复用；Danbooru 提供者与操作历史按请求的目录上下文（`rootDir`/`directoryRelPath`）逐请求构建，初始化失败直接抛出（快速失败，不降级）。每个请求在独立线程处理，收到 `$/cancelRequest` 后标记取消（尽力而为中断，线程不可强杀），已取消的请求不再返回结果；请求执行失败以 JSON-RPC error 上报。配合 TOML 配置 `[directive.autocomplete]` 设置 `protocol = "json-rpc"` 启用。

**复制工作流导出 / Copy Workflow Export**
`comfyui.copy_workflow` 子命令在用户复制图片时被应用同步调用（配合 TOML 配置 `[copy]` 能力标记），读取 PNG 内嵌的 prompt/workflow 元数据，执行与入列一致的输出目录调整后，以单行 JSON 信封 `{"content", "description"}` 输出到 stdout 供写入剪贴板。无 ComfyUI 元数据的图片输出空内容即表示不适用（前端降级复制文件本体）；`HOOK_OUTPUT_DIR=:inherit:` 时复制原始未调整的工作流。核心逻辑依赖注入（请求上下文由入口从环境变量构造、元数据加载器以参数传入），共用 `png_metadata.py` 与 `output_directory.py` 模块。

**ComfyUI 模型提示词格式配置 / ComfyUI Model Prompt Format Configuration**
维护在 `IMAGE_FUNNEL_DATA_DIR`（主应用全局数据目录）下 `comfyui_model_formats.toml` 文件中的模型标签格式派生机制。为 `CLIPTextEncode` 节点相连的模型（`ckpt_name`）推导期望的提示词标签语法，并据此把 `/add`、`/remove` 命令新增/移除的标签自动转换为该模型的格式（`anima` 格式普通标签转空格且小写、仅 `score_*` 标签保留下划线；`sdxl` 格式普通标签转下划线；`disabled` 完全跳过格式化）。**格式推导优先级：显式映射（`models[ckpt_name]`，含 `disabled`）> 提示词推理 > 默认格式**——提示词推理剔除注释与 `score_*` 标签后比较空格与下划线数量：空格多于下划线判为 `anima`，否则判为 `sdxl`，均无则无法判断并回落到默认格式；推理结果会自动记录到配置文件供后续复用。提供 `/set-model-format <model> <format>` 斜杠指令及 JSON-RPC 自动补全（模型名 + `anima`/`sdxl`/`disabled`）在笔记中快捷配置。脚本在 `IMAGE_FUNNEL_DATA_DIR` 环境变量未设置时直接抛出 RuntimeError（快速失败）。

