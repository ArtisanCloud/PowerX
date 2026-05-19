<template>
  <MonitorCenterWorkspace forced-tab="task-cron" :hide-top-tabs="true" />
</template>

<script setup lang="ts">
import MonitorCenterWorkspace from "~/components/monitor/MonitorCenterWorkspace.vue";

definePageMeta({
  title: "监控中心 - 后台任务中心",
  layout: "default",
});
</script>
