---
name: "frontend-styling"
description: "前端样式设计指南，包括 Tailwind CSS 使用、响应式设计和样式规范。Invoke when designing or styling Vue components."
---

# 前端样式设计

## 样式规范

### 颜色规范

**规则：**
- 主要按钮和交互使用 `secondary` 颜色
- 文本使用 `text-gray-900`（深色）和 `text-gray-600`（浅色）
- 背景使用 `bg-white` 和 `bg-gray-50`
- 边框使用 `border-gray-300`

参考 [SessionView.vue](src/views/SessionView.vue) 中的样式使用。

## 项目特定规则

- 主要按钮和交互使用 `secondary` 颜色
- 使用响应式设计确保在不同设备上正常显示
- 使用过渡动画提升用户体验
- 使用 `hover:`、`active:`、`focus:` 前缀定义交互状态
- 使用 `transition-` 类添加过渡效果
- 保持样式一致性
- 避免内联样式，使用 Tailwind 类名
- 使用 `scoped` 样式避免样式污染
