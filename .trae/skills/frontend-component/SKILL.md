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
- 必填参数不得超过 3 个，超过 3 个参数时考虑使用对象参数
- id 类参数不要作为字符串传递，而是定义 `{someObject:{id: string}}` 类型，方便外部传入和后续扩展
- 使用 `useTemplateRef` 来获取 DOM 元素引用
- 使用 defineModel 来定义双向绑定的模型
- define使用的类型，直接定义在 defineXXX<{...}> 中，不要声明 Props 或 Emits 接口
- 禁止通过 watch+emit 来暴露组件内部状态，用 defineExpose 代替
