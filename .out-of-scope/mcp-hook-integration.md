# MCP 钩子集成

应用不提供把钩子执行抽象为 MCP（Model Context Protocol）能力的机制：不做应用侧的 MCP client 连接管理、不发明 `image_funnel/*` 协议集、不提供编排 DSL 或参数模板，也不暴露应用自身的 MCP server 面。

## Why this is out of scope

最初设想是"钩子能力开发一次，同时接入应用与 AI 智能体"，曾设计了多个方案（桥接进程暴露专用方法集、capabilities 协商、声明式流程 DSL 等），评审后全部否决，核心原因：

- **应用不发明私有协议/DSL**。任何应用侧专用方法集（如 `image_funnel/*`）或编排 DSL 都要求钩子开发者额外学习一套 ImageFunnel 特有语言，这与"社区不了解 ImageFunnel 的 MCP 也能接入"的目标相悖——为通用接入付出的复杂度超过了它的价值。
- **MCP 集成是脚本职责**。需要接入 MCP 的钩子（如 ComfyUI）由脚本自行作为 MCP client 连接外部 MCP server；外部 server 由用户/第三方自行托管，AI 智能体直接连接，脚本复用同一 server。应用既不感知也不管理这些连接。
- **应用侧保留的唯一收益是自动补正常驻化**。避免每次补全请求反复 spawn 脚本的优化，作为独立工作（见 #98），通过 `[directive.autocomplete].protocol = "json-rpc"` 实现，与 MCP 无关。

```toml
# 应用侧不存在的 MCP 配置——已否决
# [mcp]
# server = "comfyui-mcp"
# tool = "add_prompt"
# [mcp.args] ...
```

## Prior requests

- #97 — "将钩子执行抽象为独立 MCP server（薄桥接）"，经评审大幅缩小后关闭，唯一保留项转为 #98。
