---
name: "backend-testing"
description: "后端测试指南，包括创建测试用例、运行测试和测试规范。Invoke when writing or running Go tests in the backend."
---

# 后端测试指南

## 创建测试用例

### 测试文件位置

测试文件必须与被测试文件在同一包中，文件名添加 `_test.go` 后缀。

例如：
- 被测试文件：`internal/domain/session/session.go`
- 测试文件：`internal/domain/session/session_test.go`

### 测试函数命名

使用 `Test` 前缀，后跟被测试的函数名。

例如：
```go
func TestNewSession(t *testing.T) {
    // 测试代码
}
```

## 运行测试

### 测试单个模块

```bash
go test --timeout 30s
```

### 测试所有模块

```bash
go test --timeout 600s ./...
```

### 测试特定包

```bash
go test ./internal/domain/session --timeout 30s
```

### 测试特定函数

```bash
go test -run TestNewSession ./internal/domain/session --timeout 30s
```

## 项目特定规范

### 测试结构

参考 [session_test.go](internal/domain/session/session_test.go) 中的测试示例。

### 错误处理

测试出错时，在测试中添加详细的日志输出，帮助定位问题：

```go
func TestSomething(t *testing.T) {
    result, err := DoSomething()
    if err != nil {
        t.Logf("Error occurred: %v", err)
        t.Logf("Input parameters: param1=%v, param2=%v", param1, param2)
    }
    require.NoError(t, err)
}
```

### 测试覆盖率

查看测试覆盖率：

```bash
go test -cover ./...
```

生成覆盖率报告：

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 注意事项

- 不要用 `go run` 编写测试，直接用 `go test` 运行测试
- 测试文件应该与被测试文件在同一包中
- 使用 `t.Run` 创建子测试，提高测试可读性
- 测试失败时，提供清晰的错误信息
- 保持测试的独立性，测试之间不应相互依赖
- 使用 table-driven tests 减少重复代码
