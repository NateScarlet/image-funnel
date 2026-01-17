<template>
  <div id="app" class="min-h-screen bg-slate-900 text-slate-100">
    <router-view />
    <NotificationList />
  </div>
</template>

<script setup lang="ts">
import NotificationList from "./components/NotificationList.vue";
</script>
