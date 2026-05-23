<template>
  <router-view />
  <Teleport :to="rendererEl">
    <NotificationList />

    <hotkeyHelpDialog.component container-class="sm:max-w-lg md:max-w-4xl p-6">
      <HotkeyHelp @close="hotkeyHelpDialog.close" />
    </hotkeyHelpDialog.component>

    <openDirHelpDialog.component container-class="sm:max-w-md p-6">
      <OpenDirHelp @close="openDirHelpDialog.close" />
    </openDirHelpDialog.component>
  </Teleport>
</template>

<script setup lang="ts">
import { watch } from "vue";
import NotificationList from "./components/NotificationList.vue";
import HotkeyHelp from "./components/HotkeyHelp.vue";
import OpenDirHelp from "./components/OpenDirHelp.vue";
import useFullscreenRendererElement from "./composables/useFullscreenRendererElement";
import useHotkey from "./composables/useHotkey";
import { useOpenDir } from "./composables/useOpenDir";
import useModalDialog from "./composables/useModalDialog";

const rendererEl = useFullscreenRendererElement();
const { showOpenDirHelp } = useOpenDir();

const hotkeyHelpDialog = useModalDialog();
const openDirHelpDialog = useModalDialog({
  onDidClose() {
    showOpenDirHelp.value = false;
  },
});

watch(showOpenDirHelp, (val) => {
  if (val) {
    openDirHelpDialog.open();
  } else {
    openDirHelpDialog.close();
  }
});

useHotkey(
  ["?", "shift+?"],
  () => {
    if (hotkeyHelpDialog.visible.value) {
      hotkeyHelpDialog.close();
    } else {
      hotkeyHelpDialog.open();
    }
  },
  {
    description: "显示/隐藏快捷键帮助",
    category: "全局",
  },
);
</script>
