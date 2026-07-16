<template>
  <UAlert
    :icon="icon"
    :color="color"
    variant="subtle"
    :title="title"
    :description="description"
  />
</template>

<script setup lang="ts">
import type { MetadataViewState } from "~/types/metadata-governance";

const props = defineProps<{
  state: MetadataViewState
  title: string
  description: string
}>();

const icon = computed(() => {
  switch (props.state) {
    case "loading":
      return "i-lucide-loader-circle";
    case "no_permission":
      return "i-lucide-lock";
    case "missing_selection":
      return "i-lucide-mouse-pointer-click";
    case "error":
      return "i-lucide-circle-alert";
    default:
      return "i-lucide-info";
  }
});

const color = computed(() => {
  switch (props.state) {
    case "error":
      return "error";
    case "no_permission":
      return "warning";
    default:
      return "neutral";
  }
});
</script>
