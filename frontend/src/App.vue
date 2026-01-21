<template>
  <router-view />
  <Teleport :to="rendererEl">
    <NotificationList />
  </Teleport>
</template>

<script setup lang="ts">
import NotificationList from "./components/NotificationList.vue";
import useFullscreenRendererElement from "./composables/useFullscreenRendererElement";

const rendererEl = useFullscreenRendererElement();
</script>
