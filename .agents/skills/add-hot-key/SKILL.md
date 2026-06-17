---
name: "add-hot-key"
description: "前端添加快捷键的指南"
---

使用 `@/composables/useHotkeys`
添加的快捷键会自动显示在帮助里，并且在组件卸载时自动注销。

## 注册单个快捷键 (对象格式)

对于常规的单个快捷键绑定，推荐使用对象 (Record) 格式：

```typescript
useHotkeys(
  {
    "ctrl+shift+c": async (e) => {
      // 若页面上有文本处于被选中状态，则不拦截，走浏览器原生的复制行为
      const selection = window.getSelection()?.toString();
      if (selection) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      await copyAbsoluteFilePath();
    },
  },
  {
    preventDefault: false,
    stopPropagation: false,
    description: "复制绝对路径",
    category: "图片操作",
  },
);
```

## 注册多个快捷键 (数组格式)

当需要为一个操作绑定多个不同的组合键时（例如问号键在不同键盘布局下可能需要 Shift），**请勿使用逗号拼接 Map Key**。应该传入数组格式的 `HotkeyBinding[]`：

```typescript
useHotkeys(
  [
    {
      keys: ["?", "shift+?"],
      handler: (e) => {
        e.preventDefault();
        e.stopPropagation();
        toggleHelpDialog();
      },
    },
  ],
  {
    description: "显示/隐藏快捷键帮助",
    category: "全局",
    global: true,
  },
);
```
