---
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
│   ├── enums/             # Enum 类型定义
│   ├── queries/            # Query 定义
│   ├── subscriptions/      # Subscription 定义
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

在更新 resolver 实现之前
运行 `.\scripts\generate-graphql.ps1` 命令来同时更新前后端的 GraphQL 相关代码。

### 代码生成规则

- `graph/` 目录只包含 `.graphql` 定义文件
- `internal/interfaces/graphql/` 目录包含所有生成的 Go 代码
- `frontend/src/graphql/` 目录包含前端 GraphQL 相关代码
- 自定义代码放在 `resolver.go`、`scalars.go`、`*.resolvers.go` 和前端的对应目录中

## Schema 文件组织规则

- 每个文件围绕一个字段/类型，放在对应的子目录中
- 文件命名和其中主要的字段或类型相同，使用 snake_case（如 `image_filters.graphql`）
- 使用 `extend type` 形式定义根对象上的额外字段（除了第一个字段）

## 常见任务

**添加 GraphQL 元素**：在 `graph/` 对应子目录创建 `.graphql` 文件，运行 `.\scripts\generate-graphql.ps1` 生成代码，在 `internal/interfaces/graphql/*.resolvers.go`、`internal/application/` 和 `internal/domain/` 实现逻辑。

**前端使用 GraphQL**：在 `frontend/src/graphql/` 对应目录创建 `.gql` 文件，使用 `@/graphql/utils/` 工具函数执行查询或变更。

**Schema 组织**：Type/Enum 单独文件，使用 `@goModel` 指定 Go 类型；Query/Subscription 用 `extend type` 定义；文件命名使用 snake_case。

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

- 不要手动编辑生成的文件（`generated.go`、`models_gen.go` 和 `generated.ts`），而是运行 `.\scripts\generate-graphql.ps1` 生成新代码
- 修改 schema 后必须重新生成代码
- 不用保持 schema 的向后兼容性（前后端总是一起更新）
