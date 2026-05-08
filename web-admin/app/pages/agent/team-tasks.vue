<script setup lang="ts">
import AgentWorkspace from "~/components/agent/AgentWorkspace.vue";

definePageMeta({
  title: "团队任务",
  layout: "default",
});
</script>

<template>
  <AgentWorkspace forced-workspace-mode="team" />
</template>
