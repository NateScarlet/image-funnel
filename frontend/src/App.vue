<template>
  <router-view />
  <Teleport :to="rendererEl">
    <NotificationList />
    <HotkeyHelpModal :show="showHotkeyHelp" @close="showHotkeyHelp = false" />
  </Teleport>
</template>

<script setup lang="ts">
import { ref } from "vue";
import NotificationList from "./components/NotificationList.vue";
import HotkeyHelpModal from "./components/HotkeyHelpModal.vue";
import useFullscreenRendererElement from "./composables/useFullscreenRendererElement";
import useHotkey from "./composables/useHotkey";

const rendererEl = useFullscreenRendererElement();
const showHotkeyHelp = ref(false);

useHotkey(
  ["?", "shift+?"],
  () => {
    showHotkeyHelp.value = !showHotkeyHelp.value;
  },
  {
    description: "显示/隐藏快捷键帮助",
  },
);
</script>
