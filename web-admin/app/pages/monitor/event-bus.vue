<template>
  <MonitorCenterWorkspace forced-tab="event-fabric" :hide-top-tabs="true" />
</template>

<script setup lang="ts">
import MonitorCenterWorkspace from "~/components/monitor/MonitorCenterWorkspace.vue";

definePageMeta({
  title: "监控中心 - 事件总线",
  layout: "default",
});
</script>

