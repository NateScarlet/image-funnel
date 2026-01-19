---
name: "graphql"
description: "GraphQL 开发指南，包含 schema 定义、resolver 实现、类型系统、代码生成和前后端集成。Invoke when working with GraphQL schema, resolvers, running generate-graphql.ps1, or implementing GraphQL-related functionality."
---

# GraphQL 开发指南

## 项目结构

GraphQL 相关文件分为两个目录：

```
├── graph/               # GraphQL schema 定义（纯定义文件）
│   ├── scalars.graphql    # Scalar 类型定义
│   ├── directives.graphql  # Directive 定义
│   ├── types/              # Type 类型定义
│   │   ├── meta.graphql
│   │   ├── image.graphql
│   │   ├── directory.graphql
│   │   ├── rating-count.graphql
│   │   ├── image-filters.graphql
│   │   ├── write-actions.graphql
│   │   ├── session.graphql
│   │   ├── session-stats.graphql
│   │   └── queue-status.graphql
│   ├── enums/             # Enum 类型定义
│   │   ├── session-status.graphql
│   │   └── image-action.graphql
│   ├── queries/            # Query 定义
│   │   ├── base.graphql    # Query 基础定义（包含 meta）
│   │   ├── session.graphql # extend Query session
│   │   └── directory.graphql # extend Query directory
│   ├── subscriptions/      # Subscription 定义
│   │   ├── base.graphql    # Subscription 基础定义
│   │   └── session.graphql # extend Subscription sessionUpdated
│   └── mutations/          # Mutation 定义
├── internal/interfaces/graphql/  # GraphQL 实现代码
│   ├── generated.go        # gqlgen 自动生成的执行代码
│   ├── models_gen.go       # gqlgen 自动生成的模型
│   ├── resolver.go        # 主 resolver 入口
│   ├── scalars.go         # 自定义标量类型（Time、Upload、URI）
│   └── *.resolvers.go     # 各 mutation/query 的 resolver 实现
└── frontend/src/graphql/    # 前端 GraphQL 相关代码
    ├── fragments/          # GraphQL 片段
    ├── mutations/          # GraphQL 变更
    ├── queries/            # GraphQL 查询
    ├── subscriptions/      # GraphQL 订阅
    ├── utils/              # GraphQL 工具函数
    ├── client.ts           # GraphQL 客户端
    └── generated.ts        # 自动生成的前端 GraphQL 代码
```

## 配置文件

`gqlgen.yml` 包含对应配置

## 开发工作流

### 修改 GraphQL schema 后

运行 `.\scripts\generate-graphql.ps1` 命令来同时更新前后端的 GraphQL 相关代码。

该脚本会：
1. 执行 `go generate ./internal/interfaces/graphql` 生成后端代码
2. 执行 `pnpm generate:graphql` 生成前端代码
3. 检查未实现的 resolver

### 代码生成规则

- `graph/` 目录只包含 `.graphql` 定义文件
- `internal/interfaces/graphql/` 目录包含所有生成的 Go 代码
- `frontend/src/graphql/` 目录包含前端 GraphQL 相关代码
- 不要手动编辑生成的文件（`generated.go`、`models_gen.go`、`generated.ts`）
- 自定义代码放在 `resolver.go`、`scalars.go`、`*.resolvers.go` 和前端的对应目录中

## Schema 文件组织规则

- 每个 type 单独一个文件，放在 `graph/types/` 目录
- 每个 enum 单独一个文件，放在 `graph/enums/` 目录
- Query 使用 `extend type Query` 形式定义，除了 `base.graphql` 中的 meta
- Subscription 使用 `extend type Subscription` 形式定义
- 文件命名使用 kebab-case（如 `image-filters.graphql`）

## 常见任务

### 添加新的 GraphQL Query

1. 在 `graph/queries/` 目录下创建对应的 `.graphql` 文件
2. 使用 `extend type Query` 的形式定义（除了 `base.graphql` 中的 meta）
3. 文件命名与查询名称对应（如 `session.graphql`）
4. 运行 `.\scripts\generate-graphql.ps1` 生成 resolver 模板
5. 在 `internal/interfaces/graphql/*.resolvers.go` 中实现 resolver 逻辑
6. 在 `internal/application/` 对应的 handler 中实现应用逻辑
7. 在 `internal/domain/` 对应的 domain 中实现领域逻辑（如需要）

### 添加新的 GraphQL Mutation

1. 在 `graph/mutations/` 目录下创建变更定义文件
2. 运行 `.\scripts\generate-graphql.ps1` 生成 resolver 模板
3. 在 `internal/interfaces/graphql/*.resolvers.go` 中实现 resolver 逻辑
4. 在 `internal/application/` 对应的 handler 中实现应用逻辑
5. 在 `internal/domain/` 对应的 domain 中实现领域逻辑（如需要）

### 添加新的 GraphQL Schema 类型

1. 在 `graph/types/` 目录下创建对应的 `.graphql` 文件
2. 文件命名使用 kebab-case（如 `image-filters.graphql`）
3. 每个 type 单独一个文件
4. 使用 `@goModel` 指定对应的 Go 类型（通常在 `internal/application/` 或 `internal/domain/`）
5. 运行 `.\scripts\generate-graphql.ps1` 生成类型定义

### 添加新的 GraphQL Enum

1. 在 `graph/enums/` 目录下创建对应的 `.graphql` 文件
2. 文件命名使用 kebab-case（如 `session-status.graphql`）
3. 运行 `.\scripts\generate-graphql.ps1` 生成类型定义

### 添加新的 GraphQL Subscription

1. 在 `graph/subscriptions/` 目录下创建对应的 `.graphql` 文件
2. 使用 `extend type Subscription` 的形式定义
3. 文件命名与订阅名称对应（如 `session.graphql`）
4. 运行 `.\scripts\generate-graphql.ps1` 生成 resolver 模板
5. 在 `internal/interfaces/graphql/*.resolvers.go` 中实现 resolver 逻辑
6. 在 `internal/application/` 对应的 handler 中实现订阅逻辑

### 在前端使用 GraphQL

#### 创建新的前端查询

1. 在 `frontend/src/graphql/queries/` 目录下创建对应的 `.gql` 文件
2. 定义查询内容，使用生成的类型
3. 在组件中使用 `@/graphql/utils/useQuery` 或 `@/graphql/utils/query` 工具函数执行查询

#### 创建新的前端变更

1. 在 `frontend/src/graphql/mutations/` 目录下创建对应的 `.gql` 文件
2. 定义变更内容，使用生成的类型
3. 在组件中使用 `@/graphql/utils/mutate` 工具函数执行变更

#### 创建新的前端订阅

前端暂时不使用订阅，如果有需求请要求用户添加 useSubscription 工具函数。

## 类型映射

### 自定义标量类型

在 `internal/interfaces/graphql/scalars.go` 中定义：

- **Time**: `time.Time` 类型，使用 RFC3339Nano 格式序列化
- **Upload**: `github.com/99designs/gqlgen/graphql.Upload` 类型，用于文件上传

### Go 类型映射

使用 `@goModel` 指定 GraphQL 类型对应的 Go 类型：

```graphql
extend type Session @goModel(model: "main/internal/shared.SessionDTO") {
  id: ID!
  status: SessionStatus!
  # ...
}
```

## 注意事项

- 不要手动编辑 `generated.go`、`models_gen.go` 和 `generated.ts`，这些文件会在下次生成时被覆盖
- 自定义的 resolver 实现不会被覆盖，可以安全编辑
- 前端的查询、变更和订阅文件不会被自动覆盖，可以安全编辑
- 使用 `var _` 编译时检查确保接口实现正确
- 确保所有公共 API 都有文档注释
- 使用 context 包传递请求上下文
- 修改 schema 后必须重新生成代码
- 生成后运行 `pnpm check` 检查前端错误
- 使用 `pnpm lint:fix` 修复可以自动修复的前端错误
- 确保前后端类型定义一致
- 保持 schema 的向后兼容性
- 使用描述性名称和注释
