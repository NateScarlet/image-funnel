---
name: "frontend-development"
description: "前端开发指南，包含 Vue 3、TypeScript、GraphQL 客户端和 Tailwind CSS 的开发规范和最佳实践"
---

# 前端开发指南

## 技术栈

- **框架**: Vue 3 (Composition API)
- **语言**: TypeScript
- **样式**: Tailwind CSS
- **GraphQL 客户端**: Apollo Client
- **图标**: @mdi/js (Material Design Icons)

## 开发规范

### 类型定义

尽量基于 `./graphql/generated` 中的类型定义，不要自己定义重复的类型。

### 响应式数据

避免使用 `watch`，尽量使用 `computed` 进行数据转换和计算。

### GraphQL 数据管理

不要手动更新 GraphQL 查询结果，而是依赖 `./src/graphql/useQuery.ts` 的响应式系统自动更新。InMemoryCache 会自动更新查询结果。

### 图标使用

使用 `@mdi/js` 来获取 Material Design Icons 图标。

### 样式规范

主要按钮和交互使用 `secondary` 颜色。

## 项目结构

```
frontend/
└── src/
    ├── components/  # Vue 组件
    ├── graphql/    # GraphQL 客户端
    │   ├── fragments/    # GraphQL 片段
    │   ├── mutations/    # GraphQL 变更操作
    │   ├── queries/      # GraphQL 查询
    │   ├── subscriptions/# GraphQL 订阅
    │   ├── client.ts     # GraphQL 客户端配置
    │   └── generated.ts  # 自动生成的类型
    └── views/       # 页面视图
```

## 开发工作流

### 修改代码后

Vite 会自动热重载。

### 修改 GraphQL schema 后

运行 `.\scripts\generate-graphql.ps1` 命令来同时更新前后端的 GraphQL 相关代码。

### 检查错误

运行 `pnpm check` 来检查错误。

如果错误提示说有可以自动修复的错误，直接使用 `pnpm lint:fix` 来修复。

### 测试

访问 http://localhost:3000（前端）。

## 常见任务

### 添加新的 GraphQL 查询

1. 在 `src/graphql/queries/` 目录下创建查询文件
2. 使用 `generated.ts` 中的类型定义
3. 通过 `useQuery.ts` 的响应式系统获取数据

### 添加新的 GraphQL 变更

1. 在 `src/graphql/mutations/` 目录下创建变更文件
2. 使用 `generated.ts` 中的类型定义
3. 通过 `useMutation` 执行变更操作

### 创建新组件

1. 在 `src/components/` 目录下创建组件
2. 使用 TypeScript 定义 props 和 emits
3. 使用 Composition API 编写逻辑
4. 使用 Tailwind CSS 进行样式设计

## 注意事项

- 对于不常见的情况或特殊修复，添加注释说明
- 保持组件的单一职责原则
- 遵循 Vue 3 Composition API 的最佳实践
- 使用 TypeScript 的类型系统确保类型安全
