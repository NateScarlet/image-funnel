<template>
  <Transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="opacity-0 scale-95"
    enter-to-class="opacity-100 scale-100"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="opacity-100 scale-100"
    leave-to-class="opacity-0 scale-95"
  >
    <div
      v-if="modelValue"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-md"
      @click="close"
    >
      <div
        class="bg-primary-900/90 border border-primary-700/50 rounded-2xl p-6 max-w-md w-full mx-4 shadow-2xl space-y-4"
        @click.stop
      >
        <!-- 弹窗标题栏 -->
        <div class="flex justify-between items-start">
          <h3
            class="text-base font-bold text-white flex items-center gap-2 select-none"
          >
            <svg
              class="w-5 h-5 text-secondary-400 animate-pulse"
              viewBox="0 0 24 24"
            >
              <path :d="mdiFolderOpen" fill="currentColor" />
            </svg>
            需要安装本地路径插件
          </h3>
          <button
            class="text-primary-400 hover:text-white transition-colors p-1 rounded-lg hover:bg-white/5"
            title="关闭 (Esc)"
            @click="close"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24">
              <path :d="mdiClose" fill="currentColor" />
            </svg>
          </button>
        </div>

        <!-- 弹窗正文与步骤 -->
        <div class="text-sm text-primary-200 space-y-3 leading-relaxed">
          <p>
            要直接在 Windows
            资源管理器中打开当前目录，或定位文件并聚焦，您需要安装一个轻量级的注册表及协议插件。
          </p>
          <div
            class="bg-black/30 border border-white/5 rounded-xl p-3.5 space-y-2 text-xs text-primary-300"
          >
            <p class="font-bold text-white mb-1.5 select-none">安装指南：</p>
            <p class="flex gap-1.5">
              <span class="text-secondary-400 font-bold">1.</span>
              点击下方按钮下载插件压缩包；
            </p>
            <p class="flex gap-1.5">
              <span class="text-secondary-400 font-bold">2.</span>
              <span
                ><span class="text-secondary-400 font-semibold"
                  >必须解压到单独文件夹</span
                >
                后再运行，不支持在压缩包内直接安装；</span
              >
            </p>
            <p class="flex gap-1.5">
              <span class="text-secondary-400 font-bold">3.</span>
              <span
                >双击运行
                <code
                  class="bg-primary-800 px-1 py-0.5 rounded font-mono text-white select-all"
                  >安装.cmd</code
                >
                并允许注册表修改。</span
              >
            </p>
          </div>
        </div>

        <!-- 弹窗操作按钮 -->
        <div class="flex items-center gap-3 pt-2">
          <a
            href="/static/open-dir/windows/setup.zip"
            download="本地路径插件.zip"
            class="flex-1 py-2.5 bg-secondary-600 hover:bg-secondary-500 text-white text-center rounded-xl text-sm font-semibold transition-all active:scale-98 shadow-lg shadow-secondary-900/20"
            @click="close"
          >
            下载插件 (.zip)
          </a>
          <button
            class="px-4 py-2.5 bg-primary-800 hover:bg-primary-700 border border-primary-700 text-primary-200 hover:text-white rounded-xl text-sm font-semibold transition-all"
            @click="close"
          >
            取消
          </button>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { mdiClose, mdiFolderOpen } from "@mdi/js";

const { modelValue } = defineProps<{
  modelValue: boolean;
}>();

const emit = defineEmits<(e: "update:modelValue", value: boolean) => void>();

function close() {
  emit("update:modelValue", false);
}
</script>
