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
│   ├── schema.graphql  # 主 GraphQL schema 定义
│   ├── models_gen.go   # gqlgen 自动生成的模型
│   ├── resolver.go     # 主 resolver 入口
│   ├── scalars.go      # 自定义标量类型（Time、Upload、URI）
│   ├── *.resolvers.go  # 各 mutation/query 的 resolver 实现
│   └── mutations/      # Mutation 定义文件
└── internal/
    ├── preset/          # 预设管理
    ├── scanner/         # 图片扫描
    ├── session/         # 会话管理
    └── xmp/             # XMP 文件处理
```

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

1. 在 `graph/schema.graphql` 中定义查询
2. 运行 `.\scripts\generate-graphql.ps1` 生成 resolver 模板
3. 在 `graph/*.resolvers.go` 中实现 resolver 逻辑
4. 在 `internal/` 对应的 domain 中实现业务逻辑

### 添加新的 GraphQL Mutation

1. 在 `graph/schema.graphql` 中定义变更
2. 在 `graph/mutations/` 目录下创建变更定义文件
3. 运行 `.\scripts\generate-graphql.ps1` 生成 resolver 模板
4. 在 `graph/*.resolvers.go` 中实现 resolver 逻辑
5. 在 `internal/` 对应的 domain 中实现业务逻辑

### 添加新的 Domain

1. 在 `internal/` 目录下创建新的 domain 目录
2. 定义 domain 的接口和模型
3. 实现 domain 的业务逻辑
4. 编写测试用例
5. 在 GraphQL resolver 中调用 domain 方法

## 注意事项

- 对于不常见的情况或特殊修复，添加注释说明
- 遵循 Go 语言的最佳实践
- 保持代码的可读性和可维护性
- 确保所有公共 API 都有文档注释
- 使用 context 包传递请求上下文
