<template>
  <Teleport :to="rendererEl">
    <Transition name="modal">
      <div
        v-if="show"
        class="fixed inset-0 bg-black/75 backdrop-blur-sm flex items-center justify-center z-50 p-4"
        data-no-gesture
        @click.self="$emit('close')"
      >
        <!-- 使用 Glassmorphism 并带有微妙的边框和过渡动画 -->
        <div
          class="bg-primary-850/90 border border-primary-700/60 rounded-2xl max-w-lg w-full p-6 shadow-2xl transition-all transform scale-100 flex flex-col max-h-[85vh]"
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

          <!-- 快捷键列表内容区 -->
          <div class="flex-1 overflow-y-auto pr-1 space-y-2.5 scrollbar-thin">
            <div
              v-for="item in activeHotkeys"
              :key="item.id"
              class="flex items-center justify-between py-2.5 border-b border-primary-800/40 last:border-b-0 hover:bg-primary-800/30 px-3 rounded-xl transition-colors"
            >
              <span class="text-sm text-primary-300 font-medium mr-4">{{
                item.description
              }}</span>
              <div class="flex items-center gap-1.5 flex-wrap justify-end">
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
                  <template v-for="(keyName, keyIdx) in combo" :key="keyIdx">
                    <span
                      v-if="keyIdx > 0"
                      class="text-primary-500 text-xs font-light select-none"
                      >+</span
                    >
                    <kbd
                      class="px-2 py-0.5 min-w-[24px] text-center bg-primary-950 text-primary-100 rounded-lg border border-primary-800 font-mono text-xs shadow-md select-none"
                    >
                      {{ keyName }}
                    </kbd>
                  </template>
                </div>
              </div>
            </div>
            <div
              v-if="activeHotkeys.length === 0"
              class="text-center py-8 text-primary-400 text-sm"
            >
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

defineProps<{
  show: boolean;
}>();

defineEmits<(e: "close") => void>();

const rendererEl = useFullscreenRendererElement();
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
