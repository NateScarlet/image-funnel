<template>
  <div id="app" class="min-h-screen bg-slate-900 text-slate-100">
    <router-view />
    <Teleport :to="rendererEl">
      <NotificationList />
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import NotificationList from "./components/NotificationList.vue";
import useFullscreenRendererElement from "./composables/useFullscreenRendererElement";

const rendererEl = useFullscreenRendererElement();
</script>
