---
name: "local-storage"
description: "前端 localStorage 使用规范，包括 useStorage composable 和 key 命名规则。Invoke when using localStorage or implementing storage features in frontend."
---

建议使用 `useStorage` composable 处理 localStorage 操作，除非你确定不需要响应式更新。

在组件 `<script lang="ts">` 初始化部分，而不是 `<script setup lang="ts">` 中定义 `useStorage` composable，实现共享状态。
组件可以同时存在这两个 script 标签。

key 命名规则：

- 使用简短名称
- 加上 `_{随机字符串}` 后缀避免意外冲突并且方便代码搜索
- 示例：`my_settings_abc123`

```typescript
const { model, flush, reload, clear } = useStorage(
  localStorage,
  "my_settings_abc123",
  () => defaultValue
);
```

useStorage已经处理了响应式优化，对于复杂对象尽量原地修改然后调用flush而不是每次都重建
