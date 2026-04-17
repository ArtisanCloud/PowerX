<template>
  <MonitorCenterWorkspace forced-tab="logs-trace" :hide-top-tabs="true" />
</template>

<script setup lang="ts">
import MonitorCenterWorkspace from "~/components/monitor/MonitorCenterWorkspace.vue";

definePageMeta({
  title: "监控中心 - Logs / Trace",
  layout: "default",
});
</script>

