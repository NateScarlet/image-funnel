<template>
  <Teleport :to="rendererEl">
    <Transition name="modal">
      <div
        v-if="show"
        class="fixed inset-0 bg-black/75 backdrop-blur-sm flex items-center justify-center z-50 p-4"
        data-no-gesture
        @click.self="$emit('close')"
      >
        <!-- 模态框主体：使用不透明背景增强视觉层级，在桌面端采用宽屏布局 -->
        <div
          class="bg-primary-800 border border-primary-700 rounded-2xl w-full max-w-lg md:max-w-4xl p-6 shadow-2xl transition-all transform scale-100 flex flex-col max-h-[85vh]"
        >
          <!-- 头部区域 -->
          <div
            class="flex items-center justify-between border-b border-primary-700 pb-3 mb-4 shrink-0"
          >
            <h3
              class="text-lg font-bold text-primary-50 flex items-center gap-2"
            >
              <svg class="w-5 h-5 text-secondary-400" viewBox="0 0 24 24">
                <path :d="mdiKeyboardOutline" fill="currentColor" />
              </svg>
              <span>快捷键说明</span>
            </h3>
            <button
              class="text-primary-400 hover:text-primary-200 transition-colors p-1.5 rounded-lg hover:bg-primary-750"
              @click="$emit('close')"
            >
              <svg class="w-5 h-5" viewBox="0 0 24 24">
                <path :d="mdiClose" fill="currentColor" />
              </svg>
            </button>
          </div>

          <!-- 快捷键列表内容区：使用双列网格按分组分列展示，分组内部的快捷键垂直排列，确保逻辑清晰 -->
          <div class="flex-1 overflow-y-auto pr-1 scrollbar-thin">
            <div
              v-if="groupedHotkeys.length > 0"
              class="grid grid-cols-1 md:grid-cols-2 gap-6 py-2"
            >
              <div
                v-for="group in groupedHotkeys"
                :key="group.name"
                class="space-y-2.5"
              >
                <!-- 分组标题：提供清晰的分组识别度 -->
                <h4
                  class="text-xs font-bold text-primary-400 tracking-wider uppercase px-2 select-none"
                >
                  {{ group.name }}
                </h4>
                <!-- 分组内快捷键列表：保持单列，防止如“评分”等关联快捷键被拆开 -->
                <div class="space-y-2">
                  <div
                    v-for="item in group.items"
                    :key="item.id"
                    class="flex items-center justify-between py-2 px-3.5 bg-primary-900/40 hover:bg-primary-900/80 border border-primary-800/30 hover:border-primary-700/40 rounded-xl transition-all duration-200"
                  >
                    <span
                      class="text-xs md:text-sm text-primary-200 font-medium mr-4"
                      >{{ item.description }}</span
                    >
                    <div
                      class="flex items-center gap-1.5 flex-wrap justify-end"
                    >
                      <div
                        v-for="(combo, comboIdx) in item.keys"
                        :key="comboIdx"
                        class="flex items-center gap-1"
                      >
                        <!-- 支持多组快捷键的 or 拼接 -->
                        <span
                          v-if="comboIdx > 0"
                          class="text-primary-500 text-xs px-1 select-none"
                          >/</span
                        >
                        <template
                          v-for="(keyName, keyIdx) in combo"
                          :key="keyIdx"
                        >
                          <span
                            v-if="keyIdx > 0"
                            class="text-primary-500 text-xs font-light select-none"
                            >+</span
                          >
                          <kbd
                            class="px-2 py-0.5 min-w-6 text-center bg-primary-950 text-primary-100 rounded-lg border border-primary-800 font-mono text-xs shadow-md select-none"
                          >
                            {{ keyName }}
                          </kbd>
                        </template>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div v-else class="text-center py-8 text-primary-400 text-sm">
              当前页面无可用快捷键
            </div>
          </div>

          <!-- 底部提示 -->
          <div
            class="mt-4 pt-3 border-t border-primary-700/60 text-center text-xs text-primary-400 shrink-0"
          >
            提示：在输入框内聚焦时快捷键默认失效
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { activeHotkeys } from "../composables/useHotkey";
import { mdiKeyboardOutline, mdiClose } from "@mdi/js";
import useFullscreenRendererElement from "@/composables/useFullscreenRendererElement";
import { computed, toValue } from "vue";

defineProps<{
  show: boolean;
}>();

defineEmits<(e: "close") => void>();

const rendererEl = useFullscreenRendererElement();

// 将可用的快捷键按 category 进行分组
const groupedHotkeys = computed(() => {
  const list = activeHotkeys.value.filter(
    (item) => item.enabled === undefined || toValue(item.enabled),
  );

  const groups: Record<string, typeof list> = {};
  for (const item of list) {
    const cat = item.category || "其他";
    if (!groups[cat]) {
      groups[cat] = [];
    }
    groups[cat].push(item);
  }

  // 固定的分组显示顺序，保证全局和主要交互在最前面，操作类在后
  const categoryOrder = [
    "全局",
    "目录导航",
    "图片浏览",
    "筛选会话",
    "图片评分",
    "图片标签",
    "图片操作",
    "其他",
  ];

  return Object.keys(groups)
    .sort((a, b) => {
      const idxA = categoryOrder.indexOf(a);
      const idxB = categoryOrder.indexOf(b);
      const orderA = idxA !== -1 ? idxA : 999;
      const orderB = idxB !== -1 ? idxB : 999;
      return orderA - orderB;
    })
    .map((name) => ({
      name,
      items: groups[name],
    }));
});
</script>

<style scoped>
/* 浮层淡入淡出动画 */
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

/* 内部卡片弹出和收起时的缩放效果 */
.modal-enter-active .transform,
.modal-leave-active .transform {
  transition: transform 0.25s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.modal-enter-from .transform {
  transform: scale(0.95);
}

.modal-leave-to .transform {
  transform: scale(0.95);
}

/* 滚动条优化 */
.scrollbar-thin::-webkit-scrollbar {
  width: 4px;
}
.scrollbar-thin::-webkit-scrollbar-track {
  background: transparent;
}
.scrollbar-thin::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
}
.scrollbar-thin::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.25);
}
</style>
