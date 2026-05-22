<template>
  <router-view />
  <Teleport :to="rendererEl">
    <NotificationList />
    <HotkeyHelpModal :show="showHotkeyHelp" @close="showHotkeyHelp = false" />
    <OpenDirHelpDialog v-model="showOpenDirHelp" />
  </Teleport>
</template>

<script setup lang="ts">
import { ref } from "vue";
import NotificationList from "./components/NotificationList.vue";
import HotkeyHelpModal from "./components/HotkeyHelpModal.vue";
import OpenDirHelpDialog from "./components/OpenDirHelpDialog.vue";
import useFullscreenRendererElement from "./composables/useFullscreenRendererElement";
import useHotkey from "./composables/useHotkey";
import { useOpenDir } from "./composables/useOpenDir";

const rendererEl = useFullscreenRendererElement();
const showHotkeyHelp = ref(false);
const { showOpenDirHelp } = useOpenDir();

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
