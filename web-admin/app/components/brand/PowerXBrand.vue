<script setup lang="ts">
withDefaults(defineProps<{ compact?: boolean }>(), { compact: false });
</script>

<template>
  <img
    :src="compact ? '/images/powerx-mark.svg' : '/images/powerx-wordmark.svg'"
    :width="compact ? 24 : 100"
    :height="compact ? 24 : 32"
    class="powerx-brand brightness-0 dark:invert"
    :class="{ 'powerx-brand--compact': compact }"
    alt="PowerX"
    draggable="false"
    data-testid="powerx-brand"
  />
</template>

<style scoped>
.powerx-brand {
  display: block;
  flex: none;
  width: 100px;
  height: 32px;
}

.powerx-brand--compact {
  width: 24px;
  height: 24px;
}
</style>
