---
name: "modal-drawer"
description: "前端模态框与抽屉（Modal/Dialog/Drawer）使用规范，包括 useModalDialog 和 useModalDrawer 的使用方法与状态控制设计。Invoke when creating or modifying modals, dialogs, or drawers."
---

建议使用项目中统一的模态框/抽屉/全屏遮罩基础设施来构建弹窗、对话框、侧边抽屉或全屏浏览器。

## 统一组件与 Composable

- **对话框 (Dialog)**: 使用 `useModalDialog` 和 `ModalDialog` 组件。
- **侧边抽屉 (Drawer)**: 使用 `useModalDrawer` 和 `ModalDrawer` 组件。
- **全屏模态框 (Fullscreen Modal)**: 使用 `useModalFullscreen` 和 `ModalFullscreen` 组件。

## 核心设计规范

### 1. 避免双向同步的反设计 (v-model 消除)

- 禁止在业务对话框组件上使用 `v-model` 来同步外部/内部的打开关闭状态。
- 应当由**调用者 (Caller) 直接调用 `useModalDialog` / `useModalFullscreen` / `useModalDrawer` 声明控制器**，并直接持有 `open` 和 `close` 方法来控制开关，实现清晰的单向控制流。
- **严禁使用 `defineExpose` 暴露子组件内部的打开/关闭方法**，所有状态应当完全由外部控制器直接控制。

### 2. 纯展示与插槽架构 (Slot)

- 业务对话框或抽屉的内容组件（例如 `MemoEditorDialog.vue`）应当只负责展现其内容结构（如头部、编辑器主体、底部），**内部严禁包裹 Teleport、Transition 或对话框骨架**。
- 调用者通过插槽 (Slot) 的形式将内容组件放入控制器包装组件中。
- 不要在插槽组件上添加关于 `visible` 的冗余 `v-if` 条件（如 `v-if="xxxDialog.visible.value"`），包装器在不可见时会自动处理销毁和退场动画。

```vue
<!-- 调用端模板示例 -->
<memoDialog.component container-class="sm:max-w-3xl">
  <MemoEditorDialog
    :memo="selectedMemo"
    @close="memoDialog.close"
  />
</memoDialog.component>
```

### 3. 数据卫语句与 v-if 的挂载位置

- 渲染所需的异步数据存在性校验的 `v-if` 条件（如 `v-if="session"`、`v-if="currentImage"`）**必须直接绑定在控制器包装组件本身上**，而非绑定在内部内容组件上。
- **目的**：
  1. 防止数据未加载就绪时弹出带有遮罩层的空弹窗骨架，保障视觉体验。
  2. 保证关闭 Transition 退场动画在播放期间（数据在 `afterLeave` 被重置前）数据实体依旧完好，并在动画播放结束后再由包装容器组件的销毁将数据和弹窗一并彻底卸载。

```vue
<!-- 推荐的数据判定绑定示例 -->
<commitDialog.component v-if="session" container-class="sm:max-w-md p-6">
  <CommitForm :session="session" @committed="commitDialog.close" />
</commitDialog.component>
```

### 4. 命名规范与多单词原则

- 所有业务弹窗内容组件必须符合 Vue 的**多单词组件命名规范**（至少两个单词）。
- 文件名和组件名**不要包含 `Dialog` 或 `Modal` 后缀**（因为它们仅包含内容，不是弹窗骨架本身）。
- **对于需要提交数据的表单类弹窗内容组件，应统一使用 `Form` 后缀**（如 `UpdateSessionForm.vue`、`MoveImagesForm.vue`）。
- **对于纯展示类、非表单交互的弹窗内容组件，直接去掉后缀**（如 `HotkeyHelp.vue`、`OpenDirHelp.vue`）。

### 5. 关闭时的生命周期清理 (onWillClose)

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

### 6. 快捷键与层级控制

- `ModalDialog`、`ModalFullscreen` 和 `ModalDrawer` 内部已集成了 `useHotkey` 并在显示时自动拦截 `Escape` 键执行关闭。
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
