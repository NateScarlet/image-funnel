---
name: "backend-infrastructure"
description: "创建新的 Infrastructure 实现，包括 Repository 实现、外部服务适配器和技术实现。Invoke when implementing technical details or external integrations."
---

# 创建新的 Infrastructure 实现

## 创建步骤

### 1. 创建 Infrastructure 目录

在 `internal/infrastructure/` 目录下创建新的技术实现目录：

```bash
mkdir -p internal/infrastructure/example
```

常见的技术实现目录：
- `inmem` - 内存实现
- `localfs` - 本地文件系统实现
- `urlconv` - URL 转换和签名
- `xmpsidecar` - XMP 侧边文件处理
- `ebus` - 事件总线实现

### 2. 实现 Repository

参考 [session_repository.go](internal/infrastructure/inmem/session_repository.go) 中的仓储实现。

创建 `repository.go` 文件。

### 3. 实现文件系统扫描器（示例）

参考 [localfs/scanner.go](internal/infrastructure/localfs/scanner.go) 中的扫描器实现。

创建 `scanner.go` 文件。

### 4. 实现事件总线（示例）

参考 [ebus/event_bus.go](internal/infrastructure/ebus/event_bus.go) 中的事件总线实现。

创建 `event_bus.go` 文件。

### 5. 实现外部服务适配器（示例）

创建 `adapter.go` 文件。

### 6. 编写测试用例

参考 [session_repository_test.go](internal/infrastructure/inmem/session_repository_test.go) 中的测试示例。

创建 `repository_test.go` 文件。

### 7. 在应用层中使用

参考 [application/root.go](internal/application/root.go) 中的集成示例。

在 `internal/application/root.go` 中注入基础设施实现。

## 项目特定规则

### Repository 实现规则

- 实现领域层定义的接口
- 使用 `sync.RWMutex` 进行并发控制
- 使用 `var _` 编译时检查确保接口实现正确

### Infrastructure 层规则

- 基础设施层实现领域层定义的接口
- 基础设施层按技术组织（如 inmem、localfs、urlconv）
- 基础设施层包含技术细节和外部依赖
- 使用适当的并发控制（如 `sync.RWMutex`）
- 使用 `var _` 编译时检查确保接口实现正确
- 确保所有公共 API 都有文档注释
- 测试应该覆盖并发场景
