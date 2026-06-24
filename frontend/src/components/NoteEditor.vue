<template>
  <div class="flex flex-col w-full text-left">
    <!-- 快捷指令按钮栏 + 派发按钮 -->
    <div
      v-if="directives.length || dispatchableHooks.length"
      class="flex flex-wrap gap-2 mb-3 items-center select-none text-left"
    >
      <template v-if="directives.length">
        <span
          class="text-xs font-bold text-primary-400 uppercase tracking-wider flex items-center gap-1"
        >
          <svg class="w-4 h-4 text-secondary-400" viewBox="0 0 24 24">
            <path :d="mdiConsole" fill="currentColor" />
          </svg>
          常用指令:
        </span>
        <button
          v-for="h in directives"
          :key="h.id"
          type="button"
          class="px-2 py-1 text-xs font-semibold rounded-lg bg-primary-800 hover:bg-primary-700 border border-primary-700/60 hover:border-secondary-500/50 text-primary-300 hover:text-white transition-all active:scale-95 cursor-pointer flex items-center gap-1 group"
          :title="h.directive!.usage"
          @click="insertDirective(h.directive!.name)"
        >
          <span class="text-secondary-400 group-hover:text-secondary-300"
            >/{{ h.directive!.name }}</span
          >
          <span
            class="text-xs scale-90 origin-left font-normal text-primary-500 group-hover:text-primary-400"
            >({{ h.name }})</span
          >
        </button>
      </template>

      <!-- 派发按钮：可即时触发的笔记钩子，放入二级菜单防止误触 -->
      <div
        v-if="dispatchableHooks.length"
        ref="dispatchMenuRef"
        class="relative"
      >
        <button
          type="button"
          class="px-2 py-1 text-xs font-semibold rounded-lg bg-secondary-900/60 hover:bg-secondary-800 border border-secondary-700/60 hover:border-secondary-500/50 text-secondary-300 hover:text-white transition-all active:scale-95 cursor-pointer flex items-center gap-1"
          :disabled="isDispatching"
          @click.stop="showDispatchMenu = !showDispatchMenu"
        >
          <svg class="w-3.5 h-3.5 text-secondary-400" viewBox="0 0 24 24">
            <path :d="mdiLightningBolt" fill="currentColor" />
          </svg>
          <span>动作 ({{ dispatchableHooks.length }})</span>
          <svg
            class="w-3 h-3 text-secondary-400 transition-transform"
            :class="{ 'rotate-180': showDispatchMenu }"
            viewBox="0 0 24 24"
          >
            <path :d="mdiChevronDown" fill="currentColor" />
          </svg>
        </button>

        <!-- 下拉菜单 -->
        <div
          v-if="showDispatchMenu"
          class="absolute top-full left-0 mt-2 z-50 bg-primary-900 border border-primary-700 rounded-xl shadow-2xl p-2 w-fit min-w-40 flex flex-col gap-1"
          @click.stop
        >
          <div
            class="px-3 py-2 text-xs font-bold text-primary-400 border-b border-primary-800 uppercase tracking-wider select-none"
          >
            选择动作
          </div>
          <button
            v-for="h in dispatchableHooks"
            :key="'dispatch-' + h.id"
            type="button"
            class="text-left px-3 py-2 rounded-lg flex items-center gap-2 transition-colors text-sm font-medium text-secondary-300 hover:bg-secondary-800 hover:text-white cursor-pointer whitespace-nowrap"
            :disabled="isDispatching"
            @click="
              triggerDispatch(h.id, h.name);
              showDispatchMenu = false;
            "
          >
            <svg
              class="w-4 h-4 text-secondary-400 shrink-0"
              viewBox="0 0 24 24"
            >
              <path :d="mdiLightningBolt" fill="currentColor" />
            </svg>
            <span>{{ h.name }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 文本编辑区 -->
    <div class="relative w-full group">
      <!-- 自动完成浮层 -->
      <Teleport to="body">
        <div
          v-if="autocompleteState?.show && filteredDirectives.length"
          ref="floatingEl"
          :style="floatingStyles"
          class="fixed z-50 bg-primary-900 border border-primary-700 rounded-xl shadow-2xl p-2 max-h-56 overflow-y-auto w-72 backdrop-blur-md flex flex-col gap-1"
        >
          <div
            class="px-3 py-2 text-xs font-bold text-primary-400 border-b border-primary-800 uppercase tracking-wider select-none"
          >
            选择笔记指令
          </div>
          <button
            v-for="(h, idx) in filteredDirectives"
            :key="h.id"
            type="button"
            class="w-full text-left px-3 py-2 rounded-lg flex flex-col transition-colors cursor-pointer"
            :class="
              idx === activeIndex
                ? 'bg-secondary-500 text-white'
                : 'hover:bg-primary-800 text-primary-200'
            "
            @click="insertDirective(h.directive!.name)"
            @mouseenter="activeIndex = idx"
          >
            <div class="flex items-center gap-2 font-bold text-xs sm:text-sm">
              <span
                :class="
                  idx === activeIndex ? 'text-white' : 'text-secondary-400'
                "
                >/{{ h.directive!.name }}</span
              >
              <span class="text-xs opacity-75 font-normal">({{ h.name }})</span>
            </div>
            <div
              class="text-xs opacity-80"
              :class="[
                idx === activeIndex ? 'text-white' : 'text-primary-400',
                filteredDirectives.length === 1
                  ? 'wrap-break-word whitespace-pre-wrap'
                  : 'truncate',
              ]"
            >
              {{ h.directive!.usage }}
            </div>
          </button>
        </div>
      </Teleport>

      <textarea
        ref="textarea"
        v-model="model"
        class="w-full bg-primary-800/50 hover:bg-primary-800 focus:bg-primary-800 border border-primary-700 focus:border-secondary-500/50 rounded-xl px-4 py-3 sm:px-8 sm:py-6 text-sm sm:text-xl text-primary-100 placeholder-primary-500 outline-none transition-all duration-300 resize-none leading-relaxed min-h-30 sm:min-h-60 overflow-y-auto"
        :placeholder="placeholder"
        data-no-gesture
        @input="handleInput"
        @keyup="checkAutocomplete"
        @click="checkAutocomplete"
        @keydown.up.prevent="handleKeyUp"
        @keydown.down.prevent="handleKeyDown"
        @keydown.enter="handleKeyEnter"
        @keydown.esc.prevent="handleKeyEsc"
        @blur="handleBlur"
      ></textarea>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, useTemplateRef } from "vue";
import { useFloating, offset, flip, shift, autoUpdate } from "@floating-ui/vue";
import useQuery from "@/graphql/utils/useQuery";
import mutate from "@/graphql/utils/mutate";
import { HooksDocument, DispatchNoteHookDocument } from "@/graphql/generated";
import useTextAreaAutoHeight from "@/composables/useTextAreaAutoHeight";
import useNotification from "@/composables/useNotification";
import useClickOutside from "@/composables/useClickOutside";
import { mdiConsole, mdiLightningBolt, mdiChevronDown } from "@mdi/js";

const props = defineProps<{
  placeholder?: string;
  /** 当前笔记 ID，为空时不显示派发按钮 */
  noteId?: string;
  onBeforeDispatch?: () => Promise<void>;
}>();

const model = defineModel<string>({ required: true });

const emit = defineEmits<(e: "input") => void>();

const textareaRef = useTemplateRef<HTMLTextAreaElement>("textarea");
const floatingEl = ref<HTMLElement | null>(null);
const activeIndex = ref(0);

const { floatingStyles } = useFloating(textareaRef, floatingEl, {
  placement: "top-start",
  strategy: "fixed",
  whileElementsMounted: autoUpdate,
  middleware: [
    offset({
      mainAxis: 8,
      crossAxis: 16,
    }),
    flip(),
    shift(),
  ],
});

// 使用声明式缓存加载，避免多余的组件内部状态
const { data: hooksData } = useQuery(HooksDocument, {
  fetchPolicy: "cache-first",
});

const directives = computed(() => {
  return hooksData.value?.hooks.filter((h) => h.directive != null) || [];
});

// 可即时派发的钩子：canDispatchByNote 且当前笔记已保存（有 noteId）
const dispatchableHooks = computed(() => {
  if (!props.noteId) return [];
  return (
    hooksData.value?.hooks.filter((h) => {
      if (!h.canDispatchByNote) return false;
      if (!h.directive) return false;
      // 检查当前笔记内容中是否包含该指令（要求处于行首，前面只有空格或制表符，且指令后面有单词边界）
      const escapedName = h.directive.name.replace(
        /[.*+?^${}()|[\]\\]/g,
        "\\$&",
      );
      const regex = new RegExp(`(?:^|\\r?\\n)[ \\t]*/${escapedName}\\b`);
      return regex.test(model.value);
    }) || []
  );
});

const { showSuccess, showError, showInfo, remove } = useNotification();
const isDispatching = ref(false);

// 下拉菜单状态
const showDispatchMenu = ref(false);
const dispatchMenuRef = useTemplateRef<HTMLDivElement>("dispatchMenuRef");

useClickOutside(dispatchMenuRef, () => {
  showDispatchMenu.value = false;
});

async function triggerDispatch(hookId: string, hookName: string) {
  if (!props.noteId || isDispatching.value) return;
  isDispatching.value = true;

  if (props.onBeforeDispatch) {
    try {
      await props.onBeforeDispatch();
    } catch (err) {
      showError(
        `保存笔记修改失败: ${err instanceof Error ? err.message : String(err)}`,
      );
      isDispatching.value = false;
      return;
    }
  }

  const infoNotificationId = showInfo(`正在执行动作 ${hookName}...`, 0);

  try {
    const { error } = await mutate(DispatchNoteHookDocument, {
      variables: {
        input: {
          hookId,
          noteId: props.noteId,
        },
      },
    });

    if (error) {
      showError(`执行动作 ${hookName} 失败：${error.message}`);
    } else {
      showSuccess(`动作 ${hookName} 已成功触发`);
    }
  } catch (err) {
    showError(
      `执行动作 ${hookName} 出错：${
        err instanceof Error ? err.message : String(err)
      }`,
    );
  } finally {
    remove(infoNotificationId);
    isDispatching.value = false;
  }
}

const autocompleteState = ref<{
  show: boolean;
  query: string;
  triggerIndex: number;
  selectionStart: number;
} | null>(null);

const filteredDirectives = computed(() => {
  if (!autocompleteState.value?.show) return [];
  const q = autocompleteState.value.query;
  const list = directives.value;
  if (!q) return list;
  return list.filter(
    (h) => h.directive?.name.toLowerCase().includes(q) ?? false,
  );
});

// 自动高度绑定
useTextAreaAutoHeight(textareaRef, model);

function checkAutocomplete() {
  const el = textareaRef.value;
  if (!el) {
    autocompleteState.value = null;
    return;
  }

  const start = el.selectionStart;
  const end = el.selectionEnd;

  if (start !== end) {
    autocompleteState.value = null;
    return;
  }

  const text = model.value;
  const textBeforeCursor = text.slice(0, start);

  // 仅在行首（前面可以有空格或制表符）输入 '/' 时触发斜杠命令，避免干扰普通文本
  const match = textBeforeCursor.match(/(?:\r?\n|^)[ \t]*\/(\w*)$/);
  if (match) {
    const query = match[1];
    const matchedStr = match[0];
    const slashOffset = matchedStr.indexOf("/");
    const triggerIndex = (match.index ?? 0) + slashOffset;

    autocompleteState.value = {
      show: true,
      query: query.toLowerCase(),
      triggerIndex,
      selectionStart: start,
    };
    activeIndex.value = 0;
  } else {
    autocompleteState.value = null;
  }
}

function handleInput() {
  emit("input");
  checkAutocomplete();
}

function insertDirective(dirName: string) {
  const el = textareaRef.value;
  if (!el) return;

  const text = model.value;
  const start = el.selectionStart;

  let before: string;
  let after: string;
  let newCursorPos = 0;

  if (autocompleteState.value) {
    const triggerIdx = autocompleteState.value.triggerIndex;
    before = text.slice(0, triggerIdx);
    after = text.slice(start);
    model.value = before + `/${dirName} ` + after;
    newCursorPos = triggerIdx + dirName.length + 2;
  } else {
    // 快捷按钮点击插入，确保指令在新行
    before = text.slice(0, start);
    after = text.slice(start);

    // 检查是否需要先换行：如果光标前不是行首（前面有非空白字符）
    const needsNewline = before.length > 0 && !/(?:^|\n)[ \t]*$/.test(before);
    const prefix = needsNewline ? "\n" : "";

    model.value = before + prefix + `/${dirName} ` + after;
    newCursorPos = start + prefix.length + dirName.length + 2;
  }

  autocompleteState.value = null;

  nextTick(() => {
    el.focus();
    el.setSelectionRange(newCursorPos, newCursorPos);
    emit("input");
  });
}

function handleKeyUp() {
  if (autocompleteState.value?.show && filteredDirectives.value.length) {
    activeIndex.value =
      (activeIndex.value - 1 + filteredDirectives.value.length) %
      filteredDirectives.value.length;
  }
}

function handleKeyDown() {
  if (autocompleteState.value?.show && filteredDirectives.value.length) {
    activeIndex.value =
      (activeIndex.value + 1) % filteredDirectives.value.length;
  }
}

function handleKeyEnter(e: KeyboardEvent) {
  if (autocompleteState.value?.show && filteredDirectives.value.length) {
    e.preventDefault();
    insertDirective(
      filteredDirectives.value[activeIndex.value].directive?.name ?? "",
    );
  }
}

function handleKeyEsc() {
  if (autocompleteState.value?.show) {
    autocompleteState.value = null;
  }
}

function handleBlur() {
  // 延迟关闭，确保点击浮层内指令能够成功触发 click 事件
  setTimeout(() => {
    autocompleteState.value = null;
  }, 200);
}

function focus() {
  textareaRef.value?.focus();
}

defineExpose({ focus });
</script>
