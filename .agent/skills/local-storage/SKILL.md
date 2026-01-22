---
description: "前端 localStorage 使用规范，包括 useStorage composable 和 key 命名规则。Invoke when using localStorage or implementing storage features in frontend."
---

建议使用 `useStorage` composable 处理 localStorage 操作，除非你确定不需要响应式更新。

在组件 `<script lang="ts">` 初始化部分，而不是 `<script setup lang="ts">` 中定义 `useStorage` composable，实现共享状态。
组件可以同时存在这两个 script 标签。

key 命名规则：
- 使用简短名称
- 加上 `@{随机字符串}` 后缀避免意外冲突
- 示例：`settings@abc123`

```typescript
const { model, flush, reload, clear } = useStorage(localStorage, 'settings@abc123', () => defaultValue);
```
