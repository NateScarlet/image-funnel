---
name: "backend-build"
description: "后端编译和构建指南，包括重新编译、运行测试和启动调试器。Invoke when modifying backend code or building project."
---

# 后端编译和构建

## 修改代码后

调试器会自动重新编译（如使用 `dlv`）。

## 重新编译

运行以下命令来重新编译前端和后端：

```bash
.\scripts\build.ps1
```

这个脚本会：

1. 编译前端代码
2. 编译后端代码
3. 生成必要的文件

## 运行测试

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

## 启动调试器

### 使用 VS Code 调试器

项目已配置好 VS Code 调试启动器，位于 `image-funnel.code-workspace`。

直接按 **F5** 键启动调试器。

## 开发工作流

### 1. 修改代码

修改后端代码后，调试器会自动重新编译。

### 2. 添加测试用例

修改后端代码后，添加必要的测试用例。

### 3. 运行测试

运行测试确保代码正确：

```bash
go test --timeout 30s
```

### 4. 重新编译

如果需要重新编译前端和后端：

```bash
.\scripts\build.ps1
```

## 注意事项

- 修改代码后，调试器会自动重新编译
- 添加测试用例后，运行测试确保代码正确
- 使用 `go test` 运行测试，不要用 `go run` 编写测试
- 测试出错时，在测试中添加详细的日志输出，帮助定位问题
- 确保所有依赖都正确安装
- 使用 `go mod tidy` 清理不必要的依赖
