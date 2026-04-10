<template>
  <MonitorCenterWorkspace forced-tab="websocket" :hide-top-tabs="true" />
</template>

<script setup lang="ts">
import MonitorCenterWorkspace from "~/components/monitor/MonitorCenterWorkspace.vue";

definePageMeta({
  title: "监控中心 - WebSocket",
  layout: "default",
});
</script>

