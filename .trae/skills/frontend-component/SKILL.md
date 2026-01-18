---
name: "frontend-component"
description: "创建新组件，包括组件结构、TypeScript 类型定义、Composition API 和 Tailwind CSS 样式。Invoke when creating new Vue components."
---

# 创建新组件

## 创建组件文件

在 `src/components/` 目录下创建组件文件：

## 图标使用

使用 `@mdi/js` 来获取 Material Design Icons 图标。

## 注意事项

- 使用 TypeScript 定义 props 和 emits
- 使用 Composition API 编写逻辑
- 使用 Tailwind CSS 进行样式设计，避免自定义样式，可以使用任意值语法（如 `class="text-[#ff6b6b]"`）
- 主要按钮和交互使用 `secondary` 颜色
- 保持组件的单一职责原则
- 使用 `computed` 派生数据，避免重复计算
- 避免使用 `watch`，尽量使用 `computed`
- 使用 `useEventListeners` 来管理事件监听，避免手动处理
