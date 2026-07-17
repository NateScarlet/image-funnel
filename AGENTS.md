# AGENTS.md

This file provides guidance to AI agents working with the ImageFunnel repository.

## 项目概览

ImageFunnel 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流帮助用户从大量生成结果中快速筛选出优质图片。

### 核心特性

- **XMP Sidecar 优先**: 不修改原始图片文件，通过独立的 XMP 文件存储筛选结果
- **专业工具兼容**: Adobe Lightroom/Bridge、XnView 等专业图片管理工具可直接读取评分
- **移动优先**: 深度优化的响应式 Web 界面，支持触摸手势
- **三态分类**: 保留/搁置/排除，避免决策疲劳
- **全功能撤销**: 支持跨轮、跨筛选条件的撤销操作
- **高性能**: 智能预加载、按需加载

### 技术栈

- **前端**: Vue 3 + TypeScript + Vite + Tailwind CSS 4
- **后端**: Go 1.24 + gqlgen + gorilla/mux
- **API**: GraphQL (Apollo Client + WebSocket subscriptions)
- **数据存储**: XMP sidecar 文件，无数据库

## 项目结构

```
image-funnel/
├── cmd/                 # 应用入口
│   └── server/
├── frontend/            # 前端项目
│   ├── src/
│   │   ├── components/  # Vue 组件
│   │   ├── composables/ # 可复用逻辑
│   │   ├── graphql/     # GraphQL 查询、变更、订阅
│   │   ├── views/       # 页面视图
│   │   └── utils/       # 工具函数
├── graph/               # GraphQL schema
│   ├── enums/
│   ├── mutations/
│   ├── queries/
│   ├── subscriptions/
│   └── types/
├── internal/            # 后端业务逻辑（六边形架构）
│   ├── domain/          # 核心业务逻辑，零外部依赖
│   │   ├── session/     # Session 聚合
│   │   ├── image/       # Image 实体
│   │   ├── directory/   # Directory 实体
│   │   ├── metadata/    # 元数据接口
│   │   └── note/        # Note 实体
│   ├── application/     # 应用层，业务层的简单封装
│   ├── infrastructure/  # 基础设施层
│   ├── interfaces/      # 接口层
│   │   ├── graphql/     # GraphQL resolvers
│   │   └── http/        # HTTP 路由
│   └── shared/          # 共享的无逻辑基础结构
└── scripts/             # 脚本
    ├── build.ps1        # 构建脚本
    └── generate-graphql.ps1 # 更新 GraphQL 相关代码
```

## 常用命令

```bash
# 前端
pnpm dev                 # 启动 Vite 开发服务器 (端口 8080)
pnpm build               # 生产环境构建
pnpm check               # oxlint 类型检查 + lint 自动修复 (修改前端后必须运行)
pnpm lint                # 仅 oxlint
pnpm lint:fix            # oxlint auto-fix
pnpm fmt                 # 使用 oxfmt 格式化代码

# 后端
go test --timeout 30s ./...      # 运行所有 Go 测试 (修改后端后必须运行)
go test --timeout 30s ./internal/domain/session  # 运行特定包的测试

# 构建与生成
pwsh scripts/build.ps1            # 完整构建 (前端 + Go，输出到 build/latest/)
pwsh scripts/run.ps1              # 开发模式 (同时运行前端和后端)
pwsh scripts/generate-graphql.ps1 # 重新生成 GraphQL 代码 (Go + TypeScript)
```

## 架构说明

后端遵循**六边形架构**（端口与适配器）：

1. **领域层 (domain)**: 核心业务逻辑，不依赖任何外部库
2. **应用层 (application)**: 编排领域层，提供用例
3. **基础设施层 (infrastructure)**: 技术实现（如内存存储、本地文件系统、XMP sidecar 等）
4. **接口层 (interfaces)**: 对外适配器（GraphQL、HTTP）

数据流程示例（标记图片）：

1. 用户操作 → Apollo Client 发送 `markImage` 变更
2. GraphQL resolver → `application/session.Handler.MarkImage()`
3. 应用层 → `domain/session.Service.MarkImage()`
4. Session 聚合更新队列、撤销栈、统计数据
5. 通过 `pubsub.Topic` 发布变更 → WebSocket 订阅 → 前端 UI 更新

## 关键约定

- **遵守规范：**　修改代码前，查看 `CODING_STANDARDS.md` 中的通用原则和相关语言的要求。
- **立即实现：**　立即实现所有用户要求的功能，不能偷懒用注释标记为以后实现
- **完整重构：**　内部接口更改应修改所有调用者使用新的接口，不得以向下兼容为由保留旧的接口，
- **注释**: 使用中文添加对理解上下文有帮助的注释，避免简单翻译代码
- **Region 注释**: 使用 `// #region {分组名称}` / `// #endregion` 包裹长段关联代码
- **不修改生成的代码**: 使用对应脚本重新生成
- **构建**: 优先使用 `scripts/build.ps1`，避免直接运行底层命令
- **临时产物**: 工具临时产物放在 `.scratch`，不主动清理
- **持久化上下文：** 遇到问题优先查看 `CONTEXT.md` 和 `CONTEXT-MAP.md`，用户澄清后**总是**更新对应 `CONTEXT.md`

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues (no external PR triage). See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles use their default names. See `docs/agents/triage-labels.md`.

### Domain docs

Multi-context ("app" and "example_hooks"). See `docs/agents/domain.md`.
