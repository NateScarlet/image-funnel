---
name: "frontend-graphql-mutation"
description: "添加新的 GraphQL 变更，包括创建变更文件、使用类型定义和执行变更操作。Invoke when adding new GraphQL mutations to the frontend."
---

# 添加新的 GraphQL 变更

## 创建变更文件

在 `src/graphql/mutations/` 目录下创建gql变更文件　
然后执行 `pnpm graphql-codegen` 生成类型定义
之后可以直接从 `@/graphql/generated` 导入生成的类型

## 使用变更

在组件中使用 `@/graphql/utils/mutate` 进行 mutations。
不需要手动处理错误，有全局错误处理机制。

## 注意事项

- 使用 `gql` 标签定义变更
- 变更名称使用大写字母和下划线
- 使用 `generated.ts` 中的类型定义
- 定义 TypeScript 类型用于类型安全
- 使用 `generated.ts` 中的类型定义，不要自己定义重复的类型
- 合理使用缓存更新策略
- 考虑使用乐观更新提高用户体验
- 处理错误情况，提供用户反馈
- 使用 `refetchQueries` 保持数据一致性
- 避免在变更中执行过多操作
