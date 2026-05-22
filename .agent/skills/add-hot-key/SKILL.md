---
name: "add-hot-key"
description: "前端添加快捷键的指南"
---

使用 `@/composables/useHotKey`
添加的快捷键会自动显示在帮助里，并且在组件卸载时自动注销

示例

```typescript
useHotkey(
  "ctrl+shift+c",
  async (e) => {
    // 若页面上有文本处于被选中状态，则不拦截，走浏览器原生的复制行为
    const selection = window.getSelection()?.toString();
    if (selection) {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
    await copyAbsoluteFilePath();
  },
  {
    preventDefault: false,
    stopPropagation: false,
    description: "复制绝对路径",
  },
);
```
