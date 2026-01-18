---
name: "backend-development"
description: "后端开发指南，包含 Go 语言、GraphQL、领域驱动设计（DDD）和测试的开发规范和最佳实践"
---

# 后端开发指南

所有包名为："main/"　下的子包，例如："main/internal/"，"main/graph/" 等。

## 技术栈

- **语言**: Go
- **GraphQL 框架**: gqlgen
- **架构**: 领域驱动设计（DDD）
- **测试**: testify

## 开发规范

### 字段访问

所有字段没有特别理由，都不应该导出，只能通过方法访问。

### Getter 方法

- 应该处理 nil 值，返回默认值或空字符串等
- 避免给 getter 添加 `Get` 前缀

### Setter 方法

应该验证输入值的有效性，避免无效状态。

### 构建函数

- 使用 `NewXXX` 风格命名
- 校验参数是否有效
- 参数顺序与字段顺序一致

### Options 模式

使用 Options 模式来指定命名参数。命名参数的名称以 `{函数名称}With` 开头，后面跟着参数名的驼峰式命名。

### 架构设计

使用领域驱动设计（DDD）架构，将业务逻辑与数据访问分离。

## 项目结构

```
├── cmd/server/          # 后端入口
├── graph/               # GraphQL schema 和 resolver
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
│   ├── mutations/          # Mutation 定义
│   ├── models_gen.go       # gqlgen 自动生成的模型
│   ├── resolver.go        # 主 resolver 入口
│   ├── scalars.go         # 自定义标量类型（Time、Upload、URI）
│   └── *.resolvers.go     # 各 mutation/query 的 resolver 实现
└── internal/
    ├── application/       # 应用层：协调领域对象执行业务用例
    │   ├── session/        # Session 应用服务
    │   │   ├── handler.go    # Session 处理器
    │   │   ├── dto.go        # Session 数据传输对象
    │   │   ├── event_bus.go  # Session 事件总线接口
    │   │   └── url_signer.go # URL 签名接口
    │   ├── directory/     # Directory 应用服务
    │   │   ├── handler.go    # Directory 处理器
    │   │   └── dto.go        # Directory 数据传输对象
    │   └── root.go        # 应用层根，组合所有处理器
    ├── domain/           # 领域层：核心业务逻辑和领域模型
    │   ├── session/        # Session 聚合
    │   │   ├── session.go    # Session 实体
    │   │   ├── repository.go # Session 仓储接口
    │   │   ├── events.go     # Session 领域事件
    │   │   └── xmp_helper.go # XMP 辅助方法
    │   ├── directory/     # Directory 聚合
    │   │   ├── directory.go  # Directory 实体
    │   │   ├── scanner.go    # Directory 扫描器接口
    │   │   └── repository.go # Directory 仓储接口
    │   └── metadata/      # Metadata 聚合
    │       ├── xmp.go        # XMP 元数据
    │       ├── repository.go # Metadata 仓储接口
    │       └── in_memory.go  # 内存仓储实现
    ├── infrastructure/   # 基础设施层：技术实现细节
    │   ├── inmem/         # 内存实现
    │   │   └── session_repository.go # Session 内存仓储
    │   ├── localfs/       # 本地文件系统实现
    │   │   ├── scanner.go    # 文件系统扫描器
    │   │   └── scanner_test.go
    │   ├── urlconv/       # URL 转换和签名
    │   │   ├── signer.go     # URL 签名器
    │   │   └── signer_test.go
    │   ├── xmpsidecar/    # XMP 侧边文件处理
    │   │   ├── repository.go # XMP 仓储实现
    │   │   ├── repository_test.go
    │   │   └── samples/     # XMP 测试样本
    │   └── ebus/          # 事件总线实现
    │       ├── event_bus.go  # 事件总线
    │       └── event_bus_test.go
    ├── pubsub/           # 发布订阅抽象
    │   ├── topic.go       # Topic 接口
    │   ├── in_memory.go   # 内存 Topic 实现
    │   ├── in_memory_test.go
    │   └── error.go       # 错误定义
    └── util/             # 通用工具
        ├── atomic_save.go # 原子文件保存
        └── atomic_save_test.go
```

## DDD 层次说明

### Domain 层（领域层）

领域层是核心业务逻辑层，包含：

- **实体（Entity）**：具有唯一标识的对象，如 `Session`
- **值对象（Value Object）**：没有标识符的对象，如 `ImageFilters`
- **聚合（Aggregate）**：一组相关实体和值对象的集合，如 `Session` 聚合包含 `Image` 实体
- **仓储接口（Repository Interface）**：定义数据访问的抽象接口
- **领域事件（Domain Events）**：表示领域内发生的重要事件

**规则**：
- 领域层不依赖任何外部框架或基础设施
- 领域层只包含纯业务逻辑
- 使用接口定义依赖，不依赖具体实现

### Application 层（应用层）

应用层协调领域对象执行业务用例：

- **Handler**：处理用例的协调逻辑
- **DTO（Data Transfer Object）**：数据传输对象，用于层间数据传递
- **Root**：应用层根，组合所有处理器供外部使用

**规则**：
- 应用层依赖领域层接口
- 应用层不直接访问基础设施
- 应用层处理事务边界
- 应用层负责领域事件的发布

### Infrastructure 层（基础设施层）

基础设施层提供技术实现：

- **Repository 实现**：实现领域层定义的仓储接口
- **外部服务适配器**：适配外部系统接口
- **技术实现**：如文件系统、数据库、消息队列等

**规则**：
- 基础设施层实现领域层定义的接口
- 基础设施层按技术组织（如 inmem、localfs、urlconv）
- 基础设施层包含技术细节和外部依赖

### Interfaces 层（接口层）

接口层处理外部交互：

- **GraphQL Resolver**：处理 GraphQL 查询和变更
- **HTTP Handler**：处理 HTTP 请求

**规则**：
- 接口层依赖应用层
- 接口层不直接访问领域层
- 接口层处理请求/响应转换

## 开发工作流

### 修改代码后

调试器会自动重新编译（如使用 `dlv`）。

### 修改 GraphQL schema 后

运行 `.\scripts\generate-graphql.ps1` 命令来同时更新前后端的 GraphQL 相关代码。

### 添加测试用例

修改后端代码后，添加必要的测试用例。

### 运行测试

- 测试单个模块：`go test --timeout 30s`
- 测试所有模块：`go test --timeout 600s ./...`

### 重新编译

运行 `.\scripts\build.ps1` 来重新编译前端和后端。

### 测试

访问 http://localhost:8080（GraphQL Playground）。

## 测试规范

### 测试框架

使用 `github.com/stretchr/testify/assert` 或 `require` 来验证结果是否符合预期。

### 测试执行

不要用 `go run` 编写测试，直接用 `go test` 运行测试。

### 错误处理

测试出错时，在测试中添加详细的日志输出，帮助定位问题。

## 常见任务

### 添加新的 GraphQL Query

1. 在 `graph/queries/` 目录下创建对应的 `.graphql` 文件
2. 使用 `extend type Query` 的形式定义（除了 `base.graphql` 中的 meta）
3. 文件命名与查询名称对应（如 `session.graphql`）
4. 运行 `.\scripts\generate-graphql.ps1` 生成 resolver 模板
5. 在 `graph/*.resolvers.go` 中实现 resolver 逻辑
6. 在 `internal/application/` 对应的 handler 中实现应用逻辑
7. 在 `internal/domain/` 对应的 domain 中实现领域逻辑（如需要）

### 添加新的 GraphQL Mutation

1. 在 `graph/mutations/` 目录下创建变更定义文件
2. 运行 `.\scripts\generate-graphql.ps1` 生成 resolver 模板
3. 在 `graph/*.resolvers.go` 中实现 resolver 逻辑
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
5. 在 `graph/*.resolvers.go` 中实现 resolver 逻辑
6. 在 `internal/application/` 对应的 handler 中实现订阅逻辑

### 添加新的 Domain

1. 在 `internal/domain/` 目录下创建新的 domain 目录
2. 定义 domain 的接口和模型
3. 实现 domain 的业务逻辑
4. 编写测试用例
5. 在 GraphQL resolver 中调用 domain 方法

### 添加新的 Application Handler

1. 在 `internal/application/` 目录下创建新的 handler 目录
2. 定义 Handler 结构体和接口
3. 实现业务用例
4. 在 `internal/application/root.go` 中添加新 handler
5. 编写测试用例

### 添加新的 Infrastructure 实现

1. 在 `internal/infrastructure/` 目录下创建新的技术实现目录
2. 实现领域层定义的接口
3. 编写测试用例
4. 在 `internal/application/` 或 `cmd/server/` 中使用新实现

## 注意事项

- 对于不常见的情况或特殊修复，添加注释说明
- 遵循 Go 语言的最佳实践
- 保持代码的可读性和可维护性
- 确保所有公共 API 都有文档注释
- 使用 context 包传递请求上下文
- GraphQL Schema 文件组织规则：
  - 每个 type 单独一个文件，放在 `graph/types/` 目录
  - 每个 enum 单独一个文件，放在 `graph/enums/` 目录
  - Query 使用 `extend type Query` 形式定义，除了 `base.graphql` 中的 meta
  - Subscription 使用 `extend type Subscription` 形式定义
  - 文件命名使用 kebab-case（如 `image-filters.graphql`）
- 使用 `var _` 编译时检查确保接口实现正确
- 领域层使用 UUID 生成唯一标识符
- 基础设施层使用适当的并发控制（如 `sync.RWMutex`）
- 错误命名使用 `Err{Entity}{Action}` 格式（如 `ErrSessionNotFound`）
