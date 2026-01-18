---
name: "frontend-check"
description: "前端错误检查，包括运行 lint、typecheck 和自动修复。Invoke when modifying frontend code or checking for errors."
---

# 前端错误检查

## 检查错误

运行以下命令检查错误：

```bash
pnpm check
```

## 开发工作流

### 修改代码后

1. 修改前端代码
2. 运行 `pnpm check` 检查错误
3. 如果有可以自动修复的错误，运行 `pnpm lint:fix`
4. 手动修复无法自动修复的错误
5. 重新运行 `pnpm check` 确认修复

### 提交代码前

1. 运行 `pnpm check` 确保没有错误
2. 运行 `pnpm lint:fix` 自动修复
3. 手动修复剩余错误
4. 确认所有错误已修复

## 注意事项

- 修改代码后运行 `pnpm check` 检查错误
- 使用 `pnpm lint:fix` 自动修复可以修复的错误
- 手动修复无法自动修复的错误
- 确保所有错误都已修复后再提交代码
- 保持代码风格一致
- 使用 TypeScript 类型系统确保类型安全
