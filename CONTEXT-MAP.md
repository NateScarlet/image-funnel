# ImageFunnel 上下文映射 / Context Map

本项目是一个多上下文代码库，将核心应用领域与外部集成进行了解耦划分。

- **主应用 / Main App**: 包含核心的 ImageFunnel 筛选逻辑（前端界面、后端业务 domain、接口和 screening session 会话管理等）。
  - 上下文定义文件 / Context File: [CONTEXT.md](CONTEXT.md)
  - 架构决策目录 / ADRs Directory: [docs/adr/](docs/adr/)
- **外部钩子示例 / Example Hooks**: 包含与 ComfyUI 工作流、提示词标签自动补全（例如 Danbooru 搜索）、以及图片物理归类分流（Fork）相关的实现逻辑。
  - 上下文定义文件 / Context File: [example_hooks/CONTEXT.md](example_hooks/CONTEXT.md)
