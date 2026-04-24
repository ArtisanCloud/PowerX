<script setup lang="ts">
import AgentWorkspace from "~/components/agent/AgentWorkspace.vue";

definePageMeta({
  title: "智能会话",
  layout: "default",
});
</script>

<template>
  <AgentWorkspace forced-workspace-mode="smart" />
</template>
