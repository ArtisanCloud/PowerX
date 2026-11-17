<script setup lang="ts">
defineProps<{ label: string; value: string; detail?: string }>();
</script>
<template>
  <UCard :ui="{ body: 'p-4 sm:p-4' }">
    <p class="text-sm text-muted">{{ label }}</p>
    <p class="mt-1 break-words text-2xl font-semibold tabular-nums">{{ value }}</p>
    <p v-if="detail" class="mt-1 break-words text-xs text-muted">{{ detail }}</p>
  </UCard>
</template>
