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
ComfyUI 提示词编辑时的输入联想，包含两种能力：按关键字给出候选标签（搜索），以及按已有标签推荐关联标签（关联联想）。候选标签受类目与 NSFW 开关约束。`/add` 指令在无输入且工作流含区域标记、且尚未指定 `--region`/`--node` 目标时，优先以 `--region <name>` 选项形式直接建议全部可用区域；选定目标或工作流无区域后才进入关联标签推荐。

**两种数据来源（入口按环境变量选择，二选一或均未配置）：**

- **在线服务**：`DANBOORU_SEARCH_URL` 非空时启用，走在线语义搜索服务，结果经本地缓存装饰。
- **本地编译产物**：`DANBOORU_DATA_DIR` 显式非空时**优先**启用（即使在线地址同时配置），完全不依赖在线服务；未设置时默认使用应用数据目录下的本地 Danbooru 目录，且**仅当该目录已存在编译产物**时才启用（未编译则回落在线服务，避免破坏在线用户）。本地产物由独立脚本从外部项目的源数据编译（参数为源数据目录），运行时缺产物时快速失败并提示运行编译脚本。

两种来源均以 `DANBOORU_SEARCH_INCLUDE_NSFW` 控制是否包含 NSFW；源数据的类目数字编码在编译期映射为 `General`/`Artist`/`Copyright`/`Character`/`Meta`。

[接口文档](https://sakizuki-danboorusearch.hf.space/api/openapi.json)

**强匹配 / Exact Match**
查询与标签名或某个中文别名精确相等。命中强匹配即认为用户意图明确，**字面匹配**与**语义层**都会据此让路（语义层不再追加），因此强命中路径的可用性不受语义层依赖影响。

**字面匹配 / Literal Match**
基于字符的分层匹配：从最严格到最宽松依次判定（精确 > 中文别名精确 > 前缀 > 子串 > 模糊笔误容忍），同层按标签热度降序，并容忍常见笔误。完全离线，无需任何外部服务。

**语义层 / Semantic Layer**
**仅在字面匹配没有任何强匹配时**追加的召回阶段：把查询与标签都表示为向量，按相似度找出语义相近的标签，补充「只记得语义、说不出标签名」的场景。召回结果经 NSFW 过滤与热度加权后并入同一份排序——因此字面命中并非总是排在语义命中之前，强相似度的语义命中会排在宽松的字面命中之上。语义层只作用于搜索，**关联联想**行为不变。

**标签向量 / Tag Embedding**
标签在语义层中参与相似度比较的向量表示，按标签名、中文名与释义分视图保存，编译期生成，行序与标签表严格一致。

语义层需要**标签向量产物**与**可达的嵌入服务**同时成立才激活；缺任一时补全完全退回纯字面匹配，对未配置用户零行为变化。嵌入服务由用户通过 `HOOK_AUTOCOMPLETE_EMBEDDING_PROVIDER_URL` 指定（OpenAI 兼容接口），查询的向量结果会被缓存，连续输入不重复请求；嵌入服务超时或失败时本次查询跳过语义层，仍返回字面结果并附一条错误提示，补全始终可用。取舍与理由见 [ADR 0005](../docs/adr/0005-remote-openai-compatible-embeddings-for-danbooru-semantic-layer.md)。

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

**ComfyUI 模型提示词格式 / ComfyUI Model Prompt Format**
某个模型期望的提示词标签语法，在笔记中通过 `/set-model-format` 指令配置。三种取值：`anima`（普通标签小写且以空格分隔，`score_*` 评分标签保留下划线）、`sdxl`（标签以下划线连接）、`disabled`（跳过一切重排，作为 opt-out）。

**格式键 / Format Key**
用于确定提示词格式的模型标识，从 `CLIPTextEncode` 沿 `clip` 连线回溯得到。检查点/UNet 与 `DualCLIPLoader` 可作为格式键；普通 `CLIPLoader` 只提供文本编码权重、不决定标签格式，其链接终止时沿 `model` 连线回退到检查点。

**格式推导 / Format Derivation**
从格式键到生效格式的判定。优先级：显式映射（含 `disabled`）> 提示词推理 > 默认格式。提示词推理剔除注释与 `score_*` 标签后比较空格与下划线数量：空格多于下划线判为 `anima`，否则判为 `sdxl`，均无则无法判断并回落到默认格式。

**目标提示词节点 / Target Prompt Node**
一次 `/add` 请求最终会写入的 `CLIPTextEncode` 节点。确定方式与提交路径一致：显式 `--region`/`--node` 优先，未指定时按 `--neg` 取 `positive`/`negative` 默认区域，多目标时取第一个目标。`/add` 补全的建议插入文本按该节点所属模型的提示词格式重排，因此与同一输入提交后落盘的文本形态一致；`/remove` 与 `/adjust prompt` 的建议给出的是工作流中已有原文，保持不变。

**建议文本格式化 / Suggestion Text Formatting**
把补全候选标签转成可直接插入的文本：先按目标提示词节点的模型格式重排，再按重排结果决定是否用引号包裹（`anima` 把下划线换成空格后会新增引号需求）。展示用文本（`displayText`）保持原文含中文名。已见提示词同样重排到目标格式后再比对，否则建议被格式化后与原文形态不同，「(已有)」标记会失效。格式化依赖由补全入口按指令构建并注入（仅 `/add` 需要，其余指令注入空实现），复用提交路径的同一实现（含首次推理后写回配置的副作用，见 ADR 0006）；`IMAGE_FUNNEL_DATA_DIR` 缺失时快速失败，不静默输出原文建议。

