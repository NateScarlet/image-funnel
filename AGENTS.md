# AGENTS.md

This file provides guidance to AI agents working with the ImageFunnel repository.

## 项目概览

ImageFunnel 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流帮助用户从大量生成结果中快速筛选出优质图片。

### 核心特性

- **XMP Sidecar 优先**: 不修改原始图片文件，通过独立的 XMP 文件存储筛选结果
- **专业工具兼容**: Adobe Lightroom/Bridge、XnView 等专业图片管理工具可直接读取评分
- **移动优先**: 深度优化的响应式 Web 界面，支持触摸手势
- **三态分类**: 保留(5星)/搁置(3星)/排除，避免决策疲劳
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
│   │   └── memo/        # Memo 实体
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
pnpm check               # TypeScript 类型检查 + ESLint (修改前端后必须运行)
pnpm lint                # 仅 ESLint

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

### Go

- **无 `Get` 前缀**: 查询方法直接使用大写名称，如 `Session()` 而非 `GetSession()`
- **`iter.Seq` / `iter.Seq2[T, error]`**: 使用迭代器模式减少数组分配
- **构造函数**: 使用 `New` 前缀，如需清理，将清理函数作为第二个返回值
- **编译时接口检查**: 使用 `var _ Interface = (*Impl)(nil)`
- **错误处理**: 绝不静默忽略错误，使用 `errors` 包处理
- **业务错误**: 使用 `internal/apperror` 包
- **枚举**: 放在 `internal/shared/enums.go`
- **日志**: 使用 zap，日志消息小写，记录耗时用 `duration` 字段，长耗时操作前后用 `will`/`did` 前缀
- **测试**: 新增功能时添加对应的单元测试，测试文件名与逻辑文件对应
- **Context**: 使用 `context.Context` 传递请求上下文

### Vue / TypeScript

- **声明式优先**: 使用 `computed` 而非 `watch` 维护状态
- **模板引用**: 使用 `useTemplateRef`（单个）或 `@/composables/useTemplateRefs`（数组）
- **Composable 参数**: 使用 `MaybeRefOrGetter` 除非有特殊理由
- **null vs undefined**: 返回值使用 `undefined`，参数接受 `null`
- **GraphQL 类型**: 直接使用 `@/graphql/generated` 生成的类型
- **加载状态**: 使用样式和动画，避免文本提示导致布局抖动
- **lodash 替代**: 使用 `es-toolkit`
- **`useStorage`**: 用于 localStorage，键格式为 `name@randomSuffix`，在 `<script lang="ts">` 块中定义以共享状态

### GraphQL

- **Fragment**: 使用 fragment 避免重复定义查询字段，命名不带 Fragment 后缀
- **Schema 文件**: 每个字段/类型一个文件，使用 `snake_case` 命名
- **类型扩展**: 根对象字段超出第一个时使用 `extend type`
- **Schema 变更后**: 运行 `pwsh scripts/generate-graphql.ps1`，然后更新 resolvers

### 通用

- **注释**: 使用中文添加对理解上下文有帮助的注释，避免简单翻译代码
- **Region 注释**: 使用 `// #region {分组名称}` / `// #endregion` 包裹长段关联代码
- **ID**: 客户端不应尝试解析 ID，格式不固定
- **不修改生成的代码**: 使用对应脚本重新生成
- **构建**: 优先使用 `scripts/build.ps1`，避免直接运行底层命令
