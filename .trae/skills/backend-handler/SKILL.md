---
name: "backend-handler"
description: "创建新的 Application Handler，包括 DTO、Handler 实现和集成到 Root。Invoke when adding new use cases or application services."
---

# 创建新的 Application Handler

## 创建步骤

### 1. 创建 Handler 目录

在 `internal/application/` 目录下创建新的 handler 目录：

```bash
mkdir -p internal/application/example
```

### 2. 定义 DTO

参考 [session/handler.go](internal/application/session/handler.go) 中的 DTO 定义。

创建 `dto.go` 文件。

### 3. 定义 Handler 接口

参考 [session/handler.go](internal/application/session/handler.go#L11-L30) 中的 Handler 定义。

创建 `handler.go` 文件。

### 4. 实现 Handler 方法

参考 [session/handler.go](internal/application/session/handler.go#L32-L64) 中的方法实现。

在 `handler.go` 中实现方法。

### 5. 集成到 Root

参考 [application/root.go](internal/application/root.go) 中的 Root 定义。

在 `internal/application/root.go` 中添加新 handler。

### 6. 编写测试用例

创建 `handler_test.go` 文件。

### 7. 在接口层使用（GraphQL Resolver）

在 `internal/interfaces/graphql/` 中创建 resolver。

## 项目特定规则

### DTO 规则

- DTO 用于层间数据传递
- 使用 JSON 标签便于序列化
- DTO 应该简单，不包含业务逻辑

### Handler 接口规则

- Handler 接口定义用例的协调逻辑
- Handler 依赖领域层接口，不依赖具体实现
- 使用 `NewHandler` 风格命名构造函数
- 私有字段使用小写字母开头

### Handler 方法规则

- 应用层处理事务边界
- 应用层负责领域事件的发布（如果需要）
- Handler 方法应该简洁，主要协调领域对象
- 错误处理应该清晰，避免吞掉错误

## 注意事项

- 应用层依赖领域层接口，不直接访问基础设施
- 应用层处理事务边界
- 应用层负责领域事件的发布
- Handler 方法应该简洁，主要协调领域对象
- DTO 应该简单，不包含业务逻辑
- 确保所有公共 API 都有文档注释
- 使用 context 包传递请求上下文
