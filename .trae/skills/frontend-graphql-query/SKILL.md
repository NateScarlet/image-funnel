---
name: "frontend-graphql-query"
description: "添加新的 GraphQL 查询，包括创建查询文件、使用类型定义和响应式数据获取。Invoke when adding new GraphQL queries to the frontend."
---

# 添加新的 GraphQL 查询

## 创建查询文件

在 `src/graphql/queries/` 目录下创建gql查询文件　
然后执行 `pnpm graphql-codegen` 生成类型定义
之后可以直接从 `@/graphql/generated` 导入生成的类型

## 数据操作

在组件中使用 `@/graphql/utils/useQuery` 获取数据，数据是响应式的不需要手动处理。
在组件中使用 `@/graphql/utils/query` 进行单次异步查询，可搭配 `@/composables/useAsyncTask` 实现复杂查询。

## 项目特定规则

- 使用 `gql` 标签定义查询
- 查询名称使用大写字母和下划线
- 使用 `generated.ts` 中的类型定义
- 定义 TypeScript 类型用于类型安全
- 使用 `useQuery` 的响应式系统自动更新数据
- 避免使用 `watch`，尽量使用 `computed` 进行数据转换
- 不要手动更新 GraphQL 查询结果，InMemoryCache 会自动更新
- 使用 `computed` 派生数据
