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
ComfyUI 提示词编辑时基于 [DanbooruSearchOnline](https://github.com/SuzumiyaAkizuki/DanbooruSearchOnline) 的输入联想。支持根据关键字进行语义搜索（Suggestions）或根据已有标签推荐关联标签（Related Tags）。

[接口文档](https://sakizuki-danboorusearch.hf.space/api/openapi.json)

**目录分流 / Fork**
根据指令参数将筛选保留的图片及配套的 XMP 文件，移动到同级按规则命名的子目录（例如 `原目录名,suffix`）中，以实现图片的物理归类。

**运行器 / Runner**
外部 Python 脚本的统一调度入口 `runner.py`，负责解析命令行参数并分发给具体子命令模块。
