---
name: "modal-drawer"
description: "前端模态框与抽屉（Modal/Dialog/Drawer）使用规范，包括 useModalDialog 和 useModalDrawer 的使用方法与状态控制设计。Invoke when creating or modifying modals, dialogs, or drawers."
---

建议使用项目中统一的模态框/抽屉基础设施来构建对话框或侧边抽屉。

## 统一组件与 Composable

- **对话框 (Dialog)**: 使用 `useModalDialog` 和 `ModalDialog` 组件。
- **侧边抽屉 (Drawer)**: 使用 `useModalDrawer` 和 `ModalDrawer` 组件。

## 核心设计规范

### 1. 避免双向同步的反设计 (v-model 消除)

- 禁止在业务对话框组件上使用 `v-model` 来同步外部/内部的打开关闭状态。
- 应当由**调用者 (Caller) 直接调用 `useModalDialog` / `useModalDrawer` 声明控制器**，并直接持有 `open` 和 `close` 方法来控制开关，实现清晰的单向控制流。

### 2. 纯展示与插槽架构 (Slot)

- 业务对话框或抽屉的内容组件（例如 `MemoEditorDialog.vue`）应当只负责展现其内容结构（如头部、编辑器主体、底部），**内部严禁包裹 Teleport、Transition 或对话框骨架**。
- 调用者通过插槽 (Slot) 的形式将内容组件放入控制器中：

```vue
<!-- 调用端模板示例 -->
<memoDialog.component container-class="sm:max-w-3xl">
  <MemoEditorDialog
    v-if="selectedMemo"
    ref="memoDialogRef"
    :memo="selectedMemo"
    @close="memoDialog.close"
  />
</memoDialog.component>
```

### 3. 关闭时的生命周期清理 (onWillClose)

- 任何需要在弹窗关闭时执行的状态清理、数据刷入（如 `flush()`）操作，**必须注册在 `onWillClose` 钩子中**。
- 严禁在 `onDidClose` 钩子中执行此类依赖组件 ref 的逻辑。因为 `onDidClose` 触发时退场动画已播放完毕，组件已被完全卸载，此时获取到的 ref 值为 `null`。

```typescript
// 调用端 setup 示例
const memoDialog = useModalDialog({
  onDidOpen() {
    // 聚焦编辑器
    nextTick(() => {
      memoDialogRef.value?.focus();
    });
  },
  onWillClose() {
    // 恢复滚动条
    document.body.style.overflow = "";
    // 在组件卸载前强制刷入/保存数据
    memoDialogRef.value?.flush();
  },
});
```

### 4. 快捷键与层级控制

- `ModalDialog` 和 `ModalDrawer` 内部已集成了 `useHotkey` 并在显示时自动拦截 `Escape` 键执行关闭。
- 如果调用端需要根据弹窗状态切换其他快捷键（如图片切换），应将快捷键的 `enabled` 绑定为控制器的 `visible` 属性：

```typescript
useHotkey(
  "escape",
  () => {
    memoDialog.close();
  },
  {
    enabled: memoDialog.visible, // 仅在弹窗打开时启用
  }
);
```
