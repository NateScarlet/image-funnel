---
name: "backend-domain"
description: "创建新的 Domain 聚合，包括实体、仓储接口和领域事件。Invoke when adding new business logic or domain models."
---

# 创建新的 Domain

## 创建步骤

### 1. 创建 Domain 目录

在 `internal/domain/` 目录下创建新的 domain 目录：

```bash
mkdir -p internal/domain/example
```

### 2. 定义实体

参考 [session.go](internal/domain/session/session.go) 中的实体定义。

创建实体文件 `example.go`。

### 3. 定义仓储接口

参考 [repository.go](internal/domain/session/repository.go) 中的仓储接口。

创建仓储接口文件 `repository.go`。

### 4. 定义领域事件（可选）

如果需要领域事件，创建 `events.go`。

### 5. 编写测试用例

参考 [session_test.go](internal/domain/session/session_test.go) 中的测试示例。

创建测试文件 `example_test.go`。

### 6. 实现仓储（Infrastructure 层）

参考 [session_repository.go](internal/infrastructure/inmem/session_repository.go) 中的仓储实现。

直接创建 `internal/infrastructure/inmem/example_repository.go`。不要按领域分包，避免名称冲突。

### 7. 在接口层使用（可选）

在 GraphQL resolver 中使用新的 domain。

## 项目特定规则

### 实体规则

- 所有字段不导出，只能通过方法访问
- Getter 方法处理 nil 值，返回默认值
- Setter 方法验证输入值的有效性
- 使用 `NewXXX` 风格命名构造函数
- 校验参数是否有效
- 参数顺序与字段顺序一致
- 使用 UUID 生成唯一标识符
- 错误命名使用 `Err{Entity}{Action}` 格式

### 仓储接口规则

- 定义所有必要的数据访问方法
- 接口方法返回错误

### Infrastructure 实现规则

- 使用 `sync.RWMutex` 进行并发控制
- 实现 domain 层定义的接口
- 使用 `var _` 编译时检查确保接口实现正确

## 注意事项

- 领域层不依赖任何外部框架或基础设施
- 领域层只包含纯业务逻辑
- 使用接口定义依赖，不依赖具体实现
- 确保所有公共 API 都有文档注释
- 遵循 Go 语言的最佳实践
