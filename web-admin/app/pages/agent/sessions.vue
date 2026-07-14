<script setup lang="ts">
import AgentWorkspace from "~/components/agent/AgentWorkspace.vue";

definePageMeta({
  title: "agent.sessions.title",
  layout: "default",
});
</script>

<template>
  <AgentWorkspace forced-workspace-mode="smart" />
</template>
